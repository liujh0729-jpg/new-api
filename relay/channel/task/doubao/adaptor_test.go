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
		Duration: 10,
	})
	if err != nil {
		t.Fatalf("convertToRequestPayload returned error: %v", err)
	}
	if payload.Duration == nil || int(*payload.Duration) != 10 {
		t.Fatalf("duration = %v, want 10", payload.Duration)
	}
}
