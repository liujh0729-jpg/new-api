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

func setupLogUserFilterTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	previousDB := DB
	previousLogDB := LOG_DB
	previousRedisEnabled := common.RedisEnabled

	dsn := fmt.Sprintf("file:log_user_filter_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&User{}, &Log{}))

	DB = db
	LOG_DB = db
	common.RedisEnabled = false

	t.Cleanup(func() {
		DB = previousDB
		LOG_DB = previousLogDB
		common.RedisEnabled = previousRedisEnabled
	})

	return db
}

func TestGetAllLogsFiltersByUsernameOrUserID(t *testing.T) {
	db := setupLogUserFilterTestDB(t)

	user := User{
		Username:    "User_082602",
		DisplayName: "测试用户",
		Password:    "unused",
		AffCode:     "aff-082602",
		Role:        common.RoleCommonUser,
		Status:      common.UserStatusEnabled,
	}
	require.NoError(t, db.Create(&user).Error)
	other := User{
		Username: "other-user",
		Password: "unused",
		AffCode:  "aff-other",
		Role:     common.RoleCommonUser,
		Status:   common.UserStatusEnabled,
	}
	require.NoError(t, db.Create(&other).Error)

	require.NoError(t, db.Create(&Log{
		UserId:    user.Id,
		Username:  user.Username,
		Type:      LogTypeSystem,
		Content:   "新用户创建",
		CreatedAt: common.GetTimestamp(),
	}).Error)
	require.NoError(t, db.Create(&Log{
		UserId:    other.Id,
		Username:  other.Username,
		Type:      LogTypeSystem,
		Content:   "其他用户",
		CreatedAt: common.GetTimestamp(),
	}).Error)

	byName, total, err := GetAllLogs(LogTypeUnknown, 0, 0, "", "User_082602", "", 0, 20, 0, "", "")
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, byName, 1)
	require.Equal(t, user.Id, byName[0].UserId)

	byID, total, err := GetAllLogs(LogTypeUnknown, 0, 0, "", fmt.Sprintf("%d", user.Id), "", 0, 20, 0, "", "")
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, byID, 1)
	require.Equal(t, "User_082602", byID[0].Username)

	byDisplayName, total, err := GetAllLogs(LogTypeUnknown, 0, 0, "", "测试用户", "", 0, 20, 0, "", "")
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, byDisplayName, 1)
	require.Equal(t, user.Id, byDisplayName[0].UserId)
}

func TestGetAllLogsFindsHistoricalLogWithEmptyUsernameByUserID(t *testing.T) {
	db := setupLogUserFilterTestDB(t)

	user := User{
		Username: "User_082603",
		Password: "unused",
		AffCode:  "aff-082603",
		Role:     common.RoleCommonUser,
		Status:   common.UserStatusEnabled,
	}
	require.NoError(t, db.Create(&user).Error)
	require.NoError(t, db.Create(&Log{
		UserId:    user.Id,
		Username:  "",
		Type:      LogTypeSystem,
		Content:   "历史空用户名日志",
		CreatedAt: common.GetTimestamp(),
	}).Error)

	logs, total, err := GetAllLogs(LogTypeUnknown, 0, 0, "", "User_082603", "", 0, 20, 0, "", "")
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, logs, 1)
	require.Equal(t, user.Id, logs[0].UserId)
}
