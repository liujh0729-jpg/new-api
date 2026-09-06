package seedancepublic

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

func TestJavaErrorRedactionRetainsActionableConstraints(t *testing.T) {
	raw := []byte(`{"code":"InvalidParameter","message":"video pixel count must be greater than 407696 for model doubao-seedance-2-0-fast in r2v; credential=test-secret","param":"content[1]","request_id":"req-public","debug":{"apikey":"test-secret"}}`)
	result := Error(raw, "test-secret")
	var got map[string]any
	require.NoError(t, common.Unmarshal(result, &got))
	require.Equal(t, "InvalidParameter", got["code"])
	require.Contains(t, got["message"], "407696")
	require.Equal(t, "content[1]", got["param"])
	require.Equal(t, "req-public", got["request_id"])
	for _, word := range []string{"doubao", "r2v", "test-secret", "debug", "apikey"} {
		require.NotContains(t, string(result), word)
	}
}

func TestPublicErrorEscapesAndCredentialVariants(t *testing.T) {
	result, err := Task([]byte(`{"id":"task_x","error":{"code":"failed","message":"\u8d85\u5206 failed"},"service_tier":"\u8d85\u5206"}`), false)
	require.NoError(t, err)
	require.Contains(t, string(result), "video_processing_failed")
	require.NotContains(t, string(result), "service_tier")
	for _, message := range []string{"Authorization: Bearer abc-private", "api_key=abc-private", "access-token: abc-private"} {
		require.NotContains(t, ErrorText(message), "abc-private")
	}
	require.Equal(t, failureMessage, ErrorText("<!DOCTYPE html><html>private stacktrace</html>"))
}

func TestJavaPrivateMetadataIsRemovedRecursively(t *testing.T) {
	source := map[string]any{
		"billing_mode": "task_pricing", "group_ratio": 1, "total_tokens": int64(785700),
		"nested": []any{map[string]any{"baseSource": "platform", "provider_name": "vendor", "billingScope": "base", "billingMode": "BYOK", "base_status": "succeeded", "estimated_awcoin": 4, "costAwcoinPerSecond": 2, "authorization": "secret", "aipdd_meta": map[string]any{"debug": true}}},
	}
	result, err := common.Marshal(Clean(source))
	require.NoError(t, err)
	require.JSONEq(t, `{"billing_mode":"task_pricing","group_ratio":1,"total_tokens":785700,"nested":[{}]}`, string(result))
}

func TestPublicTaskErrorDoesNotExposeSupplierAttribution(t *testing.T) {
	for _, openAI := range []bool{false, true} {
		result, err := Task([]byte(`{"id":"task_public","status":"failed","error":{"code":"video_processing_failed","message":"Please try again later","request_id":"req_public","provider":"volcengine","upstream_code":"supplier_job_123","retryable":true}}`), openAI)
		require.NoError(t, err)
		var task struct {
			Error map[string]any `json:"error"`
		}
		require.NoError(t, common.Unmarshal(result, &task))
		require.Equal(t, "video_processing_failed", task.Error["code"])
		require.Equal(t, "req_public", task.Error["request_id"])
		require.Equal(t, true, task.Error["retryable"])
		require.NotContains(t, task.Error, "provider")
		require.NotContains(t, task.Error, "upstream_code")
		require.NotContains(t, string(result), "volcengine")
		require.NotContains(t, string(result), "supplier_job_123")
	}
}
