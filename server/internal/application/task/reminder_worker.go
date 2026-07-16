package task

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"app/internal/store"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	reminderWorkerInterval = 15 * time.Second
	reminderWorkerBatch    = 50
	maxReminderRetryDelay  = 5 * time.Minute
)

var errNoDueReminder = errors.New("no due task reminder")

func (s *Service) RunReminderWorker(ctx context.Context) {
	s.processReminderBatchAndLog(ctx)
	ticker := time.NewTicker(reminderWorkerInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.processReminderBatchAndLog(ctx)
		}
	}
}

func (s *Service) processReminderBatchAndLog(ctx context.Context) {
	processed, err := s.ProcessDueReminders(ctx, reminderWorkerBatch)
	if err != nil {
		slog.Error("process task reminders", "processed", processed, "error", err)
	}
}

func (s *Service) ProcessDueReminders(ctx context.Context, limit int) (int, error) {
	if limit <= 0 {
		limit = reminderWorkerBatch
	}
	processed := 0
	attempted := 0
	var batchError error
	for attempted < limit && ctx.Err() == nil {
		attempted++
		didProcess, taskID, scheduledAt, notification, err := s.processNextDueReminder(ctx)
		if err != nil {
			batchError = errors.Join(batchError, err)
			if taskID != "" && scheduledAt != nil {
				if retryErr := s.recordReminderFailure(ctx, taskID, *scheduledAt, err); retryErr != nil {
					batchError = errors.Join(batchError, retryErr)
				}
			}
			continue
		}
		if !didProcess {
			break
		}
		processed++
		if s.notifications != nil {
			s.notifications.PublishTaskReminderNotification(ctx, notification)
		}
	}
	return processed, batchError
}

func (s *Service) processNextDueReminder(ctx context.Context) (bool, string, *time.Time, any, error) {
	var taskValue store.Task
	var scheduledAt *time.Time
	var notification any
	now := s.now().UTC()
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		query := tx.Model(&store.Task{}).
			Select("tasks.*").
			Joins("JOIN task_reminders ON task_reminders.task_id = tasks.id").
			Where("task_reminders.next_trigger_at IS NOT NULL AND task_reminders.next_trigger_at <= ?", now).
			Where("task_reminders.retry_at IS NULL OR task_reminders.retry_at <= ?", now).
			Order("task_reminders.next_trigger_at ASC").
			Limit(1)
		if tx.Dialector.Name() == "postgres" {
			query = query.Clauses(clause.Locking{Strength: "UPDATE", Table: clause.Table{Name: "tasks"}, Options: "SKIP LOCKED"})
		}
		if err := query.First(&taskValue).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errNoDueReminder
			}
			return err
		}
		var reminder store.TaskReminder
		reminderQuery := tx.Where("task_id = ?", taskValue.ID)
		if tx.Dialector.Name() == "postgres" {
			reminderQuery = reminderQuery.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		if err := reminderQuery.First(&reminder).Error; err != nil {
			return err
		}
		if reminder.NextTriggerAt == nil || reminder.NextTriggerAt.After(now) || reminder.RetryAt != nil && reminder.RetryAt.After(now) {
			return errNoDueReminder
		}
		due := reminder.NextTriggerAt.UTC()
		scheduledAt = &due
		taskValue.Reminder = &reminder

		occurrence, err := latestReminderOccurrence(reminder, now)
		if err != nil {
			return err
		}
		if occurrence == nil || occurrence.Before(due) {
			occurrence = &due
		}
		var next *time.Time
		result := "skipped_terminal"
		terminal := taskValue.Status == StatusDone || taskValue.Status == StatusCanceled
		if !terminal {
			next, err = nextReminderOccurrence(reminder, now)
			if err != nil {
				return err
			}
			if taskValue.AssigneeUserID == nil {
				result = "skipped_no_assignee"
			} else if s.notifications == nil {
				result = "skipped_unavailable"
			} else {
				notification, err = s.notifications.PrepareTaskReminderNotification(ctx, tx, taskValue, *occurrence)
				if err != nil {
					return err
				}
				if notification == nil {
					result = "skipped_recipient"
				} else {
					result = "sent"
				}
			}
		}
		updates := map[string]any{
			"next_trigger_at": next, "last_occurrence_at": occurrence, "last_processed_at": now,
			"last_result": result, "consecutive_failures": 0, "retry_at": nil,
			"last_error": "", "updated_at": now,
		}
		return tx.Model(&store.TaskReminder{}).Where("id = ?", reminder.ID).Updates(updates).Error
	})
	if errors.Is(err, errNoDueReminder) {
		return false, "", nil, nil, nil
	}
	if err != nil {
		return false, taskValue.ID, scheduledAt, nil, err
	}
	return true, taskValue.ID, scheduledAt, notification, nil
}

func (s *Service) recordReminderFailure(ctx context.Context, taskID string, scheduledAt time.Time, processError error) error {
	var reminder store.TaskReminder
	if err := s.db.WithContext(ctx).Where("task_id = ? AND next_trigger_at = ?", taskID, scheduledAt).First(&reminder).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}
	delay := time.Second << min(reminder.ConsecutiveFailures, 8)
	if delay > maxReminderRetryDelay {
		delay = maxReminderRetryDelay
	}
	now := s.now().UTC()
	retryAt := now.Add(delay)
	message := processError.Error()
	if len(message) > 1000 {
		message = message[:1000]
	}
	return s.db.WithContext(ctx).Model(&store.TaskReminder{}).
		Where("task_id = ? AND next_trigger_at = ?", taskID, scheduledAt).
		Updates(map[string]any{
			"consecutive_failures": gorm.Expr("consecutive_failures + 1"),
			"retry_at":             retryAt, "last_error": message, "updated_at": now,
		}).Error
}
