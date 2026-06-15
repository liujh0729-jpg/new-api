package aipdd

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/aipddcatalog"
	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
)

const (
	ChannelName = "aipdd"

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

type ossUploadResponse struct {
	Code    int            `json:"code"`
	Message string         `json:"message"`
	Data    map[string]any `json:"data"`
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
	cfg, ok := a.resolveModelConfig(ginRequestContext(c), req.Model)
	if !ok {
		return service.TaskErrorWrapperLocal(fmt.Errorf("unsupported AIPDD model: %s", req.Model), "unsupported_model", http.StatusBadRequest)
	}
	if endpoint := endpointTypeFromPath(c.Request.URL.Path); endpoint != "" && endpoint != cfg.EndpointType {
		return service.TaskErrorWrapperLocal(
			fmt.Errorf("%s must be used with %s endpoint", cfg.ModelName, cfg.EndpointType),
			"invalid_endpoint",
			http.StatusBadRequest,
		)
	}
	if cfg.BillingType == constant.AIPDDBillingTypeDurationSeconds {
		duration, err := normalizeDurationSeconds(&req)
		if err != nil {
			return service.TaskErrorWrapperLocal(err, "invalid_duration", http.StatusBadRequest)
		}
		req.Duration = duration
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
	if !ok || cfg.BillingType != constant.AIPDDBillingTypeDurationSeconds {
		return nil
	}
	duration, err := normalizeDurationSeconds(&req)
	if err != nil {
		return nil
	}
	return map[string]float64{"seconds": float64(duration)}
}

func (a *TaskAdaptor) BuildRequestURL(_ *relaycommon.RelayInfo) (string, error) {
	return fmt.Sprintf("%s/shared-tasks/tasks", a.baseURL), nil
}

func (a *TaskAdaptor) BuildRequestHeader(_ *gin.Context, req *http.Request, _ *relaycommon.RelayInfo) error {
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", a.apiKey)
	return nil
}

func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return nil, err
	}
	normalizeTaskSubmitReq(&req)

	cfg, err := a.resolveRequestModelConfig(ginRequestContext(c), req, info)
	if err != nil {
		return nil, err
	}
	if err := a.applyMultipartUploads(c, info, cfg, &req); err != nil {
		return nil, err
	}
	c.Set("task_request", req)

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

	cfg, _ := a.resolveModelConfig(ginRequestContext(c), firstNonEmpty(info.UpstreamModelName, info.OriginModelName))
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

	var detail taskDetailResponse
	if err := common.Unmarshal(originTask.Data, &detail); err == nil && detail.Data != nil {
		urls := extractResultURLs(aipddResultPayload(detail.Data))
		if len(urls) > 1 {
			openAIVideo.SetMetadata("urls", urls)
		}
		if originTask.Status == model.TaskStatusFailure {
			openAIVideo.Error = &dto.OpenAIVideoError{
				Message: firstNonEmpty(taskResultText(aipddResultPayload(detail.Data)), detail.Data.Message),
				Code:    "aipdd_task_failed",
			}
		}
	}

	return common.Marshal(openAIVideo)
}

func (a *TaskAdaptor) convertToRequestPayload(req relaycommon.TaskSubmitReq, info *relaycommon.RelayInfo) (*createTaskPayload, error) {
	cfg, err := a.resolveRequestModelConfig(context.Background(), req, info)
	if err != nil {
		return nil, err
	}
	if cfg.BillingType == constant.AIPDDBillingTypeDurationSeconds {
		duration, err := normalizeDurationSeconds(&req)
		if err != nil {
			return nil, err
		}
		req.Duration = duration
	}

	content := cloneWorkflowMetadata(req.Metadata)
	applyModelDefaults(content, req, cfg)
	if err := validateTaskContent(content, cfg); err != nil {
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

func (a *TaskAdaptor) resolveModelConfig(ctx context.Context, modelName string) (modelConfig, bool) {
	cfg, ok := resolveModelConfig(modelName)
	if ok {
		return cfg, true
	}
	if err := a.refreshCatalog(ctx); err != nil {
		return modelConfig{}, false
	}
	return resolveModelConfig(modelName)
}

func (a *TaskAdaptor) refreshCatalog(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	client, err := service.GetHttpClientWithProxy(a.proxy)
	if err != nil {
		return fmt.Errorf("new proxy http client failed: %w", err)
	}
	if client == nil {
		client = http.DefaultClient
	}
	catalog, err := aipddcatalog.Fetch(ctx, client, a.baseURL, a.apiKey)
	if err != nil {
		return err
	}
	if len(catalog.Capabilities) > 0 {
		constant.SetAIPDDCapabilities(catalog.Capabilities)
		ModelList = constant.GetAIPDDTaskModelList()
		model.InvalidatePricingCache()
	}
	return nil
}

func (a *TaskAdaptor) applyMultipartUploads(c *gin.Context, info *relaycommon.RelayInfo, cfg modelConfig, req *relaycommon.TaskSubmitReq) error {
	contentType := c.GetHeader("Content-Type")
	if !strings.Contains(contentType, gin.MIMEMultipartPOSTForm) {
		return nil
	}

	form, err := common.ParseMultipartFormReusable(c)
	if err != nil {
		return err
	}
	defer form.RemoveAll()
	if len(form.File) == 0 {
		return nil
	}

	uploadedTargets := make(map[string]bool)
	for _, directOnly := range []bool{true, false} {
		fieldNames := make([]string, 0, len(form.File))
		for fieldName := range form.File {
			fieldNames = append(fieldNames, fieldName)
		}
		sort.Strings(fieldNames)

		for _, fieldName := range fieldNames {
			target, direct, ok := resolveAIPDDUploadTarget(cfg, fieldName)
			if !ok || direct != directOnly || uploadedTargets[target] {
				continue
			}
			fileHeaders := form.File[fieldName]
			if len(fileHeaders) == 0 {
				continue
			}
			url, err := a.uploadFileToOSS(c, info, target, fileHeaders[0])
			if err != nil {
				return err
			}
			setUploadedFileURL(req, target, url)
			uploadedTargets[target] = true
		}
	}
	return nil
}

func resolveAIPDDUploadTarget(cfg modelConfig, fieldName string) (target string, direct bool, ok bool) {
	if len(cfg.UploadTargets) == 0 {
		return "", false, false
	}
	normalized := strings.ToLower(strings.TrimSpace(fieldName))
	if normalized == "" {
		return "", false, false
	}
	for _, uploadTarget := range cfg.UploadTargets {
		if strings.ToLower(strings.TrimSpace(uploadTarget.ParamKey)) == normalized {
			return uploadTarget.ParamKey, true, true
		}
	}
	for _, uploadTarget := range cfg.UploadTargets {
		for _, alias := range uploadTarget.Aliases {
			if strings.ToLower(strings.TrimSpace(alias)) == normalized {
				return uploadTarget.ParamKey, false, true
			}
		}
	}
	return "", false, false
}

func (a *TaskAdaptor) uploadFileToOSS(c *gin.Context, info *relaycommon.RelayInfo, paramKey string, fileHeader *multipart.FileHeader) (string, error) {
	if fileHeader == nil {
		return "", fmt.Errorf("missing upload file for %s", paramKey)
	}

	proxy := ""
	if info != nil {
		proxy = info.ChannelSetting.Proxy
	}
	uploadedURL, err := UploadFileToOSS(c.Request.Context(), a.baseURL, a.apiKey, proxy, fileHeader)
	if err != nil {
		return "", fmt.Errorf("upload %s to AIPDD OSS failed: %w", paramKey, err)
	}
	return uploadedURL, nil
}

func UploadFileToOSS(ctx context.Context, baseURL, apiKey, proxy string, fileHeader *multipart.FileHeader) (string, error) {
	if fileHeader == nil {
		return "", fmt.Errorf("missing upload file")
	}
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return "", fmt.Errorf("AIPDD API key is empty")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	var requestBody bytes.Buffer
	contentType, err := writeOSSUploadMultipart(&requestBody, fileHeader)
	if err != nil {
		return "", err
	}

	uri, err := buildOSSUploadURL(baseURL)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, uri, &requestBody)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("X-API-Key", apiKey)

	client, err := service.GetHttpClientWithProxy(proxy)
	if err != nil {
		return "", fmt.Errorf("new proxy http client failed: %w", err)
	}
	if client == nil {
		client = http.DefaultClient
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("upload to AIPDD OSS failed: %w", err)
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read AIPDD OSS upload response failed: %w", err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return "", fmt.Errorf("AIPDD OSS upload failed with status %d: %s", resp.StatusCode, strings.TrimSpace(string(responseBody)))
	}

	var uploadResp ossUploadResponse
	if err := common.Unmarshal(responseBody, &uploadResp); err != nil {
		return "", fmt.Errorf("unmarshal AIPDD OSS upload response failed: %w", err)
	}
	if uploadResp.Code != 0 && uploadResp.Code != http.StatusOK {
		return "", fmt.Errorf("AIPDD OSS upload failed: %s", strings.TrimSpace(uploadResp.Message))
	}
	if uploadResp.Data == nil {
		return "", fmt.Errorf("AIPDD OSS upload response data is empty")
	}
	uploadedURL := firstNonEmpty(
		anyToString(uploadResp.Data["url"]),
		anyToString(uploadResp.Data["public_url"]),
		anyToString(uploadResp.Data["publicUrl"]),
		anyToString(uploadResp.Data["file_url"]),
		anyToString(uploadResp.Data["download_url"]),
	)
	if uploadedURL == "" {
		return "", fmt.Errorf("AIPDD OSS upload response url is empty")
	}
	return uploadedURL, nil
}

func (a *TaskAdaptor) buildOSSUploadURL() (string, error) {
	return buildOSSUploadURL(a.baseURL)
}

func buildOSSUploadURL(baseURL string) (string, error) {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		baseURL = constant.ChannelBaseURLs[constant.ChannelTypeAIPDD]
	}
	uri, err := url.Parse(strings.TrimRight(baseURL, "/") + "/oss/upload")
	if err != nil {
		return "", err
	}
	query := uri.Query()
	query.Set("prefix", "files")
	uri.RawQuery = query.Encode()
	return uri.String(), nil
}

func writeOSSUploadMultipart(buf *bytes.Buffer, fileHeader *multipart.FileHeader) (string, error) {
	writer := multipart.NewWriter(buf)
	file, err := fileHeader.Open()
	if err != nil {
		return "", err
	}
	defer file.Close()

	contentType := strings.TrimSpace(fileHeader.Header.Get("Content-Type"))
	sniff := make([]byte, 512)
	n, readErr := io.ReadFull(file, sniff)
	if readErr != nil && readErr != io.EOF && readErr != io.ErrUnexpectedEOF {
		return "", readErr
	}
	if contentType == "" || contentType == "application/octet-stream" {
		contentType = http.DetectContentType(sniff[:n])
	}

	filename := strings.TrimSpace(fileHeader.Filename)
	if filename == "" {
		filename = "upload.bin"
	}
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename="%s"`, escapeMultipartQuotes(filename)))
	header.Set("Content-Type", contentType)
	part, err := writer.CreatePart(header)
	if err != nil {
		return "", err
	}
	if n > 0 {
		if _, err := part.Write(sniff[:n]); err != nil {
			return "", err
		}
	}
	if _, err := io.Copy(part, file); err != nil {
		return "", err
	}
	if err := writer.Close(); err != nil {
		return "", err
	}
	return writer.FormDataContentType(), nil
}

func escapeMultipartQuotes(value string) string {
	replacer := strings.NewReplacer("\\", "\\\\", `"`, `\"`)
	return replacer.Replace(value)
}

func setUploadedFileURL(req *relaycommon.TaskSubmitReq, target, uploadedURL string) {
	if req.Metadata == nil {
		req.Metadata = map[string]interface{}{}
	}
	req.Metadata[target] = uploadedURL

	switch target {
	case "image":
		req.Image = uploadedURL
		req.Images = []string{uploadedURL}
	case "load_video", "video", "motion_video", "audio":
		if req.InputReference == "" {
			req.InputReference = uploadedURL
		}
	}
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

func normalizeDurationSeconds(req *relaycommon.TaskSubmitReq) (int, error) {
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
			values = append(values, firstString(req.Images))
		case constant.AIPDDWorkflowSourceInputReference:
			values = append(values, req.InputReference)
		case constant.AIPDDWorkflowSourceDuration:
			if req.Duration > 0 {
				values = append(values, strconv.Itoa(req.Duration))
			}
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
	return nil
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
	if len(req.Images) == 0 && strings.TrimSpace(req.Image) != "" {
		req.Images = []string{req.Image}
	}
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
	case "prompt", "model", "mode", "image", "images", "size", "duration", "seconds", "input_reference", "group":
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
