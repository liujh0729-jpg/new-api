package constant

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAIPDDCatalogModelsAreNotHardExcludedByFamily(t *testing.T) {
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

	require.Equal(t, []string{"aipdd-funasr", "aipdd_lightx2v", "seedvr2-upscale", "aipdd-index-tts"}, GetAIPDDTaskModelList())
	require.Equal(t, []string{"fun-asr-nano", "lightx2v", "seedvr2", "qwen3:8b"}, GetAIPDDOpenAIModelList())
	require.Equal(t, []string{
		"aipdd-funasr", "aipdd_lightx2v", "seedvr2-upscale", "aipdd-index-tts",
		"fun-asr-nano", "lightx2v", "seedvr2", "qwen3:8b",
	}, GetAIPDDModelList())
	require.True(t, IsAIPDDFunASRModel("AIPDD_Fun-ASR_Nano"))
	require.False(t, IsAIPDDExcludedModel("AIPDD-LightX2V"))
	require.False(t, IsAIPDDExcludedModel("seedvr2-upscale"))
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

func TestMergedLegacyModelsStayExecutableButAreNotAdvertised(t *testing.T) {
	originalCapabilities := GetAIPDDCapabilities()
	originalOpenAIModels := GetAIPDDOpenAIModelList()
	t.Cleanup(func() {
		SetAIPDDCapabilities(originalCapabilities)
		SetAIPDDOpenAIModels(originalOpenAIModels)
	})

	SetAIPDDCapabilities([]AIPDDCapability{
		{ModelName: AIPDDModelLTX23, ScriptCode: AIPDDModelLTX23},
		{ModelName: AIPDDModelUnifiedLTX23, ScriptCode: AIPDDModelUnifiedLTX23},
	})
	SetAIPDDOpenAIModels([]string{
		"aipdd_qwen_image_edit_single_reference",
		AIPDDModelQwenImageEdit,
	})

	require.Equal(t, []string{AIPDDModelUnifiedLTX23}, GetAIPDDTaskModelList())
	require.Equal(t, []string{AIPDDModelQwenImageEdit}, GetAIPDDOpenAIModelList())
	_, executable := GetAIPDDCapability(AIPDDModelLTX23)
	require.True(t, executable)
	require.True(t, IsAIPDDMergedLegacyPublicModel("AIPDD_LTX_2.3"))
	require.False(t, IsAIPDDMergedLegacyPublicModel(AIPDDModelUnifiedLTX23))
}

func TestAIPDDImageEndpointsSeparateGenerationImageToImageAndEditing(t *testing.T) {
	originalCapabilities := GetAIPDDCapabilities()
	t.Cleanup(func() { SetAIPDDCapabilities(originalCapabilities) })

	SetAIPDDCapabilities([]AIPDDCapability{
		{ModelName: "text-image", TaskKind: "text_to_image", EndpointType: EndpointTypeImageGeneration},
		{ModelName: "reference-image", TaskKind: "image_to_image", EndpointType: EndpointTypeImageGeneration},
		{ModelName: AIPDDModelQwenImageEdit, TaskKind: "image_to_image", EndpointType: EndpointTypeImageGeneration},
	})

	textImage, ok := GetAIPDDCapability("text-image")
	require.True(t, ok)
	require.Equal(t, EndpointTypeImageGeneration, textImage.EndpointType)
	referenceImage, ok := GetAIPDDCapability("reference-image")
	require.True(t, ok)
	require.Equal(t, EndpointTypeImageToImage, referenceImage.EndpointType)
	imageEdit, ok := GetAIPDDCapability(AIPDDModelQwenImageEdit)
	require.True(t, ok)
	require.Equal(t, EndpointTypeImageEdit, imageEdit.EndpointType)
}
