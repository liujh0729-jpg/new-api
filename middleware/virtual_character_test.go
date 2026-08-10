package middleware

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestVirtualCharacterRequestHasExternalReferences(t *testing.T) {
	tests := []struct {
		name string
		req  relaycommon.TaskSubmitReq
		want bool
	}{
		{name: "none", req: relaycommon.TaskSubmitReq{}, want: false},
		{name: "empty content", req: relaycommon.TaskSubmitReq{Metadata: map[string]interface{}{"content": []interface{}{}}}, want: false},
		{name: "image list", req: relaycommon.TaskSubmitReq{Images: []string{"https://example.com/ref.png"}}, want: true},
		{name: "first frame", req: relaycommon.TaskSubmitReq{FirstFrame: "https://example.com/ref.png"}, want: true},
		{name: "metadata content", req: relaycommon.TaskSubmitReq{Metadata: map[string]interface{}{"content": []interface{}{map[string]interface{}{"type": "image_url"}}}}, want: true},
		{name: "malformed content", req: relaycommon.TaskSubmitReq{Metadata: map[string]interface{}{"content": "unexpected"}}, want: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.want, virtualCharacterRequestHasExternalReferences(test.req))
		})
	}
}

func TestBindVirtualCharacterUsesActiveOwnedAsset(t *testing.T) {
	previousDB := model.DB
	previousSecret := common.CryptoSecret
	previousConfigured := common.CryptoSecretConfigured
	db, err := gorm.Open(sqlite.Open("file:middleware_virtual_character?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.VirtualCharacter{}, &model.VirtualCharacterAsset{}, &model.VirtualCharacterProviderAccount{}, &model.VirtualCharacterTask{}))
	model.DB = db
	common.CryptoSecret = strings.Repeat("k", 32)
	common.CryptoSecretConfigured = true
	t.Cleanup(func() {
		model.DB = previousDB
		common.CryptoSecret = previousSecret
		common.CryptoSecretConfigured = previousConfigured
		if sqlDB, dbErr := db.DB(); dbErr == nil {
			_ = sqlDB.Close()
		}
	})

	account := &model.VirtualCharacterProviderAccount{ID: 1, Enabled: true, OfficialEnabled: true, RealPersonEnabled: true, Region: "cn-beijing", ProjectName: "default"}
	require.NoError(t, db.Create(account).Error)
	slot := 1
	character := &model.VirtualCharacter{UserID: 101, Slot: &slot, Scope: model.VirtualCharacterScopePrivate, SourceType: model.VirtualCharacterSourceVolcAIGC, Name: "Virtual", Status: model.VirtualCharacterStatusActive, ValidationStatus: model.VirtualCharacterValidationAccepted, ProviderAccountID: account.ID, ProviderGroupID: "group-1"}
	require.NoError(t, db.Create(character).Error)
	asset := &model.VirtualCharacterAsset{CharacterID: character.ID, ProviderAccountID: account.ID, ProviderAssetID: "provider-asset-1", Name: "Look 1", AssetType: model.VirtualCharacterAssetTypeImage, Status: model.VirtualCharacterAssetStatusActive, IsPrimary: true}
	require.NoError(t, db.Create(asset).Error)
	require.NoError(t, db.Model(character).Update("primary_asset_id", asset.ID).Error)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("id", 101) }, BindVirtualCharacter())
	router.POST("/v1/video/generations", func(c *gin.Context) {
		var req relaycommon.TaskSubmitReq
		require.NoError(t, common.UnmarshalBodyReusable(c, &req))
		require.Equal(t, []string{"asset://provider-asset-1"}, req.Images)
		boundAsset, ok := GetBoundVirtualCharacterAsset(c)
		require.True(t, ok)
		require.Equal(t, asset.ID, boundAsset.ID)
		c.Set(VirtualCharacterTaskClaimedKey, true)
		c.Status(http.StatusNoContent)
	})

	body := fmt.Sprintf(`{"character_id":%d,"character_asset_id":%d,"model":"doubao-seedance-2-0-260128","prompt":"test"}`, character.ID, asset.ID)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/video/generations", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusNoContent, recorder.Code)
	var link model.VirtualCharacterTask
	require.NoError(t, db.Where("character_id = ?", character.ID).First(&link).Error)
	require.Equal(t, asset.ID, link.CharacterAssetID)
	require.Equal(t, asset.ProviderAssetID, link.ProviderAssetID)

	// Non-Seedance models are rejected.
	badRecorder := httptest.NewRecorder()
	badBody := fmt.Sprintf(`{"character_id":%d,"character_asset_id":%d,"model":"other-video","prompt":"test"}`, character.ID, asset.ID)
	badRequest := httptest.NewRequest(http.MethodPost, "/v1/video/generations", strings.NewReader(badBody))
	badRequest.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(badRecorder, badRequest)
	require.Equal(t, http.StatusBadRequest, badRecorder.Code)
	require.Contains(t, badRecorder.Body.String(), "Seedance")

	// The same asset is invisible to another user.
	otherRouter := gin.New()
	otherRouter.Use(func(c *gin.Context) { c.Set("id", 202) }, BindVirtualCharacter())
	otherRouter.POST("/v1/video/generations", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	otherRecorder := httptest.NewRecorder()
	otherRequest := httptest.NewRequest(http.MethodPost, "/v1/video/generations", strings.NewReader(body))
	otherRequest.Header.Set("Content-Type", "application/json")
	otherRouter.ServeHTTP(otherRecorder, otherRequest)
	require.Equal(t, http.StatusNotFound, otherRecorder.Code)
}

func TestBindVirtualCharacterBlocksReservedRealPersonSource(t *testing.T) {
	previousDB := model.DB
	previousSecret := common.CryptoSecret
	previousConfigured := common.CryptoSecretConfigured
	db, err := gorm.Open(sqlite.Open("file:middleware_virtual_character_source?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.VirtualCharacter{}, &model.VirtualCharacterAsset{}, &model.VirtualCharacterProviderAccount{}, &model.VirtualCharacterTask{}))
	model.DB = db
	common.CryptoSecret = strings.Repeat("k", 32)
	common.CryptoSecretConfigured = true
	t.Cleanup(func() {
		model.DB = previousDB
		common.CryptoSecret = previousSecret
		common.CryptoSecretConfigured = previousConfigured
		if sqlDB, dbErr := db.DB(); dbErr == nil {
			_ = sqlDB.Close()
		}
	})

	account := &model.VirtualCharacterProviderAccount{
		ID: 1, Enabled: true, OfficialEnabled: false, VirtualEnabled: true, RealPersonEnabled: true,
		Region: "cn-beijing", ProjectName: "default",
	}
	require.NoError(t, db.Create(account).Error)
	slot := 1
	character := &model.VirtualCharacter{
		UserID: 303, Slot: &slot, Scope: model.VirtualCharacterScopePrivate,
		SourceType: model.VirtualCharacterSourceVolcRealPerson, Name: "Actor", Status: model.VirtualCharacterStatusActive,
		ValidationStatus: model.VirtualCharacterValidationAccepted, ProviderAccountID: account.ID, ProviderGroupID: "group-real",
	}
	require.NoError(t, db.Create(character).Error)
	asset := &model.VirtualCharacterAsset{
		CharacterID: character.ID, ProviderAccountID: account.ID, ProviderAssetID: "asset-real",
		Name: "Look", AssetType: model.VirtualCharacterAssetTypeImage, Status: model.VirtualCharacterAssetStatusActive, IsPrimary: true,
	}
	require.NoError(t, db.Create(asset).Error)
	require.NoError(t, db.Model(character).Update("primary_asset_id", asset.ID).Error)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("id", 303) }, BindVirtualCharacter())
	router.POST("/v1/video/generations", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	body := fmt.Sprintf(`{"character_id":%d,"character_asset_id":%d,"model":"doubao-seedance-2-0-260128","prompt":"test"}`, character.ID, asset.ID)
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/video/generations", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, req)
	require.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	require.Contains(t, recorder.Body.String(), "not available yet")
}

func TestBindVirtualCharacterAllowsOfficialPresetSource(t *testing.T) {
	previousDB := model.DB
	previousSecret := common.CryptoSecret
	previousConfigured := common.CryptoSecretConfigured
	db, err := gorm.Open(sqlite.Open("file:middleware_virtual_character_preset?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.VirtualCharacter{}, &model.VirtualCharacterAsset{}, &model.VirtualCharacterProviderAccount{}, &model.VirtualCharacterTask{}))
	model.DB = db
	common.CryptoSecret = strings.Repeat("k", 32)
	common.CryptoSecretConfigured = true
	t.Cleanup(func() {
		model.DB = previousDB
		common.CryptoSecret = previousSecret
		common.CryptoSecretConfigured = previousConfigured
		if sqlDB, dbErr := db.DB(); dbErr == nil {
			_ = sqlDB.Close()
		}
	})

	account := &model.VirtualCharacterProviderAccount{
		ID: 1, Enabled: true, OfficialEnabled: true, VirtualEnabled: true,
		Region: "cn-beijing", ProjectName: "default",
	}
	require.NoError(t, db.Create(account).Error)
	character := &model.VirtualCharacter{
		Scope: model.VirtualCharacterScopePublic, SourceType: model.VirtualCharacterSourceVolcPreset,
		Name: "Preset", Status: model.VirtualCharacterStatusActive,
		ValidationStatus: model.VirtualCharacterValidationAccepted, ProviderAccountID: account.ID,
	}
	require.NoError(t, db.Create(character).Error)
	asset := &model.VirtualCharacterAsset{
		CharacterID: character.ID, ProviderAccountID: account.ID, ProviderAssetID: "asset-preset",
		Name: "Look", AssetType: model.VirtualCharacterAssetTypeImage, Status: model.VirtualCharacterAssetStatusActive, IsPrimary: true,
	}
	require.NoError(t, db.Create(asset).Error)
	require.NoError(t, db.Model(character).Update("primary_asset_id", asset.ID).Error)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("id", 404) }, BindVirtualCharacter())
	router.POST("/v1/video/generations", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	body := fmt.Sprintf(`{"character_id":%d,"character_asset_id":%d,"model":"doubao-seedance-2-0-260128","prompt":"test"}`, character.ID, asset.ID)
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/video/generations", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, req)
	require.Equal(t, http.StatusNoContent, recorder.Code)
}
