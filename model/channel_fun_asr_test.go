package model

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/stretchr/testify/require"
)

func TestAIPDDChannelHidesExcludedModels(t *testing.T) {
	channel := Channel{
		Type:   constant.ChannelTypeAIPDD,
		Models: "aipdd-index-tts,aipdd-funasr,fun-asr-nano,aipdd_lightx2v,seedvr2-upscale",
	}

	require.Equal(t, []string{"aipdd-index-tts"}, channel.GetModels())
}

func TestExcludedAIPDDModelsAreExcludedFromAbilitySelection(t *testing.T) {
	require.Equal(t, []string{"safe-model"}, filterDisabledAIPDDModels([]string{"funasr", "safe-model", "fun-asr-nano", "lightx2v", "seedvr2"}))

	channel, err := GetChannel("default", "aipdd-funasr", 0)
	require.NoError(t, err)
	require.Nil(t, channel)

	channel, err = GetChannel("default", "aipdd_lightx2v", 0)
	require.NoError(t, err)
	require.Nil(t, channel)
}
