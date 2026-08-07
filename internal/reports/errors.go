package reports

import "errors"

var (
	ErrReportNotFound    = errors.New("report not found")
	ErrInvalidTenantID   = errors.New("tenant_id must not be empty")
	ErrInvalidReportID   = errors.New("report ID is invalid")
	ErrInvalidTaskID     = errors.New("task_id must not be empty")
	ErrInvalidReportType = errors.New("report_type must not be empty")
)
