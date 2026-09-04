package model

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/cachex"
	"github.com/samber/hot"
	"gorm.io/gorm"
)

const (
	MembershipMultiplierScale int64 = 1_000_000

	DefaultMembershipCode        = "NORMAL"
	DefaultMembershipDisplayName = "普通用户"

	UserMembershipStatusActive  = "active"
	UserMembershipStatusRevoked = "revoked"

	UserMembershipSourceAdmin     = "admin"
	UserMembershipSourceMigration = "migration"
)

var (
	ErrMembershipLevelNotFound = errors.New("membership level not found")
	ErrUserMembershipNotFound  = errors.New("user membership not found")
)

const membershipSnapshotCacheNamespace = "new-api:user_membership_snapshot:v1"

var (
	membershipSnapshotCacheOnce sync.Once
	membershipSnapshotCache     *cachex.HybridCache[MembershipSnapshot]
)

// MembershipLevel is an independent commercial discount level. It deliberately
// does not reuse the NewAPI group field, which continues to control routing and
// group pricing.
type MembershipLevel struct {
	Id            int    `json:"id"`
	Code          string `json:"code" gorm:"type:varchar(64);not null;uniqueIndex"`
	DisplayName   string `json:"display_name" gorm:"type:varchar(128);not null"`
	MultiplierPPM int64  `json:"multiplier_ppm" gorm:"type:bigint;not null;default:1000000"`
	Rank          int    `json:"rank" gorm:"type:int;not null;default:0;index"`
	SortOrder     int    `json:"sort_order" gorm:"type:int;not null;default:0"`
	Enabled       bool   `json:"enabled" gorm:"not null;default:true;index"`
	IsDefault     bool   `json:"is_default" gorm:"not null;default:false;index"`
	ArchivedAt    int64  `json:"archived_at" gorm:"type:bigint;not null;default:0;index"`
	CreatedAt     int64  `json:"created_at" gorm:"type:bigint;not null"`
	UpdatedAt     int64  `json:"updated_at" gorm:"type:bigint;not null"`
}

func (level *MembershipLevel) BeforeCreate(_ *gorm.DB) error {
	if err := level.NormalizeAndValidate(); err != nil {
		return err
	}
	now := common.GetTimestamp()
	if level.CreatedAt == 0 {
		level.CreatedAt = now
	}
	level.UpdatedAt = now
	return nil
}

func (level *MembershipLevel) NormalizeAndValidate() error {
	level.Code = strings.ToUpper(strings.TrimSpace(level.Code))
	level.DisplayName = strings.TrimSpace(level.DisplayName)
	if !isValidMembershipCode(level.Code) {
		return errors.New("membership code must be 1-64 characters using A-Z, 0-9, _ or -")
	}
	if level.DisplayName == "" || len([]rune(level.DisplayName)) > 128 {
		return errors.New("membership display name must be 1-128 characters")
	}
	if level.MultiplierPPM <= 0 || level.MultiplierPPM > MembershipMultiplierScale {
		return fmt.Errorf("membership multiplier_ppm must be between 1 and %d", MembershipMultiplierScale)
	}
	if level.Code == DefaultMembershipCode {
		level.MultiplierPPM = MembershipMultiplierScale
		level.Rank = 0
		level.Enabled = true
		level.IsDefault = true
		level.ArchivedAt = 0
	}
	return nil
}

func isValidMembershipCode(code string) bool {
	if len(code) == 0 || len(code) > 64 {
		return false
	}
	for index, char := range code {
		isLetter := char >= 'A' && char <= 'Z'
		isDigit := char >= '0' && char <= '9'
		if !isLetter && !isDigit && char != '_' && char != '-' {
			return false
		}
		if index == 0 && !isLetter && !isDigit {
			return false
		}
	}
	return true
}

// UserMembership records an independently auditable grant. Grants are never
// stacked; the resolver selects the active grant whose level has the highest
// rank. Revocation preserves history.
type UserMembership struct {
	Id                int    `json:"id"`
	UserId            int    `json:"user_id" gorm:"type:int;not null;index;index:idx_user_membership_resolution,priority:1"`
	MembershipLevelId int    `json:"membership_level_id" gorm:"type:int;not null;index"`
	StartsAt          int64  `json:"starts_at" gorm:"type:bigint;not null;index;index:idx_user_membership_resolution,priority:3"`
	EndsAt            int64  `json:"ends_at" gorm:"type:bigint;not null;default:0;index"`
	Status            string `json:"status" gorm:"type:varchar(16);not null;default:'active';index;index:idx_user_membership_resolution,priority:2"`
	Source            string `json:"source" gorm:"type:varchar(32);not null;default:'admin'"`
	Note              string `json:"note" gorm:"type:varchar(500);not null;default:''"`
	CreatedBy         int    `json:"created_by" gorm:"type:int;not null;default:0;index"`
	RevokedBy         int    `json:"revoked_by" gorm:"type:int;not null;default:0"`
	RevokedAt         int64  `json:"revoked_at" gorm:"type:bigint;not null;default:0"`
	CreatedAt         int64  `json:"created_at" gorm:"type:bigint;not null"`
	UpdatedAt         int64  `json:"updated_at" gorm:"type:bigint;not null"`

	Level *MembershipLevel `json:"level,omitempty" gorm:"foreignKey:MembershipLevelId"`
}

func (grant *UserMembership) BeforeCreate(_ *gorm.DB) error {
	if err := grant.NormalizeAndValidate(); err != nil {
		return err
	}
	now := common.GetTimestamp()
	if grant.StartsAt == 0 {
		grant.StartsAt = now
	}
	if grant.CreatedAt == 0 {
		grant.CreatedAt = now
	}
	grant.UpdatedAt = now
	return grant.ValidateTimeWindow()
}

func (grant *UserMembership) NormalizeAndValidate() error {
	grant.Status = strings.ToLower(strings.TrimSpace(grant.Status))
	if grant.Status == "" {
		grant.Status = UserMembershipStatusActive
	}
	if grant.Status != UserMembershipStatusActive && grant.Status != UserMembershipStatusRevoked {
		return errors.New("membership grant status must be active or revoked")
	}
	grant.Source = strings.ToLower(strings.TrimSpace(grant.Source))
	if grant.Source == "" {
		grant.Source = UserMembershipSourceAdmin
	}
	if len(grant.Source) > 32 {
		return errors.New("membership grant source must not exceed 32 characters")
	}
	grant.Note = strings.TrimSpace(grant.Note)
	if len([]rune(grant.Note)) > 500 {
		return errors.New("membership grant note must not exceed 500 characters")
	}
	if grant.UserId <= 0 {
		return errors.New("membership grant user_id must be positive")
	}
	if grant.MembershipLevelId <= 0 {
		return errors.New("membership grant membership_level_id must be positive")
	}
	return grant.ValidateTimeWindow()
}

func (grant *UserMembership) ValidateTimeWindow() error {
	if grant.StartsAt < 0 || grant.EndsAt < 0 {
		return errors.New("membership grant timestamps must not be negative")
	}
	if grant.EndsAt != 0 && grant.EndsAt <= grant.StartsAt {
		return errors.New("membership grant ends_at must be later than starts_at")
	}
	return nil
}

// MembershipSnapshot is safe to freeze into billing snapshots and return to
// clients. Applied pricing policy (for example the AP Seedance 480p exemption)
// is evaluated later because it depends on the resolved request parameters.
type MembershipSnapshot struct {
	GrantId        int    `json:"grant_id"`
	LevelId        int    `json:"level_id"`
	Code           string `json:"code"`
	DisplayName    string `json:"display_name"`
	MultiplierPPM  int64  `json:"multiplier_ppm"`
	Rank           int    `json:"rank"`
	StartsAt       int64  `json:"starts_at"`
	EndsAt         int64  `json:"ends_at"`
	NextChangeAt   int64  `json:"next_change_at,omitempty"`
	ResolvedAt     int64  `json:"resolved_at"`
	FallbackNormal bool   `json:"fallback_normal"`
}

func (snapshot MembershipSnapshot) Multiplier() float64 {
	ppm := snapshot.MultiplierPPM
	if ppm <= 0 || ppm > MembershipMultiplierScale {
		ppm = MembershipMultiplierScale
	}
	return float64(ppm) / float64(MembershipMultiplierScale)
}

func defaultMembershipSnapshot(now int64) MembershipSnapshot {
	return MembershipSnapshot{
		Code:           DefaultMembershipCode,
		DisplayName:    DefaultMembershipDisplayName,
		MultiplierPPM:  MembershipMultiplierScale,
		ResolvedAt:     now,
		FallbackNormal: true,
	}
}

func membershipSnapshotCacheTTL() time.Duration {
	seconds := common.GetEnvOrDefault("MEMBERSHIP_SNAPSHOT_CACHE_TTL", 120)
	if seconds <= 0 {
		seconds = 120
	}
	return time.Duration(seconds) * time.Second
}

func membershipSnapshotCacheCapacity() int {
	capacity := common.GetEnvOrDefault("MEMBERSHIP_SNAPSHOT_CACHE_CAP", 20000)
	if capacity <= 0 {
		capacity = 20000
	}
	return capacity
}

func getMembershipSnapshotCache() *cachex.HybridCache[MembershipSnapshot] {
	membershipSnapshotCacheOnce.Do(func() {
		defaultTTL := membershipSnapshotCacheTTL()
		membershipSnapshotCache = cachex.NewHybridCache[MembershipSnapshot](cachex.HybridCacheConfig[MembershipSnapshot]{
			Namespace: cachex.Namespace(membershipSnapshotCacheNamespace),
			Redis:     common.RDB,
			RedisEnabled: func() bool {
				return common.RedisEnabled && common.RDB != nil
			},
			RedisCodec: cachex.JSONCodec[MembershipSnapshot]{},
			Memory: func() *hot.HotCache[string, MembershipSnapshot] {
				return hot.NewHotCache[string, MembershipSnapshot](hot.LRU, membershipSnapshotCacheCapacity()).
					WithTTL(defaultTTL).
					WithJanitor().
					Build()
			},
		})
	})
	return membershipSnapshotCache
}

func membershipSnapshotCacheKey(userId int) string {
	if userId <= 0 {
		return ""
	}
	return strconv.Itoa(userId)
}

func InvalidateUserMembershipCache(userId int) {
	key := membershipSnapshotCacheKey(userId)
	if key == "" {
		return
	}
	_, _ = getMembershipSnapshotCache().DeleteMany([]string{key})
}

func InvalidateAllMembershipCaches() {
	_ = getMembershipSnapshotCache().Purge()
}

func EnsureDefaultMembershipLevel() error {
	if DB == nil {
		return errors.New("database is not initialized")
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		var level MembershipLevel
		err := tx.Where("code = ?", DefaultMembershipCode).First(&level).Error
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			level = MembershipLevel{
				Code:          DefaultMembershipCode,
				DisplayName:   DefaultMembershipDisplayName,
				MultiplierPPM: MembershipMultiplierScale,
				Rank:          0,
				SortOrder:     0,
				Enabled:       true,
				IsDefault:     true,
			}
			if err := tx.Create(&level).Error; err != nil {
				return err
			}
		case err != nil:
			return err
		default:
			updates := map[string]any{
				"display_name":   level.DisplayName,
				"multiplier_ppm": MembershipMultiplierScale,
				"rank":           0,
				"enabled":        true,
				"is_default":     true,
				"archived_at":    0,
				"updated_at":     common.GetTimestamp(),
			}
			if strings.TrimSpace(level.DisplayName) == "" {
				updates["display_name"] = DefaultMembershipDisplayName
			}
			if err := tx.Model(&MembershipLevel{}).Where("id = ?", level.Id).Updates(updates).Error; err != nil {
				return err
			}
		}
		return tx.Model(&MembershipLevel{}).
			Where("code <> ? AND is_default = ?", DefaultMembershipCode, true).
			Updates(map[string]any{"is_default": false, "updated_at": common.GetTimestamp()}).Error
	})
}

func ListMembershipLevels(includeArchived bool) ([]MembershipLevel, error) {
	levels := make([]MembershipLevel, 0)
	query := DB.Model(&MembershipLevel{})
	if !includeArchived {
		query = query.Where("archived_at = 0")
	}
	err := query.Order("sort_order ASC").Order("rank ASC").Order("id ASC").Find(&levels).Error
	return levels, err
}

func GetMembershipLevelById(id int) (*MembershipLevel, error) {
	if id <= 0 {
		return nil, ErrMembershipLevelNotFound
	}
	var level MembershipLevel
	if err := DB.First(&level, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrMembershipLevelNotFound
		}
		return nil, err
	}
	return &level, nil
}

func GetMembershipLevelByCode(code string) (*MembershipLevel, error) {
	code = strings.ToUpper(strings.TrimSpace(code))
	var level MembershipLevel
	if err := DB.Where("code = ?", code).First(&level).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrMembershipLevelNotFound
		}
		return nil, err
	}
	return &level, nil
}

func CreateMembershipLevel(level *MembershipLevel) error {
	if level == nil {
		return errors.New("membership level is nil")
	}
	if level.Code == DefaultMembershipCode || strings.EqualFold(strings.TrimSpace(level.Code), DefaultMembershipCode) {
		return errors.New("NORMAL membership level is managed by the system")
	}
	if err := level.NormalizeAndValidate(); err != nil {
		return err
	}
	level.IsDefault = false
	level.ArchivedAt = 0
	if err := DB.Select(
		"code", "display_name", "multiplier_ppm", "rank", "sort_order", "enabled",
		"is_default", "archived_at", "created_at", "updated_at",
	).Create(level).Error; err != nil {
		return err
	}
	InvalidateAllMembershipCaches()
	return nil
}

// UpdateMembershipLevel updates the mutable commercial fields. NORMAL is kept
// immutable except for its display and sort order so the fallback can never be
// discounted or disabled.
func UpdateMembershipLevel(level *MembershipLevel) error {
	if level == nil || level.Id <= 0 {
		return ErrMembershipLevelNotFound
	}
	existing, err := GetMembershipLevelById(level.Id)
	if err != nil {
		return err
	}
	level.Code = existing.Code
	level.IsDefault = existing.IsDefault
	level.ArchivedAt = existing.ArchivedAt
	if existing.Code == DefaultMembershipCode {
		level.MultiplierPPM = MembershipMultiplierScale
		level.Rank = 0
		level.Enabled = true
		level.IsDefault = true
		level.ArchivedAt = 0
	}
	if err := level.NormalizeAndValidate(); err != nil {
		return err
	}
	updates := map[string]any{
		"display_name":   level.DisplayName,
		"multiplier_ppm": level.MultiplierPPM,
		"rank":           level.Rank,
		"sort_order":     level.SortOrder,
		"enabled":        level.Enabled,
		"updated_at":     common.GetTimestamp(),
	}
	if err := DB.Model(&MembershipLevel{}).Where("id = ?", level.Id).Updates(updates).Error; err != nil {
		return err
	}
	InvalidateAllMembershipCaches()
	return nil
}

func ArchiveMembershipLevel(id int) error {
	level, err := GetMembershipLevelById(id)
	if err != nil {
		return err
	}
	if level.Code == DefaultMembershipCode {
		return errors.New("NORMAL membership level cannot be archived")
	}
	now := common.GetTimestamp()
	if err := DB.Model(&MembershipLevel{}).Where("id = ?", id).
		Updates(map[string]any{"enabled": false, "archived_at": now, "updated_at": now}).Error; err != nil {
		return err
	}
	InvalidateAllMembershipCaches()
	return nil
}

func CreateUserMembership(grant *UserMembership) error {
	if grant == nil {
		return errors.New("membership grant is nil")
	}
	if err := grant.NormalizeAndValidate(); err != nil {
		return err
	}
	var userCount int64
	if err := DB.Model(&User{}).Where("id = ?", grant.UserId).Count(&userCount).Error; err != nil {
		return err
	}
	if userCount == 0 {
		return errors.New("user not found")
	}
	level, err := GetMembershipLevelById(grant.MembershipLevelId)
	if err != nil {
		return err
	}
	if !level.Enabled || level.ArchivedAt != 0 || level.Code == DefaultMembershipCode {
		return errors.New("membership level is not grantable")
	}
	if err := DB.Create(grant).Error; err != nil {
		return err
	}
	InvalidateUserMembershipCache(grant.UserId)
	return nil
}

func RevokeUserMembership(id int, revokedBy int) error {
	if id <= 0 {
		return ErrUserMembershipNotFound
	}
	var grant UserMembership
	if err := DB.First(&grant, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrUserMembershipNotFound
		}
		return err
	}
	if grant.Status == UserMembershipStatusRevoked {
		InvalidateUserMembershipCache(grant.UserId)
		return nil
	}
	now := common.GetTimestamp()
	if err := DB.Model(&UserMembership{}).Where("id = ?", id).Updates(map[string]any{
		"status":     UserMembershipStatusRevoked,
		"revoked_by": revokedBy,
		"revoked_at": now,
		"updated_at": now,
	}).Error; err != nil {
		return err
	}
	InvalidateUserMembershipCache(grant.UserId)
	return nil
}

func ListUserMemberships(userId int) ([]UserMembership, error) {
	grants := make([]UserMembership, 0)
	err := DB.Preload("Level").Where("user_id = ?", userId).
		Order("created_at DESC").Order("id DESC").Find(&grants).Error
	return grants, err
}

func GetUserMembershipById(id int) (*UserMembership, error) {
	if id <= 0 {
		return nil, ErrUserMembershipNotFound
	}
	var grant UserMembership
	if err := DB.Preload("Level").First(&grant, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrUserMembershipNotFound
		}
		return nil, err
	}
	return &grant, nil
}

func ResolveUserMembership(userId int) (MembershipSnapshot, error) {
	return ResolveUserMembershipAt(userId, common.GetTimestamp(), true)
}

// ResolveUserMembershipAt resolves an effective level at an explicit instant.
// Cache is used only for real-time resolution; historical/frozen calculations
// always query the database for deterministic behavior.
func ResolveUserMembershipAt(userId int, now int64, useCache bool) (MembershipSnapshot, error) {
	if now <= 0 {
		now = common.GetTimestamp()
	}
	if userId <= 0 {
		return defaultMembershipSnapshot(now), nil
	}
	cacheKey := membershipSnapshotCacheKey(userId)
	if useCache {
		if cached, found, err := getMembershipSnapshotCache().Get(cacheKey); err == nil && found {
			if cached.NextChangeAt == 0 || cached.NextChangeAt > now {
				return cached, nil
			}
		}
	}

	type membershipResolutionRow struct {
		GrantId       int
		LevelId       int
		Code          string
		DisplayName   string
		MultiplierPPM int64
		Rank          int
		StartsAt      int64
		EndsAt        int64
		CreatedAt     int64
	}
	var row membershipResolutionRow
	err := DB.Table("user_memberships AS um").
		Select("um.id AS grant_id, ml.id AS level_id, ml.code, ml.display_name, ml.multiplier_ppm, ml.rank, um.starts_at, um.ends_at, um.created_at").
		Joins("JOIN membership_levels AS ml ON ml.id = um.membership_level_id").
		Where("um.user_id = ? AND um.status = ?", userId, UserMembershipStatusActive).
		Where("um.starts_at <= ? AND (um.ends_at = 0 OR um.ends_at > ?)", now, now).
		Where("ml.enabled = ? AND ml.archived_at = 0", true).
		Order("ml.rank DESC").Order("um.created_at DESC").Order("um.id DESC").
		Limit(1).Scan(&row).Error
	if err != nil {
		return MembershipSnapshot{}, err
	}

	nextChangeAt, err := getNextMembershipChangeAt(userId, now)
	if err != nil {
		return MembershipSnapshot{}, err
	}

	snapshot := defaultMembershipSnapshot(now)
	if row.GrantId != 0 {
		snapshot = MembershipSnapshot{
			GrantId:       row.GrantId,
			LevelId:       row.LevelId,
			Code:          row.Code,
			DisplayName:   row.DisplayName,
			MultiplierPPM: row.MultiplierPPM,
			Rank:          row.Rank,
			StartsAt:      row.StartsAt,
			EndsAt:        row.EndsAt,
			ResolvedAt:    now,
		}
	}
	snapshot.NextChangeAt = nextChangeAt
	if useCache {
		ttl := membershipSnapshotCacheTTL()
		if nextChangeAt > now {
			untilChange := time.Duration(nextChangeAt-now) * time.Second
			if untilChange < ttl {
				ttl = untilChange
			}
		}
		if ttl <= 0 {
			ttl = time.Second
		}
		_ = getMembershipSnapshotCache().SetWithTTL(cacheKey, snapshot, ttl)
	}
	return snapshot, nil
}

func getNextMembershipChangeAt(userId int, now int64) (int64, error) {
	var futureStart int64
	err := DB.Table("user_memberships AS um").
		Joins("JOIN membership_levels AS ml ON ml.id = um.membership_level_id").
		Where("um.user_id = ? AND um.status = ? AND um.starts_at > ?", userId, UserMembershipStatusActive, now).
		Where("ml.enabled = ? AND ml.archived_at = 0", true).
		Select("COALESCE(MIN(um.starts_at), 0)").Scan(&futureStart).Error
	if err != nil {
		return 0, err
	}

	var activeEnd int64
	err = DB.Table("user_memberships AS um").
		Joins("JOIN membership_levels AS ml ON ml.id = um.membership_level_id").
		Where("um.user_id = ? AND um.status = ? AND um.starts_at <= ? AND um.ends_at > ?", userId, UserMembershipStatusActive, now, now).
		Where("ml.enabled = ? AND ml.archived_at = 0", true).
		Select("COALESCE(MIN(um.ends_at), 0)").Scan(&activeEnd).Error
	if err != nil {
		return 0, err
	}
	if futureStart == 0 {
		return activeEnd, nil
	}
	if activeEnd == 0 || futureStart < activeEnd {
		return futureStart, nil
	}
	return activeEnd, nil
}
