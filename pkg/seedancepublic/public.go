// Package seedancepublic defines the customer-visible Seedance boundary.
package seedancepublic

import (
	"encoding/json"
	"strings"
	"unicode"

	"github.com/QuantumNous/new-api/common"
)

func IsModel(name string) bool { return strings.Contains(strings.ToLower(name), "seedance") }

// Normalize spelling variants so camelCase, separators and mixed case cannot
// expose the same private detail through a different field spelling.
func Internal(text string) bool {
	compact := strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return unicode.ToLower(r)
		}
		return -1
	}, text)
	for _, term := range []string{"超分", "增强", "superresolution", "upscal", "enhance", "seedvr", "mediakit", "sourceresolution", "originalresolution", "baseresolution", "executionchain", "executionroute", "basevideo", "upstreammodel", "framecoefficient", "coefficientsource", "usageformula", "usagesnapshot", "seedancearkequivalent", "等效token", "标定系数"} {
		if strings.Contains(compact, term) {
			return true
		}
	}
	return false
}

func Text(value, fallback string) string {
	if Internal(value) {
		return fallback
	}
	return value
}

// Clean is used for optional catalog/log metadata, including nested arrays.
func Clean(value any) any {
	switch v := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(v))
		for k, child := range v {
			if !Internal(k) && !privateMetadataField(k, child) {
				if clean := Clean(child); clean != nil {
					out[k] = clean
				}
			}
		}
		return out
	case []any:
		out := make([]any, 0, len(v))
		for _, child := range v {
			if clean := Clean(child); clean != nil {
				out = append(out, clean)
			}
		}
		return out
	case string:
		if Internal(v) {
			return nil
		}
		return ErrorText(v)
	}
	return value
}

func fields(raw []byte, allowed string) map[string]json.RawMessage {
	var source map[string]json.RawMessage
	_ = common.Unmarshal(raw, &source)
	out := map[string]json.RawMessage{}
	for _, key := range strings.Fields(allowed) {
		var decoded string
		if common.Unmarshal(source[key], &decoded) == nil && Internal(decoded) {
			continue
		}
		if v, exists := source[key]; exists && !Internal(string(v)) && !strings.HasPrefix(strings.TrimSpace(string(v)), "{") && (!strings.HasPrefix(strings.TrimSpace(string(v)), "[") || key == "urls") {
			out[key] = v
		}
	}
	return out
}

// Task uses an allowlist instead of forwarding unknown supplier extensions.
// RawMessage preserves the exact int64 usage values without float conversion.
func Task(data []byte, openAI bool, secrets ...string) ([]byte, error) {
	var source map[string]json.RawMessage
	if err := common.Unmarshal(data, &source); err != nil {
		return nil, err
	}
	allowed := "id model status created_at updated_at duration resolution ratio seed frames framespersecond fps output_format generate_audio watermark draft service_tier execution_expires_after return_last_frame"
	if openAI {
		allowed = "id task_id object model status progress created_at completed_at seconds size remixed_from_video_id expires_at"
	}
	out := fields(data, allowed)
	if v := source["content"]; !openAI && len(v) > 0 {
		out["content"], _ = common.Marshal(fields(v, "video_url last_frame_url"))
	}
	if v := source["metadata"]; openAI && len(v) > 0 {
		out["metadata"], _ = common.Marshal(fields(v, "url urls duration resolution output_format last_frame_url"))
	}
	if v := source["usage"]; len(v) > 0 {
		usage := fields(v, "completion_tokens total_tokens")
		if !openAI {
			var rawUsage map[string]json.RawMessage
			_ = common.Unmarshal(v, &rawUsage)
			if tools := fields(rawUsage["tool_usage"], "web_search search"); len(tools) > 0 {
				usage["tool_usage"], _ = common.Marshal(tools)
			}
		}
		out["usage"], _ = common.Marshal(usage)
	}
	if v := source["error"]; len(v) > 0 && string(v) != "null" {
		out["error"] = Error(v, secrets...)
	}
	return common.Marshal(out)
}

// Media replaces supplier URLs before filtering, preserving usage precision.
func Media(data []byte, openAI bool, videoURL, frameURL string, secrets ...string) ([]byte, error) {
	var payload map[string]json.RawMessage
	if err := common.Unmarshal(data, &payload); err != nil {
		return nil, err
	}
	key, videoKey := "content", "video_url"
	if openAI {
		key, videoKey = "metadata", "url"
	}
	media := map[string]json.RawMessage{}
	_ = common.Unmarshal(payload[key], &media)
	if media == nil {
		media = map[string]json.RawMessage{}
	}
	delete(media, "urls")
	delete(media, videoKey)
	delete(media, "last_frame_url")
	if videoURL != "" {
		media[videoKey], _ = common.Marshal(videoURL)
	}
	if frameURL != "" {
		media["last_frame_url"], _ = common.Marshal(frameURL)
	}
	if len(media) > 0 {
		payload[key], _ = common.Marshal(media)
	} else {
		delete(payload, key)
	}
	encoded, err := common.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return Task(encoded, openAI, secrets...)
}
