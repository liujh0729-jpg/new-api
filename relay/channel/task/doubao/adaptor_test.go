package doubao

import (
	"net/http/httptest"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
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
