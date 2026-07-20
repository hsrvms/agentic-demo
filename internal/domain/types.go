// Package domain defines shared types that cross module boundaries.
//
// Every module imports from here. No module defines its own version
// of a cross-cutting type.
package domain

import "time"

// TenantID identifies a tenant. Always required — a function that
// doesn't take a TenantID cannot access tenant-scoped data.
type TenantID string

// RawDocument is unparsed content extracted by a connector.
// The chunker converts these into Chunks.
type RawDocument struct {
	Content  string
	Metadata map[string]string // source, document_type, title, etc.
}

// Chunk is a segment of a document with its vector embedding.
// This is the unit of storage in the Knowledge Store.
type Chunk struct {
	ID           string
	Content      string
	Embedding    []float32
	Source       string
	DocumentType string
	Date         time.Time
	Metadata     map[string]string
}

// RankedChunk is a chunk returned from a similarity query with its distance.
type RankedChunk struct {
	Chunk    Chunk
	Distance float64 // cosine distance: 0 = identical, 2 = opposite
}

// CompletionResult is the LLM's response to a completion request.
type CompletionResult struct {
	Text         string
	ToolCalls    []ToolCall
	InputTokens  int
	OutputTokens int
	Model        string
	Latency      time.Duration
}

// ToolCall represents an LLM-requested tool invocation.
type ToolCall struct {
	ID       string
	Name     string
	Params   map[string]interface{}
	IsFinal  bool // true when the LLM has produced its final answer alongside tool calls
}

// Message is a message in an LLM conversation.
type Message struct {
	Role       string     // "system", "user", "assistant", "tool"
	Content    string
	ToolCalls  []ToolCall // populated on assistant messages with tool calls
	ToolCallID string     // populated on tool result messages
}

// ToolSchema describes a tool for the LLM's function-calling interface.
type ToolSchema struct {
	Name        string
	Description string
	Parameters  map[string]interface{} // JSON Schema
}

// ToolResult is the outcome of a tool invocation.
type ToolResult struct {
	Output string
	Error  string
}

// ReportConfig specifies what kind of report to generate.
type ReportConfig struct {
	Type        ReportType
	FocusAreas  []string
	DeliveryMethod string // "web", "email"
}

// ReportType determines the context gathering strategy.
type ReportType string

const (
	ReportDaily    ReportType = "daily"
	ReportWeekly   ReportType = "weekly"
	ReportMonthly  ReportType = "monthly"
	ReportOnDemand ReportType = "on_demand"
)

// Report is a generated strategic report.
type Report struct {
	ID        string
	Content   string
	Type      ReportType
	Date      time.Time
	Metadata  map[string]string
}

// IngestionResult summarizes a completed ingestion run.
type IngestionResult struct {
	ChunksProcessed int
	Errors          []string
	Duration        time.Duration
}
