package seedancepublic

import (
	"encoding/json"
	"regexp"
	"strings"

	"github.com/QuantumNous/new-api/common"
)

var (
	privateModelInError = regexp.MustCompile("(?i)\\b(?:model\\s+[`'\"]?)?doubao[-_]seedance[-_][a-z0-9._-]+\\b[`'\"]?")
	privateModeInError  = regexp.MustCompile(`(?i)\s+in\s+(?:r2v|i2v|t2v|v2v)\b`)
	credentialInError   = regexp.MustCompile(`(?i)\b(?:bearer\s+|(?:api[_ -]?key|access[_ -]?token|authorization)\s*[=:]\s*["']?(?:(?:bearer|basic)\s+)?)[^\s"',;<>}]+`)
)

const failureMessage = "Video processing failed. Please try again later."

// ErrorText keeps useful parameter constraints while removing private model
// routing and credentials echoed by a provider. Secrets must come from the
// request/task context, never from an additional credential lookup.
func ErrorText(value string, secrets ...string) string {
	for _, secret := range secrets {
		if secret != "" {
			value = strings.ReplaceAll(value, secret, "[redacted]")
		}
	}
	value = credentialInError.ReplaceAllString(value, "[redacted]")
	value = privateModelInError.ReplaceAllString(value, "the selected model")
	value = privateModeInError.ReplaceAllString(value, "")
	lower := strings.ToLower(strings.TrimSpace(value))
	if Internal(value) || strings.Contains(lower, "<html") || strings.Contains(lower, "<!doctype") {
		return failureMessage
	}
	return value
}

// Error projects only documented error fields; arbitrary nested data is not
// part of a public error. Decode strings before checking them so JSON escapes
// cannot bypass the same rules applied to ordinary UTF-8 text.
func Error(raw []byte, secrets ...string) json.RawMessage {
	var source map[string]json.RawMessage
	if common.Unmarshal(raw, &source) != nil || source == nil {
		return json.RawMessage(`{"code":"video_processing_failed","message":"Video processing failed. Please try again later."}`)
	}
	var decoded any
	_ = common.Unmarshal(raw, &decoded)
	if hasInternalValue(decoded) {
		return json.RawMessage(`{"code":"video_processing_failed","message":"Video processing failed. Please try again later."}`)
	}
	// Supplier attribution and raw supplier codes belong only in private
	// diagnostics, even when their values contain no known processing keyword.
	out := fields(raw, "code message type param request_id category retryable")
	for key, rawValue := range out {
		var value string
		if common.Unmarshal(rawValue, &value) == nil {
			out[key], _ = common.Marshal(ErrorText(value, secrets...))
		}
	}
	encoded, _ := common.Marshal(out)
	return encoded
}

func hasInternalValue(value any) bool {
	switch v := value.(type) {
	case string:
		return Internal(v)
	case map[string]any:
		for key, child := range v {
			if Internal(key) || hasInternalValue(child) {
				return true
			}
		}
	case []any:
		for _, child := range v {
			if hasInternalValue(child) {
				return true
			}
		}
	}
	return false
}
