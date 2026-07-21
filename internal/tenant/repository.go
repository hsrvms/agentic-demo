package tenant

import (
	"context"
	"errors"
	"fmt"

	"github.com/agentic-demo/platform/internal/db"
	"github.com/agentic-demo/platform/internal/domain"
	"github.com/google/uuid"
)

// Repository abstracts tenant and membership data access.
type Repository interface {
	CreateTenant(ctx context.Context, id, name string) (domain.Tenant, error)
	GetTenantByID(ctx context.Context, id domain.TenantID) (domain.Tenant, error)
	ListTenantsByUser(ctx context.Context, userID uuid.UUID) ([]domain.Tenant, error)
	CreateMembership(ctx context.Context, userID uuid.UUID, tenantID domain.TenantID, role domain.Role) (domain.TenantMembership, error)
	GetMembership(ctx context.Context, userID uuid.UUID, tenantID domain.TenantID) (domain.TenantMembership, error)
}

// pgRepository wraps sqlc-generated queries.
type pgRepository struct {
	queries *db.Queries
}

func NewRepository(queries *db.Queries) Repository {
	return &pgRepository{queries: queries}
}

func (r *pgRepository) CreateTenant(ctx context.Context, id, name string) (domain.Tenant, error) {
	row, err := r.queries.CreateTenant(ctx, db.CreateTenantParams{
		ID:   id,
		Name: name,
	})
	if err != nil {
		return domain.Tenant{}, fmt.Errorf("create tenant: %w", err)
	}
	return toDomainTenant(&row), nil
}

func (r *pgRepository) GetTenantByID(ctx context.Context, id domain.TenantID) (domain.Tenant, error) {
	row, err := r.queries.GetTenantByID(ctx, string(id))
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return domain.Tenant{}, err
		}
		return domain.Tenant{}, ErrTenantNotFound
	}
	return toDomainTenant(&row), nil
}

func (r *pgRepository) ListTenantsByUser(ctx context.Context, userID uuid.UUID) ([]domain.Tenant, error) {
	rows, err := r.queries.ListTenantsByUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list tenants: %w", err)
	}
	tenants := make([]domain.Tenant, len(rows))
	for i := range rows {
		tenants[i] = toDomainTenant(&rows[i])
	}
	return tenants, nil
}

func (r *pgRepository) CreateMembership(ctx context.Context, userID uuid.UUID, tenantID domain.TenantID, role domain.Role) (domain.TenantMembership, error) {
	row, err := r.queries.CreateMembership(ctx, db.CreateMembershipParams{
		UserID:   userID,
		TenantID: string(tenantID),
		Role:     string(role),
	})
	if err != nil {
		return domain.TenantMembership{}, fmt.Errorf("create membership: %w", err)
	}
	return toDomainMembership(&row), nil
}

func (r *pgRepository) GetMembership(ctx context.Context, userID uuid.UUID, tenantID domain.TenantID) (domain.TenantMembership, error) {
	row, err := r.queries.GetMembership(ctx, db.GetMembershipParams{
		UserID:   userID,
		TenantID: string(tenantID),
	})
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return domain.TenantMembership{}, err
		}
		return domain.TenantMembership{}, ErrMembershipNotFound
	}
	return toDomainMembership(&row), nil
}

func toDomainTenant(row *db.Tenant) domain.Tenant {
	return domain.Tenant{
		ID:        domain.TenantID(row.ID),
		Name:      row.Name,
		Status:    domain.TenantStatus(row.Status),
		Settings:  row.Settings,
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}
}

func toDomainMembership(row *db.TenantMembership) domain.TenantMembership {
	return domain.TenantMembership{
		UserID:    row.UserID,
		TenantID:  domain.TenantID(row.TenantID),
		Role:      domain.Role(row.Role),
		CreatedAt: row.CreatedAt,
	}
}
