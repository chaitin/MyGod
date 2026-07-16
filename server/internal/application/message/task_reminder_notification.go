package message

import (
	"context"
	"errors"
	"fmt"

	conversationapp "app/internal/application/conversation"
	taskapp "app/internal/application/task"
	"app/internal/store"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func (s *Service) PrepareTaskReminderNotification(ctx context.Context, tx *gorm.DB, cmd TaskReminderNotificationCommand) (*TaskNotificationResult, error) {
	if cmd.AssigneeUserID == nil {
		return nil, nil
	}
	recipient, valid, err := taskapp.ResolveNotificationRecipient(tx.WithContext(ctx), cmd.ProjectID, *cmd.AssigneeUserID)
	if err != nil || !valid {
		return nil, err
	}
	assistant, err := s.loadTaskNotificationAssistantApp(ctx, tx)
	if err != nil {
		return nil, err
	}
	if !assistant.Enabled {
		return nil, nil
	}
	conversation, err := conversationapp.EnsureBuiltinAssistantConversationTx(tx.WithContext(ctx), assistant, recipient, s.nowUTC())
	if err != nil {
		return nil, err
	}
	if s.taskReminderBodies == nil {
		return nil, errors.New("task reminder body builder is required")
	}
	body, summary, err := s.taskReminderBodies.BuildTaskReminderBody(ctx, cmd)
	if err != nil {
		return nil, err
	}
	clientMessageID := fmt.Sprintf("task-reminder:%s:%d:%s", cmd.ID, cmd.OccurrenceAt.Unix(), recipient.ID)
	existing, found, err := findExistingMessageByClientMessageID(tx, conversation.ID, store.MessageSenderTypeApp, assistant.ID, clientMessageID)
	if err != nil {
		return nil, err
	}
	if found {
		return &TaskNotificationResult{Created: false, Message: newMessage(existing), RecipientUserID: recipient.ID}, nil
	}
	now := s.nowUTC()
	message := store.Message{
		ID: uuid.NewString(), ConversationID: conversation.ID, Seq: conversation.LastMessageSeq + 1,
		SenderType: store.MessageSenderTypeApp, SenderID: &assistant.ID, ClientMessageID: &clientMessageID,
		Body: body, Summary: summary, CreatedAt: now, UpdatedAt: now,
	}
	if err := tx.Create(&message).Error; err != nil {
		return nil, err
	}
	if err := tx.Model(&store.Conversation{}).Where("id = ?", conversation.ID).Updates(map[string]any{
		"last_message_at": message.CreatedAt, "last_message_id": message.ID,
		"last_message_seq": message.Seq, "last_message_summary": message.Summary, "updated_at": now,
	}).Error; err != nil {
		return nil, err
	}
	return &TaskNotificationResult{Created: true, Message: newMessage(message), RecipientUserID: recipient.ID}, nil
}

func (s *Service) PublishTaskReminderNotification(ctx context.Context, notification *TaskNotificationResult) {
	s.PublishTaskNotification(ctx, notification)
}
