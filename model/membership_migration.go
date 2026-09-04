package model

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var legacyVIPGroupRank = map[string]int{
	"VIP-T1": 60,
	"VIP1":   50,
	"VIP2":   40,
	"VIP3":   30,
	"VIP4":   20,
	"VIP5":   10,
}

type LegacyVIPGroupPreflight struct {
	Group                      string  `json:"group"`
	GroupRatio                 float64 `json:"group_ratio"`
	ProposedMultiplierPPM      int64   `json:"proposed_multiplier_ppm"`
	Users                      int64   `json:"users"`
	Tokens                     int64   `json:"tokens"`
	Abilities                  int64   `json:"abilities"`
	Channels                   int64   `json:"channels"`
	SubscriptionPlans          int64   `json:"subscription_plans"`
	UserSubscriptions          int64   `json:"user_subscriptions"`
	ExistingMembershipLevelId  int     `json:"existing_membership_level_id,omitempty"`
	ExistingMembershipLevelPPM int64   `json:"existing_membership_level_ppm,omitempty"`
	MembershipLevelConflict    bool    `json:"membership_level_conflict"`
}

type LegacyVIPMigrationPreflight struct {
	Groups                 []LegacyVIPGroupPreflight `json:"groups"`
	TotalUsers             int64                     `json:"total_users"`
	TotalTokens            int64                     `json:"total_tokens"`
	TotalAbilities         int64                     `json:"total_abilities"`
	TotalChannels          int64                     `json:"total_channels"`
	TotalSubscriptionPlans int64                     `json:"total_subscription_plans"`
	TotalUserSubscriptions int64                     `json:"total_user_subscriptions"`
	ConflictingLevelCodes  []string                  `json:"conflicting_level_codes"`
	Ready                  bool                      `json:"ready"`
}

type LegacyVIPMigrationResult struct {
	LegacyVIPMigrationPreflight
	CreatedLevels      int   `json:"created_levels"`
	CreatedGrants      int   `json:"created_grants"`
	MigratedUsers      int64 `json:"migrated_users"`
	MigratedTokens     int64 `json:"migrated_tokens"`
	UpdatedChannels    int   `json:"updated_channels"`
	DeletedAbilities   int64 `json:"deleted_abilities"`
	ClearedPlanRefs    int64 `json:"cleared_plan_refs"`
	ClearedUserSubRefs int64 `json:"cleared_user_subscription_refs"`
}

func LegacyVIPGroupCodes() []string {
	codes := make([]string, 0, len(legacyVIPGroupRank))
	for code := range legacyVIPGroupRank {
		codes = append(codes, code)
	}
	sort.Slice(codes, func(left, right int) bool {
		return legacyVIPGroupRank[codes[left]] > legacyVIPGroupRank[codes[right]]
	})
	return codes
}

func legacyVIPMultiplierPPM(code string, ratios map[string]float64) int64 {
	ratio, found := legacyVIPRatio(code, ratios)
	if !found {
		ratio = 1
	}
	ppm := int64(math.Round(ratio * float64(MembershipMultiplierScale)))
	if ppm <= 0 || ppm > MembershipMultiplierScale {
		return MembershipMultiplierScale
	}
	return ppm
}

func legacyVIPRatio(code string, ratios map[string]float64) (float64, bool) {
	for group, configured := range ratios {
		if strings.EqualFold(strings.TrimSpace(group), code) {
			return configured, true
		}
	}
	return 0, false
}

func PreflightLegacyVIPGroupMigration() (*LegacyVIPMigrationPreflight, error) {
	if DB == nil {
		return nil, errors.New("database is not initialized")
	}
	ratios := ratio_setting.GetGroupRatioCopy()
	channels := make([]Channel, 0)
	if err := DB.Select("id", "group").Find(&channels).Error; err != nil {
		return nil, err
	}
	result := &LegacyVIPMigrationPreflight{Groups: make([]LegacyVIPGroupPreflight, 0, len(legacyVIPGroupRank))}
	groupColumn := membershipMigrationGroupColumn()
	for _, code := range LegacyVIPGroupCodes() {
		item := LegacyVIPGroupPreflight{
			Group:                 code,
			GroupRatio:            float64(legacyVIPMultiplierPPM(code, ratios)) / float64(MembershipMultiplierScale),
			ProposedMultiplierPPM: legacyVIPMultiplierPPM(code, ratios),
		}
		upperCode := strings.ToUpper(code)
		if err := DB.Model(&User{}).Where("UPPER("+groupColumn+") = ?", upperCode).Count(&item.Users).Error; err != nil {
			return nil, err
		}
		if err := DB.Model(&Token{}).Where("UPPER("+groupColumn+") = ?", upperCode).Count(&item.Tokens).Error; err != nil {
			return nil, err
		}
		if err := DB.Model(&Ability{}).Where("UPPER("+groupColumn+") = ?", upperCode).Count(&item.Abilities).Error; err != nil {
			return nil, err
		}
		for index := range channels {
			if channelHasLegacyVIPGroup(channels[index].Group, code) {
				item.Channels++
			}
		}
		if err := DB.Model(&SubscriptionPlan{}).Where("UPPER(upgrade_group) = ?", upperCode).Count(&item.SubscriptionPlans).Error; err != nil {
			return nil, err
		}
		if err := DB.Model(&UserSubscription{}).Where("UPPER(upgrade_group) = ?", upperCode).Count(&item.UserSubscriptions).Error; err != nil {
			return nil, err
		}
		var existing MembershipLevel
		err := DB.Where("code = ?", code).First(&existing).Error
		if err == nil {
			item.ExistingMembershipLevelId = existing.Id
			item.ExistingMembershipLevelPPM = existing.MultiplierPPM
			if _, configured := legacyVIPRatio(code, ratios); !configured {
				item.ProposedMultiplierPPM = existing.MultiplierPPM
				item.GroupRatio = float64(existing.MultiplierPPM) / float64(MembershipMultiplierScale)
			}
			item.MembershipLevelConflict = existing.MultiplierPPM != item.ProposedMultiplierPPM
			if item.MembershipLevelConflict {
				result.ConflictingLevelCodes = append(result.ConflictingLevelCodes, code)
			}
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
		result.TotalUsers += item.Users
		result.TotalTokens += item.Tokens
		result.TotalAbilities += item.Abilities
		result.TotalChannels += item.Channels
		result.TotalSubscriptionPlans += item.SubscriptionPlans
		result.TotalUserSubscriptions += item.UserSubscriptions
		result.Groups = append(result.Groups, item)
	}
	result.Ready = len(result.ConflictingLevelCodes) == 0
	return result, nil
}

func membershipMigrationGroupColumn() string {
	if commonGroupCol != "" {
		return commonGroupCol
	}
	if common.UsingPostgreSQL {
		return `"group"`
	}
	return "`group`"
}

func channelHasLegacyVIPGroup(channelGroups string, wanted string) bool {
	for _, group := range strings.Split(channelGroups, ",") {
		if strings.EqualFold(strings.TrimSpace(group), wanted) {
			return true
		}
	}
	return false
}

func filterLegacyVIPGroups(channelGroups string) string {
	seen := make(map[string]struct{})
	remaining := make([]string, 0)
	for _, rawGroup := range strings.Split(channelGroups, ",") {
		group := strings.TrimSpace(rawGroup)
		if group == "" || isLegacyVIPGroup(group) {
			continue
		}
		key := strings.ToLower(group)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		remaining = append(remaining, group)
	}
	if len(remaining) == 0 {
		return "default"
	}
	return strings.Join(remaining, ",")
}

func isLegacyVIPGroup(group string) bool {
	_, exists := legacyVIPGroupRank[strings.ToUpper(strings.TrimSpace(group))]
	return exists
}

func withoutLegacyVIPNumberMap(input map[string]float64) map[string]float64 {
	output := make(map[string]float64, len(input))
	for key, value := range input {
		if !isLegacyVIPGroup(key) {
			output[key] = value
		}
	}
	if _, ok := output["default"]; !ok {
		output["default"] = 1
	}
	return output
}

func withoutLegacyVIPStringMap(input map[string]string) map[string]string {
	output := make(map[string]string, len(input))
	for key, value := range input {
		if !isLegacyVIPGroup(key) {
			output[key] = value
		}
	}
	if _, ok := output["default"]; !ok {
		output["default"] = "默认分组"
	}
	return output
}

func buildLegacyVIPOptionUpdates() (map[string]string, error) {
	groupRatios := withoutLegacyVIPNumberMap(ratio_setting.GetGroupRatioCopy())
	groupGroupRatios := ratio_setting.GetGroupRatioSetting().GroupGroupRatio.ReadAll()
	cleanGroupGroupRatios := make(map[string]map[string]float64, len(groupGroupRatios))
	for source, targets := range groupGroupRatios {
		if isLegacyVIPGroup(source) {
			continue
		}
		cleanTargets := make(map[string]float64, len(targets))
		for target, value := range targets {
			if !isLegacyVIPGroup(target) {
				cleanTargets[target] = value
			}
		}
		cleanGroupGroupRatios[source] = cleanTargets
	}
	usableGroups := withoutLegacyVIPStringMap(setting.GetUserUsableGroupsCopy())
	specialGroups := ratio_setting.GetGroupRatioSetting().GroupSpecialUsableGroup.ReadAll()
	cleanSpecialGroups := make(map[string]map[string]string, len(specialGroups))
	for source, targets := range specialGroups {
		if isLegacyVIPGroup(source) {
			continue
		}
		cleanTargets := make(map[string]string, len(targets))
		for rawTarget, description := range targets {
			target := strings.TrimPrefix(strings.TrimPrefix(rawTarget, "-:"), "+:")
			if !isLegacyVIPGroup(target) {
				cleanTargets[rawTarget] = description
			}
		}
		cleanSpecialGroups[source] = cleanTargets
	}

	updates := map[string]any{
		"GroupRatio":       groupRatios,
		"GroupGroupRatio":  cleanGroupGroupRatios,
		"UserUsableGroups": usableGroups,
		"group_ratio_setting.group_special_usable_group": cleanSpecialGroups,
	}
	encoded := make(map[string]string, len(updates))
	for key, value := range updates {
		data, err := common.Marshal(value)
		if err != nil {
			return nil, fmt.Errorf("encode %s: %w", key, err)
		}
		encoded[key] = string(data)
	}
	return encoded, nil
}

func ApplyLegacyVIPGroupMigration() (*LegacyVIPMigrationResult, error) {
	preflight, err := PreflightLegacyVIPGroupMigration()
	if err != nil {
		return nil, err
	}
	if !preflight.Ready {
		return nil, fmt.Errorf("membership level conflicts must be resolved first: %s", strings.Join(preflight.ConflictingLevelCodes, ", "))
	}
	optionUpdates, err := buildLegacyVIPOptionUpdates()
	if err != nil {
		return nil, err
	}
	result := &LegacyVIPMigrationResult{LegacyVIPMigrationPreflight: *preflight}
	affectedUserIDs := make([]int, 0, preflight.TotalUsers)

	err = DB.Transaction(func(tx *gorm.DB) error {
		now := common.GetTimestamp()
		groupColumn := membershipMigrationGroupColumn()
		levels := make(map[string]MembershipLevel, len(legacyVIPGroupRank))
		for _, item := range preflight.Groups {
			var level MembershipLevel
			findErr := tx.Where("code = ?", item.Group).First(&level).Error
			if errors.Is(findErr, gorm.ErrRecordNotFound) {
				level = MembershipLevel{
					Code:          item.Group,
					DisplayName:   item.Group,
					MultiplierPPM: item.ProposedMultiplierPPM,
					Rank:          legacyVIPGroupRank[item.Group],
					SortOrder:     legacyVIPGroupRank[item.Group],
					Enabled:       true,
				}
				if err := tx.Create(&level).Error; err != nil {
					return err
				}
				result.CreatedLevels++
			} else if findErr != nil {
				return findErr
			}
			levels[item.Group] = level
		}

		var users []User
		if err := tx.Where("UPPER("+groupColumn+") IN ?", LegacyVIPGroupCodes()).Find(&users).Error; err != nil {
			return err
		}
		for index := range users {
			user := &users[index]
			level := levels[strings.ToUpper(strings.TrimSpace(user.Group))]
			grant := UserMembership{
				UserId: user.Id, MembershipLevelId: level.Id, StartsAt: now,
				Status: UserMembershipStatusActive, Source: UserMembershipSourceMigration,
				Note: "由旧 VIP 分组迁移", CreatedBy: 0,
			}
			if err := tx.Create(&grant).Error; err != nil {
				return err
			}
			result.CreatedGrants++
			affectedUserIDs = append(affectedUserIDs, user.Id)
		}
		userUpdate := tx.Model(&User{}).Where("UPPER("+groupColumn+") IN ?", LegacyVIPGroupCodes()).Update("group", "default")
		if userUpdate.Error != nil {
			return userUpdate.Error
		}
		result.MigratedUsers = userUpdate.RowsAffected
		tokenUpdate := tx.Model(&Token{}).Where("UPPER("+groupColumn+") IN ?", LegacyVIPGroupCodes()).Update("group", "default")
		if tokenUpdate.Error != nil {
			return tokenUpdate.Error
		}
		result.MigratedTokens = tokenUpdate.RowsAffected

		var channels []Channel
		if err := tx.Find(&channels).Error; err != nil {
			return err
		}
		for index := range channels {
			channel := &channels[index]
			cleanGroups := filterLegacyVIPGroups(channel.Group)
			if cleanGroups == channel.Group {
				continue
			}
			channel.Group = cleanGroups
			if err := tx.Model(&Channel{}).Where("id = ?", channel.Id).Update("group", cleanGroups).Error; err != nil {
				return err
			}
			if err := channel.UpdateAbilities(tx); err != nil {
				return err
			}
			result.UpdatedChannels++
		}
		deleteAbilities := tx.Where("UPPER("+groupColumn+") IN ?", LegacyVIPGroupCodes()).Delete(&Ability{})
		if deleteAbilities.Error != nil {
			return deleteAbilities.Error
		}
		result.DeletedAbilities = deleteAbilities.RowsAffected
		planUpdate := tx.Model(&SubscriptionPlan{}).Where("UPPER(upgrade_group) IN ?", LegacyVIPGroupCodes()).Update("upgrade_group", "")
		if planUpdate.Error != nil {
			return planUpdate.Error
		}
		result.ClearedPlanRefs = planUpdate.RowsAffected
		subUpdate := tx.Model(&UserSubscription{}).Where("UPPER(upgrade_group) IN ?", LegacyVIPGroupCodes()).Update("upgrade_group", "")
		if subUpdate.Error != nil {
			return subUpdate.Error
		}
		result.ClearedUserSubRefs = subUpdate.RowsAffected

		for key, value := range optionUpdates {
			option := Option{Key: key, Value: value}
			if err := tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "key"}},
				DoUpdates: clause.AssignmentColumns([]string{"value"}),
			}).Create(&option).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	for key, value := range optionUpdates {
		if err := updateOptionMap(key, value); err != nil {
			return nil, fmt.Errorf("refresh migrated option %s: %w", key, err)
		}
	}
	for _, userId := range affectedUserIDs {
		_ = InvalidateUserCache(userId)
		InvalidateUserMembershipCache(userId)
	}
	InvalidateAllMembershipCaches()
	InitChannelCache()
	return result, nil
}
