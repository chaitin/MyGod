package message

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	conversationapp "app/internal/application/conversation"
	taskapp "app/internal/application/task"
	"app/internal/appregistry"
	"app/internal/store"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func (s *Service) PrepareTaskNotification(
	ctx context.Context,
	tx *gorm.DB,
	cmd TaskNotificationCommand,
) (*TaskNotificationResult, error) {
	if cmd.AssigneeUserID == nil {
		return nil, nil
	}
	recipient, valid, err := taskapp.ResolveNotificationRecipient(
		tx.WithContext(ctx), cmd.ProjectID, *cmd.AssigneeUserID,
	)
	if !valid {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	assistant, err := s.loadTaskNotificationAssistantApp(ctx, tx)
	if err != nil {
		return nil, err
	}
	if !assistant.Enabled {
		return nil, nil
	}
	conversation, err := conversationapp.EnsureBuiltinAssistantConversationTx(
		tx.WithContext(ctx), assistant, recipient, s.nowUTC(),
	)
	if err != nil {
		return nil, err
	}
	if s.taskNotificationBodies == nil {
		return nil, errors.New("task notification body builder is required")
	}
	cmd.AssigneeName = taskNotificationUserDisplayName(recipient)
	body, summary, err := s.taskNotificationBodies.BuildTaskNotificationBody(ctx, cmd)
	if err != nil {
		return nil, err
	}
	clientMessageID := fmt.Sprintf(
		"task-notification:%s:%d:%s", cmd.ID, cmd.UpdatedAt.UnixMicro(), recipient.ID,
	)
	existing, found, err := findExistingMessageByClientMessageID(
		tx, conversation.ID, store.MessageSenderTypeApp, assistant.ID, clientMessageID,
	)
	if err != nil {
		return nil, err
	}
	if found {
		return &TaskNotificationResult{
			Created: false, Message: newMessage(existing), RecipientUserID: recipient.ID,
		}, nil
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
	return &TaskNotificationResult{
		Created: true, Message: newMessage(message), RecipientUserID: recipient.ID,
	}, nil
}

func taskNotificationUserDisplayName(user store.User) string {
	if nickname := strings.TrimSpace(user.Nickname); nickname != "" {
		return nickname
	}
	return strings.TrimSpace(user.Name)
}

func (s *Service) PublishTaskNotification(ctx context.Context, notification *TaskNotificationResult) {
	if notification == nil || !notification.Created || s.notifications == nil {
		return
	}
	s.notifications.PublishMessageCreated(ctx, []Delivery{{
		Message: notification.Message, UserID: notification.RecipientUserID,
	}})
}

func (s *Service) loadTaskNotificationAssistantApp(ctx context.Context, tx *gorm.DB) (store.App, error) {
	var assistant store.App
	err := tx.WithContext(ctx).First(&assistant, "id = ?", appregistry.AIAssistantAppID).Error
	if err == nil {
		return assistant, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return store.App{}, err
	}
	return appregistry.EnsureAIAssistantApp(tx.WithContext(ctx), s.apps)
}

func (s *Service) nowUTC() time.Time {
	return time.Now().UTC()
}
