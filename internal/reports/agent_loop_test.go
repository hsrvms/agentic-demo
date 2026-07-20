package reports

import (
	"context"
	"testing"

	"github.com/agentic-demo/platform/internal/domain"
	"github.com/agentic-demo/platform/internal/llm"
	"github.com/agentic-demo/platform/internal/usage"
)

// scriptedLLMClient returns CompletionResults from a queue.
type scriptedLLMClient struct {
	results []domain.CompletionResult
	idx     int
}

func (s *scriptedLLMClient) Complete(ctx context.Context, msgs []domain.Message, opts llm.Options) (domain.CompletionResult, error) {
	if s.idx >= len(s.results) {
		// Return empty result to avoid infinite loop.
		return domain.CompletionResult{Text: "fallback"}, nil
	}
	r := s.results[s.idx]
	s.idx++
	return r, nil
}

// scriptedToolRegistry implements tools.ToolRegistry with configurable schemas and results.
type scriptedToolRegistry struct {
	schemas []domain.ToolSchema
	results map[string]domain.ToolResult // key: toolName
}

func (s *scriptedToolRegistry) ListTools(ctx context.Context, tenantID domain.TenantID) []domain.ToolSchema {
	return s.schemas
}

func (s *scriptedToolRegistry) Invoke(ctx context.Context, tenantID domain.TenantID, name string, params map[string]interface{}) domain.ToolResult {
	if r, ok := s.results[name]; ok {
		return r
	}
	return domain.ToolResult{Error: "tool not found"}
}

func newAgentLoop(llmResults []domain.CompletionResult, schemas []domain.ToolSchema, toolResults map[string]domain.ToolResult) *AgentLoop {
	return &AgentLoop{
		llmClient:    &scriptedLLMClient{results: llmResults},
		tools:        &scriptedToolRegistry{schemas: schemas, results: toolResults},
		emitter:      usage.NoOpEmitter{},
		maxToolCalls: 10,
		maxLLMCalls:  15,
	}
}

func TestAgentLoop_TextOnlyTermination(t *testing.T) {
	agent := newAgentLoop(
		[]domain.CompletionResult{
			{Text: "Final report content", Model: "test-model", InputTokens: 100, OutputTokens: 200},
		},
		nil, nil,
	)

	result, err := agent.Run(context.Background(), "tenant-a", domain.ReportConfig{Type: domain.ReportDaily}, "gathered context")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Content != "Final report content" {
		t.Errorf("Content = %q, want %q", result.Content, "Final report content")
	}
	if result.Metadata["llm_calls"] != "1" {
		t.Errorf("llm_calls = %s, want 1", result.Metadata["llm_calls"])
	}
	if result.Metadata["tool_calls"] != "0" {
		t.Errorf("tool_calls = %s, want 0", result.Metadata["tool_calls"])
	}
}

func TestAgentLoop_ToolCallsThenText(t *testing.T) {
	toolSchema := domain.ToolSchema{
		Name:        "search",
		Description: "search the web",
		Parameters:  map[string]interface{}{"type": "object"},
	}

	agent := newAgentLoop(
		[]domain.CompletionResult{
			{
				// First LLM call: returns a tool call.
				ToolCalls: []domain.ToolCall{
					{ID: "call_1", Name: "search", Params: map[string]interface{}{"query": "trends"}},
				},
			},
			{
				// Second LLM call: text-only response.
				Text: "Report with search results", Model: "test-model", InputTokens: 200, OutputTokens: 300,
			},
		},
		[]domain.ToolSchema{toolSchema},
		map[string]domain.ToolResult{
			"search": {Output: "search results here"},
		},
	)

	result, err := agent.Run(context.Background(), "tenant-a", domain.ReportConfig{Type: domain.ReportDaily}, "gathered context")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Metadata["tool_calls"] != "1" {
		t.Errorf("tool_calls = %s, want 1", result.Metadata["tool_calls"])
	}
	if result.Metadata["llm_calls"] != "2" {
		t.Errorf("llm_calls = %s, want 2", result.Metadata["llm_calls"])
	}
	if result.Content != "Report with search results" {
		t.Errorf("Content = %q, want %q", result.Content, "Report with search results")
	}
}

func TestAgentLoop_DuplicateToolCallDetected(t *testing.T) {
	toolSchema := domain.ToolSchema{
		Name:        "search",
		Description: "search the web",
		Parameters:  map[string]interface{}{"type": "object"},
	}

	// Same tool+params called twice.
	agent := newAgentLoop(
		[]domain.CompletionResult{
			{
				ToolCalls: []domain.ToolCall{
					{ID: "call_1", Name: "search", Params: map[string]interface{}{"query": "x"}},
				},
			},
			{
				ToolCalls: []domain.ToolCall{
					{ID: "call_2", Name: "search", Params: map[string]interface{}{"query": "x"}},
				},
			},
			{
				Text: "Report after duplicate detection", Model: "test-model", InputTokens: 100, OutputTokens: 200,
			},
		},
		[]domain.ToolSchema{toolSchema},
		map[string]domain.ToolResult{
			"search": {Output: "search results"},
		},
	)

	result, err := agent.Run(context.Background(), "tenant-a", domain.ReportConfig{Type: domain.ReportDaily}, "gathered context")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Only 1 unique tool invocation; the duplicate should be detected.
	if result.Metadata["tool_calls"] != "1" {
		t.Errorf("tool_calls = %s, want 1 (duplicate should not count)", result.Metadata["tool_calls"])
	}
}

func TestAgentLoop_ToolReturnsError(t *testing.T) {
	toolSchema := domain.ToolSchema{
		Name:        "search",
		Description: "search the web",
		Parameters:  map[string]interface{}{"type": "object"},
	}

	agent := newAgentLoop(
		[]domain.CompletionResult{
			{
				ToolCalls: []domain.ToolCall{
					{ID: "call_1", Name: "search", Params: map[string]interface{}{"query": "trends"}},
				},
			},
			{
				Text: "Report despite tool error", Model: "test-model", InputTokens: 100, OutputTokens: 200,
			},
		},
		[]domain.ToolSchema{toolSchema},
		map[string]domain.ToolResult{
			"search": {Error: "search API timeout"},
		},
	)

	result, err := agent.Run(context.Background(), "tenant-a", domain.ReportConfig{Type: domain.ReportDaily}, "gathered context")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// The loop should continue despite tool error.
	if result.Metadata["tool_calls"] != "1" {
		t.Errorf("tool_calls = %s, want 1", result.Metadata["tool_calls"])
	}
	if result.Content != "Report despite tool error" {
		t.Errorf("Content = %q, want %q", result.Content, "Report despite tool error")
	}
}

func TestAgentLoop_MaxToolCallsExceeded(t *testing.T) {
	toolSchema := domain.ToolSchema{
		Name:        "search",
		Description: "search",
		Parameters:  map[string]interface{}{"type": "object"},
	}

	// LLM keeps requesting tool calls, exceeding maxToolCalls.
	results := []domain.CompletionResult{}
	for i := 0; i < 3; i++ {
		results = append(results, domain.CompletionResult{
			ToolCalls: []domain.ToolCall{
				{ID: "call_" + string(rune('a'+i)), Name: "search", Params: map[string]interface{}{"q": i}},
			},
		})
	}

	agent := &AgentLoop{
		llmClient:    &scriptedLLMClient{results: results},
		tools:        &scriptedToolRegistry{schemas: []domain.ToolSchema{toolSchema}, results: map[string]domain.ToolResult{"search": {Output: "result"}}},
		emitter:      usage.NoOpEmitter{},
		maxToolCalls: 2,
		maxLLMCalls:  15,
	}

	result, err := agent.Run(context.Background(), "tenant-a", domain.ReportConfig{Type: domain.ReportDaily}, "gathered context")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Metadata["synthesized"] != "true" {
		t.Error("expected synthesized=true when max tool calls exceeded")
	}
	// tool_calls should be at most maxToolCalls (2).
	if result.Metadata["tool_calls"] != "2" {
		t.Errorf("tool_calls = %s, want 2", result.Metadata["tool_calls"])
	}
}

func TestAgentLoop_MaxLLMCallsExceeded(t *testing.T) {
	toolSchema := domain.ToolSchema{
		Name:        "search",
		Description: "search",
		Parameters:  map[string]interface{}{"type": "object"},
	}

	// LLM returns tool calls every time, exceeding maxLLMCalls.
	results := []domain.CompletionResult{}
	for i := 0; i < 5; i++ {
		results = append(results, domain.CompletionResult{
			ToolCalls: []domain.ToolCall{
				{ID: "call_" + string(rune('a'+i)), Name: "search", Params: map[string]interface{}{"q": i}},
			},
		})
	}

	agent := &AgentLoop{
		llmClient:    &scriptedLLMClient{results: results},
		tools:        &scriptedToolRegistry{schemas: []domain.ToolSchema{toolSchema}, results: map[string]domain.ToolResult{"search": {Output: "result"}}},
		emitter:      usage.NoOpEmitter{},
		maxToolCalls: 10,
		maxLLMCalls:  2,
	}

	result, err := agent.Run(context.Background(), "tenant-a", domain.ReportConfig{Type: domain.ReportDaily}, "gathered context")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Metadata["synthesized"] != "true" {
		t.Error("expected synthesized=true when max LLM calls exceeded")
	}
	// llm_calls = maxLLMCalls + 1 (synthesis).
	if result.Metadata["llm_calls"] != "3" {
		t.Errorf("llm_calls = %s, want 3", result.Metadata["llm_calls"])
	}
}

func TestAgentLoop_SynthesisRemovesTools(t *testing.T) {
	toolSchema := domain.ToolSchema{
		Name:        "search",
		Description: "search",
		Parameters:  map[string]interface{}{"type": "object"},
	}

	// Track the synthesis call to verify ToolSchemas is nil.
	var synthesisOpts llm.Options
	synthesisLLM := &scriptedLLMClient{
		results: []domain.CompletionResult{
			{
				// LLM returns tool calls to trigger forced synthesis.
				ToolCalls: []domain.ToolCall{
					{ID: "call_1", Name: "search", Params: map[string]interface{}{"q": 1}},
				},
			},
			{
				ToolCalls: []domain.ToolCall{
					{ID: "call_2", Name: "search", Params: map[string]interface{}{"q": 2}},
				},
			},
			{
				ToolCalls: []domain.ToolCall{
					{ID: "call_3", Name: "search", Params: map[string]interface{}{"q": 3}},
				},
			},
		},
	}

	// Wrap the scriptedLLMClient to capture the synthesis call.
	captureLLM := &captureLLMClient{
		inner: synthesisLLM,
		onCall: func(opts llm.Options) {
			synthesisOpts = opts
		},
	}

	agent := &AgentLoop{
		llmClient:    captureLLM,
		tools:        &scriptedToolRegistry{schemas: []domain.ToolSchema{toolSchema}, results: map[string]domain.ToolResult{"search": {Output: "result"}}},
		emitter:      usage.NoOpEmitter{},
		maxToolCalls: 2, // Force synthesis after 2 tool calls.
		maxLLMCalls:  15,
	}

	_, err := agent.Run(context.Background(), "tenant-a", domain.ReportConfig{Type: domain.ReportDaily}, "gathered context")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// The synthesis call should have ToolSchemas=nil.
	if synthesisOpts.ToolSchemas != nil {
		t.Errorf("synthesis call ToolSchemas = %v, want nil", synthesisOpts.ToolSchemas)
	}
}

// captureLLMClient wraps an LLMClient to capture the Options on each call.
type captureLLMClient struct {
	inner  llm.LLMClient
	onCall func(llm.Options)
}

func (c *captureLLMClient) Complete(ctx context.Context, msgs []domain.Message, opts llm.Options) (domain.CompletionResult, error) {
	c.onCall(opts)
	return c.inner.Complete(ctx, msgs, opts)
}