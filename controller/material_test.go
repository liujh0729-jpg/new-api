package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSetMaterialContentHeadersUsesInlineDisposition(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	material := &model.Material{
		Type:     model.MaterialTypeImage,
		MimeType: "image/png",
		FileName: "result",
		Url:      "https://oss.example.com/result",
	}

	setMaterialContentHeaders(ctx, material, "application/octet-stream")

	require.Equal(t, "image/png", recorder.Header().Get("Content-Type"))
	require.Equal(t, `inline; filename=result.png`, recorder.Header().Get("Content-Disposition"))
}

func TestCopyMaterialProxyRequestHeadersForwardsRange(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/pg/material/file/1", nil)
	ctx.Request.Header.Set("Range", "bytes=0-1023")
	ctx.Request.Header.Set("If-Range", "etag-123")

	req, err := http.NewRequest(http.MethodGet, "https://example.com/video.mp4", nil)
	require.NoError(t, err)

	copyMaterialProxyRequestHeaders(ctx, req)

	require.Equal(t, "bytes=0-1023", req.Header.Get("Range"))
	require.Equal(t, "etag-123", req.Header.Get("If-Range"))
}

func TestCopyMaterialProxyResponseHeadersSkipsDownloadHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	header := http.Header{}
	header.Set("Content-Disposition", "attachment; filename=download.png")
	header.Set("Content-Type", "application/octet-stream")
	header.Set("Content-Range", "bytes 0-1023/2048")

	copyMaterialProxyResponseHeaders(ctx, header)

	require.Empty(t, recorder.Header().Get("Content-Disposition"))
	require.Empty(t, recorder.Header().Get("Content-Type"))
	require.Equal(t, "bytes 0-1023/2048", recorder.Header().Get("Content-Range"))
}

func TestRequestGeneratedMaterialRemoteMetadataUsesHeadContentLength(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodHead, r.Method)
		w.Header().Set("Content-Type", "image/webp")
		w.Header().Set("Content-Length", "12345")
	}))
	defer server.Close()

	metadata := requestGeneratedMaterialRemoteMetadata(
		context.Background(),
		server.Client(),
		http.MethodHead,
		server.URL,
	)

	require.EqualValues(t, 12345, metadata.FileSize)
	require.Equal(t, "image/webp", metadata.MimeType)
}

func TestRequestGeneratedMaterialRemoteMetadataUsesContentRangeTotal(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "bytes=0-0", r.Header.Get("Range"))
		w.Header().Set("Content-Type", "image/jpeg")
		w.Header().Set("Content-Range", "bytes 0-0/98765")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write([]byte{1})
	}))
	defer server.Close()

	metadata := requestGeneratedMaterialRemoteMetadata(
		context.Background(),
		server.Client(),
		http.MethodGet,
		server.URL,
	)

	require.EqualValues(t, 98765, metadata.FileSize)
	require.Equal(t, "image/jpeg", metadata.MimeType)
}
