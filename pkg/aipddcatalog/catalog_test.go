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
							{"paramKey": "prompt", "paramName": "视频描述", "dataType": "string", "isRequired": true, "orderNo": 1, "uiType": "textarea", "defaultValue": ""},
							{"paramKey": "image", "paramName": "参考图片", "dataType": "string", "isRequired": true, "orderNo": 2, "uiType": "image_url"},
							{"paramKey": "negativePrompt", "paramName": "负向描述", "dataType": "string", "isRequired": false, "orderNo": 3, "uiType": "textarea", "defaultValue": ""},
							{"paramKey": "width", "paramName": "视频宽度", "dataType": "int", "isRequired": true, "orderNo": 4, "uiType": "number", "defaultValue": 1920},
							{"paramKey": "height", "paramName": "视频高度", "dataType": "int", "isRequired": true, "orderNo": 5, "uiType": "number", "defaultValue": 1088},
							{"paramKey": "numFrames", "paramName": "视频帧数", "dataType": "int", "isRequired": true, "orderNo": 6, "uiType": "number", "defaultValue": 121},
							{"paramKey": "frameRate", "paramName": "帧率", "dataType": "int", "isRequired": false, "orderNo": 7, "uiType": "number", "defaultValue": 24}
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
	require.True(t, dynamic.RequiredWorkflowParams["width"])
	require.True(t, dynamic.RequiredWorkflowParams["height"])
	require.True(t, dynamic.RequiredWorkflowParams["numFrames"])
	requireWorkflowSource(t, dynamic.WorkflowDefaults, "negativePrompt", constant.AIPDDWorkflowSourceMetadata, "negative_prompt")
	requireNoWorkflowSourceType(t, dynamic.WorkflowDefaults, "negativePrompt", constant.AIPDDWorkflowSourcePrompt)
	requireWorkflowSource(t, dynamic.WorkflowDefaults, "width", constant.AIPDDWorkflowSourceStatic, "1920")
	requireWorkflowSource(t, dynamic.WorkflowDefaults, "height", constant.AIPDDWorkflowSourceStatic, "1088")
	requireWorkflowSource(t, dynamic.WorkflowDefaults, "numFrames", constant.AIPDDWorkflowSourceStatic, "121")
	requireWorkflowSource(t, dynamic.WorkflowDefaults, "frameRate", constant.AIPDDWorkflowSourceMetadata, "fps")
	requireWorkflowSource(t, dynamic.WorkflowDefaults, "frameRate", constant.AIPDDWorkflowSourceStatic, "24")
}

func TestFetchCatalogMapsImageCountParamsFromOpenAINParam(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/capabilities":
			_, _ = w.Write([]byte(`{
				"code": 0,
				"message": "fetched",
				"data": [
					{
						"id": "script-image-batch",
						"code": "aipdd_image_batch",
						"name": "Image Batch",
						"adapterCode": "comfyui",
						"endpointType": "image-generation",
						"taskKind": "text_to_image",
						"priceAWcoin": 100,
						"inputModalities": ["text"],
						"outputModalities": ["image"],
						"params": [
							{"paramKey": "prompt", "paramName": "提示词", "dataType": "string", "isRequired": true, "orderNo": 1},
							{"paramKey": "batch_size", "paramName": "生成数量", "dataType": "int", "isRequired": false, "orderNo": 2, "defaultValue": 1}
						]
					}
				]
			}`))
		case "/system/awcoin-rate":
			_, _ = w.Write([]byte(`{"code":200,"message":"ok","data":{"usd":0.0015}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	catalog, err := Fetch(ctx, server.Client(), server.URL, "test-key")
	require.NoError(t, err)
	require.Len(t, catalog.Capabilities, 1)

	dynamic := catalog.Capabilities[0]
	requireWorkflowSource(t, dynamic.WorkflowDefaults, "batch_size", constant.AIPDDWorkflowSourceMetadata, "n")
	requireWorkflowSource(t, dynamic.WorkflowDefaults, "batch_size", constant.AIPDDWorkflowSourceMetadata, "image_count")
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

func requireWorkflowSource(t *testing.T, defaults []constant.AIPDDWorkflowParamDefault, paramKey string, sourceType constant.AIPDDWorkflowSourceType, key string) {
	t.Helper()
	for _, item := range defaults {
		if item.ParamKey != paramKey {
			continue
		}
		for _, source := range item.Sources {
			if source.Type == sourceType && source.Key == key {
				return
			}
		}
		t.Fatalf("missing workflow source %s/%s for %s in %#v", sourceType, key, paramKey, item.Sources)
	}
	t.Fatalf("missing workflow default for %s in %#v", paramKey, defaults)
}

func requireNoWorkflowSourceType(t *testing.T, defaults []constant.AIPDDWorkflowParamDefault, paramKey string, sourceType constant.AIPDDWorkflowSourceType) {
	t.Helper()
	for _, item := range defaults {
		if item.ParamKey != paramKey {
			continue
		}
		for _, source := range item.Sources {
			if source.Type == sourceType {
				t.Fatalf("unexpected workflow source type %s for %s in %#v", sourceType, paramKey, item.Sources)
			}
		}
		return
	}
	t.Fatalf("missing workflow default for %s in %#v", paramKey, defaults)
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
