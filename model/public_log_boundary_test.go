package model

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestPublicLogsRemovePrivateProcessingMetadata(t *testing.T) {
	logs := []*Log{{ModelName: "ap_seedance", Content: "MediaKit enhancement failed",
		Other: `{"admin_info":{"key":"private"},"upstream_model":"private-model","nested":[{"stage":"super-resolution"}],"completion_tokens":123,"billing_mode":"task_pricing"}`}}
	formatUserLogs(logs, 0)
	require.NotContains(t, logs[0].Content, "MediaKit")
	require.NotContains(t, logs[0].Other, "private-model")
	require.NotContains(t, logs[0].Other, "super-resolution")
	require.Contains(t, logs[0].Other, `"completion_tokens":123`)
	require.Contains(t, logs[0].Other, `"billing_mode":"task_pricing"`)
}
