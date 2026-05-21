package middleware

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
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
