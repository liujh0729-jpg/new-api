package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/stretchr/testify/require"
)

func TestRemoveLegacyAIPDDModelsClearsCatalogAbilitiesAndChannel(t *testing.T) {
	truncateTables(t)

	channel := Channel{
		Type:   constant.ChannelTypeAIPDD,
		Key:    "aipdd-test-key",
		Name:   "AIPDD",
		Status: common.ChannelStatusEnabled,
		Group:  "default",
		Models: "aipdd-flux-gguf,aipdd-indextts,catalog-model",
	}
	require.NoError(t, DB.Create(&channel).Error)
	require.NoError(t, DB.Create(&Ability{
		Group: "default", Model: "aipdd-flux-gguf", ChannelId: channel.Id, Enabled: true,
	}).Error)
	require.NoError(t, DB.Create(&Ability{
		Group: "default", Model: "catalog-model", ChannelId: channel.Id, Enabled: true,
	}).Error)
	require.NoError(t, DB.Create(&Model{
		ModelName: "aipdd-flux-gguf", Icon: constant.AIPDDLogoPath, Status: 1,
	}).Error)
	require.NoError(t, DB.Create(&Model{
		ModelName: "catalog-model", Icon: constant.AIPDDLogoPath, Status: 1,
	}).Error)

	require.NoError(t, RemoveLegacyAIPDDModels())

	var stored Channel
	require.NoError(t, DB.First(&stored, channel.Id).Error)
	require.Equal(t, []string{"catalog-model"}, stored.GetModels())

	var abilities []Ability
	require.NoError(t, DB.Where("channel_id = ?", channel.Id).Find(&abilities).Error)
	require.Len(t, abilities, 1)
	require.Equal(t, "catalog-model", abilities[0].Model)

	var legacyCount int64
	require.NoError(t, DB.Unscoped().Model(&Model{}).Where("model_name IN ?", legacyAIPDDModelNames).Count(&legacyCount).Error)
	require.Zero(t, legacyCount)

	var kept Model
	require.NoError(t, DB.Where("model_name = ?", "catalog-model").First(&kept).Error)
}

func TestRemoveLegacyAIPDDModelsIsIdempotent(t *testing.T) {
	truncateTables(t)

	require.NoError(t, RemoveLegacyAIPDDModels())
	require.NoError(t, RemoveLegacyAIPDDModels())

	var abilityCount int64
	require.NoError(t, DB.Model(&Ability{}).Where("model IN ?", legacyAIPDDModelNames).Count(&abilityCount).Error)
	require.Zero(t, abilityCount)
}
