package aipddcatalog

import (
	"sort"
	"strings"
	"sync"
)

var (
	explicitFreeModelsMu   sync.RWMutex
	explicitFreeModelNames map[string]struct{}
)

// FilterFreeModels removes LLMs that the AIPDD catalog explicitly declares as
// free. NewAPI deliberately does not mirror these promotional models into its
// managed catalog, channels, abilities, or pricing candidates.
func (catalog *AtomicCatalog) FilterFreeModels() {
	if catalog == nil {
		return
	}
	models := catalog.Models[:0]
	for _, model := range catalog.Models {
		if model.Pricing.Free {
			continue
		}
		models = append(models, model)
	}
	catalog.Models = models
}

// ExplicitFreeModelNames returns available LLM IDs whose catalog pricing is
// deliberately free. Validation guarantees these IDs use the free- prefix and
// every token-price lane is zero.
func (catalog AtomicCatalog) ExplicitFreeModelNames() []string {
	names := make([]string, 0)
	for _, model := range catalog.Models {
		name := strings.TrimSpace(model.ID)
		if name == "" || !model.Available || !model.Pricing.Enabled || !model.Pricing.Free {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// SetExplicitFreeModels atomically replaces the activated catalog's free-model
// set. It does not persist or overwrite an administrator's local pricing.
func SetExplicitFreeModels(names []string) {
	next := make(map[string]struct{}, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name != "" {
			next[name] = struct{}{}
		}
	}
	explicitFreeModelsMu.Lock()
	explicitFreeModelNames = next
	explicitFreeModelsMu.Unlock()
}

// IsExplicitFreeModel reports whether the active, validated AIPDD catalog
// declares modelName as an available free LLM.
func IsExplicitFreeModel(modelName string) bool {
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		return false
	}
	explicitFreeModelsMu.RLock()
	defer explicitFreeModelsMu.RUnlock()
	_, ok := explicitFreeModelNames[modelName]
	return ok
}

func ResetExplicitFreeModels() {
	explicitFreeModelsMu.Lock()
	explicitFreeModelNames = nil
	explicitFreeModelsMu.Unlock()
}

// BenefitDescription is the NewAPI model description for a token-market free
// model, for example "hy3 福利免费版". Paid or malformed IDs return empty.
func BenefitDescription(modelName string) string {
	modelName = strings.TrimSpace(modelName)
	const prefix = "free-"
	if len(modelName) <= len(prefix) || !strings.EqualFold(modelName[:len(prefix)], prefix) {
		return ""
	}
	base := strings.TrimSpace(modelName[len(prefix):])
	if base == "" {
		return ""
	}
	return base + " 福利免费版"
}
