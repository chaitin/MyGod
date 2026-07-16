package httpserver

import (
	"context"
	"encoding/json"
	"time"

	messageapp "app/internal/application/message"
	messagecontentapp "app/internal/application/messagecontent"
	taskapp "app/internal/application/task"
	"app/internal/store"

	"gorm.io/gorm"
)

func (s *Server) PrepareTaskReminderNotification(ctx context.Context, tx *gorm.DB, task store.Task, occurrenceAt time.Time) (any, error) {
	return s.messages.PrepareTaskReminderNotification(ctx, tx, messageapp.TaskReminderNotificationCommand{
		AssigneeUserID: task.AssigneeUserID, Description: task.Description, ID: task.ID,
		ProjectID: task.ProjectID, Title: task.Title, OccurrenceAt: occurrenceAt, Timezone: taskapp.ReminderTimezone,
	})
}

func (s *Server) PublishTaskReminderNotification(ctx context.Context, prepared any) {
	if prepared == nil {
		return
	}
	notification, ok := prepared.(*messageapp.TaskNotificationResult)
	if ok {
		s.messages.PublishTaskReminderNotification(ctx, notification)
	}
}

func buildTaskReminderBody(ctx context.Context, cmd messageapp.TaskReminderNotificationCommand) (json.RawMessage, string, error) {
	return messagecontentapp.NewService(messagecontentapp.Dependencies{}).BuildTaskReminderBody(ctx, cmd)
}
