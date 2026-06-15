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
