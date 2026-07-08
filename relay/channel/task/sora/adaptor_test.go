package sora

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
)

func TestParseTaskResultTreatsUnknownAsQueued(t *testing.T) {
	adaptor := &TaskAdaptor{}

	result, err := adaptor.ParseTaskResult([]byte(`{"id":"task_123","object":"video","status":"unknown","progress":0}`))
	if err != nil {
		t.Fatalf("ParseTaskResult returned error: %v", err)
	}

	if result.Status != model.TaskStatusQueued {
		t.Fatalf("status = %q, want %q", result.Status, model.TaskStatusQueued)
	}
}

func TestParseTaskResultTreatsSubmittedAsQueued(t *testing.T) {
	adaptor := &TaskAdaptor{}

	result, err := adaptor.ParseTaskResult([]byte(`{"id":"task_123","object":"video","status":"submitted","progress":0}`))
	if err != nil {
		t.Fatalf("ParseTaskResult returned error: %v", err)
	}

	if result.Status != model.TaskStatusQueued {
		t.Fatalf("status = %q, want %q", result.Status, model.TaskStatusQueued)
	}
}
