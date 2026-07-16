package model

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/pkg/aipddcatalog"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/config"
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

func TestPreviousAIPDDCatalogModelsUsesManagedChannelForLegacySnapshot(t *testing.T) {
	truncateTables(t)
	channel := Channel{
		Type: constant.ChannelTypeAIPDD, Name: aipddEnvChannelName, Key: "sk-test",
		Group: "default", Models: "legacy-task,legacy-llm", Status: 1,
	}
	require.NoError(t, DB.Create(&channel).Error)
	require.NoError(t, DB.Create(&AIPDDCatalogSnapshot{
		ID: aipddCatalogSnapshotID, SchemaVersion: 1, Revision: "legacy-revision",
		SourceBaseURL: "https://aipdd.example",
		Payload:       `{"schemaVersion":1,"revision":"legacy-revision","awcoinRate":{"rmbPerAwcoin":0.01,"usdPerAwcoin":0.001},"capabilities":[{"id":"AP Seedance","adapterCode":"seedance","execution":{"protocol":"seedance_official","path":"/api/v3/contents/generations/tasks"},"pricing":{"pricingModel":"per_second","currency":"awcoin","enabled":true,"byResolution":{"720p":{"defaultDurationSeconds":5,"defaultFramesPerSecond":24,"priceVariants":[{"hasReferenceVideo":false,"amountAwcoinPerSecond":10}]}}}}],"models":[]}`,
	}).Error)

	models, err := previousAIPDDCatalogModels("https://aipdd.example")
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"legacy-task", "legacy-llm"}, models)
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
	firstResult, err := applyAIPDDCatalog(first, "https://aipdd.example", "sk-test")
	require.NoError(t, err)
	require.Zero(t, firstResult.UpdatedPrices)

	preserveAIPDDPricingRuntime(t)
	localPricing := map[string]string{
		"ModelPrice":                   `{"task-old":1.25,"llm-old":2.5,"unrelated-model":3.75}`,
		"ModelRatio":                   `{"task-old":4.25,"llm-old":5.5,"unrelated-model":6.75}`,
		"billing_setting.billing_mode": `{"task-old":"task_pricing","llm-old":"tiered_expr","unrelated-model":"ratio"}`,
		"billing_setting.billing_expr": `{"llm-old":"tier(\"local\", p * 1 + c * 2)"}`,
		"billing_setting.task_pricing": `{"task-old":{"unit":"second","no_reference_video_unit_price":0.12,"reference_video_policy":"same"}}`,
	}
	for key, value := range localPricing {
		require.NoError(t, UpdateOption(key, value))
	}

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
	require.Zero(t, result.UpdatedPrices)

	var staleModels, staleAbilities int64
	require.NoError(t, DB.Unscoped().Model(&Model{}).Where("model_name IN ?", []string{"task-old", "llm-old"}).Count(&staleModels).Error)
	require.NoError(t, DB.Model(&Ability{}).Where("model IN ?", []string{"task-old", "llm-old"}).Count(&staleAbilities).Error)
	require.Zero(t, staleModels)
	require.Zero(t, staleAbilities)

	var managed Channel
	require.NoError(t, DB.Where("type = ? AND name = ?", constant.ChannelTypeAIPDD, "AIPDD").First(&managed).Error)
	require.Equal(t, "llm-new,task-new", managed.Models)

	for key, expected := range localPricing {
		var option Option
		require.NoError(t, DB.Where("key = ?", key).First(&option).Error)
		require.JSONEq(t, expected, option.Value, key)
	}
	price, ok := ratio_setting.GetModelPrice("task-old", false)
	require.True(t, ok)
	require.Equal(t, 1.25, price)
	ratio, ok, _ := ratio_setting.GetModelRatio("task-old")
	require.True(t, ok)
	require.Equal(t, 4.25, ratio)
	require.Equal(t, "task_pricing", billing_setting.GetBillingMode("task-old"))
	require.Equal(t, "tier(\"local\", p * 1 + c * 2)", mustAIPDDBillingExpr(t, "llm-old"))
}

func TestApplyAIPDDCatalogDoesNotCreatePricingOptions(t *testing.T) {
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
					TargetResolution:          "1080p",
					DefaultDurationSeconds:    5,
					DefaultFramesPerSecond:    24,
					AmountAWCoinPerSecond:     40.1,
					TextInputAWCoinPerSecond:  40.1,
					ImageInputAWCoinPerSecond: 40.1,
					VideoInputAWCoinPerSecond: 30,
					AudioInputAWCoinPerSecond: 40.1,
				},
			},
		},
	}}

	result, err := applyAIPDDCatalog(catalog, "https://aipdd.example", "sk-test")
	require.NoError(t, err)
	require.Zero(t, result.UpdatedPrices)

	for _, key := range []string{
		"ModelPrice",
		"ModelRatio",
		"billing_setting.billing_mode",
		"billing_setting.billing_expr",
		"billing_setting.task_pricing",
	} {
		var count int64
		require.NoError(t, DB.Model(&Option{}).Where("key = ?", key).Count(&count).Error)
		require.Zero(t, count, key)
	}

	capability, ok := constant.GetAIPDDCapability("AP Seedance")
	require.True(t, ok)
	require.NotNil(t, capability.SeedancePricing)
	require.Contains(t, capability.SeedancePricing.ByResolution, "1080p")
}

func TestEnsureAIPDDOpenAIModelDefaultsDoesNotCreateLocalPricing(t *testing.T) {
	truncateTables(t)
	constant.ResetAIPDDCapabilities()
	constant.ResetAIPDDOpenAIModels()
	t.Cleanup(func() {
		constant.ResetAIPDDCapabilities()
		constant.ResetAIPDDOpenAIModels()
	})

	require.NoError(t, EnsureAIPDDOpenAIModelDefaults([]string{"aipdd-local-price-boundary-test"}))

	for _, key := range []string{
		"ModelPrice",
		"ModelRatio",
		"billing_setting.billing_mode",
		"billing_setting.billing_expr",
		"billing_setting.task_pricing",
	} {
		var count int64
		require.NoError(t, DB.Model(&Option{}).Where("key = ?", key).Count(&count).Error)
		require.Zero(t, count, key)
	}
}

func preserveAIPDDPricingRuntime(t *testing.T) {
	t.Helper()
	modelPrice := ratio_setting.ModelPrice2JSONString()
	modelRatio := ratio_setting.ModelRatio2JSONString()
	billingConfig := make(map[string]string)
	for key, value := range config.GlobalConfig.ExportAllConfigs() {
		if len(key) >= len("billing_setting.") && key[:len("billing_setting.")] == "billing_setting." {
			billingConfig[key] = value
		}
	}
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(modelPrice))
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(modelRatio))
		require.NoError(t, config.GlobalConfig.LoadFromDB(billingConfig))
	})
}

func mustAIPDDBillingExpr(t *testing.T, modelName string) string {
	t.Helper()
	expr, ok := billing_setting.GetBillingExpr(modelName)
	require.True(t, ok)
	return expr
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
