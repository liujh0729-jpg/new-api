package model

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	AIPDDFinanceOutboxPending = "PENDING"
	AIPDDFinanceOutboxDone    = "DONE"
	AIPDDFinanceOutboxIgnored = "IGNORED"
	AIPDDFinanceOutboxDead    = "DEAD"

	aipddFinanceOutboxMaxAttempts = 20
)

// AIPDDFinanceOrder is a settlement mirror, not a projection of ordinary request logs.
type AIPDDFinanceOrder struct {
	ID                   string `gorm:"type:varchar(36);primaryKey" json:"id"`
	PlatformOrderID      string `gorm:"type:varchar(191);uniqueIndex:uk_aipdd_finance_order_scope,priority:3" json:"platform_order_id"`
	LatestAttemptID      string `gorm:"type:varchar(255)" json:"latest_attempt_id"`
	InstanceID           string `gorm:"type:varchar(36);uniqueIndex:uk_aipdd_finance_order_scope,priority:2;index" json:"instance_id"`
	RequestID            string `gorm:"type:varchar(191);index" json:"request_id"`
	UserID               int    `gorm:"index" json:"user_id"`
	TokenID              int    `gorm:"index" json:"token_id"`
	ChannelID            int    `gorm:"uniqueIndex:uk_aipdd_finance_order_scope,priority:1;index" json:"channel_id"`
	Model                string `gorm:"type:varchar(191);index" json:"model"`
	OrderStatus          string `gorm:"type:varchar(32);index" json:"order_status"`
	LocalBillingStatus   string `gorm:"type:varchar(32);default:'UNKNOWN';index" json:"local_billing_status"`
	CostStatus           string `gorm:"type:varchar(32);index" json:"cost_status"`
	SettlementRevision   int64  `gorm:"default:0" json:"settlement_revision"`
	CustomerChargeQuota  int64  `json:"customer_charge_quota"`
	CustomerChargeRMBMic int64  `json:"customer_charge_rmb_mic"`
	PendingChargeQuota   int64  `json:"pending_charge_quota"`
	PendingChargeRMBMic  int64  `json:"pending_charge_rmb_mic"`
	CustomerRateSnapshot string `gorm:"type:text" json:"customer_rate_snapshot"`
	AIPDDChargeAWCoin    int64  `json:"aipdd_charge_awcoin"`
	AIPDDChargeRMBMic    *int64 `json:"aipdd_charge_rmb_mic"`
	ActualSpendAWCoin    *int64 `json:"actual_spend_awcoin"`
	BaseModelCostRMBMic  *int64 `json:"base_model_cost_rmb_mic"`
	AIPDDModelCostRMBMic *int64 `json:"aipdd_model_cost_rmb_mic"`
	ActualSpendRMBMic    *int64 `json:"actual_spend_rmb_mic"`
	ProfitRMBMic         *int64 `json:"profit_rmb_mic"`
	UpstreamReference    string `gorm:"type:varchar(191)" json:"upstream_reference"`
	SourceType           string `gorm:"type:varchar(40)" json:"source_type"`
	SourceID             string `gorm:"type:varchar(191)" json:"source_id"`
	UpstreamSnapshot     string `gorm:"type:text" json:"upstream_snapshot"`
	OccurredAt           int64  `gorm:"index" json:"occurred_at"`
	SettledAt            *int64 `json:"settled_at"`
	CreatedAt            int64  `json:"created_at"`
	UpdatedAt            int64  `json:"updated_at"`
}

type AIPDDFinanceMovement struct {
	ID             string `gorm:"type:varchar(36);primaryKey" json:"id"`
	FinanceOrderID string `gorm:"type:varchar(36);index;not null" json:"finance_order_id"`
	IdempotencyKey string `gorm:"type:varchar(255);uniqueIndex;not null" json:"idempotency_key"`
	Component      string `gorm:"type:varchar(40);not null" json:"component"`
	QuotaDelta     int64  `json:"quota_delta"`
	RMBDeltaMic    int64  `json:"rmb_delta_mic"`
	Evidence       string `gorm:"type:text" json:"evidence"`
	OccurredAt     int64  `gorm:"index" json:"occurred_at"`
	CreatedAt      int64  `json:"created_at"`
}

type AIPDDFinanceInbox struct {
	EventID            string `gorm:"type:varchar(36);primaryKey" json:"event_id"`
	SourceSequence     int64  `gorm:"index" json:"source_sequence"`
	PlatformOrderID    string `gorm:"type:varchar(191);index" json:"platform_order_id"`
	SettlementRevision int64  `json:"settlement_revision"`
	Payload            string `gorm:"type:text" json:"payload"`
	ProcessedAt        int64  `json:"processed_at"`
	ErrorMessage       string `gorm:"type:text" json:"error_message"`
	CreatedAt          int64  `json:"created_at"`
}

type AIPDDFinanceOutbox struct {
	ID              string `gorm:"type:varchar(36);primaryKey" json:"id"`
	EventKey        string `gorm:"type:varchar(255);uniqueIndex;not null" json:"event_key"`
	PlatformOrderID string `gorm:"type:varchar(191);index" json:"platform_order_id"`
	ChannelID       int    `gorm:"index" json:"channel_id"`
	InstanceID      string `gorm:"type:varchar(36);index" json:"instance_id"`
	EventType       string `gorm:"type:varchar(40)" json:"event_type"`
	Payload         string `gorm:"type:text" json:"payload"`
	State           string `gorm:"type:varchar(20);index" json:"state"`
	Attempts        int    `json:"attempts"`
	NextAttemptAt   int64  `gorm:"index" json:"next_attempt_at"`
	LastError       string `gorm:"type:text" json:"last_error"`
	CreatedAt       int64  `json:"created_at"`
	UpdatedAt       int64  `json:"updated_at"`
}

type AIPDDFinanceCursor struct {
	ID                  string `gorm:"type:varchar(36);primaryKey" json:"id"`
	ChannelID           int    `gorm:"uniqueIndex:uk_aipdd_finance_cursor,priority:1" json:"channel_id"`
	InstanceID          string `gorm:"type:varchar(36);uniqueIndex:uk_aipdd_finance_cursor,priority:2" json:"instance_id"`
	LastSequence        int64  `json:"last_sequence"`
	NextPullAt          int64  `gorm:"index" json:"next_pull_at"`
	ConsecutiveFailures int    `json:"consecutive_failures"`
	LastPullError       string `gorm:"type:text" json:"last_pull_error"`
	LastPullErrorAt     int64  `json:"last_pull_error_at"`
	PoisonSequence      int64  `json:"poison_sequence"`
	PoisonError         string `gorm:"type:text" json:"poison_error"`
	PoisonAt            int64  `json:"poison_at"`
	UpdatedAt           int64  `json:"updated_at"`
}

func (AIPDDFinanceOrder) TableName() string    { return "aipdd_finance_order" }
func (AIPDDFinanceMovement) TableName() string { return "aipdd_finance_movement" }
func (AIPDDFinanceInbox) TableName() string    { return "aipdd_finance_inbox" }
func (AIPDDFinanceOutbox) TableName() string   { return "aipdd_finance_outbox" }
func (AIPDDFinanceCursor) TableName() string   { return "aipdd_finance_cursor" }

type AIPDDSettlementEnvelope struct {
	SchemaVersion      int                      `json:"schema_version"`
	EventID            string                   `json:"event_id"`
	Sequence           int64                    `json:"sequence"`
	EventType          string                   `json:"event_type"`
	SettlementRevision int64                    `json:"settlement_revision"`
	OccurredAt         string                   `json:"occurred_at"`
	Order              AIPDDSettlementOrderData `json:"order"`
}

type AIPDDSettlementOrderData struct {
	PlatformOrderID      string  `json:"platform_order_id"`
	LatestAttemptID      string  `json:"latest_attempt_id"`
	InstanceID           string  `json:"instance_id"`
	NewAPIUserID         string  `json:"newapi_user_id"`
	NewAPITokenID        string  `json:"newapi_token_id"`
	SourceType           string  `json:"source_type"`
	SourceID             string  `json:"source_id"`
	UpstreamReference    string  `json:"upstream_reference"`
	Model                string  `json:"model"`
	OrderStatus          string  `json:"order_status"`
	CostStatus           string  `json:"cost_status"`
	SettlementRevision   int64   `json:"settlement_revision"`
	CustomerChargeAWCoin string  `json:"customer_charge_awcoin"`
	CustomerChargeRMB    *string `json:"customer_charge_rmb"`
	ActualSpendAWCoin    *string `json:"actual_spend_awcoin"`
	BaseModelCostRMB     *string `json:"base_model_cost_rmb"`
	AIPDDModelCostRMB    *string `json:"aipdd_model_cost_rmb"`
	ActualSpendRMB       *string `json:"actual_spend_rmb"`
	ProfitRMB            *string `json:"profit_rmb"`
	OccurredAt           string  `json:"occurred_at"`
	SettledAt            *string `json:"settled_at"`
}

func EnsureAIPDDFinanceOrder(instanceID, platformOrderID, attemptID string, userID, tokenID, channelID int, modelName string) error {
	now := time.Now().Unix()
	return DB.Transaction(func(tx *gorm.DB) error {
		var existing AIPDDFinanceOrder
		err := tx.Where("channel_id = ? AND instance_id = ? AND platform_order_id = ?", channelID, instanceID, platformOrderID).First(&existing).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			existing = AIPDDFinanceOrder{
				ID: uuid.NewString(), PlatformOrderID: platformOrderID, LatestAttemptID: attemptID,
				InstanceID: instanceID, RequestID: platformOrderID, UserID: userID, TokenID: tokenID,
				ChannelID: channelID, Model: modelName, OrderStatus: "PROCESSING", LocalBillingStatus: "PENDING", CostStatus: "PENDING",
				OccurredAt: now, CreatedAt: now, UpdatedAt: now,
			}
			if err = tx.Create(&existing).Error; err != nil {
				return err
			}
		} else if err != nil {
			return err
		} else if existing.LatestAttemptID != attemptID {
			if err = tx.Model(&existing).Updates(map[string]interface{}{
				"latest_attempt_id": attemptID, "channel_id": channelID, "updated_at": now,
			}).Error; err != nil {
				return err
			}
		}
		return enqueueAIPDDFinanceOutbox(tx, financeEventKey("attempt", attemptID), platformOrderID, channelID, instanceID, "REFRESH", nil, now)
	})
}

func RecordLocalAIPDDFinanceSettlement(instanceID, platformOrderID string, channelID int, actualQuota int64, rmbMic int64, rateSnapshot string, status string) error {
	now := time.Now().Unix()
	return DB.Transaction(func(tx *gorm.DB) error {
		var order AIPDDFinanceOrder
		if err := tx.Where("instance_id = ? AND platform_order_id = ? AND channel_id = ?", instanceID, platformOrderID, channelID).First(&order).Error; err != nil {
			return err
		}
		if order.CustomerChargeQuota == actualQuota && order.LocalBillingStatus == status {
			return nil
		}
		quotaDelta := actualQuota - order.CustomerChargeQuota
		rmbDelta := rmbMic - order.CustomerChargeRMBMic
		component := "CUSTOMER_CHARGE"
		if quotaDelta == 0 && rmbDelta == 0 {
			component = "ORDER_STATUS"
		}
		movement := AIPDDFinanceMovement{
			ID: uuid.NewString(), FinanceOrderID: order.ID,
			IdempotencyKey: financeEventKey("local-settlement", channelID, instanceID, platformOrderID, actualQuota, status),
			Component:      component, QuotaDelta: quotaDelta, RMBDeltaMic: rmbDelta,
			Evidence: rateSnapshot, OccurredAt: now, CreatedAt: now,
		}
		result := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "idempotency_key"}}, DoNothing: true}).Create(&movement)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return nil
		}
		updates := map[string]interface{}{
			"customer_charge_quota": actualQuota, "customer_charge_rmb_mic": rmbMic,
			"pending_charge_quota": 0, "pending_charge_rmb_mic": 0,
			"customer_rate_snapshot": rateSnapshot, "local_billing_status": status, "updated_at": now,
		}
		if order.CostStatus == "CONFIRMED" && order.AIPDDChargeRMBMic != nil {
			profit := rmbMic - *order.AIPDDChargeRMBMic
			updates["profit_rmb_mic"] = profit
		} else {
			updates["profit_rmb_mic"] = nil
		}
		if err := tx.Model(&order).Updates(updates).Error; err != nil {
			return err
		}
		return enqueueAIPDDFinanceOutbox(tx,
			financeEventKey("settlement", channelID, instanceID, platformOrderID, actualQuota, status),
			platformOrderID, order.ChannelID, order.InstanceID, "REFRESH", nil, now)
	})
}

func BeginLocalAIPDDFinanceSettlement(instanceID, platformOrderID string, channelID int, expectedQuota, expectedRMBMic int64) error {
	result := DB.Model(&AIPDDFinanceOrder{}).
		Where("instance_id = ? AND platform_order_id = ? AND channel_id = ?", instanceID, platformOrderID, channelID).
		Updates(map[string]interface{}{
			"pending_charge_quota": expectedQuota, "pending_charge_rmb_mic": expectedRMBMic,
			"local_billing_status": "SETTLEMENT_PENDING", "updated_at": time.Now().Unix(),
		})
	return result.Error
}

func MarkAIPDDFinanceSettlementReviewRequired(instanceID, platformOrderID string, channelID int) error {
	return DB.Model(&AIPDDFinanceOrder{}).
		Where("instance_id = ? AND platform_order_id = ? AND channel_id = ?", instanceID, platformOrderID, channelID).
		Updates(map[string]interface{}{"local_billing_status": "SETTLEMENT_REVIEW_REQUIRED", "updated_at": time.Now().Unix()}).Error
}

func MarkAIPDDFinanceRefundPending(instanceID, platformOrderID string, channelID int) error {
	return DB.Model(&AIPDDFinanceOrder{}).Where("instance_id = ? AND platform_order_id = ? AND channel_id = ?", instanceID, platformOrderID, channelID).
		Updates(map[string]interface{}{"local_billing_status": "REFUND_PENDING", "updated_at": time.Now().Unix()}).Error
}

func MarkStaleAIPDDFinanceRefundsForReview() error {
	now := time.Now().Unix()
	if err := DB.Model(&AIPDDFinanceOrder{}).
		Where("local_billing_status = ? AND updated_at < ?", "REFUND_PENDING", time.Now().Unix()-900).
		Updates(map[string]interface{}{"local_billing_status": "REFUND_REVIEW_REQUIRED", "updated_at": now}).Error; err != nil {
		return err
	}
	return DB.Model(&AIPDDFinanceOrder{}).
		Where("local_billing_status = ? AND updated_at < ?", "SETTLEMENT_PENDING", now-900).
		Updates(map[string]interface{}{"local_billing_status": "SETTLEMENT_REVIEW_REQUIRED", "updated_at": now}).Error
}

func enqueueAIPDDFinanceOutbox(tx *gorm.DB, eventKey, orderID string, channelID int, instanceID, eventType string, payload any, now int64) error {
	body := "{}"
	if payload != nil {
		encoded, err := common.Marshal(payload)
		if err != nil {
			return err
		}
		body = string(encoded)
	}
	event := AIPDDFinanceOutbox{
		ID: uuid.NewString(), EventKey: eventKey, PlatformOrderID: orderID, ChannelID: channelID,
		InstanceID: instanceID, EventType: eventType, Payload: body, State: AIPDDFinanceOutboxPending,
		NextAttemptAt: now, CreatedAt: now, UpdatedAt: now,
	}
	return tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "event_key"}}, DoNothing: true}).Create(&event).Error
}

func ClaimAIPDDFinanceOutbox(limit int) ([]AIPDDFinanceOutbox, error) {
	var events []AIPDDFinanceOutbox
	err := DB.Where("state = ? AND next_attempt_at <= ?", AIPDDFinanceOutboxPending, time.Now().Unix()).
		Order("created_at asc").Limit(limit).Find(&events).Error
	return events, err
}

func CompleteAIPDDFinanceOutbox(id string) error {
	return DB.Model(&AIPDDFinanceOutbox{}).Where("id = ?", id).
		Updates(map[string]interface{}{"state": AIPDDFinanceOutboxDone, "updated_at": time.Now().Unix(), "last_error": ""}).Error
}

func CloseAIPDDFinanceOutbox(id, state, reason string) error {
	switch state {
	case AIPDDFinanceOutboxIgnored, AIPDDFinanceOutboxDead:
	default:
		return fmt.Errorf("unsupported AIPDD finance outbox close state: %s", state)
	}
	now := time.Now().Unix()
	result := DB.Model(&AIPDDFinanceOutbox{}).Where("id = ? AND state = ?", id, AIPDDFinanceOutboxPending).
		Updates(map[string]interface{}{
			"state": state, "last_error": reason, "next_attempt_at": now + 365*24*3600, "updated_at": now,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func RetryAIPDDFinanceOutbox(event *AIPDDFinanceOutbox, cause error) error {
	if event == nil {
		return errors.New("AIPDD finance outbox event is required")
	}
	attempts := event.Attempts + 1
	now := time.Now().Unix()
	if attempts >= aipddFinanceOutboxMaxAttempts {
		return DB.Model(&AIPDDFinanceOutbox{}).Where("id = ? AND state = ?", event.ID, AIPDDFinanceOutboxPending).
			Updates(map[string]interface{}{
				"state": AIPDDFinanceOutboxDead, "attempts": attempts,
				"next_attempt_at": now + 365*24*3600,
				"last_error":      fmt.Sprintf("max attempts exceeded: %s", cause.Error()),
				"updated_at":      now,
			}).Error
	}
	delay := int64(1 << min(attempts, 10))
	return DB.Model(&AIPDDFinanceOutbox{}).Where("id = ?", event.ID).Updates(map[string]interface{}{
		"attempts": attempts, "next_attempt_at": now + delay,
		"last_error": cause.Error(), "updated_at": now,
	}).Error
}

// CanIgnoreOrphanAIPDDFinance404 marks local-only failures as terminal:
// upstream never accepted the order (no settlement revision) and NewAPI already
// closed customer billing as REFUNDED / NOT_CHARGED.
func CanIgnoreOrphanAIPDDFinance404(order *AIPDDFinanceOrder) bool {
	if order == nil {
		return false
	}
	if order.SettlementRevision > 0 || order.CostStatus == "CONFIRMED" {
		return false
	}
	switch order.LocalBillingStatus {
	case "REFUNDED", "NOT_CHARGED":
		return true
	default:
		return false
	}
}

func GetAIPDDFinanceOrderByScope(channelID int, instanceID, platformOrderID string) (*AIPDDFinanceOrder, error) {
	var order AIPDDFinanceOrder
	err := DB.Where("channel_id = ? AND instance_id = ? AND platform_order_id = ?",
		channelID, instanceID, platformOrderID).First(&order).Error
	if err != nil {
		return nil, err
	}
	return &order, nil
}

func IgnoreOrphanAIPDDFinanceOutbox(event *AIPDDFinanceOutbox, reason string) error {
	if event == nil {
		return errors.New("AIPDD finance outbox event is required")
	}
	if strings.TrimSpace(reason) == "" {
		reason = "ignored orphan refresh: upstream order missing and local billing already closed"
	}
	return CloseAIPDDFinanceOutbox(event.ID, AIPDDFinanceOutboxIgnored, reason)
}

// SweepOrphanAIPDDFinanceOutbox closes PENDING refresh jobs that already failed with
// upstream 404 while local billing is terminal. Used to drain historical backlog
// without waiting for AIPDD to create those orders.
func SweepOrphanAIPDDFinanceOutbox() (int64, error) {
	var events []AIPDDFinanceOutbox
	if err := DB.Where("state = ? AND last_error LIKE ?", AIPDDFinanceOutboxPending, "%returned 404%").
		Order("created_at asc").Limit(200).Find(&events).Error; err != nil {
		return 0, err
	}
	var closed int64
	for index := range events {
		event := &events[index]
		order, err := GetAIPDDFinanceOrderByScope(event.ChannelID, event.InstanceID, event.PlatformOrderID)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				if closeErr := IgnoreOrphanAIPDDFinanceOutbox(event,
					"ignored orphan refresh: local finance order missing and upstream returned 404"); closeErr == nil {
					closed++
				}
			}
			continue
		}
		if !CanIgnoreOrphanAIPDDFinance404(order) {
			continue
		}
		if err = IgnoreOrphanAIPDDFinanceOutbox(event,
			"ignored orphan refresh: upstream returned 404 and local billing already closed"); err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				continue
			}
			return closed, err
		}
		closed++
	}
	return closed, nil
}

func ApplyAIPDDSettlementEnvelope(envelope *AIPDDSettlementEnvelope, payload []byte, channelID int, cursorSequence *int64) error {
	if envelope == nil || envelope.SchemaVersion != 1 || envelope.EventID == "" || envelope.Order.PlatformOrderID == "" || envelope.Order.InstanceID == "" {
		return errors.New("invalid AIPDD settlement envelope")
	}
	if envelope.Order.SettlementRevision != 0 && envelope.Order.SettlementRevision != envelope.SettlementRevision {
		return errors.New("AIPDD settlement revision mismatch")
	}
	now := time.Now().Unix()
	return DB.Transaction(func(tx *gorm.DB) error {
		inbox := AIPDDFinanceInbox{
			EventID: envelope.EventID, SourceSequence: envelope.Sequence,
			PlatformOrderID: envelope.Order.PlatformOrderID, SettlementRevision: envelope.SettlementRevision,
			Payload: string(payload), ProcessedAt: now, CreatedAt: now,
		}
		result := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "event_id"}}, DoNothing: true}).Create(&inbox)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected > 0 {
			if err := applyAIPDDOrderSnapshot(tx, envelope, channelID, now); err != nil {
				return err
			}
		}
		if cursorSequence != nil {
			cursor := AIPDDFinanceCursor{ID: uuid.NewString(), ChannelID: channelID, InstanceID: envelope.Order.InstanceID, LastSequence: *cursorSequence, UpdatedAt: now}
			if err := tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "channel_id"}, {Name: "instance_id"}},
				DoUpdates: clause.Assignments(map[string]interface{}{"last_sequence": *cursorSequence, "updated_at": now}),
			}).Create(&cursor).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func applyAIPDDOrderSnapshot(tx *gorm.DB, envelope *AIPDDSettlementEnvelope, channelID int, now int64) error {
	data := envelope.Order
	chargeAWCoin, err := requiredInt64(data.CustomerChargeAWCoin, "customer_charge_awcoin")
	if err != nil {
		return err
	}
	chargeRMB, err := moneyMic(data.CustomerChargeRMB, "customer_charge_rmb")
	if err != nil {
		return err
	}
	actualSpendAWCoin, err := optionalInt64(data.ActualSpendAWCoin, "actual_spend_awcoin")
	if err != nil {
		return err
	}
	baseCost, err := moneyMic(data.BaseModelCostRMB, "base_model_cost_rmb")
	if err != nil {
		return err
	}
	aipddCost, err := moneyMic(data.AIPDDModelCostRMB, "aipdd_model_cost_rmb")
	if err != nil {
		return err
	}
	actualSpend, err := moneyMic(data.ActualSpendRMB, "actual_spend_rmb")
	if err != nil {
		return err
	}
	var order AIPDDFinanceOrder
	err = tx.Where("channel_id = ? AND instance_id = ? AND platform_order_id = ?", channelID, data.InstanceID, data.PlatformOrderID).First(&order).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		order = AIPDDFinanceOrder{
			ID: uuid.NewString(), InstanceID: data.InstanceID, PlatformOrderID: data.PlatformOrderID,
			RequestID: data.PlatformOrderID, UserID: atoi(data.NewAPIUserID), TokenID: atoi(data.NewAPITokenID),
			ChannelID: channelID, LocalBillingStatus: "UNKNOWN", OccurredAt: now, CreatedAt: now,
		}
	} else if err != nil {
		return err
	}
	if order.SettlementRevision >= envelope.SettlementRevision {
		return nil
	}
	previous := order
	order.LatestAttemptID = data.LatestAttemptID
	order.Model = data.Model
	order.OrderStatus = data.OrderStatus
	order.CostStatus = data.CostStatus
	order.SettlementRevision = envelope.SettlementRevision
	order.AIPDDChargeAWCoin = chargeAWCoin
	order.AIPDDChargeRMBMic = chargeRMB
	order.ActualSpendAWCoin = actualSpendAWCoin
	order.BaseModelCostRMBMic = baseCost
	order.AIPDDModelCostRMBMic = aipddCost
	order.ActualSpendRMBMic = actualSpend
	order.UpstreamReference = data.UpstreamReference
	order.SourceType = data.SourceType
	order.SourceID = data.SourceID
	order.UpstreamSnapshot = stringMust(common.Marshal(envelope))
	order.UpdatedAt = now
	if occurredAt, ok := parseAIPDDTime(data.OccurredAt); ok {
		order.OccurredAt = occurredAt
	}
	if data.SettledAt != nil {
		if settledAt, ok := parseAIPDDTime(*data.SettledAt); ok {
			order.SettledAt = &settledAt
		}
	}
	if order.CostStatus == "CONFIRMED" && order.AIPDDChargeRMBMic != nil {
		profit := order.CustomerChargeRMBMic - *order.AIPDDChargeRMBMic
		order.ProfitRMBMic = &profit
	} else {
		order.ProfitRMBMic = nil
	}
	if err := recordAIPDDSettlementRevisionMovements(tx, previous, order, envelope, now); err != nil {
		return err
	}
	return tx.Save(&order).Error
}

func recordAIPDDSettlementRevisionMovements(tx *gorm.DB, previous, current AIPDDFinanceOrder, envelope *AIPDDSettlementEnvelope, now int64) error {
	type componentDelta struct {
		name       string
		quotaDelta int64
		rmbDelta   int64
	}
	deltas := []componentDelta{
		{name: "AIPDD_SOURCE_CHARGE", quotaDelta: current.AIPDDChargeAWCoin - previous.AIPDDChargeAWCoin,
			rmbDelta: pointerValue(current.AIPDDChargeRMBMic) - pointerValue(previous.AIPDDChargeRMBMic)},
		{name: "ACTUAL_SPEND_AWCOIN", quotaDelta: pointerValue(current.ActualSpendAWCoin) - pointerValue(previous.ActualSpendAWCoin)},
		{name: "BASE_MODEL_COST", rmbDelta: pointerValue(current.BaseModelCostRMBMic) - pointerValue(previous.BaseModelCostRMBMic)},
		{name: "AIPDD_MODEL_COST", rmbDelta: pointerValue(current.AIPDDModelCostRMBMic) - pointerValue(previous.AIPDDModelCostRMBMic)},
		{name: "ACTUAL_SPEND", rmbDelta: pointerValue(current.ActualSpendRMBMic) - pointerValue(previous.ActualSpendRMBMic)},
	}
	evidence := stringMust(common.Marshal(map[string]any{
		"event_id": envelope.EventID, "sequence": envelope.Sequence,
		"settlement_revision": envelope.SettlementRevision, "cost_status": current.CostStatus,
	}))
	for _, delta := range deltas {
		if delta.quotaDelta == 0 && delta.rmbDelta == 0 {
			continue
		}
		movement := AIPDDFinanceMovement{
			ID: uuid.NewString(), FinanceOrderID: current.ID,
			IdempotencyKey: financeEventKey("upstream-revision", envelope.EventID, delta.name),
			Component:      delta.name, QuotaDelta: delta.quotaDelta, RMBDeltaMic: delta.rmbDelta,
			Evidence: evidence, OccurredAt: now, CreatedAt: now,
		}
		if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "idempotency_key"}}, DoNothing: true}).Create(&movement).Error; err != nil {
			return err
		}
	}
	return nil
}

func pointerValue(value *int64) int64 {
	if value == nil {
		return 0
	}
	return *value
}

func GetAIPDDFinanceCursor(channelID int, instanceID string) (int64, error) {
	cursor, err := GetAIPDDFinanceCursorRecord(channelID, instanceID)
	if err != nil {
		return 0, err
	}
	if cursor == nil {
		return 0, nil
	}
	return cursor.LastSequence, nil
}

func GetAIPDDFinanceCursorRecord(channelID int, instanceID string) (*AIPDDFinanceCursor, error) {
	var cursor AIPDDFinanceCursor
	err := DB.Where("channel_id = ? AND instance_id = ?", channelID, instanceID).First(&cursor).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &cursor, nil
}

func ensureAIPDDFinanceCursor(channelID int, instanceID string) (*AIPDDFinanceCursor, error) {
	cursor, err := GetAIPDDFinanceCursorRecord(channelID, instanceID)
	if err != nil {
		return nil, err
	}
	if cursor != nil {
		return cursor, nil
	}
	now := time.Now().Unix()
	cursor = &AIPDDFinanceCursor{
		ID: uuid.NewString(), ChannelID: channelID, InstanceID: instanceID, UpdatedAt: now,
	}
	if err = DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "channel_id"}, {Name: "instance_id"}},
		DoNothing: true,
	}).Create(cursor).Error; err != nil {
		return nil, err
	}
	return GetAIPDDFinanceCursorRecord(channelID, instanceID)
}

func ShouldPullAIPDDFinanceEvents(channelID int, instanceID string, now int64) (bool, *AIPDDFinanceCursor, error) {
	cursor, err := GetAIPDDFinanceCursorRecord(channelID, instanceID)
	if err != nil {
		return false, nil, err
	}
	if cursor == nil {
		return true, nil, nil
	}
	if cursor.NextPullAt > now {
		return false, cursor, nil
	}
	return true, cursor, nil
}

func RecordAIPDDFinancePullFailure(channelID int, instanceID string, cause error) error {
	cursor, err := ensureAIPDDFinanceCursor(channelID, instanceID)
	if err != nil {
		return err
	}
	if cursor == nil {
		return errors.New("failed to ensure AIPDD finance cursor")
	}
	now := time.Now().Unix()
	failures := cursor.ConsecutiveFailures + 1
	delay := int64(1 << min(failures, 10))
	message := ""
	if cause != nil {
		message = cause.Error()
	}
	return DB.Model(&AIPDDFinanceCursor{}).Where("id = ?", cursor.ID).Updates(map[string]interface{}{
		"consecutive_failures": failures,
		"next_pull_at":         now + delay,
		"last_pull_error":      message,
		"last_pull_error_at":   now,
		"updated_at":           now,
	}).Error
}

func RecordAIPDDFinancePullSuccess(channelID int, instanceID string) error {
	cursor, err := GetAIPDDFinanceCursorRecord(channelID, instanceID)
	if err != nil || cursor == nil {
		return err
	}
	if cursor.ConsecutiveFailures == 0 && cursor.NextPullAt == 0 &&
		cursor.LastPullError == "" && cursor.PoisonSequence == 0 {
		return nil
	}
	now := time.Now().Unix()
	return DB.Model(&AIPDDFinanceCursor{}).Where("id = ?", cursor.ID).Updates(map[string]interface{}{
		"consecutive_failures": 0,
		"next_pull_at":         0,
		"last_pull_error":      "",
		"last_pull_error_at":   0,
		"poison_sequence":      0,
		"poison_error":         "",
		"poison_at":            0,
		"updated_at":           now,
	}).Error
}

func RecordAIPDDFinancePoisonEvent(channelID int, instanceID string, sequence int64, cause error) error {
	cursor, err := ensureAIPDDFinanceCursor(channelID, instanceID)
	if err != nil {
		return err
	}
	if cursor == nil {
		return errors.New("failed to ensure AIPDD finance cursor")
	}
	now := time.Now().Unix()
	failures := cursor.ConsecutiveFailures + 1
	delay := int64(1 << min(failures, 10))
	message := ""
	if cause != nil {
		message = cause.Error()
	}
	return DB.Model(&AIPDDFinanceCursor{}).Where("id = ?", cursor.ID).Updates(map[string]interface{}{
		"poison_sequence":      sequence,
		"poison_error":         message,
		"poison_at":            now,
		"consecutive_failures": failures,
		"next_pull_at":         now + delay,
		"last_pull_error":      fmt.Sprintf("poison event sequence %d: %s", sequence, message),
		"last_pull_error_at":   now,
		"updated_at":           now,
	}).Error
}

func SkipAIPDDFinancePoisonEvent(channelID int, instanceID string) error {
	cursor, err := GetAIPDDFinanceCursorRecord(channelID, instanceID)
	if err != nil {
		return err
	}
	if cursor == nil || cursor.PoisonSequence <= 0 {
		return errors.New("no poison AIPDD finance event to skip")
	}
	now := time.Now().Unix()
	lastSequence := cursor.LastSequence
	if cursor.PoisonSequence > lastSequence {
		lastSequence = cursor.PoisonSequence
	}
	return DB.Model(&AIPDDFinanceCursor{}).Where("id = ?", cursor.ID).Updates(map[string]interface{}{
		"last_sequence":        lastSequence,
		"poison_sequence":      0,
		"poison_error":         "",
		"poison_at":            0,
		"consecutive_failures": 0,
		"next_pull_at":         0,
		"last_pull_error":      "",
		"last_pull_error_at":   0,
		"updated_at":           now,
	}).Error
}

func GetAIPDDChannelsForFinance() ([]Channel, error) {
	var channels []Channel
	err := DB.Where("type = ?", constant.ChannelTypeAIPDD).Order("id asc").Find(&channels).Error
	return channels, err
}

func moneyMic(value *string, field string) (*int64, error) {
	if value == nil {
		return nil, nil
	}
	d, err := decimal.NewFromString(*value)
	if err != nil {
		return nil, fmt.Errorf("invalid %s: %w", field, err)
	}
	mic := d.Mul(decimal.NewFromInt(1_000_000)).Round(0).IntPart()
	return &mic, nil
}

func optionalInt64(value *string, field string) (*int64, error) {
	if value == nil {
		return nil, nil
	}
	n, err := requiredInt64(*value, field)
	if err != nil {
		return nil, err
	}
	return &n, nil
}

func requiredInt64(value string, field string) (int64, error) {
	n, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid %s: %w", field, err)
	}
	return n, nil
}
func financeEventKey(kind string, values ...any) string {
	return kind + ":" + uuid.NewSHA1(uuid.NameSpaceOID, []byte(fmt.Sprintf("%#v", values))).String()
}
func parseAIPDDTime(value string) (int64, bool) {
	parsed, err := time.Parse("2006-01-02T15:04:05", value)
	if err != nil {
		return 0, false
	}
	return parsed.UTC().Unix(), true
}
func atoi(value string) int { n, _ := strconv.Atoi(value); return n }
func stringMust(value []byte, err error) string {
	if err != nil {
		return "{}"
	}
	return string(value)
}
