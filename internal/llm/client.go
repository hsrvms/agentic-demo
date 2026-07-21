// Package llm implements the LLM Client module.
//
// Interface: Complete(messages, options) → CompletionResult.
// Hides retry, fallback, token counting, and rate limiting behind
// a single method. Callers never talk to a provider directly.
package llm

import (
	"context"
	"fmt"
	"log"
	"math"
	"time"

	"github.com/agentic-demo/platform/internal/domain"
)

// LLMClient is the module interface.
type LLMClient interface {
	Complete(ctx context.Context, msgs []domain.Message, opts Options) (domain.CompletionResult, error)
}

// Options configures a single completion request.
type Options struct {
	TenantID    domain.TenantID
	Model       string
	MaxTokens   int
	Temperature float64
	ToolSchemas []domain.ToolSchema // nil = no tools available
}

// Provider is an internal seam — a single LLM provider adapter.
// DashScope, OpenAI, Ollama, etc. each implement this.
type Provider interface {
	Chat(ctx context.Context, msgs []domain.Message, opts Options) (domain.CompletionResult, error)
	Name() string
}

// client implements LLMClient with retry and fallback.
type client struct {
	primary    Provider
	fallback   Provider // nil = no fallback
	maxRetries int
}

// NewClient creates an LLM client with a primary provider and optional fallback.
func NewClient(primary Provider, fallback Provider) LLMClient {
	return &client{
		primary:    primary,
		fallback:   fallback,
		maxRetries: 3,
	}
}

func (c *client) Complete(ctx context.Context, msgs []domain.Message, opts Options) (domain.CompletionResult, error) {
	// Try primary with retries.
	result, err := c.callWithRetry(ctx, msgs, opts, c.primary)
	if err == nil {
		return result, nil
	}

	primaryErr := err
	log.Printf("primary provider %s failed: %v", c.primary.Name(), err)

	// Try fallback if available.
	if c.fallback != nil {
		result, err := c.callWithRetry(ctx, msgs, opts, c.fallback)
		if err == nil {
			return result, nil
		}
		return domain.CompletionResult{}, fmt.Errorf(
			"all providers failed — primary (%s): %v, fallback (%s): %v",
			c.primary.Name(), primaryErr, c.fallback.Name(), err,
		)
	}

	return domain.CompletionResult{}, fmt.Errorf("provider %s failed: %w", c.primary.Name(), primaryErr)
}

func (c *client) callWithRetry(ctx context.Context, msgs []domain.Message, opts Options, provider Provider) (domain.CompletionResult, error) {
	var lastErr error
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		result, err := provider.Chat(ctx, msgs, opts)
		if err == nil {
			return result, nil
		}

		lastErr = err

		// Don't retry on context cancellation or deadline exceeded.
		if ctx.Err() != nil {
			return domain.CompletionResult{}, ctx.Err()
		}

		// Exponential backoff: 1s, 2s, 4s.
		if attempt < c.maxRetries {
			backoff := time.Duration(math.Pow(2, float64(attempt))) * time.Second
			log.Printf("retry %d/%d for %s (backoff %v): %v", attempt+1, c.maxRetries, provider.Name(), backoff, err)

			select {
			case <-ctx.Done():
				return domain.CompletionResult{}, ctx.Err()
			case <-time.After(backoff):
			}
		}
	}
	return domain.CompletionResult{}, lastErr
}
