package scheduling

import (
	"context"
	"errors"
	"testing"

	"github.com/agentic-demo/platform/internal/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// mockRepository implements Repository for unit tests.
type mockRepository struct {
	schedules map[uuid.UUID]db.ReportSchedule
	createErr error
	findErr   error
}

func newMockRepo() *mockRepository {
	return &mockRepository{schedules: make(map[uuid.UUID]db.ReportSchedule)}
}

func (m *mockRepository) Create(_ context.Context, params *db.CreateScheduleParams) (db.ReportSchedule, error) {
	if m.createErr != nil {
		return db.ReportSchedule{}, m.createErr
	}
	s := db.ReportSchedule{
		ID:       uuid.New(),
		TenantID: params.TenantID,
		Type:     params.Type,
		CronExpr: params.CronExpr,
		Focus:    params.Focus,
		Format:   params.Format,
		Enabled:  true,
	}
	m.schedules[s.ID] = s
	return s, nil
}

func (m *mockRepository) GetByID(_ context.Context, id uuid.UUID) (db.ReportSchedule, error) {
	if m.findErr != nil {
		return db.ReportSchedule{}, m.findErr
	}
	s, ok := m.schedules[id]
	if !ok {
		return db.ReportSchedule{}, ErrScheduleNotFound
	}
	return s, nil
}

func (m *mockRepository) ListByTenant(_ context.Context, tenantID string) ([]db.ReportSchedule, error) {
	var result []db.ReportSchedule
	for k := range m.schedules {
		if m.schedules[k].TenantID == tenantID {
			result = append(result, m.schedules[k])
		}
	}
	return result, nil
}

func (m *mockRepository) Update(_ context.Context, params *db.UpdateScheduleParams) (db.ReportSchedule, error) {
	s, ok := m.schedules[params.ID]
	if !ok {
		return db.ReportSchedule{}, ErrScheduleNotFound
	}
	s.Type = params.Type
	s.CronExpr = params.CronExpr
	s.Focus = params.Focus
	s.Format = params.Format
	m.schedules[params.ID] = s
	return s, nil
}

func (m *mockRepository) Delete(_ context.Context, id uuid.UUID) error {
	delete(m.schedules, id)
	return nil
}

func (m *mockRepository) Toggle(_ context.Context, id uuid.UUID) (db.ReportSchedule, error) {
	if m.findErr != nil {
		return db.ReportSchedule{}, m.findErr
	}
	s, ok := m.schedules[id]
	if !ok {
		return db.ReportSchedule{}, ErrScheduleNotFound
	}
	s.Enabled = !s.Enabled
	m.schedules[id] = s
	return s, nil
}

func (m *mockRepository) ListAllEnabled(_ context.Context) ([]db.ReportSchedule, error) {
	var result []db.ReportSchedule
	for k := range m.schedules {
		if m.schedules[k].Enabled {
			result = append(result, m.schedules[k])
		}
	}
	return result, nil
}

// --- create tests ---

func TestCreate_Success(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo)

	s, err := svc.Create(context.Background(), &CreateScheduleParams{
		TenantID: "t_abc123",
		Type:     ScheduleDaily,
		CronExpr: "0 9 * * *",
		Focus:    "sales trends",
		Format:   "standard",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.TenantID != "t_abc123" {
		t.Errorf("TenantID = %q, want %q", s.TenantID, "t_abc123")
	}
	if s.Type != ScheduleDaily {
		t.Errorf("Type = %q, want %q", s.Type, ScheduleDaily)
	}
	if s.CronExpr != "0 9 * * *" {
		t.Errorf("CronExpr = %q, want %q", s.CronExpr, "0 9 * * *")
	}
	if s.Focus != "sales trends" {
		t.Errorf("Focus = %q, want %q", s.Focus, "sales trends")
	}
	if !s.Enabled {
		t.Error("new schedule should be enabled by default")
	}
}

func TestCreate_EmptyTenantID(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo)

	_, err := svc.Create(context.Background(), &CreateScheduleParams{
		TenantID: "",
		Type:     ScheduleDaily,
		CronExpr: "0 9 * * *",
	})
	if !errors.Is(err, ErrInvalidTenantID) {
		t.Errorf("error = %v, want %v", err, ErrInvalidTenantID)
	}
}

func TestCreate_InvalidScheduleType(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo)

	_, err := svc.Create(context.Background(), &CreateScheduleParams{
		TenantID: "t_abc123",
		Type:     "yearly",
		CronExpr: "0 9 * * *",
	})
	if !errors.Is(err, ErrInvalidScheduleType) {
		t.Errorf("error = %v, want %v", err, ErrInvalidScheduleType)
	}
}

func TestCreate_InvalidCronExpr(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo)

	_, err := svc.Create(context.Background(), &CreateScheduleParams{
		TenantID: "t_abc123",
		Type:     ScheduleDaily,
		CronExpr: "not-a-cron",
	})
	if !errors.Is(err, ErrInvalidCronExpr) {
		t.Errorf("error = %v, want %v", err, ErrInvalidCronExpr)
	}
}

func TestCreate_EmptyCronExpr(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo)

	_, err := svc.Create(context.Background(), &CreateScheduleParams{
		TenantID: "t_abc123",
		Type:     ScheduleDaily,
		CronExpr: "",
	})
	if !errors.Is(err, ErrInvalidCronExpr) {
		t.Errorf("error = %v, want %v", err, ErrInvalidCronExpr)
	}
}

func TestCreate_DefaultFormat(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo)

	s, err := svc.Create(context.Background(), &CreateScheduleParams{
		TenantID: "t_abc123",
		Type:     ScheduleWeekly,
		CronExpr: "0 9 * * 1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.Format != "standard" {
		t.Errorf("Format = %q, want %q", s.Format, "standard")
	}
}

func TestCreate_ValidCronVariants(t *testing.T) {
	tests := []struct {
		name string
		expr string
	}{
		{"daily at 9am", "0 9 * * *"},
		{"weekly Monday 8am", "0 8 * * 1"},
		{"monthly 1st at midnight", "0 0 1 * *"},
		{"every 30 minutes", "*/30 * * * *"},
		{"weekdays at 18:00", "0 18 * * 1-5"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newMockRepo()
			svc := NewService(repo)

			_, err := svc.Create(context.Background(), &CreateScheduleParams{
				TenantID: "t_abc123",
				Type:     ScheduleDaily,
				CronExpr: tt.expr,
			})
			if err != nil {
				t.Errorf("unexpected error for %q: %v", tt.expr, err)
			}
		})
	}
}

// --- get / list tests ---

func TestGetByID_Success(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo)

	created, _ := svc.Create(context.Background(), &CreateScheduleParams{
		TenantID: "t_abc123",
		Type:     ScheduleDaily,
		CronExpr: "0 9 * * *",
	})

	got, err := svc.GetByID(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ID != created.ID {
		t.Errorf("ID = %v, want %v", got.ID, created.ID)
	}
}

func TestGetByID_NotFound(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo)

	_, err := svc.GetByID(context.Background(), uuid.New())
	if !errors.Is(err, ErrScheduleNotFound) {
		t.Errorf("error = %v, want %v", err, ErrScheduleNotFound)
	}
}

func TestListByTenant(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo)

	svc.Create(context.Background(), &CreateScheduleParams{
		TenantID: "t_abc123", Type: ScheduleDaily, CronExpr: "0 9 * * *",
	})
	svc.Create(context.Background(), &CreateScheduleParams{
		TenantID: "t_abc123", Type: ScheduleWeekly, CronExpr: "0 9 * * 1",
	})
	svc.Create(context.Background(), &CreateScheduleParams{
		TenantID: "t_xyz789", Type: ScheduleDaily, CronExpr: "0 8 * * *",
	})

	abcSchedules, err := svc.ListByTenant(context.Background(), "t_abc123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(abcSchedules) != 2 {
		t.Errorf("got %d schedules for tenant abc, want 2", len(abcSchedules))
	}

	xyzSchedules, err := svc.ListByTenant(context.Background(), "t_xyz789")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(xyzSchedules) != 1 {
		t.Errorf("got %d schedules for tenant xyz, want 1", len(xyzSchedules))
	}
}

func TestListByTenant_Empty(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo)

	result, err := svc.ListByTenant(context.Background(), "t_nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("got %d schedules, want 0", len(result))
	}
}

// --- update tests ---

func TestUpdate_Success(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo)

	created, _ := svc.Create(context.Background(), &CreateScheduleParams{
		TenantID: "t_abc123", Type: ScheduleDaily, CronExpr: "0 9 * * *",
	})

	updated, err := svc.Update(context.Background(), &UpdateScheduleParams{
		ID:       created.ID,
		Type:     ScheduleWeekly,
		CronExpr: "0 10 * * 1",
		Focus:    "new focus",
		Format:   "detailed",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updated.Type != ScheduleWeekly {
		t.Errorf("Type = %q, want %q", updated.Type, ScheduleWeekly)
	}
	if updated.CronExpr != "0 10 * * 1" {
		t.Errorf("CronExpr = %q, want %q", updated.CronExpr, "0 10 * * 1")
	}
	if updated.Focus != "new focus" {
		t.Errorf("Focus = %q, want %q", updated.Focus, "new focus")
	}
	if updated.Format != "detailed" {
		t.Errorf("Format = %q, want %q", updated.Format, "detailed")
	}
}

func TestUpdate_NotFound(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo)

	_, err := svc.Update(context.Background(), &UpdateScheduleParams{
		ID:       uuid.New(),
		Type:     ScheduleDaily,
		CronExpr: "0 9 * * *",
	})
	if !errors.Is(err, ErrScheduleNotFound) {
		t.Errorf("error = %v, want %v", err, ErrScheduleNotFound)
	}
}

func TestUpdate_InvalidType(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo)

	created, _ := svc.Create(context.Background(), &CreateScheduleParams{
		TenantID: "t_abc123", Type: ScheduleDaily, CronExpr: "0 9 * * *",
	})

	_, err := svc.Update(context.Background(), &UpdateScheduleParams{
		ID:       created.ID,
		Type:     "yearly",
		CronExpr: "0 9 * * *",
	})
	if !errors.Is(err, ErrInvalidScheduleType) {
		t.Errorf("error = %v, want %v", err, ErrInvalidScheduleType)
	}
}

// --- delete tests ---

func TestDelete_Success(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo)

	created, _ := svc.Create(context.Background(), &CreateScheduleParams{
		TenantID: "t_abc123", Type: ScheduleDaily, CronExpr: "0 9 * * *",
	})

	if err := svc.Delete(context.Background(), created.ID); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err := svc.GetByID(context.Background(), created.ID)
	if !errors.Is(err, ErrScheduleNotFound) {
		t.Errorf("expected ErrScheduleNotFound after delete, got %v", err)
	}
}

// --- toggle tests ---

func TestToggle_EnableDisable(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo)

	created, _ := svc.Create(context.Background(), &CreateScheduleParams{
		TenantID: "t_abc123", Type: ScheduleDaily, CronExpr: "0 9 * * *",
	})
	if !created.Enabled {
		t.Fatal("expected new schedule to be enabled")
	}

	toggled, err := svc.Toggle(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if toggled.Enabled {
		t.Error("expected schedule to be disabled after toggle")
	}

	toggledAgain, err := svc.Toggle(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !toggledAgain.Enabled {
		t.Error("expected schedule to be re-enabled after second toggle")
	}
}

func TestToggle_NotFound(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo)

	_, err := svc.Toggle(context.Background(), uuid.New())
	if !errors.Is(err, ErrScheduleNotFound) {
		t.Errorf("error = %v, want %v", err, ErrScheduleNotFound)
	}
}

// --- listAllEnabled tests ---

func TestListAllEnabled(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo)

	a, _ := svc.Create(context.Background(), &CreateScheduleParams{
		TenantID: "t_abc123", Type: ScheduleDaily, CronExpr: "0 9 * * *",
	})
	b, _ := svc.Create(context.Background(), &CreateScheduleParams{
		TenantID: "t_xyz789", Type: ScheduleDaily, CronExpr: "0 9 * * *",
	})
	// Disable one.
	svc.Toggle(context.Background(), b.ID)

	enabled, err := svc.ListAllEnabled(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(enabled) != 1 {
		t.Errorf("got %d enabled schedules, want 1", len(enabled))
	}
	if len(enabled) > 0 && enabled[0].ID != a.ID {
		t.Errorf("enabled schedule ID = %v, want %v", enabled[0].ID, a.ID)
	}
}

func TestListAllEnabled_None(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo)

	result, err := svc.ListAllEnabled(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("got %d schedules, want 0", len(result))
	}
}

// Ensure pgtype import is used.
var _ = pgtype.Text{String: "used"}
