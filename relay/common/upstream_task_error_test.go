package common

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeUpstreamTaskErrorSeedanceCopyright(t *testing.T) {
	details := NormalizeUpstreamTaskError(
		"InputImageSensitiveContentDetected.PolicyViolation",
		"The request failed because the input image 'content[1]' may be related to copyright restrictions. Request id: 0217865025866603a41b788dd1ea248c54437243b4fd45bbbc0ca",
		"",
		"",
	)

	require.True(t, details.Matched)
	require.False(t, details.HideRaw)
	require.Equal(t, "InputImageSensitiveContentDetected.PolicyViolation", details.Code)
	require.Equal(t, "content[1]", details.Param)
	require.Equal(t, "0217865025866603a41b788dd1ea248c54437243b4fd45bbbc0ca", details.RequestID)
	require.Contains(t, details.Message, "第 2 个输入内容中的图片")
	require.Contains(t, details.Message, "版权限制")
	require.Contains(t, details.Message, details.RequestID)
}

func TestNormalizeUpstreamTaskErrorInfersSeedanceCopyrightCodeFromMessage(t *testing.T) {
	details := NormalizeUpstreamTaskError(
		"",
		"The request failed because the input video content[2] may be related to copyright restrictions.",
		"",
		"",
	)

	require.True(t, details.Matched)
	require.Equal(t, "InputVideoSensitiveContentDetected.PolicyViolation", details.Code)
	require.Equal(t, "content[2]", details.Param)
	require.Contains(t, details.Message, "第 3 个输入内容中的视频")
}

func TestNormalizeUpstreamTaskErrorMasksSuperResolutionDetails(t *testing.T) {
	details := NormalizeUpstreamTaskError(
		"SeedVR2UpscaleFailed",
		"seedvr2-upscale worker crashed while loading the super-resolution model. Request id: req-internal-1",
		"content[3]",
		"",
	)

	require.True(t, details.Matched)
	require.True(t, details.HideRaw)
	require.Equal(t, PublicTaskProcessingFailedCode, details.Code)
	require.Equal(t, "", details.Param)
	require.Equal(t, "req-internal-1", details.RequestID)
	require.NotContains(t, strings.ToLower(details.Message), "seedvr")
	require.NotContains(t, strings.ToLower(details.Message), "upscale")
	require.NotContains(t, details.Message, "超分")
}

func TestNormalizeUpstreamTaskErrorLeavesUnknownErrorsUntouched(t *testing.T) {
	details := NormalizeUpstreamTaskError(
		"InvalidParameter",
		"content[1] video pixel count must be at least 409600",
		"",
		"req-1",
	)

	require.False(t, details.Matched)
	require.Equal(t, "InvalidParameter", details.Code)
	require.Equal(t, "content[1] video pixel count must be at least 409600", details.Message)
	require.Equal(t, "content[1]", details.Param)
	require.Equal(t, "req-1", details.RequestID)
}

func TestNormalizeAIPDDTaskErrorInvalidRequest(t *testing.T) {
	details := NormalizeAIPDDTaskError(
		"InvalidParameter",
		"content[1] video pixel count must be at least 409600. Request id: req-invalid-1",
		"",
		"",
		400,
		AIPDDTaskErrorOperationCreate,
	)

	require.True(t, details.Matched)
	require.False(t, details.HideRaw)
	require.Equal(t, AIPDDErrorCodeInvalidRequest, details.Code)
	require.Equal(t, "aipdd", details.Provider)
	require.Equal(t, "invalid_request", details.Category)
	require.Equal(t, "InvalidParameter", details.UpstreamCode)
	require.False(t, details.Retryable)
	require.Equal(t, "content[1]", details.Param)
	require.Equal(t, "req-invalid-1", details.RequestID)
	require.Equal(t, "InvalidParameter", details.Data()["upstream_code"])
}

func TestNormalizeAIPDDTaskErrorRateLimitIsRetryable(t *testing.T) {
	details := NormalizeAIPDDTaskError(
		"TooManyRequests",
		"request rate exceeded",
		"",
		"",
		429,
		AIPDDTaskErrorOperationCreate,
	)

	require.Equal(t, AIPDDErrorCodeRateLimited, details.Code)
	require.Equal(t, "rate_limit", details.Category)
	require.True(t, details.Retryable)
	require.Equal(t, false, details.HideRaw)
	require.Equal(t, "aipdd", details.Data()["provider"])
	require.Equal(t, true, details.Data()["retryable"])
}

func TestNormalizeAIPDDTaskErrorHidesInternalAndBalanceDetails(t *testing.T) {
	tests := []struct {
		name       string
		code       string
		message    string
		statusCode int
		wantCode   string
	}{
		{
			name:       "upstream server error",
			code:       "InternalWorkerError",
			message:    "private worker pool node-7 crashed",
			statusCode: 500,
			wantCode:   AIPDDErrorCodeUpstreamUnavailable,
		},
		{
			name:       "upstream balance",
			code:       "InsufficientBalance",
			message:    "余额不足，需要 7900 AWCoin，请充值",
			statusCode: 402,
			wantCode:   AIPDDErrorCodeUpstreamConfiguration,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			details := NormalizeAIPDDTaskError(
				test.code,
				test.message,
				"",
				"",
				test.statusCode,
				AIPDDTaskErrorOperationExecute,
			)

			require.Equal(t, test.wantCode, details.Code)
			require.True(t, details.HideRaw)
			require.Empty(t, details.UpstreamCode)
			require.NotContains(t, details.Data(), "upstream_code")
			require.NotContains(t, details.Message, "AWCoin")
			require.NotContains(t, details.Message, "node-7")
		})
	}
}

func TestNormalizeAIPDDTaskErrorFallbackUsesOperationCode(t *testing.T) {
	tests := []struct {
		operation AIPDDTaskErrorOperation
		wantCode  string
		category  string
	}{
		{AIPDDTaskErrorOperationCreate, AIPDDErrorCodeTaskCreateFailed, "task_create"},
		{AIPDDTaskErrorOperationQuery, AIPDDErrorCodeTaskQueryFailed, "task_query"},
		{AIPDDTaskErrorOperationExecute, AIPDDErrorCodeTaskFailed, "task_execution"},
	}

	for _, test := range tests {
		details := NormalizeAIPDDTaskError("unknown", "opaque failure", "", "", 0, test.operation)
		require.Equal(t, test.wantCode, details.Code)
		require.Equal(t, test.category, details.Category)
		require.True(t, details.HideRaw)
	}
}

func TestNormalizeAIPDDTaskErrorPreservesDocumentedSeedanceCode(t *testing.T) {
	details := NormalizeAIPDDTaskError(
		"InputImageSensitiveContentDetected.PolicyViolation",
		"The input image content[1] may be related to copyright restrictions.",
		"",
		"req-copyright-aipdd",
		400,
		AIPDDTaskErrorOperationExecute,
	)

	require.Equal(t, "InputImageSensitiveContentDetected.PolicyViolation", details.Code)
	require.Equal(t, "aipdd", details.Provider)
	require.Equal(t, "copyright_policy", details.Category)
	require.Empty(t, details.UpstreamCode)
	require.False(t, details.Retryable)
	require.Equal(t, "content[1]", details.Param)
}
