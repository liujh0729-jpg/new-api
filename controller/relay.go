package controller

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	perfmetrics "github.com/QuantumNous/new-api/pkg/perf_metrics"
	"github.com/QuantumNous/new-api/relay"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/types"

	"github.com/bytedance/gopkg/util/gopool"
	"github.com/samber/lo"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

func relayHandler(c *gin.Context, info *relaycommon.RelayInfo) *types.NewAPIError {
	var err *types.NewAPIError
	switch info.RelayMode {
	case relayconstant.RelayModeImagesGenerations, relayconstant.RelayModeImagesEdits:
		err = relay.ImageHelper(c, info)
	case relayconstant.RelayModeAudioSpeech:
		fallthrough
	case relayconstant.RelayModeAudioTranslation:
		fallthrough
	case relayconstant.RelayModeAudioTranscription:
		err = relay.AudioHelper(c, info)
	case relayconstant.RelayModeRerank:
		err = relay.RerankHelper(c, info)
	case relayconstant.RelayModeEmbeddings:
		err = relay.EmbeddingHelper(c, info)
	case relayconstant.RelayModeResponses, relayconstant.RelayModeResponsesCompact:
		err = relay.ResponsesHelper(c, info)
	default:
		err = relay.TextHelper(c, info)
	}
	return err
}

func geminiRelayHandler(c *gin.Context, info *relaycommon.RelayInfo) *types.NewAPIError {
	var err *types.NewAPIError
	if strings.Contains(c.Request.URL.Path, "embed") {
		err = relay.GeminiEmbeddingHandler(c, info)
	} else {
		err = relay.GeminiHelper(c, info)
	}
	return err
}

func Relay(c *gin.Context, relayFormat types.RelayFormat) {

	requestId := c.GetString(common.RequestIdKey)
	//group := common.GetContextKeyString(c, constant.ContextKeyUsingGroup)
	//originalModel := common.GetContextKeyString(c, constant.ContextKeyOriginalModel)

	var (
		newAPIError *types.NewAPIError
		ws          *websocket.Conn
	)

	if relayFormat == types.RelayFormatOpenAIRealtime {
		var err error
		ws, err = upgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			helper.WssError(c, ws, types.NewError(err, types.ErrorCodeGetChannelFailed, types.ErrOptionWithSkipRetry()).ToOpenAIError())
			return
		}
		defer ws.Close()
	}

	defer func() {
		if newAPIError != nil {
			logger.LogError(c, fmt.Sprintf("relay error: %s", newAPIError.Error()))
			newAPIError.SetMessage(common.MessageWithRequestId(newAPIError.Error(), requestId))
			switch relayFormat {
			case types.RelayFormatOpenAIRealtime:
				helper.WssError(c, ws, newAPIError.ToOpenAIError())
			case types.RelayFormatClaude:
				c.JSON(newAPIError.StatusCode, gin.H{
					"type":  "error",
					"error": newAPIError.ToClaudeError(),
				})
			default:
				c.JSON(newAPIError.StatusCode, gin.H{
					"error": newAPIError.ToOpenAIError(),
				})
			}
		}
	}()

	request, err := helper.GetAndValidateRequest(c, relayFormat)
	if err != nil {
		// Map "request body too large" to 413 so clients can handle it correctly
		if common.IsRequestBodyTooLargeError(err) || errors.Is(err, common.ErrRequestBodyTooLarge) {
			newAPIError = types.NewErrorWithStatusCode(err, types.ErrorCodeReadRequestBodyFailed, http.StatusRequestEntityTooLarge, types.ErrOptionWithSkipRetry())
		} else {
			newAPIError = types.NewError(err, types.ErrorCodeInvalidRequest)
		}
		return
	}

	relayInfo, err := relaycommon.GenRelayInfo(c, relayFormat, request, ws)
	if err != nil {
		newAPIError = types.NewError(err, types.ErrorCodeGenRelayInfoFailed)
		return
	}

	needSensitiveCheck := setting.ShouldCheckPromptSensitive()
	needCountToken := constant.CountToken
	// Avoid building huge CombineText (strings.Join) when token counting and sensitive check are both disabled.
	var meta *types.TokenCountMeta
	if needSensitiveCheck || needCountToken {
		meta = request.GetTokenCountMeta()
	} else {
		meta = fastTokenCountMetaForPricing(request)
	}

	if needSensitiveCheck && meta != nil {
		contains, words := service.CheckSensitiveText(meta.CombineText)
		if contains {
			logger.LogWarn(c, fmt.Sprintf("user sensitive words detected: %s", strings.Join(words, ", ")))
			newAPIError = types.NewError(err, types.ErrorCodeSensitiveWordsDetected)
			return
		}
	}

	tokens, err := service.EstimateRequestToken(c, meta, relayInfo)
	if err != nil {
		newAPIError = types.NewError(err, types.ErrorCodeCountTokenFailed)
		return
	}

	relayInfo.SetEstimatePromptTokens(tokens)

	priceData, err := helper.ModelPriceHelper(c, relayInfo, tokens, meta)
	if err != nil {
		newAPIError = types.NewError(err, types.ErrorCodeModelPriceError, types.ErrOptionWithStatusCode(http.StatusBadRequest))
		return
	}

	// common.SetContextKey(c, constant.ContextKeyTokenCountMeta, meta)

	if priceData.FreeModel {
		logger.LogInfo(c, fmt.Sprintf("模型 %s 免费，跳过预扣费", relayInfo.OriginModelName))
	} else {
		newAPIError = service.PreConsumeBilling(c, priceData.QuotaToPreConsume, relayInfo)
		if newAPIError != nil {
			return
		}
	}

	defer func() {
		// Only return quota if downstream failed and quota was actually pre-consumed
		if newAPIError != nil {
			newAPIError = service.NormalizeViolationFeeError(newAPIError)
			if relayInfo.Billing != nil {
				relayInfo.Billing.Refund(c)
			} else if financeErr := service.RecordAIPDDFinanceSettlement(relayInfo, 0, "NOT_CHARGED"); financeErr != nil {
				common.SysError("record failed AIPDD finance order error: " + financeErr.Error())
			}
			service.ChargeViolationFeeIfNeeded(c, relayInfo, newAPIError)
		}
	}()

	retryParam := &service.RetryParam{
		Ctx:        c,
		TokenGroup: relayInfo.TokenGroup,
		ModelName:  relayInfo.OriginModelName,
		Retry:      common.GetPointer(0),
	}
	relayInfo.RetryIndex = 0
	relayInfo.LastError = nil

	for ; retryParam.GetRetry() <= common.RetryTimes; retryParam.IncreaseRetry() {
		relayInfo.RetryIndex = retryParam.GetRetry()
		channel, channelErr := getChannel(c, relayInfo, retryParam)
		if channelErr != nil {
			logger.LogError(c, channelErr.Error())
			newAPIError = channelErr
			break
		}

		addUsedChannel(c, channel.Id)
		if financeErr := service.PrepareAIPDDFinanceAttempt(c, relayInfo); financeErr != nil {
			newAPIError = types.NewError(financeErr, types.ErrorCodeQueryDataError)
			break
		}
		bodyStorage, bodyErr := common.GetBodyStorage(c)
		if bodyErr != nil {
			// Ensure consistent 413 for oversized bodies even when error occurs later (e.g., retry path)
			if common.IsRequestBodyTooLargeError(bodyErr) || errors.Is(bodyErr, common.ErrRequestBodyTooLarge) {
				newAPIError = types.NewErrorWithStatusCode(bodyErr, types.ErrorCodeReadRequestBodyFailed, http.StatusRequestEntityTooLarge, types.ErrOptionWithSkipRetry())
			} else {
				newAPIError = types.NewErrorWithStatusCode(bodyErr, types.ErrorCodeReadRequestBodyFailed, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
			}
			break
		}
		c.Request.Body = io.NopCloser(bodyStorage)

		switch relayFormat {
		case types.RelayFormatOpenAIRealtime:
			newAPIError = relay.WssHelper(c, relayInfo)
		case types.RelayFormatClaude:
			newAPIError = relay.ClaudeHelper(c, relayInfo)
		case types.RelayFormatGemini:
			newAPIError = geminiRelayHandler(c, relayInfo)
		default:
			newAPIError = relayHandler(c, relayInfo)
		}

		if newAPIError == nil {
			relayInfo.LastError = nil
			return
		}

		newAPIError = service.NormalizeViolationFeeError(newAPIError)
		relayInfo.LastError = newAPIError

		processChannelError(c, *types.NewChannelError(channel.Id, channel.Type, channel.Name, channel.ChannelInfo.IsMultiKey, common.GetContextKeyString(c, constant.ContextKeyChannelKey), channel.GetAutoBan()), newAPIError)

		if !shouldRetry(c, newAPIError, common.RetryTimes-retryParam.GetRetry()) {
			break
		}
	}

	useChannel := c.GetStringSlice("use_channel")
	if len(useChannel) > 1 {
		retryLogStr := fmt.Sprintf("重试：%s", strings.Trim(strings.Join(strings.Fields(fmt.Sprint(useChannel)), "->"), "[]"))
		logger.LogInfo(c, retryLogStr)
	}
	if newAPIError != nil {
		gopool.Go(func() {
			perfmetrics.RecordRelaySample(relayInfo, false, 0)
		})
	}
}

var upgrader = websocket.Upgrader{
	Subprotocols: []string{"realtime"}, // WS 握手支持的协议，如果有使用 Sec-WebSocket-Protocol，则必须在此声明对应的 Protocol TODO add other protocol
	CheckOrigin: func(r *http.Request) bool {
		return true // 允许跨域
	},
}

func addUsedChannel(c *gin.Context, channelId int) {
	useChannel := c.GetStringSlice("use_channel")
	useChannel = append(useChannel, fmt.Sprintf("%d", channelId))
	c.Set("use_channel", useChannel)
}

func fastTokenCountMetaForPricing(request dto.Request) *types.TokenCountMeta {
	if request == nil {
		return &types.TokenCountMeta{}
	}
	meta := &types.TokenCountMeta{
		TokenType: types.TokenTypeTokenizer,
	}
	switch r := request.(type) {
	case *dto.GeneralOpenAIRequest:
		maxCompletionTokens := lo.FromPtrOr(r.MaxCompletionTokens, uint(0))
		maxTokens := lo.FromPtrOr(r.MaxTokens, uint(0))
		if maxCompletionTokens > maxTokens {
			meta.MaxTokens = int(maxCompletionTokens)
		} else {
			meta.MaxTokens = int(maxTokens)
		}
	case *dto.OpenAIResponsesRequest:
		meta.MaxTokens = int(lo.FromPtrOr(r.MaxOutputTokens, uint(0)))
	case *dto.ClaudeRequest:
		meta.MaxTokens = int(lo.FromPtr(r.MaxTokens))
	case *dto.ImageRequest:
		// Pricing for image requests depends on ImagePriceRatio; safe to compute even when CountToken is disabled.
		return r.GetTokenCountMeta()
	default:
		// Best-effort: leave CombineText empty to avoid large allocations.
	}
	return meta
}

func getChannel(c *gin.Context, info *relaycommon.RelayInfo, retryParam *service.RetryParam) (*model.Channel, *types.NewAPIError) {
	if info.ChannelMeta == nil {
		autoBan := c.GetBool("auto_ban")
		autoBanInt := 1
		if !autoBan {
			autoBanInt = 0
		}
		return &model.Channel{
			Id:      c.GetInt("channel_id"),
			Type:    c.GetInt("channel_type"),
			Name:    c.GetString("channel_name"),
			AutoBan: &autoBanInt,
		}, nil
	}
	channel, selectGroup, err := service.CacheGetRandomSatisfiedChannel(retryParam)

	info.PriceData.GroupRatioInfo = helper.HandleGroupRatio(c, info)

	if err != nil {
		return nil, types.NewError(fmt.Errorf("获取分组 %s 下模型 %s 的可用渠道失败（retry）: %s", selectGroup, info.OriginModelName, err.Error()), types.ErrorCodeGetChannelFailed, types.ErrOptionWithSkipRetry())
	}
	if channel == nil {
		return nil, types.NewError(fmt.Errorf("分组 %s 下模型 %s 的可用渠道不存在（retry）", selectGroup, info.OriginModelName), types.ErrorCodeGetChannelFailed, types.ErrOptionWithSkipRetry())
	}

	newAPIError := middleware.SetupContextForSelectedChannel(c, channel, info.OriginModelName)
	if newAPIError != nil {
		return nil, newAPIError
	}
	return channel, nil
}

func shouldRetry(c *gin.Context, openaiErr *types.NewAPIError, retryTimes int) bool {
	if openaiErr == nil {
		return false
	}
	if service.ShouldSkipRetryAfterChannelAffinityFailure(c) {
		return false
	}
	if types.IsChannelError(openaiErr) {
		return true
	}
	if types.IsSkipRetryError(openaiErr) {
		return false
	}
	if retryTimes <= 0 {
		return false
	}
	if _, ok := c.Get("specific_channel_id"); ok {
		return false
	}
	code := openaiErr.StatusCode
	if code >= 200 && code < 300 {
		return false
	}
	if code < 100 || code > 599 {
		return true
	}
	if operation_setting.IsAlwaysSkipRetryCode(openaiErr.GetErrorCode()) {
		return false
	}
	return operation_setting.ShouldRetryByStatusCode(code)
}

func processChannelError(c *gin.Context, channelError types.ChannelError, err *types.NewAPIError) {
	logger.LogError(c, fmt.Sprintf("channel error (channel #%d, status code: %d): %s", channelError.ChannelId, err.StatusCode, err.Error()))
	// 不要使用context获取渠道信息，异步处理时可能会出现渠道信息不一致的情况
	// do not use context to get channel info, there may be inconsistent channel info when processing asynchronously
	if service.ShouldDisableChannel(err) && channelError.AutoBan {
		gopool.Go(func() {
			service.DisableChannel(channelError, err.ErrorWithStatusCode())
		})
	}

	if constant.ErrorLogEnabled && types.IsRecordErrorLog(err) {
		// 保存错误日志到mysql中
		userId := c.GetInt("id")
		tokenName := c.GetString("token_name")
		modelName := c.GetString("original_model")
		tokenId := c.GetInt("token_id")
		userGroup := c.GetString("group")
		channelId := c.GetInt("channel_id")
		other := make(map[string]interface{})
		if c.Request != nil && c.Request.URL != nil {
			other["request_path"] = c.Request.URL.Path
		}
		other["error_type"] = err.GetErrorType()
		other["error_code"] = err.GetErrorCode()
		other["status_code"] = err.StatusCode
		other["channel_id"] = channelId
		other["channel_name"] = c.GetString("channel_name")
		other["channel_type"] = c.GetInt("channel_type")
		adminInfo := make(map[string]interface{})
		adminInfo["use_channel"] = c.GetStringSlice("use_channel")
		isMultiKey := common.GetContextKeyBool(c, constant.ContextKeyChannelIsMultiKey)
		if isMultiKey {
			adminInfo["is_multi_key"] = true
			adminInfo["multi_key_index"] = common.GetContextKeyInt(c, constant.ContextKeyChannelMultiKeyIndex)
		}
		service.AppendChannelAffinityAdminInfo(c, adminInfo)
		other["admin_info"] = adminInfo
		startTime := common.GetContextKeyTime(c, constant.ContextKeyRequestStartTime)
		if startTime.IsZero() {
			startTime = time.Now()
		}
		useTimeSeconds := int(time.Since(startTime).Seconds())
		model.RecordErrorLog(c, userId, channelId, modelName, tokenName, err.MaskSensitiveErrorWithStatusCode(), tokenId, useTimeSeconds, common.GetContextKeyBool(c, constant.ContextKeyIsStream), userGroup, other)
	}

}

func RelayMidjourney(c *gin.Context) {
	relayInfo, err := relaycommon.GenRelayInfo(c, types.RelayFormatMjProxy, nil, nil)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"description": fmt.Sprintf("failed to generate relay info: %s", err.Error()),
			"type":        "upstream_error",
			"code":        4,
		})
		return
	}

	var mjErr *dto.MidjourneyResponse
	switch relayInfo.RelayMode {
	case relayconstant.RelayModeMidjourneyNotify:
		mjErr = relay.RelayMidjourneyNotify(c)
	case relayconstant.RelayModeMidjourneyTaskFetch, relayconstant.RelayModeMidjourneyTaskFetchByCondition:
		mjErr = relay.RelayMidjourneyTask(c, relayInfo.RelayMode)
	case relayconstant.RelayModeMidjourneyTaskImageSeed:
		mjErr = relay.RelayMidjourneyTaskImageSeed(c)
	case relayconstant.RelayModeSwapFace:
		mjErr = relay.RelaySwapFace(c, relayInfo)
	default:
		mjErr = relay.RelayMidjourneySubmit(c, relayInfo)
	}
	//err = relayMidjourneySubmit(c, relayMode)
	log.Println(mjErr)
	if mjErr != nil {
		statusCode := http.StatusBadRequest
		if mjErr.Code == 30 {
			mjErr.Result = "当前分组负载已饱和，请稍后再试，或升级账户以提升服务质量。"
			statusCode = http.StatusTooManyRequests
		}
		c.JSON(statusCode, gin.H{
			"description": fmt.Sprintf("%s %s", mjErr.Description, mjErr.Result),
			"type":        "upstream_error",
			"code":        mjErr.Code,
		})
		channelId := c.GetInt("channel_id")
		logger.LogError(c, fmt.Sprintf("relay error (channel #%d, status code %d): %s", channelId, statusCode, fmt.Sprintf("%s %s", mjErr.Description, mjErr.Result)))
	}
}

func RelayNotImplemented(c *gin.Context) {
	err := types.OpenAIError{
		Message: "API not implemented",
		Type:    "new_api_error",
		Param:   "",
		Code:    "api_not_implemented",
	}
	c.JSON(http.StatusNotImplemented, gin.H{
		"error": err,
	})
}

func RelayNotFound(c *gin.Context) {
	err := types.OpenAIError{
		Message: fmt.Sprintf("Invalid URL (%s %s)", c.Request.Method, c.Request.URL.Path),
		Type:    "invalid_request_error",
		Param:   "",
		Code:    "",
	}
	c.JSON(http.StatusNotFound, gin.H{
		"error": err,
	})
}

func RelayTaskFetch(c *gin.Context) {
	relayInfo, err := relaycommon.GenRelayInfo(c, types.RelayFormatTask, nil, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, &dto.TaskError{
			Code:       "gen_relay_info_failed",
			Message:    err.Error(),
			StatusCode: http.StatusInternalServerError,
		})
		return
	}
	if taskErr := relay.RelayTaskFetch(c, relayInfo.RelayMode); taskErr != nil {
		respondTaskError(c, taskErr)
	}
}

func RelayTask(c *gin.Context) {
	relayInfo, err := relaycommon.GenRelayInfo(c, types.RelayFormatTask, nil, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, &dto.TaskError{
			Code:       "gen_relay_info_failed",
			Message:    err.Error(),
			StatusCode: http.StatusInternalServerError,
		})
		return
	}

	if taskErr := relay.ResolveOriginTask(c, relayInfo); taskErr != nil {
		respondTaskError(c, taskErr)
		return
	}
	if abortIfOriginTaskModelForbidden(c, relayInfo) {
		return
	}

	var boundCharacter *model.VirtualCharacter
	characterLinkCreated := false
	characterTaskReady := false
	if item, bound := middleware.GetBoundVirtualCharacter(c); bound {
		boundCharacter = item
		if taskID, exists := middleware.GetVirtualCharacterTaskID(c); exists {
			relayInfo.PublicTaskID = taskID
			characterLinkCreated = true
		} else {
			relayInfo.PublicTaskID = model.GenerateTaskID()
		}
		if !characterLinkCreated {
			link := &model.VirtualCharacterTask{
				TaskID:          relayInfo.PublicTaskID,
				UserID:          relayInfo.UserId,
				CharacterID:     item.ID,
				CharacterName:   item.Name,
				CharacterScope:  item.Scope,
				ProviderAssetID: item.ProviderAssetID,
			}
			if createErr := model.CreateVirtualCharacterTaskLink(link); createErr != nil {
				respondTaskError(c, service.TaskErrorWrapperLocal(createErr, "character_task_link_failed", http.StatusInternalServerError))
				return
			}
			characterLinkCreated = true
		}
		c.Set(middleware.VirtualCharacterTaskClaimedKey, true)
	}

	var result *relay.TaskSubmitResult
	var taskErr *dto.TaskError
	defer func() {
		if taskErr != nil {
			if characterLinkCreated {
				_ = model.MarkVirtualCharacterTaskFailed(relayInfo.PublicTaskID, taskErr.Message)
				if boundCharacter != nil &&
					boundCharacter.SourceType != model.VirtualCharacterSourceVolcRealPerson &&
					boundCharacter.SourceType != model.VirtualCharacterSourceVolcAIGC &&
					service.IsVirtualCharacterRealPersonRejection(taskErr.Message) {
					_ = model.MarkVirtualCharacterBlocked(boundCharacter.ID, taskErr.Message)
				}
			}
			if relayInfo.Billing != nil && !relayInfo.SeedanceSubmissionPrepared {
				relayInfo.Billing.Refund(c)
			} else if relayInfo.Billing == nil && !relayInfo.SeedanceSubmissionPrepared {
				if financeErr := service.RecordAIPDDFinanceSettlement(relayInfo, 0, "NOT_CHARGED"); financeErr != nil {
					common.SysError("record failed AIPDD task finance order error: " + financeErr.Error())
				}
			}
		}
	}()
	var preparedTask *model.Task
	relayInfo.BeforeSeedanceGenerationSubmit = func() error {
		taskAction := relayInfo.Action
		if boundCharacter != nil {
			taskAction = model.VirtualCharacterTaskAction
		}
		var prepareErr error
		preparedTask, prepareErr = prepareTaskAndWorkflowOrder(relayInfo, taskAction)
		return prepareErr
	}

	retryParam := &service.RetryParam{
		Ctx:        c,
		TokenGroup: relayInfo.TokenGroup,
		ModelName:  relayInfo.OriginModelName,
		Retry:      common.GetPointer(0),
	}

	for ; retryParam.GetRetry() <= common.RetryTimes; retryParam.IncreaseRetry() {
		relayInfo.RetryIndex = retryParam.GetRetry()
		var channel *model.Channel

		if lockedCh, ok := relayInfo.LockedChannel.(*model.Channel); ok && lockedCh != nil {
			channel = lockedCh
			if retryParam.GetRetry() > 0 {
				if setupErr := middleware.SetupContextForSelectedChannel(c, channel, relayInfo.OriginModelName); setupErr != nil {
					taskErr = service.TaskErrorWrapperLocal(setupErr.Err, "setup_locked_channel_failed", http.StatusInternalServerError)
					break
				}
			}
		} else {
			var channelErr *types.NewAPIError
			channel, channelErr = getChannel(c, relayInfo, retryParam)
			if channelErr != nil {
				logger.LogError(c, channelErr.Error())
				taskErr = service.TaskErrorWrapperLocal(channelErr.Err, "get_channel_failed", http.StatusInternalServerError)
				break
			}
		}

		addUsedChannel(c, channel.Id)
		if financeErr := service.PrepareAIPDDFinanceAttempt(c, relayInfo); financeErr != nil {
			taskErr = service.TaskErrorWrapperLocal(financeErr, "prepare_aipdd_finance_failed", http.StatusInternalServerError)
			break
		}
		bodyStorage, bodyErr := common.GetBodyStorage(c)
		if bodyErr != nil {
			if common.IsRequestBodyTooLargeError(bodyErr) || errors.Is(bodyErr, common.ErrRequestBodyTooLarge) {
				taskErr = service.TaskErrorWrapperLocal(bodyErr, "read_request_body_failed", http.StatusRequestEntityTooLarge)
			} else {
				taskErr = service.TaskErrorWrapperLocal(bodyErr, "read_request_body_failed", http.StatusBadRequest)
			}
			break
		}
		c.Request.Body = io.NopCloser(bodyStorage)

		result, taskErr = relay.RelayTaskSubmit(c, relayInfo)
		if taskErr == nil {
			break
		}

		if !taskErr.LocalError {
			processChannelError(c,
				*types.NewChannelError(channel.Id, channel.Type, channel.Name, channel.ChannelInfo.IsMultiKey,
					common.GetContextKeyString(c, constant.ContextKeyChannelKey), channel.GetAutoBan()),
				types.NewOpenAIError(taskErr.Error, types.ErrorCodeBadResponseStatusCode, taskErr.StatusCode))
		}
		if relayInfo.SeedanceSubmissionPrepared {
			// Ark video creation has no idempotency token. Once the durable attempt
			// may have reached Ark, never let the generic channel retry loop submit
			// it a second time.
			break
		}

		if !shouldRetryTaskRelay(c, channel.Id, taskErr, common.RetryTimes-retryParam.GetRetry()) {
			break
		}
	}

	useChannel := c.GetStringSlice("use_channel")
	if len(useChannel) > 1 {
		retryLogStr := fmt.Sprintf("重试：%s", strings.Trim(strings.Join(strings.Fields(fmt.Sprint(useChannel)), "->"), "[]"))
		logger.LogInfo(c, retryLogStr)
	}

	// ── 成功：结算 + 日志 + 插入任务 ──
	// The independent Seedance channel defers its public acknowledgement until
	// the task, private order and first attempt have committed atomically. This
	// also keeps the pre-consumption refundable if the local transaction fails.
	deferredResponse := taskErr == nil && result != nil && result.DeferredHTTPStatus != 0
	if taskErr == nil {
		if !deferredResponse {
			if settleErr := service.SettleBilling(c, relayInfo, result.Quota); settleErr != nil {
				common.SysError("settle task billing error: " + settleErr.Error())
			}
			service.LogTaskConsumption(c, relayInfo)
		}

		if result.TaskPersisted {
			task := preparedTask
			var finalizeErr error
			if task == nil {
				var persisted model.Task
				finalizeErr = model.DB.Where("task_id = ?", relayInfo.PublicTaskID).First(&persisted).Error
				if finalizeErr == nil {
					task = &persisted
				}
			}
			if finalizeErr == nil && strings.TrimSpace(result.UpstreamTaskID) != "" {
				finalizeErr = service.ConfirmSeedanceGenerationSubmission(task, result.UpstreamTaskID, result.TaskData)
			}
			if finalizeErr == nil && boundCharacter != nil && strings.TrimSpace(result.UpstreamTaskID) != "" {
				payload, marshalErr := common.Marshal(task)
				if marshalErr != nil {
					finalizeErr = marshalErr
				} else if readyErr := model.MarkVirtualCharacterTaskReady(task.TaskID, result.UpstreamTaskID, task.ChannelId, string(payload)); readyErr != nil {
					finalizeErr = readyErr
				} else {
					characterTaskReady = true
					_ = model.MarkVirtualCharacterTaskActive(task.TaskID)
				}
			}
			if finalizeErr != nil {
				common.SysError("finalize prepared Seedance task error: " + finalizeErr.Error())
				// The public task/order/attempt already committed before Ark was
				// called. Returning an HTTP error here invites the client to create a
				// second logical task even though Ark may have accepted the first one.
				// Keep the durable acknowledgement and let timeout/manual recovery
				// reconcile the missing confirmation without another Ark submission.
				if markErr := service.MarkSeedanceGenerationSubmissionOutcomeUnknown(relayInfo.PublicTaskID, "ark confirmation persistence outcome unknown"); markErr != nil {
					common.SysError("mark Seedance generation confirmation unknown: " + markErr.Error())
				}
			}
		} else {
			task := model.InitTask(result.Platform, relayInfo)
			task.PrivateData.UpstreamTaskID = result.UpstreamTaskID
			task.PrivateData.LogRequestID = strings.TrimSpace(relayInfo.RequestId)
			task.PrivateData.BillingSource = relayInfo.BillingSource
			task.PrivateData.SubscriptionId = relayInfo.SubscriptionId
			task.PrivateData.SubscriptionPreConsumed = relayInfo.SubscriptionPreConsumed
			task.PrivateData.TokenId = relayInfo.TokenId
			task.PrivateData.BillingContext = &model.TaskBillingContext{
				ModelPrice:      relayInfo.PriceData.ModelPrice,
				GroupRatio:      relayInfo.PriceData.GroupRatioInfo.GroupRatio,
				ModelRatio:      relayInfo.PriceData.ModelRatio,
				OtherRatios:     relayInfo.PriceData.OtherRatios,
				OriginModelName: relayInfo.OriginModelName,
				PerCallBilling:  isTaskPerCallBilling(relayInfo),
				QuotaPerUnit:    common.QuotaPerUnit,
				USDExchangeRate: operation_setting.USDExchangeRate,
			}
			if quote := relayInfo.TaskPricingQuote; quote != nil {
				task.PrivateData.BillingContext.PerCallBilling = false
				task.PrivateData.BillingContext.GroupRatio = quote.GroupRatio
				task.PrivateData.BillingContext.BillingMode = billing_setting.BillingModeTaskPricing
				task.PrivateData.BillingContext.BillingUnit = quote.Unit
				task.PrivateData.BillingContext.PricingVariant = quote.Variant
				task.PrivateData.BillingContext.UnitPriceUSD = quote.UnitPriceUSD
				task.PrivateData.BillingContext.Quantity = quote.Quantity
				task.PrivateData.BillingContext.SaleUSD = quote.SaleUSD
				task.PrivateData.BillingContext.HasReferenceVideo = quote.HasReferenceVideo
				task.PrivateData.BillingContext.Resolution = quote.Resolution
			}
			task.PrivateData.AIPDDExecution = result.AIPDDExecution
			task.PrivateData.AIPDDFinance = relayInfo.AIPDDFinance
			task.Quota = result.Quota
			task.Data = result.TaskData
			task.Action = relayInfo.Action
			if boundCharacter != nil {
				task.Action = model.VirtualCharacterTaskAction
				payload, marshalErr := common.Marshal(task)
				if marshalErr != nil {
					common.SysError("marshal virtual character task recovery payload: " + marshalErr.Error())
				} else if readyErr := model.MarkVirtualCharacterTaskReady(task.TaskID, result.UpstreamTaskID, task.ChannelId, string(payload)); readyErr != nil {
					common.SysError("mark virtual character task ready: " + readyErr.Error())
				} else {
					characterTaskReady = true
				}
			}
			if insertErr := task.Insert(); insertErr != nil {
				common.SysError("insert task error: " + insertErr.Error())
				if boundCharacter != nil && !characterTaskReady {
					// A ready link is recovered by the maintenance worker. If the
					// recovery payload could not be recorded, surface the local gap.
					_ = model.MarkVirtualCharacterTaskFailed(task.TaskID, insertErr.Error())
				}
				if deferredResponse {
					taskErr = service.TaskErrorWrapperLocal(insertErr, "persist_task_failed", http.StatusInternalServerError)
				}
			} else if boundCharacter != nil {
				_ = model.MarkVirtualCharacterTaskActive(task.TaskID)
			}
		}
		if deferredResponse && taskErr == nil {
			if settleErr := service.SettleBilling(c, relayInfo, result.Quota); settleErr != nil {
				common.SysError("settle task billing error: " + settleErr.Error())
			}
			service.LogTaskConsumption(c, relayInfo)
			c.Data(result.DeferredHTTPStatus, result.DeferredContentType, result.DeferredBody)
		}
	}

	if taskErr != nil {
		respondTaskError(c, taskErr)
	}
}

// abortIfOriginTaskModelForbidden validates remix/continuation requests after
// ResolveOriginTask has loaded the user-owned task and resolved its model.
// Read-only task fetches never pass through RelayTask and therefore remain
// governed solely by token authentication and task ownership.
func abortIfOriginTaskModelForbidden(c *gin.Context, info *relaycommon.RelayInfo) bool {
	if info == nil || info.TaskRelayInfo == nil || info.OriginTaskID == "" {
		return false
	}
	return middleware.AbortIfTokenModelForbidden(c, info.OriginModelName)
}

func isTaskPerCallBilling(relayInfo *relaycommon.RelayInfo) bool {
	if relayInfo == nil {
		return false
	}
	if relayInfo.TaskPricingQuote != nil || billing_setting.GetBillingMode(relayInfo.OriginModelName) == billing_setting.BillingModeTaskPricing {
		return true
	}
	if constant.IsAIPDDTaskModel(relayInfo.OriginModelName) {
		return constant.IsAIPDDPerCallBillingModel(relayInfo.OriginModelName) ||
			constant.IsAIPDDExactBillingModel(relayInfo.OriginModelName)
	}
	return common.StringsContains(constant.TaskPricePatches, relayInfo.OriginModelName) || relayInfo.PriceData.UsePrice
}

const (
	taskUpstreamOverloadedMessage    = "当前分组上游负载已饱和，请稍后再试"
	taskUpstreamConfigChangedMessage = "后台配置变更请联系系统管理员"
)

// respondTaskError 统一输出 Task 错误响应（含 429 / 上游余额不足等提示改写）。
// 改写只影响返回给客户端的 Message；taskErr.Error 仍保留上游原文，供管理端错误日志排查。
func respondTaskError(c *gin.Context, taskErr *dto.TaskError) {
	publicErrorNormalized := normalizePublicTaskError(taskErr)
	if !publicErrorNormalized && taskErr.StatusCode == http.StatusTooManyRequests {
		taskErr.Message = taskUpstreamOverloadedMessage
	} else if !publicErrorNormalized && shouldMaskUpstreamBalanceTaskError(taskErr) {
		taskErr.Message = taskUpstreamConfigChangedMessage
	}
	c.JSON(taskErr.StatusCode, taskErr)
}

// normalizePublicTaskError applies documented Seedance messages at the final
// response boundary and prevents internal super-resolution model details from
// leaking even if an upstream adapter did not normalize them earlier.
func normalizePublicTaskError(taskErr *dto.TaskError) bool {
	if taskErr == nil {
		return false
	}
	if normalizeIndependentSeedanceLocalError(taskErr) {
		return true
	}
	rawError := ""
	if taskErr.Error != nil {
		rawError = taskErr.Error.Error()
	}
	rawData := ""
	if taskErr.Data != nil {
		rawData = fmt.Sprint(taskErr.Data)
	}
	isSuperResolutionError := relaycommon.IsSuperResolutionTaskError(taskErr.Code, taskErr.Message, rawError, rawData)
	if taskErr.LocalError && !isSuperResolutionError {
		return false
	}
	// AIPDD adapters attach provider/category/retryable when an error has
	// already passed through the public taxonomy. Preserve that message instead
	// of replacing it with the generic 429 or balance fallback below.
	if !isSuperResolutionError && isNormalizedAIPDDTaskError(taskErr) {
		return true
	}
	publicError := relaycommon.NormalizeUpstreamTaskError(
		taskErr.Code,
		strings.TrimSpace(taskErr.Message+"\n"+rawError+"\n"+rawData),
		"",
		"",
	)
	if !publicError.Matched {
		return false
	}

	taskErr.Code = publicError.Code
	taskErr.Message = publicError.Message
	if publicError.HideRaw {
		taskErr.Data = nil
	}
	if safeData := publicError.Data(); len(safeData) > 0 {
		if existing, ok := taskErr.Data.(map[string]any); ok && !publicError.HideRaw {
			for key, value := range safeData {
				existing[key] = value
			}
			taskErr.Data = existing
		} else {
			taskErr.Data = safeData
		}
	}
	return true
}

func normalizeIndependentSeedanceLocalError(taskErr *dto.TaskError) bool {
	if taskErr == nil || !taskErr.LocalError {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(taskErr.Code)) {
	case "seedance_model_not_published":
		taskErr.Code = "model_not_found"
		taskErr.Message = "The requested video model is unavailable"
	case "seedance_credential_unavailable", "seedance_channel_unavailable",
		"seedance_model_unavailable", "seedance_pricing_unavailable":
		taskErr.Code = "video_service_unavailable"
		taskErr.Message = "Video service is temporarily unavailable"
	case "persist_task_failed", "build_response_failed":
		taskErr.Code = "server_error"
		taskErr.Message = "Failed to create video task"
	default:
		return false
	}
	taskErr.Data = nil
	return true
}

func isNormalizedAIPDDTaskError(taskErr *dto.TaskError) bool {
	if taskErr == nil || !strings.HasPrefix(strings.ToLower(strings.TrimSpace(taskErr.Code)), "aipdd_") {
		return false
	}
	details, ok := taskErr.Data.(map[string]any)
	if !ok {
		return false
	}
	provider, ok := details["provider"].(string)
	return ok && strings.EqualFold(strings.TrimSpace(provider), "aipdd")
}

// shouldMaskUpstreamBalanceTaskError 识别上游渠道账户余额/积分不足（非本站用户额度不足）。
// 典型场景：AIPDD 返回 402 + 「余额不足…AWCoin…请充值后再创建任务」。
func shouldMaskUpstreamBalanceTaskError(taskErr *dto.TaskError) bool {
	if taskErr == nil || taskErr.LocalError {
		return false
	}
	// Prefer structured local codes even if LocalError was not set by the producer.
	code := strings.ToLower(strings.TrimSpace(taskErr.Code))
	if code == string(types.ErrorCodeInsufficientUserQuota) ||
		code == string(types.ErrorCodePreConsumeTokenQuotaFailed) ||
		strings.Contains(code, "insufficient_user_quota") ||
		strings.Contains(code, "pre_consume_token_quota") {
		return false
	}
	if taskErr.StatusCode == http.StatusPaymentRequired {
		return true
	}

	message := strings.TrimSpace(taskErr.Message)
	if message == "" {
		return false
	}
	lower := strings.ToLower(message)

	// AIPDD / AWCoin 上游积分不足
	if strings.Contains(lower, "awcoin") &&
		(strings.Contains(message, "余额不足") ||
			strings.Contains(lower, "insufficient") ||
			strings.Contains(message, "请充值")) {
		return true
	}
	if strings.Contains(message, "请充值后再创建任务") ||
		strings.Contains(lower, "please recharge") ||
		strings.Contains(lower, "please top up") {
		return true
	}

	// 其它上游常见余额文案（排除本站本地额度提示）
	if strings.Contains(message, "用户额度不足") ||
		strings.Contains(message, "订阅额度不足") ||
		strings.Contains(lower, "user quota is not enough") ||
		strings.Contains(lower, "token quota is not enough") ||
		strings.Contains(lower, "pre_consume") ||
		strings.Contains(lower, "insufficient_user_quota") {
		return false
	}

	return strings.Contains(lower, "insufficient account balance") ||
		strings.Contains(lower, "account balance is insufficient") ||
		strings.Contains(lower, "credit balance is too low") ||
		strings.Contains(lower, "not enough credits") ||
		strings.Contains(lower, "out of credit") ||
		(strings.Contains(message, "余额不足") && !strings.Contains(message, "用户")) ||
		strings.Contains(message, "余额已耗尽")
}

func shouldRetryTaskRelay(c *gin.Context, channelId int, taskErr *dto.TaskError, retryTimes int) bool {
	if taskErr == nil {
		return false
	}
	if service.ShouldSkipRetryAfterChannelAffinityFailure(c) {
		return false
	}
	if retryTimes <= 0 {
		return false
	}
	if _, ok := c.Get("specific_channel_id"); ok {
		return false
	}
	if taskErr.StatusCode == http.StatusTooManyRequests {
		return true
	}
	if taskErr.StatusCode == 307 {
		return true
	}
	if taskErr.StatusCode/100 == 5 {
		// 超时不重试
		if operation_setting.IsAlwaysSkipRetryStatusCode(taskErr.StatusCode) {
			return false
		}
		return true
	}
	if taskErr.StatusCode == http.StatusBadRequest {
		return false
	}
	if taskErr.StatusCode == 408 {
		// azure处理超时不重试
		return false
	}
	if taskErr.LocalError {
		return false
	}
	if taskErr.StatusCode/100 == 2 {
		return false
	}
	return true
}
