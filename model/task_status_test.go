package model

import (
	"testing"

	"github.com/QuantumNous/new-api/dto"
)

func TestTaskStatusNotStartToVideoStatusQueued(t *testing.T) {
	if got := TaskStatusNotStart.ToVideoStatus(); got != dto.VideoStatusQueued {
		t.Fatalf("ToVideoStatus() = %q, want %q", got, dto.VideoStatusQueued)
	}
}
