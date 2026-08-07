package ingestion

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// --- Fixture builders -------------------------------------------------------
//
// Fixtures are built from scratch in test code (never from the library under
// test) so the expected text is an independent source of truth: a minimal PDF
// with correct xref offsets, a minimal DOCX with paragraphs and a table, and a
// minimal XLSX using both inline and shared strings.

// buildMinimalPDF returns a PDF with one page per given text, each page
// rendering its text with the standard Helvetica font.
func buildMinimalPDF(texts ...string) []byte {
	if len(texts) == 0 {
		texts = []string{""}
	}

	// Object layout: 1 catalog, 2 pages, N page dicts, N content streams,
	// then one font. Object numbers are 1-based.
	n := len(texts)
	fontObj := 3 + 2*n
	lastObj := fontObj
	pageObjs := make([]int, n)
	streamObjs := make([]int, n)
	kids := make([]string, n)
	for i := 0; i < n; i++ {
		pageObjs[i] = 3 + i
		streamObjs[i] = 3 + n + i
		kids[i] = fmt.Sprintf("%d 0 R", pageObjs[i])
	}

	var buf bytes.Buffer
	buf.WriteString("%PDF-1.4\n")
	offsets := make([]int, lastObj+1)
	write := func(obj int, s string) {
		offsets[obj] = buf.Len()
		fmt.Fprintf(&buf, "%d 0 obj\n%sendobj\n", obj, s)
	}

	write(1, "<< /Type /Catalog /Pages 2 0 R >>\n")
	write(2, fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>\n", strings.Join(kids, " "), n))
	for i := 0; i < n; i++ {
		write(pageObjs[i], fmt.Sprintf(
			"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents %d 0 R /Resources << /Font << /F1 %d 0 R >> >> >>\n",
			streamObjs[i], fontObj))
		stream := "BT /F1 12 Tf 72 720 Td (" + texts[i] + ") Tj ET\n"
		write(streamObjs[i], fmt.Sprintf("<< /Length %d >>\nstream\n%sendstream\n", len(stream), stream))
	}
	write(fontObj, "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>\n")

	return finishPDF(&buf, offsets, lastObj)
}

// finishPDF appends the xref table, trailer, and EOF marker for a PDF whose
// objects were written with recorded offsets, and returns the complete bytes.
func finishPDF(buf *bytes.Buffer, offsets []int, lastObj int) []byte {
	xrefOffset := buf.Len()
	fmt.Fprintf(buf, "xref\n0 %d\n0000000000 65535 f \n", lastObj+1)
	for i := 1; i <= lastObj; i++ {
		fmt.Fprintf(buf, "%010d 00000 n \n", offsets[i])
	}
	buf.WriteString("trailer\n")
	fmt.Fprintf(buf, "<< /Size %d /Root 1 0 R >>\nstartxref\n", lastObj+1)
	fmt.Fprintf(buf, "%d\n%%%%EOF\n", xrefOffset)
	return buf.Bytes()
}

// buildMinimalDOCX returns a DOCX with two paragraphs and a one-row, two-cell
// table. The table cell paragraphs are the expected text-extraction target.
func buildMinimalDOCX() []byte {
	files := map[string]string{
		"[Content_Types].xml": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
<Default Extension="xml" ContentType="application/xml"/>
<Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>
</Types>`,
		"_rels/.rels": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>
</Relationships>`,
		"word/document.xml": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
<w:body>
<w:p><w:r><w:t>First paragraph</w:t></w:r></w:p>
<w:p><w:r><w:t>Second paragraph</w:t></w:r></w:p>
<w:tbl><w:tr><w:tc><w:p><w:r><w:t>Cell A1</w:t></w:r></w:p></w:tc><w:tc><w:p><w:r><w:t>Cell B1</w:t></w:r></w:p></w:tc></w:tr></w:tbl>
</w:body>
</w:document>`,
	}
	return zipBytes(files)
}

// buildMinimalXLSX returns a workbook with one sheet: a header row of inline
// strings and a data row mixing a shared string and a number.
func buildMinimalXLSX() []byte {
	files := map[string]string{
		"[Content_Types].xml": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
<Default Extension="xml" ContentType="application/xml"/>
<Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/>
<Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>
<Override PartName="/xl/sharedStrings.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sharedStrings+xml"/>
</Types>`,
		"_rels/.rels": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/>
</Relationships>`,
		"xl/workbook.xml": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">
<sheets><sheet name="Sheet1" sheetId="1" r:id="rId1"/></sheets>
</workbook>`,
		"xl/_rels/workbook.xml.rels": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/>
<Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/sharedStrings" Target="sharedStrings.xml"/>
</Relationships>`,
		"xl/sharedStrings.xml": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<sst xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" count="1" uniqueCount="1">
<si><t>Alice</t></si>
</sst>`,
		"xl/worksheets/sheet1.xml": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">
<sheetData>
<row r="1"><c r="A1" t="inlineStr"><is><t>Name</t></is></c><c r="B1" t="inlineStr"><is><t>Score</t></is></c></row>
<row r="2"><c r="A2" t="s"><v>0</v></c><c r="B2"><v>42</v></c></row>
</sheetData>
</worksheet>`,
	}
	return zipBytes(files)
}

// buildWordPerObjectPDF returns a PDF that emits each word as its own text
// object, advancing the text line (T*) after every word — the shape of
// web-generated PDFs (e.g. QMK docs) that pure-Go extractors render
// word-per-line with blank lines between words.
func buildWordPerObjectPDF(words ...string) []byte {
	if len(words) == 0 {
		words = []string{""}
	}

	// Object layout: 1 catalog, 2 pages, 1 page dict, 1 content stream, font.
	fontObj := 7
	var buf bytes.Buffer
	buf.WriteString("%PDF-1.4\n")
	offsets := make([]int, fontObj+1)
	write := func(obj int, s string) {
		offsets[obj] = buf.Len()
		fmt.Fprintf(&buf, "%d 0 obj\n%sendobj\n", obj, s)
	}

	write(1, "<< /Type /Catalog /Pages 2 0 R >>\n")
	write(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>\n")
	var content strings.Builder
	y := 720.0
	for _, w := range words {
		// T* after each word emits a line advance, so the extractor inserts a
		// second newline — the blank-line artifact seen in real docs PDFs.
		fmt.Fprintf(&content, "BT /F1 12 Tf 72 %.0f Td (%s) Tj T* ET\n", y, w)
		y -= 14
	}
	write(3, fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 4 0 R /Resources << /Font << /F1 %d 0 R >> >> >>\n", fontObj))
	stream := content.String()
	write(4, fmt.Sprintf("<< /Length %d >>\nstream\n%sendstream\n", len(stream), stream))
	write(fontObj, "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>\n")

	return finishPDF(&buf, offsets, fontObj)
}

// zipBytes packs the given files into a zip archive.
func zipBytes(files map[string]string) []byte {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			panic(err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			panic(err)
		}
	}
	if err := zw.Close(); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

// --- Parser seam tests ------------------------------------------------------

func TestParsePDF_ExtractsText(t *testing.T) {
	parser := NewDocumentParser()

	text, err := parser.Parse(docTypePDF, buildMinimalPDF("Hello PDF world"))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if text != "Hello PDF world" {
		t.Errorf("extracted text = %q, want %q", text, "Hello PDF world")
	}
}

func TestParsePDF_MultiPageJoinsPages(t *testing.T) {
	parser := NewDocumentParser()

	text, err := parser.Parse(docTypePDF, buildMinimalPDF("Page one text", "Page two text"))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if text != "Page one text\n\nPage two text" {
		t.Errorf("extracted text = %q, want %q", text, "Page one text\n\nPage two text")
	}
}

func TestParseDOCX_ExtractsParagraphsAndTable(t *testing.T) {
	parser := NewDocumentParser()

	text, err := parser.Parse(docTypeDOCX, buildMinimalDOCX())
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	want := "First paragraph\nSecond paragraph\nCell A1\tCell B1"
	if text != want {
		t.Errorf("extracted text = %q, want %q", text, want)
	}
}

func TestParseDOCX_TooLargeRejected(t *testing.T) {
	parser := NewDocumentParser()

	// Build a normal zip, then corrupt the central directory entry for
	// document.xml so it claims an absurd uncompressed size — the shape of a
	// real zip bomb. The parser must reject it before any decompression.
	raw := zipBytes(map[string]string{
		"word/document.xml": "<w:document/>",
	})
	// The filename appears twice (local header and central directory); the
	// central directory entry is the last occurrence.
	idx := bytes.LastIndex(raw, []byte("word/document.xml"))
	if idx < 0 {
		t.Fatal("fixture missing document.xml entry")
	}
	// Central directory entry: 46 fixed bytes before the filename. The
	// uncompressed size sits at offset 24 within the entry (little-endian).
	entry := idx - 46
	if entry < 0 || binary.LittleEndian.Uint32(raw[entry:entry+4]) != 0x02014b50 {
		t.Fatal("central directory entry not found")
	}
	binary.LittleEndian.PutUint32(raw[entry+24:entry+28], 1<<30)

	_, err := parser.Parse(docTypeDOCX, raw)
	if !errors.Is(err, ErrDocumentTooLarge) {
		t.Fatalf("expected ErrDocumentTooLarge, got %v", err)
	}
}

func TestParsePDF_WordPerObjectJoinsIntoParagraphs(t *testing.T) {
	parser := NewDocumentParser()

	// The reported symptom: a web-generated PDF whose words each arrive as
	// their own line (with blank lines between), which must be rebuilt into
	// prose paragraphs broken at sentence ends.
	text, err := parser.Parse(docTypePDF, buildWordPerObjectPDF(
		"Introduction", "to", "QMK", "Firmware.",
		"It", "powers", "custom", "keyboards.",
	))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	want := "Introduction to QMK Firmware.\n\nIt powers custom keyboards."
	if text != want {
		t.Errorf("extracted text = %q, want %q", text, want)
	}
}

func TestParsePDF_HyphenatedWrapJoins(t *testing.T) {
	parser := NewDocumentParser()

	// A word split across lines (open- / source) must rejoin without a space.
	text, err := parser.Parse(docTypePDF, buildWordPerObjectPDF("open-", "source", "project."))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if text != "open-source project." {
		t.Errorf("extracted text = %q, want %q", text, "open-source project.")
	}
}

func TestParsePDF_WellBehavedPDFUnchanged(t *testing.T) {
	parser := NewDocumentParser()

	// A PDF that already emits whole lines must not be mangled: a single
	// sentence stays one paragraph.
	text, err := parser.Parse(docTypePDF, buildMinimalPDF("Quarterly revenue grew by twenty percent."))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if text != "Quarterly revenue grew by twenty percent." {
		t.Errorf("extracted text = %q, want %q", text, "Quarterly revenue grew by twenty percent.")
	}
}

func TestParseXLSX_ExtractsRows(t *testing.T) {
	parser := NewDocumentParser()

	text, err := parser.Parse(docTypeXLSX, buildMinimalXLSX())
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	want := "Sheet: Sheet1\nName\tScore\nAlice\t42"
	if text != want {
		t.Errorf("extracted text = %q, want %q", text, want)
	}
}

func TestParse_TextAndCSVPassThrough(t *testing.T) {
	parser := NewDocumentParser()

	text, err := parser.Parse(docTypeText, []byte("hello\nworld"))
	if err != nil {
		t.Fatalf("Parse(text) failed: %v", err)
	}
	if text != "hello\nworld" {
		t.Errorf("text passthrough = %q, want %q", text, "hello\nworld")
	}

	csv, err := parser.Parse(docTypeCSV, []byte("a,b\n1,2"))
	if err != nil {
		t.Fatalf("Parse(csv) failed: %v", err)
	}
	if csv != "a,b\n1,2" {
		t.Errorf("csv passthrough = %q, want %q", csv, "a,b\n1,2")
	}
}

func TestParse_UnsupportedType(t *testing.T) {
	parser := NewDocumentParser()

	_, err := parser.Parse(docTypeUnknown, []byte("not a pdf"))
	if !errors.Is(err, ErrUnsupportedDocumentType) {
		t.Fatalf("expected ErrUnsupportedDocumentType, got %v", err)
	}
}
