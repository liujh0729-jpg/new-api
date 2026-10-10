package controller

import (
	"github.com/gin-gonic/gin"
)

func PlaygroundVideo(c *gin.Context) {
	if newAPIError := setupPlaygroundTokenContext(c); newAPIError != nil {
		c.JSON(newAPIError.StatusCode, gin.H{
			"error": newAPIError.ToOpenAIError(),
		})
		return
	}

	RelayTask(c)
}
