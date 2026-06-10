package controller

import (
	"context"
	"fmt"
	"mime/multipart"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	taskaipdd "github.com/QuantumNous/new-api/relay/channel/task/aipdd"

	"github.com/gin-gonic/gin"
)

const materialUploadMaxBytes int64 = 30 * 1024 * 1024

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
			"message": fmt.Sprintf("file size exceeds the %d MB limit", materialUploadMaxBytes/(1024*1024)),
			"error": gin.H{
				"code":    "upload_file_too_large",
				"message": fmt.Sprintf("file size exceeds the %d MB limit", materialUploadMaxBytes/(1024*1024)),
				"type":    "invalid_request_error",
			},
		})
		return
	}

	mimeType := fileHeader.Header.Get("Content-Type")

	materialType := ""
	switch {
	case strings.HasPrefix(mimeType, "image/"):
		materialType = model.MaterialTypeImage
	case strings.HasPrefix(mimeType, "video/"):
		materialType = model.MaterialTypeVideo
	case strings.HasPrefix(mimeType, "audio/"):
		materialType = model.MaterialTypeAudio
	default:
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": fmt.Sprintf("unsupported file type: %s", mimeType),
			"error": gin.H{
				"code":    "unsupported_media_type",
				"message": fmt.Sprintf("unsupported file type: %s", mimeType),
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
	material := model.Material{
		UserId:      c.GetInt("id"),
		Name:        fileHeader.Filename,
		Type:        materialType,
		MimeType:    mimeType,
		FileName:    fileHeader.Filename,
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

	material.Name = req.Name
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
	c.Header("Content-Disposition", fmt.Sprintf(`inline; filename="%s"`, material.FileName))
	c.File(material.FilePath)
}
