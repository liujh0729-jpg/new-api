package model

import (
	"errors"
	"math"
	"math/big"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	SeedanceBillAllocated              = "ALLOCATED"
	SeedanceBillVarianceOnly           = "VARIANCE_ONLY"
	SeedanceBillReconciliationRequired = "RECONCILIATION_REQUIRED"
	SeedanceBillCostArkGeneration      = "ARK_GENERATION"
	SeedanceBillCostSuperResolution    = "SUPER_RESOLUTION"
	SeedanceReconciliationOpen         = "OPEN"
	SeedanceReconciliationResolved     = "RESOLVED"
	SeedanceBillCursorIdle             = "IDLE"
	SeedanceBillCursorSyncing          = "SYNCING"
	SeedanceBillCursorFailed           = "FAILED"
)

func NormalizeSeedanceBillCostCategory(value string) (string, error) {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case SeedanceBillCostSuperResolution:
		return SeedanceBillCostSuperResolution, nil
	case "", SeedanceBillCostArkGeneration:
		return SeedanceBillCostArkGeneration, nil
	default:
		return "", errors.New("unsupported Seedance bill cost_category")
	}
}

func isSeedanceOrderTerminal(status string) bool {
	return status == SeedanceOrderSucceeded || status == SeedanceOrderFailed || status == SeedanceOrderCancelled
}

type SeedanceVolcengineBillCursor struct {
	ID            int64  `json:"id" gorm:"primaryKey"`
	ChannelID     int    `json:"channel_id" gorm:"not null;uniqueIndex:idx_seedance_bill_cursor_period"`
	BillingPeriod string `json:"billing_period" gorm:"type:varchar(32);not null;uniqueIndex:idx_seedance_bill_cursor_period"`
	Cursor        string `json:"cursor" gorm:"type:text"`
	Status        string `json:"status" gorm:"type:varchar(32);not null"`
	LastError     string `json:"last_error,omitempty" gorm:"type:text"`
	LastSyncAt    int64  `json:"last_sync_at,omitempty"`
	LeaseOwner    string `json:"-" gorm:"type:varchar(128);index"`
	LeaseUntil    int64  `json:"-" gorm:"index"`
	NextAttemptAt int64  `json:"next_attempt_at,omitempty" gorm:"index"`
	UpdatedAt     int64  `json:"updated_at"`
}

type SeedanceVolcengineBillItem struct {
	ID                    int64  `json:"id" gorm:"primaryKey"`
	ChannelID             int    `json:"channel_id" gorm:"not null;index;uniqueIndex:idx_seedance_bill_revision"`
	BillDetailID          string `json:"bill_detail_id" gorm:"type:varchar(191);not null;uniqueIndex:idx_seedance_bill_revision"`
	Revision              int    `json:"revision" gorm:"not null;uniqueIndex:idx_seedance_bill_revision"`
	BillingPeriod         string `json:"billing_period" gorm:"type:varchar(32);not null;index"`
	ProductCode           string `json:"product_code" gorm:"type:varchar(128);not null;index"`
	CostCategory          string `json:"cost_category" gorm:"type:varchar(32);not null;default:'ARK_GENERATION';index"`
	InstanceID            string `json:"instance_id,omitempty" gorm:"type:varchar(191);index"`
	UsageStartedAt        int64  `json:"usage_started_at,omitempty" gorm:"index"`
	UsageEndedAt          int64  `json:"usage_ended_at,omitempty"`
	AmountMicroRMB        int64  `json:"amount_micro_rmb" gorm:"not null"`
	SourcePayloadHash     string `json:"source_payload_hash" gorm:"type:varchar(80);not null"`
	SanitizedSourceJSON   string `json:"sanitized_source" gorm:"column:sanitized_source;type:text;not null"`
	AllocationStatus      string `json:"allocation_status" gorm:"type:varchar(32);not null;index"`
	RevisionEventQueuedAt int64  `json:"revision_event_queued_at,omitempty" gorm:"index"`
	CreatedAt             int64  `json:"created_at"`
}

type SeedanceCostAllocation struct {
	ID                int64  `json:"id" gorm:"primaryKey"`
	BillItemID        int64  `json:"bill_item_id" gorm:"not null;uniqueIndex:idx_seedance_cost_allocation;index"`
	PlatformOrderID   string `json:"platform_order_id" gorm:"type:varchar(64);not null;uniqueIndex:idx_seedance_cost_allocation;index"`
	BillRevision      int    `json:"bill_revision" gorm:"not null"`
	Weight            int64  `json:"weight" gorm:"not null"`
	AllocatedMicroRMB int64  `json:"allocated_micro_rmb" gorm:"not null"`
	RemainderRank     int    `json:"remainder_rank" gorm:"not null"`
	RuleVersion       string `json:"rule_version" gorm:"type:varchar(64);not null"`
	CreatedAt         int64  `json:"created_at"`
}

type SeedanceCostReconciliationIssue struct {
	ID           int64  `json:"id" gorm:"primaryKey"`
	BillItemID   int64  `json:"bill_item_id" gorm:"not null;index"`
	ChannelID    int    `json:"channel_id" gorm:"not null;index"`
	ReasonCode   string `json:"reason_code" gorm:"type:varchar(64);not null;index"`
	EvidenceJSON string `json:"evidence" gorm:"column:evidence;type:text;not null"`
	Status       string `json:"status" gorm:"type:varchar(32);not null;index"`
	CreatedAt    int64  `json:"created_at"`
	ResolvedAt   int64  `json:"resolved_at,omitempty"`
}

type SeedanceCostCandidate struct {
	PlatformOrderID string `json:"platform_order_id"`
	Weight          int64  `json:"weight"`
}

type SeedanceCostAllocationResult struct {
	PlatformOrderID   string `json:"platform_order_id"`
	Weight            int64  `json:"weight"`
	AllocatedMicroRMB int64  `json:"allocated_micro_rmb"`
	RemainderRank     int    `json:"remainder_rank"`
}

func ClaimSeedanceVolcengineBillCursor(channelID int, billingPeriod string, intervalSeconds int64, leaseOwner string) (*SeedanceVolcengineBillCursor, bool, error) {
	billingPeriod = strings.TrimSpace(billingPeriod)
	leaseOwner = strings.TrimSpace(leaseOwner)
	if channelID <= 0 || billingPeriod == "" || leaseOwner == "" {
		return nil, false, errors.New("bill cursor channel, period and lease owner are required")
	}
	if intervalSeconds <= 0 {
		intervalSeconds = 3600
	}
	now := time.Now().Unix()
	item := &SeedanceVolcengineBillCursor{
		ChannelID: channelID, BillingPeriod: billingPeriod, Cursor: "0", Status: SeedanceBillCursorIdle,
		NextAttemptAt: now, UpdatedAt: now,
	}
	if err := DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "channel_id"}, {Name: "billing_period"}},
		DoNothing: true,
	}).Create(item).Error; err != nil {
		return nil, false, err
	}
	var existing SeedanceVolcengineBillCursor
	if err := DB.Where("channel_id = ? AND billing_period = ?", channelID, billingPeriod).First(&existing).Error; err != nil {
		return nil, false, err
	}
	result := DB.Model(&SeedanceVolcengineBillCursor{}).
		Where("id = ? AND next_attempt_at <= ? AND (lease_until = 0 OR lease_until < ?)", existing.ID, now, now).
		Updates(map[string]any{
			"status": SeedanceBillCursorSyncing, "lease_owner": leaseOwner, "lease_until": now + 120,
			"next_attempt_at": now + intervalSeconds, "last_error": "", "updated_at": now,
		})
	if result.Error != nil {
		return nil, false, result.Error
	}
	if result.RowsAffected == 0 {
		return &existing, false, nil
	}
	if err := DB.Where("id = ?", existing.ID).First(&existing).Error; err != nil {
		return nil, false, err
	}
	return &existing, true, nil
}

func SaveSeedanceVolcengineBillCursorProgress(id int64, leaseOwner string, offset int32) error {
	now := time.Now().Unix()
	result := DB.Model(&SeedanceVolcengineBillCursor{}).
		Where("id = ? AND lease_owner = ? AND status = ?", id, leaseOwner, SeedanceBillCursorSyncing).
		Updates(map[string]any{"cursor": strconv.FormatInt(int64(offset), 10), "lease_until": now + 120, "updated_at": now})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return errors.New("bill cursor lease was lost")
	}
	return nil
}

func FinishSeedanceVolcengineBillCursor(id int64, leaseOwner string, syncErr error) error {
	now := time.Now().Unix()
	updates := map[string]any{
		"lease_owner": "", "lease_until": 0, "updated_at": now,
	}
	if syncErr == nil {
		updates["status"] = SeedanceBillCursorIdle
		updates["cursor"] = "0"
		updates["last_error"] = ""
		updates["last_sync_at"] = now
		updates["next_attempt_at"] = now + 3600
	} else {
		updates["status"] = SeedanceBillCursorFailed
		updates["last_error"] = truncateSeedanceCostError(syncErr.Error())
		updates["next_attempt_at"] = now + 300
	}
	result := DB.Model(&SeedanceVolcengineBillCursor{}).
		Where("id = ? AND lease_owner = ?", id, leaseOwner).Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return errors.New("bill cursor lease was lost")
	}
	return nil
}

func truncateSeedanceCostError(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= 1000 {
		return value
	}
	return value[:1000]
}

// AllocateSeedanceCost uses largest remainders and a stable order-id tie break.
// The returned micro-RMB values always sum exactly to amount, including
// negative bill corrections, and never rely on floating point arithmetic.
func AllocateSeedanceCost(amount int64, candidates []SeedanceCostCandidate) ([]SeedanceCostAllocationResult, error) {
	if amount == math.MinInt64 {
		return nil, errors.New("cost amount is outside the supported range")
	}
	if len(candidates) == 0 {
		return nil, errors.New("at least one cost candidate is required")
	}
	seen := make(map[string]struct{}, len(candidates))
	totalWeight := int64(0)
	for _, candidate := range candidates {
		candidate.PlatformOrderID = strings.TrimSpace(candidate.PlatformOrderID)
		if candidate.PlatformOrderID == "" || candidate.Weight <= 0 {
			return nil, errors.New("cost candidates require a unique order id and positive weight")
		}
		if _, ok := seen[candidate.PlatformOrderID]; ok {
			return nil, errors.New("duplicate cost candidate order id")
		}
		seen[candidate.PlatformOrderID] = struct{}{}
		if candidate.Weight > int64(^uint64(0)>>1)-totalWeight {
			return nil, errors.New("cost candidate weight overflow")
		}
		totalWeight += candidate.Weight
	}
	sign := int64(1)
	absAmount := amount
	if amount < 0 {
		sign = -1
		absAmount = -amount
	}
	type share struct {
		SeedanceCostAllocationResult
		remainder *big.Int
	}
	shares := make([]share, 0, len(candidates))
	allocated := int64(0)
	denominator := big.NewInt(totalWeight)
	for _, candidate := range candidates {
		numerator := new(big.Int).Mul(big.NewInt(absAmount), big.NewInt(candidate.Weight))
		quotient, remainder := new(big.Int), new(big.Int)
		quotient.QuoRem(numerator, denominator, remainder)
		base := quotient.Int64()
		allocated += base
		shares = append(shares, share{SeedanceCostAllocationResult: SeedanceCostAllocationResult{
			PlatformOrderID: candidate.PlatformOrderID, Weight: candidate.Weight, AllocatedMicroRMB: base,
		}, remainder: remainder})
	}
	sort.Slice(shares, func(i, j int) bool {
		comparison := shares[i].remainder.Cmp(shares[j].remainder)
		if comparison == 0 {
			return shares[i].PlatformOrderID < shares[j].PlatformOrderID
		}
		return comparison > 0
	})
	left := absAmount - allocated
	for i := int64(0); i < left; i++ {
		shares[i%int64(len(shares))].AllocatedMicroRMB++
	}
	results := make([]SeedanceCostAllocationResult, len(shares))
	for i := range shares {
		shares[i].RemainderRank = i + 1
		shares[i].AllocatedMicroRMB *= sign
		results[i] = shares[i].SeedanceCostAllocationResult
	}
	sort.Slice(results, func(i, j int) bool { return results[i].PlatformOrderID < results[j].PlatformOrderID })
	return results, nil
}

type SeedanceBillImport struct {
	ChannelID           int
	BillDetailID        string
	Revision            int
	BillingPeriod       string
	ProductCode         string
	CostCategory        string
	InstanceID          string
	UsageStartedAt      int64
	UsageEndedAt        int64
	AmountMicroRMB      int64
	SanitizedSourceJSON string
	Candidates          []SeedanceCostCandidate
}

func ImportSeedanceVolcengineBillAuto(input SeedanceBillImport, actorUserID int) (*SeedanceVolcengineBillItem, bool, error) {
	input.Revision = 1
	hash, err := seedanceBillImportHash(input)
	if err != nil {
		return nil, false, err
	}
	var newest SeedanceVolcengineBillItem
	err = DB.Where("channel_id = ? AND bill_detail_id = ?", input.ChannelID, strings.TrimSpace(input.BillDetailID)).
		Order("revision desc").First(&newest).Error
	if err == nil {
		if newest.SourcePayloadHash == hash && newest.AmountMicroRMB == input.AmountMicroRMB {
			return &newest, true, nil
		}
		input.Revision = newest.Revision + 1
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, err
	}
	return ImportSeedanceVolcengineBill(input, actorUserID)
}

func seedanceBillImportHash(input SeedanceBillImport) (string, error) {
	costCategory, err := NormalizeSeedanceBillCostCategory(input.CostCategory)
	if err != nil {
		return "", err
	}
	source := strings.TrimSpace(input.SanitizedSourceJSON)
	if source == "" {
		source = "{}"
	}
	candidates := append([]SeedanceCostCandidate(nil), input.Candidates...)
	for i := range candidates {
		candidates[i].PlatformOrderID = strings.TrimSpace(candidates[i].PlatformOrderID)
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].PlatformOrderID < candidates[j].PlatformOrderID
	})
	hashMaterial, err := common.Marshal(map[string]any{
		"amount_micro_rmb": input.AmountMicroRMB,
		"cost_category":    costCategory,
		"candidates":       candidates,
		"source":           source,
	})
	if err != nil {
		return "", err
	}
	return SHA256Evidence(string(hashMaterial)), nil
}

func ImportSeedanceVolcengineBill(input SeedanceBillImport, actorUserID int) (*SeedanceVolcengineBillItem, bool, error) {
	input.BillDetailID = strings.TrimSpace(input.BillDetailID)
	input.BillingPeriod = strings.TrimSpace(input.BillingPeriod)
	input.ProductCode = strings.TrimSpace(input.ProductCode)
	costCategory, err := NormalizeSeedanceBillCostCategory(input.CostCategory)
	if err != nil {
		return nil, false, err
	}
	input.CostCategory = costCategory
	if input.ChannelID <= 0 || input.BillDetailID == "" || input.Revision <= 0 || input.BillingPeriod == "" || input.ProductCode == "" {
		return nil, false, errors.New("bill channel, detail id, revision, period and product are required")
	}
	source := strings.TrimSpace(input.SanitizedSourceJSON)
	if source == "" {
		source = "{}"
	}
	canonicalCandidates := append([]SeedanceCostCandidate(nil), input.Candidates...)
	for i := range canonicalCandidates {
		canonicalCandidates[i].PlatformOrderID = strings.TrimSpace(canonicalCandidates[i].PlatformOrderID)
	}
	sort.Slice(canonicalCandidates, func(i, j int) bool {
		return canonicalCandidates[i].PlatformOrderID < canonicalCandidates[j].PlatformOrderID
	})
	hash, err := seedanceBillImportHash(SeedanceBillImport{
		AmountMicroRMB: input.AmountMicroRMB, CostCategory: input.CostCategory,
		SanitizedSourceJSON: source, Candidates: canonicalCandidates,
	})
	if err != nil {
		return nil, false, err
	}
	item := &SeedanceVolcengineBillItem{
		ChannelID: input.ChannelID, BillDetailID: input.BillDetailID, Revision: input.Revision,
		BillingPeriod: input.BillingPeriod, ProductCode: input.ProductCode, CostCategory: input.CostCategory,
		InstanceID:     strings.TrimSpace(input.InstanceID),
		UsageStartedAt: input.UsageStartedAt, UsageEndedAt: input.UsageEndedAt, AmountMicroRMB: input.AmountMicroRMB,
		SourcePayloadHash: hash, SanitizedSourceJSON: source, CreatedAt: time.Now().Unix(),
	}
	var allocations []SeedanceCostAllocationResult
	var allocationErr error
	if input.CostCategory == SeedanceBillCostSuperResolution {
		// MediaKit is already represented by the frozen super-resolution cost
		// on the order. Its official bill is variance evidence only and must not
		// be added to the Ark generation cost a second time.
		item.AllocationStatus = SeedanceBillVarianceOnly
	} else {
		allocations, allocationErr = AllocateSeedanceCost(input.AmountMicroRMB, canonicalCandidates)
		if allocationErr != nil {
			item.AllocationStatus = SeedanceBillReconciliationRequired
		} else {
			item.AllocationStatus = SeedanceBillAllocated
		}
	}
	duplicate := false
	err = DB.Transaction(func(tx *gorm.DB) error {
		var existing SeedanceVolcengineBillItem
		err := tx.Where("channel_id = ? AND bill_detail_id = ? AND revision = ?", item.ChannelID, item.BillDetailID, item.Revision).First(&existing).Error
		if err == nil {
			if existing.SourcePayloadHash != item.SourcePayloadHash || existing.AmountMicroRMB != item.AmountMicroRMB {
				return errors.New("bill revision payload conflicts with the stored item")
			}
			*item = existing
			duplicate = true
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		var newest SeedanceVolcengineBillItem
		newestErr := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("channel_id = ? AND bill_detail_id = ?", item.ChannelID, item.BillDetailID).
			Order("revision desc").First(&newest).Error
		if newestErr == nil && newest.Revision > item.Revision {
			duplicate = true
			*item = newest
			return nil
		}
		if newestErr != nil && !errors.Is(newestErr, gorm.ErrRecordNotFound) {
			return newestErr
		}
		if err := tx.Create(item).Error; err != nil {
			return err
		}
		if allocationErr != nil {
			reasonCode := "INVALID_ALLOCATION_CANDIDATES"
			if len(canonicalCandidates) == 0 {
				reasonCode = "NO_VERIFIED_ALLOCATION_CANDIDATES"
			}
			return tx.Create(&SeedanceCostReconciliationIssue{
				BillItemID: item.ID, ChannelID: item.ChannelID, ReasonCode: reasonCode,
				EvidenceJSON: source, Status: SeedanceReconciliationOpen, CreatedAt: time.Now().Unix(),
			}).Error
		}
		if err := applySeedanceCostAllocations(tx, item, allocations); err != nil {
			return err
		}
		return tx.Create(&SeedanceAdminAudit{
			ActorUserID: actorUserID, ResourceType: "VOLCENGINE_BILL_ITEM", ResourceID: item.BillDetailID,
			Action: "IMPORT", ChangeSummary: "imported immutable bill revision and deterministic allocations", CreatedAt: time.Now().Unix(),
		}).Error
	})
	return item, duplicate, err
}

func ReconcileSeedanceVolcengineBillItem(billItemID int64, candidates []SeedanceCostCandidate, actorUserID int) (*SeedanceVolcengineBillItem, error) {
	if billItemID <= 0 {
		return nil, errors.New("bill item id is required")
	}
	allocations, err := AllocateSeedanceCost(0, candidates)
	if err != nil {
		return nil, err
	}
	var item SeedanceVolcengineBillItem
	err = DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", billItemID).First(&item).Error; err != nil {
			return err
		}
		if item.CostCategory == SeedanceBillCostSuperResolution || item.AllocationStatus == SeedanceBillVarianceOnly {
			return errors.New("super-resolution bill items are variance evidence and cannot be allocated to Ark generation cost")
		}
		allocations, err = AllocateSeedanceCost(item.AmountMicroRMB, candidates)
		if err != nil {
			return err
		}
		var count int64
		if err := tx.Model(&SeedanceCostAllocation{}).Where("bill_item_id = ?", item.ID).Count(&count).Error; err != nil {
			return err
		}
		if count != 0 || item.AllocationStatus == SeedanceBillAllocated {
			return errors.New("bill item has already been allocated")
		}
		if err := applySeedanceCostAllocations(tx, &item, allocations); err != nil {
			return err
		}
		now := time.Now().Unix()
		if err := tx.Model(&SeedanceVolcengineBillItem{}).Where("id = ?", item.ID).
			Update("allocation_status", SeedanceBillAllocated).Error; err != nil {
			return err
		}
		if err := tx.Model(&SeedanceCostReconciliationIssue{}).
			Where("bill_item_id = ? AND status = ?", item.ID, SeedanceReconciliationOpen).
			Updates(map[string]any{"status": SeedanceReconciliationResolved, "resolved_at": now}).Error; err != nil {
			return err
		}
		if err := tx.Create(&SeedanceAdminAudit{
			ActorUserID: actorUserID, ResourceType: "VOLCENGINE_BILL_ITEM", ResourceID: item.BillDetailID,
			Action: "RECONCILE", ChangeSummary: "applied verified deterministic bill allocation", CreatedAt: now,
		}).Error; err != nil {
			return err
		}
		item.AllocationStatus = SeedanceBillAllocated
		return nil
	})
	return &item, err
}

func applySeedanceCostAllocations(tx *gorm.DB, item *SeedanceVolcengineBillItem, allocations []SeedanceCostAllocationResult) error {
	previous := map[string]int64{}
	var previousItem SeedanceVolcengineBillItem
	previousErr := tx.Where("channel_id = ? AND bill_detail_id = ? AND revision < ?", item.ChannelID, item.BillDetailID, item.Revision).
		Order("revision desc").First(&previousItem).Error
	if previousErr == nil {
		var old []SeedanceCostAllocation
		if err := tx.Where("bill_item_id = ?", previousItem.ID).Find(&old).Error; err != nil {
			return err
		}
		for _, allocation := range old {
			previous[allocation.PlatformOrderID] = allocation.AllocatedMicroRMB
		}
	} else if !errors.Is(previousErr, gorm.ErrRecordNotFound) {
		return previousErr
	}
	current := make(map[string]int64, len(allocations))
	for _, allocation := range allocations {
		var order SeedanceOrder
		if err := tx.Where("platform_order_id = ? AND channel_id = ?", allocation.PlatformOrderID, item.ChannelID).First(&order).Error; err != nil {
			return err
		}
		if !isSeedanceOrderTerminal(order.OrderStatus) {
			return errors.New("cost allocation candidates must reference terminal Seedance orders")
		}
		current[allocation.PlatformOrderID] = allocation.AllocatedMicroRMB
		if err := tx.Create(&SeedanceCostAllocation{
			BillItemID: item.ID, PlatformOrderID: allocation.PlatformOrderID, BillRevision: item.Revision,
			Weight: allocation.Weight, AllocatedMicroRMB: allocation.AllocatedMicroRMB,
			RemainderRank: allocation.RemainderRank, RuleVersion: "largest_remainder_order_id_v1", CreatedAt: time.Now().Unix(),
		}).Error; err != nil {
			return err
		}
	}
	allOrders := make(map[string]struct{}, len(previous)+len(current))
	for orderID := range previous {
		allOrders[orderID] = struct{}{}
	}
	for orderID := range current {
		allOrders[orderID] = struct{}{}
	}
	for orderID := range allOrders {
		delta, err := checkedSeedanceMoneySubtract(current[orderID], previous[orderID])
		if err != nil {
			return err
		}
		if delta == 0 {
			continue
		}
		var order SeedanceOrder
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("platform_order_id = ? AND channel_id = ?", orderID, item.ChannelID).First(&order).Error; err != nil {
			return err
		}
		if !isSeedanceOrderTerminal(order.OrderStatus) {
			return errors.New("cost allocation candidates must reference terminal Seedance orders")
		}
		currentActual := int64(0)
		if order.VolcengineActualMicroRMB != nil {
			currentActual = *order.VolcengineActualMicroRMB
		}
		updatedActual, err := checkedSeedanceMoneyAdd(currentActual, delta)
		if err != nil {
			return err
		}
		if updatedActual < 0 {
			return errors.New("bill revision would make an order's confirmed cost negative")
		}
		actualProfit, err := CalculateSeedanceProfit(order.ModelSaleMicroRMB, order.ServiceChargeTotalMicroRMB, updatedActual)
		if err != nil {
			return err
		}
		if err := tx.Model(&SeedanceOrder{}).Where("id = ?", order.ID).Updates(map[string]any{
			"volcengine_actual_micro_rmb":     updatedActual,
			"new_api_actual_profit_micro_rmb": actualProfit,
			"volcengine_cost_status":          SeedanceCostConfirmed,
			"finance_revision":                gorm.Expr("finance_revision + 1"),
			"updated_at":                      time.Now().Unix(),
		}).Error; err != nil {
			return err
		}
	}
	return nil
}

func ListSeedanceCostReconciliationIssues(status string, offset, limit int) ([]SeedanceCostReconciliationIssue, int64, error) {
	query := DB.Model(&SeedanceCostReconciliationIssue{})
	if strings.TrimSpace(status) != "" {
		query = query.Where("status = ?", strings.TrimSpace(status))
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var items []SeedanceCostReconciliationIssue
	err := query.Order("id desc").Offset(offset).Limit(limit).Find(&items).Error
	return items, total, err
}

func ListSeedanceVolcengineBillCursors(channelID int) ([]SeedanceVolcengineBillCursor, error) {
	var items []SeedanceVolcengineBillCursor
	err := DB.Where("channel_id = ?", channelID).Order("billing_period desc").Find(&items).Error
	return items, err
}
