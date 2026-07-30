package sources

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/agentic-demo/platform/internal/db"
	"github.com/agentic-demo/platform/internal/queue"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestService_Create(t *testing.T) {
	repo := newMockRepo()
	crypt := &mockCrypt{}
	tester := &mockTester{}
	jobQueue := &mockJobQueue{}

	svc := NewService(repo, crypt, tester, jobQueue)
	ctx := context.Background()

	cfg := json.RawMessage(`{"url": "https://example.com"}`)
	ds, err := svc.Create(ctx, &CreateDataSourceParams{
		TenantID:    "tenant-1",
		SourceType:  SourceTypeWebsite,
		Name:        "My Website",
		Config:      cfg,
		Credentials: []byte(`{"api_key": "secret"}`),
	})
	require.NoError(t, err)

	assert.Equal(t, "tenant-1", ds.TenantID)
	assert.Equal(t, SourceTypeWebsite, ds.SourceType)
	assert.Equal(t, "My Website", ds.Name)
	assert.Equal(t, StatusInactive, ds.Status)
	assert.NotEmpty(t, ds.Credentials) // encrypted
	assert.True(t, crypt.encryptCalled)
}

func TestService_Create_Validation(t *testing.T) {
	svc := NewService(newMockRepo(), &mockCrypt{}, &mockTester{}, &mockJobQueue{})
	ctx := context.Background()

	tests := []struct {
		name    string
		params  CreateDataSourceParams
		wantErr error
	}{
		{"empty tenant", CreateDataSourceParams{Name: "X", SourceType: SourceTypeWebsite}, ErrInvalidTenantID},
		{"empty name", CreateDataSourceParams{TenantID: "t1", SourceType: SourceTypeWebsite}, ErrInvalidName},
		{"invalid type", CreateDataSourceParams{TenantID: "t1", Name: "X", SourceType: "bad"}, ErrInvalidSourceType},
		{"invalid config", CreateDataSourceParams{TenantID: "t1", Name: "X", SourceType: SourceTypeWebsite, Config: json.RawMessage(`not json`)}, ErrInvalidConfig},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.Create(ctx, &tt.params)
			require.ErrorIs(t, err, tt.wantErr)
		})
	}
}

func TestService_GetByID(t *testing.T) {
	repo := newMockRepo()
	crypt := &mockCrypt{}
	svc := NewService(repo, crypt, &mockTester{}, &mockJobQueue{})
	ctx := context.Background()

	// Create a source first.
	_, err := svc.Create(ctx, &CreateDataSourceParams{
		TenantID:    "tenant-1",
		SourceType:  SourceTypeWebsite,
		Name:        "Test",
		Config:      json.RawMessage(`{}`),
		Credentials: []byte(`secret`),
	})
	require.NoError(t, err)

	ds, err := svc.GetByID(ctx, uuid.Nil)
	require.NoError(t, err)
	assert.Equal(t, "Test", ds.Name)
	assert.True(t, crypt.decryptCalled)
}

func TestService_ListByTenant(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo, &mockCrypt{}, &mockTester{}, &mockJobQueue{})
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		_, err := svc.Create(ctx, &CreateDataSourceParams{
			TenantID:   "tenant-1",
			SourceType: SourceTypeWebsite,
			Name:       "Source",
			Config:     json.RawMessage(`{}`),
		})
		require.NoError(t, err)
	}

	page, err := svc.ListByTenant(ctx, "tenant-1", 1, 10)
	require.NoError(t, err)
	assert.Len(t, page.Sources, 3)
	assert.Equal(t, 3, page.TotalCount)
	// Credentials should be nil in list.
	for _, s := range page.Sources {
		assert.Nil(t, s.Credentials)
	}
}

func TestService_ListByTenant_EmptyTenant(t *testing.T) {
	svc := NewService(newMockRepo(), &mockCrypt{}, &mockTester{}, &mockJobQueue{})
	ctx := context.Background()

	_, err := svc.ListByTenant(ctx, "", 1, 10)
	require.ErrorIs(t, err, ErrInvalidTenantID)
}

func TestService_Update(t *testing.T) {
	repo := newMockRepo()
	crypt := &mockCrypt{}
	svc := NewService(repo, crypt, &mockTester{}, &mockJobQueue{})
	ctx := context.Background()

	_, err := svc.Create(ctx, &CreateDataSourceParams{
		TenantID:   "tenant-1",
		SourceType: SourceTypeWebsite,
		Name:       "Original",
		Config:     json.RawMessage(`{}`),
	})
	require.NoError(t, err)

	newName := "Updated"
	newStatus := StatusActive
	ds, err := svc.Update(ctx, uuid.Nil, UpdateDataSourceParams{
		Name:   &newName,
		Status: &newStatus,
	})
	require.NoError(t, err)
	assert.Equal(t, "Updated", ds.Name)
	assert.Equal(t, StatusActive, ds.Status)
}

func TestService_Update_EmptyName(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo, &mockCrypt{}, &mockTester{}, &mockJobQueue{})
	ctx := context.Background()

	_, err := svc.Create(ctx, &CreateDataSourceParams{
		TenantID:   "tenant-1",
		SourceType: SourceTypeWebsite,
		Name:       "Original",
		Config:     json.RawMessage(`{}`),
	})
	require.NoError(t, err)

	emptyName := ""
	_, err = svc.Update(ctx, uuid.Nil, UpdateDataSourceParams{
		Name: &emptyName,
	})
	require.ErrorIs(t, err, ErrInvalidName)
}

func TestService_Delete(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo, &mockCrypt{}, &mockTester{}, &mockJobQueue{})
	ctx := context.Background()

	_, err := svc.Create(ctx, &CreateDataSourceParams{
		TenantID:   "tenant-1",
		SourceType: SourceTypeWebsite,
		Name:       "To Delete",
		Config:     json.RawMessage(`{}`),
	})
	require.NoError(t, err)

	err = svc.Delete(ctx, uuid.Nil)
	require.NoError(t, err)
}

func TestService_TestConnection(t *testing.T) {
	repo := newMockRepo()
	tester := &mockTester{result: ConnectionTestResult{Success: true, Message: "ok"}}
	svc := NewService(repo, &mockCrypt{}, tester, &mockJobQueue{})
	ctx := context.Background()

	_, err := svc.Create(ctx, &CreateDataSourceParams{
		TenantID:   "tenant-1",
		SourceType: SourceTypeFileUpload,
		Name:       "Upload",
		Config:     json.RawMessage(`{}`),
	})
	require.NoError(t, err)

	result, err := svc.TestConnection(ctx, uuid.Nil)
	require.NoError(t, err)
	assert.True(t, result.Success)
	assert.True(t, tester.called)
}

func TestService_Sync(t *testing.T) {
	repo := newMockRepo()
	jobQueue := &mockJobQueue{}
	svc := NewService(repo, &mockCrypt{}, &mockTester{}, jobQueue)
	ctx := context.Background()

	_, err := svc.Create(ctx, &CreateDataSourceParams{
		TenantID:   "tenant-1",
		SourceType: SourceTypeWebsite,
		Name:       "Sync Source",
		Config:     json.RawMessage(`{}`),
	})
	require.NoError(t, err)

	err = svc.Sync(ctx, uuid.Nil)
	require.NoError(t, err)
	assert.True(t, jobQueue.enqueueCalled)
}

// --- mocks ---

type mockCrypt struct {
	encryptCalled bool
	decryptCalled bool
}

func (m *mockCrypt) Encrypt(plaintext []byte) ([]byte, error) {
	m.encryptCalled = true
	// "encrypt" by reversing (obviously not real, just for testing).
	result := make([]byte, len(plaintext))
	copy(result, plaintext)
	return result, nil
}

func (m *mockCrypt) Decrypt(ciphertext []byte) ([]byte, error) {
	m.decryptCalled = true
	result := make([]byte, len(ciphertext))
	copy(result, ciphertext)
	return result, nil
}

type mockTester struct {
	called bool
	result ConnectionTestResult
}

func (m *mockTester) TestConnection(ctx context.Context, source *DataSource) (ConnectionTestResult, error) {
	m.called = true
	if m.result.Message == "" {
		return ConnectionTestResult{Success: true, Message: "mock ok"}, nil
	}
	return m.result, nil
}

type mockJobQueue struct {
	enqueueCalled bool
}

func (m *mockJobQueue) Enqueue(ctx context.Context, job queue.Job) (*queue.JobResult, error) {
	m.enqueueCalled = true
	return &queue.JobResult{ID: "job-1", Queue: job.Queue}, nil
}

func (m *mockJobQueue) EnqueueAt(ctx context.Context, job queue.Job, processAt time.Time) (*queue.JobResult, error) {
	return &queue.JobResult{ID: "job-1", Queue: job.Queue}, nil
}

func (m *mockJobQueue) Close() error {
	return nil
}

// mockRepo is a simplified in-memory repository for service tests.
type mockRepo struct {
	sources []db.DataSourceConfig
}

func newMockRepo() *mockRepo {
	return &mockRepo{}
}

func (m *mockRepo) Create(ctx context.Context, params *db.CreateDataSourceParams) (db.DataSourceConfig, error) {
	now := time.Now()
	row := db.DataSourceConfig{
		ID:             uuid.Nil,
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

func (m *mockRepo) GetByID(ctx context.Context, id uuid.UUID) (db.DataSourceConfig, error) {
	if len(m.sources) == 0 {
		return db.DataSourceConfig{}, ErrNotFound
	}
	return m.sources[0], nil
}

func (m *mockRepo) ListByTenant(ctx context.Context, params *db.ListDataSourcesByTenantParams) ([]db.DataSourceConfig, error) {
	var result []db.DataSourceConfig
	for i := range m.sources {
		s := &m.sources[i]
		if s.TenantID == params.TenantID {
			result = append(result, *s)
		}
	}
	return result, nil
}

func (m *mockRepo) CountByTenant(ctx context.Context, tenantID string) (int32, error) {
	var count int32
	for i := range m.sources {
		if m.sources[i].TenantID == tenantID {
			count++
		}
	}
	return count, nil
}

func (m *mockRepo) Update(ctx context.Context, params *db.UpdateDataSourceParams) (db.DataSourceConfig, error) {
	if len(m.sources) == 0 {
		return db.DataSourceConfig{}, ErrNotFound
	}
	s := m.sources[0]
	s.Name = params.Name
	s.Config = params.Config
	s.Credentials = params.Credentials
	s.Status = params.Status
	s.UpdatedAt = time.Now()
	m.sources[0] = s
	return s, nil
}

func (m *mockRepo) Delete(ctx context.Context, id uuid.UUID) error {
	m.sources = nil
	return nil
}

func (m *mockRepo) UpdateSyncStatus(ctx context.Context, params *db.UpdateDataSourceSyncStatusParams) (db.DataSourceConfig, error) {
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