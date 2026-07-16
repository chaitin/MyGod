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
