// Package llm implements the LLM Client module.
//
// Interface: Complete(messages, options) → CompletionResult.
// Hides retry, fallback, token counting, rate limiting, and budget
// enforcement behind a single method. Callers never talk to a provider directly.
package llm

import (
	"context"
	"fmt"
	"log"
	"math"
	"time"

	"github.com/agentic-demo/platform/internal/budget"
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

// client implements LLMClient with retry, fallback, and budget enforcement.
type client struct {
	primary       Provider
	fallback      Provider // nil = no fallback
	maxRetries    int
	budgetChecker budget.BudgetChecker // nil = no budget enforcement
}

// NewClient creates an LLM client with a primary provider and optional fallback.
func NewClient(primary, fallback Provider) LLMClient {
	return &client{
		primary:    primary,
		fallback:   fallback,
		maxRetries: 3,
	}
}

// NewClientWithBudget creates an LLM client with budget enforcement.
func NewClientWithBudget(primary, fallback Provider, budgetChecker budget.BudgetChecker) LLMClient {
	return &client{
		primary:       primary,
		fallback:      fallback,
		maxRetries:    3,
		budgetChecker: budgetChecker,
	}
}

func (c *client) Complete(ctx context.Context, msgs []domain.Message, opts Options) (domain.CompletionResult, error) {
	// Budget check before making the API call.
	if c.budgetChecker != nil && opts.TenantID != "" {
		// Estimate tokens: we don't know exact counts yet, so use a rough estimate
		// based on message count. This is a pre-check; actual usage is tracked
		// by the UsageEmitter after the call.
		estimatedInput := estimateInputTokens(msgs)
		estimatedOutput := opts.MaxTokens
		if estimatedOutput <= 0 {
			estimatedOutput = 1024 // default estimate
		}

		result, err := c.budgetChecker.CheckBudget(ctx, opts.TenantID, opts.Model, estimatedInput, estimatedOutput)
		if err != nil {
			return domain.CompletionResult{}, fmt.Errorf("budget check: %w", err)
		}
		if !result.Allowed {
			return domain.CompletionResult{}, budget.ErrBudgetExceeded
		}
	}

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

// estimateInputTokens provides a rough estimate of input tokens based on
// total character count of all messages. This is a pre-check for budget
// enforcement; actual token counts are tracked by the UsageEmitter.
func estimateInputTokens(msgs []domain.Message) int {
	totalChars := 0
	for _, m := range msgs {
		totalChars += len(m.Content)
	}
	// Rough estimate: ~4 characters per token.
	if totalChars == 0 {
		return 0
	}
	return totalChars / 4
}
