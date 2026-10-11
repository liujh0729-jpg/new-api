package model

import (
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/aipddcatalog"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// syncAIPDDLLMTokenPricingTx provisions the default retail price of every
// per-token AIPDD model from the catalog's wholesale table, marked up by
// AIPDDRetailMarkup.
//
// Without this importer the LLM side of the catalog has no local price at all and
// falls back to the neutral ratio, which sells output tokens below what AIPDD
// charges for them. Task models keep their own importers because a per-call or
// per-second price cannot be expressed as a token expression.
func syncAIPDDLLMTokenPricingTx(
	tx *gorm.DB,
	catalog aipddcatalog.AtomicCatalog,
) (int, map[string]string, error) {
	// A missing exchange basis is a deployment problem, not a model problem, so it
	// still fails the sync; the checks mirror the task pricing importers.
	if !finitePositive(catalog.AWCoinRate.RMBPerAWCoin) {
		return 0, nil, fmt.Errorf("AIPDD catalog rmbPerAwcoin must be finite and greater than 0")
	}
	if !finitePositive(operation_setting.USDExchangeRate) {
		return 0, nil, fmt.Errorf("New API USDExchangeRate must be finite and greater than 0")
	}
	billingModes, err := loadAIPDDBillingModesTx(tx)
	if err != nil {
		return 0, nil, err
	}
	billingExprs, err := loadAIPDDBillingExprsTx(tx)
	if err != nil {
		return 0, nil, err
	}
	pinned, err := loadAIPDDPinnedModelsTx(tx)
	if err != nil {
		return 0, nil, err
	}

	updated := 0
	for _, model := range catalog.Models {
		modelName := strings.TrimSpace(model.ID)
		if modelName == "" {
			continue
		}
		if pinned.contains(modelName) {
			continue
		}
		if !IsAIPDDRetailTokenModel(model) {
			// The catalog stopped pricing this model (free, disabled pricing or no
			// longer per-token). Drop only the rows this catalog owns so a
			// manually priced model keeps its administrator-set price.
			changed := false
			if _, ok := billingExprs[modelName]; ok {
				delete(billingExprs, modelName)
				changed = true
			}
			if billingModes[modelName] == billing_setting.BillingModeTieredExpr {
				delete(billingModes, modelName)
				changed = true
			}
			if changed {
				updated++
			}
			continue
		}

		expr, err := AIPDDLLMRetailExpr(catalog, model)
		if err != nil {
			logAIPDDPricingSkipped(catalog.Revision, modelName, err)
			continue
		}
		if err := billing_setting.SmokeTestExpr(expr); err != nil {
			logAIPDDPricingSkipped(catalog.Revision, modelName, err)
			continue
		}
		if billingModes[modelName] != billing_setting.BillingModeTieredExpr || billingExprs[modelName] != expr {
			updated++
		}
		billingModes[modelName] = billing_setting.BillingModeTieredExpr
		billingExprs[modelName] = expr
	}

	if updated == 0 {
		return 0, nil, nil
	}
	billingModeJSON, err := common.Marshal(billingModes)
	if err != nil {
		return 0, nil, fmt.Errorf("serialize AIPDD LLM billing modes: %w", err)
	}
	billingExprJSON, err := common.Marshal(billingExprs)
	if err != nil {
		return 0, nil, fmt.Errorf("serialize AIPDD LLM billing expressions: %w", err)
	}
	updates := map[string]string{
		aipddBillingModeOptionKey: string(billingModeJSON),
		aipddBillingExprOptionKey: string(billingExprJSON),
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

func loadAIPDDBillingExprsTx(tx *gorm.DB) (map[string]string, error) {
	raw, err := loadAIPDDOptionValueTx(tx, aipddBillingExprOptionKey)
	if err != nil {
		return nil, err
	}
	exprs := make(map[string]string)
	if strings.TrimSpace(raw) == "" {
		return exprs, nil
	}
	if err := common.UnmarshalJsonStr(raw, &exprs); err != nil {
		return nil, fmt.Errorf("parse %s: %w", aipddBillingExprOptionKey, err)
	}
	return exprs, nil
}
