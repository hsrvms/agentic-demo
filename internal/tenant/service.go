package tenant

import (
	"context"
	"errors"
	"strings"

	"github.com/agentic-demo/platform/internal/domain"
	"github.com/google/uuid"
)

type tenantService struct {
	repo Repository
}

// NewService creates a TenantService.
func NewService(repo Repository) TenantService {
	return &tenantService{repo: repo}
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
