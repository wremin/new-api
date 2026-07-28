package middleware

import (
	"fmt"
	"net/http"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/gin-gonic/gin"
)

// assetsRateLimiter 独立于模型请求限流器，避免素材接口的调用挤占模型限流配额。
var assetsRateLimiter common.InMemoryRateLimiter

const assetsRateLimitWindowSeconds int64 = 60

func init() {
	assetsRateLimiter.Init(common.RateLimitKeyExpirationDuration)
}

// AssetsRateLimit 按用户维度限制素材接口调用频次。
//
// 素材接口完全免费且不写日志，限流是本期唯一的滥用防线：
// 全站共用一个上游账号，单个用户高频刷量会挤占上游审核队列。
func AssetsRateLimit() func(c *gin.Context) {
	return func(c *gin.Context) {
		maxCount := operation_setting.GetAssetsRateLimitCount()
		if maxCount <= 0 {
			c.Next()
			return
		}

		userId := c.GetInt("id")
		key := fmt.Sprintf("assets_rate_limit:%d", userId)

		if common.RedisEnabled {
			ok, err := redisAssetsRateLimit(c, key, maxCount)
			if err != nil {
				// Redis 异常时放行，不因限流组件故障影响正常功能
				c.Next()
				return
			}
			if !ok {
				abortAssetsRateLimit(c, maxCount)
				return
			}
			c.Next()
			return
		}

		if !assetsRateLimiter.Request(key, maxCount, assetsRateLimitWindowSeconds) {
			abortAssetsRateLimit(c, maxCount)
			return
		}
		c.Next()
	}
}

func abortAssetsRateLimit(c *gin.Context, maxCount int) {
	c.JSON(http.StatusTooManyRequests, gin.H{
		"error": gin.H{
			"message": fmt.Sprintf("asset api rate limit exceeded, max %d requests per minute", maxCount),
			"type":    "invalid_request_error",
			"code":    "assets_rate_limit_exceeded",
		},
	})
	c.Abort()
}

func redisAssetsRateLimit(c *gin.Context, key string, maxCount int) (bool, error) {
	ctx := c.Request.Context()
	rdb := common.RDB
	if rdb == nil {
		return true, nil
	}
	count, err := rdb.Incr(ctx, key).Result()
	if err != nil {
		return false, err
	}
	if count == 1 {
		// 首次计数时设置窗口过期时间
		if err := rdb.Expire(ctx, key, time.Duration(assetsRateLimitWindowSeconds)*time.Second).Err(); err != nil {
			return false, err
		}
	}
	return count <= int64(maxCount), nil
}
