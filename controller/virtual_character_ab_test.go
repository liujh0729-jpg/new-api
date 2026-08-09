package controller

import (
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestParseVirtualCharacterCatalogValidatesVersionDuplicatesAndURLs(t *testing.T) {
	version, entries, err := parseVirtualCharacterCatalog("catalog.json", []byte(`{"version":"2026-08","items":[{"asset_id":"asset-a","name":"A","cover_url":"https://example.com/a.png","tags":["official"]}]}`), "")
	require.NoError(t, err)
	require.Equal(t, "2026-08", version)
	require.Len(t, entries, 1)
	require.True(t, entries[0].Enabled)

	_, _, err = parseVirtualCharacterCatalog("catalog.json", []byte(`{"version":"v1","items":[{"asset_id":"asset-a","name":"A","cover_url":"https://example.com/a.png"},{"asset_id":"asset-a","name":"B","cover_url":"https://example.com/b.png"}]}`), "")
	require.ErrorContains(t, err, "duplicate asset_id")

	_, _, err = parseVirtualCharacterCatalog("catalog.csv", []byte("asset_id,name,cover_url\nasset-a,A,not-a-url\n"), "v1")
	require.ErrorContains(t, err, "invalid cover_url")
}

func TestValidateVolcCharacterAssetUploadEnforcesTypeAndLimits(t *testing.T) {
	header := &multipart.FileHeader{Filename: "actor.mp4", Size: 50 << 20, Header: textproto.MIMEHeader{"Content-Type": []string{"video/mp4"}}}
	typeName, mimeType, err := validateVolcCharacterAssetUpload(header, "Video")
	require.NoError(t, err)
	require.Equal(t, model.VirtualCharacterAssetTypeVideo, typeName)
	require.Equal(t, "video/mp4", mimeType)

	header.Size++
	_, _, err = validateVolcCharacterAssetUpload(header, "Video")
	require.ErrorContains(t, err, "50 MB")
	header.Filename = "actor.exe"
	header.Size = 1
	_, _, err = validateVolcCharacterAssetUpload(header, "Video")
	require.ErrorContains(t, err, "extension")
}

func TestValidationCallbackRejectsTokenMismatchAndReplay(t *testing.T) {
	db := setupVirtualCharacterControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.VirtualCharacterValidationSession{}))
	previousSecret := common.CryptoSecret
	previousConfigured := common.CryptoSecretConfigured
	previousAddress := system_setting.ServerAddress
	common.CryptoSecret = strings.Repeat("s", 32)
	common.CryptoSecretConfigured = true
	system_setting.ServerAddress = "https://new-api.example.com"
	t.Cleanup(func() {
		common.CryptoSecret = previousSecret
		common.CryptoSecretConfigured = previousConfigured
		system_setting.ServerAddress = previousAddress
	})

	token, err := common.EncryptSensitiveValue("expected-token")
	require.NoError(t, err)
	link, err := common.EncryptSensitiveValue("https://example.com/h5")
	require.NoError(t, err)
	state := "callback-state"
	session := &model.VirtualCharacterValidationSession{ID: "session-security", UserID: 88, Status: model.VirtualCharacterValidationPending, StateHash: hashValidationState(state), EncryptedBytedToken: token, EncryptedH5Link: link, Name: "Actor", TagsJSON: "[]", ExpiresAt: time.Now().Add(time.Minute).Unix()}
	require.NoError(t, model.DB.Create(session).Error)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/api/virtual-characters/validation/callback", VirtualCharacterValidationCallback)
	request := httptest.NewRequest(http.MethodGet, "/api/virtual-characters/validation/callback?state="+state+"&bytedToken=wrong-token&resultCode=10000&secret=must-not-be-stored", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusFound, recorder.Code)

	var stored model.VirtualCharacterValidationSession
	require.NoError(t, model.DB.Where("id = ?", session.ID).First(&stored).Error)
	require.Equal(t, model.VirtualCharacterValidationFailed, stored.Status)
	require.Equal(t, "validation token mismatch", stored.LastError)
	require.NotContains(t, stored.LastError, "wrong-token")
	require.NotContains(t, stored.LastError, "must-not-be-stored")

	// Replaying a terminal callback keeps the fixed terminal state.
	replay := httptest.NewRecorder()
	router.ServeHTTP(replay, httptest.NewRequest(http.MethodGet, "/api/virtual-characters/validation/callback?state="+state+"&bytedToken=expected-token&resultCode=10000", nil))
	require.Equal(t, http.StatusFound, replay.Code)
	require.NoError(t, model.DB.Where("id = ?", session.ID).First(&stored).Error)
	require.Equal(t, model.VirtualCharacterValidationFailed, stored.Status)
}

func TestValidationSessionEndpointHidesOtherUsersSessions(t *testing.T) {
	db := setupVirtualCharacterControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.VirtualCharacterValidationSession{}))
	session := &model.VirtualCharacterValidationSession{ID: "session-owner", UserID: 91, Status: model.VirtualCharacterValidationPending, StateHash: "owner-state", ExpiresAt: time.Now().Add(time.Minute).Unix()}
	require.NoError(t, model.DB.Create(session).Error)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/sessions/:id", func(c *gin.Context) {
		c.Set("id", 92)
		GetVirtualCharacterValidationSession(c)
	})
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/sessions/"+session.ID, nil))
	require.Equal(t, http.StatusNotFound, recorder.Code)
}
