package middleware

import (
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/wearwhere/wearwhere_be/internal/auth/domain"
	"github.com/wearwhere/wearwhere_be/pkg/httpx"
)

// RateLimit applies a fixed-window per-user (or per-IP for unauthenticated)
// rate limit of `perMin` requests / minute. Implementation uses Redis INCR
// with a 60s TTL — simple, sufficient for SRS NFR-18 (100 req/min/user).
func RateLimit(rdb *redis.Client, perMin int) gin.HandlerFunc {
	return func(c *gin.Context) {
		subject := c.ClientIP()
		if uid, ok := UserID(c); ok {
			subject = "u:" + uid.String()
		}
		key := fmt.Sprintf("ratelimit:%s", subject)

		n, err := rdb.Incr(c, key).Result()
		if err != nil {
			c.Next() // fail-open: never lock users out because Redis is down
			return
		}
		if n == 1 {
			_ = rdb.Expire(c, key, time.Minute).Err()
		}
		if n > int64(perMin) {
			httpx.ErrorFromApp(c, domain.ErrRateLimited)
			return
		}
		c.Next()
	}
}
