package doubao

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	rootcommon "github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestEstimateBillingIncludesDurationSeconds(t *testing.T) {
	gin.SetMode(gin.TestMode)
	adaptor := &TaskAdaptor{}

	tests := []struct {
		name string
		req  relaycommon.TaskSubmitReq
		want float64
	}{
		{
			name: "seconds field",
			req: relaycommon.TaskSubmitReq{
				Seconds: "10",
			},
			want: 10,
		},
		{
			name: "duration field",
			req: relaycommon.TaskSubmitReq{
				Duration: 6,
			},
			want: 6,
		},
		{
			name: "metadata duration",
			req: relaycommon.TaskSubmitReq{
				Metadata: map[string]interface{}{"duration": float64(8)},
			},
			want: 8,
		},
		{
			name: "default duration",
			req:  relaycommon.TaskSubmitReq{},
			want: defaultDurationSeconds,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Set("task_request", tt.req)

			ratios := adaptor.EstimateBilling(ctx, &relaycommon.RelayInfo{})
			if ratios["seconds"] != tt.want {
				t.Fatalf("seconds ratio = %v, want %v; ratios=%#v", ratios["seconds"], tt.want, ratios)
			}
		})
	}
}

func TestEstimateBillingUsesMappedModelForVideoInputDiscount(t *testing.T) {
	gin.SetMode(gin.TestMode)
	adaptor := &TaskAdaptor{}
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Set("task_request", relaycommon.TaskSubmitReq{
		Duration: 5,
		Metadata: map[string]interface{}{
			"content": []interface{}{
				map[string]interface{}{
					"type":      "video_url",
					"video_url": map[string]interface{}{"url": "https://example.com/input.mp4"},
				},
			},
		},
	})

	ratios := adaptor.EstimateBilling(ctx, &relaycommon.RelayInfo{
		OriginModelName: "doubao-seedance-2.0",
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "doubao-seedance-2-0-260128",
		},
	})
	want := 28.0 / 46.0
	if ratios["video_input"] != want {
		t.Fatalf("video_input ratio = %v, want %v; ratios=%#v", ratios["video_input"], want, ratios)
	}
}

func TestConvertToRequestPayloadForwardsTopLevelDuration(t *testing.T) {
	adaptor := &TaskAdaptor{}
	payload, err := adaptor.convertToRequestPayload(&relaycommon.TaskSubmitReq{
		Model:    "doubao-seedance-2.0",
		Prompt:   "cinematic shot",
		Duration: 15,
	})
	if err != nil {
		t.Fatalf("convertToRequestPayload returned error: %v", err)
	}
	if payload.Duration == nil || int(*payload.Duration) != 15 {
		t.Fatalf("duration = %v, want 15", payload.Duration)
	}
	if payload.Resolution != "720p" {
		t.Fatalf("resolution = %q, want 720p", payload.Resolution)
	}
}

func TestConvertToRequestPayloadPreservesExplicitResolution(t *testing.T) {
	adaptor := &TaskAdaptor{}
	payload, err := adaptor.convertToRequestPayload(&relaycommon.TaskSubmitReq{
		Model:  "doubao-seedance-2-0-260128",
		Prompt: "cinematic shot",
		Metadata: map[string]interface{}{
			"resolution": "480p",
		},
	})
	if err != nil {
		t.Fatalf("convertToRequestPayload returned error: %v", err)
	}
	if payload.Resolution != "480p" {
		t.Fatalf("resolution = %q, want 480p", payload.Resolution)
	}
}

func TestValidateSeedanceRequestAllowsMediaOnlyForSeedance20(t *testing.T) {
	err := validateSeedanceRequest(relaycommon.TaskSubmitReq{
		Model: "doubao-seedance-2-0-fast-260128",
		Metadata: map[string]interface{}{
			"content": []interface{}{
				map[string]interface{}{
					"type":      "image_url",
					"image_url": map[string]interface{}{"url": "data:image/png;base64,aaa"},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("validateSeedanceRequest returned error: %v", err)
	}
}

func TestValidateSeedanceRequestRejectsAudioOnly(t *testing.T) {
	err := validateSeedanceRequest(relaycommon.TaskSubmitReq{
		Model:  "doubao-seedance-2-0-fast-260128",
		Prompt: "match the beat",
		Metadata: map[string]interface{}{
			"content": []interface{}{
				map[string]interface{}{
					"type":      "audio_url",
					"audio_url": map[string]interface{}{"url": "data:audio/mp3;base64,aaa"},
				},
			},
		},
	})
	if err == nil {
		t.Fatalf("validateSeedanceRequest returned nil, want audio-only error")
	}
}

func TestValidateSeedanceRequestRejectsNonSeedanceMediaOnly(t *testing.T) {
	err := validateSeedanceRequest(relaycommon.TaskSubmitReq{
		Model: "doubao-seedance-1-0-pro-250528",
		Metadata: map[string]interface{}{
			"content": []interface{}{
				map[string]interface{}{
					"type":      "image_url",
					"image_url": map[string]interface{}{"url": "https://example.com/input.png"},
				},
			},
		},
	})
	if err == nil {
		t.Fatalf("validateSeedanceRequest returned nil, want prompt required error")
	}
}

func TestConvertToRequestPayloadOmitsEmptyTextContent(t *testing.T) {
	adaptor := &TaskAdaptor{}
	payload, err := adaptor.convertToRequestPayload(&relaycommon.TaskSubmitReq{
		Model: "doubao-seedance-2-0-fast-260128",
		Metadata: map[string]interface{}{
			"content": []interface{}{
				map[string]interface{}{
					"type":      "image_url",
					"image_url": map[string]interface{}{"url": "data:image/png;base64,aaa"},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("convertToRequestPayload returned error: %v", err)
	}
	for _, item := range payload.Content {
		if item.Type == "text" {
			t.Fatalf("payload contains empty text item: %#v", payload.Content)
		}
	}
}

func TestConvertToRequestPayloadPreservesReferenceRoles(t *testing.T) {
	adaptor := &TaskAdaptor{}
	payload, err := adaptor.convertToRequestPayload(&relaycommon.TaskSubmitReq{
		Model: "doubao-seedance-2-0-fast-260128",
		Metadata: map[string]interface{}{
			"content": []interface{}{
				map[string]interface{}{
					"type":      "image_url",
					"role":      "reference_image",
					"image_url": map[string]interface{}{"url": "https://example.com/reference.png"},
				},
				map[string]interface{}{
					"type":      "video_url",
					"role":      "reference_video",
					"video_url": map[string]interface{}{"url": "https://example.com/reference.mp4"},
				},
				map[string]interface{}{
					"type":      "audio_url",
					"role":      "reference_audio",
					"audio_url": map[string]interface{}{"url": "https://example.com/reference.mp3"},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("convertToRequestPayload returned error: %v", err)
	}
	if len(payload.Content) != 3 {
		t.Fatalf("content length = %d, want 3; content=%#v", len(payload.Content), payload.Content)
	}
	wantRoles := []string{"reference_image", "reference_video", "reference_audio"}
	for i, want := range wantRoles {
		if payload.Content[i].Role != want {
			t.Fatalf("content[%d].Role = %q, want %q; content=%#v", i, payload.Content[i].Role, want, payload.Content)
		}
	}
}

func TestTopLevelContentTextIsValidatedAndTakesPriorityOverPrompt(t *testing.T) {
	var req relaycommon.TaskSubmitReq
	err := rootcommon.Unmarshal([]byte(`{
        "model":"doubao-seedance-2-0-fast-260128",
        "prompt":"legacy prompt",
        "content":[
            {"type":"text","text":"official content"},
            {"type":"image_url","image_url":{"url":"https://example.com/reference.png"}}
        ]
    }`), &req)
	if err != nil {
		t.Fatalf("unmarshal request returned error: %v", err)
	}
	if err := validateSeedanceRequest(req); err != nil {
		t.Fatalf("validateSeedanceRequest returned error: %v", err)
	}

	payload, err := (&TaskAdaptor{}).convertToRequestPayload(&req)
	if err != nil {
		t.Fatalf("convertToRequestPayload returned error: %v", err)
	}
	if len(payload.Content) != 2 {
		t.Fatalf("content length = %d, want 2; content=%#v", len(payload.Content), payload.Content)
	}
	if payload.Content[0].Type != "text" || payload.Content[0].Text != "official content" {
		t.Fatalf("top-level text content was not preserved: %#v", payload.Content[0])
	}
	for _, item := range payload.Content {
		if item.Text == "legacy prompt" {
			t.Fatalf("prompt should not override non-empty content: %#v", payload.Content)
		}
	}
}

func TestTopLevelContentTextWithoutPromptIsAccepted(t *testing.T) {
	var req relaycommon.TaskSubmitReq
	if err := rootcommon.Unmarshal([]byte(`{
        "model":"doubao-seedance-2-0-fast-260128",
        "content":[{"type":"text","text":"text-only content"}]
    }`), &req); err != nil {
		t.Fatalf("unmarshal request returned error: %v", err)
	}
	if err := validateSeedanceRequest(req); err != nil {
		t.Fatalf("validateSeedanceRequest returned error: %v", err)
	}

	payload, err := (&TaskAdaptor{}).convertToRequestPayload(&req)
	if err != nil {
		t.Fatalf("convertToRequestPayload returned error: %v", err)
	}
	if len(payload.Content) != 1 || payload.Content[0].Text != "text-only content" {
		t.Fatalf("text-only content was not forwarded: %#v", payload.Content)
	}
}

func TestOfficialTopLevelParametersAreForwardedToDoubao(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/v3/contents/generations/tasks", strings.NewReader(`{
		"model":"doubao-seedance-2-5-260628",
		"content":[{"type":"text","text":"official request"}],
		"resolution":"1080p",
		"ratio":"adaptive",
		"duration":-1,
		"generate_audio":false,
		"watermark":false,
		"return_last_frame":false,
		"output_format":"mov",
		"omni_reference_task_type":"auto",
		"priority":0,
		"execution_expires_after":172800,
		"safety_identifier":"tenant-user"
	}`))
	ctx.Request.Header.Set("Content-Type", "application/json")
	info := &relaycommon.RelayInfo{
		ChannelMeta:   &relaycommon.ChannelMeta{},
		TaskRelayInfo: &relaycommon.TaskRelayInfo{},
	}
	adaptor := &TaskAdaptor{}
	require.Nil(t, adaptor.ValidateRequestAndSetAction(ctx, info))

	body, err := adaptor.BuildRequestBody(ctx, info)
	require.NoError(t, err)
	data, err := io.ReadAll(body)
	require.NoError(t, err)
	var payload map[string]any
	require.NoError(t, rootcommon.Unmarshal(data, &payload))
	require.Equal(t, "doubao-seedance-2-5-260628", payload["model"])
	require.Equal(t, "1080p", payload["resolution"])
	require.Equal(t, "adaptive", payload["ratio"])
	require.Equal(t, float64(-1), payload["duration"])
	require.Equal(t, false, payload["generate_audio"])
	require.Equal(t, false, payload["watermark"])
	require.Equal(t, false, payload["return_last_frame"])
	require.Equal(t, "mov", payload["output_format"])
	require.Equal(t, "auto", payload["omni_reference_task_type"])
	require.Equal(t, float64(0), payload["priority"])
	require.Equal(t, float64(172800), payload["execution_expires_after"])
	require.Equal(t, "tenant-user", payload["safety_identifier"])
}

func TestConvertToRequestPayloadMarksImageListAsReferenceImages(t *testing.T) {
	adaptor := &TaskAdaptor{}
	payload, err := adaptor.convertToRequestPayload(&relaycommon.TaskSubmitReq{
		Model:  "doubao-seedance-2-0-260128",
		Prompt: "让图片1中的角色挥手",
		Images: []string{"asset://asset-character"},
	})
	if err != nil {
		t.Fatalf("convertToRequestPayload returned error: %v", err)
	}
	if len(payload.Content) != 2 {
		t.Fatalf("content length = %d, want 2; content=%#v", len(payload.Content), payload.Content)
	}
	if payload.Content[0].Role != "reference_image" {
		t.Fatalf("image role = %q, want reference_image", payload.Content[0].Role)
	}
	if payload.Content[0].ImageURL == nil || payload.Content[0].ImageURL.URL != "asset://asset-character" {
		t.Fatalf("unexpected image reference: %#v", payload.Content[0])
	}
}

func TestDoResponseUsesSeedanceOfficialCreateShapeOnOfficialPath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/v3/contents/generations/tasks", nil)
	response := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(`{"id":"upstream-task"}`)),
	}

	taskID, _, taskErr := (&TaskAdaptor{}).DoResponse(ctx, response, &relaycommon.RelayInfo{
		OriginModelName: "doubao-seedance-2-5-260628",
		TaskRelayInfo:   &relaycommon.TaskRelayInfo{PublicTaskID: "task-public"},
	})
	require.Nil(t, taskErr)
	require.Equal(t, "upstream-task", taskID)

	var body map[string]any
	require.NoError(t, rootcommon.Unmarshal(recorder.Body.Bytes(), &body))
	require.Equal(t, map[string]any{"id": "task-public"}, body)
}

func TestDoResponseKeepsOpenAIVideoCreateShapeOnV1Path(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", nil)
	response := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(`{"id":"upstream-task"}`)),
	}

	_, _, taskErr := (&TaskAdaptor{}).DoResponse(ctx, response, &relaycommon.RelayInfo{
		OriginModelName: "doubao-seedance-2-5-260628",
		TaskRelayInfo:   &relaycommon.TaskRelayInfo{PublicTaskID: "task-public"},
	})
	require.Nil(t, taskErr)

	var body map[string]any
	require.NoError(t, rootcommon.Unmarshal(recorder.Body.Bytes(), &body))
	require.Equal(t, "task-public", body["id"])
	require.Equal(t, "task-public", body["task_id"])
	require.Equal(t, "video", body["object"])
	require.Equal(t, "queued", body["status"])
}

func TestDoResponseUsesAcceptedForIndependentSeedanceOpenAIPath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", nil)
	response := &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"id":"upstream-task"}`))}

	_, _, taskErr := (&TaskAdaptor{}).DoResponse(ctx, response, &relaycommon.RelayInfo{
		OriginModelName: "doubao-seedance-2-5-260628",
		ChannelMeta:     &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeSeedance},
		TaskRelayInfo:   &relaycommon.TaskRelayInfo{PublicTaskID: "task-public"},
	})
	require.Nil(t, taskErr)
	require.Equal(t, http.StatusAccepted, recorder.Code)
}

func TestConvertToSeedanceOfficialTaskPreservesOfficialFields(t *testing.T) {
	task := &model.Task{
		TaskID: "task-public",
		Status: model.TaskStatusSuccess,
		Properties: model.Properties{
			OriginModelName: "doubao-seedance-2-5-260628",
		},
	}
	task.SetData(map[string]any{
		"id":            "upstream-private",
		"task_id":       "upstream-private",
		"model":         "upstream-model",
		"status":        "succeeded",
		"duration":      5,
		"resolution":    "480p",
		"output_format": "mp4",
		"content":       map[string]any{"video_url": "https://cdn.example.com/result.mp4"},
		"usage": map[string]any{
			"completion_tokens": 800000,
			"total_tokens":      800000,
		},
	})

	data, err := (&TaskAdaptor{}).ConvertToSeedanceOfficialTask(task)
	require.NoError(t, err)
	var response map[string]any
	require.NoError(t, rootcommon.Unmarshal(data, &response))
	require.Equal(t, "task-public", response["id"])
	require.NotContains(t, response, "task_id")
	require.Equal(t, "doubao-seedance-2-5-260628", response["model"])
	require.Equal(t, "succeeded", response["status"])
	require.Equal(t, float64(5), response["duration"])
	require.Equal(t, "480p", response["resolution"])
	require.Equal(t, "mp4", response["output_format"])
	require.Equal(t, "https://cdn.example.com/result.mp4", response["content"].(map[string]any)["video_url"])
	require.Equal(t, float64(800000), response["usage"].(map[string]any)["total_tokens"])
}

func TestConvertToSeedanceOfficialTaskNormalizesFailure(t *testing.T) {
	task := &model.Task{
		TaskID:     "task-failed",
		Status:     model.TaskStatusFailure,
		FailReason: "provider rejected request",
	}
	task.SetData(map[string]any{"id": "upstream-private", "status": "failed"})

	data, err := (&TaskAdaptor{}).ConvertToSeedanceOfficialTask(task)
	require.NoError(t, err)
	var response map[string]any
	require.NoError(t, rootcommon.Unmarshal(data, &response))
	require.Equal(t, "failed", response["status"])
	require.Equal(t, "seedance_task_failed", response["error"].(map[string]any)["code"])
	require.Equal(t, "provider rejected request", response["error"].(map[string]any)["message"])
}

func TestConvertToSeedanceOfficialTaskDoesNotAddFailureToCancelledTask(t *testing.T) {
	task := &model.Task{
		TaskID: "task-public-cancelled",
		Status: model.TaskStatusFailure,
		Data:   []byte(`{"id":"private-task","status":"cancelled"}`),
		Properties: model.Properties{
			OriginModelName: "Public Seedance",
		},
	}

	data, err := (&TaskAdaptor{}).ConvertToSeedanceOfficialTask(task)

	require.NoError(t, err)
	require.JSONEq(t, `{"id":"task-public-cancelled","model":"Public Seedance","status":"cancelled"}`, string(data))
}
