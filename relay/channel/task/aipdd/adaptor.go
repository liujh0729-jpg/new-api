package aipdd

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
)

const (
	ChannelName                       = "aipdd"
	seedanceOfficialPayloadContextKey = "aipdd_seedance_official_payload"

	ModelFluxGGUF         = constant.AIPDDModelFluxGGUF
	ModelFluxGGUFT2I      = constant.AIPDDModelFluxGGUFT2I
	ModelWan22Wanx        = constant.AIPDDModelWan22Wanx
	ModelWan22Animater    = constant.AIPDDModelWan22Animater
	ModelMimicMotion      = constant.AIPDDModelMimicMotion
	ModelLatentsync15     = constant.AIPDDModelLatentsync15
	ModelIndexTTS         = constant.AIPDDModelIndexTTS
	defaultTaskNamePrefix = "new-api"
)

var ModelList = constant.GetAIPDDTaskModelList()

type modelConfig = constant.AIPDDCapability

type TaskAdaptor struct {
	taskcommon.BaseBilling
	apiKey  string
	baseURL string
	proxy   string
}

type createTaskPayload struct {
	RequestID    string         `json:"requestId,omitempty"`
	TaskName     string         `json:"taskName,omitempty"`
	TaskTypeCode string         `json:"taskTypeCode"`
	Priority     int            `json:"priority,omitempty"`
	Input        map[string]any `json:"input"`
	Requirements map[string]any `json:"requirements,omitempty"`
}

type createTaskResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    struct {
		ID           string  `json:"id"`
		TaskID       string  `json:"taskId"`
		RequestID    string  `json:"requestId"`
		TaskName     string  `json:"taskName"`
		TaskTypeCode string  `json:"taskTypeCode"`
		Status       string  `json:"status"`
		TaskType     string  `json:"task_type"`
		TaskStatus   int     `json:"task_status"`
		TaskCost     float64 `json:"task_cost"`
		TaskTime     string  `json:"task_time"`
	} `json:"data"`
}

type taskDetailResponse struct {
	Code    int        `json:"code"`
	Message string     `json:"message"`
	Data    *aipddTask `json:"data"`
}

// seedanceOfficialTaskResponse is the response shape used by the official
// Seedance task endpoint.  It is intentionally separate from taskDetailResponse:
// the official endpoint returns the task directly at the top level, while the
// legacy AIPDD endpoint wraps it in data.
type seedanceOfficialTaskResponse struct {
	ID      string `json:"id"`
	Status  string `json:"status"`
	Code    any    `json:"code"`
	Message string `json:"message"`
	Content struct {
		VideoURL string `json:"video_url"`
	} `json:"content"`
	Error any `json:"error"`
}

type aipddTask struct {
	ID             string  `json:"id"`
	TaskID         string  `json:"taskId"`
	RequestID      string  `json:"requestId"`
	TaskName       string  `json:"task_name"`
	TaskType       string  `json:"task_type"`
	TaskTypeCode   string  `json:"taskTypeCode"`
	TaskStatus     int     `json:"task_status"`
	Status         string  `json:"status"`
	Progress       int     `json:"progress"`
	Stage          string  `json:"stage"`
	ResultReady    bool    `json:"resultReady"`
	Message        string  `json:"message"`
	TaskTime       string  `json:"task_time"`
	TaskCost       float64 `json:"task_cost"`
	TaskService    string  `json:"task_service"`
	TaskContent    string  `json:"task_content"`
	DrawUserID     string  `json:"draw_user_id"`
	DrawUserReward float64 `json:"draw_user_reward"`
	DrawTime       string  `json:"draw_time"`
	TaskResult     any     `json:"task_result"`
	ReqID          string  `json:"req_id"`
	ReqIP          string  `json:"req_ip"`
	IsPay          int     `json:"is_pay"`
	Icon           float64 `json:"icon"`
	ScriptID       string  `json:"script_id"`
	ScriptCode     string  `json:"script_code"`
	Output         any     `json:"output"`
	ObjectRefs     any     `json:"objectRefs"`
	DownloadRefs   any     `json:"downloadRefs"`
	ResultStatus   string  `json:"resultStatus"`
	Checksum       string  `json:"checksum"`
}

func (a *TaskAdaptor) Init(info *relaycommon.RelayInfo) {
	a.baseURL = strings.TrimRight(info.ChannelBaseUrl, "/")
	if a.baseURL == "" {
		a.baseURL = constant.ChannelBaseURLs[constant.ChannelTypeAIPDD]
	}
	a.apiKey = info.ApiKey
	a.proxy = info.ChannelSetting.Proxy
}

func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) *dto.TaskError {
	var req relaycommon.TaskSubmitReq
	if err := common.UnmarshalBodyReusable(c, &req); err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
	}

	mergeUnknownFieldsIntoMetadata(c, &req)
	normalizeTaskSubmitReq(&req)

	if strings.TrimSpace(req.Model) == "" && strings.TrimSpace(info.OriginModelName) != "" {
		req.Model = info.OriginModelName
	}
	if strings.TrimSpace(req.Model) == "" {
		return service.TaskErrorWrapperLocal(fmt.Errorf("model field is required"), "missing_model", http.StatusBadRequest)
	}
	configModelName := firstNonEmpty(info.UpstreamModelName, req.Model, info.OriginModelName)
	cfg, ok := a.resolveModelConfig(ginRequestContext(c), configModelName)
	if !ok {
		return service.TaskErrorWrapperLocal(fmt.Errorf("unsupported AIPDD model: %s", configModelName), "unsupported_model", http.StatusBadRequest)
	}
	if endpoint := endpointTypeFromPath(c.Request.URL.Path); endpoint != "" && endpoint != cfg.EndpointType {
		return service.TaskErrorWrapperLocal(
			fmt.Errorf("%s must be used with %s endpoint", cfg.ModelName, cfg.EndpointType),
			"invalid_endpoint",
			http.StatusBadRequest,
		)
	}
	if cfg.ExecutionProtocol == "seedance_official" {
		var raw map[string]any
		if err := common.UnmarshalBodyReusable(c, &raw); err != nil {
			return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
		}
		payload, errorCode, err := normalizeAndValidateSeedanceOfficialPayload(raw)
		if err != nil {
			return service.TaskErrorWrapperLocal(err, errorCode, http.StatusBadRequest)
		}
		c.Set(seedanceOfficialPayloadContextKey, payload)
	}
	if cfg.BillingType == constant.AIPDDBillingTypeDurationSeconds && cfg.SeedancePricing == nil {
		duration, err := normalizeDurationSeconds(&req, cfg)
		if err != nil {
			return service.TaskErrorWrapperLocal(err, "invalid_duration", http.StatusBadRequest)
		}
		req.Duration = duration
	}
	if err := normalizeLtxRequestDuration(&req, cfg); err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_duration", http.StatusBadRequest)
	}
	if isLtx23Config(cfg) {
		if _, err := buildWorkflowContent(req, cfg); err != nil {
			return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
		}
	}

	info.Action = constant.TaskActionGenerate
	c.Set("task_request", req)
	return nil
}

func (a *TaskAdaptor) EstimateBilling(c *gin.Context, info *relaycommon.RelayInfo) map[string]float64 {
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return nil
	}
	cfg, ok := a.resolveModelConfig(ginRequestContext(c), firstNonEmpty(info.UpstreamModelName, info.OriginModelName, req.Model))
	if !ok {
		return nil
	}
	ratios := map[string]float64{}
	if isSeedanceExecutionConfig(cfg) {
		raw, err := getSeedanceOfficialPayload(c)
		if err != nil {
			return nil
		}
		ratios["seconds"] = seedanceBillingSeconds(raw)
		if seedanceHasReferenceVideo(raw["content"]) {
			ratios["has_reference_video"] = 1
		}
		return ratios
	}

	if cfg.BillingType == constant.AIPDDBillingTypeDurationSeconds {
		duration, err := normalizeDurationSeconds(&req, cfg)
		if err != nil {
			return nil
		}
		ratios["seconds"] = float64(duration)
	}

	if cfg.EndpointType == constant.EndpointTypeImageGeneration {
		if count := taskSubmitReqCount(req); count > 1 {
			ratios["n"] = float64(count)
		}
	}
	if len(ratios) == 0 {
		return nil
	}
	return ratios
}

func (a *TaskAdaptor) AIPDDTaskSnapshot(info *relaycommon.RelayInfo) *model.AIPDDTaskExecutionSnapshot {
	cfg, ok := constant.GetAIPDDCapability(firstNonEmpty(info.UpstreamModelName, info.OriginModelName))
	if !ok {
		return nil
	}
	snapshot := &model.AIPDDTaskExecutionSnapshot{
		CatalogRevision: cfg.CatalogRevision, Protocol: cfg.ExecutionProtocol,
		Endpoint: cfg.ExecutionPath, BaseURL: a.baseURL,
	}
	if ratios := info.PriceData.OtherRatios; ratios != nil {
		snapshot.BillingSeconds = ratios["seconds"]
		snapshot.HasReferenceVideo = ratios["has_reference_video"] > 0
	}
	return snapshot
}

func (a *TaskAdaptor) BuildRequestURL(info *relaycommon.RelayInfo) (string, error) {
	cfg, ok := constant.GetAIPDDCapability(firstNonEmpty(info.UpstreamModelName, info.OriginModelName))
	if !ok || strings.TrimSpace(cfg.ExecutionPath) == "" {
		return "", fmt.Errorf("AIPDD execution endpoint is unavailable for %s", info.OriginModelName)
	}
	return a.baseURL + normalizeExecutionPath(cfg.ExecutionPath), nil
}

func (a *TaskAdaptor) BuildRequestHeader(_ *gin.Context, req *http.Request, _ *relaycommon.RelayInfo) error {
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", a.apiKey)
	req.Header.Set("Authorization", "Bearer "+a.apiKey)
	return nil
}

func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return nil, err
	}
	normalizeTaskSubmitReq(&req)
	c.Set("task_request", req)
	cfg, ok := constant.GetAIPDDCapability(firstNonEmpty(info.UpstreamModelName, info.OriginModelName, req.Model))
	if ok && cfg.ExecutionProtocol == "seedance_official" {
		canonical, err := getSeedanceOfficialPayload(c)
		if err != nil {
			return nil, err
		}
		raw := cloneAnyMap(canonical)
		if info.IsModelMapped {
			raw["model"] = info.UpstreamModelName
		}
		data, err := common.Marshal(raw)
		if err != nil {
			return nil, err
		}
		return bytes.NewReader(data), nil
	}

	payload, err := a.convertToRequestPayload(req, info)
	if err != nil {
		return nil, err
	}
	data, err := common.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(data), nil
}

func (a *TaskAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	return channel.DoTaskApiRequest(a, c, info, requestBody)
}

func (a *TaskAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (taskID string, taskData []byte, taskErr *dto.TaskError) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", nil, service.TaskErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
	}
	_ = resp.Body.Close()

	cfg, _ := constant.GetAIPDDCapability(firstNonEmpty(info.UpstreamModelName, info.OriginModelName))
	if cfg.ExecutionProtocol == "seedance_official" {
		var official seedanceOfficialTaskResponse
		if err := common.Unmarshal(responseBody, &official); err != nil {
			return "", nil, service.TaskErrorWrapper(errors.Wrapf(err, "body: %s", responseBody), "unmarshal_response_body_failed", http.StatusInternalServerError)
		}
		if strings.TrimSpace(official.ID) == "" {
			message, _ := seedanceOfficialErrorDetails(official)
			message = firstNonEmpty(message, "Seedance task creation failed")
			statusCode := http.StatusBadGateway
			businessCode := positiveIntValue(official.Code)
			if businessCode == 0 {
				_, errorCode := seedanceOfficialErrorDetails(official)
				businessCode = positiveIntValue(errorCode)
			}
			if businessCode >= http.StatusBadRequest && businessCode < http.StatusInternalServerError {
				statusCode = businessCode
			}
			return "", nil, service.TaskErrorWrapper(fmt.Errorf("%s", message), "seedance_task_create_failed", statusCode)
		}
		writeCreateTaskResponse(c, info, cfg)
		return official.ID, responseBody, nil
	}

	var aipddResp createTaskResponse
	if err := common.Unmarshal(responseBody, &aipddResp); err != nil {
		return "", nil, service.TaskErrorWrapper(errors.Wrapf(err, "body: %s", responseBody), "unmarshal_response_body_failed", http.StatusInternalServerError)
	}
	if aipddResp.Code != 0 && aipddResp.Code != http.StatusOK {
		return "", nil, service.TaskErrorWrapper(fmt.Errorf("%s", aipddResp.Message), "aipdd_task_create_failed", http.StatusBadGateway)
	}
	upstreamTaskID := firstNonEmpty(aipddResp.Data.ID, aipddResp.Data.TaskID)
	if strings.TrimSpace(upstreamTaskID) == "" {
		return "", nil, service.TaskErrorWrapper(fmt.Errorf("task_id is empty"), "invalid_response", http.StatusInternalServerError)
	}

	writeCreateTaskResponse(c, info, cfg)

	return upstreamTaskID, responseBody, nil
}

func writeCreateTaskResponse(c *gin.Context, info *relaycommon.RelayInfo, cfg modelConfig) {
	now := time.Now().Unix()
	if cfg.EndpointType == constant.EndpointTypeOpenAIVideo {
		ov := dto.NewOpenAIVideo()
		ov.ID = info.PublicTaskID
		ov.TaskID = info.PublicTaskID
		ov.CreatedAt = now
		ov.Model = info.OriginModelName
		c.JSON(http.StatusOK, ov)
		return
	}

	object := "task"
	switch cfg.EndpointType {
	case constant.EndpointTypeImageGeneration:
		object = "image.generation.task"
	case constant.EndpointTypeAudioSpeech:
		object = "audio.speech.task"
	}
	c.JSON(http.StatusOK, gin.H{
		"id":      info.PublicTaskID,
		"task_id": info.PublicTaskID,
		"object":  object,
		"created": now,
		"model":   info.OriginModelName,
		"status":  "queued",
		"metadata": gin.H{
			"endpoint_type": cfg.EndpointType,
		},
	})
}

func (a *TaskAdaptor) FetchTask(baseURL, key string, body map[string]any, proxy string) (*http.Response, error) {
	taskID, ok := body["task_id"].(string)
	if !ok || strings.TrimSpace(taskID) == "" {
		return nil, fmt.Errorf("invalid task_id")
	}

	client, err := service.GetHttpClientWithProxy(proxy)
	if err != nil {
		return nil, fmt.Errorf("new proxy http client failed: %w", err)
	}
	if client == nil {
		client = http.DefaultClient
	}

	protocol, _ := body["execution_protocol"].(string)
	endpoint, _ := body["execution_endpoint"].(string)
	if protocol == "seedance_official" {
		if strings.TrimSpace(endpoint) == "" {
			endpoint = "/api/v3/contents/generations/tasks"
		}
		uri := strings.TrimRight(baseURL, "/") + normalizeExecutionPath(endpoint) + "/" + url.PathEscape(taskID)
		return aipddOfficialGet(client, uri, key)
	}
	detailURI := fmt.Sprintf("%s/shared-tasks/tasks/%s/detail", strings.TrimRight(baseURL, "/"), url.PathEscape(taskID))
	detailResp, detailBody, err := aipddGet(client, detailURI, key)
	if err != nil {
		return nil, err
	}
	if detailResp.StatusCode < http.StatusOK || detailResp.StatusCode >= http.StatusMultipleChoices {
		return responseWithBody(detailResp, detailBody), nil
	}

	shouldFetchResult, resultTaskID := shouldFetchAIPDDResult(detailBody)
	if !shouldFetchResult {
		return responseWithBody(detailResp, detailBody), nil
	}
	if resultTaskID == "" {
		resultTaskID = taskID
	}

	resultURI := fmt.Sprintf("%s/shared-tasks/tasks/%s/result", strings.TrimRight(baseURL, "/"), url.PathEscape(resultTaskID))
	resultResp, resultBody, err := aipddGet(client, resultURI, key)
	if err != nil {
		return nil, err
	}
	if resultResp.StatusCode < http.StatusOK || resultResp.StatusCode >= http.StatusMultipleChoices {
		return responseWithBody(resultResp, resultBody), nil
	}
	return responseWithBody(detailResp, mergeAIPDDResultBody(detailBody, resultBody)), nil
}

func aipddOfficialGet(client *http.Client, uri, key string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, uri, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-API-Key", key)
	req.Header.Set("Authorization", "Bearer "+key)
	return client.Do(req)
}

func aipddGet(client *http.Client, uri, key string) (*http.Response, []byte, error) {
	req, err := http.NewRequest(http.MethodGet, uri, nil)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-API-Key", key)

	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, err
	}
	return resp, body, nil
}

func responseWithBody(resp *http.Response, body []byte) *http.Response {
	if resp == nil {
		resp = &http.Response{StatusCode: http.StatusOK}
	}
	resp.Body = io.NopCloser(bytes.NewReader(body))
	resp.ContentLength = int64(len(body))
	if resp.Header == nil {
		resp.Header = make(http.Header)
	}
	resp.Header.Set("Content-Type", "application/json")
	return resp
}

func shouldFetchAIPDDResult(detailBody []byte) (bool, string) {
	var detail taskDetailResponse
	if err := common.Unmarshal(detailBody, &detail); err != nil || detail.Data == nil {
		return false, ""
	}
	if detail.Code != 0 && detail.Code != http.StatusOK {
		return false, ""
	}
	task := detail.Data
	status := strings.ToUpper(strings.TrimSpace(task.Status))
	if status == "SUCCESS" && task.ResultReady {
		return true, firstNonEmpty(task.ID, task.TaskID)
	}
	return false, ""
}

func mergeAIPDDResultBody(detailBody, resultBody []byte) []byte {
	var detail map[string]any
	var result map[string]any
	if err := common.Unmarshal(detailBody, &detail); err != nil {
		return resultBody
	}
	if err := common.Unmarshal(resultBody, &result); err != nil {
		return detailBody
	}

	detailData, ok := mapValue(detail["data"])
	if !ok {
		return resultBody
	}
	resultData, ok := mapValue(result["data"])
	if !ok {
		return detailBody
	}

	if taskID := firstNonEmpty(anyToString(resultData["taskId"]), anyToString(detailData["id"]), anyToString(detailData["taskId"])); taskID != "" {
		if anyToString(detailData["id"]) == "" {
			detailData["id"] = taskID
		}
		detailData["taskId"] = taskID
	}
	if status := anyToString(resultData["status"]); status != "" {
		detailData["resultStatus"] = status
	}
	for _, key := range []string{"output", "objectRefs", "downloadRefs", "checksum", "validatedAt", "updatedAt"} {
		if value, ok := resultData[key]; ok {
			detailData[key] = value
		}
	}
	detailData["task_result"] = resultData
	detail["data"] = detailData

	merged, err := common.Marshal(detail)
	if err != nil {
		return detailBody
	}
	return merged
}

func mapValue(value any) (map[string]any, bool) {
	typed, ok := value.(map[string]any)
	return typed, ok
}

func (a *TaskAdaptor) ParseTaskResult(respBody []byte) (*relaycommon.TaskInfo, error) {
	var official seedanceOfficialTaskResponse
	if err := common.Unmarshal(respBody, &official); err == nil && strings.TrimSpace(official.Status) != "" {
		info := &relaycommon.TaskInfo{Code: 0, TaskID: official.ID}
		switch strings.ToLower(strings.TrimSpace(official.Status)) {
		case "pending", "queued":
			info.Status, info.Progress = model.TaskStatusQueued, taskcommon.ProgressQueued
		case "processing", "running":
			info.Status, info.Progress = model.TaskStatusInProgress, taskcommon.ProgressInProgress
		case "succeeded", "completed":
			info.Status, info.Progress, info.Url = model.TaskStatusSuccess, taskcommon.ProgressComplete, official.Content.VideoURL
		case "failed", "cancelled", "canceled":
			message, code := seedanceOfficialErrorDetails(official)
			info.Status, info.Progress, info.Reason = model.TaskStatusFailure, taskcommon.ProgressComplete, firstNonEmpty(message, "Seedance task failed")
			info.Code = positiveIntValue(code)
		default:
			info.Status, info.Progress = model.TaskStatusInProgress, taskcommon.ProgressInProgress
		}
		return info, nil
	}
	var aipddResp taskDetailResponse
	if err := common.Unmarshal(respBody, &aipddResp); err != nil {
		return nil, errors.Wrap(err, "unmarshal aipdd task result failed")
	}

	if aipddResp.Code != 0 && aipddResp.Code != http.StatusOK {
		return &relaycommon.TaskInfo{
			Code:   aipddResp.Code,
			Status: string(model.TaskStatusFailure),
			Reason: strings.TrimSpace(aipddResp.Message),
		}, nil
	}
	if aipddResp.Data == nil {
		return nil, fmt.Errorf("aipdd task data is empty")
	}

	task := aipddResp.Data
	resultPayload := aipddResultPayload(task)
	taskInfo := &relaycommon.TaskInfo{
		Code:   0,
		TaskID: firstNonEmpty(task.ID, task.TaskID),
	}

	if reason, failed := failedTaskResultReason(resultPayload); failed {
		taskInfo.Status = model.TaskStatusFailure
		taskInfo.Progress = taskcommon.ProgressComplete
		taskInfo.Reason = reason
		return taskInfo, nil
	}

	resultURLs := extractResultURLs(resultPayload)
	if len(resultURLs) > 0 {
		taskInfo.Status = model.TaskStatusSuccess
		taskInfo.Progress = taskcommon.ProgressComplete
		taskInfo.Url = resultURLs[0]
		return taskInfo, nil
	}

	if strings.TrimSpace(task.Status) != "" {
		return parseAIPDDComputeTaskStatus(task, taskInfo), nil
	}

	switch task.TaskStatus {
	case 0:
		taskInfo.Status = model.TaskStatusSubmitted
		taskInfo.Progress = taskcommon.ProgressSubmitted
	case 1:
		taskInfo.Status = model.TaskStatusInProgress
		taskInfo.Progress = taskcommon.ProgressInProgress
	case 2:
		taskInfo.Status = model.TaskStatusFailure
		taskInfo.Progress = taskcommon.ProgressComplete
		taskInfo.Reason = taskResultText(task.TaskResult)
		if taskInfo.Reason == "" {
			taskInfo.Reason = "AIPDD task succeeded without result URL"
		}
	case 3:
		taskInfo.Status = model.TaskStatusFailure
		taskInfo.Progress = taskcommon.ProgressComplete
		taskInfo.Reason = taskResultText(task.TaskResult)
		if taskInfo.Reason == "" {
			taskInfo.Reason = "AIPDD task failed"
		}
	case 4:
		taskInfo.Status = model.TaskStatusFailure
		taskInfo.Progress = taskcommon.ProgressComplete
		taskInfo.Reason = taskResultText(task.TaskResult)
		if taskInfo.Reason == "" {
			taskInfo.Reason = "AIPDD task succeeded but result transfer failed"
		}
	default:
		taskInfo.Status = model.TaskStatusInProgress
		taskInfo.Progress = taskcommon.ProgressInProgress
	}

	return taskInfo, nil
}

func parseAIPDDComputeTaskStatus(task *aipddTask, taskInfo *relaycommon.TaskInfo) *relaycommon.TaskInfo {
	status := strings.ToUpper(strings.TrimSpace(task.Status))
	if task.Progress > 0 {
		taskInfo.Progress = strconv.Itoa(task.Progress)
	}

	switch status {
	case "QUEUED", "RETRY_WAIT":
		taskInfo.Status = model.TaskStatusSubmitted
		if taskInfo.Progress == "" {
			taskInfo.Progress = taskcommon.ProgressSubmitted
		}
	case "ASSIGNED", "RUNNING":
		taskInfo.Status = model.TaskStatusInProgress
		if taskInfo.Progress == "" {
			taskInfo.Progress = taskcommon.ProgressInProgress
		}
	case "SUCCESS":
		if task.ResultReady {
			taskInfo.Status = model.TaskStatusFailure
			taskInfo.Progress = taskcommon.ProgressComplete
			taskInfo.Reason = "AIPDD task succeeded without result URL"
		} else {
			taskInfo.Status = model.TaskStatusInProgress
			if taskInfo.Progress == "" {
				taskInfo.Progress = taskcommon.ProgressInProgress
			}
		}
	case "FAILED", "CANCELED", "EXPIRED":
		taskInfo.Status = model.TaskStatusFailure
		taskInfo.Progress = taskcommon.ProgressComplete
		taskInfo.Reason = firstNonEmpty(strings.TrimSpace(task.Message), strings.TrimSpace(task.Stage), "AIPDD task failed")
	default:
		taskInfo.Status = model.TaskStatusInProgress
		if taskInfo.Progress == "" {
			taskInfo.Progress = taskcommon.ProgressInProgress
		}
	}
	return taskInfo
}

func aipddResultPayload(task *aipddTask) any {
	if task == nil {
		return nil
	}
	if task.TaskResult != nil {
		return task.TaskResult
	}

	payload := map[string]any{}
	if task.Output != nil {
		payload["output"] = task.Output
	}
	if task.ObjectRefs != nil {
		payload["objectRefs"] = task.ObjectRefs
	}
	if task.DownloadRefs != nil {
		payload["downloadRefs"] = task.DownloadRefs
	}
	if task.Checksum != "" {
		payload["checksum"] = task.Checksum
	}
	if task.ResultStatus != "" {
		payload["status"] = task.ResultStatus
	}
	if len(payload) == 0 {
		return nil
	}
	return payload
}

func (a *TaskAdaptor) GetModelList() []string {
	return constant.GetAIPDDTaskModelList()
}

func (a *TaskAdaptor) GetChannelName() string {
	return ChannelName
}

func (a *TaskAdaptor) ConvertToOpenAIVideo(originTask *model.Task) ([]byte, error) {
	openAIVideo := originTask.ToOpenAIVideo()
	openAIVideo.TaskID = originTask.TaskID

	// Official Seedance responses are not wrapped in data.  The realtime
	// fetcher stores the latest response in originTask.Data, so parse this shape
	// before falling back to the legacy AIPDD response format.  Without this,
	// upstream error.message is discarded and clients only see a generic failure.
	var official seedanceOfficialTaskResponse
	if err := common.Unmarshal(originTask.Data, &official); err == nil && strings.TrimSpace(official.Status) != "" {
		if strings.TrimSpace(official.Content.VideoURL) != "" {
			openAIVideo.SetMetadata("url", official.Content.VideoURL)
		}
		if isSeedanceOfficialFailure(official.Status) || originTask.Status == model.TaskStatusFailure {
			message, code := seedanceOfficialErrorDetails(official)
			openAIVideo.Error = &dto.OpenAIVideoError{
				Message: firstNonEmpty(message, originTask.FailReason, "Seedance task failed"),
				Code:    firstNonEmpty(code, "seedance_task_failed"),
			}
		}
		return common.Marshal(openAIVideo)
	}

	var detail taskDetailResponse
	if err := common.Unmarshal(originTask.Data, &detail); err == nil && detail.Data != nil {
		urls := extractResultURLs(aipddResultPayload(detail.Data))
		if len(urls) > 1 {
			openAIVideo.SetMetadata("urls", urls)
		}
		if originTask.Status == model.TaskStatusFailure {
			openAIVideo.Error = &dto.OpenAIVideoError{
				Message: firstNonEmpty(taskResultText(aipddResultPayload(detail.Data)), detail.Data.Message, originTask.FailReason, "AIPDD task failed"),
				Code:    "aipdd_task_failed",
			}
		}
	} else if originTask.Status == model.TaskStatusFailure {
		openAIVideo.Error = &dto.OpenAIVideoError{
			Message: firstNonEmpty(originTask.FailReason, "AIPDD task failed"),
			Code:    "aipdd_task_failed",
		}
	}

	return common.Marshal(openAIVideo)
}

func isSeedanceOfficialFailure(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "failed", "cancelled", "canceled":
		return true
	default:
		return false
	}
}

func seedanceOfficialErrorDetails(response seedanceOfficialTaskResponse) (string, string) {
	code, message := seedanceOfficialErrorValue(response.Error)
	if strings.TrimSpace(message) == "" {
		message = strings.TrimSpace(response.Message)
	}
	if strings.TrimSpace(code) == "" {
		code = anyToString(response.Code)
	}
	return strings.TrimSpace(message), strings.TrimSpace(code)
}

func seedanceOfficialErrorValue(value any) (string, string) {
	switch typed := value.(type) {
	case string:
		text := strings.TrimSpace(typed)
		if text == "" {
			return "", ""
		}
		var parsed any
		if err := common.Unmarshal([]byte(text), &parsed); err == nil {
			if _, ok := parsed.(map[string]any); ok {
				return seedanceOfficialErrorValue(parsed)
			}
		}
		return "", text
	case map[string]any:
		code := firstNonEmpty(anyToString(typed["code"]), anyToString(typed["error_code"]))
		message := firstNonEmpty(anyToString(typed["message"]), anyToString(typed["reason"]), anyToString(typed["detail"]))
		if message == "" {
			if nested, ok := typed["error"]; ok {
				nestedCode, nestedMessage := seedanceOfficialErrorValue(nested)
				code = firstNonEmpty(code, nestedCode)
				message = nestedMessage
			}
		}
		return code, message
	default:
		return "", ""
	}
}

func (a *TaskAdaptor) convertToRequestPayload(req relaycommon.TaskSubmitReq, info *relaycommon.RelayInfo) (*createTaskPayload, error) {
	normalizeTaskSubmitReq(&req)
	cfg, err := a.resolveRequestModelConfig(context.Background(), req, info)
	if err != nil {
		return nil, err
	}
	if cfg.BillingType == constant.AIPDDBillingTypeDurationSeconds {
		duration, err := normalizeDurationSeconds(&req, cfg)
		if err != nil {
			return nil, err
		}
		req.Duration = duration
	}
	if err := normalizeLtxRequestDuration(&req, cfg); err != nil {
		return nil, err
	}

	content, err := buildWorkflowContent(req, cfg)
	if err != nil {
		return nil, err
	}

	taskName := metadataString(req.Metadata, "task_name")
	if strings.TrimSpace(taskName) == "" {
		taskName = fmt.Sprintf("%s:%s", defaultTaskNamePrefix, cfg.ModelName)
	}
	requestID := ""
	if info != nil {
		requestID = strings.TrimSpace(info.PublicTaskID)
	}
	if requestID == "" {
		requestID = metadataString(req.Metadata, "request_id")
	}

	return &createTaskPayload{
		RequestID:    requestID,
		TaskName:     taskName,
		TaskTypeCode: cfg.ScriptCode,
		Input:        content,
	}, nil
}

func buildWorkflowContent(req relaycommon.TaskSubmitReq, cfg modelConfig) (map[string]any, error) {
	content := cloneWorkflowMetadata(req.Metadata)
	explicitNumFrames := hasContentValue(content["numFrames"])
	explicitDurationSeconds := hasContentValue(content["durationSeconds"])
	explicitLength := hasContentValue(content["length"])
	applyModelDefaults(content, req, cfg)
	// The catalog normally describes this mapping in WorkflowDefaults. Keep a
	// direct fallback for prompt because it is the primary user input and older
	// or partially refreshed catalogs may omit the prompt parameter entirely.
	// AIPDD workflows can still require this field even when the catalog does
	// not advertise it, so do not gate this fallback on workflowParamAllowed.
	if !hasContentValue(content["prompt"]) {
		if prompt := strings.TrimSpace(req.Prompt); prompt != "" {
			content["prompt"] = prompt
		}
	}
	if isLtx23StartEndConfig(cfg) {
		if err := normalizeLtxStartEndContent(content, req, explicitLength, explicitNumFrames); err != nil {
			return nil, err
		}
	} else if isLtx23StandardConfig(cfg) {
		if err := normalizeAndValidateLtx23Content(content, req.Duration, explicitNumFrames, explicitDurationSeconds); err != nil {
			return nil, err
		}
	}
	if err := normalizeWorkflowTimelineData(content, cfg); err != nil {
		return nil, err
	}
	if err := validateTaskContent(content, cfg); err != nil {
		return nil, err
	}
	return content, nil
}

func resolveRequestModelConfig(req relaycommon.TaskSubmitReq, info *relaycommon.RelayInfo) (modelConfig, error) {
	modelName := ""
	if info != nil {
		modelName = strings.TrimSpace(info.UpstreamModelName)
	}
	if modelName == "" {
		modelName = strings.TrimSpace(req.Model)
	}
	cfg, ok := resolveModelConfig(modelName)
	if !ok {
		return modelConfig{}, fmt.Errorf("unsupported AIPDD model: %s", modelName)
	}
	return cfg, nil
}

func (a *TaskAdaptor) resolveRequestModelConfig(ctx context.Context, req relaycommon.TaskSubmitReq, info *relaycommon.RelayInfo) (modelConfig, error) {
	modelName := ""
	if info != nil {
		modelName = strings.TrimSpace(info.UpstreamModelName)
	}
	if modelName == "" {
		modelName = strings.TrimSpace(req.Model)
	}
	cfg, ok := a.resolveModelConfig(ctx, modelName)
	if !ok {
		return modelConfig{}, fmt.Errorf("unsupported AIPDD model: %s", modelName)
	}
	return cfg, nil
}

func resolveModelConfig(modelName string) (modelConfig, bool) {
	return constant.GetAIPDDCapability(modelName)
}

func ginRequestContext(c *gin.Context) context.Context {
	if c != nil && c.Request != nil {
		return c.Request.Context()
	}
	return context.Background()
}

func (a *TaskAdaptor) resolveModelConfig(_ context.Context, modelName string) (modelConfig, bool) {
	return resolveModelConfig(modelName)
}

func normalizeExecutionPath(path string) string {
	path = strings.TrimSpace(path)
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return strings.TrimRight(path, "/")
}

func seedanceHasReferenceVideo(content any) bool {
	items, ok := content.([]any)
	if !ok {
		return false
	}
	for _, value := range items {
		item, ok := value.(map[string]any)
		if !ok {
			continue
		}
		typeName := strings.TrimSpace(anyToString(item["type"]))
		role := strings.TrimSpace(anyToString(item["role"]))
		if strings.EqualFold(typeName, "video_url") ||
			strings.EqualFold(typeName, "video") ||
			strings.EqualFold(role, "reference_video") {
			return true
		}
		if validSeedanceVideoURL(item["video_url"]) {
			return true
		}
	}
	return false
}

func validSeedanceVideoURL(value any) bool {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed) != ""
	case map[string]any:
		return strings.TrimSpace(anyToString(typed["url"])) != ""
	default:
		return false
	}
}

func normalizeAndValidateSeedanceOfficialPayload(raw map[string]any) (map[string]any, string, error) {
	if raw == nil {
		return nil, "invalid_request", fmt.Errorf("request body is required")
	}
	payload := cloneAnyMap(raw)
	metadata, _ := payload["metadata"].(map[string]any)
	for _, key := range []string{
		"content", "resolution", "ratio", "duration", "frames", "fps", "framespersecond",
		"frames_per_second", "seed", "callback_url", "return_last_frame", "service_tier",
		"generate_audio", "priority",
	} {
		if seedanceRequestValuePresent(payload[key]) {
			continue
		}
		if value := metadata[key]; seedanceRequestValuePresent(value) {
			payload[key] = value
		}
	}

	content, contentPresent := payload["content"].([]any)
	if !contentPresent || len(content) == 0 {
		if seedanceRequestValuePresent(payload["content"]) && !contentPresent {
			return nil, "invalid_content", fmt.Errorf("content must be a non-empty array")
		}
		prompt := strings.TrimSpace(anyToString(payload["prompt"]))
		if prompt == "" {
			return nil, "missing_content", fmt.Errorf("content is required when prompt is empty")
		}
		content = []any{map[string]any{"type": "text", "text": prompt}}
		payload["content"] = content
	}
	for index, value := range content {
		item, ok := value.(map[string]any)
		if !ok || strings.TrimSpace(anyToString(item["type"])) == "" {
			return nil, "invalid_content", fmt.Errorf("content[%d] must be an object with a non-empty type", index)
		}
	}

	resolution := canonicalSeedanceResolution(payload["resolution"])
	ratio := strings.TrimSpace(anyToString(payload["ratio"]))
	if resolution == "" || ratio == "" {
		width, height, found, err := seedanceRequestDimensions(payload, metadata)
		if err != nil {
			return nil, "invalid_dimensions", err
		}
		if found {
			if resolution == "" {
				resolution, err = inferSeedanceResolution(width, height)
				if err != nil {
					return nil, "invalid_dimensions", err
				}
			}
			if ratio == "" {
				ratio, err = inferSeedanceRatio(width, height)
				if err != nil {
					return nil, "invalid_dimensions", err
				}
			}
		}
	}
	if resolution == "" {
		return nil, "missing_resolution", fmt.Errorf("resolution is required when width and height are absent")
	}
	if ratio != "" && !isSupportedSeedanceRatio(ratio) {
		return nil, "unsupported_ratio", fmt.Errorf("Seedance ratio %q is not supported", ratio)
	}
	payload["resolution"] = resolution
	if ratio != "" {
		payload["ratio"] = ratio
	}
	return payload, "", nil
}

func getSeedanceOfficialPayload(c *gin.Context) (map[string]any, error) {
	value, exists := c.Get(seedanceOfficialPayloadContextKey)
	if !exists {
		return nil, fmt.Errorf("normalized Seedance request not found in context")
	}
	payload, ok := value.(map[string]any)
	if !ok || payload == nil {
		return nil, fmt.Errorf("invalid normalized Seedance request")
	}
	return payload, nil
}

func cloneAnyMap(source map[string]any) map[string]any {
	cloned := make(map[string]any, len(source))
	for key, value := range source {
		cloned[key] = value
	}
	return cloned
}

func canonicalSeedanceResolution(value any) string {
	return strings.ToLower(strings.TrimSpace(anyToString(value)))
}

func isSeedanceExecutionConfig(cfg modelConfig) bool {
	return strings.EqualFold(strings.TrimSpace(cfg.AdapterCode), "seedance") ||
		strings.EqualFold(strings.TrimSpace(cfg.ExecutionProtocol), "seedance_official") ||
		cfg.SeedancePricing != nil
}

func seedanceRequestDimensions(payload, metadata map[string]any) (int, int, bool, error) {
	for _, source := range []map[string]any{payload, metadata} {
		if source == nil {
			continue
		}
		widthPresent := seedanceRequestValuePresent(source["width"])
		heightPresent := seedanceRequestValuePresent(source["height"])
		if !widthPresent && !heightPresent {
			continue
		}
		if !widthPresent || !heightPresent {
			return 0, 0, false, fmt.Errorf("width and height must be provided together")
		}
		width := positiveIntValue(source["width"])
		height := positiveIntValue(source["height"])
		if width <= 0 || height <= 0 {
			return 0, 0, false, fmt.Errorf("width and height must be positive integers")
		}
		return width, height, true, nil
	}
	return 0, 0, false, nil
}

func inferSeedanceResolution(width, height int) (string, error) {
	shortEdge := min(width, height)
	switch shortEdge {
	case 720:
		return "720p", nil
	case 1080:
		return "1080p", nil
	case 2160:
		return "4k", nil
	default:
		return "", fmt.Errorf("Seedance dimensions %dx%d do not map to 720p, 1080p, or 4k", width, height)
	}
}

func inferSeedanceRatio(width, height int) (string, error) {
	divisor := greatestCommonDivisor(width, height)
	ratio := fmt.Sprintf("%d:%d", width/divisor, height/divisor)
	if !isSupportedSeedanceRatio(ratio) {
		return "", fmt.Errorf("Seedance dimensions %dx%d do not map to a supported ratio", width, height)
	}
	return ratio, nil
}

func greatestCommonDivisor(a, b int) int {
	for b != 0 {
		a, b = b, a%b
	}
	return a
}

func isSupportedSeedanceRatio(ratio string) bool {
	switch ratio {
	case "16:9", "9:16", "1:1", "4:3", "3:4":
		return true
	default:
		return false
	}
}

func seedanceRequestValuePresent(value any) bool {
	switch typed := value.(type) {
	case nil:
		return false
	case string:
		return strings.TrimSpace(typed) != ""
	case []any:
		return len(typed) > 0
	default:
		return true
	}
}

func seedanceBillingSeconds(raw map[string]any) float64 {
	if duration := positiveFloat(raw["duration"]); duration > 0 {
		return duration
	}
	if frames := positiveFloat(raw["frames"]); frames > 0 {
		fps := positiveFloat(raw["fps"])
		if fps <= 0 {
			fps = positiveFloat(raw["framespersecond"])
		}
		if fps <= 0 {
			fps = positiveFloat(raw["frames_per_second"])
		}
		if fps <= 0 {
			fps = 24
		}
		return frames / fps
	}
	return 5
}

func positiveFloat(value any) float64 {
	switch typed := value.(type) {
	case float64:
		if typed > 0 {
			return typed
		}
	case float32:
		if typed > 0 {
			return float64(typed)
		}
	case int:
		if typed > 0 {
			return float64(typed)
		}
	case int64:
		if typed > 0 {
			return float64(typed)
		}
	case string:
		if parsed, err := strconv.ParseFloat(strings.TrimSpace(typed), 64); err == nil && parsed > 0 {
			return parsed
		}
	}
	return 0
}

func normalizeLtxRequestDuration(req *relaycommon.TaskSubmitReq, cfg modelConfig) error {
	if req == nil || !isLtx23Config(cfg) {
		return nil
	}
	if req.Duration <= 0 {
		req.Duration = parseDurationValue(req.Seconds)
	}
	if req.Duration <= 0 {
		req.Duration = parseDurationValue(metadataString(req.Metadata, "duration"))
	}
	if req.Duration <= 0 {
		req.Duration = parseDurationValue(metadataString(req.Metadata, "seconds"))
	}
	if req.Duration > 0 && (req.Duration < 1 || req.Duration > 20) {
		return fmt.Errorf("LTX 2.3 duration must be an integer between 1 and 20 seconds")
	}
	return nil
}

func endpointTypeFromPath(path string) constant.EndpointType {
	switch {
	case strings.HasPrefix(path, "/v1/images/generations"), strings.HasPrefix(path, "/pg/images/generations"):
		return constant.EndpointTypeImageGeneration
	case strings.HasPrefix(path, "/v1/audio/speech"), strings.HasPrefix(path, "/pg/audio/speech"):
		return constant.EndpointTypeAudioSpeech
	case strings.HasPrefix(path, "/v1/videos"), strings.HasPrefix(path, "/v1/video/generations"), strings.HasPrefix(path, "/pg/videos"), strings.HasPrefix(path, "/pg/video/generations"):
		return constant.EndpointTypeOpenAIVideo
	default:
		return ""
	}
}

func normalizeDurationSeconds(req *relaycommon.TaskSubmitReq, cfg modelConfig) (int, error) {
	duration := req.Duration
	if duration <= 0 {
		duration = parseDurationValue(req.Seconds)
	}
	if duration <= 0 {
		duration = parseDurationValue(metadataString(req.Metadata, "duration"))
	}
	if duration <= 0 {
		duration = parseDurationValue(metadataString(req.Metadata, "seconds"))
	}
	if duration <= 0 {
		duration = 5
	}
	if isLtx23Config(cfg) {
		if duration < 1 || duration > 20 {
			return 0, fmt.Errorf("LTX 2.3 duration must be an integer between 1 and 20 seconds")
		}
		return duration, nil
	}
	if duration != 5 && duration != 10 {
		return 0, fmt.Errorf("duration must be 5 or 10 seconds")
	}
	return duration, nil
}

func parseDurationValue(value string) int {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	if i, err := strconv.Atoi(value); err == nil {
		return i
	}
	if f, err := strconv.ParseFloat(value, 64); err == nil {
		i := int(f)
		if f == float64(i) {
			return i
		}
	}
	return 0
}

func applyModelDefaults(content map[string]any, req relaycommon.TaskSubmitReq, cfg modelConfig) {
	for _, item := range cfg.WorkflowDefaults {
		if strings.TrimSpace(item.ParamKey) == "" || hasContentValue(content[item.ParamKey]) {
			continue
		}
		switch item.ValueType {
		case constant.AIPDDWorkflowValueTypeInt:
			if value, ok := resolveWorkflowDefaultInt(content, req, item.Sources); ok {
				setContentInt(content, item.ParamKey, value)
			}
		default:
			if value := resolveWorkflowDefaultString(content, req, item.Sources); value != "" {
				setContentString(content, item.ParamKey, value)
			}
		}
	}
}

func resolveWorkflowDefaultString(content map[string]any, req relaycommon.TaskSubmitReq, sources []constant.AIPDDWorkflowValueSource) string {
	values := make([]string, 0, len(sources))
	for _, source := range sources {
		switch source.Type {
		case constant.AIPDDWorkflowSourceMetadata:
			values = append(values, metadataString(req.Metadata, source.Key))
		case constant.AIPDDWorkflowSourcePrompt:
			values = append(values, req.Prompt)
		case constant.AIPDDWorkflowSourceImage:
			values = append(values, req.Image)
		case constant.AIPDDWorkflowSourceFirstImage:
			values = append(values, req.FirstFrame, firstString(req.Images))
		case constant.AIPDDWorkflowSourceLastImage:
			values = append(values, req.LastFrame, req.ImageTail, secondString(req.Images))
		case constant.AIPDDWorkflowSourceInputReference:
			values = append(values, req.InputReference)
		case constant.AIPDDWorkflowSourceDuration:
			if req.Duration > 0 {
				values = append(values, strconv.Itoa(req.Duration))
			}
		case constant.AIPDDWorkflowSourceStatic:
			values = append(values, source.Key)
		}
	}
	return firstNonEmpty(values...)
}

func resolveWorkflowDefaultInt(content map[string]any, req relaycommon.TaskSubmitReq, sources []constant.AIPDDWorkflowValueSource) (int, bool) {
	for _, source := range sources {
		switch source.Type {
		case constant.AIPDDWorkflowSourceMetadata:
			if value := parseDurationValue(metadataString(req.Metadata, source.Key)); value > 0 {
				return value, true
			}
		case constant.AIPDDWorkflowSourceDuration:
			if req.Duration > 0 {
				return req.Duration, true
			}
		case constant.AIPDDWorkflowSourceStatic:
			if value := parseDurationValue(source.Key); value > 0 {
				return value, true
			}
		default:
			if value := parseDurationValue(resolveWorkflowDefaultString(content, req, []constant.AIPDDWorkflowValueSource{source})); value > 0 {
				return value, true
			}
		}
	}
	return 0, false
}

func validateTaskContent(content map[string]any, cfg modelConfig) error {
	allowed := make(map[string]bool, len(cfg.WorkflowParamKeys))
	for _, key := range cfg.WorkflowParamKeys {
		allowed[key] = true
		if cfg.RequiredWorkflowParams[key] {
			if !hasContentValue(content[key]) {
				return fmt.Errorf("%s is required for %s", key, cfg.ModelName)
			}
			continue
		}
		if !hasContentValue(content[key]) {
			delete(content, key)
		}
	}
	for key := range content {
		if !allowed[key] {
			delete(content, key)
		}
	}
	for _, constraint := range cfg.WorkflowConstraints {
		value, exists := content[constraint.ParamKey]
		if !exists || !hasContentValue(value) {
			continue
		}
		if err := validateWorkflowConstraint(value, constraint); err != nil {
			return err
		}
	}
	return nil
}

func validateWorkflowConstraint(value any, constraint constant.AIPDDWorkflowParamConstraint) error {
	numeric, numericErr := strconv.ParseFloat(anyToString(value), 64)
	if constraint.Min != nil || constraint.Max != nil {
		if numericErr != nil {
			return fmt.Errorf("%s must be numeric", constraint.ParamKey)
		}
		if constraint.Min != nil && numeric < *constraint.Min {
			return fmt.Errorf("%s must be at least %v", constraint.ParamKey, *constraint.Min)
		}
		if constraint.Max != nil && numeric > *constraint.Max {
			return fmt.Errorf("%s must be at most %v", constraint.ParamKey, *constraint.Max)
		}
	}
	if len(constraint.Allowed) > 0 {
		for _, allowed := range constraint.Allowed {
			allowedNumeric, allowedErr := strconv.ParseFloat(anyToString(allowed), 64)
			if numericErr == nil && allowedErr == nil && numeric == allowedNumeric {
				return nil
			}
			if anyToString(value) == anyToString(allowed) {
				return nil
			}
		}
		return fmt.Errorf("%s is not an allowed value", constraint.ParamKey)
	}
	return nil
}

func isLtx23Config(cfg modelConfig) bool {
	return isLtx23StandardConfig(cfg) || isLtx23StartEndConfig(cfg)
}

func isLtx23StandardConfig(cfg modelConfig) bool {
	for _, value := range []string{cfg.ModelName, cfg.ScriptCode} {
		normalized := strings.NewReplacer("_", "-", ".", "-", " ", "-").Replace(strings.ToLower(strings.TrimSpace(value)))
		if normalized == "aipdd-ltx-2-3" && !isLtx23StartEndValue(value) {
			return true
		}
	}
	return false
}

func isLtx23StartEndConfig(cfg modelConfig) bool {
	return isLtx23StartEndValue(cfg.ModelName) || isLtx23StartEndValue(cfg.ScriptCode)
}

func isLtx23StartEndValue(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	if strings.Contains(value, "首尾帧") {
		return true
	}
	return strings.Contains(value, "first") && strings.Contains(value, "last") && strings.Contains(value, "ltx")
}

func normalizeLtxStartEndContent(content map[string]any, req relaycommon.TaskSubmitReq, explicitLength, explicitNumFrames bool) error {
	if !explicitLength && explicitNumFrames {
		numFrames := positiveIntValue(req.Metadata["numFrames"])
		if numFrames <= 0 {
			return fmt.Errorf("numFrames must be a positive integer")
		}
		content["length"] = numFrames
	} else if !explicitLength && req.Duration > 0 {
		content["length"] = req.Duration*24 + 1
	}
	if value, ok := content["length"]; ok && hasContentValue(value) {
		length := positiveIntValue(value)
		if length <= 0 {
			return fmt.Errorf("length must be a positive integer")
		}
		content["length"] = length
		// The upstream first/last-frame workflow validates numFrames as a
		// separate required input even though length is its timeline alias.
		content["numFrames"] = length
	}
	return nil
}

func normalizeWorkflowTimelineData(content map[string]any, cfg modelConfig) error {
	if !workflowParamAllowed(cfg, "timeline_data") {
		return nil
	}
	value, exists := content["timeline_data"]
	if !exists || !hasContentValue(value) {
		return nil
	}
	if raw, ok := value.(string); ok {
		var parsed any
		if err := common.Unmarshal([]byte(strings.TrimSpace(raw)), &parsed); err != nil {
			return fmt.Errorf("timeline_data must be valid JSON: %w", err)
		}
		content["timeline_data"] = parsed
	}
	return nil
}

func workflowParamAllowed(cfg modelConfig, key string) bool {
	for _, paramKey := range cfg.WorkflowParamKeys {
		if paramKey == key {
			return true
		}
	}
	return false
}

func normalizeAndValidateLtx23Content(content map[string]any, duration int, explicitNumFrames bool, explicitDurationSeconds bool) error {
	const frameRate = 24
	if duration <= 0 && explicitDurationSeconds {
		duration = positiveIntValue(content["durationSeconds"])
	}
	if duration > 20 {
		return fmt.Errorf("LTX 2.3 duration must be an integer between 1 and 20 seconds")
	}
	if duration > 0 {
		expectedFrames := duration*frameRate + 1
		if explicitNumFrames && positiveIntValue(content["numFrames"]) != expectedFrames {
			return fmt.Errorf("LTX 2.3 numFrames must be %d for %d seconds", expectedFrames, duration)
		}
		if explicitDurationSeconds && positiveIntValue(content["durationSeconds"]) != duration {
			return fmt.Errorf("LTX 2.3 durationSeconds must match duration")
		}
		content["numFrames"] = expectedFrames
		content["durationSeconds"] = duration
	} else if positiveIntValue(content["numFrames"]) <= 0 {
		return fmt.Errorf("LTX 2.3 numFrames must be a positive integer")
	}

	if hasContentValue(content["frameRate"]) && positiveIntValue(content["frameRate"]) != frameRate {
		return fmt.Errorf("LTX 2.3 frameRate must be 24 FPS")
	}
	content["frameRate"] = frameRate

	width := positiveIntValue(content["width"])
	height := positiveIntValue(content["height"])
	allowedDimensions := [][2]int{{1280, 704}, {704, 1280}, {704, 704}, {640, 640}, {640, 480}, {480, 640}, {480, 480}}
	for _, dimensions := range allowedDimensions {
		if width == dimensions[0] && height == dimensions[1] {
			return nil
		}
	}
	return fmt.Errorf("LTX 2.3 resolution %dx%d is not allowed", width, height)
}

func mergeUnknownFieldsIntoMetadata(c *gin.Context, req *relaycommon.TaskSubmitReq) {
	var raw map[string]any
	if err := common.UnmarshalBodyReusable(c, &raw); err != nil {
		return
	}
	if len(raw) == 0 {
		return
	}
	if req.Metadata == nil {
		req.Metadata = map[string]interface{}{}
	}
	for key, value := range raw {
		if isKnownSubmitField(key) || key == "metadata" {
			continue
		}
		req.Metadata[key] = value
	}
}

func normalizeTaskSubmitReq(req *relaycommon.TaskSubmitReq) {
	req.FirstFrame = firstNonEmpty(
		req.FirstFrame,
		metadataString(req.Metadata, "first_frame_image"),
		metadataString(req.Metadata, "first_frame"),
		req.Image,
		firstString(req.Images),
		req.InputReference,
	)
	req.LastFrame = firstNonEmpty(
		req.LastFrame,
		metadataString(req.Metadata, "last_frame_image"),
		metadataString(req.Metadata, "last_frame"),
		req.ImageTail,
		metadataString(req.Metadata, "image_tail"),
		secondString(req.Images),
	)
	if strings.TrimSpace(req.Image) == "" {
		req.Image = req.FirstFrame
	}
	if len(req.Images) == 0 && strings.TrimSpace(req.Image) != "" {
		req.Images = []string{req.Image}
	}
	if req.N == nil && req.ImageCount != nil {
		req.N = req.ImageCount
	}
	if count := taskSubmitReqCount(*req); count > 0 {
		setTaskMetadataIntIfAbsent(req, count, "n", "image_count", "count")
	}
}

func taskSubmitReqCount(req relaycommon.TaskSubmitReq) int {
	if req.N != nil && *req.N > 0 {
		return *req.N
	}
	if req.ImageCount != nil && *req.ImageCount > 0 {
		return *req.ImageCount
	}
	for _, key := range []string{"n", "image_count", "count"} {
		if value := metadataPositiveInt(req.Metadata, key); value > 0 {
			return value
		}
	}
	return 0
}

func setTaskMetadataIntIfAbsent(req *relaycommon.TaskSubmitReq, value int, keys ...string) {
	if value <= 0 {
		return
	}
	if req.Metadata == nil {
		req.Metadata = map[string]interface{}{}
	}
	for _, key := range keys {
		if _, exists := req.Metadata[key]; !exists {
			req.Metadata[key] = value
		}
	}
}

func metadataPositiveInt(metadata map[string]interface{}, key string) int {
	if metadata == nil {
		return 0
	}
	return positiveIntValue(metadata[key])
}

func positiveIntValue(value any) int {
	switch v := value.(type) {
	case int:
		if v > 0 {
			return v
		}
	case int64:
		if v > 0 && int64(int(v)) == v {
			return int(v)
		}
	case uint:
		if v > 0 && uint(int(v)) == v {
			return int(v)
		}
	case uint64:
		if v > 0 && uint64(int(v)) == v {
			return int(v)
		}
	case float64:
		i := int(v)
		if v > 0 && v == float64(i) {
			return i
		}
	case float32:
		i := int(v)
		if v > 0 && v == float32(i) {
			return i
		}
	case string:
		return parseDurationValue(v)
	}
	return 0
}

func cloneWorkflowMetadata(metadata map[string]interface{}) map[string]any {
	out := make(map[string]any)
	for key, value := range metadata {
		if isKnownSubmitField(key) || isAIPDDPayloadField(key) {
			continue
		}
		out[key] = value
	}
	return out
}

func setContentString(content map[string]any, key string, values ...string) {
	if hasContentValue(content[key]) {
		return
	}
	if value := firstNonEmpty(values...); value != "" {
		content[key] = value
	}
}

func setContentInt(content map[string]any, key string, value int) {
	if hasContentValue(content[key]) {
		return
	}
	if value > 0 {
		content[key] = value
	}
}

func metadataString(metadata map[string]interface{}, key string) string {
	if metadata == nil {
		return ""
	}
	return anyToString(metadata[key])
}

func firstString(values []string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func secondString(values []string) string {
	if len(values) < 2 {
		return ""
	}
	return firstString(values[1:])
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func anyToString(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(v)
	case fmt.Stringer:
		return strings.TrimSpace(v.String())
	default:
		return strings.TrimSpace(fmt.Sprintf("%v", v))
	}
}

func hasContentValue(value any) bool {
	return strings.TrimSpace(anyToString(value)) != ""
}

func isKnownSubmitField(key string) bool {
	switch key {
	case "prompt", "model", "mode", "client_task_id", "image", "image_tail", "first_frame", "last_frame", "images", "size", "n", "image_count", "duration", "seconds", "input_reference", "group":
		return true
	default:
		return false
	}
}

func isAIPDDPayloadField(key string) bool {
	switch key {
	case "task_name", "task_type", "task_cost", "task_service", "task_content", "script_id", "script_code":
		return true
	default:
		return false
	}
}

func failedTaskResultReason(value any) (string, bool) {
	switch v := value.(type) {
	case string:
		var parsed any
		if err := common.Unmarshal([]byte(strings.TrimSpace(v)), &parsed); err != nil {
			return "", false
		}
		return failedTaskResultReason(parsed)
	case map[string]any:
		success, ok := v["success"].(bool)
		if ok && !success {
			return firstNonEmpty(
				anyToString(v["message"]),
				anyToString(v["error"]),
				anyToString(v["reason"]),
				"AIPDD task failed",
			), true
		}
	}
	return "", false
}

func extractResultURLs(value any) []string {
	switch v := value.(type) {
	case nil:
		return nil
	case string:
		return extractResultURLsFromString(v)
	case []string:
		return cleanURLList(v)
	case []any:
		urls := make([]string, 0, len(v))
		for _, item := range v {
			urls = append(urls, extractResultURLs(item)...)
		}
		return cleanURLList(urls)
	case map[string]any:
		for _, key := range []string{"url", "urls", "public_url", "publicUrl", "signed_url", "signedUrl", "download_url", "downloadUrl", "result", "results", "output", "outputs", "objectRefs", "downloadRefs", "video", "videos", "image", "images", "audio", "audios", "file", "files", "data"} {
			if nested, ok := v[key]; ok {
				if urls := extractResultURLs(nested); len(urls) > 0 {
					return urls
				}
			}
		}
	}
	return nil
}

func extractResultURLsFromString(value string) []string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || trimmed == "null" {
		return nil
	}
	var parsed any
	if err := common.Unmarshal([]byte(trimmed), &parsed); err == nil {
		if urls := extractResultURLs(parsed); len(urls) > 0 {
			return urls
		}
		return nil
	}
	if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
		items := strings.Split(strings.Trim(trimmed, "[]"), ",")
		urls := make([]string, 0, len(items))
		for _, item := range items {
			urls = append(urls, strings.Trim(strings.TrimSpace(item), `"'`))
		}
		return cleanURLList(urls)
	}
	return cleanURLList([]string{trimmed})
}

func taskResultText(value any) string {
	if reason, failed := failedTaskResultReason(value); failed {
		return reason
	}
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(v)
	default:
		data, err := common.Marshal(v)
		if err != nil {
			return fmt.Sprintf("%v", v)
		}
		return string(data)
	}
}

func cleanURLList(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] || !isResultURL(value) {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func isResultURL(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	if strings.HasPrefix(strings.ToLower(value), "data:") {
		return true
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return false
	}
	scheme := strings.ToLower(parsed.Scheme)
	return scheme == "http" || scheme == "https"
}
