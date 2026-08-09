package service

import (
	"fmt"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestBillingSessionRefundMarksZeroChargeAIPDDOrderNotCharged(t *testing.T) {
	previousDB := model.DB
	dsn := fmt.Sprintf("file:billing-session-%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	t.Cleanup(func() { model.DB = previousDB })
	require.NoError(t, db.AutoMigrate(
		&model.AIPDDFinanceOrder{},
		&model.AIPDDFinanceMovement{},
		&model.AIPDDFinanceOutbox{},
	))

	const (
		instanceID = "11111111-2222-4333-8444-555555555555"
		orderID    = "trusted-zero-charge-order"
		attemptID  = "trusted-zero-charge-order:0:1"
	)
	require.NoError(t, model.EnsureAIPDDFinanceOrder(
		instanceID, orderID, attemptID, 1, 2, 3, "test-model",
	))

	finance := &relaycommon.AIPDDFinanceContext{
		InstanceID: instanceID, PlatformOrderID: orderID, AttemptID: attemptID, ChannelID: 3,
	}
	session := &BillingSession{
		relayInfo: &relaycommon.RelayInfo{AIPDDFinance: finance},
		trusted:   true,
	}

	session.Refund(nil)

	var order model.AIPDDFinanceOrder
	require.NoError(t, db.Where("platform_order_id = ?", orderID).First(&order).Error)
	require.Equal(t, "NOT_CHARGED", order.LocalBillingStatus)
	require.Zero(t, order.CustomerChargeQuota)
	require.True(t, session.settled)
	require.False(t, session.refunded)

	var movements int64
	require.NoError(t, db.Model(&model.AIPDDFinanceMovement{}).
		Where("finance_order_id = ? AND component = ?", order.ID, "ORDER_STATUS").
		Count(&movements).Error)
	require.EqualValues(t, 1, movements)
}

func TestBillingSessionRefundDoesNotMarkUncertainFundingNotCharged(t *testing.T) {
	session := &BillingSession{
		relayInfo:      &relaycommon.RelayInfo{},
		fundingSettled: true,
	}

	session.Refund(nil)

	require.False(t, session.settled)
	require.False(t, session.refunded)
}
