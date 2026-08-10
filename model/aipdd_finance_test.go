package model

import (
	"errors"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestAIPDDFinanceRefundPendingEscalatesWithoutUnsafeRetry(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(
		&AIPDDFinanceOrder{}, &AIPDDFinanceMovement{}, &AIPDDFinanceInbox{},
		&AIPDDFinanceOutbox{}, &AIPDDFinanceCursor{}))
	instanceID := uuid.NewString()
	orderID := "req-refund-review-" + uuid.NewString()
	require.NoError(t, EnsureAIPDDFinanceOrder(instanceID, orderID, orderID+":0:9", 1, 2, 9, "test-model"))
	require.NoError(t, MarkAIPDDFinanceRefundPending(instanceID, orderID, 9))
	require.NoError(t, DB.Model(&AIPDDFinanceOrder{}).
		Where("instance_id = ? AND platform_order_id = ? AND channel_id = ?", instanceID, orderID, 9).
		Update("updated_at", time.Now().Unix()-901).Error)
	require.NoError(t, MarkStaleAIPDDFinanceRefundsForReview())
	var order AIPDDFinanceOrder
	require.NoError(t, DB.Where("instance_id = ? AND platform_order_id = ? AND channel_id = ?", instanceID, orderID, 9).First(&order).Error)
	require.Equal(t, "REFUND_REVIEW_REQUIRED", order.LocalBillingStatus)

	settlementOrderID := "req-settlement-review-" + uuid.NewString()
	require.NoError(t, EnsureAIPDDFinanceOrder(instanceID, settlementOrderID, settlementOrderID+":0:9", 1, 2, 9, "test-model"))
	require.NoError(t, BeginLocalAIPDDFinanceSettlement(instanceID, settlementOrderID, 9, 123, 456))
	require.NoError(t, DB.Model(&AIPDDFinanceOrder{}).
		Where("instance_id = ? AND platform_order_id = ? AND channel_id = ?", instanceID, settlementOrderID, 9).
		Update("updated_at", time.Now().Unix()-901).Error)
	require.NoError(t, MarkStaleAIPDDFinanceRefundsForReview())
	order = AIPDDFinanceOrder{}
	require.NoError(t, DB.Where("instance_id = ? AND platform_order_id = ? AND channel_id = ?", instanceID, settlementOrderID, 9).First(&order).Error)
	require.Equal(t, "SETTLEMENT_REVIEW_REQUIRED", order.LocalBillingStatus)
	require.EqualValues(t, 123, order.PendingChargeQuota)
	require.EqualValues(t, 456, order.PendingChargeRMBMic)
}

func TestAIPDDFinanceSettlementAndInboxAreIdempotent(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(
		&AIPDDFinanceOrder{}, &AIPDDFinanceMovement{}, &AIPDDFinanceInbox{},
		&AIPDDFinanceOutbox{}, &AIPDDFinanceCursor{}))
	require.NoError(t, DB.Session(&gormSessionAllowGlobalUpdate).Delete(&AIPDDFinanceCursor{}).Error)
	require.NoError(t, DB.Session(&gormSessionAllowGlobalUpdate).Delete(&AIPDDFinanceInbox{}).Error)
	require.NoError(t, DB.Session(&gormSessionAllowGlobalUpdate).Delete(&AIPDDFinanceOutbox{}).Error)
	require.NoError(t, DB.Session(&gormSessionAllowGlobalUpdate).Delete(&AIPDDFinanceMovement{}).Error)
	require.NoError(t, DB.Session(&gormSessionAllowGlobalUpdate).Delete(&AIPDDFinanceOrder{}).Error)

	instanceID := uuid.NewString()
	orderID := "req-finance-idempotency"
	require.NoError(t, EnsureAIPDDFinanceOrder(instanceID, orderID, orderID+":0:8", 1, 2, 8, "test-model"))
	require.NoError(t, RecordLocalAIPDDFinanceSettlement(instanceID, orderID, 8, 1000, 20_000, `{"quota_per_unit":"500000"}`, "CHARGED"))
	require.NoError(t, RecordLocalAIPDDFinanceSettlement(instanceID, orderID, 8, 1000, 20_000, `{"quota_per_unit":"500000"}`, "CHARGED"))

	var movementCount int64
	require.NoError(t, DB.Model(&AIPDDFinanceMovement{}).Count(&movementCount).Error)
	require.EqualValues(t, 1, movementCount)

	spend := "0.010000"
	providerCharge := "0.010000"
	zero := "0.000000"
	event := AIPDDSettlementEnvelope{
		SchemaVersion: 1, EventID: uuid.NewString(), Sequence: 4,
		EventType: "ORDER_SETTLED", SettlementRevision: 1,
		Order: AIPDDSettlementOrderData{
			PlatformOrderID: orderID, LatestAttemptID: orderID + ":0:8", InstanceID: instanceID,
			NewAPIUserID: "1", NewAPITokenID: "2", Model: "test-model",
			OrderStatus: "SUCCEEDED", CostStatus: "CONFIRMED", SettlementRevision: 1,
			CustomerChargeAWCoin: "50", CustomerChargeRMB: &providerCharge, BaseModelCostRMB: &spend,
			AIPDDModelCostRMB: &zero, ActualSpendRMB: &spend,
		},
	}
	payload, err := common.Marshal(event)
	require.NoError(t, err)
	sequence := event.Sequence
	require.NoError(t, ApplyAIPDDSettlementEnvelope(&event, payload, 8, &sequence))
	require.NoError(t, ApplyAIPDDSettlementEnvelope(&event, payload, 8, &sequence))

	var inboxCount int64
	require.NoError(t, DB.Model(&AIPDDFinanceInbox{}).Count(&inboxCount).Error)
	require.EqualValues(t, 1, inboxCount)

	var order AIPDDFinanceOrder
	require.NoError(t, DB.Where("platform_order_id = ?", orderID).First(&order).Error)
	require.Equal(t, "CHARGED", order.LocalBillingStatus)
	require.Equal(t, "SUCCEEDED", order.OrderStatus)
	require.NotNil(t, order.ActualSpendRMBMic)
	require.EqualValues(t, 10_000, *order.ActualSpendRMBMic)
	require.NotNil(t, order.ProfitRMBMic)
	require.EqualValues(t, 10_000, *order.ProfitRMBMic)
	cursor, err := GetAIPDDFinanceCursor(8, instanceID)
	require.NoError(t, err)
	require.EqualValues(t, 4, cursor)

	event.EventID = uuid.NewString()
	event.Sequence = 5
	event.SettlementRevision = 2
	event.Order.SettlementRevision = 2
	event.Order.CostStatus = "PARTIAL"
	payload, err = common.Marshal(event)
	require.NoError(t, err)
	sequence = event.Sequence
	require.NoError(t, ApplyAIPDDSettlementEnvelope(&event, payload, 8, &sequence))
	order = AIPDDFinanceOrder{}
	require.NoError(t, DB.Where("platform_order_id = ?", orderID).First(&order).Error)
	require.Nil(t, order.ProfitRMBMic, "pending or partial upstream cost must not be reported as confirmed profit")
}

func TestSweepOrphanAIPDDFinanceOutboxIgnoresRefunded404(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(
		&AIPDDFinanceOrder{}, &AIPDDFinanceMovement{}, &AIPDDFinanceInbox{},
		&AIPDDFinanceOutbox{}, &AIPDDFinanceCursor{}))
	require.NoError(t, DB.Session(&gormSessionAllowGlobalUpdate).Delete(&AIPDDFinanceOutbox{}).Error)
	require.NoError(t, DB.Session(&gormSessionAllowGlobalUpdate).Delete(&AIPDDFinanceOrder{}).Error)

	instanceID := uuid.NewString()
	orderID := "req-orphan-404-" + uuid.NewString()
	require.NoError(t, EnsureAIPDDFinanceOrder(instanceID, orderID, orderID+":0:11", 1, 2, 11, "test-model"))
	require.NoError(t, RecordLocalAIPDDFinanceSettlement(instanceID, orderID, 11, 0, 0, `{"quota_per_unit":"500000"}`, "REFUNDED"))

	require.NoError(t, DB.Model(&AIPDDFinanceOutbox{}).
		Where("platform_order_id = ? AND state = ?", orderID, AIPDDFinanceOutboxPending).
		Updates(map[string]interface{}{
			"last_error": "AIPDD finance endpoint returned 404: {\"detail\":\"not found\"}",
			"attempts":   8,
		}).Error)
	var pending int64
	require.NoError(t, DB.Model(&AIPDDFinanceOutbox{}).
		Where("platform_order_id = ? AND state = ?", orderID, AIPDDFinanceOutboxPending).Count(&pending).Error)
	require.Greater(t, pending, int64(0))

	closed, err := SweepOrphanAIPDDFinanceOutbox()
	require.NoError(t, err)
	require.EqualValues(t, pending, closed)

	var ignored int64
	require.NoError(t, DB.Model(&AIPDDFinanceOutbox{}).
		Where("platform_order_id = ? AND state = ?", orderID, AIPDDFinanceOutboxIgnored).Count(&ignored).Error)
	require.EqualValues(t, pending, ignored)
}

func TestCanIgnoreOrphanAIPDDFinance404RequiresTerminalLocalBilling(t *testing.T) {
	require.True(t, CanIgnoreOrphanAIPDDFinance404(&AIPDDFinanceOrder{LocalBillingStatus: "REFUNDED"}))
	require.True(t, CanIgnoreOrphanAIPDDFinance404(&AIPDDFinanceOrder{LocalBillingStatus: "NOT_CHARGED"}))
	require.False(t, CanIgnoreOrphanAIPDDFinance404(&AIPDDFinanceOrder{LocalBillingStatus: "CHARGED"}))
	require.False(t, CanIgnoreOrphanAIPDDFinance404(&AIPDDFinanceOrder{
		LocalBillingStatus: "REFUNDED", SettlementRevision: 1,
	}))
}

func TestAIPDDFinanceCursorBackoffAndPoisonSkip(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&AIPDDFinanceCursor{}))
	require.NoError(t, DB.Session(&gormSessionAllowGlobalUpdate).Delete(&AIPDDFinanceCursor{}).Error)

	instanceID := uuid.NewString()
	require.NoError(t, RecordAIPDDFinancePullFailure(21, instanceID, errors.New("AIPDD finance endpoint returned 500: boom")))
	cursor, err := GetAIPDDFinanceCursorRecord(21, instanceID)
	require.NoError(t, err)
	require.NotNil(t, cursor)
	require.Equal(t, 1, cursor.ConsecutiveFailures)
	require.Greater(t, cursor.NextPullAt, time.Now().Unix())

	shouldPull, _, err := ShouldPullAIPDDFinanceEvents(21, instanceID, time.Now().Unix())
	require.NoError(t, err)
	require.False(t, shouldPull)

	require.NoError(t, RecordAIPDDFinancePoisonEvent(21, instanceID, 9, errors.New("invalid envelope")))
	require.NoError(t, SkipAIPDDFinancePoisonEvent(21, instanceID))
	cursor, err = GetAIPDDFinanceCursorRecord(21, instanceID)
	require.NoError(t, err)
	require.EqualValues(t, 9, cursor.LastSequence)
	require.EqualValues(t, 0, cursor.PoisonSequence)
	require.Equal(t, 0, cursor.ConsecutiveFailures)
	require.EqualValues(t, 0, cursor.NextPullAt)
}

var gormSessionAllowGlobalUpdate = gorm.Session{AllowGlobalUpdate: true}
