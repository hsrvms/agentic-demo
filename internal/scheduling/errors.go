package scheduling

import "errors"

var (
	ErrScheduleNotFound      = errors.New("schedule not found")
	ErrScheduleAlreadyExists = errors.New("schedule already exists for this tenant and type")
	ErrInvalidCronExpr       = errors.New("invalid cron expression")
	ErrInvalidScheduleType   = errors.New("invalid schedule type: must be 'daily', 'weekly', or 'monthly'")
	ErrInvalidTenantID       = errors.New("tenant_id must not be empty")
)