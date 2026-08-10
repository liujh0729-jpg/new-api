package controller

import (
	"bytes"
	"context"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestParseVirtualCharacterCatalogValidatesVersionDuplicatesAndURLs(t *testing.T) {
	version, entries, err := parseVirtualCharacterCatalog("catalog.json", []byte(`{"version":"2026-08","items":[{"asset_id":"asset-a","name":"A","cover_url":"https://example.com/a.png","tags":["official","中国","男","20-40岁"]}]}`), "")
	require.NoError(t, err)
	require.Equal(t, "2026-08", version)
	require.Len(t, entries, 1)
	require.True(t, entries[0].Enabled)
	require.Equal(t, "中国", entries[0].Nationality)
	require.Equal(t, "男", entries[0].Gender)
	require.NotNil(t, entries[0].AgeMin)
	require.Equal(t, 20, *entries[0].AgeMin)

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

type stubVolcAssetClient struct {
	createGroupID  string
	createGroupErr error
	createAssetID  string
	createAssetErr error
}

func (s *stubVolcAssetClient) CreateVisualValidateSession(context.Context, string, string, string) (*service.VolcValidationSessionResult, error) {
	return nil, errors.New("not implemented")
}
func (s *stubVolcAssetClient) GetVisualValidateResult(context.Context, string, string) (string, error) {
	return "", errors.New("not implemented")
}
func (s *stubVolcAssetClient) CreateAssetGroup(context.Context, string, string, string) (string, error) {
	return s.createGroupID, s.createGroupErr
}
func (s *stubVolcAssetClient) UpdateAssetGroup(context.Context, string, string, string, string) error {
	return nil
}
func (s *stubVolcAssetClient) CreateAsset(context.Context, string, string, string, string, string) (string, error) {
	if s.createAssetErr != nil {
		return "", s.createAssetErr
	}
	if s.createAssetID == "" {
		return "asset-ok", nil
	}
	return s.createAssetID, nil
}
func (s *stubVolcAssetClient) UpdateAsset(context.Context, string, string, string) error {
	return nil
}
func (s *stubVolcAssetClient) GetAsset(context.Context, string, string) (*service.VolcAssetResult, error) {
	return nil, errors.New("not implemented")
}
func (s *stubVolcAssetClient) ListAssets(context.Context, string, string) ([]service.VolcAssetResult, error) {
	return nil, nil
}
func (s *stubVolcAssetClient) DeleteAsset(context.Context, string, string) error { return nil }
func (s *stubVolcAssetClient) DeleteAssetGroup(context.Context, string, string) error {
	return nil
}
func (s *stubVolcAssetClient) ListAssetGroups(context.Context, string) ([]service.VolcAssetGroupResult, error) {
	return nil, nil
}
func (s *stubVolcAssetClient) GetAssetGroup(context.Context, string, string) (*service.VolcAssetGroupResult, error) {
	return nil, errors.New("not implemented")
}

type stubVirtualCharacterStagingStorage struct{}

func (s *stubVirtualCharacterStagingStorage) UploadPrivateFile(context.Context, string, io.Reader) (*service.AIPDDStoredFile, error) {
	return &service.AIPDDStoredFile{FileID: "file-staging-1"}, nil
}
func (s *stubVirtualCharacterStagingStorage) SignFile(context.Context, string) (*service.AIPDDSignedURL, error) {
	return &service.AIPDDSignedURL{FileID: "file-staging-1", URL: "https://example.com/signed.png"}, nil
}
func (s *stubVirtualCharacterStagingStorage) DeleteFile(context.Context, string) error { return nil }

func newCreateVirtualCharacterMultipartRequest(t *testing.T, fields map[string]string, filename, contentType string, content []byte) *http.Request {
	t.Helper()
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	for key, value := range fields {
		require.NoError(t, writer.WriteField(key, value))
	}
	if filename != "" {
		part, err := writer.CreatePart(textproto.MIMEHeader{
			"Content-Disposition": []string{`form-data; name="file"; filename="` + filename + `"`},
			"Content-Type":        []string{contentType},
		})
		require.NoError(t, err)
		_, err = part.Write(content)
		require.NoError(t, err)
	}
	require.NoError(t, writer.Close())
	req := httptest.NewRequest(http.MethodPost, "/api/virtual-characters", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req
}

func TestCreateVirtualCharacterRequiresComplianceAndRollsBackOnGroupFailure(t *testing.T) {
	db := setupVirtualCharacterControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(
		&model.VirtualCharacter{},
		&model.VirtualCharacterAsset{},
		&model.VirtualCharacterProviderAccount{},
		&model.VirtualCharacterCleanupJob{},
	))

	previousSecret := common.CryptoSecret
	previousConfigured := common.CryptoSecretConfigured
	previousOptions := common.OptionMap
	previousFactory := newVolcAssetClientForVirtualCharacters
	previousStaging := newVirtualCharacterStagingStorage
	common.CryptoSecret = strings.Repeat("s", 32)
	common.CryptoSecretConfigured = true
	common.OptionMap = map[string]string{"VirtualCharacterLimit": "2"}
	newVirtualCharacterStagingStorage = func() (virtualCharacterStagingStorage, error) {
		return &stubVirtualCharacterStagingStorage{}, nil
	}
	t.Cleanup(func() {
		common.CryptoSecret = previousSecret
		common.CryptoSecretConfigured = previousConfigured
		common.OptionMap = previousOptions
		newVolcAssetClientForVirtualCharacters = previousFactory
		newVirtualCharacterStagingStorage = previousStaging
	})

	account := &model.VirtualCharacterProviderAccount{
		ID: 1, Enabled: true, VirtualEnabled: true, Region: "cn-beijing", ProjectName: "default",
	}
	require.NoError(t, db.Create(account).Error)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/virtual-characters", func(c *gin.Context) {
		c.Set("id", 77)
		CreateVirtualCharacter(c)
	})

	missingFile := httptest.NewRecorder()
	router.ServeHTTP(missingFile, newCreateVirtualCharacterMultipartRequest(t, map[string]string{
		"name": "Actor",
	}, "", "", nil))
	require.Equal(t, http.StatusBadRequest, missingFile.Code)

	newVolcAssetClientForVirtualCharacters = func(*model.VirtualCharacterProviderAccount) (service.VolcAssetClient, error) {
		return &stubVolcAssetClient{createGroupErr: errors.New("upstream create group failed")}, nil
	}
	failRecorder := httptest.NewRecorder()
	router.ServeHTTP(failRecorder, newCreateVirtualCharacterMultipartRequest(t, map[string]string{
		"name": "Actor One", "description": "d", "tags": `["a"]`,
	}, "actor.png", "image/png", []byte("png-bytes")))
	require.Equal(t, http.StatusBadGateway, failRecorder.Code)

	var failed model.VirtualCharacter
	require.NoError(t, db.Where("user_id = ?", 77).First(&failed).Error)
	require.Equal(t, model.VirtualCharacterStatusFailed, failed.Status)
	require.Nil(t, failed.Slot)

	newVolcAssetClientForVirtualCharacters = func(*model.VirtualCharacterProviderAccount) (service.VolcAssetClient, error) {
		return &stubVolcAssetClient{createGroupID: "group-ok", createAssetErr: errors.New("upstream create asset failed")}, nil
	}
	assetFailRecorder := httptest.NewRecorder()
	router.ServeHTTP(assetFailRecorder, newCreateVirtualCharacterMultipartRequest(t, map[string]string{
		"name": "Actor Asset Fail",
	}, "actor.png", "image/png", []byte("png-bytes")))
	require.Equal(t, http.StatusBadGateway, assetFailRecorder.Code)
	var assetFailed model.VirtualCharacter
	require.NoError(t, db.Where("user_id = ? AND name = ?", 77, "Actor Asset Fail").First(&assetFailed).Error)
	require.Equal(t, model.VirtualCharacterStatusFailed, assetFailed.Status)
	require.Nil(t, assetFailed.Slot)
	var cleanupJobs int64
	require.NoError(t, db.Model(&model.VirtualCharacterCleanupJob{}).Where("target_type = ? AND target_id = ?", "volc_group", "group-ok").Count(&cleanupJobs).Error)
	require.Equal(t, int64(1), cleanupJobs)

	newVolcAssetClientForVirtualCharacters = func(*model.VirtualCharacterProviderAccount) (service.VolcAssetClient, error) {
		return &stubVolcAssetClient{createGroupID: "group-ok-2", createAssetID: "asset-primary-1"}, nil
	}
	okRecorder := httptest.NewRecorder()
	router.ServeHTTP(okRecorder, newCreateVirtualCharacterMultipartRequest(t, map[string]string{
		"name": "Actor Two",
	}, "actor.png", "image/png", []byte("png-bytes")))
	require.Equal(t, http.StatusCreated, okRecorder.Code)

	var active model.VirtualCharacter
	require.NoError(t, db.Where("user_id = ? AND name = ?", 77, "Actor Two").First(&active).Error)
	require.Equal(t, model.VirtualCharacterStatusActive, active.Status)
	require.Equal(t, "group-ok-2", active.ProviderGroupID)
	require.NotNil(t, active.Slot)
	require.NotNil(t, active.PrimaryAssetID)
	require.Contains(t, active.CoverURL, "/preview")
	var asset model.VirtualCharacterAsset
	require.NoError(t, db.Where("character_id = ?", active.ID).First(&asset).Error)
	require.Equal(t, "asset-primary-1", asset.ProviderAssetID)
	require.True(t, asset.IsPrimary)
	require.Equal(t, model.VirtualCharacterAssetTypeImage, asset.AssetType)
	require.Equal(t, active.CoverURL, asset.CoverURL)
}
