package usage

import (
	"context"
	"strings"
	"time"

	"github.com/agentic-demo/platform/internal/db"
)

// UsageService manages usage data queries. Handlers use this interface.
type UsageService interface {
	GetSummary(ctx context.Context, tenantID, from, to string) (*UsageSummary, error)
	GetCurrentUsage(ctx context.Context, tenantID string) (*CurrentUsage, error)
	ListEvents(ctx context.Context, tenantID string, page, pageSize int) (*UsageEventPage, error)
}

type usageService struct {
	repo   Repository
	reader UsageReader
}

// NewService creates a UsageService.
func NewService(repo Repository, reader UsageReader) UsageService {
	return &usageService{repo: repo, reader: reader}
}

func (s *usageService) GetSummary(ctx context.Context, tenantID, from, to string) (*UsageSummary, error) {
	tenantID = strings.TrimSpace(tenantID)
	if tenantID == "" {
		return nil, ErrInvalidTenantID
	}

	fromDate, toDate, err := parseDateRange(from, to)
	if err != nil {
		return nil, ErrInvalidDateRange
	}

	rows, err := s.repo.GetDailySummary(ctx, &db.GetUsageDailySummaryParams{
		TenantID: tenantID,
		Column2:  toPgDate(fromDate),
		Column3:  toPgDate(toDate),
	})
	if err != nil {
		return nil, err
	}

	summary := &UsageSummary{
		TenantID: tenantID,
		From:     fromDate,
		To:       toDate,
		Models:   make([]ModelUsageSummary, 0),
	}

	for _, row := range rows {
		summary.TotalCostUSD += row.EstimatedCostUSD
		summary.Models = append(summary.Models, ModelUsageSummary{
			Model:           row.Model,
			InputTokens:     row.InputTokens,
			OutputTokens:    row.OutputTokens,
			ToolCalls:       row.ToolCalls,
			EmbeddingTokens: row.EmbeddingTokens,
			CostUSD:         row.EstimatedCostUSD,
		})
	}

	return summary, nil
}

func (s *usageService) GetCurrentUsage(ctx context.Context, tenantID string) (*CurrentUsage, error) {
	tenantID = strings.TrimSpace(tenantID)
	if tenantID == "" {
		return nil, ErrInvalidTenantID
	}

	return s.reader.GetCurrentUsage(ctx, tenantID)
}

func (s *usageService) ListEvents(ctx context.Context, tenantID string, page, pageSize int) (*UsageEventPage, error) {
	tenantID = strings.TrimSpace(tenantID)
	if tenantID == "" {
		return nil, ErrInvalidTenantID
	}

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	offset := int32((page - 1) * pageSize) //nolint:gosec // page/pageSize bounded above

	events, err := s.repo.ListEvents(ctx, &db.ListUsageEventsParams{
		TenantID: tenantID,
		Limit:    int32(pageSize),
		Offset:   offset,
	})
	if err != nil {
		return nil, err
	}

	total, err := s.repo.CountEvents(ctx, &db.CountUsageEventsParams{
		TenantID: tenantID,
	})
	if err != nil {
		return nil, err
	}

	return &UsageEventPage{
		Events:     events,
		TotalCount: int(total),
		Page:       page,
		PageSize:   pageSize,
	}, nil
}

// parseDateRange parses optional date strings into time.Time values.
// Empty strings are treated as unbounded (zero time).
func parseDateRange(from, to string) (fromDate, toDate time.Time, err error) {
	if from != "" {
		fromDate, err = time.Parse("2006-01-02", from)
		if err != nil {
			return time.Time{}, time.Time{}, err
		}
	}
	if to != "" {
		toDate, err = time.Parse("2006-01-02", to)
		if err != nil {
			return time.Time{}, time.Time{}, err
		}
	}

	if !fromDate.IsZero() && !toDate.IsZero() && fromDate.After(toDate) {
		return time.Time{}, time.Time{}, ErrInvalidDateRange
	}

	return fromDate, toDate, nil
}
