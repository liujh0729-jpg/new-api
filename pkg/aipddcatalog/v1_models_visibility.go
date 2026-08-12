package aipddcatalog

import (
	"sort"
	"strings"
	"sync"
)

// Runtime state used only by GET /v1/models list filtering.
// Models in this set are known AIPDD catalog entries that are not listable
// because they are explicitly unavailable or pricing-disabled. Absence from
// the set must never be treated as "disabled", so unknown / stale / non-AIPDD
// models keep their existing list behavior when catalog state is missing.
var (
	v1ModelsListHiddenMu    sync.RWMutex
	v1ModelsListHiddenNames map[string]struct{}
	v1ModelsListHiddenReady bool
)

// V1ModelsListHiddenNames returns AIPDD catalog model IDs that must not appear
// in GET /v1/models. A model is hidden only when every catalog entry for that
// ID is explicitly unavailable (available=false) or pricing-disabled.
// A missing/null available flag is not a denylist signal: Java ComfyUI
// entries historically omitted the field. Models not present in the catalog
// are omitted from the result.
func (catalog AtomicCatalog) V1ModelsListHiddenNames() []string {
	enabledByName := make(map[string]bool)
	for _, capability := range catalog.Capabilities {
		name := strings.TrimSpace(capability.ID)
		if name == "" || excludedAIPDDCatalogText(capability.AdapterCode, capability.Code, capability.ID, capability.Name) {
			continue
		}
		enabled := catalogAvailable(capability.Available) && capability.Pricing.Enabled
		if prev, ok := enabledByName[name]; ok {
			enabledByName[name] = prev || enabled
		} else {
			enabledByName[name] = enabled
		}
	}
	for _, model := range catalog.Models {
		name := strings.TrimSpace(model.ID)
		if name == "" || excludedAIPDDCatalogText(model.ID, model.Name) {
			continue
		}
		available := model.Available
		enabled := catalogAvailable(&available) && model.Pricing.Enabled
		if prev, ok := enabledByName[name]; ok {
			enabledByName[name] = prev || enabled
		} else {
			enabledByName[name] = enabled
		}
	}

	hidden := make([]string, 0)
	for name, enabled := range enabledByName {
		if !enabled {
			hidden = append(hidden, name)
		}
	}
	sort.Strings(hidden)
	return hidden
}

func catalogAvailable(available *bool) bool {
	return available == nil || *available
}

// BoolPtr is a helper for optional catalog availability flags.
func BoolPtr(v bool) *bool {
	return &v
}

// SetV1ModelsListHidden replaces the process-wide denylist used by /v1/models.
// Calling this marks catalog list-visibility state as ready.
func SetV1ModelsListHidden(names []string) {
	next := make(map[string]struct{}, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		next[name] = struct{}{}
	}
	v1ModelsListHiddenMu.Lock()
	defer v1ModelsListHiddenMu.Unlock()
	v1ModelsListHiddenNames = next
	v1ModelsListHiddenReady = true
}

// ResetV1ModelsListHidden clears list-visibility state so /v1/models keeps its
// historical unfiltered behavior until a catalog is activated again.
func ResetV1ModelsListHidden() {
	v1ModelsListHiddenMu.Lock()
	defer v1ModelsListHiddenMu.Unlock()
	v1ModelsListHiddenNames = nil
	v1ModelsListHiddenReady = false
}

// HasV1ModelsListHiddenState reports whether a successful catalog activation
// has populated list-visibility state. When false, callers must not filter.
func HasV1ModelsListHiddenState() bool {
	v1ModelsListHiddenMu.RLock()
	defer v1ModelsListHiddenMu.RUnlock()
	return v1ModelsListHiddenReady
}

// IsHiddenFromV1ModelsList reports whether modelName is a known disabled AIPDD
// catalog model that should be omitted from GET /v1/models. Returns false when
// catalog list-visibility state has not been activated.
func IsHiddenFromV1ModelsList(modelName string) bool {
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		return false
	}
	v1ModelsListHiddenMu.RLock()
	defer v1ModelsListHiddenMu.RUnlock()
	if !v1ModelsListHiddenReady {
		return false
	}
	_, ok := v1ModelsListHiddenNames[modelName]
	return ok
}
