package model

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/pkg/aipddcatalog"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const aipddCatalogSnapshotID = 1

type AIPDDCatalogSyncResult struct {
	Revision      string  `json:"revision"`
	AddedModels   int     `json:"added_models"`
	RemovedModels int     `json:"removed_models"`
	UpdatedPrices int     `json:"updated_prices"`
	USDPerAWCoin  float64 `json:"usd_per_awcoin"`
	RMBPerAWCoin  float64 `json:"rmb_per_awcoin"`
	UsedSnapshot  bool    `json:"used_snapshot"`
}

func SyncAIPDDCatalog(ctx context.Context, client *http.Client, baseURL, apiKey string) (AIPDDCatalogSyncResult, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	catalog, err := aipddcatalog.FetchAtomic(ctx, client, baseURL, apiKey)
	if err != nil {
		return restoreAIPDDCatalogSnapshot(baseURL, err)
	}
	result, err := applyAIPDDCatalog(catalog, baseURL, apiKey)
	if err != nil {
		return AIPDDCatalogSyncResult{}, err
	}
	return result, nil
}

func restoreAIPDDCatalogSnapshot(baseURL string, fetchErr error) (AIPDDCatalogSyncResult, error) {
	var snapshot AIPDDCatalogSnapshot
	if err := DB.First(&snapshot, aipddCatalogSnapshotID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return AIPDDCatalogSyncResult{}, fmt.Errorf("initial AIPDD catalog sync failed without a snapshot: %w", fetchErr)
		}
		return AIPDDCatalogSyncResult{}, err
	}
	if strings.TrimRight(snapshot.SourceBaseURL, "/") != baseURL {
		return AIPDDCatalogSyncResult{}, fmt.Errorf("AIPDD catalog sync failed and snapshot belongs to a different base URL: %w", fetchErr)
	}
	catalog, err := aipddcatalog.UnmarshalAtomic([]byte(snapshot.Payload))
	if err != nil {
		return AIPDDCatalogSyncResult{}, fmt.Errorf("AIPDD catalog sync failed and snapshot is invalid: %w", err)
	}
	activateAIPDDCatalog(catalog)
	common.SysLog("AIPDD catalog sync failed; using same-origin snapshot revision=" + catalog.Revision + ": " + fetchErr.Error())
	return AIPDDCatalogSyncResult{
		Revision: catalog.Revision, USDPerAWCoin: catalog.AWCoinRate.USDPerAWCoin,
		RMBPerAWCoin: catalog.AWCoinRate.RMBPerAWCoin, UsedSnapshot: true,
	}, nil
}

func applyAIPDDCatalog(catalog aipddcatalog.AtomicCatalog, baseURL, apiKey string) (AIPDDCatalogSyncResult, error) {
	// FetchAtomic and UnmarshalAtomic already apply this filter, but keep the
	// boundary defensive so a catalog assembled by another caller cannot
	// reintroduce an intentionally unsupported AIPDD family.
	catalog.FilterExcluded()
	if err := catalog.Validate(); err != nil {
		return AIPDDCatalogSyncResult{}, err
	}
	payload, err := aipddcatalog.MarshalAtomic(catalog)
	if err != nil {
		return AIPDDCatalogSyncResult{}, err
	}
	currentNames := catalog.ModelNames()
	currentSet := stringSet(currentNames)
	previousNames, err := previousAIPDDCatalogModels(baseURL)
	if err != nil {
		return AIPDDCatalogSyncResult{}, err
	}
	previousSet := stringSet(previousNames)

	result := AIPDDCatalogSyncResult{
		Revision:     catalog.Revision,
		USDPerAWCoin: catalog.AWCoinRate.USDPerAWCoin,
		RMBPerAWCoin: catalog.AWCoinRate.RMBPerAWCoin,
	}
	for modelName := range currentSet {
		if !previousSet[modelName] {
			result.AddedModels++
		}
	}
	for modelName := range previousSet {
		if !currentSet[modelName] {
			result.RemovedModels++
		}
	}

	tx := DB.Begin()
	if tx.Error != nil {
		return AIPDDCatalogSyncResult{}, tx.Error
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			tx.Rollback()
			panic(recovered)
		}
	}()

	vendorID, err := upsertAIPDDVendorTx(tx)
	if err == nil {
		err = upsertAIPDDModelsTx(tx, catalog, vendorID)
	}
	var channel *Channel
	if err == nil {
		channel, err = upsertManagedAIPDDChannelTx(tx, baseURL, apiKey, currentNames)
	}
	if err == nil {
		err = replaceManagedAIPDDAbilitiesTx(tx, channel)
	}
	if err == nil {
		err = cleanupExcludedAIPDDDataTx(tx, vendorID)
	}
	if err == nil {
		err = removeStaleAIPDDModelsTx(tx, previousSet, currentSet, vendorID)
	}
	if err == nil {
		err = cleanupCNProviderDefaultsTx(tx, currentSet)
	}
	if err == nil {
		snapshot := AIPDDCatalogSnapshot{
			ID: aipddCatalogSnapshotID, SchemaVersion: catalog.SchemaVersion,
			Revision: catalog.Revision, SourceBaseURL: baseURL,
			Payload: string(payload), SyncedAt: time.Now(),
		}
		err = tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "id"}},
			DoUpdates: clause.AssignmentColumns([]string{"schema_version", "revision", "source_base_url", "payload", "synced_at"}),
		}).Create(&snapshot).Error
	}
	if err != nil {
		tx.Rollback()
		return AIPDDCatalogSyncResult{}, err
	}
	if err := tx.Commit().Error; err != nil {
		return AIPDDCatalogSyncResult{}, err
	}

	activateAIPDDCatalog(catalog)
	InitChannelCache()
	InvalidatePricingCache()
	ratio_setting.InvalidateExposedDataCache()
	return result, nil
}

func previousAIPDDCatalogModels(baseURL string) ([]string, error) {
	var snapshot AIPDDCatalogSnapshot
	if err := DB.First(&snapshot, aipddCatalogSnapshotID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return managedAIPDDChannelModels()
		}
		return nil, err
	}
	if strings.TrimRight(snapshot.SourceBaseURL, "/") != baseURL {
		return nil, nil
	}
	catalog, err := aipddcatalog.UnmarshalAtomic([]byte(snapshot.Payload))
	if err != nil {
		// A live catalog has already passed the current contract before this
		// function is called. Do not interpret or migrate an incompatible legacy
		// snapshot; use only the managed channel's model names for stale cleanup,
		// then let the transaction replace the snapshot with the live catalog.
		common.SysLog("AIPDD previous snapshot is incompatible with the current contract; using managed channel models: " + err.Error())
		return managedAIPDDChannelModels()
	}
	return catalog.ModelNames(), nil
}

func managedAIPDDChannelModels() ([]string, error) {
	var channel Channel
	err := DB.Where("type = ? AND name = ?", constant.ChannelTypeAIPDD, aipddEnvChannelName).First(&channel).Error
	if err == nil {
		return channel.GetModels(), nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return nil, err
}

func activateAIPDDCatalog(catalog aipddcatalog.AtomicCatalog) {
	catalog.FilterExcluded()
	constant.SetAIPDDCapabilities(catalog.RuntimeCapabilities())
	models := make([]string, 0, len(catalog.Models))
	for _, model := range catalog.Models {
		models = append(models, model.ID)
	}
	constant.SetAIPDDOpenAIModels(models)
}

func upsertAIPDDVendorTx(tx *gorm.DB) (int, error) {
	var vendor Vendor
	err := tx.Where("name = ?", "AIPDD").First(&vendor).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		vendor = Vendor{Name: "AIPDD", Icon: constant.AIPDDLogoPath, Website: constant.AIPDDWebsiteURL, Status: 1, CreatedTime: common.GetTimestamp(), UpdatedTime: common.GetTimestamp()}
		err = tx.Create(&vendor).Error
	} else if err == nil {
		err = tx.Model(&vendor).Updates(map[string]any{"icon": constant.AIPDDLogoPath, "website": constant.AIPDDWebsiteURL, "status": 1, "updated_time": common.GetTimestamp()}).Error
	}
	return vendor.Id, err
}

func upsertAIPDDModelsTx(tx *gorm.DB, catalog aipddcatalog.AtomicCatalog, vendorID int) error {
	capabilityByName := make(map[string]aipddcatalog.AtomicCapability, len(catalog.Capabilities))
	for _, capability := range catalog.Capabilities {
		capabilityByName[capability.ID] = capability
	}
	llmByName := make(map[string]aipddcatalog.AtomicModel, len(catalog.Models))
	for _, model := range catalog.Models {
		llmByName[model.ID] = model
	}
	for _, modelName := range catalog.ModelNames() {
		description, tags := "AIPDD 上游目录同步模型。", "AIPDD"
		endpoints := []constant.EndpointType{constant.EndpointTypeOpenAI}
		if capability, ok := capabilityByName[modelName]; ok {
			description = capability.Description
			tags = "AIPDD,ComfyUI,异步任务"
			if capability.AdapterCode == "seedance" {
				tags = "AIPDD,Seedance,视频生成,异步任务"
			}
			if endpoint, ok := aipddCatalogEndpointType(capability.EndpointType); ok {
				endpoints = []constant.EndpointType{endpoint}
			}
		} else if model, ok := llmByName[modelName]; ok {
			description = model.Description
			tags = "AIPDD,Ollama,LLM,OpenAI兼容"
		}
		var item Model
		err := tx.Unscoped().Where("model_name = ?", modelName).First(&item).Error
		values := map[string]any{
			"description": description, "icon": constant.AIPDDLogoPath, "tags": tags,
			"vendor_id": vendorID, "endpoints": marshalEndpointTypes(endpoints), "status": 1,
			"sync_official": 1, "name_rule": NameRuleExact, "updated_time": common.GetTimestamp(), "deleted_at": nil,
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			item = Model{ModelName: modelName, Description: description, Icon: constant.AIPDDLogoPath, Tags: tags, VendorID: vendorID, Endpoints: marshalEndpointTypes(endpoints), Status: 1, SyncOfficial: 1, NameRule: NameRuleExact, CreatedTime: common.GetTimestamp(), UpdatedTime: common.GetTimestamp()}
			if err := tx.Create(&item).Error; err != nil {
				return err
			}
		} else if err != nil {
			return err
		} else if err := tx.Unscoped().Model(&item).Updates(values).Error; err != nil {
			return err
		}
	}
	return nil
}

func upsertManagedAIPDDChannelTx(tx *gorm.DB, baseURL, apiKey string, modelNames []string) (*Channel, error) {
	models := strings.Join(modelNames, ",")
	var channel Channel
	err := tx.Where("type = ? AND name = ?", constant.ChannelTypeAIPDD, aipddEnvChannelName).Order("id asc").First(&channel).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		channel = Channel{Type: constant.ChannelTypeAIPDD, Key: apiKey, Name: aipddEnvChannelName, Status: common.ChannelStatusEnabled, Group: "default", BaseURL: &baseURL, Models: models, CreatedTime: common.GetTimestamp()}
		if err := tx.Create(&channel).Error; err != nil {
			return nil, err
		}
		return &channel, nil
	}
	if err != nil {
		return nil, err
	}
	updates := map[string]any{"models": models, "group": "default", "base_url": baseURL, "status": common.ChannelStatusEnabled}
	if strings.TrimSpace(apiKey) != "" {
		updates["key"] = apiKey
		channel.Key = apiKey
	}
	channel.Models, channel.Group, channel.BaseURL, channel.Status = models, "default", &baseURL, common.ChannelStatusEnabled
	return &channel, tx.Model(&channel).Updates(updates).Error
}

func replaceManagedAIPDDAbilitiesTx(tx *gorm.DB, channel *Channel) error {
	if err := tx.Where("channel_id = ?", channel.Id).Delete(&Ability{}).Error; err != nil {
		return err
	}
	return channel.AddAbilities(tx)
}

func cleanupExcludedAIPDDDataTx(tx *gorm.DB, vendorID int) error {
	var channels []Channel
	if err := tx.Where("type = ?", constant.ChannelTypeAIPDD).Find(&channels).Error; err != nil {
		return err
	}
	channelIDs := make([]int, 0, len(channels))
	for _, channel := range channels {
		channelIDs = append(channelIDs, channel.Id)
	}
	if len(channelIDs) > 0 {
		var abilities []Ability
		if err := tx.Where("channel_id IN ?", channelIDs).Find(&abilities).Error; err != nil {
			return err
		}
		for _, ability := range abilities {
			if !constant.IsAIPDDExcludedModel(ability.Model) {
				continue
			}
			if err := tx.Delete(&ability).Error; err != nil {
				return err
			}
		}
	}

	var models []Model
	if err := tx.Unscoped().Where("vendor_id = ?", vendorID).Find(&models).Error; err != nil {
		return err
	}
	for _, item := range models {
		if !constant.IsAIPDDExcludedModel(item.ModelName) {
			continue
		}
		var abilityCount int64
		if err := tx.Model(&Ability{}).Where("model = ?", item.ModelName).Count(&abilityCount).Error; err != nil {
			return err
		}
		if abilityCount == 0 {
			if err := tx.Unscoped().Delete(&item).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

func removeStaleAIPDDModelsTx(tx *gorm.DB, previous, current map[string]bool, vendorID int) error {
	stale := make([]string, 0)
	for modelName := range previous {
		if !current[modelName] {
			stale = append(stale, modelName)
		}
	}
	if len(stale) == 0 {
		return nil
	}
	var channelIDs []int
	if err := tx.Model(&Channel{}).Where("type = ?", constant.ChannelTypeAIPDD).Pluck("id", &channelIDs).Error; err != nil {
		return err
	}
	if len(channelIDs) > 0 {
		if err := tx.Where("channel_id IN ? AND model IN ?", channelIDs, stale).Delete(&Ability{}).Error; err != nil {
			return err
		}
	}
	return tx.Unscoped().Where("vendor_id = ? AND model_name IN ?", vendorID, stale).Delete(&Model{}).Error
}

func cleanupCNProviderDefaultsTx(tx *gorm.DB, keepModels map[string]bool) error {
	for _, provider := range cnProviders {
		var channels []Channel
		if err := tx.Where("type = ? AND name = ?", provider.ChannelType, provider.Name).Find(&channels).Error; err != nil {
			return err
		}
		for _, channel := range channels {
			if err := tx.Where("channel_id = ?", channel.Id).Delete(&Ability{}).Error; err != nil {
				return err
			}
			if err := tx.Delete(&channel).Error; err != nil {
				return err
			}
		}
		modelNames := make([]string, 0, len(provider.Models))
		for _, item := range provider.Models {
			if !keepModels[item.ModelName] {
				modelNames = append(modelNames, item.ModelName)
			}
		}
		if len(modelNames) > 0 {
			if err := tx.Unscoped().Where("model_name IN ?", modelNames).Delete(&Model{}).Error; err != nil {
				return err
			}
		}
		var vendor Vendor
		if err := tx.Where("name = ?", provider.Name).First(&vendor).Error; err == nil {
			var count int64
			if err := tx.Model(&Model{}).Where("vendor_id = ?", vendor.Id).Count(&count).Error; err != nil {
				return err
			}
			if count == 0 {
				if err := tx.Unscoped().Delete(&vendor).Error; err != nil {
					return err
				}
			}
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
	}
	return nil
}

func stringSet(values []string) map[string]bool {
	out := make(map[string]bool, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			out[value] = true
		}
	}
	return out
}

func aipddCatalogEndpointType(value string) (constant.EndpointType, bool) {
	switch constant.EndpointType(strings.TrimSpace(value)) {
	case constant.EndpointTypeOpenAI, constant.EndpointTypeImageGeneration,
		constant.EndpointTypeOpenAIVideo, constant.EndpointTypeAudioSpeech:
		return constant.EndpointType(strings.TrimSpace(value)), true
	default:
		return "", false
	}
}
