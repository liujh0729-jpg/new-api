package model

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTaskToOpenAIVideoOnlySetsCompletedAtForTerminalTasks(t *testing.T) {
	inProgress := &Task{Status: TaskStatusInProgress, CreatedAt: 10, UpdatedAt: 20}
	require.Zero(t, inProgress.ToOpenAIVideo().CompletedAt)

	succeeded := &Task{Status: TaskStatusSuccess, CreatedAt: 10, UpdatedAt: 20, FinishTime: 19}
	require.Equal(t, int64(19), succeeded.ToOpenAIVideo().CompletedAt)

	legacyTerminal := &Task{Status: TaskStatusFailure, CreatedAt: 10, UpdatedAt: 20}
	require.Equal(t, int64(20), legacyTerminal.ToOpenAIVideo().CompletedAt)
}
