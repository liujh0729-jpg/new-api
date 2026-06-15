package controller

import (
	"context"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	taskaipdd "github.com/QuantumNous/new-api/relay/channel/task/aipdd"

	"github.com/gin-gonic/gin"
)

const (
	materialUploadMaxBytes   int64 = constant.PlaygroundUploadMaxMB * 1024 * 1024
	materialNameMaxRunes           = 191
	materialFileNameMaxRunes       = 255
)

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
	materials, total, err := model.GetMaterialsByUser(userId, pageInfo.GetStartIdx(), pageInfo.GetPageSize())
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
	keyword := c.Query("keyword")
	typeFilter := c.Query("type")
	pageInfo := common.GetPageQuery(c)
	materials, total, err := model.SearchMaterialsByUser(userId, keyword, typeFilter, pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(materials)
	common.ApiSuccess(c, pageInfo)
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
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "this material is not stored locally",
		})
		return
	}

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
