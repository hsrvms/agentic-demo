package sources

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/agentic-demo/platform/internal/db"
	"github.com/agentic-demo/platform/internal/domain"
	"github.com/agentic-demo/platform/internal/ingestion"
	"github.com/google/uuid"
)

// Reader exposes a tenant-scoped, read-only projection of a DataSource to the
// Ingestion module. It implements ingestion.SourceReader, which is the narrow
// seam the ConnectorResolver consumes. The Sources module owns the DataSource's
// real shape; the Reader projects only the fields ingestion needs.
type Reader struct {
	repo  Repository
	crypt CryptService
}

// NewReader builds a SourceReader backed by the given repository and crypt.
func NewReader(repo Repository, crypt CryptService) *Reader {
	return &Reader{repo: repo, crypt: crypt}
}

// GetProjection returns a projection of the source with the given ID, scoped to
// tenantID. It returns ErrNotFound when the source does not exist or belongs to
// another tenant, so a worker cannot extract a source outside its tenant.
func (r *Reader) GetProjection(ctx context.Context, tenantID domain.TenantID, sourceID string) (ingestion.Source, error) {
	id, err := uuid.Parse(sourceID)
	if err != nil {
		return ingestion.Source{}, fmt.Errorf("%w: %s", ErrInvalidSourceID, sourceID)
	}

	row, err := r.repo.GetByID(ctx, id)
	if err != nil {
		return ingestion.Source{}, err
	}

	// Reject a source that does not belong to the requesting tenant. Returning
	// ErrNotFound does not leak whether another tenant's source exists.
	if row.TenantID != string(tenantID) {
		return ingestion.Source{}, ErrNotFound
	}

	var credentials []byte
	if len(row.Credentials) > 0 {
		decrypted, err := r.crypt.Decrypt(row.Credentials)
		if err != nil {
			return ingestion.Source{}, fmt.Errorf("%w: %v", ErrDecryptionFailed, err)
		}
		credentials = decrypted
	}

	return ingestion.Source{
		SourceID:    row.ID.String(),
		TenantID:    domain.TenantID(row.TenantID),
		SourceType:  row.SourceType,
		Config:      json.RawMessage(row.Config),
		Credentials: credentials,
	}, nil
}

// MarkError records a failed resolution by flipping the source's status to
// error. Retry is left to manual action. The operation is tenant-scoped.
func (r *Reader) MarkError(ctx context.Context, tenantID domain.TenantID, sourceID, message string) error {
	id, err := uuid.Parse(sourceID)
	if err != nil {
		return fmt.Errorf("%w: %s", ErrInvalidSourceID, sourceID)
	}

	row, err := r.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if row.TenantID != string(tenantID) {
		return ErrNotFound
	}

	_, err = r.repo.UpdateSyncStatus(ctx, &db.UpdateDataSourceSyncStatusParams{
		ID:             id,
		Status:         string(StatusError),
		LastSyncStatus: toPgText(message),
	})
	if err != nil {
		return fmt.Errorf("mark source %s error: %w", sourceID, err)
	}
	return nil
}
