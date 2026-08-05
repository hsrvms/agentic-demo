package sources

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/agentic-demo/platform/internal/db"
	"github.com/agentic-demo/platform/internal/domain"
	"github.com/agentic-demo/platform/internal/queue"
	"github.com/google/uuid"
)

// ObjectStore is the narrow object-store seam the sources module consumes.
// It is satisfied by storage.ObjectStore (MinIO adapter or in-memory fake) but
// is defined here so the sources module depends only on what it needs:
// persisting uploaded file bytes and removing them on delete.
type ObjectStore interface {
	Put(ctx context.Context, tenantID domain.TenantID, key string, r io.Reader, size int64) error
	Delete(ctx context.Context, tenantID domain.TenantID, key string) error
}

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
	repo        Repository
	crypt       CryptService
	tester      ConnectionTester
	jobQueue    queue.JobQueue
	objectStore ObjectStore
}

// NewService creates a data source Service.
func NewService(repo Repository, crypt CryptService, tester ConnectionTester, jobQueue queue.JobQueue, objectStore ObjectStore) Service {
	return &service{
		repo:        repo,
		crypt:       crypt,
		tester:      tester,
		jobQueue:    jobQueue,
		objectStore: objectStore,
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

	// File uploads persist their bytes to the object store and reference the
	// object key in the config; the credentials column stays clear.
	if params.SourceType == SourceTypeFileUpload && len(params.File) > 0 {
		if err := s.storeFile(ctx, params.TenantID, &row, params.File); err != nil {
			return DataSource{}, err
		}
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

	// A new file upload replaces the stored object and updates the object key.
	if len(params.File) > 0 {
		if err := s.storeFile(ctx, row.TenantID, &row, params.File); err != nil {
			return DataSource{}, err
		}
	}

	return toDomain(&row), nil
}

func (s *service) Delete(ctx context.Context, id uuid.UUID) error {
	row, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}

	if err := s.repo.Delete(ctx, id); err != nil {
		return err
	}

	// Best-effort: remove the stored object if the source references one. A
	// failure here must not fail the delete — the DB row is already gone and
	// the object is recoverable via GC.
	if key := fileObjectKey(row.Config); key != "" {
		_ = s.objectStore.Delete(ctx, domain.TenantID(row.TenantID), key)
	}

	return nil
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

// storeFile persists an uploaded file's bytes to the object store and records
// the object key in the source's config. The key is tenant-relative; the store
// prefixes it with tenant/{tenantID}/ so cross-tenant access is impossible.
func (s *service) storeFile(ctx context.Context, tenantID string, row *db.DataSourceConfig, content []byte) error {
	key := "sources/" + row.ID.String() + "/file"
	if err := s.objectStore.Put(ctx, domain.TenantID(tenantID), key, bytes.NewReader(content), int64(len(content))); err != nil {
		return fmt.Errorf("store source file: %w", err)
	}

	// Once the object is written, any later failure must not leave an orphaned
	// object behind (the config/Db row would not reference it). Best-effort
	// remove the object before returning the error; GC is the safety net.
	cleanup := func(err error) error {
		_ = s.objectStore.Delete(ctx, domain.TenantID(tenantID), key)
		return err
	}

	config, err := withFileObjectKey(row.Config, key)
	if err != nil {
		return cleanup(err)
	}

	updated, err := s.repo.Update(ctx, &db.UpdateDataSourceParams{
		ID:          row.ID,
		Name:        row.Name,
		Config:      config,
		Credentials: row.Credentials,
		Status:      row.Status,
	})
	if err != nil {
		return cleanup(fmt.Errorf("record source object key: %w", err))
	}
	*row = updated
	return nil
}

// withFileObjectKey returns the config JSON with the object_key set, preserving
// any existing fields (filename, size).
func withFileObjectKey(config []byte, key string) ([]byte, error) {
	var cfg map[string]interface{}
	if len(config) > 0 {
		if err := json.Unmarshal(config, &cfg); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrInvalidConfig, err)
		}
	}
	if cfg == nil {
		cfg = map[string]interface{}{}
	}
	cfg["object_key"] = key
	return json.Marshal(cfg)
}

// fileObjectKey returns the object_key recorded in a file_upload source's
// config, or "" when the source does not reference a stored object.
func fileObjectKey(config []byte) string {
	var cfg struct {
		ObjectKey string `json:"object_key"`
	}
	if len(config) == 0 {
		return ""
	}
	if err := json.Unmarshal(config, &cfg); err != nil {
		return ""
	}
	return cfg.ObjectKey
}

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
