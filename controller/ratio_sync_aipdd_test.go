package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/pkg/aipddcatalog"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/require"
)

// The retail margin and the exchange rate are both inputs to the previewed
// prices, so every case pins them instead of inheriting machine state.
func aipddRatioTestDataEnv(t *testing.T) {
	t.Setenv("AIPDD_PRICING_MARKUP", "1.1")
	previousExchangeRate := operation_setting.USDExchangeRate
	operation_setting.USDExchangeRate = 7.3
	t.Cleanup(func() { operation_setting.USDExchangeRate = previousExchangeRate })
}

func TestAIPDDCatalogRatioDataSkipsPerSecondTaskPrices(t *testing.T) {
	aipddRatioTestDataEnv(t)
	displayAmount := 40.0
	displayVideoAmount := 60.0
	cacheRead := 0.0
	cacheWrite := 0.0
	seedancePricing := aipddcatalog.AtomicPricing{
		PricingModel: "per_second", Currency: "awcoin", PricingBasis: "display", Enabled: true,
		ByResolution: map[string]constant.AIPDDSeedanceResolutionPricing{
			"1080p": {
				TargetResolution:                 "1080p",
				DisplayAmountAWCoinPerSecond:     &displayAmount,
				DisplayVideoInputAWCoinPerSecond: &displayVideoAmount,
				DefaultDurationSeconds:           5,
				DefaultFramesPerSecond:           24,
			},
		},
	}
	require.Greater(t, aipddcatalog.TaskAWCoinPrice(seedancePricing), float64(0))

	catalog := aipddcatalog.AtomicCatalog{
		AWCoinRate: aipddcatalog.AtomicAWCoinRate{RMBPerAWCoin: 0.073, USDPerAWCoin: 0.01},
		Capabilities: []aipddcatalog.AtomicCapability{
			{
				ID: "AP Seedance", AdapterCode: "seedance",
				Pricing: seedancePricing,
			},
			{
				ID: "per-call-task", AdapterCode: "comfyui",
				Pricing: aipddcatalog.AtomicPricing{
					PricingModel: "per_call", Currency: "awcoin", Enabled: true,
					ChargeConfig: map[string]any{"amountAwcoin": float64(100)},
				},
			},
		},
		Models: []aipddcatalog.AtomicModel{{
			ID: "aipdd-llm", Available: true,
			Pricing: aipddcatalog.AtomicPricing{
				PricingModel: "per_token", Currency: "awcoin", Enabled: true,
				PromptPerMillion: 10, CompletionPerMillion: 30,
				CacheReadPerMillion: &cacheRead, CacheWritePerMillion: &cacheWrite,
			},
		}},
	}

	data := aipddCatalogRatioData(catalog)
	prices, ok := data["model_price"].(map[string]any)
	require.True(t, ok)
	require.NotContains(t, prices, "AP Seedance")
	require.InDelta(t, 1.1, prices["per-call-task"], 1e-12)

	modes, ok := data[billing_setting.BillingModeField].(map[string]string)
	require.True(t, ok)
	require.Equal(t, billing_setting.BillingModeTieredExpr, modes["aipdd-llm"])
	exprs, ok := data[billing_setting.BillingExprField].(map[string]string)
	require.True(t, ok)
	require.Equal(t, `tier("aipdd", p * 0.11 + c * 0.33 + cr * 0 + cc * 0 + cc1h * 0)`, exprs["aipdd-llm"])
}

func TestAIPDDCatalogRatioDataAnchorsDisplayPerCallPriceToRMB(t *testing.T) {
	previousExchangeRate := operation_setting.USDExchangeRate
	operation_setting.USDExchangeRate = 8
	t.Cleanup(func() { operation_setting.USDExchangeRate = previousExchangeRate })
	t.Setenv("AIPDD_PRICING_MARKUP", "1.1")

	catalog := aipddcatalog.AtomicCatalog{
		AWCoinRate: aipddcatalog.AtomicAWCoinRate{
			RMBPerAWCoin: 0.01,
			USDPerAWCoin: 0.0015,
		},
		Capabilities: []aipddcatalog.AtomicCapability{{
			ID:          "display-per-call-image",
			AdapterCode: "token_market_media",
			Pricing: aipddcatalog.AtomicPricing{
				PricingModel: "per_call",
				Currency:     "awcoin",
				Enabled:      true,
				ChargeConfig: map[string]any{"amountAwcoin": float64(100)},
			},
		}},
	}

	data := aipddCatalogRatioData(catalog)
	prices, ok := data["model_price"].(map[string]any)
	require.True(t, ok)
	require.InDelta(t, 0.1375, prices["display-per-call-image"], 1e-12)
}

func TestAIPDDCatalogRatioDataUsesCacheAwareExpressionWhenPricesArePresent(t *testing.T) {
	aipddRatioTestDataEnv(t)
	cacheRead := 2.0
	cacheWrite := 12.5
	catalog := aipddcatalog.AtomicCatalog{
		AWCoinRate: aipddcatalog.AtomicAWCoinRate{RMBPerAWCoin: 0.073, USDPerAWCoin: 0.01},
		Models: []aipddcatalog.AtomicModel{{
			ID: "market/cache-aware", Available: true,
			Pricing: aipddcatalog.AtomicPricing{
				PricingModel: "per_token", Currency: "awcoin", Enabled: true,
				PromptPerMillion: 10, CompletionPerMillion: 30,
				CacheReadPerMillion: &cacheRead, CacheWritePerMillion: &cacheWrite,
			},
		}},
	}

	data := aipddCatalogRatioData(catalog)
	exprs := data[billing_setting.BillingExprField].(map[string]string)
	require.Equal(
		t,
		`tier("aipdd", p * 0.11 + c * 0.33 + cr * 0.022 + cc * 0.1375 + cc1h * 0.1375)`,
		exprs["market/cache-aware"])
}

func TestAIPDDCatalogRatioDataSkipsFreeModel(t *testing.T) {
	aipddRatioTestDataEnv(t)
	zero := 0.0
	catalog := aipddcatalog.AtomicCatalog{
		SchemaVersion: 2,
		AWCoinRate:    aipddcatalog.AtomicAWCoinRate{RMBPerAWCoin: 0.073, USDPerAWCoin: 0.01},
		Models: []aipddcatalog.AtomicModel{{
			ID: "free-deepseek-v4-flash", Available: true,
			Pricing: aipddcatalog.AtomicPricing{
				PricingModel: "per_token", Currency: "awcoin", Enabled: true, Free: true,
				CacheReadPerMillion: &zero, CacheWritePerMillion: &zero,
			},
		}},
	}

	data := aipddCatalogRatioData(catalog)
	require.NotContains(t, data, billing_setting.BillingModeField)
	require.NotContains(t, data, billing_setting.BillingExprField)
}

func TestAIPDDCatalogRatioDataPricesUnavailableModel(t *testing.T) {
	aipddRatioTestDataEnv(t)
	zero := 0.0
	catalog := aipddcatalog.AtomicCatalog{
		SchemaVersion: 2,
		AWCoinRate:    aipddcatalog.AtomicAWCoinRate{RMBPerAWCoin: 0.073, USDPerAWCoin: 0.01},
		Models: []aipddcatalog.AtomicModel{{
			ID: "retired-model",
			Pricing: aipddcatalog.AtomicPricing{
				PricingModel: "per_token", Currency: "awcoin", Enabled: true,
				PromptPerMillion: 10, CompletionPerMillion: 30,
				CacheReadPerMillion: &zero, CacheWritePerMillion: &zero,
			},
		}},
	}

	data := aipddCatalogRatioData(catalog)
	modes, ok := data[billing_setting.BillingModeField].(map[string]string)
	require.True(t, ok)
	require.Equal(t, billing_setting.BillingModeTieredExpr, modes["retired-model"],
		"upstream availability does not exempt a published price from the margin")
	exprs, ok := data[billing_setting.BillingExprField].(map[string]string)
	require.True(t, ok)
	require.Contains(t, exprs["retired-model"], "p * 0.11")
}

func TestAIPDDCatalogRatioDataPreservesLegacyInputPricingWhenCachePricesAreMissing(t *testing.T) {
	aipddRatioTestDataEnv(t)
	catalog := aipddcatalog.AtomicCatalog{
		SchemaVersion: 1,
		AWCoinRate:    aipddcatalog.AtomicAWCoinRate{RMBPerAWCoin: 0.073, USDPerAWCoin: 0.01},
		Models: []aipddcatalog.AtomicModel{{
			ID: "deepseek-r1:8b", Available: true,
			Pricing: aipddcatalog.AtomicPricing{
				PricingModel: "per_token", Currency: "awcoin", Enabled: true,
				PromptPerMillion: 10, CompletionPerMillion: 30,
			},
		}},
	}

	data := aipddCatalogRatioData(catalog)
	exprs := data[billing_setting.BillingExprField].(map[string]string)
	require.Equal(
		t,
		`tier("aipdd", p * 0.11 + c * 0.33)`,
		exprs["deepseek-r1:8b"])
}

// An administrator's pinned price must not be offered as a sync candidate,
// otherwise applying the batch would overwrite the hand-set price.
func TestAIPDDCatalogRatioDataOmitsPinnedModels(t *testing.T) {
	aipddRatioTestDataEnv(t)
	zero := 0.0
	catalog := aipddcatalog.AtomicCatalog{
		SchemaVersion: 2,
		AWCoinRate:    aipddcatalog.AtomicAWCoinRate{RMBPerAWCoin: 0.073, USDPerAWCoin: 0.01},
		Capabilities: []aipddcatalog.AtomicCapability{{
			ID: "pinned-image", AdapterCode: "token_market_media",
			Pricing: aipddcatalog.AtomicPricing{
				PricingModel: "per_call", Currency: "awcoin", PricingBasis: "display", Enabled: true,
				ChargeConfig: map[string]any{"amountAwcoin": float64(100)},
			},
		}},
		Models: []aipddcatalog.AtomicModel{
			{
				ID: "pinned-llm", Available: true,
				Pricing: aipddcatalog.AtomicPricing{
					PricingModel: "per_token", Currency: "awcoin", Enabled: true,
					PromptPerMillion: 10, CompletionPerMillion: 30,
					CacheReadPerMillion: &zero, CacheWritePerMillion: &zero,
				},
			},
			{
				ID: "catalog-llm", Available: true,
				Pricing: aipddcatalog.AtomicPricing{
					PricingModel: "per_token", Currency: "awcoin", Enabled: true,
					PromptPerMillion: 20, CompletionPerMillion: 40,
					CacheReadPerMillion: &zero, CacheWritePerMillion: &zero,
				},
			},
		},
	}

	data := aipddCatalogRatioDataPinned(catalog, map[string]bool{"pinned-image": true, "pinned-llm": true})
	if prices, ok := data["model_price"].(map[string]any); ok {
		require.NotContains(t, prices, "pinned-image")
	}
	modes := data[billing_setting.BillingModeField].(map[string]string)
	require.NotContains(t, modes, "pinned-llm")
	require.Equal(t, billing_setting.BillingModeTieredExpr, modes["catalog-llm"])
	exprs := data[billing_setting.BillingExprField].(map[string]string)
	require.NotContains(t, exprs, "pinned-llm")
	require.Contains(t, exprs, "catalog-llm")
}

func TestStripTaskPricingSyncModelsRemovesDerivedLegacyPrices(t *testing.T) {
	data := map[string]any{
		billing_setting.BillingModeField: map[string]any{
			"seedance-local": billing_setting.BillingModeTaskPricing,
			"tiered-local":   billing_setting.BillingModeTieredExpr,
		},
		billing_setting.BillingExprField: map[string]any{
			"seedance-local": "must-not-import",
			"tiered-local":   `tier("remote", p * 1)`,
		},
		"model_price": map[string]any{
			"seedance-local": 0.12,
			"fixed-local":    0.5,
		},
		"model_ratio": map[string]any{
			"seedance-local": 9.0,
			"ratio-local":    1.5,
		},
	}

	stripTaskPricingSyncModels(data)

	for _, field := range pricingSyncFields {
		if values := valueMap(data[field]); values != nil {
			require.NotContains(t, values, "seedance-local", field)
		}
	}
	require.Equal(t, 0.5, valueMap(data["model_price"])["fixed-local"])
	require.Equal(t, 1.5, valueMap(data["model_ratio"])["ratio-local"])
	require.Equal(t, billing_setting.BillingModeTieredExpr, valueMap(data[billing_setting.BillingModeField])["tiered-local"])
}
