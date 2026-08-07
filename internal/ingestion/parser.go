// Document parsing seam.
//
// A FileConnector reads raw bytes from the object store; before those bytes
// can be chunked they must become clean text. The DocumentParser is the seam
// that turns a file's detected type + bytes into text: binary formats (PDF,
// DOCX, XLSX) are parsed by a dedicated implementation, text formats pass
// through unchanged, and anything else fails with ErrUnsupportedDocumentType.
package ingestion

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/ledongthuc/pdf"
	"github.com/xuri/excelize/v2"
)

// Document types a file can resolve to. These are the values recorded in a
// RawDocument's document_type metadata.
const (
	docTypePDF     = "pdf"
	docTypeDOCX    = "docx"
	docTypeXLSX    = "xlsx"
	docTypeText    = "text"
	docTypeCSV     = "csv"
	docTypeUnknown = "unknown"
)

// Sentinel errors returned by the parser seam.
var (
	// ErrUnsupportedDocumentType is returned when a document type has no
	// parser implementation (e.g. legacy .xls or an unknown extension).
	ErrUnsupportedDocumentType = errors.New("unsupported document type")
	// ErrDocumentTooLarge is returned when a document archive claims an
	// uncompressed size beyond the parser's cap (zip-bomb guard).
	ErrDocumentTooLarge = errors.New("document too large")
)

// DocumentParser turns a file's detected type and raw bytes into clean text.
// It is the seam the FileConnector consumes; implementations are selected by
// the document type, never by the caller.
type DocumentParser interface {
	Parse(docType string, data []byte) (string, error)
}

// documentParser is the single DocumentParser implementation. It dispatches on
// the detected document type, so the connector stays free of format knowledge.
type documentParser struct{}

// NewDocumentParser builds a DocumentParser with one implementation per
// supported document type.
func NewDocumentParser() DocumentParser {
	return &documentParser{}
}

func (p *documentParser) Parse(docType string, data []byte) (string, error) {
	switch docType {
	case docTypePDF:
		return parsePDF(data)
	case docTypeDOCX:
		return parseDOCX(data)
	case docTypeXLSX:
		return parseXLSX(data)
	case docTypeText, docTypeCSV:
		return string(data), nil
	default:
		return "", fmt.Errorf("%w: %s", ErrUnsupportedDocumentType, docType)
	}
}

// parsePDF extracts the text of every page and joins the non-empty pages.
// Page extraction returns raw content-stream text; each page is trimmed and
// pages are separated by a blank line so the chunker sees page boundaries.
func parsePDF(data []byte) (string, error) {
	r, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", fmt.Errorf("open pdf: %w", err)
	}

	var pages []string
	for i := 1; i <= r.NumPage(); i++ {
		text, err := r.Page(i).GetPlainText(nil)
		if err != nil {
			return "", fmt.Errorf("extract page %d: %w", i, err)
		}
		if trimmed := strings.TrimSpace(text); trimmed != "" {
			pages = append(pages, trimmed)
		}
	}
	return strings.Join(pages, "\n\n"), nil
}

// Cap on the uncompressed size of any single archive entry and on the number
// of entries, so a malicious DOCX (zip bomb) is rejected before decompression
// work happens. 64 MiB of XML text is far beyond any real document.
const (
	maxDOCXUncompressed = 64 << 20 // per entry, bytes
	maxDOCXEntries      = 1024
)

// wordprocessingMLNS is the namespace of the main document part. Parsing
// matches the prefix so transitional variants of the URL keep working.
const wordprocessingMLNS = "http://schemas.openxmlformats.org/wordprocessingml/"

// parseDOCX extracts text from a DOCX's main document part. Paragraphs (w:p)
// become lines built from their text runs (w:t); a tab (w:tab) becomes a tab
// character. Paragraphs inside the same table row are tab-joined so table
// cells read as columns; all other paragraphs are newline-separated. Headers,
// footers, and text boxes (separate XML parts) are out of scope.
func parseDOCX(data []byte) (string, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", fmt.Errorf("open docx archive: %w", err)
	}
	if len(zr.File) > maxDOCXEntries {
		return "", fmt.Errorf("%w: %d archive entries", ErrDocumentTooLarge, len(zr.File))
	}

	var document *zip.File
	for _, f := range zr.File {
		if f.UncompressedSize64 > maxDOCXUncompressed {
			return "", fmt.Errorf("%w: %s declares %d bytes", ErrDocumentTooLarge, f.Name, f.UncompressedSize64)
		}
		if f.Name == "word/document.xml" {
			document = f
		}
	}
	if document == nil {
		return "", fmt.Errorf("docx has no word/document.xml part")
	}

	rc, err := document.Open()
	if err != nil {
		return "", fmt.Errorf("open word/document.xml: %w", err)
	}
	defer rc.Close()

	dec := xml.NewDecoder(io.LimitReader(rc, maxDOCXUncompressed+1))

	type para struct {
		text string
		row  int // 0 = outside any table row
	}
	var paras []para
	rowID := 0
	inParagraph := false
	var text strings.Builder

	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("parse word/document.xml: %w", err)
		}

		switch t := tok.(type) {
		case xml.StartElement:
			if !isWordprocessingML(t.Name) {
				continue
			}
			switch t.Name.Local {
			case "p":
				inParagraph = true
			case "tr":
				rowID++
			case "t":
				if inParagraph {
					var s string
					if err := dec.DecodeElement(&s, &t); err != nil {
						return "", fmt.Errorf("parse word/document.xml: %w", err)
					}
					text.WriteString(s)
				}
			case "tab":
				if inParagraph {
					text.WriteString("\t")
				}
			}
		case xml.EndElement:
			if isWordprocessingML(t.Name) && t.Name.Local == "p" && inParagraph {
				if trimmed := strings.TrimSpace(text.String()); trimmed != "" {
					paras = append(paras, para{text: trimmed, row: rowID})
				}
				text.Reset()
				inParagraph = false
			}
		}
	}

	var out strings.Builder
	for i, p := range paras {
		if i == 0 {
			out.WriteString(p.text)
			continue
		}
		if p.row != 0 && p.row == paras[i-1].row {
			out.WriteString("\t")
		} else {
			out.WriteString("\n")
		}
		out.WriteString(p.text)
	}
	return out.String(), nil
}

// isWordprocessingML reports whether an element belongs to the WordprocessingML
// namespace. An empty namespace is tolerated so hand-written fixtures and
// namespace-stripped documents still parse.
func isWordprocessingML(name xml.Name) bool {
	return name.Space == "" || strings.HasPrefix(name.Space, wordprocessingMLNS)
}

// parseXLSX extracts every sheet's cells as tab-separated rows. Each sheet is
// introduced by a heading line so retrieved chunks carry their sheet context.
func parseXLSX(data []byte) (string, error) {
	f, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("open xlsx: %w", err)
	}
	defer f.Close()

	sheets := f.GetSheetList()
	if len(sheets) == 0 {
		return "", fmt.Errorf("xlsx has no sheets")
	}

	var blocks []string
	for _, sheet := range sheets {
		rows, err := f.Rows(sheet)
		if err != nil {
			return "", fmt.Errorf("read sheet %s: %w", sheet, err)
		}
		var rowsText []string
		for rows.Next() {
			cells, err := rows.Columns()
			if err != nil {
				return "", fmt.Errorf("read row in sheet %s: %w", sheet, err)
			}
			joined := strings.Join(cells, "\t")
			if strings.TrimSpace(joined) != "" {
				rowsText = append(rowsText, joined)
			}
		}
		if err := rows.Error(); err != nil {
			return "", fmt.Errorf("read sheet %s: %w", sheet, err)
		}
		blocks = append(blocks, "Sheet: "+sheet+"\n"+strings.Join(rowsText, "\n"))
	}
	return strings.TrimSpace(strings.Join(blocks, "\n\n")), nil
}
