package router

import (
	"net/http"
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
	require.True(t, routes[http.MethodPost+" /v1/virtual-characters"])
	require.True(t, routes[http.MethodGet+" /v1/virtual-characters/:id"])
}
