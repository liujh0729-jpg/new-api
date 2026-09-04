package controller

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestPrepareTaskAndWorkflowOrderPersistsGenerationIntent(t *testing.T) {
	previousDB := model.DB
	dsn := fmt.Sprintf("file:controller-seedance-submit-intent-%d?mode=memory&cache=shared", time.Now().UnixNano())
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
	common.CryptoSecret = strings.Repeat("k", 32)
	common.CryptoSecretConfigured = true
	t.Cleanup(func() {
		common.CryptoSecret = previousSecret
		common.CryptoSecretConfigured = previousConfigured
	})
	arkCredential, err := common.EncryptSensitiveValue("ark-controller-intent-secret")
	require.NoError(t, err)
	billingCredential, err := common.EncryptSensitiveValue("billing-controller-intent-secret")
	require.NoError(t, err)
	now := time.Now().Unix()
	const channelID = 9501
	config := &model.SeedanceChannelConfig{
		ChannelID: channelID, Revision: 7, InstanceID: "30000000-0000-0000-0000-000000000501",
		AIPDDBillingBaseURL: "https://billing.example.test", AIPDDBillingCredentialEncrypted: billingCredential,
		Status: model.SeedanceConfigActive, CreatedAt: now, UpdatedAt: now,
	}
	credential := &model.SeedanceVolcengineCredential{
		ChannelID: channelID, Version: 3, ArkAPIKeyEncrypted: arkCredential,
		Fingerprint: "sha256:ark", MaskedSuffix: "****", Status: model.SeedanceCredentialActive,
		CreatedAt: now,
	}
	provider := &model.MediaEnhancementProvider{
		Version: 1, ProviderType: model.SeedanceProviderDirect, AdapterType: model.SeedanceAdapterGenericHTTP,
		DisplayName: "private", ServiceEndpoint: "https://provider.example.test", ServiceCode: "private-service",
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
		ModelSaleMicroRMB: 8_000_000, ServiceChargeMicroRMB: 1_000_000,
		VolcengineUnitCostMicroRMB: 2_000_000, PricingVersion: "price-v1",
		Enabled: true, PublishedAt: now, CreatedAt: now, UpdatedAt: now,
	}
	require.NoError(t, db.Create(offering).Error)

	info := &relaycommon.RelayInfo{
		UserId: 9501, TokenId: 9502, OriginModelName: offering.DisplayName,
		UsingGroup: "VIP1", RequestURLPath: "/v1/videos",
		RequestId: "request-controller-intent", BillingSource: "wallet",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelId: channelID, ChannelType: constant.ChannelTypeSeedance,
			UpstreamModelName: offering.ProviderModelID,
		},
		TaskRelayInfo: &relaycommon.TaskRelayInfo{PublicTaskID: "task-controller-intent"},
		TaskPricingQuote: &billing_setting.TaskPricingQuote{
			Unit: billing_setting.TaskPricingUnitCall, Variant: "price-v1", Quantity: 1,
			UnitPriceUSD: 8, GroupRatio: 0.78, SaleUSD: 6.24, Quota: 6240,
		},
	}
	info.PriceData.Quota = 6240
	info.PriceData.ModelPrice = 8
	info.PriceData.GroupRatioInfo.GroupRatio = 0.78

	task, err := prepareTaskAndWorkflowOrder(info, constant.TaskActionGenerate)
	require.NoError(t, err)
	require.Equal(t, info.PublicTaskID, task.TaskID)
	require.Equal(t, 6240, task.Quota)
	require.Empty(t, task.PrivateData.UpstreamTaskID)
	require.Equal(t, offering.DisplayName, task.Properties.UpstreamModelName, "generic task projection must keep only the public model")

	var order model.SeedanceOrder
	require.NoError(t, db.Where("new_api_task_id = ?", task.TaskID).First(&order).Error)
	require.Equal(t, model.SeedanceOrderGenerationSubmitting, order.OrderStatus)
	require.Equal(t, int64(6_240_000), order.ModelSaleMicroRMB)
	require.Equal(t, config.Revision, order.AIPDDBillingConfigRevision)
	require.Equal(t, credential.Version, order.CredentialVersion)
	var pricingSnapshot struct {
		AdapterType string `json:"adapter_type"`
	}
	require.NoError(t, common.UnmarshalJsonStr(order.PricingSnapshotJSON, &pricingSnapshot))
	require.Equal(t, model.SeedanceAdapterGenericHTTP, pricingSnapshot.AdapterType)
	var attempt model.SeedanceAttempt
	require.NoError(t, db.Where("platform_order_id = ? AND stage = ?", order.PlatformOrderID, "GENERATION").First(&attempt).Error)
	require.Equal(t, "SUBMITTING", attempt.Status)
	require.Empty(t, attempt.ExternalTaskID)
	require.NotEmpty(t, attempt.AttemptID)
}
