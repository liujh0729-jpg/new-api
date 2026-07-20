package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func preserveWechatPayAmountSettings(t *testing.T) {
	t.Helper()
	previousQuotaPerUnit := common.QuotaPerUnit
	previousDisplayType := operation_setting.GetQuotaDisplayType()
	previousPrice := operation_setting.Price
	previousDiscount := operation_setting.GetPaymentSetting().AmountDiscount
	previousGroupRatio := common.TopupGroupRatio2JSONString()
	t.Cleanup(func() {
		common.QuotaPerUnit = previousQuotaPerUnit
		operation_setting.GetGeneralSetting().QuotaDisplayType = previousDisplayType
		operation_setting.Price = previousPrice
		operation_setting.GetPaymentSetting().AmountDiscount = previousDiscount
		require.NoError(t, common.UpdateTopupGroupRatioByJSONString(previousGroupRatio))
	})
}

func TestCalculateWechatPayQuotaToAddUsesOrderTimeQuota(t *testing.T) {
	preserveWechatPayAmountSettings(t)
	common.QuotaPerUnit = 500000
	operation_setting.GetGeneralSetting().QuotaDisplayType = operation_setting.QuotaDisplayTypeUSD

	quota, err := calculateWechatPayQuotaToAdd(2, 2)
	require.NoError(t, err)
	assert.EqualValues(t, 1000000, quota)
}

func TestCalculateWechatPayQuotaToAddPreservesTokenAmount(t *testing.T) {
	preserveWechatPayAmountSettings(t)
	common.QuotaPerUnit = 500000
	operation_setting.GetGeneralSetting().QuotaDisplayType = operation_setting.QuotaDisplayTypeTokens

	quota, err := calculateWechatPayQuotaToAdd(750001, 1)
	require.NoError(t, err)
	assert.EqualValues(t, 750001, quota)
}

func TestGetPayMoneyCentsRoundsWithDecimalArithmetic(t *testing.T) {
	preserveWechatPayAmountSettings(t)
	operation_setting.GetGeneralSetting().QuotaDisplayType = operation_setting.QuotaDisplayTypeUSD
	operation_setting.Price = 7.305
	operation_setting.GetPaymentSetting().AmountDiscount = map[int]float64{}
	require.NoError(t, common.UpdateTopupGroupRatioByJSONString(`{"default":1}`))

	assert.EqualValues(t, 731, getPayMoneyCents(1, "default"))
}
