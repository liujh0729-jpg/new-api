package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/stretchr/testify/require"
)

func TestMicroRMBStringSupportsMinimumInt64(t *testing.T) {
	require.Equal(t, "-9223372036854.775808", microRMBString(math.MinInt64))
}

func TestIndependentSeedanceMissingUpstreamIDWaitsForTimeoutSweeper(t *testing.T) {
	seedancePlatform := constant.TaskPlatform(fmt.Sprintf("%d", constant.ChannelTypeSeedance))
	require.True(t, preservesMissingUpstreamIDUntilTimeout(seedancePlatform))
	require.False(t, preservesMissingUpstreamIDUntilTimeout(constant.TaskPlatform("other")))
}

type fakeEnhancementProvider struct {
	submit *EnhancementResult
	query  *EnhancementResult
}

type unknownSubmissionProvider struct {
	idempotencyKeys []string
}

func (p *unknownSubmissionProvider) Submit(_ context.Context, request EnhancementSubmitRequest) (*EnhancementResult, error) {
	p.idempotencyKeys = append(p.idempotencyKeys, request.IdempotencyKey)
	return nil, errors.New("connection closed before the provider response")
}

func (p *unknownSubmissionProvider) Query(context.Context, string) (*EnhancementResult, error) {
	return nil, errors.New("query should not run before an execution task id exists")
}

func (p *unknownSubmissionProvider) Cancel(context.Context, string) error {
	return nil
}

func (p *fakeEnhancementProvider) Submit(context.Context, EnhancementSubmitRequest) (*EnhancementResult, error) {
	return p.submit, nil
}

func (p *fakeEnhancementProvider) Query(context.Context, string) (*EnhancementResult, error) {
	return p.query, nil
}

func (p *fakeEnhancementProvider) Cancel(context.Context, string) error {
	return nil
}

func TestSeedanceWorkflowHidesEnhancementAndSynchronizesOutboxOnce(t *testing.T) {
	truncate(t)
	require.NoError(t, model.DB.AutoMigrate(
		&model.SeedanceChannelConfig{},
		&model.SeedanceVolcengineCredential{},
		&model.MediaEnhancementProvider{},
		&model.SeedanceModelOffering{},
		&model.SeedanceOrder{},
		&model.SeedanceCustomerRefund{},
		&model.MediaServiceUsage{},
		&model.SeedanceAttempt{},
		&model.ServiceBillingEvent{},
		&model.ServiceBillingOutbox{},
		&model.ServiceBillingFailureAttempt{},
		&model.SeedanceAdminAudit{},
		&model.SeedanceVolcengineBillItem{},
		&model.SeedanceCostAllocation{},
	))
	for _, table := range []string{
		"seedance_channel_configs", "seedance_volcengine_credentials", "media_enhancement_providers",
		"seedance_model_offerings", "seedance_orders", "seedance_customer_refunds", "media_service_usages", "seedance_attempts",
		"service_billing_events", "service_billing_outboxes", "seedance_admin_audits",
		"service_billing_failure_attempts",
		"seedance_volcengine_bill_items", "seedance_cost_allocations",
	} {
		require.NoError(t, model.DB.Exec("DELETE FROM "+table).Error)
	}

	previousSecret, previousConfigured := common.CryptoSecret, common.CryptoSecretConfigured
	common.CryptoSecret = strings.Repeat("s", 32)
	common.CryptoSecretConfigured = true
	t.Cleanup(func() {
		common.CryptoSecret = previousSecret
		common.CryptoSecretConfigured = previousConfigured
	})

	var received atomic.Int32
	var billingStatus atomic.Int32
	billingStatus.Store(http.StatusServiceUnavailable)
	billing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received.Add(1)
		require.Equal(t, "Bearer billing-secret", r.Header.Get("Authorization"))
		require.Equal(t, "30000000-0000-0000-0000-000000000001", r.Header.Get("X-NewAPI-Instance-ID"))
		require.Contains(t, r.Header.Get("Idempotency-Key"), "service-usage:")
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.Contains(t, string(body), `"source_revision":1`)
		require.NotContains(t, string(body), "https://supplier.invalid/result.mp4")
		w.Header().Set("Content-Type", "application/json")
		if status := int(billingStatus.Load()); status != http.StatusOK {
			w.WriteHeader(status)
			return
		}
		_, _ = w.Write([]byte(`{"code":0,"message":"success","data":{"accepted":true,"duplicate":false,"ledger_line_item_id":"line-1","debit_status":"CHARGED","balance_status":"NORMAL"}}`))
	}))
	defer billing.Close()
	callbackReceived := 0
	callback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callbackReceived++
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.Contains(t, string(body), "/v1/videos/task_public_seedance/content")
		require.Contains(t, string(body), `"total_tokens":1358000`)
		require.NotContains(t, strings.ToLower(string(body)), "provider")
		w.WriteHeader(http.StatusNoContent)
	}))
	defer callback.Close()
	fetchSetting := system_setting.GetFetchSetting()
	previousSSRF := fetchSetting.EnableSSRFProtection
	fetchSetting.EnableSSRFProtection = false
	t.Cleanup(func() { fetchSetting.EnableSSRFProtection = previousSSRF })

	billingKey, err := common.EncryptSensitiveValue("billing-secret")
	require.NoError(t, err)
	arkKey, err := common.EncryptSensitiveValue("ark-secret")
	require.NoError(t, err)
	providerKey, err := common.EncryptSensitiveValue("provider-secret")
	require.NoError(t, err)
	baseURL := "https://ark.cn-beijing.volces.com"
	channel := &model.Channel{Id: 901, Type: constant.ChannelTypeSeedance, Name: "Seedance", Key: "managed", Models: "Seedance 2.5", Group: "default", Status: common.ChannelStatusEnabled, BaseURL: &baseURL}
	require.NoError(t, model.DB.Create(channel).Error)
	config := &model.SeedanceChannelConfig{
		ChannelID: 901, InstanceID: "30000000-0000-0000-0000-000000000001", AIPDDBillingBaseURL: billing.URL,
		AIPDDBillingCredentialEncrypted: billingKey, Status: model.SeedanceConfigActive,
		LastVerifiedAt: time.Now().Unix(), CreatedAt: time.Now().Unix(), UpdatedAt: time.Now().Unix(),
	}
	require.NoError(t, model.DB.Create(config).Error)
	credential := &model.SeedanceVolcengineCredential{
		ChannelID: 901, Version: 1, ArkAPIKeyEncrypted: arkKey, Fingerprint: "sha256:test", MaskedSuffix: "cret",
		Status: model.SeedanceCredentialActive, ValidatedAt: time.Now().Unix(), CreatedAt: time.Now().Unix(),
	}
	require.NoError(t, model.DB.Create(credential).Error)
	provider := &model.MediaEnhancementProvider{
		ProviderType: model.SeedanceProviderDirect, DisplayName: "private supplier", ServiceEndpoint: "https://supplier.invalid/tasks",
		CredentialEncrypted: providerKey, ServiceCode: "video_sr_v1", Status: model.SeedanceConfigActive,
		TimeoutPolicyJSON: `{}`, RetryPolicyJSON: `{}`, FallbackPolicyJSON: `{}`, CapabilitiesJSON: `{}`,
	}
	require.NoError(t, model.DB.Create(provider).Error)
	offering := &model.SeedanceModelOffering{
		ChannelID: 901, DisplayName: "Seedance 2.5", ProviderModelID: "doubao-seedance-2-5-test",
		EnhancementProviderID: provider.ID, EnhancementServiceCode: "video_sr_v1",
		EnhancementSpecificationJSON: `{"target_resolution":"3840x2160"}`, EnhancementSpecificationVersion: "spec-v1",
		ModelSaleMicroRMB: 8_000_000, ServiceChargeMicroRMB: 1_800_000,
		ProviderCostMicroRMB: pointerInt64(1_200_000), VolcengineUnitCostMicroRMB: 3_000_000,
		PricingVersion: "price-v1", Enabled: true, PublishedAt: time.Now().Unix(),
	}
	require.NoError(t, model.DB.Create(offering).Error)

	task := &model.Task{
		TaskID: "task_public_seedance", Platform: constant.TaskPlatform("59"), UserId: 22, ChannelId: 901,
		Status: model.TaskStatusNotStart, Progress: "0%", Quota: 100,
		Properties:  model.Properties{OriginModelName: "Seedance 2.5", UpstreamModelName: "doubao-seedance-2-5-test"},
		PrivateData: model.TaskPrivateData{TokenId: 33, BillingContext: &model.TaskBillingContext{QuotaPerUnit: common.QuotaPerUnit, USDExchangeRate: 7.3}},
	}
	pricingSnapshot, err := common.Marshal(map[string]any{
		"pricing_version": "price-v1", "service_charge_micro_rmb": int64(1_800_000),
		"provider_type": model.SeedanceProviderDirect, "provider_id": provider.ID,
		"service_code": "video_sr_v1", "specification": `{"target_resolution":"3840x2160"}`,
		"specification_version": "spec-v1", "provider_cost_micro_rmb": int64(1_200_000),
	})
	require.NoError(t, err)
	_, err = model.InsertTaskWithSeedanceOrder(model.SeedanceOrderCreate{
		Task: task, Config: config, Credential: credential, Offering: offering, Provider: provider,
		PricingSnapshot: string(pricingSnapshot), RequestFactsJSON: `{}`, GenerationTaskID: "ark-task-1",
		PublicProtocol: model.SeedanceProtocolOfficial, CallbackURL: callback.URL,
	})
	require.NoError(t, err)
	mutatedProviderKey, err := common.EncryptSensitiveValue("rotated-provider-secret")
	require.NoError(t, err)
	require.NoError(t, model.DB.Model(&model.MediaEnhancementProvider{}).Where("id = ?", provider.ID).Updates(map[string]any{
		"service_endpoint": "https://changed.invalid/tasks", "credential_encrypted": mutatedProviderKey,
		"status": model.SeedanceConfigDisabled,
	}).Error)

	fake := &fakeEnhancementProvider{
		submit: &EnhancementResult{ExecutionTaskID: "private-execution-1", Status: model.SeedanceUsageRunning},
		query: &EnhancementResult{
			ExecutionTaskID: "private-execution-1", Status: model.SeedanceUsageSucceeded,
			ResultURL: "https://supplier.invalid/result.mp4", UsageEvidenceJSON: `{"seconds":5}`,
		},
	}
	previousFactory := enhancementProviderFactory
	enhancementProviderFactory = func(config *model.MediaEnhancementProvider) (EnhancementProvider, error) {
		require.Equal(t, "https://supplier.invalid/tasks", config.ServiceEndpoint)
		secret, decryptErr := common.DecryptSensitiveValue(config.CredentialEncrypted)
		require.NoError(t, decryptErr)
		require.Equal(t, "provider-secret", secret)
		return fake, nil
	}
	t.Cleanup(func() { enhancementProviderFactory = previousFactory })

	err = StartSeedanceEnhancement(context.Background(), task, &relaycommon.TaskInfo{
		Status: model.TaskStatusSuccess, Url: "https://ark.invalid/generated.mp4",
	}, []byte(`{"id":"ark-task-1","status":"succeeded","usage":{"completion_tokens":1358000,"total_tokens":1358000,"tool_usage":{"search":1}}}`))
	require.NoError(t, err)

	var processing model.Task
	require.NoError(t, model.DB.Where("id = ?", task.ID).First(&processing).Error)
	require.Equal(t, model.TaskStatus(model.TaskStatusInProgress), processing.Status)
	require.Equal(t, "80%", processing.Progress)
	require.NotContains(t, string(processing.Data), "enhancement")
	require.NotContains(t, string(processing.Data), "provider")

	handled, err := HandleSeedanceWorkflowPoll(context.Background(), channel, &processing)
	require.True(t, handled)
	require.NoError(t, err)

	var completed model.Task
	require.NoError(t, model.DB.Where("id = ?", task.ID).First(&completed).Error)
	require.Equal(t, model.TaskStatus(model.TaskStatusSuccess), completed.Status)
	require.Equal(t, "https://supplier.invalid/result.mp4", completed.PrivateData.ResultURL)
	publicJSON := string(completed.Data)
	require.Contains(t, publicJSON, "/v1/videos/task_public_seedance/content")
	require.NotContains(t, publicJSON, "supplier")
	require.NotContains(t, publicJSON, "enhancement")
	require.NotContains(t, publicJSON, "super_resolution")
	require.Contains(t, publicJSON, `"completion_tokens":1358000`)
	require.NotContains(t, publicJSON, "tool_usage")
	var completedOrder model.SeedanceOrder
	require.NoError(t, model.DB.Where("new_api_task_id = ?", task.TaskID).First(&completedOrder).Error)
	require.Equal(t, model.SeedanceCallbackReady, completedOrder.CallbackStatus)
	require.Equal(t, 1, completedOrder.FinanceRevision)
	serializedOrder, err := common.Marshal(completedOrder)
	require.NoError(t, err)
	require.NotContains(t, string(serializedOrder), callback.URL)
	require.NotContains(t, string(serializedOrder), completedOrder.CallbackURLEncrypted)

	ProcessSeedanceCallbacks(context.Background(), 20)
	require.Equal(t, 1, callbackReceived)
	ProcessSeedanceCallbacks(context.Background(), 20)
	require.Equal(t, 1, callbackReceived)

	var crashedLease model.ServiceBillingOutbox
	require.NoError(t, model.DB.First(&crashedLease).Error)
	require.NoError(t, model.DB.Model(&model.ServiceBillingOutbox{}).Where("id = ?", crashedLease.ID).Updates(map[string]any{
		"status": model.SeedanceSyncSending, "lease_owner": "dead-worker", "lease_until": time.Now().Unix() - 1,
	}).Error)
	ProcessSeedanceBillingOutbox(context.Background(), 20)
	require.EqualValues(t, 1, received.Load())
	var retrying model.ServiceBillingOutbox
	require.NoError(t, model.DB.First(&retrying).Error)
	require.Equal(t, model.SeedanceSyncRetryWait, retrying.Status)
	require.Equal(t, 1, retrying.AttemptCount)
	var stillCompleted model.Task
	require.NoError(t, model.DB.Where("id = ?", task.ID).First(&stillCompleted).Error)
	require.Equal(t, model.TaskStatus(model.TaskStatusSuccess), stillCompleted.Status)
	require.NoError(t, model.DB.Model(&model.ServiceBillingOutbox{}).Where("id = ?", retrying.ID).
		Update("next_attempt_at", 0).Error)
	billingStatus.Store(http.StatusOK)
	ProcessSeedanceBillingOutbox(context.Background(), 20)
	require.EqualValues(t, 2, received.Load())
	ProcessSeedanceBillingOutbox(context.Background(), 20)
	require.EqualValues(t, 2, received.Load())
	var outbox model.ServiceBillingOutbox
	require.NoError(t, model.DB.First(&outbox).Error)
	require.Equal(t, model.SeedanceSyncSynced, outbox.Status)
	var eventCount int64
	require.NoError(t, model.DB.Model(&model.ServiceBillingEvent{}).Count(&eventCount).Error)
	require.EqualValues(t, 1, eventCount)

	var originalEvent model.ServiceBillingEvent
	require.NoError(t, model.DB.First(&originalEvent).Error)
	siblingEvent := originalEvent
	siblingEvent.ID = 0
	siblingEvent.EventID = "evt_auth_pause_sibling"
	siblingEvent.SourceRevisionKey = nil
	require.NoError(t, model.DB.Create(&siblingEvent).Error)
	require.NoError(t, model.DB.Create(&model.ServiceBillingOutbox{
		EventID: siblingEvent.EventID, Status: model.SeedanceSyncReady, CreatedAt: time.Now().Unix(), UpdatedAt: time.Now().Unix(),
	}).Error)

	billingStatus.Store(http.StatusUnauthorized)
	require.NoError(t, model.DB.Model(&model.ServiceBillingOutbox{}).Where("id = ?", outbox.ID).Updates(map[string]any{
		"status": model.SeedanceSyncReady, "next_attempt_at": 0,
	}).Error)
	ProcessSeedanceBillingOutbox(context.Background(), 20)
	require.EqualValues(t, 3, received.Load())
	var authPaused model.ServiceBillingOutbox
	require.NoError(t, model.DB.Where("id = ?", outbox.ID).First(&authPaused).Error)
	require.Equal(t, seedanceOutboxAuthPaused, authPaused.Status)
	require.Equal(t, http.StatusUnauthorized, authPaused.LastHTTPStatus)
	var pausedOutboxes []model.ServiceBillingOutbox
	require.NoError(t, model.DB.Order("id asc").Find(&pausedOutboxes).Error)
	require.Len(t, pausedOutboxes, 2)
	require.Equal(t, model.SeedanceSyncAuthPaused, pausedOutboxes[0].Status)
	require.Equal(t, model.SeedanceSyncAuthPaused, pausedOutboxes[1].Status)
	pausedConfig, err := model.GetSeedanceChannelConfig(901)
	require.NoError(t, err)
	require.Positive(t, pausedConfig.BillingAuthPausedAt)
	require.Equal(t, http.StatusUnauthorized, pausedConfig.BillingAuthLastHTTPStatus)
	var failureAttempts []model.ServiceBillingFailureAttempt
	require.NoError(t, model.DB.Order("attempt_no asc").Find(&failureAttempts).Error)
	require.Len(t, failureAttempts, 2)
	require.Equal(t, http.StatusServiceUnavailable, failureAttempts[0].HTTPStatus)
	require.Equal(t, http.StatusUnauthorized, failureAttempts[1].HTTPStatus)
	ProcessSeedanceBillingOutbox(context.Background(), 20)
	require.EqualValues(t, 3, received.Load())
	require.NoError(t, model.ReplayServiceBillingOutbox(authPaused.EventID, 1))
	resumedConfig, err := model.GetSeedanceChannelConfig(901)
	require.NoError(t, err)
	require.Zero(t, resumedConfig.BillingAuthPausedAt)
	require.Zero(t, resumedConfig.BillingAuthLastHTTPStatus)
	require.NoError(t, model.DB.Order("id asc").Find(&pausedOutboxes).Error)
	require.Equal(t, model.SeedanceSyncReady, pausedOutboxes[0].Status)
	require.Equal(t, model.SeedanceSyncReady, pausedOutboxes[1].Status)

	// Simulate a process crash immediately after a confirmed bill allocation
	// committed but before its finance revision/outbox was created.
	billItem := &model.SeedanceVolcengineBillItem{
		ChannelID: 901, BillDetailID: "bill-recovery-1", Revision: 1, BillingPeriod: "2026-09",
		ProductCode: "verified-product", AmountMicroRMB: 3_000_000,
		SourcePayloadHash: "sha256:bill", SanitizedSourceJSON: `{}`, AllocationStatus: model.SeedanceBillAllocated,
	}
	require.NoError(t, model.DB.Create(billItem).Error)
	require.NoError(t, model.DB.Create(&model.SeedanceCostAllocation{
		BillItemID: billItem.ID, PlatformOrderID: completedOrder.PlatformOrderID, BillRevision: 1,
		Weight: 1, AllocatedMicroRMB: 3_000_000, RemainderRank: 1, RuleVersion: "largest_remainder_order_id_v1",
	}).Error)
	ProcessSeedanceCostRevisionQueue(context.Background(), 20)
	ProcessSeedanceCostRevisionQueue(context.Background(), 20)
	require.NoError(t, model.DB.First(billItem, billItem.ID).Error)
	require.Positive(t, billItem.RevisionEventQueuedAt)
	require.NoError(t, model.DB.Model(&model.ServiceBillingEvent{}).Count(&eventCount).Error)
	require.EqualValues(t, 3, eventCount)
	var outboxCount int64
	require.NoError(t, model.DB.Model(&model.ServiceBillingOutbox{}).Count(&outboxCount).Error)
	require.EqualValues(t, 3, outboxCount)
}

func TestSeedanceEnhancementFailureRefundsSaleAndServiceChargeButKeepsOccurredCosts(t *testing.T) {
	truncate(t)
	require.NoError(t, model.DB.AutoMigrate(
		&model.SeedanceChannelConfig{}, &model.SeedanceVolcengineCredential{},
		&model.MediaEnhancementProvider{}, &model.SeedanceModelOffering{},
		&model.SeedanceOrder{}, &model.SeedanceCustomerRefund{}, &model.MediaServiceUsage{},
		&model.SeedanceAttempt{}, &model.ServiceBillingEvent{}, &model.ServiceBillingOutbox{},
	))
	for _, table := range []string{
		"service_billing_outboxes", "service_billing_events", "media_service_usages", "seedance_attempts",
		"seedance_customer_refunds", "seedance_orders", "seedance_model_offerings",
		"media_enhancement_providers", "seedance_volcengine_credentials", "seedance_channel_configs",
	} {
		require.NoError(t, model.DB.Exec("DELETE FROM "+table).Error)
	}

	previousSecret, previousConfigured := common.CryptoSecret, common.CryptoSecretConfigured
	common.CryptoSecret = strings.Repeat("f", 32)
	common.CryptoSecretConfigured = true
	t.Cleanup(func() {
		common.CryptoSecret = previousSecret
		common.CryptoSecretConfigured = previousConfigured
	})

	const (
		userID       = 9402
		tokenID      = 9402
		channelID    = 9401
		chargedQuota = 500
	)
	seedUser(t, userID, 9_500, chargedQuota)
	require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", userID).Update("request_count", 1).Error)
	seedToken(t, tokenID, userID, "sk-seedance-enhancement-failure", 4_500)
	require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", tokenID).Update("used_quota", chargedQuota).Error)
	baseURL := "https://ark.cn-beijing.volces.com"
	channel := &model.Channel{
		Id: channelID, Type: constant.ChannelTypeSeedance, Name: "Seedance failure contract",
		Key: "managed", Models: "Public video", Group: "default", Status: common.ChannelStatusEnabled,
		UsedQuota: chargedQuota, BaseURL: &baseURL,
	}
	require.NoError(t, model.DB.Create(channel).Error)

	billingKey, err := common.EncryptSensitiveValue("billing-failure-contract")
	require.NoError(t, err)
	arkKey, err := common.EncryptSensitiveValue("ark-failure-contract")
	require.NoError(t, err)
	providerKey, err := common.EncryptSensitiveValue("provider-failure-contract")
	require.NoError(t, err)
	now := time.Now().Unix()
	config := &model.SeedanceChannelConfig{
		ChannelID: channelID, Revision: 1, InstanceID: "30000000-0000-0000-0000-000000000401",
		AIPDDBillingBaseURL: "https://billing.invalid", AIPDDBillingCredentialEncrypted: billingKey,
		Status: model.SeedanceConfigActive, CreatedAt: now, UpdatedAt: now,
	}
	require.NoError(t, model.DB.Create(config).Error)
	credential := &model.SeedanceVolcengineCredential{
		ChannelID: channelID, Version: 1, ArkAPIKeyEncrypted: arkKey,
		Fingerprint: "sha256:enhancement-failure", MaskedSuffix: "ract",
		Status: model.SeedanceCredentialActive, ValidatedAt: now, CreatedAt: now,
	}
	require.NoError(t, model.DB.Create(credential).Error)
	provider := &model.MediaEnhancementProvider{
		ProviderType: model.SeedanceProviderDirect, DisplayName: "private failure supplier",
		ServiceEndpoint: "https://supplier.invalid/tasks", CredentialEncrypted: providerKey,
		ServiceCode: "video_sr_failure_contract", Status: model.SeedanceConfigActive,
		TimeoutPolicyJSON: `{}`, RetryPolicyJSON: `{}`, FallbackPolicyJSON: `{}`, CapabilitiesJSON: `{}`,
	}
	require.NoError(t, model.DB.Create(provider).Error)
	offering := &model.SeedanceModelOffering{
		ChannelID: channelID, DisplayName: "Public video", ProviderModelID: "private-ark-model",
		EnhancementProviderID: provider.ID, EnhancementServiceCode: provider.ServiceCode,
		EnhancementSpecificationJSON:    `{"target_resolution":"3840x2160"}`,
		EnhancementSpecificationVersion: "spec-failure-v1",
		ModelSaleMicroRMB:               8_000_000, ServiceChargeMicroRMB: 1_800_000,
		ProviderCostMicroRMB: pointerInt64(1_200_000), VolcengineUnitCostMicroRMB: 3_000_000,
		PricingVersion: "price-failure-v1", Enabled: true, PublishedAt: now,
	}
	require.NoError(t, model.DB.Create(offering).Error)

	task := &model.Task{
		TaskID: "task-seedance-enhancement-failure", Platform: constant.TaskPlatform("59"),
		UserId: userID, ChannelId: channelID, Quota: chargedQuota, Group: "default",
		Status: model.TaskStatusInProgress, Progress: "50%", CreatedAt: now, UpdatedAt: now,
		Properties: model.Properties{OriginModelName: offering.DisplayName, UpstreamModelName: offering.ProviderModelID},
		PrivateData: model.TaskPrivateData{
			TokenId: tokenID, BillingSource: BillingSourceWallet,
			BillingContext: &model.TaskBillingContext{OriginModelName: offering.DisplayName, QuotaPerUnit: common.QuotaPerUnit, USDExchangeRate: 7.3},
		},
	}
	task.SetData(map[string]any{"id": task.TaskID, "model": offering.DisplayName, "status": "running"})
	pricingSnapshot, err := common.Marshal(map[string]any{
		"pricing_version": offering.PricingVersion, "service_charge_micro_rmb": offering.ServiceChargeMicroRMB,
		"provider_type": provider.ProviderType, "provider_id": provider.ID,
		"service_code": provider.ServiceCode, "specification": offering.EnhancementSpecificationJSON,
		"specification_version":   offering.EnhancementSpecificationVersion,
		"provider_cost_micro_rmb": int64(1_200_000),
	})
	require.NoError(t, err)
	order, err := model.InsertTaskWithSeedanceOrder(model.SeedanceOrderCreate{
		Task: task, Config: config, Credential: credential, Offering: offering, Provider: provider,
		PricingSnapshot: string(pricingSnapshot), RequestFactsJSON: `{}`, GenerationTaskID: "ark-task-failure-contract",
		PublicProtocol: model.SeedanceProtocolOpenAI,
	})
	require.NoError(t, err)

	fake := &fakeEnhancementProvider{
		submit: &EnhancementResult{ExecutionTaskID: "provider-task-failure-contract", Status: model.SeedanceUsageRunning},
		query: &EnhancementResult{
			ExecutionTaskID: "provider-task-failure-contract", Status: model.SeedanceUsageFailed,
			FailureReason: "SUPPLIER_REJECTED_AFTER_ACCEPT",
		},
	}
	previousFactory := enhancementProviderFactory
	enhancementProviderFactory = func(*model.MediaEnhancementProvider) (EnhancementProvider, error) { return fake, nil }
	t.Cleanup(func() { enhancementProviderFactory = previousFactory })

	require.NoError(t, StartSeedanceEnhancement(context.Background(), task, &relaycommon.TaskInfo{
		Status: model.TaskStatusSuccess, Url: "https://ark.invalid/generated-failure-contract.mp4",
	}, []byte(`{"id":"ark-task-failure-contract","status":"succeeded"}`)))
	var processing model.Task
	require.NoError(t, model.DB.Where("task_id = ?", task.TaskID).First(&processing).Error)
	handled, err := HandleSeedanceWorkflowPoll(context.Background(), channel, &processing)
	require.True(t, handled)
	require.NoError(t, err)

	var failedTask model.Task
	require.NoError(t, model.DB.Where("task_id = ?", task.TaskID).First(&failedTask).Error)
	require.Equal(t, model.TaskStatus(model.TaskStatusFailure), failedTask.Status)
	require.Equal(t, seedanceGenericFailureMessage, failedTask.FailReason)
	require.NotContains(t, strings.ToLower(string(failedTask.Data)), "supplier")
	require.NotContains(t, strings.ToLower(string(failedTask.Data)), "provider")

	var failedOrder model.SeedanceOrder
	require.NoError(t, model.DB.First(&failedOrder, order.ID).Error)
	require.Equal(t, model.SeedanceOrderFailed, failedOrder.OrderStatus)
	require.Zero(t, failedOrder.ModelSaleMicroRMB)
	require.Zero(t, failedOrder.ServiceChargeTotalMicroRMB)
	require.EqualValues(t, 3_000_000, failedOrder.VolcengineEstimatedMicroRMB)
	require.EqualValues(t, -3_000_000, failedOrder.NewAPIEstimatedProfitMicroRMB)
	require.Equal(t, model.SeedanceCostEstimated, failedOrder.VolcengineCostStatus)
	require.Equal(t, 1, failedOrder.FinanceRevision)

	var usage model.MediaServiceUsage
	require.NoError(t, model.DB.Where("platform_order_id = ?", order.PlatformOrderID).First(&usage).Error)
	require.Equal(t, model.SeedanceUsageFailed, usage.Status)
	require.Zero(t, usage.ChargeMicroRMB)
	require.NotNil(t, usage.ProviderCostMicroRMB)
	require.EqualValues(t, 1_200_000, *usage.ProviderCostMicroRMB)
	require.Equal(t, "SUPPLIER_REJECTED_AFTER_ACCEPT", usage.FailureReason)
	require.Equal(t, "provider-task-failure-contract", usage.ExecutionTaskID)

	var event model.ServiceBillingEvent
	require.NoError(t, model.DB.Where("platform_order_id = ?", order.PlatformOrderID).First(&event).Error)
	var payload map[string]any
	require.NoError(t, common.UnmarshalJsonStr(event.PayloadJSON, &payload))
	sourceOrder := payload["source_order"].(map[string]any)
	serviceUsage := payload["service_usage"].(map[string]any)
	require.EqualValues(t, 1, sourceOrder["source_revision"])
	require.Equal(t, "0.000000", sourceOrder["model_sale_rmb"])
	require.Equal(t, "3.000000", sourceOrder["volcengine_cost_estimated_rmb"])
	require.Equal(t, "0.000000", serviceUsage["charge_rmb"])
	require.Equal(t, "1.200000", serviceUsage["provider_cost_rmb"])
	require.Equal(t, model.SeedanceUsageFailed, serviceUsage["status"])

	ProcessSeedanceCustomerRefunds(context.Background(), 20)
	ProcessSeedanceCustomerRefunds(context.Background(), 20)
	require.Equal(t, 10_000, getUserQuota(t, userID))
	require.Zero(t, getUserUsedQuota(t, userID))
	require.Equal(t, 5_000, getTokenRemainQuota(t, tokenID))
	require.Zero(t, getTokenUsedQuota(t, tokenID))
	require.Zero(t, getChannelUsedQuota(t, channelID))

	var refunds int64
	var events int64
	var outboxes int64
	var refundLogs int64
	require.NoError(t, model.DB.Model(&model.SeedanceCustomerRefund{}).Where("platform_order_id = ?", order.PlatformOrderID).Count(&refunds).Error)
	require.NoError(t, model.DB.Model(&model.ServiceBillingEvent{}).Where("platform_order_id = ?", order.PlatformOrderID).Count(&events).Error)
	require.NoError(t, model.DB.Model(&model.ServiceBillingOutbox{}).Where("event_id = ?", event.EventID).Count(&outboxes).Error)
	require.NoError(t, model.LOG_DB.Model(&model.Log{}).Where("billing_event_key = ?", "seedance-customer-refund:"+order.PlatformOrderID).Count(&refundLogs).Error)
	require.EqualValues(t, 1, refunds)
	require.EqualValues(t, 1, events)
	require.EqualValues(t, 1, outboxes)
	require.EqualValues(t, 1, refundLogs)

	handled, err = HandleSeedanceWorkflowPoll(context.Background(), channel, &failedTask)
	require.True(t, handled)
	require.NoError(t, err)
	require.NoError(t, model.DB.Model(&model.ServiceBillingEvent{}).Where("platform_order_id = ?", order.PlatformOrderID).Count(&events).Error)
	require.EqualValues(t, 1, events)
}

func TestSeedanceCallbackFailureRetriesExpiredLeaseAndDeadLettersWithoutChangingTask(t *testing.T) {
	truncate(t)
	require.NoError(t, model.DB.AutoMigrate(&model.Task{}, &model.SeedanceOrder{}))
	require.NoError(t, model.DB.Exec("DELETE FROM seedance_orders").Error)
	require.NoError(t, model.DB.Exec("DELETE FROM tasks").Error)

	previousSecret, previousConfigured := common.CryptoSecret, common.CryptoSecretConfigured
	common.CryptoSecret = strings.Repeat("c", 32)
	common.CryptoSecretConfigured = true
	t.Cleanup(func() {
		common.CryptoSecret = previousSecret
		common.CryptoSecretConfigured = previousConfigured
	})
	fetchSetting := system_setting.GetFetchSetting()
	previousSSRF := fetchSetting.EnableSSRFProtection
	fetchSetting.EnableSSRFProtection = false
	t.Cleanup(func() { fetchSetting.EnableSSRFProtection = previousSSRF })

	var received atomic.Int32
	callback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received.Add(1)
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "task_callback_retry", r.Header.Get("X-NewAPI-Task-ID"))
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer callback.Close()
	callbackCipher, err := common.EncryptSensitiveValue(callback.URL)
	require.NoError(t, err)

	now := time.Now().Unix()
	task := &model.Task{
		TaskID: "task_callback_retry", Platform: constant.TaskPlatform("59"), UserId: 77, ChannelId: 907,
		Status: model.TaskStatusFailure, Progress: "100%", Data: []byte(`{"id":"task_callback_retry","status":"failed"}`),
		Properties: model.Properties{OriginModelName: "Public video"}, CreatedAt: now - 10, UpdatedAt: now,
	}
	require.NoError(t, model.DB.Create(task).Error)
	order := &model.SeedanceOrder{
		PlatformOrderID: "order-callback-retry", NewAPITaskID: task.TaskID, ChannelID: task.ChannelId,
		Model: "Public video", OrderStatus: model.SeedanceOrderFailed,
		VolcengineCostStatus: model.SeedanceCostEstimated, SyncStatus: model.SeedanceSyncPending,
		PublicProtocol: model.SeedanceProtocolOfficial, CallbackURLEncrypted: callbackCipher,
		CallbackStatus: model.SeedanceCallbackReady, CallbackNextAttemptAt: 0,
		CreatedAt: now - 10, CompletedAt: now, UpdatedAt: now,
	}
	require.NoError(t, model.DB.Create(order).Error)

	ProcessSeedanceCallbacks(context.Background(), 20)
	require.EqualValues(t, 1, received.Load())
	require.NoError(t, model.DB.First(order, order.ID).Error)
	require.Equal(t, model.SeedanceCallbackRetryWait, order.CallbackStatus)
	require.Equal(t, 1, order.CallbackAttemptCount)
	require.Greater(t, order.CallbackNextAttemptAt, now)
	require.Empty(t, order.CallbackLeaseOwner)
	require.Zero(t, order.CallbackLeaseUntil)
	require.Equal(t, http.StatusServiceUnavailable, order.CallbackLastHTTPStatus)
	require.Equal(t, "callback returned HTTP 503", order.CallbackLastError)

	// A worker can reclaim an expired SENDING lease and must increment the same
	// callback attempt counter rather than creating a parallel delivery record.
	require.NoError(t, model.DB.Model(&model.SeedanceOrder{}).Where("id = ?", order.ID).Updates(map[string]any{
		"callback_status": model.SeedanceCallbackSending, "callback_lease_owner": "dead-worker",
		"callback_lease_until": time.Now().Unix() - 1,
	}).Error)
	ProcessSeedanceCallbacks(context.Background(), 20)
	require.EqualValues(t, 2, received.Load())
	require.NoError(t, model.DB.First(order, order.ID).Error)
	require.Equal(t, model.SeedanceCallbackRetryWait, order.CallbackStatus)
	require.Equal(t, 2, order.CallbackAttemptCount)

	// The eighth failed delivery is terminal and is not selected again.
	require.NoError(t, model.DB.Model(&model.SeedanceOrder{}).Where("id = ?", order.ID).Updates(map[string]any{
		"callback_status": model.SeedanceCallbackReady, "callback_next_attempt_at": 0,
		"callback_attempt_count": 7,
	}).Error)
	ProcessSeedanceCallbacks(context.Background(), 20)
	require.EqualValues(t, 3, received.Load())
	require.NoError(t, model.DB.First(order, order.ID).Error)
	require.Equal(t, model.SeedanceCallbackDeadLetter, order.CallbackStatus)
	require.Equal(t, 8, order.CallbackAttemptCount)
	require.Equal(t, http.StatusServiceUnavailable, order.CallbackLastHTTPStatus)
	require.Empty(t, order.CallbackLeaseOwner)
	require.Zero(t, order.CallbackLeaseUntil)
	ProcessSeedanceCallbacks(context.Background(), 20)
	require.EqualValues(t, 3, received.Load())

	var unchangedTask model.Task
	require.NoError(t, model.DB.Where("task_id = ?", task.TaskID).First(&unchangedTask).Error)
	require.Equal(t, model.TaskStatus(model.TaskStatusFailure), unchangedTask.Status)
	require.JSONEq(t, `{"id":"task_callback_retry","status":"failed"}`, string(unchangedTask.Data))
}

func TestSeedanceCallbackHonorsContextDeadlineAndRecordsTransportFailure(t *testing.T) {
	truncate(t)
	require.NoError(t, model.DB.AutoMigrate(&model.Task{}, &model.SeedanceOrder{}))
	require.NoError(t, model.DB.Exec("DELETE FROM seedance_orders").Error)
	require.NoError(t, model.DB.Exec("DELETE FROM tasks").Error)

	previousSecret, previousConfigured := common.CryptoSecret, common.CryptoSecretConfigured
	common.CryptoSecret = strings.Repeat("d", 32)
	common.CryptoSecretConfigured = true
	t.Cleanup(func() {
		common.CryptoSecret = previousSecret
		common.CryptoSecretConfigured = previousConfigured
	})
	fetchSetting := system_setting.GetFetchSetting()
	previousSSRF := fetchSetting.EnableSSRFProtection
	fetchSetting.EnableSSRFProtection = false
	t.Cleanup(func() { fetchSetting.EnableSSRFProtection = previousSSRF })

	requestStarted := make(chan struct{}, 1)
	releaseHandler := make(chan struct{})
	callback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requestStarted <- struct{}{}
		<-releaseHandler
	}))
	defer callback.Close()
	callbackCipher, err := common.EncryptSensitiveValue(callback.URL)
	require.NoError(t, err)

	now := time.Now().Unix()
	task := model.Task{
		TaskID: "task_callback_deadline", Status: model.TaskStatusSuccess, Progress: "100%",
		Data: []byte(`{"id":"task_callback_deadline","status":"succeeded"}`), CreatedAt: now, UpdatedAt: now,
	}
	require.NoError(t, model.DB.Create(&task).Error)
	order := model.SeedanceOrder{
		PlatformOrderID: "order_callback_deadline", NewAPITaskID: task.TaskID,
		PublicProtocol: model.SeedanceProtocolOpenAI, OrderStatus: model.SeedanceOrderSucceeded,
		CallbackURLEncrypted: callbackCipher, CallbackStatus: model.SeedanceCallbackReady,
		CallbackNextAttemptAt: now, CreatedAt: now, UpdatedAt: now,
	}
	require.NoError(t, model.DB.Create(&order).Error)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	startedAt := time.Now()
	ProcessSeedanceCallbacks(ctx, 1)
	close(releaseHandler)
	require.Less(t, time.Since(startedAt), 2*time.Second)
	select {
	case <-requestStarted:
	default:
		t.Fatal("callback request did not reach the test server")
	}

	var stored model.SeedanceOrder
	require.NoError(t, model.DB.First(&stored, order.ID).Error)
	require.Equal(t, model.SeedanceCallbackRetryWait, stored.CallbackStatus)
	require.Equal(t, 1, stored.CallbackAttemptCount)
	require.Equal(t, 0, stored.CallbackLastHTTPStatus)
	require.Contains(t, strings.ToLower(stored.CallbackLastError), "deadline")
	require.Empty(t, stored.CallbackLeaseOwner)
	require.Zero(t, stored.CallbackLeaseUntil)
	require.Greater(t, stored.CallbackNextAttemptAt, now)

	var unchangedTask model.Task
	require.NoError(t, model.DB.Where("task_id = ?", task.TaskID).First(&unchangedTask).Error)
	require.Equal(t, model.TaskStatus(model.TaskStatusSuccess), unchangedTask.Status)
}

func TestSeedanceOpenAICallbackUsesOnlyThePublicProjection(t *testing.T) {
	order := &model.SeedanceOrder{PublicProtocol: model.SeedanceProtocolOpenAI}
	task := &model.Task{
		TaskID: "task_callback_public", Status: model.TaskStatusSuccess, Progress: "100%",
		CreatedAt: 100, UpdatedAt: 200,
		Properties:  model.Properties{OriginModelName: "Public video", UpstreamModelName: "private-model"},
		PrivateData: model.TaskPrivateData{ResultURL: "https://supplier.invalid/private-result.mp4"},
		Data:        []byte(`{"provider_type":"DIRECT_EXTERNAL","service_code":"private-service"}`),
	}
	payload, err := seedanceCallbackPayload(order, task)
	require.NoError(t, err)
	var success map[string]any
	require.NoError(t, common.Unmarshal(payload, &success))
	require.ElementsMatch(t,
		[]string{"id", "task_id", "object", "model", "status", "progress", "created_at", "completed_at", "metadata"},
		mapKeys(success))
	require.Equal(t, "completed", success["status"])
	require.Equal(t, "Public video", success["model"])
	require.Equal(t, map[string]any{"url": taskcommon.BuildProxyURL(task.TaskID)}, success["metadata"])
	require.NotContains(t, strings.ToLower(string(payload)), "provider")
	require.NotContains(t, strings.ToLower(string(payload)), "private")
	require.NotContains(t, strings.ToLower(string(payload)), "supplier")

	task.Status = model.TaskStatusFailure
	task.Data = []byte(`{"error":"provider credential rejected"}`)
	payload, err = seedanceCallbackPayload(order, task)
	require.NoError(t, err)
	var failed map[string]any
	require.NoError(t, common.Unmarshal(payload, &failed))
	require.ElementsMatch(t,
		[]string{"id", "task_id", "object", "model", "status", "progress", "created_at", "completed_at", "error"},
		mapKeys(failed))
	require.Equal(t, "failed", failed["status"])
	require.Equal(t, map[string]any{
		"code": "video_processing_failed", "message": seedanceGenericFailureMessage,
	}, failed["error"])
	require.NotContains(t, strings.ToLower(string(payload)), "provider")
	require.NotContains(t, strings.ToLower(string(payload)), "credential")
}

func mapKeys(value map[string]any) []string {
	keys := make([]string, 0, len(value))
	for key := range value {
		keys = append(keys, key)
	}
	return keys
}

func TestSeedanceGenerationIntentSurvivesUnknownOutcomeAndAcceptsLateConfirmation(t *testing.T) {
	truncate(t)
	require.NoError(t, model.DB.AutoMigrate(
		&model.Task{}, &model.SeedanceChannelConfig{}, &model.SeedanceVolcengineCredential{},
		&model.MediaEnhancementProvider{}, &model.SeedanceModelOffering{},
		&model.SeedanceOrder{}, &model.SeedanceAttempt{}, &model.SeedanceCustomerRefund{},
		&model.MediaServiceUsage{}, &model.ServiceBillingEvent{}, &model.ServiceBillingOutbox{},
	))
	for _, table := range []string{
		"tasks", "seedance_channel_configs", "seedance_volcengine_credentials",
		"media_enhancement_providers", "seedance_model_offerings", "seedance_orders", "seedance_attempts",
		"seedance_customer_refunds", "media_service_usages", "service_billing_events", "service_billing_outboxes",
	} {
		require.NoError(t, model.DB.Exec("DELETE FROM "+table).Error)
	}

	previousSecret, previousConfigured := common.CryptoSecret, common.CryptoSecretConfigured
	common.CryptoSecret = strings.Repeat("i", 32)
	common.CryptoSecretConfigured = true
	t.Cleanup(func() {
		common.CryptoSecret = previousSecret
		common.CryptoSecretConfigured = previousConfigured
	})
	arkCredential, err := common.EncryptSensitiveValue("ark-generation-intent-secret")
	require.NoError(t, err)
	billingCredential, err := common.EncryptSensitiveValue("billing-generation-intent-secret")
	require.NoError(t, err)
	now := time.Now().Unix()
	config := &model.SeedanceChannelConfig{
		ChannelID: 9301, Revision: 1, InstanceID: "30000000-0000-0000-0000-000000000301",
		AIPDDBillingBaseURL: "https://billing.example.test", AIPDDBillingCredentialEncrypted: billingCredential,
		Status: model.SeedanceConfigActive, CreatedAt: now, UpdatedAt: now,
	}
	credential := &model.SeedanceVolcengineCredential{
		ChannelID: 9301, Version: 1, ArkAPIKeyEncrypted: arkCredential,
		Fingerprint: "sha256:ark", MaskedSuffix: "****", Status: model.SeedanceCredentialActive,
		CreatedAt: now,
	}
	provider := &model.MediaEnhancementProvider{
		Version: 1, ProviderType: model.SeedanceProviderDirect, DisplayName: "private",
		ServiceEndpoint: "https://provider.example.test", ServiceCode: "private-service",
		CapabilitiesJSON: `{}`, TimeoutPolicyJSON: `{}`, RetryPolicyJSON: `{}`, FallbackPolicyJSON: `{}`,
		Status: model.SeedanceConfigActive, CreatedAt: now, UpdatedAt: now,
	}
	require.NoError(t, model.DB.Create(config).Error)
	require.NoError(t, model.DB.Create(credential).Error)
	require.NoError(t, model.DB.Create(provider).Error)
	offering := &model.SeedanceModelOffering{
		ChannelID: 9301, DisplayName: "Public video", ProviderModelID: "private-model",
		EnhancementProviderID: provider.ID, EnhancementServiceCode: provider.ServiceCode,
		EnhancementSpecificationJSON: `{}`, EnhancementSpecificationVersion: "spec-v1",
		ModelSaleMicroRMB: 5_000_000, ServiceChargeMicroRMB: 1_000_000,
		VolcengineUnitCostMicroRMB: 2_000_000, PricingVersion: "price-v1",
		Enabled: true, PublishedAt: now, CreatedAt: now, UpdatedAt: now,
	}
	require.NoError(t, model.DB.Create(offering).Error)
	pricingSnapshotBytes, err := common.Marshal(map[string]any{
		"pricing_version": "price-v1", "service_charge_micro_rmb": int64(1_000_000),
		"provider_type": model.SeedanceProviderDirect, "provider_id": provider.ID,
		"service_code": provider.ServiceCode, "specification": `{}`, "specification_version": "spec-v1",
	})
	require.NoError(t, err)
	task := &model.Task{
		TaskID: "task-generation-intent", UserId: 9301, ChannelId: 9301,
		Platform: constant.TaskPlatform("59"), Status: model.TaskStatusNotStart,
		Properties:  model.Properties{OriginModelName: offering.DisplayName, UpstreamModelName: offering.DisplayName},
		PrivateData: model.TaskPrivateData{TokenId: 9301}, Quota: 0,
		SubmitTime: now, CreatedAt: now, UpdatedAt: now,
	}
	task.SetData(map[string]any{"id": task.TaskID, "model": offering.DisplayName, "status": "queued"})
	order, err := model.InsertTaskWithSeedanceOrder(model.SeedanceOrderCreate{
		Task: task, Config: config, Credential: credential, Offering: offering, Provider: provider,
		RequestFactsJSON: `{}`, PricingSnapshot: string(pricingSnapshotBytes),
		PublicProtocol: model.SeedanceProtocolOpenAI,
	})
	require.NoError(t, err)
	require.Equal(t, model.SeedanceOrderGenerationSubmitting, order.OrderStatus)
	var attempt model.SeedanceAttempt
	require.NoError(t, model.DB.Where("platform_order_id = ? AND stage = ?", order.PlatformOrderID, "GENERATION").First(&attempt).Error)
	require.Equal(t, "SUBMITTING", attempt.Status)
	require.Empty(t, attempt.ExternalTaskID)

	require.NoError(t, MarkSeedanceGenerationSubmissionOutcomeUnknown(task.TaskID, "transport outcome unknown"))
	require.NoError(t, model.DB.First(&attempt, attempt.ID).Error)
	require.Equal(t, model.SeedanceSubmissionOutcomeUnknown, attempt.Status)
	require.NotEmpty(t, attempt.ResponseEvidenceHash)
	var persistedTask model.Task
	require.NoError(t, model.DB.Where("task_id = ?", task.TaskID).First(&persistedTask).Error)
	handled, err := HandleSeedanceWorkflowPoll(context.Background(), &model.Channel{Type: constant.ChannelTypeSeedance}, &persistedTask)
	require.NoError(t, err)
	require.True(t, handled, "unknown Ark submissions must never enter the generic resubmission/poll path")

	require.NoError(t, ConfirmSeedanceGenerationSubmission(&persistedTask, "ark-task-late-confirmation", []byte(`{"id":"ark-task-late-confirmation"}`)))
	require.NoError(t, model.DB.Where("new_api_task_id = ?", task.TaskID).First(order).Error)
	require.Equal(t, model.SeedanceOrderGenerationProcessing, order.OrderStatus)
	require.NoError(t, model.DB.First(&attempt, attempt.ID).Error)
	require.Equal(t, model.SeedanceUsageRunning, attempt.Status)
	require.Equal(t, "ark-task-late-confirmation", attempt.ExternalTaskID)
	require.NoError(t, model.DB.Where("task_id = ?", task.TaskID).First(&persistedTask).Error)
	require.Equal(t, "ark-task-late-confirmation", persistedTask.PrivateData.UpstreamTaskID)

	// A second unknown submission never receives an upstream ID. The generic
	// timeout path must still close the order and create the durable refund and
	// zero-charge finance event exactly once.
	task2 := &model.Task{
		TaskID: "task-generation-intent-timeout", UserId: 9302, ChannelId: 9301,
		Platform: constant.TaskPlatform("59"), Status: model.TaskStatusNotStart,
		Properties:  model.Properties{OriginModelName: offering.DisplayName, UpstreamModelName: offering.DisplayName},
		PrivateData: model.TaskPrivateData{TokenId: 9302, BillingSource: "wallet"}, Quota: 100,
		SubmitTime: now, CreatedAt: now, UpdatedAt: now,
	}
	task2.SetData(map[string]any{"id": task2.TaskID, "model": offering.DisplayName, "status": "queued"})
	order2, err := model.InsertTaskWithSeedanceOrder(model.SeedanceOrderCreate{
		Task: task2, Config: config, Credential: credential, Offering: offering, Provider: provider,
		RequestFactsJSON: `{}`, PricingSnapshot: string(pricingSnapshotBytes),
		PublicProtocol: model.SeedanceProtocolOpenAI,
	})
	require.NoError(t, err)
	require.NoError(t, MarkSeedanceGenerationSubmissionOutcomeUnknown(task2.TaskID, "response lost"))
	var timedOutTask model.Task
	require.NoError(t, model.DB.Where("task_id = ?", task2.TaskID).First(&timedOutTask).Error)
	previousDispatch := dispatchSeedanceCustomerRefund
	dispatchSeedanceCustomerRefund = func(context.Context, int64) {}
	t.Cleanup(func() { dispatchSeedanceCustomerRefund = previousDispatch })
	handled, err = FailSeedanceWorkflow(context.Background(), &timedOutTask, "generation submission timeout")
	require.NoError(t, err)
	require.True(t, handled)
	require.NoError(t, model.DB.Where("id = ?", order2.ID).First(order2).Error)
	require.Equal(t, model.SeedanceOrderFailed, order2.OrderStatus)
	var refund model.SeedanceCustomerRefund
	require.NoError(t, model.DB.Where("platform_order_id = ?", order2.PlatformOrderID).First(&refund).Error)
	require.Equal(t, model.SeedanceCustomerRefundReady, refund.Status)
	var outboxCount int64
	require.NoError(t, model.DB.Model(&model.ServiceBillingOutbox{}).Count(&outboxCount).Error)
	require.EqualValues(t, 1, outboxCount)
}

func TestSeedanceCustomerRefundRecoversAfterTerminalCommitAndFlushesPendingBatchOnce(t *testing.T) {
	truncate(t)
	require.NoError(t, model.DB.AutoMigrate(
		&model.SeedanceOrder{},
		&model.SeedanceCustomerRefund{},
		&model.MediaServiceUsage{},
		&model.SeedanceAttempt{},
		&model.ServiceBillingEvent{},
		&model.ServiceBillingOutbox{},
	))

	const (
		userID      = 8201
		tokenID     = 8201
		channelID   = 8201
		quota       = 3000
		walletQuota = 10000
		tokenQuota  = 5000
	)
	seedUser(t, userID, walletQuota)
	seedToken(t, tokenID, userID, "sk-seedance-refund-recovery", tokenQuota)
	seedChannel(t, channelID)

	previousBatchUpdate := common.BatchUpdateEnabled
	common.BatchUpdateEnabled = true
	t.Cleanup(func() { common.BatchUpdateEnabled = previousBatchUpdate })
	// Model a request whose pre-consume and usage counters are still waiting in
	// the process-local batch accumulator when the terminal transaction commits.
	require.NoError(t, model.DecreaseUserQuota(userID, quota, false))
	require.NoError(t, model.DecreaseTokenQuota(tokenID, "sk-seedance-refund-recovery", quota))
	model.UpdateUserUsedQuotaAndRequestCount(userID, quota)
	model.UpdateChannelUsedQuota(channelID, quota)

	now := time.Now().Unix()
	task := &model.Task{
		TaskID: "task_seedance_refund_recovery", Platform: constant.TaskPlatform("59"),
		UserId: userID, ChannelId: channelID, Status: model.TaskStatusInProgress,
		Progress: "50%", Quota: quota, Group: "default", CreatedAt: now, UpdatedAt: now,
		Properties: model.Properties{OriginModelName: "Public video", UpstreamModelName: "Public video"},
		PrivateData: model.TaskPrivateData{
			BillingSource: BillingSourceWallet, TokenId: tokenID, LogRequestID: "request-refund-recovery",
			BillingContext: &model.TaskBillingContext{OriginModelName: "Public video", QuotaPerUnit: common.QuotaPerUnit, USDExchangeRate: 7.3},
		},
	}
	require.NoError(t, model.DB.Create(task).Error)
	pricingSnapshot := `{"pricing_version":"price-v1","service_charge_micro_rmb":1000000,"provider_type":"DIRECT","provider_id":1,"service_code":"private-code","specification":"{}","specification_version":"spec-v1"}`
	order := &model.SeedanceOrder{
		PlatformOrderID: "order-seedance-refund-recovery", NewAPITaskID: task.TaskID,
		NewAPIUserID: userID, TokenID: tokenID, ChannelID: channelID,
		InstanceID: "instance-refund-recovery", Model: "Public video",
		OrderStatus: model.SeedanceOrderGenerationProcessing, VolcengineCostStatus: model.SeedanceCostEstimated,
		SyncStatus: model.SeedanceSyncPending, ModelSaleMicroRMB: 8_000_000,
		ServiceChargeTotalMicroRMB: 1_000_000, VolcengineEstimatedMicroRMB: 3_000_000,
		NewAPIEstimatedProfitMicroRMB: 4_000_000, PricingSnapshotJSON: pricingSnapshot,
		PricingSnapshotHash: model.SHA256Evidence(pricingSnapshot), PublicProtocol: model.SeedanceProtocolOfficial,
		CallbackStatus: model.SeedanceCallbackNone, GenerationStartedAt: now, CreatedAt: now, UpdatedAt: now,
	}
	require.NoError(t, model.DB.Create(order).Error)
	require.NoError(t, model.DB.Create(&model.SeedanceAttempt{
		PlatformOrderID: order.PlatformOrderID, AttemptID: order.PlatformOrderID + ":generation:1",
		Stage: "GENERATION", AttemptNo: 1, Status: model.SeedanceUsageRunning,
		StartedAt: now, CreatedAt: now, UpdatedAt: now,
	}).Error)

	previousDispatcher := dispatchSeedanceCustomerRefund
	dispatchSeedanceCustomerRefund = func(context.Context, int64) {}
	t.Cleanup(func() { dispatchSeedanceCustomerRefund = previousDispatcher })
	require.NoError(t, failSeedanceGeneration(context.Background(), task, order, `{"status":"failed"}`))

	var queued model.SeedanceCustomerRefund
	require.NoError(t, model.DB.Where("platform_order_id = ?", order.PlatformOrderID).First(&queued).Error)
	require.Equal(t, model.SeedanceCustomerRefundReady, queued.Status)
	var failedOrder model.SeedanceOrder
	require.NoError(t, model.DB.First(&failedOrder, order.ID).Error)
	require.Equal(t, model.SeedanceOrderFailed, failedOrder.OrderStatus)

	// This call models the next worker process after the crash. It must absorb
	// both the pending batch deltas and any subsequent duplicate maintenance run.
	ProcessSeedanceCustomerRefunds(context.Background(), 20)
	ProcessSeedanceCustomerRefunds(context.Background(), 20)

	var user model.User
	require.NoError(t, model.DB.First(&user, userID).Error)
	require.Equal(t, walletQuota, user.Quota)
	require.Zero(t, user.UsedQuota)
	require.Equal(t, 1, user.RequestCount)
	require.Equal(t, tokenQuota, getTokenRemainQuota(t, tokenID))
	require.Zero(t, getTokenUsedQuota(t, tokenID))
	require.Zero(t, getChannelUsedQuota(t, channelID))
	require.NoError(t, model.DB.First(&queued, queued.ID).Error)
	require.Equal(t, model.SeedanceCustomerRefundApplied, queued.Status)
	require.Positive(t, queued.AppliedAt)
	require.Positive(t, queued.LogRecordedAt)
	require.Positive(t, queued.FinanceSettlementRecordedAt)
	var refundLogCount int64
	require.NoError(t, model.LOG_DB.Model(&model.Log{}).
		Where("billing_event_key = ?", "seedance-customer-refund:"+order.PlatformOrderID).
		Count(&refundLogCount).Error)
	require.EqualValues(t, 1, refundLogCount)
}

func TestSeedanceBillingCredentialRotationKeepsOutboxScopeAndSnapshot(t *testing.T) {
	truncate(t)
	require.NoError(t, model.DB.AutoMigrate(
		&model.SeedanceChannelConfig{}, &model.SeedanceOrder{}, &model.ServiceBillingEvent{},
		&model.ServiceBillingOutbox{}, &model.ServiceBillingFailureAttempt{}, &model.SeedanceAdminAudit{},
	))
	for _, table := range []string{
		"seedance_channel_configs", "seedance_orders", "service_billing_events",
		"service_billing_outboxes", "service_billing_failure_attempts", "seedance_admin_audits",
	} {
		require.NoError(t, model.DB.Exec("DELETE FROM "+table).Error)
	}

	previousSecret, previousConfigured := common.CryptoSecret, common.CryptoSecretConfigured
	common.CryptoSecret = strings.Repeat("r", 32)
	common.CryptoSecretConfigured = true
	t.Cleanup(func() {
		common.CryptoSecret = previousSecret
		common.CryptoSecretConfigured = previousConfigured
	})

	oldCredential, err := common.EncryptSensitiveValue("old-billing-secret")
	require.NoError(t, err)
	newCredential, err := common.EncryptSensitiveValue("new-billing-secret")
	require.NoError(t, err)
	var oldAccepted atomic.Bool
	var oldCalls atomic.Int32
	var newCalls atomic.Int32
	billing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Header.Get("Authorization") {
		case "Bearer old-billing-secret":
			oldCalls.Add(1)
			if !oldAccepted.Load() {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
		case "Bearer new-billing-secret":
			newCalls.Add(1)
		default:
			w.WriteHeader(http.StatusForbidden)
			return
		}
		require.Equal(t, "30000000-0000-0000-0000-000000000099", r.Header.Get("X-NewAPI-Instance-ID"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"accepted":true}`))
	}))
	defer billing.Close()

	const channelID = 9099
	now := time.Now().Unix()
	require.NoError(t, model.DB.Create(&model.SeedanceChannelConfig{
		ChannelID: channelID, Revision: 2, InstanceID: "30000000-0000-0000-0000-000000000099",
		AIPDDBillingBaseURL: billing.URL, AIPDDBillingCredentialEncrypted: newCredential,
		Status: model.SeedanceConfigActive, CreatedAt: now, UpdatedAt: now,
	}).Error)

	createScope := func(orderID, taskID, eventID, lineID string, revision int, credential string) {
		require.NoError(t, model.DB.Create(&model.SeedanceOrder{
			PlatformOrderID: orderID, NewAPITaskID: taskID, ChannelID: channelID,
			InstanceID:                 "30000000-0000-0000-0000-000000000099",
			AIPDDBillingConfigRevision: revision, AIPDDBillingBaseURLSnapshot: billing.URL,
			AIPDDBillingCredentialSnapshotEncrypted: credential,
			Model:                                   "Public video", OrderStatus: model.SeedanceOrderSucceeded,
			VolcengineCostStatus: model.SeedanceCostEstimated, SyncStatus: model.SeedanceSyncReady,
			PricingSnapshotJSON: `{}`, PricingSnapshotHash: "sha256:price", CreatedAt: now, UpdatedAt: now,
		}).Error)
		require.NoError(t, model.DB.Create(&model.ServiceBillingEvent{
			EventID: eventID, PlatformOrderID: orderID, ServiceLineItemID: lineID,
			Revision: 1, EventType: "SERVICE_SETTLEMENT_UPDATED", PayloadJSON: `{}`,
			PayloadHash: "sha256:event", CreatedAt: now,
		}).Error)
		require.NoError(t, model.DB.Create(&model.ServiceBillingOutbox{
			EventID: eventID, Status: model.SeedanceSyncReady, NextAttemptAt: now,
			CreatedAt: now, UpdatedAt: now,
		}).Error)
	}
	createScope("order-old-key", "task-old-key", "event-old-key", "line-old-key", 1, oldCredential)
	createScope("order-new-key", "task-new-key", "event-new-key", "line-new-key", 2, newCredential)

	ProcessSeedanceBillingOutbox(context.Background(), 20)

	var oldOutbox model.ServiceBillingOutbox
	var newOutbox model.ServiceBillingOutbox
	require.NoError(t, model.DB.Where("event_id = ?", "event-old-key").First(&oldOutbox).Error)
	require.NoError(t, model.DB.Where("event_id = ?", "event-new-key").First(&newOutbox).Error)
	require.Equal(t, model.SeedanceSyncAuthPaused, oldOutbox.Status)
	require.Equal(t, model.SeedanceSyncSynced, newOutbox.Status)
	require.EqualValues(t, 1, oldCalls.Load())
	require.EqualValues(t, 1, newCalls.Load())
	var pausedConfig model.SeedanceChannelConfig
	require.NoError(t, model.DB.First(&pausedConfig, "channel_id = ?", channelID).Error)
	require.Positive(t, pausedConfig.BillingAuthPausedAt)

	oldAccepted.Store(true)
	require.NoError(t, model.ReplayServiceBillingOutbox("event-old-key", 1))
	ProcessSeedanceBillingOutbox(context.Background(), 20)
	require.NoError(t, model.DB.Where("event_id = ?", "event-old-key").First(&oldOutbox).Error)
	require.Equal(t, model.SeedanceSyncSynced, oldOutbox.Status)
	require.EqualValues(t, 2, oldCalls.Load())
	require.EqualValues(t, 1, newCalls.Load())
	require.NoError(t, model.DB.First(&pausedConfig, "channel_id = ?", channelID).Error)
	require.Zero(t, pausedConfig.BillingAuthPausedAt)
}

func TestSeedanceBillingLegacyOrderFreezesCredentialBeforeFirstDelivery(t *testing.T) {
	truncate(t)
	require.NoError(t, model.DB.AutoMigrate(
		&model.SeedanceChannelConfig{}, &model.SeedanceOrder{}, &model.ServiceBillingEvent{},
		&model.ServiceBillingOutbox{}, &model.ServiceBillingFailureAttempt{},
	))
	for _, table := range []string{
		"seedance_channel_configs", "seedance_orders", "service_billing_events",
		"service_billing_outboxes", "service_billing_failure_attempts",
	} {
		require.NoError(t, model.DB.Exec("DELETE FROM "+table).Error)
	}

	previousSecret, previousConfigured := common.CryptoSecret, common.CryptoSecretConfigured
	common.CryptoSecret = strings.Repeat("g", 32)
	common.CryptoSecretConfigured = true
	t.Cleanup(func() {
		common.CryptoSecret = previousSecret
		common.CryptoSecretConfigured = previousConfigured
	})

	legacyCredential, err := common.EncryptSensitiveValue("legacy-billing-secret")
	require.NoError(t, err)
	rotatedCredential, err := common.EncryptSensitiveValue("rotated-billing-secret")
	require.NoError(t, err)
	var calls atomic.Int32
	authorizations := make(chan string, 2)
	billing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorizations <- r.Header.Get("Authorization")
		if calls.Add(1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"accepted":true}`))
	}))
	defer billing.Close()

	const (
		channelID = 9299
		orderID   = "order-legacy-billing-snapshot"
		eventID   = "event-legacy-billing-snapshot"
	)
	now := time.Now().Unix()
	instanceID := "30000000-0000-0000-0000-000000000299"
	require.NoError(t, model.DB.Create(&model.SeedanceChannelConfig{
		ChannelID: channelID, Revision: 3, InstanceID: instanceID,
		AIPDDBillingBaseURL: billing.URL, AIPDDBillingCredentialEncrypted: legacyCredential,
		Status: model.SeedanceConfigActive, CreatedAt: now, UpdatedAt: now,
	}).Error)
	// Simulate an order created before the destination snapshot columns existed.
	require.NoError(t, model.DB.Create(&model.SeedanceOrder{
		PlatformOrderID: orderID, NewAPITaskID: "task-legacy-billing-snapshot", ChannelID: channelID,
		InstanceID: instanceID, Model: "Public video", OrderStatus: model.SeedanceOrderSucceeded,
		VolcengineCostStatus: model.SeedanceCostEstimated, SyncStatus: model.SeedanceSyncReady,
		PricingSnapshotJSON: `{}`, PricingSnapshotHash: "sha256:price", CreatedAt: now, UpdatedAt: now,
	}).Error)
	require.NoError(t, model.DB.Create(&model.ServiceBillingEvent{
		EventID: eventID, PlatformOrderID: orderID, ServiceLineItemID: "line-legacy-billing-snapshot",
		Revision: 1, EventType: "SERVICE_SETTLEMENT_UPDATED", PayloadJSON: `{}`,
		PayloadHash: "sha256:event", CreatedAt: now,
	}).Error)
	require.NoError(t, model.DB.Create(&model.ServiceBillingOutbox{
		EventID: eventID, Status: model.SeedanceSyncReady, NextAttemptAt: now,
		CreatedAt: now, UpdatedAt: now,
	}).Error)

	ProcessSeedanceBillingOutbox(context.Background(), 20)
	var frozenOrder model.SeedanceOrder
	require.NoError(t, model.DB.Where("platform_order_id = ?", orderID).First(&frozenOrder).Error)
	require.Equal(t, 3, frozenOrder.AIPDDBillingConfigRevision)
	require.Equal(t, billing.URL, frozenOrder.AIPDDBillingBaseURLSnapshot)
	frozenCredential, err := common.DecryptSensitiveValue(frozenOrder.AIPDDBillingCredentialSnapshotEncrypted)
	require.NoError(t, err)
	require.Equal(t, "legacy-billing-secret", frozenCredential)

	require.NoError(t, model.DB.Model(&model.SeedanceChannelConfig{}).Where("channel_id = ?", channelID).
		Updates(map[string]any{
			"revision": 4, "a_ip_dd_billing_credential_encrypted": rotatedCredential, "updated_at": now + 1,
		}).Error)
	var outbox model.ServiceBillingOutbox
	require.NoError(t, model.DB.Where("event_id = ?", eventID).First(&outbox).Error)
	require.Equal(t, model.SeedanceSyncRetryWait, outbox.Status)
	require.NoError(t, model.DB.Model(&model.ServiceBillingOutbox{}).Where("id = ?", outbox.ID).
		Update("next_attempt_at", 0).Error)

	ProcessSeedanceBillingOutbox(context.Background(), 20)
	require.EqualValues(t, 2, calls.Load())
	require.Equal(t, "Bearer legacy-billing-secret", <-authorizations)
	require.Equal(t, "Bearer legacy-billing-secret", <-authorizations)
	require.NoError(t, model.DB.Where("event_id = ?", eventID).First(&outbox).Error)
	require.Equal(t, model.SeedanceSyncSynced, outbox.Status)
}

func TestSeedanceBillingOutboxRecoversFromLostResponseWithSameIdempotencyKey(t *testing.T) {
	truncate(t)
	require.NoError(t, model.DB.AutoMigrate(
		&model.SeedanceChannelConfig{}, &model.SeedanceOrder{}, &model.ServiceBillingEvent{},
		&model.ServiceBillingOutbox{}, &model.ServiceBillingFailureAttempt{},
	))
	for _, table := range []string{
		"seedance_channel_configs", "seedance_orders", "service_billing_events",
		"service_billing_outboxes", "service_billing_failure_attempts",
	} {
		require.NoError(t, model.DB.Exec("DELETE FROM "+table).Error)
	}

	previousSecret, previousConfigured := common.CryptoSecret, common.CryptoSecretConfigured
	common.CryptoSecret = strings.Repeat("l", 32)
	common.CryptoSecretConfigured = true
	t.Cleanup(func() {
		common.CryptoSecret = previousSecret
		common.CryptoSecretConfigured = previousConfigured
	})
	credential, err := common.EncryptSensitiveValue("response-loss-secret")
	require.NoError(t, err)

	var calls atomic.Int32
	keys := make(chan string, 2)
	billing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		keys <- r.Header.Get("Idempotency-Key")
		if calls.Add(1) == 1 {
			hijacker, ok := w.(http.Hijacker)
			require.True(t, ok)
			connection, _, hijackErr := hijacker.Hijack()
			require.NoError(t, hijackErr)
			_ = connection.Close()
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"accepted":true,"duplicate":true}`))
	}))
	defer billing.Close()

	const (
		channelID = 9199
		orderID   = "order-response-loss"
		eventID   = "event-response-loss"
		lineID    = "line-response-loss"
	)
	now := time.Now().Unix()
	instanceID := "30000000-0000-0000-0000-000000000199"
	require.NoError(t, model.DB.Create(&model.SeedanceChannelConfig{
		ChannelID: channelID, Revision: 1, InstanceID: instanceID,
		AIPDDBillingBaseURL: billing.URL, AIPDDBillingCredentialEncrypted: credential,
		Status: model.SeedanceConfigActive, CreatedAt: now, UpdatedAt: now,
	}).Error)
	require.NoError(t, model.DB.Create(&model.SeedanceOrder{
		PlatformOrderID: orderID, NewAPITaskID: "task-response-loss", ChannelID: channelID,
		InstanceID: instanceID, AIPDDBillingConfigRevision: 1,
		AIPDDBillingBaseURLSnapshot: billing.URL, AIPDDBillingCredentialSnapshotEncrypted: credential,
		Model: "Public video", OrderStatus: model.SeedanceOrderSucceeded,
		VolcengineCostStatus: model.SeedanceCostEstimated, SyncStatus: model.SeedanceSyncReady,
		PricingSnapshotJSON: `{}`, PricingSnapshotHash: "sha256:price", CreatedAt: now, UpdatedAt: now,
	}).Error)
	require.NoError(t, model.DB.Create(&model.ServiceBillingEvent{
		EventID: eventID, PlatformOrderID: orderID, ServiceLineItemID: lineID,
		Revision: 1, EventType: "SERVICE_SETTLEMENT_UPDATED", PayloadJSON: `{}`,
		PayloadHash: "sha256:event", CreatedAt: now,
	}).Error)
	require.NoError(t, model.DB.Create(&model.ServiceBillingOutbox{
		EventID: eventID, Status: model.SeedanceSyncReady, NextAttemptAt: now,
		CreatedAt: now, UpdatedAt: now,
	}).Error)

	ProcessSeedanceBillingOutbox(context.Background(), 20)
	var outbox model.ServiceBillingOutbox
	require.NoError(t, model.DB.Where("event_id = ?", eventID).First(&outbox).Error)
	require.Equal(t, model.SeedanceSyncRetryWait, outbox.Status)
	require.Equal(t, 1, outbox.AttemptCount)
	require.NoError(t, model.DB.Model(&model.ServiceBillingOutbox{}).Where("id = ?", outbox.ID).
		Update("next_attempt_at", 0).Error)

	ProcessSeedanceBillingOutbox(context.Background(), 20)
	require.NoError(t, model.DB.Where("event_id = ?", eventID).First(&outbox).Error)
	require.Equal(t, model.SeedanceSyncSynced, outbox.Status)
	require.EqualValues(t, 2, calls.Load())
	expectedKey := "service-usage:" + orderID + ":" + lineID + ":1"
	require.Equal(t, expectedKey, <-keys)
	require.Equal(t, expectedKey, <-keys)
	var eventCount int64
	var outboxCount int64
	require.NoError(t, model.DB.Model(&model.ServiceBillingEvent{}).Count(&eventCount).Error)
	require.NoError(t, model.DB.Model(&model.ServiceBillingOutbox{}).Count(&outboxCount).Error)
	require.EqualValues(t, 1, eventCount)
	require.EqualValues(t, 1, outboxCount)
}

func TestSeedanceCustomerRefundRestoresSubscriptionReservationExactlyOnce(t *testing.T) {
	truncate(t)
	const (
		userID    = 8202
		tokenID   = 8202
		channelID = 8202
		subID     = 8202
		quota     = 2400
	)
	seedUser(t, userID, 0, quota)
	seedToken(t, tokenID, userID, "sk-seedance-subscription-refund", 2600)
	require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", tokenID).Update("used_quota", quota).Error)
	seedChannel(t, channelID, quota)
	seedSubscription(t, subID, userID, 10000, quota)
	now := time.Now().Unix()
	require.NoError(t, model.DB.Create(&model.SubscriptionPreConsumeRecord{
		RequestId: "request-subscription-refund", UserId: userID, UserSubscriptionId: subID,
		PreConsumed: quota, Status: "consumed", CreatedAt: now, UpdatedAt: now,
	}).Error)
	task := makeTask(userID, channelID, quota, tokenID, BillingSourceSubscription, subID)
	task.TaskID = "task_seedance_subscription_refund"
	task.PrivateData.LogRequestID = "request-subscription-refund"
	task.PrivateData.SubscriptionPreConsumed = quota
	require.NoError(t, model.DB.Create(task).Error)
	refund := &model.SeedanceCustomerRefund{
		PlatformOrderID: "order-seedance-subscription-refund", NewAPITaskID: task.TaskID,
		UserID: userID, TokenID: tokenID, ChannelID: channelID, Quota: quota,
		FundingSource: BillingSourceSubscription, SubscriptionID: subID,
		SubscriptionPreConsumed: quota, SubscriptionRequestID: "request-subscription-refund",
		Reason: seedanceGenericFailureMessage, Status: model.SeedanceCustomerRefundReady,
		CreatedAt: now, UpdatedAt: now,
	}
	require.NoError(t, model.DB.Create(refund).Error)

	ProcessSeedanceCustomerRefunds(context.Background(), 20)
	ProcessSeedanceCustomerRefunds(context.Background(), 20)

	require.Zero(t, getSubscriptionUsed(t, subID))
	require.Equal(t, 5000, getTokenRemainQuota(t, tokenID))
	require.Zero(t, getTokenUsedQuota(t, tokenID))
	require.Zero(t, getUserUsedQuota(t, userID))
	require.Zero(t, getChannelUsedQuota(t, channelID))
	var record model.SubscriptionPreConsumeRecord
	require.NoError(t, model.DB.Where("request_id = ?", "request-subscription-refund").First(&record).Error)
	require.Equal(t, "refunded", record.Status)
	require.NoError(t, model.DB.First(refund, refund.ID).Error)
	require.Equal(t, model.SeedanceCustomerRefundApplied, refund.Status)
}

func TestDirectEnhancementProviderClassifiesSubmissionFailures(t *testing.T) {
	status := http.StatusUnprocessableEntity
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
	}))
	defer server.Close()
	provider := &DirectEnhancementProvider{
		config: &model.MediaEnhancementProvider{ServiceEndpoint: server.URL},
		client: server.Client(),
	}

	_, err := provider.Submit(context.Background(), EnhancementSubmitRequest{InputURL: "https://media.example/input.mp4", IdempotencyKey: "attempt-1"})
	require.Error(t, err)
	require.True(t, isDefinitiveEnhancementFailure(err))

	status = http.StatusServiceUnavailable
	_, err = provider.Submit(context.Background(), EnhancementSubmitRequest{InputURL: "https://media.example/input.mp4", IdempotencyKey: "attempt-1"})
	require.Error(t, err)
	require.False(t, isDefinitiveEnhancementFailure(err))
}

func TestDirectEnhancementProviderSubmitQueryAndCancelContract(t *testing.T) {
	var postCalls atomic.Int32
	var getCalls atomic.Int32
	var deleteCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "Bearer provider-contract-secret", r.Header.Get("Authorization"))
		switch r.Method {
		case http.MethodPost:
			postCalls.Add(1)
			require.Equal(t, "/tasks", r.URL.EscapedPath())
			require.Equal(t, "attempt-contract-1", r.Header.Get("Idempotency-Key"))
			var request directEnhancementRequest
			body, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			require.NoError(t, common.Unmarshal(body, &request))
			require.Equal(t, map[string]string{"url": "https://media.example/input.mp4"}, request.Input)
			require.Equal(t, "attempt-contract-1", request.IdempotencyKey)
			require.Equal(t, map[string]any{"target_resolution": "3840x2160"}, request.Specification)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"task_id":"execution/task 1","status":"running","usage":{"frames":2}}`))
		case http.MethodGet:
			getCalls.Add(1)
			require.Equal(t, "/tasks/execution%2Ftask%201", r.URL.EscapedPath())
			require.Empty(t, r.Header.Get("Idempotency-Key"))
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"execution_task_id":"execution/task 1","status":"completed","url":"https://result.example/video.mp4","usage":{"frames":4}}`))
		case http.MethodDelete:
			deleteCalls.Add(1)
			require.Equal(t, "/tasks/execution%2Ftask%201", r.URL.EscapedPath())
			require.Empty(t, r.Header.Get("Idempotency-Key"))
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected method %s", r.Method)
		}
	}))
	defer server.Close()

	provider := &DirectEnhancementProvider{
		config:     &model.MediaEnhancementProvider{ServiceEndpoint: server.URL + "/tasks"},
		credential: "provider-contract-secret",
		client:     server.Client(),
	}
	submitted, err := provider.Submit(context.Background(), EnhancementSubmitRequest{
		InputURL:          "https://media.example/input.mp4",
		SpecificationJSON: `{"target_resolution":"3840x2160"}`,
		IdempotencyKey:    "attempt-contract-1",
	})
	require.NoError(t, err)
	require.Equal(t, "execution/task 1", submitted.ExecutionTaskID)
	require.Equal(t, model.SeedanceUsageRunning, submitted.Status)
	require.JSONEq(t, `{"frames":2}`, submitted.UsageEvidenceJSON)

	queried, err := provider.Query(context.Background(), submitted.ExecutionTaskID)
	require.NoError(t, err)
	require.Equal(t, "execution/task 1", queried.ExecutionTaskID)
	require.Equal(t, model.SeedanceUsageSucceeded, queried.Status)
	require.Equal(t, "https://result.example/video.mp4", queried.ResultURL)
	require.JSONEq(t, `{"frames":4}`, queried.UsageEvidenceJSON)

	require.NoError(t, provider.Cancel(context.Background(), submitted.ExecutionTaskID))
	require.EqualValues(t, 1, postCalls.Load())
	require.EqualValues(t, 1, getCalls.Load())
	require.EqualValues(t, 1, deleteCalls.Load())
}

func TestUnknownEnhancementSubmissionRetriesTheSameAttemptAndIdempotencyKey(t *testing.T) {
	provider := &unknownSubmissionProvider{}
	previousFactory := enhancementProviderFactory
	enhancementProviderFactory = func(*model.MediaEnhancementProvider) (EnhancementProvider, error) {
		return provider, nil
	}
	t.Cleanup(func() { enhancementProviderFactory = previousFactory })

	usage := &model.MediaServiceUsage{
		AttemptID:         "order-1:enhancement:1",
		UsageFactsJSON:    `{"input_url":"https://media.example/input.mp4"}`,
		SpecificationJSON: `{"target_resolution":"3840x2160"}`,
		Status:            model.SeedanceUsagePending,
	}
	for range 2 {
		err := submitPendingSeedanceEnhancement(
			context.Background(), &model.Task{}, &model.SeedanceOrder{}, usage, &model.MediaEnhancementProvider{})
		require.ErrorContains(t, err, "connection closed")
	}

	require.Equal(t, []string{"order-1:enhancement:1", "order-1:enhancement:1"}, provider.idempotencyKeys)
	require.Equal(t, model.SeedanceUsagePending, usage.Status)
	require.Empty(t, usage.ExecutionTaskID)
	require.Equal(t, model.SeedanceSubmissionOutcomeUnknown, usage.FailureReason)
	require.EqualValues(t, 2, usage.UnknownSubmissionCount)
}

func TestDeadLetterRevisionIsHigherImmutableAndIdempotent(t *testing.T) {
	truncate(t)
	require.NoError(t, model.DB.AutoMigrate(
		&model.SeedanceOrder{}, &model.MediaServiceUsage{}, &model.ServiceBillingEvent{},
		&model.ServiceBillingOutbox{}, &model.SeedanceAdminAudit{},
	))
	for _, table := range []string{
		"seedance_orders", "media_service_usages", "service_billing_events",
		"service_billing_outboxes", "seedance_admin_audits",
	} {
		require.NoError(t, model.DB.Exec("DELETE FROM "+table).Error)
	}
	order := &model.SeedanceOrder{
		PlatformOrderID: "order-dead-letter", NewAPITaskID: "task-dead-letter", ChannelID: 901,
		Model: "Public video", OrderStatus: model.SeedanceOrderSucceeded,
		VolcengineCostStatus: model.SeedanceCostEstimated, VolcengineEstimatedMicroRMB: 3_000_000,
		ModelSaleMicroRMB: 8_000_000, PricingSnapshotHash: "sha256:price",
	}
	require.NoError(t, model.DB.Create(order).Error)
	usage := &model.MediaServiceUsage{
		ServiceLineItemID: "order-dead-letter:video-processing", PlatformOrderID: order.PlatformOrderID,
		ServiceType: model.SeedanceServiceTypeVideoSuperResolution, ProviderType: model.SeedanceProviderDirect,
		ServiceCode: "private-code", AttemptID: "attempt-1", Status: model.SeedanceUsageSucceeded,
		ChargeMicroRMB: 1_800_000, PriceVersion: "price-v1", UsageFactsJSON: `{"seconds":5}`,
		UsageEvidenceHash: "sha256:usage", Revision: 1, StartedAt: time.Now().Unix(),
	}
	require.NoError(t, model.DB.Create(usage).Error)
	oldEvent := &model.ServiceBillingEvent{
		EventID: "event-dead-letter", PlatformOrderID: order.PlatformOrderID,
		ServiceLineItemID: usage.ServiceLineItemID, Revision: 1,
		EventType: "SERVICE_SETTLEMENT_UPDATED", PayloadJSON: `{}`, PayloadHash: "sha256:old",
	}
	require.NoError(t, model.DB.Create(oldEvent).Error)
	require.NoError(t, model.DB.Create(&model.ServiceBillingOutbox{
		EventID: oldEvent.EventID, Status: model.SeedanceSyncDeadLetter,
	}).Error)
	require.ErrorIs(t, model.ReplayServiceBillingOutbox(oldEvent.EventID, 7), model.ErrSeedanceDeadLetterRevision)

	revised, created, err := ReviseSeedanceDeadLetter(oldEvent.EventID, 7)
	require.NoError(t, err)
	require.True(t, created)
	require.Equal(t, 2, revised.Revision)
	require.NotEqual(t, oldEvent.EventID, revised.EventID)
	replayed, createdAgain, err := ReviseSeedanceDeadLetter(oldEvent.EventID, 7)
	require.NoError(t, err)
	require.False(t, createdAgain)
	require.Equal(t, revised.EventID, replayed.EventID)

	var events int64
	require.NoError(t, model.DB.Model(&model.ServiceBillingEvent{}).Count(&events).Error)
	require.EqualValues(t, 2, events)
	require.NoError(t, model.DB.First(usage, usage.ID).Error)
	require.Equal(t, 2, usage.Revision)
	var audit model.SeedanceAdminAudit
	require.NoError(t, model.DB.Where("action = ?", "CREATE_REVISION").First(&audit).Error)
	require.Equal(t, "1", audit.BeforeVersion)
	require.Equal(t, "2", audit.AfterVersion)
}

func TestBillingValidationErrorKeepsOnlySafeCodeAndMessage(t *testing.T) {
	err := seedanceBillingValidationError([]byte(`{"code":"VALIDATION_ERROR","message":"price_version is required","debug":{"authorization":"secret"}}`))
	require.EqualError(t, err, "billing event rejected (VALIDATION_ERROR): price_version is required")
	require.NotContains(t, err.Error(), "authorization")
	require.NotContains(t, err.Error(), "secret")
	err = seedanceBillingValidationError([]byte(`{"code":{"debug":"secret"},"message":"invalid revision"}`))
	require.EqualError(t, err, "billing event rejected: invalid revision")
	require.NotContains(t, err.Error(), "secret")
	require.EqualError(t, seedanceBillingValidationError([]byte("not-json")), "billing event rejected")
}

// retryUnsafeSubmissionProvider stands in for an adapter whose upstream
// idempotency contract has not been verified.
type retryUnsafeSubmissionProvider struct {
	submissions int
}

func (p *retryUnsafeSubmissionProvider) Submit(context.Context, EnhancementSubmitRequest) (*EnhancementResult, error) {
	p.submissions++
	return nil, errors.New("connection closed before the provider response")
}

func (p *retryUnsafeSubmissionProvider) Query(context.Context, string) (*EnhancementResult, error) {
	return nil, errors.New("query should not run before an execution task id exists")
}

func (p *retryUnsafeSubmissionProvider) Cancel(context.Context, string) error {
	return ErrSeedanceRemoteCancelUnsupported
}

func (p *retryUnsafeSubmissionProvider) Capabilities() EnhancementCapabilities {
	return EnhancementCapabilities{SubmitRetrySafe: false, CancelSupported: false}
}

type finiteRetryWindowSubmissionProvider struct {
	submissions int
}

func (p *finiteRetryWindowSubmissionProvider) Submit(context.Context, EnhancementSubmitRequest) (*EnhancementResult, error) {
	p.submissions++
	return nil, errors.New("connection closed before the provider response")
}

func (p *finiteRetryWindowSubmissionProvider) Query(context.Context, string) (*EnhancementResult, error) {
	return nil, errors.New("query should not run before an execution task id exists")
}

func (p *finiteRetryWindowSubmissionProvider) Cancel(context.Context, string) error {
	return ErrSeedanceRemoteCancelUnsupported
}

func (p *finiteRetryWindowSubmissionProvider) Capabilities() EnhancementCapabilities {
	return EnhancementCapabilities{
		SubmitRetrySafe: true, SubmitRetryWindow: 24 * time.Hour, CancelSupported: false,
	}
}

type recordingCancelProvider struct {
	cancelledTaskIDs []string
}

func (p *recordingCancelProvider) Submit(context.Context, EnhancementSubmitRequest) (*EnhancementResult, error) {
	return nil, errors.New("submit is not expected")
}

func (p *recordingCancelProvider) Query(context.Context, string) (*EnhancementResult, error) {
	return nil, errors.New("query is not expected")
}

func (p *recordingCancelProvider) Cancel(_ context.Context, executionTaskID string) error {
	p.cancelledTaskIDs = append(p.cancelledTaskIDs, executionTaskID)
	return nil
}

func (p *recordingCancelProvider) Capabilities() EnhancementCapabilities {
	return EnhancementCapabilities{SubmitRetrySafe: true, CancelSupported: true}
}

// An adapter without a proven idempotency contract must not send a second POST
// after an unknown outcome, because the first request may already have created a
// billable remote task.
func TestUnknownSubmissionIsNotRetriedWhenTheAdapterIsNotRetrySafe(t *testing.T) {
	provider := &retryUnsafeSubmissionProvider{}
	previousFactory := enhancementProviderFactory
	enhancementProviderFactory = func(*model.MediaEnhancementProvider) (EnhancementProvider, error) {
		return provider, nil
	}
	t.Cleanup(func() { enhancementProviderFactory = previousFactory })

	usage := &model.MediaServiceUsage{
		AttemptID:         "order-mediakit:enhancement:1",
		UsageFactsJSON:    `{"input_url":"https://media.example/input.mp4"}`,
		SpecificationJSON: `{"scene":"aigc","resolution":"1080p","tool_version":"standard"}`,
		Status:            model.SeedanceUsagePending,
	}
	err := submitPendingSeedanceEnhancement(
		context.Background(), &model.Task{}, &model.SeedanceOrder{}, usage, &model.MediaEnhancementProvider{})
	require.ErrorContains(t, err, "connection closed")
	require.Equal(t, 1, provider.submissions)
	require.Equal(t, model.SeedanceSubmissionOutcomeUnknown, usage.FailureReason)
	require.EqualValues(t, 1, usage.UnknownSubmissionCount)

	err = submitPendingSeedanceEnhancement(
		context.Background(), &model.Task{}, &model.SeedanceOrder{}, usage, &model.MediaEnhancementProvider{})
	require.ErrorIs(t, err, ErrSeedanceSubmissionNeedsManualReview)
	require.Equal(t, 1, provider.submissions, "a second POST would risk a duplicate billable task")
	require.Equal(t, model.SeedanceUsagePending, usage.Status)
	require.Empty(t, usage.ExecutionTaskID)
}

func TestUnknownSubmissionIsNotRetriedAfterTheProviderIdempotencyWindow(t *testing.T) {
	provider := &finiteRetryWindowSubmissionProvider{}
	previousFactory := enhancementProviderFactory
	enhancementProviderFactory = func(*model.MediaEnhancementProvider) (EnhancementProvider, error) {
		return provider, nil
	}
	t.Cleanup(func() { enhancementProviderFactory = previousFactory })

	usage := &model.MediaServiceUsage{
		AttemptID:              "order-mediakit-window:enhancement:1",
		UsageFactsJSON:         `{"input_url":"https://media.example/input.mp4"}`,
		SpecificationJSON:      `{"scene":"aigc","resolution":"1080p","tool_version":"standard"}`,
		Status:                 model.SeedanceUsagePending,
		UnknownSubmissionCount: 1,
		StartedAt:              time.Now().Add(-24*time.Hour - time.Second).Unix(),
	}
	err := submitPendingSeedanceEnhancement(
		context.Background(), &model.Task{}, &model.SeedanceOrder{}, usage, &model.MediaEnhancementProvider{})
	require.ErrorIs(t, err, ErrSeedanceSubmissionNeedsManualReview)
	require.Zero(t, provider.submissions, "an expired deduplication window must never create a second billable task")

	usage.StartedAt = time.Now().Add(-23 * time.Hour).Unix()
	err = submitPendingSeedanceEnhancement(
		context.Background(), &model.Task{}, &model.SeedanceOrder{}, usage, &model.MediaEnhancementProvider{})
	require.ErrorContains(t, err, "connection closed")
	require.Equal(t, 1, provider.submissions, "the same attempt remains retryable inside the documented window")
}

// Cancelling an accepted remote task that cannot actually be stopped would refund
// the customer while the provider keeps billing, so the order must stay open.
func TestCancelIsRefusedWhenTheAdapterCannotStopAnAcceptedRemoteTask(t *testing.T) {
	truncate(t)
	require.NoError(t, model.DB.AutoMigrate(
		&model.SeedanceOrder{}, &model.MediaServiceUsage{}, &model.SeedanceAttempt{},
		&model.MediaEnhancementProvider{}, &model.Task{},
	))
	for _, table := range []string{
		"seedance_orders", "media_service_usages", "seedance_attempts", "media_enhancement_providers",
	} {
		require.NoError(t, model.DB.Exec("DELETE FROM "+table).Error)
	}
	now := time.Now().Unix()
	provider := &model.MediaEnhancementProvider{
		ProviderType: model.SeedanceProviderDirect, AdapterType: model.SeedanceAdapterVolcengineMediaKit,
		DisplayName: "火山 AI MediaKit", ServiceEndpoint: model.SeedanceMediaKitBaseURL,
		ServiceCode: model.SeedanceMediaKitServiceCode, Status: model.SeedanceConfigActive,
		CapabilitiesJSON: `{}`, TimeoutPolicyJSON: `{}`, RetryPolicyJSON: `{}`, FallbackPolicyJSON: `{}`,
		CreatedAt: now, UpdatedAt: now,
	}
	require.NoError(t, model.DB.Create(provider).Error)
	previousFactory := enhancementProviderFactory
	enhancementProviderFactory = func(config *model.MediaEnhancementProvider) (EnhancementProvider, error) {
		require.Equal(t, model.SeedanceAdapterVolcengineMediaKit, config.AdapterType)
		return &retryUnsafeSubmissionProvider{}, nil
	}
	t.Cleanup(func() { enhancementProviderFactory = previousFactory })

	task := &model.Task{
		TaskID: "task_mediakit_cancel", Platform: constant.TaskPlatform("59"), UserId: 41, ChannelId: 941,
		Status: model.TaskStatusInProgress, Progress: "80%",
		Properties: model.Properties{OriginModelName: "Public video"},
	}
	require.NoError(t, model.DB.Create(task).Error)
	pricingSnapshot, err := common.Marshal(map[string]any{
		"pricing_version": "price-v1", "service_charge_micro_rmb": int64(1_800_000),
		"provider_type": model.SeedanceProviderDirect, "provider_id": provider.ID,
		"service_code": model.SeedanceMediaKitServiceCode, "specification": `{}`,
		"specification_version": "spec-v1",
	})
	require.NoError(t, err)
	order := &model.SeedanceOrder{
		PlatformOrderID: "order-mediakit-cancel", NewAPITaskID: task.TaskID, ChannelID: 941,
		Model: "Public video", OrderStatus: model.SeedanceOrderEnhancing,
		VolcengineCostStatus: model.SeedanceCostEstimated, SyncStatus: model.SeedanceSyncPending,
		PricingSnapshotJSON: string(pricingSnapshot), PricingSnapshotHash: "sha256:price",
		ModelSaleMicroRMB: 8_000_000, VolcengineEstimatedMicroRMB: 3_000_000,
		PublicProtocol: model.SeedanceProtocolOfficial, CreatedAt: now, UpdatedAt: now,
	}
	require.NoError(t, model.DB.Create(order).Error)
	require.NoError(t, model.DB.Create(&model.MediaServiceUsage{
		ServiceLineItemID: order.PlatformOrderID + ":video-processing", PlatformOrderID: order.PlatformOrderID,
		ServiceType: model.SeedanceServiceTypeVideoSuperResolution, ProviderType: model.SeedanceProviderDirect,
		ProviderID: provider.ID, ServiceCode: model.SeedanceMediaKitServiceCode,
		AttemptID: order.PlatformOrderID + ":enhancement:1", ExecutionTaskID: "mediakit-task-0001",
		Status: model.SeedanceUsageRunning, ChargeMicroRMB: 1_800_000, PriceVersion: "price-v1",
		UsageFactsJSON: `{}`, Revision: 1, StartedAt: now, CreatedAt: now, UpdatedAt: now,
	}).Error)

	err = CancelSeedanceWorkflow(context.Background(), task, false)
	require.ErrorIs(t, err, ErrSeedanceRemoteCancelUnsupported)

	var unchangedOrder model.SeedanceOrder
	require.NoError(t, model.DB.Where("id = ?", order.ID).First(&unchangedOrder).Error)
	require.Equal(t, model.SeedanceOrderEnhancing, unchangedOrder.OrderStatus)
	require.Equal(t, int64(8_000_000), unchangedOrder.ModelSaleMicroRMB)
	var unchangedUsage model.MediaServiceUsage
	require.NoError(t, model.DB.Where("platform_order_id = ?", order.PlatformOrderID).First(&unchangedUsage).Error)
	require.Equal(t, model.SeedanceUsageRunning, unchangedUsage.Status)
	require.Equal(t, int64(1_800_000), unchangedUsage.ChargeMicroRMB)
	var refunds int64
	require.NoError(t, model.DB.Model(&model.SeedanceCustomerRefund{}).Count(&refunds).Error)
	require.Zero(t, refunds, "a refund must not be queued while the remote task keeps running")
}

func TestCancelAcceptedEnhancementStopsRemoteTaskAndRefundsExactlyOnce(t *testing.T) {
	truncate(t)
	require.NoError(t, model.DB.AutoMigrate(
		&model.SeedanceOrder{}, &model.MediaServiceUsage{}, &model.SeedanceAttempt{},
		&model.MediaEnhancementProvider{}, &model.ServiceBillingEvent{}, &model.ServiceBillingOutbox{},
		&model.SeedanceCustomerRefund{}, &model.Task{},
	))
	for _, table := range []string{
		"service_billing_outboxes", "service_billing_events", "media_service_usages", "seedance_attempts",
		"seedance_customer_refunds", "seedance_orders", "media_enhancement_providers",
	} {
		require.NoError(t, model.DB.Exec("DELETE FROM "+table).Error)
	}

	const (
		userID       = 9422
		tokenID      = 9422
		channelID    = 9421
		chargedQuota = 500
	)
	seedUser(t, userID, 9_500, chargedQuota)
	require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", userID).Update("request_count", 1).Error)
	seedToken(t, tokenID, userID, "sk-seedance-cancel-success", 4_500)
	require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", tokenID).Update("used_quota", chargedQuota).Error)
	baseURL := "https://ark.cn-beijing.volces.com"
	channel := &model.Channel{
		Id: channelID, Type: constant.ChannelTypeSeedance, Name: "Seedance cancel contract",
		Key: "managed", Status: common.ChannelStatusEnabled, UsedQuota: chargedQuota, BaseURL: &baseURL,
	}
	require.NoError(t, model.DB.Create(channel).Error)

	now := time.Now().Unix()
	provider := &model.MediaEnhancementProvider{
		ProviderType: model.SeedanceProviderDirect, AdapterType: model.SeedanceAdapterGenericHTTP,
		DisplayName: "private cancel supplier", ServiceEndpoint: "https://supplier.invalid/tasks",
		ServiceCode: "private-cancel-service", Status: model.SeedanceConfigActive,
		CapabilitiesJSON: `{}`, TimeoutPolicyJSON: `{}`, RetryPolicyJSON: `{}`, FallbackPolicyJSON: `{}`,
		CreatedAt: now, UpdatedAt: now,
	}
	require.NoError(t, model.DB.Create(provider).Error)
	recorder := &recordingCancelProvider{}
	previousFactory := enhancementProviderFactory
	enhancementProviderFactory = func(*model.MediaEnhancementProvider) (EnhancementProvider, error) { return recorder, nil }
	t.Cleanup(func() { enhancementProviderFactory = previousFactory })

	task := &model.Task{
		TaskID: "task-seedance-cancel-success", Platform: constant.TaskPlatform("59"),
		UserId: userID, ChannelId: channelID, Quota: chargedQuota, Group: "default",
		Status: model.TaskStatusInProgress, Progress: "80%", CreatedAt: now, UpdatedAt: now,
		Properties: model.Properties{OriginModelName: "Public video", UpstreamModelName: "private-ark-model"},
		PrivateData: model.TaskPrivateData{
			TokenId: tokenID, BillingSource: BillingSourceWallet,
			BillingContext: &model.TaskBillingContext{OriginModelName: "Public video", QuotaPerUnit: common.QuotaPerUnit, USDExchangeRate: 7.3},
		},
	}
	task.SetData(map[string]any{"id": task.TaskID, "model": "Public video", "status": "running"})
	require.NoError(t, model.DB.Create(task).Error)
	pricingSnapshot, err := common.Marshal(map[string]any{
		"pricing_version": "price-cancel-v1", "service_charge_micro_rmb": int64(1_800_000),
		"provider_type": model.SeedanceProviderDirect, "provider_id": provider.ID,
		"service_code": provider.ServiceCode, "specification": `{}`, "specification_version": "spec-cancel-v1",
		"provider_cost_micro_rmb": int64(1_200_000),
	})
	require.NoError(t, err)
	order := &model.SeedanceOrder{
		PlatformOrderID: "order-seedance-cancel-success", NewAPITaskID: task.TaskID,
		NewAPIUserID: userID, TokenID: tokenID, ChannelID: channelID, InstanceID: "instance-cancel-success",
		Model: "Public video", OrderStatus: model.SeedanceOrderEnhancing,
		VolcengineCostStatus: model.SeedanceCostEstimated, SyncStatus: model.SeedanceSyncPending,
		ModelSaleMicroRMB: 8_000_000, ServiceChargeTotalMicroRMB: 1_800_000,
		VolcengineEstimatedMicroRMB: 3_000_000, NewAPIEstimatedProfitMicroRMB: 3_200_000,
		PricingSnapshotJSON: string(pricingSnapshot), PricingSnapshotHash: model.SHA256Evidence(string(pricingSnapshot)),
		PublicProtocol: model.SeedanceProtocolOpenAI, CreatedAt: now, UpdatedAt: now,
	}
	require.NoError(t, model.DB.Create(order).Error)
	providerCost := int64(1_200_000)
	usage := &model.MediaServiceUsage{
		ServiceLineItemID: order.PlatformOrderID + ":video-processing", PlatformOrderID: order.PlatformOrderID,
		ServiceType: model.SeedanceServiceTypeVideoSuperResolution, ProviderType: model.SeedanceProviderDirect,
		ProviderID: provider.ID, ServiceCode: provider.ServiceCode, SpecificationJSON: `{}`,
		SpecificationVersion: "spec-cancel-v1", AttemptID: order.PlatformOrderID + ":enhancement:1",
		ExecutionTaskID: "remote-enhancement-cancel-success", Status: model.SeedanceUsageRunning,
		ChargeMicroRMB: 1_800_000, PriceVersion: "price-cancel-v1", ProviderCostMicroRMB: &providerCost,
		UsageFactsJSON: `{}`, Revision: 1, StartedAt: now, CreatedAt: now, UpdatedAt: now,
	}
	require.NoError(t, model.DB.Create(usage).Error)
	require.NoError(t, model.DB.Create(&model.SeedanceAttempt{
		PlatformOrderID: order.PlatformOrderID, AttemptID: usage.AttemptID, Stage: "ENHANCEMENT",
		AttemptNo: 1, ProviderType: provider.ProviderType, ProviderID: provider.ID,
		ServiceCode: provider.ServiceCode, SpecificationVersion: usage.SpecificationVersion,
		ExternalTaskID: usage.ExecutionTaskID, Status: model.SeedanceUsageRunning,
		StartedAt: now, CreatedAt: now, UpdatedAt: now,
	}).Error)

	require.NoError(t, CancelSeedanceWorkflow(context.Background(), task, true))
	require.Equal(t, []string{usage.ExecutionTaskID}, recorder.cancelledTaskIDs)

	var cancelledOrder model.SeedanceOrder
	require.NoError(t, model.DB.First(&cancelledOrder, order.ID).Error)
	require.Equal(t, model.SeedanceOrderCancelled, cancelledOrder.OrderStatus)
	require.Positive(t, cancelledOrder.DeletedAt)
	require.Zero(t, cancelledOrder.ModelSaleMicroRMB)
	require.Zero(t, cancelledOrder.ServiceChargeTotalMicroRMB)
	require.EqualValues(t, 3_000_000, cancelledOrder.VolcengineEstimatedMicroRMB)
	require.EqualValues(t, -3_000_000, cancelledOrder.NewAPIEstimatedProfitMicroRMB)

	var cancelledUsage model.MediaServiceUsage
	require.NoError(t, model.DB.First(&cancelledUsage, usage.ID).Error)
	require.Equal(t, model.SeedanceUsageFailed, cancelledUsage.Status)
	require.Equal(t, "CANCELLED", cancelledUsage.FailureReason)
	require.Zero(t, cancelledUsage.ChargeMicroRMB)
	require.NotNil(t, cancelledUsage.ProviderCostMicroRMB)
	require.EqualValues(t, providerCost, *cancelledUsage.ProviderCostMicroRMB)

	var cancelledTask model.Task
	require.NoError(t, model.DB.First(&cancelledTask, task.ID).Error)
	require.Equal(t, model.TaskStatus(model.TaskStatusFailure), cancelledTask.Status)
	require.Equal(t, "Task was cancelled", cancelledTask.FailReason)
	require.Contains(t, string(cancelledTask.Data), `"status":"cancelled"`)
	require.NotContains(t, strings.ToLower(string(cancelledTask.Data)), "provider")

	ProcessSeedanceCustomerRefunds(context.Background(), 20)
	ProcessSeedanceCustomerRefunds(context.Background(), 20)
	require.Equal(t, 10_000, getUserQuota(t, userID))
	require.Zero(t, getUserUsedQuota(t, userID))
	require.Equal(t, 5_000, getTokenRemainQuota(t, tokenID))
	require.Zero(t, getTokenUsedQuota(t, tokenID))
	require.Zero(t, getChannelUsedQuota(t, channelID))

	var refunds int64
	var events int64
	var outboxes int64
	require.NoError(t, model.DB.Model(&model.SeedanceCustomerRefund{}).Where("platform_order_id = ?", order.PlatformOrderID).Count(&refunds).Error)
	require.NoError(t, model.DB.Model(&model.ServiceBillingEvent{}).Where("platform_order_id = ?", order.PlatformOrderID).Count(&events).Error)
	require.NoError(t, model.DB.Model(&model.ServiceBillingOutbox{}).Count(&outboxes).Error)
	require.EqualValues(t, 1, refunds)
	require.EqualValues(t, 1, events)
	require.EqualValues(t, 1, outboxes)
}

func TestMediaKitFinanceEventKeepsExternalCostAttribution(t *testing.T) {
	now := time.Now().Unix()
	order := &model.SeedanceOrder{
		PlatformOrderID: "order-mediakit-finance", NewAPITaskID: "task-mediakit-finance",
		ChannelID: 941, InstanceID: "instance-941", Model: "Public video",
		VolcengineCostStatus: model.SeedanceCostEstimated,
		FinanceRevision:      3,
		ModelSaleMicroRMB:    8_000_000, ServiceChargeTotalMicroRMB: 1_800_000,
		VolcengineEstimatedMicroRMB: 3_000_000, PricingSnapshotHash: model.SHA256Evidence("price"),
	}
	providerCost := int64(900_000)
	usage := &model.MediaServiceUsage{
		ServiceLineItemID: order.PlatformOrderID + ":video-processing",
		PlatformOrderID:   order.PlatformOrderID,
		ServiceType:       model.SeedanceServiceTypeVideoSuperResolution,
		ProviderType:      model.SeedanceProviderDirect,
		ServiceCode:       model.SeedanceMediaKitServiceCode,
		ExecutionTaskID:   "mediakit-task-0001", Status: model.SeedanceUsageSucceeded,
		ChargeMicroRMB: 1_800_000, ProviderCostMicroRMB: &providerCost,
		PriceVersion: "price-v1", Revision: 1, StartedAt: now - 30,
	}
	event, _, err := buildSeedanceBillingEvent(order, usage, model.SeedanceOrderSucceeded, `{}`, now)
	require.NoError(t, err)
	var payload struct {
		SourceOrder struct {
			SourceRevision int `json:"source_revision"`
		} `json:"source_order"`
		ServiceUsage struct {
			ProviderType        string `json:"provider_type"`
			ServiceCode         string `json:"service_code"`
			PricingSnapshotHash string `json:"pricing_snapshot_hash"`
			UsageEvidenceHash   string `json:"usage_evidence_hash"`
		} `json:"service_usage"`
	}
	require.NoError(t, common.UnmarshalJsonStr(event.PayloadJSON, &payload))
	require.Equal(t, 3, payload.SourceOrder.SourceRevision)
	require.Equal(t, model.SeedanceProviderDirect, payload.ServiceUsage.ProviderType)
	require.Equal(t, model.SeedanceMediaKitServiceCode, payload.ServiceUsage.ServiceCode)
	require.Len(t, payload.ServiceUsage.PricingSnapshotHash, 71)
	require.Len(t, payload.ServiceUsage.UsageEvidenceHash, 71)
	require.NotContains(t, event.PayloadJSON, model.SeedanceAdapterVolcengineMediaKit)
}

func pointerInt64(value int64) *int64 {
	return &value
}
