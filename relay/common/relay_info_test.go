package common

import (
	"testing"

	rootcommon "github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/require"
)

func TestTaskSubmitReqAcceptsMultipartStyleCharacterIDAndIgnoresLegacyAssetID(t *testing.T) {
	var req TaskSubmitReq
	require.NoError(t, rootcommon.Unmarshal([]byte(`{"character_id":"12","character_asset_id":"34","model":"seedance"}`), &req))
	require.NotNil(t, req.CharacterID)
	require.EqualValues(t, 12, *req.CharacterID)
	payload, err := rootcommon.Marshal(req)
	require.NoError(t, err)
	require.NotContains(t, string(payload), "character_asset_id")
}

func TestTaskSubmitReqPromotesTopLevelContentOverMetadataContent(t *testing.T) {
	var req TaskSubmitReq
	require.NoError(t, rootcommon.Unmarshal([]byte(`{
        "model":"doubao-seedance-2-0-fast-260128",
        "prompt":"legacy prompt",
        "content":[{"type":"text","text":"official content"}],
        "metadata":{"content":[{"type":"text","text":"metadata content"}],"resolution":"720p"}
    }`), &req))

	content, ok := req.Metadata["content"].([]any)
	require.True(t, ok)
	require.Len(t, content, 1)
	item, ok := content[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "official content", item["text"])
}

func TestRelayInfoGetFinalRequestRelayFormatPrefersExplicitFinal(t *testing.T) {
	info := &RelayInfo{
		RelayFormat:             types.RelayFormatOpenAI,
		RequestConversionChain:  []types.RelayFormat{types.RelayFormatOpenAI, types.RelayFormatClaude},
		FinalRequestRelayFormat: types.RelayFormatOpenAIResponses,
	}

	require.Equal(t, types.RelayFormat(types.RelayFormatOpenAIResponses), info.GetFinalRequestRelayFormat())
}

func TestRelayInfoGetFinalRequestRelayFormatFallsBackToConversionChain(t *testing.T) {
	info := &RelayInfo{
		RelayFormat:            types.RelayFormatOpenAI,
		RequestConversionChain: []types.RelayFormat{types.RelayFormatOpenAI, types.RelayFormatClaude},
	}

	require.Equal(t, types.RelayFormat(types.RelayFormatClaude), info.GetFinalRequestRelayFormat())
}

func TestRelayInfoGetFinalRequestRelayFormatFallsBackToRelayFormat(t *testing.T) {
	info := &RelayInfo{
		RelayFormat: types.RelayFormatGemini,
	}

	require.Equal(t, types.RelayFormat(types.RelayFormatGemini), info.GetFinalRequestRelayFormat())
}

func TestRelayInfoGetFinalRequestRelayFormatNilReceiver(t *testing.T) {
	var info *RelayInfo
	require.Equal(t, types.RelayFormat(""), info.GetFinalRequestRelayFormat())
}

func TestAIPDDFinanceContextAcceptsLegacyStringIDs(t *testing.T) {
	var context AIPDDFinanceContext
	require.NoError(t, rootcommon.Unmarshal([]byte(`{
		"instance_id":"instance-1",
		"platform_order_id":"order-1",
		"attempt_id":"attempt-1",
		"newapi_user_id":"12",
		"newapi_token_id":"34",
		"channel_id":56,
		"channel_key_index":1
	}`), &context))

	require.Equal(t, 12, context.NewAPIUserID)
	require.Equal(t, 34, context.NewAPITokenID)
	payload, err := rootcommon.Marshal(context)
	require.NoError(t, err)
	require.JSONEq(t, `{
		"instance_id":"instance-1",
		"platform_order_id":"order-1",
		"attempt_id":"attempt-1",
		"newapi_user_id":12,
		"newapi_token_id":34,
		"channel_id":56,
		"channel_key_index":1
	}`, string(payload))
}

func TestAIPDDFinanceContextAcceptsNumericAndEmptyIDs(t *testing.T) {
	var numeric AIPDDFinanceContext
	require.NoError(t, rootcommon.Unmarshal([]byte(`{"newapi_user_id":12,"newapi_token_id":34}`), &numeric))
	require.Equal(t, 12, numeric.NewAPIUserID)
	require.Equal(t, 34, numeric.NewAPITokenID)

	var empty AIPDDFinanceContext
	require.NoError(t, rootcommon.Unmarshal([]byte(`{"newapi_user_id":"","newapi_token_id":null}`), &empty))
	require.Zero(t, empty.NewAPIUserID)
	require.Zero(t, empty.NewAPITokenID)
}

func TestAIPDDFinanceContextRejectsInvalidStringID(t *testing.T) {
	var context AIPDDFinanceContext
	err := rootcommon.Unmarshal([]byte(`{"newapi_user_id":"not-a-number"}`), &context)
	require.ErrorContains(t, err, "invalid newapi_user_id")
}
