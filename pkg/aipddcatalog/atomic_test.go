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
					{"id":"qwen3:8b","execution":{"protocol":"openai","path":"/v1/chat/completions"},"pricing":{"enabled":true,"promptPerMillion":10,"completionPerMillion":20,"cacheReadPerMillion":0,"cacheWritePerMillion":10}},
					{"id":"funasr-llm","execution":{"protocol":"openai","path":"/v1/chat/completions"},"pricing":{"enabled":true,"promptPerMillion":10,"completionPerMillion":20,"cacheReadPerMillion":0,"cacheWritePerMillion":10}}
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

func TestAtomicCatalogV2AcceptsVisionAndFunctionToolMetadata(t *testing.T) {
	cacheRead := 2.0
	cacheWrite := 12.5
	catalog := AtomicCatalog{
		SchemaVersion: 2,
		Revision:      "vision-tools-v2",
		AWCoinRate:    AtomicAWCoinRate{RMBPerAWCoin: 0.01, USDPerAWCoin: 0.0015},
		Models: []AtomicModel{{
			ID: "market/vision-tools", InputModalities: []string{"text", "image"}, OutputModalities: []string{"text"},
			Available: true, Execution: AtomicExecution{Protocol: "openai", Path: "/v1/chat/completions"},
			Pricing: AtomicPricing{
				Enabled: true, PromptPerMillion: 10, CompletionPerMillion: 20,
				CacheReadPerMillion: &cacheRead, CacheWritePerMillion: &cacheWrite,
			},
			Protocols: []string{"chat", "responses"},
			Features: &AtomicFeatures{
				ImageSources:  []string{"https", "data"},
				FunctionTools: AtomicFunctionTools{Basic: true, Strict: true, Streaming: true, MultiRound: true},
				Usage:         AtomicUsage{Streaming: true, NonStreaming: true},
				ByProtocol: map[string]AtomicProtocolCapabilities{
					"chat": {
						InputModalities: []string{"text", "image"},
						ImageSources:    []string{"data", "https"},
						FunctionTools:   AtomicFunctionTools{Basic: true, Streaming: true},
						Usage:           AtomicUsage{Streaming: true, NonStreaming: true},
					},
				},
			},
		}},
	}
	require.NoError(t, catalog.Validate())
	encoded, err := MarshalAtomic(catalog)
	require.NoError(t, err)
	decoded, err := UnmarshalAtomic(encoded)
	require.NoError(t, err)
	require.Equal(t, []string{"chat", "responses"}, decoded.Models[0].Protocols)
	require.True(t, decoded.Models[0].Features.FunctionTools.Strict)
	require.True(t, decoded.Models[0].Features.Usage.NonStreaming)
	require.True(t, decoded.Models[0].Features.ByProtocol["chat"].FunctionTools.Streaming)
	require.Equal(t, 2.0, *decoded.Models[0].Pricing.CacheReadPerMillion)
	require.Equal(t, 12.5, *decoded.Models[0].Pricing.CacheWritePerMillion)
}

func TestAtomicCatalogRejectsLLMWithoutFourTierPricing(t *testing.T) {
	catalog := AtomicCatalog{
		SchemaVersion: 2,
		Revision:      "missing-cache-prices",
		AWCoinRate:    AtomicAWCoinRate{RMBPerAWCoin: 0.01, USDPerAWCoin: 0.0015},
		Models: []AtomicModel{{
			ID: "market/incomplete",
			Pricing: AtomicPricing{
				Enabled: true, PromptPerMillion: 10, CompletionPerMillion: 20,
			},
		}},
	}

	require.ErrorContains(t, catalog.Validate(), "must provide cache read and cache write prices")
}

func TestAtomicCatalogValidatesThenFiltersExplicitFreeLLM(t *testing.T) {
	zero := 0.0
	catalog := AtomicCatalog{
		SchemaVersion: 2,
		Revision:      "explicit-free-model",
		AWCoinRate:    AtomicAWCoinRate{RMBPerAWCoin: 0.01, USDPerAWCoin: 0.0015},
		Models: []AtomicModel{{
			ID: "free-deepseek-v4-flash",
			Pricing: AtomicPricing{
				PricingModel: "per_token", Currency: "awcoin", Enabled: true, Free: true,
				CacheReadPerMillion: &zero, CacheWritePerMillion: &zero,
			},
		}},
	}

	require.NoError(t, catalog.Validate())
	encoded, err := MarshalAtomic(catalog)
	require.NoError(t, err)
	decoded, err := UnmarshalAtomic(encoded)
	require.NoError(t, err)
	require.Empty(t, decoded.Models)
}

func TestAtomicCatalogRejectsAccidentalOrInconsistentFreePricing(t *testing.T) {
	zero := 0.0
	one := 1.0
	base := AtomicCatalog{
		SchemaVersion: 2,
		Revision:      "invalid-free-model",
		AWCoinRate:    AtomicAWCoinRate{RMBPerAWCoin: 0.01, USDPerAWCoin: 0.0015},
		Models: []AtomicModel{{
			ID: "ordinary-zero-price",
			Pricing: AtomicPricing{
				PricingModel: "per_token", Currency: "awcoin", Enabled: true,
				CacheReadPerMillion: &zero, CacheWritePerMillion: &zero,
			},
		}},
	}
	require.ErrorContains(t, base.Validate(), "has no effective price")

	base.Models[0].Pricing.Free = true
	require.ErrorContains(t, base.Validate(), "without a free- model ID")

	base.Models[0].ID = "free-inconsistent"
	base.Models[0].Pricing.CacheWritePerMillion = &one
	require.ErrorContains(t, base.Validate(), "must have zero prices in every token lane")
}

func TestAtomicCatalogV1AcceptsLegacyLLMWithoutCachePrices(t *testing.T) {
	catalog, err := UnmarshalAtomic([]byte(`{
		"schemaVersion":1,
		"revision":"legacy-v1-without-cache-prices",
		"awcoinRate":{"rmbPerAwcoin":0.01,"usdPerAwcoin":0.0015},
		"models":[{
			"id":"deepseek-r1:8b",
			"pricing":{"promptPerMillion":10,"completionPerMillion":20}
		}]
	}`))

	require.NoError(t, err)
	require.Len(t, catalog.Models, 1)
	require.Nil(t, catalog.Models[0].Pricing.CacheReadPerMillion)
	require.Nil(t, catalog.Models[0].Pricing.CacheWritePerMillion)
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

func TestAtomicCatalogAcceptsSeedance25AutoDurationAndPricesThirtySecondHold(t *testing.T) {
	displayAmount := float64(679)
	displayVideoAmount := float64(2038)
	catalog := AtomicCatalog{
		SchemaVersion: 1,
		Revision:      "revision-seedance-25-auto-duration",
		AWCoinRate:    AtomicAWCoinRate{RMBPerAWCoin: 0.001, USDPerAWCoin: 0.0001},
		Capabilities: []AtomicCapability{{
			ID: "AP Seedance-2.5 标准版", AdapterCode: "seedance",
			Execution: AtomicExecution{Protocol: "seedance_official", Path: "/api/v3/contents/generations/tasks"},
			Pricing: AtomicPricing{
				PricingModel: "per_second", Currency: "awcoin", PricingBasis: "display", Enabled: true,
				ByResolution: map[string]constant.AIPDDSeedanceResolutionPricing{
					"480p": {
						TargetResolution:                 "480p",
						DefaultDurationSeconds:           -1,
						DefaultFramesPerSecond:           24,
						DisplayAmountAWCoinPerSecond:     &displayAmount,
						DisplayVideoInputAWCoinPerSecond: &displayVideoAmount,
					},
				},
			},
		}},
	}

	for _, modelName := range []string{
		"AP Seedance-2.5 标准版",
		"AP Seedance 2.5 标准版",
		"AP SEEDANCE_2_5 标准版",
	} {
		catalog.Capabilities[0].ID = modelName
		require.NoError(t, catalog.Validate())
	}
	require.Equal(t, float64(20370), TaskAWCoinPrice(catalog.Capabilities[0].Pricing))

	catalog.Capabilities[0].ID = "AP Seedance-2.0 标准版"
	require.ErrorContains(t, catalog.Validate(), "positive defaultDurationSeconds")
	catalog.Capabilities[0].ID = "AP Seedance-2.50 标准版"
	require.ErrorContains(t, catalog.Validate(), "positive defaultDurationSeconds")

	catalog.Capabilities[0].ID = "AP Seedance-2.5 标准版"
	for _, duration := range []float64{-2, 0} {
		catalog.Capabilities[0].Pricing.ByResolution["480p"] = constant.AIPDDSeedanceResolutionPricing{
			TargetResolution:                 "480p",
			DefaultDurationSeconds:           duration,
			DefaultFramesPerSecond:           24,
			DisplayAmountAWCoinPerSecond:     &displayAmount,
			DisplayVideoInputAWCoinPerSecond: &displayVideoAmount,
		}
		require.ErrorContains(t, catalog.Validate(), "positive defaultDurationSeconds")
	}
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

func TestAtomicCatalogAcceptsTokenMarketVideoAndBuildsRuntimeRouting(t *testing.T) {
	display := 12.0
	videoInput := 12.0
	catalog := AtomicCatalog{
		SchemaVersion: 2,
		Revision:      "revision-minimax-h3",
		AWCoinRate:    AtomicAWCoinRate{RMBPerAWCoin: 0.01, USDPerAWCoin: 0.0014},
		Capabilities: []AtomicCapability{{
			ID: "ap-minimax-h3-text-to-video", AdapterCode: "token_market_media",
			EndpointType: "openai-video", TaskKind: "video_generation",
			Execution: AtomicExecution{Protocol: "token_market_video", Path: "/v1/videos"},
			Pricing: AtomicPricing{
				PricingModel: "per_second", Currency: "awcoin", PricingBasis: "display", Enabled: true,
				ByResolution: map[string]constant.AIPDDSeedanceResolutionPricing{
					"768p": {
						TargetResolution:                 "768p",
						DisplayAmountAWCoinPerSecond:     &display,
						DisplayVideoInputAWCoinPerSecond: &videoInput,
						DefaultDurationSeconds:           1,
						DefaultFramesPerSecond:           24,
					},
				},
			},
		}},
	}

	require.NoError(t, catalog.Validate())
	require.Equal(t, []string{"ap-minimax-h3-text-to-video"}, catalog.ModelNames())
	runtimeCapabilities := catalog.RuntimeCapabilities()
	require.Len(t, runtimeCapabilities, 1)
	require.Equal(t, "token_market_video", runtimeCapabilities[0].ExecutionProtocol)
	require.Equal(t, "/v1/videos", runtimeCapabilities[0].ExecutionPath)
	require.Equal(t, constant.AIPDDBillingTypeDurationSeconds, runtimeCapabilities[0].BillingType)
	require.NotNil(t, runtimeCapabilities[0].SeedancePricing)
	require.Equal(t, 12.0, runtimeCapabilities[0].SeedancePricing.ByResolution["768p"].AmountAWCoinPerSecond)
}

func TestAtomicCatalogAcceptsTokenMarketImageWithoutDurationBilling(t *testing.T) {
	catalog := AtomicCatalog{
		SchemaVersion: 2,
		Revision:      "revision-agnes-image",
		AWCoinRate:    AtomicAWCoinRate{RMBPerAWCoin: 0.01, USDPerAWCoin: 0.0014},
		Capabilities: []AtomicCapability{{
			ID: "ap-agnes-image-2.1-flash", AdapterCode: "token_market_media",
			EndpointType: "image-generation", TaskKind: "image_generation",
			Execution: AtomicExecution{Protocol: "token_market_image", Path: "/v1/images/generations"},
			Pricing: AtomicPricing{
				PricingModel: "per_call", Currency: "awcoin", Enabled: true,
				ChargeConfig: map[string]any{"unit": "image", "amountAwcoin": float64(7)},
			},
		}},
	}

	require.NoError(t, catalog.Validate())
	runtimeCapabilities := catalog.RuntimeCapabilities()
	require.Len(t, runtimeCapabilities, 1)
	require.Equal(t, "token_market_image", runtimeCapabilities[0].ExecutionProtocol)
	require.Equal(t, constant.EndpointTypeImageGeneration, runtimeCapabilities[0].EndpointType)
	require.Equal(t, float64(7), runtimeCapabilities[0].TaskCost)
	require.Nil(t, runtimeCapabilities[0].SeedancePricing)
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

func TestAtomicCatalogNormalizesMissingPerUnitSecondUnit(t *testing.T) {
	catalog := AtomicCatalog{
		SchemaVersion: 1,
		Revision:      "revision-normalize-unit",
		AWCoinRate:    AtomicAWCoinRate{RMBPerAWCoin: 0.0001, USDPerAWCoin: 0.00001},
		Capabilities: []AtomicCapability{{
			ID:          "aipdd_ltx_2.3",
			AdapterCode: "comfyui",
			Execution:   AtomicExecution{Protocol: "shared_task", Path: "/shared-tasks/tasks"},
			Pricing: AtomicPricing{
				PricingModel: "per_unit", Currency: "awcoin", Enabled: true,
				ChargeConfig: map[string]any{"unitLabel": "second", "amount": float64(4000), "minSeconds": float64(1)},
			},
		}},
	}

	catalog.NormalizePerUnitChargeUnits()
	require.NoError(t, catalog.Validate())
	require.Equal(t, "second", catalog.Capabilities[0].Pricing.ChargeConfig["unit"])
	runtimeCapabilities := catalog.RuntimeCapabilities()
	require.Len(t, runtimeCapabilities, 1)
	require.Equal(t, constant.AIPDDBillingTypeDurationSeconds, runtimeCapabilities[0].BillingType)
	require.Equal(t, float64(4000), runtimeCapabilities[0].TaskCost)
}

func TestFetchAtomicNormalizesMissingPerUnitSecondUnit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, AtomicCatalogPath, r.URL.Path)
		_, _ = w.Write([]byte(`{
			"code":0,"message":"ok","data":{
				"schemaVersion":1,"revision":"revision-normalize-fetch","generatedAt":"2026-08-13T00:00:00",
				"awcoinRate":{"rmbPerAwcoin":0.01,"usdPerAwcoin":0.0015,"updatedAt":"2026-08-13T00:00:00"},
				"capabilities":[
					{"id":"aipdd_ltx_2.3","code":"aipdd_ltx_2.3","adapterCode":"comfyui",
					 "execution":{"protocol":"shared_task","path":"/shared-tasks/tasks"},
					 "pricing":{"pricingModel":"per_unit","currency":"awcoin","enabled":true,
					            "chargeConfig":{"amount":4000,"unitLabel":"second","minSeconds":1}}}
				],
				"models":[]
			}
		}`))
	}))
	defer server.Close()

	catalog, err := FetchAtomic(context.Background(), server.Client(), server.URL, "sk-test")
	require.NoError(t, err)
	require.Equal(t, "second", catalog.Capabilities[0].Pricing.ChargeConfig["unit"])
	require.Equal(t, []string{"aipdd_ltx_2.3"}, catalog.ModelNames())
}
