package dto

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
)

func TestFunctionToolStrictAndImageContentSurviveRoundTrip(t *testing.T) {
	input := `{"model":"vision-tool-model","messages":[{"role":"user","content":[{"type":"text","text":"describe"},{"type":"image_url","image_url":{"url":"data:image/png;base64,iVBORw0KGgo="}}]}],"tools":[{"type":"function","function":{"name":"inspect","description":"inspect image","strict":true,"parameters":{"type":"object","additionalProperties":false,"properties":{"label":{"type":"string"}},"required":["label"]}}}],"tool_choice":{"type":"function","function":{"name":"inspect"}}}`
	var request GeneralOpenAIRequest
	if err := common.Unmarshal([]byte(input), &request); err != nil {
		t.Fatalf("unmarshal request: %v", err)
	}
	if len(request.Tools) != 1 || request.Tools[0].Function.Strict == nil || !*request.Tools[0].Function.Strict {
		t.Fatalf("strict function tool was not preserved: %#v", request.Tools)
	}
	encoded, err := common.Marshal(&request)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	value := string(encoded)
	for _, expected := range []string{`"strict":true`, `"image_url"`, `"additionalProperties":false`} {
		if !strings.Contains(value, expected) {
			t.Fatalf("round-trip dropped %s: %s", expected, value)
		}
	}
}

func TestResponsesImageFunctionToolsAndCallOutputSurviveRoundTrip(t *testing.T) {
	input := `{"model":"vision-tool-model","input":[{"role":"user","content":[{"type":"input_image","image_url":"https://cdn.example.com/a.png"}]},{"type":"function_call_output","call_id":"call_42","output":"{\"ok\":true}"}],"tools":[{"type":"function","name":"inspect","strict":true,"parameters":{"type":"object","additionalProperties":false}}],"tool_choice":{"type":"function","name":"inspect"},"parallel_tool_calls":false}`
	var request OpenAIResponsesRequest
	if err := common.Unmarshal([]byte(input), &request); err != nil {
		t.Fatalf("unmarshal responses request: %v", err)
	}
	encoded, err := common.Marshal(&request)
	if err != nil {
		t.Fatalf("marshal responses request: %v", err)
	}
	value := string(encoded)
	for _, expected := range []string{`"input_image"`, `"function_call_output"`, `"call_42"`, `"strict":true`, `"parallel_tool_calls":false`} {
		if !strings.Contains(value, expected) {
			t.Fatalf("responses round-trip dropped %s: %s", expected, value)
		}
	}
}
