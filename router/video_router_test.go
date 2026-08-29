package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestVideoRouterRegistersAPIKeyVirtualCharacterEndpoints(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	SetVideoRouter(engine)

	routes := make(map[string]bool)
	for _, route := range engine.Routes() {
		routes[route.Method+" "+route.Path] = true
	}
	require.True(t, routes[http.MethodGet+" /v1/virtual-characters"])
	require.True(t, routes[http.MethodPost+" /v1/virtual-characters"])
	require.True(t, routes[http.MethodGet+" /v1/virtual-characters/:id"])
	require.True(t, routes[http.MethodPost+" /api/v3/contents/generations/tasks"])
	require.True(t, routes[http.MethodGet+" /api/v3/contents/generations/tasks/:task_id"])
	require.True(t, routes[http.MethodPost+" /v1/videos"])
	require.True(t, routes[http.MethodGet+" /v1/videos/:task_id"])
	require.False(t, routes[http.MethodPost+" /v1/video/generations"])
	require.False(t, routes[http.MethodGet+" /v1/video/generations/:task_id"])

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v3/contents/generations/tasks", nil)
	engine.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusUnauthorized, recorder.Code)
}
