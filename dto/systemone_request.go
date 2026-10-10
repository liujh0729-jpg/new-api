package dto

import (
	"encoding/json"
	"strings"

	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

// SystemOneRequest is the decision-model request served on /v1/systemone.
// Upstream, the payload is not OpenAI-shaped: besides the model it carries the
// decision "state" and a "questions" object keyed by question id. Both are
// opaque here so they reach upstream unchanged.
type SystemOneRequest struct {
	Model     string          `json:"model"`
	State     json.RawMessage `json:"state,omitempty"`
	Questions json.RawMessage `json:"questions,omitempty"`
}

func (r *SystemOneRequest) IsStream(c *gin.Context) bool {
	return false
}

func (r *SystemOneRequest) GetTokenCountMeta() *types.TokenCountMeta {
	return &types.TokenCountMeta{
		CombineText: strings.TrimSpace(string(r.State) + "\n" + string(r.Questions)),
	}
}

func (r *SystemOneRequest) SetModelName(modelName string) {
	if modelName != "" {
		r.Model = modelName
	}
}
