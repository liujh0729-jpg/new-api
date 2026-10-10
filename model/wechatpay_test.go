package model

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func insertWechatPayUserAndOrder(t *testing.T, userId int, tradeNo string, status string) {
	t.Helper()
	require.NoError(t, DB.Create(&User{
		Id:       userId,
		Username: "wechat_pay_user_" + tradeNo,
		Status:   common.UserStatusEnabled,
	}).Error)
	require.NoError(t, DB.Create(&TopUp{
		UserId:             userId,
		Amount:             2,
		Money:              2,
		PaymentAmountCents: 200,
		QuotaToAdd:         1000,
		Currency:           "CNY",
		TradeNo:            tradeNo,
		PaymentMethod:      PaymentMethodWechatNative,
		PaymentProvider:    PaymentProviderWechatPay,
		Status:             status,
		CreateTime:         time.Now().Unix(),
		ExpireTime:         time.Now().Add(time.Minute).Unix(),
	}).Error)
}

func wechatPayUserQuota(t *testing.T, userId int) int {
	t.Helper()
	user := &User{}
	require.NoError(t, DB.Select("quota").First(user, userId).Error)
	return user.Quota
}

func TestSettleWechatPayTopUpIsIdempotent(t *testing.T) {
	truncateTables(t)
	insertWechatPayUserAndOrder(t, 701, "WXN701IDEMPOTENT", common.TopUpStatusPending)
	previousQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 999999
	t.Cleanup(func() { common.QuotaPerUnit = previousQuotaPerUnit })

	first, err := SettleWechatPayTopUp("WXN701IDEMPOTENT", "wechat-transaction-701", 200, "CNY")
	require.NoError(t, err)
	assert.True(t, first.Settled)
	assert.Equal(t, 1000, first.QuotaToAdd)
	assert.Equal(t, 1000, wechatPayUserQuota(t, 701))

	duplicate, err := SettleWechatPayTopUp("WXN701IDEMPOTENT", "wechat-transaction-701", 200, "CNY")
	require.NoError(t, err)
	assert.False(t, duplicate.Settled)
	assert.Equal(t, 1000, wechatPayUserQuota(t, 701))

	_, err = SettleWechatPayTopUp("WXN701IDEMPOTENT", "different-transaction", 200, "CNY")
	require.Error(t, err)
	assert.Equal(t, 1000, wechatPayUserQuota(t, 701))
}

func TestSettleWechatPayTopUpRejectsAmountMismatch(t *testing.T) {
	truncateTables(t)
	insertWechatPayUserAndOrder(t, 702, "WXN702AMOUNT", common.TopUpStatusPending)

	_, err := SettleWechatPayTopUp("WXN702AMOUNT", "wechat-transaction-702", 1, "CNY")
	require.Error(t, err)
	assert.Zero(t, wechatPayUserQuota(t, 702))
	assert.Equal(t, common.TopUpStatusPending, GetTopUpByTradeNo("WXN702AMOUNT").Status)
}

func TestSettleWechatPayTopUpAcceptsVerifiedLatePayment(t *testing.T) {
	truncateTables(t)
	insertWechatPayUserAndOrder(t, 703, "WXN703EXPIRED", common.TopUpStatusExpired)

	settlement, err := SettleWechatPayTopUp("WXN703EXPIRED", "wechat-transaction-703", 200, "CNY")
	require.NoError(t, err)
	assert.True(t, settlement.Settled)
	assert.Equal(t, 1000, wechatPayUserQuota(t, 703))
	assert.Equal(t, common.TopUpStatusSuccess, GetTopUpByTradeNo("WXN703EXPIRED").Status)
}

func TestCompleteWechatPayTestOrderNeverCreditsUser(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.Create(&User{Id: 704, Username: "wechat_pay_test_user", Status: common.UserStatusEnabled, Quota: 77}).Error)
	require.NoError(t, SaveWechatPayConfig(&WechatPayConfig{
		AppId: "wx-test", MchId: "123456", MerchantCertificateSerial: "serial",
		MerchantCertificateFingerprint: "merchant-fingerprint", WechatPayPublicKeyId: "PUB_KEY_ID_TEST",
		WechatPayPublicKeyFingerprint: "wechat-fingerprint", MerchantCertificateEncrypted: "encrypted-cert",
		MerchantPrivateKeyEncrypted: "encrypted-key", ApiV3KeyEncrypted: "encrypted-api-v3",
		WechatPayPublicKeyEncrypted: "encrypted-public-key",
	}))
	require.NoError(t, InsertWechatPayTestOrder(&WechatPayTestOrder{
		TradeNo: "WXT704TEST", PaymentAmountCents: 1, Currency: "CNY",
		Status: common.TopUpStatusExpired, CreateTime: time.Now().Unix(), ExpireTime: time.Now().Add(-time.Minute).Unix(),
	}))

	completed, err := CompleteWechatPayTestOrder("WXT704TEST", "wechat-test-transaction", "notify-704", 1, "CNY")
	require.NoError(t, err)
	assert.True(t, completed)
	assert.Equal(t, 77, wechatPayUserQuota(t, 704))
	config, err := GetWechatPayConfig()
	require.NoError(t, err)
	assert.Positive(t, config.VerifiedAt)

	completed, err = CompleteWechatPayTestOrder("WXT704TEST", "wechat-test-transaction", "notify-704", 1, "CNY")
	require.NoError(t, err)
	assert.False(t, completed)
	assert.Equal(t, 77, wechatPayUserQuota(t, 704))

	_, err = CompleteWechatPayTestOrder("WXT704TEST", "different-test-transaction", "notify-705", 1, "CNY")
	require.Error(t, err)
	assert.Equal(t, 77, wechatPayUserQuota(t, 704))
}
