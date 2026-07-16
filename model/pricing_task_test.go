package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/stretchr/testify/require"
)

func TestPricingExposesStructuredTaskPricingAndHidesUnconfiguredSeedance(t *testing.T) {
	truncateTables(t)
	const (
		modelName         = "local Seedance pricing alias"
		upstreamModelName = "AP Seedance pricing API test"
	)

	configSnapshot := config.GlobalConfig.ExportAllConfigs()
	modelPriceSnapshot := ratio_setting.ModelPrice2JSONString()
	capabilitiesSnapshot := constant.GetAIPDDCapabilities()
	t.Cleanup(func() {
		require.NoError(t, config.GlobalConfig.LoadFromDB(configSnapshot))
		require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(modelPriceSnapshot))
		constant.SetAIPDDCapabilities(capabilitiesSnapshot)
		InvalidatePricingCache()
	})

	constant.SetAIPDDCapabilities([]constant.AIPDDCapability{{
		ModelName:    upstreamModelName,
		AdapterCode:  "seedance",
		EndpointType: constant.EndpointTypeOpenAIVideo,
	}})
	modelMapping := `{"local Seedance pricing alias":"AP Seedance pricing API test"}`
	channel := Channel{
		Type:         constant.ChannelTypeAIPDD,
		Name:         "task-pricing-public-test",
		Key:          "sk-test",
		Group:        "default",
		Models:       modelName,
		ModelMapping: &modelMapping,
		Status:       common.ChannelStatusEnabled,
	}
	require.NoError(t, DB.Create(&channel).Error)
	require.NoError(t, DB.Create(&Ability{
		Group: "default", Model: modelName, ChannelId: channel.Id, Enabled: true,
	}).Error)
	require.NoError(t, DB.Create(&Model{
		ModelName: modelName, Status: 1, NameRule: NameRuleExact,
	}).Error)

	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"billing_setting.billing_mode": `{"local Seedance pricing alias":"task_pricing"}`,
		"billing_setting.task_pricing": `{"local Seedance pricing alias":{"unit":"second","no_reference_video_unit_price":0.18,"reference_video_policy":"custom","reference_video_unit_price":0.12}}`,
	}))
	InvalidatePricingCache()
	require.True(t, IsAIPDDSeedancePricingRequiredModel(modelName))

	configured, ok := findPricingForTest(GetPricing(), modelName)
	require.True(t, ok)
	require.Equal(t, "task_pricing", configured.BillingMode)
	require.NotNil(t, configured.TaskPricing)
	require.Equal(t, 0.18, configured.TaskPricing.NoReferenceVideoUnitPrice)
	require.Equal(t, 0.12, configured.TaskPricing.ReferenceVideoUnitPrice)
	require.Equal(t, 0.12, configured.ModelPrice, "compatibility model_price must be the lowest valid per-second price")
	require.Equal(t, 1, configured.QuotaType)

	// A legacy fixed ModelPrice is deliberately not a fallback for Seedance.
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"local Seedance pricing alias":99}`))
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"billing_setting.billing_mode": `{}`,
		"billing_setting.task_pricing": `{}`,
	}))
	InvalidatePricingCache()
	_, ok = findPricingForTest(GetPricing(), modelName)
	require.False(t, ok)
}

func findPricingForTest(items []Pricing, modelName string) (Pricing, bool) {
	for _, item := range items {
		if item.ModelName == modelName {
			return item, true
		}
	}
	return Pricing{}, false
}
