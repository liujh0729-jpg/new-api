package model

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/pkg/aipddcatalog"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/stretchr/testify/require"
)

func TestSyncAIPDDCatalogFirstInstallFailureHasNoPartialData(t *testing.T) {
	truncateTables(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	_, err := SyncAIPDDCatalog(context.Background(), server.Client(), server.URL, "sk-test")
	require.Error(t, err)
	require.Contains(t, err.Error(), "without a snapshot")

	var channelCount, snapshotCount int64
	require.NoError(t, DB.Model(&Channel{}).Count(&channelCount).Error)
	require.NoError(t, DB.Model(&AIPDDCatalogSnapshot{}).Count(&snapshotCount).Error)
	require.Zero(t, channelCount)
	require.Zero(t, snapshotCount)
}

func TestSyncAIPDDCatalogFallsBackOnlyToSameOriginSnapshot(t *testing.T) {
	truncateTables(t)
	t.Cleanup(func() {
		constant.ResetAIPDDCapabilities()
		constant.ResetAIPDDOpenAIModels()
	})
	fail := false
	catalog := aipddTestCatalog("revision-1", "task-a", "llm-a")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if fail {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
			return
		}
		body, err := aipddcatalog.MarshalAtomic(catalog)
		require.NoError(t, err)
		_, _ = w.Write([]byte(`{"code":0,"message":"ok","data":` + string(body) + `}`))
	}))
	defer server.Close()

	first, err := SyncAIPDDCatalog(context.Background(), server.Client(), server.URL, "sk-test")
	require.NoError(t, err)
	require.False(t, first.UsedSnapshot)
	fail = true
	fallback, err := SyncAIPDDCatalog(context.Background(), server.Client(), server.URL, "sk-test")
	require.NoError(t, err)
	require.True(t, fallback.UsedSnapshot)
	require.Equal(t, "revision-1", fallback.Revision)

	_, err = SyncAIPDDCatalog(context.Background(), server.Client(), server.URL+"/other", "sk-test")
	require.Error(t, err)
	require.Contains(t, err.Error(), "different base URL")
}

func TestApplyAIPDDCatalogReplacesModelsAndCleansOnlySeededCNChannels(t *testing.T) {
	truncateTables(t)
	t.Cleanup(func() {
		constant.ResetAIPDDCapabilities()
		constant.ResetAIPDDOpenAIModels()
	})
	require.NoError(t, EnsureCNProviderDefaults())
	custom := Channel{Type: constant.ChannelTypeAli, Name: "用户自建阿里渠道", Key: "custom", Group: "default", Models: "custom-model", Status: 1}
	require.NoError(t, DB.Create(&custom).Error)

	first := aipddTestCatalog("revision-1", "task-old", "llm-old")
	_, err := applyAIPDDCatalog(first, "https://aipdd.example", "sk-test")
	require.NoError(t, err)

	for _, provider := range cnProviders {
		var count int64
		require.NoError(t, DB.Model(&Channel{}).Where("type = ? AND name = ?", provider.ChannelType, provider.Name).Count(&count).Error)
		require.Zero(t, count)
	}
	var customCount int64
	require.NoError(t, DB.Model(&Channel{}).Where("id = ?", custom.Id).Count(&customCount).Error)
	require.EqualValues(t, 1, customCount)

	second := aipddTestCatalog("revision-2", "task-new", "llm-new")
	result, err := applyAIPDDCatalog(second, "https://aipdd.example", "sk-test")
	require.NoError(t, err)
	require.Equal(t, 2, result.AddedModels)
	require.Equal(t, 2, result.RemovedModels)

	var staleModels, staleAbilities int64
	require.NoError(t, DB.Unscoped().Model(&Model{}).Where("model_name IN ?", []string{"task-old", "llm-old"}).Count(&staleModels).Error)
	require.NoError(t, DB.Model(&Ability{}).Where("model IN ?", []string{"task-old", "llm-old"}).Count(&staleAbilities).Error)
	require.Zero(t, staleModels)
	require.Zero(t, staleAbilities)

	var managed Channel
	require.NoError(t, DB.Where("type = ? AND name = ?", constant.ChannelTypeAIPDD, "AIPDD").First(&managed).Error)
	require.Equal(t, "llm-new,task-new", managed.Models)
}

func TestApplyAIPDDCatalogConfiguresSeedanceBasePrice(t *testing.T) {
	truncateTables(t)
	t.Cleanup(func() {
		constant.ResetAIPDDCapabilities()
		constant.ResetAIPDDOpenAIModels()
	})

	catalog := aipddTestCatalog("seedance-price-revision", "task-model", "llm-model")
	catalog.Capabilities = []aipddcatalog.AtomicCapability{{
		ID: "AP Seedance", Code: "seedance", Name: "AP Seedance", AdapterCode: "seedance",
		EndpointType: "openai-video", TaskKind: "video_generation",
		Execution: aipddcatalog.AtomicExecution{Protocol: "seedance_official", Path: "/api/v3/contents/generations/tasks"},
		Pricing: aipddcatalog.AtomicPricing{
			PricingModel: "per_second", Currency: "awcoin", Enabled: true,
			ByResolution: map[string]constant.AIPDDSeedanceResolutionPricing{
				"1080p": {
					DefaultDurationSeconds: 5, DefaultFramesPerSecond: 24,
					PriceVariants: []constant.AIPDDSeedancePriceVariant{
						{HasReferenceVideo: false, AWCoinPerSecond: 40.1, MinimumAWCoin: 100.2},
						{HasReferenceVideo: true, AWCoinPerSecond: 30, MinimumAWCoin: 120.1},
					},
				},
			},
		},
	}}

	_, err := applyAIPDDCatalog(catalog, "https://aipdd.example", "sk-test")
	require.NoError(t, err)

	price, ok := ratio_setting.GetModelPrice("AP Seedance", false)
	require.True(t, ok)
	require.InDelta(t, 0.225, price, 0.0000001)

	capability, ok := constant.GetAIPDDCapability("AP Seedance")
	require.True(t, ok)
	require.NotNil(t, capability.SeedancePricing)
	require.Contains(t, capability.SeedancePricing.ByResolution, "1080p")
}

func aipddTestCatalog(revision, taskModel, llmModel string) aipddcatalog.AtomicCatalog {
	return aipddcatalog.AtomicCatalog{
		SchemaVersion: 1,
		Revision:      revision,
		AWCoinRate: aipddcatalog.AtomicAWCoinRate{
			RMBPerAWCoin: 0.01, USDPerAWCoin: 0.0015, UpdatedAt: "2026-07-12T10:00:00",
		},
		Capabilities: []aipddcatalog.AtomicCapability{{
			ID: taskModel, Code: taskModel, Name: taskModel, AdapterCode: "comfyui",
			EndpointType: "image-generation", TaskKind: "text_to_image",
			Execution: aipddcatalog.AtomicExecution{Protocol: "shared_task", Path: "/shared-tasks/tasks"},
			Pricing: aipddcatalog.AtomicPricing{
				PricingModel: "per_call", Currency: "awcoin", Enabled: true,
				ChargeConfig: map[string]any{"amountAwcoin": float64(100)},
			},
		}},
		Models: []aipddcatalog.AtomicModel{{
			ID: llmModel, Name: llmModel,
			Execution: aipddcatalog.AtomicExecution{Protocol: "openai", Path: "/v1/chat/completions"},
			Pricing: aipddcatalog.AtomicPricing{
				PricingModel: "per_token", Currency: "awcoin", Enabled: true,
				PromptPerMillion: 10, CompletionPerMillion: 30,
			},
		}},
	}
}
