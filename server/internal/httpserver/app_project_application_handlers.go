package httpserver

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	projectapp "app/internal/application/project"
	taskapp "app/internal/application/task"
	"app/internal/realtime"
)

func (s *Server) handleAppListProjects(appID string, request realtime.Envelope) (appListProjectsResponse, error) {
	var req appListProjectsRequest
	if err := json.Unmarshal(request.Payload, &req); err != nil {
		return appListProjectsResponse{}, newAppRequestFailure("invalid_request", "请求格式错误")
	}
	runAs, err := s.resolveAppRunAs(appID, req.RunAs)
	if err != nil {
		return appListProjectsResponse{}, err
	}
	result, err := s.projects.List(context.Background(), projectapp.ListCommand{
		AccountID: runAs.ID,
		Keyword:   strings.TrimSpace(req.Keyword),
		Limit:     req.Limit,
		Cursor:    strings.TrimSpace(req.Cursor),
	})
	if err != nil {
		return appListProjectsResponse{}, mapAppProjectApplicationError(err)
	}
	projects := make([]projectResponse, 0, len(result.DetailedProjects))
	for _, value := range result.DetailedProjects {
		projects = append(projects, legacyProjectResponse(value))
	}
	return appListProjectsResponse{Projects: projects, NextCursor: result.NextCursor, RunAs: runAs.identity()}, nil
}

func (s *Server) handleAppCreateProject(appID string, request realtime.Envelope) (projectResponse, error) {
	var req appCreateProjectRequest
	if err := json.Unmarshal(request.Payload, &req); err != nil {
		return projectResponse{}, newAppRequestFailure("invalid_request", "请求格式错误")
	}
	runAs, err := s.resolveAppRunAs(appID, req.RunAs)
	if err != nil {
		return projectResponse{}, err
	}
	value, err := s.projects.Create(context.Background(), projectapp.CreateCommand{
		AccountID:   runAs.ID,
		Name:        req.Name.Value,
		Description: req.Description.Value,
		Avatar:      req.Avatar.Value,
	})
	if err != nil {
		return projectResponse{}, mapAppProjectApplicationError(err)
	}
	return legacyProjectResponse(value), nil
}

func (s *Server) handleAppGrantProjectGroup(appID string, request realtime.Envelope) (appGrantProjectGroupResponse, error) {
	var req appGrantProjectGroupRequest
	if err := json.Unmarshal(request.Payload, &req); err != nil {
		return appGrantProjectGroupResponse{}, newAppRequestFailure("invalid_request", "请求格式错误")
	}
	runAs, err := s.resolveAppRunAs(appID, req.RunAs)
	if err != nil {
		return appGrantProjectGroupResponse{}, err
	}
	result, err := s.projects.BindGroup(context.Background(), projectapp.MutateGroupCommand{
		AccountID:              runAs.ID,
		ProjectID:              req.ProjectID,
		GroupID:                req.ConversationID,
		RequireGroupMembership: true,
	})
	if err != nil {
		return appGrantProjectGroupResponse{}, mapAppProjectApplicationError(err)
	}
	return appGrantProjectGroupResponse{
		AlreadyGranted: result.AlreadyLinked,
		ConversationID: result.GroupID,
		ProjectID:      result.ProjectID,
		RunAs:          runAs.identity(),
	}, nil
}

func (s *Server) handleAppListProjectTasks(appID string, request realtime.Envelope) (appListProjectTasksResponse, error) {
	var req appListProjectTasksRequest
	if err := json.Unmarshal(request.Payload, &req); err != nil {
		return appListProjectTasksResponse{}, newAppRequestFailure("invalid_request", "请求格式错误")
	}
	runAs, err := s.resolveAppRunAs(appID, req.RunAs)
	if err != nil {
		return appListProjectTasksResponse{}, err
	}
	cmd := taskapp.ListCommand{
		AccountID: runAs.ID,
		ProjectID: req.ProjectID,
		Keyword:   strings.TrimSpace(req.Keyword),
		Limit:     req.Limit,
	}
	if len(req.Statuses) > 0 {
		cmd.Status = taskapp.Field[string]{Present: true, Value: strings.Join(req.Statuses, ",")}
	}
	if len(req.Priorities) > 0 {
		values := make([]string, 0, len(req.Priorities))
		for _, priority := range req.Priorities {
			values = append(values, strconv.FormatInt(int64(priority), 10))
		}
		cmd.Priority = taskapp.Field[string]{Present: true, Value: strings.Join(values, ",")}
	}
	if len(req.AssigneeUserIDs) > 0 {
		cmd.Assignee = taskapp.Field[string]{Present: true, Value: strings.Join(req.AssigneeUserIDs, ",")}
	}
	if strings.TrimSpace(req.Label) != "" {
		cmd.Label = taskapp.Field[string]{Present: true, Value: strings.TrimSpace(req.Label)}
	}
	cmd.StartFrom = optionalAppTaskFilter(req.StartDateFrom)
	cmd.StartTo = optionalAppTaskFilter(req.StartDateTo)
	cmd.DueFrom = optionalAppTaskFilter(req.DueDateFrom)
	cmd.DueTo = optionalAppTaskFilter(req.DueDateTo)
	cmd.Cursor = optionalAppTaskFilter(req.Cursor)
	result, err := s.tasks.List(context.Background(), cmd)
	if err != nil {
		return appListProjectTasksResponse{}, mapAppTaskApplicationError(err)
	}
	tasks := make([]taskResponse, 0, len(result.Tasks))
	for _, value := range result.Tasks {
		tasks = append(tasks, legacyTaskResponse(value))
	}
	return appListProjectTasksResponse{Tasks: tasks, NextCursor: result.NextCursor, RunAs: runAs.identity()}, nil
}

func (s *Server) handleAppCreateProjectTask(appID string, request realtime.Envelope) (taskResponse, error) {
	var req appCreateProjectTaskRequest
	if err := json.Unmarshal(request.Payload, &req); err != nil {
		return taskResponse{}, newAppRequestFailure("invalid_request", "请求格式错误")
	}
	runAs, err := s.resolveAppRunAs(appID, req.RunAs)
	if err != nil {
		return taskResponse{}, err
	}
	value, err := s.tasks.Create(context.Background(), taskapp.CreateCommand{
		AccountID:      runAs.ID,
		ProjectID:      req.ProjectID,
		Title:          appTaskStringField(req.Title),
		Description:    appTaskStringField(req.Description),
		Status:         appTaskStringField(req.Status),
		Priority:       appTaskInt16Field(req.Priority),
		AssigneeUserID: appTaskStringField(req.AssigneeUserID),
		StartDate:      appTaskStringField(req.StartDate),
		DueDate:        appTaskStringField(req.DueDate),
		Labels:         appTaskStringSliceField(req.Labels),
		Reminder:       appTaskReminderField(req.Reminder),
	})
	if err != nil {
		return taskResponse{}, mapAppTaskApplicationError(err)
	}
	return legacyTaskResponse(value), nil
}

func (s *Server) handleAppUpdateProjectTask(appID string, request realtime.Envelope) (taskResponse, error) {
	var req appUpdateProjectTaskRequest
	if err := json.Unmarshal(request.Payload, &req); err != nil {
		return taskResponse{}, newAppRequestFailure("invalid_request", "请求格式错误")
	}
	runAs, err := s.resolveAppRunAs(appID, req.RunAs)
	if err != nil {
		return taskResponse{}, err
	}
	cmd := taskapp.UpdateCommand{
		AccountID:      runAs.ID,
		ProjectID:      req.ProjectID,
		TaskID:         req.TaskID,
		Title:          appTaskStringField(req.Title),
		Description:    appTaskStringField(req.Description),
		Status:         appTaskStringField(req.Status),
		Priority:       appTaskInt16Field(req.Priority),
		AssigneeUserID: appTaskStringField(req.AssigneeUserID),
		StartDate:      appTaskStringField(req.StartDate),
		DueDate:        appTaskStringField(req.DueDate),
		Labels:         appTaskStringSliceField(req.Labels),
		Reminder:       appTaskReminderField(req.Reminder),
	}
	if !appTaskUpdatePresent(cmd) {
		return taskResponse{}, newAppRequestFailure("invalid_request", "至少需要提供一个要修改的任务字段")
	}
	if raw := strings.TrimSpace(req.ExpectedUpdated); raw != "" {
		parsed, err := time.Parse(time.RFC3339Nano, raw)
		if err != nil {
			return taskResponse{}, newAppRequestFailure("invalid_request", "expected_updated_at 格式错误")
		}
		cmd.ExpectedUpdatedAt = &parsed
	}
	value, err := s.tasks.Update(context.Background(), cmd)
	if err != nil {
		return taskResponse{}, mapAppTaskApplicationError(err)
	}
	return legacyTaskResponse(value), nil
}

func optionalAppTaskFilter(raw string) taskapp.Field[string] {
	value := strings.TrimSpace(raw)
	return taskapp.Field[string]{Present: value != "", Value: value}
}

func appTaskStringField(value taskOptionalString) taskapp.Field[string] {
	return taskapp.Field[string]{Present: value.Present, Null: value.Null, Value: value.Value}
}

func appTaskInt16Field(value taskOptionalInt16) taskapp.Field[int16] {
	return taskapp.Field[int16]{Present: value.Present, Null: value.Null, Value: value.Value}
}

func appTaskStringSliceField(value taskOptionalStringSlice) taskapp.Field[[]string] {
	return taskapp.Field[[]string]{Present: value.Present, Null: value.Null, Value: value.Value}
}

func appTaskReminderField(value taskOptionalReminder) taskapp.Field[taskapp.ReminderInput] {
	return taskapp.Field[taskapp.ReminderInput]{Present: value.Present, Null: value.Null, Value: taskapp.ReminderInput{
		Mode: value.Value.Mode, Frequency: value.Value.Frequency, Timezone: value.Value.Timezone,
		At: value.Value.At, Time: value.Value.Time, Weekdays: value.Value.Weekdays, DayOfMonth: value.Value.DayOfMonth,
	}}
}

func appTaskUpdatePresent(cmd taskapp.UpdateCommand) bool {
	return cmd.Title.Present || cmd.Description.Present || cmd.Status.Present || cmd.Priority.Present ||
		cmd.AssigneeUserID.Present || cmd.StartDate.Present || cmd.DueDate.Present || cmd.Labels.Present || cmd.Reminder.Present
}

func legacyProjectResponse(value projectapp.Project) projectResponse {
	return projectResponse{
		ID: value.ID, Name: value.Name, Description: value.Description, Avatar: value.Avatar,
		IsPersonal:      value.IsPersonal,
		Owner:           projectUserSummary{ID: value.Owner.ID, Name: value.Owner.Name, Nickname: value.Owner.Nickname, Avatar: value.Owner.Avatar},
		CurrentUserRole: value.CurrentUserRole, GroupCount: value.GroupCount, MemberCount: value.MemberCount,
		TaskCounts: projectTaskCountsResponse{Total: value.TaskCounts.Total, Todo: value.TaskCounts.Todo, InProgress: value.TaskCounts.InProgress, Done: value.TaskCounts.Done, Canceled: value.TaskCounts.Canceled},
		CreatedAt:  value.CreatedAt, UpdatedAt: value.UpdatedAt,
	}
}

func legacyTaskResponse(value taskapp.Task) taskResponse {
	response := taskResponse{
		ID: value.ID, ProjectID: value.ProjectID, Title: value.Title, Description: value.Description,
		Status: value.Status, Priority: value.Priority,
		Creator:   projectUserSummary{ID: value.Creator.ID, Name: value.Creator.Name, Nickname: value.Creator.Nickname, Avatar: value.Creator.Avatar},
		StartDate: value.StartDate, DueDate: value.DueDate, Labels: value.Labels,
		CompletedAt: value.CompletedAt, CanceledAt: value.CanceledAt, CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt,
	}
	if value.Assignee != nil {
		response.Assignee = &projectUserSummary{ID: value.Assignee.ID, Name: value.Assignee.Name, Nickname: value.Assignee.Nickname, Avatar: value.Assignee.Avatar}
	}
	if value.Reminder != nil {
		response.Reminder = &taskReminderResponse{
			Mode: value.Reminder.Mode, Frequency: value.Reminder.Frequency, Timezone: value.Reminder.Timezone,
			At: value.Reminder.At, Time: value.Reminder.Time, Weekdays: value.Reminder.Weekdays,
			DayOfMonth: value.Reminder.DayOfMonth, NextTriggerAt: value.Reminder.NextTriggerAt,
			LastProcessedAt: value.Reminder.LastProcessedAt, State: value.Reminder.State,
		}
	}
	return response
}

func mapAppProjectApplicationError(err error) error {
	switch projectapp.ErrorCodeOf(err) {
	case projectapp.CodeInvalidRequest:
		message := projectapp.ErrorMessage(err)
		if message == "个人项目不能关联群聊" {
			message = "个人项目不能授权给群聊"
		}
		return newAppRequestFailure("invalid_request", message)
	case projectapp.CodeForbidden:
		message := projectapp.ErrorMessage(err)
		if message != "授权用户不是目标群聊的有效成员" {
			message = "只有项目所有者可以授权群聊"
		}
		return newAppRequestFailure("forbidden", message)
	case projectapp.CodeNotFound:
		return newAppRequestFailure("not_found", "项目、群聊或用户不存在")
	case projectapp.CodeConflict:
		return newAppRequestFailure("conflict", projectapp.ErrorMessage(err))
	default:
		return err
	}
}

func mapAppTaskApplicationError(err error) error {
	switch taskapp.ErrorCodeOf(err) {
	case taskapp.CodeInvalidRequest:
		return newAppRequestFailure("invalid_request", taskapp.ErrorMessage(err))
	case taskapp.CodeNotFound:
		return newAppRequestFailure("not_found", "项目或任务不存在")
	case taskapp.CodeConflict:
		return newAppRequestFailure("conflict", "任务已被其他操作修改，请重新查询后再更新")
	default:
		return err
	}
}
