package seedance

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/require"
)

func TestBuildSeedanceTaskPricingQuoteAppliesMembershipAnd480pExemption(t *testing.T) {
	originalGroupRatios := ratio_setting.GroupRatio2JSONString()
	originalGroupGroupRatios := ratio_setting.GroupGroupRatio2JSONString()
	originalExchangeRate := operation_setting.USDExchangeRate
	originalQuotaPerUnit := common.QuotaPerUnit
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(originalGroupRatios))
		require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(originalGroupGroupRatios))
		operation_setting.USDExchangeRate = originalExchangeRate
		common.QuotaPerUnit = originalQuotaPerUnit
	})
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1.25}`))
	require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(`{}`))
	operation_setting.USDExchangeRate = 1
	common.QuotaPerUnit = 500_000

	info := &relaycommon.RelayInfo{
		UserGroup:       "default",
		UsingGroup:      "default",
		OriginModelName: "AP-Seedance-Pro",
		MembershipRatioInfo: types.MembershipRatioInfo{
			Code:                    "VIP1",
			ConfiguredMultiplierPPM: 800_000,
			AppliedMultiplierPPM:    800_000,
		},
	}
	offering := &model.SeedanceModelOffering{
		SourceResolution:             "480p",
		NoReferenceUnitPriceMicroRMB: 1_000_000,
		PricingVersion:               "v1",
	}

	quote480 := buildSeedanceTaskPricingQuoteForRequest(offering, info, 2, false)
	require.InDelta(t, 2.5, quote480.SaleUSD, 1e-9)
	require.EqualValues(t, 1_000_000, quote480.AppliedMemberPPM)
	require.True(t, quote480.MembershipExempt)

	offering.SourceResolution = "720p"
	quote720 := buildSeedanceTaskPricingQuoteForRequest(offering, info, 2, false)
	require.InDelta(t, 2.0, quote720.SaleUSD, 1e-9)
	require.EqualValues(t, 800_000, quote720.AppliedMemberPPM)
	require.False(t, quote720.MembershipExempt)
}
