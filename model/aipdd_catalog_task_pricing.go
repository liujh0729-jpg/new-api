package model

import (
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/aipddcatalog"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"gorm.io/gorm"
)

const (
	aipddModelPriceOptionKey  = "ModelPrice"
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
	pinned, err := loadAIPDDPinnedModelsTx(tx)
	if err != nil {
		return 0, nil, err
	}
	synced, err := loadAIPDDSyncedPricesTx(tx)
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
		if pinned.contains(modelName) {
			continue
		}

		if !tokenMarketCapabilityPriceable(capability) {
			// The capability is still listed and routable but publishes no display
			// price. Its local config stays: deleting it would drop the model back
			// to the neutral ratio, below AIPDD's wholesale cost.
			if _, ok := taskPricing[modelName]; ok {
				logAIPDDPricingSkipped(catalog.Revision, modelName,
					fmt.Errorf("capability publishes no usable display price; keeping the local task pricing"))
			}
			continue
		}

		config, err := tokenMarketTaskPricingConfig(capability, rmbPerAWCoin, rmbPerUSD)
		if err != nil {
			logAIPDDPricingSkipped(catalog.Revision, modelName, err)
			continue
		}
		write, err := takeAIPDDTaskPricingTx(synced, taskPricing, billingModes, modelName, config)
		if err != nil {
			return 0, nil, err
		}
		if !write {
			continue
		}
		updated++
		taskPricing[modelName] = config
		billingModes[modelName] = billing_setting.BillingModeTaskPricing
	}

	updates := make(map[string]string, 3)
	if updated > 0 {
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
		updates[aipddBillingModeOptionKey] = string(billingModeJSON)
		updates[aipddTaskPricingOptionKey] = string(taskPricingJSON)
	}
	recordValue, recordChanged, err := synced.writeTx(tx)
	if err != nil {
		return 0, nil, err
	}
	if recordChanged {
		updates[aipddSyncedPricesOptionKey] = recordValue
	}
	if len(updates) == 0 {
		return 0, nil, nil
	}
	for key, value := range updates {
		if err := upsertAIPDDOptionTx(tx, key, value); err != nil {
			return 0, nil, err
		}
	}
	return updated, updates, nil
}

// takeAIPDDTaskPricing applies the "an edited price wins" rule to one duration
// model and reports whether the caller must write config for it.
func takeAIPDDTaskPricingTx(
	synced *aipddSyncedPriceSet,
	taskPricing map[string]billing_setting.TaskPricingConfig,
	billingModes map[string]string,
	modelName string,
	config billing_setting.TaskPricingConfig,
) (bool, error) {
	desired, err := aipddSyncedTaskText(config)
	if err != nil {
		return false, err
	}
	current, exists := taskPricing[modelName]
	currentText := ""
	if exists {
		currentText, err = aipddSyncedTaskText(current)
		if err != nil {
			return false, err
		}
	}
	_, hasMode := billingModes[modelName]
	action := synced.decide(aipddSyncedKindTask, modelName,
		aipddSyncedStateText(billingModes[modelName], currentText), exists || hasMode,
		aipddSyncedStateText(billing_setting.BillingModeTaskPricing, desired))
	return action == aipddPriceWrite, nil
}

// syncAIPDDTokenMarketFixedPricingTx imports display-basis per-call prices for
// Token Market media models. The catalog price is denominated in AWCoin but is
// a RMB retail price, so persist the USD-equivalent value using this NewAPI
// instance's exchange rate. This keeps the public CNY price exactly aligned
// with AIPDD instead of drifting with the catalog's reference USD rate.
func syncAIPDDTokenMarketFixedPricingTx(
	tx *gorm.DB,
	catalog aipddcatalog.AtomicCatalog,
) (int, map[string]string, error) {
	modelPrices, err := loadAIPDDModelPricesTx(tx)
	if err != nil {
		return 0, nil, err
	}
	billingModes, err := loadAIPDDBillingModesTx(tx)
	if err != nil {
		return 0, nil, err
	}
	taskPricing, err := loadAIPDDTaskPricingTx(tx)
	if err != nil {
		return 0, nil, err
	}
	pinned, err := loadAIPDDPinnedModelsTx(tx)
	if err != nil {
		return 0, nil, err
	}
	synced, err := loadAIPDDSyncedPricesTx(tx)
	if err != nil {
		return 0, nil, err
	}

	rmbPerAWCoin := catalog.AWCoinRate.RMBPerAWCoin
	rmbPerUSD := operation_setting.USDExchangeRate
	updated := 0
	modelPricesChanged := false
	billingModesChanged := false
	taskPricingChanged := false
	for _, capability := range catalog.Capabilities {
		if !isTokenMarketPerCallCapability(capability) {
			continue
		}
		modelName := strings.TrimSpace(capability.ID)
		if modelName == "" {
			continue
		}
		if pinned.contains(modelName) {
			continue
		}

		modelChanged := false
		if !tokenMarketFixedCapabilityPriceable(capability) {
			// Listed and routable, but the catalog publishes no usable per-call
			// price. The stored price stays: without it the model falls back to the
			// neutral ratio, below AIPDD's wholesale cost.
			if _, ok := modelPrices[modelName]; ok {
				logAIPDDPricingSkipped(catalog.Revision, modelName,
					fmt.Errorf("capability publishes no usable per-call price; keeping the local price"))
			}
			continue
		}
		if !finitePositive(rmbPerAWCoin) {
			return 0, nil, fmt.Errorf("AIPDD catalog rmbPerAwcoin must be finite and greater than 0")
		}
		if !finitePositive(rmbPerUSD) {
			return 0, nil, fmt.Errorf("New API USDExchangeRate must be finite and greater than 0")
		}
		awcoin := aipddcatalog.TaskAWCoinPrice(capability.Pricing)
		priceUSD, err := displayAWCoinToUSD(&awcoin, rmbPerAWCoin, rmbPerUSD)
		if err != nil {
			logAIPDDPricingSkipped(catalog.Revision, modelName, err)
			continue
		}

		current, hasPrice := modelPrices[modelName]
		_, hasMode := billingModes[modelName]
		_, hasTask := taskPricing[modelName]
		switch synced.decide(aipddSyncedKindPrice, modelName,
			aipddSyncedStateText(billingModes[modelName], aipddSyncedPriceText(current)),
			hasPrice || hasMode || hasTask,
			aipddSyncedStateText("", aipddSyncedPriceText(priceUSD))) {
		case aipddPriceWrite:
			// The catalog governs this model's price.
		case aipddPriceUnchanged:
			// Already priced as the catalog wants; the stale-row cleanup below runs.
		case aipddPriceKeepManual, aipddPriceAdopted:
			// An administrator owns this price, so neither it nor the billing rows
			// that used to accompany it are touched.
			continue
		}

		if !hasPrice || aipddSyncedPriceText(current) != aipddSyncedPriceText(priceUSD) {
			modelPrices[modelName] = priceUSD
			modelPricesChanged = true
			modelChanged = true
		}

		// A model may move from duration billing to a fixed per-call price. Clear
		// only the stale task-pricing mode owned by this catalog; unrelated local
		// billing modes and prices remain untouched.
		if billingModes[modelName] == billing_setting.BillingModeTaskPricing {
			delete(billingModes, modelName)
			billingModesChanged = true
			modelChanged = true
		}
		if _, ok := taskPricing[modelName]; ok {
			delete(taskPricing, modelName)
			taskPricingChanged = true
			modelChanged = true
		}
		if modelChanged {
			updated++
		}
	}

	updates := make(map[string]string, 4)
	if modelPricesChanged {
		value, err := common.Marshal(modelPrices)
		if err != nil {
			return 0, nil, fmt.Errorf("serialize AIPDD fixed model prices: %w", err)
		}
		updates[aipddModelPriceOptionKey] = string(value)
	}
	if billingModesChanged {
		value, err := common.Marshal(billingModes)
		if err != nil {
			return 0, nil, fmt.Errorf("serialize AIPDD billing modes: %w", err)
		}
		updates[aipddBillingModeOptionKey] = string(value)
	}
	if taskPricingChanged {
		if err := billing_setting.ValidateTaskPricingMap(taskPricing); err != nil {
			return 0, nil, fmt.Errorf("validate merged AIPDD task pricing: %w", err)
		}
		value, err := common.Marshal(taskPricing)
		if err != nil {
			return 0, nil, fmt.Errorf("serialize AIPDD task pricing: %w", err)
		}
		updates[aipddTaskPricingOptionKey] = string(value)
	}
	recordValue, recordChanged, err := synced.writeTx(tx)
	if err != nil {
		return 0, nil, err
	}
	if recordChanged {
		updates[aipddSyncedPricesOptionKey] = recordValue
	}
	if len(updates) == 0 {
		return 0, nil, nil
	}
	for key, value := range updates {
		if err := upsertAIPDDOptionTx(tx, key, value); err != nil {
			return 0, nil, err
		}
	}
	return updated, updates, nil
}

// syncAIPDDSynchronousPerSecondPricingTx imports the per-second retail price of
// the media models the gateway calls synchronously and the upstream charges per
// unit of time (ASR). A per-call price would bill one second for a ten-minute
// recording, so these models are imported as unit=second task pricing without
// resolution tiers. The relay multiplies that base price by the duration the
// upstream reports for the finished request.
func syncAIPDDSynchronousPerSecondPricingTx(
	tx *gorm.DB,
	catalog aipddcatalog.AtomicCatalog,
) (int, map[string]string, error) {
	return syncAIPDDPerSecondTaskPricingTx(tx, catalog, catalog.SynchronousPerSecondPrices())
}

// syncAIPDDPerSecondTaskPricingTx provisions unit=second task pricing for models
// the upstream charges per processed or generated second. One implementation
// serves both the synchronous and the async endpoint families so the retail price,
// the retail margin and the "an edited price wins" rule cannot drift apart.
func syncAIPDDPerSecondTaskPricingTx(
	tx *gorm.DB,
	catalog aipddcatalog.AtomicCatalog,
	awcoinPerSecondByModel map[string]float64,
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
	pinned, err := loadAIPDDPinnedModelsTx(tx)
	if err != nil {
		return 0, nil, err
	}
	synced, err := loadAIPDDSyncedPricesTx(tx)
	if err != nil {
		return 0, nil, err
	}

	updated := 0
	for modelName, awcoinPerSecond := range awcoinPerSecondByModel {
		if pinned.contains(modelName) {
			continue
		}
		unitPriceUSD := aipddRetailUSD(awcoinPerSecond, rmbPerAWCoin, rmbPerUSD)
		if !finitePositive(unitPriceUSD) {
			logAIPDDPricingSkipped(catalog.Revision, modelName,
				fmt.Errorf("%g awcoin per second is not a usable retail price", awcoinPerSecond))
			continue
		}
		config := billing_setting.TaskPricingConfig{
			Unit:                      billing_setting.TaskPricingUnitSecond,
			NoReferenceVideoUnitPrice: unitPriceUSD,
			ReferenceVideoPolicy:      billing_setting.ReferenceVideoPolicySame,
		}
		if err := billing_setting.ValidateTaskPricingConfig(config); err != nil {
			logAIPDDPricingSkipped(catalog.Revision, modelName, err)
			continue
		}
		write, err := takeAIPDDTaskPricingTx(synced, taskPricing, billingModes, modelName, config)
		if err != nil {
			return 0, nil, err
		}
		if !write {
			continue
		}
		updated++
		taskPricing[modelName] = config
		billingModes[modelName] = billing_setting.BillingModeTaskPricing
	}

	updates := make(map[string]string, 3)
	if updated > 0 {
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
		updates[aipddBillingModeOptionKey] = string(billingModeJSON)
		updates[aipddTaskPricingOptionKey] = string(taskPricingJSON)
	}
	recordValue, recordChanged, err := synced.writeTx(tx)
	if err != nil {
		return 0, nil, err
	}
	if recordChanged {
		updates[aipddSyncedPricesOptionKey] = recordValue
	}
	if len(updates) == 0 {
		return 0, nil, nil
	}
	for key, value := range updates {
		if err := upsertAIPDDOptionTx(tx, key, value); err != nil {
			return 0, nil, err
		}
	}
	return updated, updates, nil
}

// syncAIPDDAsyncPerSecondPricingTx imports the per-second retail price of async
// task models (submitted via /shared-tasks/tasks or similar) where the upstream
// charges per unit of time. Without this importer, such models would be callable
// but unpriced once the upstream marks them available.
func syncAIPDDAsyncPerSecondPricingTx(
	tx *gorm.DB,
	catalog aipddcatalog.AtomicCatalog,
) (int, map[string]string, error) {
	return syncAIPDDPerSecondTaskPricingTx(tx, catalog, catalog.AsyncPerSecondPrices())
}

func loadAIPDDModelPricesTx(tx *gorm.DB) (map[string]float64, error) {
	raw, err := loadAIPDDOptionValueTx(tx, aipddModelPriceOptionKey)
	if err != nil {
		return nil, err
	}
	prices := make(map[string]float64)
	if strings.TrimSpace(raw) == "" {
		return prices, nil
	}
	if err := common.UnmarshalJsonStr(raw, &prices); err != nil {
		return nil, fmt.Errorf("parse %s: %w", aipddModelPriceOptionKey, err)
	}
	return prices, nil
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

func isTokenMarketPerCallCapability(capability aipddcatalog.AtomicCapability) bool {
	// token_market_shared submits through /shared-tasks/tasks but carries the
	// same per-call display prices, and its pricing check tolerates an
	// empty pricingBasis. Restricting this import to token_market_media left a
	// shared-adapter model (image edit) with no fixed price, so it fell back to
	// token ratio billing and was undercharged.
	adapter := strings.ToLower(strings.TrimSpace(capability.AdapterCode))
	if adapter != "token_market_media" && adapter != "token_market_shared" {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(capability.Pricing.PricingModel), "per_call")
}

// tokenMarketCapabilityPriceable and tokenMarketFixedCapabilityPriceable decide
// whether a catalog capability carries a wholesale price to resell. Upstream's
// availability flag is not consulted: an unavailable capability is still listed on
// the managed channel and stays routable, and a routable model without a retail
// price falls back to the default ratio, below AIPDD's wholesale cost.
func tokenMarketCapabilityPriceable(capability aipddcatalog.AtomicCapability) bool {
	return capability.Pricing.Enabled &&
		strings.EqualFold(strings.TrimSpace(capability.Pricing.Currency), "awcoin") &&
		strings.EqualFold(strings.TrimSpace(capability.Pricing.PricingBasis), "display")
}

func tokenMarketFixedCapabilityPriceable(capability aipddcatalog.AtomicCapability) bool {
	basis := strings.TrimSpace(capability.Pricing.PricingBasis)
	return capability.Pricing.Enabled &&
		strings.EqualFold(strings.TrimSpace(capability.Pricing.Currency), "awcoin") &&
		(basis == "" || strings.EqualFold(basis, "display"))
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
	price := aipddRetailUSD(*value, rmbPerAWCoin, rmbPerUSD)
	if !finitePositive(price) {
		return 0, fmt.Errorf("converted USD price must be finite and greater than 0")
	}
	return price, nil
}

func finitePositive(value float64) bool {
	return value > 0 && !math.IsNaN(value) && !math.IsInf(value, 0)
}

// finiteNonNegative accepts a free price lane: AIPDD publishes models whose input
// or output tokens cost nothing wholesale, and reselling a free lane at zero is
// correct rather than a missing price.
func finiteNonNegative(value float64) bool {
	return value >= 0 && !math.IsNaN(value) && !math.IsInf(value, 0)
}

func nearlySamePrice(left, right float64) bool {
	scale := math.Max(1, math.Max(math.Abs(left), math.Abs(right)))
	return math.Abs(left-right) <= 1e-12*scale
}
