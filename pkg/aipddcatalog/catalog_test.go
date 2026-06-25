package aipddcatalog

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/constant"
	"github.com/stretchr/testify/require"
)

func TestFetchBuildsCatalogFromScriptsAndFeeRules(t *testing.T) {
	seenAPIKey := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-Key") == "test-key" {
			seenAPIKey = true
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/scripts/admin/comfyui_workflow":
			_, _ = w.Write([]byte(`{
				"code": 200,
				"message": "ok",
				"data": [
					{
						"id": "script-flux",
						"code": "FLUX-GGUF-T2I-V2",
						"name": "Flux T2I",
						"description": "text to image",
						"priceAWcoin": 123,
						"params": [
							{"paramKey": "text", "dataType": "string", "isRequired": true, "orderNo": 1}
						]
					},
					{
						"id": "script-video",
						"code": "custom_video",
						"name": "Custom Video",
						"description": "video workflow",
						"priceAWcoin": 1,
						"endpointType": "audio-speech",
						"taskKind": "voice_clone",
						"inputModalities": ["audio", "text"],
						"outputModalities": ["audio"],
						"params": [
							{"paramKey": "audio", "dataType": "string", "isRequired": true, "orderNo": 1, "uiType": "audio_url"},
							{"paramKey": "prompt", "dataType": "string", "isRequired": false, "orderNo": 2}
						]
					}
				]
			}`))
		case "/fee-rules":
			require.Equal(t, "1", r.URL.Query().Get("page"))
			require.Equal(t, "100", r.URL.Query().Get("pageSize"))
			_, _ = w.Write([]byte(`{
				"code": 200,
				"message": "ok",
				"data": {
					"total": 2,
					"page": 1,
					"pageSize": 100,
					"list": [
						{"key": "FLUX-GGUF-T2I-V2", "name": "Flux T2I", "type": "task", "price": 200, "unit": "call"},
						{"key": "custom_video", "name": "Custom Video", "type": "task", "price": 500, "unit": "call"}
					]
				}
			}`))
		case "/system/awcoin-rate":
			_, _ = w.Write([]byte(`{
				"code": 200,
				"message": "ok",
				"data": {
					"rmb": 0.01,
					"usd": 0.0015,
					"updatedAt": "2026-06-03T21:38:28"
				}
			}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	catalog, err := Fetch(ctx, server.Client(), server.URL, "test-key")
	require.NoError(t, err)
	require.True(t, seenAPIKey)
	require.Equal(t, []string{constant.AIPDDModelFluxGGUFT2I, "custom_video"}, catalog.ModelNames())
	require.Equal(t, 0.0015, catalog.AWCoinUSDRate)
	require.Equal(t, 0.3, catalog.ModelPrices[constant.AIPDDModelFluxGGUFT2I])
	require.Equal(t, 0.75, catalog.ModelPrices["custom_video"])

	known := catalog.Capabilities[0]
	require.Equal(t, constant.AIPDDModelFluxGGUFT2I, known.ModelName)
	require.Equal(t, "script-flux", known.ScriptID)
	require.Equal(t, "FLUX-GGUF-T2I-V2", known.ScriptCode)
	require.Equal(t, constant.EndpointTypeImageGeneration, known.EndpointType)

	dynamic := catalog.Capabilities[1]
	require.Equal(t, "custom_video", dynamic.ModelName)
	require.Equal(t, constant.EndpointTypeAudioSpeech, dynamic.EndpointType)
	require.Equal(t, "voice_clone", dynamic.TaskKind)
	require.Equal(t, []string{"audio", "text"}, dynamic.InputModalities)
	require.Equal(t, []string{"audio"}, dynamic.OutputModalities)
	require.True(t, dynamic.RequiredWorkflowParams["audio"])
	require.Len(t, dynamic.UploadTargets, 1)
	require.Equal(t, "audio", dynamic.UploadTargets[0].ParamKey)
}

func TestFetchPrefersUnifiedCapabilitiesCatalog(t *testing.T) {
	seenAPIKey := false
	legacyRequested := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-Key") == "test-key" {
			seenAPIKey = true
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/capabilities":
			_, _ = w.Write([]byte(`{
				"code": 0,
				"message": "fetched",
				"data": [
					{
						"id": "qwen3:8b",
						"code": "llm:qwen3:8b",
						"name": "qwen3:8b",
						"adapterCode": "llm",
						"endpointType": "llm-chat",
						"taskKind": "chat_completion",
						"priceAWcoin": 1,
						"inputModalities": ["text"],
						"outputModalities": ["text"],
						"params": []
					},
					{
						"id": "script-flux",
						"code": "FLUX-GGUF-T2I-V2",
						"name": "Flux T2I",
						"description": "text to image",
						"adapterCode": "comfyui",
						"endpointType": "image-generation",
						"taskKind": "text_to_image",
						"priceAWcoin": 100,
						"inputModalities": ["text"],
						"outputModalities": ["image"],
						"params": [
							{"paramKey": "text", "dataType": "string", "isRequired": true, "orderNo": 1}
						]
					},
					{
						"id": "script-ltx2",
						"code": "aipdd_ltx2",
						"name": "AIPDD LTX2",
						"description": "image to video",
						"adapterCode": "ltx2_python",
						"endpointType": "openai-video",
						"taskKind": "image_to_video",
						"priceAWcoin": 500,
						"inputModalities": ["image", "text"],
						"outputModalities": ["video"],
						"params": [
							{"paramKey": "image", "dataType": "string", "isRequired": true, "orderNo": 1, "uiType": "image_url"}
						]
					}
				]
			}`))
		case "/system/awcoin-rate":
			_, _ = w.Write([]byte(`{
				"code": 200,
				"message": "ok",
				"data": {"usd": 0.0015}
			}`))
		case "/scripts/admin/comfyui_workflow":
			legacyRequested = true
			http.NotFound(w, r)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	catalog, err := Fetch(ctx, server.Client(), server.URL, "test-key")
	require.NoError(t, err)
	require.True(t, seenAPIKey)
	require.False(t, legacyRequested)
	require.Equal(t, []string{constant.AIPDDModelFluxGGUFT2I, "aipdd_ltx2"}, catalog.ModelNames())
	require.Equal(t, 0.0015, catalog.AWCoinUSDRate)
	require.Equal(t, 0.15, catalog.ModelPrices[constant.AIPDDModelFluxGGUFT2I])
	require.Equal(t, 0.75, catalog.ModelPrices["aipdd_ltx2"])

	known := catalog.Capabilities[0]
	require.Equal(t, constant.AIPDDModelFluxGGUFT2I, known.ModelName)
	require.Equal(t, "script-flux", known.ScriptID)
	require.Equal(t, "FLUX-GGUF-T2I-V2", known.ScriptCode)
	require.Equal(t, constant.EndpointTypeImageGeneration, known.EndpointType)

	dynamic := catalog.Capabilities[1]
	require.Equal(t, "aipdd_ltx2", dynamic.ModelName)
	require.Equal(t, constant.EndpointTypeOpenAIVideo, dynamic.EndpointType)
	require.Equal(t, "image_to_video", dynamic.TaskKind)
	require.Equal(t, []string{"image", "text"}, dynamic.InputModalities)
	require.Equal(t, []string{"video"}, dynamic.OutputModalities)
	require.True(t, dynamic.RequiredWorkflowParams["image"])
}

func TestFetchOpenAIModels(t *testing.T) {
	seenAPIKey := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-Key") == "test-key" {
			seenAPIKey = true
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/models":
			_, _ = w.Write([]byte(`{
				"object": "list",
				"data": [
					{"id": "gemma3:1b", "object": "model"},
					{"id": "gemma3:1b", "object": "model"},
					{"id": "qwen2.5:0.5b", "object": "model"},
					{"id": "", "object": "model"}
				]
			}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	models, err := FetchOpenAIModels(ctx, server.Client(), server.URL, "test-key")
	require.NoError(t, err)
	require.True(t, seenAPIKey)
	require.Equal(t, []string{"gemma3:1b", "qwen2.5:0.5b"}, models)
}

func TestFetchFallsBackToScriptPricesWhenFeeRulesUnavailable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/scripts/admin/comfyui_workflow":
			_, _ = w.Write([]byte(`{
				"code": 200,
				"message": "ok",
				"data": [
					{
						"id": "script-flux",
						"code": "FLUX-GGUF-T2I-V2",
						"name": "Flux T2I",
						"priceAWcoin": 123,
						"params": [
							{"paramKey": "text", "dataType": "string", "isRequired": true, "orderNo": 1}
						]
					}
				]
			}`))
		case "/fee-rules":
			http.NotFound(w, r)
		case "/system/awcoin-rate":
			_, _ = w.Write([]byte(`{
				"code": 200,
				"message": "ok",
				"data": {"usd": 0.0015}
			}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	catalog, err := Fetch(ctx, server.Client(), server.URL, "test-key")
	require.NoError(t, err)
	require.Equal(t, []string{constant.AIPDDModelFluxGGUFT2I}, catalog.ModelNames())
	require.Equal(t, 0.1845, catalog.ModelPrices[constant.AIPDDModelFluxGGUFT2I])
	require.Equal(t, float64(123), catalog.Capabilities[0].TaskCost)
}

func TestConvertUpstreamPriceToModelPriceFallsBackToAWCoinRMBRate(t *testing.T) {
	t.Setenv(envUSDPerCoin, "")
	t.Setenv(envCoinsPerRMB, "")
	t.Setenv(envUSD2RMB, "")

	require.Equal(t, 0.273973, ConvertUpstreamPriceToModelPrice(200))
}
