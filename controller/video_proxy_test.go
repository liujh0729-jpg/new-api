package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSetVideoProxyContentHeadersUsesTaskIDFilename(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)

	setVideoProxyContentHeaders(ctx, "task_gW5J0A27EDFe8tS4YaoeNaaCTmNjhoin", "https://oss.example.com/result/content", "application/octet-stream")

	require.Equal(t, "video/mp4", recorder.Header().Get("Content-Type"))
	require.Equal(t, `inline; filename=task_gW5J0A27EDFe8tS4YaoeNaaCTmNjhoin.mp4`, recorder.Header().Get("Content-Disposition"))
}

func TestSetVideoProxyContentHeadersInfersExtensionFromURL(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)

	setVideoProxyContentHeaders(ctx, "task_123", "https://oss.example.com/videos/output.webm?token=1", "")

	require.Equal(t, "video/webm", recorder.Header().Get("Content-Type"))
	require.Equal(t, `inline; filename=task_123.webm`, recorder.Header().Get("Content-Disposition"))
}

func TestWriteVideoDataURLAddsDownloadFilename(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)

	err := writeVideoDataURL(ctx, "task_data", "data:video/mp4;base64,AAAA")

	require.NoError(t, err)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "video/mp4", recorder.Header().Get("Content-Type"))
	require.Equal(t, `inline; filename=task_data.mp4`, recorder.Header().Get("Content-Disposition"))
}

func TestCopyVideoProxyRequestHeadersForwardsRange(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/v1/videos/task_123/content", nil)
	ctx.Request.Header.Set("Range", "bytes=0-1023")
	ctx.Request.Header.Set("If-Range", "etag-123")

	req, err := http.NewRequest(http.MethodGet, "https://example.com/video.mp4", nil)
	require.NoError(t, err)

	copyVideoProxyRequestHeaders(ctx, req)

	require.Equal(t, "bytes=0-1023", req.Header.Get("Range"))
	require.Equal(t, "etag-123", req.Header.Get("If-Range"))
}
