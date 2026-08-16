package model

import (
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	VirtualCharacterAuthorizationPending             = "pending"
	VirtualCharacterAuthorizationSynchronizing       = "synchronizing"
	VirtualCharacterAuthorizationActive              = "active"
	VirtualCharacterAuthorizationAmbiguous           = "ambiguous"
	VirtualCharacterAuthorizationProviderUnavailable = "provider_unavailable"
	VirtualCharacterAuthorizationExpired             = "expired"
	VirtualCharacterAuthorizationRevoked             = "revoked"
	VirtualCharacterAuthorizationFailed              = "failed"
	VirtualCharacterProviderAssetAwaitingUpload      = "AwaitingUpload"
	VirtualCharacterProviderAssetProcessing          = "Processing"

	VirtualCharacterRealPersonGroupType    = "LivenessFace"
	VirtualCharacterRealPersonAgreement    = "volc-official-h5-v1"
	VirtualCharacterDefaultRealPersonLimit = 5
)

// VirtualCharacterAuthorization is the auditable, local verification gate for
// a natural person. The official provider H5 flow supplies the consent and
// identity evidence; the legacy scope columns remain for database compatibility.
type VirtualCharacterAuthorization struct {
	ID                    int64  `json:"id" gorm:"primaryKey;autoIncrement"`
	CharacterID           int64  `json:"character_id" gorm:"uniqueIndex"`
	UserID                int    `json:"user_id" gorm:"index"`
	Status                string `json:"status" gorm:"type:varchar(32);index"`
	ValidFrom             int64  `json:"valid_from" gorm:"index"`
	ValidUntil            int64  `json:"valid_until" gorm:"index"`
	CommercialUseAllowed  bool   `json:"commercial_use_allowed"`
	PurposesJSON          string `json:"-" gorm:"type:text"`
	RegionsJSON           string `json:"-" gorm:"type:text"`
	PlatformsJSON         string `json:"-" gorm:"type:text"`
	IndustriesJSON        string `json:"-" gorm:"type:text"`
	AgreementVersion      string `json:"agreement_version" gorm:"type:varchar(64)"`
	AgreementReference    string `json:"agreement_reference,omitempty" gorm:"type:varchar(191);index"`
	ConsentReceiptHash    string `json:"consent_receipt_hash,omitempty" gorm:"type:varchar(64);index"`
	HolderScopeAcceptedAt int64  `json:"holder_scope_accepted_at,omitempty" gorm:"index"`
	ProviderGroupType     string `json:"provider_group_type" gorm:"type:varchar(32)"`
	ProviderGroupStatus   string `json:"provider_group_status,omitempty" gorm:"type:varchar(32)"`
	ProviderAssetStatus   string `json:"provider_asset_status,omitempty" gorm:"type:varchar(32)"`
	ProviderCheckedAt     int64  `json:"provider_checked_at,omitempty" gorm:"index"`
	AuthorizedAt          int64  `json:"authorized_at,omitempty" gorm:"index"`
	RevokedAt             int64  `json:"revoked_at,omitempty" gorm:"index"`
	ExpiredAt             int64  `json:"expired_at,omitempty" gorm:"index"`
	LastError             string `json:"last_error,omitempty" gorm:"type:text"`
	CreatedAt             int64  `json:"created_at" gorm:"autoCreateTime;index"`
	UpdatedAt             int64  `json:"updated_at" gorm:"autoUpdateTime"`
}

// VirtualCharacterTaskReference preserves every character and authorization
// snapshot used by one video task. A task may combine a verified person with
// one or more official virtual portraits, so the legacy one-task/one-character
// link is not sufficient for audit or deletion safety.
type VirtualCharacterTaskReference struct {
	ID                        int64  `json:"id" gorm:"primaryKey;autoIncrement"`
	TaskID                    string `json:"task_id" gorm:"type:varchar(191);index;uniqueIndex:uk_virtual_character_task_reference"`
	UserID                    int    `json:"user_id" gorm:"index"`
	CharacterID               int64  `json:"character_id" gorm:"index;uniqueIndex:uk_virtual_character_task_reference"`
	CharacterName             string `json:"character_name" gorm:"type:varchar(191)"`
	CharacterScope            string `json:"character_scope" gorm:"type:varchar(16)"`
	SourceType                string `json:"source_type" gorm:"type:varchar(32)"`
	ProviderAssetID           string `json:"provider_asset_id" gorm:"type:varchar(191);index"`
	AuthorizationSnapshotJSON string `json:"-" gorm:"type:text"`
	CreatedAt                 int64  `json:"created_at" gorm:"autoCreateTime;index"`
}

func GetVirtualCharacterRealPersonLimit() int {
	common.OptionMapRWMutex.RLock()
	raw := strings.TrimSpace(common.OptionMap["VirtualCharacterRealPersonLimit"])
	common.OptionMapRWMutex.RUnlock()
	limit, err := strconv.Atoi(raw)
	if err != nil || limit <= 0 {
		return VirtualCharacterDefaultRealPersonLimit
	}
	if limit > 1000 {
		return 1000
	}
	return limit
}

func CountRealPersonVirtualCharacters(userID int) (int64, error) {
	var count int64
	err := DB.Model(&VirtualCharacter{}).
		Where("user_id = ? AND scope = ? AND source_type = ? AND real_person_slot IS NOT NULL",
			userID, VirtualCharacterScopePrivate, VirtualCharacterSourceVolcRealPerson).
		Count(&count).Error
	return count, err
}

func ReserveRealPersonVirtualCharacter(userID, providerAccountID int, name, description, tagsJSON string) (*VirtualCharacter, *VirtualCharacterAuthorization, int, error) {
	if userID <= 0 || providerAccountID <= 0 {
		return nil, nil, 0, errors.New("invalid user or provider account")
	}
	now := time.Now().Unix()
	limit := GetVirtualCharacterRealPersonLimit()
	var character *VirtualCharacter
	var authorization *VirtualCharacterAuthorization
	err := DB.Transaction(func(tx *gorm.DB) error {
		for slot := 1; slot <= limit; slot++ {
			candidateSlot := slot
			item := &VirtualCharacter{
				UserID: userID, RealPersonSlot: &candidateSlot, Scope: VirtualCharacterScopePrivate,
				Name: name, Description: description, TagsJSON: tagsJSON,
				SourceType: VirtualCharacterSourceVolcRealPerson, Status: VirtualCharacterStatusCreating,
				ValidationStatus: VirtualCharacterValidationUnverified, ProviderAccountID: providerAccountID,
			}
			result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(item)
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected == 1 {
				character = item
				break
			}
		}
		if character == nil {
			return errors.New("real-person character limit reached")
		}
		authorization = &VirtualCharacterAuthorization{
			CharacterID: character.ID, UserID: userID, Status: VirtualCharacterAuthorizationPending,
			ValidFrom: now, AgreementVersion: VirtualCharacterRealPersonAgreement,
			ProviderGroupType: VirtualCharacterRealPersonGroupType,
		}
		return tx.Create(authorization).Error
	})
	return character, authorization, limit, err
}

func DeleteRealPersonReservation(characterID int64, userID int) error {
	if characterID <= 0 || userID <= 0 {
		return errors.New("invalid real-person reservation")
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("character_id = ? AND user_id = ?", characterID, userID).Delete(&VirtualCharacterAuthorization{}).Error; err != nil {
			return err
		}
		return tx.Unscoped().Where("id = ? AND user_id = ? AND source_type = ?", characterID, userID, VirtualCharacterSourceVolcRealPerson).Delete(&VirtualCharacter{}).Error
	})
}

func CancelReservedVirtualCharacterValidation(sessionID string, userID int) (bool, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" || userID <= 0 {
		return false, gorm.ErrRecordNotFound
	}
	cancelled := false
	err := DB.Transaction(func(tx *gorm.DB) error {
		var session VirtualCharacterValidationSession
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND user_id = ?", sessionID, userID).First(&session).Error; err != nil {
			return err
		}
		if session.Status != VirtualCharacterValidationPending {
			return nil
		}
		now := time.Now().Unix()
		if err := tx.Model(&session).Updates(map[string]any{
			"status": VirtualCharacterValidationCancelled, "result_code": "cancelled",
			"last_error": "validation cancelled", "consumed_at": now,
			"encrypted_byted_token": "", "encrypted_h5_link": "", "updated_at": now,
		}).Error; err != nil {
			return err
		}
		if session.CharacterID > 0 {
			if err := tx.Where("character_id = ? AND user_id = ?", session.CharacterID, userID).Delete(&VirtualCharacterAuthorization{}).Error; err != nil {
				return err
			}
			if err := tx.Unscoped().Where("id = ? AND user_id = ? AND source_type = ? AND validation_status = ?",
				session.CharacterID, userID, VirtualCharacterSourceVolcRealPerson, VirtualCharacterValidationUnverified).
				Delete(&VirtualCharacter{}).Error; err != nil {
				return err
			}
		}
		cancelled = true
		return nil
	})
	return cancelled, err
}

func GetVirtualCharacterAuthorization(characterID int64) (*VirtualCharacterAuthorization, error) {
	var item VirtualCharacterAuthorization
	err := DB.Where("character_id = ?", characterID).First(&item).Error
	return &item, err
}

func GetOwnedVirtualCharacterByProviderAssetID(assetID string, userID int) (*VirtualCharacter, error) {
	assetID = strings.TrimPrefix(strings.TrimSpace(assetID), "asset://")
	var item VirtualCharacter
	err := DB.Where("provider_asset_id = ? AND ((scope = ? AND status = ?) OR (scope = ? AND user_id = ?))",
		assetID, VirtualCharacterScopePublic, VirtualCharacterStatusActive, VirtualCharacterScopePrivate, userID).
		First(&item).Error
	return &item, err
}

func GetRegisteredVirtualCharacterByProviderAssetID(assetID string) (*VirtualCharacter, error) {
	assetID = strings.TrimPrefix(strings.TrimSpace(assetID), "asset://")
	var items []VirtualCharacter
	err := DB.Where("provider_asset_id = ?", assetID).Order("id ASC").Limit(2).Find(&items).Error
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	if len(items) > 1 {
		return nil, errors.New("provider asset id is registered more than once")
	}
	return &items[0], nil
}

func CompleteReservedVirtualCharacterValidation(sessionID, providerGroupID, consentReceiptHash string) (*VirtualCharacter, error) {
	providerGroupID = strings.TrimSpace(providerGroupID)
	if providerGroupID == "" {
		return nil, errors.New("provider group id is required")
	}
	var character VirtualCharacter
	err := DB.Transaction(func(tx *gorm.DB) error {
		var session VirtualCharacterValidationSession
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", sessionID).First(&session).Error; err != nil {
			return err
		}
		if session.Status == VirtualCharacterValidationSucceeded && session.CharacterID > 0 {
			return tx.First(&character, session.CharacterID).Error
		}
		if session.Status != VirtualCharacterValidationPending || session.ExpiresAt <= time.Now().Unix() || session.CharacterID <= 0 {
			return errors.New("validation session is no longer pending")
		}
		if strings.TrimSpace(consentReceiptHash) == "" {
			return errors.New("real-person validation evidence is missing")
		}
		if err := tx.Where("id = ? AND user_id = ? AND source_type = ?", session.CharacterID, session.UserID, VirtualCharacterSourceVolcRealPerson).First(&character).Error; err != nil {
			return err
		}
		var authorization VirtualCharacterAuthorization
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("character_id = ? AND user_id = ?", character.ID, character.UserID).First(&authorization).Error; err != nil {
			return err
		}
		if authorization.Status != VirtualCharacterAuthorizationPending {
			return errors.New("real-person authorization is no longer pending")
		}
		now := time.Now().Unix()
		if err := tx.Model(&character).Updates(map[string]any{
			"provider_group_id": providerGroupID, "provider_asset_id": "",
			"status": VirtualCharacterStatusCreating, "validation_status": VirtualCharacterValidationAccepted,
			"asset_poll_attempts": 0, "asset_next_poll_at": 0, "last_error": "", "updated_at": now,
		}).Error; err != nil {
			return err
		}
		if err := tx.Model(&authorization).Updates(map[string]any{
			"status": VirtualCharacterAuthorizationSynchronizing, "agreement_reference": session.ID,
			"consent_receipt_hash": strings.TrimSpace(consentReceiptHash), "authorized_at": now,
			"holder_scope_accepted_at": now,
			"provider_group_type":      VirtualCharacterRealPersonGroupType, "provider_group_status": "Active",
			"provider_asset_status": VirtualCharacterProviderAssetAwaitingUpload,
			"last_error":            "", "updated_at": now,
		}).Error; err != nil {
			return err
		}
		return tx.Model(&session).Updates(map[string]any{
			"status": VirtualCharacterValidationSucceeded, "provider_group_id": providerGroupID,
			"result_code": "10000", "last_error": "", "consumed_at": now,
			"holder_scope_accepted_at": now,
			"encrypted_byted_token":    "", "encrypted_h5_link": "", "updated_at": now,
		}).Error
	})
	return &character, err
}

func MarkRealPersonVirtualCharacterAwaitingAssetUpload(characterID int64) error {
	if characterID <= 0 {
		return errors.New("invalid real-person character")
	}
	now := time.Now().Unix()
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&VirtualCharacter{}).
			Where("id = ? AND source_type = ? AND provider_asset_id = ?", characterID, VirtualCharacterSourceVolcRealPerson, "").
			Updates(map[string]any{
				"status": VirtualCharacterStatusCreating, "asset_poll_attempts": 0,
				"asset_next_poll_at": 0, "last_error": "", "updated_at": now,
			}).Error; err != nil {
			return err
		}
		return tx.Model(&VirtualCharacterAuthorization{}).
			Where("character_id = ? AND status NOT IN ?", characterID, []string{VirtualCharacterAuthorizationExpired, VirtualCharacterAuthorizationRevoked}).
			Updates(map[string]any{
				"status":                VirtualCharacterAuthorizationSynchronizing,
				"provider_asset_status": VirtualCharacterProviderAssetAwaitingUpload,
				"provider_checked_at":   now, "last_error": "", "updated_at": now,
			}).Error
	})
}

func AttachRealPersonVirtualCharacterImage(characterID int64, providerAssetID, stagingFileID, mimeType string, fileSize int64) error {
	providerAssetID = strings.TrimPrefix(strings.TrimSpace(providerAssetID), "asset://")
	if characterID <= 0 || providerAssetID == "" {
		return errors.New("invalid real-person character asset")
	}
	now := time.Now().Unix()
	return DB.Transaction(func(tx *gorm.DB) error {
		var character VirtualCharacter
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", characterID).First(&character).Error; err != nil {
			return err
		}
		if character.SourceType != VirtualCharacterSourceVolcRealPerson || character.ValidationStatus != VirtualCharacterValidationAccepted || strings.TrimSpace(character.ProviderGroupID) == "" {
			return errors.New("real-person character has not completed identity validation")
		}
		if strings.TrimSpace(character.ProviderAssetID) != "" {
			return errors.New("real-person character asset has already been uploaded")
		}
		var authorization VirtualCharacterAuthorization
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("character_id = ?", characterID).First(&authorization).Error; err != nil {
			return err
		}
		if authorization.Status == VirtualCharacterAuthorizationExpired || authorization.Status == VirtualCharacterAuthorizationRevoked || authorization.HolderScopeAcceptedAt <= 0 || strings.TrimSpace(authorization.ConsentReceiptHash) == "" {
			return errors.New("real-person authorization is not valid for asset upload")
		}
		if err := tx.Model(&character).Updates(map[string]any{
			"provider_asset_id": providerAssetID, "staging_file_id": strings.TrimSpace(stagingFileID),
			"mime_type": strings.TrimSpace(mimeType), "file_size": fileSize,
			"cover_url": virtualCharacterPreviewPath(characterID), "status": VirtualCharacterStatusCreating,
			"asset_poll_attempts": 0, "asset_next_poll_at": now, "last_error": "", "updated_at": now,
		}).Error; err != nil {
			return err
		}
		return tx.Model(&authorization).Updates(map[string]any{
			"status":                VirtualCharacterAuthorizationSynchronizing,
			"provider_asset_status": VirtualCharacterProviderAssetProcessing,
			"provider_checked_at":   now, "last_error": "", "updated_at": now,
		}).Error
	})
}

func FailReservedVirtualCharacterValidation(sessionID, status, resultCode, reason string) error {
	if status != VirtualCharacterValidationFailed && status != VirtualCharacterValidationExpired {
		return errors.New("invalid validation terminal status")
	}
	now := time.Now().Unix()
	return DB.Transaction(func(tx *gorm.DB) error {
		var session VirtualCharacterValidationSession
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", sessionID).First(&session).Error; err != nil {
			return err
		}
		if session.Status != VirtualCharacterValidationPending {
			return nil
		}
		if err := tx.Model(&session).Updates(map[string]any{
			"status": status, "result_code": resultCode, "last_error": reason,
			"consumed_at": now, "encrypted_byted_token": "", "encrypted_h5_link": "", "updated_at": now,
		}).Error; err != nil {
			return err
		}
		if session.CharacterID <= 0 {
			return nil
		}
		if err := tx.Where("character_id = ? AND user_id = ?", session.CharacterID, session.UserID).Delete(&VirtualCharacterAuthorization{}).Error; err != nil {
			return err
		}
		return tx.Unscoped().Where("id = ? AND user_id = ? AND source_type = ? AND validation_status = ?",
			session.CharacterID, session.UserID, VirtualCharacterSourceVolcRealPerson, VirtualCharacterValidationUnverified).
			Delete(&VirtualCharacter{}).Error
	})
}

func MarkRealPersonVirtualCharacterSynchronizing(characterID int64, groupStatus, assetStatus string, checkedAt int64, attempts int, nextAt int64) error {
	if characterID <= 0 {
		return errors.New("invalid real-person character")
	}
	now := time.Now().Unix()
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&VirtualCharacter{}).
			Where("id = ? AND source_type = ?", characterID, VirtualCharacterSourceVolcRealPerson).
			Updates(map[string]any{
				"status": VirtualCharacterStatusCreating, "asset_poll_attempts": attempts,
				"asset_next_poll_at": nextAt, "last_error": "", "updated_at": now,
			}).Error; err != nil {
			return err
		}
		return tx.Model(&VirtualCharacterAuthorization{}).Where("character_id = ?", characterID).Updates(map[string]any{
			"provider_group_status": strings.TrimSpace(groupStatus),
			"provider_asset_status": strings.TrimSpace(assetStatus),
			"provider_checked_at":   checkedAt, "status": VirtualCharacterAuthorizationSynchronizing,
			"last_error": "", "updated_at": now,
		}).Error
	})
}

func BlockRealPersonVirtualCharacter(characterID int64, authorizationStatus, reason string) error {
	now := time.Now().Unix()
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&VirtualCharacter{}).Where("id = ? AND source_type = ?", characterID, VirtualCharacterSourceVolcRealPerson).Updates(map[string]any{
			"status": VirtualCharacterStatusBlocked, "asset_next_poll_at": 0,
			"last_error": reason, "updated_at": now,
		}).Error; err != nil {
			return err
		}
		return tx.Model(&VirtualCharacterAuthorization{}).Where("character_id = ?", characterID).Updates(map[string]any{
			"status": authorizationStatus, "provider_checked_at": now,
			"last_error": reason, "updated_at": now,
		}).Error
	})
}

func ActivateRealPersonVirtualCharacter(characterID int64, providerAssetID, providerAssetStatus string, checkedAt int64) error {
	providerAssetID = strings.TrimPrefix(strings.TrimSpace(providerAssetID), "asset://")
	if providerAssetID == "" {
		return errors.New("provider asset id is required")
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		var authorization VirtualCharacterAuthorization
		if err := tx.Where("character_id = ?", characterID).First(&authorization).Error; err != nil {
			return err
		}
		now := time.Now().Unix()
		if authorization.HolderScopeAcceptedAt <= 0 || strings.TrimSpace(authorization.ConsentReceiptHash) == "" {
			return errors.New("real-person authorization evidence is incomplete")
		}
		if authorization.ValidUntil > 0 && authorization.ValidUntil <= now {
			if err := tx.Model(&authorization).Updates(map[string]any{"status": VirtualCharacterAuthorizationExpired, "expired_at": now, "last_error": "authorization expired", "provider_checked_at": checkedAt, "updated_at": now}).Error; err != nil {
				return err
			}
			return tx.Model(&VirtualCharacter{}).Where("id = ?", characterID).Updates(map[string]any{"status": VirtualCharacterStatusBlocked, "last_error": "authorization expired", "updated_at": now}).Error
		}
		if err := tx.Model(&VirtualCharacter{}).Where("id = ? AND source_type = ?", characterID, VirtualCharacterSourceVolcRealPerson).Updates(map[string]any{
			"provider_asset_id": providerAssetID, "cover_url": virtualCharacterPreviewPath(characterID),
			"mime_type": "image/*", "status": VirtualCharacterStatusActive,
			"asset_next_poll_at": 0, "asset_poll_attempts": 0, "last_error": "", "updated_at": now,
		}).Error; err != nil {
			return err
		}
		return tx.Model(&authorization).Updates(map[string]any{
			"status": VirtualCharacterAuthorizationActive, "provider_group_status": "Active",
			"provider_asset_status": providerAssetStatus, "provider_checked_at": checkedAt,
			"last_error": "", "updated_at": now,
		}).Error
	})
}

func RevokeRealPersonAuthorization(characterID int64, userID int, reason string) error {
	now := time.Now().Unix()
	return DB.Transaction(func(tx *gorm.DB) error {
		var character VirtualCharacter
		if err := tx.Where("id = ? AND user_id = ? AND source_type = ?", characterID, userID, VirtualCharacterSourceVolcRealPerson).First(&character).Error; err != nil {
			return err
		}
		if err := tx.Model(&VirtualCharacterAuthorization{}).Where("character_id = ? AND user_id = ?", characterID, userID).Updates(map[string]any{
			"status": VirtualCharacterAuthorizationRevoked, "revoked_at": now,
			"last_error": reason, "updated_at": now,
		}).Error; err != nil {
			return err
		}
		return tx.Model(&character).Updates(map[string]any{
			"real_person_slot": nil, "status": VirtualCharacterStatusDeleting,
			"asset_next_poll_at": 0, "cleanup_next_at": now, "last_error": reason, "updated_at": now,
		}).Error
	})
}

func ExpireRealPersonAuthorization(characterID int64, reason string) error {
	now := time.Now().Unix()
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&VirtualCharacterAuthorization{}).Where("character_id = ? AND status NOT IN ?", characterID, []string{VirtualCharacterAuthorizationExpired, VirtualCharacterAuthorizationRevoked}).Updates(map[string]any{
			"status": VirtualCharacterAuthorizationExpired, "expired_at": now,
			"last_error": reason, "updated_at": now,
		}).Error; err != nil {
			return err
		}
		return tx.Model(&VirtualCharacter{}).Where("id = ? AND source_type = ?", characterID, VirtualCharacterSourceVolcRealPerson).Updates(map[string]any{
			"real_person_slot": nil, "status": VirtualCharacterStatusDeleting,
			"asset_next_poll_at": 0, "cleanup_next_at": now, "last_error": reason, "updated_at": now,
		}).Error
	})
}

func CreateVirtualCharacterTaskReferences(taskID string, userID int, characters []*VirtualCharacter, snapshots map[int64]string) error {
	return createVirtualCharacterTaskReferences(DB, taskID, userID, characters, snapshots)
}

func CreateVirtualCharacterTaskBinding(link *VirtualCharacterTask, characters []*VirtualCharacter, snapshots map[int64]string) error {
	if link == nil {
		return errors.New("virtual character task link is required")
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		if link.Status == "" {
			link.Status = VirtualCharacterTaskStatusSubmitting
		}
		if err := tx.Create(link).Error; err != nil {
			return err
		}
		return createVirtualCharacterTaskReferences(tx, link.TaskID, link.UserID, characters, snapshots)
	})
}

func RollbackVirtualCharacterTaskBinding(taskID string) error {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return errors.New("virtual character task id is required")
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("task_id = ?", taskID).Delete(&VirtualCharacterTaskReference{}).Error; err != nil {
			return err
		}
		return tx.Where("task_id = ? AND status = ?", taskID, VirtualCharacterTaskStatusSubmitting).Delete(&VirtualCharacterTask{}).Error
	})
}

func createVirtualCharacterTaskReferences(db *gorm.DB, taskID string, userID int, characters []*VirtualCharacter, snapshots map[int64]string) error {
	if strings.TrimSpace(taskID) == "" || userID <= 0 || len(characters) == 0 {
		return errors.New("invalid virtual character task references")
	}
	items := make([]VirtualCharacterTaskReference, 0, len(characters))
	seen := make(map[int64]struct{}, len(characters))
	for _, character := range characters {
		if character == nil || character.ID <= 0 {
			continue
		}
		if _, exists := seen[character.ID]; exists {
			continue
		}
		seen[character.ID] = struct{}{}
		items = append(items, VirtualCharacterTaskReference{
			TaskID: taskID, UserID: userID, CharacterID: character.ID,
			CharacterName: character.Name, CharacterScope: character.Scope, SourceType: character.SourceType,
			ProviderAssetID: character.ProviderAssetID, AuthorizationSnapshotJSON: snapshots[character.ID],
		})
	}
	if len(items) == 0 {
		return errors.New("virtual character task references are empty")
	}
	return db.Clauses(clause.OnConflict{DoNothing: true}).Create(&items).Error
}

func DeleteExpiredVirtualCharacterTaskReferences(cutoff int64) error {
	return DB.Where("created_at < ?", cutoff).Delete(&VirtualCharacterTaskReference{}).Error
}

func ListVirtualCharacterTaskReferences(userID int, taskIDs []string) (map[string][]VirtualCharacterTaskReference, error) {
	result := make(map[string][]VirtualCharacterTaskReference, len(taskIDs))
	if userID <= 0 || len(taskIDs) == 0 {
		return result, nil
	}
	var items []VirtualCharacterTaskReference
	if err := DB.Where("user_id = ? AND task_id IN ?", userID, taskIDs).Order("id ASC").Find(&items).Error; err != nil {
		return nil, err
	}
	for i := range items {
		item := items[i]
		result[item.TaskID] = append(result[item.TaskID], item)
	}
	return result, nil
}

func ListRealPersonVirtualCharactersDueForProviderCheck(now, staleBefore int64, limit int) ([]VirtualCharacter, error) {
	var items []VirtualCharacter
	err := DB.Table("virtual_characters").
		Select("virtual_characters.*").
		Joins("JOIN virtual_character_authorizations ON virtual_character_authorizations.character_id = virtual_characters.id").
		Where("virtual_characters.source_type = ? AND virtual_characters.status IN ?", VirtualCharacterSourceVolcRealPerson, []string{VirtualCharacterStatusActive, VirtualCharacterStatusBlocked, VirtualCharacterStatusCreating}).
		Where("virtual_characters.provider_asset_id <> ?", "").
		Where("(virtual_character_authorizations.valid_until > 0 AND virtual_character_authorizations.valid_until <= ?) OR virtual_character_authorizations.provider_checked_at <= ?", now, staleBefore).
		Order("virtual_character_authorizations.provider_checked_at ASC, virtual_characters.id ASC").Limit(limit).Scan(&items).Error
	return items, err
}

func ListExpiredPendingVirtualCharacterValidationSessions(now int64, limit int) ([]VirtualCharacterValidationSession, error) {
	var items []VirtualCharacterValidationSession
	err := DB.Where("status = ? AND expires_at <= ?", VirtualCharacterValidationPending, now).
		Order("expires_at ASC").Limit(limit).Find(&items).Error
	return items, err
}

func HasUnfinishedVirtualCharacterReferenceTasks(characterID int64) (bool, error) {
	var pendingLinks int64
	if err := DB.Table("virtual_character_task_references").
		Joins("JOIN virtual_character_tasks ON virtual_character_tasks.task_id = virtual_character_task_references.task_id").
		Where("virtual_character_task_references.character_id = ? AND virtual_character_tasks.status IN ?", characterID, []string{VirtualCharacterTaskStatusSubmitting, VirtualCharacterTaskStatusReady}).
		Count(&pendingLinks).Error; err != nil {
		return false, err
	}
	if pendingLinks > 0 {
		return true, nil
	}
	var taskIDs []string
	if err := DB.Model(&VirtualCharacterTaskReference{}).Where("character_id = ?", characterID).Pluck("task_id", &taskIDs).Error; err != nil {
		return false, err
	}
	if len(taskIDs) == 0 {
		return false, nil
	}
	var count int64
	err := DB.Model(&Task{}).Where("task_id IN ? AND status NOT IN ?", taskIDs, []TaskStatus{TaskStatusSuccess, TaskStatusFailure}).Count(&count).Error
	return count > 0, err
}

func NormalizeLegacyRealPersonSlots() error {
	var characters []VirtualCharacter
	authorizedCharacterIDs := DB.Model(&VirtualCharacterAuthorization{}).Select("character_id")
	if err := DB.Where("source_type = ?", VirtualCharacterSourceVolcRealPerson).
		Where("id NOT IN (?)", authorizedCharacterIDs).Find(&characters).Error; err != nil {
		return err
	}
	now := time.Now().Unix()
	for i := range characters {
		character := &characters[i]
		if err := DB.Transaction(func(tx *gorm.DB) error {
			if err := tx.Model(character).Updates(map[string]any{
				"real_person_slot": nil, "slot": nil, "status": VirtualCharacterStatusBlocked,
				"validation_status": VirtualCharacterValidationRejected,
				"last_error":        "legacy real-person row requires fresh authorization",
			}).Error; err != nil {
				return err
			}
			item := VirtualCharacterAuthorization{
				CharacterID: character.ID, UserID: character.UserID,
				Status: VirtualCharacterAuthorizationFailed, ValidFrom: now, ValidUntil: now,
				AgreementVersion: "legacy-unverified", ProviderGroupType: VirtualCharacterRealPersonGroupType,
				LastError: "legacy real-person row requires fresh authorization",
			}
			return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&item).Error
		}); err != nil {
			return err
		}
	}
	return nil
}
