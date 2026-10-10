package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/require"
)

func TestRelayErrorHandlerPreservesRecoveryHeaders(t *testing.T) {
	t.Parallel()
	header := http.Header{}
	header.Set("Retry-After", "45")
	header.Set("X-RateLimit-Reset", "1798483200")
	response := &http.Response{
		StatusCode: http.StatusTooManyRequests,
		Header:     header,
		Body: io.NopCloser(strings.NewReader(`{"error":{"message":"rate limited","type":"rate_limit_error","code":"rate_limit"}}`)),
	}

	apiErr := RelayErrorHandler(context.Background(), response, false)
	require.NotNil(t, apiErr)
	require.Equal(t, "45", apiErr.RetryAfter)
	require.Equal(t, "1798483200", apiErr.RateLimitReset)
}

func TestTaskErrorFromAPIError_MarksLocalError(t *testing.T) {
	t.Parallel()

	apiErr := types.NewErrorWithStatusCode(
		errors.New("用户额度不足, 剩余额度: ¥1"),
		types.ErrorCodeInsufficientUserQuota,
		http.StatusForbidden,
	)
	taskErr := TaskErrorFromAPIError(apiErr)
	require.NotNil(t, taskErr)
	require.True(t, taskErr.LocalError)
	require.Equal(t, string(types.ErrorCodeInsufficientUserQuota), taskErr.Code)
	require.Equal(t, http.StatusForbidden, taskErr.StatusCode)
	require.Contains(t, taskErr.Message, "用户额度不足")
}

func TestTaskErrorFromAPIError_Nil(t *testing.T) {
	t.Parallel()
	require.Nil(t, TaskErrorFromAPIError(nil))
}

func TestResetStatusCode(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name             string
		statusCode       int
		statusCodeConfig string
		expectedCode     int
	}{
		{
			name:             "map string value",
			statusCode:       429,
			statusCodeConfig: `{"429":"503"}`,
			expectedCode:     503,
		},
		{
			name:             "map int value",
			statusCode:       429,
			statusCodeConfig: `{"429":503}`,
			expectedCode:     503,
		},
		{
			name:             "skip invalid string value",
			statusCode:       429,
			statusCodeConfig: `{"429":"bad-code"}`,
			expectedCode:     429,
		},
		{
			name:             "skip status code 200",
			statusCode:       200,
			statusCodeConfig: `{"200":503}`,
			expectedCode:     200,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			newAPIError := &types.NewAPIError{
				StatusCode: tc.statusCode,
			}
			ResetStatusCode(newAPIError, tc.statusCodeConfig)
			require.Equal(t, tc.expectedCode, newAPIError.StatusCode)
		})
	}
}
