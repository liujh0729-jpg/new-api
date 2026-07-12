package aipddcatalog

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/stretchr/testify/require"
)

func TestFetchAtomicFiltersExcludedFamiliesOnReceiver(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, AtomicCatalogPath, r.URL.Path)
		_, _ = w.Write([]byte(`{
			"code":0,"message":"ok","data":{
				"schemaVersion":1,"revision":"revision-1","generatedAt":"2026-07-12T10:00:00",
				"awcoinRate":{"rmbPerAwcoin":0.01,"usdPerAwcoin":0.0015,"updatedAt":"2026-07-12T09:00:00"},
				"capabilities":[
					{"id":"keep-comfy","code":"keep-comfy","adapterCode":"comfyui","execution":{"protocol":"shared_task","path":"/shared-tasks/tasks"},"pricing":{"enabled":true,"chargeConfig":{"amountAwcoin":10}}},
					{"id":"seedvr2-upscale","code":"seedvr2-upscale","adapterCode":"comfyui","execution":{"protocol":"shared_task","path":"/shared-tasks/tasks"},"pricing":{"enabled":true,"chargeConfig":{"amountAwcoin":10}}}
				],
				"models":[
					{"id":"qwen3:8b","execution":{"protocol":"openai","path":"/v1/chat/completions"},"pricing":{"enabled":true,"promptPerMillion":10,"completionPerMillion":20}},
					{"id":"funasr-llm","execution":{"protocol":"openai","path":"/v1/chat/completions"},"pricing":{"enabled":true,"promptPerMillion":10,"completionPerMillion":20}}
				]
			}
		}`))
	}))
	defer server.Close()

	catalog, err := FetchAtomic(context.Background(), server.Client(), server.URL, "sk-test")
	require.NoError(t, err)
	require.Equal(t, []string{"keep-comfy", "qwen3:8b"}, catalog.ModelNames())
}

func TestTaskAWCoinPriceUsesMinimumDeterministicSeedanceEstimate(t *testing.T) {
	pricing := AtomicPricing{ByResolution: map[string]constant.AIPDDSeedanceResolutionPricing{
		"4k": {
			DefaultDurationSeconds: 5,
			PriceVariants: []constant.AIPDDSeedancePriceVariant{{
				AWCoinPerSecond: 100, MinimumAWCoin: 600,
			}},
		},
		"720p": {
			DefaultDurationSeconds: 5,
			PriceVariants: []constant.AIPDDSeedancePriceVariant{{
				AWCoinPerSecond: 20.1, MinimumAWCoin: 50,
			}},
		},
	}}

	require.Equal(t, float64(101), TaskAWCoinPrice(pricing))
}
