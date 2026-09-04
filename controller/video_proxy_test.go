package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestAdminVideoProxyUsesExplicitTaskOwner(t *testing.T) {
	previousDB := model.DB
	db, err := gorm.Open(sqlite.Open("file:admin-video-proxy?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	t.Cleanup(func() {
		if sqlDB, sqlErr := db.DB(); sqlErr == nil {
			_ = sqlDB.Close()
		}
		model.DB = previousDB
	})
	require.NoError(t, db.AutoMigrate(&model.Task{}))
	require.NoError(t, db.Create(&model.Task{
		TaskID: "task_owned_by_101",
		UserId: 101,
		Status: model.TaskStatusInProgress,
	}).Error)

	request := func(userID string) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Params = gin.Params{{Key: "task_id", Value: "task_owned_by_101"}}
		ctx.Request = httptest.NewRequest(http.MethodGet, "/api/task/task_owned_by_101/content?user_id="+userID, nil)
		AdminVideoProxy(ctx)
		return recorder
	}

	wrongOwner := request("202")
	require.Equal(t, http.StatusNotFound, wrongOwner.Code)
	require.Contains(t, wrongOwner.Body.String(), "Task not found")

	actualOwner := request("101")
	require.Equal(t, http.StatusBadRequest, actualOwner.Code)
	require.Contains(t, actualOwner.Body.String(), "Task is not completed yet")
}

func TestAdminVideoProxyRequiresValidTaskOwner(t *testing.T) {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "task_id", Value: "task_123"}}
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/task/task_123/content?user_id=invalid", nil)

	AdminVideoProxy(ctx)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Contains(t, recorder.Body.String(), "valid user_id is required")
	require.Equal(t, "private, max-age=86400", recorder.Header().Get("Cache-Control"))
}

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

func TestWriteVideoDataURLPreservesPrivateCacheControl(t *testing.T) {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Header("Cache-Control", "private, max-age=86400")

	err := writeVideoDataURL(ctx, "task_admin", "data:video/mp4;base64,AAAA")

	require.NoError(t, err)
	require.Equal(t, "private, max-age=86400", recorder.Header().Get("Cache-Control"))
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

func TestCopyVideoProxyResponseHeadersSanitizesPrivateWorkflowMetadata(t *testing.T) {
	dst := http.Header{}
	src := http.Header{
		"Content-Type":       []string{`video/mp4; provider="private-supplier"`},
		"Content-Range":      []string{"bytes 0-9/10"},
		"Etag":               []string{`"private-supplier-upscale-v1"`},
		"X-Provider":         []string{"private-supplier"},
		"X-Upstream-Request": []string{"secret-request-id"},
		"Set-Cookie":         []string{"supplier_session=secret"},
	}

	copyVideoProxyResponseHeaders(dst, src, true)

	require.Equal(t, "video/mp4", dst.Get("Content-Type"))
	require.Equal(t, "bytes 0-9/10", dst.Get("Content-Range"))
	require.Empty(t, dst.Get("Etag"))
	require.Empty(t, dst.Get("X-Provider"))
	require.Empty(t, dst.Get("X-Upstream-Request"))
	require.Empty(t, dst.Get("Set-Cookie"))
}

func TestSanitizeVideoContentTypeDropsPrivateParameters(t *testing.T) {
	require.Equal(t, "video/mp4", sanitizeVideoContentType(`video/mp4; provider="private-supplier"`))
	require.Empty(t, sanitizeVideoContentType("not a media type"))
}

func TestParseSameOriginVideoProxyTaskIDRelativeURL(t *testing.T) {
	taskID, ok := parseSameOriginVideoProxyTaskID("/v1/videos/task_123/content")

	require.True(t, ok)
	require.Equal(t, "task_123", taskID)
}

func TestParseSameOriginVideoProxyTaskIDConfiguredServerURL(t *testing.T) {
	previous := system_setting.ServerAddress
	system_setting.ServerAddress = "https://new-api.example.com"
	t.Cleanup(func() {
		system_setting.ServerAddress = previous
	})

	taskID, ok := parseSameOriginVideoProxyTaskID("https://new-api.example.com/v1/videos/task_abc/content")

	require.True(t, ok)
	require.Equal(t, "task_abc", taskID)
}

func TestParseSameOriginVideoProxyTaskIDRejectsExternalURL(t *testing.T) {
	previous := system_setting.ServerAddress
	system_setting.ServerAddress = "https://new-api.example.com"
	t.Cleanup(func() {
		system_setting.ServerAddress = previous
	})

	_, ok := parseSameOriginVideoProxyTaskID("https://api.openai.com/v1/videos/task_abc/content")

	require.False(t, ok)
}
