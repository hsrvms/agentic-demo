package sources

import (
	"context"
	"errors"
	"testing"

	"github.com/agentic-demo/platform/internal/db"
	"github.com/agentic-demo/platform/internal/ingestion"
	"github.com/google/uuid"
)

func seedReaderRepo(t *testing.T, tenantID string, creds []byte) *mockRepo {
	t.Helper()
	repo := newMockRepo()
	_, err := repo.Create(context.Background(), &db.CreateDataSourceParams{
		TenantID:    tenantID,
		SourceType:  "file_upload",
		Name:        "Notes",
		Config:      []byte(`{"filename":"notes.txt","size":"5","object_key":"sources/src/file"}`),
		Credentials: creds,
		Status:      "active",
	})
	if err != nil {
		t.Fatalf("seed repo: %v", err)
	}
	return repo
}

func TestReader_GetProjection(t *testing.T) {
	ctx := context.Background()
	id := uuid.New().String()
	repo := seedReaderRepo(t, "tenant-a", []byte("encrypted-secret"))
	repo.sources[0].ID = uuid.MustParse(id)

	r := NewReader(repo, &mockCrypt{})

	src, err := r.GetProjection(ctx, "tenant-a", id)
	if err != nil {
		t.Fatalf("GetProjection: %v", err)
	}
	if src.SourceID != id {
		t.Errorf("SourceID = %q, want %q", src.SourceID, id)
	}
	if src.TenantID != "tenant-a" {
		t.Errorf("TenantID = %q, want tenant-a", src.TenantID)
	}
	if src.SourceType != "file_upload" {
		t.Errorf("SourceType = %q, want file_upload", src.SourceType)
	}
	// Decrypted credentials are projected.
	if string(src.Credentials) != "encrypted-secret" {
		t.Errorf("Credentials = %q, want decrypted", src.Credentials)
	}
}

func TestReader_GetProjection_ForeignSourceRejected(t *testing.T) {
	ctx := context.Background()
	id := uuid.New().String()
	repo := seedReaderRepo(t, "tenant-b", nil)
	repo.sources[0].ID = uuid.MustParse(id)

	r := NewReader(repo, &mockCrypt{})

	// The source belongs to tenant-b; tenant-a must not see it.
	_, err := r.GetProjection(ctx, "tenant-a", id)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestReader_GetProjection_MissingSource(t *testing.T) {
	r := NewReader(newMockRepo(), &mockCrypt{})

	_, err := r.GetProjection(context.Background(), "tenant-a", uuid.New().String())
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestReader_GetProjection_InvalidID(t *testing.T) {
	r := NewReader(newMockRepo(), &mockCrypt{})

	_, err := r.GetProjection(context.Background(), "tenant-a", "not-a-uuid")
	if !errors.Is(err, ErrInvalidSourceID) {
		t.Fatalf("expected ErrInvalidSourceID, got %v", err)
	}
}

func TestReader_MarkError(t *testing.T) {
	ctx := context.Background()
	id := uuid.New().String()
	repo := seedReaderRepo(t, "tenant-a", nil)
	repo.sources[0].ID = uuid.MustParse(id)

	r := NewReader(repo, &mockCrypt{})

	if err := r.MarkError(ctx, "tenant-a", id, "connector resolution failed: unsupported source type"); err != nil {
		t.Fatalf("MarkError: %v", err)
	}

	// The source status flips to error and the message is recorded.
	if repo.sources[0].Status != string(StatusError) {
		t.Errorf("status = %q, want error", repo.sources[0].Status)
	}
	if !repo.sources[0].LastSyncStatus.Valid || repo.sources[0].LastSyncStatus.String == "" {
		t.Error("expected LastSyncStatus to record the failure message")
	}
}

func TestReader_MarkError_ForeignSourceRejected(t *testing.T) {
	ctx := context.Background()
	id := uuid.New().String()
	repo := seedReaderRepo(t, "tenant-b", nil)
	repo.sources[0].ID = uuid.MustParse(id)

	r := NewReader(repo, &mockCrypt{})

	err := r.MarkError(ctx, "tenant-a", id, "boom")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

// Compile-time check: Reader satisfies the ingestion.SourceReader seam.
var _ ingestion.SourceReader = (*Reader)(nil)