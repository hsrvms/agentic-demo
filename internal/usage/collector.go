package usage

import (
	"context"
	"encoding/json"
	"log/slog"
	"math"
	"strconv"
	"time"

	"github.com/agentic-demo/platform/internal/db"
	"github.com/redis/go-redis/v9"
)

const (
	defaultFlushInterval  = 30 * time.Second
	defaultRollupInterval = 1 * time.Hour
	flushBatchSize        = 100
	flushPopTimeout       = 1 * time.Second
)

// UsageCollector drains the Redis flush queue and rolls up counters
// into PostgreSQL usage_events and usage_daily tables.
type UsageCollector struct {
	client *redis.Client
	repo   Repository
	logger *slog.Logger

	flushInterval  time.Duration
	rollupInterval time.Duration
}

// CollectorOption configures a UsageCollector.
type CollectorOption func(*UsageCollector)

// WithFlushInterval sets the interval between flush cycles.
func WithFlushInterval(d time.Duration) CollectorOption {
	return func(c *UsageCollector) { c.flushInterval = d }
}

// WithRollupInterval sets the interval between rollup cycles.
func WithRollupInterval(d time.Duration) CollectorOption {
	return func(c *UsageCollector) { c.rollupInterval = d }
}

// NewCollector creates a UsageCollector.
// redisAddr can be a redis:// URL or host:port.
func NewCollector(redisAddr string, repo Repository, logger *slog.Logger, opts ...CollectorOption) (*UsageCollector, error) {
	opt, err := redis.ParseURL(redisAddr)
	if err != nil {
		opt = &redis.Options{Addr: redisAddr}
	}

	c := &UsageCollector{
		client:         redis.NewClient(opt),
		repo:           repo,
		logger:         logger,
		flushInterval:  defaultFlushInterval,
		rollupInterval: defaultRollupInterval,
	}

	for _, o := range opts {
		o(c)
	}

	return c, nil
}

// Close releases the Redis connection.
func (c *UsageCollector) Close() error {
	return c.client.Close()
}

// Start begins the background flush and rollup loops.
// It blocks until ctx is cancelled.
func (c *UsageCollector) Start(ctx context.Context) {
	c.logger.Info("usage collector started",
		"flush_interval", c.flushInterval,
		"rollup_interval", c.rollupInterval,
	)

	flushTicker := time.NewTicker(c.flushInterval)
	defer flushTicker.Stop()

	rollupTicker := time.NewTicker(c.rollupInterval)
	defer rollupTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			c.logger.Info("usage collector stopping")
			// Final flush before exit.
			flushCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			c.flush(flushCtx)
			cancel()
			return

		case <-flushTicker.C:
			c.flush(ctx)

		case <-rollupTicker.C:
			c.rollup(ctx)
		}
	}
}

// flush drains the Redis flush queue and inserts events into usage_events.
func (c *UsageCollector) flush(ctx context.Context) {
	inserted := 0
	for inserted < flushBatchSize {
		// Check context before each pop.
		select {
		case <-ctx.Done():
			return
		default:
		}

		result, err := c.client.BRPop(ctx, flushPopTimeout, flushQueue).Result()
		if err != nil {
			// Timeout or error — stop this flush cycle.
			if err != redis.Nil && err != context.Canceled && err != context.DeadlineExceeded {
				c.logger.Warn("flush: error popping from queue", "error", err)
			}
			break
		}

		// result[0] = key, result[1] = value
		if len(result) < 2 {
			continue
		}

		var event UsageEvent
		if err := json.Unmarshal([]byte(result[1]), &event); err != nil {
			c.logger.Warn("flush: failed to unmarshal event", "error", err)
			continue
		}

		payload, err := json.Marshal(event)
		if err != nil {
			c.logger.Warn("flush: failed to marshal event payload", "error", err)
			continue
		}

		_, err = c.repo.CreateEvent(ctx, &db.CreateUsageEventParams{
			TenantID:  string(event.TenantID),
			EventType: string(event.Type),
			Payload:   payload,
		})
		if err != nil {
			c.logger.Warn("flush: failed to insert event", "error", err)
			// Re-queue the event by pushing it back.
			c.client.LPush(ctx, flushQueue, result[1])
			break
		}
		inserted++
	}

	if inserted > 0 {
		c.logger.Debug("flush: inserted events", "count", inserted)
	}
}

// rollup scans Redis usage counters and upserts into usage_daily.
func (c *UsageCollector) rollup(ctx context.Context) {
	var cursor uint64
	var totalUpserted int

	for {
		keys, nextCursor, err := c.client.Scan(ctx, cursor, "usage:*", 100).Result()
		if err != nil {
			c.logger.Warn("rollup: scan error", "error", err)
			return
		}

		for _, key := range keys {
			// Parse key: usage:{tenant_id}:{date}:{model}
			tenantID, date, model, ok := parseUsageKey(key)
			if !ok {
				continue
			}

			fields, err := c.client.HGetAll(ctx, key).Result()
			if err != nil {
				c.logger.Warn("rollup: HGetAll error", "key", key, "error", err)
				continue
			}
			if len(fields) == 0 {
				continue
			}

			parsedDate, err := time.Parse("2006-01-02", date)
			if err != nil {
				c.logger.Warn("rollup: invalid date in key", "key", key)
				continue
			}

			params := db.UpsertUsageDailyParams{
				TenantID:         tenantID,
				Date:             toPgDate(parsedDate),
				LlmModel:         model,
				InputTokens:      parseFieldInt64(fields, "input_tokens"),
				OutputTokens:     parseFieldInt64(fields, "output_tokens"),
				ToolCalls:        toInt32(parseFieldInt64(fields, "tool_calls")),
				EmbeddingTokens:  parseFieldInt64(fields, "embedding_tokens"),
				EstimatedCostUsd: toPgNumeric(parseFieldFloat64(fields, "estimated_cost_usd")),
				ReportsGenerated: toInt32(parseFieldInt64(fields, "reports_generated")),
			}

			if _, err := c.repo.UpsertDaily(ctx, &params); err != nil {
				c.logger.Warn("rollup: upsert error", "key", key, "error", err)
				continue
			}
			totalUpserted++
		}

		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}

	if totalUpserted > 0 {
		c.logger.Info("rollup: completed", "keys_upserted", totalUpserted)
	}
}

// parseUsageKey extracts tenant_id, date, and model from a Redis usage key.
// Format: usage:{tenant_id}:{date}:{model}
func parseUsageKey(key string) (tenantID, date, model string, ok bool) {
	// Expected format: "usage:{tenant}:{date}:{model}"
	// Skip the "usage:" prefix.
	if len(key) < 7 || key[:6] != "usage:" {
		return "", "", "", false
	}
	rest := key[6:]
	parts := splitN(rest, ":", 3)
	if len(parts) != 3 {
		return "", "", "", false
	}
	return parts[0], parts[1], parts[2], true
}

// splitN splits s by sep into at most n parts.
func splitN(s, sep string, n int) []string {
	result := make([]string, 0, n)
	for i := 0; i < n-1; i++ {
		idx := indexOf(s, sep)
		if idx < 0 {
			result = append(result, s)
			return result
		}
		result = append(result, s[:idx])
		s = s[idx+len(sep):]
	}
	if s != "" {
		result = append(result, s)
	}
	return result
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

func parseFieldInt64(fields map[string]string, key string) int64 {
	v, ok := fields[key]
	if !ok {
		return 0
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return 0
	}
	return n
}

func parseFieldFloat64(fields map[string]string, key string) float64 {
	v, ok := fields[key]
	if !ok {
		return 0
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return 0
	}
	return f
}

// toInt32 clamps a 64-bit counter to the int32 range used by the DB schema.
func toInt32(n int64) int32 {
	if n > math.MaxInt32 {
		return math.MaxInt32
	}
	if n < math.MinInt32 {
		return math.MinInt32
	}
	return int32(n)
}
