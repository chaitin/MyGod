package httpserver

import (
	"encoding/json"
	"errors"
	"time"

	"app/internal/realtime"
	"app/internal/store"

	"gorm.io/gorm"
)

type appMessageCreatedPayload struct {
	Conversation appMessageConversationPayload `json:"conversation"`
	Message      appMessagePayload             `json:"message"`
	Sender       appMessageSenderPayload       `json:"sender"`
}

type appMessageConversationPayload struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
}

type appMessagePayload struct {
	Body        json.RawMessage          `json:"body"`
	CreatedAt   time.Time                `json:"created_at"`
	DelegatedBy *appMessageSenderPayload `json:"delegated_by,omitempty"`
	ID          string                   `json:"id"`
	Seq         int64                    `json:"seq"`
	Sender      *appMessageSenderPayload `json:"sender,omitempty"`
	Summary     string                   `json:"summary"`
}

type appMessageSenderPayload struct {
	Email    string `json:"email,omitempty"`
	ID       string `json:"id"`
	Name     string `json:"name"`
	Nickname string `json:"nickname"`
	Type     string `json:"type"`
}

func (s *Server) dispatchAppMessageCreatedEvent(sender store.User, message store.Message) error {
	conversation, appID, ok, err := s.findMessageConversationApp(message.ConversationID)
	if err != nil || !ok {
		return err
	}

	if s.appConnections == nil {
		return nil
	}

	s.appConnections.SendToApp(appID, realtime.NewEvent(realtime.EventMessageCreated, appMessageCreatedPayload{
		Conversation: appMessageConversationPayload{
			ID:   conversation.ID,
			Name: conversation.Name,
			Type: conversation.Kind,
		},
		Message: appMessagePayload{
			Body:      message.Body,
			CreatedAt: message.CreatedAt,
			ID:        message.ID,
			Seq:       message.Seq,
			Summary:   message.Summary,
		},
		Sender: appMessageSenderPayload{
			Email:    sender.Email,
			ID:       sender.ID,
			Name:     sender.Name,
			Nickname: sender.Nickname,
			Type:     store.MessageSenderTypeUser,
		},
	}))

	return nil
}

func (s *Server) findMessageConversationApp(conversationID string) (store.Conversation, string, bool, error) {
	var conversation store.Conversation
	if err := s.db.First(&conversation, "id = ?", conversationID).Error; err != nil {
		return store.Conversation{}, "", false, err
	}
	if conversation.Kind != store.ConversationKindApp {
		return store.Conversation{}, "", false, nil
	}

	var member store.ConversationMember
	err := s.db.First(
		&member,
		"conversation_id = ? AND member_type = ? AND left_at IS NULL",
		conversationID,
		store.ConversationMemberTypeApp,
	).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return store.Conversation{}, "", false, nil
	}
	if err != nil {
		return store.Conversation{}, "", false, err
	}

	return conversation, member.MemberID, true, nil
}
