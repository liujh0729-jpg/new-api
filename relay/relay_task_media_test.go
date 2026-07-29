package relay

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/require"
)

func TestBuildTaskFetchDataOmitsFormatWhileImageIsProcessing(t *testing.T) {
	task := &model.Task{
		TaskID: "task_image_processing",
		Status: model.TaskStatusInProgress,
	}

	data := buildTaskFetchData(task, &relaycommon.TaskInfo{}, []byte(`{"data":{"status":"RUNNING"}}`), "image")

	require.NotContains(t, data, "format")
	require.Empty(t, data["url"])
	require.Nil(t, data["output"])
	require.Equal(t, "processing", data["status"])
}

func TestBuildTaskFetchDataReturnsRealImageFormatOnSuccess(t *testing.T) {
	tests := []struct {
		name       string
		resultURL  string
		rawBody    string
		wantFormat string
	}{
		{
			name:       "URL extension",
			resultURL:  "https://cdn.example.com/results/generated.webp?signature=abc",
			rawBody:    `{"data":{"status":"SUCCESS"}}`,
			wantFormat: "webp",
		},
		{
			name:       "MIME metadata",
			resultURL:  "https://cdn.example.com/download/signed-result",
			rawBody:    `{"data":{"output":{"url":"https://cdn.example.com/download/signed-result","mimeType":"image/jpeg"}}}`,
			wantFormat: "jpeg",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			task := &model.Task{
				TaskID: "task_image_success",
				Status: model.TaskStatusSuccess,
				PrivateData: model.TaskPrivateData{
					ResultURL: test.resultURL,
				},
			}

			data := buildTaskFetchData(task, &relaycommon.TaskInfo{}, []byte(test.rawBody), "image")

			require.Equal(t, test.wantFormat, data["format"])
			require.Equal(t, test.resultURL, data["url"])
			require.Equal(t, []string{test.resultURL}, data["output"])
		})
	}
}

func TestBuildTaskFetchDataOmitsFakeMediaFieldsOnFailure(t *testing.T) {
	task := &model.Task{
		TaskID:     "task_image_failure",
		Status:     model.TaskStatusFailure,
		FailReason: "no worker accepted the task",
	}

	applyTaskResultURL(task, "", "image")
	data := buildTaskFetchData(task, &relaycommon.TaskInfo{}, []byte(`{"data":{"status":"FAILED"}}`), "image")

	require.NotContains(t, data, "format")
	require.Empty(t, data["url"])
	require.Nil(t, data["output"])
	require.Equal(t, "no worker accepted the task", data["error"])
	require.Empty(t, task.PrivateData.ResultURL)
}

func TestVideoSuccessKeepsMP4ProxyFallback(t *testing.T) {
	task := &model.Task{
		TaskID: "task_video_success",
		Status: model.TaskStatusSuccess,
	}

	applyTaskResultURL(task, "", "video")
	data := buildTaskFetchData(task, &relaycommon.TaskInfo{}, []byte(`{"data":{"status":"SUCCESS"}}`), "video")

	require.True(t, strings.HasSuffix(task.PrivateData.ResultURL, "/v1/videos/task_video_success/content"))
	require.Equal(t, "mp4", data["format"])
	require.Equal(t, task.PrivateData.ResultURL, data["url"])
}

func TestAudioSuccessUsesAudioFormatWithoutVideoProxy(t *testing.T) {
	const resultURL = "https://cdn.example.com/audio/result.wav"
	task := &model.Task{
		TaskID: "task_audio_success",
		Status: model.TaskStatusSuccess,
	}

	applyTaskResultURL(task, resultURL, "audio")
	data := buildTaskFetchData(task, &relaycommon.TaskInfo{}, []byte(`{"data":{"status":"SUCCESS"}}`), "audio")

	require.Equal(t, resultURL, task.PrivateData.ResultURL)
	require.NotContains(t, task.PrivateData.ResultURL, "/v1/videos/")
	require.Equal(t, "wav", data["format"])
}

func TestResolveTaskMediaInfoSupportsSnapshotsAndLegacyTasks(t *testing.T) {
	t.Run("snapshot is authoritative", func(t *testing.T) {
		task := &model.Task{
			PrivateData: model.TaskPrivateData{
				AIPDDExecution: &model.AIPDDTaskExecutionSnapshot{
					EndpointType:     constant.EndpointTypeImageGeneration,
					MediaType:        "image",
					TaskKind:         "image_to_image",
					OutputModalities: []string{"image"},
				},
			},
		}

		info := resolveTaskMediaInfo(task, "/v1/video/generations/task_legacy")

		require.Equal(t, constant.EndpointTypeImageGeneration, info.EndpointType)
		require.Equal(t, "image", info.MediaType)
		require.Equal(t, "image_to_image", info.TaskKind)
	})

	t.Run("legacy route fallback", func(t *testing.T) {
		info := resolveTaskMediaInfo(&model.Task{}, "/v1/images/generations/task_legacy")

		require.Equal(t, constant.EndpointTypeImageGeneration, info.EndpointType)
		require.Equal(t, "image", info.MediaType)
		require.Equal(t, []string{"image"}, info.OutputModalities)
	})

	t.Run("legacy model catalog fallback", func(t *testing.T) {
		task := &model.Task{
			Properties: model.Properties{
				OriginModelName: constant.AIPDDModelFluxGGUF,
			},
		}

		info := resolveTaskMediaInfo(task, "")

		require.Equal(t, constant.EndpointTypeImageGeneration, info.EndpointType)
		require.Equal(t, "image", info.MediaType)
	})
}

func TestTaskModel2DtoExposesPersistedMediaMetadata(t *testing.T) {
	task := &model.Task{
		TaskID: "task_image_log",
		PrivateData: model.TaskPrivateData{
			AIPDDExecution: &model.AIPDDTaskExecutionSnapshot{
				EndpointType:     constant.EndpointTypeImageGeneration,
				MediaType:        "image",
				TaskKind:         "image_to_image",
				OutputModalities: []string{"image"},
			},
		},
	}

	taskDTO := TaskModel2Dto(task)

	require.Equal(t, string(constant.EndpointTypeImageGeneration), taskDTO.EndpointType)
	require.Equal(t, "image", taskDTO.MediaType)
	require.Equal(t, "image_to_image", taskDTO.TaskKind)
	require.Equal(t, []string{"image"}, taskDTO.OutputModalities)
}

func TestTaskModel2DtoDoesNotExposeFailureReasonAsResultURL(t *testing.T) {
	taskDTO := TaskModel2Dto(&model.Task{
		TaskID:     "task_failed_log",
		Status:     model.TaskStatusFailure,
		FailReason: "no worker accepted the task",
	})

	require.Empty(t, taskDTO.ResultURL)
	require.Equal(t, "no worker accepted the task", taskDTO.FailReason)
}
