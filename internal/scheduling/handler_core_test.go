package scheduling

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockService implements ScheduleService for testing.
type mockService struct {
	createResult ReportSchedule
	createErr    error
	getResult    ReportSchedule
	getErr       error
	listResult   []ReportSchedule
	listErr      error
	updateResult ReportSchedule
	updateErr    error
	deleteErr    error
	toggleResult ReportSchedule
	toggleErr    error
}

func (m *mockService) Create(ctx context.Context, params *CreateScheduleParams) (ReportSchedule, error) {
	if m.createErr != nil {
		return ReportSchedule{}, m.createErr
	}
	if m.createResult.ID == uuid.Nil {
		m.createResult.ID = uuid.New()
	}
	m.createResult.TenantID = params.TenantID
	m.createResult.Type = params.Type
	m.createResult.CronExpr = params.CronExpr
	return m.createResult, nil
}

func (m *mockService) GetByID(ctx context.Context, id uuid.UUID) (ReportSchedule, error) {
	if m.getErr != nil {
		return ReportSchedule{}, m.getErr
	}
	return m.getResult, nil
}

func (m *mockService) ListByTenant(ctx context.Context, tenantID string) ([]ReportSchedule, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	return m.listResult, nil
}

func (m *mockService) Update(ctx context.Context, params *UpdateScheduleParams) (ReportSchedule, error) {
	if m.updateErr != nil {
		return ReportSchedule{}, m.updateErr
	}
	return m.updateResult, nil
}

func (m *mockService) Delete(ctx context.Context, id uuid.UUID) error {
	return m.deleteErr
}

func (m *mockService) Toggle(ctx context.Context, id uuid.UUID) (ReportSchedule, error) {
	if m.toggleErr != nil {
		return ReportSchedule{}, m.toggleErr
	}
	return m.toggleResult, nil
}

func (m *mockService) ListAllEnabled(ctx context.Context) ([]ReportSchedule, error) {
	return nil, nil
}

func TestHandlerCore_List(t *testing.T) {
	svc := &mockService{
		listResult: []ReportSchedule{
			{ID: uuid.New(), TenantID: "tenant-1", Type: ScheduleDaily, CronExpr: "0 8 * * *"},
			{ID: uuid.New(), TenantID: "tenant-1", Type: ScheduleWeekly, CronExpr: "0 9 * * 1"},
		},
	}
	core := NewHandlerCore(svc)

	result, err := core.List(context.Background(), "tenant-1")
	require.NoError(t, err)
	assert.Len(t, result.Schedules, 2)
	assert.Equal(t, ScheduleDaily, result.Schedules[0].Type)
	assert.Equal(t, "0 8 * * *", result.Schedules[0].CronExpr)
	assert.Equal(t, ScheduleWeekly, result.Schedules[1].Type)
}

func TestHandlerCore_List_Empty(t *testing.T) {
	svc := &mockService{
		listResult: []ReportSchedule{},
	}
	core := NewHandlerCore(svc)

	result, err := core.List(context.Background(), "tenant-1")
	require.NoError(t, err)
	assert.Len(t, result.Schedules, 0)
}

func TestHandlerCore_List_Error(t *testing.T) {
	svc := &mockService{
		listErr: ErrInvalidTenantID,
	}
	core := NewHandlerCore(svc)

	_, err := core.List(context.Background(), "")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidTenantID)
}

func TestHandlerCore_Get(t *testing.T) {
	id := uuid.New()
	now := time.Now()
	svc := &mockService{
		getResult: ReportSchedule{
			ID:        id,
			TenantID:  "tenant-1",
			Type:      ScheduleMonthly,
			CronExpr:  "0 7 1 * *",
			Focus:     "sales trends",
			Format:    "standard",
			Enabled:   true,
			CreatedAt: now,
			UpdatedAt: now,
		},
	}
	core := NewHandlerCore(svc)

	result, err := core.Get(context.Background(), id)
	require.NoError(t, err)
	assert.Equal(t, id, result.Schedule.ID)
	assert.Equal(t, "tenant-1", result.Schedule.TenantID)
	assert.Equal(t, ScheduleMonthly, result.Schedule.Type)
	assert.Equal(t, "sales trends", result.Schedule.Focus)
	assert.True(t, result.Schedule.Enabled)
}

func TestHandlerCore_Get_Error(t *testing.T) {
	svc := &mockService{
		getErr: ErrScheduleNotFound,
	}
	core := NewHandlerCore(svc)

	_, err := core.Get(context.Background(), uuid.New())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrScheduleNotFound)
}

func TestHandlerCore_Create(t *testing.T) {
	svc := &mockService{
		createResult: ReportSchedule{
			ID:       uuid.New(),
			Type:     ScheduleDaily,
			CronExpr: "0 8 * * *",
			Focus:    "daily recap",
			Format:   "standard",
			Enabled:  true,
		},
	}
	core := NewHandlerCore(svc)

	result, err := core.Create(context.Background(), "tenant-1", &CreateScheduleParams{
		TenantID: "tenant-1",
		Type:     ScheduleDaily,
		CronExpr: "0 8 * * *",
		Focus:    "daily recap",
		Format:   "standard",
	})
	require.NoError(t, err)
	assert.Equal(t, ScheduleDaily, result.Schedule.Type)
	assert.Equal(t, "0 8 * * *", result.Schedule.CronExpr)
	assert.Equal(t, "daily recap", result.Schedule.Focus)
}

func TestHandlerCore_Create_Error(t *testing.T) {
	svc := &mockService{
		createErr: ErrInvalidScheduleType,
	}
	core := NewHandlerCore(svc)

	_, err := core.Create(context.Background(), "tenant-1", &CreateScheduleParams{
		Type: ScheduleType("invalid"),
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidScheduleType)
}

func TestHandlerCore_Update(t *testing.T) {
	id := uuid.New()
	svc := &mockService{
		updateResult: ReportSchedule{
			ID:       id,
			Type:     ScheduleWeekly,
			CronExpr: "0 9 * * 1",
			Focus:    "weekly summary",
			Format:   "detailed",
			Enabled:  true,
		},
	}
	core := NewHandlerCore(svc)

	result, err := core.Update(context.Background(), &UpdateScheduleParams{
		ID:       id,
		Type:     ScheduleWeekly,
		CronExpr: "0 9 * * 1",
		Focus:    "weekly summary",
		Format:   "detailed",
	})
	require.NoError(t, err)
	assert.Equal(t, ScheduleWeekly, result.Schedule.Type)
	assert.Equal(t, "weekly summary", result.Schedule.Focus)
}

func TestHandlerCore_Update_Error(t *testing.T) {
	svc := &mockService{
		updateErr: ErrScheduleNotFound,
	}
	core := NewHandlerCore(svc)

	_, err := core.Update(context.Background(), &UpdateScheduleParams{
		ID: uuid.New(),
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrScheduleNotFound)
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
		deleteErr: ErrScheduleNotFound,
	}
	core := NewHandlerCore(svc)

	_, err := core.Delete(context.Background(), uuid.New())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrScheduleNotFound)
}

func TestHandlerCore_Toggle(t *testing.T) {
	id := uuid.New()
	svc := &mockService{
		toggleResult: ReportSchedule{
			ID:      id,
			Enabled: false,
		},
	}
	core := NewHandlerCore(svc)

	result, err := core.Toggle(context.Background(), id)
	require.NoError(t, err)
	assert.False(t, result.Schedule.Enabled)
}

func TestHandlerCore_Toggle_Error(t *testing.T) {
	svc := &mockService{
		toggleErr: ErrScheduleNotFound,
	}
	core := NewHandlerCore(svc)

	_, err := core.Toggle(context.Background(), uuid.New())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrScheduleNotFound)
}
