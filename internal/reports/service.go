package reports

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/agentic-demo/platform/internal/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
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

// --- generation job tracking ---

func (s *reportService) TrackGenerationJob(ctx context.Context, tenantID, taskID, reportType, focus string) error {
	if strings.TrimSpace(tenantID) == "" {
		return ErrInvalidTenantID
	}
	if strings.TrimSpace(taskID) == "" {
		return ErrInvalidTaskID
	}
	if strings.TrimSpace(reportType) == "" {
		return ErrInvalidReportType
	}

	_, err := s.repo.CreateGenerationJob(ctx, &db.CreateGenerationJobParams{
		TenantID:   tenantID,
		TaskID:     taskID,
		ReportType: reportType,
		Focus:      strings.TrimSpace(focus),
	})
	if err != nil {
		return fmt.Errorf("track generation job: %w", err)
	}
	return nil
}

func (s *reportService) ListGenerationJobs(ctx context.Context, tenantID string, limit int) ([]GenerationJob, error) {
	if strings.TrimSpace(tenantID) == "" {
		return nil, ErrInvalidTenantID
	}
	if limit < 1 {
		limit = 10
	}
	if limit > 50 {
		limit = 50
	}

	rows, err := s.repo.ListGenerationJobsByTenant(ctx, &db.ListGenerationJobsByTenantParams{
		TenantID: tenantID,
		Limit:    int32(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("list generation jobs: %w", err)
	}

	jobs := make([]GenerationJob, len(rows))
	for i := range rows {
		jobs[i] = toGenerationJob(&rows[i])
	}
	return jobs, nil
}

func (s *reportService) MarkGenerationJobRunning(ctx context.Context, taskID string) error {
	return s.updateGenerationJob(ctx, taskID, GenerationJobRunning, "", nil)
}

func (s *reportService) MarkGenerationJobSucceeded(ctx context.Context, taskID string) error {
	return s.updateGenerationJob(ctx, taskID, GenerationJobSucceeded, "", ptr(time.Now()))
}

func (s *reportService) MarkGenerationJobFailed(ctx context.Context, taskID, errMsg string) error {
	return s.updateGenerationJob(ctx, taskID, GenerationJobFailed, errMsg, ptr(time.Now()))
}

func (s *reportService) updateGenerationJob(ctx context.Context, taskID string, status GenerationJobStatus, errMsg string, finishedAt *time.Time) error {
	params := &db.UpdateGenerationJobParams{
		TaskID: taskID,
		Status: string(status),
		Error:  errMsg,
	}
	if finishedAt != nil {
		params.FinishedAt = pgtype.Timestamptz{Time: *finishedAt, Valid: true}
	}
	if err := s.repo.UpdateGenerationJob(ctx, params); err != nil {
		return fmt.Errorf("update generation job %s: %w", taskID, err)
	}
	return nil
}

// toGenerationJob converts a sqlc row into a domain GenerationJob.
func toGenerationJob(row *db.ReportGenerationJob) GenerationJob {
	job := GenerationJob{
		ID:         row.ID,
		TenantID:   row.TenantID,
		TaskID:     row.TaskID,
		ReportType: row.ReportType,
		Focus:      row.Focus,
		Status:     GenerationJobStatus(row.Status),
		Error:      row.Error,
		EnqueuedAt: row.EnqueuedAt,
	}
	if row.FinishedAt.Valid {
		t := row.FinishedAt.Time
		job.FinishedAt = &t
	}
	return job
}

func ptr[T any](v T) *T { return &v }

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
