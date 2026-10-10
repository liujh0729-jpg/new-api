package router

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestIsAIPDDSynchronousMediaMatchesCapabilityFlag(t *testing.T) {
	gin.SetMode(gin.TestMode)
	constant.SetAIPDDCapabilities([]constant.AIPDDCapability{
		{ModelName: "ap-agnes-image-2.1-flash", ExecutionProtocol: "token_market_image", Synchronous: true},
		{ModelName: "ap-asr", ExecutionProtocol: "shared_task", Synchronous: true},
		{ModelName: "ap-agnes-video-2.5", ExecutionProtocol: "token_market_video"},
		{ModelName: "ap-indextts-2", ExecutionProtocol: "token_market_audio"},
	})
	t.Cleanup(constant.ResetAIPDDCapabilities)

	ctx, _ := gin.CreateTestContext(nil)
	for _, modelName := range []string{"ap-agnes-image-2.1-flash", "ap-asr"} {
		common.SetContextKey(ctx, constant.ContextKeyOriginalModel, modelName)
		require.True(t, isAIPDDSynchronousMedia(ctx))
	}

	for _, modelName := range []string{"ap-agnes-video-2.5", "ap-indextts-2", "unknown-model"} {
		common.SetContextKey(ctx, constant.ContextKeyOriginalModel, modelName)
		require.False(t, isAIPDDSynchronousMedia(ctx))
	}
}
