package middleware

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

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
