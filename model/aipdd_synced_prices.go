package model

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// aipddSyncedPricesOptionKey remembers, for every price row a catalog syncer
// wrote, the exact value it wrote. Each importer compares the stored row against
// that record before touching it: a row that differs from the record was changed
// by an administrator, so the human price governs and the syncer leaves the model
// completely alone. This is what makes "default at 1.1x, an edited price wins"
// hold without the operator having to remember to pin anything.
//
// The pin list stays a separate, stronger lever: it freezes a price the syncer
// itself wrote, so it survives an upstream reprice.
const aipddSyncedPricesOptionKey = "AIPDDSyncedPrices"

// The record is keyed by pricing kind plus model name, because one model is owned
// by exactly one importer (per-token expression, fixed per-call price, or
// per-second task pricing) and each kind stores its price in a different option.
const (
	aipddSyncedKindExpr  = "expr"
	aipddSyncedKindPrice = "price"
	aipddSyncedKindTask  = "task"
)

// aipddSyncedStateSeparator joins the billing mode and the price value into one
// fingerprint. It is a control character so it can never appear in a model name,
// billing expression or JSON price.
const aipddSyncedStateSeparator = "\x1f"

type aipddSyncedPriceAction int

const (
	// aipddPriceWrite means the syncer owns the row (or the row does not exist),
	// so the catalog price must be provisioned.
	aipddPriceWrite aipddSyncedPriceAction = iota
	// aipddPriceUnchanged means the stored price already equals the catalog price.
	aipddPriceUnchanged
	// aipddPriceKeepManual means the stored price is not what this syncer last
	// wrote. An administrator edited it, so nothing about the model is touched.
	aipddPriceKeepManual
	// aipddPriceAdopted means a price row exists that predates the record. It is
	// recorded as ours and left untouched for this pass, so a hand-set price is
	// never clobbered by the upgrade itself; the following pass resumes following
	// the catalog from that baseline.
	aipddPriceAdopted
)

type aipddSyncedPriceSet struct {
	values  map[string]string
	changed bool
}

func aipddSyncedPriceKey(kind, modelName string) string {
	return kind + "|" + strings.TrimSpace(modelName)
}

// aipddSyncedStateText fingerprints the two stored rows one importer owns: the
// model's billing mode and its price. An edit to either one counts as a human
// decision about how this model is billed.
func aipddSyncedStateText(mode string, price string) string {
	return mode + aipddSyncedStateSeparator + price
}

func loadAIPDDSyncedPricesTx(tx *gorm.DB) (*aipddSyncedPriceSet, error) {
	raw, err := loadAIPDDOptionValueTx(tx, aipddSyncedPricesOptionKey)
	if err != nil {
		return nil, err
	}
	records, err := parseAIPDDSyncedPrices(raw)
	if err != nil {
		return nil, err
	}
	return &aipddSyncedPriceSet{values: records}, nil
}

func parseAIPDDSyncedPrices(raw string) (map[string]string, error) {
	records := make(map[string]string)
	if strings.TrimSpace(raw) == "" {
		return records, nil
	}
	if err := common.UnmarshalJsonStr(raw, &records); err != nil {
		return nil, fmt.Errorf("parse %s: %w", aipddSyncedPricesOptionKey, err)
	}
	return records, nil
}

// decide returns what the caller must do with one model. current is the stored
// fingerprint, exists whether the model has any local pricing at all, and desired
// the fingerprint of the state this syncer is about to write.
func (s *aipddSyncedPriceSet) decide(kind, modelName, current string, exists bool, desired string) aipddSyncedPriceAction {
	key := aipddSyncedPriceKey(kind, modelName)
	recorded, hadRecord := s.values[key]
	if !exists {
		// No local price: provisioning one is what keeps a routable model off the
		// neutral default ratio, even if a stale record survives from an earlier
		// life of this model.
		s.set(key, desired)
		return aipddPriceWrite
	}
	if !hadRecord {
		s.set(key, current)
		return aipddPriceAdopted
	}
	if recorded != current {
		return aipddPriceKeepManual
	}
	if current == desired {
		return aipddPriceUnchanged
	}
	s.set(key, desired)
	return aipddPriceWrite
}

func (s *aipddSyncedPriceSet) set(key string, value string) {
	if s.values[key] == value {
		return
	}
	s.values[key] = value
	s.changed = true
}

// writeTx persists the record inside the caller's transaction when it changed and
// returns the serialized value so the caller can hand it back for the in-memory
// option map refresh.
func (s *aipddSyncedPriceSet) writeTx(tx *gorm.DB) (string, bool, error) {
	if !s.changed {
		return "", false, nil
	}
	value, err := s.marshal()
	if err != nil {
		return "", false, err
	}
	if err := upsertAIPDDOptionTx(tx, aipddSyncedPricesOptionKey, value); err != nil {
		return "", false, err
	}
	return value, true, nil
}

func (s *aipddSyncedPriceSet) marshal() (string, error) {
	value, err := common.Marshal(s.values)
	if err != nil {
		return "", fmt.Errorf("serialize %s: %w", aipddSyncedPricesOptionKey, err)
	}
	return string(value), nil
}

// upsertAIPDDOptionTx writes one option row, creating it when absent. Callers that
// must also refresh the in-memory option map return the value from the sync result.
func upsertAIPDDOptionTx(tx *gorm.DB, key string, value string) error {
	option := Option{Key: key, Value: value}
	return tx.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{"value"}),
	}).Create(&option).Error
}

// aipddSyncedPriceText serializes a fixed per-call price. FormatFloat with -1
// precision round-trips exactly, so re-reading the stored price produces the same
// text this syncer recorded.
func aipddSyncedPriceText(priceUSD float64) string {
	return strconv.FormatFloat(priceUSD, 'f', -1, 64)
}

func aipddSyncedTaskText(config billing_setting.TaskPricingConfig) (string, error) {
	value, err := common.Marshal(config)
	if err != nil {
		return "", fmt.Errorf("serialize AIPDD task pricing fingerprint: %w", err)
	}
	return string(value), nil
}
