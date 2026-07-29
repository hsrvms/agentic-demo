package queue

import (
	"encoding/json"
	"fmt"

	"github.com/hibiken/asynq"
)

// Task type constants identify job categories in the queue.
const (
	TypeIngestionScheduled   = "ingestion:scheduled"
	TypeIngestionManual      = "ingestion:manual"
	TypeIngestionFileUpload  = "ingestion:file_upload"
	TypeReportDaily          = "report:daily"
	TypeReportWeekly         = "report:weekly"
	TypeReportMonthly        = "report:monthly"
	TypeReportOnDemand       = "report:on_demand"
	TypeDeliveryEmail        = "delivery:email"
)

// Queue names for routing tasks to workers with different priorities.
const (
	QueueIngestion = "ingestion"
	QueueReport    = "report"
	QueueDelivery  = "delivery"
)

// IngestionPayload carries data for ingestion tasks.
type IngestionPayload struct {
	TenantID string `json:"tenant_id"`
	SourceID string `json:"source_id"`
}

// ReportPayload carries data for report generation tasks.
type ReportPayload struct {
	TenantID       string   `json:"tenant_id"`
	ReportType     string   `json:"report_type"`
	FocusAreas     []string `json:"focus_areas,omitempty"`
	DeliveryMethod string   `json:"delivery_method,omitempty"`
	ScheduleID     string   `json:"schedule_id,omitempty"`
}

// DeliveryPayload carries data for delivery tasks.
type DeliveryPayload struct {
	TenantID       string `json:"tenant_id"`
	ReportID       string `json:"report_id"`
	RecipientEmail string `json:"recipient_email,omitempty"`
}

// NewIngestionTask creates an asynq task for ingestion with validated payload.
func NewIngestionTask(payload IngestionPayload) (*asynq.Task, error) {
	if payload.TenantID == "" {
		return nil, fmt.Errorf("ingestion payload: tenant_id is required")
	}
	if payload.SourceID == "" {
		return nil, fmt.Errorf("ingestion payload: source_id is required")
	}
	return newTask(TypeIngestionManual, payload, QueueIngestion)
}

// NewReportTask creates an asynq task for report generation with validated payload.
func NewReportTask(payload *ReportPayload) (*asynq.Task, error) {
	if payload.TenantID == "" {
		return nil, fmt.Errorf("report payload: tenant_id is required")
	}
	if payload.ReportType == "" {
		return nil, fmt.Errorf("report payload: report_type is required")
	}
	taskType, err := reportTypeToTaskType(payload.ReportType)
	if err != nil {
		return nil, err
	}
	return newTask(taskType, payload, QueueReport)
}

// NewDeliveryTask creates an asynq task for email delivery with validated payload.
func NewDeliveryTask(payload DeliveryPayload) (*asynq.Task, error) {
	if payload.TenantID == "" {
		return nil, fmt.Errorf("delivery payload: tenant_id is required")
	}
	if payload.ReportID == "" {
		return nil, fmt.Errorf("delivery payload: report_id is required")
	}
	return newTask(TypeDeliveryEmail, payload, QueueDelivery)
}

// reportTypeToTaskType maps a report type string to the corresponding task type.
func reportTypeToTaskType(reportType string) (string, error) {
	switch reportType {
	case "daily":
		return TypeReportDaily, nil
	case "weekly":
		return TypeReportWeekly, nil
	case "monthly":
		return TypeReportMonthly, nil
	case "on_demand":
		return TypeReportOnDemand, nil
	default:
		return "", fmt.Errorf("invalid report type: %q", reportType)
	}
}

// newTask JSON-encodes the payload and creates an asynq task.
func newTask(taskType string, payload any, queue string) (*asynq.Task, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal payload: %w", err)
	}
	return asynq.NewTask(taskType, data, asynq.Queue(queue)), nil
}
