package reports

import (
	"strings"
	"testing"

	"github.com/agentic-demo/platform/internal/domain"
)

// TestBuildContext_Empty tests the empty-context fallback message.
func TestBuildContext_Empty(t *testing.T) {
	got := BuildContext(nil)
	if !strings.Contains(got, "No relevant data found") {
		t.Errorf("empty context should mention lack of data, got: %q", got)
	}
}

// TestBuildContext_IncludesFullDocument verifies the context includes the full
// document content behind a matched chunk, not just the fragment.
func TestBuildContext_IncludesFullDocument(t *testing.T) {
	results := []RetrievedContext{
		{
			Source:       "financials",
			DocumentType: "report",
			Similarity:   0.9,
			Document: domain.Document{
				ID:      "doc-1",
				Source:  "financials",
				Content: "The full annual report text with many more details than the matched fragment.",
			},
		},
	}

	got := BuildContext(results)

	if !strings.Contains(got, "The full annual report text") {
		t.Errorf("context should include full document content, got: %q", got)
	}
	if !strings.Contains(got, "financials") {
		t.Errorf("context should include the source name, got: %q", got)
	}
	if !strings.Contains(got, "report") {
		t.Errorf("context should include the document type, got: %q", got)
	}
}

// TestBuildContext_FormatsSourceAndSimilarity verifies the per-source header.
func TestBuildContext_FormatsSourceAndSimilarity(t *testing.T) {
	results := []RetrievedContext{
		{
			Source:       "crm",
			DocumentType: "text",
			Similarity:   0.75,
			Document: domain.Document{
				ID:      "doc-2",
				Source:  "crm",
				Content: "CRM content",
			},
		},
	}

	got := BuildContext(results)

	if !strings.Contains(got, "Source 1: crm (text, similarity: 0.75)") {
		t.Errorf("header should render source, type and similarity, got: %q", got)
	}
}

// TestBuildContext_HasTrailingCitationInstruction verifies the citation prompt.
func TestBuildContext_HasTrailingCitationInstruction(t *testing.T) {
	got := BuildContext([]RetrievedContext{{
		Document: domain.Document{Content: "content"},
	}})
	if !strings.Contains(got, "Cite sources by number") {
		t.Errorf("context should instruct citing sources by number, got: %q", got)
	}
}
