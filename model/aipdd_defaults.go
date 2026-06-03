package model

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/pkg/aipddcatalog"
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
	syncAIPDDCatalogFromEnv(key)

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

func syncAIPDDCatalogFromEnv(key string) {
	key = strings.TrimSpace(key)
	if key == "" || !isAIPDDCatalogSyncOnBootEnabled(key) {
		return
	}

	timeoutSeconds := common.GetEnvOrDefault(aipddCatalogSyncTimeoutSecondsEnvName, 10)
	if timeoutSeconds <= 0 {
		timeoutSeconds = 10
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSeconds)*time.Second)
	defer cancel()

	catalog, err := aipddcatalog.Fetch(ctx, http.DefaultClient, getAIPDDBaseURLFromEnv(), key)
	if err != nil {
		common.SysLog("AIPDD catalog sync on boot failed, fallback to built-in defaults: " + err.Error())
		return
	}
	if len(catalog.Capabilities) == 0 {
		common.SysLog("AIPDD catalog sync on boot returned no models, fallback to built-in defaults")
		return
	}
	constant.SetAIPDDCapabilities(catalog.Capabilities)
	common.SysLog("AIPDD catalog synced on boot: models=" + strings.Join(catalog.ModelNames(), ","))
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
