package httpserver

import (
	"encoding/json"
	"testing"

	conversationapp "app/internal/application/conversation"
	messageapp "app/internal/application/message"
)

func TestConversationApplicationMessageResponseUsesEmptyReactionArray(t *testing.T) {
	response := newConversationApplicationMessageResponse(conversationapp.Message{
		ConversationID: "conversation-id", ID: "message-id",
		Sender: conversationapp.MessageIdentity{Type: "system"},
	})
	encoded, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(encoded, &payload); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if reactions, ok := payload["reactions"].([]any); !ok || len(reactions) != 0 {
		t.Fatalf("reactions = %#v, want empty array", payload["reactions"])
	}
}

func TestMessageReactionEventIncludesUsers(t *testing.T) {
	event := realtimeMessageReactionsUpdatedEvent(messageapp.ReactionEvent{
		ConversationID: "conversation-id", MessageID: "message-id", ReactionVersion: 2,
		Reactions: []messageapp.ReactionCount{{
			Count: 4, Text: "👍", Users: []messageapp.ReactionUser{
				{ID: "user-1", Name: "Alice"}, {ID: "user-2", Name: "Bob"}, {ID: "user-3", Name: "Carol"},
			},
		}},
	})
	var payload messageReactionsUpdatedEventPayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		t.Fatalf("unmarshal reaction event: %v", err)
	}
	if len(payload.Reactions) != 1 || len(payload.Reactions[0].Users) != 3 ||
		payload.Reactions[0].Users[2] != (messageReactionUserResponse{ID: "user-3", Name: "Carol"}) {
		t.Fatalf("reaction event payload = %#v", payload)
	}
}
