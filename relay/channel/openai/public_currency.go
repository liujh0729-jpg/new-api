package openai

import (
	"math"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/setting/operation_setting"
)

const publicUsageCurrencyCNY = "CNY"

func publicUsageExchangeRate() float64 {
	rate := operation_setting.USDExchangeRate
	if rate <= 0 || math.IsNaN(rate) || math.IsInf(rate, 0) {
		return 1
	}
	return rate
}

func usageNumberAsFloat64(value any) (float64, bool) {
	switch number := value.(type) {
	case float64:
		return number, true
	case float32:
		return float64(number), true
	case int:
		return float64(number), true
	case int64:
		return float64(number), true
	case uint:
		return float64(number), true
	case uint64:
		return float64(number), true
	default:
		return 0, false
	}
}

func convertUsageCostFieldsToCNY(value any, exchangeRate float64) bool {
	changed := false
	switch typed := value.(type) {
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		for _, key := range keys {
			child := typed[key]
			normalizedKey := strings.ToLower(strings.TrimSpace(key))
			if strings.HasSuffix(normalizedKey, "_usd") {
				if amount, ok := usageNumberAsFloat64(child); ok && !math.IsNaN(amount) && !math.IsInf(amount, 0) {
					cnyKey := key[:len(key)-len("_usd")] + "_cny"
					typed[cnyKey] = amount * exchangeRate
					changed = true
				}
				delete(typed, key)
				changed = true
				continue
			}
			if normalizedKey == "cost" || strings.HasSuffix(normalizedKey, "_cost") {
				if amount, ok := usageNumberAsFloat64(child); ok && !math.IsNaN(amount) && !math.IsInf(amount, 0) {
					typed[key] = amount * exchangeRate
					changed = true
				}
			}
			if convertUsageCostFieldsToCNY(typed[key], exchangeRate) {
				changed = true
			}
		}
	case []any:
		for _, child := range typed {
			if convertUsageCostFieldsToCNY(child, exchangeRate) {
				changed = true
			}
		}
	}
	return changed
}

func convertPublicUsageCostsToCNY(responseBody []byte) []byte {
	if len(responseBody) == 0 {
		return responseBody
	}
	var payload map[string]any
	if err := common.Unmarshal(responseBody, &payload); err != nil {
		return responseBody
	}
	usage, ok := payload["usage"].(map[string]any)
	if !ok || usage == nil {
		return responseBody
	}
	currency, _ := usage["currency"].(string)
	if strings.EqualFold(strings.TrimSpace(currency), publicUsageCurrencyCNY) {
		return responseBody
	}
	if currency != "" && !strings.EqualFold(strings.TrimSpace(currency), "USD") {
		return responseBody
	}

	exchangeRate := publicUsageExchangeRate()
	if !convertUsageCostFieldsToCNY(usage, exchangeRate) {
		return responseBody
	}
	usage["currency"] = publicUsageCurrencyCNY
	usage["usd_exchange_rate"] = exchangeRate
	converted, err := common.Marshal(payload)
	if err != nil {
		return responseBody
	}
	return converted
}

func publicUsageCopyWithCNYCost(usage *dto.Usage) *dto.Usage {
	if usage == nil {
		return nil
	}
	publicUsage := *usage
	amount, ok := usageNumberAsFloat64(publicUsage.Cost)
	if !ok || math.IsNaN(amount) || math.IsInf(amount, 0) {
		return &publicUsage
	}
	exchangeRate := publicUsageExchangeRate()
	publicUsage.Cost = amount * exchangeRate
	publicUsage.Currency = publicUsageCurrencyCNY
	publicUsage.USDExchangeRate = exchangeRate
	return &publicUsage
}
