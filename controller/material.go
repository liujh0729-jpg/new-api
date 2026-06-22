package controller

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	taskaipdd "github.com/QuantumNous/new-api/relay/channel/task/aipdd"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/system_setting"

	"github.com/gin-gonic/gin"
)

const (
	materialUploadMaxBytes             int64 = constant.PlaygroundUploadMaxMB * 1024 * 1024
	materialNameMaxRunes                     = 191
	materialFileNameMaxRunes                 = 255
	materialURLMaxRunes                      = 1024
	materialMetadataProbeTimeout             = 5 * time.Second
	materialMetadataImageProbeMaxBytes       = 32 * 1024 * 1024
)

type generatedMaterialRemoteMetadata struct {
	FileSize int64
	MimeType string
}

func uploadMaterialFile(ctx context.Context, header *multipart.FileHeader) (url string, storageType string, err error) {
	channel, err := getPlaygroundAIPDDUploadChannel()
	if err != nil {
		return "", "", fmt.Errorf("aipdd channel unavailable: %v", err)
	}

	apiKey, _, apiErr := channel.GetNextEnabledKey()
	if apiErr != nil {
		return "", "", fmt.Errorf("aipdd channel key unavailable: %v", apiErr)
	}
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return "", "", fmt.Errorf("aipdd channel key is empty")
	}

	uploadedURL, err := taskaipdd.UploadFileToOSS(
		ctx,
		channel.GetBaseURL(),
		apiKey,
		channel.GetSetting().Proxy,
		header,
	)
	if err != nil {
		return "", "", fmt.Errorf("oss upload failed: %v", err)
	}

	return uploadedURL, model.StorageTypeOSS, nil
}

func UploadMaterial(c *gin.Context) {
	fileHeader, err := c.FormFile("file")
	if err != nil {
		if isMaterialUploadTooLargeError(err) {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{
				"success": false,
				"message": materialUploadTooLargeMessage(),
				"error": gin.H{
					"code":    "upload_file_too_large",
					"message": materialUploadTooLargeMessage(),
					"type":    "invalid_request_error",
				},
			})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "file is required",
			"error": gin.H{
				"code":    "invalid_upload",
				"message": "file is required",
				"type":    "invalid_request_error",
			},
		})
		return
	}

	if fileHeader.Size > materialUploadMaxBytes {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{
			"success": false,
			"message": materialUploadTooLargeMessage(),
			"error": gin.H{
				"code":    "upload_file_too_large",
				"message": materialUploadTooLargeMessage(),
				"type":    "invalid_request_error",
			},
		})
		return
	}

	fileHeader.Filename = sanitizeMaterialFileName(fileHeader.Filename)
	sourceType, err := normalizeMaterialSourceType(c.PostForm("source_type"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": err.Error(),
			"error": gin.H{
				"code":    "invalid_source_type",
				"message": err.Error(),
				"type":    "invalid_request_error",
			},
		})
		return
	}
	mimeType, materialType, err := detectMaterialFileType(fileHeader)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": err.Error(),
			"error": gin.H{
				"code":    "unsupported_media_type",
				"message": err.Error(),
				"type":    "invalid_request_error",
			},
		})
		return
	}

	uploadedURL, storageType, err := uploadMaterialFile(c.Request.Context(), fileHeader)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{
			"success": false,
			"message": err.Error(),
			"error": gin.H{
				"code":    "upload_failed",
				"message": err.Error(),
				"type":    "invalid_request_error",
			},
		})
		return
	}

	now := common.GetTimestamp()
	fileName := truncateRunes(fileHeader.Filename, materialFileNameMaxRunes)
	material := model.Material{
		UserId:      c.GetInt("id"),
		Name:        truncateRunes(fileName, materialNameMaxRunes),
		Type:        materialType,
		SourceType:  sourceType,
		MimeType:    mimeType,
		FileName:    fileName,
		Url:         uploadedURL,
		StorageType: storageType,
		FileSize:    fileHeader.Size,
		Status:      1,
		CreatedTime: now,
		UpdatedTime: now,
	}

	err = material.Insert()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "failed to save material record",
			"error": gin.H{
				"code":    "database_error",
				"message": err.Error(),
				"type":    "internal_error",
			},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    material,
	})
}

func GetMaterials(c *gin.Context) {
	userId := c.GetInt("id")
	pageInfo := common.GetPageQuery(c)
	filters, err := getMaterialSearchFilters(c, false)
	if err != nil {
		materialBadRequest(c, err)
		return
	}
	materials, total, err := model.SearchMaterialsByUser(userId, filters, pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(materials)
	common.ApiSuccess(c, pageInfo)
}

func SearchMaterials(c *gin.Context) {
	userId := c.GetInt("id")
	pageInfo := common.GetPageQuery(c)
	filters, err := getMaterialSearchFilters(c, true)
	if err != nil {
		materialBadRequest(c, err)
		return
	}
	materials, total, err := model.SearchMaterialsByUser(userId, filters, pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(materials)
	common.ApiSuccess(c, pageInfo)
}

func CreateGeneratedMaterial(c *gin.Context) {
	userId := c.GetInt("id")
	var req struct {
		Name     string   `json:"name"`
		Type     string   `json:"type"`
		MimeType string   `json:"mime_type"`
		FileName string   `json:"file_name"`
		Url      string   `json:"url"`
		FileSize int64    `json:"file_size"`
		Width    *int     `json:"width"`
		Height   *int     `json:"height"`
		Duration *float64 `json:"duration"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}

	materialURL, err := normalizeGeneratedMaterialURL(req.Url)
	if err != nil {
		materialBadRequest(c, err)
		return
	}
	materialType, mimeType, err := normalizeGeneratedMaterialMedia(req.Type, req.MimeType, materialURL)
	if err != nil {
		materialBadRequest(c, err)
		return
	}
	remoteMetadata := probeGeneratedMaterialRemoteMetadata(c.Request.Context(), materialURL, userId)
	if strings.TrimSpace(req.MimeType) == "" && remoteMetadata.MimeType != "" {
		if inferredType := materialTypeFromMime(remoteMetadata.MimeType); inferredType == materialType {
			mimeType = remoteMetadata.MimeType
		}
	}

	now := common.GetTimestamp()
	fileName := generatedMaterialFileName(req.FileName, materialURL, materialType, mimeType, now)
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = fileName
	}
	name = truncateRunes(name, materialNameMaxRunes)
	fileSize := req.FileSize
	if fileSize <= 0 && remoteMetadata.FileSize > 0 {
		fileSize = remoteMetadata.FileSize
	}
	if fileSize < 0 {
		fileSize = 0
	}

	material := &model.Material{
		UserId:      userId,
		Name:        name,
		Type:        materialType,
		SourceType:  model.MaterialSourceTypeAIOutput,
		MimeType:    mimeType,
		FileName:    fileName,
		Url:         materialURL,
		StorageType: model.StorageTypeOSS,
		FileSize:    fileSize,
		Width:       req.Width,
		Height:      req.Height,
		Duration:    req.Duration,
		Status:      1,
		CreatedTime: now,
		UpdatedTime: now,
	}

	savedMaterial, err := model.CreateGeneratedMaterial(material)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, savedMaterial)
}

func UpdateMaterial(c *gin.Context) {
	userId := c.GetInt("id")

	var req struct {
		Id   int    `json:"id"`
		Name string `json:"name"`
	}
	err := c.ShouldBindJSON(&req)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	material, err := model.GetMaterialByIdAndUser(req.Id, userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	name, err := normalizeMaterialName(req.Name)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	material.Name = name
	material.UpdatedTime = common.GetTimestamp()
	err = material.UpdateName()
	if err != nil {
		common.ApiError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    material,
	})
}

func DeleteMaterial(c *gin.Context) {
	userId := c.GetInt("id")
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}

	err = model.DeleteMaterialByIdAndUser(id, userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
}

func ServeMaterialFile(c *gin.Context) {
	userId := c.GetInt("id")
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "invalid material id",
		})
		return
	}

	material, err := model.GetMaterialByIdAndUser(id, userId)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"message": "material not found",
		})
		return
	}

	if material.StorageType != model.StorageTypeLocal {
		serveRemoteMaterialFile(c, material)
		return
	}

	serveLocalMaterialFile(c, material)
}

func serveLocalMaterialFile(c *gin.Context, material *model.Material) {
	if material.FilePath == "" {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"message": "file path not found",
		})
		return
	}

	if _, err := os.Stat(material.FilePath); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"message": "file not found on disk",
		})
		return
	}

	c.Header("Content-Type", material.MimeType)
	c.Header("Content-Disposition", mime.FormatMediaType("inline", map[string]string{
		"filename": sanitizeMaterialFileName(material.FileName),
	}))
	c.File(material.FilePath)
}

func serveRemoteMaterialFile(c *gin.Context, material *model.Material) {
	materialURL := strings.TrimSpace(material.Url)
	if materialURL == "" {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"message": "material url not found",
		})
		return
	}

	if taskID, ok := parseSameOriginVideoProxyTaskID(materialURL); ok {
		serveTaskVideoMaterialFile(c, material, taskID)
		return
	}

	if isSameOriginMaterialPath(materialURL) {
		c.Redirect(http.StatusTemporaryRedirect, materialURL)
		return
	}

	fetchSetting := system_setting.GetFetchSetting()
	if err := common.ValidateURLWithFetchSetting(materialURL, fetchSetting.EnableSSRFProtection, fetchSetting.AllowPrivateIp, fetchSetting.DomainFilterMode, fetchSetting.IpFilterMode, fetchSetting.DomainList, fetchSetting.IpList, fetchSetting.AllowedPorts, fetchSetting.ApplyIPFilterForDomain); err != nil {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"message": fmt.Sprintf("request blocked: %v", err),
		})
		return
	}

	client, err := service.GetHttpClientWithProxy("")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "failed to create proxy client",
		})
		return
	}
	if client == nil {
		client = http.DefaultClient
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 60*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, materialURL, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "failed to create proxy request",
		})
		return
	}
	copyMaterialProxyRequestHeaders(c, req)

	resp, err := client.Do(req)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{
			"success": false,
			"message": "failed to fetch material content",
		})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		c.JSON(http.StatusBadGateway, gin.H{
			"success": false,
			"message": fmt.Sprintf("upstream service returned status %d", resp.StatusCode),
		})
		return
	}

	copyMaterialProxyResponseHeaders(c, resp.Header)
	updateMaterialRemoteMetadata(material, generatedMaterialMetadataFromResponse(resp))
	setMaterialContentHeaders(c, material, resp.Header.Get("Content-Type"))
	c.Writer.Header().Set("Cache-Control", "private, max-age=86400")
	c.Writer.WriteHeader(resp.StatusCode)
	if _, err = io.Copy(c.Writer, resp.Body); err != nil {
		common.SysLog(fmt.Sprintf("failed to stream material content: %s", err.Error()))
	}
}

func serveTaskVideoMaterialFile(c *gin.Context, material *model.Material, taskID string) {
	metadata := probeGeneratedTaskVideoMetadata(c.Request.Context(), material.UserId, taskID)
	updateMaterialRemoteMetadata(material, metadata)
	if failure := serveTaskVideoContent(c, taskID, material.UserId); failure != nil {
		c.JSON(failure.status, gin.H{
			"success": false,
			"message": failure.message,
		})
	}
}

func generatedMaterialMetadataFromResponse(resp *http.Response) generatedMaterialRemoteMetadata {
	if resp == nil {
		return generatedMaterialRemoteMetadata{}
	}
	return generatedMaterialRemoteMetadata{
		FileSize: responseMetadataFileSize(resp),
		MimeType: normalizeMimeType(resp.Header.Get("Content-Type")),
	}
}

func updateMaterialRemoteMetadata(material *model.Material, metadata generatedMaterialRemoteMetadata) {
	if material == nil {
		return
	}
	fileSize := int64(0)
	if material.FileSize <= 0 && metadata.FileSize > 0 {
		fileSize = metadata.FileSize
	}
	mimeType := ""
	if shouldUpdateMaterialMimeType(material, metadata.MimeType) {
		mimeType = metadata.MimeType
	}
	if fileSize == 0 && mimeType == "" {
		return
	}
	if err := material.UpdateRemoteMetadata(fileSize, mimeType); err != nil {
		common.SysLog(fmt.Sprintf("failed to update material metadata: id=%d, error=%s", material.Id, err.Error()))
	}
}

func shouldUpdateMaterialMimeType(material *model.Material, mimeType string) bool {
	if material == nil {
		return false
	}
	mimeType = normalizeMimeType(mimeType)
	if mimeType == "" || isGenericMaterialMime(mimeType) {
		return false
	}
	if inferredType := materialTypeFromMime(mimeType); inferredType != "" && inferredType != material.Type {
		return false
	}
	existing := normalizeMimeType(material.MimeType)
	return existing == "" || isGenericMaterialMime(existing)
}

func probeGeneratedMaterialRemoteMetadata(ctx context.Context, materialURL string, userId int) generatedMaterialRemoteMetadata {
	materialURL = strings.TrimSpace(materialURL)
	if materialURL == "" {
		return generatedMaterialRemoteMetadata{}
	}
	if taskID, ok := parseSameOriginVideoProxyTaskID(materialURL); ok {
		return probeGeneratedTaskVideoMetadata(ctx, userId, taskID)
	}
	if isSameOriginMaterialPath(materialURL) {
		return generatedMaterialRemoteMetadata{}
	}

	parsedURL, err := url.Parse(materialURL)
	if err != nil || parsedURL == nil || parsedURL.Host == "" {
		return generatedMaterialRemoteMetadata{}
	}
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return generatedMaterialRemoteMetadata{}
	}

	fetchSetting := system_setting.GetFetchSetting()
	if err = common.ValidateURLWithFetchSetting(materialURL, fetchSetting.EnableSSRFProtection, fetchSetting.AllowPrivateIp, fetchSetting.DomainFilterMode, fetchSetting.IpFilterMode, fetchSetting.DomainList, fetchSetting.IpList, fetchSetting.AllowedPorts, fetchSetting.ApplyIPFilterForDomain); err != nil {
		return generatedMaterialRemoteMetadata{}
	}

	client, err := service.GetHttpClientWithProxy("")
	if err != nil || client == nil {
		client = http.DefaultClient
	}

	metadata := requestGeneratedMaterialRemoteMetadata(ctx, client, http.MethodHead, materialURL)
	if metadata.FileSize > 0 {
		return metadata
	}
	fallbackMetadata := requestGeneratedMaterialRemoteMetadata(ctx, client, http.MethodGet, materialURL)
	if fallbackMetadata.MimeType == "" {
		fallbackMetadata.MimeType = metadata.MimeType
	}
	return fallbackMetadata
}

func requestGeneratedMaterialRemoteMetadata(ctx context.Context, client *http.Client, method string, materialURL string) generatedMaterialRemoteMetadata {
	return requestGeneratedMaterialRemoteMetadataWithHeaders(ctx, client, method, materialURL, nil)
}

func requestGeneratedMaterialRemoteMetadataWithHeaders(ctx context.Context, client *http.Client, method string, materialURL string, headers http.Header) generatedMaterialRemoteMetadata {
	requestCtx, cancel := context.WithTimeout(ctx, materialMetadataProbeTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(requestCtx, method, materialURL, nil)
	if err != nil {
		return generatedMaterialRemoteMetadata{}
	}
	copyHeaderValues(req.Header, headers)
	if method == http.MethodGet {
		req.Header.Set("Range", "bytes=0-0")
	}

	resp, err := client.Do(req)
	if err != nil {
		return generatedMaterialRemoteMetadata{}
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusBadRequest {
		return generatedMaterialRemoteMetadata{}
	}
	mimeType := normalizeMimeType(resp.Header.Get("Content-Type"))
	fileSize := responseMetadataFileSize(resp)
	if method == http.MethodGet && fileSize == 0 {
		fileSize = readGeneratedMaterialProbeBodySize(resp.Body, resp.StatusCode, mimeType)
	}

	return generatedMaterialRemoteMetadata{
		FileSize: fileSize,
		MimeType: mimeType,
	}
}

func probeGeneratedTaskVideoMetadata(ctx context.Context, userId int, taskID string) generatedMaterialRemoteMetadata {
	task, exists, err := model.GetByTaskId(userId, taskID)
	if err != nil || !exists || task == nil || task.Status != model.TaskStatusSuccess {
		return generatedMaterialRemoteMetadata{}
	}
	target, failure := resolveTaskVideoContentTarget(ctx, task)
	if failure != nil || target == nil {
		return generatedMaterialRemoteMetadata{}
	}
	if strings.HasPrefix(target.URL, "data:") {
		return generatedMaterialDataURLMetadata(target.URL)
	}

	client, err := service.GetHttpClientWithProxy(target.Proxy)
	if err != nil || client == nil {
		client = http.DefaultClient
	}
	metadata := requestGeneratedMaterialRemoteMetadataWithHeaders(ctx, client, http.MethodHead, target.URL, target.Headers)
	if metadata.FileSize > 0 {
		return metadata
	}
	fallbackMetadata := requestGeneratedMaterialRemoteMetadataWithHeaders(ctx, client, http.MethodGet, target.URL, target.Headers)
	if fallbackMetadata.MimeType == "" {
		fallbackMetadata.MimeType = metadata.MimeType
	}
	return fallbackMetadata
}

func generatedMaterialDataURLMetadata(dataURL string) generatedMaterialRemoteMetadata {
	header, payload, ok := strings.Cut(strings.TrimSpace(dataURL), ",")
	if !ok || !strings.HasPrefix(header, "data:") {
		return generatedMaterialRemoteMetadata{}
	}
	mimeType := strings.TrimPrefix(header, "data:")
	mimeType = strings.TrimSuffix(mimeType, ";base64")
	metadata := generatedMaterialRemoteMetadata{MimeType: normalizeMimeType(mimeType)}
	if strings.Contains(header, ";base64") {
		metadata.FileSize = decodedBase64Length(payload)
	}
	return metadata
}

func decodedBase64Length(payload string) int64 {
	payload = strings.TrimSpace(payload)
	if payload == "" {
		return 0
	}
	padding := 0
	for i := len(payload) - 1; i >= 0 && payload[i] == '='; i-- {
		padding++
	}
	size := 0
	if padding > 0 {
		size = base64.StdEncoding.DecodedLen(len(payload)) - padding
	} else {
		size = (len(payload) * 6) / 8
	}
	if size < 0 {
		return 0
	}
	return int64(size)
}

func responseMetadataFileSize(resp *http.Response) int64 {
	if resp == nil {
		return 0
	}
	if total := parseContentRangeTotal(resp.Header.Get("Content-Range")); total > 0 {
		return total
	}
	if resp.StatusCode == http.StatusPartialContent {
		return 0
	}
	if resp.ContentLength > 0 {
		return resp.ContentLength
	}
	return 0
}

func readGeneratedMaterialProbeBodySize(body io.Reader, statusCode int, mimeType string) int64 {
	if body == nil {
		return 0
	}
	limit := int64(1024)
	if statusCode == http.StatusOK && strings.HasPrefix(normalizeMimeType(mimeType), "image/") {
		limit = materialMetadataImageProbeMaxBytes + 1
	}
	readBytes, _ := io.Copy(io.Discard, io.LimitReader(body, limit))
	if statusCode == http.StatusOK &&
		strings.HasPrefix(normalizeMimeType(mimeType), "image/") &&
		readBytes > 0 &&
		readBytes <= materialMetadataImageProbeMaxBytes {
		return readBytes
	}
	return 0
}

func parseContentRangeTotal(value string) int64 {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	index := strings.LastIndex(value, "/")
	if index < 0 || index+1 >= len(value) {
		return 0
	}
	total := strings.TrimSpace(value[index+1:])
	if total == "" || total == "*" {
		return 0
	}
	parsed, err := strconv.ParseInt(total, 10, 64)
	if err != nil || parsed < 0 {
		return 0
	}
	return parsed
}

func copyMaterialProxyRequestHeaders(c *gin.Context, req *http.Request) {
	for _, header := range []string{"Range", "If-Range"} {
		if value := c.GetHeader(header); value != "" {
			req.Header.Set(header, value)
		}
	}
}

func copyMaterialProxyResponseHeaders(c *gin.Context, header http.Header) {
	for key, values := range header {
		switch strings.ToLower(key) {
		case "content-disposition", "content-type", "transfer-encoding", "connection", "keep-alive", "proxy-authenticate", "proxy-authorization", "te", "trailer", "upgrade":
			continue
		default:
			for _, value := range values {
				c.Writer.Header().Add(key, value)
			}
		}
	}
}

func setMaterialContentHeaders(c *gin.Context, material *model.Material, upstreamContentType string) {
	contentType := normalizeMaterialContentType(material, upstreamContentType)
	c.Writer.Header().Set("Content-Type", contentType)
	c.Writer.Header().Set("Content-Disposition", mime.FormatMediaType("inline", map[string]string{
		"filename": materialProxyFilename(material, contentType),
	}))
}

func normalizeMaterialContentType(material *model.Material, upstreamContentType string) string {
	materialMime := normalizeMimeType(material.MimeType)
	if materialMime != "" && !isGenericMaterialMime(materialMime) {
		return materialMime
	}
	upstreamContentType = strings.TrimSpace(upstreamContentType)
	upstreamMediaType := upstreamContentType
	if parsed, _, err := mime.ParseMediaType(upstreamContentType); err == nil {
		upstreamMediaType = parsed
	}
	if upstreamMediaType != "" && !isGenericMaterialMime(upstreamMediaType) {
		return upstreamContentType
	}
	if inferred := materialMimeFromURL(material.Url); inferred != "" {
		return inferred
	}
	return defaultMaterialMimeType(material.Type)
}

func materialProxyFilename(material *model.Material, contentType string) string {
	filename := sanitizeMaterialFileName(material.FileName)
	if filename == "upload.bin" {
		filename = sanitizeMaterialFileName(material.Name)
	}
	if path.Ext(filename) != "" {
		return filename
	}
	if ext := defaultMaterialExtension(material.Type, contentType); ext != "" {
		return filename + ext
	}
	return filename
}

func detectMaterialFileType(header *multipart.FileHeader) (mimeType string, materialType string, err error) {
	declaredMime := normalizeMimeType(header.Header.Get("Content-Type"))
	detectedMime, err := sniffUploadedFileMime(header)
	if err != nil {
		return "", "", err
	}

	if detectedType := materialTypeFromMime(detectedMime); detectedType != "" {
		return detectedMime, detectedType, nil
	}

	extensionMime := materialMimeFromExtension(header.Filename)
	extensionType := materialTypeFromMime(extensionMime)
	declaredType := materialTypeFromMime(declaredMime)
	if extensionType != "" && isGenericMaterialMime(detectedMime) {
		if declaredType == extensionType {
			return declaredMime, extensionType, nil
		}
		return extensionMime, extensionType, nil
	}

	mediaType := firstNonEmptyString(detectedMime, declaredMime, extensionMime)
	return "", "", fmt.Errorf("unsupported file type: %s", mediaType)
}

func sniffUploadedFileMime(header *multipart.FileHeader) (string, error) {
	file, err := header.Open()
	if err != nil {
		return "", err
	}
	defer file.Close()

	sniff := make([]byte, 512)
	n, readErr := io.ReadFull(file, sniff)
	if readErr != nil && readErr != io.EOF && readErr != io.ErrUnexpectedEOF {
		return "", readErr
	}
	if n == 0 {
		return "", fmt.Errorf("file is empty")
	}
	return normalizeMimeType(http.DetectContentType(sniff[:n])), nil
}

func materialTypeFromMime(mimeType string) string {
	mimeType = normalizeMimeType(mimeType)
	switch {
	case strings.HasPrefix(mimeType, "image/"):
		return model.MaterialTypeImage
	case strings.HasPrefix(mimeType, "video/"):
		return model.MaterialTypeVideo
	case strings.HasPrefix(mimeType, "audio/"):
		return model.MaterialTypeAudio
	default:
		return ""
	}
}

func normalizeMaterialSourceType(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return model.MaterialSourceTypeUpload, nil
	}
	switch value {
	case model.MaterialSourceTypeUpload, model.MaterialSourceTypeAIOutput:
		return value, nil
	default:
		return "", fmt.Errorf("unsupported material source type: %s", value)
	}
}

func normalizeGeneratedMaterialURL(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("material url is required")
	}
	if utf8.RuneCountInString(value) > materialURLMaxRunes {
		return "", fmt.Errorf("material url cannot exceed %d characters", materialURLMaxRunes)
	}
	if strings.HasPrefix(strings.ToLower(value), "data:") {
		return "", fmt.Errorf("data urls must be uploaded as files")
	}
	if strings.HasPrefix(value, "//") {
		return "", fmt.Errorf("material url must be http, https, or an absolute path")
	}
	if isSameOriginMaterialPath(value) {
		return value, nil
	}
	parsedURL, err := url.Parse(value)
	if err != nil || parsedURL == nil {
		return "", fmt.Errorf("invalid material url")
	}
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return "", fmt.Errorf("material url must be http, https, or an absolute path")
	}
	if parsedURL.Host == "" {
		return "", fmt.Errorf("invalid material url")
	}
	return value, nil
}

func normalizeGeneratedMaterialMedia(requestType string, requestMimeType string, materialURL string) (string, string, error) {
	mimeType := normalizeMimeType(requestMimeType)
	materialType := strings.ToLower(strings.TrimSpace(requestType))
	if materialType == "" && mimeType != "" {
		materialType = materialTypeFromMime(mimeType)
	}
	if mimeType == "" {
		mimeType = materialMimeFromURL(materialURL)
	}
	if materialType == "" && mimeType != "" {
		materialType = materialTypeFromMime(mimeType)
	}
	if !isSupportedMaterialType(materialType) {
		return "", "", fmt.Errorf("unsupported material type: %s", firstNonEmptyString(materialType, requestType, mimeType))
	}
	inferredType := materialTypeFromMime(mimeType)
	if inferredType != "" && inferredType != materialType {
		return "", "", fmt.Errorf("material type does not match mime type")
	}
	if mimeType == "" {
		mimeType = defaultMaterialMimeType(materialType)
	}
	return materialType, mimeType, nil
}

func isSupportedMaterialType(materialType string) bool {
	switch materialType {
	case model.MaterialTypeImage, model.MaterialTypeVideo, model.MaterialTypeAudio:
		return true
	default:
		return false
	}
}

func materialMimeFromURL(rawURL string) string {
	parsedURL, err := url.Parse(rawURL)
	if err == nil && parsedURL != nil {
		if mimeType := materialMimeFromExtension(parsedURL.Path); mimeType != "" {
			return mimeType
		}
	}
	return materialMimeFromExtension(rawURL)
}

func isSameOriginMaterialPath(value string) bool {
	return strings.HasPrefix(value, "/") && !strings.HasPrefix(value, "//")
}

func defaultMaterialMimeType(materialType string) string {
	switch materialType {
	case model.MaterialTypeImage:
		return "image/png"
	case model.MaterialTypeVideo:
		return "video/mp4"
	case model.MaterialTypeAudio:
		return "audio/mpeg"
	default:
		return "application/octet-stream"
	}
}

func defaultMaterialExtension(materialType string, mimeType string) string {
	if extensions, err := mime.ExtensionsByType(mimeType); err == nil && len(extensions) > 0 {
		return extensions[0]
	}
	switch materialType {
	case model.MaterialTypeImage:
		return ".png"
	case model.MaterialTypeVideo:
		return ".mp4"
	case model.MaterialTypeAudio:
		return ".mp3"
	default:
		return ".bin"
	}
}

func generatedMaterialFileName(requestFileName string, rawURL string, materialType string, mimeType string, timestamp int64) string {
	fileName := sanitizeMaterialFileName(requestFileName)
	if fileName == "upload.bin" {
		if parsedURL, err := url.Parse(rawURL); err == nil && parsedURL != nil {
			fileName = sanitizeMaterialFileName(path.Base(parsedURL.Path))
		}
	}
	if fileName == "upload.bin" || filepath.Ext(fileName) == "" {
		fileName = fmt.Sprintf("ai-output-%d%s", timestamp, defaultMaterialExtension(materialType, mimeType))
	}
	return truncateRunes(fileName, materialFileNameMaxRunes)
}

func materialMimeFromExtension(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	if ext == "" {
		return ""
	}
	if mimeType := normalizeMimeType(mime.TypeByExtension(ext)); mimeType != "" {
		return mimeType
	}
	switch ext {
	case ".heic", ".heif":
		return "image/heif"
	case ".m4v":
		return "video/mp4"
	case ".mkv":
		return "video/x-matroska"
	case ".3gp":
		return "video/3gpp"
	case ".m4a":
		return "audio/mp4"
	case ".oga":
		return "audio/ogg"
	default:
		return ""
	}
}

func normalizeMimeType(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return ""
	}
	if mediaType, _, err := mime.ParseMediaType(value); err == nil {
		return strings.ToLower(mediaType)
	}
	if idx := strings.Index(value, ";"); idx >= 0 {
		return strings.TrimSpace(value[:idx])
	}
	return value
}

func isGenericMaterialMime(mimeType string) bool {
	switch normalizeMimeType(mimeType) {
	case "", "application/octet-stream", "application/ogg", "application/mp4", "application/x-riff", "application/x-matroska":
		return true
	default:
		return false
	}
}

func sanitizeMaterialFileName(filename string) string {
	filename = strings.TrimSpace(strings.ReplaceAll(filename, "\\", "/"))
	filename = filepath.Base(filename)
	if filename == "." || filename == string(filepath.Separator) || filename == "" {
		return "upload.bin"
	}
	return filename
}

func normalizeMaterialName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("material name cannot be empty")
	}
	if utf8.RuneCountInString(name) > materialNameMaxRunes {
		return "", fmt.Errorf("material name cannot exceed %d characters", materialNameMaxRunes)
	}
	return name, nil
}

func getMaterialSearchFilters(c *gin.Context, includeKeyword bool) (model.MaterialSearchFilters, error) {
	createdAfter, err := parseMaterialTimestampQuery(c, "created_after")
	if err != nil {
		return model.MaterialSearchFilters{}, err
	}
	createdBefore, err := parseMaterialTimestampQuery(c, "created_before")
	if err != nil {
		return model.MaterialSearchFilters{}, err
	}
	if createdAfter > 0 && createdBefore > 0 && createdAfter > createdBefore {
		return model.MaterialSearchFilters{}, fmt.Errorf("created_after cannot be greater than created_before")
	}

	filters := model.MaterialSearchFilters{
		TypeFilter:       c.Query("type"),
		SourceTypeFilter: c.Query("source_type"),
		CreatedAfter:     createdAfter,
		CreatedBefore:    createdBefore,
	}
	if includeKeyword {
		filters.Keyword = c.Query("keyword")
	}
	return filters, nil
}

func parseMaterialTimestampQuery(c *gin.Context, key string) (int64, error) {
	value := strings.TrimSpace(c.Query(key))
	if value == "" {
		return 0, nil
	}
	timestamp, err := strconv.ParseInt(value, 10, 64)
	if err != nil || timestamp < 0 {
		return 0, fmt.Errorf("%s must be a valid timestamp", key)
	}
	return timestamp, nil
}

func materialBadRequest(c *gin.Context, err error) {
	c.JSON(http.StatusBadRequest, gin.H{
		"success": false,
		"message": err.Error(),
		"error": gin.H{
			"code":    "invalid_request",
			"message": err.Error(),
			"type":    "invalid_request_error",
		},
	})
}

func truncateRunes(value string, maxRunes int) string {
	if maxRunes <= 0 || utf8.RuneCountInString(value) <= maxRunes {
		return value
	}
	runes := []rune(value)
	return string(runes[:maxRunes])
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func materialUploadTooLargeMessage() string {
	return fmt.Sprintf("file size exceeds the %d MB limit", materialUploadMaxBytes/(1024*1024))
}

func isMaterialUploadTooLargeError(err error) bool {
	if common.IsRequestBodyTooLargeError(err) {
		return true
	}
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "request body too large")
}
