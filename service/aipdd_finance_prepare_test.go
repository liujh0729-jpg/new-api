package service

import (
	"fmt"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestPrepareAIPDDFinanceAttemptWithoutSiteIdentityStillKeepsLedger(t *testing.T) {
	previousDB := model.DB
	dsn := fmt.Sprintf("file:aipdd-finance-unconfigured-%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	t.Cleanup(func() { model.DB = previousDB })
	require.NoError(t, db.AutoMigrate(&model.AIPDDTransitOrder{}))
	t.Setenv(aipddInstanceIDEnv, "")
	c := aipddFinanceTestContext(constant.ChannelTypeAIPDD, 5)
	info := &relaycommon.RelayInfo{RequestId: "finance-unconfigured-order", OriginModelName: "test-model"}

	require.NoError(t, PrepareAIPDDFinanceAttempt(c, info))

	// The ledger must survive an unbound deployment, but no site identity may be
	// invented for the upstream request.
	require.NotNil(t, info.AIPDDFinance, "local accounting must continue without a site identity")
	require.Empty(t, info.AIPDDFinance.InstanceID)
	require.NotNil(t, info.ChannelMeta)
	var order model.AIPDDTransitOrder
	require.NoError(t, db.Where("platform_order_id = ?", "finance-unconfigured-order").First(&order).Error)
	require.Empty(t, order.InstanceID)
	require.Equal(t, model.AIPDDTransitPending, order.Status)
}

func TestRecordAIPDDFinanceSettlementWritesEstimateWithoutSiteIdentity(t *testing.T) {
	previousDB := model.DB
	dsn := fmt.Sprintf("file:aipdd-finance-estimate-%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	t.Cleanup(func() { model.DB = previousDB })
	require.NoError(t, db.AutoMigrate(&model.AIPDDTransitOrder{}))
	constant.ResetAIPDDTokenPrices()
	t.Cleanup(constant.ResetAIPDDTokenPrices)
	constant.SetAIPDDTokenPrices([]constant.AIPDDModelTokenPrice{{
		ModelName: "ap-glm-5.2", PromptPerMillion: 80_000, CompletionPerMillion: 280_000,
	}}, 0.0001)
	require.NoError(t, model.EnsureAIPDDTransitOrder(
		"", "estimate-only-order", "estimate-only-order:0:1", 11, 22, 1, 0, "ap-glm-5.2"))

	// No upstream settlement read is attempted because InstanceID is empty, so this
	// must return without touching the network.
	require.NoError(t, recordAIPDDFinanceSettlement(
		&relaycommon.AIPDDFinanceContext{PlatformOrderID: "estimate-only-order"},
		33, "CHARGED", AIPDDCostBasis{
			ModelName: "ap-glm-5.2",
			Usage:     &dto.Usage{PromptTokens: 1_000, CompletionTokens: 500},
		}))

	var order model.AIPDDTransitOrder
	require.NoError(t, db.Where("platform_order_id = ?", "estimate-only-order").First(&order).Error)
	require.NotNil(t, order.EstimatedSourceAWCoin)
	require.EqualValues(t, 220, *order.EstimatedSourceAWCoin)
	require.NotNil(t, order.EstimatedSourceRMBMic)
	require.EqualValues(t, 22_000, *order.EstimatedSourceRMBMic)
	require.Nil(t, order.SourceChargeAWCoin)
	require.Nil(t, order.SourceChargeRMBMic)
	// Still PENDING: only an upstream settlement read moves the row to SETTLED,
	// and that read is skipped while no site identity is configured.
	require.Equal(t, model.AIPDDTransitPending, order.Status)
}

func TestResolveAIPDDFinanceInstanceIDEmptyWhenUnconfigured(t *testing.T) {
	t.Setenv(aipddInstanceIDEnv, "")

	resolved, err := resolveAIPDDFinanceInstanceID()

	require.NoError(t, err)
	require.Empty(t, resolved)
}

func TestResolveAIPDDFinanceInstanceIDRejectsMalformedOverride(t *testing.T) {
	t.Setenv(aipddInstanceIDEnv, "site-1057")

	resolved, err := resolveAIPDDFinanceInstanceID()

	require.ErrorContains(t, err, aipddInstanceIDEnv+" must be a UUID")
	require.Empty(t, resolved)
}

func TestResolveAIPDDFinanceInstanceIDHonorsExplicitOverride(t *testing.T) {
	const configured = "aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee"
	t.Setenv(aipddInstanceIDEnv, configured)

	resolved, err := resolveAIPDDFinanceInstanceID()

	require.NoError(t, err)
	require.Equal(t, configured, resolved)
}

func TestPrepareAIPDDFinanceAttemptDisabledKeepsRelayAvailable(t *testing.T) {
	t.Setenv(aipddFinanceEnabledEnv, "false")
	c := aipddFinanceTestContext(constant.ChannelTypeAIPDD, 3)
	info := &relaycommon.RelayInfo{
		RequestId: "finance-disabled-order", OriginModelName: "test-model",
		UserId: 1, TokenId: 2,
		AIPDDFinance: &relaycommon.AIPDDFinanceContext{
			InstanceID: uuid.NewString(), PlatformOrderID: "finance-disabled-order", ChannelID: 1,
		},
	}
	require.NoError(t, PrepareAIPDDFinanceAttempt(c, info))
	require.Nil(t, info.AIPDDFinance)
	require.NotNil(t, info.ChannelMeta)
}

func TestPrepareAIPDDFinanceAttemptSupportsMultiKeyChannel(t *testing.T) {
	previousDB := model.DB
	dsn := fmt.Sprintf("file:aipdd-finance-multi-key-%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	t.Cleanup(func() { model.DB = previousDB })
	require.NoError(t, db.AutoMigrate(&model.AIPDDTransitOrder{}))
	t.Setenv(aipddInstanceIDEnv, "11111111-2222-4333-8444-555555555555")
	c := aipddFinanceTestContext(constant.ChannelTypeAIPDD, 7)
	c.Set(string(constant.ContextKeyChannelIsMultiKey), true)
	c.Set(string(constant.ContextKeyChannelMultiKeyIndex), 3)
	info := &relaycommon.RelayInfo{RequestId: "finance-order", OriginModelName: "test-model"}

	err = PrepareAIPDDFinanceAttempt(c, info)

	require.NoError(t, err)
	require.NotNil(t, info.AIPDDFinance)
	require.Equal(t, 3, info.AIPDDFinance.ChannelKeyIndex)
	require.Equal(t, "finance-order:0:7", info.AIPDDFinance.AttemptID)
	var order model.AIPDDTransitOrder
	require.NoError(t, db.Where("platform_order_id = ?", "finance-order").First(&order).Error)
	require.Equal(t, 3, order.ChannelKeyIndex)
	require.Equal(t, "finance-order:0:7", order.LatestAttemptID)
}

func TestPrepareAIPDDFinanceAttemptClosesPreviousChannel(t *testing.T) {
	previousDB := model.DB
	dsn := fmt.Sprintf("file:aipdd-finance-prepare-%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	t.Cleanup(func() { model.DB = previousDB })
	require.NoError(t, db.AutoMigrate(&model.AIPDDTransitOrder{}))

	const orderID = "cross-channel-finance-order"
	const instanceID = "11111111-2222-4333-8444-555555555555"
	t.Setenv(aipddInstanceIDEnv, instanceID)
	require.NoError(t, model.EnsureAIPDDTransitOrder(
		instanceID, orderID, orderID+":0:1", 11, 22, 1, 0, "test-model"))
	c := aipddFinanceTestContext(constant.ChannelTypeAIPDD, 2)
	info := &relaycommon.RelayInfo{
		RequestId: orderID, OriginModelName: "test-model", RetryIndex: 1, UserId: 11, TokenId: 22,
		AIPDDFinance: &relaycommon.AIPDDFinanceContext{
			InstanceID: instanceID, PlatformOrderID: orderID, ChannelID: 1,
		},
	}

	require.NoError(t, PrepareAIPDDFinanceAttempt(c, info))
	require.NotNil(t, info.AIPDDFinance)
	require.Equal(t, 2, info.AIPDDFinance.ChannelID)
	require.Equal(t, orderID+":1:2", info.AIPDDFinance.AttemptID)
	require.Equal(t, 11, info.AIPDDFinance.NewAPIUserID)
	require.Equal(t, 22, info.AIPDDFinance.NewAPITokenID)

	var order model.AIPDDTransitOrder
	require.NoError(t, db.Where("platform_order_id = ?", orderID).First(&order).Error)
	require.Equal(t, 2, order.ChannelID)
	require.Equal(t, orderID+":1:2", order.LatestAttemptID)
	require.Equal(t, model.AIPDDTransitPending, order.Status)
}

func aipddFinanceTestContext(channelType, channelID int) *gin.Context {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Set(string(constant.ContextKeyChannelType), channelType)
	c.Set(string(constant.ContextKeyChannelId), channelID)
	c.Set(string(constant.ContextKeyChannelKey), "sk-aipdd-test")
	return c
}
