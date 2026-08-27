package model

import (
	"fmt"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupUserAffiliateTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	previousDB := DB
	previousLogDB := LOG_DB
	previousRedisEnabled := common.RedisEnabled
	previousQuotaForNewUser := common.QuotaForNewUser
	previousQuotaForInvitee := common.QuotaForInvitee
	previousQuotaForInviter := common.QuotaForInviter

	dsn := fmt.Sprintf("file:user_affiliate_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&User{}, &Log{}))

	DB = db
	LOG_DB = db
	common.RedisEnabled = false
	common.QuotaForNewUser = 0
	common.QuotaForInvitee = 0
	common.QuotaForInviter = 0

	t.Cleanup(func() {
		DB = previousDB
		LOG_DB = previousLogDB
		common.RedisEnabled = previousRedisEnabled
		common.QuotaForNewUser = previousQuotaForNewUser
		common.QuotaForInvitee = previousQuotaForInvitee
		common.QuotaForInviter = previousQuotaForInviter
	})

	return db
}

func createAffiliateTestInviter(t *testing.T, db *gorm.DB, username string) User {
	t.Helper()

	inviter := User{
		Username: username,
		Password: "unused",
		AffCode:  username + "-code",
		Role:     common.RoleCommonUser,
		Status:   common.UserStatusEnabled,
	}
	require.NoError(t, db.Create(&inviter).Error)
	return inviter
}

func assertInvitationRecorded(t *testing.T, db *gorm.DB, inviterId int, inviteeId int) {
	t.Helper()

	var inviter User
	require.NoError(t, db.First(&inviter, inviterId).Error)
	require.Equal(t, 1, inviter.AffCount)
	require.Zero(t, inviter.AffQuota)
	require.Zero(t, inviter.AffHistoryQuota)

	var invitee User
	require.NoError(t, db.First(&invitee, inviteeId).Error)
	require.Equal(t, inviterId, invitee.InviterId)
}

func TestInsertRecordsInvitationWhenInviterRewardDisabled(t *testing.T) {
	db := setupUserAffiliateTestDB(t)
	inviter := createAffiliateTestInviter(t, db, "password-inviter")

	invitee := User{
		Username: "password-invitee",
		Password: "password123",
		Role:     common.RoleCommonUser,
		Status:   common.UserStatusEnabled,
	}
	require.NoError(t, invitee.Insert(inviter.Id))

	assertInvitationRecorded(t, db, inviter.Id, invitee.Id)
}

func TestOAuthInsertPersistsAndCountsInvitationWhenRewardDisabled(t *testing.T) {
	db := setupUserAffiliateTestDB(t)
	inviter := createAffiliateTestInviter(t, db, "oauth-inviter")

	invitee := User{
		Username: "oauth-invitee",
		Role:     common.RoleCommonUser,
		Status:   common.UserStatusEnabled,
	}
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return invitee.InsertWithTx(tx, inviter.Id)
	}))
	invitee.FinalizeOAuthUserCreation(inviter.Id)

	assertInvitationRecorded(t, db, inviter.Id, invitee.Id)
}

func TestInsertAlwaysRecordsUserCreatedLogWhenGiftQuotaDisabled(t *testing.T) {
	db := setupUserAffiliateTestDB(t)

	user := User{
		Username: "User_082602",
		Password: "password123",
		Role:     common.RoleCommonUser,
		Status:   common.UserStatusEnabled,
	}
	require.NoError(t, user.Insert(0))

	var logs []Log
	require.NoError(t, db.Where("user_id = ?", user.Id).Order("id asc").Find(&logs).Error)
	require.Len(t, logs, 1)
	require.Equal(t, LogTypeSystem, logs[0].Type)
	require.Equal(t, "User_082602", logs[0].Username)
	require.Equal(t, "新用户创建", logs[0].Content)
	require.NotZero(t, logs[0].CreatedAt)
}

func TestOAuthInsertAlwaysRecordsUserCreatedLogWhenGiftQuotaDisabled(t *testing.T) {
	db := setupUserAffiliateTestDB(t)

	user := User{
		Username: "User_082603",
		Role:     common.RoleCommonUser,
		Status:   common.UserStatusEnabled,
	}
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return user.InsertWithTx(tx, 0)
	}))
	user.FinalizeOAuthUserCreation(0)

	var logs []Log
	require.NoError(t, db.Where("user_id = ?", user.Id).Order("id asc").Find(&logs).Error)
	require.Len(t, logs, 1)
	require.Equal(t, LogTypeSystem, logs[0].Type)
	require.Equal(t, "User_082603", logs[0].Username)
	require.Equal(t, "新用户创建", logs[0].Content)
}

func TestInsertRecordsGiftLogTogetherWithCreatedLog(t *testing.T) {
	db := setupUserAffiliateTestDB(t)
	common.QuotaForNewUser = 1000

	user := User{
		Username: "gifted-user",
		Password: "password123",
		Role:     common.RoleCommonUser,
		Status:   common.UserStatusEnabled,
	}
	require.NoError(t, user.Insert(0))

	var logs []Log
	require.NoError(t, db.Where("user_id = ?", user.Id).Order("id asc").Find(&logs).Error)
	require.Len(t, logs, 2)
	require.Equal(t, "新用户创建", logs[0].Content)
	require.Contains(t, logs[1].Content, "新用户注册赠送")
}

func TestInsertPreservesConfiguredInviterReward(t *testing.T) {
	db := setupUserAffiliateTestDB(t)
	common.QuotaForInviter = 250
	inviter := createAffiliateTestInviter(t, db, "reward-inviter")

	invitee := User{
		Username: "reward-invitee",
		Password: "password123",
		Role:     common.RoleCommonUser,
		Status:   common.UserStatusEnabled,
	}
	require.NoError(t, invitee.Insert(inviter.Id))

	var updatedInviter User
	require.NoError(t, db.First(&updatedInviter, inviter.Id).Error)
	require.Equal(t, 1, updatedInviter.AffCount)
	require.Equal(t, 250, updatedInviter.AffQuota)
	require.Equal(t, 250, updatedInviter.AffHistoryQuota)
}
