/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
package controller

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestShouldMaskUpstreamBalanceTaskError(t *testing.T) {
	t.Parallel()

	require.True(t, shouldMaskUpstreamBalanceTaskError(&dto.TaskError{
		StatusCode: http.StatusPaymentRequired,
		Message:    "余额不足：本次任务预计需要预扣 7900 AWCoin，请充值后再创建任务",
		LocalError: false,
	}))
	require.True(t, shouldMaskUpstreamBalanceTaskError(&dto.TaskError{
		StatusCode: http.StatusBadRequest,
		Message:    "余额不足：本次任务预计需要预扣 6000 AWCoin，请充值后再创建任务",
		LocalError: false,
	}))
	require.True(t, shouldMaskUpstreamBalanceTaskError(&dto.TaskError{
		StatusCode: http.StatusForbidden,
		Message:    "please recharge before creating a task",
		Error:      errors.New("please recharge before creating a task"),
		LocalError: false,
	}))

	require.False(t, shouldMaskUpstreamBalanceTaskError(&dto.TaskError{
		StatusCode: http.StatusPaymentRequired,
		Message:    "余额不足：本次任务预计需要预扣 7900 AWCoin，请充值后再创建任务",
		LocalError: true,
	}))
	require.False(t, shouldMaskUpstreamBalanceTaskError(&dto.TaskError{
		StatusCode: http.StatusForbidden,
		Message:    "用户额度不足, 剩余额度: ¥75.6",
		LocalError: false,
	}))
	require.False(t, shouldMaskUpstreamBalanceTaskError(&dto.TaskError{
		StatusCode: http.StatusForbidden,
		Message:    "token quota is not enough, token remain quota: ¥4.65, need quota: ¥8.45",
		LocalError: false,
	}))
	require.False(t, shouldMaskUpstreamBalanceTaskError(&dto.TaskError{
		StatusCode: http.StatusPaymentRequired,
		Code:       "insufficient_user_quota",
		Message:    "余额不足",
		LocalError: false,
	}))
	require.False(t, shouldMaskUpstreamBalanceTaskError(&dto.TaskError{
		StatusCode: http.StatusForbidden,
		Code:       "pre_consume_token_quota_failed",
		Message:    "please recharge before creating a task",
		LocalError: false,
	}))
	require.False(t, shouldMaskUpstreamBalanceTaskError(&dto.TaskError{
		StatusCode: http.StatusBadRequest,
		Message:    "content[1] video pixel count must be at least 409600",
		LocalError: false,
	}))
}

func TestRespondTaskErrorMasksUpstreamBalanceMessage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	original := "余额不足：本次任务预计需要预扣 7900 AWCoin，请充值后再创建任务"
	taskErr := &dto.TaskError{
		Code:       "fail_to_fetch_task",
		Message:    original,
		StatusCode: http.StatusPaymentRequired,
		Error:      errors.New(original),
		LocalError: false,
	}

	respondTaskError(c, taskErr)

	require.Equal(t, taskUpstreamConfigChangedMessage, taskErr.Message)
	require.Equal(t, original, taskErr.Error.Error())
	require.Equal(t, http.StatusPaymentRequired, w.Code)
	require.Contains(t, w.Body.String(), taskUpstreamConfigChangedMessage)
	require.NotContains(t, w.Body.String(), "AWCoin")
}

func TestRespondTaskErrorMasksSuperResolutionDetails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	original := "SeedVR2UpscaleFailed: internal super-resolution worker crashed. Request id: req-internal-final"
	taskErr := &dto.TaskError{
		Code:       "SeedVR2UpscaleFailed",
		Message:    original,
		Data:       map[string]any{"internal_model": "seedvr2-upscale"},
		StatusCode: http.StatusInternalServerError,
		Error:      errors.New(original),
		LocalError: false,
	}

	respondTaskError(c, taskErr)

	require.Equal(t, relaycommon.PublicTaskProcessingFailedCode, taskErr.Code)
	require.NotContains(t, strings.ToLower(taskErr.Message), "seedvr")
	require.NotContains(t, strings.ToLower(taskErr.Message), "upscale")
	require.NotContains(t, taskErr.Message, "超分")
	require.Equal(t, original, taskErr.Error.Error())
	require.NotContains(t, strings.ToLower(w.Body.String()), "seedvr")
	require.NotContains(t, strings.ToLower(w.Body.String()), "upscale")
	require.Contains(t, w.Body.String(), "req-internal-final")
}

func TestRespondTaskErrorMasksSuperResolutionDetailsFoundOnlyInData(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	taskErr := &dto.TaskError{
		Code:       "task_failed",
		Message:    "task failed",
		Data:       map[string]any{"internal_model": "seedvr2-upscale", "detail": "worker crashed"},
		StatusCode: http.StatusInternalServerError,
		Error:      errors.New("task failed"),
		LocalError: true,
	}

	respondTaskError(c, taskErr)

	require.Equal(t, relaycommon.PublicTaskProcessingFailedCode, taskErr.Code)
	require.NotContains(t, strings.ToLower(w.Body.String()), "seedvr")
	require.NotContains(t, strings.ToLower(w.Body.String()), "upscale")
	require.NotContains(t, w.Body.String(), "worker crashed")
}

func TestRespondTaskErrorKeepsDocumentedSeedanceRateLimitMessage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	original := "The request has exceeded the quota. Request ID: req-quota-1"
	taskErr := &dto.TaskError{
		Code:       "QuotaExceeded",
		Message:    original,
		StatusCode: http.StatusTooManyRequests,
		Error:      errors.New(original),
		LocalError: false,
	}

	respondTaskError(c, taskErr)

	require.Equal(t, "QuotaExceeded", taskErr.Code)
	require.Contains(t, taskErr.Message, "额度或排队数量已达上限")
	require.NotEqual(t, taskUpstreamOverloadedMessage, taskErr.Message)
	require.Contains(t, w.Body.String(), "req-quota-1")
}

func TestRespondTaskErrorPreservesNormalizedAIPDDMessage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	taskErr := &dto.TaskError{
		Code:    relaycommon.AIPDDErrorCodeRateLimited,
		Message: "上游请求过于频繁，请稍后重试。",
		Data: map[string]any{
			"provider":  "aipdd",
			"category":  "rate_limit",
			"retryable": true,
		},
		StatusCode: http.StatusTooManyRequests,
		Error:      errors.New("raw upstream rate exceeded"),
	}

	respondTaskError(c, taskErr)

	require.Equal(t, "上游请求过于频繁，请稍后重试。", taskErr.Message)
	require.NotEqual(t, taskUpstreamOverloadedMessage, taskErr.Message)
	require.Contains(t, w.Body.String(), relaycommon.AIPDDErrorCodeRateLimited)
	require.Contains(t, w.Body.String(), `"retryable":true`)
}
