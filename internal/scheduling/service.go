package scheduling

import (
	"context"
	"strings"

	"github.com/agentic-demo/platform/internal/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/robfig/cron/v3"
)

type scheduleService struct {
	repo Repository
}

// NewService creates a ScheduleService.
func NewService(repo Repository) ScheduleService {
	return &scheduleService{repo: repo}
}

func (s *scheduleService) Create(ctx context.Context, params *CreateScheduleParams) (ReportSchedule, error) {
	if err := validateCreateParams(params); err != nil {
		return ReportSchedule{}, err
	}

	row, err := s.repo.Create(ctx, &db.CreateScheduleParams{
		TenantID: params.TenantID,
		Type:     string(params.Type),
		CronExpr: params.CronExpr,
		Focus:    toPgText(params.Focus),
		Format:   scheduleFormat(params.Format),
	})
	if err != nil {
		return ReportSchedule{}, err
	}
	return toDomain(&row), nil
}

func (s *scheduleService) GetByID(ctx context.Context, id uuid.UUID) (ReportSchedule, error) {
	row, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return ReportSchedule{}, err
	}
	return toDomain(&row), nil
}

func (s *scheduleService) ListByTenant(ctx context.Context, tenantID string) ([]ReportSchedule, error) {
	rows, err := s.repo.ListByTenant(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	return toDomainSlice(rows), nil
}

func (s *scheduleService) Update(ctx context.Context, params *UpdateScheduleParams) (ReportSchedule, error) {
	if err := validateUpdateParams(params); err != nil {
		return ReportSchedule{}, err
	}

	row, err := s.repo.Update(ctx, &db.UpdateScheduleParams{
		ID:       params.ID,
		Type:     string(params.Type),
		CronExpr: params.CronExpr,
		Focus:    toPgText(params.Focus),
		Format:   scheduleFormat(params.Format),
	})
	if err != nil {
		return ReportSchedule{}, err
	}
	return toDomain(&row), nil
}

func (s *scheduleService) Delete(ctx context.Context, id uuid.UUID) error {
	return s.repo.Delete(ctx, id)
}

func (s *scheduleService) Toggle(ctx context.Context, id uuid.UUID) (ReportSchedule, error) {
	row, err := s.repo.Toggle(ctx, id)
	if err != nil {
		return ReportSchedule{}, err
	}
	return toDomain(&row), nil
}

func (s *scheduleService) ListAllEnabled(ctx context.Context) ([]ReportSchedule, error) {
	rows, err := s.repo.ListAllEnabled(ctx)
	if err != nil {
		return nil, err
	}
	return toDomainSlice(rows), nil
}

// --- validation ---

func validateCreateParams(params *CreateScheduleParams) error {
	if strings.TrimSpace(params.TenantID) == "" {
		return ErrInvalidTenantID
	}
	if strings.TrimSpace(params.TenantID) == "" {
		return ErrInvalidTenantID
	}
	if err := validateScheduleType(params.Type); err != nil {
		return err
	}
	if err := validateCronExpr(params.CronExpr); err != nil {
		return err
	}
	return nil
}

func validateUpdateParams(params *UpdateScheduleParams) error {
	if err := validateScheduleType(params.Type); err != nil {
		return err
	}
	if err := validateCronExpr(params.CronExpr); err != nil {
		return err
	}
	return nil
}

func validateScheduleType(t ScheduleType) error {
	switch t {
	case ScheduleDaily, ScheduleWeekly, ScheduleMonthly:
		return nil
	default:
		return ErrInvalidScheduleType
	}
}

func validateCronExpr(expr string) error {
	if strings.TrimSpace(expr) == "" {
		return ErrInvalidCronExpr
	}
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	if _, err := parser.Parse(expr); err != nil {
		return ErrInvalidCronExpr
	}
	return nil
}

func scheduleFormat(f string) string {
	if f == "" {
		return "standard"
	}
	return f
}

// --- domain conversion ---

func toDomain(row *db.ReportSchedule) ReportSchedule {
	return ReportSchedule{
		ID:        row.ID,
		TenantID:  row.TenantID,
		Type:      ScheduleType(row.Type),
		CronExpr:  row.CronExpr,
		Focus:     pgTextToString(row.Focus),
		Format:    row.Format,
		Enabled:   row.Enabled,
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}
}

func toDomainSlice(rows []db.ReportSchedule) []ReportSchedule {
	result := make([]ReportSchedule, len(rows))
	for i := range rows {
		result[i] = toDomain(&rows[i])
	}
	return result
}

func toPgText(s string) pgtype.Text {
	if s == "" {
		return pgtype.Text{Valid: false}
	}
	return pgtype.Text{String: s, Valid: true}
}

func pgTextToString(t pgtype.Text) string {
	if t.Valid {
		return t.String
	}
	return ""
}
