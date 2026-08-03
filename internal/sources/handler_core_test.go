package sources

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockService implements Service for testing.
type mockService struct {
	createResult DataSource
	createErr    error
	getResult    DataSource
	getErr       error
	listResult   DataSourcePage
	listErr      error
	updateResult DataSource
	updateErr    error
	deleteErr    error
	testResult   ConnectionTestResult
	testErr      error
	syncErr      error
}

func (m *mockService) Create(ctx context.Context, params *CreateDataSourceParams) (DataSource, error) {
	if m.createErr != nil {
		return DataSource{}, m.createErr
	}
	if m.createResult.ID == uuid.Nil {
		m.createResult.ID = uuid.New()
	}
	m.createResult.TenantID = params.TenantID
	m.createResult.SourceType = params.SourceType
	m.createResult.Name = params.Name
	return m.createResult, nil
}

func (m *mockService) GetByID(ctx context.Context, id uuid.UUID) (DataSource, error) {
	if m.getErr != nil {
		return DataSource{}, m.getErr
	}
	return m.getResult, nil
}

func (m *mockService) ListByTenant(ctx context.Context, tenantID string, page, pageSize int) (DataSourcePage, error) {
	if m.listErr != nil {
		return DataSourcePage{}, m.listErr
	}
	return m.listResult, nil
}

func (m *mockService) Update(ctx context.Context, id uuid.UUID, params UpdateDataSourceParams) (DataSource, error) {
	if m.updateErr != nil {
		return DataSource{}, m.updateErr
	}
	return m.updateResult, nil
}

func (m *mockService) Delete(ctx context.Context, id uuid.UUID) error {
	return m.deleteErr
}

func (m *mockService) TestConnection(ctx context.Context, id uuid.UUID) (ConnectionTestResult, error) {
	if m.testErr != nil {
		return ConnectionTestResult{}, m.testErr
	}
	return m.testResult, nil
}

func (m *mockService) Sync(ctx context.Context, id uuid.UUID) error {
	return m.syncErr
}

func TestHandlerCore_List(t *testing.T) {
	svc := &mockService{
		listResult: DataSourcePage{
			Sources: []DataSource{
				{ID: uuid.New(), Name: "Source 1", TenantID: "tenant-1"},
				{ID: uuid.New(), Name: "Source 2", TenantID: "tenant-1"},
			},
			TotalCount: 2,
			Page:       1,
			PageSize:   20,
		},
	}
	core := NewHandlerCore(svc)

	result, err := core.List(context.Background(), "tenant-1", 1, 20)
	require.NoError(t, err)
	assert.Equal(t, 2, result.TotalCount)
	assert.Len(t, result.Sources, 2)
	assert.Equal(t, "Source 1", result.Sources[0].Name)
	assert.Equal(t, 1, result.Page)
	assert.Equal(t, 20, result.PageSize)
}

func TestHandlerCore_List_DefaultsPagination(t *testing.T) {
	svc := &mockService{
		listResult: DataSourcePage{
			Sources:    []DataSource{},
			TotalCount: 0,
			Page:       1,
			PageSize:   20,
		},
	}
	core := NewHandlerCore(svc)

	result, err := core.List(context.Background(), "tenant-1", 0, 0)
	require.NoError(t, err)
	assert.Equal(t, 0, result.TotalCount)
	assert.Len(t, result.Sources, 0)
}

func TestHandlerCore_List_Error(t *testing.T) {
	svc := &mockService{
		listErr: ErrInvalidTenantID,
	}
	core := NewHandlerCore(svc)

	_, err := core.List(context.Background(), "", 1, 20)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidTenantID)
}

func TestHandlerCore_Get(t *testing.T) {
	id := uuid.New()
	svc := &mockService{
		getResult: DataSource{
			ID:         id,
			TenantID:   "tenant-1",
			SourceType: SourceTypeWebsite,
			Name:       "Test Source",
			Status:     StatusActive,
		},
	}
	core := NewHandlerCore(svc)

	result, err := core.Get(context.Background(), id)
	require.NoError(t, err)
	assert.Equal(t, "Test Source", result.DataSource.Name)
	assert.Equal(t, SourceTypeWebsite, result.DataSource.SourceType)
	assert.Equal(t, StatusActive, result.DataSource.Status)
}

func TestHandlerCore_Get_Error(t *testing.T) {
	svc := &mockService{
		getErr: ErrNotFound,
	}
	core := NewHandlerCore(svc)

	_, err := core.Get(context.Background(), uuid.New())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestHandlerCore_Create(t *testing.T) {
	svc := &mockService{
		createResult: DataSource{
			ID:         uuid.New(),
			Name:       "New Source",
			SourceType: SourceTypeWebsite,
			Status:     StatusInactive,
		},
	}
	core := NewHandlerCore(svc)

	result, err := core.Create(context.Background(), "tenant-1", &CreateDataSourceParams{
		TenantID:   "tenant-1",
		SourceType: SourceTypeWebsite,
		Name:       "New Source",
	})
	require.NoError(t, err)
	assert.Equal(t, "New Source", result.DataSource.Name)
	assert.Equal(t, SourceTypeWebsite, result.DataSource.SourceType)
}

func TestHandlerCore_Create_Error(t *testing.T) {
	svc := &mockService{
		createErr: ErrInvalidName,
	}
	core := NewHandlerCore(svc)

	_, err := core.Create(context.Background(), "tenant-1", &CreateDataSourceParams{
		Name: "",
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidName)
}

func TestHandlerCore_Update(t *testing.T) {
	id := uuid.New()
	svc := &mockService{
		updateResult: DataSource{
			ID:     id,
			Name:   "Updated Source",
			Status: StatusActive,
		},
	}
	core := NewHandlerCore(svc)

	name := "Updated Source"
	result, err := core.Update(context.Background(), id, UpdateDataSourceParams{
		Name: &name,
	})
	require.NoError(t, err)
	assert.Equal(t, "Updated Source", result.DataSource.Name)
}

func TestHandlerCore_Update_Error(t *testing.T) {
	svc := &mockService{
		updateErr: ErrNotFound,
	}
	core := NewHandlerCore(svc)

	_, err := core.Update(context.Background(), uuid.New(), UpdateDataSourceParams{})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestHandlerCore_Delete(t *testing.T) {
	svc := &mockService{}
	core := NewHandlerCore(svc)

	result, err := core.Delete(context.Background(), uuid.New())
	require.NoError(t, err)
	assert.Equal(t, DeleteResult{}, result)
}

func TestHandlerCore_Delete_Error(t *testing.T) {
	svc := &mockService{
		deleteErr: ErrNotFound,
	}
	core := NewHandlerCore(svc)

	_, err := core.Delete(context.Background(), uuid.New())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestHandlerCore_TestConnection(t *testing.T) {
	svc := &mockService{
		testResult: ConnectionTestResult{Success: true, Message: "ok", Latency: "5ms"},
	}
	core := NewHandlerCore(svc)

	result, err := core.TestConnection(context.Background(), uuid.New())
	require.NoError(t, err)
	assert.True(t, result.Success)
	assert.Equal(t, "ok", result.Message)
	assert.Equal(t, "5ms", result.Latency)
}

func TestHandlerCore_TestConnection_Error(t *testing.T) {
	svc := &mockService{
		testErr: ErrNotFound,
	}
	core := NewHandlerCore(svc)

	_, err := core.TestConnection(context.Background(), uuid.New())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestHandlerCore_Sync(t *testing.T) {
	svc := &mockService{}
	core := NewHandlerCore(svc)

	result, err := core.Sync(context.Background(), uuid.New())
	require.NoError(t, err)
	assert.Equal(t, SyncResult{}, result)
}

func TestHandlerCore_Sync_Error(t *testing.T) {
	svc := &mockService{
		syncErr: ErrNotFound,
	}
	core := NewHandlerCore(svc)

	_, err := core.Sync(context.Background(), uuid.New())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNotFound)
}