package ingestion

import (
	"context"
	"fmt"
	"io"

	"github.com/agentic-demo/platform/internal/domain"
)

// Connector extracts raw documents from a data source.
// Each connector type (file, web, CRM) implements this interface.
// This is an internal seam within the Ingestion module.
type Connector interface {
	Extract(ctx context.Context) ([]domain.RawDocument, error)
}

// FileConnector reads an uploaded file's bytes from the object store and
// extracts its content as a single document. It never reads from a local
// path — the bytes live in the tenant-scoped object store, so the connector
// survives worker restarts and is tenant-isolated by construction.
type FileConnector struct {
	tenantID  domain.TenantID
	sourceID  string
	objectKey string // tenant-relative key into the object store
	filename  string
	docType   string
	objects   ObjectReader
}

// Extract reads the object referenced by the config and returns it as a single
// RawDocument. The document ID is the source ID, stable across re-ingestion so
// a source's documents can be replaced without accumulating duplicates.
func (c *FileConnector) Extract(ctx context.Context) ([]domain.RawDocument, error) {
	// Defense-in-depth: never read a key that could escape the tenant's prefix.
	if err := validateObjectKey(c.objectKey); err != nil {
		return nil, fmt.Errorf("%w: %s", err, c.objectKey)
	}

	rc, err := c.objects.Get(ctx, c.tenantID, c.objectKey)
	if err != nil {
		return nil, fmt.Errorf("read file %s: %w", c.objectKey, err)
	}
	defer rc.Close()

	content, err := io.ReadAll(rc)
	if err != nil {
		return nil, fmt.Errorf("read file %s: %w", c.objectKey, err)
	}

	return []domain.RawDocument{
		{
			ID:      c.sourceID,
			Content: string(content),
			Metadata: map[string]string{
				"source":        c.filename,
				"document_type": c.docType,
				"file_path":     c.objectKey,
			},
		},
	}, nil
}

func extensionToDocType(ext string) string {
	switch ext {
	case ".pdf":
		return "pdf"
	case ".docx":
		return "docx"
	case ".csv":
		return "csv"
	case ".xlsx", ".xls":
		return "xlsx"
	case ".txt", ".md":
		return "text"
	default:
		return "unknown"
	}
}
