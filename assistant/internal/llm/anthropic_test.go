package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"assistant/internal/config"
)

func TestAnthropicClientGenerateUsesMessagesAPI(t *testing.T) {
	var gotPath string
	var gotAPIKey string
	var gotVersion string
	var gotModel string
	var gotMaxTokens int
	var gotSystem string
	var gotRole string
	var gotContent string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAPIKey = r.Header.Get("x-api-key")
		gotVersion = r.Header.Get("anthropic-version")

		var request struct {
			Model     string `json:"model"`
			MaxTokens int    `json:"max_tokens"`
			System    []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"system"`
			Messages []struct {
				Role    string `json:"role"`
				Content []struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		gotModel = request.Model
		gotMaxTokens = request.MaxTokens
		if len(request.System) == 1 {
			gotSystem = request.System[0].Text
		}
		if len(request.Messages) == 1 {
			gotRole = request.Messages[0].Role
			if len(request.Messages[0].Content) == 1 {
				gotContent = request.Messages[0].Content[0].Text
			}
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id": "msg_test",
			"model": "claude-sonnet",
			"role": "assistant",
			"stop_reason": "end_turn",
			"stop_sequence": null,
			"type": "message",
			"usage": {"input_tokens": 10, "output_tokens": 5},
			"content": [
				{"type": "text", "text": "你好，我是模型回复"}
			]
		}`))
	}))
	defer server.Close()

	client := NewAnthropicClient(config.LLMConfig{
		BaseURL:   server.URL,
		APIKey:    "test-api-key",
		ModelName: "claude-sonnet",
	})
	client.HTTPClient = server.Client()

	reply, err := client.Generate(context.Background(), Request{
		System: "你是 MyGod 助手",
		Messages: []Message{
			{
				Role:    "user",
				Content: "你好",
			},
		},
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	if gotPath != "/v1/messages" {
		t.Fatalf("path = %q, want /v1/messages", gotPath)
	}
	if gotAPIKey != "test-api-key" {
		t.Fatalf("x-api-key = %q, want test-api-key", gotAPIKey)
	}
	if gotVersion != AnthropicVersion {
		t.Fatalf("anthropic-version = %q, want %s", gotVersion, AnthropicVersion)
	}
	if gotModel != "claude-sonnet" {
		t.Fatalf("model = %q, want claude-sonnet", gotModel)
	}
	if gotMaxTokens != 4096 {
		t.Fatalf("max_tokens = %d, want 4096", gotMaxTokens)
	}
	if gotSystem != "你是 MyGod 助手" {
		t.Fatalf("system = %q, want system prompt", gotSystem)
	}
	if gotRole != "user" {
		t.Fatalf("role = %q, want user", gotRole)
	}
	if gotContent != "你好" {
		t.Fatalf("content = %q, want 你好", gotContent)
	}
	if reply != "你好，我是模型回复" {
		t.Fatalf("reply = %q, want model text", reply)
	}
}

func TestAnthropicClientDoesNotDuplicateV1Path(t *testing.T) {
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id": "msg_test",
			"model": "claude-sonnet",
			"role": "assistant",
			"stop_reason": "end_turn",
			"stop_sequence": null,
			"type": "message",
			"usage": {"input_tokens": 1, "output_tokens": 1},
			"content":[{"type":"text","text":"ok"}]
		}`))
	}))
	defer server.Close()

	client := NewAnthropicClient(config.LLMConfig{
		BaseURL:   server.URL + "/v1",
		APIKey:    "test-api-key",
		ModelName: "claude-sonnet",
	})
	client.HTTPClient = server.Client()

	if _, err := client.Generate(context.Background(), Request{
		Messages: []Message{{Role: "user", Content: "ping"}},
	}); err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if gotPath != "/v1/messages" {
		t.Fatalf("path = %q, want /v1/messages", gotPath)
	}
}

func TestAnthropicClientCreateMessageSendsToolsAndParsesBlocks(t *testing.T) {
	var gotToolName string
	var gotToolDescription string
	var gotToolPropertyType string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Tools []struct {
				Name        string `json:"name"`
				Description string `json:"description"`
				InputSchema struct {
					Type       string `json:"type"`
					Properties struct {
						Query struct {
							Type string `json:"type"`
						} `json:"query"`
					} `json:"properties"`
				} `json:"input_schema"`
			} `json:"tools"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if len(request.Tools) == 1 {
			gotToolName = request.Tools[0].Name
			gotToolDescription = request.Tools[0].Description
			gotToolPropertyType = request.Tools[0].InputSchema.Properties.Query.Type
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id": "msg_test",
			"model": "claude-sonnet",
			"role": "assistant",
			"stop_reason": "tool_use",
			"stop_sequence": null,
			"type": "message",
			"usage": {"input_tokens": 10, "output_tokens": 5},
			"content": [
				{"type": "thinking", "thinking": "需要先查资料", "signature": "sig-test"},
				{"type": "text", "text": "我先查一下。"},
				{"type": "tool_use", "id": "toolu_1", "name": "main__search", "input": {"query": "mygod"}}
			]
		}`))
	}))
	defer server.Close()

	client := NewAnthropicClient(config.LLMConfig{
		BaseURL:   server.URL,
		APIKey:    "test-api-key",
		ModelName: "claude-sonnet",
	})
	client.HTTPClient = server.Client()

	response, err := client.CreateMessage(context.Background(), Request{
		Messages: []Message{{Role: RoleUser, Blocks: []Block{{Type: BlockTypeText, Text: "查一下"}}}},
		Tools: []Tool{
			{
				Name:        "main__search",
				Description: "Search documents",
				InputSchema: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"query": map[string]any{"type": "string"},
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateMessage() error = %v", err)
	}

	if gotToolName != "main__search" {
		t.Fatalf("tool name = %q, want main__search", gotToolName)
	}
	if gotToolDescription != "Search documents" {
		t.Fatalf("tool description = %q, want Search documents", gotToolDescription)
	}
	if gotToolPropertyType != "string" {
		t.Fatalf("tool query type = %q, want string", gotToolPropertyType)
	}
	if len(response.Blocks) != 3 {
		t.Fatalf("block count = %d, want 3", len(response.Blocks))
	}
	if response.Blocks[0].Type != BlockTypeThinking || response.Blocks[0].Thinking != "需要先查资料" || response.Blocks[0].ThinkingSignature != "sig-test" {
		t.Fatalf("thinking block = %+v, want parsed thinking", response.Blocks[0])
	}
	if response.Blocks[1].Type != BlockTypeText || response.Blocks[1].Text != "我先查一下。" {
		t.Fatalf("text block = %+v, want parsed text", response.Blocks[1])
	}
	var toolInput struct {
		Query string `json:"query"`
	}
	if err := json.Unmarshal(response.Blocks[2].ToolInput, &toolInput); err != nil {
		t.Fatalf("unmarshal tool input: %v", err)
	}
	if response.Blocks[2].Type != BlockTypeToolUse || response.Blocks[2].ToolUseID != "toolu_1" || response.Blocks[2].ToolName != "main__search" || toolInput.Query != "mygod" {
		t.Fatalf("tool use block = %+v, want parsed tool use", response.Blocks[2])
	}
}

func TestAnthropicClientCreateMessageSendsToolUseAndToolResultBlocks(t *testing.T) {
	var gotAssistantToolUseID string
	var gotAssistantToolName string
	var gotAssistantToolInput string
	var gotUserToolResultID string
	var gotUserToolResultContent string
	var gotUserToolResultIsError bool

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Messages []struct {
				Role    string `json:"role"`
				Content []struct {
					Type      string          `json:"type"`
					ID        string          `json:"id"`
					Name      string          `json:"name"`
					Input     json.RawMessage `json:"input"`
					ToolUseID string          `json:"tool_use_id"`
					Content   json.RawMessage `json:"content"`
					IsError   bool            `json:"is_error"`
				} `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if len(request.Messages) == 2 {
			gotAssistantToolUseID = request.Messages[0].Content[0].ID
			gotAssistantToolName = request.Messages[0].Content[0].Name
			gotAssistantToolInput = string(request.Messages[0].Content[0].Input)
			gotUserToolResultID = request.Messages[1].Content[0].ToolUseID
			gotUserToolResultContent = decodeTestToolResultContent(t, request.Messages[1].Content[0].Content)
			gotUserToolResultIsError = request.Messages[1].Content[0].IsError
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id": "msg_test",
			"model": "claude-sonnet",
			"role": "assistant",
			"stop_reason": "end_turn",
			"stop_sequence": null,
			"type": "message",
			"usage": {"input_tokens": 1, "output_tokens": 1},
			"content":[{"type":"text","text":"ok"}]
		}`))
	}))
	defer server.Close()

	client := NewAnthropicClient(config.LLMConfig{
		BaseURL:   server.URL,
		APIKey:    "test-api-key",
		ModelName: "claude-sonnet",
	})
	client.HTTPClient = server.Client()

	_, err := client.CreateMessage(context.Background(), Request{
		Messages: []Message{
			{
				Role: RoleAssistant,
				Blocks: []Block{
					{
						Type:      BlockTypeToolUse,
						ToolUseID: "toolu_1",
						ToolName:  "main__search",
						ToolInput: json.RawMessage(`{"query":"mygod"}`),
					},
				},
			},
			{
				Role: RoleUser,
				Blocks: []Block{
					{
						Type:      BlockTypeToolResult,
						ToolUseID: "toolu_1",
						Text:      "tool failed",
						IsError:   true,
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateMessage() error = %v", err)
	}

	if gotAssistantToolUseID != "toolu_1" {
		t.Fatalf("assistant tool use id = %q, want toolu_1", gotAssistantToolUseID)
	}
	if gotAssistantToolName != "main__search" {
		t.Fatalf("assistant tool name = %q, want main__search", gotAssistantToolName)
	}
	if gotAssistantToolInput != `{"query":"mygod"}` {
		t.Fatalf("assistant tool input = %s, want original JSON", gotAssistantToolInput)
	}
	if gotUserToolResultID != "toolu_1" {
		t.Fatalf("tool result id = %q, want toolu_1", gotUserToolResultID)
	}
	if gotUserToolResultContent != "tool failed" {
		t.Fatalf("tool result content = %q, want tool failed", gotUserToolResultContent)
	}
	if !gotUserToolResultIsError {
		t.Fatal("tool result is_error = false, want true")
	}
}

func decodeTestToolResultContent(t *testing.T, raw json.RawMessage) string {
	t.Helper()
	var content string
	if err := json.Unmarshal(raw, &content); err == nil {
		return content
	}

	var blocks []struct {
		Text string `json:"text"`
		Type string `json:"type"`
	}
	if err := json.Unmarshal(raw, &blocks); err != nil {
		t.Fatalf("decode tool result content: %v; raw=%s", err, raw)
	}
	if len(blocks) == 0 {
		return ""
	}
	return blocks[0].Text
}
