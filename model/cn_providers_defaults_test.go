package model

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/stretchr/testify/require"
)

func TestCNProviderDefaultsExcludeLingYiWanWu(t *testing.T) {
	for _, provider := range cnProviders {
		require.NotEqual(t, constant.ChannelTypeLingYiWanWu, provider.ChannelType)
	}
}

func TestCNProviderDefaultsUseMainstreamModels(t *testing.T) {
	models := allCNProviderDefaultModels()

	require.Contains(t, models, "qwen3.7-max")
	require.Contains(t, models, "qwen3.7-plus")
	require.Contains(t, models, "qwen3.6-flash")
	require.NotContains(t, models, "qwen-turbo")
	require.NotContains(t, models, "qwen-plus")
	require.NotContains(t, models, "qwen-max")

	require.Contains(t, models, "glm-5.2")
	require.Contains(t, models, "glm-5.1")
	require.Contains(t, models, "glm-5-turbo")
	require.Contains(t, models, "glm-5v-turbo")
	require.NotContains(t, models, "glm-4.7")
	require.NotContains(t, models, "glm-4.7-flashx")

	require.Contains(t, models, "deepseek-v4-pro")
	require.Contains(t, models, "deepseek-v4-flash")
	require.NotContains(t, models, "deepseek-chat")
	require.NotContains(t, models, "deepseek-reasoner")

	require.Contains(t, models, "kimi-k2.7-code")
	require.Contains(t, models, "kimi-k2.7-code-highspeed")
	require.Contains(t, models, "kimi-k2.6")
	require.NotContains(t, models, "kimi-k2-thinking")

	require.Contains(t, models, "MiniMax-M3")
	require.Contains(t, models, "MiniMax-M2.7")
	require.Contains(t, models, "MiniMax-M2.7-highspeed")
	require.Contains(t, models, "speech-2.8-hd")
	require.Contains(t, models, "speech-2.8-turbo")
	require.NotContains(t, models, "MiniMax-M2.5")
	require.NotContains(t, models, "speech-2.5-hd-preview")

	require.Contains(t, models, "hy3-preview")
	require.NotContains(t, models, "hunyuan-2.0-thinking-20251109")
	require.NotContains(t, models, "hunyuan-2.0-instruct-20251111")
	require.NotContains(t, models, "hunyuan-t1-latest")
}

func TestCNProviderDefaultsUseTencentTokenHubOpenAIChannel(t *testing.T) {
	provider := findCNProviderByName(t, "腾讯混元")

	require.Equal(t, constant.ChannelTypeOpenAI, provider.ChannelType)
	require.Equal(t, "https://tokenhub.tencentmaas.com/v1", provider.BaseURL)
	require.Contains(t, providerModels(provider), "hy3-preview")
}

func TestCNProviderDefaultsUseCurrentDoubaoModels(t *testing.T) {
	doubaoProvider := findCNProviderByChannelType(t, constant.ChannelTypeVolcEngine)
	models := providerModels(doubaoProvider)

	require.Contains(t, models, "doubao-seed-evolving")
	require.Contains(t, models, "doubao-seed-2-1-pro-260628")
	require.Contains(t, models, "doubao-seed-2-1-turbo-260628")
	require.Contains(t, models, "doubao-seed-character-260628")
	require.Contains(t, models, "doubao-seedream-5-0-260128")
	require.Contains(t, models, "doubao-seedream-5-0-lite-260128")
	require.Contains(t, models, "doubao-seedance-2-0-260128")
	require.Contains(t, models, "doubao-seedance-2-0-fast-260128")
	require.Contains(t, models, "doubao-embedding-vision-251215")

	require.NotContains(t, models, "Doubao-pro-32k")
	require.NotContains(t, models, "Doubao-lite-32k")
	require.NotContains(t, models, "doubao-seed-1-6-thinking-250715")
	require.NotContains(t, models, "doubao-seedance-1-0-pro-250528")
	require.NotContains(t, models, "Doubao-embedding")
}

func findCNProviderByChannelType(t *testing.T, channelType int) *cnProviderInfo {
	t.Helper()
	for i := range cnProviders {
		if cnProviders[i].ChannelType == channelType {
			return &cnProviders[i]
		}
	}
	require.Failf(t, "provider not found", "channel type: %d", channelType)
	return nil
}

func findCNProviderByName(t *testing.T, namePart string) *cnProviderInfo {
	t.Helper()
	for i := range cnProviders {
		if strings.Contains(cnProviders[i].Name, namePart) {
			return &cnProviders[i]
		}
	}
	require.Failf(t, "provider not found", "name contains: %s", namePart)
	return nil
}

func providerModels(provider *cnProviderInfo) map[string]bool {
	models := make(map[string]bool, len(provider.Models))
	for _, item := range provider.Models {
		models[item.ModelName] = true
	}
	return models
}

func allCNProviderDefaultModels() map[string]bool {
	models := make(map[string]bool)
	for i := range cnProviders {
		for _, item := range cnProviders[i].Models {
			models[item.ModelName] = true
		}
	}
	return models
}
