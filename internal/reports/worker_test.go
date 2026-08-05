package reports

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/agentic-demo/platform/internal/domain"
	"github.com/agentic-demo/platform/internal/knowledge"
	"github.com/agentic-demo/platform/internal/llm"
	"github.com/agentic-demo/platform/internal/usage"
)

// mockKS implements knowledge.KnowledgeStore for worker tests.
type mockKS struct {
	chunks    []domain.RankedChunk
	documents map[string]domain.Document
	queryErr  error
	getDocErr error
	gotTenant domain.TenantID
	gotDocIDs []string
}

func (m *mockKS) Store(_ context.Context, _ domain.TenantID, _ []domain.Document, _ []domain.Chunk) error {
	return nil
}

func (m *mockKS) ReplaceSource(_ context.Context, _ domain.TenantID, _ string, _ []domain.Document, _ []domain.Chunk) error {
	return nil
}

func (m *mockKS) Query(_ context.Context, tenantID domain.TenantID, _ string, _ int, _ knowledge.QueryFilters) ([]domain.RankedChunk, error) {
	m.gotTenant = tenantID
	return m.chunks, m.queryErr
}

func (m *mockKS) GetDocument(_ context.Context, tenantID domain.TenantID, documentID string) (domain.Document, error) {
	m.gotTenant = tenantID
	m.gotDocIDs = append(m.gotDocIDs, documentID)
	if m.getDocErr != nil {
		return domain.Document{}, m.getDocErr
	}
	doc, ok := m.documents[documentID]
	if !ok {
		return domain.Document{}, errors.New("document not found")
	}
	return doc, nil
}

func (m *mockKS) DeleteTenantData(_ context.Context, _ domain.TenantID) error {
	return nil
}

func (m *mockKS) GetStats(_ context.Context, _ domain.TenantID) (map[string]int, error) {
	return nil, nil
}

// messageCaptureLLMClient records the user message it receives and returns a fixed answer.
type messageCaptureLLMClient struct {
	userMessage string
}

func (c *messageCaptureLLMClient) Complete(_ context.Context, msgs []domain.Message, _ llm.Options) (domain.CompletionResult, error) {
	for _, m := range msgs {
		if m.Role == "user" {
			c.userMessage = m.Content
		}
	}
	return domain.CompletionResult{Text: "Final report", Model: "test", InputTokens: 10, OutputTokens: 20}, nil
}

func newWorkerForTest(ks knowledge.KnowledgeStore, llmC llm.LLMClient) *ReportWorker {
	return NewReportWorker(ks, llmC, &scriptedToolRegistry{}, usage.NoOpEmitter{}, 10, 15, 0)
}

// TestGenerateReport_ContextIncludesFullDocument verifies that the context
// passed to the LLM contains the full document behind the matched chunk, not
// merely the matched fragment.
func TestGenerateReport_ContextIncludesFullDocument(t *testing.T) {
	ks := &mockKS{
		chunks: []domain.RankedChunk{{
			Chunk: domain.Chunk{
				ID:           "chunk-1",
				Content:      "Revenue grew 15%",
				Source:       "financials",
				DocumentType: "report",
				DocumentID:   "doc-1",
			},
			Distance: 0.1,
		}},
		documents: map[string]domain.Document{
			"doc-1": {
				ID:      "doc-1",
				Source:  "financials",
				Content: "The full financial report with every detail the LLM needs to reason.",
			},
		},
	}
	llmClient := &messageCaptureLLMClient{}
	worker := newWorkerForTest(ks, llmClient)

	_, err := worker.GenerateReport(context.Background(), "tenant-a", domain.ReportConfig{Type: domain.ReportDaily})
	if err != nil {
		t.Fatalf("GenerateReport failed: %v", err)
	}

	if !strings.Contains(llmClient.userMessage, "The full financial report with every detail the LLM needs to reason.") {
		t.Errorf("LLM user message should include the full document content, got: %q", llmClient.userMessage)
	}
}

// TestGenerateReport_GetDocumentIsTenantScoped verifies the document retrieval
// is scoped to the report's tenant.
func TestGenerateReport_GetDocumentIsTenantScoped(t *testing.T) {
	ks := &mockKS{
		chunks: []domain.RankedChunk{{
			Chunk: domain.Chunk{
				ID:         "chunk-1",
				Content:    "fragment",
				Source:     "src",
				DocumentID: "doc-1",
			},
		}},
		documents: map[string]domain.Document{
			"doc-1": {ID: "doc-1", Source: "src", Content: "full doc"},
		},
	}
	llmClient := &messageCaptureLLMClient{}
	worker := newWorkerForTest(ks, llmClient)

	_, err := worker.GenerateReport(context.Background(), "tenant-a", domain.ReportConfig{Type: domain.ReportDaily})
	if err != nil {
		t.Fatalf("GenerateReport failed: %v", err)
	}

	if ks.gotTenant != "tenant-a" {
		t.Errorf("GetDocument called with tenant %q, want %q", ks.gotTenant, "tenant-a")
	}
	if len(ks.gotDocIDs) != 1 || ks.gotDocIDs[0] != "doc-1" {
		t.Errorf("GetDocument called with doc IDs %v, want [doc-1]", ks.gotDocIDs)
	}
}

// TestGenerateReport_GetDocumentErrorFallsBack verifies that a failure to
// expand a matched chunk to its full document degrades gracefully: report
// generation still succeeds and the LLM receives the chunk fragment as context
// rather than the report failing on a single stale document_id.
func TestGenerateReport_GetDocumentErrorFallsBack(t *testing.T) {
	ks := &mockKS{
		chunks: []domain.RankedChunk{{
			Chunk: domain.Chunk{
				ID:         "chunk-1",
				Content:    "fragment",
				Source:     "src",
				DocumentID: "doc-1",
			},
		}},
		getDocErr: errors.New("document not found"),
	}
	llmClient := &messageCaptureLLMClient{}
	worker := newWorkerForTest(ks, llmClient)

	_, err := worker.GenerateReport(context.Background(), "tenant-a", domain.ReportConfig{Type: domain.ReportDaily})
	if err != nil {
		t.Fatalf("GenerateReport should not fail when a document is unavailable, got: %v", err)
	}
	if !strings.Contains(llmClient.userMessage, "fragment") {
		t.Errorf("LLM context should fall back to the chunk fragment, got: %q", llmClient.userMessage)
	}
}

// TestGenerateReport_DeduplicatesDocuments verifies each document is fetched
// once even when multiple matched chunks reference it.
func TestGenerateReport_DeduplicatesDocuments(t *testing.T) {
	ks := &mockKS{
		chunks: []domain.RankedChunk{
			{Chunk: domain.Chunk{ID: "c1", Content: "c1", Source: "src", DocumentID: "doc-1"}},
			{Chunk: domain.Chunk{ID: "c2", Content: "c2", Source: "src", DocumentID: "doc-1"}},
		},
		documents: map[string]domain.Document{
			"doc-1": {ID: "doc-1", Source: "src", Content: "full doc"},
		},
	}
	llmClient := &messageCaptureLLMClient{}
	worker := newWorkerForTest(ks, llmClient)

	_, err := worker.GenerateReport(context.Background(), "tenant-a", domain.ReportConfig{Type: domain.ReportDaily})
	if err != nil {
		t.Fatalf("GenerateReport failed: %v", err)
	}

	if len(ks.gotDocIDs) != 1 {
		t.Errorf("GetDocument called %d times, want 1 (deduplicated)", len(ks.gotDocIDs))
	}
}