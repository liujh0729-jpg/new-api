package service

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/stretchr/testify/require"
)

func aipddCostEstimateTestCapabilities(t *testing.T, capabilities ...constant.AIPDDCapability) {
	t.Helper()
	previous := constant.GetAIPDDCapabilities()
	constant.SetAIPDDCapabilities(capabilities)
	t.Cleanup(func() { constant.SetAIPDDCapabilities(previous) })
}

func TestEstimateAIPDDSourceCostPricesTokenUsagePerTier(t *testing.T) {
	constant.ResetAIPDDTokenPrices()
	t.Cleanup(constant.ResetAIPDDTokenPrices)
	constant.SetAIPDDTokenPrices([]constant.AIPDDModelTokenPrice{{
		ModelName: "ap-glm-5.2", PromptPerMillion: 80_000, CompletionPerMillion: 280_000, CacheReadPerMillion: 20_000,
	}}, 0.0001)

	estimate, ok := EstimateAIPDDSourceCost(AIPDDCostBasis{
		ModelName: "ap-glm-5.2",
		Usage: &dto.Usage{
			PromptTokens: 1_000, CompletionTokens: 500,
			PromptTokensDetails: dto.InputTokenDetails{CachedTokens: 200},
		},
	})

	// 800 uncached prompt + 500 completion + 200 cache-read tokens, in AWCoin:
	// (800*80000 + 500*280000 + 200*20000) / 1e6 == 208
	require.True(t, ok)
	require.EqualValues(t, 208, estimate.AWCoin)
	require.EqualValues(t, 20_800, estimate.RMBMic)
}

func TestEstimateAIPDDSourceCostWithoutPublishedRateStaysEmpty(t *testing.T) {
	constant.ResetAIPDDTokenPrices()
	t.Cleanup(constant.ResetAIPDDTokenPrices)

	estimate, ok := EstimateAIPDDSourceCost(AIPDDCostBasis{
		ModelName: "ap-glm-5.2",
		Usage:     &dto.Usage{PromptTokens: 1_000, CompletionTokens: 500},
	})

	require.False(t, ok)
	require.Equal(t, AIPDDCostEstimate{}, estimate)
}

func TestEstimateAIPDDSourceCostRecoversUnitCountFromSettledQuota(t *testing.T) {
	constant.SetAIPDDTokenPrices(nil, 0.0001)
	t.Cleanup(constant.ResetAIPDDTokenPrices)
	aipddCostEstimateTestCapabilities(t, constant.AIPDDCapability{
		ModelName: "ap-image-2.0", TaskCost: 2_000,
		BillingType: constant.AIPDDBillingTypePerCall,
	})

	estimate, ok := EstimateAIPDDSourceCost(AIPDDCostBasis{
		ModelName:    "ap-image-2.0",
		ChargedQuota: 28_000,
		UnitPriceUSD: 0.028,
		GroupRatio:   1,
	})

	// The settled quota is two images at the local retail price, so the estimate
	// covers two images at the published price: 2 * 2000 AWCoin == 0.4 RMB.
	require.True(t, ok)
	require.EqualValues(t, 4_000, estimate.AWCoin)
	require.EqualValues(t, 400_000, estimate.RMBMic)
}

func TestEstimateAIPDDSourceCostUsesPublishedPerSecondPriceForDurationModels(t *testing.T) {
	constant.SetAIPDDTokenPrices(nil, 0.0001)
	t.Cleanup(constant.ResetAIPDDTokenPrices)
	aipddCostEstimateTestCapabilities(t, constant.AIPDDCapability{
		ModelName: "ap-ltx-2.5", TaskCost: 4_000,
		BillingType: constant.AIPDDBillingTypeDurationSeconds,
	})

	estimate, ok := EstimateAIPDDSourceCost(AIPDDCostBasis{
		ModelName:   "ap-ltx-2.5",
		Quantity:    5,
		BillingUnit: "second",
	})

	require.True(t, ok)
	require.EqualValues(t, 20_000, estimate.AWCoin)
	require.EqualValues(t, 2_000_000, estimate.RMBMic)
}

func TestEstimateAIPDDSourceCostWithoutCatalogEntryStaysEmpty(t *testing.T) {
	aipddCostEstimateTestCapabilities(t)
	constant.SetAIPDDTokenPrices(nil, 0.0001)
	t.Cleanup(constant.ResetAIPDDTokenPrices)

	_, ok := EstimateAIPDDSourceCost(AIPDDCostBasis{
		ModelName: "unknown-model",
		Usage:     &dto.Usage{PromptTokens: 10, CompletionTokens: 10},
	})

	require.False(t, ok)
}
