package aipdd

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
)

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
		BillingType:            constant.AIPDDBillingTypePerCall,
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
	ctx.Set("task_request", relaycommon.TaskSubmitReq{Model: modelName, Duration: 20})
	if ratios := adaptor.EstimateBilling(ctx, relayInfoWithModel(modelName)); ratios != nil {
		t.Fatalf("per-call LTX must not add seconds billing: %#v", ratios)
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
		BillingType:            constant.AIPDDBillingTypePerCall,
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
