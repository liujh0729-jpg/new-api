package constant

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAIPDDExcludedModelFamiliesAreFiltered(t *testing.T) {
	originalCapabilities := GetAIPDDCapabilities()
	originalOpenAIModels := GetAIPDDOpenAIModelList()
	t.Cleanup(func() {
		SetAIPDDCapabilities(originalCapabilities)
		SetAIPDDOpenAIModels(originalOpenAIModels)
	})

	SetAIPDDCapabilities([]AIPDDCapability{
		{ModelName: "aipdd-funasr", ScriptCode: "Fun-ASR"},
		{ModelName: "aipdd_lightx2v", ScriptCode: "LightX2V"},
		{ModelName: "seedvr2-upscale", ScriptCode: "SeedVR2"},
		{ModelName: "aipdd-index-tts", ScriptCode: "aipdd_IndexTTS"},
	})
	SetAIPDDOpenAIModels([]string{"fun-asr-nano", "lightx2v", "seedvr2", "qwen3:8b"})

	require.Equal(t, []string{"aipdd-index-tts"}, GetAIPDDTaskModelList())
	require.Equal(t, []string{"qwen3:8b"}, GetAIPDDOpenAIModelList())
	require.Equal(t, []string{"aipdd-index-tts", "qwen3:8b"}, GetAIPDDModelList())
	require.True(t, IsAIPDDFunASRModel("AIPDD_Fun-ASR_Nano"))
	require.True(t, IsAIPDDExcludedModel("AIPDD-LightX2V"))
	require.True(t, IsAIPDDExcludedModel("seedvr2-upscale"))
}

func TestSetAIPDDCapabilitiesPreservesAuthenticatedSeedanceBillingMode(t *testing.T) {
	originalCapabilities := GetAIPDDCapabilities()
	t.Cleanup(func() { SetAIPDDCapabilities(originalCapabilities) })

	byokPrice := 0.7
	SetAIPDDCapabilities([]AIPDDCapability{{
		ModelName: "AP Seedance-2.5 标准版",
		SeedancePricing: &AIPDDSeedancePricing{
			BillingMode: "BYOK",
			ByResolution: map[string]AIPDDSeedanceResolutionPricing{
				"480p": {BYOKAmountAWCoinPerSecond: &byokPrice},
			},
		},
	}})

	capability, ok := GetAIPDDCapability("AP Seedance-2.5 标准版")
	require.True(t, ok)
	require.NotNil(t, capability.SeedancePricing)
	require.Equal(t, "BYOK", capability.SeedancePricing.BillingMode)
	require.Equal(t, 0.7, *capability.SeedancePricing.ByResolution["480p"].BYOKAmountAWCoinPerSecond)
}
