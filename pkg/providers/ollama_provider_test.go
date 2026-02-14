package providers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOllamaProvider_InterfaceCompliance(t *testing.T) {
	provider := NewOllamaProvider("http://localhost:11434", "", "")

	// Verify it implements LLMProvider
	var _ LLMProvider = provider
}

func TestOllamaProvider_ModelPrefixStripping(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"ollama/llama3.2", "llama3.2"},
		{"ollama/mistral:7b", "mistral:7b"},
		{"llama3.2", "llama3.2"},
		{"mistral:7b", "mistral:7b"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := tt.input
			if strings.HasPrefix(result, "ollama/") {
				result = result[7:]
			}
			if result != tt.expected {
				t.Errorf("got %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestOllamaProvider_ResponseParsing(t *testing.T) {
	provider := NewOllamaProvider("http://localhost:11434", "", "")

	responseJSON := `{
		"choices": [{
			"message": {
				"content": "Hello, world!",
				"tool_calls": []
			},
			"finish_reason": "stop"
		}],
		"usage": {
			"prompt_tokens": 10,
			"completion_tokens": 5,
			"total_tokens": 15
		}
	}`

	resp, err := provider.parseResponse([]byte(responseJSON))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Content != "Hello, world!" {
		t.Errorf("got content %q, want %q", resp.Content, "Hello, world!")
	}

	if resp.FinishReason != "stop" {
		t.Errorf("got finish_reason %q, want %q", resp.FinishReason, "stop")
	}

	if resp.Usage == nil {
		t.Fatal("expected usage info")
	}

	if resp.Usage.TotalTokens != 15 {
		t.Errorf("got total_tokens %d, want %d", resp.Usage.TotalTokens, 15)
	}
}

func TestOllamaProvider_ResponseParsingWithToolCalls(t *testing.T) {
	provider := NewOllamaProvider("http://localhost:11434", "", "")

	responseJSON := `{
		"choices": [{
			"message": {
				"content": "",
				"tool_calls": [{
					"id": "call_123",
					"type": "function",
					"function": {
						"name": "read_file",
						"arguments": "{\"path\": \"/tmp/test.txt\"}"
					}
				}]
			},
			"finish_reason": "tool_calls"
		}]
	}`

	resp, err := provider.parseResponse([]byte(responseJSON))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(resp.ToolCalls) != 1 {
		t.Fatalf("got %d tool calls, want 1", len(resp.ToolCalls))
	}

	tc := resp.ToolCalls[0]
	if tc.ID != "call_123" {
		t.Errorf("got tool call ID %q, want %q", tc.ID, "call_123")
	}

	if tc.Name != "read_file" {
		t.Errorf("got tool call name %q, want %q", tc.Name, "read_file")
	}

	if tc.Arguments["path"] != "/tmp/test.txt" {
		t.Errorf("got path %q, want %q", tc.Arguments["path"], "/tmp/test.txt")
	}
}

func TestOllamaProvider_Chat(t *testing.T) {
	// Create a test server that mimics Ollama's OpenAI-compatible endpoint
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		if r.Method != "POST" {
			t.Errorf("unexpected method: %s", r.Method)
		}

		var req map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("failed to decode request: %v", err)
		}

		// Verify model name has ollama/ prefix stripped
		if model, ok := req["model"].(string); ok {
			if strings.HasPrefix(model, "ollama/") {
				t.Errorf("model name should have ollama/ prefix stripped: %s", model)
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []map[string]interface{}{
				{
					"message": map[string]interface{}{
						"content":    "Test response",
						"tool_calls": []interface{}{},
					},
					"finish_reason": "stop",
				},
			},
		})
	}))
	defer server.Close()

	provider := NewOllamaProvider(server.URL, "", "")

	messages := []Message{
		{Role: "user", Content: "Hello"},
	}

	resp, err := provider.Chat(context.Background(), messages, nil, "ollama/llama3.2", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Content != "Test response" {
		t.Errorf("got content %q, want %q", resp.Content, "Test response")
	}
}

func TestOllamaProvider_ListModels(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tags" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"models": []map[string]interface{}{
				{"name": "llama3.2:latest"},
				{"name": "mistral:7b"},
				{"name": "codellama:13b"},
			},
		})
	}))
	defer server.Close()

	provider := NewOllamaProvider(server.URL, "", "")

	models, err := provider.ListModels(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(models) != 3 {
		t.Fatalf("got %d models, want 3", len(models))
	}

	expected := []string{"llama3.2:latest", "mistral:7b", "codellama:13b"}
	for i, m := range models {
		if m != expected[i] {
			t.Errorf("model[%d] = %q, want %q", i, m, expected[i])
		}
	}
}

func TestOllamaProvider_HealthCheck(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tags" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"models":[]}`))
	}))
	defer server.Close()

	provider := NewOllamaProvider(server.URL, "", "")

	err := provider.HealthCheck(context.Background())
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestOllamaProvider_HealthCheckFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	provider := NewOllamaProvider(server.URL, "", "")

	err := provider.HealthCheck(context.Background())
	if err == nil {
		t.Error("expected error for failed health check")
	}
}

func TestOllamaProvider_DefaultAPIBase(t *testing.T) {
	provider := NewOllamaProvider("", "", "")
	if provider.apiBase != "http://localhost:11434" {
		t.Errorf("got apiBase %q, want %q", provider.apiBase, "http://localhost:11434")
	}
}

func TestOllamaProvider_GetDefaultModel(t *testing.T) {
	provider := NewOllamaProvider("", "", "")
	if provider.GetDefaultModel() != "llama3.2" {
		t.Errorf("got default model %q, want %q", provider.GetDefaultModel(), "llama3.2")
	}
}
