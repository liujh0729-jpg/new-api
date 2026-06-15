package controller

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	taskaipdd "github.com/QuantumNous/new-api/relay/channel/task/aipdd"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
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
	mimeType, _, err := detectMaterialFileType(fileHeader)
	if err != nil {
		playgroundReferenceUploadError(c, http.StatusBadRequest, "unsupported_media_type", err.Error())
		return
	}

	channel, err := getPlaygroundAIPDDUploadChannel()
	if err != nil {
		playgroundReferenceUploadError(c, http.StatusServiceUnavailable, "aipdd_channel_unavailable", err.Error())
		return
	}

	apiKey, _, apiErr := channel.GetNextEnabledKey()
	if apiErr != nil {
		playgroundReferenceUploadError(c, http.StatusServiceUnavailable, "aipdd_channel_key_unavailable", apiErr.Error())
		return
	}
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		playgroundReferenceUploadError(c, http.StatusServiceUnavailable, "aipdd_channel_key_empty", "AIPDD channel key is empty")
		return
	}

	uploadedURL, err := taskaipdd.UploadFileToOSS(
		c.Request.Context(),
		channel.GetBaseURL(),
		apiKey,
		channel.GetSetting().Proxy,
		fileHeader,
	)
	if err != nil {
		playgroundReferenceUploadError(c, http.StatusBadGateway, "aipdd_oss_upload_failed", err.Error())
		return
	}

	common.ApiSuccess(c, gin.H{
		"url":        uploadedURL,
		"filename":   fileHeader.Filename,
		"media_type": mimeType,
	})
}

func getPlaygroundAIPDDUploadChannel() (*model.Channel, error) {
	var channel model.Channel
	err := model.DB.
		Where("type = ? AND status = ?", constant.ChannelTypeAIPDD, common.ChannelStatusEnabled).
		Order("id asc").
		First(&channel).
		Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("enabled AIPDD channel is not configured")
	}
	if err != nil {
		return nil, err
	}
	return &channel, nil
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
