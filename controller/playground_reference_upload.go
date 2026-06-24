package controller

import (
	"net/http"

	"github.com/QuantumNous/new-api/common"

	"github.com/gin-gonic/gin"
)

const playgroundReferenceUploadMaxBytes int64 = materialUploadMaxBytes

func PlaygroundUploadReferenceMedia(c *gin.Context) {
	fileHeader, err := c.FormFile("file")
	if err != nil {
		if isMaterialUploadTooLargeError(err) {
			playgroundReferenceUploadError(c, http.StatusRequestEntityTooLarge, "upload_file_too_large", materialUploadTooLargeMessage())
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
			materialUploadTooLargeMessage(),
		)
		return
	}
	fileHeader.Filename = sanitizeMaterialFileName(fileHeader.Filename)
	mimeType, materialType, err := detectMaterialFileType(fileHeader)
	if err != nil {
		playgroundReferenceUploadError(c, http.StatusBadRequest, "unsupported_media_type", err.Error())
		return
	}

	storedFile, err := uploadMaterialFile(c, fileHeader, mimeType, materialType)
	if err != nil {
		playgroundReferenceUploadError(c, http.StatusBadGateway, "local_material_upload_failed", err.Error())
		return
	}

	common.ApiSuccess(c, gin.H{
		"url":        storedFile.URL,
		"filename":   fileHeader.Filename,
		"media_type": mimeType,
	})
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
