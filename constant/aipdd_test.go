package constant

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAIPDDFunASRModelsAreFiltered(t *testing.T) {
	originalCapabilities := GetAIPDDCapabilities()
	originalOpenAIModels := GetAIPDDOpenAIModelList()
	t.Cleanup(func() {
		SetAIPDDCapabilities(originalCapabilities)
		SetAIPDDOpenAIModels(originalOpenAIModels)
	})

	SetAIPDDCapabilities([]AIPDDCapability{
		{ModelName: "aipdd-funasr", ScriptCode: "Fun-ASR"},
		{ModelName: "aipdd-index-tts", ScriptCode: "aipdd_IndexTTS"},
	})
	SetAIPDDOpenAIModels([]string{"fun-asr-nano", "qwen3:8b"})

	require.Equal(t, []string{"aipdd-index-tts"}, GetAIPDDTaskModelList())
	require.Equal(t, []string{"qwen3:8b"}, GetAIPDDOpenAIModelList())
	require.Equal(t, []string{"aipdd-index-tts", "qwen3:8b"}, GetAIPDDModelList())
	require.True(t, IsAIPDDFunASRModel("AIPDD_Fun-ASR_Nano"))
}
