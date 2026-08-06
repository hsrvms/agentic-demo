package sources

import (
	"context"

	"github.com/google/uuid"
)

// --- Result types ---

// ListResult holds the result of a list operation.
type ListResult struct {
	Sources    []DataSource
	TotalCount int
	Page       int
	PageSize   int
}

// GetResult holds a single data source with credentials.
type GetResult struct {
	DataSource DataSource
}

// CreateResult holds a newly created data source.
type CreateResult struct {
	DataSource DataSource
}

// UpdateResult holds an updated data source.
type UpdateResult struct {
	DataSource DataSource
}

// DeleteResult is empty — success is indicated by a nil error.
type DeleteResult struct{}

// TestConnectionResult holds the outcome of a connection test.
type TestConnectionResult struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Latency string `json:"latency,omitempty"`
}

// SyncResult is empty — success is indicated by a nil error.
type SyncResult struct{}

// --- HandlerCore ---

// HandlerCore holds transport-agnostic handler logic for the sources domain.
// It calls the Service interface and returns result structs. No knowledge of
// HTTP, templ, or serialization.
type HandlerCore struct {
	service Service
}

// NewHandlerCore creates a HandlerCore.
func NewHandlerCore(service Service) *HandlerCore {
	return &HandlerCore{service: service}
}

// List fetches a paginated list of data sources for a tenant.
func (c *HandlerCore) List(ctx context.Context, tenantID string, page, pageSize int) (ListResult, error) {
	result, err := c.service.ListByTenant(ctx, tenantID, page, pageSize)
	if err != nil {
		return ListResult{}, err
	}
	return ListResult(result), nil
}

// Get fetches a single data source by ID.
func (c *HandlerCore) Get(ctx context.Context, id uuid.UUID) (GetResult, error) {
	ds, err := c.service.GetByID(ctx, id)
	if err != nil {
		return GetResult{}, err
	}
	return GetResult{DataSource: ds}, nil
}

// Create creates a new data source.
func (c *HandlerCore) Create(ctx context.Context, tenantID string, params *CreateDataSourceParams) (CreateResult, error) {
	ds, err := c.service.Create(ctx, params)
	if err != nil {
		return CreateResult{}, err
	}
	return CreateResult{DataSource: ds}, nil
}

// Update modifies an existing data source.
func (c *HandlerCore) Update(ctx context.Context, id uuid.UUID, params UpdateDataSourceParams) (UpdateResult, error) {
	ds, err := c.service.Update(ctx, id, params)
	if err != nil {
		return UpdateResult{}, err
	}
	return UpdateResult{DataSource: ds}, nil
}

// Delete removes a data source.
func (c *HandlerCore) Delete(ctx context.Context, id uuid.UUID) (DeleteResult, error) {
	if err := c.service.Delete(ctx, id); err != nil {
		return DeleteResult{}, err
	}
	return DeleteResult{}, nil
}

// TestConnection tests connectivity to a data source.
func (c *HandlerCore) TestConnection(ctx context.Context, id uuid.UUID) (TestConnectionResult, error) {
	result, err := c.service.TestConnection(ctx, id)
	if err != nil {
		return TestConnectionResult{}, err
	}
	return TestConnectionResult(result), nil
}

// Sync triggers a manual sync for a data source.
func (c *HandlerCore) Sync(ctx context.Context, id uuid.UUID) (SyncResult, error) {
	if err := c.service.Sync(ctx, id); err != nil {
		return SyncResult{}, err
	}
	return SyncResult{}, nil
}
