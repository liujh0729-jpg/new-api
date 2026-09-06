package relay

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
)

func TestPublicTaskProjectionKeepsPrivateSnapshot(t *testing.T) {
	task := &model.Task{TaskID: "task_public", Status: model.TaskStatusSuccess,
		Properties: model.Properties{OriginModelName: "ap_seedance", UpstreamModelName: "doubao-seedance-private"}}
	task.PrivateData.ResultURL = "https://supplier.invalid/upscale.mp4?token=private"
	task.Data = []byte(`{"id":"private-id","status":"succeeded","model":"private-model","content":{"video_url":"https://supplier.invalid/private.mp4","base_video_url":"https://supplier.invalid/base.mp4"},"source_resolution":"480p","metadata":{"stage":"private-stage"},"usage":{"completion_tokens":9223372036854775807,"total_tokens":9223372036854775807}}`)
	original := string(task.Data)
	result := TaskModel2Dto(task)
	encoded, err := common.Marshal(result)
	require.NoError(t, err)
	for _, value := range []string{"supplier.invalid", "upscale", "source_resolution", "private-model", "doubao-seedance-private", "private-stage", "private-id"} {
		require.NotContains(t, string(encoded), value)
	}
	require.Contains(t, string(encoded), "/v1/videos/task_public/content")
	require.Contains(t, string(encoded), "9223372036854775807")
	require.Equal(t, original, string(task.Data))
	admin, err := common.Marshal(TaskModel2Dto(task, true))
	require.NoError(t, err)
	require.Contains(t, string(admin), "supplier.invalid")
	task.PrivateData.ResultURL = ""
	task.FailReason = "https://supplier.invalid/legacy.mp4"
	legacy, err := common.Marshal(TaskModel2Dto(task))
	require.NoError(t, err)
	require.NotContains(t, string(legacy), "supplier.invalid")
}

func TestPublicFailedAndMalformedTasksDoNotExposeDetails(t *testing.T) {
	task := &model.Task{TaskID: "task_failed", Status: model.TaskStatusFailure,
		Properties: model.Properties{OriginModelName: "ap_seedance"},
		FailReason: "private enhancement stage failed", Data: []byte(`{"status":"failed","error":{"message":"private enhancement stage failed"}}`)}
	result := TaskModel2Dto(task)
	require.NotContains(t, result.FailReason, "enhancement")
	task.Data = []byte(`not-json-private-detail`)
	task.Status, task.FailReason = model.TaskStatusSuccess, ""
	result = TaskModel2Dto(task)
	require.Equal(t, `{}`, string(result.Data))
}

func TestCompactFetchUsesAuthenticatedMedia(t *testing.T) {
	task := &model.Task{TaskID: "task_compact", Status: model.TaskStatusSuccess,
		Properties: model.Properties{OriginModelName: "ap_seedance"}}
	task.PrivateData.ResultURL = "https://supplier.invalid/private.mp4"
	result := buildTaskFetchData(task, nil, nil, "video")
	require.True(t, strings.HasSuffix(result["url"].(string), "/v1/videos/task_compact/content"))
	encoded, err := common.Marshal(result)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "supplier.invalid")
}
