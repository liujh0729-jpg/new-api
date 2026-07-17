package relay

import (
	"math"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

const (
	testTaskPricingOriginModel   = "test-local-seedance"
	testTaskPricingUpstreamModel = "test-upstream-seedance"
)

type frozenQuoteBilling struct{}

func (*frozenQuoteBilling) Settle(int) error         { return nil }
func (*frozenQuoteBilling) Refund(*gin.Context)      {}
func (*frozenQuoteBilling) NeedsRefund() bool        { return false }
func (*frozenQuoteBilling) GetPreConsumedQuota() int { return 0 }
func (*frozenQuoteBilling) Reserve(int) error        { return nil }

func TestRelayTaskSubmitFreezesLocalTaskQuoteAcrossRetries(t *testing.T) {
	service.InitHttpClient()
	restoreTaskPricingGlobals(t)
	oldQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 1000
	t.Cleanup(func() { common.QuotaPerUnit = oldQuotaPerUnit })

	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1}`))
	loadTaskPricingConfig(t, billing_setting.ReferenceVideoPolicyCustom, 0.12, 0.18)
	constant.SetAIPDDCapabilities(taskPricingTestCapabilities(0.001, 10))

	var upstreamCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls.Add(1)
		require.Equal(t, "/api/v3/contents/generations/tasks", r.URL.Path)
		require.Equal(t, "Bearer sk-test", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"upstream-task","status":"queued"}`))
	}))
	defer server.Close()

	requestBody := `{"model":"test-local-seedance","resolution":"1080p","duration":2.25,"content":[{"type":"video_url","video_url":{"url":"https://cdn.example.com/reference.mp4"}}]}`
	info := &relaycommon.RelayInfo{
		OriginModelName: testTaskPricingOriginModel,
		UserGroup:       "default",
		UsingGroup:      "default",
		Billing:         &frozenQuoteBilling{},
		TaskRelayInfo:   &relaycommon.TaskRelayInfo{},
	}

	firstContext, _ := taskPricingRelayContext(server.URL, requestBody)
	firstResult, taskErr := RelayTaskSubmit(firstContext, info)
	require.Nil(t, taskErr)
	require.NotNil(t, firstResult)
	require.Equal(t, int(math.Round(0.18*2.25*1000)), firstResult.Quota)
	require.NotNil(t, info.TaskPricingQuote)
	require.Equal(t, testTaskPricingOriginModel, info.OriginModelName)
	require.Equal(t, testTaskPricingUpstreamModel, info.UpstreamModelName)
	require.Equal(t, billing_setting.TaskPricingVariantReferenceVideo, info.TaskPricingQuote.Variant)
	require.Equal(t, billing_setting.TaskPricingUnitSecond, info.TaskPricingQuote.Unit)
	require.Equal(t, 0.18, info.TaskPricingQuote.UnitPriceUSD)
	require.Equal(t, 2.25, info.TaskPricingQuote.Quantity)
	require.Equal(t, "1080p", info.TaskPricingQuote.Resolution)
	require.Equal(t, 1.0, info.TaskPricingQuote.GroupRatio)
	require.InDelta(t, 0.18*2.25, info.TaskPricingQuote.BaseUSD, 1e-12)
	require.InDelta(t, 0.18*2.25, info.TaskPricingQuote.SaleUSD, 1e-12)
	require.Equal(t, firstResult.Quota, info.TaskPricingQuote.Quota)
	require.True(t, info.TaskPricingQuote.HasReferenceVideo)
	require.NotNil(t, firstResult.AIPDDExecution)
	require.Equal(t, "1080p", firstResult.AIPDDExecution.Resolution)
	require.Zero(t, firstResult.AIPDDExecution.EstimatedAWCoin)
	require.Zero(t, firstResult.AIPDDExecution.USDPerAWCoin)

	// Simulate settings and provider-cost changes between channel attempts. The
	// second attempt must reuse the first local retail quote in full.
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":3}`))
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"billing_setting.billing_mode": `{}`,
		"billing_setting.task_pricing": `{}`,
	}))
	constant.SetAIPDDCapabilities(taskPricingTestCapabilities(99, 100_000))

	secondContext, _ := taskPricingRelayContext(server.URL, requestBody)
	secondResult, taskErr := RelayTaskSubmit(secondContext, info)
	require.Nil(t, taskErr)
	require.NotNil(t, secondResult)
	require.Equal(t, firstResult.Quota, secondResult.Quota)
	require.Equal(t, 0.18, info.PriceData.ModelPrice)
	require.Equal(t, 1.0, info.PriceData.GroupRatioInfo.GroupRatio)
	require.Equal(t, map[string]float64{
		"seconds":             2.25,
		"has_reference_video": 1,
	}, info.PriceData.OtherRatios)
	require.Equal(t, int32(2), upstreamCalls.Load())
}

func TestRelayTaskSubmitDisabledReferenceVideoStopsBeforeUpstream(t *testing.T) {
	service.InitHttpClient()
	restoreTaskPricingGlobals(t)
	loadTaskPricingConfig(t, billing_setting.ReferenceVideoPolicyDisabled, 0.12, 0)
	constant.SetAIPDDCapabilities(taskPricingTestCapabilities(0.001, 10))

	var upstreamCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		upstreamCalls.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	ctx, _ := taskPricingRelayContext(server.URL, `{"model":"test-local-seedance","resolution":"1080p","duration":5,"content":[{"type":"video","role":"reference_video"}]}`)
	info := &relaycommon.RelayInfo{
		OriginModelName: testTaskPricingOriginModel,
		UserGroup:       "default",
		UsingGroup:      "default",
		TaskRelayInfo:   &relaycommon.TaskRelayInfo{},
	}

	result, taskErr := RelayTaskSubmit(ctx, info)
	require.Nil(t, result)
	require.NotNil(t, taskErr)
	require.Equal(t, http.StatusBadRequest, taskErr.StatusCode)
	require.Equal(t, "reference_video_not_allowed", taskErr.Code)
	require.Zero(t, upstreamCalls.Load())
	require.Nil(t, info.Billing)
}

func TestRelayTaskSubmitRejectsSupportedResolutionWithoutLocalTier(t *testing.T) {
	service.InitHttpClient()
	restoreTaskPricingGlobals(t)
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1}`))
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"billing_setting.billing_mode": `{"test-local-seedance":"task_pricing"}`,
		"billing_setting.task_pricing": `{"test-local-seedance":{"unit":"second","by_resolution":{"720p":{"no_reference_video_unit_price":0.08,"reference_video_policy":"same"}}}}`,
	}))
	constant.SetAIPDDCapabilities(taskPricingTestCapabilities(0.001, 10))

	var upstreamCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		upstreamCalls.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	ctx, _ := taskPricingRelayContext(server.URL, `{"model":"test-local-seedance","resolution":"1080p","duration":5,"content":[{"type":"text","text":"hello"}]}`)
	info := &relaycommon.RelayInfo{
		OriginModelName: testTaskPricingOriginModel,
		UserGroup:       "default",
		UsingGroup:      "default",
		Billing:         &frozenQuoteBilling{},
		TaskRelayInfo:   &relaycommon.TaskRelayInfo{},
	}

	result, taskErr := RelayTaskSubmit(ctx, info)
	require.Nil(t, result)
	require.NotNil(t, taskErr)
	require.Equal(t, "resolution_price_not_configured", taskErr.Code)
	require.Zero(t, upstreamCalls.Load())
}

func TestRelayTaskSubmitAllowsExplicitFreeGroup(t *testing.T) {
	service.InitHttpClient()
	restoreTaskPricingGlobals(t)
	loadTaskPricingConfig(t, billing_setting.ReferenceVideoPolicySame, 0.12, 0)
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"free":0}`))
	constant.SetAIPDDCapabilities(taskPricingTestCapabilities(0.001, 10))

	var upstreamCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		upstreamCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"upstream-task","status":"queued"}`))
	}))
	defer server.Close()

	ctx, _ := taskPricingRelayContext(server.URL, `{"model":"test-local-seedance","resolution":"1080p","duration":5,"content":[{"type":"text","text":"hello"}]}`)
	info := &relaycommon.RelayInfo{
		OriginModelName: testTaskPricingOriginModel,
		UserGroup:       "free",
		UsingGroup:      "free",
		TaskRelayInfo:   &relaycommon.TaskRelayInfo{},
	}

	result, taskErr := RelayTaskSubmit(ctx, info)
	require.Nil(t, taskErr)
	require.NotNil(t, result)
	require.Zero(t, result.Quota)
	require.True(t, info.PriceData.FreeModel)
	require.NotNil(t, info.TaskPricingQuote)
	require.Zero(t, info.TaskPricingQuote.GroupRatio)
	require.Zero(t, info.TaskPricingQuote.SaleUSD)
	require.Nil(t, info.Billing)
	require.Equal(t, int32(1), upstreamCalls.Load())
}

func TestRelayTaskSubmitRejectsLegacyModelPriceWithoutTaskPricing(t *testing.T) {
	service.InitHttpClient()
	restoreTaskPricingGlobals(t)
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"billing_setting.billing_mode": `{}`,
		"billing_setting.task_pricing": `{}`,
	}))
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"test-local-seedance":99}`))
	constant.SetAIPDDCapabilities(taskPricingTestCapabilities(0.001, 10))

	var upstreamCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		upstreamCalls.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	ctx, _ := taskPricingRelayContext(server.URL, `{"model":"test-local-seedance","resolution":"1080p","duration":5,"content":[{"type":"text","text":"hello"}]}`)
	info := &relaycommon.RelayInfo{
		OriginModelName: testTaskPricingOriginModel,
		UserGroup:       "default",
		UsingGroup:      "default",
		TaskRelayInfo:   &relaycommon.TaskRelayInfo{},
	}

	result, taskErr := RelayTaskSubmit(ctx, info)
	require.Nil(t, result)
	require.NotNil(t, taskErr)
	require.Equal(t, http.StatusBadRequest, taskErr.StatusCode)
	require.Equal(t, "model_price_error", taskErr.Code)
	require.Zero(t, upstreamCalls.Load())
}

func TestRelayTaskSubmitFramesFPSKeepsExactSecondsUntilFinalRound(t *testing.T) {
	service.InitHttpClient()
	restoreTaskPricingGlobals(t)
	oldQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 1000
	t.Cleanup(func() { common.QuotaPerUnit = oldQuotaPerUnit })
	loadTaskPricingConfig(t, billing_setting.ReferenceVideoPolicySame, 0.12, 0)
	constant.SetAIPDDCapabilities(taskPricingTestCapabilities(0.001, 10))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"upstream-task","status":"queued"}`))
	}))
	defer server.Close()

	ctx, _ := taskPricingRelayContext(server.URL, `{"model":"test-local-seedance","resolution":"1080p","frames":49,"frames_per_second":24,"content":[{"type":"text","text":"hello"}]}`)
	info := &relaycommon.RelayInfo{
		OriginModelName: testTaskPricingOriginModel,
		UserGroup:       "default",
		UsingGroup:      "default",
		Billing:         &frozenQuoteBilling{},
		TaskRelayInfo:   &relaycommon.TaskRelayInfo{},
	}

	result, taskErr := RelayTaskSubmit(ctx, info)
	require.Nil(t, taskErr)
	require.NotNil(t, result)
	require.NotNil(t, info.TaskPricingQuote)
	require.InDelta(t, 49.0/24, info.TaskPricingQuote.Quantity, 1e-12)
	require.Equal(t, int(math.Round(0.12*(49.0/24)*1000)), result.Quota)
	require.Equal(t, 245, result.Quota)
}

func taskPricingRelayContext(baseURL, requestBody string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", strings.NewReader(requestBody))
	ctx.Request.Header.Set("Content-Type", "application/json")
	common.SetContextKey(ctx, constant.ContextKeyChannelType, constant.ChannelTypeAIPDD)
	common.SetContextKey(ctx, constant.ContextKeyChannelId, 1001)
	common.SetContextKey(ctx, constant.ContextKeyChannelBaseUrl, baseURL)
	common.SetContextKey(ctx, constant.ContextKeyChannelKey, "sk-test")
	common.SetContextKey(ctx, constant.ContextKeyOriginalModel, testTaskPricingOriginModel)
	ctx.Set("model_mapping", `{"test-local-seedance":"test-upstream-seedance"}`)
	return ctx, recorder
}

func restoreTaskPricingGlobals(t *testing.T) {
	t.Helper()
	configSnapshot := config.GlobalConfig.ExportAllConfigs()
	groupRatioSnapshot := ratio_setting.GroupRatio2JSONString()
	modelPriceSnapshot := ratio_setting.ModelPrice2JSONString()
	capabilitiesSnapshot := constant.GetAIPDDCapabilities()
	t.Cleanup(func() {
		require.NoError(t, config.GlobalConfig.LoadFromDB(configSnapshot))
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(groupRatioSnapshot))
		require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(modelPriceSnapshot))
		constant.SetAIPDDCapabilities(capabilitiesSnapshot)
	})
}

func loadTaskPricingConfig(t *testing.T, policy string, basePrice, referencePrice float64) {
	t.Helper()
	pricing := `{"test-local-seedance":{"unit":"second","no_reference_video_unit_price":` +
		strings.TrimRight(strings.TrimRight(fmtFloat(basePrice), "0"), ".") +
		`,"reference_video_policy":"` + policy + `"`
	if policy == billing_setting.ReferenceVideoPolicyCustom {
		pricing += `,"reference_video_unit_price":` + strings.TrimRight(strings.TrimRight(fmtFloat(referencePrice), "0"), ".")
	}
	pricing += `}}`
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"billing_setting.billing_mode": `{"test-local-seedance":"task_pricing"}`,
		"billing_setting.task_pricing": pricing,
	}))
}

func fmtFloat(value float64) string {
	return strconv.FormatFloat(value, 'f', 6, 64)
}

func taskPricingTestCapabilities(usdPerAWCoin, awcoinPerSecond float64) []constant.AIPDDCapability {
	makeCapability := func(modelName string) constant.AIPDDCapability {
		return constant.AIPDDCapability{
			ModelName: modelName, TaskKind: "video_generation",
			EndpointType:    constant.EndpointTypeOpenAIVideo,
			BillingType:     constant.AIPDDBillingTypeDurationSeconds,
			CatalogRevision: "test-revision", ExecutionProtocol: "seedance_official",
			ExecutionPath: "/api/v3/contents/generations/tasks", AWCoinUSDPerCoin: usdPerAWCoin,
			SeedancePricing: &constant.AIPDDSeedancePricing{ByResolution: map[string]constant.AIPDDSeedanceResolutionPricing{
				"1080p": {
					TargetResolution:          "1080p",
					DefaultDurationSeconds:    5,
					DefaultFramesPerSecond:    24,
					AmountAWCoinPerSecond:     awcoinPerSecond,
					TextInputAWCoinPerSecond:  awcoinPerSecond,
					ImageInputAWCoinPerSecond: awcoinPerSecond,
					VideoInputAWCoinPerSecond: awcoinPerSecond * 2,
					AudioInputAWCoinPerSecond: awcoinPerSecond,
				},
			}},
		}
	}
	return []constant.AIPDDCapability{makeCapability(testTaskPricingUpstreamModel)}
}
