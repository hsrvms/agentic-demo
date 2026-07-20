package ingestion

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/agentic-demo/platform/internal/domain"
)

// Connector extracts raw documents from a data source.
// Each connector type (file, web, CRM) implements this interface.
// This is an internal seam within the Ingestion module.
type Connector interface {
	Extract(ctx context.Context) ([]domain.RawDocument, error)
}

// FileConnector reads a file and extracts its content as a single document.
// Phase 0 supports plain text and CSV. PDF/DOCX added in Phase 1.
type FileConnector struct {
	Path string
}

func (c *FileConnector) Extract(ctx context.Context) ([]domain.RawDocument, error) {
	content, err := os.ReadFile(c.Path)
	if err != nil {
		return nil, fmt.Errorf("read file %s: %w", c.Path, err)
	}

	ext := strings.ToLower(filepath.Ext(c.Path))
	docType := extensionToDocType(ext)
	filename := filepath.Base(c.Path)

	return []domain.RawDocument{
		{
			Content: string(content),
			Metadata: map[string]string{
				"source":        filename,
				"document_type": docType,
				"file_path":     c.Path,
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
