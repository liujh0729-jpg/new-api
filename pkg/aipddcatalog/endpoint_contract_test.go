package aipddcatalog

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/stretchr/testify/require"
)

// The live catalog dispatches shared-compute capabilities through
// token_market_shared and publishes an embeddings protocol. Rejecting either
// discards the whole catalog, so both must validate.
func TestAtomicCatalogAcceptsSharedAdapterAndEmbeddingsProtocol(t *testing.T) {
	zero := 0.0
	catalog := AtomicCatalog{
		SchemaVersion: 2,
		Revision:      "shared-and-embeddings",
		AWCoinRate:    AtomicAWCoinRate{RMBPerAWCoin: 0.0001, USDPerAWCoin: 0.00014},
		Capabilities: []AtomicCapability{{
			ID: "ap-ltx-2.5", Code: "LTX-2.5", Name: "LTX 2.5", AdapterCode: "token_market_shared",
			EndpointType: string(constant.EndpointTypeOpenAIVideo), TaskKind: "video_generation",
			Execution: AtomicExecution{Protocol: "shared_task", Path: "/shared-tasks/tasks"},
			Pricing: AtomicPricing{
				PricingModel: "per_unit", Currency: "awcoin", Enabled: true,
				ChargeConfig: map[string]any{"amountAwcoin": 4000.0, "unit": "second"},
			},
		}},
		Models: []AtomicModel{{
			ID: "ap-qwen3-embedding:0.6b", Execution: AtomicExecution{Protocol: "openai", Path: "/v1/embeddings"},
			Pricing: AtomicPricing{
				PricingModel: "per_token", Currency: "awcoin", Enabled: true,
				PromptPerMillion: 300, CompletionPerMillion: 0,
				CacheReadPerMillion: &zero, CacheWritePerMillion: &zero,
			},
			Protocols: []string{"embeddings"},
		}},
	}

	require.NoError(t, catalog.Validate())
	require.Equal(t, []constant.EndpointType{constant.EndpointTypeEmbeddings},
		catalog.ModelEndpointTypes()["ap-qwen3-embedding:0.6b"])
}

func TestAtomicCatalogAcceptsSystemOneProtocol(t *testing.T) {
	zero := 0.0
	catalog := AtomicCatalog{
		SchemaVersion: 2,
		Revision:      "decision-model",
		AWCoinRate:    AtomicAWCoinRate{RMBPerAWCoin: 0.0001, USDPerAWCoin: 0.00014},
		Models: []AtomicModel{{
			ID: "jev", Execution: AtomicExecution{Protocol: "systemone", Path: "/v1/systemone"},
			Pricing: AtomicPricing{
				PricingModel: "per_token", Currency: "awcoin", Enabled: true,
				PromptPerMillion: 5000, CompletionPerMillion: 5000,
				CacheReadPerMillion: &zero, CacheWritePerMillion: &zero,
			},
			Protocols: []string{"systemone"},
			Features: &AtomicFeatures{ByProtocol: map[string]AtomicProtocolCapabilities{
				"systemone": {InputModalities: []string{"text"}},
			}},
		}},
	}

	require.NoError(t, catalog.Validate())
	require.Equal(t, []constant.EndpointType{constant.EndpointTypeSystemOne},
		catalog.ModelEndpointTypes()["jev"])
}

// The catalog labels image-to-image as image-generation and routes both it and
// ASR through the shared-task API, but the upstream answers them synchronously
// on their media endpoints. The runtime capability must reflect the endpoint
// the gateway actually calls.
func TestAtomicCatalogMarksSynchronousTaskKinds(t *testing.T) {
	catalog := AtomicCatalog{
		SchemaVersion: 1,
		Revision:      "synchronous-media",
		AWCoinRate:    AtomicAWCoinRate{RMBPerAWCoin: 0.0001, USDPerAWCoin: 0.00014},
		Capabilities: []AtomicCapability{
			{
				ID: "ap-qwen-image-edit", Code: "QWEN-IMAGE-EDIT", AdapterCode: "token_market_shared",
				EndpointType: string(constant.EndpointTypeImageGeneration), TaskKind: "image_to_image",
				Execution: AtomicExecution{Protocol: "shared_task", Path: "/shared-tasks/tasks"},
				Pricing:   AtomicPricing{PricingModel: "per_call", Currency: "awcoin", Enabled: true, ChargeConfig: map[string]any{"amountAwcoin": 10000.0, "unit": "image"}},
			},
			{
				ID: "ap-asr", Code: "ASR", AdapterCode: "token_market_shared",
				EndpointType: string(constant.EndpointTypeAudioTranscriptions), TaskKind: "speech_to_text",
				Execution: AtomicExecution{Protocol: "shared_task", Path: "/shared-tasks/tasks"},
				Pricing:   AtomicPricing{PricingModel: "per_unit", Currency: "awcoin", Enabled: true, ChargeConfig: map[string]any{"amountAwcoin": 100.0, "unit": "second"}},
			},
		},
	}
	require.NoError(t, catalog.Validate())

	capabilities := catalog.RuntimeCapabilities()
	require.Len(t, capabilities, 2)

	edits := capabilities[0]
	require.Equal(t, "ap-qwen-image-edit", edits.ModelName)
	require.True(t, edits.Synchronous)
	require.Equal(t, "/v1/images/edits", edits.ExecutionPath)
	require.Equal(t, []constant.EndpointType{constant.EndpointTypeImageEdits}, edits.EndpointTypes)

	transcription := capabilities[1]
	require.Equal(t, "ap-asr", transcription.ModelName)
	require.True(t, transcription.Synchronous)
	require.Equal(t, "/v1/audio/transcriptions", transcription.ExecutionPath)
	require.Equal(t, []constant.EndpointType{constant.EndpointTypeAudioTranscriptions}, transcription.EndpointTypes)
	require.Equal(t, constant.AIPDDBillingTypeDurationSeconds, transcription.BillingType)
}

// chargeConfig.amountAwcoin is only the cheapest tier; the retail tiers are
// published separately in RMB and must reach the billing path in AWCoin.
func TestAtomicCatalogConvertsImagePricingTiers(t *testing.T) {
	catalog := AtomicCatalog{
		SchemaVersion: 1,
		Revision:      "image-tiers",
		AWCoinRate:    AtomicAWCoinRate{RMBPerAWCoin: 0.0001, USDPerAWCoin: 0.00014},
		Capabilities: []AtomicCapability{{
			ID: "ap-image-2.0", Code: "IMAGE-2.0", AdapterCode: "token_market_media",
			EndpointType: string(constant.EndpointTypeImageGeneration), TaskKind: "image_generation",
			Execution: AtomicExecution{Protocol: "token_market_image", Path: "/v1/images/generations"},
			Pricing: AtomicPricing{
				PricingModel: "per_call", Currency: "awcoin", Enabled: true,
				ChargeConfig: map[string]any{"amountAwcoin": 2000.0, "unit": "image"},
				ImagePricing: &AtomicImagePricing{
					DefaultUnitRmb: 0.2,
					SupportsEdits:  true,
					Tiers: []AtomicImagePricingTier{
						{Match: map[string]any{"quality": "4K"}, UnitRmb: 0.6},
						{Match: map[string]any{"size": "1024x1024"}, UnitRmb: 0.3},
					},
				},
			},
		}},
	}
	require.NoError(t, catalog.Validate())

	capability := catalog.RuntimeCapabilities()[0]
	require.Equal(t, []constant.EndpointType{constant.EndpointTypeImageGeneration, constant.EndpointTypeImageEdits}, capability.EndpointTypes)
	require.True(t, capability.Synchronous)

	imagePricing := capability.ImagePricing
	require.NotNil(t, imagePricing)
	require.InDelta(t, 2000.0, imagePricing.DefaultUnitAWCoin, 1e-6)
	require.Len(t, imagePricing.Tiers, 2)
	require.Equal(t, map[string]string{"quality": "4K"}, imagePricing.Tiers[0].Match)
	require.InDelta(t, 6000.0, imagePricing.Tiers[0].UnitAWCoin, 1e-6)
	require.Equal(t, map[string]string{"size": "1024x1024"}, imagePricing.Tiers[1].Match)
	require.InDelta(t, 3000.0, imagePricing.Tiers[1].UnitAWCoin, 1e-6)
	// The floor price only covers the cheapest tier, so a 4K request has to be
	// billed three times the synced per-call price.
	require.InDelta(t, 3.0, imagePricing.RatioFor("", "4096x4096"), 1e-6)
	require.InDelta(t, 1.5, imagePricing.RatioFor("", "1024x1024"), 1e-6)
	require.InDelta(t, 1.0, imagePricing.RatioFor("", ""), 1e-6)

	// Runtime lookup is the single source of truth for both pricing and the
	// stored model metadata.
	constant.SetAIPDDCapabilities([]constant.AIPDDCapability{capability})
	t.Cleanup(constant.ResetAIPDDCapabilities)
	require.Equal(t, []constant.EndpointType{constant.EndpointTypeImageGeneration, constant.EndpointTypeImageEdits},
		constant.GetAIPDDEndpointTypes("ap-image-2.0"))
}
