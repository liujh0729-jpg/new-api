package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func preserveWechatPayAmountSettings(t *testing.T) {
	t.Helper()
	previousQuotaPerUnit := common.QuotaPerUnit
	previousDisplayType := operation_setting.GetQuotaDisplayType()
	previousPrice := operation_setting.Price
	previousExchangeRate := operation_setting.USDExchangeRate
	previousMinTopUp := operation_setting.MinTopUp
	previousDiscount := operation_setting.GetPaymentSetting().AmountDiscount
	previousGroupRatio := common.TopupGroupRatio2JSONString()
	t.Cleanup(func() {
		common.QuotaPerUnit = previousQuotaPerUnit
		operation_setting.GetGeneralSetting().QuotaDisplayType = previousDisplayType
		operation_setting.Price = previousPrice
		operation_setting.USDExchangeRate = previousExchangeRate
		operation_setting.MinTopUp = previousMinTopUp
		operation_setting.GetPaymentSetting().AmountDiscount = previousDiscount
		require.NoError(t, common.UpdateTopupGroupRatioByJSONString(previousGroupRatio))
	})
}

func stringPointer(value string) *string {
	return &value
}

func TestBuildDomesticTopUpQuoteUsesCNYCreditValue(t *testing.T) {
	preserveWechatPayAmountSettings(t)
	common.QuotaPerUnit = 500000
	operation_setting.GetGeneralSetting().QuotaDisplayType = operation_setting.QuotaDisplayTypeCNY
	operation_setting.USDExchangeRate = 7.3
	operation_setting.Price = 7.3
	operation_setting.MinTopUp = 1
	operation_setting.GetPaymentSetting().AmountDiscount = map[int]float64{}
	require.NoError(t, common.UpdateTopupGroupRatioByJSONString(`{"default":1}`))

	quote, err := buildDomesticTopUpQuote(1, stringPointer(model.TopUpAmountUnitCNY), "default")
	require.NoError(t, err)
	assert.EqualValues(t, 100, quote.PaymentAmountCents)
	assert.EqualValues(t, 68493, quote.QuotaToAdd)
	assert.Equal(t, "1.00", quote.PaymentAmount.StringFixed(2))
}

func TestBuildDomesticTopUpQuoteSeparatesDisplayRateAndPrice(t *testing.T) {
	preserveWechatPayAmountSettings(t)
	common.QuotaPerUnit = 500000
	operation_setting.GetGeneralSetting().QuotaDisplayType = operation_setting.QuotaDisplayTypeCNY
	operation_setting.USDExchangeRate = 7.3
	operation_setting.Price = 8
	operation_setting.MinTopUp = 1
	operation_setting.GetPaymentSetting().AmountDiscount = map[int]float64{}
	require.NoError(t, common.UpdateTopupGroupRatioByJSONString(`{"default":1}`))

	quote, err := buildDomesticTopUpQuote(1, stringPointer(model.TopUpAmountUnitCNY), "default")
	require.NoError(t, err)
	assert.EqualValues(t, 110, quote.PaymentAmountCents)
	assert.EqualValues(t, 68493, quote.QuotaToAdd)
}

func TestBuildDomesticTopUpQuotePreservesLegacyCNYRequestSemantics(t *testing.T) {
	preserveWechatPayAmountSettings(t)
	common.QuotaPerUnit = 500000
	operation_setting.GetGeneralSetting().QuotaDisplayType = operation_setting.QuotaDisplayTypeCNY
	operation_setting.USDExchangeRate = 7.3
	operation_setting.Price = 7.305
	operation_setting.MinTopUp = 1
	operation_setting.GetPaymentSetting().AmountDiscount = map[int]float64{}
	require.NoError(t, common.UpdateTopupGroupRatioByJSONString(`{"default":1}`))

	quote, err := buildDomesticTopUpQuote(1, nil, "default")
	require.NoError(t, err)
	assert.Equal(t, model.TopUpAmountUnitUSD, quote.AmountUnit)
	assert.EqualValues(t, 731, quote.PaymentAmountCents)
	assert.EqualValues(t, 500000, quote.QuotaToAdd)
}

func TestBuildDomesticTopUpQuoteRejectsStaleAmountUnit(t *testing.T) {
	preserveWechatPayAmountSettings(t)
	operation_setting.GetGeneralSetting().QuotaDisplayType = operation_setting.QuotaDisplayTypeUSD
	operation_setting.Price = 7.3
	operation_setting.MinTopUp = 1

	_, err := buildDomesticTopUpQuote(1, stringPointer(model.TopUpAmountUnitCNY), "default")
	require.ErrorContains(t, err, "充值单位已变化")
}

func TestBuildDomesticTopUpQuoteAppliesGroupRatioDiscountAndRounding(t *testing.T) {
	preserveWechatPayAmountSettings(t)
	common.QuotaPerUnit = 500000
	operation_setting.GetGeneralSetting().QuotaDisplayType = operation_setting.QuotaDisplayTypeCNY
	operation_setting.USDExchangeRate = 7.3
	operation_setting.Price = 7.3
	operation_setting.MinTopUp = 1
	operation_setting.GetPaymentSetting().AmountDiscount = map[int]float64{10: 0.8}
	require.NoError(t, common.UpdateTopupGroupRatioByJSONString(`{"vip":1.25}`))

	quote, err := buildDomesticTopUpQuote(10, stringPointer(model.TopUpAmountUnitCNY), "vip")
	require.NoError(t, err)
	assert.EqualValues(t, 1000, quote.PaymentAmountCents)
	assert.EqualValues(t, 684931, quote.QuotaToAdd)
	assert.Equal(t, "10.00", quote.PaymentAmount.StringFixed(2))
}

func TestBuildDomesticTopUpQuoteSupportsUSDAndTokenUnits(t *testing.T) {
	t.Run("USD", func(t *testing.T) {
		preserveWechatPayAmountSettings(t)
		common.QuotaPerUnit = 500000
		operation_setting.GetGeneralSetting().QuotaDisplayType = operation_setting.QuotaDisplayTypeUSD
		operation_setting.Price = 7.3
		operation_setting.MinTopUp = 1
		operation_setting.GetPaymentSetting().AmountDiscount = map[int]float64{}
		require.NoError(t, common.UpdateTopupGroupRatioByJSONString(`{"default":1}`))

		quote, err := buildDomesticTopUpQuote(2, stringPointer(model.TopUpAmountUnitUSD), "default")
		require.NoError(t, err)
		assert.EqualValues(t, 1460, quote.PaymentAmountCents)
		assert.EqualValues(t, 1000000, quote.QuotaToAdd)
	})

	t.Run("TOKENS", func(t *testing.T) {
		preserveWechatPayAmountSettings(t)
		common.QuotaPerUnit = 500000
		operation_setting.GetGeneralSetting().QuotaDisplayType = operation_setting.QuotaDisplayTypeTokens
		operation_setting.Price = 7.3
		operation_setting.MinTopUp = 1
		operation_setting.GetPaymentSetting().AmountDiscount = map[int]float64{}
		require.NoError(t, common.UpdateTopupGroupRatioByJSONString(`{"default":1}`))

		quote, err := buildDomesticTopUpQuote(500000, stringPointer(model.TopUpAmountUnitTokens), "default")
		require.NoError(t, err)
		assert.EqualValues(t, 730, quote.PaymentAmountCents)
		assert.EqualValues(t, 500000, quote.QuotaToAdd)
	})
}

func TestBuildDomesticTopUpQuoteRejectsInvalidAmountsAndConfiguration(t *testing.T) {
	t.Run("below minimum", func(t *testing.T) {
		preserveWechatPayAmountSettings(t)
		operation_setting.GetGeneralSetting().QuotaDisplayType = operation_setting.QuotaDisplayTypeCNY
		operation_setting.USDExchangeRate = 7.3
		operation_setting.Price = 7.3
		operation_setting.MinTopUp = 1

		_, err := buildDomesticTopUpQuote(0, stringPointer(model.TopUpAmountUnitCNY), "default")
		require.Error(t, err)
	})

	t.Run("invalid exchange rate", func(t *testing.T) {
		preserveWechatPayAmountSettings(t)
		common.QuotaPerUnit = 500000
		operation_setting.GetGeneralSetting().QuotaDisplayType = operation_setting.QuotaDisplayTypeCNY
		operation_setting.USDExchangeRate = 0
		operation_setting.Price = 7.3
		operation_setting.MinTopUp = 1

		_, err := buildDomesticTopUpQuote(1, stringPointer(model.TopUpAmountUnitCNY), "default")
		require.ErrorContains(t, err, "展示汇率无效")
	})

	t.Run("payment below one cent", func(t *testing.T) {
		preserveWechatPayAmountSettings(t)
		common.QuotaPerUnit = 500000
		operation_setting.GetGeneralSetting().QuotaDisplayType = operation_setting.QuotaDisplayTypeCNY
		operation_setting.USDExchangeRate = 7.3
		operation_setting.Price = 0.001
		operation_setting.MinTopUp = 1
		operation_setting.GetPaymentSetting().AmountDiscount = map[int]float64{}
		require.NoError(t, common.UpdateTopupGroupRatioByJSONString(`{"default":1}`))

		_, err := buildDomesticTopUpQuote(1, stringPointer(model.TopUpAmountUnitCNY), "default")
		require.ErrorContains(t, err, "充值金额过低")
	})

	t.Run("zero quota", func(t *testing.T) {
		preserveWechatPayAmountSettings(t)
		common.QuotaPerUnit = 0.5
		operation_setting.GetGeneralSetting().QuotaDisplayType = operation_setting.QuotaDisplayTypeCNY
		operation_setting.USDExchangeRate = 7.3
		operation_setting.Price = 7.3
		operation_setting.MinTopUp = 1
		operation_setting.GetPaymentSetting().AmountDiscount = map[int]float64{}
		require.NoError(t, common.UpdateTopupGroupRatioByJSONString(`{"default":1}`))

		_, err := buildDomesticTopUpQuote(1, stringPointer(model.TopUpAmountUnitCNY), "default")
		require.ErrorContains(t, err, "充值额度无效")
	})
}
