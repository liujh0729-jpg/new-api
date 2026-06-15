package controller

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/aipddcatalog"
	"github.com/QuantumNous/new-api/service"
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

func refreshAIPDDCatalogForChannel(ctx context.Context, channel *model.Channel) (aipddcatalog.Catalog, error) {
	if channel == nil {
		return aipddcatalog.Catalog{}, fmt.Errorf("channel is nil")
	}
	client, err := getAIPDDHTTPClientForChannel(channel)
	if err != nil {
		return aipddcatalog.Catalog{}, err
	}

	catalog, err := aipddcatalog.Fetch(ctx, client, getAIPDDBaseURLForChannel(channel), getAIPDDChannelCatalogKey(channel))
	if err != nil {
		return aipddcatalog.Catalog{}, err
	}
	if len(catalog.Capabilities) > 0 {
		constant.SetAIPDDCapabilities(catalog.Capabilities)
		model.InvalidatePricingCache()
	}
	return catalog, nil
}

func fetchAIPDDModelIDs(ctx context.Context, client *http.Client, baseURL, key string) ([]string, error) {
	catalog, catalogErr := aipddcatalog.Fetch(ctx, client, baseURL, key)
	if catalogErr == nil && len(catalog.Capabilities) > 0 {
		constant.SetAIPDDCapabilities(catalog.Capabilities)
		model.InvalidatePricingCache()
	}

	openAIModels, openAIErr := aipddcatalog.FetchOpenAIModels(ctx, client, baseURL, key)
	if openAIErr == nil {
		if err := model.EnsureAIPDDOpenAIModelDefaults(openAIModels); err != nil {
			return nil, err
		}
	}

	if catalogErr != nil && openAIErr != nil {
		return nil, fmt.Errorf("fetch AIPDD task catalog failed: %v; fetch AIPDD OpenAI models failed: %w", catalogErr, openAIErr)
	}
	if catalogErr != nil {
		common.SysLog("AIPDD task catalog fetch failed while syncing channel models: " + catalogErr.Error())
	}
	if openAIErr != nil {
		common.SysLog("AIPDD OpenAI model fetch failed while syncing channel models: " + openAIErr.Error())
	}

	return normalizeModelNames(mergeModelNames(catalog.ModelNames(), openAIModels)), nil
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
