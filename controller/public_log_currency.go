package controller

import (
	"math"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

var publicLogUSDPriceFields = map[string]struct{}{
	"model_price":                 {},
	"web_search_price":            {},
	"file_search_price":           {},
	"audio_input_price":           {},
	"image_generation_call_price": {},
}

type publicTokenLogItem struct {
	*model.Log
	Currency string `json:"currency"`
}

func numberAsFloat64(value any) (float64, bool) {
	switch number := value.(type) {
	case float64:
		return number, true
	case float32:
		return float64(number), true
	case int:
		return float64(number), true
	case int8:
		return float64(number), true
	case int16:
		return float64(number), true
	case int32:
		return float64(number), true
	case int64:
		return float64(number), true
	case uint:
		return float64(number), true
	case uint8:
		return float64(number), true
	case uint16:
		return float64(number), true
	case uint32:
		return float64(number), true
	case uint64:
		return float64(number), true
	default:
		return 0, false
	}
}

func convertPublicLogMoneyFields(value any, exchangeRate float64) {
	switch typed := value.(type) {
	case map[string]interface{}:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		for _, key := range keys {
			child := typed[key]
			normalizedKey := strings.ToLower(strings.TrimSpace(key))
			if strings.HasSuffix(normalizedKey, "_usd") {
				if amount, ok := numberAsFloat64(child); ok && !math.IsNaN(amount) && !math.IsInf(amount, 0) {
					cnyKey := key[:len(key)-len("_usd")] + "_cny"
					typed[cnyKey] = amount * exchangeRate
				}
				delete(typed, key)
				continue
			}
			if _, ok := publicLogUSDPriceFields[normalizedKey]; ok {
				if amount, ok := numberAsFloat64(child); ok && !math.IsNaN(amount) && !math.IsInf(amount, 0) {
					typed[key] = amount * exchangeRate
				}
			}
			convertPublicLogMoneyFields(typed[key], exchangeRate)
		}
	case []interface{}:
		for _, child := range typed {
			convertPublicLogMoneyFields(child, exchangeRate)
		}
	}
}

func normalizePublicLogOtherToCNY(raw string, fallbackExchangeRate float64) string {
	other, err := common.StrToMap(raw)
	if err != nil || other == nil {
		return raw
	}
	if strings.EqualFold(strings.TrimSpace(anyToString(other["currency"])), pricingResponseCurrency) {
		return raw
	}

	exchangeRate := fallbackExchangeRate
	if snapshotRate, ok := numberAsFloat64(other["usd_exchange_rate"]); ok && snapshotRate > 0 &&
		!math.IsNaN(snapshotRate) && !math.IsInf(snapshotRate, 0) {
		exchangeRate = snapshotRate
	}
	if exchangeRate <= 0 || math.IsNaN(exchangeRate) || math.IsInf(exchangeRate, 0) {
		return raw
	}

	convertPublicLogMoneyFields(other, exchangeRate)
	other["currency"] = pricingResponseCurrency
	other["usd_exchange_rate"] = exchangeRate
	return common.MapToJsonStr(other)
}

func anyToString(value any) string {
	text, _ := value.(string)
	return text
}

func buildPublicTokenLogItems(logs []*model.Log) []*publicTokenLogItem {
	items := make([]*publicTokenLogItem, 0, len(logs))
	fallbackExchangeRate := pricingResponseExchangeRate()
	for _, logItem := range logs {
		if logItem == nil {
			continue
		}
		publicLog := *logItem
		publicLog.Other = normalizePublicLogOtherToCNY(publicLog.Other, fallbackExchangeRate)
		items = append(items, &publicTokenLogItem{
			Log:      &publicLog,
			Currency: pricingResponseCurrency,
		})
	}
	return items
}
