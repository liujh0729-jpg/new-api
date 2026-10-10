package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	appi18n "github.com/QuantumNous/new-api/i18n"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

const originTaskAllowedModel = "AP Seedance-2.0 高性价比版"

func TestOriginTaskModelAccessAllowsWhitelistedModel(t *testing.T) {
	ctx := newOriginTaskModelAccessContext(t)
	info := &relaycommon.RelayInfo{
		OriginModelName: originTaskAllowedModel,
		TaskRelayInfo: &relaycommon.TaskRelayInfo{
			OriginTaskID: "task_origin",
		},
	}

	require.False(t, abortIfOriginTaskModelForbidden(ctx, info))
	require.False(t, ctx.IsAborted())
}

func TestOriginTaskModelAccessRejectsNonWhitelistedModel(t *testing.T) {
	ctx := newOriginTaskModelAccessContext(t)
	info := &relaycommon.RelayInfo{
		OriginModelName: "not-allowed",
		TaskRelayInfo: &relaycommon.TaskRelayInfo{
			OriginTaskID: "task_origin",
		},
	}

	require.True(t, abortIfOriginTaskModelForbidden(ctx, info))
	require.True(t, ctx.IsAborted())
	require.Equal(t, http.StatusForbidden, ctx.Writer.Status())
}

func TestOriginTaskModelAccessSkipsOrdinarySubmit(t *testing.T) {
	ctx := newOriginTaskModelAccessContext(t)
	info := &relaycommon.RelayInfo{
		OriginModelName: "not-allowed",
		TaskRelayInfo:   &relaycommon.TaskRelayInfo{},
	}

	require.False(t, abortIfOriginTaskModelForbidden(ctx, info))
	require.False(t, ctx.IsAborted())
}

func newOriginTaskModelAccessContext(t *testing.T) *gin.Context {
	t.Helper()
	require.NoError(t, appi18n.Init())

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/videos/task_origin/remix", nil)
	common.SetContextKey(ctx, constant.ContextKeyTokenModelLimitEnabled, true)
	common.SetContextKey(ctx, constant.ContextKeyTokenModelLimit, map[string]bool{
		originTaskAllowedModel: true,
	})
	return ctx
}
