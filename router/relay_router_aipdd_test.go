package router

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestIsAIPDDSynchronousMediaOnlyMatchesTokenMarketImages(t *testing.T) {
	gin.SetMode(gin.TestMode)
	constant.SetAIPDDCapabilities([]constant.AIPDDCapability{
		{ModelName: "ap-agnes-image-2.1-flash", ExecutionProtocol: "token_market_image"},
		{ModelName: "ap-agnes-video-2.5", ExecutionProtocol: "token_market_video"},
	})
	t.Cleanup(constant.ResetAIPDDCapabilities)

	ctx, _ := gin.CreateTestContext(nil)
	common.SetContextKey(ctx, constant.ContextKeyOriginalModel, "ap-agnes-image-2.1-flash")
	require.True(t, isAIPDDSynchronousMedia(ctx))

	common.SetContextKey(ctx, constant.ContextKeyOriginalModel, "ap-agnes-video-2.5")
	require.False(t, isAIPDDSynchronousMedia(ctx))
}
