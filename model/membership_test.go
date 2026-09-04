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

func setupMembershipTestDB(t *testing.T) {
	t.Helper()
	oldDB := DB
	oldUsingSQLite := common.UsingSQLite
	dsn := fmt.Sprintf("file:membership_%d?mode=memory&cache=shared", time.Now().UnixNano())
	testDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	DB = testDB
	common.UsingSQLite = true
	require.NoError(t, DB.AutoMigrate(&User{}, &MembershipLevel{}, &UserMembership{}))
	require.NoError(t, EnsureDefaultMembershipLevel())
	InvalidateAllMembershipCaches()
	t.Cleanup(func() {
		InvalidateAllMembershipCaches()
		sqlDB, sqlErr := testDB.DB()
		if sqlErr == nil {
			_ = sqlDB.Close()
		}
		DB = oldDB
		common.UsingSQLite = oldUsingSQLite
	})
}

func createMembershipTestUser(t *testing.T, username string) User {
	t.Helper()
	user := User{Username: username, Password: "membership-test-password", Group: "default"}
	require.NoError(t, DB.Create(&user).Error)
	return user
}

func createMembershipTestLevel(t *testing.T, code string, ppm int64, rank int) MembershipLevel {
	t.Helper()
	level := MembershipLevel{
		Code:          code,
		DisplayName:   code,
		MultiplierPPM: ppm,
		Rank:          rank,
		Enabled:       true,
	}
	require.NoError(t, CreateMembershipLevel(&level))
	return level
}

func TestResolveUserMembershipFallsBackToNormal(t *testing.T) {
	setupMembershipTestDB(t)
	user := createMembershipTestUser(t, "membership-normal")

	snapshot, err := ResolveUserMembershipAt(user.Id, 1_800_000_000, false)
	require.NoError(t, err)
	require.Equal(t, DefaultMembershipCode, snapshot.Code)
	require.Equal(t, MembershipMultiplierScale, snapshot.MultiplierPPM)
	require.True(t, snapshot.FallbackNormal)
}

func TestResolveUserMembershipUsesHighestActiveRankAndFallsBack(t *testing.T) {
	setupMembershipTestDB(t)
	user := createMembershipTestUser(t, "membership-rank")
	vip1 := createMembershipTestLevel(t, "VIP1", 800_000, 10)
	vip2 := createMembershipTestLevel(t, "VIP2", 700_000, 20)

	require.NoError(t, CreateUserMembership(&UserMembership{
		UserId: user.Id, MembershipLevelId: vip1.Id, StartsAt: 100, EndsAt: 0,
	}))
	require.NoError(t, CreateUserMembership(&UserMembership{
		UserId: user.Id, MembershipLevelId: vip2.Id, StartsAt: 200, EndsAt: 400,
	}))

	duringVIP2, err := ResolveUserMembershipAt(user.Id, 300, false)
	require.NoError(t, err)
	require.Equal(t, "VIP2", duringVIP2.Code)
	require.Equal(t, int64(700_000), duringVIP2.MultiplierPPM)
	require.Equal(t, int64(400), duringVIP2.NextChangeAt)

	afterVIP2, err := ResolveUserMembershipAt(user.Id, 500, false)
	require.NoError(t, err)
	require.Equal(t, "VIP1", afterVIP2.Code)
	require.Equal(t, int64(800_000), afterVIP2.MultiplierPPM)
}

func TestResolveUserMembershipTracksFutureStartAsCacheBoundary(t *testing.T) {
	setupMembershipTestDB(t)
	user := createMembershipTestUser(t, "membership-future")
	vip := createMembershipTestLevel(t, "VIP-FUTURE", 850_000, 10)
	require.NoError(t, CreateUserMembership(&UserMembership{
		UserId: user.Id, MembershipLevelId: vip.Id, StartsAt: 500, EndsAt: 900,
	}))

	before, err := ResolveUserMembershipAt(user.Id, 400, false)
	require.NoError(t, err)
	require.Equal(t, DefaultMembershipCode, before.Code)
	require.Equal(t, int64(500), before.NextChangeAt)

	during, err := ResolveUserMembershipAt(user.Id, 600, false)
	require.NoError(t, err)
	require.Equal(t, "VIP-FUTURE", during.Code)
	require.Equal(t, int64(900), during.NextChangeAt)
}

func TestMembershipMultiplierValidation(t *testing.T) {
	setupMembershipTestDB(t)
	for _, ppm := range []int64{0, -1, MembershipMultiplierScale + 1} {
		level := MembershipLevel{Code: fmt.Sprintf("INVALID_%d", ppm+2), DisplayName: "invalid", MultiplierPPM: ppm, Enabled: true}
		require.Error(t, CreateMembershipLevel(&level))
	}

	valid := MembershipLevel{Code: "VALID", DisplayName: "valid", MultiplierPPM: 1, Enabled: true}
	require.NoError(t, CreateMembershipLevel(&valid))
}

func TestMembershipCacheInvalidatedOnGrantAndRevoke(t *testing.T) {
	setupMembershipTestDB(t)
	user := createMembershipTestUser(t, "membership-cache")
	vip := createMembershipTestLevel(t, "VIP-CACHE", 750_000, 10)

	initial, err := ResolveUserMembership(user.Id)
	require.NoError(t, err)
	require.Equal(t, DefaultMembershipCode, initial.Code)

	grant := UserMembership{UserId: user.Id, MembershipLevelId: vip.Id}
	require.NoError(t, CreateUserMembership(&grant))
	active, err := ResolveUserMembership(user.Id)
	require.NoError(t, err)
	require.Equal(t, "VIP-CACHE", active.Code)

	require.NoError(t, RevokeUserMembership(grant.Id, 99))
	revoked, err := ResolveUserMembership(user.Id)
	require.NoError(t, err)
	require.Equal(t, DefaultMembershipCode, revoked.Code)
}
