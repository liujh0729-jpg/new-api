package controller

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/aipddcatalog"
	"github.com/QuantumNous/new-api/service"
)

func refreshAIPDDCatalogForChannel(ctx context.Context, channel *model.Channel) (aipddcatalog.Catalog, error) {
	if channel == nil {
		return aipddcatalog.Catalog{}, fmt.Errorf("channel is nil")
	}
	baseURL := constant.ChannelBaseURLs[constant.ChannelTypeAIPDD]
	if channel.GetBaseURL() != "" {
		baseURL = channel.GetBaseURL()
	}

	client, err := service.GetHttpClientWithProxy(channel.GetSetting().Proxy)
	if err != nil {
		return aipddcatalog.Catalog{}, fmt.Errorf("new proxy http client failed: %w", err)
	}
	if client == nil {
		client = http.DefaultClient
	}

	catalog, err := aipddcatalog.Fetch(ctx, client, baseURL, getAIPDDChannelCatalogKey(channel))
	if err != nil {
		return aipddcatalog.Catalog{}, err
	}
	if len(catalog.Capabilities) > 0 {
		constant.SetAIPDDCapabilities(catalog.Capabilities)
		model.InvalidatePricingCache()
	}
	return catalog, nil
}

func getAIPDDChannelCatalogKey(channel *model.Channel) string {
	key, _, err := channel.GetNextEnabledKey()
	if err == nil {
		return strings.TrimSpace(key)
	}
	return strings.TrimSpace(strings.Split(channel.Key, "\n")[0])
}
