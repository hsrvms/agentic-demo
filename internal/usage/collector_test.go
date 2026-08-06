package usage

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

func TestCollector_FlushesEvents(t *testing.T) {
	mr := miniredis.NewMiniRedis()
	require.NoError(t, mr.Start())
	defer mr.Close()

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	repo := &mockRepository{}
	logger := testLogger()

	collector := &UsageCollector{
		client:         client,
		repo:           repo,
		logger:         logger,
		flushInterval:  50 * time.Millisecond,
		rollupInterval: time.Hour, // effectively disabled for this test
	}

	// Push an event to the flush queue.
	event := UsageEvent{
		Type:     EventLLM,
		TenantID: "tenant-1",
		LLM:      &LLMUsage{Model: "qwen-max", InputTokens: 100, OutputTokens: 50},
	}
	serialized, err := json.Marshal(event)
	require.NoError(t, err)
	require.NoError(t, client.LPush(context.Background(), flushQueue, string(serialized)).Err())

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	go collector.Start(ctx)

	// Wait for flush to happen.
	time.Sleep(200 * time.Millisecond)
	cancel()

	// Verify event was inserted into the mock repository.
	repo.mu.Lock()
	events := repo.events
	repo.mu.Unlock()

	require.Len(t, events, 1)
	assert.Equal(t, "tenant-1", events[0].TenantID)
	assert.Equal(t, "llm_usage", events[0].EventType)
}

func TestCollector_ContextCancellation(t *testing.T) {
	mr := miniredis.NewMiniRedis()
	require.NoError(t, mr.Start())
	defer mr.Close()

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	repo := &mockRepository{}
	logger := testLogger()

	collector := &UsageCollector{
		client:         client,
		repo:           repo,
		logger:         logger,
		flushInterval:  50 * time.Millisecond,
		rollupInterval: time.Hour,
	}

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		collector.Start(ctx)
		close(done)
	}()

	// Cancel immediately.
	time.Sleep(10 * time.Millisecond)
	cancel()

	select {
	case <-done:
		// Success — goroutine exited.
	case <-time.After(2 * time.Second):
		t.Fatal("collector did not stop after context cancellation")
	}
}

func TestCollector_Rollup(t *testing.T) {
	mr := miniredis.NewMiniRedis()
	require.NoError(t, mr.Start())
	defer mr.Close()

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	repo := &mockRepository{}
	logger := testLogger()

	collector := &UsageCollector{
		client:         client,
		repo:           repo,
		logger:         logger,
		flushInterval:  time.Hour, // effectively disabled
		rollupInterval: 50 * time.Millisecond,
	}

	// Set up a usage hash key.
	key := "usage:tenant-1:2026-07-29:qwen-max"
	require.NoError(t, client.HSet(context.Background(), key,
		"input_tokens", "1000",
		"output_tokens", "500",
		"tool_calls", "3",
		"embedding_tokens", "0",
		"estimated_cost_usd", "0.005",
		"reports_generated", "1",
	).Err())

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	go collector.Start(ctx)

	time.Sleep(200 * time.Millisecond)
	cancel()

	// Verify at least one upsert was called (rollup may fire multiple times).
	repo.mu.Lock()
	rows := repo.dailyRows
	repo.mu.Unlock()

	require.GreaterOrEqual(t, len(rows), 1)
	assert.Equal(t, "tenant-1", rows[0].TenantID)
	assert.Equal(t, "qwen-max", rows[0].Model)
	assert.Equal(t, int64(1000), rows[0].InputTokens)
	assert.Equal(t, int64(500), rows[0].OutputTokens)
}

func TestParseUsageKey(t *testing.T) {
	tests := []struct {
		key        string
		wantTenant string
		wantDate   string
		wantModel  string
		wantOk     bool
	}{
		{"usage:tenant-1:2026-07-29:qwen-max", "tenant-1", "2026-07-29", "qwen-max", true},
		{"usage:abc:2026-01-01:text-embedding-v4", "abc", "2026-01-01", "text-embedding-v4", true},
		{"usage:tenant:date", "", "", "", false},
		{"other:key", "", "", "", false},
		{"", "", "", "", false},
		{"usage:", "", "", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			tenant, date, model, ok := parseUsageKey(tt.key)
			assert.Equal(t, tt.wantOk, ok)
			if ok {
				assert.Equal(t, tt.wantTenant, tenant)
				assert.Equal(t, tt.wantDate, date)
				assert.Equal(t, tt.wantModel, model)
			}
		})
	}
}

func TestParseFieldInt64(t *testing.T) {
	fields := map[string]string{
		"input_tokens":  "1000",
		"output_tokens": "500",
		"missing":       "",
	}

	assert.Equal(t, int64(1000), parseFieldInt64(fields, "input_tokens"))
	assert.Equal(t, int64(500), parseFieldInt64(fields, "output_tokens"))
	assert.Equal(t, int64(0), parseFieldInt64(fields, "missing"))
	assert.Equal(t, int64(0), parseFieldInt64(fields, "nonexistent"))
}

func TestParseFieldFloat64(t *testing.T) {
	fields := map[string]string{
		"estimated_cost_usd": "0.005",
		"zero":               "0",
	}

	assert.InDelta(t, 0.005, parseFieldFloat64(fields, "estimated_cost_usd"), 0.00001)
	assert.Equal(t, float64(0), parseFieldFloat64(fields, "zero"))
	assert.Equal(t, float64(0), parseFieldFloat64(fields, "nonexistent"))
}
