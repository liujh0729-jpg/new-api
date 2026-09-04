package controller

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestSeedanceAdminMetricsExposeRequiredFailureAndUnknownSubmissionLabels(t *testing.T) {
	previousDB := model.DB
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	t.Cleanup(func() {
		if sqlDB, sqlErr := db.DB(); sqlErr == nil {
			_ = sqlDB.Close()
		}
		model.DB = previousDB
	})
	require.NoError(t, db.AutoMigrate(
		&model.Channel{}, &model.SeedanceOrder{}, &model.MediaServiceUsage{},
		&model.ServiceBillingEvent{}, &model.ServiceBillingOutbox{},
		&model.ServiceBillingFailureAttempt{},
		&model.SeedanceCostReconciliationIssue{},
	))
	require.NoError(t, db.Create(&model.Channel{
		Id: 901, Type: constant.ChannelTypeSeedance, Name: "Seedance metrics", Key: "encrypted-elsewhere",
	}).Error)
	require.NoError(t, db.Create(&model.SeedanceOrder{
		PlatformOrderID: "order-metrics", NewAPITaskID: "task-metrics", ChannelID: 901,
		Model: "Public video", OrderStatus: model.SeedanceOrderEnhancing,
	}).Error)
	require.NoError(t, db.Create([]model.MediaServiceUsage{
		{
			ServiceLineItemID: "line-failed", PlatformOrderID: "order-metrics", AttemptID: "attempt-failed",
			ProviderType: model.SeedanceProviderDirect, ServiceCode: "private-code", Status: model.SeedanceUsageFailed,
			FailureReason: "PROVIDER_REJECTED", StartedAt: 100, CompletedAt: 105,
		},
		{
			ServiceLineItemID: "line-unknown-direct", PlatformOrderID: "order-metrics", AttemptID: "attempt-unknown-direct",
			ProviderType: model.SeedanceProviderDirect, ServiceCode: "private-code", Status: model.SeedanceUsagePending,
			FailureReason: model.SeedanceSubmissionOutcomeUnknown, UnknownSubmissionCount: 1,
		},
		{
			ServiceLineItemID: "line-unknown-internal", PlatformOrderID: "order-metrics", AttemptID: "attempt-unknown-internal",
			ProviderType: model.SeedanceProviderAIPDD, ServiceCode: "private-code", Status: model.SeedanceUsagePending,
			FailureReason: model.SeedanceSubmissionOutcomeUnknown, UnknownSubmissionCount: 1,
		},
		{
			ServiceLineItemID: "line-not-submitted", PlatformOrderID: "order-metrics", AttemptID: "attempt-not-submitted",
			ProviderType: model.SeedanceProviderDirect, ServiceCode: "private-code", Status: model.SeedanceUsagePending,
		},
	}).Error)
	require.NoError(t, db.Create(&model.ServiceBillingEvent{
		EventID: "event-metrics", PlatformOrderID: "order-metrics", ServiceLineItemID: "line-failed",
		Revision: 1, EventType: "SERVICE_SETTLEMENT_UPDATED", PayloadJSON: `{}`, PayloadHash: "sha256:event",
	}).Error)
	require.NoError(t, db.Create(&model.ServiceBillingOutbox{
		EventID: "event-metrics", Status: model.SeedanceSyncRetryWait,
	}).Error)
	require.NoError(t, db.Create([]model.ServiceBillingFailureAttempt{
		{EventID: "event-metrics", AttemptNo: 1, HTTPStatus: http.StatusServiceUnavailable},
		{EventID: "event-metrics", AttemptNo: 2, HTTPStatus: http.StatusServiceUnavailable},
		{EventID: "event-metrics", AttemptNo: 3, HTTPStatus: 0},
	}).Error)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/seedance-admin/metrics?channel_id=901", nil)

	GetSeedanceAdminMetrics(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Data struct {
			Failures []struct {
				ProviderType string `json:"provider_type"`
				ServiceCode  string `json:"service_code"`
				Reason       string `json:"reason"`
				Value        int64  `json:"value"`
			} `json:"media_enhancement_failures_total"`
			Unknown []struct {
				ProviderType string `json:"provider_type"`
				Value        int64  `json:"value"`
			} `json:"media_enhancement_unknown_submissions_total"`
			SyncFailures []struct {
				StatusCode int   `json:"status_code"`
				Value      int64 `json:"value"`
			} `json:"seedance_billing_sync_failures_total"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.Len(t, response.Data.Failures, 1)
	require.Len(t, response.Data.Unknown, 2)
	require.Equal(t, "PROVIDER_REJECTED", response.Data.Failures[0].Reason)
	require.EqualValues(t, 1, response.Data.Failures[0].Value)
	require.ElementsMatch(t, []string{model.SeedanceProviderDirect, model.SeedanceProviderAIPDD}, []string{
		response.Data.Unknown[0].ProviderType, response.Data.Unknown[1].ProviderType,
	})
	for _, metric := range response.Data.Unknown {
		require.EqualValues(t, 1, metric.Value)
	}
	require.ElementsMatch(t, []struct {
		StatusCode int   `json:"status_code"`
		Value      int64 `json:"value"`
	}{
		{StatusCode: 0, Value: 1},
		{StatusCode: http.StatusServiceUnavailable, Value: 2},
	}, response.Data.SyncFailures)
	rows, total, err := model.ListServiceBillingOutbox(model.ServiceBillingOutboxQuery{
		ChannelID: 901, PlatformOrderID: "order-metrics", Limit: 10,
	})
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Len(t, rows, 1)
	require.Equal(t, "order-metrics", rows[0].PlatformOrderID)
	require.Equal(t, "line-failed", rows[0].ServiceLineItemID)
	_, total, err = model.ListServiceBillingOutbox(model.ServiceBillingOutboxQuery{ChannelID: 902, Limit: 10})
	require.NoError(t, err)
	require.Zero(t, total)
}

func TestSeedanceProviderPoliciesRejectConfigurationTheRuntimeCannotHonor(t *testing.T) {
	valid := &model.MediaEnhancementProvider{
		CapabilitiesJSON:   `{}`,
		TimeoutPolicyJSON:  `{"timeout_seconds":600}`,
		RetryPolicyJSON:    `{"mode":"SAME_ATTEMPT_UNKNOWN_ONLY"}`,
		FallbackPolicyJSON: `{"mode":"NONE"}`,
	}
	require.NoError(t, validateSeedanceProviderPolicies(valid))

	unsupportedRetry := *valid
	unsupportedRetry.RetryPolicyJSON = `{"mode":"SAME_ATTEMPT_UNKNOWN_ONLY","max_attempts":3}`
	require.ErrorContains(t, validateSeedanceProviderPolicies(&unsupportedRetry), "unsupported field")

	unsupportedFallback := *valid
	unsupportedFallback.FallbackPolicyJSON = `{"mode":"NEXT_PROVIDER"}`
	require.ErrorContains(t, validateSeedanceProviderPolicies(&unsupportedFallback), "must be NONE")

	invalidShape := *valid
	invalidShape.TimeoutPolicyJSON = `[]`
	require.ErrorContains(t, validateSeedanceProviderPolicies(&invalidShape), "JSON object")
}

func TestSeedanceAdminOrderViewSurfacesUnknownEnhancementSubmission(t *testing.T) {
	previousDB := model.DB
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	t.Cleanup(func() {
		if sqlDB, sqlErr := db.DB(); sqlErr == nil {
			_ = sqlDB.Close()
		}
		model.DB = previousDB
	})
	require.NoError(t, db.AutoMigrate(&model.SeedanceOrder{}, &model.MediaServiceUsage{}))
	order := model.SeedanceOrder{
		PlatformOrderID: "order-admin-diagnostic", NewAPITaskID: "task-admin-diagnostic",
		Model: "Public video", OrderStatus: model.SeedanceOrderEnhancing,
	}
	require.NoError(t, db.Create(&order).Error)
	require.NoError(t, db.Create(&model.MediaServiceUsage{
		ServiceLineItemID: "line-admin-diagnostic", PlatformOrderID: order.PlatformOrderID,
		ServiceType: model.SeedanceServiceTypeVideoSuperResolution,
		ProviderType: model.SeedanceProviderDirect, ServiceCode: model.SeedanceMediaKitServiceCode,
		SpecificationVersion: "spec-v1", AttemptID: "attempt-admin-diagnostic",
		Status: model.SeedanceUsagePending, FailureReason: model.SeedanceSubmissionOutcomeUnknown,
		PriceVersion: "price-v1", Revision: 1,
	}).Error)

	views, err := newSeedanceOrderViews([]model.SeedanceOrder{order})
	require.NoError(t, err)
	require.Len(t, views, 1)
	require.Equal(t, model.SeedanceSubmissionOutcomeUnknown, views[0].EnhancementStatus)
	require.Equal(t, model.SeedanceSubmissionOutcomeUnknown, views[0].EnhancementFailureReason)
}

func TestTrustedSeedanceServiceEndpointRejectsEmbeddedCredentialsAndQuery(t *testing.T) {
	require.True(t, isTrustedServiceEndpoint("https://services.example.com/private/tasks"))
	require.True(t, isTrustedServiceEndpoint("http://127.0.0.1:8080/tasks"))
	require.False(t, isTrustedServiceEndpoint("https://user:secret@services.example.com/tasks"))
	require.False(t, isTrustedServiceEndpoint("https://services.example.com/tasks?api_key=secret"))
	require.False(t, isTrustedServiceEndpoint("http://services.example.com/tasks"))
}
