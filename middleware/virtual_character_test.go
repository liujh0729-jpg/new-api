package middleware

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
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

func TestBindVirtualCharacterUsesActiveOwnedAssetAndStableChannel(t *testing.T) {
	previousDB := model.DB
	previousSecret := common.CryptoSecret
	previousConfigured := common.CryptoSecretConfigured
	previousOptions := common.OptionMap
	db, err := gorm.Open(sqlite.Open("file:middleware_virtual_character?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.VirtualCharacter{}, &model.VirtualCharacterAsset{}, &model.VirtualCharacterProviderAccount{}, &model.VirtualCharacterTask{}, &model.Channel{}))
	model.DB = db
	common.CryptoSecret = strings.Repeat("k", 32)
	common.CryptoSecretConfigured = true
	common.OptionMap = map[string]string{"VirtualCharacterModels": "doubao-seedance-2-0-260128"}
	t.Cleanup(func() {
		model.DB = previousDB
		common.CryptoSecret = previousSecret
		common.CryptoSecretConfigured = previousConfigured
		common.OptionMap = previousOptions
		if sqlDB, dbErr := db.DB(); dbErr == nil {
			_ = sqlDB.Close()
		}
	})

	channel := &model.Channel{Id: 9, Name: "stable-volc", Type: constant.ChannelTypeVolcEngine, Key: "video-key", Status: common.ChannelStatusEnabled, Models: "doubao-seedance-2-0-260128"}
	require.NoError(t, db.Create(channel).Error)
	account := &model.VirtualCharacterProviderAccount{ID: 1, Enabled: true, OfficialEnabled: true, RealPersonEnabled: true, ChannelID: channel.Id, Region: "cn-beijing", ProjectName: "default"}
	require.NoError(t, db.Create(account).Error)
	slot := 1
	character := &model.VirtualCharacter{UserID: 101, Slot: &slot, Scope: model.VirtualCharacterScopePrivate, SourceType: model.VirtualCharacterSourceVolcRealPerson, Name: "Actor", Status: model.VirtualCharacterStatusActive, ValidationStatus: model.VirtualCharacterValidationAccepted, ProviderAccountID: account.ID, ProviderGroupID: "group-1"}
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
		require.Equal(t, channel.Id, c.GetInt("channel_id"))
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

func TestChannelSupportsVirtualCharacterModelUsesExactNames(t *testing.T) {
	channel := &model.Channel{Models: "other-model, doubao-seedance-2-0-260128"}
	require.True(t, channelSupportsVirtualCharacterModel(channel, "doubao-seedance-2-0-260128"))
	require.False(t, channelSupportsVirtualCharacterModel(channel, "seedance-2-0"))
}
