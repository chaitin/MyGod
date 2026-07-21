package httpserver

import (
	"context"

	messageapp "app/internal/application/message"
	"app/internal/realtime"
	"app/internal/store"
)

func (s *Server) PublishMessageCreated(_ context.Context, deliveries []messageapp.Delivery) {
	for _, delivery := range deliveries {
		s.realtime.SendToUsers(
			[]string{delivery.UserID},
			realtimeMessageCreatedEvent(legacyMessageResponse(delivery.Message)),
		)
	}
}

func (s *Server) PublishSharedMessageCreated(_ context.Context, userIDs []string, message messageapp.Message) {
	s.realtime.SendToUsers(userIDs, realtimeMessageCreatedEvent(legacyMessageResponse(message)))
}

func (s *Server) PublishMessageUpdated(_ context.Context, deliveries []messageapp.Delivery) {
	for _, delivery := range deliveries {
		s.realtime.SendToUsers(
			[]string{delivery.UserID},
			realtimeMessageUpdatedEvent(legacyMessageResponse(delivery.Message)),
		)
	}
}

func (s *Server) PublishMembersMentioned(_ context.Context, userIDs []string, conversationID string, seq int64) {
	if len(userIDs) == 0 {
		return
	}
	s.realtime.SendToUsers(userIDs, realtimeConversationMemberMentionedEvent(conversationID, seq))
}

func (s *Server) PublishMessageReactionsUpdated(_ context.Context, userIDs []string, event messageapp.ReactionEvent) {
	if len(userIDs) == 0 {
		return
	}
	s.realtime.SendToUsers(userIDs, realtimeMessageReactionsUpdatedEvent(event))
}

func (s *Server) DeliverAppEvents(_ context.Context, events []messageapp.AppEvent) {
	if s.appConnections == nil {
		return
	}
	for _, event := range events {
		s.appConnections.SendToApp(event.AppID, realtime.NewCursorEvent(event.Cursor, event.Event, event.Payload))
	}
}

func legacyMessageResponse(value messageapp.Message) messageResponse {
	reactions := make([]messageReactionResponse, len(value.Reactions))
	for index, reaction := range value.Reactions {
		reactions[index] = messageReactionResponse{
			Count: reaction.Count, ReactedByMe: reaction.ReactedByMe, Text: reaction.Text,
			Users: newLegacyMessageReactionUserResponses(reaction.Users),
		}
	}
	response := messageResponse{
		ClientMessageID:  value.ClientMessageID,
		Body:             value.Body,
		ConversationID:   value.ConversationID,
		CreatedAt:        value.CreatedAt,
		ID:               value.ID,
		ReplyToMessageID: value.ReplyToMessageID,
		ReactionVersion:  value.ReactionVersion,
		Reactions:        reactions,
		RevokedAt:        value.RevokedAt,
		RevokedByUserID:  value.RevokedByUserID,
		Sender:           messageSenderResponse{ID: value.Sender.ID, Type: value.Sender.Type},
		Seq:              value.Seq,
	}
	if value.DelegatedBy != nil {
		response.DelegatedBy = &messageDelegatedByResponse{
			ID: value.DelegatedBy.ID, Name: value.DelegatedBy.Name, Type: value.DelegatedBy.Type,
		}
	}
	if value.ReplyTo != nil {
		response.ReplyTo = &messageReplyToResponse{
			ID: value.ReplyTo.ID,
			Sender: messageReplyToSenderResponse{
				ID: value.ReplyTo.Sender.ID, Name: value.ReplyTo.Sender.Name, Type: value.ReplyTo.Sender.Type,
			},
			Seq: value.ReplyTo.Seq, Summary: value.ReplyTo.Summary,
		}
	}
	if value.Topic != nil {
		recentReplies := make([]messageTopicReplyResponse, len(value.Topic.RecentReplies))
		for index, reply := range value.Topic.RecentReplies {
			recentReplies[index] = messageTopicReplyResponse{
				CreatedAt: reply.CreatedAt, ID: reply.ID,
				Sender: messageSenderResponse{ID: reply.Sender.ID, Type: reply.Sender.Type}, Summary: reply.Summary,
			}
		}
		response.Topic = &messageTopicResponse{
			Archived: value.Topic.Archived, ConversationID: value.Topic.ConversationID,
			RecentReplies: recentReplies,
		}
	}
	return response
}

func newLegacyMessageReactionUserResponses(values []messageapp.ReactionUser) []messageReactionUserResponse {
	result := make([]messageReactionUserResponse, len(values))
	for index, value := range values {
		result[index] = messageReactionUserResponse{ID: value.ID, Name: value.Name}
	}
	return result
}

func legacyStoredMessage(value messageapp.Message) store.Message {
	var senderID *string
	if value.Sender.ID != "" {
		sender := value.Sender.ID
		senderID = &sender
	}
	var clientMessageID *string
	if value.ClientMessageID != "" {
		clientID := value.ClientMessageID
		clientMessageID = &clientID
	}
	return store.Message{
		ID: value.ID, ConversationID: value.ConversationID, Seq: value.Seq,
		SenderType: value.Sender.Type, SenderID: senderID, ClientMessageID: clientMessageID,
		Body: value.Body, Summary: value.Summary, CreatedAt: value.CreatedAt,
	}
}
