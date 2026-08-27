package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/stretchr/testify/require"
)

func TestAttachModelGroupRatiosShowsVIPDiscountOnlyForSeedance(t *testing.T) {
	groupRatioSnapshot := ratio_setting.GroupRatio2JSONString()
	capabilitiesSnapshot := constant.GetAIPDDCapabilities()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(groupRatioSnapshot))
		constant.SetAIPDDCapabilities(capabilitiesSnapshot)
	})

	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"VIP1":0.78}`))
	constant.SetAIPDDCapabilities([]constant.AIPDDCapability{
		{ModelName: "seedance-pricing-model", AdapterCode: "seedance"},
	})
	pricing := []model.Pricing{
		{ModelName: "regular-llm"},
		{ModelName: "seedance-pricing-model"},
	}

	attachModelGroupRatios(pricing, "default", map[string]float64{"default": 1, "VIP1": 0.78})

	require.Equal(t, map[string]float64{"default": 1, "VIP1": 1}, pricing[0].GroupRatio)
	require.Equal(t, map[string]float64{"default": 1, "VIP1": 0.78}, pricing[1].GroupRatio)
}
