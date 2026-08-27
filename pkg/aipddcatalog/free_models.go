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
