package controller

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/aipddcatalog"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

func getAIPDDBaseURLForChannel(channel *model.Channel) string {
	baseURL := constant.ChannelBaseURLs[constant.ChannelTypeAIPDD]
	if channel != nil && channel.GetBaseURL() != "" {
		baseURL = channel.GetBaseURL()
	}
	return baseURL
}

func getAIPDDHTTPClientForChannel(channel *model.Channel) (*http.Client, error) {
	if channel == nil {
		return nil, fmt.Errorf("channel is nil")
	}
	client, err := service.GetHttpClientWithProxy(channel.GetSetting().Proxy)
	if err != nil {
		return nil, fmt.Errorf("new proxy http client failed: %w", err)
	}
	if client == nil {
		client = http.DefaultClient
	}
	return client, nil
}

func refreshAIPDDCatalogForChannel(ctx context.Context, channel *model.Channel) (aipddcatalog.AtomicCatalog, error) {
	if channel == nil {
		return aipddcatalog.AtomicCatalog{}, fmt.Errorf("channel is nil")
	}
	client, err := getAIPDDHTTPClientForChannel(channel)
	if err != nil {
		return aipddcatalog.AtomicCatalog{}, err
	}

	return aipddcatalog.FetchAtomic(ctx, client, getAIPDDBaseURLForChannel(channel), getAIPDDChannelCatalogKey(channel))
}

func fetchAIPDDModelIDs(ctx context.Context, client *http.Client, baseURL, key string) ([]string, error) {
	catalog, err := aipddcatalog.FetchAtomic(ctx, client, baseURL, key)
	if err != nil {
		return nil, err
	}
	return catalog.ModelNames(), nil
}

func fetchAIPDDModelIDsForChannel(ctx context.Context, channel *model.Channel) ([]string, error) {
	if channel == nil {
		return nil, fmt.Errorf("channel is nil")
	}
	client, err := getAIPDDHTTPClientForChannel(channel)
	if err != nil {
		return nil, err
	}
	return fetchAIPDDModelIDs(ctx, client, getAIPDDBaseURLForChannel(channel), getAIPDDChannelCatalogKey(channel))
}

func getAIPDDChannelCatalogKey(channel *model.Channel) string {
	key, _, err := channel.GetNextEnabledKey()
	if err == nil {
		return strings.TrimSpace(key)
	}
	return strings.TrimSpace(strings.Split(channel.Key, "\n")[0])
}

// SyncAIPDDChannelCatalog atomically replaces the managed AIPDD channel's
// upstream model and capability metadata while preserving local pricing.
func SyncAIPDDChannelCatalog(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	channel, err := model.GetChannelById(id, true)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if channel.Type != constant.ChannelTypeAIPDD || channel.Name != "AIPDD" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "仅系统管理的 AIPDD 渠道支持目录同步"})
		return
	}
	client, err := getAIPDDHTTPClientForChannel(channel)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()
	result, err := model.SyncAIPDDCatalog(
		ctx,
		client,
		getAIPDDBaseURLForChannel(channel),
		getAIPDDChannelCatalogKey(channel),
	)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"success": false, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": result})
}
