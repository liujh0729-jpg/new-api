package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/aipddcatalog"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type listModelsResponse struct {
	Success bool               `json:"success"`
	Data    []dto.OpenAIModels `json:"data"`
	Object  string             `json:"object"`
}

type userModelsResponse struct {
	Success bool     `json:"success"`
	Data    []string `json:"data"`
}

type dashboardModelsResponse struct {
	Success bool             `json:"success"`
	Data    map[int][]string `json:"data"`
}

func setupModelListControllerTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	initModelListColumnNames(t)

	gin.SetMode(gin.TestMode)
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.RedisEnabled = false

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	model.LOG_DB = db

	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Channel{}, &model.Ability{}, &model.Model{}, &model.Vendor{}, &model.Option{}))

	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})

	return db
}

func initModelListColumnNames(t *testing.T) {
	t.Helper()

	originalIsMasterNode := common.IsMasterNode
	originalSQLitePath := common.SQLitePath
	originalUsingSQLite := common.UsingSQLite
	originalUsingMySQL := common.UsingMySQL
	originalUsingPostgreSQL := common.UsingPostgreSQL
	originalSQLDSN, hadSQLDSN := os.LookupEnv("SQL_DSN")
	defer func() {
		common.IsMasterNode = originalIsMasterNode
		common.SQLitePath = originalSQLitePath
		common.UsingSQLite = originalUsingSQLite
		common.UsingMySQL = originalUsingMySQL
		common.UsingPostgreSQL = originalUsingPostgreSQL
		if hadSQLDSN {
			require.NoError(t, os.Setenv("SQL_DSN", originalSQLDSN))
		} else {
			require.NoError(t, os.Unsetenv("SQL_DSN"))
		}
	}()

	common.IsMasterNode = false
	common.SQLitePath = fmt.Sprintf("file:%s_init?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	common.UsingSQLite = false
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	require.NoError(t, os.Setenv("SQL_DSN", "local"))

	require.NoError(t, model.InitDB())
	if model.DB != nil {
		sqlDB, err := model.DB.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	}
}

func withTieredBillingConfig(t *testing.T, modes map[string]string, exprs map[string]string) {
	t.Helper()

	saved := map[string]string{}
	require.NoError(t, config.GlobalConfig.SaveToDB(func(key, value string) error {
		if strings.HasPrefix(key, "billing_setting.") {
			saved[key] = value
		}
		return nil
	}))
	t.Cleanup(func() {
		require.NoError(t, config.GlobalConfig.LoadFromDB(saved))
		model.InvalidatePricingCache()
	})

	modeBytes, err := common.Marshal(modes)
	require.NoError(t, err)
	exprBytes, err := common.Marshal(exprs)
	require.NoError(t, err)

	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"billing_setting.billing_mode": string(modeBytes),
		"billing_setting.billing_expr": string(exprBytes),
	}))
	model.InvalidatePricingCache()
}

func withSelfUseModeDisabled(t *testing.T) {
	t.Helper()

	original := operation_setting.SelfUseModeEnabled
	operation_setting.SelfUseModeEnabled = false
	t.Cleanup(func() {
		operation_setting.SelfUseModeEnabled = original
	})
}

func decodeListModelsResponse(t *testing.T, recorder *httptest.ResponseRecorder) map[string]struct{} {
	t.Helper()

	require.Equal(t, http.StatusOK, recorder.Code)
	var payload listModelsResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
	require.True(t, payload.Success)
	require.Equal(t, "list", payload.Object)

	ids := make(map[string]struct{}, len(payload.Data))
	for _, item := range payload.Data {
		ids[item.Id] = struct{}{}
	}
	return ids
}

func pricingByModelName(pricings []model.Pricing) map[string]model.Pricing {
	byName := make(map[string]model.Pricing, len(pricings))
	for _, pricing := range pricings {
		byName[pricing.ModelName] = pricing
	}
	return byName
}

func TestChannelListModelsIncludesTaskAdaptorModels(t *testing.T) {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/channel/models", nil)

	ChannelListModels(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	var payload listModelsResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
	require.True(t, payload.Success)

	ids := make(map[string]bool, len(payload.Data))
	for _, item := range payload.Data {
		ids[item.Id] = true
	}

	require.True(t, ids["suno_music"], "expected Suno task model in channel model list")
	require.True(t, ids["kling-v2-master"], "expected Kling task model in channel model list")
	require.True(t, ids["viduq2"], "expected Vidu task model in channel model list")
	require.True(t, ids["doubao-seedance-2-0-260128"], "expected Doubao video task model in channel model list")
	require.True(t, ids["MiniMax-Hailuo-2.3"], "expected Hailuo task model in channel model list")
	require.True(t, ids["sora-2-pro"], "expected Sora task model in channel model list")
}

func TestRetrieveModelIncludesTaskAdaptorModels(t *testing.T) {
	aiModel, ok := getOpenAIModel("suno_music")

	require.True(t, ok)
	require.Equal(t, "suno_music", aiModel.Id)
	require.Equal(t, "suno", aiModel.OwnedBy)
}

func TestDashboardListModelsMergesTaskAdaptorModelsByChannelType(t *testing.T) {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/dashboard/models", nil)

	DashboardListModels(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	var payload dashboardModelsResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
	require.True(t, payload.Success)

	require.Contains(t, payload.Data[constant.ChannelTypeSunoAPI], "suno_music")
	require.Contains(t, payload.Data[constant.ChannelTypeKling], "kling-v2-master")
	require.Contains(t, payload.Data[constant.ChannelTypeVidu], "viduq2")
	require.Contains(t, payload.Data[constant.ChannelTypeDoubaoVideo], "doubao-seedance-2-0-260128")
	require.Contains(t, payload.Data[constant.ChannelTypeSora], "sora-2-pro")

	require.Contains(t, payload.Data[constant.ChannelTypeAli], "qwen3.7-plus")
	require.Contains(t, payload.Data[constant.ChannelTypeAli], "wan2.7-t2v")
	require.Contains(t, payload.Data[constant.ChannelTypeMiniMax], "MiniMax-M3")
	require.Contains(t, payload.Data[constant.ChannelTypeMiniMax], "MiniMax-Hailuo-2.3")
}

func TestPricingIncludesAIPDDCatalogModelsByDefault(t *testing.T) {
	setupModelListControllerTestDB(t)
	model.InvalidatePricingCache()

	pricingByName := pricingByModelName(model.GetPricing())
	vendorByID := map[int]model.PricingVendor{}
	for _, vendor := range model.GetVendors() {
		vendorByID[vendor.ID] = vendor
	}

	for _, modelName := range constant.AIPDDTaskModelList {
		item, ok := pricingByName[modelName]
		require.True(t, ok, "expected AIPDD model %s in pricing catalog", modelName)
		require.Contains(t, item.EnableGroup, "default")
		expectedEndpoints := constant.GetAIPDDEndpointTypes(modelName)
		require.Equal(t, expectedEndpoints, item.SupportedEndpointTypes)
		require.Equal(t, "/aipdd-logo.png", item.Icon)
		vendor := vendorByID[item.VendorID]
		require.Equal(t, "AIPDD", vendor.Name)
		require.Equal(t, "/aipdd-logo.png", vendor.Icon)
		require.Equal(t, constant.AIPDDWebsiteURL, vendor.Website)
	}
}

func TestPricingBackfillsAIPDDLegacyOpenAIIcon(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	model.InvalidatePricingCache()

	vendor := model.Vendor{Name: "AIPDD", Icon: "OpenAI", Status: 1}
	require.NoError(t, db.Create(&vendor).Error)
	for _, modelName := range constant.AIPDDTaskModelList {
		require.NoError(t, db.Create(&model.Model{
			ModelName: modelName,
			Icon:      "OpenAI",
			VendorID:  vendor.Id,
			Status:    1,
			NameRule:  model.NameRuleExact,
		}).Error)
	}

	pricingByName := pricingByModelName(model.GetPricing())
	for _, modelName := range constant.AIPDDTaskModelList {
		item, ok := pricingByName[modelName]
		require.True(t, ok, "expected AIPDD model %s in pricing catalog", modelName)
		require.Equal(t, "/aipdd-logo.png", item.Icon)
	}

	var storedVendor model.Vendor
	require.NoError(t, db.First(&storedVendor, vendor.Id).Error)
	require.Equal(t, "/aipdd-logo.png", storedVendor.Icon)
	require.Equal(t, constant.AIPDDWebsiteURL, storedVendor.Website)

	var storedModels []model.Model
	require.NoError(t, db.Where("vendor_id = ?", vendor.Id).Find(&storedModels).Error)
	require.Len(t, storedModels, len(constant.AIPDDTaskModelList))
	for _, storedModel := range storedModels {
		require.Equal(t, "/aipdd-logo.png", storedModel.Icon)
	}
}

func TestPricingBackfillsDefaultVendorForExistingDoubaoModels(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	model.InvalidatePricingCache()

	channel := &model.Channel{
		Type:   constant.ChannelTypeDoubaoVideo,
		Key:    "doubao-test-key",
		Name:   "doubao-video",
		Models: "doubao-seedance-2.0",
		Status: common.ChannelStatusEnabled,
		Group:  "default",
	}
	require.NoError(t, channel.Insert())
	require.NoError(t, db.Create(&model.Model{
		ModelName:    "doubao-seedance-2.0",
		Status:       1,
		SyncOfficial: 1,
		NameRule:     model.NameRuleExact,
	}).Error)

	pricingByName := pricingByModelName(model.GetPricing())
	item, ok := pricingByName["doubao-seedance-2.0"]
	require.True(t, ok)
	require.NotZero(t, item.VendorID)

	var vendor model.Vendor
	require.NoError(t, db.First(&vendor, item.VendorID).Error)
	require.Equal(t, "字节跳动", vendor.Name)
	require.Equal(t, "Doubao.Color", vendor.Icon)

	var stored model.Model
	require.NoError(t, db.Where("model_name = ?", "doubao-seedance-2.0").First(&stored).Error)
	require.Equal(t, vendor.Id, stored.VendorID)
}

func TestAIPDDChannelEmptyModelsUseDefaultAbilities(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	model.InvalidatePricingCache()

	channel := &model.Channel{
		Type:   constant.ChannelTypeAIPDD,
		Key:    "aipdd-test-key",
		Name:   "aipdd",
		Status: common.ChannelStatusEnabled,
		Group:  "default",
	}
	require.NoError(t, channel.Insert())

	var abilities []model.Ability
	require.NoError(t, db.Where("channel_id = ?", channel.Id).Find(&abilities).Error)
	require.Len(t, abilities, len(constant.AIPDDTaskModelList))

	abilityModels := map[string]bool{}
	for _, ability := range abilities {
		abilityModels[ability.Model] = true
	}
	for _, modelName := range constant.AIPDDTaskModelList {
		require.True(t, abilityModels[modelName], "expected ability for %s", modelName)
	}
}

func TestEnsureAIPDDChannelDefaultsBackfillsExistingBlankChannel(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	model.InvalidatePricingCache()

	channel := model.Channel{
		Type:   constant.ChannelTypeAIPDD,
		Key:    "aipdd-test-key",
		Name:   "legacy-aipdd",
		Status: common.ChannelStatusEnabled,
		Group:  "default",
	}
	require.NoError(t, db.Create(&channel).Error)

	require.NoError(t, model.EnsureAIPDDChannelDefaults())

	var stored model.Channel
	require.NoError(t, db.First(&stored, channel.Id).Error)
	require.Equal(t, strings.Join(constant.AIPDDTaskModelList, ","), stored.Models)

	groupModels := model.GetGroupEnabledModels("default")
	for _, modelName := range constant.AIPDDTaskModelList {
		require.Contains(t, groupModels, modelName)
	}
}

func TestGetUserModelsExcludesAIPDDTaskModelsFromPlayground(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	model.InvalidatePricingCache()

	require.NoError(t, db.Create(&model.User{
		Id:       1002,
		Username: "playground-user",
		Password: "password",
		Group:    "default",
		Status:   common.UserStatusEnabled,
	}).Error)
	require.NoError(t, db.Create(&model.Channel{
		Type:   constant.ChannelTypeAIPDD,
		Key:    "aipdd-test-key",
		Name:   "legacy-aipdd",
		Status: common.ChannelStatusEnabled,
		Group:  "default",
	}).Error)

	require.NoError(t, model.EnsureAIPDDChannelDefaults())

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/user/models", nil)
	ctx.Set("id", 1002)

	GetUserModels(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	var payload userModelsResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
	require.True(t, payload.Success)
	for _, modelName := range constant.AIPDDTaskModelList {
		require.NotContains(t, payload.Data, modelName)
	}
}

func TestGetUserModelsIncludesAIPDDTaskModelsWhenRequested(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	model.InvalidatePricingCache()

	require.NoError(t, db.Create(&model.User{
		Id:       1010,
		Username: "token-models-user",
		Password: "password",
		Group:    "default",
		Status:   common.UserStatusEnabled,
	}).Error)
	require.NoError(t, db.Create(&model.Channel{
		Type:   constant.ChannelTypeAIPDD,
		Key:    "aipdd-token-key",
		Name:   "legacy-aipdd-token",
		Status: common.ChannelStatusEnabled,
		Group:  "default",
	}).Error)

	require.NoError(t, model.EnsureAIPDDChannelDefaults())

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/user/models?include_task_models=1", nil)
	ctx.Set("id", 1010)

	GetUserModels(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	var payload userModelsResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
	require.True(t, payload.Success)
	require.NotEmpty(t, constant.AIPDDTaskModelList)
	for _, modelName := range constant.AIPDDTaskModelList {
		require.Contains(t, payload.Data, modelName)
	}
}

func TestGetUserModelsIncludesImageToImageModelsForPlayground(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	model.InvalidatePricingCache()

	require.NoError(t, db.Create(&model.User{
		Id:       1003,
		Username: "playground-image-user",
		Password: "password",
		Group:    "default",
		Status:   common.UserStatusEnabled,
	}).Error)

	openAIChannel := &model.Channel{
		Type:   constant.ChannelTypeOpenAI,
		Key:    "openai-test-key",
		Name:   "openai",
		Models: "gpt-image-1,gpt-4o,custom-t2i,custom-img2img,custom-image-edit",
		Status: common.ChannelStatusEnabled,
		Group:  "default",
	}
	require.NoError(t, openAIChannel.Insert())

	imageGenerationEndpoints, err := common.Marshal(map[string]string{
		string(constant.EndpointTypeImageGeneration): "/v1/images/generations",
	})
	require.NoError(t, err)
	imageToImageEndpoints, err := common.Marshal(map[string]string{
		string(constant.EndpointTypeImageToImage): "/v1/images/edits",
	})
	require.NoError(t, err)
	imageEditEndpoints, err := common.Marshal(map[string]string{
		string(constant.EndpointTypeImageEdit): "/v1/images/edits",
	})
	require.NoError(t, err)
	require.NoError(t, db.Create(&model.Model{
		ModelName: "custom-t2i",
		Tags:      "文生图",
		Endpoints: string(imageGenerationEndpoints),
		Status:    1,
		NameRule:  model.NameRuleExact,
	}).Error)
	require.NoError(t, db.Create(&model.Model{
		ModelName: "custom-image-edit",
		Tags:      "图片编辑",
		Endpoints: string(imageEditEndpoints),
		Status:    1,
		NameRule:  model.NameRuleExact,
	}).Error)
	require.NoError(t, db.Create(&model.Model{
		ModelName: "custom-img2img",
		Tags:      "图生图",
		Endpoints: string(imageToImageEndpoints),
		Status:    1,
		NameRule:  model.NameRuleExact,
	}).Error)

	require.NoError(t, db.Create(&model.Channel{
		Type:   constant.ChannelTypeAIPDD,
		Key:    "aipdd-test-key",
		Name:   "legacy-aipdd",
		Status: common.ChannelStatusEnabled,
		Group:  "default",
	}).Error)
	require.NoError(t, model.EnsureAIPDDChannelDefaults())

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/user/models?endpoint_type=image-generation", nil)
	ctx.Set("id", 1003)

	GetUserModels(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	var payload userModelsResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
	require.True(t, payload.Success)
	require.Contains(t, payload.Data, "gpt-image-1")
	require.Contains(t, payload.Data, "custom-t2i")
	require.Contains(t, payload.Data, constant.AIPDDModelFluxGGUFT2I)
	require.NotContains(t, payload.Data, "custom-img2img")
	require.NotContains(t, payload.Data, "custom-image-edit")
	require.NotContains(t, payload.Data, constant.AIPDDModelFluxGGUF)
	require.NotContains(t, payload.Data, "gpt-4o")
	require.NotContains(t, payload.Data, constant.AIPDDModelIndexTTS)

	model.InvalidatePricingCache()
	recorder = httptest.NewRecorder()
	ctx, _ = gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/user/models?endpoint_type=image-to-image", nil)
	ctx.Set("id", 1003)

	GetUserModels(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
	require.True(t, payload.Success)
	require.Contains(t, payload.Data, "custom-img2img")
	require.Contains(t, payload.Data, constant.AIPDDModelFluxGGUF)
	require.NotContains(t, payload.Data, "custom-t2i")
	require.NotContains(t, payload.Data, "custom-image-edit")
	require.NotContains(t, payload.Data, constant.AIPDDModelFluxGGUFT2I)

	model.InvalidatePricingCache()
	recorder = httptest.NewRecorder()
	ctx, _ = gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/user/models?endpoint_type=image-edit", nil)
	ctx.Set("id", 1003)

	GetUserModels(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
	require.True(t, payload.Success)
	require.Contains(t, payload.Data, "custom-image-edit")
	require.NotContains(t, payload.Data, "custom-t2i")
	require.NotContains(t, payload.Data, "custom-img2img")
}

func TestGetUserModelsFiltersChatEndpointForPlayground(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	constant.ResetAIPDDOpenAIModels()
	t.Cleanup(constant.ResetAIPDDOpenAIModels)
	model.InvalidatePricingCache()

	require.NoError(t, db.Create(&model.User{
		Id:       1004,
		Username: "playground-chat-user",
		Password: "password",
		Group:    "default",
		Status:   common.UserStatusEnabled,
	}).Error)

	openAIChannel := &model.Channel{
		Type:   constant.ChannelTypeOpenAI,
		Key:    "openai-test-key",
		Name:   "openai",
		Models: "gpt-4o",
		Status: common.ChannelStatusEnabled,
		Group:  "default",
	}
	require.NoError(t, openAIChannel.Insert())

	doubaoVideoChannel := &model.Channel{
		Type:   constant.ChannelTypeDoubaoVideo,
		Key:    "doubao-test-key",
		Name:   "doubao-video",
		Models: "doubao-seedance-2.0",
		Status: common.ChannelStatusEnabled,
		Group:  "default",
	}
	require.NoError(t, doubaoVideoChannel.Insert())

	volcImageChannel := &model.Channel{
		Type:   constant.ChannelTypeVolcEngine,
		Key:    "volc-test-key",
		Name:   "volcengine",
		Models: "doubao-seedream-4-5",
		Status: common.ChannelStatusEnabled,
		Group:  "default",
	}
	require.NoError(t, volcImageChannel.Insert())

	aipddChannel := &model.Channel{
		Type:   constant.ChannelTypeAIPDD,
		Key:    "aipdd-test-key",
		Name:   "aipdd",
		Models: "gemma3:1b," + constant.AIPDDModelFluxGGUFT2I,
		Status: common.ChannelStatusEnabled,
		Group:  "default",
	}
	require.NoError(t, aipddChannel.Insert())

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/user/models?endpoint_type=openai", nil)
	ctx.Set("id", 1004)

	GetUserModels(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	var payload userModelsResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
	require.True(t, payload.Success)
	require.Contains(t, payload.Data, "gpt-4o")
	require.Contains(t, payload.Data, "gemma3:1b")
	require.NotContains(t, payload.Data, "doubao-seedance-2.0")
	require.NotContains(t, payload.Data, "doubao-seedream-4-5")
	require.NotContains(t, payload.Data, constant.AIPDDModelFluxGGUFT2I)
}

func TestListModelsIncludesTieredBillingModel(t *testing.T) {
	withSelfUseModeDisabled(t)
	withTieredBillingConfig(t, map[string]string{
		"zz-tiered-visible-model":      "tiered_expr",
		"zz-tiered-empty-expr-model":   "tiered_expr",
		"zz-tiered-missing-expr-model": "tiered_expr",
	}, map[string]string{
		"zz-tiered-visible-model":    `tier("base", p * 1 + c * 2)`,
		"zz-tiered-empty-expr-model": "   ",
	})

	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.Create(&model.User{
		Id:       1001,
		Username: "model-list-user",
		Password: "password",
		Group:    "default",
		Status:   common.UserStatusEnabled,
	}).Error)
	require.NoError(t, db.Create(&[]model.Ability{
		{Group: "default", Model: "zz-tiered-visible-model", ChannelId: 1, Enabled: true},
		{Group: "default", Model: "zz-tiered-empty-expr-model", ChannelId: 1, Enabled: true},
		{Group: "default", Model: "zz-tiered-missing-expr-model", ChannelId: 1, Enabled: true},
		{Group: "default", Model: "zz-unpriced-model", ChannelId: 1, Enabled: true},
	}).Error)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	ctx.Set("id", 1001)

	ListModels(ctx, constant.ChannelTypeOpenAI)

	ids := decodeListModelsResponse(t, recorder)
	require.Contains(t, ids, "zz-tiered-visible-model")
	require.NotContains(t, ids, "zz-tiered-empty-expr-model")
	require.NotContains(t, ids, "zz-tiered-missing-expr-model")
	require.NotContains(t, ids, "zz-unpriced-model")

	pricingByName := pricingByModelName(model.GetPricing())
	visiblePricing, ok := pricingByName["zz-tiered-visible-model"]
	require.True(t, ok)
	require.Equal(t, "tiered_expr", visiblePricing.BillingMode)
	require.NotEmpty(t, visiblePricing.BillingExpr)

	emptyExprPricing, ok := pricingByName["zz-tiered-empty-expr-model"]
	require.True(t, ok)
	require.Empty(t, emptyExprPricing.BillingMode)
	require.Empty(t, emptyExprPricing.BillingExpr)

	missingExprPricing, ok := pricingByName["zz-tiered-missing-expr-model"]
	require.True(t, ok)
	require.Empty(t, missingExprPricing.BillingMode)
	require.Empty(t, missingExprPricing.BillingExpr)
}

func TestListModelsIncludesDefaultPricedAIPDDModelWhenModelPriceOptionIsStale(t *testing.T) {
	withSelfUseModeDisabled(t)
	savedModelPrice := ratio_setting.ModelPrice2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(savedModelPrice))
		model.InvalidatePricingCache()
	})

	staleModelPrices := map[string]float64{}
	for modelName, price := range ratio_setting.GetDefaultModelPriceMap() {
		if modelName == constant.AIPDDModelFluxGGUFT2I {
			continue
		}
		staleModelPrices[modelName] = price
	}
	staleBytes, err := common.Marshal(staleModelPrices)
	require.NoError(t, err)
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(string(staleBytes)))

	db := setupModelListControllerTestDB(t)
	model.InvalidatePricingCache()
	require.NoError(t, db.Create(&model.User{
		Id:       1005,
		Username: "model-list-aipdd-user",
		Password: "password",
		Group:    "default",
		Status:   common.UserStatusEnabled,
	}).Error)

	channel := &model.Channel{
		Type:   constant.ChannelTypeAIPDD,
		Key:    "aipdd-test-key",
		Name:   "aipdd",
		Models: strings.Join(constant.AIPDDTaskModelList, ","),
		Status: common.ChannelStatusEnabled,
		Group:  "default",
	}
	require.NoError(t, channel.Insert())

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	ctx.Set("id", 1005)

	ListModels(ctx, constant.ChannelTypeOpenAI)

	ids := decodeListModelsResponse(t, recorder)
	require.Contains(t, ids, constant.AIPDDModelFluxGGUFT2I)
}

func TestListModelsTokenLimitIncludesTieredBillingModel(t *testing.T) {
	withSelfUseModeDisabled(t)
	withTieredBillingConfig(t, map[string]string{
		"zz-token-tiered-visible-model":      "tiered_expr",
		"zz-token-tiered-empty-expr-model":   "tiered_expr",
		"zz-token-tiered-missing-expr-model": "tiered_expr",
	}, map[string]string{
		"zz-token-tiered-visible-model":    `tier("base", p * 1 + c * 2)`,
		"zz-token-tiered-empty-expr-model": "",
	})

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	common.SetContextKey(ctx, constant.ContextKeyTokenModelLimitEnabled, true)
	common.SetContextKey(ctx, constant.ContextKeyTokenModelLimit, map[string]bool{
		"zz-token-tiered-visible-model":      true,
		"zz-token-tiered-empty-expr-model":   true,
		"zz-token-tiered-missing-expr-model": true,
		"zz-token-unpriced-model":            true,
	})

	ListModels(ctx, constant.ChannelTypeOpenAI)

	ids := decodeListModelsResponse(t, recorder)
	require.Contains(t, ids, "zz-token-tiered-visible-model")
	require.NotContains(t, ids, "zz-token-tiered-empty-expr-model")
	require.NotContains(t, ids, "zz-token-tiered-missing-expr-model")
	require.NotContains(t, ids, "zz-token-unpriced-model")
}

func TestListModelsTokenLimitIncludesExplicitAIPDDFreeModel(t *testing.T) {
	withSelfUseModeDisabled(t)
	t.Cleanup(aipddcatalog.ResetExplicitFreeModels)
	aipddcatalog.SetExplicitFreeModels([]string{"free-catalog-list-model"})

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	common.SetContextKey(ctx, constant.ContextKeyTokenModelLimitEnabled, true)
	common.SetContextKey(ctx, constant.ContextKeyTokenModelLimit, map[string]bool{
		"free-catalog-list-model": true,
		"zz-unpriced-model":       true,
	})

	ListModels(ctx, constant.ChannelTypeOpenAI)

	ids := decodeListModelsResponse(t, recorder)
	require.Contains(t, ids, "free-catalog-list-model")
	require.NotContains(t, ids, "zz-unpriced-model")
}

func TestListModelsHidesDisabledAIPDDCatalogModelsOnly(t *testing.T) {
	originalSelfUse := operation_setting.SelfUseModeEnabled
	operation_setting.SelfUseModeEnabled = true
	t.Cleanup(func() {
		operation_setting.SelfUseModeEnabled = originalSelfUse
		aipddcatalog.ResetV1ModelsListHidden()
	})
	aipddcatalog.ResetV1ModelsListHidden()

	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.Create(&model.User{
		Id:       1101,
		Username: "v1-models-aipdd-filter-user",
		Password: "password",
		Group:    "default",
		Status:   common.UserStatusEnabled,
	}).Error)

	enabledTask := "aipdd-list-enabled-task"
	unavailableTask := "aipdd-list-unavailable-task"
	pricingDisabledTask := "aipdd-list-pricing-disabled-task"
	nonAIPDD := "gpt-custom-open-model"
	legacyMissing := "aipdd-list-legacy-missing"

	require.NoError(t, db.Create(&[]model.Ability{
		{Group: "default", Model: enabledTask, ChannelId: 1, Enabled: true},
		{Group: "default", Model: unavailableTask, ChannelId: 1, Enabled: true},
		{Group: "default", Model: pricingDisabledTask, ChannelId: 1, Enabled: true},
		{Group: "default", Model: nonAIPDD, ChannelId: 2, Enabled: true},
		{Group: "default", Model: legacyMissing, ChannelId: 1, Enabled: true},
	}).Error)
	require.NoError(t, db.Create(&[]model.Model{
		{ModelName: enabledTask, Status: 1},
		{ModelName: unavailableTask, Status: 1},
		{ModelName: pricingDisabledTask, Status: 1},
		{ModelName: nonAIPDD, Status: 1},
		{ModelName: legacyMissing, Status: 1},
	}).Error)

	catalog := aipddcatalog.AtomicCatalog{
		Capabilities: []aipddcatalog.AtomicCapability{
			{ID: enabledTask, Available: aipddcatalog.BoolPtr(true), Pricing: aipddcatalog.AtomicPricing{Enabled: true}},
			{ID: unavailableTask, Available: aipddcatalog.BoolPtr(false), Pricing: aipddcatalog.AtomicPricing{Enabled: true}},
			{ID: pricingDisabledTask, Available: aipddcatalog.BoolPtr(true), Pricing: aipddcatalog.AtomicPricing{Enabled: false}},
		},
	}
	aipddcatalog.SetV1ModelsListHidden(catalog.V1ModelsListHiddenNames())

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	ctx.Set("id", 1101)
	ListModels(ctx, constant.ChannelTypeOpenAI)

	ids := decodeListModelsResponse(t, recorder)
	require.Contains(t, ids, enabledTask)
	require.Contains(t, ids, nonAIPDD)
	require.Contains(t, ids, legacyMissing)
	require.Contains(t, ids, unavailableTask)
	require.NotContains(t, ids, pricingDisabledTask)

	adminRecorder := httptest.NewRecorder()
	adminCtx, _ := gin.CreateTestContext(adminRecorder)
	adminCtx.Request = httptest.NewRequest(http.MethodGet, "/api/models/?p=1&page_size=100", nil)
	GetAllModelsMeta(adminCtx)
	require.Equal(t, http.StatusOK, adminRecorder.Code)

	var adminPayload struct {
		Success bool `json:"success"`
		Data    struct {
			Items []model.Model `json:"items"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(adminRecorder.Body.Bytes(), &adminPayload))
	require.True(t, adminPayload.Success)
	adminNames := make(map[string]struct{}, len(adminPayload.Data.Items))
	for _, item := range adminPayload.Data.Items {
		adminNames[item.ModelName] = struct{}{}
	}
	require.Contains(t, adminNames, unavailableTask)
	require.Contains(t, adminNames, pricingDisabledTask)
	require.Contains(t, adminNames, enabledTask)
}

func TestListModelsDoesNotFilterWhenAIPDDCatalogStateUnavailable(t *testing.T) {
	originalSelfUse := operation_setting.SelfUseModeEnabled
	operation_setting.SelfUseModeEnabled = true
	t.Cleanup(func() {
		operation_setting.SelfUseModeEnabled = originalSelfUse
		aipddcatalog.ResetV1ModelsListHidden()
	})
	aipddcatalog.ResetV1ModelsListHidden()

	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.Create(&model.User{
		Id:       1102,
		Username: "v1-models-no-catalog-user",
		Password: "password",
		Group:    "default",
		Status:   common.UserStatusEnabled,
	}).Error)
	require.NoError(t, db.Create(&[]model.Ability{
		{Group: "default", Model: "aipdd-would-be-hidden", ChannelId: 1, Enabled: true},
		{Group: "default", Model: "gpt-custom-open-model", ChannelId: 2, Enabled: true},
	}).Error)

	require.False(t, aipddcatalog.HasV1ModelsListHiddenState())

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	ctx.Set("id", 1102)
	ListModels(ctx, constant.ChannelTypeOpenAI)

	ids := decodeListModelsResponse(t, recorder)
	require.Contains(t, ids, "aipdd-would-be-hidden")
	require.Contains(t, ids, "gpt-custom-open-model")
}
