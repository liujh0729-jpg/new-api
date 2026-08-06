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
	"testing"

	"github.com/QuantumNous/new-api/dto"
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
