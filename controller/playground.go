package controller

import (
	"errors"
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

func setupPlaygroundTokenContext(c *gin.Context) *types.NewAPIError {
	useAccessToken := c.GetBool("use_access_token")
	if useAccessToken {
		return types.NewError(errors.New("暂不支持使用 access token"), types.ErrorCodeAccessDenied, types.ErrOptionWithSkipRetry())
	}

	userId := c.GetInt("id")

	// Write user context to ensure acceptUnsetRatio is available.
	userCache, err := model.GetUserCache(userId)
	if err != nil {
		return types.NewError(err, types.ErrorCodeQueryDataError, types.ErrOptionWithSkipRetry())
	}
	userCache.WriteContext(c)

	usingGroup := common.GetContextKeyString(c, constant.ContextKeyUsingGroup)
	if c.Request.Method != "GET" {
		playgroundRequest := &dto.PlayGroundRequest{}
		if err := common.UnmarshalBodyReusable(c, playgroundRequest); err != nil {
			return types.NewError(err, types.ErrorCodeInvalidRequest, types.ErrOptionWithSkipRetry())
		}
		if playgroundRequest.Group != "" {
			usingGroup = playgroundRequest.Group
			common.SetContextKey(c, constant.ContextKeyUsingGroup, usingGroup)
			common.SetContextKey(c, constant.ContextKeyTokenGroup, usingGroup)
		}
	}
	if usingGroup == "" {
		usingGroup = userCache.Group
	}

	tempToken := &model.Token{
		UserId: userId,
		Name:   fmt.Sprintf("playground-%s", usingGroup),
		Group:  usingGroup,
	}
	_ = middleware.SetupContextForToken(c, tempToken)
	return nil
}

func Playground(c *gin.Context, relayFormat types.RelayFormat) {
	var newAPIError *types.NewAPIError

	defer func() {
		if newAPIError != nil {
			c.JSON(newAPIError.StatusCode, gin.H{
				"error": newAPIError.ToOpenAIError(),
			})
		}
	}()

	newAPIError = setupPlaygroundTokenContext(c)
	if newAPIError != nil {
		return
	}

	Relay(c, relayFormat)
}

func PlaygroundTask(c *gin.Context) {
	if newAPIError := setupPlaygroundTokenContext(c); newAPIError != nil {
		c.JSON(newAPIError.StatusCode, gin.H{
			"error": newAPIError.ToOpenAIError(),
		})
		return
	}

	RelayTask(c)
}
