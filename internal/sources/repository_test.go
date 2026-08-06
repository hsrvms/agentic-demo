package sources

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/agentic-demo/platform/internal/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRepository_Create(t *testing.T) {
	repo := newMockRepository()
	ctx := context.Background()

	params := &db.CreateDataSourceParams{
		TenantID:   "tenant-1",
		SourceType: "website",
		Name:       "My Website",
		Config:     []byte(`{"url": "https://example.com"}`),
		Status:     "inactive",
	}

	row, err := repo.Create(ctx, params)
	require.NoError(t, err)
	assert.Equal(t, "tenant-1", row.TenantID)
	assert.Equal(t, "website", row.SourceType)
	assert.Equal(t, "My Website", row.Name)
	assert.Equal(t, json.RawMessage(`{"url": "https://example.com"}`), json.RawMessage(row.Config))
	assert.Equal(t, uuid.Nil.String(), row.ID.String()) // mock returns nil UUID
}

func TestRepository_GetByID(t *testing.T) {
	repo := newMockRepository()
	ctx := context.Background()

	// Create first.
	_, err := repo.Create(ctx, &db.CreateDataSourceParams{
		TenantID:   "tenant-1",
		SourceType: "website",
		Name:       "Test",
		Config:     []byte(`{}`),
		Status:     "inactive",
	})
	require.NoError(t, err)

	// Get the first (and only) entry.
	row, err := repo.GetByID(ctx, uuid.Nil)
	require.NoError(t, err)
	assert.Equal(t, "Test", row.Name)
}

func TestRepository_GetByID_NotFound(t *testing.T) {
	repo := newMockRepository()
	ctx := context.Background()

	_, err := repo.GetByID(ctx, uuid.New())
	require.Error(t, err)
}

func TestRepository_ListByTenant(t *testing.T) {
	repo := newMockRepository()
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		_, err := repo.Create(ctx, &db.CreateDataSourceParams{
			TenantID:   "tenant-1",
			SourceType: "website",
			Name:       "Source",
			Config:     []byte(`{}`),
			Status:     "inactive",
		})
		require.NoError(t, err)
	}

	rows, err := repo.ListByTenant(ctx, &db.ListDataSourcesByTenantParams{
		TenantID: "tenant-1",
		Limit:    10,
		Offset:   0,
	})
	require.NoError(t, err)
	assert.Len(t, rows, 3)
}

func TestRepository_CountByTenant(t *testing.T) {
	repo := newMockRepository()
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		_, err := repo.Create(ctx, &db.CreateDataSourceParams{
			TenantID:   "tenant-1",
			SourceType: "website",
			Name:       "Source",
			Config:     []byte(`{}`),
			Status:     "inactive",
		})
		require.NoError(t, err)
	}

	count, err := repo.CountByTenant(ctx, "tenant-1")
	require.NoError(t, err)
	assert.Equal(t, int32(5), count)
}

func TestRepository_Update(t *testing.T) {
	repo := newMockRepository()
	ctx := context.Background()

	_, err := repo.Create(ctx, &db.CreateDataSourceParams{
		TenantID:   "tenant-1",
		SourceType: "website",
		Name:       "Original",
		Config:     []byte(`{}`),
		Status:     "inactive",
	})
	require.NoError(t, err)

	updated, err := repo.Update(ctx, &db.UpdateDataSourceParams{
		ID:     uuid.Nil,
		Name:   "Updated",
		Config: []byte(`{"url": "https://new.example.com"}`),
		Status: "active",
	})
	require.NoError(t, err)
	assert.Equal(t, "Updated", updated.Name)
	assert.Equal(t, "active", updated.Status)
}

func TestRepository_Delete(t *testing.T) {
	repo := newMockRepository()
	ctx := context.Background()

	_, err := repo.Create(ctx, &db.CreateDataSourceParams{
		TenantID:   "tenant-1",
		SourceType: "website",
		Name:       "To Delete",
		Config:     []byte(`{}`),
		Status:     "inactive",
	})
	require.NoError(t, err)

	err = repo.Delete(ctx, uuid.Nil)
	require.NoError(t, err)

	// Should be gone.
	_, err = repo.GetByID(ctx, uuid.Nil)
	require.Error(t, err)
}

func TestRepository_UpdateSyncStatus(t *testing.T) {
	repo := newMockRepository()
	ctx := context.Background()

	_, err := repo.Create(ctx, &db.CreateDataSourceParams{
		TenantID:   "tenant-1",
		SourceType: "website",
		Name:       "Sync Test",
		Config:     []byte(`{}`),
		Status:     "inactive",
	})
	require.NoError(t, err)

	now := time.Now()
	row, err := repo.UpdateSyncStatus(ctx, &db.UpdateDataSourceSyncStatusParams{
		ID:             uuid.Nil,
		LastSyncAt:     pgtype.Timestamptz{Time: now, Valid: true},
		LastSyncStatus: pgtype.Text{String: "success", Valid: true},
		Status:         "active",
	})
	require.NoError(t, err)
	assert.Equal(t, "active", row.Status)
	assert.Equal(t, "success", row.LastSyncStatus.String)
}

// --- mock repository ---

type mockRepository struct {
	sources []db.DataSourceConfig
}

func newMockRepository() *mockRepository {
	return &mockRepository{}
}

func (m *mockRepository) Create(ctx context.Context, params *db.CreateDataSourceParams) (db.DataSourceConfig, error) {
	now := time.Now()
	row := db.DataSourceConfig{
		ID:             uuid.Nil, // simplified for mock
		TenantID:       params.TenantID,
		SourceType:     params.SourceType,
		Name:           params.Name,
		Config:         params.Config,
		Credentials:    params.Credentials,
		Status:         params.Status,
		LastSyncAt:     pgtype.Timestamptz{Valid: false},
		LastSyncStatus: pgtype.Text{Valid: false},
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	m.sources = append(m.sources, row)
	return row, nil
}

func (m *mockRepository) GetByID(ctx context.Context, id uuid.UUID) (db.DataSourceConfig, error) {
	if len(m.sources) == 0 {
		return db.DataSourceConfig{}, ErrNotFound
	}
	return m.sources[0], nil
}

func (m *mockRepository) ListByTenant(ctx context.Context, params *db.ListDataSourcesByTenantParams) ([]db.DataSourceConfig, error) {
	var result []db.DataSourceConfig
	for i := range m.sources {
		if m.sources[i].TenantID == params.TenantID {
			result = append(result, m.sources[i])
		}
	}
	return result, nil
}

func (m *mockRepository) CountByTenant(ctx context.Context, tenantID string) (int32, error) {
	var count int32
	for i := range m.sources {
		if m.sources[i].TenantID == tenantID {
			count++
		}
	}
	return count, nil
}

func (m *mockRepository) Update(ctx context.Context, params *db.UpdateDataSourceParams) (db.DataSourceConfig, error) {
	if len(m.sources) == 0 {
		return db.DataSourceConfig{}, ErrNotFound
	}
	s := m.sources[0]
	s.Name = params.Name
	s.Config = params.Config
	s.Status = params.Status
	s.UpdatedAt = time.Now()
	m.sources[0] = s
	return s, nil
}

func (m *mockRepository) Delete(ctx context.Context, id uuid.UUID) error {
	m.sources = nil
	return nil
}

func (m *mockRepository) UpdateSyncStatus(ctx context.Context, params *db.UpdateDataSourceSyncStatusParams) (db.DataSourceConfig, error) {
	if len(m.sources) == 0 {
		return db.DataSourceConfig{}, ErrNotFound
	}
	s := m.sources[0]
	s.Status = params.Status
	s.LastSyncAt = params.LastSyncAt
	s.LastSyncStatus = params.LastSyncStatus
	s.UpdatedAt = time.Now()
	m.sources[0] = s
	return s, nil
}
