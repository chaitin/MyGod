package realtime

import (
	"encoding/json"

	"github.com/google/uuid"
)

const (
	ProtocolVersion = 1

	KindRequest  = "request"
	KindResponse = "response"
	KindEvent    = "event"

	EventMessageCreated          = "message.created"
	EventMessageUpdated          = "message.updated"
	EventMessageReactionsUpdated = "message.reactions_updated"
	EventMemberMentioned         = "conversation.member_mentioned"
	EventMemberChoiceReceived    = "conversation.member_choice_received"
	EventMessageChoiceUpdated    = "message.choice_updated"
	EventConversationMuteUpdated = "conversation.mute_updated"
	EventConversationPinUpdated  = "conversation.pin_updated"
	EventConversationRemoved     = "conversation.removed"
	EventConversationRestored    = "conversation.restored"
	EventTopicCreated            = "topic.created"
	EventTopicParticipated       = "topic.participated"
	EventTopicArchived           = "topic.archived"
	EventTopicClosed             = "topic.closed"
	EventSystemReady             = "system.ready"
)

type Envelope struct {
	V       int             `json:"v"`
	Kind    string          `json:"kind"`
	Cursor  int64           `json:"cursor,omitempty"`
	ID      string          `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Event   string          `json:"event,omitempty"`
	ReplyTo string          `json:"reply_to,omitempty"`
	OK      *bool           `json:"ok,omitempty"`
	Payload json.RawMessage `json:"payload,omitempty"`
	Error   *ErrorPayload   `json:"error,omitempty"`
}

type ErrorPayload struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func NewEvent(event string, payload any) Envelope {
	return Envelope{
		V:       ProtocolVersion,
		Kind:    KindEvent,
		ID:      uuid.NewString(),
		Event:   event,
		Payload: mustMarshalPayload(payload),
	}
}

func NewCursorEvent(cursor int64, event string, payload any) Envelope {
	message := NewEvent(event, payload)
	message.Cursor = cursor
	return message
}

func NewResponse(replyTo string, payload any) Envelope {
	ok := true
	return Envelope{
		V:       ProtocolVersion,
		Kind:    KindResponse,
		ID:      uuid.NewString(),
		ReplyTo: replyTo,
		OK:      &ok,
		Payload: mustMarshalPayload(payload),
	}
}

func NewErrorResponse(replyTo string, code string, message string) Envelope {
	ok := false
	return Envelope{
		V:       ProtocolVersion,
		Kind:    KindResponse,
		ID:      uuid.NewString(),
		ReplyTo: replyTo,
		OK:      &ok,
		Error: &ErrorPayload{
			Code:    code,
			Message: message,
		},
	}
}

func mustMarshalPayload(payload any) json.RawMessage {
	if payload == nil {
		return nil
	}

	content, err := json.Marshal(payload)
	if err != nil {
		panic(err)
	}

	return content
}
