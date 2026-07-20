package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/agentic-demo/platform/internal/domain"
)

// DashScopeProvider implements Provider for the DashScope API
// (OpenAI-compatible endpoint).
type DashScopeProvider struct {
	apiKey  string
	baseURL string
	model   string
	http    *http.Client
}

func NewDashScopeProvider(apiKey, baseURL, model string) *DashScopeProvider {
	return &DashScopeProvider{
		apiKey:  apiKey,
		baseURL: baseURL,
		model:   model,
		http:    &http.Client{Timeout: 120 * time.Second},
	}
}

func (p *DashScopeProvider) Name() string { return "dashscope" }

func (p *DashScopeProvider) Chat(ctx context.Context, msgs []domain.Message, opts Options) (domain.CompletionResult, error) {
	model := opts.Model
	if model == "" {
		model = p.model
	}

	reqBody := chatRequest{
		Model:    model,
		Messages: toAPIMessages(msgs),
	}
	if opts.MaxTokens > 0 {
		reqBody.MaxTokens = opts.MaxTokens
	}
	if opts.Temperature > 0 {
		reqBody.Temperature = opts.Temperature
	}
	if len(opts.ToolSchemas) > 0 {
		reqBody.Tools = toAPITools(opts.ToolSchemas)
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return domain.CompletionResult{}, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return domain.CompletionResult{}, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)

	start := time.Now()
	resp, err := p.http.Do(req)
	if err != nil {
		return domain.CompletionResult{}, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()
	latency := time.Since(start)

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return domain.CompletionResult{}, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return domain.CompletionResult{}, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var chatResp chatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return domain.CompletionResult{}, fmt.Errorf("unmarshal response: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return domain.CompletionResult{}, fmt.Errorf("empty response from model")
	}

	choice := chatResp.Choices[0]
	result := domain.CompletionResult{
		Text:         choice.Message.Content,
		InputTokens:  chatResp.Usage.PromptTokens,
		OutputTokens: chatResp.Usage.CompletionTokens,
		Model:        chatResp.Model,
		Latency:      latency,
	}

	// Parse tool calls if present.
	for _, tc := range choice.Message.ToolCalls {
		var params map[string]interface{}
		if tc.Function.Arguments != "" {
			_ = json.Unmarshal([]byte(tc.Function.Arguments), &params)
		}
		result.ToolCalls = append(result.ToolCalls, domain.ToolCall{
			ID:     tc.ID,
			Name:   tc.Function.Name,
			Params: params,
		})
	}

	return result, nil
}

// Embed generates embeddings via DashScope's OpenAI-compatible endpoint.
// This implements the knowledge.Embedder interface.
type DashScopeEmbedder struct {
	apiKey  string
	baseURL string
	model   string
	dim     int
	http    *http.Client
}

func NewDashScopeEmbedder(apiKey, baseURL, model string) *DashScopeEmbedder {
	dim := 1024 // text-embedding-v3 default
	if model == "text-embedding-v1" {
		dim = 1536
	}
	return &DashScopeEmbedder{
		apiKey:  apiKey,
		baseURL: baseURL,
		model:   model,
		dim:     dim,
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

func (e *DashScopeEmbedder) Dimension() int { return e.dim }

func (e *DashScopeEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	reqBody := embeddingRequest{
		Model: e.model,
		Input: texts,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal embedding request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.baseURL+"/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create embedding request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.apiKey)

	resp, err := e.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("embedding http request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read embedding response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("embedding API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var embResp embeddingResponse
	if err := json.Unmarshal(respBody, &embResp); err != nil {
		return nil, fmt.Errorf("unmarshal embedding response: %w", err)
	}

	if len(embResp.Data) == 0 {
		return nil, fmt.Errorf("empty embedding response")
	}

	results := make([][]float32, len(embResp.Data))
	for i, item := range embResp.Data {
		results[i] = item.Embedding
	}
	return results, nil
}

// --- API types (OpenAI-compatible) ---

type chatRequest struct {
	Model       string     `json:"model"`
	Messages    []apiMsg   `json:"messages"`
	MaxTokens   int        `json:"max_tokens,omitempty"`
	Temperature float64    `json:"temperature,omitempty"`
	Tools       []apiTool  `json:"tools,omitempty"`
}

type apiMsg struct {
	Role       string       `json:"role"`
	Content    string       `json:"content,omitempty"`
	ToolCalls  []apiToolCall `json:"tool_calls,omitempty"`
	ToolCallID string       `json:"tool_call_id,omitempty"`
}

type apiToolCall struct {
	ID       string         `json:"id"`
	Type     string         `json:"type"`
	Function apiFunctionCall `json:"function"`
}

type apiFunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type apiTool struct {
	Type     string     `json:"type"`
	Function apiToolFn  `json:"function"`
}

type apiToolFn struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
}

type chatResponse struct {
	ID      string       `json:"id"`
	Model   string       `json:"model"`
	Choices []apiChoice  `json:"choices"`
	Usage   apiUsage     `json:"usage"`
}

type apiChoice struct {
	Index        int    `json:"index"`
	Message      apiMsg `json:"message"`
	FinishReason string `json:"finish_reason"`
}

type apiUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type embeddingRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type embeddingResponse struct {
	Data  []embeddingData `json:"data"`
	Usage apiUsage        `json:"usage"`
}

type embeddingData struct {
	Embedding []float32 `json:"embedding"`
	Index     int       `json:"index"`
}

// --- Conversion helpers ---

func toAPIMessages(msgs []domain.Message) []apiMsg {
	result := make([]apiMsg, len(msgs))
	for i, m := range msgs {
		msg := apiMsg{
			Role:    m.Role,
			Content: m.Content,
		}
		if m.ToolCallID != "" {
			msg.ToolCallID = m.ToolCallID
		}
		if len(m.ToolCalls) > 0 {
			for _, tc := range m.ToolCalls {
				args, _ := json.Marshal(tc.Params)
				msg.ToolCalls = append(msg.ToolCalls, apiToolCall{
					ID:   tc.ID,
					Type: "function",
					Function: apiFunctionCall{
						Name:      tc.Name,
						Arguments: string(args),
					},
				})
			}
		}
		result[i] = msg
	}
	return result
}

func toAPITools(schemas []domain.ToolSchema) []apiTool {
	tools := make([]apiTool, len(schemas))
	for i, s := range schemas {
		tools[i] = apiTool{
			Type: "function",
			Function: apiToolFn{
				Name:        s.Name,
				Description: s.Description,
				Parameters:  s.Parameters,
			},
		}
	}
	return tools
}
