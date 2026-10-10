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

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
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
	t.Setenv(virtualCharacterAIPDDBaseURLEnv, server.URL)
	storage, err := NewAIPDDVirtualCharacterStorage()
	require.NoError(t, err)

	uploaded, err := storage.UploadPrivateFile(context.Background(), "role.png", strings.NewReader("fictional-image"))
	require.NoError(t, err)
	require.Equal(t, "file-ref", uploaded.FileID)
	require.True(t, uploaded.IsPrivate)

	asset, err := storage.CreateDigitalAsset(context.Background(), "role", uploaded.FileID, uploaded.Size)
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
