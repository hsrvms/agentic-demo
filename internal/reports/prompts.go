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

// BuildContext composes retrieved chunks into a context document for the agent loop.
func BuildContext(chunks []domain.RankedChunk) string {
	if len(chunks) == 0 {
		return "No relevant data found in the knowledge base. Generate a report noting the lack of data."
	}

	var b strings.Builder
	b.WriteString("The following data was retrieved from the knowledge base for this report:\n\n")

	for i, rc := range chunks {
		b.WriteString(fmt.Sprintf("--- Source %d: %s (%s, similarity: %.2f) ---\n",
			i+1, rc.Chunk.Source, rc.Chunk.DocumentType, 1.0-rc.Distance))
		b.WriteString(rc.Chunk.Content)
		b.WriteString("\n\n")
	}

	b.WriteString("\nUse this data to generate the report. Cite sources by number.")
	return b.String()
}
