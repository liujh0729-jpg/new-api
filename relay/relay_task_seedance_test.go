package relay

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestIndependentSeedancePersistsAttemptBeforeArkAndNeverResubmitsUnknownOutcome(t *testing.T) {
	service.InitHttpClient()
	previousDB := model.DB
	dsn := fmt.Sprintf("file:relay-seedance-submit-intent-%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&model.Task{}, &model.SeedanceChannelConfig{}, &model.SeedanceVolcengineCredential{},
		&model.MediaEnhancementProvider{}, &model.SeedanceModelOffering{},
		&model.SeedanceOrder{}, &model.SeedanceAttempt{},
	))
	model.DB = db
	t.Cleanup(func() { model.DB = previousDB })

	previousSecret, previousConfigured := common.CryptoSecret, common.CryptoSecretConfigured
	common.CryptoSecret = strings.Repeat("j", 32)
	common.CryptoSecretConfigured = true
	groupRatios := ratio_setting.GroupRatio2JSONString()
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1}`))
	t.Cleanup(func() {
		common.CryptoSecret = previousSecret
		common.CryptoSecretConfigured = previousConfigured
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(groupRatios))
	})
	arkCredential, err := common.EncryptSensitiveValue("ark-submit-intent-secret")
	require.NoError(t, err)
	now := time.Now().Unix()
	const channelID = 9401
	config := &model.SeedanceChannelConfig{
		ChannelID: channelID, Revision: 1, InstanceID: "30000000-0000-0000-0000-000000000401",
		Status: model.SeedanceConfigActive, CreatedAt: now, UpdatedAt: now,
	}
	credential := &model.SeedanceVolcengineCredential{
		ChannelID: channelID, Version: 1, ArkAPIKeyEncrypted: arkCredential,
		Fingerprint: "sha256:ark", MaskedSuffix: "****", Status: model.SeedanceCredentialActive,
		CreatedAt: now,
	}
	provider := &model.MediaEnhancementProvider{
		Version: 1, ProviderType: model.SeedanceProviderDirect, DisplayName: "private",
		ServiceEndpoint: "https://provider.example.test", ServiceCode: "private-service",
		CapabilitiesJSON: `{}`, TimeoutPolicyJSON: `{}`, RetryPolicyJSON: `{}`, FallbackPolicyJSON: `{}`,
		Status: model.SeedanceConfigActive, CreatedAt: now, UpdatedAt: now,
	}
	require.NoError(t, db.Create(config).Error)
	require.NoError(t, db.Create(credential).Error)
	require.NoError(t, db.Create(provider).Error)
	offering := &model.SeedanceModelOffering{
		ChannelID: channelID, DisplayName: "Public Seedance", ProviderModelID: "private-seedance-model",
		EnhancementProviderID: provider.ID, EnhancementServiceCode: provider.ServiceCode,
		EnhancementSpecificationJSON: `{}`, EnhancementSpecificationVersion: "spec-v1",
		ModelSaleMicroRMB: 5_000_000, ServiceChargeMicroRMB: 1_000_000,
		VolcengineUnitCostMicroRMB: 2_000_000, PricingVersion: "price-v1",
		Enabled: true, PublishedAt: now, CreatedAt: now, UpdatedAt: now,
	}
	require.NoError(t, db.Create(offering).Error)

	var arkCalls atomic.Int32
	var intentWasDurable atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		arkCalls.Add(1)
		var taskCount, orderCount, attemptCount int64
		_ = db.Model(&model.Task{}).Count(&taskCount).Error
		_ = db.Model(&model.SeedanceOrder{}).Count(&orderCount).Error
		_ = db.Model(&model.SeedanceAttempt{}).Where("status = ? AND external_task_id = ''", "SUBMITTING").Count(&attemptCount).Error
		intentWasDurable.Store(taskCount == 1 && orderCount == 1 && attemptCount == 1)
		hijacker, ok := w.(http.Hijacker)
		if !ok {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		connection, _, hijackErr := hijacker.Hijack()
		if hijackErr == nil {
			_ = connection.Close()
		}
	}))
	defer server.Close()

	requestBody := `{"model":"Public Seedance","content":[{"type":"text","text":"hello"}],"duration":5,"resolution":"720p"}`
	newContext := func() *gin.Context {
		ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
		ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", strings.NewReader(requestBody))
		ctx.Request.Header.Set("Content-Type", "application/json")
		common.SetContextKey(ctx, constant.ContextKeyChannelType, constant.ChannelTypeSeedance)
		common.SetContextKey(ctx, constant.ContextKeyChannelId, channelID)
		common.SetContextKey(ctx, constant.ContextKeyChannelBaseUrl, server.URL)
		common.SetContextKey(ctx, constant.ContextKeyChannelKey, "managed")
		common.SetContextKey(ctx, constant.ContextKeyOriginalModel, offering.DisplayName)
		ctx.Set("model_mapping", `{"Public Seedance":"private-seedance-model"}`)
		return ctx
	}
	info := &relaycommon.RelayInfo{
		OriginModelName: offering.DisplayName, UserId: 9401, UserGroup: "default", UsingGroup: "default",
		Billing: &frozenQuoteBilling{}, TaskRelayInfo: &relaycommon.TaskRelayInfo{},
	}
	info.BeforeSeedanceGenerationSubmit = func() error {
		task := model.InitTask(constant.TaskPlatform("59"), info)
		task.Quota = info.PriceData.Quota
		task.Action = info.Action
		task.Properties.UpstreamModelName = task.Properties.OriginModelName
		task.SetData(map[string]any{"id": task.TaskID, "model": task.Properties.OriginModelName, "status": "queued"})
		_, insertErr := model.InsertTaskWithSeedanceOrder(model.SeedanceOrderCreate{
			Task: task, Config: config, Credential: credential, Offering: offering, Provider: provider,
			RequestFactsJSON: `{}`, PricingSnapshot: `{"pricing_version":"price-v1"}`,
			PublicProtocol: model.SeedanceProtocolOpenAI,
		})
		return insertErr
	}

	result, taskErr := RelayTaskSubmit(newContext(), info)
	require.Nil(t, taskErr)
	require.NotNil(t, result)
	require.True(t, result.TaskPersisted)
	require.Equal(t, http.StatusAccepted, result.DeferredHTTPStatus)
	require.Empty(t, result.UpstreamTaskID)
	require.True(t, intentWasDurable.Load(), "Ark must observe the local task/order/attempt already committed")
	require.EqualValues(t, 1, arkCalls.Load())
	var attempt model.SeedanceAttempt
	require.NoError(t, db.First(&attempt).Error)
	require.Equal(t, model.SeedanceSubmissionOutcomeUnknown, attempt.Status)

	second, taskErr := RelayTaskSubmit(newContext(), info)
	require.Nil(t, taskErr)
	require.NotNil(t, second)
	require.True(t, second.TaskPersisted)
	require.EqualValues(t, 1, arkCalls.Load(), "an unknown generation attempt must not be submitted twice")
}

func TestIsDefinitiveArkCreateRejection(t *testing.T) {
	for _, testCase := range []struct {
		status     int
		definitive bool
	}{
		{status: http.StatusBadRequest, definitive: true},
		{status: http.StatusUnauthorized, definitive: true},
		{status: http.StatusTooManyRequests, definitive: true},
		{status: http.StatusRequestTimeout, definitive: false},
		{status: http.StatusTemporaryRedirect, definitive: false},
		{status: http.StatusInternalServerError, definitive: false},
		{status: http.StatusServiceUnavailable, definitive: false},
	} {
		t.Run(strconv.Itoa(testCase.status), func(t *testing.T) {
			require.Equal(t, testCase.definitive, isDefinitiveArkCreateRejection(testCase.status))
		})
	}
}

func TestIndependentSeedanceCreateRejectionDoesNotExposeArkDetails(t *testing.T) {
	body := []byte(`{"error":{"code":"InvalidModel","message":"private-seedance-model rejected for account 12345678"}}`)

	badRequest := independentSeedanceCreateRejectionError(body, http.StatusBadRequest)
	require.Equal(t, "video_request_rejected", badRequest.Code)
	require.Equal(t, "The video request was rejected", badRequest.Message)
	require.Equal(t, http.StatusBadRequest, badRequest.StatusCode)
	require.Nil(t, badRequest.Data)

	upstreamAuth := independentSeedanceCreateRejectionError(body, http.StatusUnauthorized)
	require.Equal(t, "video_service_unavailable", upstreamAuth.Code)
	require.Equal(t, "Video service is temporarily unavailable", upstreamAuth.Message)
	require.Equal(t, http.StatusServiceUnavailable, upstreamAuth.StatusCode)
	require.Nil(t, upstreamAuth.Data)

	for _, publicValue := range []string{badRequest.Message, upstreamAuth.Message} {
		require.NotContains(t, publicValue, "private-seedance-model")
		require.NotContains(t, publicValue, "12345678")
	}
}

func TestIsSeedanceOfficialTaskRequiresAIPDDOfficialProtocol(t *testing.T) {
	aipddPlatform := constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeAIPDD))
	official := &model.Task{
		Platform: aipddPlatform,
		PrivateData: model.TaskPrivateData{AIPDDExecution: &model.AIPDDTaskExecutionSnapshot{
			Protocol: "seedance_official",
		}},
	}
	require.True(t, isSeedanceOfficialTask(official))

	wrongProtocol := *official
	wrongProtocol.PrivateData.AIPDDExecution = &model.AIPDDTaskExecutionSnapshot{Protocol: "legacy"}
	require.False(t, isSeedanceOfficialTask(&wrongProtocol))

	wrongPlatform := *official
	wrongPlatform.Platform = constant.TaskPlatform("other")
	require.False(t, isSeedanceOfficialTask(&wrongPlatform))
}

func TestIsSeedanceOfficialTaskSupportsDirectDoubaoPlatforms(t *testing.T) {
	for _, channelType := range []int{constant.ChannelTypeDoubaoVideo, constant.ChannelTypeVolcEngine} {
		task := &model.Task{Platform: constant.TaskPlatform(strconv.Itoa(channelType))}
		require.True(t, isSeedanceOfficialTask(task), "channel type %d should support Seedance official fetch", channelType)
	}
}

func TestSeedanceFetchIsIsolatedToTheCreationProtocol(t *testing.T) {
	previousDB := model.DB
	dsn := fmt.Sprintf("file:relay-seedance-protocol-%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Task{}, &model.SeedanceOrder{}))
	model.DB = db
	t.Cleanup(func() { model.DB = previousDB })

	task := &model.Task{
		TaskID: "task_openai_only", UserId: 42, ChannelId: 901,
		Platform: constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeSeedance)),
		Status:   model.TaskStatusQueued, Properties: model.Properties{OriginModelName: "Public Seedance"},
	}
	task.SetData(map[string]any{"id": task.TaskID, "model": "Public Seedance", "status": "queued"})
	require.NoError(t, db.Create(task).Error)
	require.NoError(t, db.Create(&model.SeedanceOrder{
		PlatformOrderID: "01990a4c-8f5a-7ca2-9f95-77cc19375c93", NewAPITaskID: task.TaskID,
		NewAPIUserID: task.UserId, ChannelID: task.ChannelId, OrderStatus: model.SeedanceOrderGenerationProcessing,
		PublicProtocol: model.SeedanceProtocolOpenAI,
	}).Error)

	official, _ := gin.CreateTestContext(httptest.NewRecorder())
	official.Request = httptest.NewRequest(http.MethodGet, "/api/v3/contents/generations/tasks/"+task.TaskID, nil)
	official.Params = gin.Params{{Key: "task_id", Value: task.TaskID}}
	official.Set("id", task.UserId)
	_, taskErr := taskFetchByIDRespBodyBuilder(official)
	require.NotNil(t, taskErr)
	require.Equal(t, http.StatusNotFound, taskErr.StatusCode)

	openAI, _ := gin.CreateTestContext(httptest.NewRecorder())
	openAI.Request = httptest.NewRequest(http.MethodGet, "/v1/videos/"+task.TaskID, nil)
	openAI.Params = gin.Params{{Key: "task_id", Value: task.TaskID}}
	openAI.Set("id", task.UserId)
	body, taskErr := taskFetchByIDRespBodyBuilder(openAI)
	require.Nil(t, taskErr)
	var response map[string]any
	require.NoError(t, common.Unmarshal(body, &response))
	require.Equal(t, task.TaskID, response["id"])
	require.Equal(t, "queued", response["status"])
}

func TestIsSeedanceOfficialTaskFallsBackToCatalogForOlderTasks(t *testing.T) {
	const modelName = "seedance-official-catalog-test"
	constant.SetAIPDDCapabilities([]constant.AIPDDCapability{{
		ModelName: modelName, ExecutionProtocol: "seedance_official",
	}})
	t.Cleanup(constant.ResetAIPDDCapabilities)

	task := &model.Task{
		Platform: constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeAIPDD)),
		Properties: model.Properties{
			OriginModelName: modelName,
		},
	}
	require.True(t, isSeedanceOfficialTask(task))
}

func TestSeedanceOfficialFetchReturnsTopLevelTaskAndEnforcesOwnership(t *testing.T) {
	previousDB := model.DB
	dsn := fmt.Sprintf("file:relay-seedance-official-%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Task{}))
	model.DB = db
	t.Cleanup(func() { model.DB = previousDB })

	requestedDuration := 6.0
	task := &model.Task{
		TaskID: "task_owned", UserId: 42, ChannelId: 999999,
		Platform: constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeAIPDD)),
		Status:   model.TaskStatusSuccess,
		Properties: model.Properties{
			OriginModelName: "AP Seedance",
		},
		PrivateData: model.TaskPrivateData{AIPDDExecution: &model.AIPDDTaskExecutionSnapshot{
			Protocol: "seedance_official", RequestedDuration: &requestedDuration,
		}},
	}
	task.SetData(map[string]any{
		"id": "cgt-private", "status": "succeeded",
		"content": map[string]any{"video_url": "https://cdn.example.com/out.mp4"},
	})
	require.NoError(t, db.Create(task).Error)

	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/v3/contents/generations/tasks/task_owned", nil)
	ctx.Params = gin.Params{{Key: "task_id", Value: "task_owned"}}
	ctx.Set("id", 42)

	body, taskErr := taskFetchByIDRespBodyBuilder(ctx)
	require.Nil(t, taskErr)
	var response map[string]any
	require.NoError(t, common.Unmarshal(body, &response))
	require.Equal(t, "task_owned", response["id"])
	require.Equal(t, "succeeded", response["status"])
	require.Equal(t, requestedDuration, response["duration"])
	require.NotContains(t, response, "data")
	require.NotContains(t, response, "code")

	otherUserCtx, _ := gin.CreateTestContext(httptest.NewRecorder())
	otherUserCtx.Request = httptest.NewRequest(http.MethodGet, "/api/v3/contents/generations/tasks/task_owned", nil)
	otherUserCtx.Params = gin.Params{{Key: "task_id", Value: "task_owned"}}
	otherUserCtx.Set("id", 43)
	_, taskErr = taskFetchByIDRespBodyBuilder(otherUserCtx)
	require.NotNil(t, taskErr)
	require.Equal(t, http.StatusNotFound, taskErr.StatusCode)

	directTask := &model.Task{
		TaskID:   "task_direct_doubao",
		UserId:   42,
		Platform: constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeDoubaoVideo)),
		Status:   model.TaskStatusSuccess,
		Properties: model.Properties{
			OriginModelName: "doubao-seedance-2-5-260628",
		},
	}
	directTask.SetData(map[string]any{
		"id":            "upstream-direct-private",
		"model":         "doubao-seedance-2-5-260628",
		"status":        "succeeded",
		"duration":      5,
		"resolution":    "480p",
		"output_format": "mp4",
		"content":       map[string]any{"video_url": "https://cdn.example.com/direct.mp4"},
	})
	require.NoError(t, db.Create(directTask).Error)

	directCtx, _ := gin.CreateTestContext(httptest.NewRecorder())
	directCtx.Request = httptest.NewRequest(http.MethodGet, "/api/v3/contents/generations/tasks/task_direct_doubao", nil)
	directCtx.Params = gin.Params{{Key: "task_id", Value: "task_direct_doubao"}}
	directCtx.Set("id", 42)
	directBody, directErr := taskFetchByIDRespBodyBuilder(directCtx)
	require.Nil(t, directErr)
	var directResponse map[string]any
	require.NoError(t, common.Unmarshal(directBody, &directResponse))
	require.Equal(t, "task_direct_doubao", directResponse["id"])
	require.Equal(t, "succeeded", directResponse["status"])
	require.Equal(t, "doubao-seedance-2-5-260628", directResponse["model"])
	require.Equal(t, "https://cdn.example.com/direct.mp4", directResponse["content"].(map[string]any)["video_url"])
	require.NotContains(t, directResponse, "task_id")
	require.NotContains(t, directResponse, "object")
}
