package seedancepublic

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestTaskAllowlistKeepsPublicContractAndExactUsage(t *testing.T) {
	data := []byte(`{"id":"task_public","model":"Seedance 2.5 4K","status":"succeeded","duration":8,"resolution":"4k","ratio":"16:9","output_format":"mp4","unknown_extension":{"pipeline":"secret"},"sourceResolution":"720p","superResolution":{"scale":4},"metadata":{"trace":"private"},"usage":{"completion_tokens":9223372036854775807,"total_tokens":9223372036854775807,"formula":"private","tool_usage":{"web_search":1,"enhance":2}},"content":{"video_url":"/v1/videos/task_public/content","last_frame_url":"/v1/videos/task_public/last-frame","trace":"private"}}`)
	result, err := Task(data, false)
	require.NoError(t, err)
	require.Contains(t, string(result), `9223372036854775807`)
	require.Contains(t, string(result), `"resolution":"4k"`)
	require.Contains(t, string(result), `"web_search":1`)
	require.Contains(t, string(result), `"duration":8`)
	for _, word := range []string{"unknown_extension", "sourceResolution", "superResolution", "formula", "enhance", "trace", "private"} {
		require.NotContains(t, string(result), word)
	}
}

func TestTaskHidesPrivateErrorAndUnexpectedNestedValue(t *testing.T) {
	for _, word := range []string{"SuperResolution", "super-resolution", "upscaling", "SeedVR", "MediaKit", "enhancement", "超分", "增强"} {
		data, err := common.Marshal(map[string]any{"id": "task_test", "status": "failed", "error": map[string]any{"code": "vendor_failure", "message": word + " model failed", "provider": "private"}, "duration": map[string]any{"route": "private"}})
		require.NoError(t, err)
		body, err := Task(data, false)
		require.NoError(t, err)
		require.NotContains(t, string(body), word)
		require.NotContains(t, string(body), "provider")
		require.NotContains(t, string(body), "duration")
		require.Contains(t, string(body), "video_processing_failed")
	}
}

func TestCleanNestedArraysAndReplaceMediaURLs(t *testing.T) {
	value := map[string]any{"items": []any{"safe", "超分完成", map[string]any{"sourceResolution": "720p", "ok": true}}}
	body, err := common.Marshal(Clean(value))
	require.NoError(t, err)
	require.JSONEq(t, `{"items":["safe",{"ok":true}]}`, string(body))
	result, err := Media([]byte(`{"id":"task_test","status":"completed","metadata":{"url":"https://supplier.invalid/upscale.mp4","urls":["https://supplier.invalid/private.mp4"],"duration":5},"usage":{"completion_tokens":9223372036854775807,"total_tokens":9223372036854775807}}`), true, "/v1/videos/task_test/content", "/v1/videos/task_test/last-frame")
	require.NoError(t, err)
	require.NotContains(t, string(result), "supplier")
	require.Contains(t, string(result), "/v1/videos/task_test/content")
	require.Contains(t, string(result), "9223372036854775807")
}
