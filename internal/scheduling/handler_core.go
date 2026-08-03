package scheduling

import (
	"context"

	"github.com/google/uuid"
)

// --- Result types ---

// ListResult holds the result of listing schedules for a tenant.
type ListResult struct {
	Schedules []ReportSchedule
}

// GetResult holds a single schedule.
type GetResult struct {
	Schedule ReportSchedule
}

// CreateResult holds a newly created schedule.
type CreateResult struct {
	Schedule ReportSchedule
}

// UpdateResult holds an updated schedule.
type UpdateResult struct {
	Schedule ReportSchedule
}

// DeleteResult is empty — success is indicated by a nil error.
type DeleteResult struct{}

// ToggleResult holds a schedule after toggling its enabled state.
type ToggleResult struct {
	Schedule ReportSchedule
}

// --- HandlerCore ---

// HandlerCore holds transport-agnostic handler logic for the scheduling domain.
// It calls the ScheduleService interface and returns result structs. No knowledge of
// HTTP, templ, or serialization.
type HandlerCore struct {
	service ScheduleService
}

// NewHandlerCore creates a HandlerCore.
func NewHandlerCore(service ScheduleService) *HandlerCore {
	return &HandlerCore{service: service}
}

// List fetches all schedules for a tenant.
func (c *HandlerCore) List(ctx context.Context, tenantID string) (ListResult, error) {
	schedules, err := c.service.ListByTenant(ctx, tenantID)
	if err != nil {
		return ListResult{}, err
	}
	return ListResult{Schedules: schedules}, nil
}

// Get fetches a single schedule by ID.
func (c *HandlerCore) Get(ctx context.Context, id uuid.UUID) (GetResult, error) {
	s, err := c.service.GetByID(ctx, id)
	if err != nil {
		return GetResult{}, err
	}
	return GetResult{Schedule: s}, nil
}

// Create creates a new schedule.
func (c *HandlerCore) Create(ctx context.Context, tenantID string, params *CreateScheduleParams) (CreateResult, error) {
	s, err := c.service.Create(ctx, params)
	if err != nil {
		return CreateResult{}, err
	}
	return CreateResult{Schedule: s}, nil
}

// Update modifies an existing schedule.
func (c *HandlerCore) Update(ctx context.Context, params *UpdateScheduleParams) (UpdateResult, error) {
	s, err := c.service.Update(ctx, params)
	if err != nil {
		return UpdateResult{}, err
	}
	return UpdateResult{Schedule: s}, nil
}

// Delete removes a schedule.
func (c *HandlerCore) Delete(ctx context.Context, id uuid.UUID) (DeleteResult, error) {
	if err := c.service.Delete(ctx, id); err != nil {
		return DeleteResult{}, err
	}
	return DeleteResult{}, nil
}

// Toggle toggles the enabled state of a schedule.
func (c *HandlerCore) Toggle(ctx context.Context, id uuid.UUID) (ToggleResult, error) {
	s, err := c.service.Toggle(ctx, id)
	if err != nil {
		return ToggleResult{}, err
	}
	return ToggleResult{Schedule: s}, nil
}