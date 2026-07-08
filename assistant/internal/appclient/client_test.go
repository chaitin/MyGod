package appclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"
	"time"

	"assistant/internal/agent"
	"assistant/internal/config"
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
			ID:   "user-1",
			Name: "Alice",
			Type: "user",
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

	handleServerMessage(context.Background(), websocket.TextMessage, raw, requester, replyAgent, func(message envelope) error {
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
	if historyPayload.Limit != defaultConversationContextLimit {
		t.Fatalf("history limit = %d, want %d", historyPayload.Limit, defaultConversationContextLimit)
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

func TestNewToolRegistryIncludesBuiltinSleepTool(t *testing.T) {
	registry, sources, err := newToolRegistry(context.Background(), nil)
	defer func() {
		if sources != nil {
			mcpclient.CloseSources(sources)
		}
	}()
	if err != nil {
		t.Fatalf("newToolRegistry() error = %v", err)
	}

	for _, tool := range registry.Tools() {
		if tool.Name == "builtin__sleep" {
			return
		}
	}
	t.Fatalf("tools = %+v, want builtin__sleep", registry.Tools())
}

func TestUserAgentRunnerCancelsPreviousMessageFromSameUser(t *testing.T) {
	runner := newUserAgentRunner()
	requester := appRequestFunc(func(ctx context.Context, method string, payload any) (json.RawMessage, error) {
		return json.Marshal(appListConversationMessagesResponsePayload{})
	})

	firstStarted := make(chan struct{})
	firstCanceled := make(chan struct{})
	secondDone := make(chan struct{})
	replyAgent := replyAgentFunc(func(ctx context.Context, request agent.Request, sink agent.OutputSink) error {
		switch request.MessageID {
		case "message-1":
			close(firstStarted)
			<-ctx.Done()
			close(firstCanceled)
			_ = sink.SendMarkdown(context.Background(), "旧回复")
			return ctx.Err()
		case "message-2":
			err := sink.SendMarkdown(ctx, "新回复")
			close(secondDone)
			return err
		default:
			t.Fatalf("unexpected message id %q", request.MessageID)
			return nil
		}
	})

	var sent sentMessages
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	handleParsedServerMessage(ctx, testMessageCreatedEnvelope(t, "user-1", "message-1", 1, "第一条"), requester, replyAgent, runner, sent.write)
	waitForSignal(t, firstStarted, "first agent to start")

	handleParsedServerMessage(ctx, testMessageCreatedEnvelope(t, "user-1", "message-2", 2, "第二条"), requester, replyAgent, runner, sent.write)
	waitForSignal(t, firstCanceled, "first agent to be canceled")
	waitForSignal(t, secondDone, "second agent to finish")

	if got := sent.contents(t); !slices.Equal(got, []string{"新回复"}) {
		t.Fatalf("sent messages = %v, want only latest reply", got)
	}
}

func TestUserAgentRunnerDoesNotCancelDifferentUsers(t *testing.T) {
	runner := newUserAgentRunner()
	firstStarted := make(chan struct{})
	firstReleased := make(chan struct{})
	firstDone := make(chan struct{})
	firstCanceled := make(chan struct{}, 1)
	secondDone := make(chan struct{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sink := agent.OutputSinkFunc(func(ctx context.Context, content string) error {
		return nil
	})

	runner.Start(ctx, "user-1", sink, func(ctx context.Context, sink agent.OutputSink) error {
		close(firstStarted)
		select {
		case <-ctx.Done():
			firstCanceled <- struct{}{}
			return ctx.Err()
		case <-firstReleased:
			close(firstDone)
			return nil
		}
	})
	waitForSignal(t, firstStarted, "first user job to start")

	runner.Start(ctx, "user-2", sink, func(ctx context.Context, sink agent.OutputSink) error {
		close(secondDone)
		return nil
	})
	waitForSignal(t, secondDone, "second user job to finish")

	select {
	case <-firstCanceled:
		t.Fatal("first user job was canceled by different user's message")
	default:
	}

	close(firstReleased)
	waitForSignal(t, firstDone, "first user job to finish")
}

func TestUserAgentRunnerCancelAllCancelsOutstandingJobs(t *testing.T) {
	runner := newUserAgentRunner()
	firstStarted := make(chan struct{})
	firstCanceled := make(chan struct{})
	secondStarted := make(chan struct{})
	secondCanceled := make(chan struct{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sink := agent.OutputSinkFunc(func(ctx context.Context, content string) error {
		return nil
	})

	runner.Start(ctx, "user-1", sink, func(ctx context.Context, sink agent.OutputSink) error {
		close(firstStarted)
		<-ctx.Done()
		close(firstCanceled)
		return ctx.Err()
	})
	runner.Start(ctx, "user-2", sink, func(ctx context.Context, sink agent.OutputSink) error {
		close(secondStarted)
		<-ctx.Done()
		close(secondCanceled)
		return ctx.Err()
	})
	waitForSignal(t, firstStarted, "first user job to start")
	waitForSignal(t, secondStarted, "second user job to start")

	runner.CancelAll()

	waitForSignal(t, firstCanceled, "first user job to be canceled")
	waitForSignal(t, secondCanceled, "second user job to be canceled")
}

func TestUserAgentRunnerKeepsCurrentCheckAtomicWithSend(t *testing.T) {
	runner := newUserAgentRunner()
	sendStarted := make(chan struct{})
	releaseSend := make(chan struct{})
	firstDone := make(chan struct{})
	replacementStartAttempted := make(chan struct{})
	replacementReturned := make(chan struct{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	blockingSink := agent.OutputSinkFunc(func(ctx context.Context, content string) error {
		close(sendStarted)
		<-releaseSend
		return nil
	})

	runner.Start(ctx, "user-1", blockingSink, func(ctx context.Context, sink agent.OutputSink) error {
		err := sink.SendMarkdown(ctx, "旧回复")
		close(firstDone)
		return err
	})
	waitForSignal(t, sendStarted, "first send to start")

	go func() {
		close(replacementStartAttempted)
		runner.Start(ctx, "user-1", agent.OutputSinkFunc(func(ctx context.Context, content string) error {
			return nil
		}), func(ctx context.Context, sink agent.OutputSink) error {
			return nil
		})
		close(replacementReturned)
	}()
	waitForSignal(t, replacementStartAttempted, "replacement job to start")

	select {
	case <-replacementReturned:
		t.Fatal("replacement job started while previous send was between current check and write")
	case <-time.After(50 * time.Millisecond):
	}

	close(releaseSend)
	waitForSignal(t, firstDone, "first send to finish")
	waitForSignal(t, replacementReturned, "replacement job to start")
}

type sentMessages struct {
	mu       sync.Mutex
	messages []envelope
}

func (s *sentMessages) write(message envelope) error {
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
		ID:      "event-" + messageID,
		Event:   eventMessageCreated,
		Payload: payload,
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
