package middleware

import (
	"path"
	"strings"

	"github.com/gin-gonic/gin"
)

const (
	noStoreCacheControl    = "no-store, no-cache, must-revalidate, proxy-revalidate, max-age=0"
	immutableCacheControl  = "public, max-age=31536000, immutable"
	revalidateCacheControl = "no-cache"
)

func Cache() func(c *gin.Context) {
	return func(c *gin.Context) {
		requestPath := c.Request.URL.Path
		baseName := path.Base(requestPath)

		switch {
		case requestPath == "/" || requestPath == "/index.html":
			setNoStoreHeaders(c)
		case !strings.Contains(baseName, "."):
			setNoStoreHeaders(c)
		case strings.HasPrefix(requestPath, "/static/"):
			c.Header("Cache-Control", immutableCacheControl)
		default:
			c.Header("Cache-Control", revalidateCacheControl)
		}
		c.Header("Cache-Version", "b688f2fb5be447c25e5aa3bd063087a83db32a288bf6a4f35f2d8db310e40b14")
		c.Next()
	}
}

func setNoStoreHeaders(c *gin.Context) {
	c.Header("Cache-Control", noStoreCacheControl)
	c.Header("Pragma", "no-cache")
	c.Header("Expires", "0")
}
