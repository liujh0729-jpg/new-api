package model

import (
	"errors"
	"strings"

	"github.com/QuantumNous/new-api/constant"
	"gorm.io/gorm"
)

// EnsureAIPDDDefaults is the one-shot AIPDD bootstrap used during system
// initialization. It keeps channel abilities and model metadata in sync with
// the local AIPDD catalog.
func EnsureAIPDDDefaults() error {
	if err := validateAIPDDBootstrapKey(getAIPDDKeyFromEnv()); err != nil {
		return err
	}
	changed, err := ensureAIPDDModelCatalogDefaults()
	if err != nil {
		return err
	}
	if err := EnsureAIPDDChannelDefaults(); err != nil {
		return err
	}
	if changed {
		InvalidatePricingCache()
	}
	return nil
}

func ensureAIPDDModelCatalogDefaults() (bool, error) {
	vendorID, changed, err := ensureAIPDDVendor()
	if err != nil {
		return false, err
	}

	for _, catalog := range defaultCatalogModels {
		if catalog.ChannelType != constant.ChannelTypeAIPDD {
			continue
		}
		itemChanged, err := ensureAIPDDModelCatalogItem(catalog, vendorID)
		if err != nil {
			return false, err
		}
		changed = changed || itemChanged
	}
	return changed, nil
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
