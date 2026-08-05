package reports

import (
	"fmt"
	"strings"

	"github.com/agentic-demo/platform/internal/domain"
)

// BuildPrompt creates a query string for context gathering based on report type.
func BuildPrompt(config domain.ReportConfig) string {
	var parts []string

	switch config.Type {
	case domain.ReportDaily:
		parts = append(parts, "recent signals, anomalies, key business metrics, immediate action items")
	case domain.ReportWeekly:
		parts = append(parts, "weekly trends, patterns compared to prior week, emerging opportunities")
	case domain.ReportMonthly:
		parts = append(parts, "strategic trends, long-term patterns, high-level recommendations, market shifts")
	case domain.ReportOnDemand:
		parts = append(parts, "key business data, relevant insights")
	}

	if len(config.FocusAreas) > 0 {
		parts = append(parts, "focus on: "+strings.Join(config.FocusAreas, ", "))
	}

	return strings.Join(parts, "; ")
}

// RetrievedContext pairs a matched chunk with the full document it came from.
// Report generation expands each matched chunk to its complete document so the
// LLM reasons over full context rather than a fragment.
type RetrievedContext struct {
	Source       string
	DocumentType string
	Similarity   float64
	Document     domain.Document
}

// BuildContext composes retrieved documents into a context document for the
// agent loop. Each entry carries the full document behind a matched chunk, not
// just the matched fragment.
func BuildContext(results []RetrievedContext) string {
	if len(results) == 0 {
		return "No relevant data found in the knowledge base. Generate a report noting the lack of data."
	}

	var b strings.Builder
	b.WriteString("The following data was retrieved from the knowledge base for this report:\n\n")

	for i, rc := range results {
		fmt.Fprintf(&b, "--- Source %d: %s (%s, similarity: %.2f) ---\n",
			i+1, rc.Source, rc.DocumentType, rc.Similarity)
		b.WriteString(rc.Document.Content)
		b.WriteString("\n\n")
	}

	b.WriteString("\nUse this data to generate the report. Cite sources by number.")
	return b.String()
}
