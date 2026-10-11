package model

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/aipddcatalog"
	"github.com/QuantumNous/new-api/setting/operation_setting"
)

const (
	aipddPricingMarkupEnv      = "AIPDD_PRICING_MARKUP"
	defaultAIPDDPricingMarkup  = 1.10
	aipddRetailExprTierName    = "aipdd"
	aipddPerTokenPricingModel  = "per_token"
	aipddAWCoinPricingCurrency = "awcoin"
	aipddBillingExprOptionKey  = "billing_setting.billing_expr"
)

// AIPDDRetailMarkup is the multiplier between the price AIPDD publishes for a
// model and the default retail price this gateway provisions for it. Without it
// the site resells every catalog model at cost and earns nothing.
//
// A markup below 1 is accepted on purpose: selling at a loss is an operator
// decision, and silently correcting it would hide the price actually charged.
func AIPDDRetailMarkup() float64 {
	raw := strings.TrimSpace(common.GetEnvOrDefaultString(aipddPricingMarkupEnv, ""))
	if raw == "" {
		return defaultAIPDDPricingMarkup
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil || math.IsNaN(value) || math.IsInf(value, 0) || value <= 0 {
		common.SysLog(fmt.Sprintf("invalid %s=%q, using default AIPDD pricing markup %g",
			aipddPricingMarkupEnv, raw, defaultAIPDDPricingMarkup))
		return defaultAIPDDPricingMarkup
	}
	return value
}

// aipddRetailUSD converts an AIPDD AWCoin price into this instance's retail USD
// price. The catalog's reference USD rate is deliberately unused: AIPDD prices
// are RMB-denominated, so dividing the marked-up RMB price by the local exchange
// rate keeps the public CNY price exact instead of drifting with upstream.
func aipddRetailUSD(awcoinPrice, rmbPerAWCoin, rmbPerUSD float64) float64 {
	return awcoinPrice * rmbPerAWCoin * AIPDDRetailMarkup() / rmbPerUSD
}

// AIPDDLLMRetailExpr builds the default billing expression for a per-token AIPDD
// model. Coefficients are USD per million tokens per token class, so cache reads
// and cache writes keep their own published price instead of collapsing into the
// prompt rate the way a plain model ratio would.
func AIPDDLLMRetailExpr(catalog aipddcatalog.AtomicCatalog, model aipddcatalog.AtomicModel) (string, error) {
	rmbPerAWCoin := catalog.AWCoinRate.RMBPerAWCoin
	rmbPerUSD := operation_setting.USDExchangeRate
	if !finitePositive(rmbPerAWCoin) {
		return "", fmt.Errorf("AIPDD catalog rmbPerAwcoin must be finite and greater than 0")
	}
	if !finitePositive(rmbPerUSD) {
		return "", fmt.Errorf("New API USDExchangeRate must be finite and greater than 0")
	}
	promptUSD := aipddRetailUSD(model.Pricing.PromptPerMillion, rmbPerAWCoin, rmbPerUSD)
	completionUSD := aipddRetailUSD(model.Pricing.CompletionPerMillion, rmbPerAWCoin, rmbPerUSD)
	if !finitePositive(promptUSD) || math.IsNaN(completionUSD) || math.IsInf(completionUSD, 0) || completionUSD < 0 {
		return "", fmt.Errorf("model %q per-token prices are not usable", model.ID)
	}
	if model.Pricing.CacheReadPerMillion == nil || model.Pricing.CacheWritePerMillion == nil {
		// A schema v1 catalog publishes no cache prices. Leaving the cache
		// variables out keeps their documented semantics: cached tokens stay in
		// p and are billed at the ordinary prompt price.
		return fmt.Sprintf("tier(%q, p * %s + c * %s)",
			aipddRetailExprTierName, formatRetailPrice(promptUSD), formatRetailPrice(completionUSD)), nil
	}
	cacheReadUSD := aipddRetailUSD(*model.Pricing.CacheReadPerMillion, rmbPerAWCoin, rmbPerUSD)
	cacheWriteUSD := aipddRetailUSD(*model.Pricing.CacheWritePerMillion, rmbPerAWCoin, rmbPerUSD)
	if cacheReadUSD < 0 || cacheWriteUSD < 0 || math.IsNaN(cacheReadUSD) || math.IsNaN(cacheWriteUSD) ||
		math.IsInf(cacheReadUSD, 0) || math.IsInf(cacheWriteUSD, 0) {
		return "", fmt.Errorf("model %q cache prices are not usable", model.ID)
	}
	return fmt.Sprintf("tier(%q, p * %s + c * %s + cr * %s + cc * %s + cc1h * %s)",
		aipddRetailExprTierName,
		formatRetailPrice(promptUSD), formatRetailPrice(completionUSD),
		formatRetailPrice(cacheReadUSD), formatRetailPrice(cacheWriteUSD), formatRetailPrice(cacheWriteUSD)), nil
}

func formatRetailPrice(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}

// aipddPricingSkips remembers which models were already reported for which
// catalog revision, so one unpublishable price is logged once per revision
// instead of once per sync.
var aipddPricingSkips = struct {
	mu       sync.Mutex
	revision map[string]string
}{revision: make(map[string]string)}

// logAIPDDPricingSkipped reports a per-model price problem and lets the caller
// continue. One broken upstream price must not abort the catalog transaction:
// that would leave every AIPDD model unpriced — and therefore unsellable — until
// someone fixes the upstream entry, while the gateway itself keeps serving.
func logAIPDDPricingSkipped(revision, modelName string, err error) {
	aipddPricingSkips.mu.Lock()
	defer aipddPricingSkips.mu.Unlock()
	if aipddPricingSkips.revision[modelName] == revision {
		return
	}
	aipddPricingSkips.revision[modelName] = revision
	common.SysError(fmt.Sprintf("AIPDD catalog revision %q: skipped retail price for %q: %v", revision, modelName, err))
}

// IsAIPDDRetailTokenModel reports whether a catalog model carries a per-token
// wholesale price that must be resold at a margin. Upstream's availability flag
// is deliberately ignored: an unavailable model is still listed on the managed
// channel and stays routable, and a routable model with no retail price falls
// back to the default ratio, which is below AIPDD's wholesale cost.
func IsAIPDDRetailTokenModel(model aipddcatalog.AtomicModel) bool {
	return model.Pricing.Enabled &&
		!model.Pricing.Free &&
		strings.EqualFold(strings.TrimSpace(model.Pricing.PricingModel), aipddPerTokenPricingModel) &&
		strings.EqualFold(strings.TrimSpace(model.Pricing.Currency), aipddAWCoinPricingCurrency)
}
