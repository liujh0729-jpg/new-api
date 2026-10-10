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
	require.NoError(t, db.AutoMigrate(&model.AIPDDTransitOrder{}))

	const (
		instanceID = "11111111-2222-4333-8444-555555555555"
		orderID    = "trusted-zero-charge-order"
	)
	require.NoError(t, model.EnsureAIPDDTransitOrder(
		instanceID, orderID, orderID+":0:3", 1, 2, 3, 0, "test-model",
	))

	finance := &relaycommon.AIPDDFinanceContext{
		InstanceID: instanceID, PlatformOrderID: orderID, ChannelID: 3,
	}
	session := &BillingSession{
		relayInfo: &relaycommon.RelayInfo{AIPDDFinance: finance},
		trusted:   true,
	}

	session.Refund(nil)

	var order model.AIPDDTransitOrder
	require.NoError(t, db.Where("platform_order_id = ?", orderID).First(&order).Error)
	require.Equal(t, model.AIPDDTransitFailed, order.Status)
	require.Zero(t, order.CustomerChargeQuota)
	require.True(t, session.settled)
	require.False(t, session.refunded)
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
