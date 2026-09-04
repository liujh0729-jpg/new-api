package controller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay"
	"github.com/QuantumNous/new-api/relay/channel"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

func seedancePublicError(c *gin.Context, status int, code string, message string) {
	c.JSON(status, gin.H{"error": gin.H{"code": code, "message": message, "type": "invalid_request_error"}})
}

func ListSeedanceOfficialTasks(c *gin.Context) {
	page, ok := seedanceBoundedQueryInt(c, "page_num", 1)
	if !ok {
		seedancePublicError(c, http.StatusBadRequest, "invalid_request_error", "page_num must be an integer from 1 to 500")
		return
	}
	pageSize, ok := seedanceBoundedQueryInt(c, "page_size", 20)
	if !ok {
		seedancePublicError(c, http.StatusBadRequest, "invalid_request_error", "page_size must be an integer from 1 to 500")
		return
	}
	var tasks []*model.Task
	platform := constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeSeedance))
	err := model.DB.Model(&model.Task{}).
		Joins("JOIN seedance_orders ON seedance_orders.new_api_task_id = tasks.task_id AND seedance_orders.deleted_at = 0 AND seedance_orders.public_protocol = ?", model.SeedanceProtocolOfficial).
		Where("tasks.user_id = ? AND tasks.platform = ?", c.GetInt("id"), platform).
		Order("tasks.id DESC").Find(&tasks).Error
	if err != nil {
		seedancePublicError(c, http.StatusInternalServerError, "server_error", "Failed to list tasks")
		return
	}
	requestedIDs := seedanceRequestedTaskIDs(c)
	statusFilter := strings.ToLower(strings.TrimSpace(c.Query("filter.status")))
	modelFilter := strings.TrimSpace(c.Query("filter.model"))
	serviceTierFilter := strings.TrimSpace(c.Query("filter.service_tier"))
	filtered := make([]*model.Task, 0, len(tasks))
	for _, task := range tasks {
		if !seedanceTaskMatchesFilters(task, requestedIDs, statusFilter, modelFilter, serviceTierFilter) {
			continue
		}
		filtered = append(filtered, task)
	}
	total := len(filtered)
	start := (page - 1) * pageSize
	if start > total {
		start = total
	}
	end := start + pageSize
	if end > total {
		end = total
	}
	adaptor := relay.GetTaskAdaptor(platform)
	converter, ok := adaptor.(channel.SeedanceOfficialTaskConverter)
	if !ok {
		seedancePublicError(c, http.StatusInternalServerError, "server_error", "Task response is unavailable")
		return
	}
	items := make([]json.RawMessage, 0, end-start)
	for _, task := range filtered[start:end] {
		body, convertErr := converter.ConvertToSeedanceOfficialTask(task)
		if convertErr != nil {
			seedancePublicError(c, http.StatusInternalServerError, "server_error", "Task response is unavailable")
			return
		}
		items = append(items, json.RawMessage(body))
	}
	c.JSON(http.StatusOK, gin.H{"items": items, "total": total})
}

func seedanceBoundedQueryInt(c *gin.Context, name string, fallback int) (int, bool) {
	raw := strings.TrimSpace(c.Query(name))
	if raw == "" {
		return fallback, true
	}
	value, err := strconv.Atoi(raw)
	return value, err == nil && value >= 1 && value <= 500
}

func seedanceRequestedTaskIDs(c *gin.Context) map[string]struct{} {
	result := map[string]struct{}{}
	for _, value := range c.QueryArray("filter.task_ids") {
		for _, item := range strings.Split(value, ",") {
			if item = strings.TrimSpace(item); item != "" {
				result[item] = struct{}{}
			}
		}
	}
	return result
}

func seedanceTaskMatchesFilters(task *model.Task, taskIDs map[string]struct{}, statusFilter string, modelFilter string, serviceTierFilter string) bool {
	if task == nil {
		return false
	}
	if len(taskIDs) > 0 {
		if _, ok := taskIDs[task.TaskID]; !ok {
			return false
		}
	}
	if modelFilter != "" && task.Properties.OriginModelName != modelFilter {
		return false
	}
	var data map[string]any
	_ = common.Unmarshal(task.Data, &data)
	publicStatus := strings.ToLower(strings.TrimSpace(fmt.Sprint(data["status"])))
	if publicStatus == "" {
		switch task.Status {
		case model.TaskStatusSuccess:
			publicStatus = "succeeded"
		case model.TaskStatusFailure:
			publicStatus = "failed"
		case model.TaskStatusInProgress:
			publicStatus = "running"
		default:
			publicStatus = "queued"
		}
	}
	if statusFilter != "" && publicStatus != statusFilter {
		return false
	}
	if serviceTierFilter != "" && strings.TrimSpace(fmt.Sprint(data["service_tier"])) != serviceTierFilter {
		return false
	}
	return true
}

func DeleteSeedanceOfficialTask(c *gin.Context) {
	deleteSeedanceTask(c, false)
}

func DeleteSeedanceOpenAIVideo(c *gin.Context) {
	deleteSeedanceTask(c, true)
}

func deleteSeedanceTask(c *gin.Context, deleteAfter bool) {
	taskID := strings.TrimSpace(c.Param("task_id"))
	task, exists, err := model.GetByTaskId(c.GetInt("id"), taskID)
	if err != nil {
		seedancePublicError(c, http.StatusInternalServerError, "server_error", "Failed to query task")
		return
	}
	if !exists || task == nil || task.Platform != constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeSeedance)) {
		seedancePublicError(c, http.StatusNotFound, "not_found", "task not found")
		return
	}
	order, err := model.GetVisibleSeedanceOrderByTaskID(task.TaskID)
	if err != nil {
		seedancePublicError(c, http.StatusNotFound, "not_found", "task not found")
		return
	}
	expectedProtocol := model.SeedanceProtocolOfficial
	if deleteAfter {
		expectedProtocol = model.SeedanceProtocolOpenAI
	}
	if order.PublicProtocol != expectedProtocol {
		seedancePublicError(c, http.StatusNotFound, "not_found", "task not found")
		return
	}
	if (order.OrderStatus == model.SeedanceOrderGenerationProcessing || order.OrderStatus == model.SeedanceOrderGenerationSubmitting || order.OrderStatus == model.SeedanceOrderReceived) &&
		strings.TrimSpace(task.PrivateData.UpstreamTaskID) != "" {
		if err := cancelSeedanceGenerationUpstream(c.Request.Context(), task); err != nil {
			seedancePublicError(c, http.StatusServiceUnavailable, "upstream_unavailable", "Video service is temporarily unavailable")
			return
		}
	}
	if err := service.CancelSeedanceWorkflow(c.Request.Context(), task, deleteAfter); err != nil {
		if errors.Is(err, service.ErrSeedanceRemoteCancelUnsupported) {
			// The message stays neutral: an ordinary user must not learn that an
			// internal processing stage owns the task or which supplier runs it.
			seedancePublicError(c, http.StatusConflict, "task_not_cancellable", "This task has already started processing and can no longer be cancelled")
			return
		}
		seedancePublicError(c, http.StatusServiceUnavailable, "upstream_unavailable", "Video service is temporarily unavailable")
		return
	}
	if deleteAfter {
		c.Status(http.StatusNoContent)
		return
	}
	var updated model.Task
	if err := model.DB.Where("id = ?", task.ID).First(&updated).Error; err != nil {
		seedancePublicError(c, http.StatusInternalServerError, "server_error", "Task response is unavailable")
		return
	}
	adaptor := relay.GetTaskAdaptor(updated.Platform)
	converter, ok := adaptor.(channel.SeedanceOfficialTaskConverter)
	if !ok {
		seedancePublicError(c, http.StatusInternalServerError, "server_error", "Task response is unavailable")
		return
	}
	body, err := converter.ConvertToSeedanceOfficialTask(&updated)
	if err != nil {
		seedancePublicError(c, http.StatusInternalServerError, "server_error", "Task response is unavailable")
		return
	}
	c.Data(http.StatusOK, "application/json", body)
}

func cancelSeedanceGenerationUpstream(ctx context.Context, task *model.Task) error {
	channelModel, err := model.CacheGetChannel(task.ChannelId)
	if err != nil {
		return err
	}
	key, _, err := model.ResolveSeedanceArkAPIKeyForTask(task.TaskID, task.ChannelId)
	if err != nil {
		return err
	}
	baseURL := strings.TrimRight(channelModel.GetBaseURL(), "/")
	if baseURL == "" {
		baseURL = strings.TrimRight(constant.ChannelBaseURLs[constant.ChannelTypeSeedance], "/")
	}
	endpoint := baseURL + relayconstant.SeedanceOfficialTasksPath + "/" + url.PathEscape(task.GetUpstreamTaskID())
	requestCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(requestCtx, http.MethodDelete, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	client, err := service.GetHttpClientWithProxy(channelModel.GetSetting().Proxy)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1024*1024))
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("generation provider returned HTTP %d", resp.StatusCode)
	}
	return nil
}
