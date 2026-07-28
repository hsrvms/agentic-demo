// Package config loads and validates environment configuration at startup.
// Configuration fails fast when invalid — no silent defaults.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	DatabaseURL string
	RedisURL    string

	// LLM (chat) — uses OpenAI-compatible endpoint (e.g. MaaS workspace).
	DashScopeAPIKey  string
	DashScopeBaseURL string

	// Embeddings — uses native DashScope API (public endpoint).
	// Falls back to DashScopeAPIKey / DashScopeBaseURL if not set.
	DashScopeEmbeddingAPIKey  string
	DashScopeEmbeddingBaseURL string

	LLMModel       string
	EmbeddingModel string

	MaxToolCalls        int
	MaxLLMCalls         int
	MaxExecutionMinutes int

	JWTSecret string
}

func Load() (*Config, error) {
	cfg := &Config{
		DatabaseURL:              getEnv("DATABASE_URL", "postgres://platform:platform@localhost:5432/platform?sslmode=disable"),
		RedisURL:                 getEnv("REDIS_URL", "redis://localhost:6379/0"),
		DashScopeAPIKey:          getEnv("DASHSCOPE_API_KEY", ""),
		DashScopeBaseURL:         getEnv("DASHSCOPE_BASE_URL", "https://dashscope-intl.aliyuncs.com/compatible-mode/v1"),
		DashScopeEmbeddingAPIKey: getEnv("DASHSCOPE_EMBEDDING_API_KEY", getEnv("DASHSCOPE_API_KEY", "")),
		DashScopeEmbeddingBaseURL: getEnv("DASHSCOPE_EMBEDDING_BASE_URL", "https://dashscope-intl.aliyuncs.com/api/v1"),
		LLMModel:                 getEnv("LLM_MODEL", "qwen-max"),
		EmbeddingModel:           getEnv("EMBEDDING_MODEL", "text-embedding-v4"),
		MaxToolCalls:             getEnvInt("MAX_TOOL_CALLS", 10),
		MaxLLMCalls:              getEnvInt("MAX_LLM_CALLS", 15),
		MaxExecutionMinutes:      getEnvInt("MAX_EXECUTION_MINUTES", 10),
		JWTSecret:                getEnv("JWT_SECRET", "dev-secret-change-in-production"),
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (c *Config) MaxExecutionDuration() time.Duration {
	return time.Duration(c.MaxExecutionMinutes) * time.Minute
}

func (c *Config) validate() error {
	var missing []string
	if c.DatabaseURL == "" {
		missing = append(missing, "DATABASE_URL")
	}
	if c.DashScopeAPIKey == "" {
		missing = append(missing, "DASHSCOPE_API_KEY")
	}
	if c.DashScopeEmbeddingAPIKey == "" {
		missing = append(missing, "DASHSCOPE_EMBEDDING_API_KEY (or DASHSCOPE_API_KEY)")
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required environment variables: %s", strings.Join(missing, ", "))
	}
	return nil
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func getEnvInt(key string, defaultVal int) int {
	val := os.Getenv(key)
	if val == "" {
		return defaultVal
	}
	n, err := strconv.Atoi(val)
	if err != nil {
		return defaultVal
	}
	return n
}
