package httpserver

import (
	"context"
	"encoding/json"
	"fmt"

	messageapp "app/internal/application/message"
	messagecontentapp "app/internal/application/messagecontent"
	"app/internal/store"

	"gorm.io/gorm"
)

const taskNotificationDescriptionFallback = "暂无描述"

func (s *Server) PrepareTaskNotification(ctx context.Context, tx *gorm.DB, task store.Task) (any, error) {
	return s.messages.PrepareTaskNotification(ctx, tx, messageapp.TaskNotificationCommand{
		AssigneeUserID: task.AssigneeUserID, Description: task.Description, ID: task.ID,
		ProjectID: task.ProjectID, Title: task.Title, UpdatedAt: task.UpdatedAt,
	})
}

func (s *Server) PublishTaskNotification(ctx context.Context, prepared any) {
	if prepared == nil {
		return
	}
	notification, ok := prepared.(*messageapp.TaskNotificationResult)
	if !ok {
		return
	}
	s.messages.PublishTaskNotification(ctx, notification)
}

func buildTaskNotificationBody(ctx context.Context, task store.Task) (json.RawMessage, string, error) {
	return messagecontentapp.NewService(messagecontentapp.Dependencies{}).BuildTaskNotificationBody(
		ctx,
		messageapp.TaskNotificationCommand{
			AssigneeUserID: task.AssigneeUserID,
			Description:    task.Description,
			ID:             task.ID,
			ProjectID:      task.ProjectID,
			Title:          task.Title,
			UpdatedAt:      task.UpdatedAt,
		},
	)
}

func taskNotificationClientMessageID(task store.Task, recipientUserID string) string {
	return fmt.Sprintf("task-notification:%s:%d:%s", task.ID, task.UpdatedAt.UnixMicro(), recipientUserID)
}
