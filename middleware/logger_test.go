package middleware

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSafeGinLogPathRedactsVirtualCharacterCallbackQuery(t *testing.T) {
	request := httptest.NewRequest("GET", "/api/virtual-characters/validation/callback?bytedToken=secret-token&resultCode=10000&state=secret-state", nil)
	path := safeGinLogPath(gin.LogFormatterParams{Request: request, Path: request.URL.RequestURI()})
	require.Equal(t, "/api/virtual-characters/validation/callback", path)
	require.NotContains(t, path, "secret-token")
	require.NotContains(t, path, "secret-state")
}
