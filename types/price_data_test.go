package types

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAPSeedance480PMembershipPolicy(t *testing.T) {
	membership := MembershipRatioInfo{
		Code:                    "VIP1",
		DisplayName:             "VIP 1",
		ConfiguredMultiplierPPM: 800_000,
		AppliedMultiplierPPM:    800_000,
	}

	for _, modelName := range []string{
		"ap-seedance-2.0",
		"AP Seedance-2.0 标准版",
		"prefix_ap_seedance_model",
	} {
		applied := ApplyMembershipPricingPolicy(membership, modelName, " 480P ")
		require.True(t, applied.Exempt, modelName)
		require.Equal(t, int64(MembershipMultiplierScale), applied.AppliedMultiplierPPM, modelName)
		require.Equal(t, int64(800_000), applied.ConfiguredMultiplierPPM, modelName)
		require.Equal(t, "ap_seedance_480p", applied.ExemptionReason, modelName)
	}

	notExempt := ApplyMembershipPricingPolicy(membership, "AP Seedance-2.0 标准版", "720p")
	require.False(t, notExempt.Exempt)
	require.Equal(t, int64(800_000), notExempt.AppliedMultiplierPPM)

	nonAP := ApplyMembershipPricingPolicy(membership, "seedance-2.0", "480p")
	require.False(t, nonAP.Exempt)
	require.Equal(t, int64(800_000), nonAP.AppliedMultiplierPPM)
}

func TestZeroMembershipInfoMeansNormalMultiplier(t *testing.T) {
	var membership MembershipRatioInfo
	require.Equal(t, 1.0, membership.ConfiguredMultiplier())
	require.Equal(t, 1.0, membership.AppliedMultiplier())
}
