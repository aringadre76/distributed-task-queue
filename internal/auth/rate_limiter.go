package auth

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"go.uber.org/zap"
)

type RateLimiter struct {
	redis  *redis.Client
	logger *zap.Logger
}

type RateLimitConfig struct {
	RequestsPerMinute int
	BurstLimit        int
	WindowDuration    time.Duration
}

type RateLimitResult struct {
	Allowed       bool
	Limit         int
	Remaining     int
	ResetTime     time.Time
	RetryAfter    time.Duration
}

func NewRateLimiter(redisClient *redis.Client, logger *zap.Logger) *RateLimiter {
	return &RateLimiter{
		redis:  redisClient,
		logger: logger,
	}
}

func (rl *RateLimiter) CheckLimit(ctx context.Context, key string, config RateLimitConfig) (*RateLimitResult, error) {
	now := time.Now()
	window := now.Truncate(config.WindowDuration)
	redisKey := fmt.Sprintf("rate_limit:%s:%d", key, window.Unix())

	pipe := rl.redis.Pipeline()
	
	incrCmd := pipe.Incr(ctx, redisKey)
	expireCmd := pipe.Expire(ctx, redisKey, config.WindowDuration)
	
	_, err := pipe.Exec(ctx)
	if err != nil {
		return nil, err
	}

	count := int(incrCmd.Val())
	
	if count == 1 {
		expireCmd.Err()
	}

	resetTime := window.Add(config.WindowDuration)
	remaining := config.RequestsPerMinute - count
	if remaining < 0 {
		remaining = 0
	}

	result := &RateLimitResult{
		Allowed:    count <= config.RequestsPerMinute,
		Limit:      config.RequestsPerMinute,
		Remaining:  remaining,
		ResetTime:  resetTime,
		RetryAfter: time.Until(resetTime),
	}

	return result, nil
}

func (rl *RateLimiter) RateLimitMiddleware(config RateLimitConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		key := rl.generateKey(c)
		
		result, err := rl.CheckLimit(c.Request.Context(), key, config)
		if err != nil {
			rl.logger.Error("Rate limit check failed", zap.Error(err))
			c.Next()
			return
		}

		c.Header("X-RateLimit-Limit", strconv.Itoa(result.Limit))
		c.Header("X-RateLimit-Remaining", strconv.Itoa(result.Remaining))
		c.Header("X-RateLimit-Reset", strconv.FormatInt(result.ResetTime.Unix(), 10))

		if !result.Allowed {
			c.Header("Retry-After", strconv.Itoa(int(result.RetryAfter.Seconds())))
			
			rl.logger.Warn("Rate limit exceeded",
				zap.String("key", key),
				zap.String("ip", c.ClientIP()),
				zap.String("path", c.Request.URL.Path))
			
			c.JSON(429, gin.H{
				"error":       "rate limit exceeded",
				"retry_after": result.RetryAfter.Seconds(),
				"limit":       result.Limit,
				"reset_time":  result.ResetTime.Unix(),
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

func (rl *RateLimiter) TenantRateLimitMiddleware(defaultConfig RateLimitConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		tenantID := GetTenantID(c)
		if tenantID == "" {
			c.Next()
			return
		}

		config := rl.getTenantConfig(tenantID, defaultConfig)
		key := fmt.Sprintf("tenant:%s", tenantID)
		
		result, err := rl.CheckLimit(c.Request.Context(), key, config)
		if err != nil {
			rl.logger.Error("Tenant rate limit check failed", zap.Error(err))
			c.Next()
			return
		}

		c.Header("X-RateLimit-Limit", strconv.Itoa(result.Limit))
		c.Header("X-RateLimit-Remaining", strconv.Itoa(result.Remaining))
		c.Header("X-RateLimit-Reset", strconv.FormatInt(result.ResetTime.Unix(), 10))

		if !result.Allowed {
			c.Header("Retry-After", strconv.Itoa(int(result.RetryAfter.Seconds())))
			
			rl.logger.Warn("Tenant rate limit exceeded",
				zap.String("tenant_id", tenantID),
				zap.String("ip", c.ClientIP()),
				zap.String("path", c.Request.URL.Path))
			
			c.JSON(429, gin.H{
				"error":       "tenant rate limit exceeded",
				"retry_after": result.RetryAfter.Seconds(),
				"limit":       result.Limit,
				"reset_time":  result.ResetTime.Unix(),
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

func (rl *RateLimiter) UserRateLimitMiddleware(config RateLimitConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := GetUserID(c)
		if userID == "" {
			c.Next()
			return
		}

		key := fmt.Sprintf("user:%s", userID)
		
		result, err := rl.CheckLimit(c.Request.Context(), key, config)
		if err != nil {
			rl.logger.Error("User rate limit check failed", zap.Error(err))
			c.Next()
			return
		}

		c.Header("X-RateLimit-Limit", strconv.Itoa(result.Limit))
		c.Header("X-RateLimit-Remaining", strconv.Itoa(result.Remaining))
		c.Header("X-RateLimit-Reset", strconv.FormatInt(result.ResetTime.Unix(), 10))

		if !result.Allowed {
			c.Header("Retry-After", strconv.Itoa(int(result.RetryAfter.Seconds())))
			
			rl.logger.Warn("User rate limit exceeded",
				zap.String("user_id", userID),
				zap.String("ip", c.ClientIP()),
				zap.String("path", c.Request.URL.Path))
			
			c.JSON(429, gin.H{
				"error":       "user rate limit exceeded",
				"retry_after": result.RetryAfter.Seconds(),
				"limit":       result.Limit,
				"reset_time":  result.ResetTime.Unix(),
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

func (rl *RateLimiter) generateKey(c *gin.Context) string {
	userID := GetUserID(c)
	if userID != "" {
		return fmt.Sprintf("user:%s", userID)
	}

	tenantID := GetTenantID(c)
	if tenantID != "" {
		return fmt.Sprintf("tenant:%s", tenantID)
	}

	return fmt.Sprintf("ip:%s", c.ClientIP())
}

func (rl *RateLimiter) getTenantConfig(tenantID string, defaultConfig RateLimitConfig) RateLimitConfig {
	configKey := fmt.Sprintf("tenant_config:%s", tenantID)
	
	limit, err := rl.redis.HGet(context.Background(), configKey, "requests_per_minute").Int()
	if err == nil && limit > 0 {
		defaultConfig.RequestsPerMinute = limit
	}

	burst, err := rl.redis.HGet(context.Background(), configKey, "burst_limit").Int()
	if err == nil && burst > 0 {
		defaultConfig.BurstLimit = burst
	}

	return defaultConfig
}

func (rl *RateLimiter) SetTenantLimits(ctx context.Context, tenantID string, config RateLimitConfig) error {
	configKey := fmt.Sprintf("tenant_config:%s", tenantID)
	
	pipe := rl.redis.Pipeline()
	pipe.HSet(ctx, configKey, "requests_per_minute", config.RequestsPerMinute)
	pipe.HSet(ctx, configKey, "burst_limit", config.BurstLimit)
	pipe.Expire(ctx, configKey, 24*time.Hour)
	
	_, err := pipe.Exec(ctx)
	return err
}

func (rl *RateLimiter) GetLimitStatus(ctx context.Context, key string, config RateLimitConfig) (*RateLimitResult, error) {
	now := time.Now()
	window := now.Truncate(config.WindowDuration)
	redisKey := fmt.Sprintf("rate_limit:%s:%d", key, window.Unix())

	count, err := rl.redis.Get(ctx, redisKey).Int()
	if err != nil {
		if err == redis.Nil {
			count = 0
		} else {
			return nil, err
		}
	}

	resetTime := window.Add(config.WindowDuration)
	remaining := config.RequestsPerMinute - count
	if remaining < 0 {
		remaining = 0
	}

	return &RateLimitResult{
		Allowed:    count < config.RequestsPerMinute,
		Limit:      config.RequestsPerMinute,
		Remaining:  remaining,
		ResetTime:  resetTime,
		RetryAfter: time.Until(resetTime),
	}, nil
} 