package model

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/stretchr/testify/require"
)

func TestAIPDDChannelHidesFunASRModels(t *testing.T) {
	channel := Channel{
		Type:   constant.ChannelTypeAIPDD,
		Models: "aipdd-index-tts,aipdd-funasr,fun-asr-nano",
	}

	require.Equal(t, []string{"aipdd-index-tts"}, channel.GetModels())
}

func TestFunASRModelsAreExcludedFromAbilitySelection(t *testing.T) {
	require.Equal(t, []string{"safe-model"}, filterDisabledFunASRModels([]string{"funasr", "safe-model", "fun-asr-nano"}))

	channel, err := GetChannel("default", "aipdd-funasr", 0)
	require.NoError(t, err)
	require.Nil(t, channel)
}
