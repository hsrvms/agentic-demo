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
	// IsAdmin reports whether the user is an admin of the tenant. It is false
	// for a non-member and for a viewer.
	IsAdmin(ctx context.Context, tenantID domain.TenantID, userID uuid.UUID) (bool, error)
	// Delete removes a tenant and all of its data: its object-store objects,
	// its knowledge-base chunks and documents, and finally marks the tenant
	// deleted. After it returns, no fragment of the tenant remains in the
	// object store or the vector index.
	Delete(ctx context.Context, tenantID domain.TenantID) error
}

// KnowledgeStore is the narrow knowledge-base teardown seam the tenant module
// consumes. It is satisfied by knowledge.KnowledgeStore but is defined here so
// the tenant module depends only on the teardown it needs.
type KnowledgeStore interface {
	DeleteTenantData(ctx context.Context, tenantID domain.TenantID) error
}

// ObjectStore is the narrow object-store teardown seam the tenant module
// consumes. It is satisfied by storage.ObjectStore but is defined here so the
// tenant module depends only on the bulk teardown it needs.
type ObjectStore interface {
	DeleteTenant(ctx context.Context, tenantID domain.TenantID) error
}
