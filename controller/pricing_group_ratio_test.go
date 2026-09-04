package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/stretchr/testify/require"
)

func TestAttachModelGroupRatiosUsesGlobalGroupDiscountForAllModels(t *testing.T) {
	groupRatioSnapshot := ratio_setting.GroupRatio2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(groupRatioSnapshot))
	})

	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"VIP1":0.78}`))
	pricing := []model.Pricing{
		{ModelName: "regular-llm"},
		{ModelName: "seedance-pricing-model", IsIndependentSeedance: true},
	}

	attachModelGroupRatios(pricing, "default", map[string]float64{"default": 1, "VIP1": 0.78})

	require.Equal(t, map[string]float64{"default": 1, "VIP1": 0.78}, pricing[0].GroupRatio)
	require.Equal(t, map[string]float64{"default": 1, "VIP1": 0.78}, pricing[1].GroupRatio)
}

func TestDiscountedSeedanceSaleUsesTheSameVIPRatioAsTheQuote(t *testing.T) {
	sale, err := discountedSeedanceSaleMicroRMB(8_000_000, 0.78)
	require.NoError(t, err)
	require.Equal(t, int64(6_240_000), sale)
}

func TestPricingUsableGroupsKeepsAnonymousViewerOnDefault(t *testing.T) {
	groups := pricingUsableGroups("", false)
	require.Len(t, groups, 1)
	require.Contains(t, groups, "default")
}
