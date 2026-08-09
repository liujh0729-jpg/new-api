package model

import (
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	VirtualCharacterAssetTypeImage = "Image"
	VirtualCharacterAssetTypeVideo = "Video"
	VirtualCharacterAssetTypeAudio = "Audio"

	VirtualCharacterAssetStatusProcessing = "Processing"
	VirtualCharacterAssetStatusActive     = "Active"
	VirtualCharacterAssetStatusFailed     = "Failed"
	VirtualCharacterAssetStatusDeleting   = "Deleting"

	VirtualCharacterValidationPending   = "pending"
	VirtualCharacterValidationSucceeded = "succeeded"
	VirtualCharacterValidationFailed    = "failed"
	VirtualCharacterValidationExpired   = "expired"

	VirtualCharacterCleanupPending = "pending"
	VirtualCharacterCleanupRunning = "running"
	VirtualCharacterCleanupDone    = "done"
	VirtualCharacterCleanupFailed  = "failed"
)

type VirtualCharacterAsset struct {
	ID                int64          `json:"id" gorm:"primaryKey;autoIncrement"`
	CharacterID       int64          `json:"character_id" gorm:"index;uniqueIndex:uk_virtual_character_provider_asset"`
	ProviderAccountID int            `json:"provider_account_id" gorm:"index"`
	ProviderAssetID   string         `json:"provider_asset_id" gorm:"type:varchar(191);index;uniqueIndex:uk_virtual_character_provider_asset"`
	Name              string         `json:"name" gorm:"type:varchar(191)"`
	AssetType         string         `json:"asset_type" gorm:"type:varchar(16);index"`
	Status            string         `json:"status" gorm:"type:varchar(20);index"`
	IsPrimary         bool           `json:"is_primary" gorm:"index"`
	CoverURL          string         `json:"cover_url,omitempty" gorm:"type:text"`
	StagingFileID     string         `json:"-" gorm:"type:varchar(191);index"`
	MimeType          string         `json:"mime_type,omitempty" gorm:"type:varchar(100)"`
	FileSize          int64          `json:"file_size,omitempty"`
	LastError         string         `json:"last_error,omitempty" gorm:"type:text"`
	PollAttempts      int            `json:"-"`
	NextPollAt        int64          `json:"-" gorm:"index"`
	CreatedAt         int64          `json:"created_at" gorm:"autoCreateTime;index"`
	UpdatedAt         int64          `json:"updated_at" gorm:"autoUpdateTime"`
	DeletedAt         gorm.DeletedAt `json:"-" gorm:"index"`
}

type VirtualCharacterValidationSession struct {
	ID                  string `json:"id" gorm:"type:varchar(64);primaryKey"`
	UserID              int    `json:"user_id" gorm:"index"`
	ProviderAccountID   int    `json:"provider_account_id" gorm:"index"`
	Status              string `json:"status" gorm:"type:varchar(20);index"`
	StateHash           string `json:"-" gorm:"type:varchar(64);uniqueIndex"`
	EncryptedBytedToken string `json:"-" gorm:"type:text"`
	EncryptedH5Link     string `json:"-" gorm:"type:text"`
	Name                string `json:"name" gorm:"type:varchar(191)"`
	Description         string `json:"description" gorm:"type:text"`
	TagsJSON            string `json:"-" gorm:"type:text"`
	Language            string `json:"language" gorm:"type:varchar(16)"`
	ProviderGroupID     string `json:"provider_group_id,omitempty" gorm:"type:varchar(191);index"`
	CharacterID         int64  `json:"character_id,omitempty" gorm:"index"`
	ResultCode          string `json:"result_code,omitempty" gorm:"type:varchar(64)"`
	LastError           string `json:"last_error,omitempty" gorm:"type:text"`
	ExpiresAt           int64  `json:"expires_at" gorm:"index"`
	ConsumedAt          int64  `json:"consumed_at,omitempty" gorm:"index"`
	CreatedAt           int64  `json:"created_at" gorm:"autoCreateTime;index"`
	UpdatedAt           int64  `json:"updated_at" gorm:"autoUpdateTime"`
}

type VirtualCharacterProviderAccount struct {
	ID                 int    `json:"id" gorm:"primaryKey;autoIncrement"`
	Enabled            bool   `json:"enabled" gorm:"index"`
	OfficialEnabled    bool   `json:"official_enabled"`
	RealPersonEnabled  bool   `json:"real_person_enabled"`
	EncryptedAccessKey string `json:"-" gorm:"type:text"`
	EncryptedSecretKey string `json:"-" gorm:"type:text"`
	Region             string `json:"region" gorm:"type:varchar(64)"`
	ProjectName        string `json:"project_name" gorm:"type:varchar(191)"`
	ChannelID          int    `json:"channel_id" gorm:"index"`
	LastCheckStatus    string `json:"last_check_status,omitempty" gorm:"type:varchar(20)"`
	LastCheckError     string `json:"last_check_error,omitempty" gorm:"type:text"`
	LastCheckedAt      int64  `json:"last_checked_at,omitempty"`
	CreatedAt          int64  `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt          int64  `json:"updated_at" gorm:"autoUpdateTime"`
}

type VirtualCharacterCatalogImport struct {
	ID             int64  `json:"id" gorm:"primaryKey;autoIncrement"`
	Version        string `json:"version" gorm:"type:varchar(191);index"`
	ContentHash    string `json:"content_hash" gorm:"type:varchar(64);index"`
	Status         string `json:"status" gorm:"type:varchar(20);index"`
	Total          int    `json:"total"`
	Created        int    `json:"created"`
	Updated        int    `json:"updated"`
	Offlined       int    `json:"offlined"`
	OperatorUserID int    `json:"operator_user_id" gorm:"index"`
	LastError      string `json:"last_error,omitempty" gorm:"type:text"`
	CreatedAt      int64  `json:"created_at" gorm:"autoCreateTime;index"`
}

type VirtualCharacterCleanupJob struct {
	ID                int64  `json:"id" gorm:"primaryKey;autoIncrement"`
	CharacterID       int64  `json:"character_id,omitempty" gorm:"index"`
	AssetID           int64  `json:"asset_id,omitempty" gorm:"index"`
	ProviderAccountID int    `json:"provider_account_id,omitempty" gorm:"index"`
	TargetType        string `json:"target_type" gorm:"type:varchar(32);index"`
	TargetID          string `json:"target_id" gorm:"type:varchar(191);index"`
	SecondaryTargetID string `json:"secondary_target_id,omitempty" gorm:"type:varchar(191)"`
	Status            string `json:"status" gorm:"type:varchar(20);index"`
	Attempts          int    `json:"attempts"`
	NextAttemptAt     int64  `json:"next_attempt_at" gorm:"index"`
	LastError         string `json:"last_error,omitempty" gorm:"type:text"`
	CreatedAt         int64  `json:"created_at" gorm:"autoCreateTime;index"`
	UpdatedAt         int64  `json:"updated_at" gorm:"autoUpdateTime"`
}

type VirtualCharacterCatalogEntry struct {
	AssetID     string
	Name        string
	Description string
	TagsJSON    string
	CoverURL    string
	Enabled     bool
}

type VirtualCharacterCatalogStats struct {
	Total    int `json:"total"`
	Created  int `json:"created"`
	Updated  int `json:"updated"`
	Offlined int `json:"offlined"`
}

func GetEnabledVirtualCharacterProviderAccount() (*VirtualCharacterProviderAccount, error) {
	var account VirtualCharacterProviderAccount
	err := DB.Where("enabled = ?", true).Order("id ASC").First(&account).Error
	return &account, err
}

func GetVirtualCharacterProviderAccount() (*VirtualCharacterProviderAccount, error) {
	var account VirtualCharacterProviderAccount
	err := DB.Order("id ASC").First(&account).Error
	return &account, err
}

func SaveVirtualCharacterProviderAccount(account *VirtualCharacterProviderAccount) error {
	if account == nil {
		return errors.New("provider account is required")
	}
	if strings.TrimSpace(account.Region) == "" {
		account.Region = "cn-beijing"
	}
	if strings.TrimSpace(account.ProjectName) == "" {
		account.ProjectName = "default"
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		if account.Enabled {
			if err := tx.Model(&VirtualCharacterProviderAccount{}).Where("id <> ?", account.ID).Update("enabled", false).Error; err != nil {
				return err
			}
		}
		return tx.Save(account).Error
	})
}

func ListVirtualCharacterAssets(characterID int64, includeDeleting bool) ([]VirtualCharacterAsset, error) {
	query := DB.Where("character_id = ?", characterID)
	if !includeDeleting {
		query = query.Where("status <> ?", VirtualCharacterAssetStatusDeleting)
	}
	var assets []VirtualCharacterAsset
	err := query.Order("is_primary DESC, id ASC").Find(&assets).Error
	return assets, err
}

func GetVirtualCharacterAssetForUser(characterID, assetID int64, userID int) (*VirtualCharacterAsset, *VirtualCharacter, error) {
	character, err := GetAccessibleVirtualCharacter(characterID, userID)
	if err != nil {
		return nil, nil, err
	}
	var asset VirtualCharacterAsset
	query := DB.Where("character_id = ?", characterID)
	if assetID > 0 {
		query = query.Where("id = ?", assetID)
	} else if character.PrimaryAssetID != nil {
		query = query.Where("id = ?", *character.PrimaryAssetID)
	} else {
		query = query.Where("is_primary = ?", true)
	}
	if err := query.First(&asset).Error; err != nil {
		return nil, nil, err
	}
	return &asset, character, nil
}

func CreateVirtualCharacterAsset(asset *VirtualCharacterAsset) error {
	if asset == nil || asset.CharacterID <= 0 || strings.TrimSpace(asset.ProviderAssetID) == "" {
		return errors.New("invalid virtual character asset")
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&VirtualCharacterAsset{}).Where("character_id = ? AND deleted_at IS NULL", asset.CharacterID).Count(&count).Error; err != nil {
			return err
		}
		asset.IsPrimary = count == 0
		if err := tx.Create(asset).Error; err != nil {
			return err
		}
		if asset.IsPrimary {
			return tx.Model(&VirtualCharacter{}).Where("id = ?", asset.CharacterID).Updates(map[string]any{"primary_asset_id": asset.ID, "updated_at": time.Now().Unix()}).Error
		}
		return nil
	})
}

func SetVirtualCharacterPrimaryAsset(characterID, assetID int64, userID int) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		var character VirtualCharacter
		if err := tx.Where("id = ? AND scope = ? AND user_id = ?", characterID, VirtualCharacterScopePrivate, userID).First(&character).Error; err != nil {
			return err
		}
		var asset VirtualCharacterAsset
		if err := tx.Where("id = ? AND character_id = ? AND status = ?", assetID, characterID, VirtualCharacterAssetStatusActive).First(&asset).Error; err != nil {
			return err
		}
		if err := tx.Model(&VirtualCharacterAsset{}).Where("character_id = ?", characterID).Update("is_primary", false).Error; err != nil {
			return err
		}
		if err := tx.Model(&asset).Update("is_primary", true).Error; err != nil {
			return err
		}
		return tx.Model(&character).Updates(map[string]any{"primary_asset_id": assetID, "cover_url": asset.CoverURL, "updated_at": time.Now().Unix()}).Error
	})
}

func MarkVirtualCharacterAssetTerminal(assetID int64, status, reason string) error {
	if status != VirtualCharacterAssetStatusActive && status != VirtualCharacterAssetStatusFailed {
		return errors.New("invalid asset terminal status")
	}
	return DB.Model(&VirtualCharacterAsset{}).Where("id = ?", assetID).Updates(map[string]any{
		"status": status, "last_error": reason, "staging_file_id": "", "next_poll_at": 0, "updated_at": time.Now().Unix(),
	}).Error
}

func ListVirtualCharacterAssetsToPoll(now int64, limit int) ([]VirtualCharacterAsset, error) {
	var assets []VirtualCharacterAsset
	err := DB.Where("status = ? AND next_poll_at <= ?", VirtualCharacterAssetStatusProcessing, now).
		Order("next_poll_at ASC, id ASC").Limit(limit).Find(&assets).Error
	return assets, err
}

func RetryVirtualCharacterAssetPoll(assetID int64, attempts int, nextAt int64, reason string) error {
	return DB.Model(&VirtualCharacterAsset{}).Where("id = ?", assetID).Updates(map[string]any{
		"poll_attempts": attempts, "next_poll_at": nextAt, "last_error": reason, "updated_at": time.Now().Unix(),
	}).Error
}

func CreateVirtualCharacterValidationSession(session *VirtualCharacterValidationSession) error {
	if session == nil || strings.TrimSpace(session.ID) == "" || session.UserID <= 0 || strings.TrimSpace(session.StateHash) == "" {
		return errors.New("invalid validation session")
	}
	if session.Status == "" {
		session.Status = VirtualCharacterValidationPending
	}
	return DB.Create(session).Error
}

func GetOwnedVirtualCharacterValidationSession(id string, userID int) (*VirtualCharacterValidationSession, error) {
	var item VirtualCharacterValidationSession
	err := DB.Where("id = ? AND user_id = ?", strings.TrimSpace(id), userID).First(&item).Error
	if err == nil && item.Status == VirtualCharacterValidationPending && item.ExpiresAt <= time.Now().Unix() {
		_ = DB.Model(&item).Updates(map[string]any{"status": VirtualCharacterValidationExpired, "last_error": "validation session expired", "updated_at": time.Now().Unix()}).Error
		item.Status = VirtualCharacterValidationExpired
		item.LastError = "validation session expired"
	}
	return &item, err
}

func GetVirtualCharacterValidationSessionByStateHash(stateHash string) (*VirtualCharacterValidationSession, error) {
	var item VirtualCharacterValidationSession
	err := DB.Where("state_hash = ?", stateHash).First(&item).Error
	return &item, err
}

func MarkVirtualCharacterValidationFailed(id, resultCode, reason string) error {
	return DB.Model(&VirtualCharacterValidationSession{}).Where("id = ? AND status = ?", id, VirtualCharacterValidationPending).Updates(map[string]any{
		"status": VirtualCharacterValidationFailed, "result_code": resultCode, "last_error": reason, "consumed_at": time.Now().Unix(), "updated_at": time.Now().Unix(),
	}).Error
}

func MarkVirtualCharacterValidationExpired(id string) error {
	return DB.Model(&VirtualCharacterValidationSession{}).Where("id = ? AND status = ?", id, VirtualCharacterValidationPending).Updates(map[string]any{
		"status": VirtualCharacterValidationExpired, "result_code": "expired", "last_error": "validation session expired", "consumed_at": time.Now().Unix(), "updated_at": time.Now().Unix(),
	}).Error
}

func CompleteVirtualCharacterValidation(id, providerGroupID string) (*VirtualCharacter, error) {
	var character *VirtualCharacter
	err := DB.Transaction(func(tx *gorm.DB) error {
		var session VirtualCharacterValidationSession
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", id).First(&session).Error; err != nil {
			return err
		}
		if session.Status == VirtualCharacterValidationSucceeded && session.CharacterID > 0 {
			var existing VirtualCharacter
			if err := tx.First(&existing, session.CharacterID).Error; err != nil {
				return err
			}
			character = &existing
			return nil
		}
		if session.Status != VirtualCharacterValidationPending || session.ExpiresAt <= time.Now().Unix() {
			return errors.New("validation session is no longer pending")
		}
		limit := getVirtualCharacterEffectiveLimitDB(tx, session.UserID)
		var created *VirtualCharacter
		for slot := 1; slot <= limit; slot++ {
			candidateSlot := slot
			item := &VirtualCharacter{UserID: session.UserID, Slot: &candidateSlot, Scope: VirtualCharacterScopePrivate, Name: session.Name, Description: session.Description, TagsJSON: session.TagsJSON, SourceType: VirtualCharacterSourceVolcRealPerson, Status: VirtualCharacterStatusActive, ValidationStatus: VirtualCharacterValidationAccepted, ProviderAccountID: session.ProviderAccountID, ProviderGroupID: providerGroupID}
			result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(item)
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected == 1 {
				created = item
				break
			}
		}
		if created == nil {
			return errors.New("virtual character limit reached")
		}
		if err := tx.Model(&session).Updates(map[string]any{"status": VirtualCharacterValidationSucceeded, "provider_group_id": providerGroupID, "character_id": created.ID, "result_code": "10000", "last_error": "", "consumed_at": time.Now().Unix(), "encrypted_h5_link": "", "updated_at": time.Now().Unix()}).Error; err != nil {
			return err
		}
		character = created
		return nil
	})
	return character, err
}

func BeginVirtualCharacterAssetDelete(characterID, assetID int64, userID int) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		var character VirtualCharacter
		if err := tx.Where("id = ? AND scope = ? AND user_id = ?", characterID, VirtualCharacterScopePrivate, userID).First(&character).Error; err != nil {
			return err
		}
		var asset VirtualCharacterAsset
		if err := tx.Where("id = ? AND character_id = ?", assetID, characterID).First(&asset).Error; err != nil {
			return err
		}
		now := time.Now().Unix()
		if err := tx.Model(&asset).Updates(map[string]any{"status": VirtualCharacterAssetStatusDeleting, "is_primary": false, "updated_at": now}).Error; err != nil {
			return err
		}
		if err := tx.Create(&VirtualCharacterCleanupJob{CharacterID: characterID, AssetID: asset.ID, ProviderAccountID: asset.ProviderAccountID, TargetType: "volc_asset", TargetID: asset.ProviderAssetID, Status: VirtualCharacterCleanupPending, NextAttemptAt: now}).Error; err != nil {
			return err
		}
		if character.PrimaryAssetID != nil && *character.PrimaryAssetID == asset.ID {
			var replacement VirtualCharacterAsset
			result := tx.Where("character_id = ? AND id <> ? AND status = ?", characterID, asset.ID, VirtualCharacterAssetStatusActive).Order("id ASC").First(&replacement)
			updates := map[string]any{"primary_asset_id": nil, "cover_url": "", "updated_at": now}
			if result.Error == nil {
				if err := tx.Model(&replacement).Update("is_primary", true).Error; err != nil {
					return err
				}
				updates["primary_asset_id"] = replacement.ID
				updates["cover_url"] = replacement.CoverURL
			} else if !errors.Is(result.Error, gorm.ErrRecordNotFound) {
				return result.Error
			}
			if err := tx.Model(&character).Updates(updates).Error; err != nil {
				return err
			}
		}
		var remaining int64
		if err := tx.Model(&VirtualCharacterAsset{}).Where("character_id = ? AND id <> ? AND status <> ?", characterID, asset.ID, VirtualCharacterAssetStatusDeleting).Count(&remaining).Error; err != nil {
			return err
		}
		if remaining == 0 {
			if err := tx.Model(&character).Updates(map[string]any{"status": VirtualCharacterStatusDeleting, "slot": nil, "primary_asset_id": nil, "updated_at": now}).Error; err != nil {
				return err
			}
			if strings.TrimSpace(character.ProviderGroupID) != "" {
				if err := tx.Create(&VirtualCharacterCleanupJob{CharacterID: characterID, ProviderAccountID: character.ProviderAccountID, TargetType: "volc_group", TargetID: character.ProviderGroupID, Status: VirtualCharacterCleanupPending, NextAttemptAt: now + 5}).Error; err != nil {
					return err
				}
			}
		}
		return nil
	})
}

func BeginVirtualCharacterGroupDelete(characterID int64, userID int) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		var character VirtualCharacter
		if err := tx.Where("id = ? AND scope = ? AND user_id = ?", characterID, VirtualCharacterScopePrivate, userID).First(&character).Error; err != nil {
			return err
		}
		now := time.Now().Unix()
		if err := tx.Model(&character).Updates(map[string]any{"status": VirtualCharacterStatusDeleting, "slot": nil, "primary_asset_id": nil, "updated_at": now}).Error; err != nil {
			return err
		}
		var assets []VirtualCharacterAsset
		if err := tx.Where("character_id = ?", characterID).Find(&assets).Error; err != nil {
			return err
		}
		for i := range assets {
			asset := &assets[i]
			if asset.Status != VirtualCharacterAssetStatusDeleting {
				if err := tx.Model(asset).Updates(map[string]any{"status": VirtualCharacterAssetStatusDeleting, "is_primary": false, "updated_at": now}).Error; err != nil {
					return err
				}
			}
			if strings.TrimSpace(asset.ProviderAssetID) != "" {
				if err := tx.Create(&VirtualCharacterCleanupJob{CharacterID: characterID, AssetID: asset.ID, ProviderAccountID: asset.ProviderAccountID, TargetType: "volc_asset", TargetID: asset.ProviderAssetID, Status: VirtualCharacterCleanupPending, NextAttemptAt: now}).Error; err != nil {
					return err
				}
			}
		}
		if strings.TrimSpace(character.ProviderGroupID) != "" {
			if err := tx.Create(&VirtualCharacterCleanupJob{CharacterID: characterID, ProviderAccountID: character.ProviderAccountID, TargetType: "volc_group", TargetID: character.ProviderGroupID, Status: VirtualCharacterCleanupPending, NextAttemptAt: now + 5}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func ApplyVirtualCharacterCatalog(version, contentHash string, entries []VirtualCharacterCatalogEntry, operatorUserID, providerAccountID int) (*VirtualCharacterCatalogStats, error) {
	stats := &VirtualCharacterCatalogStats{Total: len(entries)}
	err := DB.Transaction(func(tx *gorm.DB) error {
		var previouslyActive []VirtualCharacter
		if err := tx.Where("scope = ? AND source_type = ? AND status = ?", VirtualCharacterScopePublic, VirtualCharacterSourceVolcPreset, VirtualCharacterStatusActive).Find(&previouslyActive).Error; err != nil {
			return err
		}
		offlinedCandidates := make(map[string]struct{}, len(previouslyActive))
		for i := range previouslyActive {
			var activeAsset VirtualCharacterAsset
			query := tx.Where("character_id = ?", previouslyActive[i].ID)
			if previouslyActive[i].PrimaryAssetID != nil {
				query = query.Where("id = ?", *previouslyActive[i].PrimaryAssetID)
			} else {
				query = query.Where("is_primary = ?", true)
			}
			if err := query.First(&activeAsset).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			if strings.TrimSpace(activeAsset.ProviderAssetID) != "" {
				offlinedCandidates[activeAsset.ProviderAssetID] = struct{}{}
			}
		}
		if err := tx.Model(&VirtualCharacter{}).Where("scope = ? AND source_type = ?", VirtualCharacterScopePublic, VirtualCharacterSourceVolcPreset).Updates(map[string]any{"status": VirtualCharacterStatusOffline, "updated_at": time.Now().Unix()}).Error; err != nil {
			return err
		}
		for _, entry := range entries {
			assetID := strings.TrimPrefix(strings.TrimSpace(entry.AssetID), "asset://")
			var character VirtualCharacter
			characterIDs := tx.Model(&VirtualCharacterAsset{}).Select("character_id").Where("provider_asset_id = ?", assetID)
			err := tx.Where("scope = ? AND source_type = ? AND id IN (?)", VirtualCharacterScopePublic, VirtualCharacterSourceVolcPreset, characterIDs).First(&character).Error
			status := VirtualCharacterStatusOffline
			if entry.Enabled {
				status = VirtualCharacterStatusActive
				delete(offlinedCandidates, assetID)
			}
			if errors.Is(err, gorm.ErrRecordNotFound) {
				character = VirtualCharacter{UserID: 0, Scope: VirtualCharacterScopePublic, Name: entry.Name, Description: entry.Description, TagsJSON: entry.TagsJSON, SourceType: VirtualCharacterSourceVolcPreset, Status: status, ValidationStatus: VirtualCharacterValidationAccepted, CoverURL: entry.CoverURL, ProviderAccountID: providerAccountID, CatalogVersion: version}
				if err := tx.Create(&character).Error; err != nil {
					return err
				}
				asset := VirtualCharacterAsset{CharacterID: character.ID, ProviderAccountID: providerAccountID, ProviderAssetID: assetID, Name: entry.Name, AssetType: VirtualCharacterAssetTypeImage, Status: VirtualCharacterAssetStatusActive, IsPrimary: true, CoverURL: entry.CoverURL}
				if err := tx.Create(&asset).Error; err != nil {
					return err
				}
				if err := tx.Model(&character).Update("primary_asset_id", asset.ID).Error; err != nil {
					return err
				}
				stats.Created++
				continue
			}
			if err != nil {
				return err
			}
			if err := tx.Model(&character).Updates(map[string]any{"name": entry.Name, "description": entry.Description, "tags_json": entry.TagsJSON, "cover_url": entry.CoverURL, "status": status, "validation_status": VirtualCharacterValidationAccepted, "provider_account_id": providerAccountID, "catalog_version": version, "updated_at": time.Now().Unix()}).Error; err != nil {
				return err
			}
			assetUpdate := tx.Model(&VirtualCharacterAsset{}).Where("character_id = ? AND provider_asset_id = ?", character.ID, assetID).Updates(map[string]any{"name": entry.Name, "cover_url": entry.CoverURL, "status": VirtualCharacterAssetStatusActive, "updated_at": time.Now().Unix()})
			if assetUpdate.Error != nil {
				return assetUpdate.Error
			}
			if assetUpdate.RowsAffected == 0 {
				asset := VirtualCharacterAsset{CharacterID: character.ID, ProviderAccountID: providerAccountID, ProviderAssetID: assetID, Name: entry.Name, AssetType: VirtualCharacterAssetTypeImage, Status: VirtualCharacterAssetStatusActive, IsPrimary: true, CoverURL: entry.CoverURL}
				if err := tx.Create(&asset).Error; err != nil {
					return err
				}
				if err := tx.Model(&character).Update("primary_asset_id", asset.ID).Error; err != nil {
					return err
				}
			}
			stats.Updated++
		}
		stats.Offlined = len(offlinedCandidates)
		return tx.Create(&VirtualCharacterCatalogImport{Version: version, ContentHash: contentHash, Status: "succeeded", Total: stats.Total, Created: stats.Created, Updated: stats.Updated, Offlined: stats.Offlined, OperatorUserID: operatorUserID}).Error
	})
	return stats, err
}

func GetLatestVirtualCharacterCatalogImport() (*VirtualCharacterCatalogImport, error) {
	var item VirtualCharacterCatalogImport
	err := DB.Order("id DESC").First(&item).Error
	return &item, err
}

func CreateVirtualCharacterCleanupJob(job *VirtualCharacterCleanupJob) error {
	if job == nil || strings.TrimSpace(job.TargetType) == "" || strings.TrimSpace(job.TargetID) == "" {
		return errors.New("invalid cleanup job")
	}
	if job.Status == "" {
		job.Status = VirtualCharacterCleanupPending
	}
	if job.NextAttemptAt == 0 {
		job.NextAttemptAt = time.Now().Unix()
	}
	return DB.Create(job).Error
}

func ListVirtualCharacterCleanupJobs(now int64, limit int) ([]VirtualCharacterCleanupJob, error) {
	var jobs []VirtualCharacterCleanupJob
	err := DB.Where("status IN ? AND next_attempt_at <= ?", []string{VirtualCharacterCleanupPending, VirtualCharacterCleanupFailed}, now).
		Order("next_attempt_at ASC, id ASC").Limit(limit).Find(&jobs).Error
	return jobs, err
}

func CompleteVirtualCharacterCleanupJob(id int64) error {
	return DB.Model(&VirtualCharacterCleanupJob{}).Where("id = ?", id).Updates(map[string]any{"status": VirtualCharacterCleanupDone, "last_error": "", "updated_at": time.Now().Unix()}).Error
}

func RetryVirtualCharacterCleanupJob(id int64, attempts int, nextAt int64, reason string) error {
	return DB.Model(&VirtualCharacterCleanupJob{}).Where("id = ?", id).Updates(map[string]any{"status": VirtualCharacterCleanupFailed, "attempts": attempts, "next_attempt_at": nextAt, "last_error": reason, "updated_at": time.Now().Unix()}).Error
}

// MigrateVirtualCharacterABData converts public rows into offline actor groups
// with a single provider asset and irreversibly removes legacy fictional rows.
func MigrateVirtualCharacterABData() error {
	return DB.Transaction(func(tx *gorm.DB) error {
		var publicItems []VirtualCharacter
		if err := tx.Unscoped().Where("scope = ? AND source_type IN ?", VirtualCharacterScopePublic, []string{VirtualCharacterSourceVolc, ""}).Find(&publicItems).Error; err != nil {
			return err
		}
		for i := range publicItems {
			item := &publicItems[i]
			if err := tx.Model(item).Updates(map[string]any{"source_type": VirtualCharacterSourceVolcPreset, "status": VirtualCharacterStatusOffline, "updated_at": time.Now().Unix()}).Error; err != nil {
				return err
			}
			if strings.TrimSpace(item.VolcAssetID) == "" {
				continue
			}
			var count int64
			if err := tx.Model(&VirtualCharacterAsset{}).Where("character_id = ?", item.ID).Count(&count).Error; err != nil {
				return err
			}
			if count == 0 {
				asset := VirtualCharacterAsset{CharacterID: item.ID, ProviderAssetID: strings.TrimPrefix(item.VolcAssetID, "asset://"), Name: item.Name, AssetType: VirtualCharacterAssetTypeImage, Status: VirtualCharacterAssetStatusActive, IsPrimary: true, CoverURL: item.CoverURL}
				if err := tx.Create(&asset).Error; err != nil {
					return err
				}
				if err := tx.Model(item).Update("primary_asset_id", asset.ID).Error; err != nil {
					return err
				}
			}
		}
		var privateItems []VirtualCharacter
		if err := tx.Unscoped().Where("scope = ? AND source_type IN ?", VirtualCharacterScopePrivate, []string{VirtualCharacterSourceAIPDD, ""}).Find(&privateItems).Error; err != nil {
			return err
		}
		for i := range privateItems {
			item := &privateItems[i]
			if item.AIPDDAssetID > 0 {
				if err := tx.Create(&VirtualCharacterCleanupJob{CharacterID: item.ID, TargetType: "aipdd_asset", TargetID: strings.TrimSpace(stringInt64(item.AIPDDAssetID)), Status: VirtualCharacterCleanupPending, NextAttemptAt: time.Now().Unix()}).Error; err != nil {
					return err
				}
			}
			if strings.TrimSpace(item.AIPDDFileID) != "" {
				if err := tx.Create(&VirtualCharacterCleanupJob{CharacterID: item.ID, TargetType: "aipdd_file", TargetID: item.AIPDDFileID, Status: VirtualCharacterCleanupPending, NextAttemptAt: time.Now().Unix()}).Error; err != nil {
					return err
				}
			}
			if err := tx.Unscoped().Delete(&VirtualCharacter{}, item.ID).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func stringInt64(value int64) string {
	if value == 0 {
		return "0"
	}
	negative := value < 0
	if negative {
		value = -value
	}
	buf := make([]byte, 0, 20)
	for value > 0 {
		buf = append(buf, byte('0'+value%10))
		value /= 10
	}
	if negative {
		buf = append(buf, '-')
	}
	for left, right := 0, len(buf)-1; left < right; left, right = left+1, right-1 {
		buf[left], buf[right] = buf[right], buf[left]
	}
	return string(buf)
}

func UpsertVirtualCharacterProviderAccount(account *VirtualCharacterProviderAccount) error {
	return DB.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "id"}}, DoUpdates: clause.AssignmentColumns([]string{"enabled", "official_enabled", "real_person_enabled", "encrypted_access_key", "encrypted_secret_key", "region", "project_name", "channel_id", "updated_at"})}).Create(account).Error
}
