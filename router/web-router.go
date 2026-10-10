package router

import (
	"embed"
	"net/http"
	"path"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/gin-contrib/gzip"
	"github.com/gin-contrib/static"
	"github.com/gin-gonic/gin"
)

// WebAssets holds the embedded frontend assets.
type WebAssets struct {
	BuildFS   embed.FS
	IndexPage []byte
}

func SetWebRouter(router *gin.Engine, assets WebAssets) {
	webFS := common.EmbedFolder(assets.BuildFS, "web/default/dist")
	router.Use(gzip.Gzip(gzip.DefaultCompression))
	router.Use(middleware.GlobalWebRateLimit())
	router.Use(middleware.Cache())
	router.Use(static.Serve("/", webFS))
	router.NoRoute(func(c *gin.Context) {
		c.Set(middleware.RouteTagKey, "web")
		requestPath := c.Request.URL.Path
		if strings.HasPrefix(requestPath, "/v1") ||
			strings.HasPrefix(requestPath, "/api") ||
			strings.HasPrefix(requestPath, "/assets") ||
			strings.HasPrefix(requestPath, "/static") ||
			path.Ext(requestPath) != "" {
			controller.RelayNotFound(c)
			return
		}
		c.Header("Cache-Control", "no-store, no-cache, must-revalidate, proxy-revalidate, max-age=0")
		c.Header("Pragma", "no-cache")
		c.Header("Expires", "0")
		c.Data(http.StatusOK, "text/html; charset=utf-8", assets.IndexPage)
	})
}
