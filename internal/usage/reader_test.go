package usage

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRedisReader_GetCurrentUsage(t *testing.T) {
	mr := miniredis.NewMiniRedis()
	require.NoError(t, mr.Start())
	defer mr.Close()

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	// Set up usage hash keys for the current month.
	now := time.Now().UTC()
	today := now.Format("2006-01-02")

	key1 := "usage:tenant-1:" + today + ":qwen-max"
	require.NoError(t, client.HSet(context.Background(), key1,
		"input_tokens", "1000",
		"output_tokens", "500",
		"tool_calls", "3",
		"embedding_tokens", "0",
		"estimated_cost_usd", "0.005",
		"reports_generated", "1",
	).Err())

	key2 := "usage:tenant-1:" + today + ":text-embedding-v4"
	require.NoError(t, client.HSet(context.Background(), key2,
		"input_tokens", "0",
		"output_tokens", "0",
		"tool_calls", "0",
		"embedding_tokens", "100",
		"estimated_cost_usd", "0.00001",
		"reports_generated", "0",
	).Err())

	reader := &redisReader{client: client}

	result, err := reader.GetCurrentUsage(context.Background(), "tenant-1")
	require.NoError(t, err)

	assert.Equal(t, "tenant-1", result.TenantID)
	assert.Equal(t, int64(1000), result.TotalInputTokens)
	assert.Equal(t, int64(500), result.TotalOutputTokens)
	assert.Equal(t, int64(3), result.TotalToolCalls)
	assert.Equal(t, int64(100), result.TotalEmbeddingTokens)
	assert.InDelta(t, 0.00501, result.TotalCostUSD, 0.00001)
	assert.Equal(t, int64(1), result.ReportsGenerated)

	// Verify by-model breakdown.
	require.Len(t, result.ByModel, 2)

	// Find qwen-max entry.
	var qwenMax *ModelUsage
	for i := range result.ByModel {
		if result.ByModel[i].Model == "qwen-max" {
			qwenMax = &result.ByModel[i]
			break
		}
	}
	require.NotNil(t, qwenMax)
	assert.Equal(t, int64(1000), qwenMax.InputTokens)
	assert.Equal(t, int64(500), qwenMax.OutputTokens)
	assert.Equal(t, int64(3), qwenMax.ToolCalls)
}

func TestRedisReader_NoData(t *testing.T) {
	mr := miniredis.NewMiniRedis()
	require.NoError(t, mr.Start())
	defer mr.Close()

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	reader := &redisReader{client: client}

	result, err := reader.GetCurrentUsage(context.Background(), "tenant-1")
	require.NoError(t, err)

	assert.Equal(t, "tenant-1", result.TenantID)
	assert.Equal(t, int64(0), result.TotalInputTokens)
	assert.Equal(t, int64(0), result.TotalOutputTokens)
	assert.Empty(t, result.ByModel)
}

func TestRedisReader_CrossTenantIsolation(t *testing.T) {
	mr := miniredis.NewMiniRedis()
	require.NoError(t, mr.Start())
	defer mr.Close()

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	now := time.Now().UTC()
	today := now.Format("2006-01-02")

	// Set up keys for two tenants.
	require.NoError(t, client.HSet(context.Background(),
		"usage:tenant-1:"+today+":qwen-max",
		"input_tokens", "100",
	).Err())
	require.NoError(t, client.HSet(context.Background(),
		"usage:tenant-2:"+today+":qwen-max",
		"input_tokens", "999",
	).Err())

	reader := &redisReader{client: client}

	result, err := reader.GetCurrentUsage(context.Background(), "tenant-1")
	require.NoError(t, err)

	// Should only see tenant-1's data.
	assert.Equal(t, int64(100), result.TotalInputTokens)
	assert.Len(t, result.ByModel, 1)
	assert.Equal(t, "qwen-max", result.ByModel[0].Model)
}

func TestRedisReader_OldDataExcluded(t *testing.T) {
	mr := miniredis.NewMiniRedis()
	require.NoError(t, mr.Start())
	defer mr.Close()

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	// Set up a key from last month.
	now := time.Now().UTC()
	lastMonth := now.AddDate(0, -1, 0).Format("2006-01-02")

	require.NoError(t, client.HSet(context.Background(),
		"usage:tenant-1:"+lastMonth+":qwen-max",
		"input_tokens", "9999",
	).Err())

	reader := &redisReader{client: client}

	result, err := reader.GetCurrentUsage(context.Background(), "tenant-1")
	require.NoError(t, err)

	// Old data should be excluded.
	assert.Equal(t, int64(0), result.TotalInputTokens)
	assert.Empty(t, result.ByModel)
}