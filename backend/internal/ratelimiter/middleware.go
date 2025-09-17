package ratelimit

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"backend/internal/auth"
	"backend/pkg/logger"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

type RateLimiter struct {
	rdb       *redis.Client
	limit     int
	window    time.Duration
	keyPrefix string
}

func NewRateLimiter(rdb *redis.Client, limit int, window time.Duration) *RateLimiter {
	return &RateLimiter{
		rdb:       rdb,
		limit:     limit,
		window:    window,
		keyPrefix: "ratelimit:",
	}
}

func (rl *RateLimiter) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		userID := auth.GetUserIDFromCtx(ctx)
		if userID == "" {
			userID = c.ClientIP()
		}

		key := rl.keyPrefix + userID + ":" + strconv.FormatInt(time.Now().Unix()/int64(rl.window.Seconds()), 10)
		fmt.Printf("key: %s\n", key)
		val, err := rl.rdb.Incr(ctx, key).Result()
		if err != nil {
			logger.L.Error("redis incr failed", zap.Error(err))
			c.Next()
			return
		}
		if val == 1 {
			_ = rl.rdb.Expire(ctx, key, rl.window).Err()
		}

		if val > int64(rl.limit) {
			c.JSON(http.StatusTooManyRequests, gin.H{
				"error":      "rate limit exceeded",
				"limit":      rl.limit,
				"windowSecs": int(rl.window.Seconds()),
			})
			c.Abort()
			return
		}

		c.Next()
	}
}
