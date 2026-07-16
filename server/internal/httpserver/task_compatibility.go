package httpserver

import (
	"bytes"
	"encoding/json"
	"time"
)

type taskResponse struct {
	ID          string                `json:"id"`
	ProjectID   string                `json:"project_id"`
	Title       string                `json:"title"`
	Description string                `json:"description"`
	Status      string                `json:"status"`
	Priority    int16                 `json:"priority"`
	Assignee    *projectUserSummary   `json:"assignee" extensions:"x-nullable"`
	Creator     projectUserSummary    `json:"creator"`
	StartDate   *string               `json:"start_date" extensions:"x-nullable"`
	DueDate     *string               `json:"due_date" extensions:"x-nullable"`
	Labels      []string              `json:"labels"`
	CompletedAt *time.Time            `json:"completed_at" extensions:"x-nullable"`
	CanceledAt  *time.Time            `json:"canceled_at" extensions:"x-nullable"`
	CreatedAt   time.Time             `json:"created_at"`
	UpdatedAt   time.Time             `json:"updated_at"`
	Reminder    *taskReminderResponse `json:"reminder" extensions:"x-nullable"`
}

type taskReminderResponse struct {
	Mode            string     `json:"mode"`
	Frequency       string     `json:"frequency,omitempty"`
	Timezone        string     `json:"timezone"`
	At              *time.Time `json:"at,omitempty"`
	Time            string     `json:"time,omitempty"`
	Weekdays        []int16    `json:"weekdays,omitempty"`
	DayOfMonth      *int16     `json:"day_of_month,omitempty"`
	NextTriggerAt   *time.Time `json:"next_trigger_at"`
	LastProcessedAt *time.Time `json:"last_processed_at"`
	State           string     `json:"state"`
}

type taskReminderRequest struct {
	Mode       string  `json:"mode"`
	Frequency  string  `json:"frequency,omitempty"`
	Timezone   string  `json:"timezone"`
	At         string  `json:"at,omitempty"`
	Time       string  `json:"time,omitempty"`
	Weekdays   []int16 `json:"weekdays,omitempty"`
	DayOfMonth int16   `json:"day_of_month,omitempty"`
}

type taskOptionalReminder struct {
	Present bool
	Null    bool
	Value   taskReminderRequest
}

type taskOptionalString struct {
	Present bool
	Null    bool
	Value   string
}

type taskOptionalInt16 struct {
	Present bool
	Null    bool
	Value   int16
}

type taskOptionalStringSlice struct {
	Present bool
	Null    bool
	Value   []string
}

func (value *taskOptionalString) UnmarshalJSON(raw []byte) error {
	value.Present = true
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		value.Null = true
		return nil
	}
	return json.Unmarshal(raw, &value.Value)
}

func (value *taskOptionalInt16) UnmarshalJSON(raw []byte) error {
	value.Present = true
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		value.Null = true
		return nil
	}
	return json.Unmarshal(raw, &value.Value)
}

func (value *taskOptionalStringSlice) UnmarshalJSON(raw []byte) error {
	value.Present = true
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		value.Null = true
		return nil
	}
	return json.Unmarshal(raw, &value.Value)
}

func (value *taskOptionalReminder) UnmarshalJSON(raw []byte) error {
	value.Present = true
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		value.Null = true
		return nil
	}
	return json.Unmarshal(raw, &value.Value)
}
