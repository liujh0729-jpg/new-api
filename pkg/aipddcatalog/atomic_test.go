package aipddcatalog

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/stretchr/testify/require"
)

func TestFetchAtomicFiltersExcludedFamiliesOnReceiver(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, AtomicCatalogPath, r.URL.Path)
		_, _ = w.Write([]byte(`{
			"code":0,"message":"ok","data":{
				"schemaVersion":1,"revision":"revision-1","generatedAt":"2026-07-12T10:00:00",
				"awcoinRate":{"rmbPerAwcoin":0.01,"usdPerAwcoin":0.0015,"updatedAt":"2026-07-12T09:00:00"},
				"capabilities":[
					{"id":"keep-comfy","code":"keep-comfy","adapterCode":"comfyui","execution":{"protocol":"shared_task","path":"/shared-tasks/tasks"},"pricing":{"enabled":true,"chargeConfig":{"amountAwcoin":10}}},
					{"id":"seedvr2-upscale","code":"seedvr2-upscale","adapterCode":"comfyui","execution":{"protocol":"shared_task","path":"/shared-tasks/tasks"},"pricing":{"enabled":true,"chargeConfig":{"amountAwcoin":10}}},
					{"id":"aipdd_lightx2v_ltx23_distilled_fp8_i2av","code":"aipdd_lightx2v_ltx23_distilled_fp8_i2av","adapterCode":"lightx2v_python","execution":{"protocol":"shared_task","path":"/shared-tasks/tasks"},"pricing":{"enabled":true,"chargeConfig":{"amountAwcoin":10}}}
				],
				"models":[
					{"id":"qwen3:8b","execution":{"protocol":"openai","path":"/v1/chat/completions"},"pricing":{"enabled":true,"promptPerMillion":10,"completionPerMillion":20}},
					{"id":"funasr-llm","execution":{"protocol":"openai","path":"/v1/chat/completions"},"pricing":{"enabled":true,"promptPerMillion":10,"completionPerMillion":20}}
				]
			}
		}`))
	}))
	defer server.Close()

	catalog, err := FetchAtomic(context.Background(), server.Client(), server.URL, "sk-test")
	require.NoError(t, err)
	require.Equal(t, []string{"keep-comfy", "qwen3:8b"}, catalog.ModelNames())
	runtimeCapabilities := catalog.RuntimeCapabilities()
	require.Len(t, runtimeCapabilities, 1)
	require.Equal(t, "keep-comfy", runtimeCapabilities[0].ModelName)
}

func TestTaskAWCoinPriceUsesStrictSeedanceDisplayContract(t *testing.T) {
	display4K := float64(100)
	display720P := float64(20.1)
	pricing := AtomicPricing{ByResolution: map[string]constant.AIPDDSeedanceResolutionPricing{
		"4k": {
			TargetResolution:             "4k",
			DefaultDurationSeconds:       5,
			DefaultFramesPerSecond:       24,
			AmountAWCoinPerSecond:        100,
			TextInputAWCoinPerSecond:     100,
			ImageInputAWCoinPerSecond:    100,
			VideoInputAWCoinPerSecond:    120,
			AudioInputAWCoinPerSecond:    100,
			DisplayAmountAWCoinPerSecond: &display4K,
		},
		"720p": {
			TargetResolution:             "720p",
			DefaultDurationSeconds:       5,
			DefaultFramesPerSecond:       24,
			AmountAWCoinPerSecond:        20.1,
			TextInputAWCoinPerSecond:     20.1,
			ImageInputAWCoinPerSecond:    20.1,
			VideoInputAWCoinPerSecond:    30,
			AudioInputAWCoinPerSecond:    20.1,
			DisplayAmountAWCoinPerSecond: &display720P,
		},
	}}

	require.Equal(t, float64(101), TaskAWCoinPrice(pricing))
}

func TestAtomicCatalogPrefersExplicitDisplayPricesAndKeepsBYOKSeparate(t *testing.T) {
	displayAmount := float64(4620)
	byokAmount := float64(600)
	displayVideoAmount := float64(12770)
	byokVideoAmount := float64(1670)
	catalog := AtomicCatalog{
		SchemaVersion: 1,
		Revision:      "revision-display-pricing",
		AWCoinRate:    AtomicAWCoinRate{RMBPerAWCoin: 0.01, USDPerAWCoin: 0.001},
		Capabilities: []AtomicCapability{{
			ID: "AP Seedance", AdapterCode: "seedance",
			Execution: AtomicExecution{Protocol: "seedance_official", Path: "/api/v3/contents/generations/tasks"},
			Pricing: AtomicPricing{
				PricingModel: "per_second", Currency: "awcoin", PricingBasis: "display", Enabled: true,
				ByResolution: map[string]constant.AIPDDSeedanceResolutionPricing{
					"720p": {
						TargetResolution:                 "720p",
						DefaultDurationSeconds:           5,
						DefaultFramesPerSecond:           24,
						AmountAWCoinPerSecond:            600,
						DisplayAmountAWCoinPerSecond:     &displayAmount,
						BYOKAmountAWCoinPerSecond:        &byokAmount,
						TextInputAWCoinPerSecond:         600,
						ImageInputAWCoinPerSecond:        600,
						VideoInputAWCoinPerSecond:        1670,
						DisplayVideoInputAWCoinPerSecond: &displayVideoAmount,
						BYOKVideoInputAWCoinPerSecond:    &byokVideoAmount,
						AudioInputAWCoinPerSecond:        600,
					},
				},
			},
		}},
	}

	require.NoError(t, catalog.Validate())
	require.Equal(t, float64(23100), TaskAWCoinPrice(catalog.Capabilities[0].Pricing))
	runtimeCapabilities := catalog.RuntimeCapabilities()
	require.Len(t, runtimeCapabilities, 1)
	resolution := runtimeCapabilities[0].SeedancePricing.ByResolution["720p"]
	require.Equal(t, float64(4620), resolution.AmountAWCoinPerSecond)
	require.Equal(t, float64(4620), resolution.TextInputAWCoinPerSecond)
	require.Equal(t, float64(12770), resolution.VideoInputAWCoinPerSecond)
	require.Equal(t, float64(600), *resolution.BYOKAmountAWCoinPerSecond)
	require.Equal(t, float64(1670), *resolution.BYOKVideoInputAWCoinPerSecond)
}

func TestAtomicCatalogDoesNotFallbackWhenExplicitDisplayPriceIsInvalid(t *testing.T) {
	zero := float64(0)
	displayVideoAmount := float64(12770)
	catalog := AtomicCatalog{
		SchemaVersion: 1,
		Revision:      "revision-invalid-display-pricing",
		AWCoinRate:    AtomicAWCoinRate{RMBPerAWCoin: 0.01, USDPerAWCoin: 0.001},
		Capabilities: []AtomicCapability{{
			ID: "AP Seedance", AdapterCode: "seedance",
			Execution: AtomicExecution{Protocol: "seedance_official", Path: "/api/v3/contents/generations/tasks"},
			Pricing: AtomicPricing{
				PricingModel: "per_second", Currency: "awcoin", PricingBasis: "display", Enabled: true,
				ByResolution: map[string]constant.AIPDDSeedanceResolutionPricing{
					"720p": {
						TargetResolution:                 "720p",
						DefaultDurationSeconds:           5,
						DefaultFramesPerSecond:           24,
						AmountAWCoinPerSecond:            600,
						DisplayAmountAWCoinPerSecond:     &zero,
						VideoInputAWCoinPerSecond:        1670,
						DisplayVideoInputAWCoinPerSecond: &displayVideoAmount,
					},
				},
			},
		}},
	}

	err := catalog.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "displayAmountAwcoinPerSecond")
}

func TestAtomicCatalogRejectsLegacySeedancePricingContract(t *testing.T) {
	catalog := AtomicCatalog{
		SchemaVersion: 1,
		Revision:      "revision-legacy",
		AWCoinRate:    AtomicAWCoinRate{RMBPerAWCoin: 0.01, USDPerAWCoin: 0.001},
		Capabilities: []AtomicCapability{{
			ID: "AP Seedance", AdapterCode: "seedance",
			Execution: AtomicExecution{Protocol: "seedance_official", Path: "/api/v3/contents/generations/tasks"},
			Pricing: AtomicPricing{
				PricingModel: "per_second", Currency: "awcoin", Enabled: true,
				ByResolution: map[string]constant.AIPDDSeedanceResolutionPricing{
					"720p": {TargetResolution: "720p", DefaultDurationSeconds: 5, DefaultFramesPerSecond: 24},
				},
			},
		}},
	}

	err := catalog.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "pricingBasis")
}

func TestAtomicCatalogMapsPerUnitSecondToDurationBilling(t *testing.T) {
	catalog := AtomicCatalog{
		SchemaVersion: 1,
		Revision:      "revision-duration",
		AWCoinRate:    AtomicAWCoinRate{RMBPerAWCoin: 0.0001, USDPerAWCoin: 0.00001},
		Capabilities: []AtomicCapability{{
			ID: "aipdd_ltx_2.3", AdapterCode: "comfyui",
			Execution: AtomicExecution{Protocol: "shared_task", Path: "/shared-tasks/tasks"},
			Pricing: AtomicPricing{
				PricingModel: "per_unit", Currency: "awcoin", Enabled: true,
				ChargeConfig: map[string]any{"unit": "second", "amount": float64(1800)},
			},
		}},
	}

	require.NoError(t, catalog.Validate())
	runtimeCapabilities := catalog.RuntimeCapabilities()
	require.Len(t, runtimeCapabilities, 1)
	require.Equal(t, constant.AIPDDBillingTypeDurationSeconds, runtimeCapabilities[0].BillingType)
	require.Equal(t, float64(1800), runtimeCapabilities[0].TaskCost)
}

func TestAtomicCatalogRejectsUnsupportedPerUnitChargeUnit(t *testing.T) {
	catalog := AtomicCatalog{
		SchemaVersion: 1,
		Revision:      "revision-invalid-duration",
		AWCoinRate:    AtomicAWCoinRate{RMBPerAWCoin: 0.0001, USDPerAWCoin: 0.00001},
		Capabilities: []AtomicCapability{{
			ID: "unsupported-unit", AdapterCode: "comfyui",
			Execution: AtomicExecution{Protocol: "shared_task", Path: "/shared-tasks/tasks"},
			Pricing: AtomicPricing{
				PricingModel: "per_unit", Currency: "awcoin", Enabled: true,
				ChargeConfig: map[string]any{"unit": "minute", "amount": float64(1800)},
			},
		}},
	}

	err := catalog.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "unsupported charge unit")
}
