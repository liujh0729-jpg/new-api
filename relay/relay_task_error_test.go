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
package relay

import (
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/require"
)

func TestTaskErrorFromUpstreamResponsePreservesOpenAIError(t *testing.T) {
	taskErr := taskErrorFromUpstreamResponse(
		[]byte(`{"error":{"code":"InvalidParameter","message":"content[1] video pixel count must be at least 409600","type":"invalid_request_error"},"request_id":"req-seedance-1"}`),
		400,
	)

	if taskErr.Code != "InvalidParameter" {
		t.Fatalf("expected upstream code InvalidParameter, got %q", taskErr.Code)
	}
	if taskErr.Message != "content[1] video pixel count must be at least 409600" {
		t.Fatalf("expected upstream message to be preserved, got %q", taskErr.Message)
	}
	if taskErr.StatusCode != 400 {
		t.Fatalf("expected status code 400, got %d", taskErr.StatusCode)
	}
	details, ok := taskErr.Data.(map[string]any)
	if !ok {
		t.Fatalf("expected structured upstream details, got %#v", taskErr.Data)
	}
	if details["type"] != "invalid_request_error" || details["request_id"] != "req-seedance-1" {
		t.Fatalf("expected upstream type and request id to be preserved, got %#v", details)
	}
}

func TestTaskErrorFromUpstreamResponseFallsBackForUnknownBody(t *testing.T) {
	taskErr := taskErrorFromUpstreamResponse([]byte("upstream unavailable"), 503)

	if taskErr.Code != "fail_to_fetch_task" {
		t.Fatalf("expected fallback code, got %q", taskErr.Code)
	}
	if taskErr.Message != "upstream unavailable" {
		t.Fatalf("expected fallback message, got %q", taskErr.Message)
	}
	if taskErr.StatusCode != 503 {
		t.Fatalf("expected status code 503, got %d", taskErr.StatusCode)
	}
}

func TestTaskErrorFromUpstreamResponseRedactsAccountID(t *testing.T) {
	taskErr := taskErrorFromUpstreamResponse(
		[]byte(`{"error":{"code":"ModelNotActivated","message":"Your account 2101505868 has not activated the model doubao-seedance-2-0-mini. Please activate the model service in the Ark Console. Request id: 02178627930751198a941fd3b1c6e321c170880bc52c9313087e1","type":"invalid_request_error"}}`),
		400,
	)

	expected := "Your account [redacted] has not activated the model doubao-seedance-2-0-mini. Please activate the model service in the Ark Console. Request id: 02178627930751198a941fd3b1c6e321c170880bc52c9313087e1"
	if taskErr.Message != expected {
		t.Fatalf("expected account ID to be redacted, got %q", taskErr.Message)
	}
}

func TestTaskErrorFromUpstreamResponseRedactsAccountIDInFallbackBody(t *testing.T) {
	taskErr := taskErrorFromUpstreamResponse(
		[]byte("upstream rejected 当前账号ID为2101505868，请检查模型权限"),
		400,
	)

	if taskErr.Message != "upstream rejected 当前账号ID为[redacted]，请检查模型权限" {
		t.Fatalf("expected fallback account ID to be redacted, got %q", taskErr.Message)
	}
}

func TestTaskErrorFromUpstreamResponseLocalizesSeedanceCopyrightError(t *testing.T) {
	taskErr := taskErrorFromUpstreamResponse(
		[]byte(`{"error":{"code":"InputImageSensitiveContentDetected.PolicyViolation","message":"The request failed because the input image 'content[1]' may be related to copyright restrictions. Request id: req-copyright-2","type":"invalid_request_error"}}`),
		400,
	)

	require.Equal(t, "InputImageSensitiveContentDetected.PolicyViolation", taskErr.Code)
	require.Contains(t, taskErr.Message, "第 2 个输入内容中的图片")
	require.Contains(t, taskErr.Message, "版权限制")
	require.NotContains(t, taskErr.Message, "The request failed")
	details, ok := taskErr.Data.(map[string]any)
	require.True(t, ok)
	require.Equal(t, "content[1]", details["param"])
	require.Equal(t, "req-copyright-2", details["request_id"])
	require.Equal(t, "invalid_request_error", details["type"])
}

func TestTaskErrorFromUpstreamResponseMasksSuperResolutionError(t *testing.T) {
	rawMessage := "seedvr2-upscale worker failed while loading the super-resolution model. Request id: req-internal-2"
	taskErr := taskErrorFromUpstreamResponse(
		[]byte(`{"error":{"code":"SeedVR2UpscaleFailed","message":"`+rawMessage+`","type":"upstream_error"}}`),
		500,
	)

	require.Equal(t, relaycommon.PublicTaskProcessingFailedCode, taskErr.Code)
	require.NotContains(t, strings.ToLower(taskErr.Message), "seedvr")
	require.NotContains(t, strings.ToLower(taskErr.Message), "upscale")
	require.NotContains(t, taskErr.Message, "超分")
	require.Equal(t, rawMessage, taskErr.Error.Error())
	details, ok := taskErr.Data.(map[string]any)
	require.True(t, ok)
	require.Equal(t, "req-internal-2", details["request_id"])
	require.NotContains(t, details, "type")
}

func TestTaskErrorFromAIPDDUpstreamResponseUsesStableEnvelope(t *testing.T) {
	taskErr := taskErrorFromUpstreamResponse(
		[]byte(`{"error":{"code":"TooManyRequests","message":"upstream rate exceeded","type":"upstream_error"},"request_id":"req-rate-1"}`),
		http.StatusTooManyRequests,
		true,
	)

	require.Equal(t, relaycommon.AIPDDErrorCodeRateLimited, taskErr.Code)
	require.Contains(t, taskErr.Message, "请求过于频繁")
	details, ok := taskErr.Data.(map[string]any)
	require.True(t, ok)
	require.Equal(t, "aipdd", details["provider"])
	require.Equal(t, "rate_limit", details["category"])
	require.Equal(t, true, details["retryable"])
	require.Equal(t, "TooManyRequests", details["upstream_code"])
	require.Equal(t, "req-rate-1", details["request_id"])
}

func TestTaskErrorFromAIPDDUpstreamResponseHidesSuperResolutionError(t *testing.T) {
	rawMessage := "seedvr2-upscale internal worker crashed"
	taskErr := taskErrorFromUpstreamResponse(
		[]byte(`{"error":{"code":"SeedVR2UpscaleFailed","message":"`+rawMessage+`","type":"upstream_error"}}`),
		http.StatusInternalServerError,
		true,
	)

	require.Equal(t, relaycommon.PublicTaskProcessingFailedCode, taskErr.Code)
	details, ok := taskErr.Data.(map[string]any)
	require.True(t, ok)
	require.Equal(t, "aipdd", details["provider"])
	require.Equal(t, "task_processing", details["category"])
	require.Equal(t, true, details["retryable"])
	require.NotContains(t, details, "upstream_code")
	require.NotContains(t, strings.ToLower(taskErr.Message), "seedvr")
	require.NotContains(t, strings.ToLower(taskErr.Message), "upscale")
}

func TestTaskModel2DtoMasksSuperResolutionFailureData(t *testing.T) {
	task := &model.Task{
		TaskID:     "task-upscale-failure",
		Status:     model.TaskStatusFailure,
		FailReason: "SeedVR2UpscaleFailed: internal upscaler crashed",
		Data:       []byte(`{"status":"failed","error":{"code":"SeedVR2UpscaleFailed","message":"internal seedvr2-upscale worker crashed"}}`),
	}

	taskDTO := TaskModel2Dto(task)
	require.Equal(t, relaycommon.PublicTaskProcessingFailedCode, extractTaskDTOErrorCode(t, taskDTO.Data))
	require.NotContains(t, strings.ToLower(taskDTO.FailReason), "seedvr")
	require.NotContains(t, strings.ToLower(taskDTO.FailReason), "upscale")
	require.NotContains(t, strings.ToLower(string(taskDTO.Data)), "seedvr")
	require.NotContains(t, strings.ToLower(string(taskDTO.Data)), "upscale")
}

func TestTaskModel2DtoNormalizesAIPDDFailureData(t *testing.T) {
	task := &model.Task{
		TaskID:     "task-aipdd-failure",
		Platform:   constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeAIPDD)),
		Status:     model.TaskStatusFailure,
		FailReason: "opaque render worker failed",
		Data:       []byte(`{"status":"failed","error":{"message":"opaque render worker failed"}}`),
	}

	taskDTO := TaskModel2Dto(task)
	require.Equal(t, relaycommon.AIPDDErrorCodeTaskFailed, extractTaskDTOErrorCode(t, taskDTO.Data))
	require.Contains(t, taskDTO.FailReason, "任务处理失败")
	require.NotContains(t, string(taskDTO.Data), "opaque render worker")

	var payload struct {
		Error map[string]any `json:"error"`
	}
	require.NoError(t, common.Unmarshal(taskDTO.Data, &payload))
	require.Equal(t, "aipdd", payload.Error["provider"])
	require.Equal(t, "task_execution", payload.Error["category"])
	require.Equal(t, false, payload.Error["retryable"])
}

func extractTaskDTOErrorCode(t *testing.T, data []byte) string {
	t.Helper()
	var payload struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	require.NoError(t, common.Unmarshal(data, &payload))
	return payload.Error.Code
}
