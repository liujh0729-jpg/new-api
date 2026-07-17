package relay

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/gin-gonic/gin"
)

type TaskSubmitResult struct {
	UpstreamTaskID string
	TaskData       []byte
	Platform       constant.TaskPlatform
	Quota          int
	AIPDDExecution *model.AIPDDTaskExecutionSnapshot
	//PerCallPrice   types.PriceData
}

func taskErrorFromUpstreamResponse(responseBody []byte, statusCode int) *dto.TaskError {
	var errorResponse dto.GeneralErrorResponse
	if err := common.Unmarshal(responseBody, &errorResponse); err == nil {
		if upstreamError := errorResponse.TryToOpenAIError(); upstreamError != nil {
			code := "fail_to_fetch_task"
			if upstreamError.Code != nil {
				if upstreamCode := strings.TrimSpace(fmt.Sprint(upstreamError.Code)); upstreamCode != "" {
					code = upstreamCode
				}
			}
			return service.TaskErrorWrapper(errors.New(upstreamError.Message), code, statusCode)
		}
	}

	return service.TaskErrorWrapper(fmt.Errorf("%s", string(responseBody)), "fail_to_fetch_task", statusCode)
}

func isValidClientPublicTaskID(taskID string) bool {
	if !strings.HasPrefix(taskID, "task_") || len(taskID) < len("task_")+8 || len(taskID) > 64 {
		return false
	}
	for _, r := range taskID {
		if r >= 'a' && r <= 'z' {
			continue
		}
		if r >= 'A' && r <= 'Z' {
			continue
		}
		if r >= '0' && r <= '9' {
			continue
		}
		if r == '_' || r == '-' {
			continue
		}
		return false
	}
	return true
}

// ResolveOriginTask 处理基于已有任务的提交（remix / continuation）：
// 查找原始任务、从中提取模型名称、将渠道锁定到原始任务的渠道
// （通过 info.LockedChannel，重试时复用同一渠道并轮换 key），
// 以及提取 OtherRatios（时长、分辨率）。
// 该函数在控制器的重试循环之前调用一次，其结果通过 info 字段和上下文持久化。
func ResolveOriginTask(c *gin.Context, info *relaycommon.RelayInfo) *dto.TaskError {
	// 检测 remix action
	path := c.Request.URL.Path
	if strings.Contains(path, "/v1/videos/") && strings.HasSuffix(path, "/remix") {
		info.Action = constant.TaskActionRemix
	}

	// 提取 remix 任务的 video_id
	if info.Action == constant.TaskActionRemix {
		videoID := c.Param("video_id")
		if strings.TrimSpace(videoID) == "" {
			return service.TaskErrorWrapperLocal(fmt.Errorf("video_id is required"), "invalid_request", http.StatusBadRequest)
		}
		info.OriginTaskID = videoID
	}

	if info.OriginTaskID == "" {
		return nil
	}

	// 查找原始任务
	originTask, exist, err := model.GetByTaskId(info.UserId, info.OriginTaskID)
	if err != nil {
		return service.TaskErrorWrapper(err, "get_origin_task_failed", http.StatusInternalServerError)
	}
	if !exist {
		return service.TaskErrorWrapperLocal(errors.New("task_origin_not_exist"), "task_not_exist", http.StatusBadRequest)
	}

	// 从原始任务推导模型名称
	if info.OriginModelName == "" {
		if originTask.Properties.OriginModelName != "" {
			info.OriginModelName = originTask.Properties.OriginModelName
		} else if originTask.Properties.UpstreamModelName != "" {
			info.OriginModelName = originTask.Properties.UpstreamModelName
		} else {
			var taskData map[string]interface{}
			_ = common.Unmarshal(originTask.Data, &taskData)
			if m, ok := taskData["model"].(string); ok && m != "" {
				info.OriginModelName = m
			}
		}
	}

	// 锁定到原始任务的渠道（重试时复用同一渠道，轮换 key）
	ch, err := model.GetChannelById(originTask.ChannelId, true)
	if err != nil {
		return service.TaskErrorWrapperLocal(err, "channel_not_found", http.StatusBadRequest)
	}
	if ch.Status != common.ChannelStatusEnabled {
		return service.TaskErrorWrapperLocal(errors.New("the channel of the origin task is disabled"), "task_channel_disable", http.StatusBadRequest)
	}
	info.LockedChannel = ch

	if originTask.ChannelId != info.ChannelId {
		key, _, newAPIError := ch.GetNextEnabledKey()
		if newAPIError != nil {
			return service.TaskErrorWrapper(newAPIError, "channel_no_available_key", newAPIError.StatusCode)
		}
		common.SetContextKey(c, constant.ContextKeyChannelKey, key)
		common.SetContextKey(c, constant.ContextKeyChannelType, ch.Type)
		common.SetContextKey(c, constant.ContextKeyChannelBaseUrl, ch.GetBaseURL())
		common.SetContextKey(c, constant.ContextKeyChannelId, originTask.ChannelId)

		info.ChannelBaseUrl = ch.GetBaseURL()
		info.ChannelId = originTask.ChannelId
		info.ChannelType = ch.Type
		info.ApiKey = key
	}

	// 提取 remix 参数（时长、分辨率 → OtherRatios）
	if info.Action == constant.TaskActionRemix {
		if originTask.PrivateData.BillingContext != nil {
			billingContext := originTask.PrivateData.BillingContext
			// 新的 remix 逻辑：直接从原始任务的 BillingContext 中提取 OtherRatios（如果存在）
			for s, f := range billingContext.OtherRatios {
				info.PriceData.AddOtherRatio(s, f)
			}
			if billingContext.BillingMode == billing_setting.BillingModeTaskPricing {
				info.TaskPricingFacts = &relaycommon.TaskPricingFacts{
					Quantity:          billingContext.Quantity,
					Resolution:        billingContext.Resolution,
					HasReferenceVideo: billingContext.HasReferenceVideo,
				}
			}
		} else {
			// 旧的 remix 逻辑：直接从 task data 解析 seconds 和 size（如果存在）
			var taskData map[string]interface{}
			_ = common.Unmarshal(originTask.Data, &taskData)
			secondsStr, _ := taskData["seconds"].(string)
			seconds, _ := strconv.Atoi(secondsStr)
			if seconds <= 0 {
				seconds = 4
			}
			sizeStr, _ := taskData["size"].(string)
			if info.PriceData.OtherRatios == nil {
				info.PriceData.OtherRatios = map[string]float64{}
			}
			info.PriceData.OtherRatios["seconds"] = float64(seconds)
			info.PriceData.OtherRatios["size"] = 1
			if sizeStr == "1792x1024" || sizeStr == "1024x1792" {
				info.PriceData.OtherRatios["size"] = 1.666667
			}
		}
	}

	return nil
}

// RelayTaskSubmit 完成 task 提交的全部流程（每次尝试调用一次）：
// 刷新渠道元数据 → 确定 platform/adaptor → 验证请求 →
// 估算计费(EstimateBilling) → 计算价格 → 预扣费（仅首次）→
// 构建/发送/解析上游请求 → 提交后计费调整(AdjustBillingOnSubmit)。
// 控制器负责 defer Refund 和成功后 Settle。
func RelayTaskSubmit(c *gin.Context, info *relaycommon.RelayInfo) (*TaskSubmitResult, *dto.TaskError) {
	info.InitChannelMeta(c)

	// 1. 确定 platform → 创建适配器 → 验证请求
	platform := constant.TaskPlatform(c.GetString("platform"))
	if platform == "" {
		platform = GetTaskPlatform(c)
	}
	adaptor := GetTaskAdaptor(platform)
	if adaptor == nil {
		return nil, service.TaskErrorWrapperLocal(fmt.Errorf("invalid api platform: %s", platform), "invalid_api_platform", http.StatusBadRequest)
	}
	adaptor.Init(info)

	// Resolve a request model before provider validation whenever one is already
	// known. AIPDD aliases are local names and do not exist in the upstream
	// capability catalog, so validating the alias before mapping would reject a
	// perfectly valid route as unsupported.
	modelName := info.OriginModelName
	applyModelMapping := func(originModelName string) *dto.TaskError {
		info.OriginModelName = originModelName
		info.UpstreamModelName = originModelName
		if err := helper.ModelMappedHelper(c, info, nil); err != nil {
			return service.TaskErrorWrapperLocal(err, "model_mapping_failed", http.StatusBadRequest)
		}
		return nil
	}
	if modelName != "" {
		if taskErr := applyModelMapping(modelName); taskErr != nil {
			return nil, taskErr
		}
	}
	if taskErr := adaptor.ValidateRequestAndSetAction(c, info); taskErr != nil {
		return nil, taskErr
	}

	// 2. 确定模型名称
	if modelName == "" {
		modelName = service.CoverTaskActionToModelName(platform, info.Action)
		if taskErr := applyModelMapping(modelName); taskErr != nil {
			return nil, taskErr
		}
	}
	taskPricingMode := info.TaskPricingQuote != nil ||
		billing_setting.GetBillingMode(modelName) == billing_setting.BillingModeTaskPricing
	if (constant.IsAIPDDTaskPricingModel(info.UpstreamModelName) ||
		model.IsAIPDDTaskPricingRequiredModel(modelName)) &&
		!taskPricingMode {
		return nil, service.TaskErrorWrapperLocal(
			fmt.Errorf("model %s task pricing is not configured", modelName),
			"model_price_error",
			http.StatusBadRequest,
		)
	}

	// 3. 预生成公开 task ID（仅首次）
	if info.PublicTaskID == "" {
		if req, err := relaycommon.GetTaskRequest(c); err == nil && info.IsPlayground {
			clientTaskID := strings.TrimSpace(req.ClientTaskID)
			if clientTaskID != "" {
				if !isValidClientPublicTaskID(clientTaskID) {
					return nil, service.TaskErrorWrapperLocal(fmt.Errorf("invalid client_task_id"), "invalid_client_task_id", http.StatusBadRequest)
				}
				info.PublicTaskID = clientTaskID
			}
		}
		if info.PublicTaskID == "" {
			info.PublicTaskID = model.GenerateTaskID()
		}
	}

	// 4. 价格计算：基础模型价格
	info.OriginModelName = modelName
	if quote := info.TaskPricingQuote; quote != nil {
		// A retry keeps the first attempt's complete local quote even if an
		// administrator edits or removes the live pricing configuration meanwhile.
		info.PriceData.ModelPrice = quote.UnitPriceUSD
		info.PriceData.UsePrice = true
		info.PriceData.Quota = quote.Quota
		info.PriceData.GroupRatioInfo.GroupRatio = quote.GroupRatio
		info.PriceData.FreeModel = quote.GroupRatio == 0
	} else {
		priceData, err := helper.ModelPriceHelperPerCall(c, info)
		if err != nil {
			return nil, service.TaskErrorWrapper(err, "model_price_error", http.StatusBadRequest)
		}
		info.PriceData = priceData
	}

	// 5. 计费估算：让适配器根据用户请求提供 OtherRatios（时长、分辨率等）
	//    必须在 ModelPriceHelperPerCall 之后调用（它会重建 PriceData）。
	//    ResolveOriginTask 可能已在 remix 路径中预设了 OtherRatios，此处合并。
	if estimatedRatios := adaptor.EstimateBilling(c, info); len(estimatedRatios) > 0 {
		for k, v := range estimatedRatios {
			info.PriceData.AddOtherRatio(k, v)
		}
	}

	// 6. 将 OtherRatios 应用到基础额度，或使用适配器给出的不可分解精确额度。
	exactQuota := false
	if taskPricingMode {
		quote := info.TaskPricingQuote
		if quote == nil {
			factsProvider, ok := adaptor.(channel.TaskPricingFactsProvider)
			if !ok {
				return nil, service.TaskErrorWrapperLocal(
					fmt.Errorf("task pricing facts are unavailable for model %s", info.OriginModelName),
					"task_pricing_facts_unavailable",
					http.StatusBadRequest,
				)
			}
			facts, factsErr := factsProvider.EstimateTaskPricingFacts(c, info)
			if factsErr != nil {
				return nil, factsErr
			}
			info.TaskPricingFacts = &facts
			resolved, quoteErr := billing_setting.QuoteTaskPricing(
				info.OriginModelName,
				facts.Quantity,
				facts.Resolution,
				info.PriceData.GroupRatioInfo.GroupRatio,
				common.QuotaPerUnit,
				facts.HasReferenceVideo,
			)
			if quoteErr != nil {
				code := "model_price_error"
				if errors.Is(quoteErr, billing_setting.ErrReferenceVideoDisabled) {
					code = "reference_video_not_allowed"
				} else if errors.Is(quoteErr, billing_setting.ErrTaskPricingResolutionNotConfigured) {
					code = "resolution_price_not_configured"
				} else if errors.Is(quoteErr, billing_setting.ErrTaskPricingResolutionRequired) {
					code = "missing_resolution"
				}
				return nil, service.TaskErrorWrapperLocal(quoteErr, code, http.StatusBadRequest)
			}
			info.TaskPricingQuote = &resolved
			quote = info.TaskPricingQuote
		}
		info.PriceData.ModelPrice = quote.UnitPriceUSD
		info.PriceData.UsePrice = true
		info.PriceData.Quota = quote.Quota
		// A quote survives channel retries. Restore its frozen group ratio as well
		// as the price and request facts so a concurrent settings change cannot
		// alter the task's retail charge or audit snapshot mid-request.
		info.PriceData.GroupRatioInfo.GroupRatio = quote.GroupRatio
		info.PriceData.FreeModel = quote.GroupRatio == 0
		info.PriceData.OtherRatios = map[string]float64{
			"seconds":             quote.Quantity,
			"has_reference_video": boolFloat64(quote.HasReferenceVideo),
		}
		exactQuota = true
	}
	if !exactQuota {
		if estimator, ok := adaptor.(channel.ExactTaskBillingEstimator); ok {
			quota, details, exactErr := estimator.EstimateExactQuota(c, info)
			if exactErr != nil {
				return nil, service.TaskErrorWrapperLocal(exactErr, "model_price_error", http.StatusBadRequest)
			}
			if quota > 0 {
				info.PriceData.Quota = quota
				info.PriceData.OtherRatios = details
				exactQuota = true
			}
		}
	}
	if !exactQuota && !common.StringsContains(constant.TaskPricePatches, modelName) {
		for _, ra := range info.PriceData.OtherRatios {
			if ra != 1.0 {
				info.PriceData.Quota = int(float64(info.PriceData.Quota) * ra)
			}
		}
	}

	// 7. 预扣费（仅首次 — 重试时 info.Billing 已存在，跳过）
	if info.Billing == nil && !info.PriceData.FreeModel {
		info.ForcePreConsume = true
		if apiErr := service.PreConsumeBilling(c, info.PriceData.Quota, info); apiErr != nil {
			return nil, service.TaskErrorFromAPIError(apiErr)
		}
	}

	// 8. 构建请求体
	requestBody, err := adaptor.BuildRequestBody(c, info)
	if err != nil {
		return nil, service.TaskErrorWrapper(err, "build_request_failed", http.StatusInternalServerError)
	}

	// 9. 发送请求
	resp, err := adaptor.DoRequest(c, info, requestBody)
	if err != nil {
		return nil, service.TaskErrorWrapper(err, "do_request_failed", http.StatusInternalServerError)
	}
	if resp != nil && resp.StatusCode != http.StatusOK {
		responseBody, _ := io.ReadAll(resp.Body)
		return nil, taskErrorFromUpstreamResponse(responseBody, resp.StatusCode)
	}

	// 10. 返回 OtherRatios 给下游（header 必须在 DoResponse 写 body 之前设置）
	otherRatios := info.PriceData.OtherRatios
	if otherRatios == nil {
		otherRatios = map[string]float64{}
	}
	ratiosJSON, _ := common.Marshal(otherRatios)
	c.Header("X-New-Api-Other-Ratios", string(ratiosJSON))

	// 11. 解析响应
	upstreamTaskID, taskData, taskErr := adaptor.DoResponse(c, resp, info)
	if taskErr != nil {
		return nil, taskErr
	}

	// 11. 提交后计费调整：让适配器根据上游实际返回调整 OtherRatios
	finalQuota := info.PriceData.Quota
	if adjustedRatios := adaptor.AdjustBillingOnSubmit(info, taskData); info.TaskPricingQuote == nil && len(adjustedRatios) > 0 {
		// 基于调整后的 ratios 重新计算 quota
		finalQuota = recalcQuotaFromRatios(info, adjustedRatios)
		info.PriceData.OtherRatios = adjustedRatios
		info.PriceData.Quota = finalQuota
	}

	result := &TaskSubmitResult{
		UpstreamTaskID: upstreamTaskID,
		TaskData:       taskData,
		Platform:       platform,
		Quota:          finalQuota,
	}
	if provider, ok := adaptor.(channel.AIPDDTaskSnapshotProvider); ok {
		result.AIPDDExecution = provider.AIPDDTaskSnapshot(info)
	}
	return result, nil
}

func boolFloat64(value bool) float64 {
	if value {
		return 1
	}
	return 0
}

// recalcQuotaFromRatios 根据 adjustedRatios 重新计算 quota。
// 公式: baseQuota × ∏(ratio) — 其中 baseQuota 是不含 OtherRatios 的基础额度。
func recalcQuotaFromRatios(info *relaycommon.RelayInfo, ratios map[string]float64) int {
	// 从 PriceData 获取不含 OtherRatios 的基础价格
	baseQuota := info.PriceData.Quota
	// 先除掉原有的 OtherRatios 恢复基础额度
	for _, ra := range info.PriceData.OtherRatios {
		if ra != 1.0 && ra > 0 {
			baseQuota = int(float64(baseQuota) / ra)
		}
	}
	// 应用新的 ratios
	result := float64(baseQuota)
	for _, ra := range ratios {
		if ra != 1.0 {
			result *= ra
		}
	}
	return int(result)
}

var fetchRespBuilders = map[int]func(c *gin.Context) (respBody []byte, taskResp *dto.TaskError){
	relayconstant.RelayModeSunoFetchByID:  sunoFetchByIDRespBodyBuilder,
	relayconstant.RelayModeSunoFetch:      sunoFetchRespBodyBuilder,
	relayconstant.RelayModeVideoFetchByID: videoFetchByIDRespBodyBuilder,
}

func RelayTaskFetch(c *gin.Context, relayMode int) (taskResp *dto.TaskError) {
	respBuilder, ok := fetchRespBuilders[relayMode]
	if !ok {
		taskResp = service.TaskErrorWrapperLocal(errors.New("invalid_relay_mode"), "invalid_relay_mode", http.StatusBadRequest)
	}

	respBody, taskErr := respBuilder(c)
	if taskErr != nil {
		return taskErr
	}
	if len(respBody) == 0 {
		respBody = []byte("{\"code\":\"success\",\"data\":null}")
	}

	c.Writer.Header().Set("Content-Type", "application/json")
	_, err := io.Copy(c.Writer, bytes.NewBuffer(respBody))
	if err != nil {
		taskResp = service.TaskErrorWrapper(err, "copy_response_body_failed", http.StatusInternalServerError)
		return
	}
	return
}

func sunoFetchRespBodyBuilder(c *gin.Context) (respBody []byte, taskResp *dto.TaskError) {
	userId := c.GetInt("id")
	var condition = struct {
		IDs    []any  `json:"ids"`
		Action string `json:"action"`
	}{}
	err := c.BindJSON(&condition)
	if err != nil {
		taskResp = service.TaskErrorWrapper(err, "invalid_request", http.StatusBadRequest)
		return
	}
	var tasks []any
	if len(condition.IDs) > 0 {
		taskModels, err := model.GetByTaskIds(userId, condition.IDs)
		if err != nil {
			taskResp = service.TaskErrorWrapper(err, "get_tasks_failed", http.StatusInternalServerError)
			return
		}
		for _, task := range taskModels {
			tasks = append(tasks, TaskModel2Dto(task))
		}
	} else {
		tasks = make([]any, 0)
	}
	respBody, err = common.Marshal(dto.TaskResponse[[]any]{
		Code: "success",
		Data: tasks,
	})
	return
}

func sunoFetchByIDRespBodyBuilder(c *gin.Context) (respBody []byte, taskResp *dto.TaskError) {
	taskId := c.Param("id")
	userId := c.GetInt("id")

	originTask, exist, err := model.GetByTaskId(userId, taskId)
	if err != nil {
		taskResp = service.TaskErrorWrapper(err, "get_task_failed", http.StatusInternalServerError)
		return
	}
	if !exist {
		taskResp = service.TaskErrorWrapperLocal(errors.New("task_not_exist"), "task_not_exist", http.StatusBadRequest)
		return
	}

	respBody, err = common.Marshal(dto.TaskResponse[any]{
		Code: "success",
		Data: TaskModel2Dto(originTask),
	})
	return
}

func videoFetchByIDRespBodyBuilder(c *gin.Context) (respBody []byte, taskResp *dto.TaskError) {
	taskId := c.Param("task_id")
	if taskId == "" {
		taskId = c.GetString("task_id")
	}
	userId := c.GetInt("id")

	originTask, exist, err := model.GetByTaskId(userId, taskId)
	if err != nil {
		taskResp = service.TaskErrorWrapper(err, "get_task_failed", http.StatusInternalServerError)
		return
	}
	if !exist {
		taskResp = service.TaskErrorWrapperLocal(errors.New("task_not_exist"), "task_not_exist", http.StatusBadRequest)
		return
	}

	isOpenAIVideoAPI := strings.HasPrefix(c.Request.RequestURI, "/v1/videos/")

	// Gemini/Vertex/AIPDD 支持实时查询：用户 fetch 时直接从上游拉取最新状态
	if realtimeResp := tryRealtimeFetch(originTask, isOpenAIVideoAPI); len(realtimeResp) > 0 {
		respBody = realtimeResp
		return
	}

	// OpenAI Video API 格式: 走各 adaptor 的 ConvertToOpenAIVideo
	if isOpenAIVideoAPI {
		adaptor := GetTaskAdaptor(originTask.Platform)
		if adaptor == nil {
			taskResp = service.TaskErrorWrapperLocal(fmt.Errorf("invalid channel id: %d", originTask.ChannelId), "invalid_channel_id", http.StatusBadRequest)
			return
		}
		if converter, ok := adaptor.(channel.OpenAIVideoConverter); ok {
			openAIVideoData, err := converter.ConvertToOpenAIVideo(originTask)
			if err != nil {
				taskResp = service.TaskErrorWrapper(err, "convert_to_openai_video_failed", http.StatusInternalServerError)
				return
			}
			respBody = openAIVideoData
			return
		}
		taskResp = service.TaskErrorWrapperLocal(fmt.Errorf("not_implemented:%s", originTask.Platform), "not_implemented", http.StatusNotImplemented)
		return
	}

	// 通用 TaskDto 格式
	respBody, err = common.Marshal(dto.TaskResponse[any]{
		Code: "success",
		Data: TaskModel2Dto(originTask),
	})
	if err != nil {
		taskResp = service.TaskErrorWrapper(err, "marshal_response_failed", http.StatusInternalServerError)
	}
	return
}

// tryRealtimeFetch 尝试从上游实时拉取 Gemini/Vertex/AIPDD 任务状态。
// 仅当渠道类型支持直接查询时触发；其他渠道或出错时返回 nil。
// 当非 OpenAI Video API 时，还会构建自定义格式的响应体。
func tryRealtimeFetch(task *model.Task, isOpenAIVideoAPI bool) []byte {
	channelModel, err := model.GetChannelById(task.ChannelId, true)
	if err != nil {
		return nil
	}
	if channelModel.Type != constant.ChannelTypeVertexAi &&
		channelModel.Type != constant.ChannelTypeGemini &&
		channelModel.Type != constant.ChannelTypeAIPDD {
		return nil
	}

	baseURL := constant.ChannelBaseURLs[channelModel.Type]
	if channelModel.GetBaseURL() != "" {
		baseURL = channelModel.GetBaseURL()
	}
	proxy := channelModel.GetSetting().Proxy
	adaptor := GetTaskAdaptor(constant.TaskPlatform(strconv.Itoa(channelModel.Type)))
	if adaptor == nil {
		return nil
	}

	fetchBody := map[string]any{"task_id": task.GetUpstreamTaskID(), "action": task.Action}
	if snapshot := task.PrivateData.AIPDDExecution; snapshot != nil {
		if strings.TrimSpace(snapshot.BaseURL) != "" {
			baseURL = snapshot.BaseURL
		}
		fetchBody["execution_protocol"] = snapshot.Protocol
		fetchBody["execution_endpoint"] = snapshot.Endpoint
		fetchBody["catalog_revision"] = snapshot.CatalogRevision
	}
	resp, err := adaptor.FetchTask(baseURL, channelModel.Key, fetchBody, proxy)
	if err != nil || resp == nil {
		return nil
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil
	}

	ti, err := adaptor.ParseTaskResult(body)
	if err != nil || ti == nil {
		return nil
	}

	snap := task.Snapshot()

	if channelModel.Type == constant.ChannelTypeAIPDD {
		task.Data = body
	}

	// 将上游最新状态更新到 task
	if ti.Status != "" {
		task.Status = model.TaskStatus(ti.Status)
	}
	if ti.Progress != "" {
		task.Progress = ti.Progress
	}
	if task.Status == model.TaskStatusFailure && strings.TrimSpace(ti.Reason) != "" {
		// Keep the upstream failure reason on the task itself.  The playground
		// endpoint returns a compact task envelope and otherwise used to replace
		// every Seedance failure with a generic "task failed" message.
		task.FailReason = strings.TrimSpace(ti.Reason)
	}
	if strings.HasPrefix(ti.Url, "data:") {
		// data: URI — kept in Data, not ResultURL
	} else if ti.Url != "" {
		task.PrivateData.ResultURL = ti.Url
	} else if task.Status == model.TaskStatusSuccess {
		// No URL from adaptor — construct proxy URL using public task ID
		task.PrivateData.ResultURL = taskcommon.BuildProxyURL(task.TaskID)
	}

	if !snap.Equal(task.Snapshot()) {
		_, _ = task.UpdateWithStatus(snap.Status)
	}

	// OpenAI Video API 由调用者的 ConvertToOpenAIVideo 分支处理
	if isOpenAIVideoAPI {
		return nil
	}

	// 非 OpenAI Video API: 构建自定义格式响应
	format := detectVideoFormat(body)
	output := extractTaskOutputURLs(task)
	var taskError any
	if task.Status == model.TaskStatusFailure {
		taskError = strings.TrimSpace(task.FailReason)
		if taskError == "" {
			taskError = strings.TrimSpace(ti.Reason)
		}
		if taskError == "" {
			taskError = "AIPDD task failed"
		}
	}
	out := map[string]any{
		"error":    taskError,
		"format":   format,
		"metadata": map[string]any{"urls": output},
		"output":   output,
		"status":   mapTaskStatusToSimple(task.Status),
		"task_id":  task.TaskID,
		"url":      task.GetResultURL(),
	}
	respBody, _ := common.Marshal(dto.TaskResponse[any]{
		Code: "success",
		Data: out,
	})
	return respBody
}

// detectVideoFormat 从 Gemini/Vertex 原始响应中探测视频格式
func detectVideoFormat(rawBody []byte) string {
	var raw map[string]any
	if err := common.Unmarshal(rawBody, &raw); err != nil {
		return "mp4"
	}
	respObj, ok := raw["response"].(map[string]any)
	if !ok {
		return "mp4"
	}
	vids, ok := respObj["videos"].([]any)
	if !ok || len(vids) == 0 {
		return "mp4"
	}
	v0, ok := vids[0].(map[string]any)
	if !ok {
		return "mp4"
	}
	mt, ok := v0["mimeType"].(string)
	if !ok || mt == "" || strings.Contains(mt, "mp4") {
		return "mp4"
	}
	return mt
}

// mapTaskStatusToSimple 将内部 TaskStatus 映射为简化状态字符串
func mapTaskStatusToSimple(status model.TaskStatus) string {
	switch status {
	case model.TaskStatusSuccess:
		return "succeeded"
	case model.TaskStatusFailure:
		return "failed"
	case model.TaskStatusQueued, model.TaskStatusSubmitted:
		return "queued"
	default:
		return "processing"
	}
}

func TaskModel2Dto(task *model.Task) *dto.TaskDto {
	output := extractTaskOutputURLs(task)
	var metadata map[string]any
	if len(output) > 0 {
		metadata = map[string]any{
			"url":  output[0],
			"urls": output,
		}
	}
	return &dto.TaskDto{
		ID:         task.ID,
		CreatedAt:  task.CreatedAt,
		UpdatedAt:  task.UpdatedAt,
		TaskID:     task.TaskID,
		Platform:   string(task.Platform),
		UserId:     task.UserId,
		Group:      task.Group,
		ChannelId:  task.ChannelId,
		Quota:      task.Quota,
		Action:     task.Action,
		Status:     string(task.Status),
		FailReason: task.FailReason,
		ResultURL:  task.GetResultURL(),
		SubmitTime: task.SubmitTime,
		StartTime:  task.StartTime,
		FinishTime: task.FinishTime,
		Progress:   task.Progress,
		Properties: task.Properties,
		Username:   task.Username,
		Output:     output,
		Metadata:   metadata,
		Data:       task.Data,
	}
}

func extractTaskOutputURLs(task *model.Task) []string {
	if task == nil || task.Status != model.TaskStatusSuccess {
		return nil
	}
	urls := make([]string, 0)
	if resultURL := strings.TrimSpace(task.GetResultURL()); resultURL != "" {
		urls = append(urls, resultURL)
	}
	var raw any
	if len(task.Data) > 0 && common.Unmarshal(task.Data, &raw) == nil {
		urls = append(urls, extractURLsFromTaskData(raw)...)
	}
	return cleanTaskOutputURLs(urls)
}

func extractURLsFromTaskData(value any) []string {
	switch v := value.(type) {
	case nil:
		return nil
	case string:
		return extractURLsFromTaskString(v)
	case []string:
		return v
	case []any:
		urls := make([]string, 0, len(v))
		for _, item := range v {
			urls = append(urls, extractURLsFromTaskData(item)...)
		}
		return urls
	case map[string]any:
		for _, key := range []string{"task_result", "url", "urls", "result", "results", "output", "outputs", "video", "videos", "image", "images", "audio", "audios", "file", "files", "data"} {
			if nested, ok := v[key]; ok {
				if urls := extractURLsFromTaskData(nested); len(urls) > 0 {
					return urls
				}
			}
		}
	}
	return nil
}

func extractURLsFromTaskString(value string) []string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || trimmed == "null" {
		return nil
	}
	var parsed any
	if err := common.Unmarshal([]byte(trimmed), &parsed); err == nil {
		return extractURLsFromTaskData(parsed)
	}
	if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
		items := strings.Split(strings.Trim(trimmed, "[]"), ",")
		urls := make([]string, 0, len(items))
		for _, item := range items {
			urls = append(urls, strings.Trim(strings.TrimSpace(item), `"'`))
		}
		return urls
	}
	return []string{trimmed}
}

func cleanTaskOutputURLs(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		if !strings.HasPrefix(value, "http://") &&
			!strings.HasPrefix(value, "https://") &&
			!strings.HasPrefix(value, "data:") &&
			!strings.HasPrefix(value, "/") {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}
