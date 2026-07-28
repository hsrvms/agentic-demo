package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/agentic-demo/platform/internal/domain"
	"github.com/agentic-demo/platform/internal/ingestion"
	"github.com/agentic-demo/platform/internal/reports"
	"github.com/hibiken/asynq"
)

// HandlerDeps bundles the domain workers needed by queue handlers.
type HandlerDeps struct {
	IngestWorker *ingestion.IngestWorker
	ReportWorker *reports.ReportWorker
	RateLimiter  TenantRateLimiter
	Logger       *slog.Logger
}

// RegisterHandlers registers all task handlers on the given mux.
func RegisterHandlers(mux *asynq.ServeMux, deps HandlerDeps) {
	ingest := &IngestHandler{worker: deps.IngestWorker, rateLimiter: deps.RateLimiter}
	report := &ReportHandler{worker: deps.ReportWorker, rateLimiter: deps.RateLimiter}
	delivery := &DeliveryHandler{logger: deps.Logger}

	mux.Handle(TypeIngestionScheduled, ingest)
	mux.Handle(TypeIngestionManual, ingest)
	mux.Handle(TypeIngestionFileUpload, ingest)
	mux.Handle(TypeReportDaily, report)
	mux.Handle(TypeReportWeekly, report)
	mux.Handle(TypeReportMonthly, report)
	mux.Handle(TypeReportOnDemand, report)
	mux.Handle(TypeDeliveryEmail, delivery)
}

// IngestHandler processes ingestion tasks by delegating to IngestWorker.
type IngestHandler struct {
	worker      *ingestion.IngestWorker
	rateLimiter TenantRateLimiter
}

func (h *IngestHandler) ProcessTask(ctx context.Context, task *asynq.Task) error {
	var payload IngestionPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("unmarshal ingestion payload: %w", err)
	}

	if h.rateLimiter != nil {
		if err := h.rateLimiter.Acquire(ctx, payload.TenantID); err != nil {
			return err
		}
		defer h.rateLimiter.Release(ctx, payload.TenantID)
	}

	_, err := h.worker.Ingest(ctx, domain.TenantID(payload.TenantID), payload.SourceID)
	if err != nil {
		return fmt.Errorf("ingest %s/%s: %w", payload.TenantID, payload.SourceID, err)
	}
	return nil
}

// ReportHandler processes report tasks by delegating to ReportWorker.
type ReportHandler struct {
	worker      *reports.ReportWorker
	rateLimiter TenantRateLimiter
}

func (h *ReportHandler) ProcessTask(ctx context.Context, task *asynq.Task) error {
	var payload ReportPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("unmarshal report payload: %w", err)
	}

	if h.rateLimiter != nil {
		if err := h.rateLimiter.Acquire(ctx, payload.TenantID); err != nil {
			return err
		}
		defer h.rateLimiter.Release(ctx, payload.TenantID)
	}

	config := domain.ReportConfig{
		Type:           domain.ReportType(payload.ReportType),
		FocusAreas:     payload.FocusAreas,
		DeliveryMethod: payload.DeliveryMethod,
	}

	_, err := h.worker.GenerateReport(ctx, domain.TenantID(payload.TenantID), config)
	if err != nil {
		return fmt.Errorf("generate report for %s: %w", payload.TenantID, err)
	}
	return nil
}

// DeliveryHandler is a stub for email delivery. Full implementation
// is deferred to a later workstream.
type DeliveryHandler struct {
	logger *slog.Logger
}

func (h *DeliveryHandler) ProcessTask(ctx context.Context, task *asynq.Task) error {
	var payload DeliveryPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("unmarshal delivery payload: %w", err)
	}

	h.logger.Info("delivery stub: would send email",
		"tenant_id", payload.TenantID,
		"report_id", payload.ReportID,
	)
	return nil
}
