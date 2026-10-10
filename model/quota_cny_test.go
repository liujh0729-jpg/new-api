package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/assert"
)

func preserveQuotaCurrencySettings(t *testing.T) {
	t.Helper()
	quotaPerUnit := common.QuotaPerUnit
	usdExchangeRate := operation_setting.USDExchangeRate
	t.Cleanup(func() {
		common.QuotaPerUnit = quotaPerUnit
		operation_setting.USDExchangeRate = usdExchangeRate
	})
}

func TestLogQuotaCNYUsesBillingSnapshot(t *testing.T) {
	preserveQuotaCurrencySettings(t)
	common.QuotaPerUnit = 999999
	operation_setting.USDExchangeRate = 8

	log := &Log{
		Quota: 5000,
		Other: common.MapToJsonStr(map[string]interface{}{
			"quota_per_unit":    500000,
			"usd_exchange_rate": 7.3,
		}),
	}
	log.setQuotaCNY()

	assert.Equal(t, 0.073, log.QuotaCNY)
}

func TestLogQuotaCNYLegacyFallbackUsesCurrentSettings(t *testing.T) {
	preserveQuotaCurrencySettings(t)
	common.QuotaPerUnit = 500000
	operation_setting.USDExchangeRate = 7.3

	log := &Log{Quota: 5000}
	log.setQuotaCNY()

	assert.Equal(t, 0.073, log.QuotaCNY)
}

func TestTaskQuotaCNYUsesBillingSnapshot(t *testing.T) {
	preserveQuotaCurrencySettings(t)
	common.QuotaPerUnit = 999999
	operation_setting.USDExchangeRate = 8

	task := &Task{
		Quota: 5000,
		PrivateData: TaskPrivateData{
			BillingContext: &TaskBillingContext{
				QuotaPerUnit:    500000,
				USDExchangeRate: 7.3,
			},
		},
	}

	assert.Equal(t, 0.073, task.GetQuotaCNY())
}
