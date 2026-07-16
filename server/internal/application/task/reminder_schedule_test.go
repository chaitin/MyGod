package task

import (
	"testing"
	"time"

	"app/internal/store"
)

func TestReminderScheduleCalculatesCalendarOccurrences(t *testing.T) {
	t.Run("daily in configured timezone", func(t *testing.T) {
		now := time.Date(2026, 7, 15, 1, 0, 0, 0, time.UTC)
		reminder, err := normalizeReminder(ReminderInput{
			Mode: ReminderModeRecurring, Frequency: ReminderDaily,
			Timezone: "Asia/Singapore", Time: "09:30",
		}, now, StatusTodo)
		if err != nil {
			t.Fatalf("normalize reminder: %v", err)
		}
		want := time.Date(2026, 7, 15, 1, 30, 0, 0, time.UTC)
		if reminder.NextTriggerAt == nil || !reminder.NextTriggerAt.Equal(want) {
			t.Fatalf("next trigger = %v, want %v", reminder.NextTriggerAt, want)
		}
		if reminder.Timezone != ReminderTimezone {
			t.Fatalf("timezone = %q, want %q", reminder.Timezone, ReminderTimezone)
		}
	})

	t.Run("weekly supports multiple weekdays", func(t *testing.T) {
		now := time.Date(2026, 7, 15, 10, 0, 0, 0, time.UTC) // Wednesday
		reminder, err := normalizeReminder(ReminderInput{
			Mode: ReminderModeRecurring, Frequency: ReminderWeekly,
			Timezone: "UTC", Time: "09:00", Weekdays: []int16{5, 1, 5},
		}, now, StatusTodo)
		if err != nil {
			t.Fatalf("normalize reminder: %v", err)
		}
		want := time.Date(2026, 7, 17, 1, 0, 0, 0, time.UTC)
		if reminder.NextTriggerAt == nil || !reminder.NextTriggerAt.Equal(want) {
			t.Fatalf("next trigger = %v, want %v", reminder.NextTriggerAt, want)
		}
		if len(reminder.Weekdays) != 2 || reminder.Weekdays[0] != 1 || reminder.Weekdays[1] != 5 {
			t.Fatalf("weekdays = %v", reminder.Weekdays)
		}
	})

	t.Run("monthly skips short months", func(t *testing.T) {
		now := time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC)
		reminder, err := normalizeReminder(ReminderInput{
			Mode: ReminderModeRecurring, Frequency: ReminderMonthly,
			Timezone: "UTC", Time: "09:00", DayOfMonth: 31,
		}, now, StatusTodo)
		if err != nil {
			t.Fatalf("normalize reminder: %v", err)
		}
		want := time.Date(2026, 5, 31, 1, 0, 0, 0, time.UTC)
		if reminder.NextTriggerAt == nil || !reminder.NextTriggerAt.Equal(want) {
			t.Fatalf("next trigger = %v, want %v", reminder.NextTriggerAt, want)
		}
	})
}

func TestReminderScheduleRejectsInvalidRules(t *testing.T) {
	now := time.Date(2026, 7, 15, 1, 0, 0, 0, time.UTC)
	tests := []ReminderInput{
		{Mode: ReminderModeOnce, Timezone: "UTC", At: now.Format(time.RFC3339)},
		{Mode: ReminderModeRecurring, Frequency: ReminderWeekly, Timezone: "UTC", Time: "09:00"},
		{Mode: ReminderModeRecurring, Frequency: ReminderMonthly, Timezone: "UTC", Time: "09:00", DayOfMonth: 32},
	}
	for _, input := range tests {
		if _, err := normalizeReminder(input, now, StatusTodo); ErrorCodeOf(err) != CodeInvalidRequest {
			t.Fatalf("input = %#v, error = %v", input, err)
		}
	}
}

func TestLatestReminderOccurrenceUsesMostRecentMissedSchedule(t *testing.T) {
	timeOfDay := "09:00"
	frequency := ReminderDaily
	reminder := store.TaskReminder{
		Mode: ReminderModeRecurring, Frequency: &frequency, Timezone: "UTC", TimeOfDay: &timeOfDay,
	}
	got, err := latestReminderOccurrence(reminder, time.Date(2026, 7, 18, 14, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("latest occurrence: %v", err)
	}
	want := time.Date(2026, 7, 18, 1, 0, 0, 0, time.UTC)
	if got == nil || !got.Equal(want) {
		t.Fatalf("latest occurrence = %v, want %v", got, want)
	}
}
