package httpserver

import (
	"context"

	conversationapp "app/internal/application/conversation"
)

func (s *Server) PublishConversationMessage(_ context.Context, userIDs []string, message conversationapp.Message) {
	s.realtime.SendToUsers(userIDs, realtimeMessageCreatedEvent(newConversationApplicationMessageResponse(message)))
}

func (s *Server) PublishConversationRemoved(_ context.Context, userIDs []string, conversationID string) {
	s.realtime.SendToUsers(userIDs, realtimeConversationRemovedEvent(conversationID))
}

func newConversationApplicationMessageResponse(message conversationapp.Message) messageResponse {
	response := messageResponse{
		ClientMessageID: message.ClientMessageID,
		ConversationID:  message.ConversationID,
		CreatedAt:       message.CreatedAt,
		ID:              message.ID,
		Sender:          messageSenderResponse{ID: message.Sender.ID, Type: message.Sender.Type},
		Seq:             message.Seq,
	}
	if message.RevokedAt == nil {
		response.Body = message.Body
	} else {
		response.RevokedAt = message.RevokedAt
		response.RevokedByUserID = message.RevokedByUserID
	}
	response.ReplyToMessageID = message.ReplyToMessageID
	if message.DelegatedBy != nil {
		response.DelegatedBy = &messageDelegatedByResponse{
			ID: message.DelegatedBy.ID, Name: message.DelegatedBy.Name, Type: message.DelegatedBy.Type,
		}
	}
	return response
}
