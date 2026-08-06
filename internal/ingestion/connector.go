// Connector resolution seam.
//
// The Ingestion module never sees a DataSource's real shape. It consumes a
// read-only, tenant-scoped projection (Source) through a narrow SourceReader
// that the Sources module implements, and reads object bytes through a narrow
// ObjectReader satisfied by the Storage module. The ConnectorResolver is the
// single place that maps a SourceType to extraction behaviour.
package ingestion

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/agentic-demo/platform/internal/domain"
)

// Source is a read-only projection of a DataSource, sufficient to build a
// Connector. It carries the fields the resolver needs and nothing more.
// Credentials are decrypted plaintext; nil when the source has none.
type Source struct {
	SourceID    string
	TenantID    domain.TenantID
	SourceType  string
	Config      json.RawMessage
	Credentials []byte
}

// SourceReader is the narrow, ingestion-defined seam the resolver uses to load
// and update a tenant-scoped DataSource. Implemented by the Sources module.
//
// GetProjection returns a projection of the source with the given ID, or an
// error when it does not exist or does not belong to tenantID. MarkError
// records a failed resolution on the source so its status flips to error;
// retry is left to manual action.
type SourceReader interface {
	GetProjection(ctx context.Context, tenantID domain.TenantID, sourceID string) (Source, error)
	MarkError(ctx context.Context, tenantID domain.TenantID, sourceID string, message string) error
}

// ObjectReader is the narrow object-store read seam the file Connector
// consumes. It is satisfied by storage.ObjectStore (MinIO adapter or the
// in-memory fake). Get returns a reader the caller must close.
type ObjectReader interface {
	Get(ctx context.Context, tenantID domain.TenantID, key string) (io.ReadCloser, error)
}

// ConnectorResolver turns a tenant-scoped DataSource into a Connector at
// ingest time. It is the single place that maps a SourceType to extraction
// behaviour.
type ConnectorResolver interface {
	// Resolve returns a Connector built from the source's config and decrypted
	// credentials. Resolution is tenant-scoped: it always uses the requesting
	// tenantID and rejects a source that belongs to another tenant. A failed
	// resolution marks the source as error and leaves retry to manual action.
	Resolve(ctx context.Context, tenantID domain.TenantID, sourceID string) (Connector, error)
}

// Source type strings mirrored from the Sources module. The resolver must map
// these to extraction behaviour without importing the sources package.
const sourceTypeFileUpload = "file_upload"

// Sentinel errors returned by the resolver.
var (
	// ErrSourceNotFound is returned when the source does not exist.
	ErrSourceNotFound = errors.New("source not found")
	// ErrForeignSource is returned when the source belongs to another tenant.
	ErrForeignSource = errors.New("source does not belong to tenant")
	// ErrUnsupportedSourceType is returned for a source type with no connector.
	ErrUnsupportedSourceType = errors.New("unsupported source type")
	// ErrInvalidObjectKey is returned when a config references an object key
	// that could escape the requesting tenant's prefix.
	ErrInvalidObjectKey = errors.New("invalid object key")
	// ErrMissingObjectKey is returned when a file_upload source has no object_key.
	ErrMissingObjectKey = errors.New("missing object key")
	// ErrResolutionFailed wraps every permanent resolution failure so the queue
	// can skip retry — a broken source is retried manually, not by the queue.
	ErrResolutionFailed = errors.New("connector resolution failed")
)

// connectorResolver resolves a DataSource into a Connector.
type connectorResolver struct {
	sources SourceReader
	objects ObjectReader
}

// NewConnectorResolver builds a ConnectorResolver from the narrow seams it
// consumes. sources is implemented by the Sources module; objects is any
// ObjectStore adapter (MinIO or the in-memory fake).
func NewConnectorResolver(sources SourceReader, objects ObjectReader) ConnectorResolver {
	return &connectorResolver{sources: sources, objects: objects}
}

func (r *connectorResolver) Resolve(ctx context.Context, tenantID domain.TenantID, sourceID string) (Connector, error) {
	src, err := r.sources.GetProjection(ctx, tenantID, sourceID)
	if err != nil {
		// Best-effort: flip the source to error so a broken source is surfaced
		// and retried manually rather than silently retried. A source that
		// cannot be loaded at all (e.g. not found) is still reported as a
		// permanent resolution failure so the queue skips it.
		_ = r.sources.MarkError(ctx, tenantID, sourceID, err.Error())
		return nil, resolutionFailed(fmt.Errorf("load source %s: %w", sourceID, err))
	}

	// Defense-in-depth: reject a source that did not resolve to the requesting
	// tenant, even if the reader returned one.
	if src.TenantID != tenantID {
		connErr := fmt.Errorf("%w: source %s belongs to tenant %s", ErrForeignSource, sourceID, src.TenantID)
		_ = r.sources.MarkError(ctx, tenantID, sourceID, connErr.Error())
		return nil, resolutionFailed(connErr)
	}

	switch src.SourceType {
	case sourceTypeFileUpload:
		return r.buildFileConnector(ctx, &src)
	default:
		// Only the resolution seam and the file connector are built in this
		// scope; website and CRM connectors are future work. A real source of
		// an unresolved type cannot be ingested yet, so mark it error rather
		// than silently resolving to nothing.
		connErr := fmt.Errorf("%w: %s", ErrUnsupportedSourceType, src.SourceType)
		_ = r.sources.MarkError(ctx, tenantID, src.SourceID, connErr.Error())
		return nil, resolutionFailed(connErr)
	}
}

// buildFileConnector builds a Connector that reads the uploaded file's bytes
// from the object store. It validates the object key before returning so a
// config that references another tenant's object never reaches the store.
func (r *connectorResolver) buildFileConnector(ctx context.Context, src *Source) (Connector, error) {
	cfg, err := parseFileConfig(src.Config)
	if err != nil {
		_ = r.sources.MarkError(ctx, src.TenantID, src.SourceID, err.Error())
		return nil, resolutionFailed(err)
	}
	if cfg.ObjectKey == "" {
		connErr := fmt.Errorf("%w: %s", ErrMissingObjectKey, src.SourceID)
		_ = r.sources.MarkError(ctx, src.TenantID, src.SourceID, connErr.Error())
		return nil, resolutionFailed(connErr)
	}
	if err := validateObjectKey(cfg.ObjectKey); err != nil {
		connErr := fmt.Errorf("%w: %s", err, cfg.ObjectKey)
		_ = r.sources.MarkError(ctx, src.TenantID, src.SourceID, connErr.Error())
		return nil, resolutionFailed(connErr)
	}

	return &FileConnector{
		tenantID:  src.TenantID,
		sourceID:  src.SourceID,
		objectKey: cfg.ObjectKey,
		filename:  cfg.Filename,
		docType:   extensionToDocType(strings.ToLower(extensionOf(cfg.Filename))),
		objects:   r.objects,
	}, nil
}

// fileUploadConfig is the {filename, size, object_key} shape recorded in a
// file_upload source's config.
type fileUploadConfig struct {
	Filename  string `json:"filename"`
	Size      int64  `json:"size"`
	ObjectKey string `json:"object_key"`
}

// parseFileConfig unmarshals a file_upload source's config.
func parseFileConfig(raw json.RawMessage) (fileUploadConfig, error) {
	var cfg fileUploadConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return cfg, fmt.Errorf("invalid file config: %w", err)
	}
	return cfg, nil
}

// extensionOf returns the file extension (including the dot) of a filename.
func extensionOf(filename string) string {
	idx := strings.LastIndex(filename, ".")
	if idx < 0 {
		return ""
	}
	return filename[idx:]
}

// validateObjectKey verifies that a tenant-relative object key cannot escape
// the requesting tenant's prefix. It mirrors the Storage module's key
// enforcement at the resolver boundary so a config referencing another
// tenant's object is rejected before any read.
func validateObjectKey(key string) error {
	if key == "" {
		return ErrInvalidObjectKey
	}
	if strings.HasPrefix(key, "/") || strings.HasPrefix(key, "tenant/") {
		return ErrInvalidObjectKey
	}
	for _, part := range strings.Split(key, "/") {
		if part == ".." {
			return ErrInvalidObjectKey
		}
	}
	return nil
}

// resolutionFailed wraps err with ErrResolutionFailed so the queue can detect
// a permanent resolution failure and skip retry. Both sentinel errors stay
// reachable via errors.Is.
func resolutionFailed(err error) error {
	return errors.Join(ErrResolutionFailed, err)
}
