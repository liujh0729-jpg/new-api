package model

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/pkg/aipddcatalog"
	"github.com/stretchr/testify/require"
)

func TestEnsureAIPDDChannelDefaultsCreatesChannelFromEnv(t *testing.T) {
	truncateTables(t)
	t.Setenv("AIPDD_API_KEY", "aipdd-env-key")

	require.NoError(t, EnsureAIPDDChannelDefaults())

	var channel Channel
	require.NoError(t, DB.Where("type = ?", constant.ChannelTypeAIPDD).First(&channel).Error)
	require.Equal(t, "aipdd-env-key", channel.Key)
	require.Equal(t, "AIPDD", channel.Name)
	require.Equal(t, common.ChannelStatusEnabled, channel.Status)
	require.Equal(t, "default", channel.Group)
	require.NotNil(t, channel.BaseURL)
	require.Equal(t, constant.ChannelBaseURLs[constant.ChannelTypeAIPDD], *channel.BaseURL)
	require.Equal(t, strings.Join(constant.AIPDDTaskModelList, ","), channel.Models)

	var abilities []Ability
	require.NoError(t, DB.Where("channel_id = ?", channel.Id).Find(&abilities).Error)
	require.Len(t, abilities, len(constant.AIPDDTaskModelList))
}

func TestEnsureAIPDDChannelDefaultsRequiresEnvWhenEnabled(t *testing.T) {
	truncateTables(t)
	t.Setenv("AIPDD_API_KEY", "")
	t.Setenv("AIPDD_BOOTSTRAP_REQUIRED", "true")

	err := EnsureAIPDDChannelDefaults()

	require.Error(t, err)
	require.Contains(t, err.Error(), "AIPDD_API_KEY is required")
}

func TestEnsureAIPDDChannelDefaultsSyncsExistingChannelFromEnv(t *testing.T) {
	truncateTables(t)
	t.Setenv("AIPDD_API_KEY", "aipdd-env-key-new")

	channel := Channel{
		Type:   constant.ChannelTypeAIPDD,
		Key:    "aipdd-env-key-old",
		Name:   "legacy-aipdd",
		Status: common.ChannelStatusManuallyDisabled,
		Group:  "default",
	}
	require.NoError(t, DB.Create(&channel).Error)

	require.NoError(t, EnsureAIPDDChannelDefaults())

	var stored Channel
	require.NoError(t, DB.First(&stored, channel.Id).Error)
	require.Equal(t, "aipdd-env-key-new", stored.Key)
	require.Equal(t, common.ChannelStatusEnabled, stored.Status)
	require.Equal(t, strings.Join(constant.AIPDDTaskModelList, ","), stored.Models)

	var abilities []Ability
	require.NoError(t, DB.Where("channel_id = ?", channel.Id).Find(&abilities).Error)
	require.Len(t, abilities, len(constant.AIPDDTaskModelList))
	for _, ability := range abilities {
		require.True(t, ability.Enabled)
	}
}

func TestEnsureAIPDDChannelDefaultsOverwritesExistingChannelFromEnv(t *testing.T) {
	truncateTables(t)
	t.Setenv("AIPDD_API_KEY", "aipdd-env-key-new")
	t.Setenv("AIPDD_BASE_URL", "https://aipdd.example.com")
	t.Setenv("AIPDD_CHANNEL_OVERWRITE_ON_BOOT", "true")

	oldBaseURL := "https://old-aipdd.example.com"
	channel := Channel{
		Type:    constant.ChannelTypeAIPDD,
		Key:     "aipdd-env-key-old",
		Name:    "legacy-aipdd",
		Status:  common.ChannelStatusManuallyDisabled,
		Group:   "vip",
		BaseURL: &oldBaseURL,
		Models:  "legacy-model",
	}
	require.NoError(t, DB.Create(&channel).Error)
	require.NoError(t, channel.AddAbilities(nil))

	require.NoError(t, EnsureAIPDDChannelDefaults())

	var stored Channel
	require.NoError(t, DB.First(&stored, channel.Id).Error)
	require.Equal(t, "AIPDD", stored.Name)
	require.Equal(t, "aipdd-env-key-new", stored.Key)
	require.Equal(t, common.ChannelStatusEnabled, stored.Status)
	require.Equal(t, "default", stored.Group)
	require.NotNil(t, stored.BaseURL)
	require.Equal(t, "https://aipdd.example.com", *stored.BaseURL)
	require.Equal(t, strings.Join(constant.GetAIPDDModelList(), ","), stored.Models)

	var abilities []Ability
	require.NoError(t, DB.Where("channel_id = ?", channel.Id).Find(&abilities).Error)
	require.Len(t, abilities, len(constant.GetAIPDDModelList()))
	for _, ability := range abilities {
		require.NotEqual(t, "legacy-model", ability.Model)
		require.True(t, ability.Enabled)
		require.Equal(t, "default", ability.Group)
	}
}

func TestEnsureAIPDDChannelDefaultsBackfillsMissingCatalogModels(t *testing.T) {
	truncateTables(t)

	legacyModels := make([]string, 0, len(constant.AIPDDTaskModelList)-1)
	for _, modelName := range constant.AIPDDTaskModelList {
		if modelName != constant.AIPDDModelFluxGGUFT2I {
			legacyModels = append(legacyModels, modelName)
		}
	}
	channel := Channel{
		Type:   constant.ChannelTypeAIPDD,
		Key:    "aipdd-test-key",
		Name:   "AIPDD",
		Status: common.ChannelStatusEnabled,
		Group:  "default",
		Models: strings.Join(legacyModels, ","),
	}
	require.NoError(t, DB.Create(&channel).Error)

	require.NoError(t, EnsureAIPDDChannelDefaults())

	var stored Channel
	require.NoError(t, DB.First(&stored, channel.Id).Error)
	storedModels := stored.GetModels()
	require.Len(t, storedModels, len(constant.AIPDDTaskModelList))
	require.Contains(t, storedModels, constant.AIPDDModelFluxGGUFT2I)

	var abilities []Ability
	require.NoError(t, DB.Where("channel_id = ?", channel.Id).Find(&abilities).Error)
	require.Len(t, abilities, len(constant.AIPDDTaskModelList))
	for _, ability := range abilities {
		require.True(t, ability.Enabled)
	}
}

func TestFixAbilityBackfillsAIPDDMissingCatalogModels(t *testing.T) {
	truncateTables(t)

	legacyModels := make([]string, 0, len(constant.AIPDDTaskModelList)-1)
	for _, modelName := range constant.AIPDDTaskModelList {
		if modelName != constant.AIPDDModelFluxGGUFT2I {
			legacyModels = append(legacyModels, modelName)
		}
	}
	channel := Channel{
		Type:   constant.ChannelTypeAIPDD,
		Key:    "aipdd-test-key",
		Name:   "AIPDD",
		Status: common.ChannelStatusEnabled,
		Group:  "default",
		Models: strings.Join(legacyModels, ","),
	}
	require.NoError(t, DB.Create(&channel).Error)
	require.NoError(t, channel.AddAbilities(nil))

	success, fails, err := FixAbility()

	require.NoError(t, err)
	require.Equal(t, 1, success)
	require.Equal(t, 0, fails)

	var stored Channel
	require.NoError(t, DB.First(&stored, channel.Id).Error)
	require.Contains(t, stored.GetModels(), constant.AIPDDModelFluxGGUFT2I)

	var abilities []Ability
	require.NoError(t, DB.Where("channel_id = ?", channel.Id).Find(&abilities).Error)
	require.Len(t, abilities, len(constant.AIPDDTaskModelList))
	abilityModels := map[string]bool{}
	for _, ability := range abilities {
		abilityModels[ability.Model] = true
	}
	require.True(t, abilityModels[constant.AIPDDModelFluxGGUFT2I])
}

func TestEnsureAIPDDChannelDefaultsNormalizesSmokeTestChannel(t *testing.T) {
	truncateTables(t)

	channel := Channel{
		Type:   constant.ChannelTypeAIPDD,
		Key:    "aipdd-test-key",
		Name:   "AIPDD smoke test",
		Status: common.ChannelStatusEnabled,
		Group:  "default",
		Models: strings.Join(constant.AIPDDTaskModelList, ","),
	}
	require.NoError(t, DB.Create(&channel).Error)

	require.NoError(t, EnsureAIPDDChannelDefaults())

	var stored Channel
	require.NoError(t, DB.First(&stored, channel.Id).Error)
	require.Equal(t, "AIPDD", stored.Name)
	require.NotNil(t, stored.BaseURL)
	require.Equal(t, constant.ChannelBaseURLs[constant.ChannelTypeAIPDD], *stored.BaseURL)
}

func TestEnsureAIPDDDefaultsSkipsWhenAtomicSyncDisabled(t *testing.T) {
	truncateTables(t)
	t.Setenv("AIPDD_API_KEY", "aipdd-env-key")

	require.NoError(t, EnsureAIPDDDefaults())

	var count int64
	require.NoError(t, DB.Model(&Channel{}).Where("type = ?", constant.ChannelTypeAIPDD).Count(&count).Error)
	require.Zero(t, count)
}

func TestEnsureAIPDDDefaultsRestoresRuntimeSnapshotWhenSyncDisabled(t *testing.T) {
	truncateTables(t)
	constant.ResetAIPDDCapabilities()
	constant.ResetAIPDDOpenAIModels()
	preserveAIPDDPricingRuntime(t)
	t.Cleanup(func() {
		constant.ResetAIPDDCapabilities()
		constant.ResetAIPDDOpenAIModels()
		InvalidatePricingCache()
	})

	const (
		baseURL   = "https://aipdd.snapshot.test"
		modelName = "AP Seedance snapshot test"
	)
	displayAmount := 40.0
	displayVideoAmount := 60.0
	catalog := aipddTestCatalog("snapshot-runtime-revision", "unused-task", "unused-llm")
	catalog.Capabilities = []aipddcatalog.AtomicCapability{{
		ID: modelName, Code: "seedance", Name: modelName, AdapterCode: "seedance",
		EndpointType: "openai-video", TaskKind: "video_generation",
		Execution: aipddcatalog.AtomicExecution{Protocol: "seedance_official", Path: "/api/v3/contents/generations/tasks"},
		Pricing: aipddcatalog.AtomicPricing{
			PricingModel: "per_second", Currency: "awcoin", PricingBasis: "display", Enabled: true,
			ByResolution: map[string]constant.AIPDDSeedanceResolutionPricing{
				"720p": {
					TargetResolution:                 "720p",
					DisplayAmountAWCoinPerSecond:     &displayAmount,
					DisplayVideoInputAWCoinPerSecond: &displayVideoAmount,
					DefaultDurationSeconds:           5,
					DefaultFramesPerSecond:           24,
				},
			},
		},
	}}
	catalog.Models = nil
	_, err := applyAIPDDCatalog(catalog, baseURL, "sk-snapshot-test")
	require.NoError(t, err)
	require.NoError(t, UpdateOption(
		"billing_setting.task_pricing",
		`{"AP Seedance snapshot test":{"unit":"second","by_resolution":{"720p":{"no_reference_video_unit_price":0.08,"reference_video_policy":"custom","reference_video_unit_price":0.12}}}}`,
	))
	require.NoError(t, UpdateOption(
		"billing_setting.billing_mode",
		`{"AP Seedance snapshot test":"task_pricing"}`,
	))

	var channelBefore Channel
	require.NoError(t, DB.Where("type = ?", constant.ChannelTypeAIPDD).First(&channelBefore).Error)
	var snapshotBefore AIPDDCatalogSnapshot
	require.NoError(t, DB.First(&snapshotBefore, aipddCatalogSnapshotID).Error)
	var abilityCountBefore int64
	require.NoError(t, DB.Model(&Ability{}).Count(&abilityCountBefore).Error)

	constant.ResetAIPDDCapabilities()
	constant.ResetAIPDDOpenAIModels()
	InvalidatePricingCache()
	_, found := findPricingForTest(GetPricing(), modelName)
	require.False(t, found, "compiled defaults do not contain dynamic Seedance metadata")

	t.Setenv("AIPDD_API_KEY", "sk-snapshot-test")
	t.Setenv("AIPDD_BASE_URL", baseURL)
	t.Setenv("AIPDD_CATALOG_SYNC_ON_BOOT", "false")
	require.NoError(t, EnsureAIPDDDefaults())

	pricing, found := findPricingForTest(GetPricing(), modelName)
	require.True(t, found)
	require.Equal(t, "task_pricing", pricing.BillingMode)
	require.Equal(t, []string{"720p"}, pricing.TaskPricingResolutions)
	capability, found := constant.GetAIPDDCapability(modelName)
	require.True(t, found)
	require.Contains(t, capability.SeedancePricing.ByResolution, "720p")

	var channelAfter Channel
	require.NoError(t, DB.Where("type = ?", constant.ChannelTypeAIPDD).First(&channelAfter).Error)
	var snapshotAfter AIPDDCatalogSnapshot
	require.NoError(t, DB.First(&snapshotAfter, aipddCatalogSnapshotID).Error)
	var abilityCountAfter int64
	require.NoError(t, DB.Model(&Ability{}).Count(&abilityCountAfter).Error)
	require.Equal(t, channelBefore, channelAfter)
	require.Equal(t, snapshotBefore, snapshotAfter)
	require.Equal(t, abilityCountBefore, abilityCountAfter)
}

func TestEnsureAIPDDDefaultsSyncsDynamicCatalogOnBoot(t *testing.T) {
	truncateTables(t)
	constant.ResetAIPDDCapabilities()
	constant.ResetAIPDDOpenAIModels()
	t.Cleanup(func() {
		constant.ResetAIPDDCapabilities()
		constant.ResetAIPDDOpenAIModels()
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/new-api/catalog":
			require.Equal(t, "sk-test-env-key", r.Header.Get("X-API-Key"))
			_, _ = w.Write([]byte(`{
				"code": 0,
				"message": "fetched",
				"data": {
					"schemaVersion": 1,
					"revision": "test-revision",
					"generatedAt": "2026-07-12T10:00:00",
					"awcoinRate": {"rmbPerAwcoin": 0.01, "usdPerAwcoin": 0.0015, "updatedAt": "2026-07-12T09:00:00"},
					"capabilities": [{
						"id": "dynamic-script-id",
						"code": "dynamic-aipdd-video",
						"name": "Dynamic AIPDD Video",
						"description": "dynamic model from upstream",
						"adapterCode": "comfyui",
						"endpointType": "openai-video",
						"taskKind": "image_to_video",
						"inputModalities": ["image", "text"],
						"outputModalities": ["video"],
						"available": false,
						"execution": {"protocol": "shared_task", "path": "/shared-tasks/tasks"},
						"pricing": {"pricingModel": "per_call", "currency": "awcoin", "enabled": true, "chargeConfig": {"amountAwcoin": 500}},
						"params": [
							{"paramKey": "image", "dataType": "string", "isRequired": true, "orderNo": 1, "uiType": "image_url"},
							{"paramKey": "prompt", "dataType": "string", "isRequired": true, "orderNo": 2, "uiType": "textarea"}
						]
					}],
					"models": [
						{"id": "gemma3:1b", "name": "gemma3:1b", "available": false, "execution": {"protocol": "openai", "path": "/v1/chat/completions"}, "pricing": {"pricingModel": "per_token", "currency": "awcoin", "enabled": true, "promptPerMillion": 10, "completionPerMillion": 30}},
						{"id": "qwen2.5:0.5b", "name": "qwen2.5:0.5b", "available": true, "execution": {"protocol": "openai", "path": "/v1/chat/completions"}, "pricing": {"pricingModel": "per_token", "currency": "awcoin", "enabled": true, "promptPerMillion": 20, "completionPerMillion": 40}}
					]
				}
			}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	t.Setenv("AIPDD_API_KEY", "sk-test-env-key")
	t.Setenv("AIPDD_BASE_URL", server.URL)
	t.Setenv("AIPDD_CATALOG_SYNC_ON_BOOT", "true")

	require.NoError(t, EnsureAIPDDDefaults())

	var channel Channel
	require.NoError(t, DB.Where("type = ?", constant.ChannelTypeAIPDD).First(&channel).Error)
	require.Equal(t, server.URL, *channel.BaseURL)
	require.Equal(t, "dynamic-script-id,gemma3:1b,qwen2.5:0.5b", channel.Models)

	var ability Ability
	require.NoError(t, DB.Where("channel_id = ? AND model = ?", channel.Id, "dynamic-script-id").First(&ability).Error)
	require.True(t, ability.Enabled)
	var llmAbility Ability
	require.NoError(t, DB.Where("channel_id = ? AND model = ?", channel.Id, "gemma3:1b").First(&llmAbility).Error)
	require.True(t, llmAbility.Enabled)

	var item Model
	require.NoError(t, DB.Where("model_name = ?", "dynamic-script-id").First(&item).Error)
	require.Equal(t, constant.AIPDDLogoPath, item.Icon)
	require.Contains(t, item.Tags, "ComfyUI")
	require.Equal(t, marshalEndpointTypes([]constant.EndpointType{constant.EndpointTypeOpenAIVideo}), item.Endpoints)
	var llmItem Model
	require.NoError(t, DB.Where("model_name = ?", "gemma3:1b").First(&llmItem).Error)
	require.Equal(t, constant.AIPDDLogoPath, llmItem.Icon)
	require.Contains(t, llmItem.Tags, "LLM")
	require.Equal(t, marshalEndpointTypes([]constant.EndpointType{constant.EndpointTypeOpenAI}), llmItem.Endpoints)

	capability, ok := constant.GetAIPDDCapability("dynamic-script-id")
	require.True(t, ok)
	require.Equal(t, "image_to_video", capability.TaskKind)
	require.Equal(t, []string{"image", "text"}, capability.InputModalities)
	require.Equal(t, []string{"video"}, capability.OutputModalities)

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

func TestEnsureAIPDDDefaultsFailsWhenAtomicCatalogUnavailable(t *testing.T) {
	truncateTables(t)
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()

	t.Setenv("AIPDD_API_KEY", "sk-test-env-key")
	t.Setenv("AIPDD_BASE_URL", server.URL)
	t.Setenv("AIPDD_CATALOG_SYNC_ON_BOOT", "true")

	err := EnsureAIPDDDefaults()
	require.Error(t, err)
	require.Contains(t, err.Error(), "AIPDD atomic catalog sync failed")

	var channelCount, vendorCount int64
	require.NoError(t, DB.Model(&Channel{}).Where("type = ?", constant.ChannelTypeAIPDD).Count(&channelCount).Error)
	require.NoError(t, DB.Model(&Vendor{}).Where("name = ?", "AIPDD").Count(&vendorCount).Error)
	require.Zero(t, channelCount)
	require.Zero(t, vendorCount)
}

func TestEnsureAIPDDDefaultsRequiresEnvBeforeCatalogSync(t *testing.T) {
	truncateTables(t)
	t.Setenv("AIPDD_API_KEY", "")
	t.Setenv("AIPDD_BOOTSTRAP_REQUIRED", "true")

	err := EnsureAIPDDDefaults()

	require.Error(t, err)
	require.Contains(t, err.Error(), "AIPDD_API_KEY is required")

	var count int64
	require.NoError(t, DB.Model(&Vendor{}).Where("name = ?", "AIPDD").Count(&count).Error)
	require.Zero(t, count)
}
