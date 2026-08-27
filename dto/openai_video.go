package dto

import (
	"strconv"
	"strings"
)

const (
	VideoStatusUnknown    = "unknown"
	VideoStatusQueued     = "queued"
	VideoStatusInProgress = "in_progress"
	VideoStatusCompleted  = "completed"
	VideoStatusFailed     = "failed"
)

type OpenAIVideo struct {
	ID                 string            `json:"id"`
	TaskID             string            `json:"task_id,omitempty"` //兼容旧接口 待废弃
	Object             string            `json:"object"`
	Model              string            `json:"model"`
	Status             string            `json:"status"` // Should use VideoStatus constants: VideoStatusQueued, VideoStatusInProgress, VideoStatusCompleted, VideoStatusFailed
	Progress           int               `json:"progress"`
	CreatedAt          int64             `json:"created_at"`
	CompletedAt        int64             `json:"completed_at,omitempty"`
	ExpiresAt          int64             `json:"expires_at,omitempty"`
	Seconds            string            `json:"seconds,omitempty"`
	Size               string            `json:"size,omitempty"`
	RemixedFromVideoID string            `json:"remixed_from_video_id,omitempty"`
	Error              *OpenAIVideoError `json:"error,omitempty"`
	Metadata           map[string]any    `json:"metadata,omitempty"`
	Usage              *VideoUsage       `json:"usage,omitempty"`
}

// VideoUsage mirrors the compact usage object returned by the official
// Seedance task API. These are retail-equivalent output tokens and are not an
// input to New API's task-pricing settlement.
type VideoUsage struct {
	CompletionTokens int64 `json:"completion_tokens"`
	TotalTokens      int64 `json:"total_tokens"`
}

func (m *OpenAIVideo) SetProgressStr(progress string) {
	progress = strings.TrimSuffix(progress, "%")
	m.Progress, _ = strconv.Atoi(progress)
}
func (m *OpenAIVideo) SetMetadata(k string, v any) {
	if m.Metadata == nil {
		m.Metadata = make(map[string]any)
	}
	m.Metadata[k] = v
}
func NewOpenAIVideo() *OpenAIVideo {
	return &OpenAIVideo{
		Object: "video",
		Status: VideoStatusQueued,
	}
}

type OpenAIVideoError struct {
	Message      string `json:"message"`
	Code         string `json:"code"`
	Param        string `json:"param,omitempty"`
	RequestID    string `json:"request_id,omitempty"`
	Provider     string `json:"provider,omitempty"`
	Category     string `json:"category,omitempty"`
	UpstreamCode string `json:"upstream_code,omitempty"`
	Retryable    *bool  `json:"retryable,omitempty"`
}
