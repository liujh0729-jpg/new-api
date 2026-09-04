package model

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	VirtualCharacterValidationPending   = "pending"
	VirtualCharacterValidationSucceeded = "succeeded"
	VirtualCharacterValidationFailed    = "failed"
	VirtualCharacterValidationExpired   = "expired"
	VirtualCharacterValidationCancelled = "cancelled"

	VirtualCharacterCleanupPending = "pending"
	VirtualCharacterCleanupRunning = "running"
	VirtualCharacterCleanupDone    = "done"
	VirtualCharacterCleanupFailed  = "failed"
)

type VirtualCharacterValidationSession struct {
	ID                    string `json:"id" gorm:"type:varchar(64);primaryKey"`
	UserID                int    `json:"user_id" gorm:"index"`
	ProviderAccountID     int    `json:"provider_account_id" gorm:"index"`
	Status                string `json:"status" gorm:"type:varchar(20);index"`
	StateHash             string `json:"-" gorm:"type:varchar(64);uniqueIndex"`
	EncryptedBytedToken   string `json:"-" gorm:"type:text"`
	EncryptedH5Link       string `json:"-" gorm:"type:text"`
	Name                  string `json:"name" gorm:"type:varchar(191)"`
	Description           string `json:"description" gorm:"type:text"`
	TagsJSON              string `json:"-" gorm:"type:text"`
	Language              string `json:"language" gorm:"type:varchar(16)"`
	ProviderGroupID       string `json:"provider_group_id,omitempty" gorm:"type:varchar(191);index"`
	CharacterID           int64  `json:"character_id,omitempty" gorm:"index"`
	HolderScopeAcceptedAt int64  `json:"holder_scope_accepted_at,omitempty" gorm:"index"`
	ResultCode            string `json:"result_code,omitempty" gorm:"type:varchar(64)"`
	LastError             string `json:"last_error,omitempty" gorm:"type:text"`
	ExpiresAt             int64  `json:"expires_at" gorm:"index"`
	ConsumedAt            int64  `json:"consumed_at,omitempty" gorm:"index"`
	CreatedAt             int64  `json:"created_at" gorm:"autoCreateTime;index"`
	UpdatedAt             int64  `json:"updated_at" gorm:"autoUpdateTime"`
}

type VirtualCharacterProviderAccount struct {
	ID                 int    `json:"id" gorm:"primaryKey;autoIncrement"`
	Enabled            bool   `json:"enabled" gorm:"index"`
	OfficialEnabled    bool   `json:"official_enabled"`
	VirtualEnabled     bool   `json:"virtual_enabled"`
	RealPersonEnabled  bool   `json:"real_person_enabled"`
	QuotaPlan          string `json:"quota_plan" gorm:"type:varchar(16)"` // free | paid | custom
	CreateAssetQPM     int    `json:"create_asset_qpm"`
	EncryptedAccessKey string `json:"-" gorm:"type:text"`
	EncryptedSecretKey string `json:"-" gorm:"type:text"`
	Region             string `json:"region" gorm:"type:varchar(64)"`
	ProjectName        string `json:"project_name" gorm:"type:varchar(191)"`
	ChannelID          int    `json:"-" gorm:"index"` // deprecated: generation uses the distributor-selected Seedance channel
	LastCheckStatus    string `json:"last_check_status,omitempty" gorm:"type:varchar(20)"`
	LastCheckError     string `json:"last_check_error,omitempty" gorm:"type:text"`
	LastCheckedAt      int64  `json:"last_checked_at,omitempty"`
	CreatedAt          int64  `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt          int64  `json:"updated_at" gorm:"autoUpdateTime"`
}

func (a *VirtualCharacterProviderAccount) EffectiveQuotaPlan() string {
	if a == nil {
		return VirtualCharacterQuotaPlanFree
	}
	switch strings.ToLower(strings.TrimSpace(a.QuotaPlan)) {
	case VirtualCharacterQuotaPlanPaid:
		return VirtualCharacterQuotaPlanPaid
	case VirtualCharacterQuotaPlanCustom:
		return VirtualCharacterQuotaPlanCustom
	default:
		return VirtualCharacterQuotaPlanFree
	}
}

func (a *VirtualCharacterProviderAccount) EffectiveCreateAssetQPM() int {
	if a == nil || a.CreateAssetQPM <= 0 {
		return VirtualCharacterDefaultCreateAssetQPM
	}
	if a.CreateAssetQPM > 300 {
		return 300
	}
	return a.CreateAssetQPM
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

type VirtualCharacterCatalogEntry struct {
	AssetID     string
	Name        string
	Description string
	TagsJSON    string
	CoverURL    string
	Enabled     bool
	Nationality string
	Gender      string
	AgeMin      *int
	AgeMax      *int
	Occupation  string
	Temperament string
}

type VirtualCharacterCatalogStats struct {
	Total    int `json:"total"`
	Created  int `json:"created"`
	Updated  int `json:"updated"`
	Offlined int `json:"offlined"`
}

type VirtualCharacterCleanupJob struct {
	ID                int64  `json:"id" gorm:"primaryKey;autoIncrement"`
	CharacterID       int64  `json:"character_id,omitempty" gorm:"index"`
	ProviderAccountID int    `json:"provider_account_id,omitempty" gorm:"index"`
	AIPDDChannelID    int    `json:"aipdd_channel_id,omitempty" gorm:"index"`
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
	if account.CreateAssetQPM <= 0 {
		account.CreateAssetQPM = VirtualCharacterDefaultCreateAssetQPM
	}
	account.QuotaPlan = account.EffectiveQuotaPlan()
	return DB.Transaction(func(tx *gorm.DB) error {
		if account.Enabled {
			if err := tx.Model(&VirtualCharacterProviderAccount{}).Where("id <> ?", account.ID).Update("enabled", false).Error; err != nil {
				return err
			}
		}
		return tx.Save(account).Error
	})
}

func CountVirtualCharacterProviderAssets() (int64, error) {
	var count int64
	err := DB.Model(&VirtualCharacter{}).
		Where("provider_asset_id <> ? AND status <> ?", "", VirtualCharacterStatusDeleting).
		Count(&count).Error
	return count, err
}

// CreateAIGCVirtualCharacter reserves a private slot for a user-created virtual
// character (volc_aigc). The provider group must be attached after CreateAssetGroup.
func CreateAIGCVirtualCharacter(userID, providerAccountID int, name, description, tagsJSON, assetType string) (*VirtualCharacter, int, error) {
	if userID <= 0 || providerAccountID <= 0 {
		return nil, 0, errors.New("invalid user or provider account")
	}
	assetType = EffectiveVirtualCharacterAssetType(assetType)
	limit := GetVirtualCharacterEffectiveLimit(userID)
	for slot := 1; slot <= limit; slot++ {
		candidateSlot := slot
		item := &VirtualCharacter{
			UserID:            userID,
			Slot:              &candidateSlot,
			Scope:             VirtualCharacterScopePrivate,
			Name:              name,
			Description:       description,
			TagsJSON:          tagsJSON,
			SourceType:        VirtualCharacterSourceVolcAIGC,
			Status:            VirtualCharacterStatusCreating,
			ValidationStatus:  VirtualCharacterValidationAccepted,
			ProviderAccountID: providerAccountID,
			AssetType:         assetType,
		}
		result := DB.Clauses(clause.OnConflict{DoNothing: true}).Create(item)
		if result.Error != nil {
			return nil, limit, result.Error
		}
		if result.RowsAffected == 1 {
			return item, limit, nil
		}
	}
	return nil, limit, errors.New("virtual character limit reached")
}

func AttachVirtualCharacterProviderGroup(characterID int64, providerGroupID string) error {
	providerGroupID = strings.TrimSpace(providerGroupID)
	if characterID <= 0 || providerGroupID == "" {
		return errors.New("invalid character or provider group")
	}
	return DB.Model(&VirtualCharacter{}).Where("id = ?", characterID).Updates(map[string]any{
		"provider_group_id": providerGroupID,
		"status":            VirtualCharacterStatusCreating,
		"last_error":        "",
		"updated_at":        time.Now().Unix(),
	}).Error
}

func ReleasePrivateVirtualCharacterSlot(characterID int64, reason string) error {
	if characterID <= 0 {
		return errors.New("invalid character id")
	}
	now := time.Now().Unix()
	return DB.Model(&VirtualCharacter{}).Where("id = ?", characterID).Updates(map[string]any{
		"slot":            nil,
		"status":          VirtualCharacterStatusFailed,
		"last_error":      reason,
		"cleanup_next_at": now,
		"updated_at":      now,
	}).Error
}

// DiscardFailedAIGCVirtualCharacterUpload hides an AIGC character whose only
// image failed to upload or activate. Provider resources are queued for
// deletion in the same transaction. Records with cleanup work are soft-deleted
// until that work completes; records without provider resources are removed
// immediately.
func DiscardFailedAIGCVirtualCharacterUpload(characterID int64, providerGroupID, reason string) error {
	if characterID <= 0 {
		return errors.New("invalid character id")
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		var character VirtualCharacter
		if err := tx.Where("id = ? AND source_type = ?", characterID, VirtualCharacterSourceVolcAIGC).First(&character).Error; err != nil {
			return err
		}
		now := time.Now().Unix()
		groupID := strings.TrimSpace(providerGroupID)
		if groupID == "" {
			groupID = strings.TrimSpace(character.ProviderGroupID)
		}
		aipddAssetID := ""
		if character.AIPDDAssetID > 0 {
			aipddAssetID = stringInt64(character.AIPDDAssetID)
		}
		targets := []struct {
			targetType string
			targetID   string
		}{
			{targetType: "volc_asset", targetID: character.ProviderAssetID},
			{targetType: "aipdd_asset", targetID: aipddAssetID},
			{targetType: "aipdd_file", targetID: character.StagingFileID},
			{targetType: "volc_group", targetID: groupID},
		}
		hasCleanupTarget := false
		for _, target := range targets {
			if strings.TrimSpace(target.targetID) == "" {
				continue
			}
			hasCleanupTarget = true
			if err := createCleanupJobIfAbsent(tx, character.ID, character.ProviderAccountID, target.targetType, target.targetID, now, character.AIPDDChannelID); err != nil {
				return err
			}
		}
		if err := tx.Model(&character).Updates(map[string]any{
			"slot":               nil,
			"status":             VirtualCharacterStatusDeleting,
			"asset_next_poll_at": 0,
			"last_error":         reason,
			"cleanup_next_at":    now,
			"updated_at":         now,
		}).Error; err != nil {
			return err
		}
		if !hasCleanupTarget {
			return tx.Unscoped().Delete(&character).Error
		}
		return tx.Delete(&character).Error
	})
}

func AttachVirtualCharacterImage(
	characterID int64,
	providerAssetID, stagingFileID string,
	aipddAssetID int64,
	aipddChannelID int,
	mimeType, assetType string,
	fileSize int64,
) error {
	providerAssetID = strings.TrimPrefix(strings.TrimSpace(providerAssetID), "asset://")
	if characterID <= 0 || providerAssetID == "" {
		return errors.New("invalid character asset")
	}
	now := time.Now().Unix()
	return DB.Model(&VirtualCharacter{}).Where("id = ?", characterID).Updates(map[string]any{
		"provider_asset_id":   providerAssetID,
		"staging_file_id":     strings.TrimSpace(stagingFileID),
		"a_ip_dd_asset_id":    aipddAssetID,
		"aipdd_channel_id":    aipddChannelID,
		"asset_type":          EffectiveVirtualCharacterAssetType(assetType),
		"mime_type":           strings.TrimSpace(mimeType),
		"file_size":           fileSize,
		"cover_url":           virtualCharacterPreviewPath(characterID),
		"status":              VirtualCharacterStatusCreating,
		"asset_poll_attempts": 0,
		"asset_next_poll_at":  now,
		"last_error":          "",
		"updated_at":          now,
	}).Error
}

func virtualCharacterPreviewPath(characterID int64) string {
	return "/api/virtual-characters/" + strconv.FormatInt(characterID, 10) + "/preview"
}

func ListVirtualCharactersToPoll(now int64, limit int) ([]VirtualCharacter, error) {
	var characters []VirtualCharacter
	err := DB.Where("status = ? AND asset_next_poll_at <= ? AND provider_asset_id <> ?",
		VirtualCharacterStatusCreating, now, "").
		Order("asset_next_poll_at ASC, id ASC").Limit(limit).Find(&characters).Error
	return characters, err
}

func MarkVirtualCharacterImageTerminal(characterID int64, active bool, reason string) error {
	now := time.Now().Unix()
	updates := map[string]any{
		"asset_next_poll_at":  0,
		"asset_poll_attempts": 0,
		"last_error":          reason,
		"updated_at":          now,
	}
	if active {
		updates["status"] = VirtualCharacterStatusActive
		updates["last_error"] = ""
	} else {
		updates["status"] = VirtualCharacterStatusFailed
		updates["slot"] = nil
	}
	return DB.Model(&VirtualCharacter{}).Where("id = ?", characterID).Updates(updates).Error
}

func RetryVirtualCharacterImagePoll(characterID int64, attempts int, nextAt int64, reason string) error {
	return DB.Model(&VirtualCharacter{}).Where("id = ?", characterID).Updates(map[string]any{
		"asset_poll_attempts": attempts, "asset_next_poll_at": nextAt, "last_error": reason, "updated_at": time.Now().Unix(),
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
		_ = FailReservedVirtualCharacterValidation(item.ID, VirtualCharacterValidationExpired, "expired", "validation session expired")
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
		"status": VirtualCharacterValidationFailed, "result_code": resultCode, "last_error": reason, "consumed_at": time.Now().Unix(),
		"encrypted_byted_token": "", "encrypted_h5_link": "", "updated_at": time.Now().Unix(),
	}).Error
}

func MarkVirtualCharacterValidationExpired(id string) error {
	return DB.Model(&VirtualCharacterValidationSession{}).Where("id = ? AND status = ?", id, VirtualCharacterValidationPending).Updates(map[string]any{
		"status": VirtualCharacterValidationExpired, "result_code": "expired", "last_error": "validation session expired", "consumed_at": time.Now().Unix(),
		"encrypted_byted_token": "", "encrypted_h5_link": "", "updated_at": time.Now().Unix(),
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
			item := &VirtualCharacter{UserID: session.UserID, Slot: &candidateSlot, Scope: VirtualCharacterScopePrivate, Name: session.Name, Description: session.Description, TagsJSON: session.TagsJSON, SourceType: VirtualCharacterSourceVolcRealPerson, Status: VirtualCharacterStatusActive, ValidationStatus: VirtualCharacterValidationAccepted, ProviderAccountID: session.ProviderAccountID, ProviderGroupID: providerGroupID, AssetType: VirtualCharacterAssetTypeImage}
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

func BeginVirtualCharacterGroupDelete(characterID int64, userID int) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		var character VirtualCharacter
		if err := tx.Where("id = ? AND scope = ? AND user_id = ?", characterID, VirtualCharacterScopePrivate, userID).First(&character).Error; err != nil {
			return err
		}
		now := time.Now().Unix()
		if err := tx.Model(&character).Updates(map[string]any{"status": VirtualCharacterStatusDeleting, "slot": nil, "asset_next_poll_at": 0, "updated_at": now}).Error; err != nil {
			return err
		}
		hasCleanupTarget := false
		if strings.TrimSpace(character.ProviderAssetID) != "" {
			hasCleanupTarget = true
			if err := createCleanupJobIfAbsent(tx, characterID, character.ProviderAccountID, "volc_asset", character.ProviderAssetID, now); err != nil {
				return err
			}
		}
		if character.AIPDDAssetID > 0 {
			hasCleanupTarget = true
			if err := createCleanupJobIfAbsent(tx, characterID, character.ProviderAccountID, "aipdd_asset", stringInt64(character.AIPDDAssetID), now, character.AIPDDChannelID); err != nil {
				return err
			}
		}
		if strings.TrimSpace(character.StagingFileID) != "" {
			hasCleanupTarget = true
			if err := createCleanupJobIfAbsent(tx, characterID, character.ProviderAccountID, "aipdd_file", character.StagingFileID, now, character.AIPDDChannelID); err != nil {
				return err
			}
		}
		if strings.TrimSpace(character.ProviderGroupID) != "" {
			hasCleanupTarget = true
			if err := createCleanupJobIfAbsent(tx, characterID, character.ProviderAccountID, "volc_group", character.ProviderGroupID, now); err != nil {
				return err
			}
		}
		if !hasCleanupTarget {
			return tx.Unscoped().Delete(&character).Error
		}
		return nil
	})
}

func virtualCharacterCatalogUpdates(entry VirtualCharacterCatalogEntry, status string, providerAccountID int, version string) map[string]any {
	updates := map[string]any{
		"name":                entry.Name,
		"description":         entry.Description,
		"tags_json":           entry.TagsJSON,
		"nationality":         entry.Nationality,
		"gender":              entry.Gender,
		"occupation":          entry.Occupation,
		"temperament":         entry.Temperament,
		"cover_url":           entry.CoverURL,
		"provider_asset_id":   strings.TrimPrefix(strings.TrimSpace(entry.AssetID), "asset://"),
		"status":              status,
		"validation_status":   VirtualCharacterValidationAccepted,
		"provider_account_id": providerAccountID,
		"asset_type":          VirtualCharacterAssetTypeImage,
		"catalog_version":     version,
		"updated_at":          time.Now().Unix(),
	}
	if entry.AgeMin != nil {
		updates["age_min"] = *entry.AgeMin
	} else {
		updates["age_min"] = nil
	}
	if entry.AgeMax != nil {
		updates["age_max"] = *entry.AgeMax
	} else {
		updates["age_max"] = nil
	}
	return updates
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
			if assetID := strings.TrimPrefix(strings.TrimSpace(previouslyActive[i].ProviderAssetID), "asset://"); assetID != "" {
				offlinedCandidates[assetID] = struct{}{}
			}
		}
		if err := tx.Model(&VirtualCharacter{}).Where("scope = ? AND source_type = ?", VirtualCharacterScopePublic, VirtualCharacterSourceVolcPreset).Updates(map[string]any{"status": VirtualCharacterStatusOffline, "updated_at": time.Now().Unix()}).Error; err != nil {
			return err
		}
		for _, entry := range entries {
			assetID := strings.TrimPrefix(strings.TrimSpace(entry.AssetID), "asset://")
			var character VirtualCharacter
			err := tx.Where("scope = ? AND source_type = ? AND provider_asset_id = ?", VirtualCharacterScopePublic, VirtualCharacterSourceVolcPreset, assetID).First(&character).Error
			status := VirtualCharacterStatusOffline
			if entry.Enabled {
				status = VirtualCharacterStatusActive
				delete(offlinedCandidates, assetID)
			}
			if errors.Is(err, gorm.ErrRecordNotFound) {
				character = VirtualCharacter{
					UserID: 0, Scope: VirtualCharacterScopePublic, Name: entry.Name, Description: entry.Description,
					TagsJSON: entry.TagsJSON, Nationality: entry.Nationality, Gender: entry.Gender,
					AgeMin: entry.AgeMin, AgeMax: entry.AgeMax, Occupation: entry.Occupation, Temperament: entry.Temperament,
					SourceType: VirtualCharacterSourceVolcPreset, Status: status, ValidationStatus: VirtualCharacterValidationAccepted,
					CoverURL: entry.CoverURL, ProviderAccountID: providerAccountID, ProviderAssetID: assetID,
					AssetType: VirtualCharacterAssetTypeImage, CatalogVersion: version,
				}
				if err := tx.Create(&character).Error; err != nil {
					return err
				}
				stats.Created++
				continue
			}
			if err != nil {
				return err
			}
			if err := tx.Model(&character).Updates(virtualCharacterCatalogUpdates(entry, status, providerAccountID, version)).Error; err != nil {
				return err
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
	return createCleanupJobIfAbsent(DB, job.CharacterID, job.ProviderAccountID, job.TargetType, job.TargetID, job.NextAttemptAt, job.AIPDDChannelID)
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

func HasIncompleteVirtualCharacterCleanupJobs(characterID, excludeJobID int64) (bool, error) {
	var count int64
	query := DB.Model(&VirtualCharacterCleanupJob{}).
		Where("character_id = ? AND id <> ? AND status <> ?", characterID, excludeJobID, VirtualCharacterCleanupDone)
	if err := query.Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

type legacyVirtualCharacterAsset struct {
	ID                int64 `gorm:"primaryKey"`
	CharacterID       int64 `gorm:"index"`
	ProviderAccountID int
	ProviderAssetID   string
	Name              string
	AssetType         string
	Status            string
	IsPrimary         bool
	CoverURL          string
	StagingFileID     string
	MimeType          string
	FileSize          int64
	LastError         string
	PollAttempts      int
	NextPollAt        int64
	CreatedAt         int64
	UpdatedAt         int64
	DeletedAt         gorm.DeletedAt
}

func (legacyVirtualCharacterAsset) TableName() string { return "virtual_character_assets" }

type legacyVirtualCharacterPrimary struct {
	ID             int64
	PrimaryAssetID *int64
}

func (legacyVirtualCharacterPrimary) TableName() string { return "virtual_characters" }

type VirtualCharacterCollapsePreflightStats struct {
	CharactersWithLegacyAssets int64
	LegacyAssets               int64
	MinimumExtraAssets         int64
	SoftDeletedAssets          int64
}

func GetVirtualCharacterCollapsePreflightStats() (VirtualCharacterCollapsePreflightStats, error) {
	stats := VirtualCharacterCollapsePreflightStats{}
	if !DB.Migrator().HasTable(&legacyVirtualCharacterAsset{}) {
		return stats, nil
	}
	if err := DB.Unscoped().Model(&legacyVirtualCharacterAsset{}).Count(&stats.LegacyAssets).Error; err != nil {
		return stats, err
	}
	if err := DB.Unscoped().Model(&legacyVirtualCharacterAsset{}).Distinct("character_id").Count(&stats.CharactersWithLegacyAssets).Error; err != nil {
		return stats, err
	}
	if err := DB.Unscoped().Model(&legacyVirtualCharacterAsset{}).Where("deleted_at IS NOT NULL").Count(&stats.SoftDeletedAssets).Error; err != nil {
		return stats, err
	}
	stats.MinimumExtraAssets = stats.LegacyAssets - stats.CharactersWithLegacyAssets
	return stats, nil
}

// MigrateVirtualCharacterABData converts legacy public rows into offline actor
// groups with a single provider asset, and removes only legacy private rows
// (aipdd / empty / bare volc). User-created volc_aigc and volc_real_person
// characters must survive restarts — this migration runs on every boot.
func MigrateVirtualCharacterABData() error {
	return DB.Transaction(func(tx *gorm.DB) error {
		var publicItems []VirtualCharacter
		if err := tx.Unscoped().Where("scope = ? AND source_type IN ?", VirtualCharacterScopePublic, []string{VirtualCharacterSourceVolc, ""}).Find(&publicItems).Error; err != nil {
			return err
		}
		for i := range publicItems {
			item := &publicItems[i]
			updates := map[string]any{"source_type": VirtualCharacterSourceVolcPreset, "status": VirtualCharacterStatusOffline, "updated_at": time.Now().Unix()}
			if assetID := strings.TrimPrefix(strings.TrimSpace(item.VolcAssetID), "asset://"); assetID != "" {
				updates["provider_asset_id"] = assetID
			}
			if err := tx.Model(item).Updates(updates).Error; err != nil {
				return err
			}
		}
		var privateItems []VirtualCharacter
		// Only purge pre-AB private rows. New private characters use volc_aigc / volc_real_person.
		if err := tx.Unscoped().Where("scope = ? AND source_type IN ?", VirtualCharacterScopePrivate, []string{VirtualCharacterSourceAIPDD, VirtualCharacterSourceVolc, ""}).Find(&privateItems).Error; err != nil {
			return err
		}
		now := time.Now().Unix()
		for i := range privateItems {
			item := &privateItems[i]
			if tx.Migrator().HasTable(&legacyVirtualCharacterAsset{}) {
				var assets []legacyVirtualCharacterAsset
				if err := tx.Unscoped().Where("character_id = ?", item.ID).Find(&assets).Error; err != nil {
					return err
				}
				for j := range assets {
					asset := &assets[j]
					if err := enqueueLegacyVirtualCharacterAssetCleanup(tx, item.ID, item.ProviderAccountID, asset, now); err != nil {
						return err
					}
				}
			}
			if strings.TrimSpace(item.ProviderGroupID) != "" {
				if err := createCleanupJobIfAbsent(tx, item.ID, item.ProviderAccountID, "volc_group", item.ProviderGroupID, now); err != nil {
					return err
				}
			}
			if item.AIPDDAssetID > 0 {
				if err := createCleanupJobIfAbsent(tx, item.ID, item.ProviderAccountID, "aipdd_asset", strings.TrimSpace(stringInt64(item.AIPDDAssetID)), now, item.AIPDDChannelID); err != nil {
					return err
				}
			}
			if strings.TrimSpace(item.AIPDDFileID) != "" {
				if err := createCleanupJobIfAbsent(tx, item.ID, item.ProviderAccountID, "aipdd_file", item.AIPDDFileID, now, item.AIPDDChannelID); err != nil {
					return err
				}
			}
			if err := tx.Unscoped().Where("character_id = ?", item.ID).Delete(&VirtualCharacterValidationSession{}).Error; err != nil {
				return err
			}
			if err := tx.Unscoped().Delete(&VirtualCharacter{}, item.ID).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func MigrateVirtualCharacterCollapseAssets() error {
	if !DB.Migrator().HasTable(&legacyVirtualCharacterAsset{}) {
		return nil
	}
	stats, err := GetVirtualCharacterCollapsePreflightStats()
	if err != nil {
		return err
	}
	common.SysLog(fmt.Sprintf(
		"virtual character single-image migration preflight: characters_with_assets=%d legacy_assets=%d minimum_extra_assets=%d soft_deleted_assets=%d; verify a restorable database backup before startup",
		stats.CharactersWithLegacyAssets, stats.LegacyAssets, stats.MinimumExtraAssets, stats.SoftDeletedAssets,
	))
	hasPrimaryAssetID := DB.Migrator().HasColumn(&legacyVirtualCharacterPrimary{}, "PrimaryAssetID")
	if err := DB.Transaction(func(tx *gorm.DB) error {
		var characters []VirtualCharacter
		if err := tx.Unscoped().Order("id ASC").Find(&characters).Error; err != nil {
			return err
		}
		now := time.Now().Unix()
		for i := range characters {
			character := &characters[i]
			var assets []legacyVirtualCharacterAsset
			if err := tx.Unscoped().Where("character_id = ?", character.ID).Order("id ASC").Find(&assets).Error; err != nil {
				return err
			}
			var primary legacyVirtualCharacterPrimary
			if hasPrimaryAssetID {
				if err := tx.Table(primary.TableName()).Select("id, primary_asset_id").Where("id = ?", character.ID).Scan(&primary).Error; err != nil {
					return err
				}
			}
			keeper := selectLegacyVirtualCharacterAsset(assets, primary.PrimaryAssetID)
			if keeper == nil {
				// Public presets may have been migrated directly from the legacy
				// volc_asset_id column before this collapse runs. Preserve that
				// provider reference when no child asset row exists.
				if character.SourceType == VirtualCharacterSourceVolcRealPerson {
					// Keep the dormant real-person group. Any unusable child assets
					// are still queued below before the old table is removed.
				} else if strings.TrimSpace(character.ProviderAssetID) != "" {
					if character.Scope == VirtualCharacterScopePublic {
						if err := tx.Model(&VirtualCharacter{}).Unscoped().Where("id = ?", character.ID).Updates(map[string]any{
							"status": VirtualCharacterStatusOffline, "updated_at": now,
						}).Error; err != nil {
							return err
						}
					}
				} else {
					status := VirtualCharacterStatusOffline
					updates := map[string]any{"provider_asset_id": "", "staging_file_id": "", "asset_next_poll_at": 0, "updated_at": now, "status": status}
					if character.Scope == VirtualCharacterScopePrivate {
						updates["status"] = VirtualCharacterStatusFailed
						updates["slot"] = nil
						updates["last_error"] = "no usable image was found during the single-image migration"
					}
					if err := tx.Model(&VirtualCharacter{}).Unscoped().Where("id = ?", character.ID).Updates(updates).Error; err != nil {
						return err
					}
				}
			} else {
				status := VirtualCharacterStatusFailed
				nextPollAt := int64(0)
				var slot any
				if character.Slot != nil {
					slot = *character.Slot
				}
				switch keeper.Status {
				case "Active":
					status = VirtualCharacterStatusActive
				case "Processing":
					status = VirtualCharacterStatusCreating
					nextPollAt = keeper.NextPollAt
					if nextPollAt <= 0 {
						nextPollAt = now
					}
				default:
					slot = nil
				}
				coverURL := keeper.CoverURL
				if character.Scope == VirtualCharacterScopePrivate || isLegacyVirtualCharacterPreviewURL(coverURL) {
					coverURL = virtualCharacterPreviewPath(character.ID)
				}
				providerAccountID := keeper.ProviderAccountID
				if providerAccountID <= 0 {
					providerAccountID = character.ProviderAccountID
				}
				updates := map[string]any{
					"provider_account_id": providerAccountID,
					"provider_asset_id":   strings.TrimPrefix(strings.TrimSpace(keeper.ProviderAssetID), "asset://"),
					"staging_file_id":     keeper.StagingFileID,
					"mime_type":           keeper.MimeType,
					"file_size":           keeper.FileSize,
					"cover_url":           coverURL,
					"status":              status,
					"slot":                slot,
					"last_error":          keeper.LastError,
					"asset_poll_attempts": keeper.PollAttempts,
					"asset_next_poll_at":  nextPollAt,
					"updated_at":          now,
				}
				if err := tx.Model(&VirtualCharacter{}).Unscoped().Where("id = ?", character.ID).Updates(updates).Error; err != nil {
					return err
				}
			}
			for j := range assets {
				asset := &assets[j]
				if keeper != nil && asset.ID == keeper.ID {
					continue
				}
				if err := enqueueLegacyVirtualCharacterAssetCleanup(tx, character.ID, character.ProviderAccountID, asset, now); err != nil {
					return err
				}
			}
			if keeper == nil && character.Scope == VirtualCharacterScopePrivate && character.SourceType != VirtualCharacterSourceVolcRealPerson && strings.TrimSpace(character.ProviderAssetID) == "" {
				if err := createCleanupJobIfAbsent(tx, character.ID, character.ProviderAccountID, "volc_group", character.ProviderGroupID, now); err != nil {
					return err
				}
			}
			if keeper != nil && keeper.Status == "Failed" && character.Scope == VirtualCharacterScopePrivate {
				if err := enqueueLegacyVirtualCharacterAssetCleanup(tx, character.ID, character.ProviderAccountID, keeper, now); err != nil {
					return err
				}
				if err := createCleanupJobIfAbsent(tx, character.ID, character.ProviderAccountID, "volc_group", character.ProviderGroupID, now); err != nil {
					return err
				}
			}
		}
		return nil
	}); err != nil {
		return err
	}
	return DB.Migrator().DropTable(&legacyVirtualCharacterAsset{})
}

func selectLegacyVirtualCharacterAsset(assets []legacyVirtualCharacterAsset, primaryID *int64) *legacyVirtualCharacterAsset {
	isUsable := func(asset *legacyVirtualCharacterAsset) bool {
		if asset == nil || asset.DeletedAt.Valid || asset.AssetType != "Image" || strings.TrimSpace(asset.ProviderAssetID) == "" {
			return false
		}
		switch asset.Status {
		case "Active", "Processing", "Failed":
			return true
		default:
			return false
		}
	}
	if primaryID != nil {
		for i := range assets {
			if assets[i].ID == *primaryID && isUsable(&assets[i]) {
				return &assets[i]
			}
		}
	}
	for i := range assets {
		if assets[i].IsPrimary && isUsable(&assets[i]) {
			return &assets[i]
		}
	}
	for _, status := range []string{"Active", "Processing", "Failed"} {
		for i := range assets {
			if assets[i].Status == status && isUsable(&assets[i]) {
				return &assets[i]
			}
		}
	}
	return nil
}

func enqueueLegacyVirtualCharacterAssetCleanup(tx *gorm.DB, characterID int64, providerAccountID int, asset *legacyVirtualCharacterAsset, now int64) error {
	if asset == nil {
		return nil
	}
	accountID := asset.ProviderAccountID
	if accountID <= 0 {
		accountID = providerAccountID
	}
	if err := createCleanupJobIfAbsent(tx, characterID, accountID, "volc_asset", asset.ProviderAssetID, now); err != nil {
		return err
	}
	return createCleanupJobIfAbsent(tx, characterID, accountID, "aipdd_file", asset.StagingFileID, now)
}

func isLegacyVirtualCharacterPreviewURL(value string) bool {
	value = strings.TrimSpace(value)
	return strings.HasPrefix(value, "/api/virtual-characters/") && strings.Contains(value, "/assets/") && strings.HasSuffix(value, "/preview")
}

func createCleanupJobIfAbsent(tx *gorm.DB, characterID int64, providerAccountID int, targetType, targetID string, now int64, aipddChannelIDs ...int) error {
	targetID = strings.TrimSpace(targetID)
	if targetType == "" || targetID == "" || ((targetType == "aipdd_asset" || targetType == "aipdd_file") && targetID == "0") {
		return nil
	}
	aipddChannelID := 0
	if len(aipddChannelIDs) > 0 {
		aipddChannelID = aipddChannelIDs[0]
	}
	var existing VirtualCharacterCleanupJob
	result := tx.Where("target_type = ? AND target_id = ?", targetType, targetID).Limit(1).Find(&existing)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected > 0 {
		if existing.AIPDDChannelID == 0 && aipddChannelID > 0 {
			return tx.Model(&existing).Update("aipdd_channel_id", aipddChannelID).Error
		}
		return nil
	}
	return tx.Create(&VirtualCharacterCleanupJob{
		CharacterID:       characterID,
		ProviderAccountID: providerAccountID,
		AIPDDChannelID:    aipddChannelID,
		TargetType:        targetType,
		TargetID:          targetID,
		Status:            VirtualCharacterCleanupPending,
		NextAttemptAt:     now,
	}).Error
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
	return DB.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "id"}}, DoUpdates: clause.AssignmentColumns([]string{"enabled", "official_enabled", "virtual_enabled", "real_person_enabled", "quota_plan", "create_asset_qpm", "encrypted_access_key", "encrypted_secret_key", "region", "project_name", "channel_id", "updated_at"})}).Create(account).Error
}
