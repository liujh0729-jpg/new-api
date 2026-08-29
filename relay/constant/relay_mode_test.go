package constant

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPath2RelayModeUsesGenericFetchForNonVideoTasks(t *testing.T) {
	require.Equal(t, RelayModeTaskFetchByID, Path2RelayMode("/v1/images/generations/task_image"))
	require.Equal(t, RelayModeTaskFetchByID, Path2RelayMode("/pg/images/generations/task_image"))
	require.Equal(t, RelayModeTaskFetchByID, Path2RelayMode("/v1/audio/speech/task_audio"))
	require.Equal(t, RelayModeVideoFetchByID, Path2RelayMode("/v1/videos/task_video"))
	require.Equal(t, RelayModeVideoSubmit, Path2RelayMode("/v1/videos"))
	require.Equal(t, RelayModeVideoFetchByID, Path2RelayMode("/pg/video/generations/task_video"))
	require.Equal(t, RelayModeVideoSubmit, Path2RelayMode(SeedanceOfficialTasksPath))
	require.Equal(t, RelayModeVideoFetchByID, Path2RelayMode(SeedanceOfficialTasksPath+"/task_seedance"))
	require.Equal(t, RelayModeUnknown, Path2RelayMode(SeedanceOfficialTasksPath+"-invalid"))
	require.Equal(t, RelayModeUnknown, Path2RelayMode("/v1/video/generations"))
	require.Equal(t, RelayModeUnknown, Path2RelayMode("/v1/video/generations/task_video"))
}
