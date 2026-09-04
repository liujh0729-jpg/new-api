package controller

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

func ListSeedanceBaseModels(c *gin.Context) {
	items, err := model.ListCurrentSeedanceBaseModels(strings.EqualFold(c.Query("include_archived"), "true"))
	if err != nil {
		seedanceAdminFailure(c, http.StatusInternalServerError, err)
		return
	}
	common.ApiSuccess(c, items)
}

func SaveSeedanceBaseModel(c *gin.Context) {
	var item model.SeedanceBaseModel
	if err := c.ShouldBindJSON(&item); err != nil {
		seedanceAdminFailure(c, http.StatusBadRequest, err)
		return
	}
	if err := model.SaveSeedanceBaseModel(&item, c.GetInt("id")); err != nil {
		seedanceAdminFailure(c, http.StatusBadRequest, err)
		return
	}
	common.ApiSuccess(c, item)
}

func ArchiveSeedanceBaseModel(c *gin.Context) {
	archiveSeedanceCatalogModel(c, "base")
}

func ListSeedanceEnhancementModels(c *gin.Context) {
	items, err := model.ListCurrentSeedanceEnhancementModels(strings.EqualFold(c.Query("include_archived"), "true"))
	if err != nil {
		seedanceAdminFailure(c, http.StatusInternalServerError, err)
		return
	}
	common.ApiSuccess(c, items)
}

func SaveSeedanceEnhancementModel(c *gin.Context) {
	var item model.SeedanceEnhancementModel
	if err := c.ShouldBindJSON(&item); err != nil {
		seedanceAdminFailure(c, http.StatusBadRequest, err)
		return
	}
	provider, err := model.GetMediaEnhancementProvider(item.ProviderID)
	if err != nil {
		seedanceAdminFailure(c, http.StatusBadRequest, errors.New("active enhancement provider is required"))
		return
	}
	if providerAdapterType(provider) == model.SeedanceAdapterVolcengineMediaKit {
		if err := service.ValidateSeedanceMediaKitSpecification(item.SpecificationJSON); err != nil {
			seedanceAdminFailure(c, http.StatusBadRequest, err)
			return
		}
	}
	if err := model.SaveSeedanceEnhancementModel(&item, c.GetInt("id")); err != nil {
		seedanceAdminFailure(c, http.StatusBadRequest, err)
		return
	}
	common.ApiSuccess(c, item)
}

func ArchiveSeedanceEnhancementModel(c *gin.Context) {
	archiveSeedanceCatalogModel(c, "enhancement")
}

func archiveSeedanceCatalogModel(c *gin.Context, kind string) {
	id, err := strconv.ParseInt(strings.TrimSpace(c.Param("id")), 10, 64)
	if err != nil || id <= 0 {
		seedanceAdminFailure(c, http.StatusBadRequest, errors.New("valid Seedance catalog model id is required"))
		return
	}
	if err := model.ArchiveSeedanceCatalogModel(kind, id, c.GetInt("id")); err != nil {
		seedanceAdminFailure(c, http.StatusBadRequest, err)
		return
	}
	common.ApiSuccess(c, gin.H{"id": id, "archived": true})
}
