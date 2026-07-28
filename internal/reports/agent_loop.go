package reports

import (
	"context"
	"fmt"
	"strings"

	"github.com/agentic-demo/platform/internal/domain"
	"github.com/agentic-demo/platform/internal/llm"
	"github.com/agentic-demo/platform/internal/tools"
	"github.com/agentic-demo/platform/internal/usage"
)

// AgentResult is the output of the agent loop.
type AgentResult struct {
	Content  string
	Metadata map[string]string
}

// AgentLoop executes the bounded LLM reasoning loop.
//
// Protocol:
//
//	Phase 1: Context gathering (done by caller, passed in)
//	Phase 2: LLM reasoning loop with optional tool calls
//	Phase 3: Synthesis — final report generation
//
// Termination conditions (any one):
//   - LLM returns text-only response (no tool calls)
//   - Max tool calls reached
//   - Max LLM calls reached
//   - Context deadline exceeded
//   - Duplicate tool call detected
type AgentLoop struct {
	llmClient    llm.LLMClient
	tools        tools.ToolRegistry
	emitter      usage.UsageEmitter
	maxToolCalls int
	maxLLMCalls  int
}

func (a *AgentLoop) Run(ctx context.Context, tenantID domain.TenantID, config domain.ReportConfig, gatheredContext string) (AgentResult, error) {
	// Build initial messages.
	systemPrompt := buildSystemPrompt(config)
	msgs := []domain.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: gatheredContext},
	}

	// Get available tools for this tenant.
	toolSchemas := a.tools.ListTools(ctx, tenantID)

	opts := llm.Options{
		TenantID:    tenantID,
		Temperature: 0.3, // Low temperature for factual reports.
		ToolSchemas: toolSchemas,
	}

	var toolCallCount int
	var llmCallCount int
	// Track tool calls for duplicate detection: "name:params_hash" → true.
	toolCallHistory := make(map[string]bool)

	// Phase 2: Agent loop.
	for llmCallCount < a.maxLLMCalls {
		result, err := a.llmClient.Complete(ctx, msgs, opts)
		if err != nil {
			return AgentResult{}, fmt.Errorf("LLM call %d: %w", llmCallCount+1, err)
		}
		llmCallCount++

		// Emit usage.
		_ = a.emitter.EmitUsage(ctx, usage.UsageEvent{
			Type:     usage.EventLLM,
			TenantID: tenantID,
			LLM: &usage.LLMUsage{
				Model:        result.Model,
				InputTokens:  result.InputTokens,
				OutputTokens: result.OutputTokens,
			},
		})

		// If no tool calls, the LLM is done reasoning — this is the final answer.
		if len(result.ToolCalls) == 0 {
			return AgentResult{
				Content: result.Text,
				Metadata: map[string]string{
					"llm_calls":     fmt.Sprintf("%d", llmCallCount),
					"tool_calls":    fmt.Sprintf("%d", toolCallCount),
					"input_tokens":  fmt.Sprintf("%d", result.InputTokens),
					"output_tokens": fmt.Sprintf("%d", result.OutputTokens),
				},
			}, nil
		}

		// Process tool calls.
		// Append the assistant's response (with tool calls) to messages.
		msgs = append(msgs, domain.Message{
			Role:      "assistant",
			ToolCalls: result.ToolCalls,
		})

		for _, tc := range result.ToolCalls {
			if toolCallCount >= a.maxToolCalls {
				// Max tool calls reached — force synthesis.
				return a.synthesize(ctx, msgs, opts, tenantID, llmCallCount, toolCallCount)
			}

			// Duplicate detection.
			callKey := fmt.Sprintf("%s:%v", tc.Name, tc.Params)
			if toolCallHistory[callKey] {
				// Return error as tool result instead of executing.
				msgs = append(msgs, domain.Message{
					Role:       "tool",
					ToolCallID: tc.ID,
					Content:    `{"error": "duplicate_call", "message": "This call was already made with the same parameters."}`,
				})
				continue
			}
			toolCallHistory[callKey] = true

			// Invoke the tool.
			toolResult := a.tools.Invoke(ctx, tenantID, tc.Name, tc.Params)

			// Emit tool usage.
			_ = a.emitter.EmitUsage(ctx, usage.UsageEvent{
				Type:     usage.EventTool,
				TenantID: tenantID,
				Tool: &usage.ToolUsage{
					ToolName: tc.Name,
					Success:  toolResult.Error == "",
				},
			})

			// Append tool result to messages.
			content := toolResult.Output
			if toolResult.Error != "" {
				content = fmt.Sprintf(`{"error": "%s failed: %s"}`, tc.Name, toolResult.Error)
			}
			msgs = append(msgs, domain.Message{
				Role:       "tool",
				ToolCallID: tc.ID,
				Content:    content,
			})

			toolCallCount++
		}
	}

	// Max LLM calls reached — force synthesis with what we have.
	return a.synthesize(ctx, msgs, opts, tenantID, llmCallCount, toolCallCount)
}

// synthesize makes a final LLM call asking for the report, without tools.
func (a *AgentLoop) synthesize(ctx context.Context, msgs []domain.Message, opts llm.Options, tenantID domain.TenantID, llmCalls, toolCalls int) (AgentResult, error) {
	// Remove tools to force text-only response.
	synthOpts := opts
	synthOpts.ToolSchemas = nil

	synthMsgs := append(msgs, domain.Message{
		Role:    "user",
		Content: "You have gathered enough context. Please produce the final report now. Include source citations.",
	})

	result, err := a.llmClient.Complete(ctx, synthMsgs, synthOpts)
	if err != nil {
		return AgentResult{}, fmt.Errorf("synthesis call: %w", err)
	}

	return AgentResult{
		Content: result.Text,
		Metadata: map[string]string{
			"llm_calls":     fmt.Sprintf("%d", llmCalls+1),
			"tool_calls":    fmt.Sprintf("%d", toolCalls),
			"synthesized":   "true",
			"input_tokens":  fmt.Sprintf("%d", result.InputTokens),
			"output_tokens": fmt.Sprintf("%d", result.OutputTokens),
		},
	}, nil
}

func buildSystemPrompt(config domain.ReportConfig) string {
	var b strings.Builder
	b.WriteString("You are a strategic business analyst. Generate a ")
	b.WriteString(string(config.Type))
	b.WriteString(" strategic report based on the provided data.\n\n")
	b.WriteString("Guidelines:\n")
	b.WriteString("- Ground every claim in the provided data\n")
	b.WriteString("- Include source citations\n")
	b.WriteString("- Be specific with numbers and trends\n")
	b.WriteString("- Provide actionable recommendations\n")

	if len(config.FocusAreas) > 0 {
		b.WriteString("\nFocus areas: ")
		b.WriteString(strings.Join(config.FocusAreas, ", "))
	}

	switch config.Type {
	case domain.ReportDaily:
		b.WriteString("\n\nEmphasize recent signals, anomalies, and immediate action items.")
	case domain.ReportWeekly:
		b.WriteString("\n\nEmphasize trends, patterns compared to the prior week, and medium-term implications.")
	case domain.ReportMonthly:
		b.WriteString("\n\nEmphasize strategic trends, long-term patterns, and high-level recommendations.")
	}

	return b.String()
}
