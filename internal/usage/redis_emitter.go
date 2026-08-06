package usage

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	// flushQueue is the Redis list key for the event flush queue.
	flushQueue = "usage:flush:queue"

	// maxQueueLen caps the flush queue to prevent unbounded growth.
	maxQueueLen = 10000

	// keyTTL is the TTL for usage counter hash keys in Redis.
	keyTTL = 48 * time.Hour
)

// RedisEmitter implements UsageEmitter with atomic Redis counters
// and a push-based flush queue for the collector.
type RedisEmitter struct {
	client  *redis.Client
	nowFunc func() time.Time
}

// NewRedisEmitter creates a Redis-backed UsageEmitter.
// redisAddr can be a redis:// URL or host:port.
func NewRedisEmitter(redisAddr string) (*RedisEmitter, error) {
	opt, err := redis.ParseURL(redisAddr)
	if err != nil {
		opt = &redis.Options{Addr: redisAddr}
	}
	return &RedisEmitter{
		client:  redis.NewClient(opt),
		nowFunc: time.Now,
	}, nil
}

// Close releases the Redis connection.
func (e *RedisEmitter) Close() error {
	return e.client.Close()
}

// Client returns the underlying Redis client for use by the collector.
func (e *RedisEmitter) Client() *redis.Client {
	return e.client
}

// EmitUsage increments Redis counters for the event and pushes to the flush queue.
func (e *RedisEmitter) EmitUsage(ctx context.Context, event UsageEvent) error {
	now := e.nowFunc
	if now == nil {
		now = time.Now
	}
	date := now().UTC().Format("2006-01-02")
	model := eventModel(event)

	key := redisKey(string(event.TenantID), date, model)

	pipe := e.client.Pipeline()

	switch event.Type {
	case EventLLM:
		if event.LLM != nil {
			pipe.HIncrBy(ctx, key, "input_tokens", int64(event.LLM.InputTokens))
			pipe.HIncrBy(ctx, key, "output_tokens", int64(event.LLM.OutputTokens))
			pipe.HIncrBy(ctx, key, "reports_generated", 1)
			cost := computeCost(model, event.LLM.InputTokens, event.LLM.OutputTokens)
			pipe.HIncrByFloat(ctx, key, "estimated_cost_usd", cost)
		}
	case EventTool:
		pipe.HIncrBy(ctx, key, "tool_calls", 1)
	case EventEmbedding:
		if event.Embedding != nil {
			pipe.HIncrBy(ctx, key, "embedding_tokens", int64(event.Embedding.ChunksProcessed))
			cost := computeCost(model, event.Embedding.ChunksProcessed, 0)
			pipe.HIncrByFloat(ctx, key, "estimated_cost_usd", cost)
		}
	}

	pipe.Expire(ctx, key, keyTTL)

	// Serialize event and push to flush queue.
	serialized, err := json.Marshal(event)
	if err != nil {
		// Counters are already incremented — log and continue.
		_, _ = pipe.Exec(ctx)
		return fmt.Errorf("serialize usage event: %w", err)
	}
	pipe.LPush(ctx, flushQueue, string(serialized))
	pipe.LTrim(ctx, flushQueue, 0, int64(maxQueueLen-1))

	_, err = pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("emit usage: %w", err)
	}
	return nil
}

// redisKey builds the Redis hash key for usage counters.
func redisKey(tenantID, date, model string) string {
	return fmt.Sprintf("usage:%s:%s:%s", tenantID, date, model)
}

// eventModel extracts the model name from a usage event.
// Uses sentinel values for events without an explicit model.
func eventModel(event UsageEvent) string {
	switch event.Type {
	case EventLLM:
		if event.LLM != nil && event.LLM.Model != "" {
			return event.LLM.Model
		}
		return "_unknown_"
	case EventTool:
		if event.Tool != nil && event.Tool.Model != "" {
			return event.Tool.Model
		}
		return "_tool_"
	case EventEmbedding:
		if event.Embedding != nil && event.Embedding.Model != "" {
			return event.Embedding.Model
		}
		return "_embedding_"
	default:
		return "_unknown_"
	}
}
