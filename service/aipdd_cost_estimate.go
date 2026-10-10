package service

import (
	"math"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/shopspring/decimal"
)

// AIPDDCostEstimate is the reverse-derived source cost of one transit order:
// the AIPDD published list price of what the request actually consumed, priced
// in AWCoin and converted with the exchange rate the upstream publishes beside
// its price list. It is an estimate, not the settlement AIPDD reports for a
// bound delivery site.
type AIPDDCostEstimate struct {
	AWCoin int64
	RMBMic int64
}

// AIPDDCostBasis carries the consumption evidence of one order. Only the fields
// relevant to the model's billing basis are read; everything else may stay zero.
type AIPDDCostBasis struct {
	ModelName    string
	Usage        *dto.Usage
	ChargedQuota int64
	// BillingUnit is the unit the upstream charges by ("call", "image", "second").
	// An empty unit falls back to the catalog capability's own unit.
	BillingUnit string
	// Quantity is the billed number of units. When it is unknown the estimator
	// reverse-derives it from the customer charge and the local retail unit price.
	Quantity     float64
	UnitPriceUSD float64
	GroupRatio   float64
	Resolution   string
}

// appliesAIPDDCostEstimate reports whether a settlement carries enough charge
// evidence to reverse-derive a source cost. Only a successful charge does.
func appliesAIPDDCostEstimate(status string, actualQuota int) bool {
	return status == "CHARGED" && actualQuota > 0
}

// EstimateAIPDDSourceCost returns the published-price cost of one order, or false
// when this deployment has no AIPDD price basis yet (no catalog applied, or the
// model is not in it). A missing estimate must never block settlement.
func EstimateAIPDDSourceCost(basis AIPDDCostBasis) (AIPDDCostEstimate, bool) {
	rate := constant.GetAIPDDRMBPerAWCoin()
	if !(rate > 0) || math.IsNaN(rate) || math.IsInf(rate, 0) {
		return AIPDDCostEstimate{}, false
	}
	awCoin, ok := estimateAIPDDSourceAWCoin(basis)
	if !ok || !(awCoin > 0) || math.IsNaN(awCoin) || math.IsInf(awCoin, 0) {
		return AIPDDCostEstimate{}, false
	}
	rmbMic := decimal.NewFromFloat(awCoin).
		Mul(decimal.NewFromFloat(rate)).
		Mul(decimal.NewFromInt(1_000_000)).Round(0).IntPart()
	return AIPDDCostEstimate{
		AWCoin: int64(math.Round(awCoin)),
		RMBMic: rmbMic,
	}, true
}

func estimateAIPDDSourceAWCoin(basis AIPDDCostBasis) (float64, bool) {
	if price, ok := constant.GetAIPDDTokenPrice(basis.ModelName); ok && basis.Usage != nil {
		return tokenAWCoinCost(price, basis.Usage)
	}
	capability, ok := constant.GetAIPDDCapability(basis.ModelName)
	if !ok {
		return 0, false
	}
	unitAWCoin, ok := unitPriceAWCoin(capability, basis)
	if !ok {
		return 0, false
	}
	quantity, ok := billedQuantity(basis)
	if !ok {
		return 0, false
	}
	return unitAWCoin * quantity, true
}

// tokenAWCoinCost prices the reported usage against the four published token
// tiers. Cached input is billed at the cache-read tier, so it is subtracted
// from the prompt tier instead of being charged twice.
func tokenAWCoinCost(price constant.AIPDDModelTokenPrice, usage *dto.Usage) (float64, bool) {
	prompt := int64(usage.PromptTokens)
	cached := int64(usage.PromptTokensDetails.CachedTokens)
	completion := int64(usage.CompletionTokens)
	if prompt < 0 || cached < 0 || completion < 0 || cached > prompt {
		return 0, false
	}
	cost := float64(prompt-cached)*price.PromptPerMillion +
		float64(completion)*price.CompletionPerMillion +
		float64(cached)*price.CacheReadPerMillion
	if !(cost >= 0) {
		return 0, false
	}
	return cost / 1_000_000, true
}

func unitPriceAWCoin(capability constant.AIPDDCapability, basis AIPDDCostBasis) (float64, bool) {
	unit := basis.BillingUnit
	if unit == "" {
		if capability.BillingType == constant.AIPDDBillingTypeDurationSeconds {
			unit = "second"
		} else {
			unit = "call"
		}
	}
	if unit == "second" {
		if perSecond, ok := durationPriceAWCoin(capability, basis.Resolution); ok {
			return perSecond, true
		}
	}
	if capability.TaskCost > 0 && !math.IsNaN(capability.TaskCost) {
		return capability.TaskCost, true
	}
	return 0, false
}

// durationPriceAWCoin resolves the published per-second price. Resolution is
// only known for the models that report it, so the cheapest published tier is
// used as the floor when it is not.
func durationPriceAWCoin(capability constant.AIPDDCapability, resolution string) (float64, bool) {
	if capability.SeedancePricing != nil && len(capability.SeedancePricing.ByResolution) > 0 {
		if tier, ok := capability.SeedancePricing.ByResolution[resolution]; ok &&
			tier.AmountAWCoinPerSecond > 0 {
			return tier.AmountAWCoinPerSecond, true
		}
		var cheapest float64
		for _, tier := range capability.SeedancePricing.ByResolution {
			if tier.AmountAWCoinPerSecond > 0 && (cheapest == 0 || tier.AmountAWCoinPerSecond < cheapest) {
				cheapest = tier.AmountAWCoinPerSecond
			}
		}
		if cheapest > 0 {
			return cheapest, true
		}
	}
	if capability.BillingType == constant.AIPDDBillingTypeDurationSeconds && capability.TaskCost > 0 {
		return capability.TaskCost, true
	}
	return 0, false
}

// billedQuantity prefers the metered quantity frozen at request time. Without
// it, the quantity is recovered from the customer charge: quota is the product
// of quantity, local retail unit price and group ratio, so dividing those back
// out leaves the same quantity the upstream was paid for.
func billedQuantity(basis AIPDDCostBasis) (float64, bool) {
	if basis.Quantity > 0 && !math.IsNaN(basis.Quantity) {
		return basis.Quantity, true
	}
	if basis.ChargedQuota <= 0 {
		return 1, true
	}
	if basis.UnitPriceUSD <= 0 || basis.GroupRatio <= 0 || common.QuotaPerUnit <= 0 ||
		math.IsNaN(basis.UnitPriceUSD) || math.IsNaN(basis.GroupRatio) {
		return 0, false
	}
	quantity := float64(basis.ChargedQuota) / (basis.UnitPriceUSD * common.QuotaPerUnit * basis.GroupRatio)
	if !(quantity > 0) || math.IsNaN(quantity) || math.IsInf(quantity, 0) {
		return 0, false
	}
	return quantity, true
}

// aipddRelayCostBasis reads the consumption evidence a synchronous request
// leaves behind: token usage for per-token models, the frozen task-pricing
// quote or per-call price for everything else.
func aipddRelayCostBasis(info *relaycommon.RelayInfo, chargedQuota int) AIPDDCostBasis {
	// The published price list is keyed by AIPDD's own model id, so the upstream
	// name wins whenever the deployment maps models.
	modelName := strings.TrimSpace(info.OriginModelName)
	if info.ChannelMeta != nil {
		if upstream := strings.TrimSpace(info.UpstreamModelName); upstream != "" {
			modelName = upstream
		}
	}
	basis := AIPDDCostBasis{
		ModelName:    modelName,
		Usage:        info.SettlementUsage,
		ChargedQuota: int64(max(0, chargedQuota)),
		GroupRatio:   info.PriceData.GroupRatioInfo.GroupRatio,
	}
	if quote := info.TaskPricingQuote; quote != nil {
		basis.BillingUnit = quote.Unit
		basis.Quantity = quote.Quantity
		basis.UnitPriceUSD = quote.UnitPriceUSD
		basis.Resolution = quote.Resolution
		if quote.GroupRatio > 0 {
			basis.GroupRatio = quote.GroupRatio
		}
		return basis
	}
	if info.PriceData.UsePrice {
		// The billed unit count (images per request, seconds of ASR) is already
		// inside the settled quota, so it is recovered by dividing the local retail
		// unit price back out instead of assuming one unit.
		basis.UnitPriceUSD = info.PriceData.ModelPrice
	}
	return basis
}

// aipddTaskCostBasis rebuilds the same evidence for an asynchronous task from
// the billing snapshot persisted at submission time.
func aipddTaskCostBasis(task *model.Task, chargedQuota int) AIPDDCostBasis {
	billing := task.PrivateData.BillingContext
	if billing == nil || billing.OriginModelName == "" {
		return AIPDDCostBasis{}
	}
	quota := int64(max(0, chargedQuota))
	if quota == 0 {
		quota = int64(max(0, task.Quota))
	}
	unitPrice := billing.UnitPriceUSD
	if unitPrice <= 0 {
		unitPrice = billing.ModelPrice
	}
	modelName := strings.TrimSpace(task.Properties.UpstreamModelName)
	if modelName == "" {
		modelName = billing.OriginModelName
	}
	return AIPDDCostBasis{
		ModelName:    modelName,
		ChargedQuota: quota,
		BillingUnit:  billing.BillingUnit,
		Quantity:     billing.Quantity,
		UnitPriceUSD: unitPrice,
		GroupRatio:   billing.GroupRatio,
		Resolution:   billing.Resolution,
	}
}
