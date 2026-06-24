package controller

import (
	"bytes"
	"context"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
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

func TestRequestGeneratedMaterialRemoteMetadataIgnoresPartialContentLengthWithoutTotal(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "bytes=0-0", r.Header.Get("Range"))
		w.Header().Set("Content-Type", "video/mp4")
		w.Header().Set("Content-Length", "1")
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

	require.Zero(t, metadata.FileSize)
	require.Equal(t, "video/mp4", metadata.MimeType)
}

func TestGeneratedMaterialDataURLMetadata(t *testing.T) {
	metadata := generatedMaterialDataURLMetadata("data:video/mp4;base64,AAAA")

	require.EqualValues(t, 3, metadata.FileSize)
	require.Equal(t, "video/mp4", metadata.MimeType)
}

func TestSaveMaterialReaderWritesLocalFileAndStaticURL(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tempDir := t.TempDir()
	t.Setenv(materialStoragePathEnv, tempDir)
	t.Setenv(materialPublicBaseURLEnv, "https://cdn.example.com")

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/pg/material/upload", nil)

	storedFile, err := saveMaterialReader(ctx, bytes.NewReader([]byte("image-bytes")), 11, "image/png", model.MaterialTypeImage)

	require.NoError(t, err)
	require.EqualValues(t, 11, storedFile.FileSize)
	require.Equal(t, "image/png", storedFile.MimeType)
	require.True(t, strings.HasPrefix(storedFile.URL, "https://cdn.example.com/static/materials/"))
	require.True(t, strings.HasSuffix(storedFile.URL, ".png"))
	require.FileExists(t, storedFile.FilePath)

	data, err := os.ReadFile(storedFile.FilePath)
	require.NoError(t, err)
	require.Equal(t, []byte("image-bytes"), data)
}

func TestUploadMaterialHandlerStoresLocalJPEGWithDevURL(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupMaterialControllerTestDB(t)
	tempDir := t.TempDir()
	t.Setenv(materialStoragePathEnv, tempDir)
	t.Setenv(materialPublicBaseURLEnv, "")
	previousServerAddress := system_setting.ServerAddress
	system_setting.ServerAddress = "https://newapi.jumcp.com"
	t.Cleanup(func() {
		system_setting.ServerAddress = previousServerAddress
	})

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	fileWriter, err := writer.CreateFormFile("file", "sample.jpg")
	require.NoError(t, err)
	_, err = fileWriter.Write(testJPEGBytes())
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set("id", 42)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/pg/material/upload", body)
	ctx.Request.Host = "127.0.0.1:3000"
	ctx.Request.Header.Set("Content-Type", writer.FormDataContentType())

	UploadMaterial(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Success bool           `json:"success"`
		Data    model.Material `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.Success)
	require.Equal(t, model.StorageTypeLocal, response.Data.StorageType)
	require.Equal(t, model.MaterialTypeImage, response.Data.Type)
	require.Equal(t, "image/jpeg", response.Data.MimeType)
	require.Equal(t, "sample.jpg", response.Data.FileName)
	require.True(t, strings.HasPrefix(response.Data.Url, "http://127.0.0.1:3000/static/materials/"), response.Data.Url)
	require.True(t, strings.HasSuffix(response.Data.Url, ".jpg"), response.Data.Url)
	require.FileExists(t, response.Data.FilePath)

	var saved model.Material
	require.NoError(t, model.DB.First(&saved, response.Data.Id).Error)
	require.Equal(t, response.Data.Url, saved.Url)
	require.Equal(t, model.StorageTypeLocal, saved.StorageType)
}

func TestBuildMaterialStaticURLFallbacks(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previousServerAddress := system_setting.ServerAddress
	defer func() {
		system_setting.ServerAddress = previousServerAddress
	}()

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/x", nil)
	ctx.Request.Host = "origin.example.com"
	ctx.Request.Header.Set("X-Forwarded-Proto", "https")

	t.Setenv(materialPublicBaseURLEnv, "https://materials.example.com/base")
	system_setting.ServerAddress = "https://server.example.com"
	require.Equal(t, "https://materials.example.com/base/static/materials/file.png", buildMaterialStaticURL(ctx, "file.png"))

	t.Setenv(materialPublicBaseURLEnv, "")
	require.Equal(t, "https://server.example.com/static/materials/file.png", buildMaterialStaticURL(ctx, "file.png"))

	system_setting.ServerAddress = ""
	require.Equal(t, "https://origin.example.com/static/materials/file.png", buildMaterialStaticURL(ctx, "file.png"))

	system_setting.ServerAddress = "https://newapi.jumcp.com"
	require.Equal(t, "https://origin.example.com/static/materials/file.png", buildMaterialStaticURL(ctx, "file.png"))

	ctx.Request.Host = "127.0.0.1:3000"
	ctx.Request.Header.Del("X-Forwarded-Proto")
	system_setting.ServerAddress = "https://newapi.jumcp.com"
	require.Equal(t, "http://127.0.0.1:3000/static/materials/file.png", buildMaterialStaticURL(ctx, "file.png"))
}

func TestDefaultMaterialExtensionPrefersStableJPEGExtension(t *testing.T) {
	require.Equal(t, ".jpg", defaultMaterialExtension(model.MaterialTypeImage, "image/jpeg"))
	require.Equal(t, ".png", defaultMaterialExtension(model.MaterialTypeImage, "image/png"))
}

func TestDownloadMaterialURLToLocalRejectsNonOK(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tempDir := t.TempDir()
	t.Setenv(materialStoragePathEnv, tempDir)
	t.Setenv(materialPublicBaseURLEnv, "https://cdn.example.com")
	withMaterialDownloadFetchSetting(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/pg/material/ai-output", nil)

	_, err := downloadMaterialURLToLocal(ctx, server.URL, nil, "", model.MaterialTypeImage, "image/png")

	require.Error(t, err)
	require.Contains(t, err.Error(), "status 404")
}

func TestDownloadMaterialURLToLocalRejectsMimeMismatch(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tempDir := t.TempDir()
	t.Setenv(materialStoragePathEnv, tempDir)
	t.Setenv(materialPublicBaseURLEnv, "https://cdn.example.com")
	withMaterialDownloadFetchSetting(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("not an image"))
	}))
	defer server.Close()

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/pg/material/ai-output", nil)

	_, err := downloadMaterialURLToLocal(ctx, server.URL, nil, "", model.MaterialTypeImage, "image/png")

	require.Error(t, err)
	require.Contains(t, err.Error(), "unsupported material content type")
}

func TestRemoveMaterialLocalFileOnlyDeletesLocalStorage(t *testing.T) {
	tempDir := t.TempDir()
	localFile, err := os.CreateTemp(tempDir, "material-*.png")
	require.NoError(t, err)
	require.NoError(t, localFile.Close())

	ossFile, err := os.CreateTemp(tempDir, "oss-*.png")
	require.NoError(t, err)
	require.NoError(t, ossFile.Close())

	removeMaterialLocalFile(&model.Material{
		Id:          1,
		StorageType: model.StorageTypeLocal,
		FilePath:    localFile.Name(),
	})
	removeMaterialLocalFile(&model.Material{
		Id:          2,
		StorageType: model.StorageTypeOSS,
		FilePath:    ossFile.Name(),
	})

	require.NoFileExists(t, localFile.Name())
	require.FileExists(t, ossFile.Name())
}

func withMaterialDownloadFetchSetting(t *testing.T) {
	t.Helper()
	fetchSetting := system_setting.GetFetchSetting()
	previous := *fetchSetting
	*fetchSetting = system_setting.FetchSetting{
		EnableSSRFProtection: false,
	}
	t.Cleanup(func() {
		*fetchSetting = previous
	})
}

func setupMaterialControllerTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	previousDB := model.DB
	previousLogDB := model.LOG_DB
	previousUsingSQLite := common.UsingSQLite
	previousUsingMySQL := common.UsingMySQL
	previousUsingPostgreSQL := common.UsingPostgreSQL
	previousRedisEnabled := common.RedisEnabled

	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.RedisEnabled = false

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Material{}))
	model.DB = db
	model.LOG_DB = db

	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
		model.DB = previousDB
		model.LOG_DB = previousLogDB
		common.UsingSQLite = previousUsingSQLite
		common.UsingMySQL = previousUsingMySQL
		common.UsingPostgreSQL = previousUsingPostgreSQL
		common.RedisEnabled = previousRedisEnabled
	})

	return db
}

func testJPEGBytes() []byte {
	return []byte{
		0xff, 0xd8, 0xff, 0xe0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0x00, 0x01,
		0x01, 0x01, 0x00, 0x48, 0x00, 0x48, 0x00, 0x00, 0xff, 0xdb, 0x00,
		0x43, 0x00, 0xff, 0xd9,
	}
}
