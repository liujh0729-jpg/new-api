package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestApplyQueryUserIDHeaderSetsMissingHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/pg/material/file/1?user_id=42", nil)

	applyQueryUserIDHeader(ctx)

	require.Equal(t, "42", ctx.Request.Header.Get("New-Api-User"))
}

func TestApplyQueryUserIDHeaderPreservesExistingHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/pg/material/file/1?user_id=42", nil)
	ctx.Request.Header.Set("New-Api-User", "7")

	applyQueryUserIDHeader(ctx)

	require.Equal(t, "7", ctx.Request.Header.Get("New-Api-User"))
}
