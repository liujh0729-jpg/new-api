package model

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAllocateSeedanceCostConservesEveryMicroRMB(t *testing.T) {
	allocations, err := AllocateSeedanceCost(10, []SeedanceCostCandidate{
		{PlatformOrderID: "order-c", Weight: 1},
		{PlatformOrderID: "order-a", Weight: 1},
		{PlatformOrderID: "order-b", Weight: 1},
	})
	require.NoError(t, err)
	require.Equal(t, []SeedanceCostAllocationResult{
		{PlatformOrderID: "order-a", Weight: 1, AllocatedMicroRMB: 4, RemainderRank: 1},
		{PlatformOrderID: "order-b", Weight: 1, AllocatedMicroRMB: 3, RemainderRank: 2},
		{PlatformOrderID: "order-c", Weight: 1, AllocatedMicroRMB: 3, RemainderRank: 3},
	}, allocations)
	require.Equal(t, int64(10), allocatedTotal(allocations))
}

func TestAllocateSeedanceCostSupportsNegativeRevision(t *testing.T) {
	allocations, err := AllocateSeedanceCost(-11, []SeedanceCostCandidate{
		{PlatformOrderID: "order-a", Weight: 2},
		{PlatformOrderID: "order-b", Weight: 1},
	})
	require.NoError(t, err)
	require.Equal(t, int64(-11), allocatedTotal(allocations))
	require.Equal(t, int64(-7), allocations[0].AllocatedMicroRMB)
	require.Equal(t, int64(-4), allocations[1].AllocatedMicroRMB)
}

func TestAllocateSeedanceCostRejectsAmbiguousCandidates(t *testing.T) {
	_, err := AllocateSeedanceCost(1, []SeedanceCostCandidate{
		{PlatformOrderID: "order-a", Weight: 1},
		{PlatformOrderID: "order-a", Weight: 1},
	})
	require.ErrorContains(t, err, "duplicate")
	_, err = AllocateSeedanceCost(1, nil)
	require.Error(t, err)
}

func TestImportSeedanceSuperResolutionBillIsVarianceOnly(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(
		&SeedanceVolcengineBillItem{}, &SeedanceCostAllocation{},
		&SeedanceCostReconciliationIssue{}, &SeedanceAdminAudit{},
	))
	require.NoError(t, DB.Exec("DELETE FROM seedance_cost_allocations").Error)
	require.NoError(t, DB.Exec("DELETE FROM seedance_cost_reconciliation_issues").Error)
	require.NoError(t, DB.Exec("DELETE FROM seedance_volcengine_bill_items").Error)
	t.Cleanup(func() {
		_ = DB.Exec("DELETE FROM seedance_cost_allocations").Error
		_ = DB.Exec("DELETE FROM seedance_cost_reconciliation_issues").Error
		_ = DB.Exec("DELETE FROM seedance_volcengine_bill_items").Error
	})

	item, duplicate, err := ImportSeedanceVolcengineBill(SeedanceBillImport{
		ChannelID: 88, BillDetailID: "mediakit-bill-1", Revision: 1,
		BillingPeriod: "2026-09", ProductCode: "AI_MEDIAKIT",
		CostCategory: SeedanceBillCostSuperResolution, AmountMicroRMB: 900_000,
		Candidates: []SeedanceCostCandidate{{PlatformOrderID: "must-not-be-allocated", Weight: 1}},
	}, 100)
	require.NoError(t, err)
	require.False(t, duplicate)
	require.Equal(t, SeedanceBillVarianceOnly, item.AllocationStatus)

	var allocations int64
	require.NoError(t, DB.Model(&SeedanceCostAllocation{}).Where("bill_item_id = ?", item.ID).Count(&allocations).Error)
	require.Zero(t, allocations)
	var issues int64
	require.NoError(t, DB.Model(&SeedanceCostReconciliationIssue{}).Where("bill_item_id = ?", item.ID).Count(&issues).Error)
	require.Zero(t, issues)
}

func TestImportSeedanceBillAppliesOnlyRevisionDeltaAndHashesCandidates(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(
		&SeedanceOrder{}, &SeedanceVolcengineBillItem{}, &SeedanceCostAllocation{},
		&SeedanceCostReconciliationIssue{}, &SeedanceAdminAudit{},
	))
	for _, table := range []string{
		"seedance_cost_allocations", "seedance_cost_reconciliation_issues",
		"seedance_volcengine_bill_items", "seedance_admin_audits", "seedance_orders",
	} {
		require.NoError(t, DB.Exec("DELETE FROM "+table).Error)
	}
	t.Cleanup(func() {
		for _, table := range []string{
			"seedance_cost_allocations", "seedance_cost_reconciliation_issues",
			"seedance_volcengine_bill_items", "seedance_admin_audits", "seedance_orders",
		} {
			_ = DB.Exec("DELETE FROM " + table).Error
		}
	})
	now := time.Now().Unix()
	for _, orderID := range []string{"order-a", "order-b"} {
		require.NoError(t, DB.Create(&SeedanceOrder{
			PlatformOrderID: orderID, NewAPITaskID: "task-" + orderID, NewAPIUserID: 1,
			ChannelID: 77, InstanceID: "instance", VolcengineCredentialID: 1, CredentialVersion: 1,
			Model: "Public Video", OrderStatus: SeedanceOrderSucceeded, VolcengineCostStatus: SeedanceCostEstimated,
			SyncStatus: SeedanceSyncSynced, ModelSaleMicroRMB: 100, ServiceChargeTotalMicroRMB: 10,
			VolcengineEstimatedMicroRMB: 20, NewAPIEstimatedProfitMicroRMB: 70,
			PricingSnapshotJSON: `{}`, PricingSnapshotHash: "sha256:test", PublicProtocol: SeedanceProtocolOfficial,
			CallbackStatus: SeedanceCallbackNone, CreatedAt: now, UpdatedAt: now,
		}).Error)
	}
	candidates := []SeedanceCostCandidate{{PlatformOrderID: "order-b", Weight: 1}, {PlatformOrderID: "order-a", Weight: 1}}
	first, duplicate, err := ImportSeedanceVolcengineBill(SeedanceBillImport{
		ChannelID: 77, BillDetailID: "bill-1", Revision: 1, BillingPeriod: "2026-09",
		ProductCode: "video", AmountMicroRMB: 10, SanitizedSourceJSON: `{"row":"safe"}`, Candidates: candidates,
	}, 1)
	require.NoError(t, err)
	require.False(t, duplicate)
	require.Equal(t, SeedanceBillAllocated, first.AllocationStatus)

	_, _, err = ImportSeedanceVolcengineBill(SeedanceBillImport{
		ChannelID: 77, BillDetailID: "bill-1", Revision: 1, BillingPeriod: "2026-09",
		ProductCode: "video", AmountMicroRMB: 10, SanitizedSourceJSON: `{"row":"safe"}`,
		Candidates: []SeedanceCostCandidate{{PlatformOrderID: "order-a", Weight: 2}, {PlatformOrderID: "order-b", Weight: 1}},
	}, 1)
	require.ErrorContains(t, err, "conflicts")

	_, duplicate, err = ImportSeedanceVolcengineBill(SeedanceBillImport{
		ChannelID: 77, BillDetailID: "bill-1", Revision: 2, BillingPeriod: "2026-09",
		ProductCode: "video", AmountMicroRMB: 14, SanitizedSourceJSON: `{"row":"revised"}`, Candidates: candidates,
	}, 1)
	require.NoError(t, err)
	require.False(t, duplicate)
	for _, orderID := range []string{"order-a", "order-b"} {
		var order SeedanceOrder
		require.NoError(t, DB.Where("platform_order_id = ?", orderID).First(&order).Error)
		require.NotNil(t, order.VolcengineActualMicroRMB)
		require.Equal(t, int64(7), *order.VolcengineActualMicroRMB)
		require.Equal(t, int64(83), *order.NewAPIActualProfitMicroRMB)
		require.Equal(t, 2, order.FinanceRevision)
	}

	raw, duplicate, err := ImportSeedanceVolcengineBillAuto(SeedanceBillImport{
		ChannelID: 77, BillDetailID: "bill-needs-review", BillingPeriod: "2026-09",
		ProductCode: "video", AmountMicroRMB: 6, SanitizedSourceJSON: `{"row":"unmapped"}`,
	}, 0)
	require.NoError(t, err)
	require.False(t, duplicate)
	require.Equal(t, 1, raw.Revision)
	require.Equal(t, SeedanceBillReconciliationRequired, raw.AllocationStatus)

	same, duplicate, err := ImportSeedanceVolcengineBillAuto(SeedanceBillImport{
		ChannelID: 77, BillDetailID: "bill-needs-review", BillingPeriod: "2026-09",
		ProductCode: "video", AmountMicroRMB: 6, SanitizedSourceJSON: `{"row":"unmapped"}`,
	}, 0)
	require.NoError(t, err)
	require.True(t, duplicate)
	require.Equal(t, raw.ID, same.ID)

	reconciled, err := ReconcileSeedanceVolcengineBillItem(raw.ID, candidates, 1)
	require.NoError(t, err)
	require.Equal(t, SeedanceBillAllocated, reconciled.AllocationStatus)
	var resolvedIssue SeedanceCostReconciliationIssue
	require.NoError(t, DB.Where("bill_item_id = ?", raw.ID).First(&resolvedIssue).Error)
	require.Equal(t, SeedanceReconciliationResolved, resolvedIssue.Status)
	for _, orderID := range []string{"order-a", "order-b"} {
		var order SeedanceOrder
		require.NoError(t, DB.Where("platform_order_id = ?", orderID).First(&order).Error)
		require.Equal(t, int64(10), *order.VolcengineActualMicroRMB)
		require.Equal(t, 3, order.FinanceRevision)
	}

	revised, duplicate, err := ImportSeedanceVolcengineBillAuto(SeedanceBillImport{
		ChannelID: 77, BillDetailID: "bill-needs-review", BillingPeriod: "2026-09",
		ProductCode: "video", AmountMicroRMB: 8, SanitizedSourceJSON: `{"row":"revised-unmapped"}`,
	}, 0)
	require.NoError(t, err)
	require.False(t, duplicate)
	require.Equal(t, 2, revised.Revision)
	_, err = ReconcileSeedanceVolcengineBillItem(revised.ID, candidates, 1)
	require.NoError(t, err)
	for _, orderID := range []string{"order-a", "order-b"} {
		var order SeedanceOrder
		require.NoError(t, DB.Where("platform_order_id = ?", orderID).First(&order).Error)
		require.Equal(t, int64(11), *order.VolcengineActualMicroRMB)
		require.Equal(t, 4, order.FinanceRevision)
	}
}

func TestBackfillSeedanceOrderFinanceRevisionUsesHighestExistingLineRevision(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&SeedanceOrder{}, &MediaServiceUsage{}))
	require.NoError(t, DB.Exec("DELETE FROM media_service_usages").Error)
	require.NoError(t, DB.Exec("DELETE FROM seedance_orders").Error)
	t.Cleanup(func() {
		_ = DB.Exec("DELETE FROM media_service_usages").Error
		_ = DB.Exec("DELETE FROM seedance_orders").Error
	})
	now := time.Now().Unix()
	terminal := &SeedanceOrder{
		PlatformOrderID: "order-finance-revision-terminal", NewAPITaskID: "task-finance-revision-terminal",
		NewAPIUserID: 1, ChannelID: 91, InstanceID: "instance-91", VolcengineCredentialID: 1,
		CredentialVersion: 1, Model: "Public Video", OrderStatus: SeedanceOrderSucceeded,
		VolcengineCostStatus: SeedanceCostEstimated, SyncStatus: SeedanceSyncSynced,
		PricingSnapshotJSON: `{}`, PricingSnapshotHash: SHA256Evidence(`{}`),
		PublicProtocol: SeedanceProtocolOfficial, CallbackStatus: SeedanceCallbackNone,
		CreatedAt: now, UpdatedAt: now,
	}
	require.NoError(t, DB.Create(terminal).Error)
	nonTerminal := *terminal
	nonTerminal.ID = 0
	nonTerminal.PlatformOrderID = "order-finance-revision-running"
	nonTerminal.NewAPITaskID = "task-finance-revision-running"
	nonTerminal.OrderStatus = SeedanceOrderEnhancing
	require.NoError(t, DB.Create(&nonTerminal).Error)
	require.NoError(t, DB.Create(&MediaServiceUsage{
		ServiceLineItemID: terminal.PlatformOrderID + ":video-processing",
		PlatformOrderID:   terminal.PlatformOrderID, ServiceType: SeedanceServiceTypeVideoSuperResolution,
		ProviderType: SeedanceProviderDirect, ServiceCode: "video_sr_v1", AttemptID: "attempt-finance-revision",
		Status: SeedanceUsageSucceeded, PriceVersion: "price-v1", Revision: 4,
		StartedAt: now - 1, CompletedAt: now, CreatedAt: now, UpdatedAt: now,
	}).Error)

	require.NoError(t, BackfillSeedanceOrderFinanceRevision())
	require.NoError(t, DB.First(terminal, terminal.ID).Error)
	require.NoError(t, DB.First(&nonTerminal, nonTerminal.ID).Error)
	require.Equal(t, 4, terminal.FinanceRevision)
	require.Zero(t, nonTerminal.FinanceRevision)
}

func TestImportSeedanceBillRejectsNonTerminalOrderCandidate(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(
		&SeedanceOrder{}, &SeedanceVolcengineBillItem{}, &SeedanceCostAllocation{},
		&SeedanceCostReconciliationIssue{}, &SeedanceAdminAudit{},
	))
	for _, table := range []string{
		"seedance_cost_allocations", "seedance_cost_reconciliation_issues",
		"seedance_volcengine_bill_items", "seedance_admin_audits", "seedance_orders",
	} {
		require.NoError(t, DB.Exec("DELETE FROM "+table).Error)
	}
	t.Cleanup(func() {
		for _, table := range []string{
			"seedance_cost_allocations", "seedance_cost_reconciliation_issues",
			"seedance_volcengine_bill_items", "seedance_admin_audits", "seedance_orders",
		} {
			_ = DB.Exec("DELETE FROM " + table).Error
		}
	})
	now := time.Now().Unix()
	order := &SeedanceOrder{
		PlatformOrderID: "order-running-cost-candidate", NewAPITaskID: "task-running-cost-candidate",
		NewAPIUserID: 1, ChannelID: 92, InstanceID: "instance-92", VolcengineCredentialID: 1,
		CredentialVersion: 1, Model: "Public Video", OrderStatus: SeedanceOrderEnhancing,
		VolcengineCostStatus: SeedanceCostEstimated, SyncStatus: SeedanceSyncPending,
		PricingSnapshotJSON: `{}`, PricingSnapshotHash: SHA256Evidence(`{}`),
		PublicProtocol: SeedanceProtocolOfficial, CallbackStatus: SeedanceCallbackNone,
		CreatedAt: now, UpdatedAt: now,
	}
	require.NoError(t, DB.Create(order).Error)

	_, _, err := ImportSeedanceVolcengineBill(SeedanceBillImport{
		ChannelID: 92, BillDetailID: "bill-running-candidate", Revision: 1, BillingPeriod: "2026-09",
		ProductCode: "video", AmountMicroRMB: 10, SanitizedSourceJSON: `{}`,
		Candidates: []SeedanceCostCandidate{{PlatformOrderID: order.PlatformOrderID, Weight: 1}},
	}, 1)
	require.ErrorContains(t, err, "terminal Seedance orders")
	require.NoError(t, DB.First(order, order.ID).Error)
	require.Zero(t, order.FinanceRevision)
	var bills int64
	require.NoError(t, DB.Model(&SeedanceVolcengineBillItem{}).Count(&bills).Error)
	require.Zero(t, bills)
}

func allocatedTotal(items []SeedanceCostAllocationResult) int64 {
	var total int64
	for _, item := range items {
		total += item.AllocatedMicroRMB
	}
	return total
}
