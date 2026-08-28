package middleware

import (
	"bytes"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

const testVirtualCharacterVideoModel = "AP Seedance-2.0 VIP"

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

func TestBindVirtualCharacterUsesActiveOwnedImageAndIgnoresLegacyAssetID(t *testing.T) {
	previousDB := model.DB
	previousSecret := common.CryptoSecret
	previousConfigured := common.CryptoSecretConfigured
	db, err := gorm.Open(sqlite.Open("file:middleware_virtual_character?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.VirtualCharacter{}, &model.VirtualCharacterProviderAccount{}, &model.VirtualCharacterTask{}, &model.VirtualCharacterAuthorization{}, &model.VirtualCharacterTaskReference{}))
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
	character := &model.VirtualCharacter{UserID: 101, Slot: &slot, Scope: model.VirtualCharacterScopePrivate, SourceType: model.VirtualCharacterSourceVolcAIGC, Name: "Virtual", Status: model.VirtualCharacterStatusActive, ValidationStatus: model.VirtualCharacterValidationAccepted, ProviderAccountID: account.ID, ProviderGroupID: "group-1", ProviderAssetID: "provider-asset-1"}
	require.NoError(t, db.Create(character).Error)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("id", 101)
		common.SetContextKey(c, constant.ContextKeyChannelType, constant.ChannelTypeAIPDD)
	}, BindVirtualCharacter())
	router.POST("/v1/video/generations", func(c *gin.Context) {
		var req relaycommon.TaskSubmitReq
		require.NoError(t, common.UnmarshalBodyReusable(c, &req))
		require.Equal(t, constant.ChannelTypeAIPDD, common.GetContextKeyInt(c, constant.ContextKeyChannelType))
		require.Equal(t, []string{"asset://provider-asset-1"}, req.Images)
		require.Equal(t, "application/json", c.GetHeader("Content-Type"))
		_, leakedLegacyAssetID := req.Metadata["character_asset_id"]
		require.False(t, leakedLegacyAssetID)
		boundCharacter, ok := GetBoundVirtualCharacter(c)
		require.True(t, ok)
		require.Equal(t, character.ID, boundCharacter.ID)
		c.Set(VirtualCharacterTaskClaimedKey, true)
		c.Status(http.StatusNoContent)
	})

	body := fmt.Sprintf(`{"character_id":%d,"character_asset_id":999999,"model":"%s","prompt":"test"}`, character.ID, testVirtualCharacterVideoModel)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/video/generations", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusNoContent, recorder.Code)
	var link model.VirtualCharacterTask
	require.NoError(t, db.Where("character_id = ?", character.ID).First(&link).Error)
	require.Equal(t, character.ProviderAssetID, link.ProviderAssetID)

	// Multipart clients may continue sending the removed character_asset_id.
	// It is discarded, and the rewritten request is delivered downstream as JSON.
	multipartBody := &bytes.Buffer{}
	writer := multipart.NewWriter(multipartBody)
	require.NoError(t, writer.WriteField("character_id", fmt.Sprintf("%d", character.ID)))
	require.NoError(t, writer.WriteField("character_asset_id", "999999"))
	require.NoError(t, writer.WriteField("model", testVirtualCharacterVideoModel))
	require.NoError(t, writer.WriteField("prompt", "multipart test"))
	require.NoError(t, writer.Close())
	multipartRecorder := httptest.NewRecorder()
	multipartRequest := httptest.NewRequest(http.MethodPost, "/v1/video/generations", multipartBody)
	multipartRequest.Header.Set("Content-Type", writer.FormDataContentType())
	router.ServeHTTP(multipartRecorder, multipartRequest)
	require.Equal(t, http.StatusNoContent, multipartRecorder.Code, multipartRecorder.Body.String())

	// Non-Seedance models are rejected.
	badRecorder := httptest.NewRecorder()
	badBody := fmt.Sprintf(`{"character_id":%d,"character_asset_id":999999,"model":"other-video","prompt":"test"}`, character.ID)
	badRequest := httptest.NewRequest(http.MethodPost, "/v1/video/generations", strings.NewReader(badBody))
	badRequest.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(badRecorder, badRequest)
	require.Equal(t, http.StatusBadRequest, badRecorder.Code)
	require.Contains(t, badRecorder.Body.String(), "Seedance")

	// Channel type is not part of character-model eligibility. A user-visible
	// model containing "seedance" is sufficient.
	arbitraryChannelRouter := gin.New()
	arbitraryChannelRouter.Use(func(c *gin.Context) {
		c.Set("id", 101)
		common.SetContextKey(c, constant.ContextKeyChannelType, constant.ChannelTypeDoubaoVideo)
	}, BindVirtualCharacter())
	arbitraryChannelRouter.POST("/v1/video/generations", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	arbitraryChannelRecorder := httptest.NewRecorder()
	arbitraryChannelRequest := httptest.NewRequest(http.MethodPost, "/v1/video/generations", strings.NewReader(body))
	arbitraryChannelRequest.Header.Set("Content-Type", "application/json")
	arbitraryChannelRouter.ServeHTTP(arbitraryChannelRecorder, arbitraryChannelRequest)
	require.Equal(t, http.StatusNoContent, arbitraryChannelRecorder.Code)

	// The same private character is invisible to another user.
	otherRouter := gin.New()
	otherRouter.Use(func(c *gin.Context) {
		c.Set("id", 202)
		common.SetContextKey(c, constant.ContextKeyChannelType, constant.ChannelTypeAIPDD)
	}, BindVirtualCharacter())
	otherRouter.POST("/v1/video/generations", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	otherRecorder := httptest.NewRecorder()
	otherRequest := httptest.NewRequest(http.MethodPost, "/v1/video/generations", strings.NewReader(body))
	otherRequest.Header.Set("Content-Type", "application/json")
	otherRouter.ServeHTTP(otherRecorder, otherRequest)
	require.Equal(t, http.StatusNotFound, otherRecorder.Code)
}

func TestBindVirtualCharacterAllowsAuthorizedRealPersonSource(t *testing.T) {
	previousDB := model.DB
	previousSecret := common.CryptoSecret
	previousConfigured := common.CryptoSecretConfigured
	db, err := gorm.Open(sqlite.Open("file:middleware_virtual_character_source?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.VirtualCharacter{}, &model.VirtualCharacterProviderAccount{}, &model.VirtualCharacterTask{}, &model.VirtualCharacterAuthorization{}, &model.VirtualCharacterTaskReference{}))
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
		Region: "cn-beijing", ProjectName: "default", ChannelID: 7,
	}
	require.NoError(t, db.Create(account).Error)
	slot := 1
	character := &model.VirtualCharacter{
		UserID: 303, RealPersonSlot: &slot, Scope: model.VirtualCharacterScopePrivate,
		SourceType: model.VirtualCharacterSourceVolcRealPerson, Name: "Actor", Status: model.VirtualCharacterStatusActive,
		ValidationStatus: model.VirtualCharacterValidationAccepted, ProviderAccountID: account.ID, ProviderGroupID: "group-real", ProviderAssetID: "asset-real",
	}
	require.NoError(t, db.Create(character).Error)
	now := time.Now().Unix()
	require.NoError(t, db.Create(&model.VirtualCharacterAuthorization{
		CharacterID: character.ID, UserID: character.UserID, Status: model.VirtualCharacterAuthorizationActive,
		ValidFrom: now - 60, ValidUntil: now + 3600, ProviderGroupType: model.VirtualCharacterRealPersonGroupType,
		ProviderGroupStatus: "Active", ProviderAssetStatus: "Active", ProviderCheckedAt: now,
		AgreementReference: "session-1", ConsentReceiptHash: strings.Repeat("a", 64), HolderScopeAcceptedAt: now - 120,
	}).Error)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("id", 303)
		common.SetContextKey(c, constant.ContextKeyChannelId, 7)
		common.SetContextKey(c, constant.ContextKeyChannelType, constant.ChannelTypeAIPDD)
	}, BindVirtualCharacter())
	router.POST("/v1/video/generations", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	body := fmt.Sprintf(`{"character_id":%d,"model":"%s","prompt":"test"}`, character.ID, testVirtualCharacterVideoModel)
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/video/generations", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, req)
	require.Equal(t, http.StatusNoContent, recorder.Code, recorder.Body.String())
}

func TestBindVirtualCharacterAllowsOfficialPresetSource(t *testing.T) {
	previousDB := model.DB
	previousSecret := common.CryptoSecret
	previousConfigured := common.CryptoSecretConfigured
	db, err := gorm.Open(sqlite.Open("file:middleware_virtual_character_preset?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.VirtualCharacter{}, &model.VirtualCharacterProviderAccount{}, &model.VirtualCharacterTask{}, &model.VirtualCharacterAuthorization{}, &model.VirtualCharacterTaskReference{}))
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
		ValidationStatus: model.VirtualCharacterValidationAccepted, ProviderAccountID: account.ID, ProviderAssetID: "asset-preset",
	}
	require.NoError(t, db.Create(character).Error)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("id", 404)
		common.SetContextKey(c, constant.ContextKeyChannelType, constant.ChannelTypeAIPDD)
	}, BindVirtualCharacter())
	router.POST("/v1/video/generations", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	body := fmt.Sprintf(`{"character_id":%d,"model":"%s","prompt":"test"}`, character.ID, testVirtualCharacterVideoModel)
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/video/generations", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, req)
	require.Equal(t, http.StatusNoContent, recorder.Code)
}

func TestBindVirtualCharacterAuthorizesMixedDirectAssetReferences(t *testing.T) {
	previousDB := model.DB
	previousSecret := common.CryptoSecret
	previousConfigured := common.CryptoSecretConfigured
	db, err := gorm.Open(sqlite.Open("file:middleware_virtual_character_direct_assets?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&model.VirtualCharacter{}, &model.VirtualCharacterProviderAccount{}, &model.VirtualCharacterTask{},
		&model.VirtualCharacterAuthorization{}, &model.VirtualCharacterTaskReference{},
	))
	model.DB = db
	common.CryptoSecret = strings.Repeat("d", 32)
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
		ID: 1, Enabled: true, OfficialEnabled: true, VirtualEnabled: true, RealPersonEnabled: true,
		Region: "cn-beijing", ProjectName: "default", ChannelID: 54,
	}
	require.NoError(t, db.Create(account).Error)
	preset := &model.VirtualCharacter{
		Scope: model.VirtualCharacterScopePublic, SourceType: model.VirtualCharacterSourceVolcPreset,
		Name: "Preset", Status: model.VirtualCharacterStatusActive, ValidationStatus: model.VirtualCharacterValidationAccepted,
		ProviderAccountID: account.ID, ProviderAssetID: "asset-preset",
	}
	require.NoError(t, db.Create(preset).Error)
	slot := 1
	realPerson := &model.VirtualCharacter{
		UserID: 701, RealPersonSlot: &slot, Scope: model.VirtualCharacterScopePrivate,
		SourceType: model.VirtualCharacterSourceVolcRealPerson, Name: "Actor", Status: model.VirtualCharacterStatusActive,
		ValidationStatus: model.VirtualCharacterValidationAccepted, ProviderAccountID: account.ID,
		ProviderGroupID: "group-real", ProviderAssetID: "asset-real",
	}
	require.NoError(t, db.Create(realPerson).Error)
	now := time.Now().Unix()
	require.NoError(t, db.Create(&model.VirtualCharacterAuthorization{
		CharacterID: realPerson.ID, UserID: realPerson.UserID, Status: model.VirtualCharacterAuthorizationActive,
		ValidFrom: now - 60, ValidUntil: now + 3600, ProviderGroupType: model.VirtualCharacterRealPersonGroupType,
		ProviderGroupStatus: "Active", ProviderAssetStatus: "Active", ProviderCheckedAt: now,
		AgreementReference: "session-2", ConsentReceiptHash: strings.Repeat("b", 64), HolderScopeAcceptedAt: now - 120,
	}).Error)

	newRouter := func(userID int) *gin.Engine {
		router := gin.New()
		router.Use(func(c *gin.Context) {
			c.Set("id", userID)
			common.SetContextKey(c, constant.ContextKeyChannelId, 54)
			common.SetContextKey(c, constant.ContextKeyChannelType, constant.ChannelTypeAIPDD)
		}, BindVirtualCharacter())
		router.POST("/v1/videos", func(c *gin.Context) {
			c.Set(VirtualCharacterTaskClaimedKey, true)
			c.Status(http.StatusNoContent)
		})
		return router
	}
	body := `{"model":"` + testVirtualCharacterVideoModel + `","prompt":"test","content":[{"type":"image_url","image_url":{"url":"asset://asset-preset"},"role":"reference_image"},{"type":"image_url","image_url":{"url":"asset://asset-real"},"role":"reference_image"}]}`
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/videos", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	newRouter(realPerson.UserID).ServeHTTP(recorder, request)
	require.Equal(t, http.StatusNoContent, recorder.Code, recorder.Body.String())
	var references []model.VirtualCharacterTaskReference
	require.NoError(t, db.Order("character_id ASC").Find(&references).Error)
	require.Len(t, references, 2)
	require.NotEmpty(t, references[1].AuthorizationSnapshotJSON)

	forbidden := httptest.NewRecorder()
	forbiddenRequest := httptest.NewRequest(http.MethodPost, "/v1/videos", strings.NewReader(body))
	forbiddenRequest.Header.Set("Content-Type", "application/json")
	newRouter(702).ServeHTTP(forbidden, forbiddenRequest)
	require.Equal(t, http.StatusForbidden, forbidden.Code, forbidden.Body.String())

	unknownBody := `{"model":"` + testVirtualCharacterVideoModel + `","prompt":"test","content":[{"type":"image_url","image_url":{"url":"asset://asset-unknown"}}]}`
	unknown := httptest.NewRecorder()
	unknownRequest := httptest.NewRequest(http.MethodPost, "/v1/videos", strings.NewReader(unknownBody))
	unknownRequest.Header.Set("Content-Type", "application/json")
	newRouter(realPerson.UserID).ServeHTTP(unknown, unknownRequest)
	require.Equal(t, http.StatusNotFound, unknown.Code, unknown.Body.String())
}

func TestBindVirtualCharacterInjectsVideoAndAudioContent(t *testing.T) {
	previousDB := model.DB
	previousSecret := common.CryptoSecret
	previousConfigured := common.CryptoSecretConfigured
	db, err := gorm.Open(sqlite.Open("file:middleware_virtual_character_media?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.VirtualCharacter{}, &model.VirtualCharacterProviderAccount{}, &model.VirtualCharacterTask{}, &model.VirtualCharacterAuthorization{}, &model.VirtualCharacterTaskReference{}))
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

	account := &model.VirtualCharacterProviderAccount{ID: 1, Enabled: true, OfficialEnabled: true, VirtualEnabled: true, Region: "cn-beijing", ProjectName: "default"}
	require.NoError(t, db.Create(account).Error)
	videoSlot, audioSlot := 1, 2
	video := &model.VirtualCharacter{
		UserID: 201, Slot: &videoSlot, Scope: model.VirtualCharacterScopePrivate, SourceType: model.VirtualCharacterSourceVolcAIGC,
		Name: "Clip", Status: model.VirtualCharacterStatusActive, ValidationStatus: model.VirtualCharacterValidationAccepted,
		ProviderAccountID: account.ID, ProviderAssetID: "video-asset-1", AssetType: model.VirtualCharacterAssetTypeVideo,
	}
	audio := &model.VirtualCharacter{
		UserID: 201, Slot: &audioSlot, Scope: model.VirtualCharacterScopePrivate, SourceType: model.VirtualCharacterSourceVolcAIGC,
		Name: "Voice", Status: model.VirtualCharacterStatusActive, ValidationStatus: model.VirtualCharacterValidationAccepted,
		ProviderAccountID: account.ID, ProviderAssetID: "audio-asset-1", AssetType: model.VirtualCharacterAssetTypeAudio,
	}
	require.NoError(t, db.Create(video).Error)
	require.NoError(t, db.Create(audio).Error)

	newRouter := func(assertContent func(t *testing.T, req relaycommon.TaskSubmitReq)) *gin.Engine {
		router := gin.New()
		router.Use(func(c *gin.Context) {
			c.Set("id", 201)
			common.SetContextKey(c, constant.ContextKeyChannelType, constant.ChannelTypeAIPDD)
		}, BindVirtualCharacter())
		router.POST("/v1/video/generations", func(c *gin.Context) {
			var req relaycommon.TaskSubmitReq
			require.NoError(t, common.UnmarshalBodyReusable(c, &req))
			assertContent(t, req)
			c.Set(VirtualCharacterTaskClaimedKey, true)
			c.Status(http.StatusNoContent)
		})
		return router
	}

	videoRecorder := httptest.NewRecorder()
	videoRequest := httptest.NewRequest(http.MethodPost, "/v1/video/generations", strings.NewReader(fmt.Sprintf(
		`{"character_id":%d,"model":"%s","prompt":"walk"}`, video.ID, testVirtualCharacterVideoModel,
	)))
	videoRequest.Header.Set("Content-Type", "application/json")
	newRouter(func(t *testing.T, req relaycommon.TaskSubmitReq) {
		require.Empty(t, req.Images)
		content, ok := req.Metadata["content"].([]interface{})
		require.True(t, ok)
		require.Len(t, content, 2)
		media := content[1].(map[string]interface{})
		require.Equal(t, "video_url", media["type"])
		require.Equal(t, "reference_video", media["role"])
		require.Equal(t, "asset://video-asset-1", media["video_url"].(map[string]interface{})["url"])
	}).ServeHTTP(videoRecorder, videoRequest)
	require.Equal(t, http.StatusNoContent, videoRecorder.Code, videoRecorder.Body.String())

	audioRecorder := httptest.NewRecorder()
	audioRequest := httptest.NewRequest(http.MethodPost, "/v1/video/generations", strings.NewReader(fmt.Sprintf(
		`{"character_id":%d,"model":"%s","prompt":"speak"}`, audio.ID, testVirtualCharacterVideoModel,
	)))
	audioRequest.Header.Set("Content-Type", "application/json")
	newRouter(func(t *testing.T, req relaycommon.TaskSubmitReq) {
		require.Empty(t, req.Images)
		content, ok := req.Metadata["content"].([]interface{})
		require.True(t, ok)
		media := content[1].(map[string]interface{})
		require.Equal(t, "audio_url", media["type"])
		require.Equal(t, "reference_audio", media["role"])
		require.Equal(t, "asset://audio-asset-1", media["audio_url"].(map[string]interface{})["url"])
	}).ServeHTTP(audioRecorder, audioRequest)
	require.Equal(t, http.StatusNoContent, audioRecorder.Code, audioRecorder.Body.String())
}

func TestVirtualCharacterAssetReferenceIDsCollectsAudioURL(t *testing.T) {
	ids, err := virtualCharacterAssetReferenceIDs(relaycommon.TaskSubmitReq{}, map[string]interface{}{
		"content": []interface{}{
			map[string]interface{}{
				"type":      "audio_url",
				"audio_url": map[string]interface{}{"url": "asset://audio-asset-1"},
			},
		},
	})
	require.NoError(t, err)
	require.Equal(t, []string{"audio-asset-1"}, ids)
}
