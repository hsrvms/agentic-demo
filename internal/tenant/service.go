package tenant

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/agentic-demo/platform/internal/domain"
	"github.com/google/uuid"
)

type tenantService struct {
	repo      Repository
	knowledge KnowledgeStore
	objects   ObjectStore
}

// NewService creates a TenantService. knowledge and objects are the teardown
// seams the tenant module uses to cascade deletion: the knowledge base (chunks
// and documents) and the object store (uploaded file bytes).
func NewService(repo Repository, knowledge KnowledgeStore, objects ObjectStore) TenantService {
	return &tenantService{repo: repo, knowledge: knowledge, objects: objects}
}

func (s *tenantService) Create(ctx context.Context, ownerID uuid.UUID, name string) (domain.Tenant, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return domain.Tenant{}, ErrInvalidName
	}

	// Generate a tenant ID (prefixed for readability).
	tenantID := "t_" + uuid.New().String()[:8]

	tenant, err := s.repo.CreateTenant(ctx, tenantID, name)
	if err != nil {
		return domain.Tenant{}, err
	}

	// Auto-add the creator as admin.
	_, err = s.repo.CreateMembership(ctx, ownerID, domain.TenantID(tenantID), domain.RoleAdmin)
	if err != nil {
		return domain.Tenant{}, err
	}

	return tenant, nil
}

func (s *tenantService) AddMember(ctx context.Context, tenantID domain.TenantID, userID uuid.UUID, role domain.Role) (domain.TenantMembership, error) {
	if role != domain.RoleAdmin && role != domain.RoleViewer {
		return domain.TenantMembership{}, ErrInvalidRole
	}

	// Verify tenant exists.
	if _, err := s.repo.GetTenantByID(ctx, tenantID); err != nil {
		if errors.Is(err, ErrTenantNotFound) {
			return domain.TenantMembership{}, err
		}
		return domain.TenantMembership{}, err
	}

	// Check for existing membership.
	_, err := s.repo.GetMembership(ctx, userID, tenantID)
	if err == nil {
		return domain.TenantMembership{}, ErrAlreadyExists
	}
	if !errors.Is(err, ErrMembershipNotFound) {
		return domain.TenantMembership{}, err
	}

	return s.repo.CreateMembership(ctx, userID, tenantID, role)
}

func (s *tenantService) ListByUser(ctx context.Context, userID uuid.UUID) ([]domain.Tenant, error) {
	return s.repo.ListTenantsByUser(ctx, userID)
}

func (s *tenantService) GetByID(ctx context.Context, tenantID domain.TenantID) (domain.Tenant, error) {
	return s.repo.GetTenantByID(ctx, tenantID)
}

func (s *tenantService) IsMember(ctx context.Context, tenantID domain.TenantID, userID uuid.UUID) (bool, error) {
	_, err := s.repo.GetMembership(ctx, userID, tenantID)
	if err != nil {
		if errors.Is(err, ErrMembershipNotFound) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// IsAdmin reports whether the user holds the admin role in the tenant. A
// non-member and a viewer both return false, nil.
func (s *tenantService) IsAdmin(ctx context.Context, tenantID domain.TenantID, userID uuid.UUID) (bool, error) {
	mem, err := s.repo.GetMembership(ctx, userID, tenantID)
	if err != nil {
		if errors.Is(err, ErrMembershipNotFound) {
			return false, nil
		}
		return false, err
	}
	return mem.Role == domain.RoleAdmin, nil
}

// Delete cascades a tenant's teardown across its data stores. It verifies the
// tenant exists, then removes its object-store objects, its knowledge-base
// chunks and documents, and finally marks the tenant deleted.
//
// Data is removed before the tenant is marked deleted so a mid-cascade failure
// leaves the tenant active (and retryable) rather than a deleted tenant that
// still holds fragments. Success means no fragment remains in the vector index
// or the object store.
func (s *tenantService) Delete(ctx context.Context, tenantID domain.TenantID) error {
	if _, err := s.repo.GetTenantByID(ctx, tenantID); err != nil {
		return err
	}

	if err := s.objects.DeleteTenant(ctx, tenantID); err != nil {
		return fmt.Errorf("delete tenant object store: %w", err)
	}
	if err := s.knowledge.DeleteTenantData(ctx, tenantID); err != nil {
		return fmt.Errorf("delete tenant knowledge base: %w", err)
	}
	if err := s.repo.DeleteTenant(ctx, tenantID); err != nil {
		return fmt.Errorf("mark tenant deleted: %w", err)
	}
	return nil
}
