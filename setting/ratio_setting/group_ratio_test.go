package ratio_setting

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/stretchr/testify/require"
)

func TestResolveModelGroupRatioLimitsFixedVIPDiscountsToSeedance(t *testing.T) {
	groupRatioSnapshot := GroupRatio2JSONString()
	groupGroupRatioSnapshot := GroupGroupRatio2JSONString()
	capabilitiesSnapshot := constant.GetAIPDDCapabilities()
	t.Cleanup(func() {
		require.NoError(t, UpdateGroupRatioByJSONString(groupRatioSnapshot))
		require.NoError(t, UpdateGroupGroupRatioByJSONString(groupGroupRatioSnapshot))
		constant.SetAIPDDCapabilities(capabilitiesSnapshot)
	})

	require.NoError(t, UpdateGroupRatioByJSONString(`{"default":1,"VIP1":0.78,"custom-sale":0.8}`))
	require.NoError(t, UpdateGroupGroupRatioByJSONString(`{"member":{"VIP1":0.7,"custom-sale":0.6}}`))
	constant.SetAIPDDCapabilities([]constant.AIPDDCapability{
		{ModelName: "seedance-vip-model", AdapterCode: "seedance"},
	})

	ratio, special := ResolveModelGroupRatio("regular-llm", "member", "VIP1")
	require.Equal(t, 1.0, ratio)
	require.False(t, special)

	ratio, special = ResolveModelGroupRatio("seedance-vip-model", "member", "VIP1")
	require.Equal(t, 0.7, ratio)
	require.True(t, special)

	ratio, special = ResolveModelGroupRatio("regular-llm", "member", "custom-sale")
	require.Equal(t, 0.6, ratio)
	require.True(t, special)
}
