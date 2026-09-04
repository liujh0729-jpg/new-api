package model

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCalculateSeedanceTimedAmountRoundsOnceAtOrderBoundary(t *testing.T) {
	amount, err := CalculateSeedanceTimedAmount(1_000_001, 2_250)
	require.NoError(t, err)
	require.EqualValues(t, 2_250_002, amount)

	amount, err = CalculateSeedanceTimedAmount(1, 500)
	require.NoError(t, err)
	require.EqualValues(t, 1, amount)

	_, err = CalculateSeedanceTimedAmount(math.MaxInt64, math.MaxInt64)
	require.ErrorIs(t, err, errSeedanceMoneyOverflow)
}

func TestSeedanceMoneyCalculationsAreExactAtInt64Boundaries(t *testing.T) {
	profit, err := CalculateSeedanceProfit(math.MaxInt64, math.MaxInt64, math.MaxInt64)
	require.NoError(t, err)
	require.Equal(t, int64(-math.MaxInt64), profit)

	failureProfit, err := CalculateSeedanceFailureProfit(math.MaxInt64)
	require.NoError(t, err)
	require.Equal(t, int64(-math.MaxInt64), failureProfit)

	_, err = CalculateSeedanceProfit(0, math.MaxInt64, math.MaxInt64)
	require.ErrorContains(t, err, "exceeds")
}

func TestSeedanceMoneySnapshotRejectsNegativeValues(t *testing.T) {
	require.ErrorContains(t, ValidateSeedanceMoneySnapshot(-1, 0, 0, nil), "non-negative")
	providerCost := int64(-1)
	require.ErrorContains(t, ValidateSeedanceMoneySnapshot(0, 0, 0, &providerCost), "non-negative")
}
