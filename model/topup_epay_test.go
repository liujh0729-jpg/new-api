package model

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func insertEpayTopUpForTest(t *testing.T, userID int, tradeNo string, paymentCents, quotaToAdd int64) {
	t.Helper()
	require.NoError(t, DB.Create(&User{
		Id:       userID,
		Username: "epay_user_" + tradeNo,
		Status:   common.UserStatusEnabled,
	}).Error)
	require.NoError(t, DB.Create(&TopUp{
		UserId:             userID,
		Amount:             1,
		AmountUnit:         TopUpAmountUnitCNY,
		Money:              float64(paymentCents) / 100,
		PaymentAmountCents: paymentCents,
		QuotaToAdd:         quotaToAdd,
		Currency:           "CNY",
		TradeNo:            tradeNo,
		PaymentMethod:      "alipay",
		PaymentProvider:    PaymentProviderEpay,
		Status:             common.TopUpStatusPending,
		CreateTime:         time.Now().Unix(),
	}).Error)
}

func TestSettleEpayTopUpUsesSnapshotAndIsIdempotent(t *testing.T) {
	truncateTables(t)
	insertEpayTopUpForTest(t, 801, "EPAY801SNAPSHOT", 100, 68493)

	previousQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 999999
	t.Cleanup(func() { common.QuotaPerUnit = previousQuotaPerUnit })

	order, quotaToAdd, settled, err := SettleEpayTopUp("EPAY801SNAPSHOT", "wxpay", 100)
	require.NoError(t, err)
	assert.True(t, settled)
	assert.EqualValues(t, 68493, quotaToAdd)
	assert.Equal(t, common.TopUpStatusSuccess, order.Status)
	assert.Equal(t, "wxpay", order.PaymentMethod)
	assert.Equal(t, 68493, getUserQuotaForPaymentGuardTest(t, 801))

	_, _, settled, err = SettleEpayTopUp("EPAY801SNAPSHOT", "wxpay", 100)
	require.NoError(t, err)
	assert.False(t, settled)
	assert.Equal(t, 68493, getUserQuotaForPaymentGuardTest(t, 801))
}

func TestSettleEpayTopUpRejectsAmountMismatch(t *testing.T) {
	truncateTables(t)
	insertEpayTopUpForTest(t, 802, "EPAY802AMOUNT", 100, 68493)

	_, _, _, err := SettleEpayTopUp("EPAY802AMOUNT", "alipay", 99)
	require.ErrorIs(t, err, ErrPaymentAmountMismatch)
	assert.Zero(t, getUserQuotaForPaymentGuardTest(t, 802))
	assert.Equal(t, common.TopUpStatusPending, GetTopUpByTradeNo("EPAY802AMOUNT").Status)
}

func TestSettleEpayTopUpSupportsLegacyPendingOrder(t *testing.T) {
	truncateTables(t)
	insertEpayTopUpForTest(t, 803, "EPAY803LEGACY", 0, 0)
	require.NoError(t, DB.Model(&TopUp{}).
		Where("trade_no = ?", "EPAY803LEGACY").
		Updates(map[string]interface{}{
			"amount":      2,
			"amount_unit": "",
			"currency":    "",
		}).Error)

	previousQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 500000
	t.Cleanup(func() { common.QuotaPerUnit = previousQuotaPerUnit })

	_, quotaToAdd, settled, err := SettleEpayTopUp("EPAY803LEGACY", "alipay", 1460)
	require.NoError(t, err)
	assert.True(t, settled)
	assert.EqualValues(t, 1000000, quotaToAdd)
	assert.Equal(t, 1000000, getUserQuotaForPaymentGuardTest(t, 803))
}
