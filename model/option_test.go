package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/require"
)

func TestUpdateDisplayInCurrencyPreservesCurrencyDisplayType(t *testing.T) {
	originalOptionMap := common.OptionMap
	originalDisplayType := operation_setting.GetGeneralSetting().QuotaDisplayType
	t.Cleanup(func() {
		common.OptionMap = originalOptionMap
		operation_setting.GetGeneralSetting().QuotaDisplayType = originalDisplayType
	})

	common.OptionMap = map[string]string{}

	operation_setting.GetGeneralSetting().QuotaDisplayType = operation_setting.QuotaDisplayTypeCNY
	require.NoError(t, updateOptionMap("DisplayInCurrencyEnabled", "true"))
	require.Equal(t, operation_setting.QuotaDisplayTypeCNY, operation_setting.GetQuotaDisplayType())

	operation_setting.GetGeneralSetting().QuotaDisplayType = operation_setting.QuotaDisplayTypeCustom
	require.NoError(t, updateOptionMap("DisplayInCurrencyEnabled", "true"))
	require.Equal(t, operation_setting.QuotaDisplayTypeCustom, operation_setting.GetQuotaDisplayType())
}

func TestUpdateDisplayInCurrencyTogglesTokenMode(t *testing.T) {
	originalOptionMap := common.OptionMap
	originalDisplayType := operation_setting.GetGeneralSetting().QuotaDisplayType
	t.Cleanup(func() {
		common.OptionMap = originalOptionMap
		operation_setting.GetGeneralSetting().QuotaDisplayType = originalDisplayType
	})

	common.OptionMap = map[string]string{}

	operation_setting.GetGeneralSetting().QuotaDisplayType = operation_setting.QuotaDisplayTypeCNY
	require.NoError(t, updateOptionMap("DisplayInCurrencyEnabled", "false"))
	require.Equal(t, operation_setting.QuotaDisplayTypeTokens, operation_setting.GetQuotaDisplayType())

	require.NoError(t, updateOptionMap("DisplayInCurrencyEnabled", "true"))
	require.Equal(t, operation_setting.QuotaDisplayTypeUSD, operation_setting.GetQuotaDisplayType())
}

func TestValidateMinTopUpRequiresPositiveInteger(t *testing.T) {
	require.NoError(t, validateOptionValue("MinTopUp", "1"))
	require.NoError(t, validateOptionValue("MinTopUp", "100"))

	for _, value := range []string{"", "0", "-1", "0.01", "1.5", "invalid"} {
		t.Run(value, func(t *testing.T) {
			require.Error(t, validateOptionValue("MinTopUp", value))
		})
	}
}

func TestUpdateOptionMapRejectsInvalidMinTopUpBeforeMutation(t *testing.T) {
	originalOptionMap := common.OptionMap
	t.Cleanup(func() { common.OptionMap = originalOptionMap })
	common.OptionMap = map[string]string{"MinTopUp": "1"}

	require.Error(t, updateOptionMap("MinTopUp", "0.01"))
	require.Equal(t, "1", common.OptionMap["MinTopUp"])
}

func TestUpdateOptionRejectsInvalidMinTopUpBeforeDatabaseWrite(t *testing.T) {
	truncateTables(t)

	require.Error(t, UpdateOption("MinTopUp", "0.01"))
	var count int64
	require.NoError(t, DB.Model(&Option{}).Where(&Option{Key: "MinTopUp"}).Count(&count).Error)
	require.Zero(t, count)
}

func TestLoadOptionsRepairsLegacyInvalidMinTopUp(t *testing.T) {
	truncateTables(t)
	previousOptionMap := common.OptionMap
	previousMinTopUp := operation_setting.MinTopUp
	t.Cleanup(func() {
		common.OptionMap = previousOptionMap
		operation_setting.MinTopUp = previousMinTopUp
	})
	common.OptionMap = map[string]string{"MinTopUp": "1"}
	require.NoError(t, DB.Create(&Option{Key: "MinTopUp", Value: "0.01"}).Error)

	loadOptionsFromDatabase()

	var option Option
	require.NoError(t, DB.Where(&Option{Key: "MinTopUp"}).First(&option).Error)
	require.Equal(t, "1", option.Value)
	require.Equal(t, "1", common.OptionMap["MinTopUp"])
	require.Equal(t, 1, operation_setting.MinTopUp)
}
