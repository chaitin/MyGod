package task

import (
	"errors"
	"slices"
	"sort"
	"strings"
	"time"

	"app/internal/store"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"gorm.io/gorm"
)

const reminderTimeLayout = "15:04"

func normalizeReminder(input ReminderInput, now time.Time, status string) (*store.TaskReminder, error) {
	mode := strings.TrimSpace(input.Mode)
	value := &store.TaskReminder{
		ID: uuid.NewString(), Mode: mode, Timezone: ReminderTimezone, Weekdays: pq.Int64Array{},
		CreatedAt: now.UTC(), UpdatedAt: now.UTC(),
	}
	switch mode {
	case ReminderModeOnce:
		if strings.TrimSpace(input.Frequency) != "" || input.Time != "" || len(input.Weekdays) > 0 || input.DayOfMonth != 0 {
			return nil, invalid("一次性提醒配置无效", nil)
		}
		at, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(input.At))
		if err != nil || at.Second() != 0 || at.Nanosecond() != 0 {
			return nil, invalid("一次性提醒时间必须是精确到分钟的 RFC3339 时间", err)
		}
		at = at.UTC()
		if !at.After(now.UTC()) {
			return nil, invalid("一次性提醒时间必须晚于当前时间", nil)
		}
		value.OnceAt = &at
	case ReminderModeRecurring:
		if strings.TrimSpace(input.At) != "" {
			return nil, invalid("重复提醒配置无效", nil)
		}
		location, err := time.LoadLocation(ReminderTimezone)
		if err != nil {
			return nil, invalid("提醒时区无效", err)
		}
		frequency := strings.TrimSpace(input.Frequency)
		if frequency != ReminderDaily && frequency != ReminderWeekly && frequency != ReminderMonthly {
			return nil, invalid("重复提醒频率无效", nil)
		}
		parsedTime, err := time.ParseInLocation(reminderTimeLayout, strings.TrimSpace(input.Time), location)
		if err != nil || parsedTime.Format(reminderTimeLayout) != strings.TrimSpace(input.Time) {
			return nil, invalid("重复提醒时间格式必须为 HH:mm", err)
		}
		timeOfDay := parsedTime.Format(reminderTimeLayout)
		value.Frequency = &frequency
		value.TimeOfDay = &timeOfDay
		switch frequency {
		case ReminderDaily:
			if len(input.Weekdays) > 0 || input.DayOfMonth != 0 {
				return nil, invalid("每天提醒配置无效", nil)
			}
		case ReminderWeekly:
			weekdays, err := normalizeReminderWeekdays(input.Weekdays)
			if err != nil || input.DayOfMonth != 0 {
				return nil, invalid("每周提醒日期无效", err)
			}
			value.Weekdays = weekdays
		case ReminderMonthly:
			if len(input.Weekdays) > 0 || input.DayOfMonth < 1 || input.DayOfMonth > 31 {
				return nil, invalid("每月提醒日期必须为 1 到 31", nil)
			}
			day := input.DayOfMonth
			value.DayOfMonth = &day
		}
	default:
		return nil, invalid("提醒模式无效", nil)
	}
	if status != StatusDone && status != StatusCanceled {
		next, err := nextReminderOccurrence(*value, now.UTC())
		if err != nil {
			return nil, invalid("无法计算下次提醒时间", err)
		}
		value.NextTriggerAt = next
	}
	return value, nil
}

func normalizeReminderWeekdays(values []int16) (pq.Int64Array, error) {
	if len(values) == 0 || len(values) > 7 {
		return nil, errors.New("weekdays required")
	}
	seen := map[int16]struct{}{}
	result := make([]int, 0, len(values))
	for _, value := range values {
		if value < 1 || value > 7 {
			return nil, errors.New("weekday out of range")
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, int(value))
	}
	sort.Ints(result)
	weekdays := make(pq.Int64Array, len(result))
	for index, value := range result {
		weekdays[index] = int64(value)
	}
	return weekdays, nil
}

func nextReminderOccurrence(reminder store.TaskReminder, after time.Time) (*time.Time, error) {
	if reminder.Mode == ReminderModeOnce {
		if reminder.OnceAt != nil && reminder.OnceAt.After(after) {
			value := reminder.OnceAt.UTC()
			return &value, nil
		}
		return nil, nil
	}
	location, hour, minute, err := reminderLocationAndTime(reminder)
	if err != nil {
		return nil, err
	}
	localAfter := after.In(location)
	for offset := 0; offset < 366*6; offset++ {
		date := localAfter.AddDate(0, 0, offset)
		if !reminderMatchesDate(reminder, date) {
			continue
		}
		candidate, valid := exactLocalReminderTime(date, hour, minute, location)
		if valid && candidate.After(after) {
			value := candidate.UTC()
			return &value, nil
		}
	}
	return nil, errors.New("no future reminder occurrence")
}

func latestReminderOccurrence(reminder store.TaskReminder, at time.Time) (*time.Time, error) {
	if reminder.Mode == ReminderModeOnce {
		if reminder.OnceAt != nil && !reminder.OnceAt.After(at) {
			value := reminder.OnceAt.UTC()
			return &value, nil
		}
		return nil, nil
	}
	location, hour, minute, err := reminderLocationAndTime(reminder)
	if err != nil {
		return nil, err
	}
	localAt := at.In(location)
	for offset := 0; offset < 366*6; offset++ {
		date := localAt.AddDate(0, 0, -offset)
		if !reminderMatchesDate(reminder, date) {
			continue
		}
		candidate, valid := exactLocalReminderTime(date, hour, minute, location)
		if valid && !candidate.After(at) {
			value := candidate.UTC()
			return &value, nil
		}
	}
	return nil, errors.New("no previous reminder occurrence")
}

func reminderLocationAndTime(reminder store.TaskReminder) (*time.Location, int, int, error) {
	location, err := time.LoadLocation(ReminderTimezone)
	if err != nil || reminder.TimeOfDay == nil {
		return nil, 0, 0, errors.New("invalid reminder schedule")
	}
	parsed, err := time.ParseInLocation(reminderTimeLayout, *reminder.TimeOfDay, location)
	if err != nil {
		return nil, 0, 0, err
	}
	return location, parsed.Hour(), parsed.Minute(), nil
}

func reminderMatchesDate(reminder store.TaskReminder, date time.Time) bool {
	if reminder.Frequency == nil {
		return false
	}
	switch *reminder.Frequency {
	case ReminderDaily:
		return true
	case ReminderWeekly:
		weekday := int64(date.Weekday())
		if weekday == 0 {
			weekday = 7
		}
		return slices.Contains(reminder.Weekdays, weekday)
	case ReminderMonthly:
		return reminder.DayOfMonth != nil && date.Day() == int(*reminder.DayOfMonth)
	default:
		return false
	}
}

func exactLocalReminderTime(date time.Time, hour, minute int, location *time.Location) (time.Time, bool) {
	candidate := time.Date(date.Year(), date.Month(), date.Day(), hour, minute, 0, 0, location)
	local := candidate.In(location)
	return candidate, local.Year() == date.Year() && local.Month() == date.Month() && local.Day() == date.Day() && local.Hour() == hour && local.Minute() == minute
}

func remindersEqual(left, right *store.TaskReminder) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return left.Mode == right.Mode && equalOptionalString(left.Frequency, right.Frequency) &&
		left.Timezone == right.Timezone && equalOptionalTime(left.OnceAt, right.OnceAt) &&
		equalOptionalString(left.TimeOfDay, right.TimeOfDay) && slices.Equal(left.Weekdays, right.Weekdays) &&
		equalOptionalInt16(left.DayOfMonth, right.DayOfMonth)
}

func applyReminderMutation(tx *gorm.DB, taskValue store.Task, previousStatus string, field Field[ReminderInput], now time.Time) (bool, *store.TaskReminder, error) {
	current := taskValue.Reminder
	if field.Present {
		if field.Null {
			if current == nil {
				return false, nil, nil
			}
			if err := tx.Where("task_id = ?", taskValue.ID).Delete(&store.TaskReminder{}).Error; err != nil {
				return false, nil, err
			}
			return true, nil, nil
		}
		next, err := normalizeReminder(field.Value, now, taskValue.Status)
		if err != nil {
			return false, current, err
		}
		next.TaskID = taskValue.ID
		if remindersEqual(current, next) {
			return applyReminderStatusTransition(tx, current, previousStatus, taskValue.Status, now)
		}
		if current == nil {
			if err := tx.Create(next).Error; err != nil {
				return false, nil, err
			}
			return true, next, nil
		}
		next.ID = current.ID
		next.CreatedAt = current.CreatedAt
		updates := map[string]any{
			"mode": next.Mode, "frequency": next.Frequency, "timezone": next.Timezone,
			"once_at": next.OnceAt, "time_of_day": next.TimeOfDay, "weekdays": next.Weekdays,
			"day_of_month": next.DayOfMonth, "next_trigger_at": next.NextTriggerAt,
			"last_occurrence_at": nil, "last_processed_at": nil, "last_result": "",
			"consecutive_failures": 0, "retry_at": nil, "last_error": "", "updated_at": now,
		}
		if err := tx.Model(&store.TaskReminder{}).Where("id = ? AND task_id = ?", current.ID, taskValue.ID).Updates(updates).Error; err != nil {
			return false, current, err
		}
		return true, next, nil
	}
	return applyReminderStatusTransition(tx, current, previousStatus, taskValue.Status, now)
}

func applyReminderStatusTransition(tx *gorm.DB, reminder *store.TaskReminder, previousStatus, status string, now time.Time) (bool, *store.TaskReminder, error) {
	if reminder == nil || previousStatus == status {
		return false, reminder, nil
	}
	wasTerminal := previousStatus == StatusDone || previousStatus == StatusCanceled
	isTerminal := status == StatusDone || status == StatusCanceled
	if wasTerminal == isTerminal {
		return false, reminder, nil
	}
	var next *time.Time
	if !isTerminal {
		calculated, err := nextReminderOccurrence(*reminder, now)
		if err != nil {
			return false, reminder, err
		}
		next = calculated
	}
	if equalOptionalTime(reminder.NextTriggerAt, next) {
		return false, reminder, nil
	}
	if err := tx.Model(&store.TaskReminder{}).Where("id = ?", reminder.ID).Updates(map[string]any{
		"next_trigger_at": next, "retry_at": nil, "consecutive_failures": 0, "last_error": "", "updated_at": now,
	}).Error; err != nil {
		return false, reminder, err
	}
	reminder.NextTriggerAt = next
	reminder.RetryAt = nil
	reminder.ConsecutiveFailures = 0
	reminder.LastError = ""
	reminder.UpdatedAt = now
	return true, reminder, nil
}

func newReminder(value store.TaskReminder, taskStatus string) *Reminder {
	result := &Reminder{
		Mode: value.Mode, Timezone: value.Timezone, NextTriggerAt: value.NextTriggerAt,
		LastProcessedAt: value.LastProcessedAt,
	}
	if value.Frequency != nil {
		result.Frequency = *value.Frequency
	}
	if value.OnceAt != nil {
		at := value.OnceAt.UTC()
		result.At = &at
	}
	if value.TimeOfDay != nil {
		result.Time = *value.TimeOfDay
	}
	result.Weekdays = make([]int16, len(value.Weekdays))
	for index, weekday := range value.Weekdays {
		result.Weekdays[index] = int16(weekday)
	}
	if value.DayOfMonth != nil {
		day := *value.DayOfMonth
		result.DayOfMonth = &day
	}
	switch {
	case taskStatus == StatusDone || taskStatus == StatusCanceled:
		result.State = "paused"
	case value.NextTriggerAt != nil:
		result.State = "scheduled"
	case value.Mode == ReminderModeOnce && value.LastResult == "sent":
		result.State = "fired"
	default:
		result.State = "expired"
	}
	return result
}

func equalOptionalString(left, right *string) bool {
	return left == nil && right == nil || left != nil && right != nil && *left == *right
}

func equalOptionalInt16(left, right *int16) bool {
	return left == nil && right == nil || left != nil && right != nil && *left == *right
}

func equalOptionalTime(left, right *time.Time) bool {
	return left == nil && right == nil || left != nil && right != nil && left.Equal(*right)
}
