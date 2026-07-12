package model

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"gorm.io/gorm"
)

// EnsureAIPDDDefaults is the one-shot AIPDD bootstrap used during system
// initialization. It keeps channel abilities and model metadata in sync with
// the local AIPDD catalog.
func EnsureAIPDDDefaults() error {
	key := getAIPDDKeyFromEnv()
	if err := validateAIPDDBootstrapKey(key); err != nil {
		return err
	}
	if strings.TrimSpace(key) == "" || !isAIPDDCatalogSyncOnBootEnabled(key) {
		return nil
	}
	timeoutSeconds := common.GetEnvOrDefault(aipddCatalogSyncTimeoutSecondsEnvName, 10)
	if timeoutSeconds <= 0 {
		timeoutSeconds = 10
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSeconds)*time.Second)
	defer cancel()
	result, err := SyncAIPDDCatalog(ctx, nil, getAIPDDBaseURLFromEnv(), key)
	if err != nil {
		return fmt.Errorf("AIPDD atomic catalog sync failed: %w", err)
	}
	common.SysLog(fmt.Sprintf("AIPDD atomic catalog ready: revision=%s, added=%d, removed=%d, snapshot=%t", result.Revision, result.AddedModels, result.RemovedModels, result.UsedSnapshot))
	return nil
}

func EnsureAIPDDOpenAIModelDefaults(modelNames []string) error {
	constant.SetAIPDDOpenAIModels(modelNames)
	modelNames = constant.GetAIPDDOpenAIModelList()
	if len(modelNames) == 0 {
		return nil
	}

	changed, err := ensureAIPDDModelCatalogDefaults()
	if err != nil {
		return err
	}
	if err := syncAIPDDOpenAIModelRatios(modelNames); err != nil {
		return err
	}
	if changed {
		InvalidatePricingCache()
	}
	return nil
}

func syncAIPDDOpenAIModelRatios(modelNames []string) error {
	if len(modelNames) == 0 {
		return nil
	}

	ratios := ratio_setting.GetModelRatioCopy()
	var option Option
	err := DB.Where(&Option{Key: "ModelRatio"}).First(&option).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	if err == nil && strings.TrimSpace(option.Value) != "" {
		if unmarshalErr := common.Unmarshal([]byte(option.Value), &ratios); unmarshalErr != nil {
			return unmarshalErr
		}
	}

	changed := false
	for _, modelName := range modelNames {
		modelName = strings.TrimSpace(modelName)
		if modelName == "" {
			continue
		}
		if _, exists := ratios[modelName]; exists {
			continue
		}
		ratios[modelName] = 1
		changed = true
	}
	if !changed {
		return nil
	}

	bytes, err := common.Marshal(ratios)
	if err != nil {
		return err
	}
	if err := ratio_setting.UpdateModelRatioByJSONString(string(bytes)); err != nil {
		return err
	}

	option.Key = "ModelRatio"
	option.Value = string(bytes)
	return DB.Save(&option).Error
}

func isAIPDDCatalogSyncOnBootEnabled(key string) bool {
	raw := strings.TrimSpace(common.GetEnvOrDefaultString(aipddCatalogSyncOnBootEnvName, ""))
	if raw != "" {
		return common.GetEnvOrDefaultBool(aipddCatalogSyncOnBootEnvName, true)
	}
	return strings.HasPrefix(strings.TrimSpace(key), "sk-")
}

func ensureAIPDDModelCatalogDefaults() (bool, error) {
	vendorID, changed, err := ensureAIPDDVendor()
	if err != nil {
		return false, err
	}

	for _, catalog := range aipddCurrentCatalogModels() {
		itemChanged, err := ensureAIPDDModelCatalogItem(catalog, vendorID)
		if err != nil {
			return false, err
		}
		changed = changed || itemChanged
	}
	for _, modelName := range constant.GetAIPDDOpenAIModelList() {
		itemChanged, err := ensureAIPDDModelCatalogItem(aipddOpenAIModelCatalog(modelName), vendorID)
		if err != nil {
			return false, err
		}
		changed = changed || itemChanged
	}
	return changed, nil
}

func aipddOpenAIModelCatalog(modelName string) defaultCatalogModel {
	modelName = strings.TrimSpace(modelName)
	return defaultCatalogModel{
		ModelName:     modelName,
		VendorName:    "AIPDD",
		Description:   "AIPDD OpenAI 兼容 LLM 模型 " + modelName + "，通过 /v1/chat/completions 调用。",
		Icon:          constant.AIPDDLogoPath,
		Tags:          "AIPDD,LLM,文本生成,OpenAI兼容",
		ChannelType:   constant.ChannelTypeAIPDD,
		EndpointTypes: []constant.EndpointType{constant.EndpointTypeOpenAI},
	}
}

func ensureAIPDDVendor() (int, bool, error) {
	var vendor Vendor
	err := DB.Where("name = ?", "AIPDD").First(&vendor).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		vendor = Vendor{
			Name:    "AIPDD",
			Icon:    constant.AIPDDLogoPath,
			Website: constant.AIPDDWebsiteURL,
			Status:  1,
		}
		if err := vendor.Insert(); err != nil {
			return 0, false, err
		}
		return vendor.Id, true, nil
	}
	if err != nil {
		return 0, false, err
	}

	updates := map[string]interface{}{}
	if shouldReplaceDefaultIcon(vendor.Icon, constant.AIPDDLogoPath) {
		updates["icon"] = constant.AIPDDLogoPath
	}
	if strings.TrimSpace(vendor.Website) == "" {
		updates["website"] = constant.AIPDDWebsiteURL
	}
	if len(updates) > 0 {
		if err := DB.Model(&Vendor{}).Where("id = ?", vendor.Id).Updates(updates).Error; err != nil {
			return 0, false, err
		}
		return vendor.Id, true, nil
	}
	return vendor.Id, false, nil
}

func ensureAIPDDModelCatalogItem(catalog defaultCatalogModel, vendorID int) (bool, error) {
	endpoints := marshalEndpointTypes(catalog.EndpointTypes)

	var item Model
	err := DB.Where("model_name = ?", catalog.ModelName).First(&item).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		item = Model{
			ModelName:    catalog.ModelName,
			Description:  catalog.Description,
			Icon:         catalog.Icon,
			Tags:         catalog.Tags,
			VendorID:     vendorID,
			Endpoints:    endpoints,
			Status:       1,
			SyncOfficial: 1,
			NameRule:     NameRuleExact,
		}
		if err := item.Insert(); err != nil {
			return false, err
		}
		return true, nil
	}
	if err != nil {
		return false, err
	}
	if item.SyncOfficial == 0 {
		return false, nil
	}

	updates := map[string]interface{}{}
	if item.VendorID == 0 {
		updates["vendor_id"] = vendorID
	}
	if strings.TrimSpace(item.Description) == "" {
		updates["description"] = catalog.Description
	}
	if shouldReplaceDefaultIcon(item.Icon, catalog.Icon) {
		updates["icon"] = catalog.Icon
	}
	if strings.TrimSpace(item.Tags) == "" {
		updates["tags"] = catalog.Tags
	}
	if strings.TrimSpace(item.Endpoints) == "" {
		updates["endpoints"] = endpoints
	}
	if len(updates) == 0 {
		return false, nil
	}
	return true, DB.Model(&Model{}).Where("id = ?", item.Id).Updates(updates).Error
}
