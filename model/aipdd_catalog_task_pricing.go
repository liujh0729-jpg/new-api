package model

import (
	"errors"
	"fmt"
	"math"
	"reflect"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/aipddcatalog"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	aipddBillingModeOptionKey = "billing_setting.billing_mode"
	aipddTaskPricingOptionKey = "billing_setting.task_pricing"
)

// syncAIPDDTokenMarketTaskPricingTx imports the final display sale price for
// Token Market duration models. Seedance intentionally remains administrator-
// managed because its downstream retail price comes from suggestedRetail...
// rather than the authenticated display price.
func syncAIPDDTokenMarketTaskPricingTx(
	tx *gorm.DB,
	catalog aipddcatalog.AtomicCatalog,
) (int, map[string]string, error) {
	rmbPerAWCoin := catalog.AWCoinRate.RMBPerAWCoin
	rmbPerUSD := operation_setting.USDExchangeRate
	if !finitePositive(rmbPerAWCoin) {
		return 0, nil, fmt.Errorf("AIPDD catalog rmbPerAwcoin must be finite and greater than 0")
	}
	if !finitePositive(rmbPerUSD) {
		return 0, nil, fmt.Errorf("New API USDExchangeRate must be finite and greater than 0")
	}

	billingModes, err := loadAIPDDBillingModesTx(tx)
	if err != nil {
		return 0, nil, err
	}
	taskPricing, err := loadAIPDDTaskPricingTx(tx)
	if err != nil {
		return 0, nil, err
	}

	updated := 0
	for _, capability := range catalog.Capabilities {
		if !isTokenMarketDurationCapability(capability) {
			continue
		}
		modelName := strings.TrimSpace(capability.ID)
		if modelName == "" {
			continue
		}

		if !tokenMarketCapabilityAvailable(capability) {
			changed := false
			if _, ok := taskPricing[modelName]; ok {
				delete(taskPricing, modelName)
				changed = true
			}
			if billingModes[modelName] == billing_setting.BillingModeTaskPricing {
				delete(billingModes, modelName)
				changed = true
			}
			if changed {
				updated++
			}
			continue
		}

		config, err := tokenMarketTaskPricingConfig(capability, rmbPerAWCoin, rmbPerUSD)
		if err != nil {
			return 0, nil, fmt.Errorf("build Token Market task pricing for %q: %w", modelName, err)
		}
		previous, exists := taskPricing[modelName]
		if billingModes[modelName] != billing_setting.BillingModeTaskPricing ||
			!exists || !reflect.DeepEqual(previous, config) {
			updated++
		}
		taskPricing[modelName] = config
		billingModes[modelName] = billing_setting.BillingModeTaskPricing
	}

	if updated == 0 {
		return 0, nil, nil
	}
	if err := billing_setting.ValidateTaskPricingMap(taskPricing); err != nil {
		return 0, nil, fmt.Errorf("validate merged AIPDD task pricing: %w", err)
	}

	billingModeJSON, err := common.Marshal(billingModes)
	if err != nil {
		return 0, nil, fmt.Errorf("serialize AIPDD billing modes: %w", err)
	}
	taskPricingJSON, err := common.Marshal(taskPricing)
	if err != nil {
		return 0, nil, fmt.Errorf("serialize AIPDD task pricing: %w", err)
	}
	updates := map[string]string{
		aipddBillingModeOptionKey: string(billingModeJSON),
		aipddTaskPricingOptionKey: string(taskPricingJSON),
	}
	for key, value := range updates {
		option := Option{Key: key, Value: value}
		if err := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "key"}},
			DoUpdates: clause.AssignmentColumns([]string{"value"}),
		}).Create(&option).Error; err != nil {
			return 0, nil, err
		}
	}
	return updated, updates, nil
}

func loadAIPDDBillingModesTx(tx *gorm.DB) (map[string]string, error) {
	raw, err := loadAIPDDOptionValueTx(tx, aipddBillingModeOptionKey)
	if err != nil {
		return nil, err
	}
	modes := make(map[string]string)
	if strings.TrimSpace(raw) == "" {
		return modes, nil
	}
	if err := common.UnmarshalJsonStr(raw, &modes); err != nil {
		return nil, fmt.Errorf("parse %s: %w", aipddBillingModeOptionKey, err)
	}
	return modes, nil
}

func loadAIPDDTaskPricingTx(tx *gorm.DB) (map[string]billing_setting.TaskPricingConfig, error) {
	raw, err := loadAIPDDOptionValueTx(tx, aipddTaskPricingOptionKey)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(raw) == "" {
		return make(map[string]billing_setting.TaskPricingConfig), nil
	}
	configs, err := billing_setting.ParseTaskPricingMapJSON(raw)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", aipddTaskPricingOptionKey, err)
	}
	return configs, nil
}

func loadAIPDDOptionValueTx(tx *gorm.DB, key string) (string, error) {
	var option Option
	err := tx.Where("key = ?", key).First(&option).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return option.Value, nil
}

func isTokenMarketDurationCapability(capability aipddcatalog.AtomicCapability) bool {
	return strings.EqualFold(strings.TrimSpace(capability.AdapterCode), "token_market_media") &&
		strings.EqualFold(strings.TrimSpace(capability.Pricing.PricingModel), "per_second")
}

func tokenMarketCapabilityAvailable(capability aipddcatalog.AtomicCapability) bool {
	return capability.Pricing.Enabled &&
		(capability.Available == nil || *capability.Available) &&
		strings.EqualFold(strings.TrimSpace(capability.Pricing.Currency), "awcoin") &&
		strings.EqualFold(strings.TrimSpace(capability.Pricing.PricingBasis), "display")
}

func tokenMarketTaskPricingConfig(
	capability aipddcatalog.AtomicCapability,
	rmbPerAWCoin float64,
	rmbPerUSD float64,
) (billing_setting.TaskPricingConfig, error) {
	config := billing_setting.TaskPricingConfig{
		Unit:         billing_setting.TaskPricingUnitSecond,
		ByResolution: make(map[string]billing_setting.TaskPricingTier, len(capability.Pricing.ByResolution)),
	}
	for rawResolution, price := range capability.Pricing.ByResolution {
		resolution, err := billing_setting.NormalizeTaskPricingResolution(rawResolution)
		if err != nil {
			return billing_setting.TaskPricingConfig{}, err
		}
		if _, exists := config.ByResolution[resolution]; exists {
			return billing_setting.TaskPricingConfig{}, fmt.Errorf("duplicate resolution %q after normalization", resolution)
		}
		noReferenceUSD, err := displayAWCoinToUSD(price.DisplayAmountAWCoinPerSecond, rmbPerAWCoin, rmbPerUSD)
		if err != nil {
			return billing_setting.TaskPricingConfig{}, fmt.Errorf("resolution %q display price: %w", resolution, err)
		}
		tier := billing_setting.TaskPricingTier{
			NoReferenceVideoUnitPrice: noReferenceUSD,
			ReferenceVideoPolicy:      billing_setting.ReferenceVideoPolicySame,
		}
		if price.DisplayVideoInputAWCoinPerSecond != nil {
			referenceUSD, err := displayAWCoinToUSD(price.DisplayVideoInputAWCoinPerSecond, rmbPerAWCoin, rmbPerUSD)
			if err != nil {
				return billing_setting.TaskPricingConfig{}, fmt.Errorf("resolution %q video-input display price: %w", resolution, err)
			}
			if !nearlySamePrice(noReferenceUSD, referenceUSD) {
				tier.ReferenceVideoPolicy = billing_setting.ReferenceVideoPolicyCustom
				tier.ReferenceVideoUnitPrice = referenceUSD
			}
		}
		config.ByResolution[resolution] = tier
	}
	if err := billing_setting.ValidateTaskPricingConfig(config); err != nil {
		return billing_setting.TaskPricingConfig{}, err
	}
	return config, nil
}

func displayAWCoinToUSD(value *float64, rmbPerAWCoin, rmbPerUSD float64) (float64, error) {
	if value == nil || !finitePositive(*value) {
		return 0, fmt.Errorf("AWCoin price must be finite and greater than 0")
	}
	price := *value * rmbPerAWCoin / rmbPerUSD
	if !finitePositive(price) {
		return 0, fmt.Errorf("converted USD price must be finite and greater than 0")
	}
	return price, nil
}

func finitePositive(value float64) bool {
	return value > 0 && !math.IsNaN(value) && !math.IsInf(value, 0)
}

func nearlySamePrice(left, right float64) bool {
	scale := math.Max(1, math.Max(math.Abs(left), math.Abs(right)))
	return math.Abs(left-right) <= 1e-12*scale
}
