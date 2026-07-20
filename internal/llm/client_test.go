package llm

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/agentic-demo/platform/internal/domain"
)

// mockProvider implements Provider with configurable response sequences.
type mockProvider struct {
	name    string
	results []domain.CompletionResult // queue of results to return
	errs    []error                   // queue of errors to return
	callIdx int
}

func (m *mockProvider) Name() string { return m.name }

func (m *mockProvider) Chat(ctx context.Context, msgs []domain.Message, opts Options) (domain.CompletionResult, error) {
	if m.callIdx < len(m.errs) && m.errs[m.callIdx] != nil {
		err := m.errs[m.callIdx]
		m.callIdx++
		return domain.CompletionResult{}, err
	}
	if len(m.results) > 0 {
		// Return the last available result for any call past the error queue.
		idx := m.callIdx
		if idx >= len(m.results) {
			idx = len(m.results) - 1
		}
		m.callIdx++
		return m.results[idx], nil
	}
	return domain.CompletionResult{}, errors.New("mockProvider: no more results")
}

func TestLLMClient_SuccessOnFirstTry(t *testing.T) {
	expected := domain.CompletionResult{
		Text:         "hello world",
		Model:        "test-model",
		InputTokens:  10,
		OutputTokens: 5,
	}
	primary := &mockProvider{
		name:    "primary",
		results: []domain.CompletionResult{expected},
	}

	c := &client{
		primary:    primary,
		fallback:   nil,
		maxRetries: 0,
	}

	result, err := c.Complete(context.Background(), nil, Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Text != expected.Text {
		t.Errorf("Text = %q, want %q", result.Text, expected.Text)
	}
	if result.Model != expected.Model {
		t.Errorf("Model = %q, want %q", result.Model, expected.Model)
	}
}

func TestLLMClient_RetryThenSuccess(t *testing.T) {
	expected := domain.CompletionResult{
		Text:  "success after retries",
		Model: "test-model",
	}
	transientErr := errors.New("transient failure")

	primary := &mockProvider{
		name: "primary",
		// 2 errors, then 1 success.
		errs:    []error{transientErr, transientErr, nil},
		results: []domain.CompletionResult{expected},
	}

	// maxRetries=2 gives 3 total attempts, backoff: 1s + 2s = 3s max.
	c := &client{
		primary:    primary,
		fallback:   nil,
		maxRetries: 2,
	}

	start := time.Now()
	result, err := c.Complete(context.Background(), nil, Options{})
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Text != expected.Text {
		t.Errorf("Text = %q, want %q", result.Text, expected.Text)
	}
	if primary.callIdx != 3 {
		t.Errorf("expected 3 calls, got %d", primary.callIdx)
	}
	// Should have waited at least 2s (1s + 2s backoff).
	if elapsed < 2*time.Second {
		t.Errorf("elapsed = %v, expected at least 2s for backoff", elapsed)
	}
}

func TestLLMClient_PrimaryFailsFallbackSucceeds(t *testing.T) {
	expected := domain.CompletionResult{
		Text:  "fallback result",
		Model: "fallback-model",
	}
	primaryErr := errors.New("primary down")

	primary := &mockProvider{
		name: "primary",
		errs: []error{primaryErr},
	}
	fallback := &mockProvider{
		name:    "fallback",
		results: []domain.CompletionResult{expected},
	}

	c := &client{
		primary:    primary,
		fallback:   fallback,
		maxRetries: 0, // no retries for faster test
	}

	result, err := c.Complete(context.Background(), nil, Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Text != expected.Text {
		t.Errorf("Text = %q, want %q", result.Text, expected.Text)
	}
	if fallback.callIdx != 1 {
		t.Errorf("fallback was not called (callIdx=%d)", fallback.callIdx)
	}
}

func TestLLMClient_AllProvidersFail(t *testing.T) {
	primaryErr := errors.New("primary error")
	fallbackErr := errors.New("fallback error")

	primary := &mockProvider{
		name: "primary",
		errs: []error{primaryErr},
	}
	fallback := &mockProvider{
		name: "fallback",
		errs: []error{fallbackErr},
	}

	c := &client{
		primary:    primary,
		fallback:   fallback,
		maxRetries: 0,
	}

	_, err := c.Complete(context.Background(), nil, Options{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	errStr := err.Error()
	if !strings.Contains(errStr, "all providers failed") {
		t.Errorf("error should mention 'all providers failed': %q", errStr)
	}
	if !strings.Contains(errStr, "primary error") {
		t.Errorf("error should contain primary error: %q", errStr)
	}
	if !strings.Contains(errStr, "fallback error") {
		t.Errorf("error should contain fallback error: %q", errStr)
	}
}

func TestLLMClient_ContextCancelled(t *testing.T) {
	primary := &mockProvider{
		name: "primary",
		errs: []error{errors.New("transient")},
	}

	c := &client{
		primary:    primary,
		fallback:   nil,
		maxRetries: 2,
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := c.Complete(ctx, nil, Options{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
	// Should not have retried after cancellation.
	if primary.callIdx > 1 {
		t.Errorf("expected at most 1 call after cancellation, got %d", primary.callIdx)
	}
}