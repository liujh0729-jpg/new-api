package relay

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	neturl "net/url"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

type TaskSubmitResult struct {
	UpstreamTaskID      string
	TaskData            []byte
	Platform            constant.TaskPlatform
	Quota               int
	CallbackURL         string
	DeferredHTTPStatus  int
	DeferredContentType string
	DeferredBody        []byte
	TaskPersisted       bool
	AIPDDExecution      *model.AIPDDTaskExecutionSnapshot
	//PerCallPrice   types.PriceData
}

func quoteCatalogSeedancePricing(
	capability constant.AIPDDCapability,
	facts relaycommon.TaskPricingFacts,
	groupRatio float64,
	quotaPerUnit float64,
) (billing_setting.TaskPricingQuote, error) {
	if capability.SeedancePricing == nil || capability.AWCoinUSDPerCoin <= 0 ||
		math.IsNaN(capability.AWCoinUSDPerCoin) || math.IsInf(capability.AWCoinUSDPerCoin, 0) {
		return billing_setting.TaskPricingQuote{}, billing_setting.ErrTaskPricingNotConfigured
	}
	if facts.Quantity <= 0 || math.IsNaN(facts.Quantity) || math.IsInf(facts.Quantity, 0) ||
		groupRatio < 0 || math.IsNaN(groupRatio) || math.IsInf(groupRatio, 0) ||
		quotaPerUnit <= 0 || math.IsNaN(quotaPerUnit) || math.IsInf(quotaPerUnit, 0) {
		return billing_setting.TaskPricingQuote{}, billing_setting.ErrInvalidTaskPricing
	}
	resolution, err := billing_setting.NormalizeTaskPricingResolution(facts.Resolution)
	if err != nil {
		return billing_setting.TaskPricingQuote{}, fmt.Errorf("%w: %v", billing_setting.ErrTaskPricingResolutionRequired, err)
	}
	var tier constant.AIPDDSeedanceResolutionPricing
	found := false
	for rawResolution, candidate := range capability.SeedancePricing.ByResolution {
		canonical, normalizeErr := billing_setting.NormalizeTaskPricingResolution(rawResolution)
		if normalizeErr == nil && canonical == resolution {
			tier, found = candidate, true
			break
		}
	}
	if !found {
		return billing_setting.TaskPricingQuote{}, fmt.Errorf(
			"%w: model %q resolution %q",
			billing_setting.ErrTaskPricingResolutionNotConfigured,
			capability.ModelName,
			resolution,
		)
	}
	variant := billing_setting.TaskPricingVariantNoReferenceVideo
	awcoinPerSecond := tier.AmountAWCoinPerSecond
	byok := strings.EqualFold(strings.TrimSpace(capability.SeedancePricing.BillingMode), "BYOK")
	if byok && tier.BYOKAmountAWCoinPerSecond != nil {
		awcoinPerSecond = *tier.BYOKAmountAWCoinPerSecond
	}
	if facts.HasReferenceVideo {
		variant = billing_setting.TaskPricingVariantReferenceVideo
		awcoinPerSecond = tier.VideoInputAWCoinPerSecond
		if byok && tier.BYOKVideoInputAWCoinPerSecond != nil {
			awcoinPerSecond = *tier.BYOKVideoInputAWCoinPerSecond
		}
	}
	if awcoinPerSecond <= 0 || math.IsNaN(awcoinPerSecond) || math.IsInf(awcoinPerSecond, 0) {
		return billing_setting.TaskPricingQuote{}, billing_setting.ErrInvalidTaskPricing
	}
	unitPriceUSD := awcoinPerSecond * capability.AWCoinUSDPerCoin
	baseUSD := unitPriceUSD * facts.Quantity
	saleUSD := baseUSD * groupRatio
	quotaValue := saleUSD * quotaPerUnit
	if unitPriceUSD <= 0 || math.IsNaN(quotaValue) || math.IsInf(quotaValue, 0) || quotaValue < 0 {
		return billing_setting.TaskPricingQuote{}, billing_setting.ErrInvalidTaskPricing
	}
	return billing_setting.TaskPricingQuote{
		Unit:              billing_setting.TaskPricingUnitSecond,
		Variant:           variant,
		UnitPriceUSD:      unitPriceUSD,
		Quantity:          facts.Quantity,
		GroupRatio:        groupRatio,
		BaseUSD:           baseUSD,
		SaleUSD:           saleUSD,
		Quota:             billingexpr.QuotaRound(quotaValue),
		HasReferenceVideo: facts.HasReferenceVideo,
		Resolution:        resolution,
	}, nil
}

var upstreamAccountIDPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\b((?:your\s+)?account(?:[\s_-]*id)?\s*(?:is\s+|[=:：#]\s*)?)\d{4,}\b`),
	regexp.MustCompile(`((?:您的?|当前)?账号(?:\s*(?:ID|id))?\s*(?:为|是|[=:：])?\s*)\d{4,}`),
}

func sanitizeUpstreamErrorMessage(message string) string {
	for _, pattern := range upstreamAccountIDPatterns {
		message = pattern.ReplaceAllString(message, `${1}[redacted]`)
	}
	return message
}

func taskErrorFromUpstreamResponse(responseBody []byte, statusCode int, aipddProvider ...bool) *dto.TaskError {
	isAIPDD := len(aipddProvider) > 0 && aipddProvider[0]
	normalizeError := func(code, message, param, requestID string) relaycommon.PublicUpstreamTaskError {
		if isAIPDD {
			return relaycommon.NormalizeAIPDDTaskError(
				code,
				message,
				param,
				requestID,
				statusCode,
				relaycommon.AIPDDTaskErrorOperationCreate,
			)
		}
		return relaycommon.NormalizeUpstreamTaskError(code, message, param, requestID)
	}
	var errorResponse dto.GeneralErrorResponse
	if err := common.Unmarshal(responseBody, &errorResponse); err == nil {
		if upstreamError := errorResponse.TryToOpenAIError(); upstreamError != nil {
			rawCode := "fail_to_fetch_task"
			if upstreamError.Code != nil {
				if upstreamCode := strings.TrimSpace(fmt.Sprint(upstreamError.Code)); upstreamCode != "" {
					rawCode = upstreamCode
				}
			}
			rawMessage := sanitizeUpstreamErrorMessage(upstreamError.Message)
			param := strings.TrimSpace(upstreamError.Param)
			requestID := ""
			var requestIDs struct {
				RequestID      string `json:"request_id"`
				CamelRequestID string `json:"requestId"`
			}
			if err := common.Unmarshal(responseBody, &requestIDs); err == nil {
				requestID = strings.TrimSpace(requestIDs.RequestID)
				if requestID == "" {
					requestID = strings.TrimSpace(requestIDs.CamelRequestID)
				}
			}

			publicError := normalizeError(rawCode, rawMessage, param, requestID)
			if param == "" {
				param = publicError.Param
			}
			if requestID == "" {
				requestID = publicError.RequestID
			}
			code, message := rawCode, rawMessage
			if publicError.Matched {
				code, message = publicError.Code, publicError.Message
			}

			taskErr := service.TaskErrorWrapper(errors.New(rawMessage), code, statusCode)
			taskErr.Message = message
			details := make(map[string]any, 3)
			if errorType := strings.TrimSpace(upstreamError.Type); errorType != "" && !publicError.HideRaw {
				details["type"] = errorType
			}
			if publicError.Matched {
				for key, value := range publicError.Data() {
					details[key] = value
				}
			}
			if param != "" {
				details["param"] = param
			}
			if requestID != "" {
				details["request_id"] = requestID
			}
			if len(details) > 0 {
				taskErr.Data = details
			}
			return taskErr
		}
	}

	rawMessage := sanitizeUpstreamErrorMessage(string(responseBody))
	taskErr := service.TaskErrorWrapper(fmt.Errorf("%s", rawMessage), "fail_to_fetch_task", statusCode)
	publicError := normalizeError("", rawMessage, "", "")
	if publicError.Matched {
		taskErr.Code = publicError.Code
		taskErr.Message = publicError.Message
		if data := publicError.Data(); len(data) > 0 {
			taskErr.Data = data
		}
	}
	return taskErr
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
	if relayconstant.IsSeedanceOfficialTasksPath(c.Request.URL.Path) {
		if _, ok := adaptor.(channel.SeedanceOfficialTaskConverter); !ok {
			return nil, service.TaskErrorWrapperLocal(
				fmt.Errorf("Seedance official endpoint is not supported by channel platform %s", platform),
				"invalid_endpoint",
				http.StatusBadRequest,
			)
		}
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
	catalogCapability, hasCatalogSeedancePricing := constant.GetAIPDDCapability(info.UpstreamModelName)
	hasCatalogSeedancePricing = hasCatalogSeedancePricing && catalogCapability.SeedancePricing != nil &&
		catalogCapability.AWCoinUSDPerCoin > 0
	useCatalogSeedancePricing := hasCatalogSeedancePricing &&
		billing_setting.GetBillingMode(modelName) != billing_setting.BillingModeTaskPricing
	taskPricingMode := info.TaskPricingQuote != nil || useCatalogSeedancePricing ||
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
	} else if useCatalogSeedancePricing {
		groupRatioInfo := helper.HandleGroupRatio(c, info)
		info.PriceData = types.PriceData{
			UsePrice:       true,
			FreeModel:      groupRatioInfo.GroupRatio == 0,
			GroupRatioInfo: groupRatioInfo,
		}
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
			var resolved billing_setting.TaskPricingQuote
			var quoteErr error
			if useCatalogSeedancePricing {
				resolved, quoteErr = quoteCatalogSeedancePricing(
					catalogCapability,
					facts,
					info.PriceData.GroupRatioInfo.GroupRatio,
					common.QuotaPerUnit,
				)
			} else {
				resolved, quoteErr = billing_setting.QuoteTaskPricing(
					info.OriginModelName,
					facts.Quantity,
					facts.Resolution,
					info.PriceData.GroupRatioInfo.GroupRatio,
					common.QuotaPerUnit,
					facts.HasReferenceVideo,
				)
			}
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
	isIndependentSeedance := platform == constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeSeedance))
	wasSeedanceSubmissionPrepared := info.SeedanceSubmissionPrepared
	if isIndependentSeedance && wasSeedanceSubmissionPrepared {
		// A previous invocation already crossed the durable pre-submit boundary.
		// Without an Ark idempotency token we cannot distinguish "never sent" from
		// "accepted but response lost", so a repeated invocation only returns the
		// existing public task and never sends another create request.
		return deferredIndependentSeedanceResult(info, platform, info.PriceData.Quota, "", nil)
	}
	if isIndependentSeedance && !info.SeedanceSubmissionPrepared {
		if info.BeforeSeedanceGenerationSubmit == nil {
			return nil, service.TaskErrorWrapperLocal(
				errors.New("Seedance submission persistence hook is unavailable"),
				"persist_task_failed", http.StatusInternalServerError,
			)
		}
		if err := info.BeforeSeedanceGenerationSubmit(); err != nil {
			return nil, service.TaskErrorWrapperLocal(err, "persist_task_failed", http.StatusInternalServerError)
		}
		info.SeedanceSubmissionPrepared = true
	}

	// Ratios are known before the asynchronous request is sent. Emit the header
	// here so an accepted Seedance intent has the same public response metadata
	// even when the Ark response is lost.
	otherRatios := info.PriceData.OtherRatios
	if otherRatios == nil {
		otherRatios = map[string]float64{}
	}
	ratiosJSON, _ := common.Marshal(otherRatios)
	c.Header("X-New-Api-Other-Ratios", string(ratiosJSON))

	// 9. 发送请求
	resp, err := adaptor.DoRequest(c, info, requestBody)
	if err != nil {
		if isIndependentSeedance && info.SeedanceSubmissionPrepared {
			if markErr := service.MarkSeedanceGenerationSubmissionOutcomeUnknown(info.PublicTaskID, "ark create transport outcome unknown"); markErr != nil {
				common.SysError("mark Seedance generation submission unknown: " + markErr.Error())
			}
			return deferredIndependentSeedanceResult(info, platform, info.PriceData.Quota, "", nil)
		}
		return nil, service.TaskErrorWrapper(err, "do_request_failed", http.StatusInternalServerError)
	}
	if resp == nil {
		if isIndependentSeedance && info.SeedanceSubmissionPrepared {
			if markErr := service.MarkSeedanceGenerationSubmissionOutcomeUnknown(info.PublicTaskID, "ark create empty response outcome unknown"); markErr != nil {
				common.SysError("mark Seedance generation empty response unknown: " + markErr.Error())
			}
			return deferredIndependentSeedanceResult(info, platform, info.PriceData.Quota, "", nil)
		}
		return nil, service.TaskErrorWrapper(errors.New("upstream returned an empty response"), "empty_response", http.StatusBadGateway)
	}
	if !isSuccessfulTaskSubmitStatus(resp.StatusCode) {
		responseBody, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		isAIPDD := platform == constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeAIPDD))
		if isIndependentSeedance && info.SeedanceSubmissionPrepared {
			if isDefinitiveArkCreateRejection(resp.StatusCode) {
				failPreparedIndependentSeedanceSubmission(c, info.PublicTaskID, "ark create definitively rejected")
				return nil, independentSeedanceCreateRejectionError(responseBody, resp.StatusCode)
			}
			if markErr := service.MarkSeedanceGenerationSubmissionOutcomeUnknown(info.PublicTaskID, "ark create HTTP outcome unknown"); markErr != nil {
				common.SysError("mark Seedance generation HTTP response unknown: " + markErr.Error())
			}
			return deferredIndependentSeedanceResult(info, platform, info.PriceData.Quota, "", nil)
		}
		return nil, taskErrorFromUpstreamResponse(responseBody, resp.StatusCode, isAIPDD)
	}

	// 11. 解析响应
	upstreamTaskID, taskData, taskErr := adaptor.DoResponse(c, resp, info)
	if taskErr != nil {
		if isIndependentSeedance && info.SeedanceSubmissionPrepared {
			if markErr := service.MarkSeedanceGenerationSubmissionOutcomeUnknown(info.PublicTaskID, "ark create response outcome unknown"); markErr != nil {
				common.SysError("mark Seedance generation response unknown: " + markErr.Error())
			}
			return deferredIndependentSeedanceResult(info, platform, info.PriceData.Quota, "", nil)
		}
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
		CallbackURL:    info.SeedanceCallbackURL,
		TaskPersisted:  isIndependentSeedance && info.SeedanceSubmissionPrepared,
	}
	if isIndependentSeedance {
		if _, responseErr := populateIndependentSeedanceDeferredResult(result, info); responseErr != nil {
			return nil, responseErr
		}
	}
	if provider, ok := adaptor.(channel.AIPDDTaskSnapshotProvider); ok {
		result.AIPDDExecution = provider.AIPDDTaskSnapshot(info)
	}
	return result, nil
}

func deferredIndependentSeedanceResult(info *relaycommon.RelayInfo, platform constant.TaskPlatform, quota int, upstreamTaskID string, taskData []byte) (*TaskSubmitResult, *dto.TaskError) {
	result := &TaskSubmitResult{
		UpstreamTaskID: upstreamTaskID,
		TaskData:       taskData,
		Platform:       platform,
		Quota:          quota,
		CallbackURL:    info.SeedanceCallbackURL,
		TaskPersisted:  true,
	}
	return populateIndependentSeedanceDeferredResult(result, info)
}

func populateIndependentSeedanceDeferredResult(result *TaskSubmitResult, info *relaycommon.RelayInfo) (*TaskSubmitResult, *dto.TaskError) {
	result.DeferredContentType = "application/json"
	var err error
	if relayconstant.IsSeedanceOfficialTasksPath(info.RequestURLPath) {
		result.DeferredHTTPStatus = http.StatusOK
		result.DeferredBody, err = common.Marshal(map[string]any{"id": info.PublicTaskID})
	} else {
		publicVideo := dto.NewOpenAIVideo()
		publicVideo.ID = info.PublicTaskID
		publicVideo.TaskID = info.PublicTaskID
		publicVideo.CreatedAt = time.Now().Unix()
		publicVideo.Model = info.OriginModelName
		result.DeferredHTTPStatus = http.StatusAccepted
		result.DeferredBody, err = common.Marshal(publicVideo)
	}
	if err != nil {
		return nil, service.TaskErrorWrapper(err, "build_response_failed", http.StatusInternalServerError)
	}
	return result, nil
}

func failPreparedIndependentSeedanceSubmission(ctx context.Context, taskID string, evidence string) {
	var task model.Task
	if err := model.DB.Where("task_id = ?", taskID).First(&task).Error; err != nil {
		common.SysError("load prepared Seedance task for failure: " + err.Error())
		return
	}
	if _, err := service.FailSeedanceWorkflow(ctx, &task, evidence); err != nil {
		common.SysError("fail prepared Seedance generation submission: " + err.Error())
	}
}

func isSuccessfulTaskSubmitStatus(statusCode int) bool {
	return statusCode >= http.StatusOK && statusCode < http.StatusMultipleChoices
}

// Ark's content-generation create API does not expose an idempotency token.
// A response proves non-acceptance only when it is a client-side rejection;
// request timeout remains ambiguous because the server may already have acted.
func isDefinitiveArkCreateRejection(statusCode int) bool {
	return statusCode >= http.StatusBadRequest &&
		statusCode < http.StatusInternalServerError &&
		statusCode != http.StatusRequestTimeout
}

func independentSeedanceCreateRejectionError(responseBody []byte, statusCode int) *dto.TaskError {
	taskErr := taskErrorFromUpstreamResponse(responseBody, statusCode)
	taskErr.Data = nil
	switch statusCode {
	case http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound:
		// These statuses describe the server-owned Ark credential/model mapping,
		// not the caller's NewAPI authentication or public model name.
		taskErr.Code = "video_service_unavailable"
		taskErr.Message = "Video service is temporarily unavailable"
		taskErr.StatusCode = http.StatusServiceUnavailable
	case http.StatusTooManyRequests:
		taskErr.Code = "video_service_busy"
		taskErr.Message = "Video service is temporarily busy"
	default:
		taskErr.Code = "video_request_rejected"
		taskErr.Message = "The video request was rejected"
	}
	return taskErr
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
	relayconstant.RelayModeVideoFetchByID: taskFetchByIDRespBodyBuilder,
	relayconstant.RelayModeTaskFetchByID:  taskFetchByIDRespBodyBuilder,
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

func taskFetchByIDRespBodyBuilder(c *gin.Context) (respBody []byte, taskResp *dto.TaskError) {
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
		statusCode := http.StatusBadRequest
		if relayconstant.IsSeedanceOfficialTasksPath(c.Request.URL.Path) {
			statusCode = http.StatusNotFound
		}
		taskResp = service.TaskErrorWrapperLocal(errors.New("task_not_exist"), "task_not_exist", statusCode)
		return
	}

	isOpenAIVideoAPI := strings.HasPrefix(c.Request.RequestURI, "/v1/videos/")
	isSeedanceOfficialAPI := relayconstant.IsSeedanceOfficialTasksPath(c.Request.URL.Path)
	if (isOpenAIVideoAPI || isSeedanceOfficialAPI) && originTask.Platform == constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeSeedance)) {
		order, visibilityErr := model.GetVisibleSeedanceOrderByTaskID(originTask.TaskID)
		expectedProtocol := model.SeedanceProtocolOpenAI
		if isSeedanceOfficialAPI {
			expectedProtocol = model.SeedanceProtocolOfficial
		}
		if visibilityErr != nil || order.PublicProtocol != expectedProtocol {
			taskResp = service.TaskErrorWrapperLocal(errors.New("task_not_exist"), "task_not_exist", http.StatusNotFound)
			return
		}
	}
	if isSeedanceOfficialAPI && !isSeedanceOfficialTask(originTask) {
		taskResp = service.TaskErrorWrapperLocal(
			fmt.Errorf("task is not a Seedance official task"),
			"invalid_endpoint",
			http.StatusBadRequest,
		)
		return
	}
	mediaInfo := resolveTaskMediaInfo(originTask, c.Request.URL.Path)

	// Gemini/Vertex/AIPDD 支持实时查询：用户 fetch 时直接从上游拉取最新状态
	if realtimeResp := tryRealtimeFetch(c.Request.Context(), originTask, isOpenAIVideoAPI, isSeedanceOfficialAPI, mediaInfo.MediaType); len(realtimeResp) > 0 {
		respBody = realtimeResp
		return
	}

	if isSeedanceOfficialAPI {
		adaptor := GetTaskAdaptor(originTask.Platform)
		if adaptor == nil {
			taskResp = service.TaskErrorWrapperLocal(fmt.Errorf("invalid channel id: %d", originTask.ChannelId), "invalid_channel_id", http.StatusBadRequest)
			return
		}
		converter, ok := adaptor.(channel.SeedanceOfficialTaskConverter)
		if !ok {
			taskResp = service.TaskErrorWrapperLocal(fmt.Errorf("not_implemented:%s", originTask.Platform), "not_implemented", http.StatusNotImplemented)
			return
		}
		respBody, err = converter.ConvertToSeedanceOfficialTask(originTask)
		if err != nil {
			taskResp = service.TaskErrorWrapper(err, "convert_to_seedance_official_failed", http.StatusInternalServerError)
		}
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
func tryRealtimeFetch(ctx context.Context, task *model.Task, isOpenAIVideoAPI, isSeedanceOfficialAPI bool, mediaType string) []byte {
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
		service.SyncAIPDDTaskFinanceFromUpstream(task, body, ti)
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
	applyTaskResultURL(task, ti.Url, mediaType)

	transitionWon := false
	if !snap.Equal(task.Snapshot()) {
		transitionWon, _ = task.UpdateWithStatus(snap.Status)
	}
	if transitionWon && ti.Status == model.TaskStatusSuccess {
		// A realtime GET can observe the terminal response before the background
		// poller. Settle the frozen task-pricing snapshot here after winning the
		// same status CAS, otherwise the 30-second auto-duration precharge would
		// remain permanently consumed and the poller would skip the terminal row.
		service.SettleTaskBillingOnComplete(ctx, adaptor, task, ti)
	}
	if ti.Status == model.TaskStatusSuccess {
		// Usage log synchronization is intentionally independent of the status
		// CAS so querying an already-completed legacy task can backfill the log.
		service.SyncTaskEquivalentUsageLog(ctx, task, ti)
	}

	// Compatible video APIs are converted from the persisted upstream snapshot
	// by their dedicated response builders after this realtime refresh.
	if isOpenAIVideoAPI || isSeedanceOfficialAPI {
		return nil
	}

	// 非 OpenAI Video API: 按任务真实媒体类型构建自定义格式响应。
	out := buildTaskFetchData(task, ti, body, mediaType)
	respBody, _ := common.Marshal(dto.TaskResponse[any]{
		Code: "success",
		Data: out,
	})
	return respBody
}

func isSeedanceOfficialTask(task *model.Task) bool {
	if task == nil {
		return false
	}
	if task.Platform != constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeAIPDD)) {
		if task.Platform == constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeSeedance)) {
			order, err := model.GetVisibleSeedanceOrderByTaskID(task.TaskID)
			return err == nil && order.PublicProtocol == model.SeedanceProtocolOfficial
		}
		adaptor := GetTaskAdaptor(task.Platform)
		if adaptor == nil {
			return false
		}
		_, ok := adaptor.(channel.SeedanceOfficialTaskConverter)
		return ok
	}
	if snapshot := task.PrivateData.AIPDDExecution; snapshot != nil && strings.TrimSpace(snapshot.Protocol) != "" {
		return strings.EqualFold(strings.TrimSpace(snapshot.Protocol), "seedance_official")
	}
	modelName := strings.TrimSpace(task.Properties.UpstreamModelName)
	if modelName == "" {
		modelName = strings.TrimSpace(task.Properties.OriginModelName)
	}
	capability, ok := constant.GetAIPDDCapability(modelName)
	return ok && strings.EqualFold(strings.TrimSpace(capability.ExecutionProtocol), "seedance_official")
}

func applyTaskResultURL(task *model.Task, upstreamURL, mediaType string) {
	if task == nil {
		return
	}
	upstreamURL = strings.TrimSpace(upstreamURL)
	if strings.HasPrefix(upstreamURL, "data:") {
		// data: URI is already retained in Data and should not be persisted as a
		// proxy target.
		return
	}
	if upstreamURL != "" {
		task.PrivateData.ResultURL = upstreamURL
		return
	}
	if task.Status == model.TaskStatusSuccess &&
		mediaType == "video" &&
		strings.TrimSpace(task.GetResultURL()) == "" {
		// Only video tasks have a /v1/videos/{id}/content proxy endpoint.
		task.PrivateData.ResultURL = taskcommon.BuildProxyURL(task.TaskID)
	}
}

func buildTaskFetchData(task *model.Task, taskInfo *relaycommon.TaskInfo, rawBody []byte, mediaType string) map[string]any {
	output := extractTaskOutputURLs(task)
	var taskError any
	if task.Status == model.TaskStatusFailure {
		taskError = strings.TrimSpace(task.FailReason)
		if taskError == "" && taskInfo != nil {
			taskError = strings.TrimSpace(taskInfo.Reason)
		}
		if taskError == "" {
			taskError = "AIPDD task failed"
		}
	}

	resultURL := ""
	if task.Status == model.TaskStatusSuccess && len(output) > 0 {
		resultURL = output[0]
	}
	out := map[string]any{
		"error":    taskError,
		"metadata": map[string]any{"urls": output},
		"output":   output,
		"status":   mapTaskStatusToSimple(task.Status),
		"task_id":  task.TaskID,
		"url":      resultURL,
	}
	if task.Status == model.TaskStatusSuccess {
		if format := detectTaskFormat(rawBody, output, mediaType); format != "" {
			out["format"] = format
		}
	}
	return out
}

func detectTaskFormat(rawBody []byte, output []string, mediaType string) string {
	mediaType = normalizeTaskMediaType(mediaType)
	if mediaType == "" {
		return ""
	}
	var raw any
	if len(rawBody) > 0 && common.Unmarshal(rawBody, &raw) == nil {
		if format := detectFormatFromValue(raw, mediaType); format != "" {
			return format
		}
	}
	for _, value := range output {
		if format := normalizeMediaFormat(value, mediaType); format != "" {
			return format
		}
	}
	if mediaType == "video" {
		// Preserve the historical OpenAI-video fallback, but only for tasks
		// positively identified as video and only after they succeed.
		return "mp4"
	}
	return ""
}

func detectFormatFromValue(value any, mediaType string) string {
	switch typed := value.(type) {
	case string:
		return normalizeMediaFormat(typed, mediaType)
	case []any:
		for _, item := range typed {
			if format := detectFormatFromValue(item, mediaType); format != "" {
				return format
			}
		}
	case map[string]any:
		for key, item := range typed {
			normalizedKey := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(key), "_", ""))
			switch normalizedKey {
			case "mimetype", "contenttype", "format":
				if format := detectFormatFromValue(item, mediaType); format != "" {
					return format
				}
			}
		}
		for _, item := range typed {
			if format := detectFormatFromValue(item, mediaType); format != "" {
				return format
			}
		}
	}
	return ""
}

func normalizeMediaFormat(value, mediaType string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	lower := strings.ToLower(value)
	if strings.HasPrefix(lower, "data:") {
		if separator := strings.IndexByte(lower, ';'); separator >= 0 {
			lower = strings.TrimPrefix(lower[:separator], "data:")
		}
	}
	if parsed, err := neturl.Parse(value); err == nil {
		if ext := strings.TrimPrefix(strings.ToLower(path.Ext(parsed.Path)), "."); ext != "" {
			if normalized := normalizeMediaFormatToken(ext, mediaType); normalized != "" {
				return normalized
			}
		}
		for _, key := range []string{"content-type", "content_type", "mime-type", "mime_type"} {
			if normalized := normalizeMediaFormatToken(parsed.Query().Get(key), mediaType); normalized != "" {
				return normalized
			}
		}
	}
	return normalizeMediaFormatToken(lower, mediaType)
}

func normalizeMediaFormatToken(value, mediaType string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if separator := strings.IndexByte(value, ';'); separator >= 0 {
		value = strings.TrimSpace(value[:separator])
	}
	if strings.Contains(value, "/") {
		parts := strings.SplitN(value, "/", 2)
		if parts[0] != mediaType {
			return ""
		}
		value = parts[1]
	}
	value = strings.TrimPrefix(value, "x-")
	switch mediaType {
	case "image":
		switch value {
		case "jpg", "jpeg", "pjpeg":
			return "jpeg"
		case "png", "webp", "gif", "avif", "bmp", "tiff", "svg", "svg+xml":
			return strings.TrimSuffix(value, "+xml")
		}
	case "video":
		switch value {
		case "quicktime":
			return "mov"
		case "mp4", "webm", "mov", "mkv", "avi", "m4v", "mpeg":
			return value
		}
	case "audio":
		switch value {
		case "mpeg":
			return "mp3"
		case "mp3", "wav", "wave", "flac", "ogg", "opus", "aac", "m4a":
			if value == "wave" {
				return "wav"
			}
			return value
		}
	}
	return ""
}

type taskMediaInfo struct {
	EndpointType     constant.EndpointType
	MediaType        string
	TaskKind         string
	OutputModalities []string
}

func resolveTaskMediaInfo(task *model.Task, requestPath string) taskMediaInfo {
	info := taskMediaInfo{}
	if task == nil {
		return info
	}

	if snapshot := task.PrivateData.AIPDDExecution; snapshot != nil {
		info.EndpointType = snapshot.EndpointType
		info.MediaType = normalizeTaskMediaType(snapshot.MediaType)
		info.TaskKind = strings.TrimSpace(snapshot.TaskKind)
		info.OutputModalities = append([]string(nil), snapshot.OutputModalities...)
	}

	// The polling route is the strongest compatibility signal for old tasks
	// whose execution snapshot predates endpoint/media persistence.
	if info.EndpointType == "" {
		info.EndpointType = endpointTypeFromTaskPath(requestPath)
	}

	for _, modelName := range []string{task.Properties.OriginModelName, task.Properties.UpstreamModelName} {
		if strings.TrimSpace(modelName) == "" {
			continue
		}
		capability, ok := constant.GetAIPDDCapability(modelName)
		if !ok {
			continue
		}
		if info.EndpointType == "" {
			info.EndpointType = capability.EndpointType
		}
		if info.TaskKind == "" {
			info.TaskKind = strings.TrimSpace(capability.TaskKind)
		}
		if len(info.OutputModalities) == 0 {
			info.OutputModalities = append([]string(nil), capability.OutputModalities...)
		}
		break
	}

	if info.MediaType == "" {
		info.MediaType = mediaTypeFromEndpoint(info.EndpointType)
	}
	if info.MediaType == "" {
		info.MediaType = mediaTypeFromTaskKind(info.TaskKind)
	}
	if info.MediaType == "" {
		for _, modality := range info.OutputModalities {
			if mediaType := normalizeTaskMediaType(modality); mediaType != "" {
				info.MediaType = mediaType
				break
			}
		}
	}
	if info.EndpointType == "" {
		info.EndpointType = endpointTypeFromMediaType(info.MediaType)
	}
	if len(info.OutputModalities) == 0 && info.MediaType != "" {
		info.OutputModalities = []string{info.MediaType}
	}
	return info
}

func endpointTypeFromTaskPath(requestPath string) constant.EndpointType {
	switch {
	case strings.HasPrefix(requestPath, "/v1/images/edits/"),
		strings.HasPrefix(requestPath, "/pg/images/edits/"):
		return constant.EndpointTypeImageEdit
	case strings.HasPrefix(requestPath, "/v1/images/generations/"),
		strings.HasPrefix(requestPath, "/pg/images/generations/"):
		return constant.EndpointTypeImageGeneration
	case strings.HasPrefix(requestPath, "/v1/audio/speech/"),
		strings.HasPrefix(requestPath, "/pg/audio/speech/"):
		return constant.EndpointTypeAudioSpeech
	case strings.HasPrefix(requestPath, "/v1/videos/"),
		strings.HasPrefix(requestPath, "/pg/videos/"),
		strings.HasPrefix(requestPath, "/pg/video/generations/"):
		return constant.EndpointTypeOpenAIVideo
	default:
		return ""
	}
}

func mediaTypeFromEndpoint(endpointType constant.EndpointType) string {
	switch endpointType {
	case constant.EndpointTypeImageGeneration, constant.EndpointTypeImageToImage, constant.EndpointTypeImageEdit:
		return "image"
	case constant.EndpointTypeOpenAIVideo:
		return "video"
	case constant.EndpointTypeAudioSpeech:
		return "audio"
	default:
		return ""
	}
}

func endpointTypeFromMediaType(mediaType string) constant.EndpointType {
	switch normalizeTaskMediaType(mediaType) {
	case "image":
		return constant.EndpointTypeImageGeneration
	case "video":
		return constant.EndpointTypeOpenAIVideo
	case "audio":
		return constant.EndpointTypeAudioSpeech
	default:
		return ""
	}
}

func mediaTypeFromTaskKind(taskKind string) string {
	normalized := strings.NewReplacer("-", "_", " ", "_").Replace(strings.ToLower(strings.TrimSpace(taskKind)))
	switch {
	case strings.HasSuffix(normalized, "_to_image"), strings.Contains(normalized, "image_generation"):
		return "image"
	case strings.HasSuffix(normalized, "_to_video"), strings.Contains(normalized, "video_generation"):
		return "video"
	case strings.HasSuffix(normalized, "_to_speech"),
		strings.Contains(normalized, "audio_generation"),
		strings.Contains(normalized, "speech_generation"):
		return "audio"
	default:
		return ""
	}
}

func normalizeTaskMediaType(mediaType string) string {
	switch strings.ToLower(strings.TrimSpace(mediaType)) {
	case "image":
		return "image"
	case "video":
		return "video"
	case "audio":
		return "audio"
	default:
		return ""
	}
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
	isIndependentSeedance := task != nil && task.Platform == constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeSeedance))
	output := extractTaskOutputURLs(task)
	mediaInfo := resolveTaskMediaInfo(task, "")
	failReason := task.FailReason
	taskData := task.Data
	if task.Status == model.TaskStatusFailure {
		errorMessage := strings.TrimSpace(failReason + "\n" + string(taskData))
		var publicError relaycommon.PublicUpstreamTaskError
		isAIPDDTask := task.Platform == constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeAIPDD)) ||
			task.PrivateData.AIPDDExecution != nil
		if isAIPDDTask {
			publicError = relaycommon.NormalizeAIPDDTaskError(
				"",
				errorMessage,
				"",
				"",
				0,
				relaycommon.AIPDDTaskErrorOperationExecute,
			)
		} else {
			publicError = relaycommon.NormalizeUpstreamTaskError("", errorMessage, "", "")
		}
		if publicError.Matched {
			failReason = publicError.Message
			errorData := map[string]any{
				"code":    publicError.Code,
				"message": publicError.Message,
			}
			for key, value := range publicError.Data() {
				errorData[key] = value
			}
			if sanitizedData, err := common.Marshal(map[string]any{
				"status": "failed",
				"error":  errorData,
			}); err == nil {
				taskData = sanitizedData
			}
		}
	}
	taskData = sanitizePublicTaskData(taskData)
	resultURL := ""
	if task.Status == model.TaskStatusSuccess {
		resultURL = task.GetResultURL()
	}
	properties := task.Properties
	if isIndependentSeedance {
		// The independent Seedance workflow intentionally keeps the real media
		// location in Task.PrivateData so the authenticated content proxy can
		// fetch it. Generic user task DTOs must never project that private URL (or
		// arbitrary persisted provider payloads) back to the caller.
		resultURL = ""
		output = nil
		if task.Status == model.TaskStatusSuccess {
			resultURL = taskcommon.BuildProxyURL(task.TaskID)
			output = []string{resultURL}
		}
		if task.Status == model.TaskStatusFailure {
			failReason = "视频处理失败，请稍后重试"
		}
		properties.UpstreamModelName = properties.OriginModelName
		taskData = independentSeedanceUserTaskData(task, resultURL)
	}
	var metadata map[string]any
	if len(output) > 0 {
		metadata = map[string]any{
			"url":  output[0],
			"urls": output,
		}
	}
	quotaCNY := task.GetQuotaCNY()
	return &dto.TaskDto{
		ID:               task.ID,
		CreatedAt:        task.CreatedAt,
		UpdatedAt:        task.UpdatedAt,
		TaskID:           task.TaskID,
		Platform:         string(task.Platform),
		UserId:           task.UserId,
		Group:            task.Group,
		ChannelId:        task.ChannelId,
		Quota:            task.Quota,
		QuotaCNY:         quotaCNY,
		CostCNY:          quotaCNY,
		Currency:         "CNY",
		Action:           task.Action,
		Status:           string(task.Status),
		FailReason:       failReason,
		ResultURL:        resultURL,
		SubmitTime:       task.SubmitTime,
		StartTime:        task.StartTime,
		FinishTime:       task.FinishTime,
		Progress:         task.Progress,
		Properties:       properties,
		Username:         task.Username,
		Output:           output,
		Metadata:         metadata,
		EndpointType:     string(mediaInfo.EndpointType),
		MediaType:        mediaInfo.MediaType,
		TaskKind:         mediaInfo.TaskKind,
		OutputModalities: mediaInfo.OutputModalities,
		Data:             taskData,
	}
}

func independentSeedanceUserTaskData(task *model.Task, resultURL string) json.RawMessage {
	payload := map[string]any{
		"id":     task.TaskID,
		"model":  task.Properties.OriginModelName,
		"status": mapTaskStatusToSimple(task.Status),
	}
	if task.Progress != "" {
		payload["progress"] = task.Progress
	}
	switch task.Status {
	case model.TaskStatusSuccess:
		payload["content"] = map[string]any{"video_url": resultURL}
	case model.TaskStatusFailure:
		payload["error"] = map[string]any{
			"code":    "video_processing_failed",
			"message": "视频处理失败，请稍后重试",
		}
	}
	data, err := common.Marshal(payload)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return data
}

var publicTaskPrivateMoneyFields = map[string]struct{}{
	"cost":                   {},
	"task_cost":              {},
	"draw_user_reward":       {},
	"unit_price_usd":         {},
	"sale_usd":               {},
	"base_usd":               {},
	"usd_per_awcoin":         {},
	"estimated_awcoin":       {},
	"cost_awcoin_per_second": {},
	"costawcoinpersecond":    {},
}

func removePrivateTaskMoneyFields(value any) {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			normalizedKey := strings.ToLower(strings.TrimSpace(key))
			if _, private := publicTaskPrivateMoneyFields[normalizedKey]; private ||
				strings.HasSuffix(normalizedKey, "_usd") ||
				strings.HasPrefix(normalizedKey, "finance_") ||
				strings.HasPrefix(normalizedKey, "billing_") {
				delete(typed, key)
				continue
			}
			removePrivateTaskMoneyFields(child)
		}
	case []any:
		for _, child := range typed {
			removePrivateTaskMoneyFields(child)
		}
	}
}

func sanitizePublicTaskData(data json.RawMessage) json.RawMessage {
	if len(data) == 0 {
		return data
	}
	var payload any
	if err := common.Unmarshal(data, &payload); err != nil {
		return data
	}
	removePrivateTaskMoneyFields(payload)
	sanitized, err := common.Marshal(payload)
	if err != nil {
		return data
	}
	return sanitized
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
