package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/agentic-demo/platform/internal/domain"
)

func TestDashScopeProvider_ChatSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		resp := chatResponse{
			ID:    "chat-123",
			Model: "qwen-max",
			Choices: []apiChoice{
				{
					Index: 0,
					Message: apiMsg{
						Role:    "assistant",
						Content: "Hello, how can I help?",
					},
					FinishReason: "stop",
				},
			},
			Usage: apiUsage{
				PromptTokens:     50,
				CompletionTokens: 30,
				TotalTokens:      80,
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider := NewDashScopeProvider("test-key", server.URL, "qwen-max")

	result, err := provider.Chat(context.Background(), []domain.Message{
		{Role: "user", Content: "Hello"},
	}, Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Text != "Hello, how can I help?" {
		t.Errorf("Text = %q, want %q", result.Text, "Hello, how can I help?")
	}
	if result.Model != "qwen-max" {
		t.Errorf("Model = %q, want %q", result.Model, "qwen-max")
	}
	if result.InputTokens != 50 {
		t.Errorf("InputTokens = %d, want 50", result.InputTokens)
	}
	if result.OutputTokens != 30 {
		t.Errorf("OutputTokens = %d, want 30", result.OutputTokens)
	}
}

func TestDashScopeProvider_ChatError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal server error"))
	}))
	defer server.Close()

	provider := NewDashScopeProvider("test-key", server.URL, "qwen-max")

	_, err := provider.Chat(context.Background(), []domain.Message{
		{Role: "user", Content: "Hello"},
	}, Options{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	errStr := err.Error()
	if errStr[:15] != "API error (stat" {
		t.Errorf("error should start with 'API error (status 500)': %q", errStr)
	}
}

func TestDashScopeProvider_ChatWithTools(t *testing.T) {
	var capturedBody map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&capturedBody)
		resp := chatResponse{
			ID:      "chat-456",
			Model:   "qwen-max",
			Choices: []apiChoice{{Index: 0, Message: apiMsg{Role: "assistant", Content: "Using tools"}, FinishReason: "stop"}},
			Usage:   apiUsage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider := NewDashScopeProvider("test-key", server.URL, "qwen-max")

	toolSchemas := []domain.ToolSchema{
		{
			Name:        "search",
			Description: "search the web",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"query": map[string]interface{}{"type": "string"},
				},
			},
		},
	}

	_, err := provider.Chat(context.Background(), []domain.Message{
		{Role: "user", Content: "Search for news"},
	}, Options{ToolSchemas: toolSchemas})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tools, ok := capturedBody["tools"].([]interface{})
	if !ok || len(tools) != 1 {
		t.Fatalf("expected 1 tool in request, got %v", capturedBody["tools"])
	}
	tool := tools[0].(map[string]interface{})
	if tool["type"] != "function" {
		t.Errorf("tool type = %q, want function", tool["type"])
	}
	fn := tool["function"].(map[string]interface{})
	if fn["name"] != "search" {
		t.Errorf("tool name = %q, want search", fn["name"])
	}
}

func TestDashScopeProvider_EmbedSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/embeddings" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		resp := embeddingResponse{
			Data: []embeddingData{
				{Embedding: []float32{0.1, 0.2, 0.3}, Index: 0},
				{Embedding: []float32{0.4, 0.5, 0.6}, Index: 1},
			},
			Usage: apiUsage{PromptTokens: 20, TotalTokens: 20},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	embedder := NewDashScopeEmbedder("test-key", server.URL, "text-embedding-v3")

	results, err := embedder.Embed(context.Background(), []string{"text1", "text2"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 embeddings, got %d", len(results))
	}
	if len(results[0]) != 3 {
		t.Errorf("expected 3 dimensions, got %d", len(results[0]))
	}
	if results[0][0] != 0.1 || results[0][1] != 0.2 || results[0][2] != 0.3 {
		t.Errorf("first embedding = %v, want [0.1, 0.2, 0.3]", results[0])
	}
	if results[1][0] != 0.4 || results[1][1] != 0.5 || results[1][2] != 0.6 {
		t.Errorf("second embedding = %v, want [0.4, 0.5, 0.6]", results[1])
	}
}