package knowledge

import (
	"testing"
	"unicode/utf8"

	"github.com/agentic-demo/platform/internal/domain"
)

func TestRecursiveChunker_EmptyInput(t *testing.T) {
	c := NewRecursiveChunker(1000, 200)

	result := c.Chunk(nil)
	if len(result) != 0 {
		t.Errorf("Chunk(nil) = %d chunks, want 0", len(result))
	}

	result = c.Chunk([]domain.RawDocument{})
	if len(result) != 0 {
		t.Errorf("Chunk([]) = %d chunks, want 0", len(result))
	}
}

func TestRecursiveChunker_SingleShortParagraph(t *testing.T) {
	c := NewRecursiveChunker(1000, 200)

	doc := domain.RawDocument{
		Content: "This is a short paragraph.",
		Metadata: map[string]string{
			"source": "test",
		},
	}

	result := c.Chunk([]domain.RawDocument{doc})
	if len(result) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(result))
	}
	if result[0].Content == "" {
		t.Error("chunk content should not be empty")
	}
	if result[0].Source != "test" {
		t.Errorf("chunk source = %q, want %q", result[0].Source, "test")
	}
}

func TestRecursiveChunker_MultipleParagraphs(t *testing.T) {
	c := NewRecursiveChunker(200, 50)

	// Each paragraph is ~100 chars; together they exceed chunkSize (200).
	shortPara := ""
	for i := 0; i < 100; i++ {
		shortPara += "a"
	}

	doc := domain.RawDocument{
		Content:  shortPara + "\n\n" + shortPara + "\n\n" + shortPara,
		Metadata: map[string]string{"source": "test"},
	}

	result := c.Chunk([]domain.RawDocument{doc})
	// 3 paragraphs of ~100 chars each with chunkSize=200 + overlap=50.
	// Two fit per chunk, so expect 2 chunks.
	if len(result) < 2 {
		t.Fatalf("expected at least 2 chunks, got %d", len(result))
	}
	for i, ch := range result {
		if ch.Content == "" {
			t.Errorf("chunk %d content should not be empty", i)
		}
	}
}

func TestRecursiveChunker_LongParagraphSplit(t *testing.T) {
	chunkSize := 100
	c := NewRecursiveChunker(chunkSize, 20)

	// Build a paragraph 2.5x the chunk size.
	longPara := ""
	for i := 0; i < 250; i++ {
		longPara += "x"
	}

	doc := domain.RawDocument{
		Content:  longPara,
		Metadata: map[string]string{"source": "test"},
	}

	result := c.Chunk([]domain.RawDocument{doc})

	if len(result) < 2 {
		t.Fatalf("expected at least 2 chunks, got %d", len(result))
	}

	for i, ch := range result {
		runeCount := utf8.RuneCountInString(ch.Content)
		if runeCount > chunkSize {
			t.Errorf("chunk %d has %d runes, exceeds max %d", i, runeCount, chunkSize)
		}
	}
}

func TestRecursiveChunker_OverlapBetweenChunks(t *testing.T) {
	chunkSize := 100
	overlap := 20
	c := NewRecursiveChunker(chunkSize, overlap)

	// Build text long enough to force at least 2 chunks.
	longText := ""
	for i := 0; i < 300; i++ {
		longText += "x"
	}

	doc := domain.RawDocument{
		Content:  longText,
		Metadata: map[string]string{"source": "test"},
	}

	result := c.Chunk([]domain.RawDocument{doc})

	if len(result) < 2 {
		t.Fatalf("expected at least 2 chunks, got %d", len(result))
	}

	// Check overlap: last N runes of chunk[i] should match first N runes of chunk[i+1].
	for i := 0; i < len(result)-1; i++ {
		cur := []rune(result[i].Content)
		next := []rune(result[i+1].Content)

		if len(cur) < overlap || len(next) < overlap {
			continue
		}

		tail := string(cur[len(cur)-overlap:])
		head := string(next[:overlap])

		if tail != head {
			t.Errorf("overlap missing between chunk %d and %d: tail=%q head=%q", i, i+1, tail, head)
		}
	}
}