package queue

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
)

// TenantRateLimiter controls per-tenant concurrency for job processing.
type TenantRateLimiter interface {
	Acquire(ctx context.Context, tenantID string) error
	Release(ctx context.Context, tenantID string)
}

// redisRateLimiter implements TenantRateLimiter using Redis INCR/DECR
// on a per-tenant counter key.
type redisRateLimiter struct {
	client    *redis.Client
	maxActive int
}

// NewRedisRateLimiter creates a TenantRateLimiter backed by Redis.
// redisAddr can be a host:port or redis:// URL.
func NewRedisRateLimiter(redisAddr string, maxActive int) TenantRateLimiter {
	opt, err := redis.ParseURL(redisAddr)
	if err != nil {
		// Treat as host:port.
		opt = &redis.Options{Addr: redisAddr}
	}
	return &redisRateLimiter{
		client:    redis.NewClient(opt),
		maxActive: maxActive,
	}
}

func (r *redisRateLimiter) Acquire(ctx context.Context, tenantID string) error {
	key := fmt.Sprintf("tenant:%s:active_jobs", tenantID)

	// Atomic increment and check using a Lua script to ensure atomicity.
	// The script increments the counter only if it's below the limit.
	script := redis.NewScript(`
		local current = redis.call("GET", KEYS[1])
		if current and tonumber(current) >= tonumber(ARGV[1]) then
			return -1
		end
		return redis.call("INCR", KEYS[1])
	`)

	result, err := script.Run(ctx, r.client, []string{key}, r.maxActive).Int64()
	if err != nil {
		return fmt.Errorf("rate limiter acquire for %s: %w", tenantID, err)
	}
	if result == -1 {
		return fmt.Errorf("rate limit exceeded for tenant %s (max %d active jobs)", tenantID, r.maxActive)
	}
	return nil
}

func (r *redisRateLimiter) Release(ctx context.Context, tenantID string) {
	key := fmt.Sprintf("tenant:%s:active_jobs", tenantID)

	// Decrement but don't go below 0.
	script := redis.NewScript(`
		local current = redis.call("GET", KEYS[1])
		if current and tonumber(current) > 0 then
			return redis.call("DECR", KEYS[1])
		end
		return 0
	`)

	_ = script.Run(ctx, r.client, []string{key}).Err()
}
