package service

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
)

func TestSettleSeedanceOrderForActualDurationUsesFrozenUnits(t *testing.T) {
	order := &model.SeedanceOrder{
		RequestedDurationMillis:         5_000,
		SaleUnitPriceMicroRMB:           2_000_000,
		SuperResolutionUnitCostMicroRMB: 500_000,
		VolcengineEstimatedMicroRMB:     5_000_000,
	}
	snapshot := &seedancePricingSnapshot{GroupRatio: 0.8, BaseUnitCostMicroRMB: 1_000_000}

	require.NoError(t, settleSeedanceOrderForActualDuration(order, snapshot, 2.25))
	require.EqualValues(t, 2_250, order.ActualDurationMillis)
	require.EqualValues(t, 3_600_000, order.ModelSaleMicroRMB)
	require.EqualValues(t, 1_125_000, order.SuperResolutionCostMicroRMB)
	require.EqualValues(t, 2_250_000, order.VolcengineEstimatedMicroRMB)
	require.EqualValues(t, 225_000, order.NewAPIEstimatedProfitMicroRMB)
}
