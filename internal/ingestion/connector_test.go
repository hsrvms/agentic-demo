package ingestion

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/agentic-demo/platform/internal/domain"
	"github.com/agentic-demo/platform/internal/storage"
)

// fakeSourceReader is a test double for SourceReader. It serves a fixed set of
// sources keyed by source ID (it deliberately does not scope by tenant, so the
// resolver's own tenant check is exercised) and records MarkError calls.
type fakeSourceReader struct {
	sources    map[string]Source // key: sourceID
	marked     []string          // sourceIDs marked error
	getCalls   []string          // tenantID/sourceID tuples passed to GetProjection
	projectErr error             // error returned by GetProjection, when set
	markError  func(ctx context.Context, tenantID domain.TenantID, sourceID, message string) error
}

func (f *fakeSourceReader) GetProjection(ctx context.Context, tenantID domain.TenantID, sourceID string) (Source, error) {
	f.getCalls = append(f.getCalls, string(tenantID)+"/"+sourceID)
	if f.projectErr != nil {
		return Source{}, f.projectErr
	}
	src, ok := f.sources[sourceID]
	if !ok {
		return Source{}, ErrSourceNotFound
	}
	return src, nil
}

func (f *fakeSourceReader) MarkError(ctx context.Context, tenantID domain.TenantID, sourceID, message string) error {
	f.marked = append(f.marked, sourceID)
	if f.markError != nil {
		return f.markError(ctx, tenantID, sourceID, message)
	}
	return nil
}

func fileSource(tenantID, sourceID, filename, objectKey string) Source {
	cfg, _ := json.Marshal(map[string]any{
		"filename":   filename,
		"size":       int64(1024),
		"object_key": objectKey,
	})
	return Source{
		SourceID:   sourceID,
		TenantID:   domain.TenantID(tenantID),
		SourceType: sourceTypeFileUpload,
		Config:     cfg,
	}
}

func TestConnectorResolver_ResolvesFileUpload(t *testing.T) {
	objects := storage.NewMemoryObjectStore()
	ctx := context.Background()
	tenantID := domain.TenantID("tenant-a")

	if err := objects.Put(ctx, tenantID, "sources/abc/file", strings.NewReader("hello world"), 11); err != nil {
		t.Fatalf("seed object: %v", err)
	}

	reader := &fakeSourceReader{sources: map[string]Source{
		"abc": fileSource("tenant-a", "abc", "notes.txt", "sources/abc/file"),
	}}

	resolver := NewConnectorResolver(reader, objects)

	conn, err := resolver.Resolve(ctx, tenantID, "abc")
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}

	fc, ok := conn.(*FileConnector)
	if !ok {
		t.Fatalf("expected *FileConnector, got %T", conn)
	}
	if fc.tenantID != tenantID {
		t.Errorf("connector tenant = %q, want %q", fc.tenantID, tenantID)
	}
	if fc.objectKey != "sources/abc/file" {
		t.Errorf("connector objectKey = %q, want sources/abc/file", fc.objectKey)
	}

	docs, err := conn.Extract(ctx)
	if err != nil {
		t.Fatalf("Extract failed: %v", err)
	}
	if len(docs) != 1 {
		t.Fatalf("expected 1 document, got %d", len(docs))
	}
	if docs[0].Content != "hello world" {
		t.Errorf("document content = %q, want hello world", docs[0].Content)
	}
	if docs[0].ID != "abc" {
		t.Errorf("document ID = %q, want source ID abc", docs[0].ID)
	}
	if docs[0].Metadata["source"] != "notes.txt" {
		t.Errorf("document source = %q, want notes.txt", docs[0].Metadata["source"])
	}
	if docs[0].Metadata["document_type"] != "text" {
		t.Errorf("document_type = %q, want text", docs[0].Metadata["document_type"])
	}
}

func TestConnectorResolver_MissingSource(t *testing.T) {
	objects := storage.NewMemoryObjectStore()
	reader := &fakeSourceReader{sources: map[string]Source{}}

	resolver := NewConnectorResolver(reader, objects)

	_, err := resolver.Resolve(context.Background(), domain.TenantID("tenant-a"), "nope")
	if !errors.Is(err, ErrSourceNotFound) {
		t.Fatalf("expected ErrSourceNotFound, got %v", err)
	}
	if !errors.Is(err, ErrResolutionFailed) {
		t.Fatalf("expected ErrResolutionFailed wrapping, got %v", err)
	}
	// MarkError is attempted best-effort on any load failure, but the underlying
	// reader no-ops for a source that does not exist.
	if len(reader.marked) != 1 || reader.marked[0] != "nope" {
		t.Errorf("expected MarkError attempted for missing source, got %v", reader.marked)
	}
}

// TestConnectorResolver_LoadFailureMarksSourceError verifies that a source that
// exists but cannot be projected (e.g. a decryption failure) is flipped to the
// error state so the failure is surfaced and retried manually (story 8).
func TestConnectorResolver_LoadFailureMarksSourceError(t *testing.T) {
	objects := storage.NewMemoryObjectStore()
	reader := &fakeSourceReader{
		sources:    map[string]Source{},
		projectErr: errors.New("decrypt failed"),
	}

	resolver := NewConnectorResolver(reader, objects)

	_, err := resolver.Resolve(context.Background(), domain.TenantID("tenant-a"), "abc")
	if !errors.Is(err, ErrResolutionFailed) {
		t.Fatalf("expected ErrResolutionFailed, got %v", err)
	}
	if len(reader.marked) != 1 || reader.marked[0] != "abc" {
		t.Errorf("expected MarkError on load failure, got %v", reader.marked)
	}
}

func TestConnectorResolver_ForeignSourceRejected(t *testing.T) {
	objects := storage.NewMemoryObjectStore()
	// The source belongs to tenant-b; the job asks for tenant-a.
	reader := &fakeSourceReader{sources: map[string]Source{
		"abc": fileSource("tenant-b", "abc", "notes.txt", "sources/abc/file"),
	}}

	resolver := NewConnectorResolver(reader, objects)

	_, err := resolver.Resolve(context.Background(), domain.TenantID("tenant-a"), "abc")
	if !errors.Is(err, ErrForeignSource) {
		t.Fatalf("expected ErrForeignSource, got %v", err)
	}
	if !errors.Is(err, ErrResolutionFailed) {
		t.Fatalf("expected ErrResolutionFailed wrapping, got %v", err)
	}
	// The resolver always passes the requesting tenant to the reader.
	if len(reader.getCalls) != 1 || reader.getCalls[0] != "tenant-a/abc" {
		t.Errorf("GetProjection calls = %v, want [tenant-a/abc]", reader.getCalls)
	}
}

func TestConnectorResolver_UnsupportedSourceType(t *testing.T) {
	objects := storage.NewMemoryObjectStore()
	src := Source{
		SourceID:   "crm-1",
		TenantID:   domain.TenantID("tenant-a"),
		SourceType: "crm_hubspot",
		Config:     json.RawMessage(`{}`),
	}
	reader := &fakeSourceReader{sources: map[string]Source{"crm-1": src}}

	resolver := NewConnectorResolver(reader, objects)

	_, err := resolver.Resolve(context.Background(), domain.TenantID("tenant-a"), "crm-1")
	if !errors.Is(err, ErrUnsupportedSourceType) {
		t.Fatalf("expected ErrUnsupportedSourceType, got %v", err)
	}
	if !errors.Is(err, ErrResolutionFailed) {
		t.Fatalf("expected ErrResolutionFailed wrapping, got %v", err)
	}
	// A real-but-unresolvable source is marked error for manual retry.
	if len(reader.marked) != 1 || reader.marked[0] != "crm-1" {
		t.Errorf("expected source crm-1 marked error, got %v", reader.marked)
	}
}

func TestConnectorResolver_MissingObjectKey(t *testing.T) {
	objects := storage.NewMemoryObjectStore()
	cfg, _ := json.Marshal(map[string]any{"filename": "a.txt", "size": int64(1)})
	src := Source{
		SourceID:   "abc",
		TenantID:   domain.TenantID("tenant-a"),
		SourceType: sourceTypeFileUpload,
		Config:     cfg,
	}
	reader := &fakeSourceReader{sources: map[string]Source{"abc": src}}

	resolver := NewConnectorResolver(reader, objects)

	_, err := resolver.Resolve(context.Background(), domain.TenantID("tenant-a"), "abc")
	if !errors.Is(err, ErrMissingObjectKey) {
		t.Fatalf("expected ErrMissingObjectKey, got %v", err)
	}
	if len(reader.marked) != 1 || reader.marked[0] != "abc" {
		t.Errorf("expected source marked error, got %v", reader.marked)
	}
}

func TestConnectorResolver_RejectsEscapingObjectKey(t *testing.T) {
	objects := storage.NewMemoryObjectStore()
	reader := &fakeSourceReader{sources: map[string]Source{
		"abc": fileSource("tenant-a", "abc", "a.txt", "tenant/other-tenant/file"),
	}}

	resolver := NewConnectorResolver(reader, objects)

	_, err := resolver.Resolve(context.Background(), domain.TenantID("tenant-a"), "abc")
	if !errors.Is(err, ErrInvalidObjectKey) {
		t.Fatalf("expected ErrInvalidObjectKey, got %v", err)
	}
	if len(reader.marked) != 1 {
		t.Errorf("expected source marked error, got %v", reader.marked)
	}
}

func TestFileConnector_RejectsEscapingKeyOnExtract(t *testing.T) {
	objects := storage.NewMemoryObjectStore()
	conn := &FileConnector{
		tenantID:  domain.TenantID("tenant-a"),
		sourceID:  "abc",
		objectKey: "../escape",
		objects:   objects,
	}

	_, err := conn.Extract(context.Background())
	if !errors.Is(err, ErrInvalidObjectKey) {
		t.Fatalf("expected ErrInvalidObjectKey, got %v", err)
	}
}

func TestFileConnector_ReadsBytesFromObjectStore(t *testing.T) {
	ctx := context.Background()
	tenantID := domain.TenantID("tenant-a")
	objects := storage.NewMemoryObjectStore()
	if err := objects.Put(ctx, tenantID, "sources/abc/file", strings.NewReader("report data"), 11); err != nil {
		t.Fatalf("seed object: %v", err)
	}

	conn := &FileConnector{
		tenantID:  tenantID,
		sourceID:  "abc",
		objectKey: "sources/abc/file",
		filename:  "report.csv",
		docType:   "csv",
		objects:   objects,
	}

	docs, err := conn.Extract(ctx)
	if err != nil {
		t.Fatalf("Extract failed: %v", err)
	}
	if len(docs) != 1 || docs[0].Content != "report data" {
		t.Fatalf("unexpected documents: %+v", docs)
	}
	if docs[0].Metadata["document_type"] != "csv" {
		t.Errorf("document_type = %q, want csv", docs[0].Metadata["document_type"])
	}
}
