package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/stretchr/testify/require"
)

// Only the LightX2V and SeedVR2 families stay hidden; FunASR is the
// speech-to-text family the gateway now serves.
func TestAIPDDChannelHidesExcludedModels(t *testing.T) {
	channel := Channel{
		Type:   constant.ChannelTypeAIPDD,
		Models: "aipdd-index-tts,aipdd-funasr,fun-asr-nano,aipdd_lightx2v,seedvr2-upscale",
	}

	require.Equal(t, []string{"aipdd-index-tts", "aipdd-funasr", "fun-asr-nano"}, channel.GetModels())
}

func TestExcludedAIPDDModelsAreExcludedFromAbilitySelection(t *testing.T) {
	require.Equal(t, []string{"funasr", "safe-model", "fun-asr-nano"},
		filterDisabledAIPDDModels([]string{"funasr", "safe-model", "fun-asr-nano", "lightx2v", "seedvr2"}))

	truncateTables(t)
	channel := Channel{
		Type:   constant.ChannelTypeAIPDD,
		Key:    "aipdd-test-key",
		Name:   "AIPDD",
		Status: common.ChannelStatusEnabled,
		Group:  "default",
		Models: "aipdd-funasr,aipdd_lightx2v",
	}
	require.NoError(t, DB.Create(&channel).Error)
	require.NoError(t, channel.AddAbilities(nil))

	var funasrAbility Ability
	require.NoError(t, DB.Where("channel_id = ? AND model = ?", channel.Id, "aipdd-funasr").First(&funasrAbility).Error)
	require.True(t, funasrAbility.Enabled)

	var hiddenCount int64
	require.NoError(t, DB.Model(&Ability{}).Where("channel_id = ? AND model = ?", channel.Id, "aipdd_lightx2v").Count(&hiddenCount).Error)
	require.Zero(t, hiddenCount)
}
