package model

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/stretchr/testify/require"
)

func TestAIPDDChannelDoesNotApplyHardcodedModelExclusions(t *testing.T) {
	channel := Channel{
		Type:   constant.ChannelTypeAIPDD,
		Models: "aipdd-index-tts,aipdd-funasr,fun-asr-nano,aipdd_lightx2v,seedvr2-upscale",
	}

	require.Equal(t, []string{
		"aipdd-index-tts", "aipdd-funasr", "fun-asr-nano", "aipdd_lightx2v", "seedvr2-upscale",
	}, channel.GetModels())
}

func TestAbilitySelectionDefersToTheAtomicCatalog(t *testing.T) {
	models := []string{"funasr", "safe-model", "fun-asr-nano", "lightx2v", "seedvr2"}
	require.Equal(t, models, filterDisabledAIPDDModels(models))
}
