package service

import (
	"fmt"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestResolveAIPDDFinanceInstanceIDDerivesStableIdentityFromAPIKey(t *testing.T) {
	t.Setenv(aipddInstanceIDEnv, "")

	first, err := resolveAIPDDFinanceInstanceID("sk-aipdd-test")
	require.NoError(t, err)
	second, err := resolveAIPDDFinanceInstanceID("sk-aipdd-test")
	require.NoError(t, err)
	other, err := resolveAIPDDFinanceInstanceID("sk-aipdd-other")
	require.NoError(t, err)

	require.Equal(t, first, second)
	require.NotEqual(t, first, other)
	parsed, err := uuid.Parse(first)
	require.NoError(t, err)
	require.Equal(t, uuid.Version(8), parsed.Version())
}

func TestResolveAIPDDFinanceInstanceIDHonorsExplicitOverride(t *testing.T) {
	const configured = "aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee"
	t.Setenv(aipddInstanceIDEnv, configured)

	resolved, err := resolveAIPDDFinanceInstanceID("sk-aipdd-test")

	require.NoError(t, err)
	require.Equal(t, configured, resolved)
}

func TestPrepareAIPDDFinanceAttemptSkipsMultiKeyChannel(t *testing.T) {
	t.Setenv(aipddInstanceIDEnv, "11111111-2222-4333-8444-555555555555")
	c := aipddFinanceTestContext(constant.ChannelTypeAIPDD, 7)
	c.Set(string(constant.ContextKeyChannelIsMultiKey), true)
	info := &relaycommon.RelayInfo{RequestId: "finance-order", OriginModelName: "test-model"}

	err := PrepareAIPDDFinanceAttempt(c, info)

	// Multi-key channels relay without a finance order instead of failing the request,
	// matching how the reconciliation worker skips them.
	require.NoError(t, err)
	require.Nil(t, info.AIPDDFinance)
}

func TestPrepareAIPDDFinanceAttemptClosesPreviousChannel(t *testing.T) {
	previousDB := model.DB
	dsn := fmt.Sprintf("file:aipdd-finance-prepare-%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	t.Cleanup(func() { model.DB = previousDB })
	require.NoError(t, db.AutoMigrate(
		&model.AIPDDFinanceOrder{},
		&model.AIPDDFinanceMovement{},
		&model.AIPDDFinanceOutbox{},
	))

	const orderID = "cross-channel-finance-order"
	t.Setenv(aipddInstanceIDEnv, "")
	instanceID, err := resolveAIPDDFinanceInstanceID("sk-aipdd-test")
	require.NoError(t, err)
	require.NoError(t, model.EnsureAIPDDFinanceOrder(
		instanceID, orderID, orderID+":0:1", 11, 22, 1, "test-model"))
	c := aipddFinanceTestContext(constant.ChannelTypeAIPDD, 2)
	info := &relaycommon.RelayInfo{
		RequestId: orderID, OriginModelName: "test-model", RetryIndex: 1, UserId: 11, TokenId: 22,
		AIPDDFinance: &relaycommon.AIPDDFinanceContext{
			InstanceID: instanceID, PlatformOrderID: orderID, AttemptID: orderID + ":0:1", ChannelID: 1,
		},
	}

	require.NoError(t, PrepareAIPDDFinanceAttempt(c, info))
	require.NotNil(t, info.AIPDDFinance)
	require.Equal(t, 2, info.AIPDDFinance.ChannelID)
	require.Equal(t, orderID+":1:2", info.AIPDDFinance.AttemptID)

	var previous model.AIPDDFinanceOrder
	require.NoError(t, db.Where(
		"instance_id = ? AND platform_order_id = ? AND channel_id = ?", instanceID, orderID, 1,
	).First(&previous).Error)
	require.Equal(t, "NOT_CHARGED", previous.LocalBillingStatus)
}

func aipddFinanceTestContext(channelType, channelID int) *gin.Context {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Set(string(constant.ContextKeyChannelType), channelType)
	c.Set(string(constant.ContextKeyChannelId), channelID)
	c.Set(string(constant.ContextKeyChannelKey), "sk-aipdd-test")
	return c
}
