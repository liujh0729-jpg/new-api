package model

import (
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	VirtualCharacterScopePrivate = "private"
	VirtualCharacterScopePublic  = "public"

	VirtualCharacterSourceAIPDD          = "aipdd"
	VirtualCharacterSourceVolc           = "volc"
	VirtualCharacterSourceVolcPreset     = "volc_preset"
	VirtualCharacterSourceVolcAIGC       = "volc_aigc"
	VirtualCharacterSourceVolcRealPerson = "volc_real_person"

	VirtualCharacterDefaultAccountAssetCap = 50
	VirtualCharacterDefaultCreateAssetQPM  = 3

	// Volc Assets account quota plans (local guardrails; not queried from Volc).
	VirtualCharacterQuotaPlanFree   = "free"
	VirtualCharacterQuotaPlanPaid   = "paid"
	VirtualCharacterQuotaPlanCustom = "custom"

	VirtualCharacterPaidAccountAssetCap = 1000000
	VirtualCharacterPaidCreateAssetQPM  = 120

	VirtualCharacterStatusCreating = "creating"
	VirtualCharacterStatusActive   = "active"
	VirtualCharacterStatusBlocked  = "blocked"
	VirtualCharacterStatusOffline  = "offline"
	VirtualCharacterStatusDeleting = "deleting"
	VirtualCharacterStatusFailed   = "failed"

	VirtualCharacterValidationUnverified = "unverified"
	VirtualCharacterValidationAccepted   = "accepted"
	VirtualCharacterValidationRejected   = "rejected"

	VirtualCharacterTaskStatusSubmitting = "submitting"
	VirtualCharacterTaskStatusReady      = "ready"
	VirtualCharacterTaskStatusRecovering = "recovering"
	VirtualCharacterTaskStatusActive     = "active"
	VirtualCharacterTaskStatusFailed     = "failed"

	VirtualCharacterTaskAction = "virtual_character_video"

	VirtualCharacterDefaultLimit = 100
)

// Seedance / AIPDD official catalog facet values (exact-match filters).
var (
	VirtualCharacterNationalities = []string{"中国", "美国", "日本", "韩国", "英国", "法国", "印度", "巴西"}
	VirtualCharacterGenders       = []string{"男", "女"}
	VirtualCharacterAgeBands      = []VirtualCharacterAgeBand{
		{Key: "0-20", Min: 0, Max: 20},
		{Key: "20-40", Min: 20, Max: 40},
		{Key: "40-60", Min: 40, Max: 60},
		{Key: "60-80", Min: 60, Max: 80},
		{Key: "80-100", Min: 80, Max: 100},
	}
	virtualCharacterAgeBandPattern = regexp.MustCompile(`^(\d+)\s*[-~～]\s*(\d+)\s*岁?$`)
	virtualCharacterNationalitySet = virtualCharacterStringSet(VirtualCharacterNationalities)
	virtualCharacterGenderSet      = virtualCharacterStringSet(VirtualCharacterGenders)
)

type VirtualCharacterAgeBand struct {
	Key string
	Min int
	Max int
}

// VirtualCharacterListFilter holds optional list query facets.
type VirtualCharacterListFilter struct {
	Keyword     string
	Nationality string
	Gender      string
	AgeMin      *int
	AgeMax      *int
	Status      string
	SourceType  string
}

// VirtualCharacter stores role-library metadata. Private provider binary content
// lives in Volc Asset Groups; AIPDD is used only as temporary staging for uploads.
type VirtualCharacter struct {
	ID                int64          `json:"id" gorm:"primaryKey;autoIncrement"`
	UserID            int            `json:"user_id" gorm:"index;uniqueIndex:uk_virtual_character_user_slot"`
	Slot              *int           `json:"-" gorm:"uniqueIndex:uk_virtual_character_user_slot"`
	RealPersonSlot    *int           `json:"-" gorm:"uniqueIndex:uk_virtual_character_real_user_slot"`
	Scope             string         `json:"scope" gorm:"type:varchar(16);index"`
	Name              string         `json:"name" gorm:"type:varchar(191);index"`
	Description       string         `json:"description" gorm:"type:text"`
	TagsJSON          string         `json:"-" gorm:"type:text"`
	Nationality       string         `json:"nationality,omitempty" gorm:"type:varchar(64);index:idx_virtual_character_nationality_gender,priority:1"`
	Gender            string         `json:"gender,omitempty" gorm:"type:varchar(32);index:idx_virtual_character_nationality_gender,priority:2"`
	AgeMin            *int           `json:"age_min,omitempty"`
	AgeMax            *int           `json:"age_max,omitempty"`
	Occupation        string         `json:"occupation,omitempty" gorm:"type:varchar(128)"`
	Temperament       string         `json:"temperament,omitempty" gorm:"type:varchar(128)"`
	SourceType        string         `json:"source_type" gorm:"type:varchar(16);index"`
	Status            string         `json:"status" gorm:"type:varchar(20);index"`
	ValidationStatus  string         `json:"validation_status" gorm:"type:varchar(20);index"`
	CoverURL          string         `json:"cover_url,omitempty" gorm:"type:text"`
	AIPDDAssetID      int64          `json:"-" gorm:"index"`                   // deprecated: legacy private fictional path
	AIPDDFileID       string         `json:"-" gorm:"type:varchar(191);index"` // deprecated: legacy private fictional path
	VolcAssetID       string         `json:"-" gorm:"type:varchar(191);index"` // deprecated: one-time migration source for ProviderAssetID
	PublicChannelID   int            `json:"-" gorm:"index"`                   // deprecated legacy catalog field
	ProviderAccountID int            `json:"provider_account_id,omitempty" gorm:"index"`
	ProviderGroupID   string         `json:"provider_group_id,omitempty" gorm:"type:varchar(191);index"`
	ProviderAssetID   string         `json:"provider_asset_id,omitempty" gorm:"type:varchar(191);index"`
	StagingFileID     string         `json:"-" gorm:"type:varchar(191);index"`
	AssetPollAttempts int            `json:"-"`
	AssetNextPollAt   int64          `json:"-" gorm:"index"`
	CatalogVersion    string         `json:"catalog_version,omitempty" gorm:"type:varchar(191);index"`
	MimeType          string         `json:"mime_type,omitempty" gorm:"type:varchar(100)"`
	FileSize          int64          `json:"file_size,omitempty"`
	LastError         string         `json:"last_error,omitempty" gorm:"type:text"`
	CleanupAttempts   int            `json:"-"`
	CleanupNextAt     int64          `json:"-" gorm:"index"`
	CreatedAt         int64          `json:"created_at" gorm:"autoCreateTime;index"`
	UpdatedAt         int64          `json:"updated_at" gorm:"autoUpdateTime"`
	DeletedAt         gorm.DeletedAt `json:"-" gorm:"index"`
}

type VirtualCharacterUserLimit struct {
	UserID    int   `json:"user_id" gorm:"primaryKey"`
	Limit     int   `json:"limit"`
	CreatedAt int64 `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt int64 `json:"updated_at" gorm:"autoUpdateTime"`
}

// VirtualCharacterTask is both the role snapshot used by task history and a
// small recovery outbox for the gap between upstream acceptance and Task insert.
type VirtualCharacterTask struct {
	ID                        int64  `json:"id" gorm:"primaryKey;autoIncrement"`
	TaskID                    string `json:"task_id" gorm:"type:varchar(191);uniqueIndex"`
	UserID                    int    `json:"user_id" gorm:"index"`
	CharacterID               int64  `json:"character_id" gorm:"index"`
	CharacterName             string `json:"character_name" gorm:"type:varchar(191)"`
	CharacterScope            string `json:"character_scope" gorm:"type:varchar(16)"`
	ProviderAssetID           string `json:"provider_asset_id,omitempty" gorm:"type:varchar(191)"`
	AuthorizationSnapshotJSON string `json:"-" gorm:"type:text"`
	Status                    string `json:"status" gorm:"type:varchar(20);index"`
	UpstreamTaskID            string `json:"-" gorm:"type:varchar(191)"`
	ChannelID                 int    `json:"-" gorm:"index"`
	TaskPayloadJSON           string `json:"-" gorm:"type:text"`
	LastError                 string `json:"last_error,omitempty" gorm:"type:text"`
	RetryCount                int    `json:"-"`
	NextRetryAt               int64  `json:"-" gorm:"index"`
	TerminalCheckedAt         int64  `json:"-" gorm:"index"`
	CreatedAt                 int64  `json:"created_at" gorm:"autoCreateTime;index"`
	UpdatedAt                 int64  `json:"updated_at" gorm:"autoUpdateTime"`
}

func GetVirtualCharacterGlobalLimit() int {
	common.OptionMapRWMutex.RLock()
	raw := strings.TrimSpace(common.OptionMap["VirtualCharacterLimit"])
	common.OptionMapRWMutex.RUnlock()
	limit, err := strconv.Atoi(raw)
	if err != nil || limit <= 0 {
		return VirtualCharacterDefaultLimit
	}
	if limit > 10000 {
		return 10000
	}
	return limit
}

func GetVirtualCharacterAccountAssetCap() int {
	common.OptionMapRWMutex.RLock()
	raw := strings.TrimSpace(common.OptionMap["VirtualCharacterAccountAssetCap"])
	common.OptionMapRWMutex.RUnlock()
	limit, err := strconv.Atoi(raw)
	if err != nil || limit <= 0 {
		return VirtualCharacterDefaultAccountAssetCap
	}
	if limit > 5000000 {
		return 5000000
	}
	return limit
}

// NormalizeVirtualCharacterQuotaPlan returns a known plan and the asset-cap / CreateAsset QPM
// that should be applied for that plan. free/paid overwrite the numeric inputs; custom keeps them.
func NormalizeVirtualCharacterQuotaPlan(plan string, assetCap, createAssetQPM int) (normalized string, nextAssetCap, nextQPM int) {
	switch strings.ToLower(strings.TrimSpace(plan)) {
	case VirtualCharacterQuotaPlanPaid:
		return VirtualCharacterQuotaPlanPaid, VirtualCharacterPaidAccountAssetCap, VirtualCharacterPaidCreateAssetQPM
	case VirtualCharacterQuotaPlanCustom:
		if assetCap <= 0 {
			assetCap = VirtualCharacterDefaultAccountAssetCap
		}
		if createAssetQPM <= 0 {
			createAssetQPM = VirtualCharacterDefaultCreateAssetQPM
		}
		if createAssetQPM > 300 {
			createAssetQPM = 300
		}
		if assetCap > 5000000 {
			assetCap = 5000000
		}
		return VirtualCharacterQuotaPlanCustom, assetCap, createAssetQPM
	default:
		return VirtualCharacterQuotaPlanFree, VirtualCharacterDefaultAccountAssetCap, VirtualCharacterDefaultCreateAssetQPM
	}
}

// IsVirtualCharacterSeedanceModel reports whether the model name can be used
// for character-library video generation (name contains "seedance").
func IsVirtualCharacterSeedanceModel(modelName string) bool {
	return strings.Contains(strings.ToLower(strings.TrimSpace(modelName)), "seedance")
}

func GetVirtualCharacterEffectiveLimit(userID int) int {
	return getVirtualCharacterEffectiveLimitDB(DB, userID)
}

func getVirtualCharacterEffectiveLimitDB(db *gorm.DB, userID int) int {
	var override VirtualCharacterUserLimit
	if err := db.First(&override, "user_id = ?", userID).Error; err == nil && override.Limit > 0 {
		return override.Limit
	}
	return GetVirtualCharacterGlobalLimit()
}

func SetVirtualCharacterUserLimit(userID int, limit int) error {
	if userID <= 0 || limit < 0 || limit > 10000 {
		return errors.New("invalid virtual character user limit")
	}
	if limit == 0 {
		return DB.Delete(&VirtualCharacterUserLimit{}, "user_id = ?", userID).Error
	}
	now := time.Now().Unix()
	item := VirtualCharacterUserLimit{UserID: userID, Limit: limit, CreatedAt: now, UpdatedAt: now}
	return DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"limit", "updated_at"}),
	}).Create(&item).Error
}

func ReservePrivateVirtualCharacter(userID int, name, description, tagsJSON, mimeType string, fileSize int64) (*VirtualCharacter, int, error) {
	if userID <= 0 {
		return nil, 0, errors.New("invalid user id")
	}
	limit := GetVirtualCharacterEffectiveLimit(userID)
	for slot := 1; slot <= limit; slot++ {
		candidateSlot := slot
		item := &VirtualCharacter{
			UserID:           userID,
			Slot:             &candidateSlot,
			Scope:            VirtualCharacterScopePrivate,
			Name:             name,
			Description:      description,
			TagsJSON:         tagsJSON,
			SourceType:       VirtualCharacterSourceAIPDD,
			Status:           VirtualCharacterStatusCreating,
			ValidationStatus: VirtualCharacterValidationUnverified,
			MimeType:         mimeType,
			FileSize:         fileSize,
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

func CountActivePrivateVirtualCharacters(userID int) (int64, error) {
	var count int64
	err := DB.Model(&VirtualCharacter{}).
		Where("user_id = ? AND scope = ? AND source_type <> ? AND slot IS NOT NULL", userID, VirtualCharacterScopePrivate, VirtualCharacterSourceVolcRealPerson).
		Count(&count).Error
	return count, err
}

func GetVirtualCharacterByID(id int64) (*VirtualCharacter, error) {
	var item VirtualCharacter
	err := DB.First(&item, id).Error
	return &item, err
}

func GetAccessibleVirtualCharacter(id int64, userID int) (*VirtualCharacter, error) {
	var item VirtualCharacter
	err := DB.Where("id = ? AND ((scope = ? AND status = ?) OR (scope = ? AND user_id = ?))",
		id,
		VirtualCharacterScopePublic, VirtualCharacterStatusActive,
		VirtualCharacterScopePrivate, userID,
	).First(&item).Error
	return &item, err
}

func GetOwnedVirtualCharacter(id int64, userID int) (*VirtualCharacter, error) {
	var item VirtualCharacter
	err := DB.Where("id = ? AND scope = ? AND user_id = ?", id, VirtualCharacterScopePrivate, userID).First(&item).Error
	return &item, err
}

func ListVirtualCharacters(userID int, scope string, includeOffline bool, filter VirtualCharacterListFilter, offset, limit int) ([]VirtualCharacter, int64, error) {
	query := DB.Model(&VirtualCharacter{})
	switch scope {
	case VirtualCharacterScopePrivate, "":
		query = query.Where("scope = ? AND user_id = ?", VirtualCharacterScopePrivate, userID)
	case VirtualCharacterScopePublic:
		query = query.Where("scope = ?", VirtualCharacterScopePublic)
		if !includeOffline {
			query = query.Where("status = ?", VirtualCharacterStatusActive)
		}
	default:
		return nil, 0, errors.New("invalid virtual character scope")
	}
	query = applyVirtualCharacterListFilter(query, filter)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var items []VirtualCharacter
	err := query.Order("id DESC").Offset(offset).Limit(limit).Find(&items).Error
	return items, total, err
}

func applyVirtualCharacterListFilter(query *gorm.DB, filter VirtualCharacterListFilter) *gorm.DB {
	if keyword := strings.TrimSpace(filter.Keyword); keyword != "" {
		like := "%" + escapeVirtualCharacterLike(keyword) + "%"
		query = query.Where(
			"(name LIKE ? ESCAPE '!' OR description LIKE ? ESCAPE '!' OR occupation LIKE ? ESCAPE '!' OR temperament LIKE ? ESCAPE '!' OR tags_json LIKE ? ESCAPE '!')",
			like, like, like, like, like,
		)
	}
	if nationality := strings.TrimSpace(filter.Nationality); nationality != "" {
		query = query.Where("nationality = ?", nationality)
	}
	if gender := strings.TrimSpace(filter.Gender); gender != "" {
		query = query.Where("gender = ?", gender)
	}
	if filter.AgeMin != nil && filter.AgeMax != nil {
		// Official catalog stores discrete Seedance bands (e.g. 20-40 / 40-60).
		// Exact match avoids double-counting characters that sit on shared band edges.
		query = query.Where("age_min = ? AND age_max = ?", *filter.AgeMin, *filter.AgeMax)
	}
	if status := strings.TrimSpace(filter.Status); status != "" {
		query = query.Where("status = ?", status)
	}
	if sourceType := strings.TrimSpace(filter.SourceType); sourceType != "" {
		query = query.Where("source_type = ?", sourceType)
	}
	return query
}

func escapeVirtualCharacterLike(value string) string {
	value = strings.ReplaceAll(value, "!", "!!")
	value = strings.ReplaceAll(value, "%", "!%")
	value = strings.ReplaceAll(value, "_", "!_")
	return value
}

func virtualCharacterStringSet(values []string) map[string]struct{} {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		set[value] = struct{}{}
	}
	return set
}

// ParseVirtualCharacterAgeBandKey accepts "20-40" or "20-40岁" and returns the band bounds.
func ParseVirtualCharacterAgeBandKey(value string) (min, max int, ok bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, 0, false
	}
	for _, band := range VirtualCharacterAgeBands {
		if value == band.Key || value == band.Key+"岁" {
			return band.Min, band.Max, true
		}
	}
	matches := virtualCharacterAgeBandPattern.FindStringSubmatch(value)
	if len(matches) != 3 {
		return 0, 0, false
	}
	parsedMin, errMin := strconv.Atoi(matches[1])
	parsedMax, errMax := strconv.Atoi(matches[2])
	if errMin != nil || errMax != nil || parsedMin > parsedMax {
		return 0, 0, false
	}
	return parsedMin, parsedMax, true
}

// ParseVirtualCharacterAgeBandFromTags finds the first "N-M岁" style tag.
func ParseVirtualCharacterAgeBandFromTags(tags []string) (min, max int, ok bool) {
	for _, tag := range tags {
		if parsedMin, parsedMax, parsed := ParseVirtualCharacterAgeBandKey(tag); parsed {
			return parsedMin, parsedMax, true
		}
	}
	return 0, 0, false
}

func GuessVirtualCharacterNationalityFromTags(tags []string) string {
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if _, exists := virtualCharacterNationalitySet[tag]; exists {
			return tag
		}
	}
	return ""
}

func GuessVirtualCharacterGenderFromTags(tags []string) string {
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if _, exists := virtualCharacterGenderSet[tag]; exists {
			return tag
		}
	}
	return ""
}

func FormatVirtualCharacterAgeLabel(ageMin, ageMax *int) string {
	if ageMin == nil || ageMax == nil {
		return ""
	}
	return strconv.Itoa(*ageMin) + "-" + strconv.Itoa(*ageMax) + "岁"
}

// BackfillVirtualCharacterStructuredFacets fills nationality/gender/age from tags
// for public catalog rows that predate structured columns.
func BackfillVirtualCharacterStructuredFacets() error {
	var items []VirtualCharacter
	if err := DB.Where(
		"scope = ? AND source_type = ? AND (nationality = ? OR gender = ? OR age_min IS NULL OR age_max IS NULL)",
		VirtualCharacterScopePublic, VirtualCharacterSourceVolcPreset, "", "",
	).Find(&items).Error; err != nil {
		return err
	}
	for i := range items {
		item := &items[i]
		tags := decodeVirtualCharacterTagsJSON(item.TagsJSON)
		updates := map[string]any{}
		if strings.TrimSpace(item.Nationality) == "" {
			if nationality := GuessVirtualCharacterNationalityFromTags(tags); nationality != "" {
				updates["nationality"] = nationality
			}
		}
		if strings.TrimSpace(item.Gender) == "" {
			if gender := GuessVirtualCharacterGenderFromTags(tags); gender != "" {
				updates["gender"] = gender
			}
		}
		if item.AgeMin == nil || item.AgeMax == nil {
			if ageMin, ageMax, ok := ParseVirtualCharacterAgeBandFromTags(tags); ok {
				if item.AgeMin == nil {
					updates["age_min"] = ageMin
				}
				if item.AgeMax == nil {
					updates["age_max"] = ageMax
				}
			}
		}
		if len(updates) == 0 {
			continue
		}
		updates["updated_at"] = time.Now().Unix()
		if err := DB.Model(item).Updates(updates).Error; err != nil {
			return err
		}
	}
	return nil
}

func decodeVirtualCharacterTagsJSON(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var tags []string
	if err := common.Unmarshal([]byte(raw), &tags); err != nil {
		return nil
	}
	return tags
}

func UpdateVirtualCharacterMetadata(item *VirtualCharacter, name, description, tagsJSON string) error {
	return DB.Model(item).Updates(map[string]any{
		"name":        name,
		"description": description,
		"tags_json":   tagsJSON,
		"updated_at":  time.Now().Unix(),
	}).Error
}

func MarkVirtualCharacterStorage(itemID int64, fileID string, assetID int64) error {
	updates := map[string]any{"updated_at": time.Now().Unix()}
	if strings.TrimSpace(fileID) != "" {
		updates["a_ip_dd_file_id"] = fileID
	}
	if assetID > 0 {
		updates["a_ip_dd_asset_id"] = assetID
	}
	return DB.Model(&VirtualCharacter{}).Where("id = ?", itemID).Updates(updates).Error
}

func ActivateVirtualCharacter(itemID int64) error {
	return DB.Model(&VirtualCharacter{}).Where("id = ?", itemID).Updates(map[string]any{
		"status":     VirtualCharacterStatusActive,
		"last_error": "",
		"updated_at": time.Now().Unix(),
	}).Error
}

func MarkVirtualCharacterBlocked(itemID int64, reason string) error {
	return DB.Model(&VirtualCharacter{}).Where("id = ?", itemID).Updates(map[string]any{
		"status":            VirtualCharacterStatusBlocked,
		"validation_status": VirtualCharacterValidationRejected,
		"last_error":        reason,
		"updated_at":        time.Now().Unix(),
	}).Error
}

func BeginVirtualCharacterDelete(item *VirtualCharacter, reason string) error {
	if item == nil || item.ID == 0 {
		return errors.New("invalid virtual character")
	}
	now := time.Now().Unix()
	return DB.Model(item).Updates(map[string]any{
		"slot":            nil,
		"status":          VirtualCharacterStatusDeleting,
		"last_error":      reason,
		"cleanup_next_at": now,
		"updated_at":      now,
	}).Error
}

func ListVirtualCharactersPendingCleanup(now int64, limit int) ([]VirtualCharacter, error) {
	var items []VirtualCharacter
	err := DB.Where("scope = ? AND status = ? AND cleanup_next_at <= ?",
		VirtualCharacterScopePrivate, VirtualCharacterStatusDeleting, now).
		Order("cleanup_next_at ASC, id ASC").Limit(limit).Find(&items).Error
	return items, err
}

func MarkOrphanedVirtualCharactersForCleanup(now int64) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		activeUserIDs := tx.Model(&User{}).Select("id")
		if err := tx.Model(&VirtualCharacter{}).
			Where("scope = ? AND status <> ?", VirtualCharacterScopePrivate, VirtualCharacterStatusDeleting).
			Where("user_id NOT IN (?)", activeUserIDs).
			Updates(map[string]any{
				"slot": nil, "real_person_slot": nil,
				"status": VirtualCharacterStatusDeleting, "last_error": "owning account was deleted",
				"cleanup_next_at": now, "updated_at": now,
			}).Error; err != nil {
			return err
		}
		orphanedRealPeople := tx.Model(&VirtualCharacter{}).Select("id").
			Where("scope = ? AND source_type = ? AND status = ?", VirtualCharacterScopePrivate, VirtualCharacterSourceVolcRealPerson, VirtualCharacterStatusDeleting)
		return tx.Model(&VirtualCharacterAuthorization{}).
			Where("character_id IN (?) AND status NOT IN ?", orphanedRealPeople, []string{VirtualCharacterAuthorizationRevoked, VirtualCharacterAuthorizationExpired}).
			Updates(map[string]any{
				"status": VirtualCharacterAuthorizationRevoked, "revoked_at": now,
				"last_error": "owning account was deleted", "updated_at": now,
			}).Error
	})
}

func RetryVirtualCharacterCleanup(itemID int64, attempts int, nextAt int64, lastError string) error {
	return DB.Model(&VirtualCharacter{}).Where("id = ?", itemID).Updates(map[string]any{
		"cleanup_attempts": attempts,
		"cleanup_next_at":  nextAt,
		"last_error":       lastError,
		"updated_at":       time.Now().Unix(),
	}).Error
}

func CompleteVirtualCharacterCleanup(itemID int64) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&VirtualCharacter{}).Where("id = ?", itemID).Updates(map[string]any{
			"status":           VirtualCharacterStatusFailed,
			"a_ip_dd_asset_id": 0,
			"a_ip_dd_file_id":  "",
			"cleanup_next_at":  0,
			"last_error":       "",
			"updated_at":       time.Now().Unix(),
		}).Error; err != nil {
			return err
		}
		return tx.Delete(&VirtualCharacter{}, itemID).Error
	})
}

func CreatePublicVirtualCharacter(item *VirtualCharacter) error {
	if item == nil || strings.TrimSpace(item.VolcAssetID) == "" || item.PublicChannelID <= 0 {
		return errors.New("public channel and Volc asset ID are required")
	}
	item.UserID = 0
	item.Slot = nil
	item.Scope = VirtualCharacterScopePublic
	item.SourceType = VirtualCharacterSourceVolc
	item.ValidationStatus = VirtualCharacterValidationAccepted
	if item.Status == "" {
		item.Status = VirtualCharacterStatusActive
	}
	return DB.Create(item).Error
}

func UpsertPublicVirtualCharacter(item *VirtualCharacter) error {
	return upsertPublicVirtualCharacter(DB, item)
}

func UpsertPublicVirtualCharacters(items []*VirtualCharacter) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		for _, item := range items {
			if err := upsertPublicVirtualCharacter(tx, item); err != nil {
				return err
			}
		}
		return nil
	})
}

func upsertPublicVirtualCharacter(db *gorm.DB, item *VirtualCharacter) error {
	if item == nil || strings.TrimSpace(item.VolcAssetID) == "" || item.PublicChannelID <= 0 {
		return errors.New("public channel and Volc asset ID are required")
	}
	var existing VirtualCharacter
	err := db.Where("scope = ? AND public_channel_id = ? AND volc_asset_id = ?",
		VirtualCharacterScopePublic, item.PublicChannelID, item.VolcAssetID).First(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		item.UserID = 0
		item.Slot = nil
		item.Scope = VirtualCharacterScopePublic
		item.SourceType = VirtualCharacterSourceVolc
		item.ValidationStatus = VirtualCharacterValidationAccepted
		if item.Status == "" {
			item.Status = VirtualCharacterStatusActive
		}
		return db.Create(item).Error
	}
	if err != nil {
		return err
	}
	return db.Model(&existing).Updates(map[string]any{
		"name":              item.Name,
		"description":       item.Description,
		"tags_json":         item.TagsJSON,
		"cover_url":         item.CoverURL,
		"status":            item.Status,
		"validation_status": VirtualCharacterValidationAccepted,
		"updated_at":        time.Now().Unix(),
	}).Error
}

func UpdatePublicVirtualCharacter(item *VirtualCharacter) error {
	if item == nil || item.ID == 0 || item.Scope != VirtualCharacterScopePublic {
		return errors.New("invalid public virtual character")
	}
	return DB.Model(&VirtualCharacter{}).Where("id = ? AND scope = ?", item.ID, VirtualCharacterScopePublic).Updates(map[string]any{
		"name":              item.Name,
		"description":       item.Description,
		"tags_json":         item.TagsJSON,
		"cover_url":         item.CoverURL,
		"volc_asset_id":     item.VolcAssetID,
		"public_channel_id": item.PublicChannelID,
		"status":            item.Status,
		"validation_status": VirtualCharacterValidationAccepted,
		"updated_at":        time.Now().Unix(),
	}).Error
}

func DeletePublicVirtualCharacter(id int64) error {
	return DB.Model(&VirtualCharacter{}).Where("id = ? AND scope = ?", id, VirtualCharacterScopePublic).Updates(map[string]any{
		"status":     VirtualCharacterStatusOffline,
		"updated_at": time.Now().Unix(),
	}).Error
}

func CreateVirtualCharacterTaskLink(link *VirtualCharacterTask) error {
	if link == nil || strings.TrimSpace(link.TaskID) == "" || link.UserID <= 0 || link.CharacterID <= 0 {
		return errors.New("invalid virtual character task link")
	}
	if link.Status == "" {
		link.Status = VirtualCharacterTaskStatusSubmitting
	}
	return DB.Create(link).Error
}

func MarkVirtualCharacterTaskReady(taskID, upstreamTaskID string, channelID int, payloadJSON string) error {
	return DB.Model(&VirtualCharacterTask{}).Where("task_id = ?", taskID).Updates(map[string]any{
		"status":            VirtualCharacterTaskStatusReady,
		"upstream_task_id":  upstreamTaskID,
		"channel_id":        channelID,
		"task_payload_json": payloadJSON,
		"last_error":        "",
		"next_retry_at":     time.Now().Unix(),
		"updated_at":        time.Now().Unix(),
	}).Error
}

func MarkVirtualCharacterTaskActive(taskID string) error {
	return DB.Model(&VirtualCharacterTask{}).Where("task_id = ?", taskID).Updates(map[string]any{
		"status":            VirtualCharacterTaskStatusActive,
		"task_payload_json": "",
		"last_error":        "",
		"next_retry_at":     0,
		"updated_at":        time.Now().Unix(),
	}).Error
}

func MarkVirtualCharacterTaskFailed(taskID, reason string) error {
	return DB.Model(&VirtualCharacterTask{}).Where("task_id = ?", taskID).Updates(map[string]any{
		"status":     VirtualCharacterTaskStatusFailed,
		"last_error": reason,
		"updated_at": time.Now().Unix(),
	}).Error
}

func FailStaleVirtualCharacterTaskLinks(cutoff int64, reason string) error {
	return DB.Model(&VirtualCharacterTask{}).
		Where("status = ? AND created_at < ?", VirtualCharacterTaskStatusSubmitting, cutoff).
		Updates(map[string]any{
			"status":     VirtualCharacterTaskStatusFailed,
			"last_error": reason,
			"updated_at": time.Now().Unix(),
		}).Error
}

func ListVirtualCharacterTasksReady(now int64, limit int) ([]VirtualCharacterTask, error) {
	var items []VirtualCharacterTask
	err := DB.Where("status = ? AND next_retry_at <= ?", VirtualCharacterTaskStatusReady, now).
		Order("next_retry_at ASC, id ASC").Limit(limit).Find(&items).Error
	return items, err
}

func RetryVirtualCharacterTask(taskID string, retryCount int, nextRetryAt int64, reason string) error {
	return DB.Model(&VirtualCharacterTask{}).Where("task_id = ?", taskID).Updates(map[string]any{
		"retry_count":   retryCount,
		"next_retry_at": nextRetryAt,
		"last_error":    reason,
		"updated_at":    time.Now().Unix(),
	}).Error
}

func RecoverVirtualCharacterTask(item *VirtualCharacterTask) error {
	if item == nil || item.Status != VirtualCharacterTaskStatusReady || strings.TrimSpace(item.TaskPayloadJSON) == "" {
		return errors.New("virtual character task is not recoverable")
	}
	var task Task
	if err := common.UnmarshalJsonStr(item.TaskPayloadJSON, &task); err != nil {
		return err
	}
	if task.TaskID != item.TaskID || task.UserId != item.UserID {
		return errors.New("virtual character task recovery payload mismatch")
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		claim := tx.Model(&VirtualCharacterTask{}).
			Where("task_id = ? AND status = ?", item.TaskID, VirtualCharacterTaskStatusReady).
			Update("status", VirtualCharacterTaskStatusRecovering)
		if claim.Error != nil {
			return claim.Error
		}
		if claim.RowsAffected == 0 {
			var count int64
			if err := tx.Model(&Task{}).Where("task_id = ? AND user_id = ?", item.TaskID, item.UserID).Count(&count).Error; err != nil {
				return err
			}
			if count > 0 {
				return nil
			}
			return errors.New("virtual character task recovery was not claimed")
		}
		var existingTasks int64
		if err := tx.Model(&Task{}).Where("task_id = ? AND user_id = ?", item.TaskID, item.UserID).Count(&existingTasks).Error; err != nil {
			return err
		}
		if existingTasks == 0 {
			if err := tx.Create(&task).Error; err != nil {
				return err
			}
		}
		return tx.Model(&VirtualCharacterTask{}).Where("task_id = ? AND status = ?", item.TaskID, VirtualCharacterTaskStatusRecovering).Updates(map[string]any{
			"status":            VirtualCharacterTaskStatusActive,
			"task_payload_json": "",
			"last_error":        "",
			"next_retry_at":     0,
			"updated_at":        time.Now().Unix(),
		}).Error
	})
}

func ListUncheckedTerminalVirtualCharacterTasks(limit int) ([]VirtualCharacterTask, error) {
	var items []VirtualCharacterTask
	err := DB.Table("virtual_character_tasks").
		Select("virtual_character_tasks.*").
		Joins("JOIN tasks ON tasks.task_id = virtual_character_tasks.task_id").
		Where("virtual_character_tasks.terminal_checked_at = ? AND tasks.status IN ?", 0, []TaskStatus{TaskStatusSuccess, TaskStatusFailure}).
		Order("virtual_character_tasks.id ASC").Limit(limit).Scan(&items).Error
	return items, err
}

func MarkVirtualCharacterTaskTerminalChecked(taskID string) error {
	return DB.Model(&VirtualCharacterTask{}).Where("task_id = ?", taskID).Update("terminal_checked_at", time.Now().Unix()).Error
}

func DeleteExpiredVirtualCharacterTaskLinks(cutoff int64) error {
	return DB.Where("created_at < ?", cutoff).Delete(&VirtualCharacterTask{}).Error
}

func ListVirtualCharacterTaskLinks(userID int, offset, limit int, retentionCutoff int64) ([]VirtualCharacterTask, int64, error) {
	query := DB.Model(&VirtualCharacterTask{}).Where("user_id = ? AND created_at >= ?", userID, retentionCutoff)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var items []VirtualCharacterTask
	err := query.Order("id DESC").Offset(offset).Limit(limit).Find(&items).Error
	return items, total, err
}

func GetVirtualCharacterTasksByIDs(userID int, taskIDs []string) (map[string]*Task, error) {
	result := make(map[string]*Task, len(taskIDs))
	if len(taskIDs) == 0 {
		return result, nil
	}
	var tasks []*Task
	if err := DB.Where("user_id = ? AND task_id IN ?", userID, taskIDs).Find(&tasks).Error; err != nil {
		return nil, err
	}
	for _, task := range tasks {
		if task != nil {
			result[task.TaskID] = task
		}
	}
	return result, nil
}

func HasUnfinishedVirtualCharacterTasks(characterID int64) (bool, error) {
	var pendingLinks int64
	if err := DB.Model(&VirtualCharacterTask{}).
		Where("character_id = ? AND status IN ?", characterID, []string{
			VirtualCharacterTaskStatusSubmitting,
			VirtualCharacterTaskStatusReady,
		}).Count(&pendingLinks).Error; err != nil {
		return false, err
	}
	if pendingLinks > 0 {
		return true, nil
	}
	var taskIDs []string
	if err := DB.Model(&VirtualCharacterTask{}).Where("character_id = ?", characterID).Pluck("task_id", &taskIDs).Error; err != nil {
		return false, err
	}
	if len(taskIDs) == 0 {
		return HasUnfinishedVirtualCharacterReferenceTasks(characterID)
	}
	var count int64
	err := DB.Model(&Task{}).Where("task_id IN ? AND status NOT IN ?", taskIDs, []TaskStatus{TaskStatusSuccess, TaskStatusFailure}).Count(&count).Error
	if err != nil || count > 0 {
		return count > 0, err
	}
	return HasUnfinishedVirtualCharacterReferenceTasks(characterID)
}
