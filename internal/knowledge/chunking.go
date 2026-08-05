package knowledge

import (
	"strings"
	"unicode/utf8"

	"github.com/agentic-demo/platform/internal/domain"
)

// Chunker splits raw documents into chunks.
type Chunker interface {
	Chunk(docs []domain.RawDocument) []domain.Chunk
}

// RecursiveChunker splits text by paragraph, then by character count.
type RecursiveChunker struct {
	ChunkSize    int // target characters per chunk
	ChunkOverlap int // overlap characters between consecutive chunks
}

func NewRecursiveChunker(chunkSize, overlap int) *RecursiveChunker {
	if chunkSize <= 0 {
		chunkSize = 1000
	}
	if overlap <= 0 {
		overlap = 200
	}
	return &RecursiveChunker{ChunkSize: chunkSize, ChunkOverlap: overlap}
}

func (c *RecursiveChunker) Chunk(docs []domain.RawDocument) []domain.Chunk {
	var chunks []domain.Chunk
	for _, doc := range docs {
		docChunks := c.chunkDocument(doc)
		chunks = append(chunks, docChunks...)
	}
	return chunks
}

func (c *RecursiveChunker) chunkDocument(doc domain.RawDocument) []domain.Chunk {
	text := strings.TrimSpace(doc.Content)
	if text == "" {
		return nil
	}

	// Try splitting by paragraphs first.
	paragraphs := strings.Split(text, "\n\n")

	var chunks []domain.Chunk
	var current strings.Builder

	for _, para := range paragraphs {
		para = strings.TrimSpace(para)
		if para == "" {
			continue
		}

		// If adding this paragraph exceeds chunk size, flush current.
		if current.Len()+utf8.RuneCountInString(para)+2 > c.ChunkSize && current.Len() > 0 {
			chunks = append(chunks, c.makeChunk(current.String(), doc))
			// Overlap: keep the tail of the previous chunk.
			tail := c.getTail(current.String())
			current.Reset()
			current.WriteString(tail)
		}

		// If a single paragraph exceeds chunk size, split it by characters.
		if utf8.RuneCountInString(para) > c.ChunkSize {
			// Flush current buffer first.
			if current.Len() > 0 {
				chunks = append(chunks, c.makeChunk(current.String(), doc))
				current.Reset()
			}
			// Split the large paragraph.
			subChunks := c.splitBySize(para, doc)
			chunks = append(chunks, subChunks...)
			continue
		}

		if current.Len() > 0 {
			current.WriteString("\n\n")
		}
		current.WriteString(para)
	}

	// Flush remaining.
	if current.Len() > 0 {
		chunks = append(chunks, c.makeChunk(current.String(), doc))
	}

	return chunks
}

func (c *RecursiveChunker) splitBySize(text string, doc domain.RawDocument) []domain.Chunk {
	runes := []rune(text)
	var chunks []domain.Chunk
	for i := 0; i < len(runes); i += c.ChunkSize - c.ChunkOverlap {
		end := i + c.ChunkSize
		if end > len(runes) {
			end = len(runes)
		}
		chunk := string(runes[i:end])
		chunks = append(chunks, c.makeChunk(chunk, doc))
		if end == len(runes) {
			break
		}
	}
	return chunks
}

func (c *RecursiveChunker) getTail(text string) string {
	runes := []rune(text)
	if len(runes) <= c.ChunkOverlap {
		return text
	}
	return string(runes[len(runes)-c.ChunkOverlap:])
}

func (c *RecursiveChunker) makeChunk(content string, doc domain.RawDocument) domain.Chunk {
	// Copy metadata from the source document.
	metadata := make(map[string]string, len(doc.Metadata))
	for k, v := range doc.Metadata {
		metadata[k] = v
	}

	return domain.Chunk{
		Content:      content,
		Source:       doc.Metadata["source"],
		DocumentType: doc.Metadata["document_type"],
		DocumentID:   doc.ID,
		Metadata:     metadata,
	}
}
