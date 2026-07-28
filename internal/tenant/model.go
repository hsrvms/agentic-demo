package tenant

import (
	"context"

	"github.com/agentic-demo/platform/internal/domain"
	"github.com/google/uuid"
)

// TenantService is the module interface.
type TenantService interface {
	Create(ctx context.Context, ownerID uuid.UUID, name string) (domain.Tenant, error)
	AddMember(ctx context.Context, tenantID domain.TenantID, userID uuid.UUID, role domain.Role) (domain.TenantMembership, error)
	ListByUser(ctx context.Context, userID uuid.UUID) ([]domain.Tenant, error)
	GetByID(ctx context.Context, tenantID domain.TenantID) (domain.Tenant, error)
	IsMember(ctx context.Context, tenantID domain.TenantID, userID uuid.UUID) (bool, error)
}
