package model

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"gorm.io/gorm"
)

// legacyAIPDDModelNames are the ComfyUI models this gateway used to compile in
// as built-in defaults. AIPDD models now come only from the upstream catalog, so
// these names no longer resolve to a capability and every stored reference to
// them is dropped at boot.
var legacyAIPDDModelNames = []string{
	"aipdd-flux-gguf",
	"aipdd-flux-gguf-t2i",
	"aipdd-wan2.2-wanx",
	"aipdd-wan2.2-animater",
	"aipdd-mimic-motion",
	"aipdd-latentsync-1.5",
	"aipdd-indextts",
	"aipdd_ltx_2.3",
	"aipdd_ltx_2.3 (首尾帧)",
}

// RemoveLegacyAIPDDModels clears the retired default models from the model
// catalog, the ability routing table and every AIPDD channel model list.
func RemoveLegacyAIPDDModels() error {
	legacy := stringSet(legacyAIPDDModelNames)
	removed := false

	if err := DB.Transaction(func(tx *gorm.DB) error {
		var channelIDs []int
		if err := tx.Model(&Channel{}).Where("type = ?", constant.ChannelTypeAIPDD).Pluck("id", &channelIDs).Error; err != nil {
			return err
		}
		for _, channelID := range channelIDs {
			var channel Channel
			if err := tx.First(&channel, channelID).Error; err != nil {
				return err
			}
			kept := make([]string, 0, len(channel.Models))
			channelChanged := false
			for _, modelName := range channel.GetModels() {
				if legacy[modelName] {
					channelChanged = true
					continue
				}
				kept = append(kept, modelName)
			}
			if !channelChanged {
				continue
			}
			if err := tx.Model(&channel).Update("models", strings.Join(kept, ",")).Error; err != nil {
				return err
			}
			removed = true
		}
		if len(channelIDs) > 0 {
			result := tx.Where("channel_id IN ? AND model IN ?", channelIDs, legacyAIPDDModelNames).Delete(&Ability{})
			if result.Error != nil {
				return result.Error
			}
			removed = removed || result.RowsAffected > 0
		}
		result := tx.Unscoped().Where("model_name IN ?", legacyAIPDDModelNames).Delete(&Model{})
		if result.Error != nil {
			return result.Error
		}
		removed = removed || result.RowsAffected > 0
		return nil
	}); err != nil {
		return err
	}

	if !removed {
		return nil
	}
	common.SysLog("removed retired built-in AIPDD models from the database")
	InitChannelCache()
	InvalidatePricingCache()
	return nil
}
