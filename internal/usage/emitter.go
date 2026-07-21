// Package usage implements cross-cutting usage tracking.
//
// Interface: EmitUsage(event). Every module that incurs cost
// (LLM calls, tool invocations, embeddings) calls this after
// every action. Production uses RedisEmitter; tests use NoOpEmitter.
package usage

import (
	"context"

	"github.com/agentic-demo/platform/internal/domain"
)

// EventType identifies the kind of usage event.
type EventType string

const (
	EventLLM       EventType = "llm_usage"
	EventTool      EventType = "tool_usage"
	EventEmbedding EventType = "embedding_usage"
)

// UsageEvent carries one of the three event types.
// All events carry TenantID. Type-specific fields are in the
// corresponding sub-struct (only one is non-nil).
type UsageEvent struct {
	Type      EventType
	TenantID  domain.TenantID
	LLM       *LLMUsage
	Tool      *ToolUsage
	Embedding *EmbeddingUsage
}

type LLMUsage struct {
	Model        string
	InputTokens  int
	OutputTokens int
}

type ToolUsage struct {
	ToolName string
	Success  bool
}

type EmbeddingUsage struct {
	ChunksProcessed int
}

// UsageEmitter is the interface that modules depend on.
// Callers don't know or care whether events go to Redis or nowhere.
type UsageEmitter interface {
	EmitUsage(ctx context.Context, event UsageEvent) error
}

// NoOpEmitter discards all events. Used in Phase 0 and tests.
type NoOpEmitter struct{}

func (NoOpEmitter) EmitUsage(_ context.Context, _ UsageEvent) error { return nil }
