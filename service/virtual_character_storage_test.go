package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestAIPDDVirtualCharacterStorageLifecycle(t *testing.T) {
	t.Helper()
	var mu sync.Mutex
	calls := make([]string, 0, 5)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "deployment-key", r.Header.Get("X-API-Key"))
		require.Equal(t, "Bearer deployment-key", r.Header.Get("Authorization"))
		mu.Lock()
		calls = append(calls, r.Method+" "+r.URL.Path)
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/oss/upload":
			require.Equal(t, "true", r.URL.Query().Get("is_private"))
			require.Equal(t, "new-api/virtual-characters", r.URL.Query().Get("prefix"))
			require.NoError(t, r.ParseMultipartForm(1<<20))
			file, _, err := r.FormFile("file")
			require.NoError(t, err)
			payload, err := io.ReadAll(file)
			require.NoError(t, err)
			require.Equal(t, "fictional-image", string(payload))
			_, _ = io.WriteString(w, `{"code":0,"message":"ok","data":{"file_id":"file-ref","url":"https://signed.example/upload","filename":"role.png","size":17,"mime_type":"image/png","is_private":true}}`)
		case r.Method == http.MethodPost && r.URL.Path == "/digital_asset":
			var body map[string]any
			require.NoError(t, common.DecodeJson(r.Body, &body))
			require.Equal(t, "file-ref", body["url"])
			require.Equal(t, "image", body["type"])
			require.Equal(t, false, body["isOpen"])
			require.Equal(t, true, body["enabled"])
			_, _ = io.WriteString(w, `{"code":0,"message":"ok","data":{"id":88,"name":"role","type":"image","url":"file-ref","isOpen":false,"enabled":true}}`)
		case r.Method == http.MethodGet && r.URL.Path == "/oss/file/file-ref/sign":
			require.Equal(t, "259200", r.URL.Query().Get("valid_time"))
			_, _ = io.WriteString(w, `{"code":0,"message":"ok","data":{"file_id":"file-ref","url":"https://signed.example/source","expires_at":"later"}}`)
		case r.Method == http.MethodDelete && r.URL.Path == "/digital_asset/88":
			_, _ = io.WriteString(w, `{"code":0,"message":"ok","data":null}`)
		case r.Method == http.MethodDelete && r.URL.Path == "/oss/file/file-ref":
			_, _ = io.WriteString(w, `{"code":0,"message":"ok","data":{"deleted":true}}`)
		default:
			http.Error(w, fmt.Sprintf("unexpected request: %s %s", r.Method, r.URL.String()), http.StatusNotFound)
		}
	}))
	defer server.Close()

	t.Setenv(virtualCharacterAIPDDKeyEnv, "deployment-key")
	// Deployment tooling and channel forms may both provide an OpenAI-style
	// trailing /v1 even though AIPDD storage endpoints live at the API root.
	t.Setenv(virtualCharacterAIPDDBaseURLEnv, server.URL+"/v1/")
	storage, err := NewAIPDDVirtualCharacterStorage()
	require.NoError(t, err)

	uploaded, err := storage.UploadPrivateFile(context.Background(), "role.png", strings.NewReader("fictional-image"))
	require.NoError(t, err)
	require.Equal(t, "file-ref", uploaded.FileID)
	require.True(t, uploaded.IsPrivate)

	asset, err := storage.CreateDigitalAsset(context.Background(), "role", "Image", uploaded.FileID, uploaded.Size)
	require.NoError(t, err)
	require.EqualValues(t, 88, asset.ID)

	signed, err := storage.SignFile(context.Background(), uploaded.FileID)
	require.NoError(t, err)
	require.Equal(t, "https://signed.example/source", signed.URL)
	require.NoError(t, storage.DeleteDigitalAsset(context.Background(), asset.ID))
	require.NoError(t, storage.DeleteFile(context.Background(), uploaded.FileID))

	require.Equal(t, []string{
		"POST /oss/upload",
		"POST /digital_asset",
		"GET /oss/file/file-ref/sign",
		"DELETE /digital_asset/88",
		"DELETE /oss/file/file-ref",
	}, calls)
}

func TestAIPDDAssetStorageDirectUploadLifecycle(t *testing.T) {
	var mu sync.Mutex
	calls := make([]string, 0, 4)
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls = append(calls, r.Method+" "+r.URL.Path)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/oss/direct-upload/init":
			require.Equal(t, "direct-key", r.Header.Get("X-API-Key"))
			var body struct {
				FileName  string `json:"file_name"`
				Size      int64  `json:"size"`
				MimeType  string `json:"mime_type"`
				Prefix    string `json:"prefix"`
				IsPrivate bool   `json:"is_private"`
			}
			require.NoError(t, common.DecodeJson(r.Body, &body))
			require.Equal(t, "large.mp4", body.FileName)
			require.EqualValues(t, 6, body.Size)
			require.Equal(t, "video/mp4", body.MimeType)
			require.Equal(t, "new-api/playground", body.Prefix)
			require.True(t, body.IsPrivate)
			_, _ = fmt.Fprintf(w, `{"code":0,"message":"ok","data":{"file_id":"direct-file","object_key":"new-api/playground/direct-file.mp4","upload_id":"upload-1","part_size":3,"parts":[{"part_number":1,"url":%q},{"part_number":2,"url":%q}]}}`, server.URL+"/part/1", server.URL+"/part/2")
		case r.Method == http.MethodPut && r.URL.Path == "/part/1":
			require.Empty(t, r.Header.Get("Authorization"))
			payload, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			require.Equal(t, "abc", string(payload))
			w.Header().Set("ETag", `"etag-1"`)
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPut && r.URL.Path == "/part/2":
			payload, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			require.Equal(t, "def", string(payload))
			w.Header().Set("ETag", `"etag-2"`)
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPost && r.URL.Path == "/oss/direct-upload/complete":
			require.Equal(t, "direct-key", r.Header.Get("X-API-Key"))
			var body struct {
				ObjectKey string                     `json:"object_key"`
				UploadID  string                     `json:"upload_id"`
				Parts     []aipddCompletedUploadPart `json:"parts"`
			}
			require.NoError(t, common.DecodeJson(r.Body, &body))
			require.Equal(t, "new-api/playground/direct-file.mp4", body.ObjectKey)
			require.Equal(t, "upload-1", body.UploadID)
			require.Equal(t, []aipddCompletedUploadPart{
				{PartNumber: 1, ETag: `"etag-1"`},
				{PartNumber: 2, ETag: `"etag-2"`},
			}, body.Parts)
			_, _ = io.WriteString(w, `{"code":0,"message":"ok","data":{"file_id":"direct-file","filename":"large.mp4","size":6,"mime_type":"video/mp4","is_private":true,"object_key":"new-api/playground/direct-file.mp4"}}`)
		default:
			http.Error(w, "unexpected request", http.StatusNotFound)
		}
	}))
	defer server.Close()

	storage, err := newAIPDDVirtualCharacterStorage(server.URL, "direct-key", 0)
	require.NoError(t, err)
	stored, err := storage.uploadPrivateFileDirect(
		context.Background(),
		"large.mp4",
		"new-api/playground",
		"video/mp4",
		6,
		strings.NewReader("abcdef"),
	)
	require.NoError(t, err)
	require.Equal(t, "direct-file", stored.FileID)
	require.True(t, stored.IsPrivate)
	require.Equal(t, []string{
		"POST /oss/direct-upload/init",
		"PUT /part/1",
		"PUT /part/2",
		"POST /oss/direct-upload/complete",
	}, calls)
}

func TestAIPDDAssetStorageDirectUploadAbortsAfterPartFailure(t *testing.T) {
	var mu sync.Mutex
	calls := make([]string, 0, 3)
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls = append(calls, r.Method+" "+r.URL.Path)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/oss/direct-upload/init":
			_, _ = fmt.Fprintf(w, `{"code":0,"message":"ok","data":{"file_id":"abort-file","object_key":"new-api/playground/abort-file.png","upload_id":"upload-abort","part_size":3,"parts":[{"part_number":1,"url":%q}]}}`, server.URL+"/part/1")
		case r.Method == http.MethodPut && r.URL.Path == "/part/1":
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPost && r.URL.Path == "/oss/direct-upload/abort":
			var body map[string]any
			require.NoError(t, common.DecodeJson(r.Body, &body))
			require.Equal(t, "new-api/playground/abort-file.png", body["object_key"])
			require.Equal(t, "upload-abort", body["upload_id"])
			_, _ = io.WriteString(w, `{"code":0,"message":"ok","data":{"deleted":true}}`)
		default:
			http.Error(w, "unexpected request", http.StatusNotFound)
		}
	}))
	defer server.Close()

	storage, err := newAIPDDVirtualCharacterStorage(server.URL, "direct-key", 0)
	require.NoError(t, err)
	_, err = storage.uploadPrivateFileDirect(
		context.Background(),
		"abort.png",
		"new-api/playground",
		"image/png",
		3,
		strings.NewReader("abc"),
	)
	require.ErrorContains(t, err, "empty ETag")
	require.Equal(t, []string{
		"POST /oss/direct-upload/init",
		"PUT /part/1",
		"POST /oss/direct-upload/abort",
	}, calls)
}

func TestAIPDDAssetStorageRejectsInvalidDirectUploadURL(t *testing.T) {
	var mu sync.Mutex
	calls := make([]string, 0, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls = append(calls, r.Method+" "+r.URL.Path)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/oss/direct-upload/init":
			_, _ = io.WriteString(w, `{"code":0,"message":"ok","data":{"file_id":"bad-file","object_key":"new-api/playground/bad-file.png","upload_id":"upload-bad","part_size":3,"parts":[{"part_number":1,"url":"file:///tmp/not-oss"}]}}`)
		case r.Method == http.MethodPost && r.URL.Path == "/oss/direct-upload/abort":
			_, _ = io.WriteString(w, `{"code":0,"message":"ok","data":{"deleted":true}}`)
		default:
			http.Error(w, "unexpected request", http.StatusNotFound)
		}
	}))
	defer server.Close()

	storage, err := newAIPDDVirtualCharacterStorage(server.URL, "direct-key", 0)
	require.NoError(t, err)
	_, err = storage.uploadPrivateFileDirect(
		context.Background(),
		"bad.png",
		"new-api/playground",
		"image/png",
		3,
		strings.NewReader("abc"),
	)
	require.ErrorContains(t, err, "invalid URL")
	require.Equal(t, []string{
		"POST /oss/direct-upload/init",
		"POST /oss/direct-upload/abort",
	}, calls)
}

func TestAIPDDStoragePropagatesApplicationErrorWithoutResponseTarget(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodDelete, r.Method)
		require.Equal(t, "/digital_asset/88", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"code":5001,"message":"asset cleanup failed","data":null}`)
	}))
	defer server.Close()

	storage, err := newAIPDDVirtualCharacterStorage(server.URL, "direct-key", 0)
	require.NoError(t, err)
	require.ErrorContains(
		t,
		storage.DeleteDigitalAsset(context.Background(), 88),
		"asset cleanup failed",
	)
}

func TestAIPDDVirtualCharacterStorageFallsBackToEnabledChannel(t *testing.T) {
	previousDB := model.DB
	dsn := fmt.Sprintf("file:aipdd-character-storage-%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	t.Cleanup(func() {
		if sqlDB, dbErr := db.DB(); dbErr == nil {
			_ = sqlDB.Close()
		}
		model.DB = previousDB
	})
	require.NoError(t, db.AutoMigrate(&model.Channel{}))

	t.Setenv(virtualCharacterAIPDDKeyEnv, "")
	t.Setenv(virtualCharacterAIPDDBaseURLEnv, "https://ignored.example")
	baseURL := "https://channel-aipdd.example/v1"
	priority := int64(100)
	require.NoError(t, db.Create(&model.Channel{
		Type:     constant.ChannelTypeAIPDD,
		Key:      "disabled-key\nchannel-key",
		Status:   common.ChannelStatusEnabled,
		Name:     "AIPDD storage",
		BaseURL:  &baseURL,
		Priority: &priority,
		ChannelInfo: model.ChannelInfo{
			IsMultiKey:         true,
			MultiKeyStatusList: map[int]int{0: common.ChannelStatusAutoDisabled},
		},
	}).Error)

	storage, err := NewAIPDDVirtualCharacterStorage()
	require.NoError(t, err)
	require.Equal(t, "channel-key", storage.apiKey)
	require.Equal(t, "https://channel-aipdd.example", storage.baseURL)
	require.Greater(t, storage.ChannelID(), 0)

	pinned, err := NewAIPDDVirtualCharacterStorageForChannel(storage.ChannelID())
	require.NoError(t, err)
	require.Equal(t, storage.ChannelID(), pinned.ChannelID())
	require.Equal(t, "channel-key", pinned.apiKey)
}
