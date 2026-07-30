package queue

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/agentic-demo/platform/internal/delivery"
	"github.com/agentic-demo/platform/internal/domain"
	"github.com/agentic-demo/platform/internal/ingestion"
	"github.com/agentic-demo/platform/internal/knowledge"
	"github.com/agentic-demo/platform/internal/llm"
	"github.com/agentic-demo/platform/internal/reports"
	"github.com/agentic-demo/platform/internal/tools"
	"github.com/agentic-demo/platform/internal/usage"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
)

// --- Mock dependencies ---

type mockKnowledgeStore struct {
	storeCalls atomic.Int32
	queryCalls atomic.Int32
}

func (m *mockKnowledgeStore) Store(_ context.Context, _ domain.TenantID, _ []domain.Chunk) error {
	m.storeCalls.Add(1)
	return nil
}

func (m *mockKnowledgeStore) Query(_ context.Context, _ domain.TenantID, _ string, _ int, _ knowledge.QueryFilters) ([]domain.RankedChunk, error) {
	m.queryCalls.Add(1)
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
	called atomic.Bool
}

func (m *mockConnector) Extract(_ context.Context) ([]domain.RawDocument, error) {
	m.called.Store(true)
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
	called atomic.Bool
}

func (m *mockLLMClient) Complete(_ context.Context, _ []domain.Message, _ llm.Options) (domain.CompletionResult, error) {
	m.called.Store(true)
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
		"text-embedding-v4",
	)

	handler := &IngestHandler{worker: worker}

	payload := IngestionPayload{TenantID: "tenant-1", SourceID: "crm"}
	data, _ := json.Marshal(payload)
	task := asynq.NewTask(TypeIngestionManual, data)

	err := handler.ProcessTask(context.Background(), task)
	if err != nil {
		t.Fatalf("ProcessTask: %v", err)
	}

	if !conn.called.Load() {
		t.Fatal("expected connector.Extract to be called")
	}
	if ks.storeCalls.Load() != 1 {
		t.Fatalf("expected 1 Store call, got %d", ks.storeCalls.Load())
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

	if !mockLLM.called.Load() {
		t.Fatal("expected LLM client to be called")
	}
	if ks.queryCalls.Load() != 1 {
		t.Fatalf("expected 1 Query call, got %d", ks.queryCalls.Load())
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

func TestDeliveryHandler_ProcessTask_NoService(t *testing.T) {
	handler := &DeliveryHandler{logger: slog.Default()}

	payload := DeliveryPayload{TenantID: "tenant-1", ReportID: uuid.New().String()}
	data, _ := json.Marshal(payload)
	task := asynq.NewTask(TypeDeliveryEmail, data)

	// No delivery service configured — should skip without error.
	err := handler.ProcessTask(context.Background(), task)
	if err != nil {
		t.Fatalf("ProcessTask: %v", err)
	}
}

func TestDeliveryHandler_DeliversReport(t *testing.T) {
	reportID := uuid.New()
	svc := &mockReportService{}
	// Pre-populate the mock so GetByID finds it.
	svc.created = append(svc.created, reports.StoredReport{
		ID:       reportID,
		TenantID: "tenant-1",
		Type:     "daily",
		Title:    "Daily Report — Jul 29, 2025",
		Content:  "# Revenue\n\nRevenue is up.",
	})

	del := &mockDeliveryService{}

	handler := &DeliveryHandler{
		deliveryService: del,
		reportService:   svc,
		logger:          slog.Default(),
	}

	payload := DeliveryPayload{
		TenantID:       "tenant-1",
		ReportID:       reportID.String(),
		RecipientEmail: "user@example.com",
	}
	data, _ := json.Marshal(payload)
	task := asynq.NewTask(TypeDeliveryEmail, data)

	err := handler.ProcessTask(context.Background(), task)
	if err != nil {
		t.Fatalf("ProcessTask: %v", err)
	}

	if len(del.delivered) != 1 {
		t.Fatalf("expected 1 email delivered, got %d", len(del.delivered))
	}
	if del.delivered[0].To[0] != "user@example.com" {
		t.Errorf("To = %q, want %q", del.delivered[0].To[0], "user@example.com")
	}
	if !containsString(del.delivered[0].Subject, "Daily") {
		t.Errorf("Subject = %q, expected to contain 'Daily'", del.delivered[0].Subject)
	}
}

func TestDeliveryHandler_NoRecipient_SkipsDelivery(t *testing.T) {
	reportID := uuid.New()
	svc := &mockReportService{}
	svc.created = append(svc.created, reports.StoredReport{
		ID:       reportID,
		TenantID: "tenant-1",
		Type:     "daily",
		Title:    "test",
		Content:  "content",
	})

	del := &mockDeliveryService{}

	handler := &DeliveryHandler{
		deliveryService: del,
		reportService:   svc,
		logger:          slog.Default(),
	}

	payload := DeliveryPayload{
		TenantID:       "tenant-1",
		ReportID:       reportID.String(),
		RecipientEmail: "", // no recipient
	}
	data, _ := json.Marshal(payload)
	task := asynq.NewTask(TypeDeliveryEmail, data)

	err := handler.ProcessTask(context.Background(), task)
	if err != nil {
		t.Fatalf("ProcessTask: %v", err)
	}

	if len(del.delivered) != 0 {
		t.Errorf("expected 0 emails delivered, got %d", len(del.delivered))
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

func containsString(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || s != "" && searchStr(s, sub))
}

func searchStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
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
			"text-embedding-v4",
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
	if !conn.called.Load() {
		t.Fatal("expected connector to be called via mux")
	}
}

// --- mockReportService ---

type mockReportService struct {
	created   []reports.StoredReport
	createErr error
}

func (m *mockReportService) Create(_ context.Context, params *reports.CreateReportParams) (reports.StoredReport, error) {
	if m.createErr != nil {
		return reports.StoredReport{}, m.createErr
	}
	r := reports.StoredReport{
		ID:          uuid.New(),
		TenantID:    params.TenantID,
		Type:        params.Type,
		Title:       params.Title,
		Content:     params.Content,
		Focus:       params.Focus,
		ScheduleID:  params.ScheduleID,
		GeneratedAt: params.GeneratedAt,
		CreatedAt:   time.Now(),
	}
	m.created = append(m.created, r)
	return r, nil
}

func (m *mockReportService) GetByID(_ context.Context, id uuid.UUID) (reports.StoredReport, error) {
	for i := range m.created {
		if m.created[i].ID == id {
			return m.created[i], nil
		}
	}
	return reports.StoredReport{}, reports.ErrReportNotFound
}

func (m *mockReportService) ListByTenant(_ context.Context, _ string, _, _ int) (reports.ReportPage, error) {
	return reports.ReportPage{}, nil
}

func (m *mockReportService) Delete(_ context.Context, _ uuid.UUID) error {
	return nil
}

// --- mockDeliveryService ---

type mockDeliveryService struct {
	delivered []delivery.DeliverParams
	deliverErr error
}

func (m *mockDeliveryService) Deliver(_ context.Context, params delivery.DeliverParams) error {
	if m.deliverErr != nil {
		return m.deliverErr
	}
	m.delivered = append(m.delivered, params)
	return nil
}

// --- mockJobQueue ---

type mockJobQueue struct {
	jobs       []Job
	enqueueErr error
}

func (m *mockJobQueue) Enqueue(_ context.Context, job Job) (*JobResult, error) {
	if m.enqueueErr != nil {
		return nil, m.enqueueErr
	}
	m.jobs = append(m.jobs, job)
	return &JobResult{ID: "task-" + uuid.New().String()[:8], Queue: job.Queue}, nil
}

func (m *mockJobQueue) EnqueueAt(_ context.Context, job Job, _ time.Time) (*JobResult, error) {
	return m.Enqueue(context.Background(), job)
}

func (m *mockJobQueue) Close() error { return nil }

// --- ReportHandler persistence tests ---

func TestReportHandler_PersistsReport(t *testing.T) {
	mockLLM := &mockLLMClient{}
	ks := &mockKnowledgeStore{}
	svc := &mockReportService{}

	worker := reports.NewReportWorker(
		ks, mockLLM,
		tools.NewRegistry(usage.NoOpEmitter{}),
		usage.NoOpEmitter{},
		5, 5, defaultMaxDuration,
	)

	handler := &ReportHandler{
		worker:        worker,
		reportService: svc,
		logger:        slog.Default(),
	}

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

	if len(svc.created) != 1 {
		t.Fatalf("expected 1 report created, got %d", len(svc.created))
	}
	if svc.created[0].TenantID != "tenant-1" {
		t.Errorf("TenantID = %q, want %q", svc.created[0].TenantID, "tenant-1")
	}
	if svc.created[0].Type != "daily" {
		t.Errorf("Type = %q, want %q", svc.created[0].Type, "daily")
	}
	if svc.created[0].Focus != "revenue" {
		t.Errorf("Focus = %q, want %q", svc.created[0].Focus, "revenue")
	}
}

func TestReportHandler_EnqueuesDelivery(t *testing.T) {
	mockLLM := &mockLLMClient{}
	ks := &mockKnowledgeStore{}
	svc := &mockReportService{}
	q := &mockJobQueue{}

	worker := reports.NewReportWorker(
		ks, mockLLM,
		tools.NewRegistry(usage.NoOpEmitter{}),
		usage.NoOpEmitter{},
		5, 5, defaultMaxDuration,
	)

	handler := &ReportHandler{
		worker:        worker,
		reportService: svc,
		queue:         q,
		logger:        slog.Default(),
	}

	payload := ReportPayload{
		TenantID:   "tenant-1",
		ReportType: "daily",
	}
	data, _ := json.Marshal(payload)
	task := asynq.NewTask(TypeReportDaily, data)

	err := handler.ProcessTask(context.Background(), task)
	if err != nil {
		t.Fatalf("ProcessTask: %v", err)
	}

	if len(q.jobs) != 1 {
		t.Fatalf("expected 1 delivery job enqueued, got %d", len(q.jobs))
	}
	if q.jobs[0].Type != TypeDeliveryEmail {
		t.Errorf("job type = %q, want %q", q.jobs[0].Type, TypeDeliveryEmail)
	}
	if q.jobs[0].Queue != QueueDelivery {
		t.Errorf("job queue = %q, want %q", q.jobs[0].Queue, QueueDelivery)
	}
}

func TestReportHandler_WithScheduleID(t *testing.T) {
	mockLLM := &mockLLMClient{}
	ks := &mockKnowledgeStore{}
	svc := &mockReportService{}

	worker := reports.NewReportWorker(
		ks, mockLLM,
		tools.NewRegistry(usage.NoOpEmitter{}),
		usage.NoOpEmitter{},
		5, 5, defaultMaxDuration,
	)

	handler := &ReportHandler{
		worker:        worker,
		reportService: svc,
		logger:        slog.Default(),
	}

	schedID := uuid.New()
	payload := ReportPayload{
		TenantID:   "tenant-1",
		ReportType: "weekly",
		ScheduleID: schedID.String(),
	}
	data, _ := json.Marshal(payload)
	task := asynq.NewTask(TypeReportWeekly, data)

	err := handler.ProcessTask(context.Background(), task)
	if err != nil {
		t.Fatalf("ProcessTask: %v", err)
	}

	if len(svc.created) != 1 {
		t.Fatalf("expected 1 report created, got %d", len(svc.created))
	}
	if svc.created[0].ScheduleID != schedID {
		t.Errorf("ScheduleID = %v, want %v", svc.created[0].ScheduleID, schedID)
	}
}

// --- reportTitle tests ---

func TestReportTitle(t *testing.T) {
	date := time.Date(2025, 7, 29, 9, 0, 0, 0, time.UTC)

	tests := []struct {
		reportType string
		want       string
	}{
		{"daily", "Daily Report — Jul 29, 2025"},
		{"weekly", "Weekly Report — Jul 29, 2025"},
		{"monthly", "Monthly Report — Jul 29, 2025"},
		{"on_demand", "On Demand Report — Jul 29, 2025"},
	}
	for _, tt := range tests {
		t.Run(tt.reportType, func(t *testing.T) {
			got := reportTitle(tt.reportType, date)
			if got != tt.want {
				t.Errorf("reportTitle(%q) = %q, want %q", tt.reportType, got, tt.want)
			}
		})
	}
}

var defaultMaxDuration = 5 * time.Minute
