package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/aipddcatalog"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm/clause"
)

const (
	aipddRetailGLMExpr   = `tier("aipdd", p * 0.8800000000000001 + c * 3.0799999999999996 + cr * 0.011 + cc * 0.132 + cc1h * 0.132)`
	aipddRetailEmbedExpr = `tier("aipdd", p * 0.033 + c * 0 + cr * 0 + cc * 0 + cc1h * 0)`
)

// aipddRetailLLMCatalog mirrors the shapes the live catalog publishes: a chat
// model with cache pricing, an input-only embedding model, a model upstream marks
// unavailable but that the managed channel still lists, and a promotional model
// that costs nothing wholesale.
func aipddRetailLLMCatalog(revision string) aipddcatalog.AtomicCatalog {
	return aipddcatalog.AtomicCatalog{
		SchemaVersion: 2,
		Revision:      revision,
		AWCoinRate: aipddcatalog.AtomicAWCoinRate{
			RMBPerAWCoin: 0.073, USDPerAWCoin: 0.0015, UpdatedAt: "2026-10-01T00:00:00",
		},
		Models: []aipddcatalog.AtomicModel{
			{
				ID: "ap-glm-5.2", Name: "AP GLM 5.2", Available: true,
				Execution: aipddcatalog.AtomicExecution{Protocol: "openai", Path: "/v1/chat/completions"},
				Pricing: aipddcatalog.AtomicPricing{
					PricingModel: "per_token", Currency: "awcoin", Enabled: true,
					PromptPerMillion: 80, CompletionPerMillion: 280,
					CacheReadPerMillion: aipddRetailFloatPtr(1), CacheWritePerMillion: aipddRetailFloatPtr(12),
				},
			},
			{
				ID: "ap-qwen3-embedding:0.6b", Name: "AP Qwen3 Embedding", Available: true,
				Execution: aipddcatalog.AtomicExecution{Protocol: "openai", Path: "/v1/embeddings"},
				Pricing: aipddcatalog.AtomicPricing{
					PricingModel: "per_token", Currency: "awcoin", Enabled: true,
					PromptPerMillion:    3,
					CacheReadPerMillion: aipddRetailFloatPtr(0), CacheWritePerMillion: aipddRetailFloatPtr(0),
				},
			},
			{
				ID: "ap-retired", Name: "AP Retired",
				Execution: aipddcatalog.AtomicExecution{Protocol: "openai", Path: "/v1/chat/completions"},
				Pricing: aipddcatalog.AtomicPricing{
					PricingModel: "per_token", Currency: "awcoin", Enabled: true,
					PromptPerMillion: 80, CompletionPerMillion: 280,
					CacheReadPerMillion: aipddRetailFloatPtr(1), CacheWritePerMillion: aipddRetailFloatPtr(12),
				},
			},
			{
				ID: "ap-free-trial", Name: "AP Free Trial", Available: true,
				Execution: aipddcatalog.AtomicExecution{Protocol: "openai", Path: "/v1/chat/completions"},
				Pricing: aipddcatalog.AtomicPricing{
					PricingModel: "per_token", Currency: "awcoin", Enabled: true, Free: true,
					CacheReadPerMillion: aipddRetailFloatPtr(0), CacheWritePerMillion: aipddRetailFloatPtr(0),
				},
			},
		},
	}
}

func seedAIPDDBillingOptions(t *testing.T, modes map[string]string, exprs map[string]string) {
	t.Helper()
	modesJSON, err := common.Marshal(modes)
	require.NoError(t, err)
	exprsJSON, err := common.Marshal(exprs)
	require.NoError(t, err)
	for key, value := range map[string]string{
		aipddBillingModeOptionKey: string(modesJSON),
		aipddBillingExprOptionKey: string(exprsJSON),
	} {
		require.NoError(t, DB.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "key"}},
			DoUpdates: clause.AssignmentColumns([]string{"value"}),
		}).Create(&Option{Key: key, Value: value}).Error)
	}
}

func TestSyncAIPDDLLMTokenPricingTxProvisionsRetailPrices(t *testing.T) {
	truncateTables(t)
	preserveAIPDDPricingRuntime(t)
	t.Setenv("AIPDD_PRICING_MARKUP", "1.1")
	originalRate := operation_setting.USDExchangeRate
	operation_setting.USDExchangeRate = 7.3
	t.Cleanup(func() { operation_setting.USDExchangeRate = originalRate })

	seedAIPDDBillingOptions(t,
		map[string]string{"legacy-model": "ratio"},
		map[string]string{"legacy-model": `tier("legacy", p * 1)`})

	updated, updates, err := syncAIPDDLLMTokenPricingTx(DB, aipddRetailLLMCatalog("revision-1"))
	require.NoError(t, err)
	require.Equal(t, 3, updated)
	require.Len(t, updates, 2)
	require.Contains(t, updates, aipddBillingModeOptionKey)
	require.Contains(t, updates, aipddBillingExprOptionKey)

	modes, err := loadAIPDDBillingModesTx(DB)
	require.NoError(t, err)
	require.Equal(t, map[string]string{
		"ap-glm-5.2":              billing_setting.BillingModeTieredExpr,
		"ap-qwen3-embedding:0.6b": billing_setting.BillingModeTieredExpr,
		"ap-retired":              billing_setting.BillingModeTieredExpr,
		"legacy-model":            "ratio",
	}, modes)

	exprs, err := loadAIPDDBillingExprsTx(DB)
	require.NoError(t, err)
	require.Equal(t, map[string]string{
		"ap-glm-5.2":              aipddRetailGLMExpr,
		"ap-qwen3-embedding:0.6b": aipddRetailEmbedExpr,
		"ap-retired":              aipddRetailGLMExpr,
		"legacy-model":            `tier("legacy", p * 1)`,
	}, exprs)

	// Re-applying an unchanged catalog must be a no-op.
	updated, updates, err = syncAIPDDLLMTokenPricingTx(DB, aipddRetailLLMCatalog("revision-2"))
	require.NoError(t, err)
	require.Zero(t, updated)
	require.Nil(t, updates)

	// A wholesale change is detected and re-priced.
	changed := aipddRetailLLMCatalog("revision-3")
	changed.Models[0].Pricing.PromptPerMillion = 160
	updated, _, err = syncAIPDDLLMTokenPricingTx(DB, changed)
	require.NoError(t, err)
	require.Equal(t, 1, updated)
	exprs, err = loadAIPDDBillingExprsTx(DB)
	require.NoError(t, err)
	require.Equal(t,
		`tier("aipdd", p * 1.7600000000000002 + c * 3.0799999999999996 + cr * 0.011 + cc * 0.132 + cc1h * 0.132)`,
		exprs["ap-glm-5.2"])
}

func TestSyncAIPDDLLMTokenPricingTxReclaimsPricingOfUnsellableModels(t *testing.T) {
	truncateTables(t)
	preserveAIPDDPricingRuntime(t)
	t.Setenv("AIPDD_PRICING_MARKUP", "1.1")
	originalRate := operation_setting.USDExchangeRate
	operation_setting.USDExchangeRate = 7.3
	t.Cleanup(func() { operation_setting.USDExchangeRate = originalRate })

	_, _, err := syncAIPDDLLMTokenPricingTx(DB, aipddRetailLLMCatalog("revision-1"))
	require.NoError(t, err)

	seedAIPDDBillingOptions(t,
		map[string]string{
			"ap-glm-5.2":              billing_setting.BillingModeTieredExpr,
			"ap-qwen3-embedding:0.6b": billing_setting.BillingModeTieredExpr,
			"ap-retired":              billing_setting.BillingModeTieredExpr,
			"manual-model":            "ratio",
		},
		map[string]string{
			"ap-glm-5.2":              aipddRetailGLMExpr,
			"ap-qwen3-embedding:0.6b": aipddRetailEmbedExpr,
			"ap-retired":              aipddRetailGLMExpr,
			"manual-model":            `tier("manual", p * 2)`,
		})

	// The marketplace withdrew ap-retired's display price: its catalog-owned
	// expression and tiered_expr mode go away, while a mode an administrator chose
	// on purpose survives so their own pricing is not silently deleted.
	withdrawn := aipddRetailLLMCatalog("revision-2")
	withdrawn.Models[2].Pricing.Enabled = false
	updated, _, err := syncAIPDDLLMTokenPricingTx(DB, withdrawn)
	require.NoError(t, err)
	require.Equal(t, 1, updated)

	modes, err := loadAIPDDBillingModesTx(DB)
	require.NoError(t, err)
	require.NotContains(t, modes, "ap-retired")
	require.Equal(t, "ratio", modes["manual-model"])

	exprs, err := loadAIPDDBillingExprsTx(DB)
	require.NoError(t, err)
	require.NotContains(t, exprs, "ap-retired")
	require.Equal(t, `tier("manual", p * 2)`, exprs["manual-model"])
	require.Equal(t, aipddRetailGLMExpr, exprs["ap-glm-5.2"], "the models the catalog still sells stay priced")
}

func TestSyncAIPDDLLMTokenPricingTxRequiresUsableAWCoinRate(t *testing.T) {
	truncateTables(t)
	preserveAIPDDPricingRuntime(t)
	t.Setenv("AIPDD_PRICING_MARKUP", "1.1")
	originalRate := operation_setting.USDExchangeRate
	operation_setting.USDExchangeRate = 7.3
	t.Cleanup(func() { operation_setting.USDExchangeRate = originalRate })

	catalog := aipddRetailLLMCatalog("revision-1")
	catalog.AWCoinRate.RMBPerAWCoin = 0

	// Without a usable awcoin rate a wholesale price cannot become a retail price,
	// and importing nothing is safer than provisioning free chat completions.
	updated, updates, err := syncAIPDDLLMTokenPricingTx(DB, catalog)
	require.Error(t, err)
	require.Zero(t, updated)
	require.Nil(t, updates)

	modes, err := loadAIPDDBillingModesTx(DB)
	require.NoError(t, err)
	require.Empty(t, modes)
}

func TestSyncAIPDDLLMTokenPricingTxSkipsUnusableModelPrice(t *testing.T) {
	truncateTables(t)
	preserveAIPDDPricingRuntime(t)
	t.Setenv("AIPDD_PRICING_MARKUP", "1.1")
	originalRate := operation_setting.USDExchangeRate
	operation_setting.USDExchangeRate = 7.3
	t.Cleanup(func() { operation_setting.USDExchangeRate = originalRate })

	// A single model with a zero prompt price cannot become a retail expression,
	// but it must not leave every other model of the catalog unpriced either.
	catalog := aipddRetailLLMCatalog("revision-1")
	catalog.Models[0].Pricing.PromptPerMillion = 0

	updated, _, err := syncAIPDDLLMTokenPricingTx(DB, catalog)
	require.NoError(t, err)
	require.Equal(t, 2, updated)

	modes, err := loadAIPDDBillingModesTx(DB)
	require.NoError(t, err)
	require.NotContains(t, modes, "ap-glm-5.2", "an unusable price is skipped, not provisioned")
	require.Equal(t, billing_setting.BillingModeTieredExpr, modes["ap-qwen3-embedding:0.6b"])

	exprs, err := loadAIPDDBillingExprsTx(DB)
	require.NoError(t, err)
	require.NotContains(t, exprs, "ap-glm-5.2")
	require.Equal(t, aipddRetailEmbedExpr, exprs["ap-qwen3-embedding:0.6b"], "the healthy model still gets its retail price")
}

func TestSyncAIPDDLLMTokenPricingTxRespectsLowerMarkup(t *testing.T) {
	truncateTables(t)
	preserveAIPDDPricingRuntime(t)
	t.Setenv("AIPDD_PRICING_MARKUP", "1")
	originalRate := operation_setting.USDExchangeRate
	operation_setting.USDExchangeRate = 7.3
	t.Cleanup(func() { operation_setting.USDExchangeRate = originalRate })

	updated, _, err := syncAIPDDLLMTokenPricingTx(DB, aipddRetailLLMCatalog("revision-1"))
	require.NoError(t, err)
	require.Equal(t, 3, updated)

	exprs, err := loadAIPDDBillingExprsTx(DB)
	require.NoError(t, err)
	// Markup 1 means exact parity with AIPDD: the coefficients drop the 10% and
	// the rest of the expression shape is unchanged.
	require.Equal(t, `tier("aipdd", p * 0.8 + c * 2.8 + cr * 0.01 + cc * 0.11999999999999998 + cc1h * 0.11999999999999998)`, exprs["ap-glm-5.2"])
}
