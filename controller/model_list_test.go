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
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/operation_setting"
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

	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Channel{}, &model.Ability{}, &model.Model{}, &model.Vendor{}))

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
