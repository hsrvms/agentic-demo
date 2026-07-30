package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/agentic-demo/platform/internal/db"
	"github.com/agentic-demo/platform/internal/queue"
	"github.com/google/uuid"
)

// Service is the public interface for data source operations.
type Service interface {
	Create(ctx context.Context, params *CreateDataSourceParams) (DataSource, error)
	GetByID(ctx context.Context, id uuid.UUID) (DataSource, error)
	ListByTenant(ctx context.Context, tenantID string, page, pageSize int) (DataSourcePage, error)
	Update(ctx context.Context, id uuid.UUID, params UpdateDataSourceParams) (DataSource, error)
	Delete(ctx context.Context, id uuid.UUID) error
	TestConnection(ctx context.Context, id uuid.UUID) (ConnectionTestResult, error)
	Sync(ctx context.Context, id uuid.UUID) error
}

type service struct {
	repo    Repository
	crypt   CryptService
	tester  ConnectionTester
	jobQueue queue.JobQueue
}

// NewService creates a data source Service.
func NewService(repo Repository, crypt CryptService, tester ConnectionTester, jobQueue queue.JobQueue) Service {
	return &service{
		repo:     repo,
		crypt:    crypt,
		tester:   tester,
		jobQueue: jobQueue,
	}
}

func (s *service) Create(ctx context.Context, params *CreateDataSourceParams) (DataSource, error) {
	if err := s.validateCreate(params); err != nil {
		return DataSource{}, err
	}

	encrypted, err := s.encryptCredentials(params.Credentials)
	if err != nil {
		return DataSource{}, err
	}

	config := toPgConfig(params.Config)
	if config == nil {
		config = []byte("{}")
	}

	row, err := s.repo.Create(ctx, &db.CreateDataSourceParams{
		TenantID:    params.TenantID,
		SourceType:  toPgSourceType(params.SourceType),
		Name:        strings.TrimSpace(params.Name),
		Config:      config,
		Credentials: encrypted,
		Status:      string(StatusInactive),
	})
	if err != nil {
		return DataSource{}, fmt.Errorf("create source: %w", err)
	}

	return toDomain(&row), nil
}

func (s *service) GetByID(ctx context.Context, id uuid.UUID) (DataSource, error) {
	row, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return DataSource{}, err
	}

	ds := toDomain(&row)
	if len(ds.Credentials) > 0 {
		decrypted, err := s.crypt.Decrypt(ds.Credentials)
		if err != nil {
			return DataSource{}, fmt.Errorf("%w: %v", ErrDecryptionFailed, err)
		}
		ds.Credentials = decrypted
	}

	return ds, nil
}

func (s *service) ListByTenant(ctx context.Context, tenantID string, page, pageSize int) (DataSourcePage, error) {
	if strings.TrimSpace(tenantID) == "" {
		return DataSourcePage{}, ErrInvalidTenantID
	}

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	offset := int32((page - 1) * pageSize) //nolint:gosec // page/pageSize bounded above

	rows, err := s.repo.ListByTenant(ctx, &db.ListDataSourcesByTenantParams{
		TenantID: tenantID,
		Limit:    int32(pageSize),
		Offset:   offset,
	})
	if err != nil {
		return DataSourcePage{}, err
	}

	total, err := s.repo.CountByTenant(ctx, tenantID)
	if err != nil {
		return DataSourcePage{}, err
	}

	sources := make([]DataSource, len(rows))
	for i := range rows {
		ds := toDomain(&rows[i])
		// Don't return credentials in list views.
		ds.Credentials = nil
		sources[i] = ds
	}

	return DataSourcePage{
		Sources:    sources,
		TotalCount: int(total),
		Page:       page,
		PageSize:   pageSize,
	}, nil
}

func (s *service) Update(ctx context.Context, id uuid.UUID, params UpdateDataSourceParams) (DataSource, error) {
	existing, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return DataSource{}, err
	}

	updateParams := &db.UpdateDataSourceParams{
		ID:     id,
		Name:   existing.Name,
		Config: existing.Config,
		Status: existing.Status,
	}

	if params.Name != nil {
		name := strings.TrimSpace(*params.Name)
		if name == "" {
			return DataSource{}, ErrInvalidName
		}
		updateParams.Name = name
	}

	if params.Config != nil {
		if !json.Valid([]byte(*params.Config)) {
			return DataSource{}, ErrInvalidConfig
		}
		updateParams.Config = []byte(*params.Config)
	}

	if params.Credentials != nil {
		encrypted, err := s.encryptCredentials(*params.Credentials)
		if err != nil {
			return DataSource{}, err
		}
		updateParams.Credentials = encrypted
	} else {
		updateParams.Credentials = existing.Credentials
	}

	if params.Status != nil {
		updateParams.Status = string(*params.Status)
	}

	row, err := s.repo.Update(ctx, updateParams)
	if err != nil {
		return DataSource{}, fmt.Errorf("update source: %w", err)
	}

	return toDomain(&row), nil
}

func (s *service) Delete(ctx context.Context, id uuid.UUID) error {
	return s.repo.Delete(ctx, id)
}

func (s *service) TestConnection(ctx context.Context, id uuid.UUID) (ConnectionTestResult, error) {
	ds, err := s.GetByID(ctx, id)
	if err != nil {
		return ConnectionTestResult{}, err
	}

	return s.tester.TestConnection(ctx, &ds)
}

func (s *service) Sync(ctx context.Context, id uuid.UUID) error {
	row, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}

	job := queue.Job{
		Type:    queue.TypeIngestionManual,
		Queue:   queue.QueueIngestion,
		Payload: queue.IngestionPayload{TenantID: row.TenantID, SourceID: id.String()},
	}

	if _, err := s.jobQueue.Enqueue(ctx, job); err != nil {
		return fmt.Errorf("enqueue sync job: %w", err)
	}

	// Update sync status to indicate sync is in progress.
	_, err = s.repo.UpdateSyncStatus(ctx, &db.UpdateDataSourceSyncStatusParams{
		ID:             id,
		LastSyncAt:     toPgTimestamptz(timePtr(time.Now())),
		LastSyncStatus: toPgText("sync_enqueued"),
		Status:         string(StatusActive),
	})
	if err != nil {
		return fmt.Errorf("update sync status: %w", err)
	}

	return nil
}

// --- validation ---

func (s *service) validateCreate(params *CreateDataSourceParams) error {
	if strings.TrimSpace(params.TenantID) == "" {
		return ErrInvalidTenantID
	}
	if strings.TrimSpace(params.Name) == "" {
		return ErrInvalidName
	}
	if !ValidSourceType(string(params.SourceType)) {
		return ErrInvalidSourceType
	}
	if params.Config != nil && !json.Valid([]byte(params.Config)) {
		return ErrInvalidConfig
	}
	return nil
}

// --- helpers ---

func (s *service) encryptCredentials(creds []byte) ([]byte, error) {
	if len(creds) == 0 {
		return nil, nil
	}
	encrypted, err := s.crypt.Encrypt(creds)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrEncryptionFailed, err)
	}
	return encrypted, nil
}

func timePtr(t time.Time) *time.Time {
	return &t
}