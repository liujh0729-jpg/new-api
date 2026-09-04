package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestListSeedanceOfficialTasksEnforcesOwnershipProtocolFiltersAndPagination(t *testing.T) {
	previousDB := model.DB
	db, err := gorm.Open(sqlite.Open("file:seedance-official-list?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	t.Cleanup(func() {
		if sqlDB, sqlErr := db.DB(); sqlErr == nil {
			_ = sqlDB.Close()
		}
		model.DB = previousDB
	})
	require.NoError(t, db.AutoMigrate(&model.Task{}, &model.SeedanceOrder{}))

	platform := constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeSeedance))
	createTask := func(taskID string, userID int, protocol string, status model.TaskStatus, publicStatus, publicModel, tier string, deletedAt int64) {
		task := &model.Task{
			TaskID: taskID, Platform: platform, UserId: userID, Status: status,
			Properties: model.Properties{OriginModelName: publicModel},
		}
		task.SetData(map[string]any{
			"id": taskID, "model": publicModel, "status": publicStatus, "service_tier": tier,
		})
		require.NoError(t, db.Create(task).Error)
		require.NoError(t, db.Create(&model.SeedanceOrder{
			PlatformOrderID: "order-" + taskID, NewAPITaskID: taskID, NewAPIUserID: userID,
			ChannelID: 901, Model: publicModel, OrderStatus: model.SeedanceOrderGenerationProcessing,
			PublicProtocol: protocol, DeletedAt: deletedAt,
		}).Error)
	}
	createTask("task-owned-queued", 44, model.SeedanceProtocolOfficial, model.TaskStatusSubmitted, "queued", "Public A", "default", 0)
	createTask("task-owned-success", 44, model.SeedanceProtocolOfficial, model.TaskStatusSuccess, "succeeded", "Public B", "premium", 0)
	createTask("task-owned-openai", 44, model.SeedanceProtocolOpenAI, model.TaskStatusSuccess, "succeeded", "Public B", "premium", 0)
	createTask("task-other-user", 45, model.SeedanceProtocolOfficial, model.TaskStatusSuccess, "succeeded", "Public B", "premium", 0)
	createTask("task-owned-deleted", 44, model.SeedanceProtocolOfficial, model.TaskStatusSuccess, "succeeded", "Public B", "premium", 1)

	type listResponse struct {
		Items []map[string]any `json:"items"`
		Total int              `json:"total"`
	}
	request := func(target string) (*httptest.ResponseRecorder, listResponse) {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Set("id", 44)
		ctx.Request = httptest.NewRequest(http.MethodGet, target, nil)
		ListSeedanceOfficialTasks(ctx)
		var response listResponse
		if recorder.Code == http.StatusOK {
			require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
		}
		return recorder, response
	}

	recorder, firstPage := request("/api/v3/contents/generations/tasks?page_num=1&page_size=1")
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, 2, firstPage.Total)
	require.Len(t, firstPage.Items, 1)
	require.Equal(t, "task-owned-success", firstPage.Items[0]["id"])
	require.Equal(t, "Public B", firstPage.Items[0]["model"])
	require.Equal(t, "succeeded", firstPage.Items[0]["status"])

	recorder, secondPage := request("/api/v3/contents/generations/tasks?page_num=2&page_size=1")
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, 2, secondPage.Total)
	require.Equal(t, "task-owned-queued", secondPage.Items[0]["id"])

	recorder, filtered := request("/api/v3/contents/generations/tasks?filter.status=queued&filter.model=Public+A&filter.service_tier=default&filter.task_ids=task-owned-queued,task-other-user")
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, 1, filtered.Total)
	require.Len(t, filtered.Items, 1)
	require.Equal(t, "task-owned-queued", filtered.Items[0]["id"])

	recorder, _ = request("/api/v3/contents/generations/tasks?page_num=0")
	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.JSONEq(t, `{"error":{"code":"invalid_request_error","message":"page_num must be an integer from 1 to 500","type":"invalid_request_error"}}`, recorder.Body.String())
}

func TestDeleteTerminalSeedanceOfficialTaskReturnsPublicSnapshot(t *testing.T) {
	previousDB := model.DB
	db, err := gorm.Open(sqlite.Open("file:seedance-official-delete?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	t.Cleanup(func() {
		if sqlDB, sqlErr := db.DB(); sqlErr == nil {
			_ = sqlDB.Close()
		}
		model.DB = previousDB
	})
	require.NoError(t, db.AutoMigrate(&model.Task{}, &model.SeedanceOrder{}))

	task := &model.Task{
		TaskID:   "task_terminal_seedance",
		Platform: constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeSeedance)),
		UserId:   44,
		Status:   model.TaskStatusFailure,
		Data:     []byte(`{"id":"private-id","model":"Private model","status":"cancelled"}`),
		Properties: model.Properties{
			OriginModelName: "Public Seedance",
		},
	}
	require.NoError(t, db.Create(task).Error)
	require.NoError(t, db.Create(&model.SeedanceOrder{
		PlatformOrderID: "01990a4c-8f5a-7ca2-9f95-77cc19375c91",
		NewAPITaskID:    task.TaskID,
		NewAPIUserID:    task.UserId,
		ChannelID:       901,
		Model:           "Public Seedance",
		OrderStatus:     model.SeedanceOrderCancelled,
		PublicProtocol:  model.SeedanceProtocolOfficial,
	}).Error)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set("id", task.UserId)
	ctx.Params = gin.Params{{Key: "task_id", Value: task.TaskID}}
	ctx.Request = httptest.NewRequest(http.MethodDelete, "/api/v3/contents/generations/tasks/"+task.TaskID, nil)

	DeleteSeedanceOfficialTask(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.JSONEq(t, `{"id":"task_terminal_seedance","model":"Public Seedance","status":"cancelled"}`, recorder.Body.String())
	_, err = model.GetVisibleSeedanceOrderByTaskID(task.TaskID)
	require.Error(t, err)
}

func TestDeleteSeedanceTaskRejectsTheOtherPublicProtocol(t *testing.T) {
	previousDB := model.DB
	db, err := gorm.Open(sqlite.Open("file:seedance-protocol-delete?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	t.Cleanup(func() {
		if sqlDB, sqlErr := db.DB(); sqlErr == nil {
			_ = sqlDB.Close()
		}
		model.DB = previousDB
	})
	require.NoError(t, db.AutoMigrate(&model.Task{}, &model.SeedanceOrder{}))

	task := &model.Task{
		TaskID: "task_openai_seedance", Platform: constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeSeedance)),
		UserId: 45, Status: model.TaskStatusSuccess,
	}
	require.NoError(t, db.Create(task).Error)
	require.NoError(t, db.Create(&model.SeedanceOrder{
		PlatformOrderID: "01990a4c-8f5a-7ca2-9f95-77cc19375c92", NewAPITaskID: task.TaskID,
		NewAPIUserID: task.UserId, ChannelID: 901, OrderStatus: model.SeedanceOrderSucceeded,
		PublicProtocol: model.SeedanceProtocolOpenAI,
	}).Error)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set("id", task.UserId)
	ctx.Params = gin.Params{{Key: "task_id", Value: task.TaskID}}
	ctx.Request = httptest.NewRequest(http.MethodDelete, "/api/v3/contents/generations/tasks/"+task.TaskID, nil)

	DeleteSeedanceOfficialTask(ctx)

	require.Equal(t, http.StatusNotFound, recorder.Code)
	require.JSONEq(t, `{"error":{"code":"not_found","message":"task not found","type":"invalid_request_error"}}`, recorder.Body.String())
	_, err = model.GetVisibleSeedanceOrderByTaskID(task.TaskID)
	require.NoError(t, err)
}

func TestDeleteUnknownSeedanceGenerationDoesNotCallArkWithPublicTaskID(t *testing.T) {
	previousDB := model.DB
	db, err := gorm.Open(sqlite.Open("file:seedance-unknown-submit-delete?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	t.Cleanup(func() {
		if sqlDB, sqlErr := db.DB(); sqlErr == nil {
			_ = sqlDB.Close()
		}
		model.DB = previousDB
	})
	require.NoError(t, db.AutoMigrate(
		&model.Task{}, &model.SeedanceOrder{}, &model.SeedanceAttempt{},
		&model.MediaServiceUsage{}, &model.ServiceBillingEvent{}, &model.ServiceBillingOutbox{},
		&model.SeedanceCustomerRefund{},
	))

	now := int64(1_788_000_000)
	task := &model.Task{
		TaskID: "task_unknown_seedance", Platform: constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeSeedance)),
		UserId: 46, ChannelId: 999_940, Status: model.TaskStatusSubmitted,
		Properties: model.Properties{OriginModelName: "Public Seedance", UpstreamModelName: "Public Seedance"},
		SubmitTime: now, CreatedAt: now, UpdatedAt: now,
	}
	task.SetData(map[string]any{"id": task.TaskID, "model": "Public Seedance", "status": "queued"})
	require.NoError(t, db.Create(task).Error)
	order := &model.SeedanceOrder{
		PlatformOrderID: "01990a4c-8f5a-7ca2-9f95-77cc19375c94", NewAPITaskID: task.TaskID,
		NewAPIUserID: task.UserId, ChannelID: task.ChannelId, InstanceID: "instance-test",
		Model: "Public Seedance", OrderStatus: model.SeedanceOrderGenerationSubmitting,
		VolcengineCostStatus: model.SeedanceCostEstimated, SyncStatus: model.SeedanceSyncPending,
		PricingSnapshotJSON: `{"pricing_version":"price-v1","provider_type":"DIRECT_EXTERNAL","provider_id":1,"service_code":"private","specification":"{}","specification_version":"spec-v1"}`,
		PricingSnapshotHash: model.SHA256Evidence("price"), PublicProtocol: model.SeedanceProtocolOfficial,
		CreatedAt: now, UpdatedAt: now,
	}
	require.NoError(t, db.Create(order).Error)
	require.NoError(t, db.Create(&model.SeedanceAttempt{
		PlatformOrderID: order.PlatformOrderID, AttemptID: order.PlatformOrderID + ":generation:1",
		Stage: "GENERATION", AttemptNo: 1, Status: model.SeedanceSubmissionOutcomeUnknown,
		CreatedAt: now, UpdatedAt: now,
	}).Error)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set("id", task.UserId)
	ctx.Params = gin.Params{{Key: "task_id", Value: task.TaskID}}
	ctx.Request = httptest.NewRequest(http.MethodDelete, "/api/v3/contents/generations/tasks/"+task.TaskID, nil)

	// No Channel row or Ark credential exists. Success therefore proves the
	// controller did not mistake the public task ID for an upstream task ID.
	DeleteSeedanceOfficialTask(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	var cancelled model.SeedanceOrder
	require.NoError(t, db.Where("id = ?", order.ID).First(&cancelled).Error)
	require.Equal(t, model.SeedanceOrderCancelled, cancelled.OrderStatus)
	var attempt model.SeedanceAttempt
	require.NoError(t, db.Where("platform_order_id = ?", order.PlatformOrderID).First(&attempt).Error)
	require.Equal(t, model.SeedanceOrderCancelled, attempt.Status)
}
