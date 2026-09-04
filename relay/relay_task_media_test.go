package relay

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
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

		info := resolveTaskMediaInfo(task, "/v1/videos/task_openai")

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

		require.Equal(t, constant.EndpointTypeImageToImage, info.EndpointType)
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

func TestTaskModel2DtoExposesQuotaCNYFromBillingSnapshot(t *testing.T) {
	taskDTO := TaskModel2Dto(&model.Task{
		Quota: 5000,
		PrivateData: model.TaskPrivateData{
			BillingContext: &model.TaskBillingContext{
				QuotaPerUnit:    500000,
				USDExchangeRate: 7.3,
			},
		},
	})

	require.Equal(t, 5000, taskDTO.Quota)
	require.Equal(t, 0.073, taskDTO.QuotaCNY)
	require.Equal(t, 0.073, taskDTO.CostCNY)
	require.Equal(t, "CNY", taskDTO.Currency)
}

func TestTaskModel2DtoRemovesUpstreamMoneyFields(t *testing.T) {
	taskDTO := TaskModel2Dto(&model.Task{
		TaskID: "task_private_money",
		Data: common.StringToByteSlice(`{
			"data": {
				"status": "SUCCESS",
				"task_cost": 144382,
				"draw_user_reward": 12,
				"billing_scope": "provider",
				"nested": {"unit_price_usd": 0.08, "url": "https://example.com/video.mp4"}
			}
		}`),
	})

	var payload map[string]any
	require.NoError(t, common.Unmarshal(taskDTO.Data, &payload))
	data, ok := payload["data"].(map[string]any)
	require.True(t, ok)
	require.NotContains(t, data, "task_cost")
	require.NotContains(t, data, "draw_user_reward")
	require.NotContains(t, data, "billing_scope")
	nested, ok := data["nested"].(map[string]any)
	require.True(t, ok)
	require.NotContains(t, nested, "unit_price_usd")
	require.Equal(t, "https://example.com/video.mp4", nested["url"])
}

func TestTaskModel2DtoIndependentSeedanceNeverExposesPrivateWorkflow(t *testing.T) {
	task := &model.Task{
		TaskID:   "task_public_seedance_log",
		Platform: constant.TaskPlatform("59"),
		Status:   model.TaskStatusSuccess,
		Progress: "100%",
		Properties: model.Properties{
			OriginModelName:   "Public Seedance 2.5",
			UpstreamModelName: "private-provider-model",
		},
		PrivateData: model.TaskPrivateData{
			ResultURL: "https://private-supplier.example/enhancement/result.mp4",
		},
		Data: common.StringToByteSlice(`{
			"status":"succeeded",
			"provider_type":"DIRECT_EXTERNAL",
			"execution_task_id":"private-execution-id",
			"content":{"video_url":"https://private-supplier.example/enhancement/result.mp4"}
		}`),
	}

	taskDTO := TaskModel2Dto(task)
	encoded, err := common.Marshal(taskDTO)
	require.NoError(t, err)
	normalized := strings.ToLower(string(encoded))
	for _, forbidden := range []string{
		"private-supplier", "private-provider", "enhancement", "provider_type",
		"execution_task_id", "direct_external",
	} {
		require.NotContains(t, normalized, forbidden)
	}
	require.Equal(t, taskcommon.BuildProxyURL(task.TaskID), taskDTO.ResultURL)
	require.Equal(t, []string{taskcommon.BuildProxyURL(task.TaskID)}, taskDTO.Output)
	require.Equal(t, task.Properties.OriginModelName, taskDTO.Properties.(model.Properties).UpstreamModelName)
	require.Contains(t, string(taskDTO.Data), taskcommon.BuildProxyURL(task.TaskID))
}

func TestTaskModel2DtoIndependentSeedanceFailureUsesGenericMessage(t *testing.T) {
	task := &model.Task{
		TaskID:     "task_failed_seedance_log",
		Platform:   constant.TaskPlatform("59"),
		Status:     model.TaskStatusFailure,
		FailReason: "private provider upscale failed with credential secret",
		Data:       common.StringToByteSlice(`{"provider":"private","error":"upscale failed"}`),
		Properties: model.Properties{
			OriginModelName:   "Public Seedance 2.5",
			UpstreamModelName: "private-model-id",
		},
	}

	taskDTO := TaskModel2Dto(task)
	encoded, err := common.Marshal(taskDTO)
	require.NoError(t, err)
	normalized := strings.ToLower(string(encoded))
	for _, forbidden := range []string{"provider", "upscale", "credential", "secret", "private-model-id"} {
		require.NotContains(t, normalized, forbidden)
	}
	require.Equal(t, "视频处理失败，请稍后重试", taskDTO.FailReason)
	require.Empty(t, taskDTO.ResultURL)
	require.Empty(t, taskDTO.Output)
}
