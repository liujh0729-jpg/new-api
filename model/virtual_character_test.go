package model

import (
	"errors"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setVirtualCharacterTestLimit(t *testing.T, limit string) {
	t.Helper()
	common.OptionMapRWMutex.Lock()
	wasNil := common.OptionMap == nil
	if wasNil {
		common.OptionMap = make(map[string]string)
	}
	previous, existed := common.OptionMap["VirtualCharacterLimit"]
	common.OptionMap["VirtualCharacterLimit"] = limit
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		if existed {
			common.OptionMap["VirtualCharacterLimit"] = previous
		} else {
			delete(common.OptionMap, "VirtualCharacterLimit")
		}
		if wasNil {
			common.OptionMap = nil
		}
		common.OptionMapRWMutex.Unlock()
	})
}

func cleanupVirtualCharacterTables(t *testing.T) {
	t.Helper()
	if DB.Migrator().HasTable(&VirtualCharacterTaskReference{}) {
		require.NoError(t, DB.Exec("DELETE FROM virtual_character_task_references").Error)
	}
	if DB.Migrator().HasTable(&VirtualCharacterAuthorization{}) {
		require.NoError(t, DB.Exec("DELETE FROM virtual_character_authorizations").Error)
	}
	require.NoError(t, DB.Exec("DELETE FROM virtual_character_tasks").Error)
	require.NoError(t, DB.Exec("DELETE FROM virtual_character_user_limits").Error)
	require.NoError(t, DB.Exec("DELETE FROM virtual_characters").Error)
	t.Cleanup(func() {
		if DB.Migrator().HasTable(&VirtualCharacterTaskReference{}) {
			DB.Exec("DELETE FROM virtual_character_task_references")
		}
		if DB.Migrator().HasTable(&VirtualCharacterAuthorization{}) {
			DB.Exec("DELETE FROM virtual_character_authorizations")
		}
		DB.Exec("DELETE FROM virtual_character_tasks")
		DB.Exec("DELETE FROM virtual_character_user_limits")
		DB.Exec("DELETE FROM virtual_characters")
	})
}

func TestReservePrivateVirtualCharacterEnforcesPerUserQuotaAndReleasesSlot(t *testing.T) {
	cleanupVirtualCharacterTables(t)
	setVirtualCharacterTestLimit(t, "2")

	first, limit, err := ReservePrivateVirtualCharacter(101, "one", "", "[]", "image/png", 10)
	require.NoError(t, err)
	require.Equal(t, 2, limit)
	require.NotNil(t, first.Slot)

	second, _, err := ReservePrivateVirtualCharacter(101, "two", "", "[]", "image/png", 10)
	require.NoError(t, err)
	require.NotEqual(t, *first.Slot, *second.Slot)

	_, _, err = ReservePrivateVirtualCharacter(101, "three", "", "[]", "image/png", 10)
	require.ErrorContains(t, err, "limit reached")

	otherUser, _, err := ReservePrivateVirtualCharacter(202, "other", "", "[]", "image/png", 10)
	require.NoError(t, err)
	require.Equal(t, 1, *otherUser.Slot)

	releasedSlot := *first.Slot
	require.NoError(t, BeginVirtualCharacterDelete(first, "test"))
	replacement, _, err := ReservePrivateVirtualCharacter(101, "replacement", "", "[]", "image/png", 10)
	require.NoError(t, err)
	require.Equal(t, releasedSlot, *replacement.Slot)
}

func TestGetAccessibleVirtualCharacterPreservesPrivateIsolation(t *testing.T) {
	cleanupVirtualCharacterTables(t)
	setVirtualCharacterTestLimit(t, "10")

	privateItem, _, err := ReservePrivateVirtualCharacter(303, "private", "", "[]", "image/png", 10)
	require.NoError(t, err)
	require.NoError(t, ActivateVirtualCharacter(privateItem.ID))

	_, err = GetAccessibleVirtualCharacter(privateItem.ID, 404)
	require.True(t, errors.Is(err, gorm.ErrRecordNotFound))
	owned, err := GetAccessibleVirtualCharacter(privateItem.ID, 303)
	require.NoError(t, err)
	require.Equal(t, privateItem.ID, owned.ID)

	publicItem := &VirtualCharacter{Name: "public", VolcAssetID: "asset-public", PublicChannelID: 7}
	require.NoError(t, CreatePublicVirtualCharacter(publicItem))
	_, err = GetAccessibleVirtualCharacter(publicItem.ID, 404)
	require.NoError(t, err)
	require.NoError(t, DeletePublicVirtualCharacter(publicItem.ID))
	_, err = GetAccessibleVirtualCharacter(publicItem.ID, 404)
	require.True(t, errors.Is(err, gorm.ErrRecordNotFound))
}

func TestRecoverVirtualCharacterTaskCreatesTaskExactlyOnce(t *testing.T) {
	cleanupVirtualCharacterTables(t)
	task := &Task{TaskID: "vc-task-recovery", UserId: 505, Status: TaskStatusSubmitted, Action: VirtualCharacterTaskAction}
	payload, err := common.Marshal(task)
	require.NoError(t, err)
	link := &VirtualCharacterTask{
		TaskID:          task.TaskID,
		UserID:          task.UserId,
		CharacterID:     99,
		CharacterName:   "snapshot",
		CharacterScope:  VirtualCharacterScopePrivate,
		Status:          VirtualCharacterTaskStatusReady,
		TaskPayloadJSON: string(payload),
	}
	require.NoError(t, DB.Create(link).Error)
	require.NoError(t, RecoverVirtualCharacterTask(link))

	var stored Task
	require.NoError(t, DB.Where("task_id = ?", task.TaskID).First(&stored).Error)
	require.Equal(t, task.UserId, stored.UserId)
	var storedLink VirtualCharacterTask
	require.NoError(t, DB.Where("task_id = ?", task.TaskID).First(&storedLink).Error)
	require.Equal(t, VirtualCharacterTaskStatusActive, storedLink.Status)
	require.Empty(t, storedLink.TaskPayloadJSON)

	// If Task insert succeeded but the link activation update failed, recovery
	// must observe the existing Task instead of inserting a duplicate.
	require.NoError(t, DB.Model(&VirtualCharacterTask{}).Where("task_id = ?", task.TaskID).Updates(map[string]any{
		"status":            VirtualCharacterTaskStatusReady,
		"task_payload_json": string(payload),
	}).Error)
	link.Status = VirtualCharacterTaskStatusReady
	link.TaskPayloadJSON = string(payload)
	require.NoError(t, RecoverVirtualCharacterTask(link))
	var count int64
	require.NoError(t, DB.Model(&Task{}).Where("task_id = ?", task.TaskID).Count(&count).Error)
	require.EqualValues(t, 1, count)
}

func TestTaskBindingProtectsEveryReferencedCharacterUntilRollback(t *testing.T) {
	cleanupVirtualCharacterTables(t)
	require.NoError(t, DB.AutoMigrate(&VirtualCharacterAuthorization{}, &VirtualCharacterTaskReference{}))
	first := &VirtualCharacter{UserID: 606, Scope: VirtualCharacterScopePrivate, SourceType: VirtualCharacterSourceVolcAIGC, Name: "First", Status: VirtualCharacterStatusActive}
	second := &VirtualCharacter{UserID: 606, Scope: VirtualCharacterScopePrivate, SourceType: VirtualCharacterSourceVolcRealPerson, Name: "Second", Status: VirtualCharacterStatusActive}
	require.NoError(t, DB.Create(first).Error)
	require.NoError(t, DB.Create(second).Error)
	taskID := "vc-multi-reference-submitting"
	require.NoError(t, CreateVirtualCharacterTaskBinding(&VirtualCharacterTask{
		TaskID: taskID, UserID: 606, CharacterID: first.ID, CharacterName: first.Name, CharacterScope: first.Scope,
	}, []*VirtualCharacter{first, second}, map[int64]string{first.ID: `{}`, second.ID: `{}`}))

	unfinished, err := HasUnfinishedVirtualCharacterTasks(second.ID)
	require.NoError(t, err)
	require.True(t, unfinished)
	require.NoError(t, RollbackVirtualCharacterTaskBinding(taskID))
	unfinished, err = HasUnfinishedVirtualCharacterTasks(second.ID)
	require.NoError(t, err)
	require.False(t, unfinished)
}

func TestNormalizeVirtualCharacterQuotaPlan(t *testing.T) {
	plan, cap, qpm := NormalizeVirtualCharacterQuotaPlan("free", 999, 99)
	require.Equal(t, VirtualCharacterQuotaPlanFree, plan)
	require.Equal(t, VirtualCharacterDefaultAccountAssetCap, cap)
	require.Equal(t, VirtualCharacterDefaultCreateAssetQPM, qpm)

	plan, cap, qpm = NormalizeVirtualCharacterQuotaPlan("paid", 1, 1)
	require.Equal(t, VirtualCharacterQuotaPlanPaid, plan)
	require.Equal(t, VirtualCharacterPaidAccountAssetCap, cap)
	require.Equal(t, VirtualCharacterPaidCreateAssetQPM, qpm)

	plan, cap, qpm = NormalizeVirtualCharacterQuotaPlan("custom", 1234, 45)
	require.Equal(t, VirtualCharacterQuotaPlanCustom, plan)
	require.Equal(t, 1234, cap)
	require.Equal(t, 45, qpm)
}
