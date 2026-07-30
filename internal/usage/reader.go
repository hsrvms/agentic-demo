package usage

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// UsageReader reads real-time usage counters from Redis.
type UsageReader interface {
	GetCurrentUsage(ctx context.Context, tenantID string) (*CurrentUsage, error)
	Close() error
}

// redisReader implements UsageReader using Redis hash keys.
type redisReader struct {
	client *redis.Client
}

// NewRedisReader creates a UsageReader backed by Redis.
// redisAddr can be a redis:// URL or host:port.
func NewRedisReader(redisAddr string) (UsageReader, error) {
	opt, err := redis.ParseURL(redisAddr)
	if err != nil {
		opt = &redis.Options{Addr: redisAddr}
	}
	return &redisReader{
		client: redis.NewClient(opt),
	}, nil
}

// Close releases the Redis connection.
func (r *redisReader) Close() error {
	return r.client.Close()
}

// GetCurrentUsage scans all usage hash keys for the given tenant
// and aggregates them into a CurrentUsage snapshot for the current month.
func (r *redisReader) GetCurrentUsage(ctx context.Context, tenantID string) (*CurrentUsage, error) {
	now := time.Now().UTC()
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	monthEnd := monthStart.AddDate(0, 1, 0).Add(-time.Second)

	prefix := fmt.Sprintf("usage:%s:", tenantID)

	result := &CurrentUsage{
		TenantID:    tenantID,
		PeriodStart: monthStart,
		PeriodEnd:   monthEnd,
		ByModel:     make([]ModelUsage, 0),
	}

	var cursor uint64
	for {
		keys, nextCursor, err := r.client.Scan(ctx, cursor, prefix+"*", 100).Result()
		if err != nil {
			return nil, fmt.Errorf("scan usage keys: %w", err)
		}

		for _, key := range keys {
			_, dateStr, model, ok := parseUsageKey(key)
			if !ok {
				continue
			}

			// Only include keys for the current month.
			parsedDate, err := time.Parse("2006-01-02", dateStr)
			if err != nil {
				continue
			}
			if parsedDate.Before(monthStart) || parsedDate.After(monthEnd) {
				continue
			}

			fields, err := r.client.HGetAll(ctx, key).Result()
			if err != nil {
				continue
			}

			inputTokens := parseFieldInt64(fields, "input_tokens")
			outputTokens := parseFieldInt64(fields, "output_tokens")
			toolCalls := parseFieldInt64(fields, "tool_calls")
			embeddingTokens := parseFieldInt64(fields, "embedding_tokens")
			cost := parseFieldFloat64(fields, "estimated_cost_usd")
			reports := parseFieldInt64(fields, "reports_generated")

			result.TotalInputTokens += inputTokens
			result.TotalOutputTokens += outputTokens
			result.TotalToolCalls += toolCalls
			result.TotalEmbeddingTokens += embeddingTokens
			result.TotalCostUSD += cost
			result.ReportsGenerated += reports

			result.ByModel = append(result.ByModel, ModelUsage{
				Model:           model,
				InputTokens:     inputTokens,
				OutputTokens:    outputTokens,
				ToolCalls:       toolCalls,
				EmbeddingTokens: embeddingTokens,
				CostUSD:         cost,
			})
		}

		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}

	return result, nil
}