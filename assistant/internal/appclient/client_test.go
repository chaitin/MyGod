package appclient

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
)

func TestHandleServerMessageSendsFallbackReply(t *testing.T) {
	body, err := json.Marshal(textMessageBody{
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
	handleServerMessage(websocket.TextMessage, raw, func(message envelope) error {
		sent = append(sent, message)
		return nil
	})

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
	if requestPayload.Message.Type != "text" {
		t.Fatalf("message.type = %q, want text", requestPayload.Message.Type)
	}
	if requestPayload.Message.Content != fallbackReplyContent {
		t.Fatalf("message.content = %q, want fallback reply", requestPayload.Message.Content)
	}
	if strings.Contains(requestPayload.Message.Content, "大模型") {
		t.Fatalf("message.content = %q, should not mention removed model configuration", requestPayload.Message.Content)
	}
}
