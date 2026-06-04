package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestCacheHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name         string
		path         string
		cacheControl string
		pragma       string
		expires      string
	}{
		{
			name:         "root index",
			path:         "/",
			cacheControl: noStoreCacheControl,
			pragma:       "no-cache",
			expires:      "0",
		},
		{
			name:         "explicit index html",
			path:         "/index.html",
			cacheControl: noStoreCacheControl,
			pragma:       "no-cache",
			expires:      "0",
		},
		{
			name:         "spa route",
			path:         "/console/log",
			cacheControl: noStoreCacheControl,
			pragma:       "no-cache",
			expires:      "0",
		},
		{
			name:         "hashed static asset",
			path:         "/static/js/index.5603e36385.js",
			cacheControl: immutableCacheControl,
		},
		{
			name:         "public asset without hash",
			path:         "/logo.png",
			cacheControl: revalidateCacheControl,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := gin.New()
			router.Use(Cache())
			router.GET("/*path", func(c *gin.Context) {
				c.Status(http.StatusNoContent)
			})

			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			rec := httptest.NewRecorder()

			router.ServeHTTP(rec, req)

			if got := rec.Header().Get("Cache-Control"); got != tt.cacheControl {
				t.Fatalf("Cache-Control = %q, want %q", got, tt.cacheControl)
			}
			if got := rec.Header().Get("Pragma"); got != tt.pragma {
				t.Fatalf("Pragma = %q, want %q", got, tt.pragma)
			}
			if got := rec.Header().Get("Expires"); got != tt.expires {
				t.Fatalf("Expires = %q, want %q", got, tt.expires)
			}
		})
	}
}
