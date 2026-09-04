package model

import (
	"fmt"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestLegacyVIPGroupMigrationPreflightsThenMigrates(t *testing.T) {
	oldDB := DB
	oldUsingSQLite := common.UsingSQLite
	oldUsingPostgres := common.UsingPostgreSQL
	oldCommonGroupCol := commonGroupCol
	oldRatios := ratio_setting.GroupRatio2JSONString()
	oldGroupGroupRatios := ratio_setting.GroupGroupRatio2JSONString()
	oldUsableGroups := setting.UserUsableGroups2JSONString()
	oldSpecialGroups, err := common.Marshal(ratio_setting.GetGroupRatioSetting().GroupSpecialUsableGroup.ReadAll())
	require.NoError(t, err)

	dsn := fmt.Sprintf("file:membership_migration_%d?mode=memory&cache=shared", time.Now().UnixNano())
	testDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	DB = testDB
	common.UsingSQLite = true
	common.UsingPostgreSQL = false
	initCol()
	require.NoError(t, DB.AutoMigrate(
		&User{}, &Token{}, &Channel{}, &Ability{}, &Option{},
		&SubscriptionPlan{}, &UserSubscription{}, &MembershipLevel{}, &UserMembership{},
	))
	require.NoError(t, EnsureDefaultMembershipLevel())
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"VIP1":0.8}`))
	require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(`{"default":{"VIP1":0.75}}`))
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"默认分组","VIP1":"旧 VIP1"}`))
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"group_ratio_setting.group_special_usable_group": `{"default":{"VIP1":"旧 VIP1"}}`,
	}))

	user := User{Username: "legacy-vip-user", Password: "membership-test-password", Group: "VIP1"}
	require.NoError(t, DB.Create(&user).Error)
	token := Token{UserId: user.Id, Name: "legacy-vip-token", Key: "legacy-vip-key", Group: "VIP1", RemainQuota: 1000}
	require.NoError(t, DB.Create(&token).Error)
	channel := Channel{Name: "legacy-vip-channel", Group: "default,VIP1", Models: "legacy-model", Status: common.ChannelStatusEnabled}
	require.NoError(t, DB.Create(&channel).Error)
	require.NoError(t, channel.AddAbilities(nil))
	plan := SubscriptionPlan{Title: "legacy", PriceAmount: 1, UpgradeGroup: "VIP1", Enabled: true}
	require.NoError(t, DB.Create(&plan).Error)
	subscription := UserSubscription{UserId: user.Id, PlanId: plan.Id, UpgradeGroup: "VIP1", Status: "active"}
	require.NoError(t, DB.Create(&subscription).Error)

	t.Cleanup(func() {
		InvalidateAllMembershipCaches()
		_ = ratio_setting.UpdateGroupRatioByJSONString(oldRatios)
		_ = ratio_setting.UpdateGroupGroupRatioByJSONString(oldGroupGroupRatios)
		_ = setting.UpdateUserUsableGroupsByJSONString(oldUsableGroups)
		_ = config.GlobalConfig.LoadFromDB(map[string]string{
			"group_ratio_setting.group_special_usable_group": string(oldSpecialGroups),
		})
		sqlDB, sqlErr := testDB.DB()
		if sqlErr == nil {
			_ = sqlDB.Close()
		}
		DB = oldDB
		common.UsingSQLite = oldUsingSQLite
		common.UsingPostgreSQL = oldUsingPostgres
		commonGroupCol = oldCommonGroupCol
	})

	preflight, err := PreflightLegacyVIPGroupMigration()
	require.NoError(t, err)
	require.True(t, preflight.Ready)
	require.EqualValues(t, 1, preflight.TotalUsers)
	require.EqualValues(t, 1, preflight.TotalTokens)
	require.EqualValues(t, 1, preflight.TotalChannels)
	require.EqualValues(t, 1, preflight.TotalSubscriptionPlans)
	require.EqualValues(t, 1, preflight.TotalUserSubscriptions)

	result, err := ApplyLegacyVIPGroupMigration()
	require.NoError(t, err)
	require.EqualValues(t, 1, result.MigratedUsers)
	require.EqualValues(t, 1, result.MigratedTokens)
	require.Equal(t, 1, result.CreatedGrants)

	require.NoError(t, DB.First(&user, user.Id).Error)
	require.Equal(t, "default", user.Group)
	require.NoError(t, DB.First(&token, token.Id).Error)
	require.Equal(t, "default", token.Group)
	require.NoError(t, DB.First(&channel, channel.Id).Error)
	require.Equal(t, "default", channel.Group)
	require.NoError(t, DB.First(&plan, plan.Id).Error)
	require.Empty(t, plan.UpgradeGroup)
	require.NoError(t, DB.First(&subscription, subscription.Id).Error)
	require.Empty(t, subscription.UpgradeGroup)

	snapshot, err := ResolveUserMembershipAt(user.Id, common.GetTimestamp(), false)
	require.NoError(t, err)
	require.Equal(t, "VIP1", snapshot.Code)
	require.Equal(t, int64(800_000), snapshot.MultiplierPPM)

	require.NotContains(t, ratio_setting.GetGroupRatioCopy(), "VIP1")
	require.NotContains(t, setting.GetUserUsableGroupsCopy(), "VIP1")
	var legacyAbilityCount int64
	require.NoError(t, DB.Model(&Ability{}).Where("UPPER("+membershipMigrationGroupColumn()+") = ?", "VIP1").Count(&legacyAbilityCount).Error)
	require.Zero(t, legacyAbilityCount)

	second, err := ApplyLegacyVIPGroupMigration()
	require.NoError(t, err)
	require.Zero(t, second.CreatedGrants)
	require.Zero(t, second.MigratedUsers)
}
