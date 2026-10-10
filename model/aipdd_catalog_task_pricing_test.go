package model

import (
	"testing"

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

func TestSyncAIPDDSynchronousPerSecondPricing(t *testing.T) {
	truncateTables(t)
	preserveAIPDDPricingRuntime(t)
	originalRate := operation_setting.USDExchangeRate
	operation_setting.USDExchangeRate = 2
	t.Cleanup(func() { operation_setting.USDExchangeRate = originalRate })

	updated, updates, err := syncAIPDDSynchronousPerSecondPricingTx(DB, aipddASRCatalog("revision-1"))
	require.NoError(t, err)
	require.Equal(t, 1, updated)
	require.Len(t, updates, 2)
	require.Contains(t, updates, aipddBillingModeOptionKey)
	require.Contains(t, updates, aipddTaskPricingOptionKey)

	pricing, err := loadAIPDDTaskPricingTx(DB)
	require.NoError(t, err)
	// 100 awcoin/s at 0.0001 RMB per awcoin over 2 RMB per USD.
	asr, ok := pricing["ap-funasr"]
	require.True(t, ok, "the synchronous ASR model must be imported")
	require.Equal(t, billing_setting.TaskPricingUnitSecond, asr.Unit)
	require.InDelta(t, 0.005, asr.NoReferenceVideoUnitPrice, 1e-9)
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
	require.InDelta(t, 0.006, pricing["ap-funasr"].NoReferenceVideoUnitPrice, 1e-9)
}

func TestSyncAIPDDAsyncPerSecondPricing(t *testing.T) {
	truncateTables(t)
	preserveAIPDDPricingRuntime(t)
	originalRate := operation_setting.USDExchangeRate
	operation_setting.USDExchangeRate = 2
	t.Cleanup(func() { operation_setting.USDExchangeRate = originalRate })

	updated, updates, err := syncAIPDDAsyncPerSecondPricingTx(DB, aipddASRCatalog("revision-1"))
	require.NoError(t, err)
	require.Equal(t, 1, updated)
	require.Len(t, updates, 2)
	require.Contains(t, updates, aipddBillingModeOptionKey)
	require.Contains(t, updates, aipddTaskPricingOptionKey)

	pricing, err := loadAIPDDTaskPricingTx(DB)
	require.NoError(t, err)
	// 600 awcoin/s at 0.0001 RMB per awcoin over 2 RMB per USD.
	ltx, ok := pricing["ap-ltx-2.5"]
	require.True(t, ok, "the async video model must be imported")
	require.Equal(t, billing_setting.TaskPricingUnitSecond, ltx.Unit)
	require.InDelta(t, 0.03, ltx.NoReferenceVideoUnitPrice, 1e-9)

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
	require.InDelta(t, 0.036, pricing["ap-ltx-2.5"].NoReferenceVideoUnitPrice, 1e-9)
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
