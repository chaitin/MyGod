package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"app/internal/appregistry"
	"app/internal/store"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const taskNotificationDescriptionFallback = "暂无描述"

type taskNotificationResult struct {
	Created         bool
	Message         store.Message
	RecipientUserID string
}

func (s *Server) createTaskNotificationTx(
	ctx context.Context,
	tx *gorm.DB,
	task store.Task,
) (*taskNotificationResult, error) {
	if task.AssigneeUserID == nil {
		return nil, nil
	}

	recipient, err := validateTaskAssignee(
		tx.WithContext(ctx),
		task.ProjectID,
		*task.AssigneeUserID,
	)
	if errors.Is(err, errInvalidTaskAssignee) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	assistantApp, err := s.loadTaskNotificationAssistantAppTx(ctx, tx)
	if err != nil {
		return nil, err
	}
	if !assistantApp.Enabled {
		return nil, nil
	}

	conversation, err := ensureTaskNotificationConversationTx(
		tx.WithContext(ctx),
		assistantApp,
		recipient,
	)
	if err != nil {
		return nil, err
	}

	body, summary, err := buildTaskNotificationBody(ctx, task)
	if err != nil {
		return nil, err
	}
	clientMessageID := taskNotificationClientMessageID(task, recipient.ID)
	existing, found, err := findExistingMessageByClientMessageID(
		tx,
		conversation.ID,
		store.MessageSenderTypeApp,
		assistantApp.ID,
		clientMessageID,
	)
	if err != nil {
		return nil, err
	}
	if found {
		return &taskNotificationResult{
			Created:         false,
			Message:         existing,
			RecipientUserID: recipient.ID,
		}, nil
	}

	now := time.Now().UTC()
	message := store.Message{
		ID:              uuid.NewString(),
		ConversationID:  conversation.ID,
		Seq:             conversation.LastMessageSeq + 1,
		SenderType:      store.MessageSenderTypeApp,
		SenderID:        &assistantApp.ID,
		ClientMessageID: &clientMessageID,
		Body:            body,
		Summary:         summary,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := tx.Create(&message).Error; err != nil {
		return nil, err
	}
	if err := tx.Model(&store.Conversation{}).
		Where("id = ?", conversation.ID).
		Updates(map[string]any{
			"last_message_at":      message.CreatedAt,
			"last_message_id":      message.ID,
			"last_message_seq":     message.Seq,
			"last_message_summary": message.Summary,
			"updated_at":           now,
		}).Error; err != nil {
		return nil, err
	}

	return &taskNotificationResult{
		Created:         true,
		Message:         message,
		RecipientUserID: recipient.ID,
	}, nil
}

func (s *Server) loadTaskNotificationAssistantAppTx(
	ctx context.Context,
	tx *gorm.DB,
) (store.App, error) {
	var assistantApp store.App
	err := tx.WithContext(ctx).First(&assistantApp, "id = ?", appregistry.AIAssistantAppID).Error
	if err == nil {
		return assistantApp, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return store.App{}, err
	}

	// Bootstrap normally creates the built-in assistant. Keep a transactional
	// fallback for tests and recovered databases without locking the shared app
	// row during every task update.
	return appregistry.EnsureAIAssistantApp(tx.WithContext(ctx), s.cfg.Apps)
}

func (s *Server) publishTaskNotification(
	ctx context.Context,
	notification *taskNotificationResult,
) {
	if notification == nil || !notification.Created {
		return
	}

	s.sendRealtimeMessageCreatedToUsers(
		ctx,
		[]string{notification.RecipientUserID},
		notification.Message,
	)
}

func ensureTaskNotificationConversationTx(
	tx *gorm.DB,
	assistantApp store.App,
	recipient store.User,
) (store.Conversation, error) {
	now := time.Now().UTC()
	conversationID := builtinAssistantConversationID(recipient.ID)
	candidate := store.Conversation{
		ID:              conversationID,
		Kind:            store.ConversationKindApp,
		Name:            assistantApp.Name,
		Avatar:          assistantApp.Avatar,
		CreatedByUserID: recipient.ID,
		Status:          store.ConversationStatusActive,
		PostingPolicy:   store.ConversationPostingPolicyOpen,
		Visibility:      store.ConversationVisibilityPrivate,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&candidate).Error; err != nil {
		return store.Conversation{}, err
	}

	var conversation store.Conversation
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		First(&conversation, "id = ?", conversationID).Error; err != nil {
		return store.Conversation{}, err
	}
	if err := ensureBuiltinAssistantConversationFields(
		tx,
		&conversation,
		assistantApp,
		recipient.ID,
		now,
	); err != nil {
		return store.Conversation{}, err
	}
	if err := ensureBuiltinAssistantConversationMembers(
		tx,
		conversation.ID,
		assistantApp.ID,
		recipient.ID,
		now,
	); err != nil {
		return store.Conversation{}, err
	}
	if err := ensureBuiltinAssistantAppConversation(
		tx,
		assistantApp.ID,
		conversation.ID,
		recipient.ID,
		now,
	); err != nil {
		return store.Conversation{}, err
	}

	return conversation, nil
}

func buildTaskNotificationBody(
	ctx context.Context,
	task store.Task,
) (json.RawMessage, string, error) {
	description := strings.TrimSpace(task.Description)
	if description == "" {
		description = taskNotificationDescriptionFallback
	}
	description = truncateTaskNotificationDescription(description)
	body, err := json.Marshal(cardMessageBody{
		Description: description,
		Title:       entityCardTitle("任务动态", task.Title),
		Type:        messageTypeCard,
		URL:         fmt.Sprintf("/projects/%s?taskId=%s", task.ProjectID, task.ID),
	})
	if err != nil {
		return nil, "", err
	}
	normalized, err := (cardMessageBodyHandler{}).Normalize(ctx, body)
	if err != nil {
		return nil, "", err
	}
	summary, err := (cardMessageBodyHandler{}).Summary(normalized)
	if err != nil {
		return nil, "", err
	}

	return normalized, summary, nil
}

func truncateTaskNotificationDescription(description string) string {
	characters := []rune(description)
	if len(characters) <= maxCardDescription {
		return description
	}
	if maxCardDescription <= 1 {
		return string(characters[:maxCardDescription])
	}

	return strings.TrimSpace(string(characters[:maxCardDescription-1])) + "…"
}

func taskNotificationClientMessageID(task store.Task, recipientUserID string) string {
	return fmt.Sprintf(
		"task-notification:%s:%d:%s",
		task.ID,
		task.UpdatedAt.UnixMicro(),
		recipientUserID,
	)
}
