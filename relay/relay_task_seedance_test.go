package relay

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestIsSeedanceOfficialTaskRequiresAIPDDOfficialProtocol(t *testing.T) {
	aipddPlatform := constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeAIPDD))
	official := &model.Task{
		Platform: aipddPlatform,
		PrivateData: model.TaskPrivateData{AIPDDExecution: &model.AIPDDTaskExecutionSnapshot{
			Protocol: "seedance_official",
		}},
	}
	require.True(t, isSeedanceOfficialTask(official))

	wrongProtocol := *official
	wrongProtocol.PrivateData.AIPDDExecution = &model.AIPDDTaskExecutionSnapshot{Protocol: "legacy"}
	require.False(t, isSeedanceOfficialTask(&wrongProtocol))

	wrongPlatform := *official
	wrongPlatform.Platform = constant.TaskPlatform("other")
	require.False(t, isSeedanceOfficialTask(&wrongPlatform))
}

func TestIsSeedanceOfficialTaskSupportsDirectDoubaoPlatforms(t *testing.T) {
	for _, channelType := range []int{constant.ChannelTypeDoubaoVideo, constant.ChannelTypeVolcEngine} {
		task := &model.Task{Platform: constant.TaskPlatform(strconv.Itoa(channelType))}
		require.True(t, isSeedanceOfficialTask(task), "channel type %d should support Seedance official fetch", channelType)
	}
}

func TestIsSeedanceOfficialTaskFallsBackToCatalogForOlderTasks(t *testing.T) {
	const modelName = "seedance-official-catalog-test"
	constant.SetAIPDDCapabilities([]constant.AIPDDCapability{{
		ModelName: modelName, ExecutionProtocol: "seedance_official",
	}})
	t.Cleanup(constant.ResetAIPDDCapabilities)

	task := &model.Task{
		Platform: constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeAIPDD)),
		Properties: model.Properties{
			OriginModelName: modelName,
		},
	}
	require.True(t, isSeedanceOfficialTask(task))
}

func TestSeedanceOfficialFetchReturnsTopLevelTaskAndEnforcesOwnership(t *testing.T) {
	previousDB := model.DB
	dsn := fmt.Sprintf("file:relay-seedance-official-%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Task{}))
	model.DB = db
	t.Cleanup(func() { model.DB = previousDB })

	requestedDuration := 6.0
	task := &model.Task{
		TaskID: "task_owned", UserId: 42, ChannelId: 999999,
		Platform: constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeAIPDD)),
		Status:   model.TaskStatusSuccess,
		Properties: model.Properties{
			OriginModelName: "AP Seedance",
		},
		PrivateData: model.TaskPrivateData{AIPDDExecution: &model.AIPDDTaskExecutionSnapshot{
			Protocol: "seedance_official", RequestedDuration: &requestedDuration,
		}},
	}
	task.SetData(map[string]any{
		"id": "cgt-private", "status": "succeeded",
		"content": map[string]any{"video_url": "https://cdn.example.com/out.mp4"},
	})
	require.NoError(t, db.Create(task).Error)

	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/v3/contents/generations/tasks/task_owned", nil)
	ctx.Params = gin.Params{{Key: "task_id", Value: "task_owned"}}
	ctx.Set("id", 42)

	body, taskErr := taskFetchByIDRespBodyBuilder(ctx)
	require.Nil(t, taskErr)
	var response map[string]any
	require.NoError(t, common.Unmarshal(body, &response))
	require.Equal(t, "task_owned", response["id"])
	require.Equal(t, "succeeded", response["status"])
	require.Equal(t, requestedDuration, response["duration"])
	require.NotContains(t, response, "data")
	require.NotContains(t, response, "code")

	otherUserCtx, _ := gin.CreateTestContext(httptest.NewRecorder())
	otherUserCtx.Request = httptest.NewRequest(http.MethodGet, "/api/v3/contents/generations/tasks/task_owned", nil)
	otherUserCtx.Params = gin.Params{{Key: "task_id", Value: "task_owned"}}
	otherUserCtx.Set("id", 43)
	_, taskErr = taskFetchByIDRespBodyBuilder(otherUserCtx)
	require.NotNil(t, taskErr)
	require.Equal(t, http.StatusNotFound, taskErr.StatusCode)

	directTask := &model.Task{
		TaskID:   "task_direct_doubao",
		UserId:   42,
		Platform: constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeDoubaoVideo)),
		Status:   model.TaskStatusSuccess,
		Properties: model.Properties{
			OriginModelName: "doubao-seedance-2-5-260628",
		},
	}
	directTask.SetData(map[string]any{
		"id":            "upstream-direct-private",
		"model":         "doubao-seedance-2-5-260628",
		"status":        "succeeded",
		"duration":      5,
		"resolution":    "480p",
		"output_format": "mp4",
		"content":       map[string]any{"video_url": "https://cdn.example.com/direct.mp4"},
	})
	require.NoError(t, db.Create(directTask).Error)

	directCtx, _ := gin.CreateTestContext(httptest.NewRecorder())
	directCtx.Request = httptest.NewRequest(http.MethodGet, "/api/v3/contents/generations/tasks/task_direct_doubao", nil)
	directCtx.Params = gin.Params{{Key: "task_id", Value: "task_direct_doubao"}}
	directCtx.Set("id", 42)
	directBody, directErr := taskFetchByIDRespBodyBuilder(directCtx)
	require.Nil(t, directErr)
	var directResponse map[string]any
	require.NoError(t, common.Unmarshal(directBody, &directResponse))
	require.Equal(t, "task_direct_doubao", directResponse["id"])
	require.Equal(t, "succeeded", directResponse["status"])
	require.Equal(t, "doubao-seedance-2-5-260628", directResponse["model"])
	require.Equal(t, "https://cdn.example.com/direct.mp4", directResponse["content"].(map[string]any)["video_url"])
	require.NotContains(t, directResponse, "task_id")
	require.NotContains(t, directResponse, "object")
}
