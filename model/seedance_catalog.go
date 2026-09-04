package model

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	SeedanceFPSBucketUpTo30 = "LE_30"
	SeedanceFPSBucketOver30 = "GT_30"
)

// SeedanceBaseModel is one immutable revision of an Ark generation model.
// Published sale models keep the row ID so later edits cannot alter in-flight
// execution or cost snapshots.
type SeedanceBaseModel struct {
	ID              int64  `json:"id" gorm:"primaryKey"`
	Code            string `json:"code" gorm:"type:varchar(128);not null;index;uniqueIndex:idx_seedance_base_code_revision"`
	Revision        int    `json:"revision" gorm:"not null;uniqueIndex:idx_seedance_base_code_revision"`
	DisplayName     string `json:"display_name" gorm:"type:varchar(191);not null"`
	ProviderModelID string `json:"provider_model_id" gorm:"type:varchar(191);not null;index"`
	CostMatrixJSON  string `json:"cost_matrix" gorm:"column:cost_matrix;type:text;not null"`
	Enabled         bool   `json:"enabled" gorm:"not null;default:true;index"`
	Current         bool   `json:"current" gorm:"not null;default:true;index"`
	ArchivedAt      int64  `json:"archived_at,omitempty" gorm:"index"`
	CreatedAt       int64  `json:"created_at"`
	UpdatedAt       int64  `json:"updated_at"`
}

type SeedanceBaseCostEntry struct {
	SourceResolution      string `json:"source_resolution"`
	HasReferenceVideo     bool   `json:"has_reference_video"`
	CostMicroRMBPerSecond int64  `json:"cost_micro_rmb_per_second"`
}

// SeedanceEnhancementModel is one immutable revision of a super-resolution
// product. ProviderID selects the external node while AdapterType remains on
// the provider itself.
type SeedanceEnhancementModel struct {
	ID                   int64  `json:"id" gorm:"primaryKey"`
	Code                 string `json:"code" gorm:"type:varchar(128);not null;index;uniqueIndex:idx_seedance_enhance_code_revision"`
	Revision             int    `json:"revision" gorm:"not null;uniqueIndex:idx_seedance_enhance_code_revision"`
	DisplayName          string `json:"display_name" gorm:"type:varchar(191);not null"`
	ProviderID           int64  `json:"provider_id" gorm:"not null;index"`
	ServiceCode          string `json:"service_code" gorm:"type:varchar(128);not null"`
	QualityTier          string `json:"quality_tier" gorm:"type:varchar(64);not null;index"`
	SpecificationJSON    string `json:"specification" gorm:"column:specification;type:text;not null"`
	SpecificationVersion string `json:"specification_version" gorm:"type:varchar(64);not null"`
	CostMatrixJSON       string `json:"cost_matrix" gorm:"column:cost_matrix;type:text;not null"`
	Enabled              bool   `json:"enabled" gorm:"not null;default:true;index"`
	Current              bool   `json:"current" gorm:"not null;default:true;index"`
	ArchivedAt           int64  `json:"archived_at,omitempty" gorm:"index"`
	CreatedAt            int64  `json:"created_at"`
	UpdatedAt            int64  `json:"updated_at"`
}

type SeedanceEnhancementCostEntry struct {
	TargetResolution      string `json:"target_resolution"`
	FPSBucket             string `json:"fps_bucket"`
	CostMicroRMBPerSecond int64  `json:"cost_micro_rmb_per_second"`
}

func normalizeSeedanceCatalogCode(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func ensureSeedanceCatalogCode(value string, prefix string) string {
	value = normalizeSeedanceCatalogCode(value)
	if value != "" {
		return value
	}
	return prefix + "-" + uuid.NewString()
}

func NormalizeSeedanceResolution(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "480p", "720p", "1080p", "2k", "4k":
		return value, nil
	default:
		return "", fmt.Errorf("unsupported Seedance resolution %q", value)
	}
}

func SeedanceFPSBucket(outputFPS int) (string, error) {
	if outputFPS <= 0 || outputFPS > 240 {
		return "", errors.New("Seedance output_fps must be from 1 to 240")
	}
	if outputFPS <= 30 {
		return SeedanceFPSBucketUpTo30, nil
	}
	return SeedanceFPSBucketOver30, nil
}

func normalizeSeedanceFPSBucket(value string) (string, error) {
	value = strings.ToUpper(strings.TrimSpace(value))
	switch value {
	case SeedanceFPSBucketUpTo30, "<=30", "UP_TO_30":
		return SeedanceFPSBucketUpTo30, nil
	case SeedanceFPSBucketOver30, ">30", "OVER_30":
		return SeedanceFPSBucketOver30, nil
	default:
		return "", fmt.Errorf("unsupported Seedance fps_bucket %q", value)
	}
}

func ValidateSeedanceBaseCostMatrix(value string) ([]SeedanceBaseCostEntry, error) {
	entries := make([]SeedanceBaseCostEntry, 0)
	if strings.TrimSpace(value) != "" {
		if err := common.UnmarshalJsonStr(strings.TrimSpace(value), &entries); err != nil {
			return nil, fmt.Errorf("decode Seedance base cost_matrix: %w", err)
		}
		if entries == nil {
			entries = make([]SeedanceBaseCostEntry, 0)
		}
	}
	seen := make(map[string]struct{}, len(entries))
	for i := range entries {
		resolution, err := NormalizeSeedanceResolution(entries[i].SourceResolution)
		if err != nil {
			return nil, err
		}
		if entries[i].CostMicroRMBPerSecond < 0 {
			return nil, errors.New("Seedance base cost must be non-negative integer micro-RMB per second")
		}
		entries[i].SourceResolution = resolution
		key := fmt.Sprintf("%s:%t", resolution, entries[i].HasReferenceVideo)
		if _, exists := seen[key]; exists {
			return nil, fmt.Errorf("duplicate Seedance base cost row %s", key)
		}
		seen[key] = struct{}{}
	}
	return entries, nil
}

func ValidateSeedanceEnhancementCostMatrix(value string) ([]SeedanceEnhancementCostEntry, error) {
	entries := make([]SeedanceEnhancementCostEntry, 0)
	if strings.TrimSpace(value) != "" {
		if err := common.UnmarshalJsonStr(strings.TrimSpace(value), &entries); err != nil {
			return nil, fmt.Errorf("decode Seedance enhancement cost_matrix: %w", err)
		}
		if entries == nil {
			entries = make([]SeedanceEnhancementCostEntry, 0)
		}
	}
	seen := make(map[string]struct{}, len(entries))
	for i := range entries {
		resolution, err := NormalizeSeedanceResolution(entries[i].TargetResolution)
		if err != nil {
			return nil, err
		}
		bucket, err := normalizeSeedanceFPSBucket(entries[i].FPSBucket)
		if err != nil {
			return nil, err
		}
		if entries[i].CostMicroRMBPerSecond < 0 {
			return nil, errors.New("Seedance enhancement cost must be non-negative integer micro-RMB per second")
		}
		entries[i].TargetResolution = resolution
		entries[i].FPSBucket = bucket
		key := resolution + ":" + bucket
		if _, exists := seen[key]; exists {
			return nil, fmt.Errorf("duplicate Seedance enhancement cost row %s", key)
		}
		seen[key] = struct{}{}
	}
	return entries, nil
}

func ResolveSeedanceBaseUnitCost(item *SeedanceBaseModel, sourceResolution string, hasReferenceVideo bool) (int64, error) {
	if item == nil {
		return 0, errors.New("Seedance base model is required")
	}
	entries, err := ValidateSeedanceBaseCostMatrix(item.CostMatrixJSON)
	if err != nil {
		return 0, err
	}
	resolution, err := NormalizeSeedanceResolution(sourceResolution)
	if err != nil {
		return 0, err
	}
	for _, entry := range entries {
		if entry.SourceResolution == resolution && entry.HasReferenceVideo == hasReferenceVideo {
			return entry.CostMicroRMBPerSecond, nil
		}
	}
	return 0, nil
}

func ResolveSeedanceEnhancementUnitCost(item *SeedanceEnhancementModel, targetResolution string, outputFPS int) (int64, error) {
	if item == nil {
		return 0, nil
	}
	entries, err := ValidateSeedanceEnhancementCostMatrix(item.CostMatrixJSON)
	if err != nil {
		return 0, err
	}
	resolution, err := NormalizeSeedanceResolution(targetResolution)
	if err != nil {
		return 0, err
	}
	bucket, err := SeedanceFPSBucket(outputFPS)
	if err != nil {
		return 0, err
	}
	for _, entry := range entries {
		if entry.TargetResolution == resolution && entry.FPSBucket == bucket {
			return entry.CostMicroRMBPerSecond, nil
		}
	}
	return 0, nil
}

func validateSeedanceBaseModel(item *SeedanceBaseModel) error {
	if item == nil {
		return errors.New("Seedance base model is required")
	}
	item.Code = normalizeSeedanceCatalogCode(item.Code)
	item.DisplayName = strings.TrimSpace(item.DisplayName)
	item.ProviderModelID = strings.TrimSpace(item.ProviderModelID)
	if item.Code == "" || item.DisplayName == "" || item.ProviderModelID == "" {
		return errors.New("Seedance base model code, display_name and provider_model_id are required")
	}
	entries, err := ValidateSeedanceBaseCostMatrix(item.CostMatrixJSON)
	if err != nil {
		return err
	}
	encoded, err := common.Marshal(entries)
	if err != nil {
		return err
	}
	item.CostMatrixJSON = string(encoded)
	return nil
}

func validateSeedanceEnhancementModel(item *SeedanceEnhancementModel) error {
	if item == nil {
		return errors.New("Seedance enhancement model is required")
	}
	item.Code = normalizeSeedanceCatalogCode(item.Code)
	item.DisplayName = strings.TrimSpace(item.DisplayName)
	item.ServiceCode = strings.TrimSpace(item.ServiceCode)
	item.QualityTier = strings.ToLower(strings.TrimSpace(item.QualityTier))
	item.SpecificationJSON = strings.TrimSpace(item.SpecificationJSON)
	item.SpecificationVersion = strings.TrimSpace(item.SpecificationVersion)
	if item.Code == "" || item.DisplayName == "" || item.ProviderID <= 0 || item.ServiceCode == "" || item.QualityTier == "" || item.SpecificationVersion == "" {
		return errors.New("Seedance enhancement model code, display_name, provider_id, service_code, quality_tier and specification_version are required")
	}
	if item.QualityTier != "standard" && item.QualityTier != "professional" {
		return errors.New("Seedance enhancement quality_tier must be standard or professional")
	}
	var specification map[string]any
	if err := common.UnmarshalJsonStr(item.SpecificationJSON, &specification); err != nil || specification == nil {
		return errors.New("Seedance enhancement specification must be a JSON object")
	}
	entries, err := ValidateSeedanceEnhancementCostMatrix(item.CostMatrixJSON)
	if err != nil {
		return err
	}
	encoded, err := common.Marshal(entries)
	if err != nil {
		return err
	}
	item.CostMatrixJSON = string(encoded)
	return nil
}

func seedanceBaseSnapshotChanged(before, after *SeedanceBaseModel) bool {
	return before.DisplayName != after.DisplayName || before.ProviderModelID != after.ProviderModelID || before.CostMatrixJSON != after.CostMatrixJSON
}

func seedanceEnhancementSnapshotChanged(before, after *SeedanceEnhancementModel) bool {
	return before.DisplayName != after.DisplayName || before.ProviderID != after.ProviderID || before.ServiceCode != after.ServiceCode ||
		before.QualityTier != after.QualityTier || before.SpecificationJSON != after.SpecificationJSON ||
		before.SpecificationVersion != after.SpecificationVersion || before.CostMatrixJSON != after.CostMatrixJSON
}

func SaveSeedanceBaseModel(item *SeedanceBaseModel, actorUserID int) error {
	if item != nil {
		if item.ID == 0 {
			item.Code = ensureSeedanceCatalogCode(item.Code, "base")
		} else if strings.TrimSpace(item.Code) == "" {
			var existing SeedanceBaseModel
			if err := DB.Select("code").First(&existing, item.ID).Error; err != nil {
				return err
			}
			item.Code = existing.Code
		}
	}
	if err := validateSeedanceBaseModel(item); err != nil {
		return err
	}
	now := time.Now().Unix()
	return DB.Transaction(func(tx *gorm.DB) error {
		beforeRevision := 0
		if item.ID > 0 {
			var existing SeedanceBaseModel
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&existing, item.ID).Error; err != nil {
				return err
			}
			if item.Code != existing.Code {
				return errors.New("Seedance base model code is immutable")
			}
			beforeRevision = existing.Revision
			if !seedanceBaseSnapshotChanged(&existing, item) {
				if err := tx.Model(&existing).Updates(map[string]any{
					"enabled": item.Enabled, "archived_at": item.ArchivedAt, "updated_at": now,
				}).Error; err != nil {
					return err
				}
				item.Revision = existing.Revision
				item.Current = existing.Current
				item.CreatedAt = existing.CreatedAt
				item.UpdatedAt = now
				return tx.Create(&SeedanceAdminAudit{
					ActorUserID: actorUserID, ResourceType: "BASE_MODEL", ResourceID: fmt.Sprintf("%d", item.ID),
					Action: "UPDATE_STATUS", BeforeVersion: fmt.Sprintf("%d", existing.Revision), AfterVersion: fmt.Sprintf("%d", existing.Revision),
					ChangeSummary: "updated Seedance base model status", CreatedAt: now,
				}).Error
			}
			if err := tx.Model(&existing).Updates(map[string]any{"current": false, "enabled": false, "updated_at": now}).Error; err != nil {
				return err
			}
			item.ID = 0
			item.Revision = existing.Revision + 1
		} else {
			var latest int
			if err := tx.Model(&SeedanceBaseModel{}).Where("code = ?", item.Code).Select("COALESCE(MAX(revision), 0)").Scan(&latest).Error; err != nil {
				return err
			}
			item.Revision = latest + 1
			if latest > 0 {
				if err := tx.Model(&SeedanceBaseModel{}).Where("code = ? AND current = ?", item.Code, true).
					Updates(map[string]any{"current": false, "enabled": false, "updated_at": now}).Error; err != nil {
					return err
				}
			}
		}
		item.Current = true
		item.CreatedAt = now
		item.UpdatedAt = now
		if err := tx.Create(item).Error; err != nil {
			return err
		}
		return tx.Create(&SeedanceAdminAudit{
			ActorUserID: actorUserID, ResourceType: "BASE_MODEL", ResourceID: fmt.Sprintf("%d", item.ID),
			Action: "UPSERT", BeforeVersion: fmt.Sprintf("%d", beforeRevision), AfterVersion: fmt.Sprintf("%d", item.Revision),
			ChangeSummary: "updated Seedance base model revision", CreatedAt: now,
		}).Error
	})
}

func SaveSeedanceEnhancementModel(item *SeedanceEnhancementModel, actorUserID int) error {
	if item != nil {
		if item.ID == 0 {
			item.Code = ensureSeedanceCatalogCode(item.Code, "enhancement")
		} else if strings.TrimSpace(item.Code) == "" {
			var existing SeedanceEnhancementModel
			if err := DB.Select("code").First(&existing, item.ID).Error; err != nil {
				return err
			}
			item.Code = existing.Code
		}
	}
	if err := validateSeedanceEnhancementModel(item); err != nil {
		return err
	}
	var provider MediaEnhancementProvider
	if err := DB.Where("id = ? AND status = ?", item.ProviderID, SeedanceConfigActive).First(&provider).Error; err != nil {
		return errors.New("active Seedance enhancement provider is required")
	}
	if provider.AdapterType == SeedanceAdapterVolcengineMediaKit {
		item.ServiceCode = SeedanceMediaKitServiceCode
	}
	now := time.Now().Unix()
	return DB.Transaction(func(tx *gorm.DB) error {
		beforeRevision := 0
		if item.ID > 0 {
			var existing SeedanceEnhancementModel
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&existing, item.ID).Error; err != nil {
				return err
			}
			if item.Code != existing.Code {
				return errors.New("Seedance enhancement model code is immutable")
			}
			beforeRevision = existing.Revision
			if !seedanceEnhancementSnapshotChanged(&existing, item) {
				if err := tx.Model(&existing).Updates(map[string]any{
					"enabled": item.Enabled, "archived_at": item.ArchivedAt, "updated_at": now,
				}).Error; err != nil {
					return err
				}
				item.Revision = existing.Revision
				item.Current = existing.Current
				item.CreatedAt = existing.CreatedAt
				item.UpdatedAt = now
				return tx.Create(&SeedanceAdminAudit{
					ActorUserID: actorUserID, ResourceType: "ENHANCEMENT_MODEL", ResourceID: fmt.Sprintf("%d", item.ID),
					Action: "UPDATE_STATUS", BeforeVersion: fmt.Sprintf("%d", existing.Revision), AfterVersion: fmt.Sprintf("%d", existing.Revision),
					ChangeSummary: "updated Seedance enhancement model status", CreatedAt: now,
				}).Error
			}
			if err := tx.Model(&existing).Updates(map[string]any{"current": false, "enabled": false, "updated_at": now}).Error; err != nil {
				return err
			}
			item.ID = 0
			item.Revision = existing.Revision + 1
		} else {
			var latest int
			if err := tx.Model(&SeedanceEnhancementModel{}).Where("code = ?", item.Code).Select("COALESCE(MAX(revision), 0)").Scan(&latest).Error; err != nil {
				return err
			}
			item.Revision = latest + 1
			if latest > 0 {
				if err := tx.Model(&SeedanceEnhancementModel{}).Where("code = ? AND current = ?", item.Code, true).
					Updates(map[string]any{"current": false, "enabled": false, "updated_at": now}).Error; err != nil {
					return err
				}
			}
		}
		item.Current = true
		item.CreatedAt = now
		item.UpdatedAt = now
		if err := tx.Create(item).Error; err != nil {
			return err
		}
		return tx.Create(&SeedanceAdminAudit{
			ActorUserID: actorUserID, ResourceType: "ENHANCEMENT_MODEL", ResourceID: fmt.Sprintf("%d", item.ID),
			Action: "UPSERT", BeforeVersion: fmt.Sprintf("%d", beforeRevision), AfterVersion: fmt.Sprintf("%d", item.Revision),
			ChangeSummary: "updated Seedance enhancement model revision", CreatedAt: now,
		}).Error
	})
}

func ListCurrentSeedanceBaseModels(includeArchived bool) ([]SeedanceBaseModel, error) {
	query := DB.Where("current = ?", true)
	if !includeArchived {
		query = query.Where("archived_at = ?", 0)
	}
	var items []SeedanceBaseModel
	err := query.Order("id desc").Find(&items).Error
	return items, err
}

func ListCurrentSeedanceEnhancementModels(includeArchived bool) ([]SeedanceEnhancementModel, error) {
	query := DB.Where("current = ?", true)
	if !includeArchived {
		query = query.Where("archived_at = ?", 0)
	}
	var items []SeedanceEnhancementModel
	err := query.Order("id desc").Find(&items).Error
	return items, err
}

func GetSeedanceBaseModelForExecution(id int64) (*SeedanceBaseModel, error) {
	var item SeedanceBaseModel
	if err := DB.First(&item, id).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

func GetSeedanceEnhancementModelForExecution(id int64) (*SeedanceEnhancementModel, error) {
	var item SeedanceEnhancementModel
	if err := DB.First(&item, id).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

func ArchiveSeedanceCatalogModel(kind string, id int64, actorUserID int) error {
	if id <= 0 {
		return errors.New("Seedance catalog model id is required")
	}
	now := time.Now().Unix()
	return DB.Transaction(func(tx *gorm.DB) error {
		var result *gorm.DB
		resourceType := ""
		switch strings.ToLower(strings.TrimSpace(kind)) {
		case "base":
			result = tx.Model(&SeedanceBaseModel{}).Where("id = ? AND current = ?", id, true).
				Updates(map[string]any{"enabled": false, "archived_at": now, "updated_at": now})
			resourceType = "BASE_MODEL"
		case "enhancement":
			result = tx.Model(&SeedanceEnhancementModel{}).Where("id = ? AND current = ?", id, true).
				Updates(map[string]any{"enabled": false, "archived_at": now, "updated_at": now})
			resourceType = "ENHANCEMENT_MODEL"
		default:
			return errors.New("unsupported Seedance catalog model kind")
		}
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return tx.Create(&SeedanceAdminAudit{
			ActorUserID: actorUserID, ResourceType: resourceType, ResourceID: fmt.Sprintf("%d", id),
			Action: "ARCHIVE", ChangeSummary: "archived Seedance catalog model", CreatedAt: now,
		}).Error
	})
}

// BackfillSeedanceThreeLayerCatalog maps each legacy offering into immutable
// base/enhancement revisions and connects the existing row as the sale model.
// Legacy amount fields did not carry an explicit unit, so migrated rows remain
// published but are marked for Root review.
func BackfillSeedanceThreeLayerCatalog() error {
	if !DB.Migrator().HasTable(&SeedanceBaseModel{}) || !DB.Migrator().HasTable(&SeedanceEnhancementModel{}) ||
		!DB.Migrator().HasColumn(&SeedanceModelOffering{}, "base_model_id") {
		return nil
	}
	var offerings []SeedanceModelOffering
	if err := DB.Where("base_model_id = ?", 0).Order("id asc").Find(&offerings).Error; err != nil {
		return err
	}
	for i := range offerings {
		offering := offerings[i]
		if err := DB.Transaction(func(tx *gorm.DB) error {
			var locked SeedanceModelOffering
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&locked, offering.ID).Error; err != nil {
				return err
			}
			if locked.BaseModelID != 0 {
				return nil
			}
			sourceResolution := "720p"
			targetResolution := sourceResolution
			var spec struct {
				Resolution       string `json:"resolution"`
				TargetResolution string `json:"target_resolution"`
			}
			_ = common.UnmarshalJsonStr(locked.EnhancementSpecificationJSON, &spec)
			requestedTarget := spec.TargetResolution
			if strings.TrimSpace(requestedTarget) == "" {
				requestedTarget = spec.Resolution
			}
			if normalized, err := NormalizeSeedanceResolution(requestedTarget); err == nil {
				targetResolution = normalized
			}
			baseCosts, _ := common.Marshal([]SeedanceBaseCostEntry{
				{SourceResolution: sourceResolution, HasReferenceVideo: false, CostMicroRMBPerSecond: locked.VolcengineUnitCostMicroRMB},
				{SourceResolution: sourceResolution, HasReferenceVideo: true, CostMicroRMBPerSecond: locked.VolcengineUnitCostMicroRMB},
			})
			base := SeedanceBaseModel{
				Code: fmt.Sprintf("legacy-base-%d-%d", locked.ChannelID, locked.ID), Revision: 1,
				DisplayName: locked.DisplayName + " 本体", ProviderModelID: locked.ProviderModelID,
				CostMatrixJSON: string(baseCosts), Enabled: locked.Enabled, Current: true,
				CreatedAt: locked.CreatedAt, UpdatedAt: time.Now().Unix(),
			}
			if err := tx.Create(&base).Error; err != nil {
				return err
			}
			var enhancementID *int64
			if locked.EnhancementProviderID > 0 {
				enhanceCosts, _ := common.Marshal([]SeedanceEnhancementCostEntry{{
					TargetResolution: targetResolution, FPSBucket: SeedanceFPSBucketUpTo30,
					CostMicroRMBPerSecond: locked.ServiceChargeMicroRMB,
				}})
				enhance := SeedanceEnhancementModel{
					Code: fmt.Sprintf("legacy-enhancement-%d-%d", locked.ChannelID, locked.ID), Revision: 1,
					DisplayName: locked.DisplayName + " 超分", ProviderID: locked.EnhancementProviderID,
					ServiceCode: locked.EnhancementServiceCode, QualityTier: "standard",
					SpecificationJSON:    locked.EnhancementSpecificationJSON,
					SpecificationVersion: locked.EnhancementSpecificationVersion,
					CostMatrixJSON:       string(enhanceCosts), Enabled: locked.Enabled, Current: true,
					CreatedAt: locked.CreatedAt, UpdatedAt: time.Now().Unix(),
				}
				if strings.TrimSpace(enhance.SpecificationJSON) == "" {
					enhance.SpecificationJSON = "{}"
				}
				if strings.TrimSpace(enhance.SpecificationVersion) == "" {
					enhance.SpecificationVersion = "legacy"
				}
				if err := tx.Create(&enhance).Error; err != nil {
					return err
				}
				enhancementID = &enhance.ID
			}
			return tx.Model(&SeedanceModelOffering{}).Where("id = ? AND base_model_id = ?", locked.ID, 0).
				Updates(map[string]any{
					"base_model_id": base.ID, "enhancement_model_id": enhancementID,
					"source_resolution": sourceResolution, "target_resolution": targetResolution,
					"output_fps": 24, "no_reference_unit_price_micro_rmb": locked.ModelSaleMicroRMB,
					"reference_unit_price_micro_rmb": locked.ModelSaleMicroRMB,
					"migration_needs_review":         true, "updated_at": time.Now().Unix(),
				}).Error
		}); err != nil {
			return err
		}
	}
	return nil
}
