package reports

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/agentic-demo/platform/internal/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// mockReportRepo implements Repository for unit tests.
type mockReportRepo struct {
	reports   map[uuid.UUID]db.Report
	createErr error
	findErr   error
}

func newMockReportRepo() *mockReportRepo {
	return &mockReportRepo{reports: make(map[uuid.UUID]db.Report)}
}

func (m *mockReportRepo) Create(_ context.Context, params *db.CreateReportParams) (db.Report, error) {
	if m.createErr != nil {
		return db.Report{}, m.createErr
	}
	r := db.Report{
		ID:          uuid.New(),
		TenantID:    params.TenantID,
		Type:        params.Type,
		Title:       params.Title,
		Content:     params.Content,
		Citations:   params.Citations,
		Focus:       params.Focus,
		ScheduleID:  params.ScheduleID,
		GeneratedAt: params.GeneratedAt,
		CreatedAt:   time.Now(),
	}
	m.reports[r.ID] = r
	return r, nil
}

func (m *mockReportRepo) GetByID(_ context.Context, id uuid.UUID) (db.Report, error) {
	if m.findErr != nil {
		return db.Report{}, m.findErr
	}
	r, ok := m.reports[id]
	if !ok {
		return db.Report{}, ErrReportNotFound
	}
	return r, nil
}

func (m *mockReportRepo) ListByTenant(_ context.Context, params *db.ListReportsByTenantParams) ([]db.Report, error) {
	var result []db.Report
	for k := range m.reports {
		if m.reports[k].TenantID == params.TenantID {
			result = append(result, m.reports[k])
		}
	}
	// Apply limit/offset.
	start := int(params.Offset)
	if start > len(result) {
		return nil, nil
	}
	end := start + int(params.Limit)
	if end > len(result) {
		end = len(result)
	}
	return result[start:end], nil
}

func (m *mockReportRepo) CountByTenant(_ context.Context, tenantID string) (int32, error) {
	var count int32
	for k := range m.reports {
		if m.reports[k].TenantID == tenantID {
			count++
		}
	}
	return count, nil
}

func (m *mockReportRepo) Delete(_ context.Context, id uuid.UUID) error {
	delete(m.reports, id)
	return nil
}

// --- Create tests ---

func TestCreateReport_Success(t *testing.T) {
	repo := newMockReportRepo()
	svc := NewService(repo)

	now := time.Now().Truncate(time.Second)
	r, err := svc.Create(context.Background(), &CreateReportParams{
		TenantID:    "t_abc123",
		Type:        "daily",
		Title:       "Daily Report — 2025-07-29",
		Content:     "# Revenue Trends\n\nRevenue is up 12%.",
		Citations:   json.RawMessage(`[{"source":"crm","doc":"q3-report"}]`),
		Focus:       "revenue trends",
		GeneratedAt: now,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.TenantID != "t_abc123" {
		t.Errorf("TenantID = %q, want %q", r.TenantID, "t_abc123")
	}
	if r.Type != "daily" {
		t.Errorf("Type = %q, want %q", r.Type, "daily")
	}
	if r.Title != "Daily Report — 2025-07-29" {
		t.Errorf("Title = %q, want %q", r.Title, "Daily Report — 2025-07-29")
	}
	if r.Focus != "revenue trends" {
		t.Errorf("Focus = %q, want %q", r.Focus, "revenue trends")
	}
	if r.ScheduleID != uuid.Nil {
		t.Errorf("ScheduleID = %v, want nil", r.ScheduleID)
	}
}

func TestCreateReport_WithScheduleID(t *testing.T) {
	repo := newMockReportRepo()
	svc := NewService(repo)

	schedID := uuid.New()
	r, err := svc.Create(context.Background(), &CreateReportParams{
		TenantID:    "t_abc123",
		Type:        "daily",
		Title:       "Scheduled Report",
		Content:     "content",
		ScheduleID:  schedID,
		GeneratedAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.ScheduleID != schedID {
		t.Errorf("ScheduleID = %v, want %v", r.ScheduleID, schedID)
	}
}

func TestCreateReport_EmptyTenantID(t *testing.T) {
	repo := newMockReportRepo()
	svc := NewService(repo)

	_, err := svc.Create(context.Background(), &CreateReportParams{
		TenantID: "",
		Type:     "daily",
		Title:    "test",
		Content:  "test",
	})
	if !errors.Is(err, ErrInvalidTenantID) {
		t.Errorf("error = %v, want %v", err, ErrInvalidTenantID)
	}
}

func TestCreateReport_NilCitationsDefaultToEmptyArray(t *testing.T) {
	repo := newMockReportRepo()
	svc := NewService(repo)

	r, err := svc.Create(context.Background(), &CreateReportParams{
		TenantID:    "t_abc123",
		Type:        "daily",
		Title:       "test",
		Content:     "test",
		GeneratedAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(r.Citations) != "[]" {
		t.Errorf("Citations = %s, want []", r.Citations)
	}
}

// --- GetByID tests ---

func TestGetByID_Success(t *testing.T) {
	repo := newMockReportRepo()
	svc := NewService(repo)

	created, _ := svc.Create(context.Background(), &CreateReportParams{
		TenantID:    "t_abc123",
		Type:        "daily",
		Title:       "test",
		Content:     "content",
		GeneratedAt: time.Now(),
	})

	got, err := svc.GetByID(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ID != created.ID {
		t.Errorf("ID = %v, want %v", got.ID, created.ID)
	}
	if got.Title != "test" {
		t.Errorf("Title = %q, want %q", got.Title, "test")
	}
}

func TestGetByID_NotFound(t *testing.T) {
	repo := newMockReportRepo()
	svc := NewService(repo)

	_, err := svc.GetByID(context.Background(), uuid.New())
	if !errors.Is(err, ErrReportNotFound) {
		t.Errorf("error = %v, want %v", err, ErrReportNotFound)
	}
}

// --- ListByTenant tests ---

func TestListByTenant_Success(t *testing.T) {
	repo := newMockReportRepo()
	svc := NewService(repo)

	for i := 0; i < 5; i++ {
		svc.Create(context.Background(), &CreateReportParams{
			TenantID:    "t_abc123",
			Type:        "daily",
			Title:       "report",
			Content:     "content",
			GeneratedAt: time.Now(),
		})
	}
	svc.Create(context.Background(), &CreateReportParams{
		TenantID:    "t_xyz789",
		Type:        "daily",
		Title:       "other",
		Content:     "content",
		GeneratedAt: time.Now(),
	})

	page, err := svc.ListByTenant(context.Background(), "t_abc123", 1, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(page.Reports) != 5 {
		t.Errorf("got %d reports, want 5", len(page.Reports))
	}
	if page.TotalCount != 5 {
		t.Errorf("TotalCount = %d, want 5", page.TotalCount)
	}
}

func TestListByTenant_Pagination(t *testing.T) {
	repo := newMockReportRepo()
	svc := NewService(repo)

	for i := 0; i < 5; i++ {
		svc.Create(context.Background(), &CreateReportParams{
			TenantID:    "t_abc123",
			Type:        "daily",
			Title:       "report",
			Content:     "content",
			GeneratedAt: time.Now(),
		})
	}

	page1, err := svc.ListByTenant(context.Background(), "t_abc123", 1, 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(page1.Reports) != 2 {
		t.Errorf("page 1: got %d reports, want 2", len(page1.Reports))
	}
	if page1.Page != 1 {
		t.Errorf("Page = %d, want 1", page1.Page)
	}
}

func TestListByTenant_EmptyResult(t *testing.T) {
	repo := newMockReportRepo()
	svc := NewService(repo)

	page, err := svc.ListByTenant(context.Background(), "t_nonexistent", 1, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(page.Reports) != 0 {
		t.Errorf("got %d reports, want 0", len(page.Reports))
	}
}

func TestListByTenant_EmptyTenantID(t *testing.T) {
	repo := newMockReportRepo()
	svc := NewService(repo)

	_, err := svc.ListByTenant(context.Background(), "", 1, 10)
	if !errors.Is(err, ErrInvalidTenantID) {
		t.Errorf("error = %v, want %v", err, ErrInvalidTenantID)
	}
}

func TestListByTenant_DefaultPageSize(t *testing.T) {
	repo := newMockReportRepo()
	svc := NewService(repo)

	// pageSize of 0 should default to 20.
	page, err := svc.ListByTenant(context.Background(), "t_abc123", 1, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if page.PageSize != 20 {
		t.Errorf("PageSize = %d, want 20", page.PageSize)
	}
}

func TestListByTenant_MaxPageSize(t *testing.T) {
	repo := newMockReportRepo()
	svc := NewService(repo)

	page, err := svc.ListByTenant(context.Background(), "t_abc123", 1, 200)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if page.PageSize != 20 {
		t.Errorf("PageSize = %d, want 20 (capped)", page.PageSize)
	}
}

// --- Delete tests ---

func TestDeleteReport_Success(t *testing.T) {
	repo := newMockReportRepo()
	svc := NewService(repo)

	created, _ := svc.Create(context.Background(), &CreateReportParams{
		TenantID:    "t_abc123",
		Type:        "daily",
		Title:       "to delete",
		Content:     "content",
		GeneratedAt: time.Now(),
	})

	if err := svc.Delete(context.Background(), created.ID); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err := svc.GetByID(context.Background(), created.ID)
	if !errors.Is(err, ErrReportNotFound) {
		t.Errorf("expected ErrReportNotFound after delete, got %v", err)
	}
}

// Ensure pgtype import is used.
var _ = pgtype.Text{String: "used"}
