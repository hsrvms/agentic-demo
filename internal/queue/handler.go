package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/agentic-demo/platform/internal/delivery"
	"github.com/agentic-demo/platform/internal/domain"
	"github.com/agentic-demo/platform/internal/ingestion"
	"github.com/agentic-demo/platform/internal/reports"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
)

// HandlerDeps bundles the domain workers needed by queue handlers.
type HandlerDeps struct {
	IngestWorker    *ingestion.IngestWorker
	ReportWorker    *reports.ReportWorker
	ReportService   reports.ReportService
	DeliveryService delivery.DeliveryService
	Queue           JobQueue
	RateLimiter     TenantRateLimiter
	Logger          *slog.Logger
}

// RegisterHandlers registers all task handlers on the given mux.
func RegisterHandlers(mux *asynq.ServeMux, deps HandlerDeps) {
	ingest := &IngestHandler{worker: deps.IngestWorker, rateLimiter: deps.RateLimiter}
	report := &ReportHandler{
		worker:        deps.ReportWorker,
		reportService: deps.ReportService,
		queue:         deps.Queue,
		rateLimiter:   deps.RateLimiter,
		logger:        deps.Logger,
	}
	deliveryHandler := &DeliveryHandler{
		deliveryService: deps.DeliveryService,
		reportService:   deps.ReportService,
		logger:          deps.Logger,
	}

	mux.Handle(TypeIngestionScheduled, ingest)
	mux.Handle(TypeIngestionManual, ingest)
	mux.Handle(TypeIngestionFileUpload, ingest)
	mux.Handle(TypeReportDaily, report)
	mux.Handle(TypeReportWeekly, report)
	mux.Handle(TypeReportMonthly, report)
	mux.Handle(TypeReportOnDemand, report)
	mux.Handle(TypeDeliveryEmail, deliveryHandler)
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
// After generation, it persists the report and enqueues email delivery.
type ReportHandler struct {
	worker        *reports.ReportWorker
	reportService reports.ReportService
	queue         JobQueue
	rateLimiter   TenantRateLimiter
	logger        *slog.Logger
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

	generated, err := h.worker.GenerateReport(ctx, domain.TenantID(payload.TenantID), config)
	if err != nil {
		return fmt.Errorf("generate report for %s: %w", payload.TenantID, err)
	}

	// Persist the report if the service is available.
	if h.reportService != nil {
		var scheduleID uuid.UUID
		if payload.ScheduleID != "" {
			if parsed, err := uuid.Parse(payload.ScheduleID); err == nil {
				scheduleID = parsed
			}
		}

		focus := strings.Join(payload.FocusAreas, ", ")

		stored, err := h.reportService.Create(ctx, &reports.CreateReportParams{
			TenantID:    payload.TenantID,
			Type:        payload.ReportType,
			Title:       reportTitle(payload.ReportType, generated.Date),
			Content:     generated.Content,
			Focus:       focus,
			ScheduleID:  scheduleID,
			GeneratedAt: generated.Date,
		})
		if err != nil {
			return fmt.Errorf("persist report for %s: %w", payload.TenantID, err)
		}

		// Enqueue delivery if a queue is available.
		if h.queue != nil {
			deliveryPayload := DeliveryPayload{
				TenantID: payload.TenantID,
				ReportID: stored.ID.String(),
			}
			result, err := h.queue.Enqueue(ctx, Job{
				Type:    TypeDeliveryEmail,
				Queue:   QueueDelivery,
				Payload: deliveryPayload,
			})
			if err != nil {
				h.logger.Warn("failed to enqueue delivery",
					"report_id", stored.ID,
					"tenant_id", payload.TenantID,
					"error", err,
				)
			} else {
				h.logger.Info("enqueued report delivery",
					"report_id", stored.ID,
					"task_id", result.ID,
				)
			}
		}
	}

	return nil
}

// reportTitle generates a human-readable title from the report type and date.
func reportTitle(reportType string, date time.Time) string {
	label := titleCase(reportType)
	return fmt.Sprintf("%s Report — %s", label, date.Format("Jan 02, 2006"))
}

// titleCase converts snake_case or lowercase to Title Case.
func titleCase(s string) string {
	words := strings.Split(s, "_")
	for i, w := range words {
		if w != "" {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	return strings.Join(words, " ")
}

// DeliveryHandler processes email delivery tasks. It fetches the
// report from the database and sends it via the DeliveryService.
type DeliveryHandler struct {
	deliveryService delivery.DeliveryService
	reportService   reports.ReportService
	logger          *slog.Logger
}

func (h *DeliveryHandler) ProcessTask(ctx context.Context, task *asynq.Task) error {
	var payload DeliveryPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("unmarshal delivery payload: %w", err)
	}

	// If no delivery service is configured, log and succeed.
	if h.deliveryService == nil || h.reportService == nil {
		h.logger.Info("delivery: no service configured, skipping",
			"tenant_id", payload.TenantID,
			"report_id", payload.ReportID,
		)
		return nil
	}

	reportID, err := uuid.Parse(payload.ReportID)
	if err != nil {
		return fmt.Errorf("invalid report ID %q: %w", payload.ReportID, err)
	}

	report, err := h.reportService.GetByID(ctx, reportID)
	if err != nil {
		return fmt.Errorf("fetch report %s: %w", payload.ReportID, err)
	}

	subject := fmt.Sprintf("Your %s Report is ready: %s", titleCase(report.Type), report.Title)

	recipients := []string{payload.RecipientEmail}
	if payload.RecipientEmail == "" {
		h.logger.Info("delivery: no recipient email, skipping send",
			"tenant_id", payload.TenantID,
			"report_id", payload.ReportID,
		)
		return nil
	}

	if err := h.deliveryService.Deliver(ctx, delivery.DeliverParams{
		To:      recipients,
		Subject: subject,
		Body:    report.Content,
	}); err != nil {
		return fmt.Errorf("deliver report %s: %w", payload.ReportID, err)
	}

	return nil
}
