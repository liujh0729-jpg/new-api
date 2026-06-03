package controller

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"

	"github.com/gin-gonic/gin"
)

type playgroundSeedanceVideoRequest struct {
	Model string `json:"model"`
}

func PlaygroundSeedanceVideo(c *gin.Context) {
	if newAPIError := setupPlaygroundTokenContext(c); newAPIError != nil {
		c.JSON(newAPIError.StatusCode, gin.H{
			"error": newAPIError.ToOpenAIError(),
		})
		return
	}

	var req playgroundSeedanceVideoRequest
	if err := common.UnmarshalBodyReusable(c, &req); err != nil {
		playgroundSeedanceVideoError(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if !isSeedance20VideoModel(req.Model) {
		playgroundSeedanceVideoError(c, http.StatusBadRequest, "invalid_model", fmt.Sprintf("model %q is not a Seedance 2.0 video model", req.Model))
		return
	}
	RelayTask(c)
}

func playgroundSeedanceVideoError(c *gin.Context, status int, code string, message string) {
	c.JSON(status, gin.H{
		"error": gin.H{
			"code":    code,
			"message": message,
			"type":    "invalid_request_error",
		},
	})
}

func isSeedance20VideoModel(modelName string) bool {
	modelName = strings.ToLower(strings.TrimSpace(modelName))
	compact := strings.NewReplacer(" ", "-", "_", "-", ".", "-").Replace(modelName)
	return strings.Contains(modelName, "seedance-2-0") || strings.Contains(modelName, "seedance-2.0") || strings.Contains(compact, "seedance-2-0")
}
