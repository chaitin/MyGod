package message

import (
	"encoding/json"
	"errors"
	"time"

	"app/internal/store"

	"gorm.io/gorm"
)

const appEventMessageCreated = "message.created"

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
	Body      json.RawMessage `json:"body"`
	CreatedAt time.Time       `json:"created_at"`
	ID        string          `json:"id"`
	Seq       int64           `json:"seq"`
	Summary   string          `json:"summary"`
}

type appMessageSenderPayload struct {
	Email    string `json:"email,omitempty"`
	ID       string `json:"id"`
	Name     string `json:"name"`
	Nickname string `json:"nickname"`
	Type     string `json:"type"`
}

func createAppMessageEventOutbox(db *gorm.DB, conversation store.Conversation, sender store.User, message store.Message) ([]AppEvent, error) {
	var appIDs []string
	switch conversation.Kind {
	case store.ConversationKindApp:
		appID, ok, err := findMessageConversationAppID(db, message.ConversationID)
		if err != nil || !ok {
			return nil, err
		}
		appIDs = []string{appID}
	case store.ConversationKindGroup:
		var err error
		appIDs, err = findMentionedGroupAppIDs(db, conversation.ID, message.Body)
		if err != nil {
			return nil, err
		}
	default:
		return nil, nil
	}
	payload := appMessageCreatedPayload{
		Conversation: appMessageConversationPayload{ID: conversation.ID, Name: conversation.Name, Type: conversation.Kind},
		Message:      appMessagePayload{Body: message.Body, CreatedAt: message.CreatedAt, ID: message.ID, Seq: message.Seq, Summary: message.Summary},
		Sender:       appMessageSenderPayload{Email: sender.Email, ID: sender.ID, Name: sender.Name, Nickname: sender.Nickname, Type: store.MessageSenderTypeUser},
	}
	result := make([]AppEvent, 0, len(appIDs))
	for _, appID := range appIDs {
		raw, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		stored := store.AppEventOutbox{AppID: appID, Event: appEventMessageCreated, Payload: raw, CreatedAt: time.Now().UTC()}
		if err := db.Create(&stored).Error; err != nil {
			return nil, err
		}
		result = append(result, AppEvent{AppID: stored.AppID, Cursor: stored.ID, Event: stored.Event, Payload: stored.Payload})
	}
	return result, nil
}

func findMentionedGroupAppIDs(db *gorm.DB, conversationID string, body json.RawMessage) ([]string, error) {
	targets := parseMessageMentionTargets(body)
	targetSet := make(map[string]struct{}, len(targets))
	targetIDs := make([]string, 0, len(targets))
	for _, target := range targets {
		if target.All || target.MemberType != store.ConversationMemberTypeApp {
			continue
		}
		if _, ok := targetSet[target.MemberID]; ok {
			continue
		}
		targetSet[target.MemberID] = struct{}{}
		targetIDs = append(targetIDs, target.MemberID)
	}
	if len(targetIDs) == 0 {
		return nil, nil
	}
	var members []store.ConversationMember
	if err := db.Where(
		"conversation_id = ? AND member_type = ? AND member_id IN ? AND left_at IS NULL",
		conversationID, store.ConversationMemberTypeApp, targetIDs,
	).Find(&members).Error; err != nil {
		return nil, err
	}
	membership := make(map[string]struct{}, len(members))
	for _, member := range members {
		membership[member.MemberID] = struct{}{}
	}
	result := make([]string, 0, len(targetIDs))
	for _, id := range targetIDs {
		if _, ok := membership[id]; ok {
			result = append(result, id)
		}
	}
	return result, nil
}

func findMessageConversationAppID(db *gorm.DB, conversationID string) (string, bool, error) {
	var member store.ConversationMember
	err := db.First(
		&member,
		"conversation_id = ? AND member_type = ? AND left_at IS NULL",
		conversationID, store.ConversationMemberTypeApp,
	).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return member.MemberID, true, nil
}
