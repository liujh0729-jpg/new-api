package model

import (
	"testing"

	"github.com/QuantumNous/new-api/pkg/aipddcatalog"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/require"
)

func aipddRetailFloatPtr(value float64) *float64 {
	return &value
}

func TestAIPDDRetailMarkupDefault(t *testing.T) {
	t.Setenv("AIPDD_PRICING_MARKUP", "")
	require.InDelta(t, 1.10, AIPDDRetailMarkup(), 1e-12)
}

func TestAIPDDRetailMarkupEnvironmentOverride(t *testing.T) {
	t.Setenv("AIPDD_PRICING_MARKUP", "1.25")
	require.InDelta(t, 1.25, AIPDDRetailMarkup(), 1e-12)

	// Selling below cost is an operator decision, so a markup under 1 is honoured
	// instead of being corrected back to the default.
	t.Setenv("AIPDD_PRICING_MARKUP", "0.9")
	require.InDelta(t, 0.9, AIPDDRetailMarkup(), 1e-12)
}

func TestAIPDDRetailMarkupFallsBackOnUnusableValues(t *testing.T) {
	for _, raw := range []string{"abc", "0", "-1.1", "NaN", "+Inf"} {
		t.Setenv("AIPDD_PRICING_MARKUP", raw)
		require.InDelta(t, 1.10, AIPDDRetailMarkup(), 1e-12, raw)
	}
}

func TestIsAIPDDRetailTokenModel(t *testing.T) {
	tokenPricing := aipddcatalog.AtomicPricing{
		PricingModel: "per_token", Currency: "awcoin", Enabled: true,
		PromptPerMillion: 10, CompletionPerMillion: 30,
		CacheReadPerMillion: aipddRetailFloatPtr(0), CacheWritePerMillion: aipddRetailFloatPtr(0),
	}

	require.True(t, IsAIPDDRetailTokenModel(aipddcatalog.AtomicModel{ID: "m", Available: true, Pricing: tokenPricing}))

	free := tokenPricing
	free.Free = true
	disabled := tokenPricing
	disabled.Enabled = false
	perCall := tokenPricing
	perCall.PricingModel = "per_call"
	otherCurrency := tokenPricing
	otherCurrency.Currency = "usd"

	for name, pricing := range map[string]aipddcatalog.AtomicPricing{
		"free model":          free,
		"disabled pricing":    disabled,
		"per-call task model": perCall,
		"non-awcoin pricing":  otherCurrency,
	} {
		require.False(t, IsAIPDDRetailTokenModel(aipddcatalog.AtomicModel{ID: "m", Available: true, Pricing: pricing}), name)
	}

	require.False(t,
		IsAIPDDRetailTokenModel(aipddcatalog.AtomicModel{ID: "m", Pricing: tokenPricing}),
		"a catalog model that is not purchasable must not be priced")
}

func TestAIPDDLLMRetailExpr(t *testing.T) {
	originalRate := operation_setting.USDExchangeRate
	operation_setting.USDExchangeRate = 7.3
	t.Cleanup(func() { operation_setting.USDExchangeRate = originalRate })
	t.Setenv("AIPDD_PRICING_MARKUP", "1.1")

	catalog := aipddcatalog.AtomicCatalog{
		AWCoinRate: aipddcatalog.AtomicAWCoinRate{RMBPerAWCoin: 0.073, USDPerAWCoin: 0.0015},
	}

	// 80 awcoin per million prompt tokens at 0.073 RMB per awcoin over 7.3 RMB per
	// USD, plus the 10% retail margin.
	expr, err := AIPDDLLMRetailExpr(catalog, aipddcatalog.AtomicModel{
		ID: "ap-glm-5.2", Available: true,
		Pricing: aipddcatalog.AtomicPricing{
			PricingModel: "per_token", Currency: "awcoin", Enabled: true,
			PromptPerMillion: 80, CompletionPerMillion: 280,
			CacheReadPerMillion: aipddRetailFloatPtr(1), CacheWritePerMillion: aipddRetailFloatPtr(12),
		},
	})
	require.NoError(t, err)
	require.Equal(t, `tier("aipdd", p * 0.8800000000000001 + c * 3.0799999999999996 + cr * 0.011 + cc * 0.132 + cc1h * 0.132)`, expr)

	// A schema v1 catalog publishes no cache prices, so the cache variables stay
	// out and cached tokens keep being billed as ordinary input.
	expr, err = AIPDDLLMRetailExpr(catalog, aipddcatalog.AtomicModel{
		ID: "deepseek-r1:8b", Available: true,
		Pricing: aipddcatalog.AtomicPricing{
			PricingModel: "per_token", Currency: "awcoin", Enabled: true,
			PromptPerMillion: 80, CompletionPerMillion: 280,
		},
	})
	require.NoError(t, err)
	require.Equal(t, `tier("aipdd", p * 0.8800000000000001 + c * 3.0799999999999996)`, expr)

	// Embeddings are sold for input tokens only.
	expr, err = AIPDDLLMRetailExpr(catalog, aipddcatalog.AtomicModel{
		ID: "ap-qwen3-embedding:0.6b", Available: true,
		Pricing: aipddcatalog.AtomicPricing{
			PricingModel: "per_token", Currency: "awcoin", Enabled: true,
			PromptPerMillion: 3,
		},
	})
	require.NoError(t, err)
	require.Equal(t, `tier("aipdd", p * 0.033 + c * 0)`, expr)
}

func TestAIPDDLLMRetailExprRejectsUnusableRates(t *testing.T) {
	originalRate := operation_setting.USDExchangeRate
	t.Cleanup(func() { operation_setting.USDExchangeRate = originalRate })

	modelItem := aipddcatalog.AtomicModel{
		ID: "ap-glm-5.2", Available: true,
		Pricing: aipddcatalog.AtomicPricing{
			PricingModel: "per_token", Currency: "awcoin", Enabled: true,
			PromptPerMillion: 80, CompletionPerMillion: 280,
		},
	}
	catalog := aipddcatalog.AtomicCatalog{AWCoinRate: aipddcatalog.AtomicAWCoinRate{RMBPerAWCoin: 0.073}}

	operation_setting.USDExchangeRate = 0
	_, err := AIPDDLLMRetailExpr(catalog, modelItem)
	require.Error(t, err)

	operation_setting.USDExchangeRate = 7.3
	catalog.AWCoinRate.RMBPerAWCoin = 0
	_, err = AIPDDLLMRetailExpr(catalog, modelItem)
	require.Error(t, err)
}
