package reports

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/agentic-demo/platform/internal/db"
	"github.com/google/uuid"
)

type reportService struct {
	repo Repository
}

// NewService creates a ReportService.
func NewService(repo Repository) ReportService {
	return &reportService{repo: repo}
}

func (s *reportService) Create(ctx context.Context, params *CreateReportParams) (StoredReport, error) {
	if strings.TrimSpace(params.TenantID) == "" {
		return StoredReport{}, ErrInvalidTenantID
	}

	citations := params.Citations
	if citations == nil {
		citations = json.RawMessage("[]")
	}

	row, err := s.repo.Create(ctx, &db.CreateReportParams{
		TenantID:    params.TenantID,
		Type:        params.Type,
		Title:       params.Title,
		Content:     params.Content,
		Citations:   citations,
		Focus:       toPgText(params.Focus),
		ScheduleID:  toPgUUID(params.ScheduleID),
		GeneratedAt: params.GeneratedAt,
	})
	if err != nil {
		return StoredReport{}, err
	}
	return toDomain(&row), nil
}

func (s *reportService) GetByID(ctx context.Context, id uuid.UUID) (StoredReport, error) {
	row, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return StoredReport{}, err
	}
	return toDomain(&row), nil
}

func (s *reportService) ListByTenant(ctx context.Context, tenantID string, page, pageSize int) (ReportPage, error) {
	if strings.TrimSpace(tenantID) == "" {
		return ReportPage{}, ErrInvalidTenantID
	}

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	offset := int32((page - 1) * pageSize) //nolint:gosec // page/pageSize bounded above

	rows, err := s.repo.ListByTenant(ctx, &db.ListReportsByTenantParams{
		TenantID: tenantID,
		Limit:    int32(pageSize),
		Offset:   offset,
	})
	if err != nil {
		return ReportPage{}, err
	}

	total, err := s.repo.CountByTenant(ctx, tenantID)
	if err != nil {
		return ReportPage{}, err
	}

	reports := make([]StoredReport, len(rows))
	for i := range rows {
		reports[i] = toDomain(&rows[i])
	}

	return ReportPage{
		Reports:    reports,
		TotalCount: int(total),
		Page:       page,
		PageSize:   pageSize,
	}, nil
}

func (s *reportService) Delete(ctx context.Context, id uuid.UUID) error {
	return s.repo.Delete(ctx, id)
}

// --- domain conversion ---

func toDomain(row *db.Report) StoredReport {
	return StoredReport{
		ID:          row.ID,
		TenantID:    row.TenantID,
		Type:        row.Type,
		Title:       row.Title,
		Content:     row.Content,
		Citations:   json.RawMessage(row.Citations),
		Focus:       pgTextToString(row.Focus),
		ScheduleID:  pgUUIDToUUID(row.ScheduleID),
		GeneratedAt: row.GeneratedAt,
		CreatedAt:   row.CreatedAt,
	}
}
