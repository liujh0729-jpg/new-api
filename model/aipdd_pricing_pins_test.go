package model

import (
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

// A pinned model is the one case where an administrator's price must outrank the
// catalog: neither the retail expression nor the billing mode may be rewritten,
// and the catalog-owned rows must not be reclaimed either.
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

	// Unpinning hands the model back to the catalog on the next sync.
	require.NoError(t, SetAIPDDPinnedModel("ap-glm-5.2", false))
	updated, _, err = syncAIPDDLLMTokenPricingTx(DB, aipddRetailLLMCatalog("revision-2"))
	require.NoError(t, err)
	require.Equal(t, 1, updated)
	exprs, err = loadAIPDDBillingExprsTx(DB)
	require.NoError(t, err)
	require.Equal(t, aipddRetailGLMExpr, exprs["ap-glm-5.2"])
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

	// Unpinning restores the catalog-derived retail price.
	require.NoError(t, SetAIPDDPinnedModel("ap-ltx-2.5", false))
	updated, _, err = syncAIPDDAsyncPerSecondPricingTx(DB, aipddASRCatalog("revision-2"))
	require.NoError(t, err)
	require.Equal(t, 1, updated)
	pricing, err = loadAIPDDTaskPricingTx(DB)
	require.NoError(t, err)
	require.InDelta(t, 0.033, pricing["ap-ltx-2.5"].NoReferenceVideoUnitPrice, 1e-9)
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
