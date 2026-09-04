package model

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	SeedanceCustomerRefundReady   = "READY"
	SeedanceCustomerRefundApplied = "APPLIED"
)

// SeedanceCustomerRefund is the durable, order-scoped idempotency boundary for
// returning the NewAPI customer's pre-consumed quota. It is created in the
// same transaction as the failed/cancelled order transition, so a process can
// crash immediately after that commit without losing the refund obligation.
type SeedanceCustomerRefund struct {
	ID                          int64  `json:"id" gorm:"primaryKey"`
	PlatformOrderID             string `json:"platform_order_id" gorm:"type:varchar(64);not null;uniqueIndex"`
	NewAPITaskID                string `json:"newapi_task_id" gorm:"type:varchar(191);not null;uniqueIndex"`
	UserID                      int    `json:"user_id" gorm:"not null;index"`
	TokenID                     int    `json:"token_id" gorm:"index"`
	ChannelID                   int    `json:"channel_id" gorm:"not null;index"`
	Quota                       int    `json:"quota" gorm:"not null"`
	FundingSource               string `json:"funding_source" gorm:"type:varchar(32);not null"`
	SubscriptionID              int    `json:"subscription_id,omitempty" gorm:"index"`
	SubscriptionPreConsumed     int64  `json:"subscription_pre_consumed,omitempty" gorm:"not null"`
	SubscriptionRequestID       string `json:"-" gorm:"type:varchar(64);index"`
	Reason                      string `json:"reason" gorm:"type:text;not null"`
	Status                      string `json:"status" gorm:"type:varchar(32);not null;index"`
	AppliedAt                   int64  `json:"applied_at,omitempty"`
	LogRecordedAt               int64  `json:"log_recorded_at,omitempty" gorm:"index"`
	FinanceSettlementRecordedAt int64  `json:"finance_settlement_recorded_at,omitempty" gorm:"index"`
	CreatedAt                   int64  `json:"created_at"`
	UpdatedAt                   int64  `json:"updated_at"`
}

// QueueSeedanceCustomerRefundTx must be called inside the order-terminal
// transaction. A positive quota always produces exactly one durable refund
// obligation for the logical order.
func QueueSeedanceCustomerRefundTx(tx *gorm.DB, order *SeedanceOrder, task *Task, reason string) (*SeedanceCustomerRefund, error) {
	if tx == nil || order == nil || task == nil {
		return nil, errors.New("incomplete Seedance customer refund input")
	}
	if task.Quota <= 0 {
		return nil, nil
	}
	fundingSource := strings.TrimSpace(task.PrivateData.BillingSource)
	if fundingSource == "" {
		fundingSource = "wallet"
	}
	subscriptionPreConsumed := task.PrivateData.SubscriptionPreConsumed
	if fundingSource == "subscription" && subscriptionPreConsumed <= 0 {
		// Legacy tasks did not persist the exact subscription reservation. All
		// priced Seedance tasks used quota-denominated subscriptions, for which
		// the task quota is the compatible recovery value.
		subscriptionPreConsumed = int64(task.Quota)
	}
	now := time.Now().Unix()
	refund := &SeedanceCustomerRefund{
		PlatformOrderID:         order.PlatformOrderID,
		NewAPITaskID:            task.TaskID,
		UserID:                  task.UserId,
		TokenID:                 task.PrivateData.TokenId,
		ChannelID:               task.ChannelId,
		Quota:                   task.Quota,
		FundingSource:           fundingSource,
		SubscriptionID:          task.PrivateData.SubscriptionId,
		SubscriptionPreConsumed: subscriptionPreConsumed,
		SubscriptionRequestID:   strings.TrimSpace(task.PrivateData.LogRequestID),
		Reason:                  strings.TrimSpace(reason),
		Status:                  SeedanceCustomerRefundReady,
		CreatedAt:               now,
		UpdatedAt:               now,
	}
	if err := tx.Create(refund).Error; err != nil {
		return nil, err
	}
	return refund, nil
}

type seedanceRefundBatchSnapshot struct {
	userQuota        int
	tokenQuota       int
	userUsedQuota    int
	channelUsedQuota int
	requestCount     int
}

// lockSeedanceRefundBatchSnapshot freezes the in-memory batch accumulator while
// the refund transaction flushes the affected keys. Without this step, a
// pre-consume that is still waiting in the batch queue could be applied after
// the refund and leave the durable balance wrong.
func lockSeedanceRefundBatchSnapshot(refund *SeedanceCustomerRefund) (seedanceRefundBatchSnapshot, func(bool)) {
	if refund == nil || !common.BatchUpdateEnabled {
		return seedanceRefundBatchSnapshot{}, func(bool) {}
	}
	for i := 0; i < BatchUpdateTypeCount; i++ {
		batchUpdateLocks[i].Lock()
	}
	snapshot := seedanceRefundBatchSnapshot{
		userQuota:        batchUpdateStores[BatchUpdateTypeUserQuota][refund.UserID],
		tokenQuota:       batchUpdateStores[BatchUpdateTypeTokenQuota][refund.TokenID],
		userUsedQuota:    batchUpdateStores[BatchUpdateTypeUsedQuota][refund.UserID],
		channelUsedQuota: batchUpdateStores[BatchUpdateTypeChannelUsedQuota][refund.ChannelID],
		requestCount:     batchUpdateStores[BatchUpdateTypeRequestCount][refund.UserID],
	}
	return snapshot, func(committed bool) {
		if committed {
			delete(batchUpdateStores[BatchUpdateTypeUserQuota], refund.UserID)
			if refund.TokenID > 0 {
				delete(batchUpdateStores[BatchUpdateTypeTokenQuota], refund.TokenID)
			}
			delete(batchUpdateStores[BatchUpdateTypeUsedQuota], refund.UserID)
			if refund.ChannelID > 0 {
				delete(batchUpdateStores[BatchUpdateTypeChannelUsedQuota], refund.ChannelID)
			}
			delete(batchUpdateStores[BatchUpdateTypeRequestCount], refund.UserID)
		}
		for i := BatchUpdateTypeCount - 1; i >= 0; i-- {
			batchUpdateLocks[i].Unlock()
		}
	}
}

// ApplySeedanceCustomerRefund applies every local monetary/statistical reversal
// and the APPLIED marker in one database transaction. Re-running an already
// applied record is a no-op. The returned bool reports whether this call won
// the idempotency gate.
func ApplySeedanceCustomerRefund(refundID int64) (*SeedanceCustomerRefund, bool, error) {
	if refundID <= 0 {
		return nil, false, errors.New("invalid Seedance customer refund id")
	}
	var initial SeedanceCustomerRefund
	if err := DB.Where("id = ?", refundID).First(&initial).Error; err != nil {
		return nil, false, err
	}
	batchSnapshot, unlockBatch := lockSeedanceRefundBatchSnapshot(&initial)
	committed := false
	defer func() { unlockBatch(committed) }()

	var applied SeedanceCustomerRefund
	newlyApplied := false
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", refundID).First(&applied).Error; err != nil {
			return err
		}
		if applied.Status == SeedanceCustomerRefundApplied {
			return nil
		}
		if applied.Status != SeedanceCustomerRefundReady {
			return fmt.Errorf("unsupported Seedance customer refund status %q", applied.Status)
		}
		if applied.Quota <= 0 {
			return errors.New("Seedance customer refund quota must be positive")
		}

		userQuotaDelta := batchSnapshot.userQuota
		if applied.FundingSource != "subscription" {
			userQuotaDelta += applied.Quota
		}
		userUpdates := map[string]any{
			"used_quota":    gorm.Expr("used_quota + ?", batchSnapshot.userUsedQuota-applied.Quota),
			"request_count": gorm.Expr("request_count + ?", batchSnapshot.requestCount),
		}
		if userQuotaDelta != 0 {
			userUpdates["quota"] = gorm.Expr("quota + ?", userQuotaDelta)
		}
		userUpdate := tx.Model(&User{}).Where("id = ?", applied.UserID).Updates(userUpdates)
		if userUpdate.Error != nil {
			return userUpdate.Error
		}
		if userUpdate.RowsAffected == 0 {
			return fmt.Errorf("Seedance refund user %d no longer exists", applied.UserID)
		}

		if applied.FundingSource == "subscription" {
			if applied.SubscriptionID <= 0 {
				return errors.New("Seedance subscription refund is missing subscription id")
			}
			shouldAdjustSubscription := true
			if applied.SubscriptionRequestID != "" {
				var record SubscriptionPreConsumeRecord
				recordQuery := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
					Where("request_id = ?", applied.SubscriptionRequestID).First(&record)
				if recordQuery.Error == nil {
					if record.Status == "refunded" {
						shouldAdjustSubscription = false
					} else if record.Status != "consumed" {
						return fmt.Errorf("unsupported subscription pre-consume status %q", record.Status)
					} else {
						record.Status = "refunded"
						if err := tx.Save(&record).Error; err != nil {
							return err
						}
					}
				} else if !errors.Is(recordQuery.Error, gorm.ErrRecordNotFound) {
					return recordQuery.Error
				}
			}
			if shouldAdjustSubscription {
				var subscription UserSubscription
				if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
					Where("id = ?", applied.SubscriptionID).First(&subscription).Error; err != nil {
					return err
				}
				amount := applied.SubscriptionPreConsumed
				if amount <= 0 {
					amount = int64(applied.Quota)
				}
				subscription.AmountUsed -= amount
				if subscription.AmountUsed < 0 {
					subscription.AmountUsed = 0
				}
				if err := tx.Save(&subscription).Error; err != nil {
					return err
				}
			}
		}

		if applied.TokenID > 0 {
			tokenDelta := batchSnapshot.tokenQuota + applied.Quota
			if tokenDelta != 0 {
				if err := tx.Model(&Token{}).Where("id = ?", applied.TokenID).Updates(map[string]any{
					"remain_quota":  gorm.Expr("remain_quota + ?", tokenDelta),
					"used_quota":    gorm.Expr("used_quota - ?", tokenDelta),
					"accessed_time": time.Now().Unix(),
				}).Error; err != nil {
					return err
				}
			}
		}
		if applied.ChannelID > 0 {
			channelDelta := batchSnapshot.channelUsedQuota - applied.Quota
			if channelDelta != 0 {
				if err := tx.Model(&Channel{}).Where("id = ?", applied.ChannelID).
					Update("used_quota", gorm.Expr("used_quota + ?", channelDelta)).Error; err != nil {
					return err
				}
			}
		}

		now := time.Now().Unix()
		if err := tx.Model(&SeedanceCustomerRefund{}).Where("id = ? AND status = ?", applied.ID, SeedanceCustomerRefundReady).
			Updates(map[string]any{"status": SeedanceCustomerRefundApplied, "applied_at": now, "updated_at": now}).Error; err != nil {
			return err
		}
		applied.Status = SeedanceCustomerRefundApplied
		applied.AppliedAt = now
		applied.UpdatedAt = now
		newlyApplied = true
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	committed = newlyApplied
	if newlyApplied {
		// Redis is derived state. Deleting the affected entries after the durable
		// transaction makes the next read hydrate the fully flushed DB values.
		_ = InvalidateUserCache(applied.UserID)
		_ = InvalidateUserTokensCache(applied.UserID)
	}
	return &applied, newlyApplied, nil
}

func MarkSeedanceCustomerRefundLogRecorded(refundID int64) error {
	now := time.Now().Unix()
	return DB.Model(&SeedanceCustomerRefund{}).Where("id = ? AND status = ?", refundID, SeedanceCustomerRefundApplied).
		Updates(map[string]any{"log_recorded_at": now, "updated_at": now}).Error
}

func MarkSeedanceCustomerRefundFinanceRecorded(refundID int64) error {
	now := time.Now().Unix()
	return DB.Model(&SeedanceCustomerRefund{}).Where("id = ? AND status = ?", refundID, SeedanceCustomerRefundApplied).
		Updates(map[string]any{"finance_settlement_recorded_at": now, "updated_at": now}).Error
}
