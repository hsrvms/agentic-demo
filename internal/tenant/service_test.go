package tenant

import (
	"context"
	"errors"
	"testing"

	"github.com/agentic-demo/platform/internal/domain"
	"github.com/google/uuid"
)

// mockRepository implements Repository for unit tests.
type mockRepository struct {
	tenants     map[string]domain.Tenant
	memberships []domain.TenantMembership
	createErr   error
	findErr     error
}

func newMockRepo() *mockRepository {
	return &mockRepository{tenants: make(map[string]domain.Tenant)}
}

func (m *mockRepository) CreateTenant(_ context.Context, id, name string) (domain.Tenant, error) {
	if m.createErr != nil {
		return domain.Tenant{}, m.createErr
	}
	t := domain.Tenant{
		ID:     domain.TenantID(id),
		Name:   name,
		Status: domain.TenantActive,
	}
	m.tenants[id] = t
	return t, nil
}

func (m *mockRepository) GetTenantByID(_ context.Context, id domain.TenantID) (domain.Tenant, error) {
	if m.findErr != nil {
		return domain.Tenant{}, m.findErr
	}
	t, ok := m.tenants[string(id)]
	if !ok {
		return domain.Tenant{}, ErrTenantNotFound
	}
	return t, nil
}

func (m *mockRepository) ListTenantsByUser(_ context.Context, userID uuid.UUID) ([]domain.Tenant, error) {
	var result []domain.Tenant
	for _, mem := range m.memberships {
		if mem.UserID == userID {
			if t, ok := m.tenants[string(mem.TenantID)]; ok {
				result = append(result, t)
			}
		}
	}
	return result, nil
}

func (m *mockRepository) CreateMembership(_ context.Context, userID uuid.UUID, tenantID domain.TenantID, role domain.Role) (domain.TenantMembership, error) {
	if m.createErr != nil {
		return domain.TenantMembership{}, m.createErr
	}
	mem := domain.TenantMembership{
		UserID:   userID,
		TenantID: tenantID,
		Role:     role,
	}
	m.memberships = append(m.memberships, mem)
	return mem, nil
}

func (m *mockRepository) GetMembership(_ context.Context, userID uuid.UUID, tenantID domain.TenantID) (domain.TenantMembership, error) {
	if m.findErr != nil {
		return domain.TenantMembership{}, m.findErr
	}
	for _, mem := range m.memberships {
		if mem.UserID == userID && mem.TenantID == tenantID {
			return mem, nil
		}
	}
	return domain.TenantMembership{}, ErrMembershipNotFound
}

func TestCreate_Success(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo)
	ownerID := uuid.New()

	tenant, err := svc.Create(context.Background(), ownerID, "Acme Corp")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tenant.Name != "Acme Corp" {
		t.Errorf("Name = %q, want %q", tenant.Name, "Acme Corp")
	}
	if tenant.Status != domain.TenantActive {
		t.Errorf("Status = %q, want %q", tenant.Status, domain.TenantActive)
	}
	if tenant.ID == "" {
		t.Error("ID should not be empty")
	}

	// Verify auto-membership was created.
	member, err := svc.IsMember(context.Background(), tenant.ID, ownerID)
	if err != nil {
		t.Fatalf("IsMember error: %v", err)
	}
	if !member {
		t.Error("owner should be a member of the created tenant")
	}
}

func TestCreate_EmptyName(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo)

	_, err := svc.Create(context.Background(), uuid.New(), "")
	if !errors.Is(err, ErrInvalidName) {
		t.Errorf("error = %v, want %v", err, ErrInvalidName)
	}
}

func TestCreate_WhitespaceName(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo)

	_, err := svc.Create(context.Background(), uuid.New(), "   ")
	if !errors.Is(err, ErrInvalidName) {
		t.Errorf("error = %v, want %v", err, ErrInvalidName)
	}
}

func TestAddMember_Success(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo)
	ownerID := uuid.New()

	tenant, _ := svc.Create(context.Background(), ownerID, "Acme Corp")

	userID := uuid.New()
	mem, err := svc.AddMember(context.Background(), tenant.ID, userID, domain.RoleViewer)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mem.Role != domain.RoleViewer {
		t.Errorf("Role = %q, want %q", mem.Role, domain.RoleViewer)
	}
	if mem.UserID != userID {
		t.Errorf("UserID = %v, want %v", mem.UserID, userID)
	}
}

func TestAddMember_Duplicate(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo)
	ownerID := uuid.New()

	tenant, _ := svc.Create(context.Background(), ownerID, "Acme Corp")

	_, err := svc.AddMember(context.Background(), tenant.ID, ownerID, domain.RoleViewer)
	if !errors.Is(err, ErrAlreadyExists) {
		t.Errorf("error = %v, want %v", err, ErrAlreadyExists)
	}
}

func TestAddMember_InvalidRole(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo)
	ownerID := uuid.New()

	tenant, _ := svc.Create(context.Background(), ownerID, "Acme Corp")

	_, err := svc.AddMember(context.Background(), tenant.ID, uuid.New(), "superadmin")
	if !errors.Is(err, ErrInvalidRole) {
		t.Errorf("error = %v, want %v", err, ErrInvalidRole)
	}
}

func TestListByUser(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo)
	ownerID := uuid.New()

	_, _ = svc.Create(context.Background(), ownerID, "Acme Corp")
	_, _ = svc.Create(context.Background(), ownerID, "Beta Inc")

	tenants, err := svc.ListByUser(context.Background(), ownerID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tenants) != 2 {
		t.Errorf("got %d tenants, want 2", len(tenants))
	}
}

func TestListByUser_NoTenants(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo)

	tenants, err := svc.ListByUser(context.Background(), uuid.New())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tenants) != 0 {
		t.Errorf("got %d tenants, want 0", len(tenants))
	}
}

func TestGetByID(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo)
	ownerID := uuid.New()

	created, _ := svc.Create(context.Background(), ownerID, "Acme Corp")

	got, err := svc.GetByID(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Name != "Acme Corp" {
		t.Errorf("Name = %q, want %q", got.Name, "Acme Corp")
	}
}

func TestGetByID_NotFound(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo)

	_, err := svc.GetByID(context.Background(), "t_nonexistent")
	if !errors.Is(err, ErrTenantNotFound) {
		t.Errorf("error = %v, want %v", err, ErrTenantNotFound)
	}
}

func TestIsMember_True(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo)
	ownerID := uuid.New()

	tenant, _ := svc.Create(context.Background(), ownerID, "Acme Corp")

	member, err := svc.IsMember(context.Background(), tenant.ID, ownerID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !member {
		t.Error("expected IsMember = true")
	}
}

func TestIsMember_False(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo)
	ownerID := uuid.New()

	tenant, _ := svc.Create(context.Background(), ownerID, "Acme Corp")

	member, err := svc.IsMember(context.Background(), tenant.ID, uuid.New())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if member {
		t.Error("expected IsMember = false")
	}
}
