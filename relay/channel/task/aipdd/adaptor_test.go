package aipdd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestBuildRequestHeaderIncludesFinanceIdentity(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/tasks", nil)
	info := &relaycommon.RelayInfo{AIPDDFinance: &relaycommon.AIPDDFinanceContext{
		InstanceID: "instance-id", PlatformOrderID: "order-id", AttemptID: "order-id:1:8",
		NewAPIUserID: 12, NewAPITokenID: 34,
	}}
	adaptor := &TaskAdaptor{apiKey: "aipdd-key"}
	if err := adaptor.BuildRequestHeader(nil, req, info); err != nil {
		t.Fatalf("BuildRequestHeader: %v", err)
	}
	if req.Header.Get("X-AIPDD-Order-ID") != "order-id" || req.Header.Get("X-AIPDD-Instance-ID") != "instance-id" {
		t.Fatalf("finance identity headers missing: %#v", req.Header)
	}
	if req.Header.Get("X-AIPDD-Attempt-ID") != "order-id:1:8" ||
		req.Header.Get("X-AIPDD-NewAPI-User-ID") != "12" ||
		req.Header.Get("X-AIPDD-NewAPI-Token-ID") != "34" {
		t.Fatalf("finance attempt/audit headers missing: %#v", req.Header)
	}
}

func TestAIPDDTaskSnapshotPersistsImageMediaMetadata(t *testing.T) {
	const modelName = "dynamic-image-to-image"
	constant.SetAIPDDCapabilities([]constant.AIPDDCapability{
		{
			ModelName:        modelName,
			TaskKind:         "image_to_image",
			OutputModalities: []string{"image"},
			EndpointType:     constant.EndpointTypeImageGeneration,
		},
	})
	t.Cleanup(constant.ResetAIPDDCapabilities)

	snapshot := (&TaskAdaptor{}).AIPDDTaskSnapshot(&relaycommon.RelayInfo{
		OriginModelName: modelName,
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: modelName,
		},
	})

	if snapshot == nil {
		t.Fatal("expected AIPDD execution snapshot")
	}
	if snapshot.EndpointType != constant.EndpointTypeImageToImage {
		t.Fatalf("unexpected endpoint type: %s", snapshot.EndpointType)
	}
	if snapshot.MediaType != "image" {
		t.Fatalf("unexpected media type: %s", snapshot.MediaType)
	}
	if snapshot.TaskKind != "image_to_image" {
		t.Fatalf("unexpected task kind: %s", snapshot.TaskKind)
	}
	if len(snapshot.OutputModalities) != 1 || snapshot.OutputModalities[0] != "image" {
		t.Fatalf("unexpected output modalities: %#v", snapshot.OutputModalities)
	}
}

func TestQwenImageEditMultipartSelectsImagesAndBuildsCanonicalPayload(t *testing.T) {
	original := constant.GetAIPDDCapabilities()
	t.Cleanup(func() { constant.SetAIPDDCapabilities(original) })
	constant.SetAIPDDCapabilities([]constant.AIPDDCapability{{
		ModelName:         qwenImageEditModelID,
		ScriptCode:        qwenImageEditModelID,
		EndpointType:      constant.EndpointTypeImageGeneration,
		BillingType:       constant.AIPDDBillingTypePerCall,
		WorkflowParamKeys: []string{"prompt", "images", "image_1", "image_2", "image_3"},
		RequiredWorkflowParams: map[string]bool{
			"prompt": true, "images": true,
		},
	}})

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	require.NoError(t, writer.WriteField("model", qwenImageEditModelID))
	require.NoError(t, writer.WriteField("prompt", "merge the references"))
	for _, content := range []string{"\x89PNG\r\n\x1a\nfirst-image", "\x89PNG\r\n\x1a\nsecond-image"} {
		part, err := writer.CreateFormFile("image", "input.png")
		require.NoError(t, err)
		_, err = part.Write([]byte(content))
		require.NoError(t, err)
	}
	require.NoError(t, writer.Close())

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", &body)
	ctx.Request.Header.Set("Content-Type", writer.FormDataContentType())
	adaptor := &TaskAdaptor{}
	info := relayInfoWithModel(qwenImageEditModelID)
	require.Nil(t, adaptor.ValidateRequestAndSetAction(ctx, info))
	requestBody, err := adaptor.BuildRequestBody(ctx, info)
	require.NoError(t, err)
	encoded, err := io.ReadAll(requestBody)
	require.NoError(t, err)
	var payload createTaskPayload
	require.NoError(t, common.Unmarshal(encoded, &payload))
	require.Equal(t, qwenImageEditModelID, payload.Model)
	require.Len(t, payload.Input["images"], 2)
	require.Contains(t, payload.Input["image_1"], "data:image/")
	require.Contains(t, payload.Input["image_2"], "data:image/")
	require.NotContains(t, payload.Input, "image_3")
}

func TestQwenImageEditMultipartRequiresOneToThreeImages(t *testing.T) {
	for _, count := range []int{0, 4} {
		t.Run(fmt.Sprintf("images_%d", count), func(t *testing.T) {
			var body bytes.Buffer
			writer := multipart.NewWriter(&body)
			require.NoError(t, writer.WriteField("model", qwenImageEditModelID))
			require.NoError(t, writer.WriteField("prompt", "edit"))
			for index := 0; index < count; index++ {
				part, err := writer.CreateFormFile("image", fmt.Sprintf("input-%d.png", index))
				require.NoError(t, err)
				_, err = part.Write([]byte("\x89PNG\r\n\x1a\nimage"))
				require.NoError(t, err)
			}
			require.NoError(t, writer.Close())

			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", &body)
			ctx.Request.Header.Set("Content-Type", writer.FormDataContentType())
			var request relaycommon.TaskSubmitReq
			err := normalizeQwenImageEditMultipart(ctx, &request)
			require.ErrorContains(t, err, "between 1 and 3")
		})
	}
}

func TestImageEndpointRoutingMatchesOpenAITransport(t *testing.T) {
	require.False(t, capabilityAcceptsEndpoint(
		constant.EndpointTypeImageToImage,
		constant.EndpointTypeImageGeneration,
	))
	require.True(t, capabilityAcceptsEndpoint(
		constant.EndpointTypeImageToImage,
		constant.EndpointTypeImageEdit,
	))
	require.True(t, capabilityAcceptsEndpoint(
		constant.EndpointTypeImageEdit,
		constant.EndpointTypeImageEdit,
	))
	require.False(t, capabilityAcceptsEndpoint(
		constant.EndpointTypeImageEdit,
		constant.EndpointTypeImageGeneration,
	))
}

func TestImageToImageMultipartAcceptsOfficialImageArrayField(t *testing.T) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	require.NoError(t, writer.WriteField("model", ModelFluxGGUF))
	require.NoError(t, writer.WriteField("prompt", "use this reference"))
	part, err := writer.CreateFormFile("image[]", "reference.png")
	require.NoError(t, err)
	_, err = part.Write([]byte("\x89PNG\r\n\x1a\nreference-image"))
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", &body)
	ctx.Request.Header.Set("Content-Type", writer.FormDataContentType())
	adaptor := &TaskAdaptor{}
	info := relayInfoWithModel(ModelFluxGGUF)
	require.Nil(t, adaptor.ValidateRequestAndSetAction(ctx, info))
	request, err := relaycommon.GetTaskRequest(ctx)
	require.NoError(t, err)
	require.Len(t, request.Images, 1)
	require.Equal(t, request.Images[0], request.Image)
	require.Contains(t, request.Image, "data:image/")

	requestBody, err := adaptor.BuildRequestBody(ctx, info)
	require.NoError(t, err)
	encoded, err := io.ReadAll(requestBody)
	require.NoError(t, err)
	var payload createTaskPayload
	require.NoError(t, common.Unmarshal(encoded, &payload))
	require.Contains(t, payload.Input["image"], "data:image/")
}

func TestCanonicalLtx23VariantsBuildDistinctWorkflowInputs(t *testing.T) {
	original := constant.GetAIPDDCapabilities()
	t.Cleanup(func() { constant.SetAIPDDCapabilities(original) })
	constant.SetAIPDDCapabilities([]constant.AIPDDCapability{canonicalLtx23TestCapability()})
	adaptor := &TaskAdaptor{}

	standard, err := adaptor.convertToRequestPayload(relaycommon.TaskSubmitReq{
		Model: ltx23PublicModelID, Prompt: "standard", Image: "https://cdn.example/scene.png", Duration: 5,
		Metadata: map[string]interface{}{"variant": "standard", "width": 1280, "height": 704},
	}, relayInfoWithModel(ltx23PublicModelID))
	require.NoError(t, err)
	require.Equal(t, "standard", standard.Input["variant"])
	require.Equal(t, 121, standard.Input["numFrames"])

	startEnd, err := adaptor.convertToRequestPayload(relaycommon.TaskSubmitReq{
		Model: ltx23PublicModelID, Prompt: "start end", FirstFrame: "https://cdn.example/first.png", Duration: 5,
		Metadata: map[string]interface{}{"variant": "start_end", "width": 1280, "height": 704},
	}, relayInfoWithModel(ltx23PublicModelID))
	require.NoError(t, err)
	require.Equal(t, "https://cdn.example/first.png", startEnd.Input["first_frame_image"])
	require.NotContains(t, startEnd.Input, "last_frame_image")

	licon, err := adaptor.convertToRequestPayload(relaycommon.TaskSubmitReq{
		Model: ltx23PublicModelID, Prompt: "one role", Duration: 5,
		Images:   []string{"https://cdn.example/background.png", "https://cdn.example/role.png"},
		Metadata: map[string]interface{}{"variant": "licon_1role"},
	}, relayInfoWithModel(ltx23PublicModelID))
	require.NoError(t, err)
	require.Equal(t, "https://cdn.example/background.png", licon.Input["image"])
	require.Equal(t, "https://cdn.example/role.png", licon.Input["referenceImage"])
	require.Equal(t, 720, licon.Input["height"])

	_, err = adaptor.convertToRequestPayload(relaycommon.TaskSubmitReq{
		Model: ltx23PublicModelID, Prompt: "two roles", Duration: 5,
		Images:   []string{"https://cdn.example/background.png", "https://cdn.example/role1.png"},
		Metadata: map[string]interface{}{"variant": "licon_2role"},
	}, relayInfoWithModel(ltx23PublicModelID))
	require.ErrorContains(t, err, "requires exactly 3 images")

	_, err = adaptor.convertToRequestPayload(relaycommon.TaskSubmitReq{
		Model: ltx23PublicModelID, Prompt: "one role with an extra", Duration: 5,
		Images: []string{
			"https://cdn.example/background.png",
			"https://cdn.example/role.png",
			"https://cdn.example/extra.png",
		},
		Metadata: map[string]interface{}{"variant": "licon_1role"},
	}, relayInfoWithModel(ltx23PublicModelID))
	require.ErrorContains(t, err, "requires exactly 2 images")

	_, err = adaptor.convertToRequestPayload(relaycommon.TaskSubmitReq{
		Model: ltx23PublicModelID, Prompt: "missing variant", Image: "https://cdn.example/scene.png", Duration: 5,
		Metadata: map[string]interface{}{"width": 1280, "height": 704},
	}, relayInfoWithModel(ltx23PublicModelID))
	require.ErrorContains(t, err, "variant is required")
}

func canonicalLtx23TestCapability() constant.AIPDDCapability {
	keys := []string{"variant", "prompt", "image", "first_frame_image", "last_frame_image", "referenceImage", "referenceImage2", "audio", "negativePrompt", "width", "height", "durationSeconds", "numFrames", "frameRate", "seed"}
	required := make(map[string]bool, len(keys))
	required["variant"] = true
	required["prompt"] = true
	return constant.AIPDDCapability{
		ModelName: ltx23PublicModelID, ScriptCode: ltx23PublicModelID,
		EndpointType: constant.EndpointTypeOpenAIVideo, BillingType: constant.AIPDDBillingTypeDurationSeconds,
		WorkflowParamKeys: keys, RequiredWorkflowParams: required,
		WorkflowDefaults: []constant.AIPDDWorkflowParamDefault{
			{ParamKey: "variant", ValueType: constant.AIPDDWorkflowValueTypeString, Sources: []constant.AIPDDWorkflowValueSource{{Type: constant.AIPDDWorkflowSourceMetadata, Key: "variant"}, {Type: constant.AIPDDWorkflowSourceStatic, Key: "standard"}}},
			{ParamKey: "prompt", ValueType: constant.AIPDDWorkflowValueTypeString, Sources: []constant.AIPDDWorkflowValueSource{{Type: constant.AIPDDWorkflowSourcePrompt}}},
			{ParamKey: "image", ValueType: constant.AIPDDWorkflowValueTypeString, Sources: []constant.AIPDDWorkflowValueSource{{Type: constant.AIPDDWorkflowSourceImage}}},
			{ParamKey: "first_frame_image", ValueType: constant.AIPDDWorkflowValueTypeString, Sources: []constant.AIPDDWorkflowValueSource{{Type: constant.AIPDDWorkflowSourceFirstImage}}},
			{ParamKey: "last_frame_image", ValueType: constant.AIPDDWorkflowValueTypeString, Sources: []constant.AIPDDWorkflowValueSource{{Type: constant.AIPDDWorkflowSourceLastImage}}},
			{ParamKey: "referenceImage", ValueType: constant.AIPDDWorkflowValueTypeString, Sources: []constant.AIPDDWorkflowValueSource{{Type: constant.AIPDDWorkflowSourceMetadata, Key: "referenceImage"}}},
			{ParamKey: "referenceImage2", ValueType: constant.AIPDDWorkflowValueTypeString, Sources: []constant.AIPDDWorkflowValueSource{{Type: constant.AIPDDWorkflowSourceMetadata, Key: "referenceImage2"}}},
			{ParamKey: "width", ValueType: constant.AIPDDWorkflowValueTypeInt, Sources: []constant.AIPDDWorkflowValueSource{{Type: constant.AIPDDWorkflowSourceMetadata, Key: "width"}, {Type: constant.AIPDDWorkflowSourceStatic, Key: "1280"}}},
			{ParamKey: "height", ValueType: constant.AIPDDWorkflowValueTypeInt, Sources: []constant.AIPDDWorkflowValueSource{{Type: constant.AIPDDWorkflowSourceMetadata, Key: "height"}, {Type: constant.AIPDDWorkflowSourceStatic, Key: "704"}}},
			{ParamKey: "durationSeconds", ValueType: constant.AIPDDWorkflowValueTypeInt, Sources: []constant.AIPDDWorkflowValueSource{{Type: constant.AIPDDWorkflowSourceDuration}, {Type: constant.AIPDDWorkflowSourceStatic, Key: "5"}}},
			{ParamKey: "numFrames", ValueType: constant.AIPDDWorkflowValueTypeInt, Sources: []constant.AIPDDWorkflowValueSource{{Type: constant.AIPDDWorkflowSourceStatic, Key: "121"}}},
			{ParamKey: "frameRate", ValueType: constant.AIPDDWorkflowValueTypeInt, Sources: []constant.AIPDDWorkflowValueSource{{Type: constant.AIPDDWorkflowSourceStatic, Key: "24"}}},
		},
	}
}

func TestConvertToSeedanceOfficialTaskUsesPublicFieldsActualDurationAndUsage(t *testing.T) {
	requestedDuration := 6.0
	task := &model.Task{
		TaskID: "task_public",
		Status: model.TaskStatusSuccess,
		Properties: model.Properties{
			OriginModelName:   "AP Seedance-2.5 标准版",
			UpstreamModelName: "seedance-2-5-260628",
		},
		PrivateData: model.TaskPrivateData{AIPDDExecution: &model.AIPDDTaskExecutionSnapshot{
			Protocol: "seedance_official", RequestedDuration: &requestedDuration,
		}},
		Data: json.RawMessage(`{
			"id":"cgt-upstream","task_id":"cgt-upstream","model":"seedance-2-5-260628",
			"status":"completed","duration":8,"billing_mode":"internal","finance_cost":99,
			"usage":{"completion_tokens":38830,"total_tokens":38830},
			"content":{"video_url":"https://cdn.example.com/out.mp4","upstream_model_id":"internal-model"}
		}`),
	}

	data, err := (&TaskAdaptor{}).ConvertToSeedanceOfficialTask(task)
	require.NoError(t, err)
	var response map[string]any
	require.NoError(t, common.Unmarshal(data, &response))
	require.Equal(t, "task_public", response["id"])
	require.Equal(t, "AP Seedance-2.5 标准版", response["model"])
	require.Equal(t, "succeeded", response["status"])
	require.Equal(t, 8.0, response["duration"])
	require.NotContains(t, response, "task_id")
	require.NotContains(t, response, "billing_mode")
	require.NotContains(t, response, "finance_cost")
	usage, ok := response["usage"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, float64(38830), usage["completion_tokens"])
	require.Equal(t, float64(38830), usage["total_tokens"])
	content, ok := response["content"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "https://cdn.example.com/out.mp4", content["video_url"])
	require.NotContains(t, content, "upstream_model_id")
}

func TestConvertToOpenAIVideoReturnsEquivalentUsageWhenPresent(t *testing.T) {
	task := &model.Task{
		TaskID: "task_seedance_usage",
		Status: model.TaskStatusSuccess,
		Properties: model.Properties{
			OriginModelName: "AP Seedance-2.0 标准版",
		},
		Data: json.RawMessage(`{
			"id":"upstream-seedance-task","status":"succeeded",
			"usage":{"completion_tokens":1600000,"total_tokens":1600000}
		}`),
	}

	data, err := (&TaskAdaptor{}).ConvertToOpenAIVideo(task)
	require.NoError(t, err)
	var response dto.OpenAIVideo
	require.NoError(t, common.Unmarshal(data, &response))
	require.NotNil(t, response.Usage)
	require.Equal(t, int64(1_600_000), response.Usage.CompletionTokens)
	require.Equal(t, int64(1_600_000), response.Usage.TotalTokens)
}

func TestConvertToOpenAIVideoOmitsUsageWhenAIPDDEmitsNone(t *testing.T) {
	task := &model.Task{
		TaskID: "task_seedance_without_usage",
		Status: model.TaskStatusSuccess,
		Data:   json.RawMessage(`{"id":"upstream-seedance-task","status":"succeeded"}`),
	}

	data, err := (&TaskAdaptor{}).ConvertToOpenAIVideo(task)
	require.NoError(t, err)
	require.NotContains(t, string(data), `"usage"`)
}

func TestSeedanceUsageIsIdenticalAcrossPublicResponseShapes(t *testing.T) {
	tests := []struct {
		name            string
		data            string
		wantUsage       bool
		completionToken int64
		totalToken      int64
	}{
		{
			name:            "valid usage",
			data:            `{"id":"cgt-valid","status":"succeeded","usage":{"completion_tokens":190910,"total_tokens":190910}}`,
			wantUsage:       true,
			completionToken: 190910,
			totalToken:      190910,
		},
		{
			name:      "usage omitted by aipdd",
			data:      `{"id":"cgt-missing","status":"succeeded"}`,
			wantUsage: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			task := &model.Task{
				TaskID:     "task-public",
				Status:     model.TaskStatusSuccess,
				Properties: model.Properties{OriginModelName: "AP Seedance-2.0 标准版"},
				Data:       json.RawMessage(test.data),
			}

			officialData, err := (&TaskAdaptor{}).ConvertToSeedanceOfficialTask(task)
			require.NoError(t, err)
			videoData, err := (&TaskAdaptor{}).ConvertToOpenAIVideo(task)
			require.NoError(t, err)
			var official map[string]any
			var video map[string]any
			require.NoError(t, common.Unmarshal(officialData, &official))
			require.NoError(t, common.Unmarshal(videoData, &video))

			officialUsage, officialHasUsage := official["usage"].(map[string]any)
			videoUsage, videoHasUsage := video["usage"].(map[string]any)
			require.Equal(t, test.wantUsage, officialHasUsage)
			require.Equal(t, test.wantUsage, videoHasUsage)
			if test.wantUsage {
				require.Equal(t, officialUsage["completion_tokens"], videoUsage["completion_tokens"])
				require.Equal(t, officialUsage["total_tokens"], videoUsage["total_tokens"])
				require.Equal(t, float64(test.completionToken), officialUsage["completion_tokens"])
				require.Equal(t, float64(test.totalToken), officialUsage["total_tokens"])
			}
		})
	}
}

func TestSeedanceToolUsageIsKeptOnlyInOfficialResponse(t *testing.T) {
	task := &model.Task{
		TaskID:     "task-tool-usage",
		Status:     model.TaskStatusSuccess,
		Properties: model.Properties{OriginModelName: "AP Seedance-2.0 VIP"},
		Data: json.RawMessage(`{
			"id":"cgt-tool-usage","status":"succeeded",
			"usage":{"completion_tokens":52174,"total_tokens":52174,
			         "tool_usage":{"web_search":1}}
		}`),
	}

	officialData, err := (&TaskAdaptor{}).ConvertToSeedanceOfficialTask(task)
	require.NoError(t, err)
	videoData, err := (&TaskAdaptor{}).ConvertToOpenAIVideo(task)
	require.NoError(t, err)
	var official map[string]any
	var video map[string]any
	require.NoError(t, common.Unmarshal(officialData, &official))
	require.NoError(t, common.Unmarshal(videoData, &video))
	officialUsage := official["usage"].(map[string]any)
	videoUsage := video["usage"].(map[string]any)
	require.Equal(t, float64(1), officialUsage["tool_usage"].(map[string]any)["web_search"])
	require.ElementsMatch(t, []string{"completion_tokens", "total_tokens"}, mapKeys(videoUsage))
}

func TestConvertToOpenAIVideoSupportsInt64Usage(t *testing.T) {
	task := &model.Task{
		TaskID: "task-max-usage",
		Status: model.TaskStatusSuccess,
		Data: json.RawMessage(`{
			"id":"cgt-max-usage","status":"succeeded",
			"usage":{"completion_tokens":9223372036854775807,"total_tokens":9223372036854775807}
		}`),
	}

	data, err := (&TaskAdaptor{}).ConvertToOpenAIVideo(task)
	require.NoError(t, err)
	var response dto.OpenAIVideo
	require.NoError(t, common.Unmarshal(data, &response))
	require.NotNil(t, response.Usage)
	require.Equal(t, int64(9223372036854775807), response.Usage.CompletionTokens)
	require.Equal(t, int64(9223372036854775807), response.Usage.TotalTokens)
}

func TestConvertToSeedanceOfficialTaskDurationFallbackAndAutoOmission(t *testing.T) {
	requestedDuration := 5.5
	withFallback := &model.Task{
		TaskID: "task_fixed", Status: model.TaskStatusInProgress,
		Properties: model.Properties{OriginModelName: "AP Seedance"},
		PrivateData: model.TaskPrivateData{AIPDDExecution: &model.AIPDDTaskExecutionSnapshot{
			Protocol: "seedance_official", RequestedDuration: &requestedDuration,
		}},
		Data: json.RawMessage(`{"id":"cgt-fixed","status":"processing"}`),
	}
	data, err := (&TaskAdaptor{}).ConvertToSeedanceOfficialTask(withFallback)
	require.NoError(t, err)
	var response map[string]any
	require.NoError(t, common.Unmarshal(data, &response))
	require.Equal(t, "running", response["status"])
	require.Equal(t, requestedDuration, response["duration"])

	automatic := &model.Task{
		TaskID: "task_auto", Status: model.TaskStatusQueued,
		Properties:  model.Properties{OriginModelName: "AP Seedance-2.5 标准版"},
		PrivateData: model.TaskPrivateData{AIPDDExecution: &model.AIPDDTaskExecutionSnapshot{Protocol: "seedance_official"}},
		Data:        json.RawMessage(`{"id":"cgt-auto","status":"pending","duration":-1}`),
	}
	data, err = (&TaskAdaptor{}).ConvertToSeedanceOfficialTask(automatic)
	require.NoError(t, err)
	response = map[string]any{}
	require.NoError(t, common.Unmarshal(data, &response))
	require.Equal(t, "queued", response["status"])
	require.NotContains(t, response, "duration")
}

func TestConvertToSeedanceOfficialTaskNormalizesCancelledStatus(t *testing.T) {
	task := &model.Task{
		TaskID: "task_cancelled", Status: model.TaskStatusFailure,
		Properties: model.Properties{OriginModelName: "AP Seedance"},
		Data:       json.RawMessage(`{"id":"cgt-cancelled","status":"canceled","error":{"code":"cancelled"}}`),
	}
	data, err := (&TaskAdaptor{}).ConvertToSeedanceOfficialTask(task)
	require.NoError(t, err)
	var response map[string]any
	require.NoError(t, common.Unmarshal(data, &response))
	require.Equal(t, "cancelled", response["status"])
}

func TestParseTaskResultIgnoresEquivalentUsageForBilling(t *testing.T) {
	info, err := (&TaskAdaptor{}).ParseTaskResult([]byte(`{
		"id":"task-usage","model":"AP Seedance-2.0 标准版","status":"succeeded",
		"duration":5,"usage":{"completion_tokens":1600000,"total_tokens":1600000},
		"content":{"video_url":"https://cdn.example.com/out.mp4"}
	}`))
	require.NoError(t, err)
	require.Equal(t, model.TaskStatusSuccess, info.Status)
	require.Zero(t, info.CompletionTokens)
	require.Zero(t, info.TotalTokens)
	require.Equal(t, int64(1_600_000), info.EquivalentUsageCompletionTokens)
	require.Equal(t, int64(1_600_000), info.EquivalentUsageTotalTokens)
}

func TestParseTaskResultNeverUsesSeedanceUsageForBillingAcrossStatuses(t *testing.T) {
	tests := []struct {
		status string
		want   model.TaskStatus
	}{
		{status: "queued", want: model.TaskStatusQueued},
		{status: "running", want: model.TaskStatusInProgress},
		{status: "succeeded", want: model.TaskStatusSuccess},
		{status: "failed", want: model.TaskStatusFailure},
		{status: "cancelled", want: model.TaskStatusFailure},
	}

	for _, test := range tests {
		t.Run(test.status, func(t *testing.T) {
			body := `{"id":"task-status","model":"AP Seedance-2.0 标准版","status":"` + test.status +
				`","duration":4,"usage":{"completion_tokens":640000,"total_tokens":640000},` +
				`"content":{"video_url":"https://cdn.example.com/out.mp4"}}`
			info, err := (&TaskAdaptor{}).ParseTaskResult([]byte(body))
			require.NoError(t, err)
			require.Equal(t, test.want, model.TaskStatus(info.Status))
			require.Zero(t, info.CompletionTokens)
			require.Zero(t, info.TotalTokens)
			if test.want == model.TaskStatusSuccess {
				require.Equal(t, int64(640000), info.EquivalentUsageCompletionTokens)
				require.Equal(t, int64(640000), info.EquivalentUsageTotalTokens)
			} else {
				require.Zero(t, info.EquivalentUsageCompletionTokens)
				require.Zero(t, info.EquivalentUsageTotalTokens)
			}
		})
	}
}

func mapKeys(value map[string]any) []string {
	keys := make([]string, 0, len(value))
	for key := range value {
		keys = append(keys, key)
	}
	return keys
}

func TestSeedanceOfficialPublicPathRejectsNonOfficialCapability(t *testing.T) {
	const modelName = "non-official-video"
	constant.SetAIPDDCapabilities([]constant.AIPDDCapability{{
		ModelName: modelName, EndpointType: constant.EndpointTypeOpenAIVideo,
		ExecutionProtocol: "legacy", ExecutionPath: "/shared-tasks/tasks",
	}})
	t.Cleanup(constant.ResetAIPDDCapabilities)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/v3/contents/generations/tasks", strings.NewReader(`{"model":"non-official-video"}`))
	ctx.Request.Header.Set("Content-Type", "application/json")
	info := relayInfoWithModel(modelName)
	info.OriginModelName = modelName

	taskErr := (&TaskAdaptor{}).ValidateRequestAndSetAction(ctx, info)
	require.NotNil(t, taskErr)
	require.Equal(t, "invalid_endpoint", taskErr.Code)
	require.Equal(t, http.StatusBadRequest, taskErr.StatusCode)
}

func TestNormalizeSeedanceOfficialPayloadUsesSecondsAlias(t *testing.T) {
	cfg := modelConfig{ModelName: "AP Seedance-2.5 标准版", AdapterCode: "seedance", ExecutionProtocol: "seedance_official"}
	payload, code, err := normalizeAndValidateSeedanceOfficialPayload(map[string]any{
		"model":      cfg.ModelName,
		"prompt":     "camera push in",
		"resolution": "720p",
		"ratio":      "16:9",
		"seconds":    "6",
	}, cfg)

	require.NoError(t, err)
	require.Empty(t, code)
	require.Equal(t, 6, payload["duration"])
	require.NotContains(t, payload, "seconds")
}

func TestNormalizeSeedanceOfficialPayloadPrefersTopLevelSecondsOverMetadataDuration(t *testing.T) {
	cfg := modelConfig{ModelName: "AP Seedance-2.5 标准版", AdapterCode: "seedance", ExecutionProtocol: "seedance_official"}
	payload, code, err := normalizeAndValidateSeedanceOfficialPayload(map[string]any{
		"model":      cfg.ModelName,
		"prompt":     "camera push in",
		"resolution": "720p",
		"ratio":      "16:9",
		"seconds":    7.0,
		"metadata":   map[string]any{"duration": 9.0},
	}, cfg)

	require.NoError(t, err)
	require.Empty(t, code)
	require.Equal(t, 7, payload["duration"])
	require.NotContains(t, payload, "seconds")
}

func TestConvertToOpenAIVideoNormalizesSeedanceOfficialFailure(t *testing.T) {
	task := &model.Task{
		TaskID:   "task_seedance_failure",
		Status:   model.TaskStatusFailure,
		Progress: "100%",
		Properties: model.Properties{
			OriginModelName: "AP Seedance-2.0 标准版",
		},
		Data: json.RawMessage(`{
			"id":"upstream-seedance-task",
			"status":"failed",
			"error":{"code":"content_policy_violation","message":"The reference media violates the content policy."}
		}`),
	}

	data, err := (&TaskAdaptor{}).ConvertToOpenAIVideo(task)
	if err != nil {
		t.Fatalf("ConvertToOpenAIVideo returned error: %v", err)
	}

	var response dto.OpenAIVideo
	if err := common.Unmarshal(data, &response); err != nil {
		t.Fatalf("decode converted response: %v", err)
	}
	if response.Error == nil {
		t.Fatal("expected official Seedance error to be present")
	}
	require.Equal(t, relaycommon.AIPDDErrorCodeContentPolicy, response.Error.Code)
	require.Equal(t, "aipdd", response.Error.Provider)
	require.Equal(t, "content_safety", response.Error.Category)
	require.Equal(t, "content_policy_violation", response.Error.UpstreamCode)
	require.NotNil(t, response.Error.Retryable)
	require.False(t, *response.Error.Retryable)
	require.Contains(t, response.Error.Message, "内容安全审核")
	require.NotContains(t, response.Error.Message, "reference media")
}

func TestConvertToOpenAIVideoLocalizesSeedanceCopyrightFailure(t *testing.T) {
	task := &model.Task{
		TaskID:   "task_seedance_copyright_failure",
		Status:   model.TaskStatusFailure,
		Progress: "100%",
		Properties: model.Properties{
			OriginModelName: "AP Seedance-2.0 标准版",
		},
		Data: json.RawMessage(`{
			"id":"upstream-seedance-task",
			"status":"failed",
			"error":{
				"code":"InputImageSensitiveContentDetected.PolicyViolation",
				"message":"The request failed because the input image 'content[1]' may be related to copyright restrictions. Request id: req-copyright-1"
			}
		}`),
	}

	data, err := (&TaskAdaptor{}).ConvertToOpenAIVideo(task)
	if err != nil {
		t.Fatalf("ConvertToOpenAIVideo returned error: %v", err)
	}

	var response dto.OpenAIVideo
	if err := common.Unmarshal(data, &response); err != nil {
		t.Fatalf("decode converted response: %v", err)
	}
	require.NotNil(t, response.Error)
	require.Equal(t, "InputImageSensitiveContentDetected.PolicyViolation", response.Error.Code)
	require.Equal(t, "content[1]", response.Error.Param)
	require.Equal(t, "req-copyright-1", response.Error.RequestID)
	require.Equal(t, "aipdd", response.Error.Provider)
	require.Equal(t, "copyright_policy", response.Error.Category)
	require.Empty(t, response.Error.UpstreamCode)
	require.NotNil(t, response.Error.Retryable)
	require.False(t, *response.Error.Retryable)
	require.Contains(t, response.Error.Message, "第 2 个输入内容中的图片")
	require.Contains(t, response.Error.Message, "版权限制")
	require.NotContains(t, response.Error.Message, "The request failed")
}

func TestConvertToOpenAIVideoMasksSuperResolutionFailure(t *testing.T) {
	task := &model.Task{
		TaskID:   "task_seedance_upscale_failure",
		Status:   model.TaskStatusFailure,
		Progress: "100%",
		Properties: model.Properties{
			OriginModelName: "AP Seedance-2.0 标准版",
		},
		Data: json.RawMessage(`{
			"id":"upstream-seedance-task",
			"status":"failed",
			"error":{
				"code":"SeedVR2UpscaleFailed",
				"message":"seedvr2-upscale worker failed while loading the super-resolution model. Request id: req-internal-1"
			}
		}`),
	}

	data, err := (&TaskAdaptor{}).ConvertToOpenAIVideo(task)
	if err != nil {
		t.Fatalf("ConvertToOpenAIVideo returned error: %v", err)
	}

	var response dto.OpenAIVideo
	if err := common.Unmarshal(data, &response); err != nil {
		t.Fatalf("decode converted response: %v", err)
	}
	require.NotNil(t, response.Error)
	require.Equal(t, relaycommon.PublicTaskProcessingFailedCode, response.Error.Code)
	require.Equal(t, "req-internal-1", response.Error.RequestID)
	require.Equal(t, "aipdd", response.Error.Provider)
	require.Equal(t, "task_processing", response.Error.Category)
	require.Empty(t, response.Error.UpstreamCode)
	require.NotNil(t, response.Error.Retryable)
	require.True(t, *response.Error.Retryable)
	require.NotContains(t, strings.ToLower(response.Error.Message), "seedvr")
	require.NotContains(t, strings.ToLower(response.Error.Message), "upscale")
	require.NotContains(t, response.Error.Message, "超分")
}

func TestConvertToRequestPayloadBuildsIndexTTSContent(t *testing.T) {
	adaptor := &TaskAdaptor{}
	req := relaycommon.TaskSubmitReq{
		Prompt: "hello from new-api",
		Model:  ModelIndexTTS,
		Metadata: map[string]interface{}{
			"audio":       "https://cdn.example.com/reference.wav",
			"task_name":   "voice clone",
			"extra_key":   "ignored",
			"script_id":   "ignored",
			"script_code": "ignored",
		},
	}

	payload, err := adaptor.convertToRequestPayload(req, relayInfoWithModel(ModelIndexTTS))
	if err != nil {
		t.Fatalf("convertToRequestPayload returned error: %v", err)
	}
	if payload.TaskTypeCode != "aipdd_IndexTTS" {
		t.Fatalf("unexpected task type code: %s", payload.TaskTypeCode)
	}
	if payload.TaskName != "voice clone" {
		t.Fatalf("unexpected task name: %s", payload.TaskName)
	}

	content := payload.Input
	if content["audio"] != "https://cdn.example.com/reference.wav" {
		t.Fatalf("audio was not forwarded: %#v", content)
	}
	if content["text"] != "hello from new-api" {
		t.Fatalf("text did not fall back to prompt: %#v", content)
	}
	if _, ok := content["extra_key"]; ok {
		t.Fatalf("unexpected extra workflow key forwarded: %#v", content)
	}
}

func TestConvertToRequestPayloadUsesUnifiedFinanceOrderID(t *testing.T) {
	adaptor := &TaskAdaptor{}
	info := relayInfoWithModel(ModelIndexTTS)
	info.PublicTaskID = "task_public"
	info.AIPDDFinance = &relaycommon.AIPDDFinanceContext{PlatformOrderID: "finance-order-1"}

	payload, err := adaptor.convertToRequestPayload(relaycommon.TaskSubmitReq{
		Prompt: "hello",
		Model:  ModelIndexTTS,
		Metadata: map[string]interface{}{
			"audio": "https://cdn.example.com/reference.wav",
		},
	}, info)

	if err != nil {
		t.Fatalf("convertToRequestPayload returned error: %v", err)
	}
	if payload.RequestID != "finance-order-1" {
		t.Fatalf("unexpected request id: %s", payload.RequestID)
	}
}

func TestConvertToRequestPayloadDoesNotForwardFilenameForVideoModels(t *testing.T) {
	adaptor := &TaskAdaptor{}
	tests := []relaycommon.TaskSubmitReq{
		{
			Model:  ModelWan22Animater,
			Prompt: "replace subject",
			Metadata: map[string]interface{}{
				"video":           "https://cdn.example.com/uploads/input-video.mp4?x=1",
				"negative_prompt": "low quality",
				"filename":        "input-video.mp4",
			},
		},
		{
			Model: ModelLatentsync15,
			Metadata: map[string]interface{}{
				"video":     "https://cdn.example.com/uploads/input-video.mp4?x=1",
				"LoadAudio": "https://cdn.example.com/uploads/input-audio.wav",
				"filename":  "input-video.mp4",
			},
		},
	}

	for _, req := range tests {
		t.Run(req.Model, func(t *testing.T) {
			payload, err := adaptor.convertToRequestPayload(req, relayInfoWithModel(req.Model))
			if err != nil {
				t.Fatalf("convertToRequestPayload returned error: %v", err)
			}

			content := payload.Input
			if _, ok := content["filename"]; ok {
				t.Fatalf("filename should not be forwarded: %#v", content)
			}
		})
	}
}

func TestConvertToRequestPayloadForAllAIPDDModels(t *testing.T) {
	adaptor := &TaskAdaptor{}
	tests := []struct {
		name       string
		req        relaycommon.TaskSubmitReq
		wantCode   string
		wantFields map[string]string
	}{
		{
			name:     ModelFluxGGUF,
			wantCode: "FLUX-GGUF-V2",
			req: relaycommon.TaskSubmitReq{
				Model:  ModelFluxGGUF,
				Prompt: "a cinematic robot",
				Image:  "https://cdn.example.com/input.png",
			},
			wantFields: map[string]string{"image": "https://cdn.example.com/input.png", "positive_prompt": "a cinematic robot"},
		},
		{
			name:     ModelFluxGGUFT2I,
			wantCode: "FLUX-GGUF-T2I-V2",
			req: relaycommon.TaskSubmitReq{
				Model:  ModelFluxGGUFT2I,
				Prompt: "a cinematic robot",
			},
			wantFields: map[string]string{"text": "a cinematic robot"},
		},
		{
			name:     ModelWan22Wanx,
			wantCode: "aipdd_wan2.2_wanx",
			req: relaycommon.TaskSubmitReq{
				Model:    ModelWan22Wanx,
				Prompt:   "camera push in",
				Image:    "https://cdn.example.com/input.png",
				Duration: 10,
			},
			wantFields: map[string]string{"image": "https://cdn.example.com/input.png", "prompt": "camera push in"},
		},
		{
			name:     ModelWan22Animater,
			wantCode: "aipdd_Wan2.2-Animater",
			req: relaycommon.TaskSubmitReq{
				Model:  ModelWan22Animater,
				Prompt: "replace subject",
				Metadata: map[string]interface{}{
					"video":           "https://cdn.example.com/subject.mp4",
					"negative_prompt": "low quality",
				},
			},
			wantFields: map[string]string{"video": "https://cdn.example.com/subject.mp4", "positive_prompt": "replace subject"},
		},
		{
			name:     ModelMimicMotion,
			wantCode: "aipdd_mimic_motion",
			req: relaycommon.TaskSubmitReq{
				Model: ModelMimicMotion,
				Metadata: map[string]interface{}{
					"motion_video":     "https://cdn.example.com/motion.mp4",
					"appearance_image": "https://cdn.example.com/person.png",
				},
			},
			wantFields: map[string]string{"motion_video": "https://cdn.example.com/motion.mp4", "appearance_image": "https://cdn.example.com/person.png"},
		},
		{
			name:     ModelLatentsync15,
			wantCode: "aipdd_latentsync1.5",
			req: relaycommon.TaskSubmitReq{
				Model: ModelLatentsync15,
				Metadata: map[string]interface{}{
					"video":     "https://cdn.example.com/lips.mp4",
					"LoadAudio": "https://cdn.example.com/speech.wav",
				},
			},
			wantFields: map[string]string{"video": "https://cdn.example.com/lips.mp4", "LoadAudio": "https://cdn.example.com/speech.wav"},
		},
		{
			name:     ModelIndexTTS,
			wantCode: "aipdd_IndexTTS",
			req: relaycommon.TaskSubmitReq{
				Model: ModelIndexTTS,
				Metadata: map[string]interface{}{
					"input":     "hello",
					"ref_audio": "https://cdn.example.com/ref.wav",
				},
			},
			wantFields: map[string]string{"audio": "https://cdn.example.com/ref.wav", "text": "hello"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload, err := adaptor.convertToRequestPayload(tt.req, relayInfoWithModel(tt.req.Model))
			if err != nil {
				t.Fatalf("convertToRequestPayload returned error: %v", err)
			}
			if payload.TaskTypeCode != tt.wantCode {
				t.Fatalf("unexpected task type code: %s", payload.TaskTypeCode)
			}
			content := payload.Input
			for key, want := range tt.wantFields {
				if got := anyToString(content[key]); got != want {
					t.Fatalf("unexpected %s: got %q want %q in %#v", key, got, want, content)
				}
			}
		})
	}
}

func TestWan22WanxDoesNotSendDurationToJavaBackend(t *testing.T) {
	adaptor := &TaskAdaptor{}
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Set("task_request", relaycommon.TaskSubmitReq{
		Model:    ModelWan22Wanx,
		Duration: 5,
	})
	ratios := adaptor.EstimateBilling(ctx, relayInfoWithModel(ModelWan22Wanx))
	if ratios != nil {
		t.Fatalf("wanx should not provide duration billing ratios for Java backend: %#v", ratios)
	}

	payload, err := adaptor.convertToRequestPayload(relaycommon.TaskSubmitReq{
		Model:    ModelWan22Wanx,
		Prompt:   "camera push in",
		Image:    "https://cdn.example.com/input.png",
		Duration: 10,
	}, relayInfoWithModel(ModelWan22Wanx))
	if err != nil {
		t.Fatalf("convertToRequestPayload returned error: %v", err)
	}
	if _, ok := payload.Input["duration"]; ok {
		t.Fatalf("duration should not be sent to Java backend input: %#v", payload.Input)
	}
}

func TestConvertToRequestPayloadMapsOpenAIImageCountToDynamicBatchParam(t *testing.T) {
	original := constant.GetAIPDDCapabilities()
	t.Cleanup(func() {
		constant.SetAIPDDCapabilities(original)
	})

	modelName := "aipdd-dynamic-image-batch"
	constant.SetAIPDDCapabilities([]constant.AIPDDCapability{
		{
			ModelName:         modelName,
			ScriptCode:        "dynamic_image_batch",
			EndpointType:      constant.EndpointTypeImageGeneration,
			BillingType:       constant.AIPDDBillingTypePerCall,
			WorkflowParamKeys: []string{"prompt", "batch_size"},
			RequiredWorkflowParams: map[string]bool{
				"prompt":     true,
				"batch_size": false,
			},
			WorkflowDefaults: []constant.AIPDDWorkflowParamDefault{
				{ParamKey: "prompt", ValueType: constant.AIPDDWorkflowValueTypeString, Sources: []constant.AIPDDWorkflowValueSource{{Type: constant.AIPDDWorkflowSourcePrompt}}},
				{ParamKey: "batch_size", ValueType: constant.AIPDDWorkflowValueTypeInt, Sources: []constant.AIPDDWorkflowValueSource{{Type: constant.AIPDDWorkflowSourceMetadata, Key: "n"}}},
			},
		},
	})

	adaptor := &TaskAdaptor{}
	count := 4
	req := relaycommon.TaskSubmitReq{
		Model:  modelName,
		Prompt: "a cinematic robot",
		N:      &count,
	}

	payload, err := adaptor.convertToRequestPayload(req, relayInfoWithModel(modelName))
	if err != nil {
		t.Fatalf("convertToRequestPayload returned error: %v", err)
	}
	if payload.Input["batch_size"] != 4 {
		t.Fatalf("n should map to batch_size: %#v", payload.Input)
	}

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Set("task_request", req)
	ratios := adaptor.EstimateBilling(ctx, relayInfoWithModel(modelName))
	if ratios["n"] != 4 {
		t.Fatalf("image count should be applied to billing ratios: %#v", ratios)
	}
}

func TestConvertToRequestPayloadPreservesPromptWhenCatalogDefaultIsMissing(t *testing.T) {
	original := constant.GetAIPDDCapabilities()
	t.Cleanup(func() { constant.SetAIPDDCapabilities(original) })
	modelName := "aipdd-ltx-2-3-stale-catalog"
	constant.SetAIPDDCapabilities([]constant.AIPDDCapability{{
		ModelName:              modelName,
		ScriptCode:             modelName,
		EndpointType:           constant.EndpointTypeOpenAIVideo,
		BillingType:            constant.AIPDDBillingTypePerCall,
		WorkflowParamKeys:      []string{"prompt", "image"},
		RequiredWorkflowParams: map[string]bool{"prompt": true, "image": false},
	}})

	payload, err := (&TaskAdaptor{}).convertToRequestPayload(relaycommon.TaskSubmitReq{
		Model:  modelName,
		Prompt: "下雪",
	}, relayInfoWithModel(modelName))
	if err != nil {
		t.Fatalf("convertToRequestPayload returned error: %v", err)
	}
	if got := anyToString(payload.Input["prompt"]); got != "下雪" {
		t.Fatalf("prompt was not preserved in upstream input: %#v", payload.Input)
	}
}

func TestConvertToRequestPayloadAppliesDynamicLTXDefaults(t *testing.T) {
	original := constant.GetAIPDDCapabilities()
	t.Cleanup(func() {
		constant.SetAIPDDCapabilities(original)
	})
	modelName := "aipdd_ltx2_3_distilled_fp8_ti2v"
	constant.SetAIPDDCapabilities([]constant.AIPDDCapability{
		{
			ModelName:         modelName,
			ScriptCode:        modelName,
			TaskKind:          "image_to_video",
			InputModalities:   []string{"text", "image"},
			OutputModalities:  []string{"video"},
			EndpointType:      constant.EndpointTypeOpenAIVideo,
			BillingType:       constant.AIPDDBillingTypePerCall,
			WorkflowParamKeys: []string{"prompt", "image", "negativePrompt", "width", "height", "numFrames", "frameRate"},
			RequiredWorkflowParams: map[string]bool{
				"prompt":         true,
				"image":          false,
				"negativePrompt": false,
				"width":          true,
				"height":         true,
				"numFrames":      true,
				"frameRate":      false,
			},
			WorkflowDefaults: []constant.AIPDDWorkflowParamDefault{
				{ParamKey: "prompt", ValueType: constant.AIPDDWorkflowValueTypeString, Sources: []constant.AIPDDWorkflowValueSource{{Type: constant.AIPDDWorkflowSourcePrompt}}},
				{ParamKey: "image", ValueType: constant.AIPDDWorkflowValueTypeString, Sources: []constant.AIPDDWorkflowValueSource{{Type: constant.AIPDDWorkflowSourceImage}}},
				{ParamKey: "negativePrompt", ValueType: constant.AIPDDWorkflowValueTypeString, Sources: []constant.AIPDDWorkflowValueSource{{Type: constant.AIPDDWorkflowSourceMetadata, Key: "negative_prompt"}}},
				{ParamKey: "width", ValueType: constant.AIPDDWorkflowValueTypeInt, Sources: []constant.AIPDDWorkflowValueSource{{Type: constant.AIPDDWorkflowSourceStatic, Key: "1920"}}},
				{ParamKey: "height", ValueType: constant.AIPDDWorkflowValueTypeInt, Sources: []constant.AIPDDWorkflowValueSource{{Type: constant.AIPDDWorkflowSourceStatic, Key: "1088"}}},
				{ParamKey: "numFrames", ValueType: constant.AIPDDWorkflowValueTypeInt, Sources: []constant.AIPDDWorkflowValueSource{{Type: constant.AIPDDWorkflowSourceStatic, Key: "121"}}},
				{ParamKey: "frameRate", ValueType: constant.AIPDDWorkflowValueTypeInt, Sources: []constant.AIPDDWorkflowValueSource{{Type: constant.AIPDDWorkflowSourceMetadata, Key: "fps"}, {Type: constant.AIPDDWorkflowSourceStatic, Key: "24"}}},
			},
		},
	})

	adaptor := &TaskAdaptor{}
	payload, err := adaptor.convertToRequestPayload(relaycommon.TaskSubmitReq{
		Model:  modelName,
		Prompt: "camera push in",
		Image:  "https://cdn.example.com/input.png",
		Metadata: map[string]interface{}{
			"negative_prompt": "low quality",
			"fps":             "30",
		},
	}, relayInfoWithModel(modelName))
	if err != nil {
		t.Fatalf("convertToRequestPayload returned error: %v", err)
	}
	content := payload.Input
	if payload.TaskTypeCode != modelName {
		t.Fatalf("unexpected task type code: %s", payload.TaskTypeCode)
	}
	if content["prompt"] != "camera push in" || content["image"] != "https://cdn.example.com/input.png" {
		t.Fatalf("prompt/image defaults were not applied: %#v", content)
	}
	if content["negativePrompt"] != "low quality" {
		t.Fatalf("negativePrompt should use negative_prompt metadata: %#v", content)
	}
	if content["width"] != 1920 || content["height"] != 1088 || content["numFrames"] != 121 || content["frameRate"] != 30 {
		t.Fatalf("LTX numeric defaults were not applied: %#v", content)
	}
}

func TestConvertToRequestPayloadValidatesLTX23Policy(t *testing.T) {
	original := constant.GetAIPDDCapabilities()
	t.Cleanup(func() { constant.SetAIPDDCapabilities(original) })
	modelName := "aipdd_ltx_2.3"
	constant.SetAIPDDCapabilities([]constant.AIPDDCapability{{
		ModelName:              modelName,
		ScriptCode:             modelName,
		EndpointType:           constant.EndpointTypeOpenAIVideo,
		BillingType:            constant.AIPDDBillingTypeDurationSeconds,
		WorkflowParamKeys:      []string{"prompt", "image", "negativePrompt", "width", "height", "numFrames", "frameRate", "seed"},
		RequiredWorkflowParams: map[string]bool{"prompt": true, "image": true, "negativePrompt": false, "width": false, "height": false, "numFrames": true, "frameRate": false, "seed": false},
		WorkflowDefaults: []constant.AIPDDWorkflowParamDefault{
			{ParamKey: "prompt", ValueType: constant.AIPDDWorkflowValueTypeString, Sources: []constant.AIPDDWorkflowValueSource{{Type: constant.AIPDDWorkflowSourcePrompt}}},
			{ParamKey: "image", ValueType: constant.AIPDDWorkflowValueTypeString, Sources: []constant.AIPDDWorkflowValueSource{{Type: constant.AIPDDWorkflowSourceImage}}},
			{ParamKey: "negativePrompt", ValueType: constant.AIPDDWorkflowValueTypeString, Sources: []constant.AIPDDWorkflowValueSource{{Type: constant.AIPDDWorkflowSourceMetadata, Key: "negative_prompt"}}},
			{ParamKey: "width", ValueType: constant.AIPDDWorkflowValueTypeInt, Sources: []constant.AIPDDWorkflowValueSource{{Type: constant.AIPDDWorkflowSourceStatic, Key: "640"}}},
			{ParamKey: "height", ValueType: constant.AIPDDWorkflowValueTypeInt, Sources: []constant.AIPDDWorkflowValueSource{{Type: constant.AIPDDWorkflowSourceStatic, Key: "640"}}},
			{ParamKey: "numFrames", ValueType: constant.AIPDDWorkflowValueTypeInt, Sources: []constant.AIPDDWorkflowValueSource{{Type: constant.AIPDDWorkflowSourceStatic, Key: "49"}}},
			{ParamKey: "frameRate", ValueType: constant.AIPDDWorkflowValueTypeInt, Sources: []constant.AIPDDWorkflowValueSource{{Type: constant.AIPDDWorkflowSourceStatic, Key: "24"}}},
			{ParamKey: "seed", ValueType: constant.AIPDDWorkflowValueTypeInt, Sources: []constant.AIPDDWorkflowValueSource{{Type: constant.AIPDDWorkflowSourceMetadata, Key: "seed"}}},
		},
	}})

	adaptor := &TaskAdaptor{}
	valid, err := adaptor.convertToRequestPayload(relaycommon.TaskSubmitReq{
		Model: modelName, Prompt: "camera push in", Image: "https://cdn.example.com/input.png", Duration: 20,
		Metadata: map[string]interface{}{"width": 704, "height": 1280},
	}, relayInfoWithModel(modelName))
	if err != nil {
		t.Fatalf("valid LTX request failed: %v", err)
	}
	if valid.Input["numFrames"] != 481 || valid.Input["frameRate"] != 24 {
		t.Fatalf("unexpected LTX timing: %#v", valid.Input)
	}
	if _, ok := valid.Input["durationSeconds"]; ok {
		t.Fatalf("durationSeconds is not part of the upstream schema: %#v", valid.Input)
	}

	withDefaults, err := adaptor.convertToRequestPayload(relaycommon.TaskSubmitReq{
		Model: modelName, Prompt: "camera push in", Image: "https://cdn.example.com/input.png",
	}, relayInfoWithModel(modelName))
	if err != nil {
		t.Fatalf("default LTX request failed: %v", err)
	}
	if withDefaults.Input["width"] != 640 || withDefaults.Input["height"] != 640 || withDefaults.Input["numFrames"] != 49 || withDefaults.Input["frameRate"] != 24 {
		t.Fatalf("unexpected default LTX input: %#v", withDefaults.Input)
	}

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Set("task_request", relaycommon.TaskSubmitReq{
		Model: modelName, Prompt: "camera push in", Image: "https://cdn.example.com/input.png", Duration: 20,
		Metadata: map[string]interface{}{"width": 704, "height": 1280},
	})
	if ratios := adaptor.EstimateBilling(ctx, relayInfoWithModel(modelName)); ratios["seconds"] != 20 {
		t.Fatalf("duration-priced LTX must bill generated seconds: %#v", ratios)
	}
	facts, taskErr := adaptor.EstimateTaskPricingFacts(ctx, relayInfoWithModel(modelName))
	if taskErr != nil || facts.Quantity != 20 || facts.Resolution != "" || facts.HasReferenceVideo {
		t.Fatalf("unexpected explicit-duration LTX pricing facts: facts=%#v err=%#v", facts, taskErr)
	}

	defaultCtx, _ := gin.CreateTestContext(httptest.NewRecorder())
	defaultCtx.Set("task_request", relaycommon.TaskSubmitReq{
		Model: modelName, Prompt: "camera push in", Image: "https://cdn.example.com/input.png",
	})
	defaultFacts, taskErr := adaptor.EstimateTaskPricingFacts(defaultCtx, relayInfoWithModel(modelName))
	if taskErr != nil || defaultFacts.Quantity != 2 {
		t.Fatalf("default 49-frame LTX request must bill 2 seconds: facts=%#v err=%#v", defaultFacts, taskErr)
	}

	cases := []struct {
		name     string
		duration int
		metadata map[string]interface{}
	}{
		{name: "duration", duration: 21, metadata: map[string]interface{}{"width": 1280, "height": 704}},
		{name: "resolution", duration: 5, metadata: map[string]interface{}{"width": 1280, "height": 720}},
		{name: "fps", duration: 5, metadata: map[string]interface{}{"width": 1280, "height": 704, "frameRate": 25}},
		{name: "explicit zero fps", duration: 5, metadata: map[string]interface{}{"width": 1280, "height": 704, "frameRate": 0}},
		{name: "frames", duration: 5, metadata: map[string]interface{}{"width": 1280, "height": 704, "numFrames": 113}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := adaptor.convertToRequestPayload(relaycommon.TaskSubmitReq{
				Model: modelName, Prompt: "camera push in", Image: "https://cdn.example.com/input.png", Duration: tc.duration, Metadata: tc.metadata,
			}, relayInfoWithModel(modelName))
			if err == nil {
				t.Fatal("expected invalid LTX request to fail")
			}
		})
	}
}

func TestConvertToRequestPayloadMapsLTX23FirstAndLastFramesSeparately(t *testing.T) {
	original := constant.GetAIPDDCapabilities()
	t.Cleanup(func() { constant.SetAIPDDCapabilities(original) })
	modelName := "aipdd_ltx_2.3 (首尾帧)"
	constant.SetAIPDDCapabilities([]constant.AIPDDCapability{{
		ModelName:              modelName,
		ScriptCode:             modelName,
		EndpointType:           constant.EndpointTypeOpenAIVideo,
		BillingType:            constant.AIPDDBillingTypeDurationSeconds,
		WorkflowParamKeys:      []string{"first_frame_image", "last_frame_image", "audio", "local_prompts", "timeline_data", "length", "global_prompt"},
		RequiredWorkflowParams: map[string]bool{"first_frame_image": true, "last_frame_image": true, "audio": false, "local_prompts": true, "timeline_data": true, "length": true, "global_prompt": true},
		WorkflowDefaults: []constant.AIPDDWorkflowParamDefault{
			{ParamKey: "first_frame_image", ValueType: constant.AIPDDWorkflowValueTypeString, Sources: []constant.AIPDDWorkflowValueSource{{Type: constant.AIPDDWorkflowSourceFirstImage}}},
			{ParamKey: "last_frame_image", ValueType: constant.AIPDDWorkflowValueTypeString, Sources: []constant.AIPDDWorkflowValueSource{{Type: constant.AIPDDWorkflowSourceLastImage}}},
			{ParamKey: "audio", ValueType: constant.AIPDDWorkflowValueTypeString, Sources: []constant.AIPDDWorkflowValueSource{{Type: constant.AIPDDWorkflowSourceMetadata, Key: "audio"}, {Type: constant.AIPDDWorkflowSourceMetadata, Key: "audio_url"}}},
			{ParamKey: "local_prompts", ValueType: constant.AIPDDWorkflowValueTypeString, Sources: []constant.AIPDDWorkflowValueSource{{Type: constant.AIPDDWorkflowSourcePrompt}}},
			{ParamKey: "timeline_data", ValueType: constant.AIPDDWorkflowValueTypeString, Sources: []constant.AIPDDWorkflowValueSource{{Type: constant.AIPDDWorkflowSourceMetadata, Key: "timeline_data"}}},
			{ParamKey: "length", ValueType: constant.AIPDDWorkflowValueTypeInt, Sources: []constant.AIPDDWorkflowValueSource{{Type: constant.AIPDDWorkflowSourceMetadata, Key: "length"}, {Type: constant.AIPDDWorkflowSourceMetadata, Key: "numFrames"}, {Type: constant.AIPDDWorkflowSourceDuration}}},
			{ParamKey: "global_prompt", ValueType: constant.AIPDDWorkflowValueTypeString, Sources: []constant.AIPDDWorkflowValueSource{{Type: constant.AIPDDWorkflowSourcePrompt}}},
		},
	}})

	adaptor := &TaskAdaptor{}
	payload, err := adaptor.convertToRequestPayload(relaycommon.TaskSubmitReq{
		Model: modelName, Prompt: "camera push in", Duration: 20,
		FirstFrame: "https://cdn.example.com/first.png",
		Images:     []string{"https://cdn.example.com/fallback-first.png", "https://cdn.example.com/last.png"},
		Metadata: map[string]interface{}{
			"audio_url":     "https://cdn.example.com/reference.wav",
			"timeline_data": `{"segments":[]}`,
		},
	}, relayInfoWithModel(modelName))
	if err != nil {
		t.Fatalf("valid first/last LTX request failed: %v", err)
	}
	if payload.Input["first_frame_image"] != "https://cdn.example.com/first.png" {
		t.Fatalf("first frame mapped incorrectly: %#v", payload.Input)
	}
	if payload.Input["last_frame_image"] != "https://cdn.example.com/last.png" {
		t.Fatalf("last frame mapped incorrectly: %#v", payload.Input)
	}
	if payload.Input["audio"] != "https://cdn.example.com/reference.wav" || payload.Input["local_prompts"] != "camera push in" || payload.Input["global_prompt"] != "camera push in" {
		t.Fatalf("unexpected first/last LTX references: %#v", payload.Input)
	}
	if payload.Input["length"] != 481 {
		t.Fatalf("unexpected first/last LTX length: %#v", payload.Input)
	}
	if timeline, ok := payload.Input["timeline_data"].(map[string]interface{}); !ok || len(timeline) != 1 {
		t.Fatalf("timeline_data should be decoded JSON: %#v", payload.Input)
	}
	for _, unsupported := range []string{"width", "height", "durationSeconds", "numFrames", "frameRate"} {
		if _, ok := payload.Input[unsupported]; ok {
			t.Fatalf("unsupported %s should not be sent: %#v", unsupported, payload.Input)
		}
	}

	lengthCases := []struct {
		name     string
		metadata map[string]interface{}
		want     int
	}{
		{name: "explicit length and object timeline", metadata: map[string]interface{}{"audio": "https://cdn.example.com/reference.wav", "timeline_data": map[string]interface{}{"segments": []interface{}{}}, "length": 321}, want: 321},
		{name: "numFrames and array timeline", metadata: map[string]interface{}{"audio": "https://cdn.example.com/reference.wav", "timeline_data": []interface{}{}, "numFrames": 241}, want: 241},
	}
	for _, tc := range lengthCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := adaptor.convertToRequestPayload(relaycommon.TaskSubmitReq{
				Model: modelName, Prompt: "camera push in", Duration: 20,
				Images:   []string{"https://cdn.example.com/first.png", "https://cdn.example.com/last.png"},
				Metadata: tc.metadata,
			}, relayInfoWithModel(modelName))
			if err != nil {
				t.Fatalf("valid length precedence request failed: %v", err)
			}
			if result.Input["length"] != tc.want {
				t.Fatalf("unexpected length: got %#v want %d", result.Input["length"], tc.want)
			}
		})
	}

	_, err = adaptor.convertToRequestPayload(relaycommon.TaskSubmitReq{
		Model: modelName, Prompt: "camera push in", Duration: 5,
		FirstFrame: "https://cdn.example.com/first.png",
		Metadata: map[string]interface{}{
			"audio":         "https://cdn.example.com/reference.wav",
			"timeline_data": `{"segments":[]}`,
		},
	}, relayInfoWithModel(modelName))
	if err == nil {
		t.Fatal("expected missing last frame validation error")
	}

	payloadWithoutAudio, err := adaptor.convertToRequestPayload(relaycommon.TaskSubmitReq{
		Model: modelName, Prompt: "camera push in", Duration: 5,
		Images: []string{"https://cdn.example.com/first.png", "https://cdn.example.com/last.png"},
		Metadata: map[string]interface{}{
			"timeline_data": `{"segments":[]}`,
		},
	}, relayInfoWithModel(modelName))
	if err != nil {
		t.Fatalf("audio should be optional for first/last LTX requests: %v", err)
	}
	if _, ok := payloadWithoutAudio.Input["audio"]; ok {
		t.Fatalf("missing optional audio should not be sent: %#v", payloadWithoutAudio.Input)
	}

	_, err = adaptor.convertToRequestPayload(relaycommon.TaskSubmitReq{
		Model: modelName, Prompt: "camera push in", Duration: 5,
		Images: []string{"https://cdn.example.com/first.png", "https://cdn.example.com/last.png"},
		Metadata: map[string]interface{}{
			"audio": "https://cdn.example.com/reference.wav",
		},
	}, relayInfoWithModel(modelName))
	if err == nil || !strings.Contains(err.Error(), "timeline_data") {
		t.Fatalf("expected missing timeline_data validation error, got %v", err)
	}

	_, err = adaptor.convertToRequestPayload(relaycommon.TaskSubmitReq{
		Model: modelName, Prompt: "camera push in", Duration: 5,
		Images: []string{"https://cdn.example.com/first.png", "https://cdn.example.com/last.png"},
		Metadata: map[string]interface{}{
			"audio":         "https://cdn.example.com/reference.wav",
			"timeline_data": "not-json",
		},
	}, relayInfoWithModel(modelName))
	if err == nil || !strings.Contains(err.Error(), "timeline_data must be valid JSON") {
		t.Fatalf("expected invalid timeline_data error, got %v", err)
	}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/pg/video/generations", strings.NewReader(`{
		"model":"aipdd_ltx_2.3 (首尾帧)",
		"prompt":"camera push in",
		"first_frame":"https://cdn.example.com/first.png",
		"last_frame":"https://cdn.example.com/last.png",
		"audio":"https://cdn.example.com/reference.wav",
		"timeline_data":"not-json",
		"duration":5
	}`))
	ctx.Request.Header.Set("Content-Type", "application/json")
	taskErr := adaptor.ValidateRequestAndSetAction(ctx, relayInfoWithModel(modelName))
	if taskErr == nil || !taskErr.LocalError || taskErr.StatusCode != http.StatusBadRequest || !strings.Contains(taskErr.Message, "timeline_data must be valid JSON") {
		t.Fatalf("expected local 400 timeline_data error, got %#v", taskErr)
	}
}

func TestWan22WanxIgnoresUnsupportedDurationForJavaBackend(t *testing.T) {
	adaptor := &TaskAdaptor{}
	payload, err := adaptor.convertToRequestPayload(relaycommon.TaskSubmitReq{
		Model:    ModelWan22Wanx,
		Prompt:   "camera push in",
		Image:    "https://cdn.example.com/input.png",
		Duration: 7,
	}, relayInfoWithModel(ModelWan22Wanx))
	if err != nil {
		t.Fatalf("convertToRequestPayload returned error: %v", err)
	}
	if _, ok := payload.Input["duration"]; ok {
		t.Fatalf("duration should not be forwarded: %#v", payload.Input)
	}
}

func TestFluxGGUFRequiresImageForJavaBackend(t *testing.T) {
	adaptor := &TaskAdaptor{}
	_, err := adaptor.convertToRequestPayload(relaycommon.TaskSubmitReq{
		Model:  ModelFluxGGUF,
		Prompt: "a cinematic robot",
	}, relayInfoWithModel(ModelFluxGGUF))
	if err == nil {
		t.Fatal("expected missing image validation error")
	}
}

func TestPerCallBillingCapabilities(t *testing.T) {
	if !constant.IsAIPDDPerCallBillingModel(ModelWan22Animater) {
		t.Fatal("subject replacement should be per-call billed")
	}
	if !constant.IsAIPDDPerCallBillingModel(ModelWan22Wanx) {
		t.Fatal("wanx image-to-video should be per-call billed for Java backend")
	}
}

func TestDoResponseReturnsAsyncTaskForIndexTTS(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	adaptor := &TaskAdaptor{}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(`{"code":200,"message":"ok","data":{"id":"upstream-task","task_status":0}}`)),
	}
	info := relayInfoWithModel(ModelIndexTTS)
	info.OriginModelName = ModelIndexTTS
	info.PublicTaskID = "task_public"

	taskID, _, taskErr := adaptor.DoResponse(ctx, resp, info)
	if taskErr != nil {
		t.Fatalf("DoResponse returned task error: %v", taskErr)
	}
	if taskID != "upstream-task" {
		t.Fatalf("unexpected upstream task id: %s", taskID)
	}
	var body map[string]any
	if err := common.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if body["task_id"] != "task_public" || body["object"] != "audio.speech.task" {
		t.Fatalf("unexpected async task response: %#v", body)
	}
}

func TestDoResponseParsesJavaCreateTaskResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	adaptor := &TaskAdaptor{}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(`{"code":0,"message":"created","data":{"id":"java-task","taskTypeCode":"aipdd_IndexTTS","status":"QUEUED"}}`)),
	}
	info := relayInfoWithModel(ModelIndexTTS)
	info.OriginModelName = ModelIndexTTS
	info.PublicTaskID = "task_public"

	taskID, _, taskErr := adaptor.DoResponse(ctx, resp, info)
	if taskErr != nil {
		t.Fatalf("DoResponse returned task error: %v", taskErr)
	}
	if taskID != "java-task" {
		t.Fatalf("unexpected upstream task id: %s", taskID)
	}
}

func TestDoResponseUsesSeedanceOfficialCreateShapeOnlyOnOfficialPublicPath(t *testing.T) {
	constant.SetAIPDDCapabilities([]constant.AIPDDCapability{seedanceTestCapability()})
	t.Cleanup(constant.ResetAIPDDCapabilities)

	tests := []struct {
		name           string
		path           string
		officialPublic bool
	}{
		{name: "official public", path: "/api/v3/contents/generations/tasks", officialPublic: true},
		{name: "openai video unchanged", path: "/v1/videos", officialPublic: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodPost, test.path, nil)
			info := relayInfoWithModel("AP Seedance")
			info.OriginModelName = "AP Seedance"
			info.PublicTaskID = "task_public"
			resp := &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"id":"cgt-upstream"}`)),
			}

			taskID, _, taskErr := (&TaskAdaptor{}).DoResponse(ctx, resp, info)
			require.Nil(t, taskErr)
			require.Equal(t, "cgt-upstream", taskID)
			var body map[string]any
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &body))
			require.Equal(t, "task_public", body["id"])
			if test.officialPublic {
				require.Len(t, body, 1)
			} else {
				require.Equal(t, "task_public", body["task_id"])
				require.Equal(t, "video", body["object"])
			}
		})
	}
}

func TestFetchTaskFollowsJavaResultEndpoint(t *testing.T) {
	var sawDetail bool
	var sawResult bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-Key") != "aipdd-key" {
			t.Fatalf("unexpected api key header: %q", r.Header.Get("X-API-Key"))
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/shared-tasks/tasks/java-task/detail":
			sawDetail = true
			_, _ = w.Write([]byte(`{"code":0,"message":"fetched","data":{"id":"java-task","taskTypeCode":"aipdd_wan2.2_wanx","status":"SUCCESS","progress":100,"resultReady":true}}`))
		case "/shared-tasks/tasks/java-task/result":
			sawResult = true
			_, _ = w.Write([]byte(`{"code":0,"message":"fetched","data":{"taskId":"java-task","status":"PENDING_CONFIRMATION","output":{"url":"https://oss.example.com/result.mp4"}}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	adaptor := &TaskAdaptor{}
	resp, err := adaptor.FetchTask(server.URL, "aipdd-key", map[string]any{"task_id": "java-task"}, "")
	if err != nil {
		t.Fatalf("FetchTask returned error: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	info, err := adaptor.ParseTaskResult(body)
	if err != nil {
		t.Fatalf("ParseTaskResult returned error: %v", err)
	}
	if !sawDetail || !sawResult {
		t.Fatalf("expected detail and result endpoints to be called, detail=%v result=%v", sawDetail, sawResult)
	}
	if info.Status != model.TaskStatusSuccess || info.Url != "https://oss.example.com/result.mp4" {
		t.Fatalf("unexpected task info: %+v body=%s", info, string(body))
	}
}

func relayInfoWithModel(modelName string) *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: modelName,
		},
		TaskRelayInfo: &relaycommon.TaskRelayInfo{},
	}
}

func TestParseTaskResultExtractsNonJSONURLArray(t *testing.T) {
	adaptor := &TaskAdaptor{}
	body := []byte(`{"code":200,"message":"获取成功","data":{"id":"task-1","task_status":2,"task_result":"[https://cdn.example.com/a.mp4,https://cdn.example.com/b.mp4]"}}`)

	info, err := adaptor.ParseTaskResult(body)
	if err != nil {
		t.Fatalf("ParseTaskResult returned error: %v", err)
	}
	if info.Status != model.TaskStatusSuccess {
		t.Fatalf("unexpected status: %s", info.Status)
	}
	if info.Url != "https://cdn.example.com/a.mp4" {
		t.Fatalf("unexpected result URL: %s", info.Url)
	}
}

func TestParseTaskResultTreatsSuccessFalseAsFailure(t *testing.T) {
	adaptor := &TaskAdaptor{}
	body := []byte(`{"code":200,"message":"获取成功","data":{"id":"task-1","task_status":3,"task_result":"{\"success\":false,\"message\":\"render failed\"}"}}`)

	info, err := adaptor.ParseTaskResult(body)
	if err != nil {
		t.Fatalf("ParseTaskResult returned error: %v", err)
	}
	if info.Status != model.TaskStatusFailure {
		t.Fatalf("unexpected status: %s", info.Status)
	}
	if info.Reason != "render failed" {
		t.Fatalf("unexpected reason: %s", info.Reason)
	}
}

func TestParseTaskResultTreatsStatusTwoURLResultAsSuccess(t *testing.T) {
	adaptor := &TaskAdaptor{}
	body := []byte(`{"code":200,"message":"获取成功","data":{"id":"task-1","task_status":2,"task_result":"https://oss.aipdd.work/distributed_compute/task-1/result.wav"}}`)

	info, err := adaptor.ParseTaskResult(body)
	if err != nil {
		t.Fatalf("ParseTaskResult returned error: %v", err)
	}
	if info.Status != model.TaskStatusSuccess {
		t.Fatalf("unexpected status: %s", info.Status)
	}
	if info.Url != "https://oss.aipdd.work/distributed_compute/task-1/result.wav" {
		t.Fatalf("unexpected result URL: %s", info.Url)
	}
}

func TestParseTaskResultTreatsStatusFourURLResultAsSuccess(t *testing.T) {
	adaptor := &TaskAdaptor{}
	body := []byte(`{"code":200,"message":"获取成功","data":{"id":"task-1","task_status":4,"task_result":"https://oss.aipdd.work/distributed_compute/task-1/result.mp4"}}`)

	info, err := adaptor.ParseTaskResult(body)
	if err != nil {
		t.Fatalf("ParseTaskResult returned error: %v", err)
	}
	if info.Status != model.TaskStatusSuccess {
		t.Fatalf("unexpected status: %s", info.Status)
	}
	if info.Url != "https://oss.aipdd.work/distributed_compute/task-1/result.mp4" {
		t.Fatalf("unexpected result URL: %s", info.Url)
	}
}

func TestParseTaskResultTreatsCompletedErrorTextAsFailure(t *testing.T) {
	adaptor := &TaskAdaptor{}
	body := []byte(`{"code":200,"message":"获取成功","data":{"id":"task-1","task_status":3,"task_result":"ComfyUI ??: prompt_outputs_failed_validation - Prompt outputs failed validation"}}`)

	info, err := adaptor.ParseTaskResult(body)
	if err != nil {
		t.Fatalf("ParseTaskResult returned error: %v", err)
	}
	if info.Status != model.TaskStatusFailure {
		t.Fatalf("unexpected status: %s", info.Status)
	}
	if info.Url != "" {
		t.Fatalf("error text should not be treated as URL: %s", info.Url)
	}
	if info.Reason != "ComfyUI ??: prompt_outputs_failed_validation - Prompt outputs failed validation" {
		t.Fatalf("unexpected reason: %s", info.Reason)
	}
}
