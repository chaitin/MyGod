package appclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"assistant/internal/agent"
	"assistant/internal/llm"
)

type topicRouterFunc func(context.Context, agent.Request) (bool, error)

func (f topicRouterFunc) NeedsTopic(ctx context.Context, request agent.Request) (bool, error) {
	return f(ctx, request)
}

func TestModelTopicRouterReturnsBooleanDecision(t *testing.T) {
	for _, needsTopic := range []bool{false, true} {
		t.Run(fmt.Sprintf("needs_topic=%t", needsTopic), func(t *testing.T) {
			var gotRequest llm.Request
			model := llmModelFunc(func(_ context.Context, request llm.Request) (llm.Response, error) {
				gotRequest = request
				input, err := json.Marshal(map[string]bool{"needs_topic": needsTopic})
				if err != nil {
					t.Fatalf("marshal tool input: %v", err)
				}
				return llm.Response{Blocks: []llm.Block{{
					Type: llm.BlockTypeToolUse, ToolUseID: "tool-route", ToolName: decideTopicToolName, ToolInput: input,
				}}}, nil
			})
			router := newModelTopicRouter(model)
			router.timeout = 0
			history := make([]agent.HistoryMessage, 12)
			for index := range history {
				history[index] = agent.HistoryMessage{SenderName: "Alice", SenderType: "user", Summary: fmt.Sprintf("历史消息 %d", index+1)}
			}

			got, err := router.NeedsTopic(context.Background(), agent.Request{
				Conversation: agent.Conversation{ID: "conversation-1", Name: "产品群", Type: "group"},
				Content:      "帮我分析最近一个月的项目进展",
				History:      history,
			})
			if err != nil {
				t.Fatalf("NeedsTopic() error = %v", err)
			}
			if got != needsTopic {
				t.Fatalf("NeedsTopic() = %t, want %t", got, needsTopic)
			}
			if gotRequest.System != topicRouterSystemPrompt || len(gotRequest.Tools) != 1 || gotRequest.Tools[0].Name != decideTopicToolName {
				t.Fatalf("model request = %#v", gotRequest)
			}
			if len(gotRequest.Messages) != 1 {
				t.Fatalf("model messages = %#v", gotRequest.Messages)
			}
			var payload topicRoutingPayload
			if err := json.Unmarshal([]byte(gotRequest.Messages[0].Content), &payload); err != nil {
				t.Fatalf("decode routing payload: %v", err)
			}
			if payload.CurrentMessage != "帮我分析最近一个月的项目进展" || payload.Conversation.Type != "group" {
				t.Fatalf("routing payload = %#v", payload)
			}
			if len(payload.RecentHistory) != defaultTopicRouterHistory || payload.RecentHistory[0].Summary != "历史消息 3" {
				t.Fatalf("recent history = %#v", payload.RecentHistory)
			}
		})
	}
}

func TestModelTopicRouterRejectsInvalidDecisions(t *testing.T) {
	tests := []struct {
		name     string
		response llm.Response
	}{
		{
			name:     "text instead of tool",
			response: llm.Response{Blocks: []llm.Block{{Type: llm.BlockTypeText, Text: `{"needs_topic":false}`}}},
		},
		{
			name: "missing boolean",
			response: llm.Response{Blocks: []llm.Block{{
				Type: llm.BlockTypeToolUse, ToolName: decideTopicToolName, ToolInput: json.RawMessage(`{}`),
			}}},
		},
		{
			name: "unknown property",
			response: llm.Response{Blocks: []llm.Block{{
				Type: llm.BlockTypeToolUse, ToolName: decideTopicToolName,
				ToolInput: json.RawMessage(`{"needs_topic":false,"reason":"simple"}`),
			}}},
		},
		{
			name: "wrong type",
			response: llm.Response{Blocks: []llm.Block{{
				Type: llm.BlockTypeToolUse, ToolName: decideTopicToolName, ToolInput: json.RawMessage(`{"needs_topic":"false"}`),
			}}},
		},
		{
			name: "multiple calls",
			response: llm.Response{Blocks: []llm.Block{
				{Type: llm.BlockTypeToolUse, ToolName: decideTopicToolName, ToolInput: json.RawMessage(`{"needs_topic":false}`)},
				{Type: llm.BlockTypeToolUse, ToolName: decideTopicToolName, ToolInput: json.RawMessage(`{"needs_topic":true}`)},
			}},
		},
		{
			name: "unexpected tool",
			response: llm.Response{Blocks: []llm.Block{{
				Type: llm.BlockTypeToolUse, ToolName: "reply", ToolInput: json.RawMessage(`{"needs_topic":false}`),
			}}},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			router := newModelTopicRouter(llmModelFunc(func(context.Context, llm.Request) (llm.Response, error) {
				return test.response, nil
			}))
			router.timeout = 0
			if _, err := router.NeedsTopic(context.Background(), agent.Request{
				Conversation: agent.Conversation{Type: "app"}, Content: "你好",
			}); err == nil {
				t.Fatal("NeedsTopic() error = nil, want invalid decision error")
			}
		})
	}
}

func TestHandleParsedServerMessageRepliesInCurrentConversationWhenTopicIsNotNeeded(t *testing.T) {
	appID := "00000000-0000-0000-0000-000000000001"
	var agentRequests []agent.Request
	var sent []envelope
	requester := appRequestFunc(func(_ context.Context, method string, _ any) (json.RawMessage, error) {
		if method != methodConversationMessagesList {
			t.Fatalf("unexpected app request method %q", method)
		}
		return json.Marshal(appListConversationMessagesResponsePayload{
			Messages: []historyMessagePayload{
				historyTextMessage("message-1", 1, "user-1", "Alice", "你好 {(@app/"+appID+")}"),
			},
		})
	})
	router := topicRouterFunc(func(_ context.Context, request agent.Request) (bool, error) {
		if request.Conversation.ID != "conversation-group-1" || !strings.Contains(request.Content, "你好") {
			t.Fatalf("routing request = %#v", request)
		}
		return false, nil
	})
	replyAgent := replyAgentFunc(func(ctx context.Context, request agent.Request, sink agent.OutputSink) error {
		agentRequests = append(agentRequests, request)
		return sink.SendMarkdown(ctx, "你好")
	})

	handled := handleParsedServerMessageWithTopicRouter(
		context.Background(),
		testGroupMessageCreatedEnvelope(t, appID, "user-1", "message-1", 1, "你好 {(@app/"+appID+")}"),
		appID,
		requester,
		replyAgent,
		router,
		directAgentRunner{},
		func(_ context.Context, message envelope) error {
			sent = append(sent, message)
			return nil
		},
	)

	if !handled {
		t.Fatal("handleParsedServerMessageWithTopicRouter() = false, want true")
	}
	if len(agentRequests) != 1 || agentRequests[0].Conversation.ID != "conversation-group-1" || agentRequests[0].Conversation.Type != "group" {
		t.Fatalf("agent requests = %#v", agentRequests)
	}
	if len(sent) != 1 {
		t.Fatalf("sent messages = %d, want 1", len(sent))
	}
	var reply sendMessageRequestPayload
	if err := json.Unmarshal(sent[0].Payload, &reply); err != nil {
		t.Fatalf("decode reply: %v", err)
	}
	if reply.Target.Type != "group" || reply.Target.ConversationID != "conversation-group-1" {
		t.Fatalf("reply target = %#v", reply.Target)
	}
}

func TestHandleParsedServerMessageDefaultsToTopicWhenRoutingFails(t *testing.T) {
	appID := "00000000-0000-0000-0000-000000000001"
	var sent []envelope
	requester := appRequestFunc(func(_ context.Context, method string, _ any) (json.RawMessage, error) {
		switch method {
		case methodConversationMessagesList:
			return json.Marshal(appListConversationMessagesResponsePayload{
				Messages: []historyMessagePayload{
					historyTextMessage("message-1", 1, "user-1", "Alice", "请处理 {(@app/"+appID+")}"),
				},
			})
		case methodConversationTopicCreate:
			return testTopicMutationResponse("topic-1", "请处理")
		default:
			t.Fatalf("unexpected app request method %q", method)
			return nil, nil
		}
	})
	router := topicRouterFunc(func(context.Context, agent.Request) (bool, error) {
		return false, errors.New("invalid model response")
	})
	replyAgent := replyAgentFunc(func(ctx context.Context, _ agent.Request, sink agent.OutputSink) error {
		return sink.SendMarkdown(ctx, "开始处理")
	})

	handled := handleParsedServerMessageWithTopicRouter(
		context.Background(),
		testGroupMessageCreatedEnvelope(t, appID, "user-1", "message-1", 1, "请处理 {(@app/"+appID+")}"),
		appID,
		requester,
		replyAgent,
		router,
		directAgentRunner{},
		func(_ context.Context, message envelope) error {
			sent = append(sent, message)
			return nil
		},
	)

	if !handled || len(sent) != 1 {
		t.Fatalf("handled = %t, sent = %d", handled, len(sent))
	}
	var reply sendMessageRequestPayload
	if err := json.Unmarshal(sent[0].Payload, &reply); err != nil {
		t.Fatalf("decode reply: %v", err)
	}
	if reply.Target.Type != "topic" || reply.Target.ConversationID != "topic-1" {
		t.Fatalf("reply target = %#v", reply.Target)
	}
}
