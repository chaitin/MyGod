package httpserver

import (
	"bytes"
	"encoding/json"
	"errors"
	"time"

	projectapp "app/internal/application/project"
)

const personalProjectName = projectapp.PersonalWorkspaceName

type projectUserSummary struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Nickname string `json:"nickname"`
	Avatar   string `json:"avatar"`
}

type projectTaskCountsResponse struct {
	Total      int64 `json:"total"`
	Todo       int64 `json:"todo"`
	InProgress int64 `json:"in_progress"`
	Done       int64 `json:"done"`
	Canceled   int64 `json:"canceled"`
}

type projectResponse struct {
	ID              string                    `json:"id"`
	Name            string                    `json:"name"`
	Description     string                    `json:"description"`
	Avatar          string                    `json:"avatar"`
	IsPersonal      bool                      `json:"is_personal"`
	Owner           projectUserSummary        `json:"owner"`
	CurrentUserRole string                    `json:"current_user_role"`
	GroupCount      int64                     `json:"group_count"`
	MemberCount     int64                     `json:"member_count"`
	TaskCounts      projectTaskCountsResponse `json:"task_counts"`
	CreatedAt       time.Time                 `json:"created_at"`
	UpdatedAt       time.Time                 `json:"updated_at"`
}

type projectGroupMutationResponse struct{}

type projectOptionalString struct {
	Present bool
	Value   string
}

type projectOptionalStringSlice struct {
	Present bool
	Value   []string
}

func (value *projectOptionalString) UnmarshalJSON(raw []byte) error {
	value.Present = true
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return errors.New("字符串字段不能为 null")
	}
	return json.Unmarshal(raw, &value.Value)
}

func (value *projectOptionalStringSlice) UnmarshalJSON(raw []byte) error {
	value.Present = true
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return errors.New("字符串数组字段不能为 null")
	}
	return json.Unmarshal(raw, &value.Value)
}
