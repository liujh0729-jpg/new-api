package middleware

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	appi18n "github.com/QuantumNous/new-api/i18n"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

const seedanceLimitedModel = "AP Seedance-2.0 高性价比版"

func TestGetModelRequestReadsMultipartImageGenerationModel(t *testing.T) {
	ctx := newMultipartModelRequest(t, "/v1/images/generations", "aipdd-flux-gguf")

	modelRequest, shouldSelectChannel, err := getModelRequest(ctx)
	if err != nil {
		t.Fatalf("getModelRequest returned error: %v", err)
	}
	if !shouldSelectChannel {
		t.Fatal("image generation submit should select a channel")
	}
	if modelRequest.Model != "aipdd-flux-gguf" {
		t.Fatalf("unexpected model: %q", modelRequest.Model)
	}
}

func TestGetModelRequestReadsMultipartAudioSpeechModel(t *testing.T) {
	ctx := newMultipartModelRequest(t, "/v1/audio/speech", "aipdd-indextts")

	modelRequest, shouldSelectChannel, err := getModelRequest(ctx)
	if err != nil {
		t.Fatalf("getModelRequest returned error: %v", err)
	}
	if !shouldSelectChannel {
		t.Fatal("audio speech submit should select a channel")
	}
	if modelRequest.Model != "aipdd-indextts" {
		t.Fatalf("unexpected model: %q", modelRequest.Model)
	}
}

func TestDistributeAllowsOpenAIVideoFetchWithModelLimitedToken(t *testing.T) {
	ctx := newModelLimitedDistributorContext(t, http.MethodGet, "/v1/videos/task_123", "")

	Distribute()(ctx)

	require.False(t, ctx.IsAborted())
}

func TestDistributeAllowsCompatibleVideoFetchWithModelLimitedToken(t *testing.T) {
	ctx := newModelLimitedDistributorContext(t, http.MethodGet, "/v1/video/generations/task_123", "")

	Distribute()(ctx)

	require.False(t, ctx.IsAborted())
}

func TestGetModelRequestRecognizesSeedanceOfficialSubmit(t *testing.T) {
	ctx := newJSONModelRequest(http.MethodPost, "/api/v3/contents/generations/tasks", `{"model":"AP Seedance-2.0 高性价比版"}`)

	modelRequest, shouldSelectChannel, err := getModelRequest(ctx)

	require.NoError(t, err)
	require.True(t, shouldSelectChannel)
	require.Equal(t, seedanceLimitedModel, modelRequest.Model)
	require.Equal(t, relayconstant.RelayModeVideoSubmit, ctx.GetInt("relay_mode"))
}

func TestGetModelRequestRecognizesSeedanceOfficialFetch(t *testing.T) {
	ctx := newJSONModelRequest(http.MethodGet, "/api/v3/contents/generations/tasks/task_123", "")

	modelRequest, shouldSelectChannel, err := getModelRequest(ctx)

	require.NoError(t, err)
	require.False(t, shouldSelectChannel)
	require.Empty(t, modelRequest.Model)
	require.Equal(t, relayconstant.RelayModeVideoFetchByID, ctx.GetInt("relay_mode"))
}

func TestDistributeRejectsDisallowedVideoSubmitWithModelLimitedToken(t *testing.T) {
	ctx := newModelLimitedDistributorContext(t, http.MethodPost, "/v1/videos", `{"model":"not-allowed"}`)

	Distribute()(ctx)

	require.True(t, ctx.IsAborted())
	require.Equal(t, http.StatusForbidden, ctx.Writer.Status())
}

func TestAbortIfTokenModelForbiddenAllowsResolvedModel(t *testing.T) {
	ctx := newModelLimitedDistributorContext(t, http.MethodPost, "/v1/videos", "")

	require.False(t, AbortIfTokenModelForbidden(ctx, seedanceLimitedModel))
	require.False(t, ctx.IsAborted())
}

func TestPlaygroundGroupOverrideAllowsAutoWhenUserHasAutoGroup(t *testing.T) {
	restoreGroupSettings(t)
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"默认分组"}`))
	require.NoError(t, setting.UpdateAutoGroupsByJsonString(`["default"]`))

	require.True(t, isPlaygroundGroupOverrideAllowed("default", "auto"))
	require.True(t, isPlaygroundGroupOverrideAllowed("default", "default"))
	require.False(t, isPlaygroundGroupOverrideAllowed("default", "vip"))
}

func TestPlaygroundGroupOverrideRejectsAutoWithoutUsableAutoGroup(t *testing.T) {
	restoreGroupSettings(t)
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"默认分组"}`))
	require.NoError(t, setting.UpdateAutoGroupsByJsonString(`["vip"]`))

	require.False(t, isPlaygroundGroupOverrideAllowed("default", "auto"))
}

func restoreGroupSettings(t *testing.T) {
	t.Helper()

	originalAutoGroups := setting.AutoGroups2JsonString()
	originalUserUsableGroups := setting.UserUsableGroups2JSONString()
	t.Cleanup(func() {
		require.NoError(t, setting.UpdateAutoGroupsByJsonString(originalAutoGroups))
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(originalUserUsableGroups))
	})
}

func newMultipartModelRequest(t *testing.T, path, model string) *gin.Context {
	t.Helper()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("model", model); err != nil {
		t.Fatalf("write model field: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, path, &body)
	ctx.Request.Header.Set("Content-Type", writer.FormDataContentType())
	return ctx
}

func newJSONModelRequest(method, path, body string) *gin.Context {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		ctx.Request.Header.Set("Content-Type", "application/json")
	}
	return ctx
}

func newModelLimitedDistributorContext(t *testing.T, method, path, body string) *gin.Context {
	t.Helper()
	require.NoError(t, appi18n.Init())

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		ctx.Request.Header.Set("Content-Type", "application/json")
	}
	common.SetContextKey(ctx, constant.ContextKeyTokenModelLimitEnabled, true)
	common.SetContextKey(ctx, constant.ContextKeyTokenModelLimit, map[string]bool{
		seedanceLimitedModel: true,
	})
	return ctx
}
