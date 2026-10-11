package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/require"
)

// An unreachable Redis must cost the gateway its rate limit, never a customer
// request: the limiter is protection, the compute is the product.
func TestRedisRateLimitFailsOpenWhenRedisIsUnavailable(t *testing.T) {
	gin.SetMode(gin.TestMode)

	originalRDB := common.RDB
	rdb := redis.NewClient(&redis.Options{
		Addr:        "127.0.0.1:1",
		DialTimeout: 100 * time.Millisecond,
		MaxRetries:  -1,
	})
	common.RDB = rdb
	t.Cleanup(func() {
		_ = rdb.Close()
		common.RDB = originalRDB
	})

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	c.Set("id", 42)

	redisRateLimitHandler(60, 100, 50)(c)

	require.False(t, c.IsAborted(), "a Redis failure must not abort the request")
	require.Equal(t, http.StatusOK, recorder.Code)
}
