package aipdd

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSeedanceCatalogBillingFactsMatrix(t *testing.T) {
	t.Cleanup(constant.ResetAIPDDCapabilities)
	constant.SetAIPDDCapabilities([]constant.AIPDDCapability{seedanceTestCapability()})

	tests := []struct {
		name              string
		body              string
		seconds           float64
		hasReferenceVideo bool
	}{
		{name: "decimal duration", body: `{"model":"AP Seedance","resolution":"1080p","duration":2.2,"content":[{"type":"text","text":"hello"}]}`, seconds: 2.2},
		{name: "frames divided by explicit fps", body: `{"model":"AP Seedance","resolution":"1080p","frames":49,"frames_per_second":24,"content":[{"type":"text","text":"hello"}]}`, seconds: 49.0 / 24},
		{name: "frames divided by fps alias", body: `{"model":"AP Seedance","resolution":"1080p","frames":30,"fps":12,"content":[{"type":"text","text":"hello"}]}`, seconds: 2.5},
		{name: "frames divided by catalog fps", body: `{"model":"AP Seedance","resolution":"1080p","frames":49,"content":[{"type":"text","text":"hello"}]}`, seconds: 49.0 / 24},
		{name: "reference video type", body: `{"model":"AP Seedance","resolution":"1080p","duration":5,"content":[{"type":"video","role":"input"}]}`, seconds: 5, hasReferenceVideo: true},
		{name: "reference video URL type", body: `{"model":"AP Seedance","resolution":"1080p","duration":5,"content":[{"type":"video_url","video_url":{"url":"https://cdn.example.com/reference.mp4"}}]}`, seconds: 5, hasReferenceVideo: true},
		{name: "reference video role", body: `{"model":"AP Seedance","resolution":"1080p","duration":5,"content":[{"type":"input_file","role":"reference_video"}]}`, seconds: 5, hasReferenceVideo: true},
		{name: "valid video URL field", body: `{"model":"AP Seedance","resolution":"1080p","duration":5,"content":[{"type":"input_file","video_url":"https://cdn.example.com/reference.mp4"}]}`, seconds: 5, hasReferenceVideo: true},
		{name: "empty video URL field is not video", body: `{"model":"AP Seedance","resolution":"1080p","duration":5,"content":[{"type":"input_file","video_url":""}]}`, seconds: 5},
		{name: "playground metadata compatibility", body: `{"model":"AP Seedance","prompt":"hello","duration":5,"metadata":{"resolution":"1080p","content":[{"type":"video_url","role":"reference_video","video_url":{"url":"https://cdn.example.com/reference.mp4"}}]}}`, seconds: 5, hasReferenceVideo: true},
		{name: "image and audio are not video", body: `{"model":"AP Seedance","resolution":"1080p","duration":5,"content":[{"type":"image_url","image_url":{"url":"https://cdn.example.com/reference.png"}},{"type":"audio","audio_url":"https://cdn.example.com/reference.mp3"}]}`, seconds: 5},
		{name: "model default duration", body: `{"model":"AP Seedance","resolution":"1080p","content":[{"type":"text","text":"hello"}]}`, seconds: 5},
		{name: "resolution catalog default duration", body: `{"model":"AP Seedance","resolution":"4k","content":[{"type":"text","text":"hello"}]}`, seconds: 7},
		{name: "resolution catalog default fps", body: `{"model":"AP Seedance","resolution":"4k","frames":61,"content":[{"type":"text","text":"hello"}]}`, seconds: 61.0 / 30},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, info, adaptor := seedanceRequestContext(t, test.body)
			require.Nil(t, adaptor.ValidateRequestAndSetAction(ctx, info))
			facts := adaptor.EstimateBilling(ctx, info)
			require.InDelta(t, test.seconds, facts["seconds"], 0.0000001)
			if test.hasReferenceVideo {
				require.Equal(t, float64(1), facts["has_reference_video"])
			} else {
				require.NotContains(t, facts, "has_reference_video")
			}
			require.NotContains(t, facts, "aipdd_awcoin")
			require.NotContains(t, facts, "aipdd_usd")
		})
	}
}

func TestSeedanceCatalogNormalizesPlaygroundPayloadForOfficialEndpoint(t *testing.T) {
	constant.SetAIPDDCapabilities([]constant.AIPDDCapability{seedanceTestCapability()})
	t.Cleanup(constant.ResetAIPDDCapabilities)

	body := `{"model":"AP Seedance","prompt":"hello","duration":5,"metadata":{"resolution":"1080p","ratio":"16:9","content":[{"type":"image_url","role":"reference_image","image_url":{"url":"https://cdn.example.com/reference.png"}}]}}`
	ctx, info, adaptor := seedanceRequestContext(t, body)
	require.Nil(t, adaptor.ValidateRequestAndSetAction(ctx, info))

	requestBody, err := adaptor.BuildRequestBody(ctx, info)
	require.NoError(t, err)
	data, err := io.ReadAll(requestBody)
	require.NoError(t, err)

	var payload map[string]any
	require.NoError(t, common.Unmarshal(data, &payload))
	require.Equal(t, "1080p", payload["resolution"])
	require.Equal(t, "16:9", payload["ratio"])
	content, ok := payload["content"].([]any)
	require.True(t, ok)
	require.Len(t, content, 1)
	require.Equal(t, "image_url", content[0].(map[string]any)["type"])
}

func TestSeedanceRemixResolutionInheritanceAndOverride(t *testing.T) {
	constant.SetAIPDDCapabilities([]constant.AIPDDCapability{seedanceTestCapability()})
	t.Cleanup(constant.ResetAIPDDCapabilities)

	t.Run("inherits original resolution when omitted", func(t *testing.T) {
		ctx, info, adaptor := seedanceRequestContext(t, `{"model":"AP Seedance","prompt":"hello"}`)
		info.Action = constant.TaskActionRemix
		info.TaskPricingFacts = &relaycommon.TaskPricingFacts{Resolution: "720p", Quantity: 5}
		require.Nil(t, adaptor.ValidateRequestAndSetAction(ctx, info))
		payload, err := getSeedanceOfficialPayload(ctx)
		require.NoError(t, err)
		require.Equal(t, "720p", payload["resolution"])
	})

	t.Run("uses explicit remix resolution", func(t *testing.T) {
		ctx, info, adaptor := seedanceRequestContext(t, `{"model":"AP Seedance","prompt":"hello","resolution":"1080p"}`)
		info.Action = constant.TaskActionRemix
		info.TaskPricingFacts = &relaycommon.TaskPricingFacts{Resolution: "720p", Quantity: 5}
		require.Nil(t, adaptor.ValidateRequestAndSetAction(ctx, info))
		payload, err := getSeedanceOfficialPayload(ctx)
		require.NoError(t, err)
		require.Equal(t, "1080p", payload["resolution"])
	})
}

func TestSeedanceCatalogNormalizesStandardVideoRequest(t *testing.T) {
	constant.SetAIPDDCapabilities([]constant.AIPDDCapability{seedanceTestCapability()})
	t.Cleanup(constant.ResetAIPDDCapabilities)

	body := `{"model":"AP Seedance","prompt":"cinematic city","width":1280,"height":720,"generate_audio":false,"seed":0,"priority":0}`
	ctx, info, adaptor := seedanceRequestContext(t, body)
	require.Nil(t, adaptor.ValidateRequestAndSetAction(ctx, info))

	requestBody, err := adaptor.BuildRequestBody(ctx, info)
	require.NoError(t, err)
	data, err := io.ReadAll(requestBody)
	require.NoError(t, err)

	var payload map[string]any
	require.NoError(t, common.Unmarshal(data, &payload))
	require.Equal(t, "720p", payload["resolution"])
	require.Equal(t, "16:9", payload["ratio"])
	require.Equal(t, false, payload["generate_audio"])
	require.Equal(t, float64(0), payload["seed"])
	require.Equal(t, float64(0), payload["priority"])
	content := payload["content"].([]any)
	require.Len(t, content, 1)
	require.Equal(t, map[string]any{"type": "text", "text": "cinematic city"}, content[0])
}

func TestSeedanceCatalogKeepsExplicitContentAndRootFields(t *testing.T) {
	constant.SetAIPDDCapabilities([]constant.AIPDDCapability{seedanceTestCapability()})
	t.Cleanup(constant.ResetAIPDDCapabilities)

	body := `{
		"model":"AP Seedance",
		"prompt":"must not be injected",
		"resolution":"4K",
		"ratio":"9:16",
		"width":1280,
		"height":720,
		"content":[{"type":"video_url","role":"reference_video","video_url":{"url":"https://cdn.example.com/reference.mp4"},"vendor_extension":{"keep":true}}],
		"generate_audio":false,
		"priority":0,
		"metadata":{"resolution":"720p","ratio":"16:9","content":[{"type":"text","text":"metadata"}],"generate_audio":true,"priority":8}
	}`
	ctx, info, adaptor := seedanceRequestContext(t, body)
	require.Nil(t, adaptor.ValidateRequestAndSetAction(ctx, info))
	payload, err := getSeedanceOfficialPayload(ctx)
	require.NoError(t, err)
	require.Equal(t, "4k", payload["resolution"])
	require.Equal(t, "9:16", payload["ratio"])
	require.Equal(t, false, payload["generate_audio"])
	require.Equal(t, float64(0), payload["priority"])
	content := payload["content"].([]any)
	require.Len(t, content, 1)
	require.Equal(t, "video_url", content[0].(map[string]any)["type"])
	require.Equal(t, map[string]any{"keep": true}, content[0].(map[string]any)["vendor_extension"])
}

func TestSeedanceCatalogInfersResolutionAndRatioFromDimensions(t *testing.T) {
	constant.SetAIPDDCapabilities([]constant.AIPDDCapability{seedanceTestCapability()})
	t.Cleanup(constant.ResetAIPDDCapabilities)

	tests := []struct {
		width, height int
		resolution    string
		ratio         string
	}{
		{1280, 720, "720p", "16:9"},
		{720, 1280, "720p", "9:16"},
		{720, 720, "720p", "1:1"},
		{960, 720, "720p", "4:3"},
		{720, 960, "720p", "3:4"},
		{1920, 1080, "1080p", "16:9"},
		{1080, 1920, "1080p", "9:16"},
		{3840, 2160, "4k", "16:9"},
		{2160, 3840, "4k", "9:16"},
	}
	for _, test := range tests {
		t.Run(fmt.Sprintf("%dx%d", test.width, test.height), func(t *testing.T) {
			body := fmt.Sprintf(`{"model":"AP Seedance","prompt":"hello","width":%d,"height":%d}`, test.width, test.height)
			ctx, info, adaptor := seedanceRequestContext(t, body)
			require.Nil(t, adaptor.ValidateRequestAndSetAction(ctx, info))
			payload, err := getSeedanceOfficialPayload(ctx)
			require.NoError(t, err)
			require.Equal(t, test.resolution, payload["resolution"])
			require.Equal(t, test.ratio, payload["ratio"])
		})
	}
}

func TestSeedanceCatalogValidationErrorsAreHTTP400(t *testing.T) {
	constant.SetAIPDDCapabilities([]constant.AIPDDCapability{seedanceTestCapability()})
	t.Cleanup(constant.ResetAIPDDCapabilities)

	tests := []struct {
		name string
		body string
		code string
	}{
		{"missing content", `{"model":"AP Seedance","resolution":"720p"}`, "missing_content"},
		{"invalid content", `{"model":"AP Seedance","resolution":"720p","content":"hello"}`, "invalid_content"},
		{"missing resolution", `{"model":"AP Seedance","prompt":"hello"}`, "missing_resolution"},
		{"incomplete dimensions", `{"model":"AP Seedance","prompt":"hello","width":1280}`, "invalid_dimensions"},
		{"invalid dimensions", `{"model":"AP Seedance","prompt":"hello","width":1000,"height":720}`, "invalid_dimensions"},
		{"unsupported ratio", `{"model":"AP Seedance","prompt":"hello","resolution":"720p","ratio":"2:1"}`, "unsupported_ratio"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, info, adaptor := seedanceRequestContext(t, test.body)
			taskErr := adaptor.ValidateRequestAndSetAction(ctx, info)
			require.NotNil(t, taskErr)
			require.Equal(t, http.StatusBadRequest, taskErr.StatusCode)
			require.Equal(t, test.code, taskErr.Code)
		})
	}

	t.Run("unsupported model", func(t *testing.T) {
		ctx, info, adaptor := seedanceRequestContextForModel(
			t,
			"AP Missing Seedance",
			`{"model":"AP Missing Seedance","prompt":"hello","resolution":"720p"}`,
		)
		taskErr := adaptor.ValidateRequestAndSetAction(ctx, info)
		require.NotNil(t, taskErr)
		require.Equal(t, http.StatusBadRequest, taskErr.StatusCode)
		require.Equal(t, "unsupported_model", taskErr.Code)
	})
}

func TestSeedanceCatalogBillingFactsIgnoreCatalogPrices(t *testing.T) {
	models := []string{
		"AP Seedance-2.0 VIP",
		"AP Seedance-2.0 标准版",
		"AP Seedance-2.0 轻量版",
		"AP Seedance-2.0 高性价比版",
	}
	capabilities := make([]constant.AIPDDCapability, 0, len(models))
	for index, modelName := range models {
		capability := seedanceTestCapabilityForModel(modelName)
		capability.AWCoinUSDPerCoin = float64(index+1) * 99
		for resolution, pricing := range capability.SeedancePricing.ByResolution {
			amount := float64((index + 1) * 10_000)
			pricing.AmountAWCoinPerSecond = amount
			pricing.TextInputAWCoinPerSecond = amount
			pricing.ImageInputAWCoinPerSecond = amount
			pricing.AudioInputAWCoinPerSecond = amount
			pricing.VideoInputAWCoinPerSecond = amount * 2
			capability.SeedancePricing.ByResolution[resolution] = pricing
		}
		capabilities = append(capabilities, capability)
	}
	constant.SetAIPDDCapabilities(capabilities)
	t.Cleanup(constant.ResetAIPDDCapabilities)

	resolutions := []string{"720p", "1080p", "4k"}
	for _, modelName := range models {
		for _, resolution := range resolutions {
			t.Run(modelName+"/"+resolution, func(t *testing.T) {
				body := fmt.Sprintf(`{"model":%q,"resolution":%q,"duration":5,"content":[{"type":"text","text":"hello"}]}`, modelName, resolution)
				ctx, info, adaptor := seedanceRequestContextForModel(t, modelName, body)
				require.Nil(t, adaptor.ValidateRequestAndSetAction(ctx, info))
				facts := adaptor.EstimateBilling(ctx, info)
				require.Equal(t, map[string]float64{"seconds": 5}, facts)
			})
		}
	}
}

func TestSeedanceCatalogRejectsMissingResolutionPricingMetadata(t *testing.T) {
	capability := seedanceTestCapability()
	capability.AdapterCode = "seedance"
	capability.SeedancePricing = nil
	constant.SetAIPDDCapabilities([]constant.AIPDDCapability{capability})
	t.Cleanup(constant.ResetAIPDDCapabilities)

	ctx, info, adaptor := seedanceRequestContext(t, `{"model":"AP Seedance","resolution":"480p","content":[{"type":"text","text":"hello"}]}`)
	taskErr := adaptor.ValidateRequestAndSetAction(ctx, info)
	require.NotNil(t, taskErr)
	require.Equal(t, "unsupported_resolution", taskErr.Code)
}

func TestSeedanceCatalogExecutionSnapshotContainsFactsNotUpstreamPrice(t *testing.T) {
	constant.SetAIPDDCapabilities([]constant.AIPDDCapability{seedanceTestCapability()})
	t.Cleanup(constant.ResetAIPDDCapabilities)

	ctx, info, adaptor := seedanceRequestContext(t, `{"model":"AP Seedance","resolution":"1080p","duration":2.25,"content":[{"type":"video_url","video_url":{"url":"https://cdn.example.com/reference.mp4"}}]}`)
	require.Nil(t, adaptor.ValidateRequestAndSetAction(ctx, info))
	info.PriceData.OtherRatios = adaptor.EstimateBilling(ctx, info)
	facts, taskErr := adaptor.EstimateTaskPricingFacts(ctx, info)
	require.Nil(t, taskErr)
	info.TaskPricingFacts = &facts

	snapshot := adaptor.AIPDDTaskSnapshot(info)
	require.NotNil(t, snapshot)
	require.Equal(t, "revision-1", snapshot.CatalogRevision)
	require.Equal(t, "seedance_official", snapshot.Protocol)
	require.Equal(t, "/api/v3/contents/generations/tasks", snapshot.Endpoint)
	require.InDelta(t, 2.25, snapshot.BillingSeconds, 0.0000001)
	require.True(t, snapshot.HasReferenceVideo)
	require.Zero(t, snapshot.USDPerAWCoin)
	require.Zero(t, snapshot.EstimatedAWCoin)
	require.Equal(t, "1080p", snapshot.Resolution)
}

func TestSeedanceOfficialBusinessErrorsUseHTTPStatus(t *testing.T) {
	constant.SetAIPDDCapabilities([]constant.AIPDDCapability{seedanceTestCapability()})
	t.Cleanup(constant.ResetAIPDDCapabilities)

	tests := []struct {
		name       string
		body       string
		statusCode int
	}{
		{"client error", `{"code":400,"message":"invalid resolution"}`, http.StatusBadRequest},
		{"server error", `{"code":500,"message":"internal error"}`, http.StatusBadGateway},
		{"nested client error", `{"error":{"code":422,"message":"invalid content"}}`, http.StatusUnprocessableEntity},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, info, adaptor := seedanceRequestContext(t, `{"model":"AP Seedance","prompt":"hello","resolution":"720p"}`)
			resp := &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(test.body))}
			_, _, taskErr := adaptor.DoResponse(ctx, resp, info)
			require.NotNil(t, taskErr)
			require.Equal(t, test.statusCode, taskErr.StatusCode)
			require.Equal(t, "seedance_task_create_failed", taskErr.Code)
		})
	}
}

func TestSeedanceOfficialTaskStatusParsing(t *testing.T) {
	tests := []struct {
		name   string
		body   string
		status model.TaskStatus
		url    string
		reason string
	}{
		{"queued", `{"id":"task-1","status":"pending"}`, model.TaskStatusQueued, "", ""},
		{"processing", `{"id":"task-1","status":"processing"}`, model.TaskStatusInProgress, "", ""},
		{"succeeded", `{"id":"task-1","status":"succeeded","content":{"video_url":"https://cdn.example.com/result.mp4"}}`, model.TaskStatusSuccess, "https://cdn.example.com/result.mp4", ""},
		{"failed", `{"id":"task-1","status":"failed","error":{"message":"generation failed"}}`, model.TaskStatusFailure, "", "generation failed"},
	}
	adaptor := &TaskAdaptor{}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := adaptor.ParseTaskResult([]byte(test.body))
			require.NoError(t, err)
			require.Equal(t, test.status, model.TaskStatus(result.Status))
			require.Equal(t, test.url, result.Url)
			require.Equal(t, test.reason, result.Reason)
		})
	}
}

func TestSeedanceCatalogRoutesCreateAndFetchToOfficialEndpoint(t *testing.T) {
	constant.SetAIPDDCapabilities([]constant.AIPDDCapability{seedanceTestCapability()})
	t.Cleanup(constant.ResetAIPDDCapabilities)
	requestedPath := ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedPath = r.URL.Path
		require.Equal(t, "Bearer sk-test", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"upstream-task","status":"queued"}`))
	}))
	defer server.Close()

	adaptor := &TaskAdaptor{baseURL: server.URL, apiKey: "sk-test"}
	info := &relaycommon.RelayInfo{OriginModelName: "AP Seedance"}
	info.ChannelMeta = &relaycommon.ChannelMeta{UpstreamModelName: "AP Seedance"}
	url, err := adaptor.BuildRequestURL(info)
	require.NoError(t, err)
	require.Equal(t, server.URL+"/api/v3/contents/generations/tasks", url)

	resp, err := adaptor.FetchTask(server.URL, "sk-test", map[string]any{
		"task_id": "upstream-task", "execution_protocol": "seedance_official",
		"execution_endpoint": "/api/v3/contents/generations/tasks",
	}, "")
	require.NoError(t, err)
	defer resp.Body.Close()
	_, _ = io.ReadAll(resp.Body)
	require.Equal(t, "/api/v3/contents/generations/tasks/upstream-task", requestedPath)
}

func seedanceRequestContext(t *testing.T, body string) (*gin.Context, *relaycommon.RelayInfo, *TaskAdaptor) {
	return seedanceRequestContextForModel(t, "AP Seedance", body)
}

func seedanceRequestContextForModel(t *testing.T, modelName, body string) (*gin.Context, *relaycommon.RelayInfo, *TaskAdaptor) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	info := &relaycommon.RelayInfo{
		OriginModelName: modelName,
		PriceData:       types.PriceData{GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 2}},
	}
	info.ChannelMeta = &relaycommon.ChannelMeta{ChannelBaseUrl: "https://aipdd.example", ApiKey: "sk-test", UpstreamModelName: modelName}
	info.TaskRelayInfo = &relaycommon.TaskRelayInfo{}
	adaptor := &TaskAdaptor{}
	adaptor.Init(info)
	return ctx, info, adaptor
}

func seedanceTestCapability() constant.AIPDDCapability {
	return seedanceTestCapabilityForModel("AP Seedance")
}

func seedanceTestCapabilityForModel(modelName string) constant.AIPDDCapability {
	return constant.AIPDDCapability{
		ModelName: modelName, TaskKind: "video_generation",
		EndpointType:    constant.EndpointTypeOpenAIVideo,
		BillingType:     constant.AIPDDBillingTypeDurationSeconds,
		CatalogRevision: "revision-1", ExecutionProtocol: "seedance_official",
		ExecutionPath: "/api/v3/contents/generations/tasks", AWCoinUSDPerCoin: 0.001,
		SeedancePricing: &constant.AIPDDSeedancePricing{ByResolution: map[string]constant.AIPDDSeedanceResolutionPricing{
			"720p": {
				TargetResolution:          "720p",
				DefaultDurationSeconds:    5,
				DefaultFramesPerSecond:    24,
				AmountAWCoinPerSecond:     10,
				TextInputAWCoinPerSecond:  10,
				ImageInputAWCoinPerSecond: 10,
				VideoInputAWCoinPerSecond: 12,
				AudioInputAWCoinPerSecond: 10,
			},
			"1080p": {
				TargetResolution:          "1080p",
				DefaultDurationSeconds:    5,
				DefaultFramesPerSecond:    24,
				AmountAWCoinPerSecond:     40.1,
				TextInputAWCoinPerSecond:  40.1,
				ImageInputAWCoinPerSecond: 40.1,
				VideoInputAWCoinPerSecond: 30,
				AudioInputAWCoinPerSecond: 40.1,
			},
			"4k": {
				TargetResolution:          "4k",
				DefaultDurationSeconds:    7,
				DefaultFramesPerSecond:    30,
				AmountAWCoinPerSecond:     70,
				TextInputAWCoinPerSecond:  70,
				ImageInputAWCoinPerSecond: 70,
				VideoInputAWCoinPerSecond: 80,
				AudioInputAWCoinPerSecond: 70,
			},
		}},
	}
}
