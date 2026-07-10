package appclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"assistant/internal/agent"
	"assistant/internal/builtintools"
	"assistant/internal/config"
	"assistant/internal/llm"
	"assistant/internal/mcpclient"

	"github.com/gorilla/websocket"
)

type replyAgentFunc func(context.Context, agent.Request, agent.OutputSink) error

func (f replyAgentFunc) Run(ctx context.Context, request agent.Request, sink agent.OutputSink) error {
	return f(ctx, request, sink)
}

type appRequestFunc func(context.Context, string, any) (json.RawMessage, error)

func (f appRequestFunc) Request(ctx context.Context, method string, payload any) (json.RawMessage, error) {
	return f(ctx, method, payload)
}

func TestConnectionRequesterRejectsOversizedOutgoingEnvelope(t *testing.T) {
	wrote := false
	requester := newConnectionRequester(func(envelope) error {
		wrote = true
		return nil
	})

	_, err := requester.Request(context.Background(), methodMessageSend, map[string]any{
		"message": map[string]any{
			"type":    "file",
			"name":    "big.txt",
			"content": strings.Repeat("x", maxMessageBytes),
		},
	})
	if err == nil {
		t.Fatal("Request() error = nil, want oversized message error")
	}
	if !strings.Contains(err.Error(), "1MiB") {
		t.Fatalf("Request() error = %v, want 1MiB limit", err)
	}
	if wrote {
		t.Fatal("write called for oversized message")
	}
}

func TestConnectionRequesterAllowsEnvelopeLargerThan64KiB(t *testing.T) {
	var requester *connectionRequester
	requester = newConnectionRequester(func(message envelope) error {
		ok := true
		requester.HandleResponse(envelope{
			V:       protocolVersion,
			Kind:    kindResponse,
			ReplyTo: message.ID,
			OK:      &ok,
			Payload: json.RawMessage(`{"accepted":true}`),
		})
		return nil
	})

	raw, err := requester.Request(context.Background(), methodMessageSend, map[string]any{
		"message": map[string]any{
			"type":    "markdown",
			"content": strings.Repeat("x", 128*1024),
		},
	})
	if err != nil {
		t.Fatalf("Request() error = %v, want nil", err)
	}
	if string(raw) != `{"accepted":true}` {
		t.Fatalf("Request() payload = %s, want accepted response", raw)
	}
}

func TestEncodeEnvelopeRejectsMessageLargerThanOneMiB(t *testing.T) {
	payload, err := json.Marshal(map[string]any{"content": strings.Repeat("x", maxMessageBytes)})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	_, err = encodeEnvelope(envelope{
		V:       protocolVersion,
		Kind:    kindRequest,
		ID:      "request-1",
		Method:  methodMessageSend,
		Payload: payload,
	})
	if err == nil || !strings.Contains(err.Error(), "1MiB") {
		t.Fatalf("encodeEnvelope() error = %v, want 1MiB limit", err)
	}
}

func TestHandleServerMessageSendsLLMReply(t *testing.T) {
	body, err := json.Marshal(messageBody{
		Type:    "text",
		Content: "你好",
	})
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	payload, err := json.Marshal(messageCreatedPayload{
		Conversation: conversationPayload{
			ID:   "conversation-1",
			Name: "AI 女菩萨",
			Type: "app",
		},
		Message: messagePayload{
			Body:    body,
			ID:      "message-1",
			Seq:     3,
			Summary: "你好",
		},
		Sender: senderPayload{
			Email: "alice@example.com",
			ID:    "user-1",
			Name:  "Alice",
			Type:  "user",
		},
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	raw, err := json.Marshal(envelope{
		V:       protocolVersion,
		Kind:    kindEvent,
		ID:      "event-1",
		Event:   eventMessageCreated,
		Payload: payload,
	})
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}

	var sent []envelope
	var agentRequests []agent.Request
	var historyMethod string
	var historyPayload appListConversationMessagesRequestPayload
	requester := appRequestFunc(func(ctx context.Context, method string, payload any) (json.RawMessage, error) {
		historyMethod = method
		var err error
		rawPayload, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("marshal history request payload: %v", err)
		}
		if err := json.Unmarshal(rawPayload, &historyPayload); err != nil {
			t.Fatalf("unmarshal history request payload: %v", err)
		}
		return json.Marshal(appListConversationMessagesResponsePayload{
			Messages: []historyMessagePayload{
				{
					CreatedAt: time.Date(2026, 7, 8, 10, 0, 0, 0, time.UTC),
					ID:        "history-1",
					Seq:       1,
					Sender: senderPayload{
						ID:   "user-1",
						Name: "Alice",
						Type: "user",
					},
					Summary: "之前问了部署时间",
				},
				{
					CreatedAt: time.Date(2026, 7, 8, 10, 1, 0, 0, time.UTC),
					ID:        "history-2",
					Seq:       2,
					Sender: senderPayload{
						ID:   "assistant-app",
						Name: "女菩萨",
						Type: "app",
					},
					Summary: "回复预计今天下午完成",
				},
				{
					CreatedAt: time.Date(2026, 7, 8, 10, 2, 0, 0, time.UTC),
					ID:        "message-1",
					Seq:       3,
					Sender: senderPayload{
						ID:   "user-1",
						Name: "Alice",
						Type: "user",
					},
					Summary: "你好",
				},
			},
		})
	})
	replyAgent := replyAgentFunc(func(ctx context.Context, request agent.Request, sink agent.OutputSink) error {
		agentRequests = append(agentRequests, request)
		return sink.SendMarkdown(ctx, "你好，我是大模型回复")
	})

	handleServerMessage(context.Background(), websocket.TextMessage, raw, requester, replyAgent, func(_ context.Context, message envelope) error {
		sent = append(sent, message)
		return nil
	})

	if historyMethod != methodConversationMessagesList {
		t.Fatalf("history method = %q, want %s", historyMethod, methodConversationMessagesList)
	}
	if historyPayload.ConversationID != "conversation-1" {
		t.Fatalf("history conversation_id = %q, want conversation-1", historyPayload.ConversationID)
	}
	if historyPayload.BeforeOrEqualSeq != 3 {
		t.Fatalf("history before_or_equal_seq = %d, want 3", historyPayload.BeforeOrEqualSeq)
	}
	if historyPayload.Limit != 30 {
		t.Fatalf("history limit = %d, want 30", historyPayload.Limit)
	}
	if len(agentRequests) != 1 {
		t.Fatalf("agent request count = %d, want 1", len(agentRequests))
	}
	agentRequest := agentRequests[0]
	if agentRequest.Content != "你好" {
		t.Fatalf("agent content = %q, want 你好", agentRequest.Content)
	}
	if agentRequest.MessageID != "message-1" {
		t.Fatalf("agent message id = %q, want message-1", agentRequest.MessageID)
	}
	if agentRequest.Conversation.ID != "conversation-1" {
		t.Fatalf("agent conversation id = %q, want conversation-1", agentRequest.Conversation.ID)
	}
	if agentRequest.Conversation.Name != "AI 女菩萨" {
		t.Fatalf("agent conversation name = %q, want AI 女菩萨", agentRequest.Conversation.Name)
	}
	if agentRequest.Conversation.Type != "app" {
		t.Fatalf("agent conversation type = %q, want app", agentRequest.Conversation.Type)
	}
	if agentRequest.Sender.ID != "user-1" {
		t.Fatalf("agent sender id = %q, want user-1", agentRequest.Sender.ID)
	}
	if agentRequest.Sender.Email != "alice@example.com" {
		t.Fatalf("agent sender email = %q, want alice@example.com", agentRequest.Sender.Email)
	}
	if agentRequest.Sender.Name != "Alice" {
		t.Fatalf("agent sender name = %q, want Alice", agentRequest.Sender.Name)
	}
	if agentRequest.Sender.Type != "user" {
		t.Fatalf("agent sender type = %q, want user", agentRequest.Sender.Type)
	}
	if len(agentRequest.History) != 2 {
		t.Fatalf("agent history count = %d, want 2", len(agentRequest.History))
	}
	if agentRequest.History[0].Summary != "之前问了部署时间" {
		t.Fatalf("first history summary = %q, want previous summary", agentRequest.History[0].Summary)
	}
	if agentRequest.History[1].SenderName != "女菩萨" {
		t.Fatalf("second history sender = %q, want 女菩萨", agentRequest.History[1].SenderName)
	}
	if len(sent) != 1 {
		t.Fatalf("sent count = %d, want 1", len(sent))
	}
	request := sent[0]
	if request.V != protocolVersion {
		t.Fatalf("request version = %d, want %d", request.V, protocolVersion)
	}
	if request.Kind != kindRequest {
		t.Fatalf("request kind = %q, want request", request.Kind)
	}
	if request.Method != methodMessageSend {
		t.Fatalf("request method = %q, want %s", request.Method, methodMessageSend)
	}
	if request.ID == "" {
		t.Fatal("request id is empty")
	}

	var requestPayload sendMessageRequestPayload
	if err := json.Unmarshal(request.Payload, &requestPayload); err != nil {
		t.Fatalf("unmarshal request payload: %v", err)
	}
	if requestPayload.Target.Type != "app" {
		t.Fatalf("target.type = %q, want app", requestPayload.Target.Type)
	}
	if requestPayload.Target.ConversationID != "conversation-1" {
		t.Fatalf("target.conversation_id = %q, want conversation-1", requestPayload.Target.ConversationID)
	}
	if requestPayload.Message.Type != "markdown" {
		t.Fatalf("message.type = %q, want markdown", requestPayload.Message.Type)
	}
	if requestPayload.Message.Content != "你好，我是大模型回复" {
		t.Fatalf("message.content = %q, want LLM reply", requestPayload.Message.Content)
	}
}

func TestHandleParsedServerMessageIgnoresGroupMessageWithoutDirectAppMention(t *testing.T) {
	appID := "00000000-0000-0000-0000-000000000001"
	calledRequester := false
	calledAgent := false
	requester := appRequestFunc(func(ctx context.Context, method string, payload any) (json.RawMessage, error) {
		calledRequester = true
		return nil, nil
	})
	replyAgent := replyAgentFunc(func(ctx context.Context, request agent.Request, sink agent.OutputSink) error {
		calledAgent = true
		return nil
	})

	handleParsedServerMessage(
		context.Background(),
		testGroupMessageCreatedEnvelope(t, appID, "user-1", "message-1", 1, "普通群消息 {(@user/all)}"),
		appID,
		requester,
		replyAgent,
		directAgentRunner{},
		func(context.Context, envelope) error { return nil },
	)

	if calledRequester {
		t.Fatal("requester called for group message without direct app mention")
	}
	if calledAgent {
		t.Fatal("agent called for group message without direct app mention")
	}
}

func TestHandleParsedServerMessageRunsGroupMessageWithDirectAppMention(t *testing.T) {
	appID := "00000000-0000-0000-0000-000000000001"
	var agentRequests []agent.Request
	var sent []envelope
	requester := appRequestFunc(func(ctx context.Context, method string, payload any) (json.RawMessage, error) {
		if method != methodConversationMessagesList {
			t.Fatalf("unexpected app request method %q", method)
		}
		return json.Marshal(appListConversationMessagesResponsePayload{
			Messages: []historyMessagePayload{
				historyTextMessage("message-1", 1, "user-1", "Alice", "请处理 {(@app/"+appID+")}"),
			},
		})
	})
	replyAgent := replyAgentFunc(func(ctx context.Context, request agent.Request, sink agent.OutputSink) error {
		agentRequests = append(agentRequests, request)
		return sink.SendMarkdown(ctx, "收到")
	})

	handleParsedServerMessage(
		context.Background(),
		testGroupMessageCreatedEnvelope(t, appID, "user-1", "message-1", 1, "请处理 {(@app/"+appID+")}"),
		appID,
		requester,
		replyAgent,
		directAgentRunner{},
		func(_ context.Context, message envelope) error {
			sent = append(sent, message)
			return nil
		},
	)

	if len(agentRequests) != 1 {
		t.Fatalf("agent request count = %d, want 1", len(agentRequests))
	}
	if agentRequests[0].Conversation.Type != "group" {
		t.Fatalf("conversation type = %q, want group", agentRequests[0].Conversation.Type)
	}
	if len(sent) != 1 {
		t.Fatalf("sent count = %d, want 1", len(sent))
	}
}

func TestHandleParsedServerMessageRunsGroupMessageWithUppercaseDirectAppMention(t *testing.T) {
	appID := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaa1"
	mentionedAppID := strings.ToUpper(appID)
	var agentRequests []agent.Request
	requester := appRequestFunc(func(ctx context.Context, method string, payload any) (json.RawMessage, error) {
		if method != methodConversationMessagesList {
			t.Fatalf("unexpected app request method %q", method)
		}
		return json.Marshal(appListConversationMessagesResponsePayload{
			Messages: []historyMessagePayload{
				historyTextMessage("message-1", 1, "user-1", "Alice", "请处理 {(@app/"+mentionedAppID+")}"),
			},
		})
	})
	replyAgent := replyAgentFunc(func(ctx context.Context, request agent.Request, sink agent.OutputSink) error {
		agentRequests = append(agentRequests, request)
		return nil
	})

	handleParsedServerMessage(
		context.Background(),
		testGroupMessageCreatedEnvelope(t, appID, "user-1", "message-1", 1, "请处理 {(@app/"+mentionedAppID+")}"),
		appID,
		requester,
		replyAgent,
		directAgentRunner{},
		func(context.Context, envelope) error { return nil },
	)

	if len(agentRequests) != 1 {
		t.Fatalf("agent request count = %d, want 1", len(agentRequests))
	}
}

func TestNewReturnsErrorWhenMCPServerCannotInitialize(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	_, err := New(ctx, config.Config{
		Agent: config.AgentConfig{MaxTurns: config.DefaultAgentMaxTurns},
		MCP: config.MCPConfig{Servers: []config.MCPServerConfig{
			{Name: "main", URL: server.URL},
		}},
	})
	if err == nil {
		t.Fatal("New() error = nil, want MCP initialization error")
	}
}

func TestNewToolRegistryIncludesBuiltinTools(t *testing.T) {
	registry, sources, err := newToolRegistry(context.Background(), nil)
	defer func() {
		if sources != nil {
			mcpclient.CloseSources(sources)
		}
	}()
	if err != nil {
		t.Fatalf("newToolRegistry() error = %v", err)
	}

	toolNames := map[string]bool{}
	for _, tool := range registry.Tools() {
		toolNames[tool.Name] = true
	}
	for _, toolName := range []string{"builtin__sleep", "builtin__read_file_urls"} {
		if !toolNames[toolName] {
			t.Fatalf("tools = %+v, want %s", registry.Tools(), toolName)
		}
	}
}

func TestHandleServerMessageReadsTemporaryFileURLForImageAndFileMessages(t *testing.T) {
	tests := []struct {
		name             string
		body             map[string]any
		expectedSnippets []string
	}{
		{
			name: "image",
			body: map[string]any{
				"type":    "image",
				"file_id": "file-image-1",
			},
			expectedSnippets: []string{"图片", "file-image-1", "https://assets.example.test/image.webp"},
		},
		{
			name: "file",
			body: map[string]any{
				"type":       "file",
				"file_id":    "file-report-1",
				"name":       "report.pdf",
				"size_bytes": 1234,
			},
			expectedSnippets: []string{"文件", "report.pdf", "1234", "file-report-1", "https://assets.example.test/report.pdf"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var readURLPayload struct {
				FileIDs []string `json:"file_ids"`
			}
			var readURLPayloadMap map[string]any
			var agentRequests []agent.Request
			requester := appRequestFunc(func(ctx context.Context, method string, payload any) (json.RawMessage, error) {
				switch method {
				case "temporary_files.read_urls":
					rawPayload, err := json.Marshal(payload)
					if err != nil {
						t.Fatalf("marshal read URL payload: %v", err)
					}
					if err := json.Unmarshal(rawPayload, &readURLPayloadMap); err != nil {
						t.Fatalf("unmarshal read URL payload map: %v", err)
					}
					if err := json.Unmarshal(rawPayload, &readURLPayload); err != nil {
						t.Fatalf("unmarshal read URL payload: %v", err)
					}
					fileID := readURLPayload.FileIDs[0]
					readURL := "https://assets.example.test/image.webp"
					if tt.name == "file" {
						readURL = "https://assets.example.test/report.pdf"
					}
					return json.Marshal(map[string]any{
						"urls": []map[string]any{
							{
								"file_id":    fileID,
								"url":        readURL,
								"expires_at": "2026-07-08T12:00:00Z",
							},
						},
					})
				case methodConversationMessagesList:
					return json.Marshal(appListConversationMessagesResponsePayload{})
				default:
					t.Fatalf("unexpected app request method %q", method)
					return nil, nil
				}
			})
			replyAgent := replyAgentFunc(func(ctx context.Context, request agent.Request, sink agent.OutputSink) error {
				agentRequests = append(agentRequests, request)
				return nil
			})

			handleParsedServerMessage(
				context.Background(),
				testMessageCreatedEnvelopeWithBody(t, "user-1", "message-"+tt.name, 1, tt.body),
				"",
				requester,
				replyAgent,
				directAgentRunner{},
				func(context.Context, envelope) error { return nil },
			)

			if _, ok := readURLPayloadMap["conversation_id"]; ok {
				t.Fatalf("read URL payload = %#v, want file_ids only", readURLPayloadMap)
			}
			if len(readURLPayload.FileIDs) != 1 || readURLPayload.FileIDs[0] != tt.body["file_id"] {
				t.Fatalf("read URL file_ids = %#v, want body file id", readURLPayload.FileIDs)
			}
			if len(agentRequests) != 1 {
				t.Fatalf("agent request count = %d, want 1", len(agentRequests))
			}
			for _, snippet := range tt.expectedSnippets {
				if !strings.Contains(agentRequests[0].Content, snippet) {
					t.Fatalf("agent content = %q, want to contain %q", agentRequests[0].Content, snippet)
				}
			}
		})
	}
}

func TestHandleServerMessagePrefetchesCurrentFileURLAndKeepsHistoryFileIDs(t *testing.T) {
	var readURLPayload struct {
		FileIDs []string `json:"file_ids"`
	}
	var readURLPayloadMap map[string]any
	var agentRequests []agent.Request
	requester := appRequestFunc(func(ctx context.Context, method string, payload any) (json.RawMessage, error) {
		switch method {
		case methodConversationMessagesList:
			return json.Marshal(map[string]any{
				"messages": []map[string]any{
					{
						"id":         "history-image",
						"seq":        1,
						"created_at": "2026-07-08T10:00:00Z",
						"sender": map[string]any{
							"id":   "user-2",
							"name": "Bob",
							"type": "user",
						},
						"summary": "发了一张图片",
						"body": map[string]any{
							"type":    "image",
							"file_id": "file-history-image",
						},
					},
					{
						"id":         "history-file",
						"seq":        2,
						"created_at": "2026-07-08T10:01:00Z",
						"sender": map[string]any{
							"id":   "user-1",
							"name": "Alice",
							"type": "user",
						},
						"summary": "发了一个文件",
						"body": map[string]any{
							"type":       "file",
							"file_id":    "file-history-report",
							"name":       "report.pdf",
							"size_bytes": 456,
						},
					},
					{
						"id":         "message-current",
						"seq":        3,
						"created_at": "2026-07-08T10:02:00Z",
						"sender": map[string]any{
							"id":   "user-1",
							"name": "Alice",
							"type": "user",
						},
						"summary": "当前图片",
						"body": map[string]any{
							"type":    "image",
							"file_id": "file-current-image",
						},
					},
				},
			})
		case methodTemporaryFilesReadURLs:
			rawPayload, err := json.Marshal(payload)
			if err != nil {
				t.Fatalf("marshal read URL payload: %v", err)
			}
			if err := json.Unmarshal(rawPayload, &readURLPayloadMap); err != nil {
				t.Fatalf("unmarshal read URL payload map: %v", err)
			}
			if err := json.Unmarshal(rawPayload, &readURLPayload); err != nil {
				t.Fatalf("unmarshal read URL payload: %v", err)
			}
			urls := make([]map[string]any, 0, len(readURLPayload.FileIDs))
			for _, fileID := range readURLPayload.FileIDs {
				urls = append(urls, map[string]any{
					"file_id":    fileID,
					"url":        "https://assets.example.test/" + fileID,
					"expires_at": "2026-07-08T12:00:00Z",
				})
			}
			return json.Marshal(map[string]any{"urls": urls})
		default:
			t.Fatalf("unexpected app request method %q", method)
			return nil, nil
		}
	})
	replyAgent := replyAgentFunc(func(ctx context.Context, request agent.Request, sink agent.OutputSink) error {
		agentRequests = append(agentRequests, request)
		return nil
	})

	handleParsedServerMessage(
		context.Background(),
		testMessageCreatedEnvelopeWithBody(t, "user-1", "message-current", 3, map[string]any{
			"type":    "image",
			"file_id": "file-current-image",
		}),
		"",
		requester,
		replyAgent,
		directAgentRunner{},
		func(context.Context, envelope) error { return nil },
	)

	if _, ok := readURLPayloadMap["conversation_id"]; ok {
		t.Fatalf("read URL payload = %#v, want file_ids only", readURLPayloadMap)
	}
	wantFileIDs := []string{"file-current-image"}
	if !slices.Equal(readURLPayload.FileIDs, wantFileIDs) {
		t.Fatalf("read URL file_ids = %#v, want %#v", readURLPayload.FileIDs, wantFileIDs)
	}
	if len(agentRequests) != 1 {
		t.Fatalf("agent request count = %d, want 1", len(agentRequests))
	}
	agentRequest := agentRequests[0]
	if !strings.Contains(agentRequest.Content, "https://assets.example.test/file-current-image") {
		t.Fatalf("agent content = %q, want current image URL", agentRequest.Content)
	}
	if len(agentRequest.History) != 2 {
		t.Fatalf("history count = %d, want 2", len(agentRequest.History))
	}
	historyJSON, err := json.Marshal(agentRequest.History)
	if err != nil {
		t.Fatalf("marshal history: %v", err)
	}
	for _, snippet := range []string{
		`"body"`,
		`"file_id":"file-history-image"`,
		`"file_id":"file-history-report"`,
	} {
		if !strings.Contains(string(historyJSON), snippet) {
			t.Fatalf("history JSON = %s, want to contain %s", historyJSON, snippet)
		}
	}
	if strings.Contains(string(historyJSON), `"url"`) {
		t.Fatalf("history JSON = %s, want history file URLs omitted", historyJSON)
	}
}

func TestHandleServerMessageDoesNotReadHistoryFileURLs(t *testing.T) {
	var readURLCalls [][]string
	var agentRequests []agent.Request
	requester := appRequestFunc(func(ctx context.Context, method string, payload any) (json.RawMessage, error) {
		switch method {
		case methodConversationMessagesList:
			return json.Marshal(map[string]any{
				"messages": []map[string]any{
					{
						"id":         "history-image",
						"seq":        1,
						"created_at": "2026-07-08T10:00:00Z",
						"sender": map[string]any{
							"id":   "user-2",
							"name": "Bob",
							"type": "user",
						},
						"summary": "发了一张图片",
						"body": map[string]any{
							"type":    "image",
							"file_id": "file-history-image",
						},
					},
					{
						"id":         "history-expired-file",
						"seq":        2,
						"created_at": "2026-07-08T10:01:00Z",
						"sender": map[string]any{
							"id":   "user-1",
							"name": "Alice",
							"type": "user",
						},
						"summary": "发了一个过期文件",
						"body": map[string]any{
							"type":       "file",
							"file_id":    "file-history-expired",
							"name":       "expired.pdf",
							"size_bytes": 789,
						},
					},
					{
						"id":         "message-current",
						"seq":        3,
						"created_at": "2026-07-08T10:02:00Z",
						"sender": map[string]any{
							"id":   "user-1",
							"name": "Alice",
							"type": "user",
						},
						"summary": "帮我看一下",
						"body": map[string]any{
							"type":    "text",
							"content": "帮我看一下",
						},
					},
				},
			})
		case methodTemporaryFilesReadURLs:
			var readURLPayload readTemporaryFileURLsRequestPayload
			rawPayload, err := json.Marshal(payload)
			if err != nil {
				t.Fatalf("marshal read URL payload: %v", err)
			}
			if err := json.Unmarshal(rawPayload, &readURLPayload); err != nil {
				t.Fatalf("unmarshal read URL payload: %v", err)
			}
			readURLCalls = append(readURLCalls, slices.Clone(readURLPayload.FileIDs))
			return nil, errors.New("history file URLs should not be read")
		default:
			t.Fatalf("unexpected app request method %q", method)
			return nil, nil
		}
	})
	replyAgent := replyAgentFunc(func(ctx context.Context, request agent.Request, sink agent.OutputSink) error {
		agentRequests = append(agentRequests, request)
		return nil
	})

	handleParsedServerMessage(
		context.Background(),
		testMessageCreatedEnvelope(t, "user-1", "message-current", 3, "帮我看一下"),
		"",
		requester,
		replyAgent,
		directAgentRunner{},
		func(context.Context, envelope) error { return nil },
	)

	if len(readURLCalls) != 0 {
		t.Fatalf("read URL calls = %#v, want none for history-only files", readURLCalls)
	}
	if len(agentRequests) != 1 {
		t.Fatalf("agent request count = %d, want 1", len(agentRequests))
	}
	agentRequest := agentRequests[0]
	if agentRequest.Content != "帮我看一下" {
		t.Fatalf("agent content = %q, want current text", agentRequest.Content)
	}
	if len(agentRequest.History) != 2 {
		t.Fatalf("history count = %d, want 2", len(agentRequest.History))
	}

	var imageBody map[string]any
	if err := json.Unmarshal(agentRequest.History[0].Body, &imageBody); err != nil {
		t.Fatalf("unmarshal image body: %v", err)
	}
	if imageBody["file_id"] != "file-history-image" {
		t.Fatalf("image history file_id = %v, want original file_id", imageBody["file_id"])
	}
	if _, ok := imageBody["url"]; ok {
		t.Fatalf("image history url = %v, want omitted", imageBody["url"])
	}
	var expiredBody map[string]any
	if err := json.Unmarshal(agentRequest.History[1].Body, &expiredBody); err != nil {
		t.Fatalf("unmarshal expired body: %v", err)
	}
	if expiredBody["file_id"] != "file-history-expired" {
		t.Fatalf("expired history file_id = %v, want original file_id", expiredBody["file_id"])
	}
	if _, ok := expiredBody["url"]; ok {
		t.Fatalf("expired history url = %v, want omitted", expiredBody["url"])
	}
}

func TestHandleServerMessageProvidesBuiltinToolScope(t *testing.T) {
	var toolMethod string
	var toolPayload struct {
		ActorUserID      string `json:"actor_user_id"`
		TargetUserID     string `json:"target_user_id"`
		TriggerMessageID string `json:"trigger_message_id"`
		Message          struct {
			Type    string `json:"type"`
			Content string `json:"content"`
		} `json:"message"`
	}
	requester := appRequestFunc(func(ctx context.Context, method string, payload any) (json.RawMessage, error) {
		switch method {
		case methodConversationMessagesList:
			return json.Marshal(appListConversationMessagesResponsePayload{})
		case methodMessageSendAsUser:
			toolMethod = method
			rawPayload, err := json.Marshal(payload)
			if err != nil {
				t.Fatalf("marshal tool payload: %v", err)
			}
			if err := json.Unmarshal(rawPayload, &toolPayload); err != nil {
				t.Fatalf("unmarshal tool payload: %v", err)
			}
			return json.RawMessage(`{"sent":true}`), nil
		default:
			t.Fatalf("unexpected app request method %q", method)
			return nil, nil
		}
	})
	replyAgent := replyAgentFunc(func(ctx context.Context, request agent.Request, sink agent.OutputSink) error {
		if len(request.AuthorizationCandidates) != 1 || request.AuthorizationCandidates[0].Ref != "auth_1" {
			t.Fatalf("authorization candidates = %#v, want current trigger auth_1", request.AuthorizationCandidates)
		}
		_, err := builtintools.NewSource().CallTool(ctx, "send_as_user", json.RawMessage(`{
			"authorization_ref":"auth_1",
			"contact_id":"user-2",
			"type":"markdown",
			"content":"**收到**"
		}`))
		return err
	})

	handleParsedServerMessage(context.Background(), testMessageCreatedEnvelope(t, "user-1", "message-1", 1, "帮我发给 Bob"), "", requester, replyAgent, directAgentRunner{}, func(_ context.Context, message envelope) error {
		return nil
	})

	if toolMethod != methodMessageSendAsUser {
		t.Fatalf("tool method = %q, want %s", toolMethod, methodMessageSendAsUser)
	}
	if toolPayload.ActorUserID != "user-1" || toolPayload.TargetUserID != "user-2" || toolPayload.TriggerMessageID != "message-1" {
		t.Fatalf("tool payload context = %#v, want current user and trigger message", toolPayload)
	}
	if toolPayload.Message.Type != "markdown" || toolPayload.Message.Content != "**收到**" {
		t.Fatalf("tool payload message = %#v, want markdown", toolPayload.Message)
	}
}

func TestConversationAgentRunnerCancelAllCancelsOutstandingJobs(t *testing.T) {
	runner := newConversationAgentRunner(context.Background())
	firstStarted := make(chan struct{})
	firstCanceled := make(chan struct{})
	secondStarted := make(chan struct{})
	secondCanceled := make(chan struct{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sink := agent.OutputSinkFunc(func(ctx context.Context, content string) error {
		return nil
	})
	assistantAgent := agent.New(llmModelFunc(func(ctx context.Context, request llm.Request) (llm.Response, error) {
		requestJSON, err := json.Marshal(request.Messages)
		if err != nil {
			t.Fatalf("marshal request messages: %v", err)
		}
		if strings.Contains(string(requestJSON), "conversation-1") {
			close(firstStarted)
			<-ctx.Done()
			close(firstCanceled)
			return llm.Response{}, ctx.Err()
		}
		close(secondStarted)
		<-ctx.Done()
		close(secondCanceled)
		return llm.Response{}, ctx.Err()
	}))

	runner.Start(ctx, "conversation-1", sink, assistantAgent, preparedTextRun("conversation-1", "message-1", 1, "第一条"))
	runner.Start(ctx, "conversation-2", sink, assistantAgent, preparedTextRun("conversation-2", "message-2", 1, "第二条"))
	waitForSignal(t, firstStarted, "first conversation job to start")
	waitForSignal(t, secondStarted, "second conversation job to start")

	runner.CancelAll()

	waitForSignal(t, firstCanceled, "first conversation job to be canceled")
	waitForSignal(t, secondCanceled, "second conversation job to be canceled")
}

func TestConversationAgentRunnerDoesNotRunAppendedMessageWhileSendIsInProgress(t *testing.T) {
	runner := newConversationAgentRunner(context.Background())
	sendStarted := make(chan struct{})
	releaseSend := make(chan struct{})
	secondRequestSeen := make(chan struct{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var sendStartedOnce sync.Once
	blockingSink := agent.OutputSinkFunc(func(ctx context.Context, content string) error {
		sendStartedOnce.Do(func() {
			close(sendStarted)
		})
		<-releaseSend
		return nil
	})
	assistantAgent := agent.New(llmModelFunc(func(ctx context.Context, request llm.Request) (llm.Response, error) {
		requestJSON, err := json.Marshal(request.Messages)
		if err != nil {
			t.Fatalf("marshal request messages: %v", err)
		}
		if strings.Contains(string(requestJSON), "第二条") {
			close(secondRequestSeen)
		}
		return llm.Response{Blocks: []llm.Block{{Type: llm.BlockTypeText, Text: "回复"}}}, nil
	}))

	runner.Start(ctx, "conversation-1", blockingSink, assistantAgent, preparedTextRun("conversation-1", "message-1", 1, "第一条"))
	waitForSignal(t, sendStarted, "first send to start")

	runner.Start(ctx, "conversation-1", blockingSink, assistantAgent, preparedTextRun("conversation-1", "message-2", 2, "第二条"))

	select {
	case <-secondRequestSeen:
		t.Fatal("appended message ran while previous send was still in progress")
	case <-time.After(50 * time.Millisecond):
	}

	close(releaseSend)
	waitForSignal(t, secondRequestSeen, "appended message to run after first send")
}

func TestConversationAgentRunnerAppendsSameConversationMessageToActiveSession(t *testing.T) {
	model := &recordingLoopModel{
		secondRequestSeen: make(chan struct{}),
	}
	registry := &blockingToolRegistry{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	assistantAgent := agent.New(model, agent.WithToolRegistry(registry), agent.WithMaxTurns(3))
	runner := newConversationAgentRunner(context.Background())
	defer runner.CancelAll()

	requester := appRequestFunc(func(ctx context.Context, method string, payload any) (json.RawMessage, error) {
		if method != methodConversationMessagesList {
			t.Fatalf("unexpected app request method %q", method)
		}
		var historyRequest appListConversationMessagesRequestPayload
		rawPayload, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("marshal history payload: %v", err)
		}
		if err := json.Unmarshal(rawPayload, &historyRequest); err != nil {
			t.Fatalf("unmarshal history payload: %v", err)
		}
		switch historyRequest.BeforeOrEqualSeq {
		case 1:
			return json.Marshal(appListConversationMessagesResponsePayload{
				Messages: []historyMessagePayload{
					historyTextMessage("message-1", 1, "user-1", "Alice", "第一条"),
				},
			})
		case 3:
			return json.Marshal(appListConversationMessagesResponsePayload{
				Messages: []historyMessagePayload{
					historyTextMessage("message-old", 0, "user-2", "Bob", "很早以前的旧背景"),
					historyTextMessage("message-2", 2, "user-2", "Bob", "这是一条中间背景"),
					historyTextMessage("message-3", 3, "user-1", "Alice", "第二条"),
				},
			})
		default:
			t.Fatalf("history before_or_equal_seq = %d, want 1 or 3", historyRequest.BeforeOrEqualSeq)
			return nil, nil
		}
	})
	sent := &sentMessages{}

	handleParsedServerMessage(
		context.Background(),
		testMessageCreatedEnvelope(t, "user-1", "message-1", 1, "第一条"),
		"",
		requester,
		assistantAgent,
		runner,
		sent.write,
	)
	waitForSignal(t, registry.started, "first tool call to start")

	handleParsedServerMessage(
		context.Background(),
		testMessageCreatedEnvelope(t, "user-1", "message-3", 3, "第二条"),
		"",
		requester,
		assistantAgent,
		runner,
		sent.write,
	)

	close(registry.release)
	waitForSignal(t, model.secondRequestSeen, "second model request")

	secondRequest := model.requestAt(t, 1)
	secondRequestJSON, err := json.Marshal(secondRequest.Messages)
	if err != nil {
		t.Fatalf("marshal second request messages: %v", err)
	}
	for _, snippet := range []string{"第二条", "这是一条中间背景", "auth_1", "auth_3"} {
		if !strings.Contains(string(secondRequestJSON), snippet) {
			t.Fatalf("second request messages = %s, want to contain %q", secondRequestJSON, snippet)
		}
	}
	if strings.Contains(string(secondRequestJSON), "很早以前的旧背景") {
		t.Fatalf("second request messages = %s, want old history before previous trigger filtered", secondRequestJSON)
	}
}

func TestConversationAuthorizationStoreKeepsLatestFiveRefs(t *testing.T) {
	store := newConversationAuthorizationStore()
	for i := 1; i <= 6; i++ {
		ref := fmt.Sprintf("auth_%d", i)
		store.Add(preparedAuthorization{
			Authorization: builtintools.Authorization{
				ActorUserID:      fmt.Sprintf("user-%d", i),
				TriggerMessageID: fmt.Sprintf("message-%d", i),
			},
			Candidate: agent.AuthorizationCandidate{
				Ref:        ref,
				SenderID:   fmt.Sprintf("user-%d", i),
				SenderName: "User",
				MessageSeq: int64(i),
			},
			Ref: ref,
		})
	}

	if _, ok := store.ResolveAuthorization("auth_1"); ok {
		t.Fatal("auth_1 still resolves, want oldest ref evicted")
	}
	if _, ok := store.ResolveAuthorization("auth_6"); !ok {
		t.Fatal("auth_6 does not resolve, want newest ref retained")
	}
	candidates := store.Candidates()
	if len(candidates) != 5 {
		t.Fatalf("candidate count = %d, want 5", len(candidates))
	}
	if candidates[0].Ref != "auth_2" || candidates[4].Ref != "auth_6" {
		t.Fatalf("candidates = %#v, want auth_2..auth_6", candidates)
	}
}

type recordingLoopModel struct {
	mu                sync.Mutex
	requests          []llm.Request
	secondRequestSeen chan struct{}
}

func (m *recordingLoopModel) CreateMessage(ctx context.Context, request llm.Request) (llm.Response, error) {
	m.mu.Lock()
	m.requests = append(m.requests, request)
	requestCount := len(m.requests)
	m.mu.Unlock()

	switch requestCount {
	case 1:
		return llm.Response{Blocks: []llm.Block{
			{Type: llm.BlockTypeToolUse, ToolUseID: "toolu_wait", ToolName: "test__wait", ToolInput: json.RawMessage(`{}`)},
		}}, nil
	case 2:
		close(m.secondRequestSeen)
		return llm.Response{Blocks: []llm.Block{{Type: llm.BlockTypeText, Text: "处理第二条"}}}, nil
	default:
		return llm.Response{Blocks: []llm.Block{{Type: llm.BlockTypeText, Text: "完成"}}}, nil
	}
}

func (m *recordingLoopModel) requestAt(t *testing.T, index int) llm.Request {
	t.Helper()

	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.requests) <= index {
		t.Fatalf("model request count = %d, want index %d", len(m.requests), index)
	}
	return m.requests[index]
}

type blockingToolRegistry struct {
	started chan struct{}
	release chan struct{}
}

func (r *blockingToolRegistry) Tools() []mcpclient.Tool {
	return []mcpclient.Tool{{Name: "test__wait"}}
}

func (r *blockingToolRegistry) CallTool(ctx context.Context, name string, input json.RawMessage) (mcpclient.ToolResult, error) {
	close(r.started)
	select {
	case <-ctx.Done():
		return mcpclient.ToolResult{}, ctx.Err()
	case <-r.release:
		return mcpclient.ToolResult{Content: "waited"}, nil
	}
}

type llmModelFunc func(context.Context, llm.Request) (llm.Response, error)

func (f llmModelFunc) CreateMessage(ctx context.Context, request llm.Request) (llm.Response, error) {
	return f(ctx, request)
}

func preparedTextRun(conversationID string, messageID string, seq int64, content string) preparedAgentRun {
	return preparedAgentRun{
		Authorization: preparedAuthorization{
			Authorization: builtintools.Authorization{
				ActorUserID:      "user-1",
				TriggerMessageID: messageID,
			},
			Candidate: agent.AuthorizationCandidate{
				Ref:            "auth_1",
				SenderID:       "user-1",
				SenderName:     "Alice",
				MessageSeq:     seq,
				MessageSummary: content,
			},
			Ref: "auth_1",
		},
		MessageSeq: seq,
		Request: agent.Request{
			AuthorizationRef: "auth_1",
			Conversation: agent.Conversation{
				ID:   conversationID,
				Name: "AI 女菩萨",
				Type: "app",
			},
			Sender: agent.Sender{
				ID:   "user-1",
				Name: "Alice",
				Type: "user",
			},
			MessageID: messageID,
			Content:   content,
		},
		Scope: builtintools.Scope{
			ConversationID:   conversationID,
			ConversationType: "app",
		},
	}
}

type sentMessages struct {
	mu       sync.Mutex
	messages []envelope
}

func (s *sentMessages) write(_ context.Context, message envelope) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messages = append(s.messages, message)
	return nil
}

func (s *sentMessages) contents(t *testing.T) []string {
	t.Helper()

	s.mu.Lock()
	defer s.mu.Unlock()
	contents := make([]string, 0, len(s.messages))
	for _, message := range s.messages {
		var payload sendMessageRequestPayload
		if err := json.Unmarshal(message.Payload, &payload); err != nil {
			t.Fatalf("unmarshal sent payload: %v", err)
		}
		contents = append(contents, payload.Message.Content)
	}
	return contents
}

func testMessageCreatedEnvelope(t *testing.T, userID string, messageID string, seq int64, content string) envelope {
	t.Helper()

	body, err := json.Marshal(messageBody{Type: "text", Content: content})
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	return testMessageCreatedEnvelopeWithRawBody(t, userID, messageID, seq, content, body)
}

func testGroupMessageCreatedEnvelope(t *testing.T, appID string, userID string, messageID string, seq int64, content string) envelope {
	t.Helper()

	body, err := json.Marshal(messageBody{Type: "text", Content: content})
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	payload, err := json.Marshal(messageCreatedPayload{
		Conversation: conversationPayload{
			ID:   "conversation-group-1",
			Name: "产品讨论组",
			Type: "group",
		},
		Message: messagePayload{
			Body:    body,
			ID:      messageID,
			Seq:     seq,
			Summary: content,
		},
		Sender: senderPayload{
			ID:   userID,
			Name: "Alice",
			Type: "user",
		},
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return envelope{
		V:       protocolVersion,
		Kind:    kindEvent,
		ID:      "event-" + appID + "-" + messageID,
		Event:   eventMessageCreated,
		Payload: payload,
	}
}

func testMessageCreatedEnvelopeWithBody(t *testing.T, userID string, messageID string, seq int64, body map[string]any) envelope {
	t.Helper()

	rawBody, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	summary := ""
	if value, ok := body["content"].(string); ok {
		summary = value
	}
	return testMessageCreatedEnvelopeWithRawBody(t, userID, messageID, seq, summary, rawBody)
}

func testMessageCreatedEnvelopeWithRawBody(t *testing.T, userID string, messageID string, seq int64, summary string, body json.RawMessage) envelope {
	t.Helper()

	payload, err := json.Marshal(messageCreatedPayload{
		Conversation: conversationPayload{
			ID:   "conversation-1",
			Name: "AI 女菩萨",
			Type: "app",
		},
		Message: messagePayload{
			Body:    body,
			ID:      messageID,
			Seq:     seq,
			Summary: summary,
		},
		Sender: senderPayload{
			ID:   userID,
			Name: "Alice",
			Type: "user",
		},
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return envelope{
		V:       protocolVersion,
		Kind:    kindEvent,
		ID:      "event-" + messageID,
		Event:   eventMessageCreated,
		Payload: payload,
	}
}

func historyTextMessage(messageID string, seq int64, senderID string, senderName string, content string) historyMessagePayload {
	body, _ := json.Marshal(messageBody{Type: "text", Content: content})
	return historyMessagePayload{
		CreatedAt: time.Date(2026, 7, 8, 10, int(seq), 0, 0, time.UTC),
		ID:        messageID,
		Seq:       seq,
		Sender: senderPayload{
			ID:   senderID,
			Name: senderName,
			Type: "user",
		},
		Summary: content,
		Body:    body,
	}
}

func waitForSignal(t *testing.T, ch <-chan struct{}, label string) {
	t.Helper()

	select {
	case <-ch:
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for %s", label)
	}
}
