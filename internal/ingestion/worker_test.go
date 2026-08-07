package ingestion

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/agentic-demo/platform/internal/domain"
	"github.com/agentic-demo/platform/internal/knowledge"
	"github.com/agentic-demo/platform/internal/storage"
	"github.com/agentic-demo/platform/internal/usage"
	"github.com/google/uuid"
)

// staticConnector returns a fixed set of documents on every Extract call, so a
// re-ingest of the same source produces identical content.
type staticConnector struct {
	docs []domain.RawDocument
}

func (c *staticConnector) Extract(context.Context) ([]domain.RawDocument, error) {
	return c.docs, nil
}

// fixedResolver returns a fixed connector, ignoring tenant/source scoping (the
// resolver's scoping is covered in connector_test.go).
type fixedResolver struct {
	conn Connector
}

func (r *fixedResolver) Resolve(context.Context, domain.TenantID, string) (Connector, error) {
	return r.conn, nil
}

// recordingStore records every ReplaceSource call for assertion.
type recordingStore struct {
	calls      int
	lastDocs   []domain.Document
	lastChunks []domain.Chunk
}

func (s *recordingStore) Store(_ context.Context, _ domain.TenantID, _ []domain.Document, _ []domain.Chunk) error {
	return nil
}

func (s *recordingStore) ReplaceSource(_ context.Context, _ domain.TenantID, _ string, docs []domain.Document, chunks []domain.Chunk) error {
	s.calls++
	s.lastDocs = docs
	s.lastChunks = chunks
	return nil
}

func (s *recordingStore) Query(_ context.Context, _ domain.TenantID, _ string, _ int, _ knowledge.QueryFilters) ([]domain.RankedChunk, error) {
	return nil, nil
}

func (s *recordingStore) GetDocument(_ context.Context, _ domain.TenantID, _ string) (domain.Document, error) {
	return domain.Document{}, nil
}

func (s *recordingStore) DeleteTenantData(_ context.Context, _ domain.TenantID) error {
	return nil
}

func (s *recordingStore) GetStats(_ context.Context, _ domain.TenantID) (map[string]int, error) {
	return nil, nil
}

// TestIngestWorker_ReingestionDoesNotWipe verifies that re-ingesting the same
// source produces chunks again: dedup is scoped to a single ingest run, not the
// worker lifetime. If the dedup were worker-wide, the second run would collapse
// every chunk and ReplaceSource would receive an empty set, deleting the
// source's prior documents and chunks (the wipe bug).
func TestIngestWorker_ReingestionDoesNotWipe(t *testing.T) {
	ctx := context.Background()
	tenantID := domain.TenantID("tenant-a")
	sourceID := uuid.New().String()

	conn := &staticConnector{docs: []domain.RawDocument{{
		ID:      "doc-1",
		Content: "The quarterly numbers are strong.",
		Metadata: map[string]string{
			"source":        "report.txt",
			"document_type": "text",
		},
	}}}

	store := &recordingStore{}
	worker := NewIngestWorker(
		&fixedResolver{conn: conn},
		knowledge.NewRecursiveChunker(100, 20),
		store,
		usage.NoOpEmitter{},
		"text-embedding-v4",
	)

	if _, err := worker.Ingest(ctx, tenantID, sourceID); err != nil {
		t.Fatalf("first ingest failed: %v", err)
	}
	if _, err := worker.Ingest(ctx, tenantID, sourceID); err != nil {
		t.Fatalf("second ingest failed: %v", err)
	}

	if store.calls != 2 {
		t.Fatalf("ReplaceSource called %d times, want 2", store.calls)
	}
	if len(store.lastChunks) == 0 {
		t.Fatal("second (re-)ingest produced no chunks; dedup must be scoped per run, not per worker")
	}
}

// TestIngestWorker_BinaryDocumentChunked verifies the issue 68 acceptance path
// end-to-end: an uploaded PDF is resolved to a file connector, its bytes are
// parsed into clean text, and the Knowledge Base receives chunks containing
// the parsed text — not raw bytes.
func TestIngestWorker_BinaryDocumentChunked(t *testing.T) {
	ctx := context.Background()
	tenantID := domain.TenantID("tenant-a")

	objects := storage.NewMemoryObjectStore()
	pdfBytes := buildMinimalPDF("Quarterly revenue grew by twenty percent.")
	if err := objects.Put(ctx, tenantID, "sources/abc/file", bytes.NewReader(pdfBytes), int64(len(pdfBytes))); err != nil {
		t.Fatalf("seed object: %v", err)
	}
	reader := &fakeSourceReader{sources: map[string]Source{
		"abc": fileSource("tenant-a", "quarterly.pdf", "sources/abc/file"),
	}}
	resolver := NewConnectorResolver(reader, objects)

	store := &recordingStore{}
	worker := NewIngestWorker(
		resolver,
		knowledge.NewRecursiveChunker(100, 20),
		store,
		usage.NoOpEmitter{},
		"text-embedding-v4",
	)

	if _, err := worker.Ingest(ctx, tenantID, "abc"); err != nil {
		t.Fatalf("ingest failed: %v", err)
	}

	if len(store.lastDocs) != 1 {
		t.Fatalf("expected 1 document stored, got %d", len(store.lastDocs))
	}
	if strings.Contains(store.lastDocs[0].Content, "%PDF-") {
		t.Fatalf("stored document contains raw PDF bytes: %q", store.lastDocs[0].Content)
	}
	if store.lastDocs[0].Content != "Quarterly revenue grew by twenty percent." {
		t.Errorf("stored document content = %q, want parsed PDF text", store.lastDocs[0].Content)
	}

	if len(store.lastChunks) == 0 {
		t.Fatal("expected chunks for the PDF document")
	}
	found := false
	for _, chunk := range store.lastChunks {
		if strings.Contains(chunk.Content, "Quarterly revenue grew by twenty percent.") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("no chunk contains the parsed PDF text: %+v", store.lastChunks)
	}
}

// TestIngestWorker_WordPerObjectPDFChunked reproduces the reported issue-68
// regression: a web-generated PDF whose words are extracted word-per-line
// (with blank lines) must reach the Knowledge Base as joined prose paragraphs,
// not as single-word chunks.
func TestIngestWorker_WordPerObjectPDFChunked(t *testing.T) {
	ctx := context.Background()
	tenantID := domain.TenantID("tenant-a")

	objects := storage.NewMemoryObjectStore()
	pdfBytes := buildWordPerObjectPDF(
		"Introduction", "to", "QMK", "Firmware.",
		"It", "powers", "custom", "keyboards.",
	)
	if err := objects.Put(ctx, tenantID, "sources/abc/file", bytes.NewReader(pdfBytes), int64(len(pdfBytes))); err != nil {
		t.Fatalf("seed object: %v", err)
	}
	reader := &fakeSourceReader{sources: map[string]Source{
		"abc": fileSource("tenant-a", "qmk.pdf", "sources/abc/file"),
	}}
	resolver := NewConnectorResolver(reader, objects)

	store := &recordingStore{}
	worker := NewIngestWorker(
		resolver,
		knowledge.NewRecursiveChunker(100, 20),
		store,
		usage.NoOpEmitter{},
		"text-embedding-v4",
	)

	if _, err := worker.Ingest(ctx, tenantID, "abc"); err != nil {
		t.Fatalf("ingest failed: %v", err)
	}

	if strings.Contains(store.lastDocs[0].Content, "\n\nto\n\n") {
		t.Fatalf("stored document still contains the word-per-line artifact: %q", store.lastDocs[0].Content)
	}
	if !strings.Contains(store.lastDocs[0].Content, "Introduction to QMK Firmware.") {
		t.Errorf("stored document lacks joined prose: %q", store.lastDocs[0].Content)
	}

	found := false
	for _, chunk := range store.lastChunks {
		if strings.Contains(chunk.Content, "It powers custom keyboards.") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("no chunk contains joined prose: %+v", store.lastChunks)
	}
}

// TestIngestWorker_DedupCollapsesWithinRun verifies dedup collapses duplicate
// chunk content within a single ingest run (story 16), e.g. a source that
// yields the same paragraph twice stores only one chunk.
func TestIngestWorker_DedupCollapsesWithinRun(t *testing.T) {
	ctx := context.Background()
	tenantID := domain.TenantID("tenant-a")

	// One document whose content repeats the same line twice.
	conn := &staticConnector{docs: []domain.RawDocument{{
		ID:      "doc-1",
		Content: "Same line.\nSame line.\nSame line.",
		Metadata: map[string]string{
			"source":        "repeat.txt",
			"document_type": "text",
		},
	}}}

	store := &recordingStore{}
	worker := NewIngestWorker(
		&fixedResolver{conn: conn},
		knowledge.NewRecursiveChunker(100, 20),
		store,
		usage.NoOpEmitter{},
		"text-embedding-v4",
	)

	if _, err := worker.Ingest(ctx, tenantID, "source-1"); err != nil {
		t.Fatalf("ingest failed: %v", err)
	}

	if len(store.lastChunks) != 1 {
		t.Fatalf("expected 1 unique chunk after dedup, got %d", len(store.lastChunks))
	}
}
