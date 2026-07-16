package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	entitycardapp "app/internal/application/entitycard"
	messageapp "app/internal/application/message"
)

const (
	messageTypeEntityCard = "entity_card"

	entityCardTypeUser    = entitycardapp.TypeUser
	entityCardTypeApp     = entitycardapp.TypeApp
	entityCardTypeGroup   = entitycardapp.TypeGroup
	entityCardTypeProject = entitycardapp.TypeProject
	entityCardTypeTask    = entitycardapp.TypeTask
)

var errEntityCardNotFound = errors.New("对象不存在或无权访问")

type entityCardRequestError struct {
	message string
}

func (err *entityCardRequestError) Error() string {
	return err.message
}

type entityCardMessageBody struct {
	EntityID   string `json:"entity_id"`
	EntityType string `json:"entity_type"`
	Type       string `json:"type"`
}

func isEntityCardMessageBody(raw json.RawMessage) bool {
	var envelope messageBodyEnvelope
	return json.Unmarshal(raw, &envelope) == nil && strings.TrimSpace(envelope.Type) == messageTypeEntityCard
}

func (s *Server) resolveEntityCardMessageBody(ctx context.Context, userID string, raw json.RawMessage) (json.RawMessage, error) {
	body, err := s.messageContentService().Prepare(ctx, userID, raw)
	if err != nil {
		switch messageapp.ErrorCodeOf(err) {
		case messageapp.CodeInvalidRequest:
			return nil, newEntityCardRequestError(messageapp.ErrorMessage(err))
		case messageapp.CodeNotFound:
			return nil, errEntityCardNotFound
		default:
			return nil, err
		}
	}
	return body, nil
}

func entityCardTitle(entityLabel, entityName string) string {
	return entitycardapp.Title(entityLabel, entityName)
}

func newEntityCardRequestError(message string) error {
	return &entityCardRequestError{message: message}
}
