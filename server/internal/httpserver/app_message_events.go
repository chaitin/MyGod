package httpserver

import (
	"encoding/json"
	"errors"
	"time"

	"app/internal/appconnection"
	"app/internal/realtime"
	"app/internal/store"

	"gorm.io/gorm"
)

const appEventReplayPageSize = 100

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

func (s *Server) createAppMessageEventOutbox(tx *gorm.DB, conversation store.Conversation, sender store.User, message store.Message) ([]store.AppEventOutbox, error) {
	var appIDs []string
	switch conversation.Kind {
	case store.ConversationKindApp:
		appID, ok, err := findMessageConversationAppID(tx, message.ConversationID)
		if err != nil || !ok {
			return nil, err
		}
		appIDs = []string{appID}
	case store.ConversationKindGroup:
		var err error
		appIDs, err = findMentionedGroupAppIDs(tx, conversation.ID, message.Body)
		if err != nil {
			return nil, err
		}
	default:
		return nil, nil
	}

	payload := appMessageCreatedPayload{
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
	}
	events := make([]store.AppEventOutbox, 0, len(appIDs))
	for _, appID := range appIDs {
		stored, err := createStoredAppEvent(tx, appID, realtime.EventMessageCreated, payload)
		if err != nil {
			return nil, err
		}
		events = append(events, stored)
	}

	return events, nil
}

func createStoredAppEvent(db *gorm.DB, appID string, event string, payload any) (store.AppEventOutbox, error) {
	rawPayload, err := json.Marshal(payload)
	if err != nil {
		return store.AppEventOutbox{}, err
	}
	stored := store.AppEventOutbox{
		AppID:     appID,
		Event:     event,
		Payload:   rawPayload,
		CreatedAt: time.Now().UTC(),
	}
	if err := db.Create(&stored).Error; err != nil {
		return store.AppEventOutbox{}, err
	}

	return stored, nil
}

func (s *Server) deliverStoredAppEvents(events []store.AppEventOutbox) {
	if s.appConnections == nil {
		return
	}
	for _, event := range events {
		s.appConnections.SendToApp(event.AppID, realtime.NewCursorEvent(event.ID, event.Event, event.Payload))
	}
}

func (s *Server) replayAppEvents(appID string, conn *appconnection.Connection) error {
	var ack store.AppEventAck
	lastAckedCursor := int64(0)
	err := s.db.First(&ack, "app_id = ?", appID).Error
	if err == nil {
		lastAckedCursor = ack.LastAckedCursor
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}

	nextCursor := lastAckedCursor
	for {
		var events []store.AppEventOutbox
		if err := s.db.
			Where("app_id = ? AND id > ?", appID, nextCursor).
			Order("id ASC").
			Limit(appEventReplayPageSize).
			Find(&events).Error; err != nil {
			return err
		}
		for _, event := range events {
			if !conn.EnqueueReliable(realtime.NewCursorEvent(event.ID, event.Event, event.Payload)) {
				return errors.New("app connection closed during event replay")
			}
		}
		if len(events) < appEventReplayPageSize {
			return nil
		}
		nextCursor = events[len(events)-1].ID
	}
}

func findMentionedGroupAppIDs(db *gorm.DB, conversationID string, body json.RawMessage) ([]string, error) {
	targets := parseMessageMentionTargets(body)
	if len(targets) == 0 {
		return nil, nil
	}

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
	if err := db.
		Where(
			"conversation_id = ? AND member_type = ? AND member_id IN ? AND left_at IS NULL",
			conversationID,
			store.ConversationMemberTypeApp,
			targetIDs,
		).
		Find(&members).Error; err != nil {
		return nil, err
	}

	memberSet := make(map[string]struct{}, len(members))
	for _, member := range members {
		memberSet[member.MemberID] = struct{}{}
	}

	appIDs := make([]string, 0, len(targetIDs))
	for _, targetID := range targetIDs {
		if _, ok := memberSet[targetID]; ok {
			appIDs = append(appIDs, targetID)
		}
	}

	return appIDs, nil
}

func findMessageConversationAppID(db *gorm.DB, conversationID string) (string, bool, error) {
	var member store.ConversationMember
	err := db.First(
		&member,
		"conversation_id = ? AND member_type = ? AND left_at IS NULL",
		conversationID,
		store.ConversationMemberTypeApp,
	).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}

	return member.MemberID, true, nil
}
