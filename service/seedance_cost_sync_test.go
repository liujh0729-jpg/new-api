package service

import (
	"context"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
)

type fakeSeedanceVolcengineBillClient struct {
	amount            string
	configurationCode string
	calls             int
}

func (c *fakeSeedanceVolcengineBillClient) ListBillDetails(_ context.Context, period string, products []string, offset, limit int32) (*seedanceVolcengineBillPage, error) {
	c.calls++
	records := []seedanceVolcengineBillDetail{{
		BillDetailID: "bill-detail-1", BillPeriod: period, ExpenseDate: period + "-01",
		ExpenseBeginTime: period + "-01 00:00:00", ExpenseEndTime: period + "-01 00:00:05",
		Product: products[0], ConfigurationCode: c.configurationCode, InstanceNo: "ark-instance-1", PayableAmount: c.amount, Currency: "CNY",
	}, {
		BillDetailID: "bill-detail-other-model", BillPeriod: period, ExpenseDate: period + "-01",
		Product: products[0], ConfigurationCode: "Doubao_Seedance_2.0", InstanceNo: "other-instance",
		PayableAmount: "99.000000", Currency: "CNY",
	}, {
		BillDetailID: "bill-detail-other-product", BillPeriod: period, ExpenseDate: period + "-01",
		Product: "other-product", ConfigurationCode: c.configurationCode, InstanceNo: "other-product-instance",
		PayableAmount: "99.000000", Currency: "CNY",
	}}
	return &seedanceVolcengineBillPage{Items: records, Total: int32(len(records)), Offset: offset, Limit: limit}, nil
}

func TestSeedanceVolcengineBillWorkerPersistsCursorAndRevisionsWithoutGuessingAllocation(t *testing.T) {
	truncate(t)
	require.NoError(t, model.DB.AutoMigrate(
		&model.SeedanceChannelConfig{}, &model.SeedanceVolcengineCredential{},
		&model.SeedanceVolcengineBillCursor{}, &model.SeedanceVolcengineBillItem{},
		&model.SeedanceCostAllocation{}, &model.SeedanceCostReconciliationIssue{}, &model.SeedanceAdminAudit{},
	))
	for _, table := range []string{
		"seedance_volcengine_bill_cursors", "seedance_cost_allocations", "seedance_cost_reconciliation_issues",
		"seedance_volcengine_bill_items", "seedance_admin_audits", "seedance_volcengine_credentials", "seedance_channel_configs",
	} {
		require.NoError(t, model.DB.Exec("DELETE FROM "+table).Error)
	}
	t.Cleanup(func() {
		for _, table := range []string{
			"seedance_volcengine_bill_cursors", "seedance_cost_allocations", "seedance_cost_reconciliation_issues",
			"seedance_volcengine_bill_items", "seedance_admin_audits", "seedance_volcengine_credentials", "seedance_channel_configs",
		} {
			_ = model.DB.Exec("DELETE FROM " + table).Error
		}
	})

	config := &model.SeedanceChannelConfig{
		ChannelID: 781, InstanceID: "30000000-0000-0000-0000-000000000001", Status: model.SeedanceConfigActive,
		VolcengineBillSyncEnabled: true, VolcengineBillProductCodesJSON: `["ark"]`,
		VolcengineBillConfigurationCodesJSON: `["Doubao_Seedance_2.5"]`, CreatedAt: time.Now().Unix(), UpdatedAt: time.Now().Unix(),
	}
	require.NoError(t, model.DB.Create(config).Error)
	require.NoError(t, model.DB.Create(&model.SeedanceVolcengineCredential{
		ChannelID: 781, Version: 1, ArkAPIKeyEncrypted: "not-used", AccessKeyIDEncrypted: "not-used",
		SecretAccessKeyEncrypted: "not-used", Fingerprint: "sha256:test", MaskedSuffix: "test",
		Status: model.SeedanceCredentialActive, ValidatedAt: time.Now().Unix(), BillingValidatedAt: time.Now().Unix(), CreatedAt: time.Now().Unix(),
	}).Error)

	fake := &fakeSeedanceVolcengineBillClient{amount: "1.234567", configurationCode: "Doubao_Seedance_2.5"}
	factoryCalls := 0
	previousFactory := seedanceVolcengineBillClientFactory
	seedanceVolcengineBillClientFactory = func(*model.SeedanceVolcengineCredential) (seedanceVolcengineBillClient, error) {
		factoryCalls++
		return fake, nil
	}
	t.Cleanup(func() { seedanceVolcengineBillClientFactory = previousFactory })

	require.NoError(t, syncSeedanceVolcengineChannelBills(context.Background(), config, []string{"2026-09"}))
	var first model.SeedanceVolcengineBillItem
	require.NoError(t, model.DB.Where("channel_id = ? AND bill_detail_id = ?", 781, "bill-detail-1").First(&first).Error)
	require.Equal(t, int64(1_234_567), first.AmountMicroRMB)
	require.Equal(t, 1, first.Revision)
	require.Equal(t, model.SeedanceBillReconciliationRequired, first.AllocationStatus)
	var unrelatedCount int64
	require.NoError(t, model.DB.Model(&model.SeedanceVolcengineBillItem{}).
		Where("bill_detail_id IN ?", []string{"bill-detail-other-model", "bill-detail-other-product"}).Count(&unrelatedCount).Error)
	require.Zero(t, unrelatedCount)
	var cursor model.SeedanceVolcengineBillCursor
	require.NoError(t, model.DB.Where("channel_id = ? AND billing_period = ?", 781, "2026-09").First(&cursor).Error)
	require.Equal(t, model.SeedanceBillCursorIdle, cursor.Status)
	require.Equal(t, "0", cursor.Cursor)
	require.Positive(t, cursor.LastSyncAt)
	require.Equal(t, 1, factoryCalls)

	// The frequent task-polling tick must not decrypt credentials or construct
	// an SDK client while the hourly cursor is not due.
	require.NoError(t, syncSeedanceVolcengineChannelBills(context.Background(), config, []string{"2026-09"}))
	require.Equal(t, 1, factoryCalls)
	require.Equal(t, 1, fake.calls)

	require.NoError(t, model.DB.Model(&model.SeedanceVolcengineBillCursor{}).Where("id = ?", cursor.ID).Update("next_attempt_at", 0).Error)
	fake.amount = "1.334567"
	require.NoError(t, syncSeedanceVolcengineChannelBills(context.Background(), config, []string{"2026-09"}))
	var items []model.SeedanceVolcengineBillItem
	require.NoError(t, model.DB.Where("channel_id = ? AND bill_detail_id = ?", 781, "bill-detail-1").Order("revision asc").Find(&items).Error)
	require.Len(t, items, 2)
	require.Equal(t, 2, items[1].Revision)
	require.Equal(t, int64(1_334_567), items[1].AmountMicroRMB)
	require.Equal(t, 2, factoryCalls)
	require.Equal(t, 2, fake.calls)
}

func TestSeedanceBillMoneyAndCurrencyAreExact(t *testing.T) {
	value, err := exactMicroRMB("12.345600")
	require.NoError(t, err)
	require.Equal(t, int64(12_345_600), value)
	_, err = exactMicroRMB("0.0000001")
	require.Error(t, err)
	_, err = seedanceBillDetailImport(1, "2026-09", 0, seedanceVolcengineBillDetail{
		BillDetailID: "foreign", BillPeriod: "2026-09", Product: "ark", PayableAmount: "1.00", Currency: "USD",
	})
	require.ErrorContains(t, err, "unsupported currency")
}
