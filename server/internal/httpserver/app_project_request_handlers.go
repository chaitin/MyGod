package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"app/internal/realtime"
	"app/internal/store"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

var errAppTaskVersionConflict = errors.New("task version conflict")

type appListProjectsRequest struct {
	Cursor  string    `json:"cursor"`
	Keyword string    `json:"keyword"`
	Limit   int       `json:"limit"`
	RunAs   *appRunAs `json:"runas"`
}

type appListProjectsResponse struct {
	NextCursor *string           `json:"next_cursor"`
	Projects   []projectResponse `json:"projects"`
	RunAs      appRunAsIdentity  `json:"runas"`
}

type appCreateProjectRequest struct {
	Avatar      projectOptionalString `json:"avatar"`
	Description projectOptionalString `json:"description"`
	Name        projectOptionalString `json:"name"`
	RunAs       *appRunAs             `json:"runas"`
}

type appGrantProjectGroupRequest struct {
	ConversationID string    `json:"conversation_id"`
	ProjectID      string    `json:"project_id"`
	RunAs          *appRunAs `json:"runas"`
}

type appGrantProjectGroupResponse struct {
	AlreadyGranted bool             `json:"already_granted"`
	ConversationID string           `json:"conversation_id"`
	ProjectID      string           `json:"project_id"`
	RunAs          appRunAsIdentity `json:"runas"`
}

type appListProjectTasksRequest struct {
	AssigneeUserIDs []string  `json:"assignee_user_ids"`
	Cursor          string    `json:"cursor"`
	DueDateFrom     string    `json:"due_date_from"`
	DueDateTo       string    `json:"due_date_to"`
	Keyword         string    `json:"keyword"`
	Label           string    `json:"label"`
	Limit           int       `json:"limit"`
	Priorities      []int16   `json:"priorities"`
	ProjectID       string    `json:"project_id"`
	RunAs           *appRunAs `json:"runas"`
	StartDateFrom   string    `json:"start_date_from"`
	StartDateTo     string    `json:"start_date_to"`
	Statuses        []string  `json:"statuses"`
}

type appListProjectTasksResponse struct {
	NextCursor *string          `json:"next_cursor"`
	RunAs      appRunAsIdentity `json:"runas"`
	Tasks      []taskResponse   `json:"tasks"`
}

type appCreateProjectTaskRequest struct {
	AssigneeUserID taskOptionalString      `json:"assignee_user_id"`
	Description    taskOptionalString      `json:"description"`
	DueDate        taskOptionalString      `json:"due_date"`
	Labels         taskOptionalStringSlice `json:"labels"`
	Priority       taskOptionalInt16       `json:"priority"`
	ProjectID      string                  `json:"project_id"`
	RunAs          *appRunAs               `json:"runas"`
	StartDate      taskOptionalString      `json:"start_date"`
	Status         taskOptionalString      `json:"status"`
	Title          taskOptionalString      `json:"title"`
}

type appUpdateProjectTaskRequest struct {
	AssigneeUserID  taskOptionalString      `json:"assignee_user_id"`
	Description     taskOptionalString      `json:"description"`
	DueDate         taskOptionalString      `json:"due_date"`
	ExpectedUpdated string                  `json:"expected_updated_at"`
	Labels          taskOptionalStringSlice `json:"labels"`
	Priority        taskOptionalInt16       `json:"priority"`
	ProjectID       string                  `json:"project_id"`
	RunAs           *appRunAs               `json:"runas"`
	StartDate       taskOptionalString      `json:"start_date"`
	Status          taskOptionalString      `json:"status"`
	TaskID          string                  `json:"task_id"`
	Title           taskOptionalString      `json:"title"`
}

func (s *Server) handleAppListProjects(appID string, request realtime.Envelope) (appListProjectsResponse, error) {
	var req appListProjectsRequest
	if err := json.Unmarshal(request.Payload, &req); err != nil {
		return appListProjectsResponse{}, newAppRequestFailure("invalid_request", "请求格式错误")
	}
	runAs, err := s.resolveAppRunAs(appID, req.RunAs)
	if err != nil {
		return appListProjectsResponse{}, err
	}
	keyword := strings.TrimSpace(req.Keyword)
	if err := validatePostgresText(keyword, "项目搜索关键词"); err != nil {
		return appListProjectsResponse{}, newAppRequestFailure("invalid_request", err.Error())
	}
	limit := req.Limit
	if limit == 0 {
		limit = defaultProjectPageLimit
	}
	if limit < 1 || limit > maxProjectPageLimit {
		return appListProjectsResponse{}, newAppRequestFailure("invalid_request", "limit 必须为 1 到 100 的整数")
	}
	cursor, err := decodeProjectListCursor(strings.TrimSpace(req.Cursor))
	if err != nil {
		return appListProjectsResponse{}, newAppRequestFailure("invalid_request", "项目游标格式错误")
	}

	query := s.db.WithContext(context.Background()).
		Preload("OwnerUser").
		Where(projectAccessSQL(), projectAccessArgs(runAs.ID)...)
	if keyword != "" {
		pattern := "%" + escapeTaskLikePattern(strings.ToLower(keyword)) + "%"
		query = query.Where("(LOWER(name) LIKE ? ESCAPE '\\' OR LOWER(description) LIKE ? ESCAPE '\\')", pattern, pattern)
	}
	if cursor != nil {
		query = query.Where("(updated_at < ?) OR (updated_at = ? AND id < ?)", cursor.UpdatedAt, cursor.UpdatedAt, cursor.ID)
	}
	var projects []store.Project
	if err := query.Order("updated_at DESC").Order("id DESC").Limit(limit + 1).Find(&projects).Error; err != nil {
		return appListProjectsResponse{}, err
	}
	var nextCursor *string
	if len(projects) > limit {
		projects = projects[:limit]
		encoded, err := encodeProjectListCursor(projects[len(projects)-1])
		if err != nil {
			return appListProjectsResponse{}, err
		}
		nextCursor = &encoded
	}
	roles := make(map[string]string, len(projects))
	for _, project := range projects {
		roles[project.ID] = store.ProjectRoleMember
		if project.OwnerUserID == runAs.ID {
			roles[project.ID] = store.ProjectRoleOwner
		}
	}
	responses, err := s.newProjectResponses(context.Background(), projects, roles)
	if err != nil {
		return appListProjectsResponse{}, err
	}
	return appListProjectsResponse{Projects: responses, NextCursor: nextCursor, RunAs: runAs.identity()}, nil
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
	name, err := normalizeProjectName(req.Name.Value)
	if err != nil {
		return projectResponse{}, newAppRequestFailure("invalid_request", err.Error())
	}
	if err := validatePostgresText(req.Description.Value, "项目描述"); err != nil {
		return projectResponse{}, newAppRequestFailure("invalid_request", err.Error())
	}
	if err := validatePostgresText(req.Avatar.Value, "项目头像"); err != nil {
		return projectResponse{}, newAppRequestFailure("invalid_request", err.Error())
	}
	var user store.User
	if err := s.db.First(&user, "id = ? AND status = ?", runAs.ID, store.UserStatusActive).Error; err != nil {
		return projectResponse{}, mapAppProjectRequestError(err)
	}
	now := time.Now().UTC()
	project := store.Project{
		ID:              uuid.NewString(),
		Name:            name,
		Description:     req.Description.Value,
		Avatar:          req.Avatar.Value,
		OwnerUserID:     user.ID,
		OwnerUser:       user,
		CreatedByUserID: user.ID,
		IsPersonal:      false,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := s.db.WithContext(context.Background()).Create(&project).Error; err != nil {
		return projectResponse{}, mapAppProjectRequestError(err)
	}
	response, err := s.newProjectResponse(context.Background(), project, store.ProjectRoleOwner)
	if err != nil {
		return projectResponse{}, err
	}
	return response, nil
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
	projectID, err := parseProjectID(req.ProjectID)
	if err != nil {
		return appGrantProjectGroupResponse{}, newAppRequestFailure("invalid_request", err.Error())
	}
	conversationID, err := parseProjectUUID(req.ConversationID, "群聊 ID 格式错误")
	if err != nil {
		return appGrantProjectGroupResponse{}, newAppRequestFailure("invalid_request", err.Error())
	}
	alreadyGranted := false
	err = s.db.WithContext(context.Background()).Transaction(func(tx *gorm.DB) error {
		project, role, err := findAccessibleProjectForUpdate(tx, projectID, runAs.ID)
		if err != nil {
			return err
		}
		if role != store.ProjectRoleOwner {
			return errProjectOwnerRequired
		}
		if project.IsPersonal {
			return errPersonalProjectGroup
		}
		if err := requireActiveGroupConversationMember(tx, conversationID, runAs.ID, false, true); err != nil {
			return err
		}
		var count int64
		if err := tx.Model(&store.ProjectGroup{}).Where("project_id = ? AND conversation_id = ?", projectID, conversationID).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			alreadyGranted = true
			return nil
		}
		if err := requireGroupConversationProjectCapacity(tx, conversationID); err != nil {
			return err
		}
		now := time.Now().UTC()
		if err := tx.Create(&store.ProjectGroup{ProjectID: projectID, ConversationID: conversationID, LinkedByUserID: runAs.ID, CreatedAt: now}).Error; err != nil {
			return err
		}
		return updateProjectRelationTimestamp(tx, projectID, now)
	})
	if err != nil {
		return appGrantProjectGroupResponse{}, mapAppProjectRequestError(err)
	}
	return appGrantProjectGroupResponse{
		AlreadyGranted: alreadyGranted,
		ConversationID: conversationID,
		ProjectID:      projectID,
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
	projectID, err := parseProjectID(req.ProjectID)
	if err != nil {
		return appListProjectTasksResponse{}, newAppRequestFailure("invalid_request", err.Error())
	}
	filters, err := normalizeAppTaskListFilters(req)
	if err != nil {
		return appListProjectTasksResponse{}, newAppRequestFailure("invalid_request", err.Error())
	}
	if _, _, err := s.findAccessibleProject(context.Background(), projectID, runAs.ID); err != nil {
		return appListProjectTasksResponse{}, mapAppTaskRequestError(err)
	}
	query := s.db.WithContext(context.Background()).Preload("AssigneeUser").Preload("CreatedByUser").Where("project_id = ?", projectID)
	query = applyTaskListFilters(query, s.db.Dialector.Name(), filters)
	if filters.Cursor != nil {
		query = query.Where("(updated_at < ?) OR (updated_at = ? AND id < ?)", filters.Cursor.UpdatedAt, filters.Cursor.UpdatedAt, filters.Cursor.ID)
	}
	var tasks []store.Task
	if err := query.Order("updated_at DESC").Order("id DESC").Limit(filters.Limit + 1).Find(&tasks).Error; err != nil {
		return appListProjectTasksResponse{}, err
	}
	var nextCursor *string
	if len(tasks) > filters.Limit {
		tasks = tasks[:filters.Limit]
		encoded, err := encodeTaskListCursor(tasks[len(tasks)-1])
		if err != nil {
			return appListProjectTasksResponse{}, err
		}
		nextCursor = &encoded
	}
	responses := make([]taskResponse, 0, len(tasks))
	for _, task := range tasks {
		responses = append(responses, newTaskResponse(task))
	}
	return appListProjectTasksResponse{Tasks: responses, NextCursor: nextCursor, RunAs: runAs.identity()}, nil
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
	projectID, err := parseProjectID(req.ProjectID)
	if err != nil {
		return taskResponse{}, newAppRequestFailure("invalid_request", err.Error())
	}
	task, err := normalizeCreateTask(createTaskRequest{
		Title: req.Title, Description: req.Description, Status: req.Status, Priority: req.Priority,
		AssigneeUserID: req.AssigneeUserID, StartDate: req.StartDate, DueDate: req.DueDate, Labels: req.Labels,
	}, projectID, runAs.ID)
	if err != nil {
		return taskResponse{}, newAppRequestFailure("invalid_request", err.Error())
	}
	var notification *taskNotificationResult
	err = s.db.WithContext(context.Background()).Transaction(func(tx *gorm.DB) error {
		if _, _, err := findAccessibleProjectForUpdate(tx, projectID, runAs.ID); err != nil {
			return err
		}
		if task.AssigneeUserID != nil {
			if _, err := validateTaskAssignee(tx, projectID, *task.AssigneeUserID); err != nil {
				return err
			}
		}
		now := time.Now().UTC()
		task.ID = uuid.NewString()
		task.CreatedAt = now
		task.UpdatedAt = now
		setTaskTerminalTimestamps(&task, now)
		if err := tx.Create(&task).Error; err != nil {
			return err
		}
		if err := updateProjectRelationTimestamp(tx, projectID, now); err != nil {
			return err
		}
		notification, err = s.createTaskNotificationTx(
			context.Background(),
			tx,
			task,
		)
		return err
	})
	if err != nil {
		return taskResponse{}, mapAppTaskRequestError(err)
	}
	s.publishTaskNotification(context.Background(), notification)
	if err := s.db.WithContext(context.Background()).Preload("CreatedByUser").Preload("AssigneeUser").First(&task, "id = ?", task.ID).Error; err != nil {
		return taskResponse{}, mapAppTaskRequestError(err)
	}
	return newTaskResponse(task), nil
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
	projectID, err := parseProjectID(req.ProjectID)
	if err != nil {
		return taskResponse{}, newAppRequestFailure("invalid_request", err.Error())
	}
	taskID, err := parseTaskUUID(req.TaskID, "任务 ID 格式错误")
	if err != nil {
		return taskResponse{}, newAppRequestFailure("invalid_request", err.Error())
	}
	patch, err := normalizeTaskPatch(updateTaskRequest{
		Title: req.Title, Description: req.Description, Status: req.Status, Priority: req.Priority,
		AssigneeUserID: req.AssigneeUserID, StartDate: req.StartDate, DueDate: req.DueDate, Labels: req.Labels,
	})
	if err != nil {
		return taskResponse{}, newAppRequestFailure("invalid_request", err.Error())
	}
	if !normalizedTaskPatchPresent(patch) {
		return taskResponse{}, newAppRequestFailure("invalid_request", "至少需要提供一个要修改的任务字段")
	}
	var expectedUpdatedAt *time.Time
	if strings.TrimSpace(req.ExpectedUpdated) != "" {
		parsed, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(req.ExpectedUpdated))
		if err != nil {
			return taskResponse{}, newAppRequestFailure("invalid_request", "expected_updated_at 格式错误")
		}
		expectedUpdatedAt = &parsed
	}
	var task store.Task
	var notification *taskNotificationResult
	err = s.db.WithContext(context.Background()).Transaction(func(tx *gorm.DB) error {
		if _, _, err := findAccessibleProjectForUpdate(tx, projectID, runAs.ID); err != nil {
			return err
		}
		task, err = findTaskForUpdate(tx, projectID, taskID)
		if err != nil {
			return err
		}
		if expectedUpdatedAt != nil && !task.UpdatedAt.Equal(*expectedUpdatedAt) {
			return errAppTaskVersionConflict
		}
		if err := validateTaskPatchDates(task, patch); err != nil {
			return err
		}
		if patch.AssigneePresent && patch.AssigneeUserID != nil && (task.AssigneeUserID == nil || *task.AssigneeUserID != *patch.AssigneeUserID) {
			validated, err := validateTaskAssignee(tx, projectID, *patch.AssigneeUserID)
			if err != nil {
				return err
			}
			task.AssigneeUser = &validated
		}
		now := time.Now().UTC()
		updates := taskPatchUpdates(&task, patch, now)
		if len(updates) == 0 {
			return nil
		}
		updates["updated_at"] = now
		result := tx.Model(&store.Task{}).Where("id = ? AND project_id = ? AND deleted_at IS NULL", taskID, projectID).Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		task.UpdatedAt = now
		if err := updateProjectRelationTimestamp(tx, projectID, now); err != nil {
			return err
		}
		notification, err = s.createTaskNotificationTx(
			context.Background(),
			tx,
			task,
		)
		return err
	})
	if err != nil {
		return taskResponse{}, mapAppTaskRequestError(err)
	}
	s.publishTaskNotification(context.Background(), notification)
	if err := s.db.WithContext(context.Background()).Preload("CreatedByUser").Preload("AssigneeUser").First(&task, "id = ? AND project_id = ?", taskID, projectID).Error; err != nil {
		return taskResponse{}, mapAppTaskRequestError(err)
	}
	return newTaskResponse(task), nil
}

func normalizeAppTaskListFilters(req appListProjectTasksRequest) (taskListFilters, error) {
	filters := taskListFilters{Keyword: strings.TrimSpace(req.Keyword), Limit: req.Limit}
	if filters.Limit == 0 {
		filters.Limit = defaultTaskPageLimit
	}
	if filters.Limit < 1 || filters.Limit > maxTaskPageLimit {
		return filters, errors.New("limit 必须为 1 到 100 的整数")
	}
	if err := validatePostgresText(filters.Keyword, "搜索关键词"); err != nil {
		return filters, err
	}
	for _, status := range req.Statuses {
		status = strings.TrimSpace(status)
		if !validTaskStatus(status) {
			return filters, errors.New("任务状态筛选无效")
		}
		if !containsString(filters.Statuses, status) {
			filters.Statuses = append(filters.Statuses, status)
		}
	}
	for _, priority := range req.Priorities {
		if !validTaskPriority(priority) {
			return filters, errors.New("任务优先级筛选无效")
		}
		if !containsInt16(filters.Priorities, priority) {
			filters.Priorities = append(filters.Priorities, priority)
		}
	}
	for _, rawID := range req.AssigneeUserIDs {
		id, err := parseTaskUUID(rawID, "负责人 ID 格式错误")
		if err != nil {
			return filters, err
		}
		if !containsString(filters.AssigneeUserIDs, id) {
			filters.AssigneeUserIDs = append(filters.AssigneeUserIDs, id)
		}
	}
	if label := strings.TrimSpace(req.Label); label != "" {
		if _, err := normalizeTaskLabels([]string{label}); err != nil {
			return filters, err
		}
		filters.Label = &label
	}
	var err error
	if filters.StartDateFrom, err = parseOptionalAppTaskDate(req.StartDateFrom, "start_date_from"); err != nil {
		return filters, err
	}
	if filters.StartDateTo, err = parseOptionalAppTaskDate(req.StartDateTo, "start_date_to"); err != nil {
		return filters, err
	}
	if filters.DueDateFrom, err = parseOptionalAppTaskDate(req.DueDateFrom, "due_date_from"); err != nil {
		return filters, err
	}
	if filters.DueDateTo, err = parseOptionalAppTaskDate(req.DueDateTo, "due_date_to"); err != nil {
		return filters, err
	}
	if filters.StartDateFrom != nil && filters.StartDateTo != nil && filters.StartDateFrom.After(*filters.StartDateTo) {
		return filters, errors.New("开始日期筛选范围无效")
	}
	if filters.DueDateFrom != nil && filters.DueDateTo != nil && filters.DueDateFrom.After(*filters.DueDateTo) {
		return filters, errors.New("截止日期筛选范围无效")
	}
	if strings.TrimSpace(req.Cursor) != "" {
		filters.Cursor, err = decodeTaskListCursor(strings.TrimSpace(req.Cursor))
		if err != nil {
			return filters, errors.New("任务游标格式错误")
		}
	}
	return filters, nil
}

func parseOptionalAppTaskDate(value string, field string) (*time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	parsed, err := parseTaskDate(value)
	if err != nil {
		return nil, errors.New(field + " 格式必须为 YYYY-MM-DD")
	}
	return &parsed, nil
}

func normalizedTaskPatchPresent(patch normalizedTaskPatch) bool {
	return patch.Title != nil || patch.Description != nil || patch.Status != nil || patch.Priority != nil ||
		patch.AssigneePresent || patch.StartPresent || patch.DuePresent || patch.Labels != nil
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func containsInt16(values []int16, target int16) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func mapAppProjectRequestError(err error) error {
	switch {
	case errors.Is(err, errProjectOwnerRequired):
		return newAppRequestFailure("forbidden", "只有项目所有者可以授权群聊")
	case errors.Is(err, errPersonalProjectGroup):
		return newAppRequestFailure("invalid_request", "个人项目不能授权给群聊")
	case errors.Is(err, errConversationNotGroup):
		return newAppRequestFailure("invalid_request", "只能授权给群聊")
	case errors.Is(err, errConversationAccessDenied):
		return newAppRequestFailure("forbidden", "授权用户不是目标群聊的有效成员")
	case errors.Is(err, errGroupConversationProjectCap):
		return newAppRequestFailure("conflict", "群聊关联项目数量已达上限")
	case errors.Is(err, gorm.ErrRecordNotFound):
		return newAppRequestFailure("not_found", "项目、群聊或用户不存在")
	default:
		return err
	}
}

func mapAppTaskRequestError(err error) error {
	switch {
	case errors.Is(err, errAppTaskVersionConflict):
		return newAppRequestFailure("conflict", "任务已被其他操作修改，请重新查询后再更新")
	case errors.Is(err, errInvalidTaskAssignee):
		return newAppRequestFailure("invalid_request", "负责人不存在、不可用或无项目访问权限")
	case errors.Is(err, errInvalidTaskPatch):
		return newAppRequestFailure("invalid_request", err.Error())
	case errors.Is(err, gorm.ErrRecordNotFound):
		return newAppRequestFailure("not_found", "项目或任务不存在")
	default:
		return err
	}
}
