package model

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm/clause"
)

func TestParseAIPDDPinnedModels(t *testing.T) {
	pins, err := parseAIPDDPinnedModels("")
	require.NoError(t, err)
	require.Empty(t, pins)

	pins, err = parseAIPDDPinnedModels(`[" ap-glm-5.2 ","","ap-ltx-2.5"]`)
	require.NoError(t, err)
	require.Equal(t, aipddPinnedModelSet{"ap-glm-5.2": true, "ap-ltx-2.5": true}, pins)
	require.True(t, pins.contains(" ap-glm-5.2 "))
	require.False(t, pins.contains("ap-qwen3-embedding:0.6b"))

	_, err = parseAIPDDPinnedModels(`{"ap-glm-5.2":true}`)
	require.Error(t, err)

	// A corrupt value would abort every catalog sync, so the option writer rejects it.
	require.NoError(t, validateOptionValue(aipddPinnedModelsOptionKey, "[]"))
	require.NoError(t, validateOptionValue(aipddPinnedModelsOptionKey, `["ap-glm-5.2"]`))
	require.Error(t, validateOptionValue(aipddPinnedModelsOptionKey, `{"ap-glm-5.2":true}`))
}

func TestSetAIPDDPinnedModelRoundTrip(t *testing.T) {
	truncateTables(t)
	t.Cleanup(func() { delete(common.OptionMap, aipddPinnedModelsOptionKey) })

	require.NoError(t, SetAIPDDPinnedModel("ap-glm-5.2", true))
	require.NoError(t, SetAIPDDPinnedModel("ap-ltx-2.5", true))

	names, err := GetAIPDDPinnedModels()
	require.NoError(t, err)
	require.Equal(t, []string{"ap-glm-5.2", "ap-ltx-2.5"}, names)

	require.NoError(t, SetAIPDDPinnedModel("ap-glm-5.2", false))
	names, err = GetAIPDDPinnedModels()
	require.NoError(t, err)
	require.Equal(t, []string{"ap-ltx-2.5"}, names)

	require.Error(t, SetAIPDDPinnedModel("  ", true))
}

func aipddPinnedExchangeRate(t *testing.T, rate float64) {
	t.Helper()
	original := operation_setting.USDExchangeRate
	operation_setting.USDExchangeRate = rate
	t.Cleanup(func() { operation_setting.USDExchangeRate = original })
}

// loadAIPDDBillingRuntimeFromDB activates the billing rows that are in the test
// database through the same path a saved option takes at runtime, so a test can
// assert what the gateway would bill rather than only what a syncer wrote.
func loadAIPDDBillingRuntimeFromDB(t *testing.T) {
	t.Helper()
	for _, key := range []string{aipddBillingModeOptionKey, aipddBillingExprOptionKey} {
		value, err := loadAIPDDOptionValueTx(DB, key)
		require.NoError(t, err)
		if strings.TrimSpace(value) == "" {
			continue
		}
		require.NoError(t, updateOptionMap(key, value))
	}
}

// A pinned model is the case where an administrator's price must outrank the
// catalog even when it still equals the catalog value: neither the retail
// expression nor the billing mode may be rewritten, and the catalog-owned rows
// must not be reclaimed either.
func TestSyncAIPDDLLMTokenPricingTxSkipsPinnedModels(t *testing.T) {
	truncateTables(t)
	preserveAIPDDPricingRuntime(t)
	aipddPinnedExchangeRate(t, 7.3)
	t.Setenv("AIPDD_PRICING_MARKUP", "1.1")

	require.NoError(t, SetAIPDDPinnedModel("ap-glm-5.2", true))
	require.NoError(t, SetAIPDDPinnedModel("ap-retired", true))
	seedAIPDDBillingOptions(t,
		map[string]string{
			"ap-glm-5.2": billing_setting.BillingModeTieredExpr,
			"ap-retired": billing_setting.BillingModeTieredExpr,
		},
		map[string]string{
			"ap-glm-5.2": `tier("manual", p * 1 + c * 2)`,
			"ap-retired": `tier("manual", p * 3 + c * 4)`,
		})

	updated, _, err := syncAIPDDLLMTokenPricingTx(DB, aipddRetailLLMCatalog("revision-1"))
	require.NoError(t, err)
	// Only the unpinned, unpriced embedding model is provisioned.
	require.Equal(t, 1, updated)

	exprs, err := loadAIPDDBillingExprsTx(DB)
	require.NoError(t, err)
	require.Equal(t, `tier("manual", p * 1 + c * 2)`, exprs["ap-glm-5.2"], "a pinned price is never overwritten")
	require.Equal(t, `tier("manual", p * 3 + c * 4)`, exprs["ap-retired"], "a pinned price is never reclaimed")
	require.Equal(t, aipddRetailEmbedExpr, exprs["ap-qwen3-embedding:0.6b"], "unpinned models still get catalog pricing")

	// Activate the stored rows through the production path and read the runtime
	// view, because that is the expression the relay actually bills. Without it,
	// a pin that only holds in storage would pass every check above.
	loadAIPDDBillingRuntimeFromDB(t)
	runtimeExpr, ok := billing_setting.GetBillingExpr("ap-glm-5.2")
	require.True(t, ok)
	require.Equal(t, `tier("manual", p * 1 + c * 2)`, runtimeExpr, "the pinned price is the price the gateway charges")
	require.Equal(t, aipddRetailEmbedExpr, mustAIPDDBillingExpr(t, "ap-qwen3-embedding:0.6b"))

	// Unpinning does not overwrite a hand-set price either — an edited price wins,
	// which is what AIPDDSyncedPrices records. The first pass adopts the stored
	// expression as the baseline; the next catalog change reaches the model.
	require.NoError(t, SetAIPDDPinnedModel("ap-glm-5.2", false))
	updated, _, err = syncAIPDDLLMTokenPricingTx(DB, aipddRetailLLMCatalog("revision-2"))
	require.NoError(t, err)
	require.Zero(t, updated, "an unowned price is adopted, not overwritten")

	changed := aipddRetailLLMCatalog("revision-3")
	changed.Models[0].Pricing.PromptPerMillion = 160
	updated, _, err = syncAIPDDLLMTokenPricingTx(DB, changed)
	require.NoError(t, err)
	require.Equal(t, 1, updated)
	exprs, err = loadAIPDDBillingExprsTx(DB)
	require.NoError(t, err)
	require.Equal(t,
		`tier("aipdd", p * 1.76 + c * 3.08 + cr * 0.011 + cc * 0.132 + cc1h * 0.132)`,
		exprs["ap-glm-5.2"])
}

func TestSyncAIPDDAsyncPerSecondPricingTxSkipsPinnedModels(t *testing.T) {
	truncateTables(t)
	preserveAIPDDPricingRuntime(t)
	aipddPinnedExchangeRate(t, 2)
	t.Setenv("AIPDD_PRICING_MARKUP", "1.1")

	require.NoError(t, SetAIPDDPinnedModel("ap-ltx-2.5", true))
	seedAIPDDTaskPricing(t, "ap-ltx-2.5", billing_setting.TaskPricingConfig{
		Unit:                      billing_setting.TaskPricingUnitSecond,
		NoReferenceVideoUnitPrice: 0.5,
		ReferenceVideoPolicy:      billing_setting.ReferenceVideoPolicySame,
	})

	updated, _, err := syncAIPDDAsyncPerSecondPricingTx(DB, aipddASRCatalog("revision-1"))
	require.NoError(t, err)
	require.Zero(t, updated, "the only async per-second model in the fixture is pinned")

	pricing, err := loadAIPDDTaskPricingTx(DB)
	require.NoError(t, err)
	require.InDelta(t, 0.5, pricing["ap-ltx-2.5"].NoReferenceVideoUnitPrice, 1e-12)

	// Unpinning adopts the hand-set price as the baseline instead of overwriting
	// it; the catalog takes over from the next price change onward.
	require.NoError(t, SetAIPDDPinnedModel("ap-ltx-2.5", false))
	updated, _, err = syncAIPDDAsyncPerSecondPricingTx(DB, aipddASRCatalog("revision-2"))
	require.NoError(t, err)
	require.Zero(t, updated)
	pricing, err = loadAIPDDTaskPricingTx(DB)
	require.NoError(t, err)
	require.InDelta(t, 0.5, pricing["ap-ltx-2.5"].NoReferenceVideoUnitPrice, 1e-12)

	changed := aipddASRCatalog("revision-3")
	changed.Capabilities[1].Pricing.ChargeConfig["amountAwcoin"] = float64(720)
	updated, _, err = syncAIPDDAsyncPerSecondPricingTx(DB, changed)
	require.NoError(t, err)
	require.Equal(t, 1, updated)
	pricing, err = loadAIPDDTaskPricingTx(DB)
	require.NoError(t, err)
	require.InDelta(t, 0.0396, pricing["ap-ltx-2.5"].NoReferenceVideoUnitPrice, 1e-9)
}

func seedAIPDDTaskPricing(t *testing.T, modelName string, config billing_setting.TaskPricingConfig) {
	t.Helper()
	value, err := common.Marshal(map[string]billing_setting.TaskPricingConfig{modelName: config})
	require.NoError(t, err)
	require.NoError(t, DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{"value"}),
	}).Create(&Option{Key: aipddTaskPricingOptionKey, Value: string(value)}).Error)
}
