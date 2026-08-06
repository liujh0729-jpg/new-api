package service

import (
	"errors"
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/require"
)

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
