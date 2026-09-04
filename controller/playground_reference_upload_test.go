package controller

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestPlaygroundUploadReferenceMediaCreatesAIPDDDigitalAsset(t *testing.T) {
	gin.SetMode(gin.TestMode)
	imageBytes := testJPEGBytes()
	var mu sync.Mutex
	calls := make([]string, 0, 3)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "playground-key", r.Header.Get("X-API-Key"))
		require.Equal(t, "Bearer playground-key", r.Header.Get("Authorization"))
		mu.Lock()
		calls = append(calls, r.Method+" "+r.URL.Path)
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/oss/upload":
			require.Equal(t, "new-api/playground", r.URL.Query().Get("prefix"))
			require.Equal(t, "true", r.URL.Query().Get("is_private"))
			require.NoError(t, r.ParseMultipartForm(1<<20))
			file, header, err := r.FormFile("file")
			require.NoError(t, err)
			defer file.Close()
			payload, err := io.ReadAll(file)
			require.NoError(t, err)
			require.Equal(t, "reference.jpg", header.Filename)
			require.Equal(t, imageBytes, payload)
			_, _ = io.WriteString(w, fmt.Sprintf(`{"code":0,"message":"ok","data":{"file_id":"playground-file","filename":"reference.jpg","size":%d,"mime_type":"image/jpeg","is_private":true}}`, len(imageBytes)))
		case r.Method == http.MethodPost && r.URL.Path == "/digital_asset":
			var body struct {
				Name     string   `json:"name"`
				Type     string   `json:"type"`
				Labels   []string `json:"labels"`
				URL      string   `json:"url"`
				IsOpen   bool     `json:"isOpen"`
				Enabled  bool     `json:"enabled"`
				FileSize int64    `json:"fileSize"`
			}
			require.NoError(t, common.DecodeJson(r.Body, &body))
			require.Equal(t, "reference.jpg", body.Name)
			require.Equal(t, "image", body.Type)
			require.Equal(t, []string{"new-api-playground"}, body.Labels)
			require.Equal(t, "playground-file", body.URL)
			require.False(t, body.IsOpen)
			require.True(t, body.Enabled)
			require.EqualValues(t, len(imageBytes), body.FileSize)
			_, _ = io.WriteString(w, `{"code":0,"message":"ok","data":{"id":321}}`)
		case r.Method == http.MethodGet && r.URL.Path == "/oss/file/playground-file/sign":
			_, _ = io.WriteString(w, `{"code":0,"message":"ok","data":{"file_id":"playground-file","url":"https://signed.example/reference.jpg","expires_at":"later"}}`)
		default:
			http.Error(w, "unexpected request", http.StatusNotFound)
		}
	}))
	defer server.Close()

	t.Setenv("AIPDD_API_KEY", "playground-key")
	t.Setenv("AIPDD_BASE_URL", server.URL)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = newPlaygroundReferenceUploadRequest(t, "reference.jpg", imageBytes)

	PlaygroundUploadReferenceMedia(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Success bool `json:"success"`
		Data    struct {
			URL       string `json:"url"`
			Filename  string `json:"filename"`
			MediaType string `json:"media_type"`
			AssetID   int64  `json:"asset_id"`
			FileID    string `json:"file_id"`
			ChannelID int    `json:"channel_id"`
			ExpiresAt string `json:"expires_at"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.Success)
	require.Equal(t, "https://signed.example/reference.jpg", response.Data.URL)
	require.Equal(t, "reference.jpg", response.Data.Filename)
	require.Equal(t, "image/jpeg", response.Data.MediaType)
	require.EqualValues(t, 321, response.Data.AssetID)
	require.Equal(t, "playground-file", response.Data.FileID)
	require.Zero(t, response.Data.ChannelID)
	require.Equal(t, "later", response.Data.ExpiresAt)
	require.NotContains(t, recorder.Body.String(), `"material"`)
	require.Equal(t, []string{
		"POST /oss/upload",
		"POST /digital_asset",
		"GET /oss/file/playground-file/sign",
	}, calls)
}

func TestPlaygroundUploadReferenceMediaCleansAIPDDObjectsWhenSigningFails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var mu sync.Mutex
	calls := make([]string, 0, 5)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls = append(calls, r.Method+" "+r.URL.Path)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/oss/upload":
			_, _ = io.WriteString(w, `{"code":0,"message":"ok","data":{"file_id":"cleanup-file","size":1,"mime_type":"image/jpeg","is_private":true}}`)
		case r.Method == http.MethodPost && r.URL.Path == "/digital_asset":
			_, _ = io.WriteString(w, `{"code":0,"message":"ok","data":{"id":654}}`)
		case r.Method == http.MethodGet && r.URL.Path == "/oss/file/cleanup-file/sign":
			http.Error(w, `{"message":"sign failed"}`, http.StatusBadGateway)
		case r.Method == http.MethodDelete && r.URL.Path == "/digital_asset/654":
			_, _ = io.WriteString(w, `{"code":0,"message":"ok","data":null}`)
		case r.Method == http.MethodDelete && r.URL.Path == "/oss/file/cleanup-file":
			_, _ = io.WriteString(w, `{"code":0,"message":"ok","data":null}`)
		default:
			http.Error(w, "unexpected request", http.StatusNotFound)
		}
	}))
	defer server.Close()

	t.Setenv("AIPDD_API_KEY", "playground-key")
	t.Setenv("AIPDD_BASE_URL", server.URL)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = newPlaygroundReferenceUploadRequest(t, "cleanup.jpg", testJPEGBytes())

	PlaygroundUploadReferenceMedia(ctx)

	require.Equal(t, http.StatusBadGateway, recorder.Code)
	require.Equal(t, []string{
		"POST /oss/upload",
		"POST /digital_asset",
		"GET /oss/file/cleanup-file/sign",
		"DELETE /digital_asset/654",
		"DELETE /oss/file/cleanup-file",
	}, calls)
}

func newPlaygroundReferenceUploadRequest(t *testing.T, filename string, payload []byte) *http.Request {
	t.Helper()
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	fileWriter, err := writer.CreateFormFile("file", filename)
	require.NoError(t, err)
	_, err = fileWriter.Write(payload)
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	request := httptest.NewRequest(http.MethodPost, "/pg/reference-media/upload", body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	return request
}

func testJPEGBytes() []byte {
	return []byte{
		0xff, 0xd8, 0xff, 0xe0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0x00, 0x01,
		0x01, 0x01, 0x00, 0x48, 0x00, 0x48, 0x00, 0x00, 0xff, 0xdb, 0x00,
		0x43, 0x00, 0xff, 0xd9,
	}
}
