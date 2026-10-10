package model

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetByTaskIdScopesTaskToUser(t *testing.T) {
	truncateTables(t)

	insertTask(t, &Task{
		TaskID: "task_owned_by_user_101",
		UserId: 101,
		Status: TaskStatusInProgress,
		Data:   json.RawMessage(`{}`),
	})

	task, exists, err := GetByTaskId(101, "task_owned_by_user_101")
	require.NoError(t, err)
	require.True(t, exists)
	require.Equal(t, 101, task.UserId)

	task, exists, err = GetByTaskId(202, "task_owned_by_user_101")
	require.NoError(t, err)
	require.False(t, exists)
}
