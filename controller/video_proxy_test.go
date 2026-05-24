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
