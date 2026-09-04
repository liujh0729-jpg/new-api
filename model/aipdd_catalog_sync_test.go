package model

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/pkg/aipddcatalog"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/operation_setting"
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
		aipddcatalog.ResetV1ModelsListHidden()
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

func TestApplyAIPDDCatalogReplacesOnlyManagedAIPDDData(t *testing.T) {
	truncateTables(t)
	t.Cleanup(func() {
		constant.ResetAIPDDCapabilities()
		constant.ResetAIPDDOpenAIModels()
		aipddcatalog.ResetV1ModelsListHidden()
	})
	require.NoError(t, EnsureCNProviderDefaults())
	custom := Channel{Type: constant.ChannelTypeAli, Name: "用户自建阿里渠道", Key: "custom", Group: "default", Models: "custom-model", Status: 1}
	require.NoError(t, DB.Create(&custom).Error)
	const customSeedanceModel = "doubao-seedance-2-0-260128"
	customSeedance := Channel{
		Type: constant.ChannelTypeVolcEngine, Name: "Seedance-DO", Key: "custom-seedance",
		Group: "default", Models: customSeedanceModel, Status: 1,
	}
	require.NoError(t, DB.Create(&customSeedance).Error)
	require.NoError(t, customSeedance.AddAbilities(DB))

	first := aipddTestCatalog("revision-1", "task-old", "llm-old")
	firstResult, err := applyAIPDDCatalog(first, "https://aipdd.example", "sk-test")
	require.NoError(t, err)
	require.Zero(t, firstResult.UpdatedPrices)
	var firstManaged Channel
	require.NoError(t, DB.Where("type = ? AND name = ?", constant.ChannelTypeAIPDD, "AIPDD").First(&firstManaged).Error)
	require.Equal(t, "default", firstManaged.Group)
	require.NoError(t, DB.Model(&firstManaged).Update("group", "default,VIP1,VIP2,VIP3,VIP4,VIP5").Error)

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
		require.EqualValues(t, 1, count)
	}
	var customCount int64
	require.NoError(t, DB.Model(&Channel{}).Where("id = ?", custom.Id).Count(&customCount).Error)
	require.EqualValues(t, 1, customCount)
	var customSeedanceModelCount, customSeedanceAbilityCount int64
	require.NoError(t, DB.Model(&Model{}).Where("model_name = ?", customSeedanceModel).Count(&customSeedanceModelCount).Error)
	require.NoError(t, DB.Model(&Ability{}).Where("channel_id = ? AND model = ?", customSeedance.Id, customSeedanceModel).Count(&customSeedanceAbilityCount).Error)
	require.EqualValues(t, 1, customSeedanceModelCount)
	require.EqualValues(t, 1, customSeedanceAbilityCount)

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
	require.Equal(t, "default,VIP1,VIP2,VIP3,VIP4,VIP5", managed.Group)

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

func TestApplyAIPDDCatalogExcludesLegacySeedanceProxyCapabilities(t *testing.T) {
	truncateTables(t)
	t.Cleanup(func() {
		constant.ResetAIPDDCapabilities()
		constant.ResetAIPDDOpenAIModels()
		aipddcatalog.ResetV1ModelsListHidden()
	})

	displayAmount := 40.1
	displayVideoAmount := 30.0
	catalog := aipddTestCatalog("seedance-price-revision", "task-model", "llm-model")
	catalog.Capabilities = []aipddcatalog.AtomicCapability{{
		ID: "AP Seedance", Code: "seedance", Name: "AP Seedance", AdapterCode: "seedance",
		EndpointType: "openai-video", TaskKind: "video_generation", Available: aipddcatalog.BoolPtr(true),
		Execution: aipddcatalog.AtomicExecution{Protocol: "seedance_official", Path: "/api/v3/contents/generations/tasks"},
		Pricing: aipddcatalog.AtomicPricing{
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

	_, ok := constant.GetAIPDDCapability("AP Seedance")
	require.False(t, ok)
	var managed Channel
	require.NoError(t, DB.Where("type = ? AND name = ?", constant.ChannelTypeAIPDD, aipddEnvChannelName).First(&managed).Error)
	require.NotContains(t, managed.GetModels(), "AP Seedance")
	var modelCount, abilityCount int64
	require.NoError(t, DB.Unscoped().Model(&Model{}).Where("model_name = ?", "AP Seedance").Count(&modelCount).Error)
	require.NoError(t, DB.Model(&Ability{}).Where("model = ?", "AP Seedance").Count(&abilityCount).Error)
	require.Zero(t, modelCount)
	require.Zero(t, abilityCount)
	var snapshot AIPDDCatalogSnapshot
	require.NoError(t, DB.First(&snapshot, aipddCatalogSnapshotID).Error)
	var storedCatalog aipddcatalog.AtomicCatalog
	require.NoError(t, common.UnmarshalJsonStr(snapshot.Payload, &storedCatalog))
	require.Empty(t, storedCatalog.Capabilities)
}

func TestApplyAIPDDCatalogImportsTokenMarketDisplayPrices(t *testing.T) {
	truncateTables(t)
	t.Cleanup(func() {
		constant.ResetAIPDDCapabilities()
		constant.ResetAIPDDOpenAIModels()
		aipddcatalog.ResetV1ModelsListHidden()
	})
	preserveAIPDDPricingRuntime(t)
	previousExchangeRate := operation_setting.USDExchangeRate
	operation_setting.USDExchangeRate = 8
	t.Cleanup(func() { operation_setting.USDExchangeRate = previousExchangeRate })

	require.NoError(t, UpdateOption(
		aipddBillingModeOptionKey,
		`{"AP Seedance":"task_pricing","unrelated-model":"ratio"}`,
	))
	require.NoError(t, UpdateOption(
		aipddTaskPricingOptionKey,
		`{"AP Seedance":{"unit":"second","by_resolution":{"1080p":{"no_reference_video_unit_price":0.5,"reference_video_policy":"same"}}}}`,
	))

	display480p := 9.0
	display768p := 12.0
	display768pVideoInput := 15.0
	catalog := aipddTestCatalog("token-market-price-revision", "unused-task", "llm-model")
	catalog.AWCoinRate.RMBPerAWCoin = 0.01
	catalog.Capabilities = []aipddcatalog.AtomicCapability{{
		ID: "ap-minimax-h3-text-to-video", Code: "ap-minimax-h3-text-to-video",
		Name: "MiniMax H3 Text to Video", AdapterCode: "token_market_media",
		EndpointType: "openai-video", TaskKind: "video_generation", Available: aipddcatalog.BoolPtr(true),
		Execution: aipddcatalog.AtomicExecution{Protocol: "token_market_video", Path: "/v1/videos"},
		Pricing: aipddcatalog.AtomicPricing{
			PricingModel: "per_second", Currency: "awcoin", PricingBasis: "display", Enabled: true,
			ByResolution: map[string]constant.AIPDDSeedanceResolutionPricing{
				"480p": {
					TargetResolution: "480p", DisplayAmountAWCoinPerSecond: &display480p,
					DisplayVideoInputAWCoinPerSecond: &display480p,
					DefaultDurationSeconds:           1, DefaultFramesPerSecond: 24,
				},
				"768p": {
					TargetResolution: "768p", DisplayAmountAWCoinPerSecond: &display768p,
					DisplayVideoInputAWCoinPerSecond: &display768pVideoInput,
					DefaultDurationSeconds:           1, DefaultFramesPerSecond: 24,
				},
			},
		},
	}}

	result, err := applyAIPDDCatalog(catalog, "https://aipdd.example", "sk-test")
	require.NoError(t, err)
	require.Equal(t, 1, result.UpdatedPrices)
	require.Equal(t, billing_setting.BillingModeTaskPricing,
		billing_setting.GetBillingMode("ap-minimax-h3-text-to-video"))

	config, ok := billing_setting.GetTaskPricing("ap-minimax-h3-text-to-video")
	require.True(t, ok)
	require.Equal(t, billing_setting.TaskPricingUnitSecond, config.Unit)
	require.InDelta(t, 0.01125, config.ByResolution["480p"].NoReferenceVideoUnitPrice, 1e-12)
	require.Equal(t, billing_setting.ReferenceVideoPolicySame,
		config.ByResolution["480p"].ReferenceVideoPolicy)
	require.InDelta(t, 0.015, config.ByResolution["768p"].NoReferenceVideoUnitPrice, 1e-12)
	require.Equal(t, billing_setting.ReferenceVideoPolicyCustom,
		config.ByResolution["768p"].ReferenceVideoPolicy)
	require.InDelta(t, 0.01875, config.ByResolution["768p"].ReferenceVideoUnitPrice, 1e-12)
	pricingVisible := false
	for _, item := range GetPricing() {
		if item.ModelName != "ap-minimax-h3-text-to-video" {
			continue
		}
		pricingVisible = true
		require.Equal(t, billing_setting.BillingModeTaskPricing, item.BillingMode)
		require.NotNil(t, item.TaskPricing)
		require.ElementsMatch(t, []string{"480p", "768p"}, item.TaskPricingResolutions)
	}
	require.True(t, pricingVisible, "Token Market duration model must appear in the pricing list after catalog sync")

	seedance, ok := billing_setting.GetTaskPricing("AP Seedance")
	require.True(t, ok)
	require.InDelta(t, 0.5, seedance.ByResolution["1080p"].NoReferenceVideoUnitPrice, 1e-12)
	require.Equal(t, "ratio", billing_setting.GetBillingMode("unrelated-model"))

	second, err := applyAIPDDCatalog(catalog, "https://aipdd.example", "sk-test")
	require.NoError(t, err)
	require.Zero(t, second.UpdatedPrices)

	catalog.Revision = "token-market-unavailable-revision"
	catalog.Capabilities[0].Available = aipddcatalog.BoolPtr(false)
	removed, err := applyAIPDDCatalog(catalog, "https://aipdd.example", "sk-test")
	require.NoError(t, err)
	require.Equal(t, 1, removed.UpdatedPrices)
	_, ok = billing_setting.GetTaskPricing("ap-minimax-h3-text-to-video")
	require.False(t, ok)
	require.NotEqual(t, billing_setting.BillingModeTaskPricing,
		billing_setting.GetBillingMode("ap-minimax-h3-text-to-video"))
	_, ok = billing_setting.GetTaskPricing("AP Seedance")
	require.True(t, ok)
}

func TestApplyAIPDDCatalogImportsTokenMarketPerCallDisplayPrices(t *testing.T) {
	truncateTables(t)
	t.Cleanup(func() {
		constant.ResetAIPDDCapabilities()
		constant.ResetAIPDDOpenAIModels()
		aipddcatalog.ResetV1ModelsListHidden()
	})
	preserveAIPDDPricingRuntime(t)
	previousExchangeRate := operation_setting.USDExchangeRate
	operation_setting.USDExchangeRate = 8
	t.Cleanup(func() { operation_setting.USDExchangeRate = previousExchangeRate })

	require.NoError(t, UpdateOption(
		aipddModelPriceOptionKey,
		`{"disabled-image":0.5,"unrelated-model":3.75}`,
	))
	require.NoError(t, UpdateOption(
		aipddBillingModeOptionKey,
		`{"fixed-image":"task_pricing","unrelated-model":"ratio"}`,
	))
	require.NoError(t, UpdateOption(
		aipddTaskPricingOptionKey,
		`{"fixed-image":{"unit":"second","no_reference_video_unit_price":9,"reference_video_policy":"same"}}`,
	))

	catalog := aipddTestCatalog("token-market-fixed-price-revision", "unmanaged-task", "llm-model")
	catalog.Capabilities = append(catalog.Capabilities,
		aipddcatalog.AtomicCapability{
			ID: "fixed-image", Code: "fixed-image", Name: "Fixed Image", AdapterCode: "token_market_media",
			EndpointType: "image-generation", TaskKind: "image_generation", Available: aipddcatalog.BoolPtr(true),
			Execution: aipddcatalog.AtomicExecution{Protocol: "token_market_image", Path: "/v1/images/generations"},
			Pricing: aipddcatalog.AtomicPricing{
				PricingModel: "per_call", Currency: "awcoin", Enabled: true,
				ChargeConfig: map[string]any{"amountAwcoin": float64(100)},
			},
		},
		aipddcatalog.AtomicCapability{
			ID: "disabled-image", Code: "disabled-image", Name: "Disabled Image", AdapterCode: "token_market_media",
			EndpointType: "image-generation", TaskKind: "image_generation", Available: aipddcatalog.BoolPtr(false),
			Execution: aipddcatalog.AtomicExecution{Protocol: "token_market_image", Path: "/v1/images/generations"},
			Pricing: aipddcatalog.AtomicPricing{
				PricingModel: "per_call", Currency: "awcoin", Enabled: true,
				ChargeConfig: map[string]any{"amountAwcoin": float64(200)},
			},
		},
	)

	result, err := applyAIPDDCatalog(catalog, "https://aipdd.example", "sk-test")
	require.NoError(t, err)
	require.Equal(t, 2, result.UpdatedPrices)

	price, ok := ratio_setting.GetModelPrice("fixed-image", false)
	require.True(t, ok)
	require.InDelta(t, 0.125, price, 1e-12)
	_, ok = ratio_setting.GetModelPrice("disabled-image", false)
	require.False(t, ok)
	unrelatedPrice, ok := ratio_setting.GetModelPrice("unrelated-model", false)
	require.True(t, ok)
	require.Equal(t, 3.75, unrelatedPrice)
	require.NotEqual(t, billing_setting.BillingModeTaskPricing, billing_setting.GetBillingMode("fixed-image"))
	_, ok = billing_setting.GetTaskPricing("fixed-image")
	require.False(t, ok)
	require.Equal(t, "ratio", billing_setting.GetBillingMode("unrelated-model"))

	pricing, ok := findPricingForTest(GetPricing(), "fixed-image")
	require.True(t, ok)
	require.Equal(t, 1, pricing.QuotaType)
	require.InDelta(t, 0.125, pricing.ModelPrice, 1e-12)

	second, err := applyAIPDDCatalog(catalog, "https://aipdd.example", "sk-test")
	require.NoError(t, err)
	require.Zero(t, second.UpdatedPrices)
}

func TestApplyAIPDDCatalogKeepsDisabledModelsInDBAndChannel(t *testing.T) {
	truncateTables(t)
	t.Cleanup(func() {
		constant.ResetAIPDDCapabilities()
		constant.ResetAIPDDOpenAIModels()
		aipddcatalog.ResetV1ModelsListHidden()
	})

	catalog := aipddTestCatalog("disabled-list-revision", "enabled-task", "enabled-llm")
	catalog.Capabilities = append(catalog.Capabilities,
		aipddcatalog.AtomicCapability{
			ID: "unavailable-task", Code: "unavailable-task", Name: "unavailable-task",
			AdapterCode: "comfyui", EndpointType: "image-generation", TaskKind: "text_to_image",
			Available: aipddcatalog.BoolPtr(false),
			Execution: aipddcatalog.AtomicExecution{Protocol: "shared_task", Path: "/shared-tasks/tasks"},
			Pricing: aipddcatalog.AtomicPricing{
				PricingModel: "per_call", Currency: "awcoin", Enabled: true,
				ChargeConfig: map[string]any{"amountAwcoin": float64(100)},
			},
		},
		aipddcatalog.AtomicCapability{
			ID: "pricing-disabled-task", Code: "pricing-disabled-task", Name: "pricing-disabled-task",
			AdapterCode: "comfyui", EndpointType: "audio-speech", TaskKind: "text_to_speech",
			Available: aipddcatalog.BoolPtr(true),
			Execution: aipddcatalog.AtomicExecution{Protocol: "shared_task", Path: "/shared-tasks/tasks"},
			Pricing: aipddcatalog.AtomicPricing{
				PricingModel: "per_call", Currency: "awcoin", Enabled: false,
				ChargeConfig: map[string]any{"amountAwcoin": float64(100)},
			},
		},
	)
	catalog.Models = append(catalog.Models, aipddcatalog.AtomicModel{
		ID: "unavailable-llm", Name: "unavailable-llm", Available: false,
		Execution: aipddcatalog.AtomicExecution{Protocol: "openai", Path: "/v1/chat/completions"},
		Pricing: aipddcatalog.AtomicPricing{
			PricingModel: "per_token", Currency: "awcoin", Enabled: true,
			PromptPerMillion: 10, CompletionPerMillion: 30,
		},
	})

	_, err := applyAIPDDCatalog(catalog, "https://aipdd.example", "sk-test")
	require.NoError(t, err)

	require.ElementsMatch(t, []string{"pricing-disabled-task"}, catalog.V1ModelsListHiddenNames())
	require.True(t, aipddcatalog.HasV1ModelsListHiddenState())
	require.False(t, aipddcatalog.IsHiddenFromV1ModelsList("unavailable-task"))
	require.True(t, aipddcatalog.IsHiddenFromV1ModelsList("pricing-disabled-task"))
	require.False(t, aipddcatalog.IsHiddenFromV1ModelsList("unavailable-llm"))
	require.False(t, aipddcatalog.IsHiddenFromV1ModelsList("enabled-task"))
	require.False(t, aipddcatalog.IsHiddenFromV1ModelsList("enabled-llm"))

	// Sync still keeps disabled models in ModelNames / Channel / Ability / Model rows.
	require.ElementsMatch(t, []string{
		"enabled-llm", "enabled-task", "pricing-disabled-task", "unavailable-llm", "unavailable-task",
	}, catalog.ModelNames())

	var managed Channel
	require.NoError(t, DB.Where("type = ? AND name = ?", constant.ChannelTypeAIPDD, "AIPDD").First(&managed).Error)
	require.ElementsMatch(t, catalog.ModelNames(), managed.GetModels())

	for _, modelName := range []string{"unavailable-task", "pricing-disabled-task", "unavailable-llm", "enabled-task", "enabled-llm"} {
		var modelCount, abilityCount int64
		require.NoError(t, DB.Model(&Model{}).Where("model_name = ?", modelName).Count(&modelCount).Error)
		require.NoError(t, DB.Model(&Ability{}).Where("model = ? AND channel_id = ?", modelName, managed.Id).Count(&abilityCount).Error)
		require.EqualValues(t, 1, modelCount, modelName)
		require.EqualValues(t, 1, abilityCount, modelName)
	}
}

func TestApplyAIPDDCatalogSkipsFreeModelsAndCleansLegacyRows(t *testing.T) {
	truncateTables(t)
	t.Cleanup(func() {
		constant.ResetAIPDDCapabilities()
		constant.ResetAIPDDOpenAIModels()
		aipddcatalog.ResetV1ModelsListHidden()
		aipddcatalog.ResetExplicitFreeModels()
	})

	vendor := Vendor{Name: "AIPDD", Status: 1}
	require.NoError(t, DB.Create(&vendor).Error)
	require.NoError(t, DB.Create(&Model{
		ModelName: "free-legacy", VendorID: vendor.Id, Status: 1, SyncOfficial: 1,
	}).Error)
	baseURL := "https://aipdd.example"
	legacyChannel := Channel{
		Type: constant.ChannelTypeAIPDD, Name: aipddEnvChannelName, Key: "sk-test",
		Group: "default", Models: "free-legacy", Status: common.ChannelStatusEnabled,
		BaseURL: &baseURL,
	}
	require.NoError(t, DB.Create(&legacyChannel).Error)
	require.NoError(t, DB.Create(&Ability{
		Group: "default", Model: "free-legacy", ChannelId: legacyChannel.Id, Enabled: true,
	}).Error)

	catalog := aipddTestCatalog("free-description-revision", "task-model", "llm-model")
	catalog.Models = append(catalog.Models,
		aipddcatalog.AtomicModel{
			ID: "free-hy3", Name: "free-hy3", Available: true,
			Execution: aipddcatalog.AtomicExecution{Protocol: "openai", Path: "/v1/chat/completions"},
			Pricing: aipddcatalog.AtomicPricing{
				PricingModel: "per_token", Currency: "awcoin", Enabled: true, Free: true,
			},
		},
		aipddcatalog.AtomicModel{
			ID: "free-deepseek-v4-flash", Name: "free-deepseek-v4-flash",
			Description: "custom free description", Available: true,
			Execution: aipddcatalog.AtomicExecution{Protocol: "openai", Path: "/v1/chat/completions"},
			Pricing: aipddcatalog.AtomicPricing{
				PricingModel: "per_token", Currency: "awcoin", Enabled: true, Free: true,
			},
		},
	)

	result, err := applyAIPDDCatalog(catalog, baseURL, "sk-test")
	require.NoError(t, err)
	require.Equal(t, 2, result.AddedModels)
	require.Equal(t, 1, result.RemovedModels)

	var paid Model
	require.NoError(t, DB.Where("model_name = ?", "llm-model").First(&paid).Error)
	require.Equal(t, "AIPDD 上游目录同步模型。", paid.Description)

	for _, modelName := range []string{"free-legacy", "free-hy3", "free-deepseek-v4-flash"} {
		var modelCount, abilityCount int64
		require.NoError(t, DB.Unscoped().Model(&Model{}).Where("model_name = ?", modelName).Count(&modelCount).Error)
		require.NoError(t, DB.Model(&Ability{}).Where("model = ?", modelName).Count(&abilityCount).Error)
		require.Zero(t, modelCount, modelName)
		require.Zero(t, abilityCount, modelName)
	}

	var managed Channel
	require.NoError(t, DB.Where("id = ?", legacyChannel.Id).First(&managed).Error)
	require.Equal(t, "llm-model,task-model", managed.Models)
	require.False(t, aipddcatalog.IsExplicitFreeModel("free-hy3"))

	var snapshot AIPDDCatalogSnapshot
	require.NoError(t, DB.First(&snapshot, aipddCatalogSnapshotID).Error)
	require.NotContains(t, snapshot.Payload, "free-hy3")
	require.NotContains(t, snapshot.Payload, "free-deepseek-v4-flash")
}

func TestEnsureAIPDDOpenAIModelDefaultsDoesNotCreateLocalPricing(t *testing.T) {
	truncateTables(t)
	constant.ResetAIPDDCapabilities()
	constant.ResetAIPDDOpenAIModels()
	aipddcatalog.ResetV1ModelsListHidden()
	t.Cleanup(func() {
		constant.ResetAIPDDCapabilities()
		constant.ResetAIPDDOpenAIModels()
		aipddcatalog.ResetV1ModelsListHidden()
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
			EndpointType: "image-generation", TaskKind: "text_to_image", Available: aipddcatalog.BoolPtr(true),
			Execution: aipddcatalog.AtomicExecution{Protocol: "shared_task", Path: "/shared-tasks/tasks"},
			Pricing: aipddcatalog.AtomicPricing{
				PricingModel: "per_call", Currency: "awcoin", Enabled: true,
				ChargeConfig: map[string]any{"amountAwcoin": float64(100)},
			},
		}},
		Models: []aipddcatalog.AtomicModel{{
			ID: llmModel, Name: llmModel, Available: true,
			Execution: aipddcatalog.AtomicExecution{Protocol: "openai", Path: "/v1/chat/completions"},
			Pricing: aipddcatalog.AtomicPricing{
				PricingModel: "per_token", Currency: "awcoin", Enabled: true,
				PromptPerMillion: 10, CompletionPerMillion: 30,
			},
		}},
	}
}
