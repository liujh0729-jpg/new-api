package aipddcatalog

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExplicitFreeModelsRuntimeStateUsesOnlyAvailableEnabledEntries(t *testing.T) {
	t.Cleanup(ResetExplicitFreeModels)
	catalog := AtomicCatalog{Models: []AtomicModel{
		{ID: "free-ready", Available: true, Pricing: AtomicPricing{Enabled: true, Free: true}},
		{ID: "free-disabled", Available: true, Pricing: AtomicPricing{Enabled: false, Free: true}},
		{ID: "free-unavailable", Available: false, Pricing: AtomicPricing{Enabled: true, Free: true}},
		{ID: "paid-model", Available: true, Pricing: AtomicPricing{Enabled: true}},
	}}

	require.Equal(t, []string{"free-ready"}, catalog.ExplicitFreeModelNames())
	SetExplicitFreeModels(catalog.ExplicitFreeModelNames())
	require.True(t, IsExplicitFreeModel("free-ready"))
	require.False(t, IsExplicitFreeModel("free-disabled"))
	require.False(t, IsExplicitFreeModel("paid-model"))
}
