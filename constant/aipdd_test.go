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
