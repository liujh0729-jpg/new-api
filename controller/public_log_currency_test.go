package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
)

func TestNormalizePublicLogOtherToCNY(t *testing.T) {
	raw := `{
		"model_price":0.04,
		"unit_price_usd":0.08,
		"sale_usd":0.4,
		"web_search_price":10,
		"nested":{"provider_cost_usd":0.01},
		"usd_exchange_rate":7.3
	}`

	normalized := normalizePublicLogOtherToCNY(raw, 6.9)
	other, err := common.StrToMap(normalized)
	require.NoError(t, err)
	require.Equal(t, "CNY", other["currency"])
	require.Equal(t, 7.3, other["usd_exchange_rate"])
	require.InDelta(t, 0.292, other["model_price"], 1e-12)
	require.InDelta(t, 0.584, other["unit_price_cny"], 1e-12)
	require.InDelta(t, 2.92, other["sale_cny"], 1e-12)
	require.InDelta(t, 73, other["web_search_price"], 1e-12)
	require.NotContains(t, other, "unit_price_usd")
	require.NotContains(t, other, "sale_usd")
	nested, ok := other["nested"].(map[string]any)
	require.True(t, ok)
	require.InDelta(t, 0.073, nested["provider_cost_cny"], 1e-12)
	require.NotContains(t, nested, "provider_cost_usd")
}

func TestBuildPublicTokenLogItemsDoesNotMutateStoredLog(t *testing.T) {
	stored := &model.Log{Other: `{"model_price":0.25,"usd_exchange_rate":7.3}`}

	items := buildPublicTokenLogItems([]*model.Log{stored})

	require.Len(t, items, 1)
	require.Equal(t, "CNY", items[0].Currency)
	require.Equal(t, `{"model_price":0.25,"usd_exchange_rate":7.3}`, stored.Other)
	require.NotEqual(t, stored.Other, items[0].Other)
}

func TestNormalizePublicLogOtherToCNYIsIdempotent(t *testing.T) {
	raw := `{"currency":"CNY","model_price":0.292,"usd_exchange_rate":7.3}`
	require.Equal(t, raw, normalizePublicLogOtherToCNY(raw, 7.3))
}
