package common

import "math"

func GetTrustQuota() int {
	return int(10 * QuotaPerUnit)
}

// QuotaToCNY converts internal quota units to their CNY equivalent.
// The result is rounded to six decimal places to match quota display precision.
func QuotaToCNY(quota int, quotaPerUnit, usdExchangeRate float64) float64 {
	if quotaPerUnit <= 0 || usdExchangeRate <= 0 {
		return 0
	}
	amount := float64(quota) / quotaPerUnit * usdExchangeRate
	return math.Round(amount*1_000_000) / 1_000_000
}
