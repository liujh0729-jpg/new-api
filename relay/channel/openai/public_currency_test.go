package openai

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/require"
)

func TestConvertPublicUsageCostsToCNY(t *testing.T) {
	previousRate := operation_setting.USDExchangeRate
	operation_setting.USDExchangeRate = 7.3
	t.Cleanup(func() { operation_setting.USDExchangeRate = previousRate })

	converted := convertPublicUsageCostsToCNY(common.StringToByteSlice(`{
		"id":"chatcmpl-test",
		"usage":{
			"prompt_tokens":10,
			"cost":0.25,
			"cost_details":{"upstream_inference_cost":0.2,"provider_fee_usd":0.01}
		}
	}`))

	var payload map[string]any
	require.NoError(t, common.Unmarshal(converted, &payload))
	usage, ok := payload["usage"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "CNY", usage["currency"])
	require.Equal(t, 7.3, usage["usd_exchange_rate"])
	require.InDelta(t, 1.825, usage["cost"], 1e-12)
	costDetails, ok := usage["cost_details"].(map[string]any)
	require.True(t, ok)
	require.InDelta(t, 1.46, costDetails["upstream_inference_cost"], 1e-12)
	require.InDelta(t, 0.073, costDetails["provider_fee_cny"], 1e-12)
	require.NotContains(t, costDetails, "provider_fee_usd")
}

func TestConvertPublicUsageCostsToCNYLeavesTokenOnlyUsageUnchanged(t *testing.T) {
	raw := common.StringToByteSlice(`{"usage":{"prompt_tokens":10,"total_tokens":10}}`)
	require.Equal(t, raw, convertPublicUsageCostsToCNY(raw))
}

func TestConvertPublicUsageCostsToCNYIsIdempotent(t *testing.T) {
	raw := common.StringToByteSlice(`{"usage":{"cost":1.825,"currency":"CNY","usd_exchange_rate":7.3}}`)
	require.Equal(t, raw, convertPublicUsageCostsToCNY(raw))
}

func TestPublicUsageCopyWithCNYCostDoesNotMutateBillingUsage(t *testing.T) {
	previousRate := operation_setting.USDExchangeRate
	operation_setting.USDExchangeRate = 7.3
	t.Cleanup(func() { operation_setting.USDExchangeRate = previousRate })

	internal := &dto.Usage{PromptTokens: 10, Cost: 0.25}
	public := publicUsageCopyWithCNYCost(internal)

	require.Equal(t, 0.25, internal.Cost)
	require.InDelta(t, 1.825, public.Cost, 1e-12)
	require.Equal(t, "CNY", public.Currency)
	require.Equal(t, 7.3, public.USDExchangeRate)
}
