package model

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/pkg/aipddcatalog"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/require"
)

// aipddASRCatalog publishes the three shapes the importer has to tell apart: an
// ASR model billed per processed second on a synchronous endpoint, a video model
// billed per generated second through the task API, and audio billed per call.
func aipddASRCatalog(revision string) aipddcatalog.AtomicCatalog {
	return aipddcatalog.AtomicCatalog{
		SchemaVersion: 2,
		Revision:      revision,
		AWCoinRate: aipddcatalog.AtomicAWCoinRate{
			RMBPerAWCoin: 0.0001, USDPerAWCoin: 0.00015, UpdatedAt: "2026-10-01T00:00:00",
		},
		Capabilities: []aipddcatalog.AtomicCapability{
			{
				ID: "ap-funasr", Code: "ap-funasr", Name: "AP FunASR", AdapterCode: "token_market_shared",
				EndpointType: "audio-transcriptions", TaskKind: "speech_to_text", Available: aipddcatalog.BoolPtr(true),
				Execution: aipddcatalog.AtomicExecution{Protocol: "shared_task", Path: "/shared-tasks/tasks"},
				Pricing: aipddcatalog.AtomicPricing{
					PricingModel: "per_unit", Currency: "awcoin", Enabled: true,
					ChargeConfig: map[string]any{"amountAwcoin": float64(100), "unit": "second"},
				},
			},
			{
				ID: "ap-ltx-2.5", Code: "ap-ltx-2.5", Name: "AP LTX", AdapterCode: "token_market_media",
				EndpointType: "video-generations", TaskKind: "text_to_video", Available: aipddcatalog.BoolPtr(true),
				Execution: aipddcatalog.AtomicExecution{Protocol: "shared_task", Path: "/shared-tasks/tasks"},
				Pricing: aipddcatalog.AtomicPricing{
					PricingModel: "per_unit", Currency: "awcoin", Enabled: true,
					ChargeConfig: map[string]any{"amountAwcoin": float64(600), "unit": "second"},
				},
			},
			{
				ID: "ap-indextts-2", Code: "ap-indextts-2", Name: "AP IndexTTS", AdapterCode: "token_market_audio",
				EndpointType: "audio-speech", TaskKind: "text_to_speech", Available: aipddcatalog.BoolPtr(true),
				Execution: aipddcatalog.AtomicExecution{Protocol: "shared_task", Path: "/shared-tasks/tasks"},
				Pricing: aipddcatalog.AtomicPricing{
					PricingModel: "per_call", Currency: "awcoin", Enabled: true,
					ChargeConfig: map[string]any{"amountAwcoin": float64(1000)},
				},
			},
		},
	}
}

// The three predicates decide which importer owns a capability and whether it has
// a wholesale price to resell. They are the whole routing table of the pricing
// pipeline, and the "availability is irrelevant" rule lives here, so both are
// pinned down rather than left to be re-derived from the importers.
func TestAIPDDCapabilityPricingPredicates(t *testing.T) {
	duration := func(adapter, pricingModel string) aipddcatalog.AtomicCapability {
		capability := aipddcatalog.AtomicCapability{AdapterCode: adapter}
		capability.Pricing.PricingModel = pricingModel
		return capability
	}
	priceable := func(enabled bool, currency, basis string, available *bool) aipddcatalog.AtomicCapability {
		capability := aipddcatalog.AtomicCapability{Available: available}
		capability.Pricing = aipddcatalog.AtomicPricing{
			Enabled: enabled, Currency: currency, PricingBasis: basis,
		}
		return capability
	}
	available := aipddcatalog.BoolPtr(true)
	unavailable := aipddcatalog.BoolPtr(false)

	require.True(t, isTokenMarketDurationCapability(duration(" token_market_media ", "PER_SECOND")))
	require.False(t, isTokenMarketDurationCapability(duration("token_market_media", "per_call")))
	require.False(t, isTokenMarketDurationCapability(duration("token_market_shared", "per_second")),
		"the shared adapter is priced per call, and the shared per-second models belong to the per-second importers")
	require.False(t, isTokenMarketDurationCapability(duration("comfyui", "per_second")))

	// Upstream availability never decides whether a model gets a retail price.
	for _, flag := range []*bool{available, unavailable, nil} {
		require.True(t, tokenMarketCapabilityPriceable(priceable(true, " AWCoin ", " Display ", flag)))
		require.True(t, tokenMarketFixedCapabilityPriceable(priceable(true, "awcoin", "display", flag)))
	}
	require.True(t, tokenMarketFixedCapabilityPriceable(priceable(true, "awcoin", "", available)),
		"a schema v1 per-call row may omit pricingBasis")
	require.False(t, tokenMarketCapabilityPriceable(priceable(true, "awcoin", "", available)),
		"a duration model without a display basis has no per-second price to tier by")
	require.False(t, tokenMarketCapabilityPriceable(priceable(false, "awcoin", "display", available)))
	require.False(t, tokenMarketCapabilityPriceable(priceable(true, "cny", "display", available)))
	require.False(t, tokenMarketCapabilityPriceable(priceable(true, "awcoin", "actual", available)))
	require.False(t, tokenMarketFixedCapabilityPriceable(priceable(true, "awcoin", "actual", available)))
}

func TestSyncAIPDDSynchronousPerSecondPricing(t *testing.T) {
	truncateTables(t)
	preserveAIPDDPricingRuntime(t)
	t.Setenv("AIPDD_PRICING_MARKUP", "1.1")
	originalRate := operation_setting.USDExchangeRate
	operation_setting.USDExchangeRate = 2
	t.Cleanup(func() { operation_setting.USDExchangeRate = originalRate })

	updated, updates, err := syncAIPDDSynchronousPerSecondPricingTx(DB, aipddASRCatalog("revision-1"))
	require.NoError(t, err)
	require.Equal(t, 1, updated)
	require.Len(t, updates, 3)
	require.Contains(t, updates, aipddBillingModeOptionKey)
	require.Contains(t, updates, aipddTaskPricingOptionKey)
	require.Contains(t, updates, aipddSyncedPricesOptionKey)

	pricing, err := loadAIPDDTaskPricingTx(DB)
	require.NoError(t, err)
	// 100 awcoin/s at 0.0001 RMB per awcoin over 2 RMB per USD, plus the 10%
	// default retail margin.
	asr, ok := pricing["ap-funasr"]
	require.True(t, ok, "the synchronous ASR model must be imported")
	require.Equal(t, billing_setting.TaskPricingUnitSecond, asr.Unit)
	require.InDelta(t, 0.0055, asr.NoReferenceVideoUnitPrice, 1e-9)
	require.Nil(t, asr.ByResolution, "a synchronous call has no resolution to tier by")

	require.NotContains(t, pricing, "ap-ltx-2.5", "task models keep their own pricing pipeline")
	require.NotContains(t, pricing, "ap-indextts-2")

	modes, err := loadAIPDDBillingModesTx(DB)
	require.NoError(t, err)
	require.Equal(t, billing_setting.BillingModeTaskPricing, modes["ap-funasr"])

	// Re-applying the same catalog must not report changes.
	updated, updates, err = syncAIPDDSynchronousPerSecondPricingTx(DB, aipddASRCatalog("revision-2"))
	require.NoError(t, err)
	require.Zero(t, updated)
	require.Nil(t, updates)

	// A price change is reported and written.
	changed := aipddASRCatalog("revision-3")
	changed.Capabilities[0].Pricing.ChargeConfig["amountAwcoin"] = float64(120)
	updated, _, err = syncAIPDDSynchronousPerSecondPricingTx(DB, changed)
	require.NoError(t, err)
	require.Equal(t, 1, updated)
	pricing, err = loadAIPDDTaskPricingTx(DB)
	require.NoError(t, err)
	require.InDelta(t, 0.0066, pricing["ap-funasr"].NoReferenceVideoUnitPrice, 1e-9)
}

func TestSyncAIPDDAsyncPerSecondPricing(t *testing.T) {
	truncateTables(t)
	preserveAIPDDPricingRuntime(t)
	t.Setenv("AIPDD_PRICING_MARKUP", "1.1")
	originalRate := operation_setting.USDExchangeRate
	operation_setting.USDExchangeRate = 2
	t.Cleanup(func() { operation_setting.USDExchangeRate = originalRate })

	updated, updates, err := syncAIPDDAsyncPerSecondPricingTx(DB, aipddASRCatalog("revision-1"))
	require.NoError(t, err)
	require.Equal(t, 1, updated)
	require.Len(t, updates, 3)
	require.Contains(t, updates, aipddBillingModeOptionKey)
	require.Contains(t, updates, aipddTaskPricingOptionKey)
	require.Contains(t, updates, aipddSyncedPricesOptionKey)

	pricing, err := loadAIPDDTaskPricingTx(DB)
	require.NoError(t, err)
	// 600 awcoin/s at 0.0001 RMB per awcoin over 2 RMB per USD, plus the 10%
	// default retail margin.
	ltx, ok := pricing["ap-ltx-2.5"]
	require.True(t, ok, "the async video model must be imported")
	require.Equal(t, billing_setting.TaskPricingUnitSecond, ltx.Unit)
	require.InDelta(t, 0.033, ltx.NoReferenceVideoUnitPrice, 1e-9)

	require.NotContains(t, pricing, "ap-funasr", "synchronous models belong to the sync importer")
	require.NotContains(t, pricing, "ap-indextts-2")

	modes, err := loadAIPDDBillingModesTx(DB)
	require.NoError(t, err)
	require.Equal(t, billing_setting.BillingModeTaskPricing, modes["ap-ltx-2.5"])

	// Re-applying the same catalog must not report changes.
	updated, updates, err = syncAIPDDAsyncPerSecondPricingTx(DB, aipddASRCatalog("revision-2"))
	require.NoError(t, err)
	require.Zero(t, updated)
	require.Nil(t, updates)

	// A price change is reported and written.
	changed := aipddASRCatalog("revision-3")
	changed.Capabilities[1].Pricing.ChargeConfig["amountAwcoin"] = float64(720)
	updated, _, err = syncAIPDDAsyncPerSecondPricingTx(DB, changed)
	require.NoError(t, err)
	require.Equal(t, 1, updated)
	pricing, err = loadAIPDDTaskPricingTx(DB)
	require.NoError(t, err)
	require.InDelta(t, 0.0396, pricing["ap-ltx-2.5"].NoReferenceVideoUnitPrice, 1e-9)
}

func TestSyncAIPDDSynchronousPerSecondPricingWithoutSynchronousModels(t *testing.T) {
	truncateTables(t)
	preserveAIPDDPricingRuntime(t)

	updated, updates, err := syncAIPDDSynchronousPerSecondPricingTx(DB, aipddTestCatalog("revision-1", "task-old", "llm-old"))
	require.NoError(t, err)
	require.Zero(t, updated)
	require.Nil(t, updates)
}

func TestSyncAIPDDSynchronousPerSecondPricingRequiresUsableExchangeRate(t *testing.T) {
	truncateTables(t)
	preserveAIPDDPricingRuntime(t)
	originalRate := operation_setting.USDExchangeRate
	operation_setting.USDExchangeRate = 0
	t.Cleanup(func() { operation_setting.USDExchangeRate = originalRate })

	// Without a finite exchange rate the awcoin price cannot become a retail
	// price, and importing nothing is safer than importing free transcriptions.
	updated, updates, err := syncAIPDDSynchronousPerSecondPricingTx(DB, aipddASRCatalog("revision-1"))
	require.Error(t, err)
	require.Zero(t, updated)
	require.Nil(t, updates)

	pricing, err := loadAIPDDTaskPricingTx(DB)
	require.NoError(t, err)
	require.Empty(t, pricing)
}

// aipddTokenMarketCatalog publishes a correctly tiered video model next to one
// whose upstream display price is missing, the shape a broken catalog row takes.
func aipddTokenMarketCatalog(revision string) aipddcatalog.AtomicCatalog {
	return aipddcatalog.AtomicCatalog{
		SchemaVersion: 2,
		Revision:      revision,
		AWCoinRate: aipddcatalog.AtomicAWCoinRate{
			RMBPerAWCoin: 0.01, USDPerAWCoin: 0.0015, UpdatedAt: "2026-10-01T00:00:00",
		},
		Capabilities: []aipddcatalog.AtomicCapability{
			{
				ID: "ap-seedance-1-pro", Code: "ap-seedance-1-pro", Name: "AP Seedance",
				AdapterCode: "token_market_media", EndpointType: "video-generations",
				TaskKind: "text_to_video", Available: aipddcatalog.BoolPtr(true),
				Execution: aipddcatalog.AtomicExecution{Protocol: "shared_task", Path: "/shared-tasks/tasks"},
				Pricing: aipddcatalog.AtomicPricing{
					PricingModel: "per_second", Currency: "awcoin", PricingBasis: "display", Enabled: true,
					ByResolution: map[string]constant.AIPDDSeedanceResolutionPricing{
						"720p":  {TargetResolution: "720p", DisplayAmountAWCoinPerSecond: aipddRetailFloatPtr(100)},
						"1080p": {TargetResolution: "1080p", DisplayAmountAWCoinPerSecond: aipddRetailFloatPtr(200)},
					},
				},
			},
			{
				ID: "ap-broken-tier", Code: "ap-broken-tier", Name: "AP Broken Tier",
				AdapterCode: "token_market_media", EndpointType: "video-generations",
				TaskKind: "text_to_video", Available: aipddcatalog.BoolPtr(true),
				Execution: aipddcatalog.AtomicExecution{Protocol: "shared_task", Path: "/shared-tasks/tasks"},
				Pricing: aipddcatalog.AtomicPricing{
					PricingModel: "per_second", Currency: "awcoin", PricingBasis: "display", Enabled: true,
					ByResolution: map[string]constant.AIPDDSeedanceResolutionPricing{
						// The upstream published this tier without a display price.
						"4k": {TargetResolution: "4k"},
					},
				},
			},
		},
	}
}

func TestSyncAIPDDTokenMarketTaskPricingTxSkipsUnusableCapability(t *testing.T) {
	truncateTables(t)
	preserveAIPDDPricingRuntime(t)
	t.Setenv("AIPDD_PRICING_MARKUP", "1.1")
	originalRate := operation_setting.USDExchangeRate
	operation_setting.USDExchangeRate = 2
	t.Cleanup(func() { operation_setting.USDExchangeRate = originalRate })

	// One malformed model row must not cost the gateway the prices of every
	// other model in the same catalog revision.
	updated, _, err := syncAIPDDTokenMarketTaskPricingTx(DB, aipddTokenMarketCatalog("revision-1"))
	require.NoError(t, err)
	require.Equal(t, 1, updated)

	pricing, err := loadAIPDDTaskPricingTx(DB)
	require.NoError(t, err)
	require.NotContains(t, pricing, "ap-broken-tier", "an unusable display price is skipped, not provisioned")

	seedance, ok := pricing["ap-seedance-1-pro"]
	require.True(t, ok, "the healthy model must still be imported")
	require.Equal(t, billing_setting.TaskPricingUnitSecond, seedance.Unit)
	require.Len(t, seedance.ByResolution, 2)
	// 100 awcoin/s at 0.01 RMB per awcoin over 2 RMB per USD, plus the 10% margin.
	require.InDelta(t, 0.55, seedance.ByResolution["720p"].NoReferenceVideoUnitPrice, 1e-9)
	require.InDelta(t, 1.1, seedance.ByResolution["1080p"].NoReferenceVideoUnitPrice, 1e-9)

	modes, err := loadAIPDDBillingModesTx(DB)
	require.NoError(t, err)
	require.Equal(t, billing_setting.BillingModeTaskPricing, modes["ap-seedance-1-pro"])
	require.NotContains(t, modes, "ap-broken-tier")
}
