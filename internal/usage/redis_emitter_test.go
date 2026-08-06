package usage

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupRedis(t *testing.T) (*RedisEmitter, *redis.Client) {
	t.Helper()
	mr := miniredis.NewMiniRedis()
	require.NoError(t, mr.Start())

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	emitter := &RedisEmitter{
		client: client,
		nowFunc: func() time.Time {
			return time.Date(2026, 7, 29, 10, 0, 0, 0, time.UTC)
		},
	}

	t.Cleanup(func() {
		emitter.Close()
		mr.Close()
	})

	return emitter, client
}

func TestRedisEmitter_EmitLLMUsage(t *testing.T) {
	emitter, client := setupRedis(t)
	ctx := context.Background()

	err := emitter.EmitUsage(ctx, UsageEvent{
		Type:     EventLLM,
		TenantID: "tenant-1",
		LLM: &LLMUsage{
			Model:        "qwen-max",
			InputTokens:  1000,
			OutputTokens: 500,
		},
	})
	require.NoError(t, err)

	key := "usage:tenant-1:" + today() + ":qwen-max"

	inputTokens, err := client.HGet(ctx, key, "input_tokens").Result()
	require.NoError(t, err)
	assert.Equal(t, "1000", inputTokens)

	outputTokens, err := client.HGet(ctx, key, "output_tokens").Result()
	require.NoError(t, err)
	assert.Equal(t, "500", outputTokens)

	reports, err := client.HGet(ctx, key, "reports_generated").Result()
	require.NoError(t, err)
	assert.Equal(t, "1", reports)

	ttl, err := client.TTL(ctx, key).Result()
	require.NoError(t, err)
	assert.InDelta(t, 172800, ttl.Seconds(), 5)

	// Verify event is in flush queue.
	queueLen, err := client.LLen(ctx, flushQueue).Result()
	require.NoError(t, err)
	assert.Equal(t, int64(1), queueLen)

	raw, err := client.LPop(ctx, flushQueue).Result()
	require.NoError(t, err)

	var flushed UsageEvent
	require.NoError(t, json.Unmarshal([]byte(raw), &flushed))
	assert.Equal(t, EventLLM, flushed.Type)
	assert.Equal(t, "tenant-1", string(flushed.TenantID))
	assert.Equal(t, "qwen-max", flushed.LLM.Model)
}

func TestRedisEmitter_EmitToolUsage(t *testing.T) {
	emitter, client := setupRedis(t)
	ctx := context.Background()

	err := emitter.EmitUsage(ctx, UsageEvent{
		Type:     EventTool,
		TenantID: "tenant-1",
		Tool: &ToolUsage{
			ToolName: "web_search",
			Model:    "qwen-max",
			Success:  true,
		},
	})
	require.NoError(t, err)

	key := "usage:tenant-1:" + today() + ":qwen-max"
	toolCalls, err := client.HGet(ctx, key, "tool_calls").Result()
	require.NoError(t, err)
	assert.Equal(t, "1", toolCalls)
}

func TestRedisEmitter_EmitToolUsage_NoModel(t *testing.T) {
	emitter, client := setupRedis(t)
	ctx := context.Background()

	err := emitter.EmitUsage(ctx, UsageEvent{
		Type:     EventTool,
		TenantID: "tenant-1",
		Tool: &ToolUsage{
			ToolName: "web_search",
			Success:  true,
		},
	})
	require.NoError(t, err)

	key := "usage:tenant-1:" + today() + ":_tool_"
	toolCalls, err := client.HGet(ctx, key, "tool_calls").Result()
	require.NoError(t, err)
	assert.Equal(t, "1", toolCalls)
}

func TestRedisEmitter_EmitEmbeddingUsage(t *testing.T) {
	emitter, client := setupRedis(t)
	ctx := context.Background()

	err := emitter.EmitUsage(ctx, UsageEvent{
		Type:     EventEmbedding,
		TenantID: "tenant-1",
		Embedding: &EmbeddingUsage{
			Model:           "text-embedding-v4",
			ChunksProcessed: 50,
		},
	})
	require.NoError(t, err)

	key := "usage:tenant-1:" + today() + ":text-embedding-v4"
	embeddingTokens, err := client.HGet(ctx, key, "embedding_tokens").Result()
	require.NoError(t, err)
	assert.Equal(t, "50", embeddingTokens)
}

func TestRedisEmitter_EmitEmbeddingUsage_NoModel(t *testing.T) {
	emitter, client := setupRedis(t)
	ctx := context.Background()

	err := emitter.EmitUsage(ctx, UsageEvent{
		Type:     EventEmbedding,
		TenantID: "tenant-1",
		Embedding: &EmbeddingUsage{
			ChunksProcessed: 50,
		},
	})
	require.NoError(t, err)

	key := "usage:tenant-1:" + today() + ":_embedding_"
	embeddingTokens, err := client.HGet(ctx, key, "embedding_tokens").Result()
	require.NoError(t, err)
	assert.Equal(t, "50", embeddingTokens)
}

func TestRedisEmitter_FlushQueueTrim(t *testing.T) {
	emitter, client := setupRedis(t)
	ctx := context.Background()

	// Push many events to trigger trim.
	for i := 0; i < 15; i++ {
		require.NoError(t, emitter.EmitUsage(ctx, UsageEvent{
			Type:     EventLLM,
			TenantID: "tenant-1",
			LLM:      &LLMUsage{Model: "qwen-max", InputTokens: 1, OutputTokens: 1},
		}))
	}

	queueLen, err := client.LLen(ctx, flushQueue).Result()
	require.NoError(t, err)
	assert.LessOrEqual(t, queueLen, int64(maxQueueLen))
}

func TestRedisEmitter_LLMUsageCountersAggregate(t *testing.T) {
	emitter, client := setupRedis(t)
	ctx := context.Background()

	// Emit twice for the same tenant+date+model.
	for i := 0; i < 2; i++ {
		require.NoError(t, emitter.EmitUsage(ctx, UsageEvent{
			Type:     EventLLM,
			TenantID: "tenant-1",
			LLM:      &LLMUsage{Model: "qwen-max", InputTokens: 100, OutputTokens: 50},
		}))
	}

	key := "usage:tenant-1:" + today() + ":qwen-max"
	inputTokens, _ := client.HGet(ctx, key, "input_tokens").Result()
	assert.Equal(t, "200", inputTokens)
	outputTokens, _ := client.HGet(ctx, key, "output_tokens").Result()
	assert.Equal(t, "100", outputTokens)
	reports, _ := client.HGet(ctx, key, "reports_generated").Result()
	assert.Equal(t, "2", reports)
}

func TestRedisEmitter_UnknownModel(t *testing.T) {
	emitter, client := setupRedis(t)
	ctx := context.Background()

	err := emitter.EmitUsage(ctx, UsageEvent{
		Type:     EventLLM,
		TenantID: "tenant-1",
		LLM:      &LLMUsage{Model: "", InputTokens: 100, OutputTokens: 50},
	})
	require.NoError(t, err)

	key := "usage:tenant-1:" + today() + ":_unknown_"
	inputTokens, _ := client.HGet(ctx, key, "input_tokens").Result()
	assert.Equal(t, "100", inputTokens)
}

func TestRedisKey(t *testing.T) {
	key := redisKey("tenant-1", "2026-07-29", "qwen-max")
	assert.Equal(t, "usage:tenant-1:2026-07-29:qwen-max", key)
}

func TestEventModel(t *testing.T) {
	tests := []struct {
		name  string
		event UsageEvent
		want  string
	}{
		{
			name:  "LLM with model",
			event: UsageEvent{Type: EventLLM, LLM: &LLMUsage{Model: "qwen-max"}},
			want:  "qwen-max",
		},
		{
			name:  "LLM without model",
			event: UsageEvent{Type: EventLLM, LLM: &LLMUsage{}},
			want:  "_unknown_",
		},
		{
			name:  "Tool with model",
			event: UsageEvent{Type: EventTool, Tool: &ToolUsage{Model: "qwen-max"}},
			want:  "qwen-max",
		},
		{
			name:  "Tool without model",
			event: UsageEvent{Type: EventTool, Tool: &ToolUsage{}},
			want:  "_tool_",
		},
		{
			name:  "Embedding with model",
			event: UsageEvent{Type: EventEmbedding, Embedding: &EmbeddingUsage{Model: "text-embedding-v4"}},
			want:  "text-embedding-v4",
		},
		{
			name:  "Embedding without model",
			event: UsageEvent{Type: EventEmbedding, Embedding: &EmbeddingUsage{}},
			want:  "_embedding_",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, eventModel(tt.event))
		})
	}
}

func TestComputeCost(t *testing.T) {
	tests := []struct {
		model        string
		inputTokens  int
		outputTokens int
		want         float64
	}{
		{"qwen-max", 1000, 500, 0.005},         // expected: 0.002 + 0.003
		{"qwen-plus", 1000, 0, 0.001},          // expected: 0.001
		{"qwen-turbo", 0, 1000, 0.0015},        // expected: 0.0015
		{"text-embedding-v4", 1000, 0, 0.0001}, // expected: 0.0001
		{"unknown-model", 1000, 500, 0},        // not in pricing table
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			got := computeCost(tt.model, tt.inputTokens, tt.outputTokens)
			assert.InDelta(t, tt.want, got, 0.00001)
		})
	}
}

func today() string {
	return "2026-07-29" // fixed date for deterministic tests
}
