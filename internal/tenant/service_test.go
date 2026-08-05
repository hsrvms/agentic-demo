package tenant

import (
	"context"
	"errors"
	"testing"

	"github.com/agentic-demo/platform/internal/domain"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// mockRepository implements Repository for unit tests.
type mockRepository struct {
	tenants     map[string]domain.Tenant
	memberships []domain.TenantMembership
	createErr   error
	findErr     error
	deleteErr   error
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

func (m *mockRepository) DeleteTenant(ctx context.Context, tenantID domain.TenantID) error {
	if m.deleteErr != nil {
		return m.deleteErr
	}
	t, ok := m.tenants[string(tenantID)]
	if !ok {
		return ErrTenantNotFound
	}
	t.Status = domain.TenantDeleted
	m.tenants[string(tenantID)] = t
	return nil
}

func TestCreate_Success(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo, nil, nil)
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
	svc := NewService(repo, nil, nil)

	_, err := svc.Create(context.Background(), uuid.New(), "")
	if !errors.Is(err, ErrInvalidName) {
		t.Errorf("error = %v, want %v", err, ErrInvalidName)
	}
}

func TestCreate_WhitespaceName(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo, nil, nil)

	_, err := svc.Create(context.Background(), uuid.New(), "   ")
	if !errors.Is(err, ErrInvalidName) {
		t.Errorf("error = %v, want %v", err, ErrInvalidName)
	}
}

func TestAddMember_Success(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo, nil, nil)
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
	svc := NewService(repo, nil, nil)
	ownerID := uuid.New()

	tenant, _ := svc.Create(context.Background(), ownerID, "Acme Corp")

	_, err := svc.AddMember(context.Background(), tenant.ID, ownerID, domain.RoleViewer)
	if !errors.Is(err, ErrAlreadyExists) {
		t.Errorf("error = %v, want %v", err, ErrAlreadyExists)
	}
}

func TestAddMember_InvalidRole(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo, nil, nil)
	ownerID := uuid.New()

	tenant, _ := svc.Create(context.Background(), ownerID, "Acme Corp")

	_, err := svc.AddMember(context.Background(), tenant.ID, uuid.New(), "superadmin")
	if !errors.Is(err, ErrInvalidRole) {
		t.Errorf("error = %v, want %v", err, ErrInvalidRole)
	}
}

func TestListByUser(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo, nil, nil)
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
	svc := NewService(repo, nil, nil)

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
	svc := NewService(repo, nil, nil)
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
	svc := NewService(repo, nil, nil)

	_, err := svc.GetByID(context.Background(), "t_nonexistent")
	if !errors.Is(err, ErrTenantNotFound) {
		t.Errorf("error = %v, want %v", err, ErrTenantNotFound)
	}
}

func TestIsMember_True(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo, nil, nil)
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
	svc := NewService(repo, nil, nil)
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

func TestIsAdmin_OwnerIsAdmin(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo, nil, nil)
	ownerID := uuid.New()

	tenant, _ := svc.Create(context.Background(), ownerID, "Acme Corp")

	admin, err := svc.IsAdmin(context.Background(), tenant.ID, ownerID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !admin {
		t.Error("expected owner to be admin")
	}
}

func TestIsAdmin_ViewerIsNotAdmin(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo, nil, nil)
	ownerID := uuid.New()

	tenant, _ := svc.Create(context.Background(), ownerID, "Acme Corp")
	viewerID := uuid.New()
	_, err := svc.AddMember(context.Background(), tenant.ID, viewerID, domain.RoleViewer)
	require.NoError(t, err)

	admin, err := svc.IsAdmin(context.Background(), tenant.ID, viewerID)
	require.NoError(t, err)
	if admin {
		t.Error("expected viewer not to be admin")
	}
}

func TestIsAdmin_NonMemberIsNotAdmin(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo, nil, nil)
	ownerID := uuid.New()

	tenant, _ := svc.Create(context.Background(), ownerID, "Acme Corp")

	admin, err := svc.IsAdmin(context.Background(), tenant.ID, uuid.New())
	require.NoError(t, err)
	if admin {
		t.Error("expected non-member not to be admin")
	}
}

func TestDelete_CascadesKnowledgeAndObjects(t *testing.T) {
	repo := newMockRepo()
	knowledge := &mockKnowledgeStore{}
	objects := &mockObjectStore{}
	svc := NewService(repo, knowledge, objects)
	ownerID := uuid.New()

	tenant, _ := svc.Create(context.Background(), ownerID, "Acme Corp")

	require.NoError(t, svc.Delete(context.Background(), tenant.ID))

	// The tenant's knowledge base and object store must both be torn down.
	require.Len(t, knowledge.deletedTenants, 1)
	require.Equal(t, tenant.ID, knowledge.deletedTenants[0])
	require.Len(t, objects.deletedTenants, 1)
	require.Equal(t, tenant.ID, objects.deletedTenants[0])

	// The tenant must now be marked deleted.
	got, err := svc.GetByID(context.Background(), tenant.ID)
	require.NoError(t, err)
	require.Equal(t, domain.TenantDeleted, got.Status)
}

func TestDelete_NotFound(t *testing.T) {
	repo := newMockRepo()
	knowledge := &mockKnowledgeStore{}
	objects := &mockObjectStore{}
	svc := NewService(repo, knowledge, objects)

	err := svc.Delete(context.Background(), "t_nonexistent")
	require.ErrorIs(t, err, ErrTenantNotFound)

	// Nothing should have been torn down for a tenant that does not exist.
	require.Len(t, knowledge.deletedTenants, 0)
	require.Len(t, objects.deletedTenants, 0)
}

func TestDelete_ObjectStoreFailureLeavesTenantActive(t *testing.T) {
	repo := newMockRepo()
	objects := &mockObjectStore{deleteErr: errors.New("s3 unavailable")}
	knowledge := &mockKnowledgeStore{}
	svc := NewService(repo, knowledge, objects)
	ownerID := uuid.New()

	tenant, _ := svc.Create(context.Background(), ownerID, "Acme Corp")

	err := svc.Delete(context.Background(), tenant.ID)
	require.Error(t, err)

	// Data is removed before the tenant is marked deleted, so a failure leaves
	// the tenant active and retryable rather than deleted with fragments.
	got, err := svc.GetByID(context.Background(), tenant.ID)
	require.NoError(t, err)
	require.Equal(t, domain.TenantActive, got.Status)
	require.Len(t, knowledge.deletedTenants, 0)
}

func TestDelete_KnowledgeFailureLeavesTenantActive(t *testing.T) {
	repo := newMockRepo()
	objects := &mockObjectStore{}
	knowledge := &mockKnowledgeStore{deleteErr: errors.New("db unavailable")}
	svc := NewService(repo, knowledge, objects)
	ownerID := uuid.New()

	tenant, _ := svc.Create(context.Background(), ownerID, "Acme Corp")

	err := svc.Delete(context.Background(), tenant.ID)
	require.Error(t, err)

	got, err := svc.GetByID(context.Background(), tenant.ID)
	require.NoError(t, err)
	require.Equal(t, domain.TenantActive, got.Status)
}

// mockKnowledgeStore implements the KnowledgeStore teardown seam for tests.
type mockKnowledgeStore struct {
	deletedTenants []domain.TenantID
	deleteErr      error
}

func (m *mockKnowledgeStore) DeleteTenantData(_ context.Context, tenantID domain.TenantID) error {
	if m.deleteErr != nil {
		return m.deleteErr
	}
	m.deletedTenants = append(m.deletedTenants, tenantID)
	return nil
}

// mockObjectStore implements the ObjectStore teardown seam for tests.
type mockObjectStore struct {
	deletedTenants []domain.TenantID
	deleteErr      error
}

func (m *mockObjectStore) DeleteTenant(_ context.Context, tenantID domain.TenantID) error {
	if m.deleteErr != nil {
		return m.deleteErr
	}
	m.deletedTenants = append(m.deletedTenants, tenantID)
	return nil
}
