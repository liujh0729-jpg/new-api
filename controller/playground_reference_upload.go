package controller

import (
	"context"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

const playgroundReferenceUploadMaxBytes int64 = constant.PlaygroundUploadMaxMB * 1024 * 1024

func PlaygroundUploadReferenceMedia(c *gin.Context) {
	fileHeader, err := c.FormFile("file")
	if err != nil {
		if isPlaygroundReferenceUploadTooLargeError(err) {
			playgroundReferenceUploadError(c, http.StatusRequestEntityTooLarge, "upload_file_too_large", playgroundReferenceUploadTooLargeMessage())
			return
		}
		playgroundReferenceUploadError(c, http.StatusBadRequest, "invalid_upload", "file is required")
		return
	}
	if fileHeader.Size > playgroundReferenceUploadMaxBytes {
		playgroundReferenceUploadError(
			c,
			http.StatusRequestEntityTooLarge,
			"upload_file_too_large",
			playgroundReferenceUploadTooLargeMessage(),
		)
		return
	}
	fileHeader.Filename = sanitizePlaygroundReferenceFileName(fileHeader.Filename)
	mimeType, assetType, err := detectPlaygroundReferenceFileType(fileHeader)
	if err != nil {
		playgroundReferenceUploadError(c, http.StatusBadRequest, "unsupported_media_type", err.Error())
		return
	}

	storage, err := service.NewAIPDDAssetStorage()
	if err != nil {
		playgroundReferenceUploadError(c, http.StatusServiceUnavailable, "aipdd_storage_unavailable", err.Error())
		return
	}

	file, err := fileHeader.Open()
	if err != nil {
		playgroundReferenceUploadError(c, http.StatusBadRequest, "invalid_upload", err.Error())
		return
	}
	defer file.Close()

	storedFile, err := storage.UploadPrivateAssetFile(
		c.Request.Context(),
		fileHeader.Filename,
		"new-api/playground",
		mimeType,
		fileHeader.Size,
		file,
	)
	if err != nil {
		playgroundReferenceUploadError(c, http.StatusBadGateway, "aipdd_upload_failed", err.Error())
		return
	}
	cleanupFile := true
	defer func() {
		if cleanupFile {
			cleanupPlaygroundAIPDDFile(storage, storedFile.FileID)
		}
	}()

	fileSize := storedFile.Size
	if fileSize <= 0 {
		fileSize = fileHeader.Size
	}
	asset, err := storage.CreateDigitalAssetWithLabels(
		c.Request.Context(),
		strings.TrimSpace(fileHeader.Filename),
		assetType,
		storedFile.FileID,
		fileSize,
		[]string{"new-api-playground"},
	)
	if err != nil {
		playgroundReferenceUploadError(c, http.StatusBadGateway, "aipdd_asset_create_failed", err.Error())
		return
	}
	cleanupAsset := true
	defer func() {
		if cleanupAsset {
			cleanupPlaygroundAIPDDAsset(storage, asset.ID)
		}
	}()

	signed, err := storage.SignFile(c.Request.Context(), storedFile.FileID)
	if err != nil || signed == nil || strings.TrimSpace(signed.URL) == "" {
		if err == nil {
			err = fmt.Errorf("AIPDD signing returned an empty URL")
		}
		playgroundReferenceUploadError(c, http.StatusBadGateway, "aipdd_sign_failed", err.Error())
		return
	}
	cleanupAsset = false
	cleanupFile = false

	common.ApiSuccess(c, gin.H{
		"url":        signed.URL,
		"filename":   fileHeader.Filename,
		"media_type": mimeType,
		"asset_id":   asset.ID,
		"file_id":    storedFile.FileID,
		"channel_id": storage.ChannelID(),
		"expires_at": signed.ExpiresAt,
	})
}

func detectPlaygroundReferenceFileType(header *multipart.FileHeader) (mimeType string, assetType string, err error) {
	declaredMime := normalizePlaygroundReferenceMime(header.Header.Get("Content-Type"))
	detectedMime, err := sniffPlaygroundReferenceFileMime(header)
	if err != nil {
		return "", "", err
	}
	if detectedType := playgroundReferenceTypeFromMime(detectedMime); detectedType != "" {
		return detectedMime, detectedType, nil
	}

	extensionMime := playgroundReferenceMimeFromExtension(header.Filename)
	extensionType := playgroundReferenceTypeFromMime(extensionMime)
	declaredType := playgroundReferenceTypeFromMime(declaredMime)
	if extensionType != "" && isGenericPlaygroundReferenceMime(detectedMime) {
		if declaredType == extensionType {
			return declaredMime, extensionType, nil
		}
		return extensionMime, extensionType, nil
	}

	mediaType := firstNonEmptyPlaygroundReferenceString(detectedMime, declaredMime, extensionMime)
	return "", "", fmt.Errorf("unsupported file type: %s", mediaType)
}

func sniffPlaygroundReferenceFileMime(header *multipart.FileHeader) (string, error) {
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
	return normalizePlaygroundReferenceMime(http.DetectContentType(sniff[:n])), nil
}

func playgroundReferenceTypeFromMime(mimeType string) string {
	switch {
	case strings.HasPrefix(normalizePlaygroundReferenceMime(mimeType), "image/"):
		return "image"
	case strings.HasPrefix(normalizePlaygroundReferenceMime(mimeType), "video/"):
		return "video"
	case strings.HasPrefix(normalizePlaygroundReferenceMime(mimeType), "audio/"):
		return "audio"
	default:
		return ""
	}
}

func playgroundReferenceMimeFromExtension(filename string) string {
	extension := strings.ToLower(filepath.Ext(filename))
	if extension == "" {
		return ""
	}
	if mimeType := normalizePlaygroundReferenceMime(mime.TypeByExtension(extension)); mimeType != "" {
		return mimeType
	}
	switch extension {
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

func normalizePlaygroundReferenceMime(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return ""
	}
	if mediaType, _, err := mime.ParseMediaType(value); err == nil {
		return strings.ToLower(mediaType)
	}
	if index := strings.Index(value, ";"); index >= 0 {
		return strings.TrimSpace(value[:index])
	}
	return value
}

func isGenericPlaygroundReferenceMime(mimeType string) bool {
	switch normalizePlaygroundReferenceMime(mimeType) {
	case "", "application/octet-stream", "application/ogg", "application/mp4", "application/x-riff", "application/x-matroska":
		return true
	default:
		return false
	}
}

func sanitizePlaygroundReferenceFileName(filename string) string {
	filename = strings.TrimSpace(strings.ReplaceAll(filename, "\\", "/"))
	filename = filepath.Base(filename)
	if filename == "." || filename == string(filepath.Separator) || filename == "" {
		return "upload.bin"
	}
	return filename
}

func firstNonEmptyPlaygroundReferenceString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func playgroundReferenceUploadTooLargeMessage() string {
	return fmt.Sprintf("file size exceeds the %d MB limit", playgroundReferenceUploadMaxBytes/(1024*1024))
}

func isPlaygroundReferenceUploadTooLargeError(err error) bool {
	if common.IsRequestBodyTooLargeError(err) {
		return true
	}
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "request body too large")
}

func cleanupPlaygroundAIPDDAsset(storage *service.AIPDDAssetStorage, assetID int64) {
	if storage == nil || assetID <= 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := storage.DeleteDigitalAsset(ctx, assetID); err != nil {
		common.SysLog(fmt.Sprintf("failed to clean up playground AIPDD digital asset %d: %v", assetID, err))
	}
}

func cleanupPlaygroundAIPDDFile(storage *service.AIPDDAssetStorage, fileID string) {
	if storage == nil || strings.TrimSpace(fileID) == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := storage.DeleteFile(ctx, fileID); err != nil {
		common.SysLog(fmt.Sprintf("failed to clean up playground AIPDD file %s: %v", fileID, err))
	}
}

func playgroundReferenceUploadError(c *gin.Context, status int, code string, message string) {
	c.JSON(status, gin.H{
		"message": message,
		"error": gin.H{
			"code":    code,
			"message": message,
			"type":    "invalid_request_error",
		},
	})
}
