package aipdd

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSeedanceCatalogExactBillingMatrix(t *testing.T) {
	oldQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 500000
	t.Cleanup(func() {
		common.QuotaPerUnit = oldQuotaPerUnit
		constant.ResetAIPDDCapabilities()
	})
	constant.SetAIPDDCapabilities([]constant.AIPDDCapability{seedanceTestCapability()})

	tests := []struct {
		name   string
		body   string
		quota  int
		awcoin float64
	}{
		{name: "minimum and decimal duration", body: `{"model":"AP Seedance","resolution":"1080p","duration":2.2,"content":[{"type":"text","text":"hello"}]}`, quota: 101000, awcoin: 101},
		{name: "frames divided by explicit fps", body: `{"model":"AP Seedance","resolution":"1080p","frames":49,"frames_per_second":24,"content":[{"type":"text","text":"hello"}]}`, quota: 101000, awcoin: 101},
		{name: "reference video variant", body: `{"model":"AP Seedance","resolution":"1080p","duration":5,"content":[{"type":"video","role":"reference_video"}]}`, quota: 150000, awcoin: 150},
		{name: "model default duration", body: `{"model":"AP Seedance","resolution":"1080p","content":[{"type":"text","text":"hello"}]}`, quota: 201000, awcoin: 201},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, info, adaptor := seedanceRequestContext(t, test.body)
			require.Nil(t, adaptor.ValidateRequestAndSetAction(ctx, info))
			quota, details, err := adaptor.EstimateExactQuota(ctx, info)
			require.NoError(t, err)
			require.Equal(t, test.quota, quota)
			require.Equal(t, test.awcoin, details["aipdd_awcoin"])
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
	t.Helper()
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	info := &relaycommon.RelayInfo{
		OriginModelName: "AP Seedance",
		PriceData:       types.PriceData{GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 2}},
	}
	info.ChannelMeta = &relaycommon.ChannelMeta{ChannelBaseUrl: "https://aipdd.example", ApiKey: "sk-test", UpstreamModelName: "AP Seedance"}
	info.TaskRelayInfo = &relaycommon.TaskRelayInfo{}
	adaptor := &TaskAdaptor{}
	adaptor.Init(info)
	return ctx, info, adaptor
}

func seedanceTestCapability() constant.AIPDDCapability {
	return constant.AIPDDCapability{
		ModelName: "AP Seedance", TaskKind: "video_generation",
		EndpointType:    constant.EndpointTypeOpenAIVideo,
		BillingType:     constant.AIPDDBillingTypeDurationSeconds,
		CatalogRevision: "revision-1", ExecutionProtocol: "seedance_official",
		ExecutionPath: "/api/v3/contents/generations/tasks", AWCoinUSDPerCoin: 0.001,
		SeedancePricing: &constant.AIPDDSeedancePricing{ByResolution: map[string]constant.AIPDDSeedanceResolutionPricing{
			"1080p": {
				DefaultDurationSeconds: 5, DefaultFramesPerSecond: 24,
				PriceVariants: []constant.AIPDDSeedancePriceVariant{
					{HasReferenceVideo: false, AWCoinPerSecond: 40.1, MinimumAWCoin: 100.2},
					{HasReferenceVideo: true, AWCoinPerSecond: 30, MinimumAWCoin: 120.1},
				},
			},
		}},
	}
}
