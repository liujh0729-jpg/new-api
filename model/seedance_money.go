package model

import (
	"errors"
	"math"
	"math/big"
)

var errSeedanceMoneyOverflow = errors.New("Seedance money calculation exceeds the supported micro-RMB range")

// CalculateSeedanceTimedAmount converts a frozen micro-RMB/second unit price
// into an order amount. Rounding happens exactly once at the order boundary,
// half up to the nearest micro-RMB, and big.Int prevents multiplication from
// overflowing before the final int64 range check.
func CalculateSeedanceTimedAmount(unitMicroRMBPerSecond, durationMillis int64) (int64, error) {
	if unitMicroRMBPerSecond < 0 || durationMillis < 0 {
		return 0, errors.New("Seedance price and duration must be non-negative")
	}
	product := new(big.Int).Mul(big.NewInt(unitMicroRMBPerSecond), big.NewInt(durationMillis))
	product.Add(product, big.NewInt(500))
	product.Quo(product, big.NewInt(1000))
	if !product.IsInt64() {
		return 0, errSeedanceMoneyOverflow
	}
	return product.Int64(), nil
}

func checkedSeedanceMoneyAdd(left, right int64) (int64, error) {
	if (right > 0 && left > math.MaxInt64-right) || (right < 0 && left < math.MinInt64-right) {
		return 0, errSeedanceMoneyOverflow
	}
	return left + right, nil
}

func checkedSeedanceMoneySubtract(left, right int64) (int64, error) {
	if (right > 0 && left < math.MinInt64+right) || (right < 0 && left > math.MaxInt64+right) {
		return 0, errSeedanceMoneyOverflow
	}
	return left - right, nil
}

// CalculateSeedanceProfit keeps all monetary arithmetic exact and rejects a
// snapshot whose valid int64 inputs would produce an invalid int64 result.
func CalculateSeedanceProfit(sale, serviceCharge, volcengineCost int64) (int64, error) {
	if sale < 0 || serviceCharge < 0 || volcengineCost < 0 {
		return 0, errors.New("Seedance money fields must be non-negative integer micro-RMB")
	}
	profit, err := checkedSeedanceMoneySubtract(sale, serviceCharge)
	if err != nil {
		return 0, err
	}
	return checkedSeedanceMoneySubtract(profit, volcengineCost)
}

// CalculateSeedanceFailureProfit keeps provider execution cost out of NewAPI's
// profit. That cost belongs exclusively to AIPDD's service-profit ledger.
func CalculateSeedanceFailureProfit(volcengineCost int64) (int64, error) {
	if volcengineCost < 0 {
		return 0, errors.New("Seedance money fields must be non-negative integer micro-RMB")
	}
	return -volcengineCost, nil
}

func ValidateSeedanceMoneySnapshot(sale, serviceCharge, volcengineCost int64, providerCost *int64) error {
	if providerCost != nil && *providerCost < 0 {
		return errors.New("Seedance money fields must be non-negative integer micro-RMB")
	}
	if _, err := CalculateSeedanceProfit(sale, serviceCharge, volcengineCost); err != nil {
		return err
	}
	_, err := CalculateSeedanceFailureProfit(volcengineCost)
	return err
}
