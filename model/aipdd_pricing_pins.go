package model

import (
	"fmt"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// aipddPinnedModelsOptionKey stores the model names whose price an administrator
// has pinned. The catalog price importers neither write nor delete a pinned
// model's price rows, so a hand-set retail price survives every sync. Without
// this, the 5-minute catalog reconciliation would revert any price edited on the
// pricing page back to the marked-up wholesale price.
const aipddPinnedModelsOptionKey = "AIPDDPinnedModels"

// aipddPinnedModelSet is a defined type so the importers can ask it whether a
// model is pinned without repeating the trim/nil handling at every call site.
type aipddPinnedModelSet map[string]bool

func (pins aipddPinnedModelSet) contains(modelName string) bool {
	if len(pins) == 0 {
		return false
	}
	return pins[strings.TrimSpace(modelName)]
}

func loadAIPDDPinnedModelsTx(tx *gorm.DB) (aipddPinnedModelSet, error) {
	raw, err := loadAIPDDOptionValueTx(tx, aipddPinnedModelsOptionKey)
	if err != nil {
		return nil, err
	}
	return parseAIPDDPinnedModels(raw)
}

func parseAIPDDPinnedModels(raw string) (aipddPinnedModelSet, error) {
	pins := make(aipddPinnedModelSet)
	if strings.TrimSpace(raw) == "" {
		return pins, nil
	}
	var names []string
	if err := common.UnmarshalJsonStr(raw, &names); err != nil {
		return nil, fmt.Errorf("parse %s: %w", aipddPinnedModelsOptionKey, err)
	}
	for _, name := range names {
		if trimmed := strings.TrimSpace(name); trimmed != "" {
			pins[trimmed] = true
		}
	}
	return pins, nil
}

// AIPDDPinnedModelSet returns the models the administrator has pinned. Callers
// that price many models in one pass should read the set once instead of asking
// per model.
func AIPDDPinnedModelSet() (map[string]bool, error) {
	if DB == nil {
		return map[string]bool{}, nil
	}
	pins, err := loadAIPDDPinnedModelsTx(DB)
	if err != nil {
		return nil, err
	}
	return pins, nil
}

// GetAIPDDPinnedModels lists the pinned model names, sorted.
func GetAIPDDPinnedModels() ([]string, error) {
	pins, err := loadAIPDDPinnedModels()
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(pins))
	for name := range pins {
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

// SetAIPDDPinnedModel pins or unpins one model's price and persists the list
// through UpdateOption so the in-memory option map stays in step with the table.
func SetAIPDDPinnedModel(modelName string, pinned bool) error {
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		return fmt.Errorf("model name is required")
	}
	pins, err := loadAIPDDPinnedModels()
	if err != nil {
		return err
	}
	if pinned {
		pins[modelName] = true
	} else {
		delete(pins, modelName)
	}
	return UpdateOption(aipddPinnedModelsOptionKey, marshalAIPDDPinnedModels(pins))
}

func loadAIPDDPinnedModels() (aipddPinnedModelSet, error) {
	if DB == nil {
		return make(aipddPinnedModelSet), nil
	}
	return loadAIPDDPinnedModelsTx(DB)
}

func marshalAIPDDPinnedModels(pins aipddPinnedModelSet) string {
	names := make([]string, 0, len(pins))
	for name := range pins {
		names = append(names, name)
	}
	sort.Strings(names)
	value, err := common.Marshal(names)
	if err != nil {
		common.SysError("failed to serialize AIPDD pinned models: " + err.Error())
		return "[]"
	}
	return string(value)
}
