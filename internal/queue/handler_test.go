package queue

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"
	"sync"
	"testing"

	"github.com/agentic-demo/platform/internal/domain"
	"github.com/agentic-demo/platform/internal/ingestion"
	"github.com/agentic-demo/platform/internal/knowledge"
	"github.com/agentic-demo/platform/internal/llm"
	"github.com/agentic-demo/platform/internal/reports"
	"github.com/agentic-demo/platform/internal/tools"
	"github.com/agentic-demo/platform/internal/usage"
	"github.com/hibiken/asynq"
)

// --- Mock dependencies ---

type mockKnowledgeStore struct {
	mu         sync.Mutex
	storeCalls int
	queryCalls int
}

func (m *mockKnowledgeStore) Store(_ context.Context, _ domain.TenantID, _ []domain.Chunk) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.storeCalls++
	return nil
}

func (m *mockKnowledgeStore) Query(_ context.Context, _ domain.TenantID, _ string, _ int, _ knowledge.QueryFilters) ([]domain.RankedChunk, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.queryCalls++
	return []domain.RankedChunk{
		{Chunk: domain.Chunk{ID: "c1", Content: "test context"}},
	}, nil
}

func (m *mockKnowledgeStore) DeleteTenantData(_ context.Context, _ domain.TenantID) error {
	return nil
}

func (m *mockKnowledgeStore) GetStats(_ context.Context, _ domain.TenantID) (map[string]int, error) {
	return nil, nil
}

type mockConnector struct {
	called bool
}

func (m *mockConnector) Extract(_ context.Context) ([]domain.RawDocument, error) {
	m.called = true
	return []domain.RawDocument{{Content: "test document", Metadata: map[string]string{"source": "test"}}}, nil
}

type mockEmbedder struct{}

func (m *mockEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	result := make([][]float32, len(texts))
	for i := range texts {
		result[i] = []float32{0.1, 0.2, 0.3}
	}
	return result, nil
}

func (m *mockEmbedder) Dimension() int { return 3 }

type mockLLMClient struct {
	called bool
}

func (m *mockLLMClient) Complete(_ context.Context, _ []domain.Message, _ llm.Options) (domain.CompletionResult, error) {
	m.called = true
	return domain.CompletionResult{
		Text:        "Report content",
		InputTokens: 10,
		OutputTokens: 20,
	}, nil
}

// --- IngestHandler tests ---

func TestIngestHandler_ProcessTask(t *testing.T) {
	conn := &mockConnector{}
	ks := &mockKnowledgeStore{}

	worker := ingestion.NewIngestWorker(
		map[string]ingestion.Connector{"crm": conn},
		knowledge.NewRecursiveChunker(100, 20),
		&mockEmbedder{},
		ks,
		usage.NoOpEmitter{},
	)

	handler := &IngestHandler{worker: worker}

	payload := IngestionPayload{TenantID: "tenant-1", SourceID: "crm"}
	data, _ := json.Marshal(payload)
	task := asynq.NewTask(TypeIngestionManual, data)

	err := handler.ProcessTask(context.Background(), task)
	if err != nil {
		t.Fatalf("ProcessTask: %v", err)
	}

	if !conn.called {
		t.Fatal("expected connector.Extract to be called")
	}
	if ks.storeCalls != 1 {
		t.Fatalf("expected 1 Store call, got %d", ks.storeCalls)
	}
}

func TestIngestHandler_InvalidPayload(t *testing.T) {
	task := asynq.NewTask(TypeIngestionManual, []byte(`{invalid json`))

	handler := &IngestHandler{worker: nil} // worker not needed — payload fails before use

	err := handler.ProcessTask(context.Background(), task)
	if err == nil {
		t.Fatal("expected error for invalid payload")
	}
}

// --- ReportHandler tests ---

func TestReportHandler_ProcessTask(t *testing.T) {
	mockLLM := &mockLLMClient{}
	ks := &mockKnowledgeStore{}

	worker := reports.NewReportWorker(
		ks,
		mockLLM,
		tools.NewRegistry(usage.NoOpEmitter{}),
		usage.NoOpEmitter{},
		5, 5, defaultMaxDuration,
	)

	handler := &ReportHandler{worker: worker}

	payload := ReportPayload{
		TenantID:   "tenant-1",
		ReportType: "daily",
		FocusAreas: []string{"revenue"},
	}
	data, _ := json.Marshal(payload)
	task := asynq.NewTask(TypeReportDaily, data)

	err := handler.ProcessTask(context.Background(), task)
	if err != nil {
		t.Fatalf("ProcessTask: %v", err)
	}

	if !mockLLM.called {
		t.Fatal("expected LLM client to be called")
	}
	if ks.queryCalls != 1 {
		t.Fatalf("expected 1 Query call, got %d", ks.queryCalls)
	}
}

func TestReportHandler_InvalidPayload(t *testing.T) {
	task := asynq.NewTask(TypeReportDaily, []byte(`not json`))

	handler := &ReportHandler{worker: nil}

	err := handler.ProcessTask(context.Background(), task)
	if err == nil {
		t.Fatal("expected error for invalid payload")
	}
}

// --- DeliveryHandler tests ---

func TestDeliveryHandler_ProcessTask(t *testing.T) {
	handler := &DeliveryHandler{logger: slog.Default()}

	payload := DeliveryPayload{TenantID: "tenant-1", ReportID: "r1"}
	data, _ := json.Marshal(payload)
	task := asynq.NewTask(TypeDeliveryEmail, data)

	// Stub handler should succeed without error.
	err := handler.ProcessTask(context.Background(), task)
	if err != nil {
		t.Fatalf("ProcessTask: %v", err)
	}
}

func TestDeliveryHandler_InvalidPayload(t *testing.T) {
	handler := &DeliveryHandler{logger: slog.Default()}

	task := asynq.NewTask(TypeDeliveryEmail, []byte(`bad`))

	err := handler.ProcessTask(context.Background(), task)
	if err == nil {
		t.Fatal("expected error for invalid payload")
	}
}

// --- RegisterHandlers test ---

func TestRegisterHandlers(t *testing.T) {
	mux := asynq.NewServeMux()

	conn := &mockConnector{}
	ks := &mockKnowledgeStore{}
	mockLLM := &mockLLMClient{}

	deps := HandlerDeps{
		IngestWorker: ingestion.NewIngestWorker(
			map[string]ingestion.Connector{"test": conn},
			knowledge.NewRecursiveChunker(100, 20),
			&mockEmbedder{},
			ks,
			usage.NoOpEmitter{},
		),
		ReportWorker: reports.NewReportWorker(
			ks,
			mockLLM,
			tools.NewRegistry(usage.NoOpEmitter{}),
			usage.NoOpEmitter{},
			5, 5, defaultMaxDuration,
		),
		Logger: slog.Default(),
	}

	RegisterHandlers(mux, deps)

	// Verify at least one handler is registered by processing a task through the mux.
	payload := IngestionPayload{TenantID: "t1", SourceID: "test"}
	data, _ := json.Marshal(payload)
	task := asynq.NewTask(TypeIngestionManual, data)

	err := mux.ProcessTask(context.Background(), task)
	if err != nil {
		t.Fatalf("mux.ProcessTask: %v", err)
	}
	if !conn.called {
		t.Fatal("expected connector to be called via mux")
	}
}

var defaultMaxDuration = 5 * time.Minute
