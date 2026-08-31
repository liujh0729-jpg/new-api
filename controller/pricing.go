package controller

import (
	"fmt"
	"math"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"github.com/gin-gonic/gin"
)

const pricingResponseCurrency = "CNY"

func pricingResponseExchangeRate() float64 {
	rate := operation_setting.USDExchangeRate
	if rate <= 0 || math.IsNaN(rate) || math.IsInf(rate, 0) {
		return 1
	}
	return rate
}

func convertUSDPriceToCNY(amount, exchangeRate float64) float64 {
	if amount <= 0 || math.IsNaN(amount) || math.IsInf(amount, 0) {
		return amount
	}
	return amount * exchangeRate
}

// convertPricingMonetaryFieldsToCNY converts only explicit monetary fields in
// the public pricing response. Ratios and billing expressions remain calculation
// metadata, while New API's internal billing and quota settlement stay in USD.
func convertPricingMonetaryFieldsToCNY(pricing []model.Pricing, exchangeRate float64) []model.Pricing {
	converted := make([]model.Pricing, len(pricing))
	copy(converted, pricing)
	for index := range converted {
		converted[index].ModelPrice = convertUSDPriceToCNY(converted[index].ModelPrice, exchangeRate)
		if converted[index].TaskPricing == nil {
			continue
		}

		taskPricing := *converted[index].TaskPricing
		taskPricing.NoReferenceVideoUnitPrice = convertUSDPriceToCNY(
			taskPricing.NoReferenceVideoUnitPrice,
			exchangeRate,
		)
		taskPricing.ReferenceVideoUnitPrice = convertUSDPriceToCNY(
			taskPricing.ReferenceVideoUnitPrice,
			exchangeRate,
		)
		if taskPricing.ByResolution != nil {
			byResolution := make(map[string]billing_setting.TaskPricingTier, len(taskPricing.ByResolution))
			for resolution, tier := range taskPricing.ByResolution {
				tier.NoReferenceVideoUnitPrice = convertUSDPriceToCNY(
					tier.NoReferenceVideoUnitPrice,
					exchangeRate,
				)
				tier.ReferenceVideoUnitPrice = convertUSDPriceToCNY(
					tier.ReferenceVideoUnitPrice,
					exchangeRate,
				)
				byResolution[resolution] = tier
			}
			taskPricing.ByResolution = byResolution
		}
		converted[index].TaskPricing = &taskPricing
	}
	return converted
}

func pricingResponseAmountToUSD(amount float64, currency string, exchangeRate float64) (float64, error) {
	switch strings.ToUpper(strings.TrimSpace(currency)) {
	case "", "USD":
		return amount, nil
	case pricingResponseCurrency:
		if exchangeRate <= 0 || math.IsNaN(exchangeRate) || math.IsInf(exchangeRate, 0) {
			return 0, fmt.Errorf("invalid USD exchange rate for CNY pricing response")
		}
		return amount / exchangeRate, nil
	default:
		return 0, fmt.Errorf("unsupported pricing response currency %q", currency)
	}
}

func filterPricingByUsableGroups(pricing []model.Pricing, usableGroup map[string]string) []model.Pricing {
	if len(pricing) == 0 {
		return pricing
	}
	if len(usableGroup) == 0 {
		return []model.Pricing{}
	}

	filtered := make([]model.Pricing, 0, len(pricing))
	for _, item := range pricing {
		if common.StringsContains(item.EnableGroup, "all") {
			filtered = append(filtered, item)
			continue
		}
		for _, group := range item.EnableGroup {
			if _, ok := usableGroup[group]; ok {
				filtered = append(filtered, item)
				break
			}
		}
	}
	return filtered
}

func attachModelGroupRatios(pricing []model.Pricing, userGroup string, availableRatios map[string]float64) {
	for index := range pricing {
		pricing[index].GroupRatio = make(map[string]float64, len(availableRatios))
		for usingGroup := range availableRatios {
			ratio, _ := ratio_setting.ResolveModelGroupRatio(pricing[index].ModelName, userGroup, usingGroup)
			pricing[index].GroupRatio[usingGroup] = ratio
		}
	}
}

func GetPricing(c *gin.Context) {
	pricing := model.GetPricing()
	userId, exists := c.Get("id")
	usableGroup := map[string]string{}
	groupRatio := map[string]float64{}
	for s, f := range ratio_setting.GetGroupRatioCopy() {
		groupRatio[s] = f
	}
	var group string
	if exists {
		user, err := model.GetUserCache(userId.(int))
		if err == nil {
			group = user.Group
			for g := range groupRatio {
				ratio, ok := ratio_setting.GetGroupGroupRatio(group, g)
				if ok {
					groupRatio[g] = ratio
				}
			}
		}
	}

	usableGroup = service.GetUserUsableGroups(group)
	pricing = filterPricingByUsableGroups(pricing, usableGroup)
	// check groupRatio contains usableGroup
	for group := range ratio_setting.GetGroupRatioCopy() {
		if _, ok := usableGroup[group]; !ok {
			delete(groupRatio, group)
		}
	}
	attachModelGroupRatios(pricing, group, groupRatio)
	exchangeRate := pricingResponseExchangeRate()
	pricing = convertPricingMonetaryFieldsToCNY(pricing, exchangeRate)

	c.JSON(200, gin.H{
		"success":            true,
		"data":               pricing,
		"currency":           pricingResponseCurrency,
		"usd_exchange_rate":  exchangeRate,
		"vendors":            model.GetVendors(),
		"group_ratio":        groupRatio,
		"usable_group":       usableGroup,
		"supported_endpoint": model.GetSupportedEndpointMap(),
		"auto_groups":        service.GetUserAutoGroup(group),
		"pricing_version":    "cny-v1-a42d372ccf0b5dd13ecf71203521f9d2",
	})
}

func ResetModelRatio(c *gin.Context) {
	defaultStr := ratio_setting.DefaultModelRatio2JSONString()
	err := model.UpdateOption("ModelRatio", defaultStr)
	if err != nil {
		c.JSON(200, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	err = ratio_setting.UpdateModelRatioByJSONString(defaultStr)
	if err != nil {
		c.JSON(200, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	c.JSON(200, gin.H{
		"success": true,
		"message": "重置模型倍率成功",
	})
}
