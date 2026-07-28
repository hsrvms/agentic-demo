package queue

import (
	"encoding/json"
	"testing"
)

func TestNewIngestionTask_EncodesPayload(t *testing.T) {
	payload := IngestionPayload{
		TenantID: "tenant-1",
		SourceID: "crm",
	}

	task, err := NewIngestionTask(payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if task.Type() != TypeIngestionManual {
		t.Fatalf("expected type %q, got %q", TypeIngestionManual, task.Type())
	}

	// Round-trip: decode payload and verify fields match.
	var decoded IngestionPayload
	if err := json.Unmarshal(task.Payload(), &decoded); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if decoded.TenantID != "tenant-1" {
		t.Fatalf("expected TenantID 'tenant-1', got %q", decoded.TenantID)
	}
	if decoded.SourceID != "crm" {
		t.Fatalf("expected SourceID 'crm', got %q", decoded.SourceID)
	}
}

func TestNewIngestionTask_MissingTenantID(t *testing.T) {
	_, err := NewIngestionTask(IngestionPayload{SourceID: "crm"})
	if err == nil {
		t.Fatal("expected error for missing TenantID, got nil")
	}
}

func TestNewIngestionTask_MissingSourceID(t *testing.T) {
	_, err := NewIngestionTask(IngestionPayload{TenantID: "tenant-1"})
	if err == nil {
		t.Fatal("expected error for missing SourceID, got nil")
	}
}

func TestNewReportTask_EncodesPayload(t *testing.T) {
	payload := ReportPayload{
		TenantID:       "tenant-2",
		ReportType:     "daily",
		FocusAreas:     []string{"revenue", "churn"},
		DeliveryMethod: "email",
	}

	task, err := NewReportTask(payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if task.Type() != TypeReportDaily {
		t.Fatalf("expected type %q, got %q", TypeReportDaily, task.Type())
	}

	var decoded ReportPayload
	if err := json.Unmarshal(task.Payload(), &decoded); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if decoded.TenantID != "tenant-2" {
		t.Fatalf("expected TenantID 'tenant-2', got %q", decoded.TenantID)
	}
	if len(decoded.FocusAreas) != 2 {
		t.Fatalf("expected 2 FocusAreas, got %d", len(decoded.FocusAreas))
	}
}

func TestNewReportTask_MissingTenantID(t *testing.T) {
	_, err := NewReportTask(ReportPayload{ReportType: "daily"})
	if err == nil {
		t.Fatal("expected error for missing TenantID, got nil")
	}
}

func TestNewReportTask_MissingReportType(t *testing.T) {
	_, err := NewReportTask(ReportPayload{TenantID: "tenant-1"})
	if err == nil {
		t.Fatal("expected error for missing ReportType, got nil")
	}
}

func TestNewReportTask_InvalidReportType(t *testing.T) {
	_, err := NewReportTask(ReportPayload{TenantID: "tenant-1", ReportType: "bogus"})
	if err == nil {
		t.Fatal("expected error for invalid ReportType, got nil")
	}
}

func TestNewDeliveryTask_EncodesPayload(t *testing.T) {
	payload := DeliveryPayload{
		TenantID: "tenant-3",
		ReportID: "report-42",
	}

	task, err := NewDeliveryTask(payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if task.Type() != TypeDeliveryEmail {
		t.Fatalf("expected type %q, got %q", TypeDeliveryEmail, task.Type())
	}

	var decoded DeliveryPayload
	if err := json.Unmarshal(task.Payload(), &decoded); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if decoded.ReportID != "report-42" {
		t.Fatalf("expected ReportID 'report-42', got %q", decoded.ReportID)
	}
}

func TestNewDeliveryTask_MissingReportID(t *testing.T) {
	_, err := NewDeliveryTask(DeliveryPayload{TenantID: "tenant-1"})
	if err == nil {
		t.Fatal("expected error for missing ReportID, got nil")
	}
}

func TestReportTypeToTaskType(t *testing.T) {
	cases := []struct {
		reportType string
		expected   string
	}{
		{"daily", TypeReportDaily},
		{"weekly", TypeReportWeekly},
		{"monthly", TypeReportMonthly},
		{"on_demand", TypeReportOnDemand},
	}
	for _, tc := range cases {
		got, err := reportTypeToTaskType(tc.reportType)
		if err != nil {
			t.Fatalf("reportTypeToTaskType(%q): unexpected error: %v", tc.reportType, err)
		}
		if got != tc.expected {
			t.Fatalf("reportTypeToTaskType(%q) = %q, want %q", tc.reportType, got, tc.expected)
		}
	}
}
