package model

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
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

func TestEnsureAIPDDDefaultsCreatesChannelAndModelCatalog(t *testing.T) {
	truncateTables(t)
	t.Setenv("AIPDD_API_KEY", "aipdd-env-key")

	require.NoError(t, EnsureAIPDDDefaults())

	var channel Channel
	require.NoError(t, DB.Where("type = ?", constant.ChannelTypeAIPDD).First(&channel).Error)
	require.Equal(t, strings.Join(constant.AIPDDTaskModelList, ","), channel.Models)

	var vendor Vendor
	require.NoError(t, DB.Where("name = ?", "AIPDD").First(&vendor).Error)
	require.Equal(t, constant.AIPDDLogoPath, vendor.Icon)
	require.Equal(t, constant.AIPDDWebsiteURL, vendor.Website)

	for _, catalog := range defaultCatalogModels {
		if catalog.ChannelType != constant.ChannelTypeAIPDD {
			continue
		}
		var item Model
		require.NoError(t, DB.Where("model_name = ?", catalog.ModelName).First(&item).Error)
		require.Equal(t, vendor.Id, item.VendorID)
		require.Equal(t, catalog.Icon, item.Icon)
		require.Equal(t, marshalEndpointTypes(catalog.EndpointTypes), item.Endpoints)
		require.Equal(t, NameRuleExact, item.NameRule)
		require.Equal(t, 1, item.Status)
	}
}

func TestEnsureAIPDDDefaultsSyncsDynamicCatalogOnBoot(t *testing.T) {
	truncateTables(t)
	constant.ResetAIPDDCapabilities()
	t.Cleanup(constant.ResetAIPDDCapabilities)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/scripts/admin/comfyui_workflow":
			_, _ = w.Write([]byte(`{
				"code": 200,
				"message": "ok",
				"data": [
					{
						"id": "dynamic-script-id",
						"code": "dynamic-aipdd-video",
						"name": "Dynamic AIPDD Video",
						"description": "dynamic model from upstream",
						"priceAWcoin": 500,
						"endpointType": "openai-video",
						"taskKind": "image_to_video",
						"inputModalities": ["image", "text"],
						"outputModalities": ["video"],
						"params": [
							{"paramKey": "image", "dataType": "string", "isRequired": true, "orderNo": 1, "uiType": "image_url"},
							{"paramKey": "prompt", "dataType": "string", "isRequired": true, "orderNo": 2, "uiType": "textarea"}
						]
					}
				]
			}`))
		case "/fee-rules":
			_, _ = w.Write([]byte(`{
				"code": 200,
				"message": "ok",
				"data": {
					"total": 1,
					"page": 1,
					"pageSize": 100,
					"list": [
						{"key": "dynamic-aipdd-video", "name": "Dynamic AIPDD Video", "type": "comfyUI_workflow", "price": 500, "unit": "次"}
					]
				}
			}`))
		case "/system/awcoin-rate":
			_, _ = w.Write([]byte(`{
				"code": 200,
				"message": "ok",
				"data": {"rmb": 0.01, "usd": 0.0015}
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
	require.Equal(t, "dynamic-aipdd-video", channel.Models)

	var ability Ability
	require.NoError(t, DB.Where("channel_id = ? AND model = ?", channel.Id, "dynamic-aipdd-video").First(&ability).Error)
	require.True(t, ability.Enabled)

	var item Model
	require.NoError(t, DB.Where("model_name = ?", "dynamic-aipdd-video").First(&item).Error)
	require.Equal(t, constant.AIPDDLogoPath, item.Icon)
	require.Contains(t, item.Tags, "image_to_video")
	require.Equal(t, marshalEndpointTypes([]constant.EndpointType{constant.EndpointTypeOpenAIVideo}), item.Endpoints)

	capability, ok := constant.GetAIPDDCapability("dynamic-aipdd-video")
	require.True(t, ok)
	require.Equal(t, "image_to_video", capability.TaskKind)
	require.Equal(t, []string{"image", "text"}, capability.InputModalities)
	require.Equal(t, []string{"video"}, capability.OutputModalities)

	modelPrice, ok := ratio_setting.GetModelPrice("dynamic-aipdd-video", false)
	require.True(t, ok)
	require.Equal(t, 0.75, modelPrice)

	var option Option
	require.NoError(t, DB.Where(&Option{Key: "ModelPrice"}).First(&option).Error)
	require.Contains(t, option.Value, `"dynamic-aipdd-video":0.75`)
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
