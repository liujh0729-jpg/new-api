package aipddcatalog

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestV1ModelsListHiddenNamesUsesAvailableAndPricingEnabled(t *testing.T) {
	catalog := AtomicCatalog{
		Capabilities: []AtomicCapability{
			{
				ID: "enabled-task", Available: true,
				Pricing: AtomicPricing{Enabled: true},
			},
			{
				ID: "unavailable-task", Available: false,
				Pricing: AtomicPricing{Enabled: true},
			},
			{
				ID: "pricing-disabled-task", Available: true,
				Pricing: AtomicPricing{Enabled: false},
			},
		},
		Models: []AtomicModel{
			{
				ID: "enabled-llm", Available: true,
				Pricing: AtomicPricing{Enabled: true},
			},
			{
				ID: "unavailable-llm", Available: false,
				Pricing: AtomicPricing{Enabled: true},
			},
			{
				ID: "pricing-disabled-llm", Available: true,
				Pricing: AtomicPricing{Enabled: false},
			},
		},
	}

	require.Equal(t, []string{
		"pricing-disabled-llm",
		"pricing-disabled-task",
		"unavailable-llm",
		"unavailable-task",
	}, catalog.V1ModelsListHiddenNames())
}

func TestV1ModelsListHiddenRuntimeStateDoesNotHideWhenUnset(t *testing.T) {
	t.Cleanup(ResetV1ModelsListHidden)
	ResetV1ModelsListHidden()

	require.False(t, HasV1ModelsListHiddenState())
	require.False(t, IsHiddenFromV1ModelsList("unavailable-task"))

	SetV1ModelsListHidden([]string{"unavailable-task", "pricing-disabled-llm"})
	require.True(t, HasV1ModelsListHiddenState())
	require.True(t, IsHiddenFromV1ModelsList("unavailable-task"))
	require.True(t, IsHiddenFromV1ModelsList("pricing-disabled-llm"))
	require.False(t, IsHiddenFromV1ModelsList("enabled-task"))
	require.False(t, IsHiddenFromV1ModelsList("gpt-4o"))

	ResetV1ModelsListHidden()
	require.False(t, HasV1ModelsListHiddenState())
	require.False(t, IsHiddenFromV1ModelsList("unavailable-task"))
}

func TestV1ModelsListHiddenNamesIgnoresModelsAbsentFromCatalog(t *testing.T) {
	catalog := AtomicCatalog{
		Capabilities: []AtomicCapability{{
			ID: "only-known", Available: false,
			Pricing: AtomicPricing{Enabled: false},
		}},
	}
	hidden := catalog.V1ModelsListHiddenNames()
	require.Equal(t, []string{"only-known"}, hidden)
	require.NotContains(t, hidden, "legacy-missing-from-catalog")
}
