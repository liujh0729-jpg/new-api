package common

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

const (
	PublicTaskProcessingFailedCode    = "task_processing_failed"
	PublicTaskProcessingFailedMessage = "任务处理失败，请稍后重试；如持续失败，请联系客服。"

	AIPDDErrorCodeInvalidRequest        = "aipdd_invalid_request"
	AIPDDErrorCodeContentPolicy         = "aipdd_content_policy_violation"
	AIPDDErrorCodeAuthenticationFailed  = "aipdd_authentication_failed"
	AIPDDErrorCodePermissionDenied      = "aipdd_permission_denied"
	AIPDDErrorCodeResourceNotFound      = "aipdd_resource_not_found"
	AIPDDErrorCodeRequestConflict       = "aipdd_request_conflict"
	AIPDDErrorCodeRateLimited           = "aipdd_rate_limited"
	AIPDDErrorCodeUpstreamTimeout       = "aipdd_upstream_timeout"
	AIPDDErrorCodeUpstreamUnavailable   = "aipdd_upstream_unavailable"
	AIPDDErrorCodeUpstreamConfiguration = "aipdd_upstream_configuration_error"
	AIPDDErrorCodeTaskCreateFailed      = "aipdd_task_create_failed"
	AIPDDErrorCodeTaskQueryFailed       = "aipdd_task_query_failed"
	AIPDDErrorCodeTaskFailed            = "aipdd_task_failed"
)

type AIPDDTaskErrorOperation string

const (
	AIPDDTaskErrorOperationCreate  AIPDDTaskErrorOperation = "create"
	AIPDDTaskErrorOperationQuery   AIPDDTaskErrorOperation = "query"
	AIPDDTaskErrorOperationExecute AIPDDTaskErrorOperation = "execute"
)

var (
	seedanceContentParamPattern = regexp.MustCompile(`(?i)\bcontent\[(\d+)]`)
	seedanceRequestIDPattern    = regexp.MustCompile(`(?i)\brequest\s*id\s*:\s*([a-z0-9_-]+)`)
)

// PublicUpstreamTaskError is the client-safe representation of an upstream
// task error. Matched indicates that a stable public mapping was found.
// HideRaw is set for internal implementation errors that must never be exposed
// to clients, while the original error can still be retained in logs/storage.
type PublicUpstreamTaskError struct {
	Code         string
	Message      string
	Param        string
	RequestID    string
	Provider     string
	Category     string
	UpstreamCode string
	Retryable    bool
	Matched      bool
	HideRaw      bool
}

// Data returns optional structured fields that are safe to expose to clients.
func (e PublicUpstreamTaskError) Data() map[string]any {
	data := make(map[string]any, 7)
	if e.Provider != "" {
		data["provider"] = e.Provider
		data["category"] = e.Category
		data["retryable"] = e.Retryable
	}
	if e.UpstreamCode != "" && !e.HideRaw && !strings.EqualFold(e.UpstreamCode, e.Code) {
		data["upstream_code"] = e.UpstreamCode
	}
	if e.Param != "" {
		data["param"] = e.Param
	}
	if e.RequestID != "" {
		data["request_id"] = e.RequestID
	}
	return data
}

// RetryableValue returns an explicit retryable flag for normalized provider
// errors and nil for legacy errors that were not provider-normalized.
func (e PublicUpstreamTaskError) RetryableValue() *bool {
	if e.Provider == "" {
		return nil
	}
	retryable := e.Retryable
	return &retryable
}

// NormalizeAIPDDTaskError wraps every AIPDD upstream error in a stable public
// taxonomy while retaining documented Seedance codes when available.
func NormalizeAIPDDTaskError(
	code, message, param, requestID string,
	statusCode int,
	operation AIPDDTaskErrorOperation,
) PublicUpstreamTaskError {
	rawCode := strings.TrimSpace(code)
	base := NormalizeUpstreamTaskError(rawCode, message, param, requestID)
	if base.Matched {
		base.Provider = "aipdd"
		base.Category, base.Retryable = aipddCategoryAndRetryable(base.Code)
		if !base.HideRaw && rawCode != "" && !strings.EqualFold(rawCode, base.Code) {
			base.UpstreamCode = rawCode
		}
		return base
	}

	effectiveStatus := statusCode
	if parsed, err := strconv.Atoi(rawCode); err == nil && parsed >= 400 && parsed <= 599 {
		effectiveStatus = parsed
	}
	lowerCode := strings.ToLower(rawCode)
	lowerMessage := strings.ToLower(strings.TrimSpace(message))
	publicCode := ""
	publicMessage := ""
	category := ""
	retryable := false
	hideRaw := false

	switch {
	case isAIPDDUpstreamBalanceError(lowerCode, lowerMessage):
		publicCode = AIPDDErrorCodeUpstreamConfiguration
		publicMessage = "上游服务配置异常，请联系系统管理员。"
		category = "upstream_configuration"
		hideRaw = true
	case strings.Contains(lowerCode, "content_policy") || strings.Contains(lowerMessage, "content policy"):
		publicCode = AIPDDErrorCodeContentPolicy
		publicMessage = "输入内容未通过上游内容安全审核，请调整后重试。"
		category = "content_safety"
	case effectiveStatus == 400 || effectiveStatus == 422 ||
		strings.Contains(lowerCode, "invalidparameter") || strings.Contains(lowerCode, "missingparameter"):
		publicCode = AIPDDErrorCodeInvalidRequest
		publicMessage = "请求参数不正确，请检查输入内容和参数后重试。"
		category = "invalid_request"
	case effectiveStatus == 401:
		publicCode = AIPDDErrorCodeAuthenticationFailed
		publicMessage = "上游服务鉴权失败，请联系系统管理员。"
		category = "authentication"
		hideRaw = true
	case effectiveStatus == 403:
		publicCode = AIPDDErrorCodePermissionDenied
		publicMessage = "上游服务权限不足，请联系系统管理员。"
		category = "permission"
		hideRaw = true
	case effectiveStatus == 404:
		publicCode = AIPDDErrorCodeResourceNotFound
		publicMessage = "所请求的上游模型或资源暂不可用，请联系系统管理员。"
		category = "resource"
	case effectiveStatus == 408 || strings.Contains(lowerCode, "timeout") || strings.Contains(lowerMessage, "timed out"):
		publicCode = AIPDDErrorCodeUpstreamTimeout
		publicMessage = "上游任务处理超时，请稍后重试。"
		category = "timeout"
		retryable = true
		hideRaw = true
	case effectiveStatus == 409:
		publicCode = AIPDDErrorCodeRequestConflict
		publicMessage = "当前请求与已有任务冲突，请检查任务状态后重试。"
		category = "conflict"
	case effectiveStatus == 429:
		publicCode = AIPDDErrorCodeRateLimited
		publicMessage = "上游请求过于频繁，请稍后重试。"
		category = "rate_limit"
		retryable = true
	case effectiveStatus >= 500:
		publicCode = AIPDDErrorCodeUpstreamUnavailable
		publicMessage = "上游服务暂时不可用，请稍后重试。"
		category = "upstream_unavailable"
		retryable = true
		hideRaw = true
	default:
		switch operation {
		case AIPDDTaskErrorOperationCreate:
			publicCode = AIPDDErrorCodeTaskCreateFailed
			publicMessage = "任务创建失败，请检查输入内容后重试。"
			category = "task_create"
		case AIPDDTaskErrorOperationQuery:
			publicCode = AIPDDErrorCodeTaskQueryFailed
			publicMessage = "任务状态查询失败，请稍后重试。"
			category = "task_query"
		default:
			publicCode = AIPDDErrorCodeTaskFailed
			publicMessage = "任务处理失败，请调整输入内容后重试。"
			category = "task_execution"
		}
		hideRaw = true
	}

	upstreamCode := rawCode
	if hideRaw || strings.EqualFold(upstreamCode, publicCode) {
		upstreamCode = ""
	}
	return PublicUpstreamTaskError{
		Code:         publicCode,
		Message:      appendRequestID(publicMessage, base.RequestID),
		Param:        base.Param,
		RequestID:    base.RequestID,
		Provider:     "aipdd",
		Category:     category,
		UpstreamCode: upstreamCode,
		Retryable:    retryable,
		Matched:      true,
		HideRaw:      hideRaw,
	}
}

// NormalizeUpstreamTaskError translates documented Seedance error codes into
// actionable Chinese messages. It also masks errors from internal
// super-resolution/upscaling stages so implementation details are not exposed.
func NormalizeUpstreamTaskError(code, message, param, requestID string) PublicUpstreamTaskError {
	code = strings.TrimSpace(code)
	message = strings.TrimSpace(message)
	param = firstNonEmptyString(strings.TrimSpace(param), extractSeedanceContentParam(message))
	requestID = firstNonEmptyString(strings.TrimSpace(requestID), extractSeedanceRequestID(message))

	if IsSuperResolutionTaskError(code, message) {
		return PublicUpstreamTaskError{
			Code:      PublicTaskProcessingFailedCode,
			Message:   appendRequestID(PublicTaskProcessingFailedMessage, requestID),
			RequestID: requestID,
			Matched:   true,
			HideRaw:   true,
		}
	}

	canonicalCode := canonicalSeedanceErrorCode(code, message)
	if canonicalCode == "" {
		return PublicUpstreamTaskError{
			Code:      code,
			Message:   message,
			Param:     param,
			RequestID: requestID,
		}
	}

	publicMessage := seedancePublicErrorMessage(canonicalCode, param)
	if publicMessage == "" {
		return PublicUpstreamTaskError{
			Code:      code,
			Message:   message,
			Param:     param,
			RequestID: requestID,
		}
	}

	return PublicUpstreamTaskError{
		Code:      canonicalCode,
		Message:   appendRequestID(publicMessage, requestID),
		Param:     param,
		RequestID: requestID,
		Matched:   true,
	}
}

// IsSuperResolutionTaskError reports whether an error exposes an internal
// super-resolution/upscaling model or processing stage.
func IsSuperResolutionTaskError(values ...string) bool {
	joined := strings.ToLower(strings.Join(values, " "))
	compact := strings.NewReplacer("-", "", "_", "", ".", "", " ", "").Replace(joined)
	return strings.Contains(compact, "seedvr") ||
		strings.Contains(compact, "superresolution") ||
		strings.Contains(compact, "upscale") ||
		strings.Contains(compact, "upscaler") ||
		strings.Contains(compact, "upscaling") ||
		strings.Contains(joined, "超分")
}

func aipddCategoryAndRetryable(code string) (string, bool) {
	lowerCode := strings.ToLower(code)
	switch {
	case strings.Contains(lowerCode, "policyviolation"):
		return "copyright_policy", false
	case strings.Contains(lowerCode, "privacyinformation"):
		return "privacy", false
	case strings.Contains(lowerCode, "deepfake"):
		return "deepfake", false
	case strings.Contains(lowerCode, "sensitivecontentdetected"):
		return "content_safety", false
	case strings.Contains(lowerCode, "tasktypeconstraint"), strings.Contains(lowerCode, "tasktypemismatch"):
		return "invalid_request", false
	case strings.EqualFold(code, "QuotaExceeded"), strings.EqualFold(code, "RequestBurstTooFast"):
		return "rate_limit", true
	case strings.EqualFold(code, "ServerOverloaded"):
		return "upstream_unavailable", true
	case code == PublicTaskProcessingFailedCode:
		return "task_processing", true
	default:
		return "upstream", false
	}
}

func isAIPDDUpstreamBalanceError(lowerCode, lowerMessage string) bool {
	return strings.Contains(lowerCode, "insufficientbalance") ||
		strings.Contains(lowerCode, "accountoverdue") ||
		strings.Contains(lowerMessage, "awcoin") ||
		strings.Contains(lowerMessage, "insufficient account balance") ||
		strings.Contains(lowerMessage, "account balance is insufficient") ||
		strings.Contains(lowerMessage, "please recharge") ||
		strings.Contains(lowerMessage, "please top up") ||
		strings.Contains(lowerMessage, "余额不足") ||
		strings.Contains(lowerMessage, "请充值")
}

func canonicalSeedanceErrorCode(code, message string) string {
	knownCodes := []string{
		"InputTextSensitiveContentDetected.PolicyViolation",
		"InputImageSensitiveContentDetected.PolicyViolation",
		"InputVideoSensitiveContentDetected.PolicyViolation",
		"InputAudioSensitiveContentDetected.PolicyViolation",
		"OutputVideoSensitiveContentDetected.PolicyViolation",
		"OutputAudioSensitiveContentDetected.PolicyViolation",
		"InputImageSensitiveContentDetected.PrivacyInformation",
		"InputVideoSensitiveContentDetected.PrivacyInformation",
		"OutputImageSensitiveContentDetected.DeepFake",
		"InputTextSensitiveContentDetected",
		"InputImageSensitiveContentDetected",
		"InputVideoSensitiveContentDetected",
		"InputAudioSensitiveContentDetected",
		"OutputTextSensitiveContentDetected",
		"OutputImageSensitiveContentDetected",
		"OutputVideoSensitiveContentDetected",
		"OutputAudioSensitiveContentDetected",
		"SensitiveContentDetected",
		"InvalidParameter.TaskTypeConstraint",
		"InvalidParameter.TaskTypeMismatch",
		"QuotaExceeded",
		"ServerOverloaded",
		"RequestBurstTooFast",
	}

	for _, knownCode := range knownCodes {
		if strings.EqualFold(code, knownCode) {
			return knownCode
		}
	}
	for _, knownCode := range knownCodes {
		if strings.Contains(strings.ToLower(message), strings.ToLower(knownCode)) {
			return knownCode
		}
	}

	lowerMessage := strings.ToLower(message)
	policyViolation := strings.Contains(lowerMessage, "copyright restriction") ||
		strings.Contains(lowerMessage, "protected intellectual property")
	if policyViolation {
		switch {
		case strings.Contains(lowerMessage, "input image"):
			return "InputImageSensitiveContentDetected.PolicyViolation"
		case strings.Contains(lowerMessage, "input video"):
			return "InputVideoSensitiveContentDetected.PolicyViolation"
		case strings.Contains(lowerMessage, "input audio"):
			return "InputAudioSensitiveContentDetected.PolicyViolation"
		case strings.Contains(lowerMessage, "input text"):
			return "InputTextSensitiveContentDetected.PolicyViolation"
		case strings.Contains(lowerMessage, "output video"):
			return "OutputVideoSensitiveContentDetected.PolicyViolation"
		case strings.Contains(lowerMessage, "output audio"):
			return "OutputAudioSensitiveContentDetected.PolicyViolation"
		}
	}
	if strings.Contains(lowerMessage, "may contain a real person") {
		if strings.Contains(lowerMessage, "input image") {
			return "InputImageSensitiveContentDetected.PrivacyInformation"
		}
		if strings.Contains(lowerMessage, "input video") {
			return "InputVideoSensitiveContentDetected.PrivacyInformation"
		}
	}
	return ""
}

func seedancePublicErrorMessage(code, param string) string {
	switch code {
	case "InputTextSensitiveContentDetected.PolicyViolation":
		return "输入文本未通过上游版权审核，可能涉及版权限制，请修改提示词后重试。"
	case "InputImageSensitiveContentDetected.PolicyViolation":
		return locatedInputSubject(param, "图片") + "未通过上游版权审核，可能涉及版权限制，请更换确认拥有使用权的图片后重试。"
	case "InputVideoSensitiveContentDetected.PolicyViolation":
		return locatedInputSubject(param, "视频") + "未通过上游版权审核，可能涉及版权限制，请更换确认拥有使用权的视频后重试。"
	case "InputAudioSensitiveContentDetected.PolicyViolation":
		return locatedInputSubject(param, "音频") + "未通过上游版权审核，可能涉及版权限制，请更换确认拥有使用权的音频后重试。"
	case "OutputVideoSensitiveContentDetected.PolicyViolation":
		return "生成的视频可能涉及版权限制，请调整输入内容后重试。"
	case "OutputAudioSensitiveContentDetected.PolicyViolation":
		return "生成的音频可能涉及版权限制，请调整输入内容后重试。"
	case "InputImageSensitiveContentDetected.PrivacyInformation":
		return locatedInputSubject(param, "图片") + "可能包含真人，请更换素材或使用已完成授权的真人素材。"
	case "InputVideoSensitiveContentDetected.PrivacyInformation":
		return locatedInputSubject(param, "视频") + "可能包含真人，请更换素材或使用已完成授权的真人素材。"
	case "OutputImageSensitiveContentDetected.DeepFake":
		return "生成的图片可能包含伪造证件或凭据，请调整输入内容后重试。"
	case "InputTextSensitiveContentDetected", "SensitiveContentDetected":
		return "输入文本未通过上游内容安全审核，请修改后重试。"
	case "InputImageSensitiveContentDetected":
		return locatedInputSubject(param, "图片") + "未通过上游内容安全审核，请更换后重试。"
	case "InputVideoSensitiveContentDetected":
		return locatedInputSubject(param, "视频") + "未通过上游内容安全审核，请更换后重试。"
	case "InputAudioSensitiveContentDetected":
		return locatedInputSubject(param, "音频") + "未通过上游内容安全审核，请更换后重试。"
	case "OutputTextSensitiveContentDetected":
		return "生成的文本未通过上游内容安全审核，请调整输入内容后重试。"
	case "OutputImageSensitiveContentDetected":
		return "生成的图片未通过上游内容安全审核，请调整输入内容后重试。"
	case "OutputVideoSensitiveContentDetected":
		return "生成的视频未通过上游内容安全审核，请调整输入内容后重试。"
	case "OutputAudioSensitiveContentDetected":
		return "生成的音频未通过上游内容安全审核，请调整输入内容后重试。"
	case "InvalidParameter.TaskTypeConstraint":
		return "部分请求参数与当前生成模式不兼容，请检查参数和输入素材后重试。"
	case "InvalidParameter.TaskTypeMismatch":
		return "输入内容与所选生成模式不匹配，请检查提示词、素材和任务类型后重试。"
	case "QuotaExceeded":
		return "当前任务额度或排队数量已达上限，请稍后重试。"
	case "ServerOverloaded":
		return "上游服务当前繁忙，请稍后重试。"
	case "RequestBurstTooFast":
		return "请求增长过快，请降低提交速度后重试。"
	default:
		return ""
	}
}

func locatedInputSubject(param, media string) string {
	match := seedanceContentParamPattern.FindStringSubmatch(param)
	if len(match) != 2 {
		return "输入" + media
	}
	index, err := strconv.Atoi(match[1])
	if err != nil {
		return "输入" + media
	}
	return fmt.Sprintf("第 %d 个输入内容中的%s", index+1, media)
}

func extractSeedanceContentParam(message string) string {
	return seedanceContentParamPattern.FindString(message)
}

func extractSeedanceRequestID(message string) string {
	match := seedanceRequestIDPattern.FindStringSubmatch(message)
	if len(match) != 2 {
		return ""
	}
	return strings.TrimSpace(match[1])
}

func appendRequestID(message, requestID string) string {
	message = strings.TrimSpace(message)
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		return message
	}
	message = strings.TrimRight(message, "。 ") + "。"
	return message + "如需协助，请提供请求 ID：" + requestID + "。"
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
