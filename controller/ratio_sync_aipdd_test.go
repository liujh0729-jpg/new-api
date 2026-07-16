package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/pkg/aipddcatalog"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/stretchr/testify/require"
)

func TestAIPDDCatalogRatioDataSkipsPerSecondTaskPrices(t *testing.T) {
	seedancePricing := aipddcatalog.AtomicPricing{
		PricingModel: "per_second", Currency: "awcoin", Enabled: true,
		ByResolution: map[string]constant.AIPDDSeedanceResolutionPricing{
			"1080p": {
				DefaultDurationSeconds: 5,
				PriceVariants: []constant.AIPDDSeedancePriceVariant{{
					HasReferenceVideo: false,
					AWCoinPerSecond:   40,
					MinimumAWCoin:     100,
				}},
			},
		},
	}
	require.Greater(t, aipddcatalog.TaskAWCoinPrice(seedancePricing), float64(0))

	catalog := aipddcatalog.AtomicCatalog{
		AWCoinRate: aipddcatalog.AtomicAWCoinRate{USDPerAWCoin: 0.01},
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
			ID: "aipdd-llm",
			Pricing: aipddcatalog.AtomicPricing{
				PricingModel: "per_token", Currency: "awcoin", Enabled: true,
				PromptPerMillion: 10, CompletionPerMillion: 30,
			},
		}},
	}

	data := aipddCatalogRatioData(catalog)
	prices, ok := data["model_price"].(map[string]any)
	require.True(t, ok)
	require.NotContains(t, prices, "AP Seedance")
	require.InDelta(t, 1, prices["per-call-task"], 0.0000001)

	modes, ok := data[billing_setting.BillingModeField].(map[string]string)
	require.True(t, ok)
	require.Equal(t, billing_setting.BillingModeTieredExpr, modes["aipdd-llm"])
	exprs, ok := data[billing_setting.BillingExprField].(map[string]string)
	require.True(t, ok)
	require.Equal(t, `tier("aipdd", p * 0.1 + c * 0.3)`, exprs["aipdd-llm"])
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
