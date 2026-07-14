package httpserver

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"app/internal/store"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/lib/pq"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const taskDateLayout = "2006-01-02"

const (
	defaultTaskPageLimit = 50
	maxTaskPageLimit     = 100
)

var (
	errInvalidTaskAssignee = errors.New("invalid task assignee")
	errInvalidTaskPatch    = errors.New("任务字段组合无效")
)

type taskResponse struct {
	ID          string              `json:"id"`
	ProjectID   string              `json:"project_id"`
	Title       string              `json:"title"`
	Description string              `json:"description"`
	Status      string              `json:"status"`
	Priority    int16               `json:"priority"`
	Assignee    *projectUserSummary `json:"assignee" extensions:"x-nullable"`
	Creator     projectUserSummary  `json:"creator"`
	StartDate   *string             `json:"start_date" extensions:"x-nullable"`
	DueDate     *string             `json:"due_date" extensions:"x-nullable"`
	Labels      []string            `json:"labels"`
	CompletedAt *time.Time          `json:"completed_at" extensions:"x-nullable"`
	CanceledAt  *time.Time          `json:"canceled_at" extensions:"x-nullable"`
	CreatedAt   time.Time           `json:"created_at"`
	UpdatedAt   time.Time           `json:"updated_at"`
}

type taskListResponse struct {
	Tasks      []taskResponse `json:"tasks"`
	NextCursor *string        `json:"next_cursor" extensions:"x-nullable"`
}

// deleteTaskResponse documents the data object returned after deleting a task.
type deleteTaskResponse struct {
	TaskID string `json:"task_id" example:"7f8d8b84-6d2c-4b12-9a8a-019a7e2787d4"`
}

type taskListCursor struct {
	UpdatedAt string `json:"updated_at"`
	ID        string `json:"id"`
}

type taskListFilters struct {
	Keyword         string
	Statuses        []string
	Priorities      []int16
	AssigneeUserIDs []string
	Label           *string
	StartDateFrom   *time.Time
	StartDateTo     *time.Time
	DueDateFrom     *time.Time
	DueDateTo       *time.Time
	Limit           int
	Cursor          *struct {
		UpdatedAt time.Time
		ID        string
	}
}

type createTaskRequest struct {
	Title          taskOptionalString      `json:"title" swaggertype:"string" binding:"required" example:"完成发布检查"`
	Description    taskOptionalString      `json:"description" swaggertype:"string" example:"核对发布清单并记录结果"`
	Status         taskOptionalString      `json:"status" swaggertype:"string" enums:"todo,in_progress,done,canceled" example:"todo"`
	Priority       taskOptionalInt16       `json:"priority" swaggertype:"integer" format:"int32" enums:"1,2,3" example:"2"`
	AssigneeUserID taskOptionalString      `json:"assignee_user_id" swaggertype:"string" extensions:"x-nullable" example:"7f8d8b84-6d2c-4b12-9a8a-019a7e2787d4"`
	StartDate      taskOptionalString      `json:"start_date" swaggertype:"string" format:"date" extensions:"x-nullable" example:"2026-07-11"`
	DueDate        taskOptionalString      `json:"due_date" swaggertype:"string" format:"date" extensions:"x-nullable" example:"2026-07-18"`
	Labels         taskOptionalStringSlice `json:"labels" swaggertype:"array,string" example:"发布"`
}

type updateTaskRequest struct {
	Title          taskOptionalString      `json:"title" swaggertype:"string" example:"完成发布检查"`
	Description    taskOptionalString      `json:"description" swaggertype:"string" example:"核对发布清单并记录结果"`
	Status         taskOptionalString      `json:"status" swaggertype:"string" enums:"todo,in_progress,done,canceled" example:"in_progress"`
	Priority       taskOptionalInt16       `json:"priority" swaggertype:"integer" format:"int32" enums:"1,2,3" example:"2"`
	AssigneeUserID taskOptionalString      `json:"assignee_user_id" swaggertype:"string" extensions:"x-nullable" example:"7f8d8b84-6d2c-4b12-9a8a-019a7e2787d4"`
	StartDate      taskOptionalString      `json:"start_date" swaggertype:"string" format:"date" extensions:"x-nullable" example:"2026-07-11"`
	DueDate        taskOptionalString      `json:"due_date" swaggertype:"string" format:"date" extensions:"x-nullable" example:"2026-07-18"`
	Labels         taskOptionalStringSlice `json:"labels" swaggertype:"array,string" example:"发布"`
}

type normalizedTaskPatch struct {
	Title           *string
	Description     *string
	Status          *string
	Priority        *int16
	AssigneePresent bool
	AssigneeUserID  *string
	StartPresent    bool
	StartDate       *time.Time
	DuePresent      bool
	DueDate         *time.Time
	Labels          *pq.StringArray
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

// listTasks godoc
//
// @Summary 列出项目任务
// @Description 获取项目任务，支持关键字、状态、优先级、负责人、标签和日期范围筛选。
// @Tags 客户端任务
// @Produce json
// @Param project_id path string true "项目 ID"
// @Param keyword query string false "标题或描述关键字"
// @Param status query string false "状态，多个值用逗号分隔：todo、in_progress、done、canceled"
// @Param priority query string false "优先级，多个值用逗号分隔：1、2、3"
// @Param assignee_user_id query string false "负责人用户 ID，多个值用逗号分隔"
// @Param label query string false "标签"
// @Param start_date_from query string false "开始日期下限，格式 YYYY-MM-DD"
// @Param start_date_to query string false "开始日期上限，格式 YYYY-MM-DD"
// @Param due_date_from query string false "截止日期下限，格式 YYYY-MM-DD"
// @Param due_date_to query string false "截止日期上限，格式 YYYY-MM-DD"
// @Param limit query int false "每页数量，默认 50，最大 100"
// @Param cursor query string false "任务分页游标"
// @Success 200 {object} successEnvelope{data=taskListResponse}
// @Failure 400 {object} errorEnvelope
// @Failure 401 {object} errorEnvelope
// @Failure 404 {object} errorEnvelope
// @Failure 500 {object} errorEnvelope
// @Security UserSession
// @Router /api/client/projects/{project_id}/tasks [get]
func (s *Server) listTasks(c echo.Context) error {
	user, ok := currentUser(c)
	if !ok {
		return taskInternalError(c)
	}
	projectID, err := parseProjectID(c.Param("project_id"))
	if err != nil {
		return taskInvalidRequest(c, err.Error())
	}
	filters, err := parseTaskListFilters(c)
	if err != nil {
		return taskInvalidRequest(c, err.Error())
	}
	if _, _, err := s.findAccessibleProject(c.Request().Context(), projectID, user.ID); errors.Is(err, gorm.ErrRecordNotFound) {
		return taskNotFound(c)
	} else if err != nil {
		return taskInternalError(c)
	}

	query := s.db.WithContext(c.Request().Context()).
		Preload("AssigneeUser").
		Preload("CreatedByUser").
		Where("project_id = ?", projectID)
	query = applyTaskListFilters(query, s.db.Dialector.Name(), filters)
	if filters.Cursor != nil {
		query = query.Where(
			"(updated_at < ?) OR (updated_at = ? AND id < ?)",
			filters.Cursor.UpdatedAt,
			filters.Cursor.UpdatedAt,
			filters.Cursor.ID,
		)
	}
	var tasks []store.Task
	if err := query.Order("updated_at DESC").Order("id DESC").Limit(filters.Limit + 1).Find(&tasks).Error; err != nil {
		return taskInternalError(c)
	}

	var nextCursor *string
	if len(tasks) > filters.Limit {
		tasks = tasks[:filters.Limit]
		encoded, err := encodeTaskListCursor(tasks[len(tasks)-1])
		if err != nil {
			return taskInternalError(c)
		}
		nextCursor = &encoded
	}
	responses := make([]taskResponse, 0, len(tasks))
	for _, task := range tasks {
		responses = append(responses, newTaskResponse(task))
	}
	return success(c, http.StatusOK, taskListResponse{Tasks: responses, NextCursor: nextCursor})
}

// createTask godoc
//
// @Summary 创建项目任务
// @Description 在当前用户可访问的项目中创建任务，可指定负责人、状态、优先级、日期和标签。
// @Tags 客户端任务
// @Accept json
// @Produce json
// @Param project_id path string true "项目 ID"
// @Param body body createTaskRequest true "任务信息"
// @Success 201 {object} successEnvelope{data=taskResponse}
// @Failure 400 {object} errorEnvelope
// @Failure 401 {object} errorEnvelope
// @Failure 404 {object} errorEnvelope
// @Failure 500 {object} errorEnvelope
// @Security UserSession
// @Router /api/client/projects/{project_id}/tasks [post]
func (s *Server) createTask(c echo.Context) error {
	user, ok := currentUser(c)
	if !ok {
		return taskInternalError(c)
	}
	projectID, err := parseProjectID(c.Param("project_id"))
	if err != nil {
		return taskInvalidRequest(c, err.Error())
	}

	var req createTaskRequest
	if err := decodeProjectRequest(c, &req); err != nil {
		return taskInvalidRequest(c, "请求格式错误")
	}
	task, err := normalizeCreateTask(req, projectID, user.ID)
	if err != nil {
		return taskInvalidRequest(c, err.Error())
	}

	var assignee *store.User
	var notification *taskNotificationResult
	err = s.db.WithContext(c.Request().Context()).Transaction(func(tx *gorm.DB) error {
		if _, _, err := findAccessibleProjectForUpdate(tx, projectID, user.ID); err != nil {
			return err
		}
		if task.AssigneeUserID != nil {
			validated, err := validateTaskAssignee(tx, projectID, *task.AssigneeUserID)
			if err != nil {
				return err
			}
			assignee = &validated
		}

		now := time.Now().UTC()
		task.ID = uuid.NewString()
		task.CreatedAt = now
		task.UpdatedAt = now
		setTaskTerminalTimestamps(&task, now)
		if err := tx.Create(&task).Error; err != nil {
			return err
		}
		result := tx.Model(&store.Project{}).
			Where("id = ? AND deleted_at IS NULL", projectID).
			Update("updated_at", now)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		notification, err = s.createTaskNotificationTx(
			c.Request().Context(),
			tx,
			task,
		)
		return err
	})
	if errors.Is(err, errInvalidTaskAssignee) {
		return taskInvalidRequest(c, "负责人不存在、不可用或无项目访问权限")
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return taskNotFound(c)
	}
	if err != nil {
		return taskInternalError(c)
	}

	task.CreatedByUser = user
	task.AssigneeUser = assignee
	s.publishTaskNotification(c.Request().Context(), notification)
	return success(c, http.StatusCreated, newTaskResponse(task))
}

// getTask godoc
//
// @Summary 获取项目任务
// @Description 获取当前用户可访问的项目任务详情。
// @Tags 客户端任务
// @Produce json
// @Param project_id path string true "项目 ID"
// @Param task_id path string true "任务 ID"
// @Success 200 {object} successEnvelope{data=taskResponse}
// @Failure 400 {object} errorEnvelope
// @Failure 401 {object} errorEnvelope
// @Failure 404 {object} errorEnvelope
// @Failure 500 {object} errorEnvelope
// @Security UserSession
// @Router /api/client/projects/{project_id}/tasks/{task_id} [get]
func (s *Server) getTask(c echo.Context) error {
	user, ok := currentUser(c)
	if !ok {
		return taskInternalError(c)
	}
	projectID, taskID, err := parseTaskPathIDs(c)
	if err != nil {
		return taskInvalidRequest(c, err.Error())
	}
	task, err := s.findAccessibleTask(c, projectID, taskID, user.ID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return taskNotFound(c)
	}
	if err != nil {
		return taskInternalError(c)
	}
	return success(c, http.StatusOK, newTaskResponse(task))
}

// updateTask godoc
//
// @Summary 更新项目任务
// @Description 更新当前用户可访问的项目任务字段。
// @Tags 客户端任务
// @Accept json
// @Produce json
// @Param project_id path string true "项目 ID"
// @Param task_id path string true "任务 ID"
// @Param body body updateTaskRequest true "任务更新信息"
// @Success 200 {object} successEnvelope{data=taskResponse}
// @Failure 400 {object} errorEnvelope
// @Failure 401 {object} errorEnvelope
// @Failure 404 {object} errorEnvelope
// @Failure 500 {object} errorEnvelope
// @Security UserSession
// @Router /api/client/projects/{project_id}/tasks/{task_id} [patch]
func (s *Server) updateTask(c echo.Context) error {
	user, ok := currentUser(c)
	if !ok {
		return taskInternalError(c)
	}
	projectID, taskID, err := parseTaskPathIDs(c)
	if err != nil {
		return taskInvalidRequest(c, err.Error())
	}
	var req updateTaskRequest
	if err := decodeProjectRequest(c, &req); err != nil {
		return taskInvalidRequest(c, "请求格式错误")
	}
	patch, err := normalizeTaskPatch(req)
	if err != nil {
		return taskInvalidRequest(c, err.Error())
	}

	var task store.Task
	var notification *taskNotificationResult
	err = s.db.WithContext(c.Request().Context()).Transaction(func(tx *gorm.DB) error {
		if _, _, err := findAccessibleProjectForUpdate(tx, projectID, user.ID); err != nil {
			return err
		}
		task, err = findTaskForUpdate(tx, projectID, taskID)
		if err != nil {
			return err
		}
		if err := validateTaskPatchDates(task, patch); err != nil {
			return err
		}
		if patch.AssigneePresent && patch.AssigneeUserID != nil &&
			(task.AssigneeUserID == nil || *task.AssigneeUserID != *patch.AssigneeUserID) {
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
		result := tx.Model(&store.Task{}).
			Where("id = ? AND project_id = ? AND deleted_at IS NULL", taskID, projectID).
			Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		projectResult := tx.Model(&store.Project{}).
			Where("id = ? AND deleted_at IS NULL", projectID).
			Update("updated_at", now)
		if projectResult.Error != nil {
			return projectResult.Error
		}
		if projectResult.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		task.UpdatedAt = now
		notification, err = s.createTaskNotificationTx(
			c.Request().Context(),
			tx,
			task,
		)
		return err
	})
	if errors.Is(err, errInvalidTaskAssignee) {
		return taskInvalidRequest(c, "负责人不存在、不可用或无项目访问权限")
	}
	if errors.Is(err, errInvalidTaskPatch) {
		return taskInvalidRequest(c, err.Error())
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return taskNotFound(c)
	}
	if err != nil {
		return taskInternalError(c)
	}
	s.publishTaskNotification(c.Request().Context(), notification)
	return success(c, http.StatusOK, newTaskResponse(task))
}

// deleteTask godoc
//
// @Summary 删除项目任务
// @Description 删除当前用户可访问的项目任务。
// @Tags 客户端任务
// @Produce json
// @Param project_id path string true "项目 ID"
// @Param task_id path string true "任务 ID"
// @Success 200 {object} successEnvelope{data=deleteTaskResponse}
// @Failure 400 {object} errorEnvelope
// @Failure 401 {object} errorEnvelope
// @Failure 404 {object} errorEnvelope
// @Failure 500 {object} errorEnvelope
// @Security UserSession
// @Router /api/client/projects/{project_id}/tasks/{task_id} [delete]
func (s *Server) deleteTask(c echo.Context) error {
	user, ok := currentUser(c)
	if !ok {
		return taskInternalError(c)
	}
	projectID, taskID, err := parseTaskPathIDs(c)
	if err != nil {
		return taskInvalidRequest(c, err.Error())
	}
	err = s.db.WithContext(c.Request().Context()).Transaction(func(tx *gorm.DB) error {
		if _, _, err := findAccessibleProjectForUpdate(tx, projectID, user.ID); err != nil {
			return err
		}
		if _, err := findTaskForUpdate(tx, projectID, taskID); err != nil {
			return err
		}
		result := tx.Where("id = ? AND project_id = ? AND deleted_at IS NULL", taskID, projectID).
			Delete(&store.Task{})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		now := time.Now().UTC()
		projectResult := tx.Model(&store.Project{}).
			Where("id = ? AND deleted_at IS NULL", projectID).
			Update("updated_at", now)
		if projectResult.Error != nil {
			return projectResult.Error
		}
		if projectResult.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return nil
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return taskNotFound(c)
	}
	if err != nil {
		return taskInternalError(c)
	}
	return success(c, http.StatusOK, map[string]string{"task_id": taskID})
}

func parseTaskListFilters(c echo.Context) (taskListFilters, error) {
	params := c.QueryParams()
	filters := taskListFilters{Keyword: c.QueryParam("keyword"), Limit: defaultTaskPageLimit}
	if err := validatePostgresText(filters.Keyword, "搜索关键词"); err != nil {
		return filters, err
	}
	if _, present := params["status"]; present {
		statuses, err := parseTaskStatusFilter(c.QueryParam("status"))
		if err != nil {
			return filters, err
		}
		filters.Statuses = statuses
	}
	if _, present := params["priority"]; present {
		priorities, err := parseTaskPriorityFilter(c.QueryParam("priority"))
		if err != nil {
			return filters, err
		}
		filters.Priorities = priorities
	}
	if _, present := params["assignee_user_id"]; present {
		assigneeUserIDs, err := parseTaskAssigneeFilter(c.QueryParam("assignee_user_id"))
		if err != nil {
			return filters, err
		}
		filters.AssigneeUserIDs = assigneeUserIDs
	}
	if _, present := params["label"]; present {
		label := strings.TrimSpace(c.QueryParam("label"))
		if err := validatePostgresText(label, "任务标签"); err != nil {
			return filters, err
		}
		length := utf8.RuneCountInString(label)
		if length < 1 || length > 32 {
			return filters, errors.New("标签长度必须为 1 到 32 个字符")
		}
		filters.Label = &label
	}
	var err error
	if filters.StartDateFrom, err = parseTaskFilterDate(params, "start_date_from"); err != nil {
		return filters, err
	}
	if filters.StartDateTo, err = parseTaskFilterDate(params, "start_date_to"); err != nil {
		return filters, err
	}
	if filters.DueDateFrom, err = parseTaskFilterDate(params, "due_date_from"); err != nil {
		return filters, err
	}
	if filters.DueDateTo, err = parseTaskFilterDate(params, "due_date_to"); err != nil {
		return filters, err
	}
	if filters.StartDateFrom != nil && filters.StartDateTo != nil && filters.StartDateFrom.After(*filters.StartDateTo) {
		return filters, errors.New("开始日期筛选范围无效")
	}
	if filters.DueDateFrom != nil && filters.DueDateTo != nil && filters.DueDateFrom.After(*filters.DueDateTo) {
		return filters, errors.New("截止日期筛选范围无效")
	}
	if _, present := params["limit"]; present {
		limit, err := strconv.Atoi(c.QueryParam("limit"))
		if err != nil || limit < 1 || limit > maxTaskPageLimit {
			return filters, errors.New("limit 必须为 1 到 100 的整数")
		}
		filters.Limit = limit
	}
	if _, present := params["cursor"]; present {
		cursor, err := decodeTaskListCursor(c.QueryParam("cursor"))
		if err != nil {
			return filters, errors.New("任务游标格式错误")
		}
		filters.Cursor = cursor
	}
	return filters, nil
}

func parseTaskStatusFilter(value string) ([]string, error) {
	parts := strings.Split(value, ",")
	statuses := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		status := strings.TrimSpace(part)
		if status == "" || !validTaskStatus(status) {
			return nil, errors.New("任务状态筛选无效")
		}
		if _, exists := seen[status]; exists {
			continue
		}
		seen[status] = struct{}{}
		statuses = append(statuses, status)
	}
	return statuses, nil
}

func parseTaskPriorityFilter(value string) ([]int16, error) {
	parts := strings.Split(value, ",")
	priorities := make([]int16, 0, len(parts))
	seen := make(map[int16]struct{}, len(parts))
	for _, part := range parts {
		parsed, err := strconv.Atoi(strings.TrimSpace(part))
		priority := int16(parsed)
		if err != nil || parsed < int(store.TaskPriorityLow) || parsed > int(store.TaskPriorityHigh) {
			return nil, errors.New("任务优先级筛选无效")
		}
		if _, exists := seen[priority]; exists {
			continue
		}
		seen[priority] = struct{}{}
		priorities = append(priorities, priority)
	}
	return priorities, nil
}

func parseTaskAssigneeFilter(value string) ([]string, error) {
	parts := strings.Split(value, ",")
	assigneeUserIDs := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		assigneeUserID, err := parseTaskUUID(strings.TrimSpace(part), "负责人 ID 格式错误")
		if err != nil {
			return nil, err
		}
		if _, exists := seen[assigneeUserID]; exists {
			continue
		}
		seen[assigneeUserID] = struct{}{}
		assigneeUserIDs = append(assigneeUserIDs, assigneeUserID)
	}
	return assigneeUserIDs, nil
}

func parseTaskFilterDate(params map[string][]string, key string) (*time.Time, error) {
	values, present := params[key]
	if !present {
		return nil, nil
	}
	value := ""
	if len(values) > 0 {
		value = values[0]
	}
	parsed, err := parseTaskDate(value)
	if err != nil {
		return nil, errors.New(key + " 格式必须为 YYYY-MM-DD")
	}
	return &parsed, nil
}

func applyTaskListFilters(query *gorm.DB, dialect string, filters taskListFilters) *gorm.DB {
	if filters.Keyword != "" {
		pattern := "%" + escapeTaskLikePattern(strings.ToLower(filters.Keyword)) + "%"
		query = query.Where(
			"(LOWER(title) LIKE ? ESCAPE '\\' OR LOWER(description) LIKE ? ESCAPE '\\')",
			pattern,
			pattern,
		)
	}
	if len(filters.Statuses) > 0 {
		query = query.Where("status IN ?", filters.Statuses)
	}
	if len(filters.Priorities) > 0 {
		query = query.Where("priority IN ?", filters.Priorities)
	}
	if len(filters.AssigneeUserIDs) > 0 {
		query = query.Where("assignee_user_id IN ?", filters.AssigneeUserIDs)
	}
	if filters.Label != nil {
		if dialect == "sqlite" {
			query = query.Where(
				"EXISTS (SELECT 1 FROM json_each('[' || substr(tasks.labels, 2, length(tasks.labels) - 2) || ']') AS task_label WHERE task_label.value = ?)",
				*filters.Label,
			)
		} else {
			query = query.Where("tasks.labels @> ?", pq.Array([]string{*filters.Label}))
		}
	}
	if filters.StartDateFrom != nil {
		query = query.Where("start_date >= ?", *filters.StartDateFrom)
	}
	if filters.StartDateTo != nil {
		query = query.Where("start_date <= ?", *filters.StartDateTo)
	}
	if filters.DueDateFrom != nil {
		query = query.Where("due_date >= ?", *filters.DueDateFrom)
	}
	if filters.DueDateTo != nil {
		query = query.Where("due_date <= ?", *filters.DueDateTo)
	}
	return query
}

func escapeTaskLikePattern(value string) string {
	replacer := strings.NewReplacer("\\", "\\\\", "%", "\\%", "_", "\\_")
	return replacer.Replace(value)
}

func decodeTaskListCursor(value string) (*struct {
	UpdatedAt time.Time
	ID        string
}, error) {
	if value == "" {
		return nil, errors.New("empty cursor")
	}
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var cursor taskListCursor
	if err := decoder.Decode(&cursor); err != nil {
		return nil, err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, errors.New("cursor contains trailing data")
		}
		return nil, err
	}
	updatedAt, err := time.Parse(time.RFC3339Nano, cursor.UpdatedAt)
	if err != nil {
		return nil, err
	}
	id, err := parseTaskUUID(cursor.ID, "任务游标格式错误")
	if err != nil {
		return nil, err
	}
	return &struct {
		UpdatedAt time.Time
		ID        string
	}{UpdatedAt: updatedAt, ID: id}, nil
}

func encodeTaskListCursor(task store.Task) (string, error) {
	raw, err := json.Marshal(taskListCursor{
		UpdatedAt: task.UpdatedAt.Format(time.RFC3339Nano),
		ID:        task.ID,
	})
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func normalizeTaskPatch(req updateTaskRequest) (normalizedTaskPatch, error) {
	var patch normalizedTaskPatch
	if req.Title.Present {
		if req.Title.Null {
			return patch, errors.New("标题不能为 null")
		}
		title, err := normalizeTaskTitle(req.Title.Value)
		if err != nil {
			return patch, err
		}
		patch.Title = &title
	}
	if req.Description.Present {
		if req.Description.Null {
			return patch, errors.New("描述不能为 null")
		}
		if err := validatePostgresText(req.Description.Value, "任务描述"); err != nil {
			return patch, err
		}
		patch.Description = &req.Description.Value
	}
	if req.Status.Present {
		if req.Status.Null || !validTaskStatus(req.Status.Value) {
			return patch, errors.New("任务状态无效")
		}
		patch.Status = &req.Status.Value
	}
	if req.Priority.Present {
		if req.Priority.Null || !validTaskPriority(req.Priority.Value) {
			return patch, errors.New("任务优先级无效")
		}
		patch.Priority = &req.Priority.Value
	}
	if req.AssigneeUserID.Present {
		patch.AssigneePresent = true
		if !req.AssigneeUserID.Null {
			assigneeUserID, err := parseTaskUUID(req.AssigneeUserID.Value, "负责人 ID 格式错误")
			if err != nil {
				return patch, err
			}
			patch.AssigneeUserID = &assigneeUserID
		}
	}
	if req.StartDate.Present {
		patch.StartPresent = true
		if !req.StartDate.Null {
			startDate, err := parseTaskDate(req.StartDate.Value)
			if err != nil {
				return patch, errors.New("开始日期格式必须为 YYYY-MM-DD")
			}
			patch.StartDate = &startDate
		}
	}
	if req.DueDate.Present {
		patch.DuePresent = true
		if !req.DueDate.Null {
			dueDate, err := parseTaskDate(req.DueDate.Value)
			if err != nil {
				return patch, errors.New("截止日期格式必须为 YYYY-MM-DD")
			}
			patch.DueDate = &dueDate
		}
	}
	if req.Labels.Present {
		if req.Labels.Null {
			return patch, errors.New("标签不能为 null")
		}
		labels, err := normalizeTaskLabels(req.Labels.Value)
		if err != nil {
			return patch, err
		}
		patch.Labels = &labels
	}
	return patch, nil
}

func validateTaskPatchDates(task store.Task, patch normalizedTaskPatch) error {
	startDate := task.StartDate
	if patch.StartPresent {
		startDate = patch.StartDate
	}
	dueDate := task.DueDate
	if patch.DuePresent {
		dueDate = patch.DueDate
	}
	if startDate != nil && dueDate != nil && startDate.After(*dueDate) {
		return errInvalidTaskPatch
	}
	return nil
}

func taskPatchUpdates(task *store.Task, patch normalizedTaskPatch, now time.Time) map[string]any {
	updates := make(map[string]any)
	if patch.Title != nil && task.Title != *patch.Title {
		task.Title = *patch.Title
		updates["title"] = task.Title
	}
	if patch.Description != nil && task.Description != *patch.Description {
		task.Description = *patch.Description
		updates["description"] = task.Description
	}
	if patch.Status != nil && task.Status != *patch.Status {
		task.Status = *patch.Status
		setTaskTerminalTimestamps(task, now)
		updates["status"] = task.Status
		updates["completed_at"] = task.CompletedAt
		updates["canceled_at"] = task.CanceledAt
	}
	if patch.Priority != nil && task.Priority != *patch.Priority {
		task.Priority = *patch.Priority
		updates["priority"] = task.Priority
	}
	if patch.AssigneePresent && !equalTaskStringPointers(task.AssigneeUserID, patch.AssigneeUserID) {
		task.AssigneeUserID = patch.AssigneeUserID
		updates["assignee_user_id"] = patch.AssigneeUserID
		if patch.AssigneeUserID == nil {
			task.AssigneeUser = nil
		}
	}
	if patch.StartPresent && !equalTaskDates(task.StartDate, patch.StartDate) {
		task.StartDate = patch.StartDate
		updates["start_date"] = patch.StartDate
	}
	if patch.DuePresent && !equalTaskDates(task.DueDate, patch.DueDate) {
		task.DueDate = patch.DueDate
		updates["due_date"] = patch.DueDate
	}
	if patch.Labels != nil && !slices.Equal(task.Labels, *patch.Labels) {
		task.Labels = append(pq.StringArray{}, (*patch.Labels)...)
		updates["labels"] = task.Labels
	}
	return updates
}

func equalTaskStringPointers(left *string, right *string) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func equalTaskDates(left *time.Time, right *time.Time) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return left.Format(taskDateLayout) == right.Format(taskDateLayout)
}

func (s *Server) findAccessibleTask(c echo.Context, projectID string, taskID string, userID string) (store.Task, error) {
	if _, _, err := s.findAccessibleProject(c.Request().Context(), projectID, userID); err != nil {
		return store.Task{}, err
	}
	var task store.Task
	err := s.db.WithContext(c.Request().Context()).
		Preload("AssigneeUser").
		Preload("CreatedByUser").
		Where("id = ? AND project_id = ?", taskID, projectID).
		First(&task).Error
	return task, err
}

func findTaskForUpdate(tx *gorm.DB, projectID string, taskID string) (store.Task, error) {
	var task store.Task
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Preload("AssigneeUser").
		Preload("CreatedByUser").
		Where("id = ? AND project_id = ?", taskID, projectID).
		First(&task).Error
	return task, err
}

func parseTaskPathIDs(c echo.Context) (string, string, error) {
	projectID, err := parseProjectID(c.Param("project_id"))
	if err != nil {
		return "", "", err
	}
	taskID, err := parseTaskUUID(c.Param("task_id"), "任务 ID 格式错误")
	if err != nil {
		return "", "", err
	}
	return projectID, taskID, nil
}

func normalizeCreateTask(req createTaskRequest, projectID string, creatorUserID string) (store.Task, error) {
	if !req.Title.Present || req.Title.Null {
		return store.Task{}, errors.New("标题不能为空")
	}
	title, err := normalizeTaskTitle(req.Title.Value)
	if err != nil {
		return store.Task{}, err
	}

	description := ""
	if req.Description.Present {
		if req.Description.Null {
			return store.Task{}, errors.New("描述不能为 null")
		}
		if err := validatePostgresText(req.Description.Value, "任务描述"); err != nil {
			return store.Task{}, err
		}
		description = req.Description.Value
	}
	status := store.TaskStatusTodo
	if req.Status.Present {
		if req.Status.Null || !validTaskStatus(req.Status.Value) {
			return store.Task{}, errors.New("任务状态无效")
		}
		status = req.Status.Value
	}
	priority := store.TaskPriorityMedium
	if req.Priority.Present {
		if req.Priority.Null || !validTaskPriority(req.Priority.Value) {
			return store.Task{}, errors.New("任务优先级无效")
		}
		priority = req.Priority.Value
	}

	var assigneeUserID *string
	if req.AssigneeUserID.Present && !req.AssigneeUserID.Null {
		parsed, err := parseTaskUUID(req.AssigneeUserID.Value, "负责人 ID 格式错误")
		if err != nil {
			return store.Task{}, err
		}
		assigneeUserID = &parsed
	}
	startDate, err := parseCreateTaskDate(req.StartDate, "开始日期")
	if err != nil {
		return store.Task{}, err
	}
	dueDate, err := parseCreateTaskDate(req.DueDate, "截止日期")
	if err != nil {
		return store.Task{}, err
	}
	if startDate != nil && dueDate != nil && startDate.After(*dueDate) {
		return store.Task{}, errors.New("开始日期不能晚于截止日期")
	}

	labels := pq.StringArray{}
	if req.Labels.Present {
		if req.Labels.Null {
			return store.Task{}, errors.New("标签不能为 null")
		}
		labels, err = normalizeTaskLabels(req.Labels.Value)
		if err != nil {
			return store.Task{}, err
		}
	}

	return store.Task{
		ProjectID:       projectID,
		Title:           title,
		Description:     description,
		Status:          status,
		Priority:        priority,
		AssigneeUserID:  assigneeUserID,
		StartDate:       startDate,
		DueDate:         dueDate,
		Labels:          labels,
		CreatedByUserID: creatorUserID,
	}, nil
}

func normalizeTaskTitle(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if err := validatePostgresText(trimmed, "任务标题"); err != nil {
		return "", err
	}
	length := utf8.RuneCountInString(trimmed)
	if length < 1 || length > 240 {
		return "", errors.New("标题长度必须为 1 到 240 个字符")
	}
	return trimmed, nil
}

func normalizeTaskLabels(values []string) (pq.StringArray, error) {
	if len(values) > 20 {
		return nil, errors.New("标签不能超过 20 个")
	}
	result := make(pq.StringArray, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if err := validatePostgresText(trimmed, "任务标签"); err != nil {
			return nil, err
		}
		length := utf8.RuneCountInString(trimmed)
		if length < 1 || length > 32 {
			return nil, errors.New("标签长度必须为 1 到 32 个字符")
		}
		key := strings.ToLower(trimmed)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, trimmed)
	}
	return result, nil
}

func parseCreateTaskDate(value taskOptionalString, fieldName string) (*time.Time, error) {
	if !value.Present || value.Null {
		return nil, nil
	}
	parsed, err := parseTaskDate(value.Value)
	if err != nil {
		return nil, errors.New(fieldName + "格式必须为 YYYY-MM-DD")
	}
	return &parsed, nil
}

func parseTaskDate(value string) (time.Time, error) {
	if len(value) != len(taskDateLayout) {
		return time.Time{}, errors.New("invalid date")
	}
	parsed, err := time.Parse(taskDateLayout, value)
	if err != nil || parsed.Format(taskDateLayout) != value {
		return time.Time{}, errors.New("invalid date")
	}
	return parsed, nil
}

func validTaskStatus(value string) bool {
	switch value {
	case store.TaskStatusTodo, store.TaskStatusInProgress, store.TaskStatusDone, store.TaskStatusCanceled:
		return true
	default:
		return false
	}
}

func validTaskPriority(value int16) bool {
	return value >= store.TaskPriorityLow && value <= store.TaskPriorityHigh
}

func parseTaskUUID(value string, message string) (string, error) {
	parsed, err := uuid.Parse(strings.TrimSpace(value))
	if err != nil {
		return "", errors.New(message)
	}
	return parsed.String(), nil
}

func validateTaskAssignee(tx *gorm.DB, projectID string, assigneeUserID string) (store.User, error) {
	var user store.User
	err := tx.Where("id = ? AND status = ?", assigneeUserID, store.UserStatusActive).First(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return store.User{}, errInvalidTaskAssignee
	}
	if err != nil {
		return store.User{}, err
	}
	var project store.Project
	err = tx.Where("id = ?", projectID).
		Where(projectAccessSQL(), projectAccessArgs(assigneeUserID)...).
		First(&project).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return store.User{}, errInvalidTaskAssignee
	}
	if err != nil {
		return store.User{}, err
	}
	return user, nil
}

func setTaskTerminalTimestamps(task *store.Task, now time.Time) {
	switch task.Status {
	case store.TaskStatusDone:
		task.CompletedAt = &now
		task.CanceledAt = nil
	case store.TaskStatusCanceled:
		task.CanceledAt = &now
		task.CompletedAt = nil
	default:
		task.CompletedAt = nil
		task.CanceledAt = nil
	}
}

func newTaskResponse(task store.Task) taskResponse {
	labels := make([]string, len(task.Labels))
	copy(labels, task.Labels)
	response := taskResponse{
		ID:          task.ID,
		ProjectID:   task.ProjectID,
		Title:       task.Title,
		Description: task.Description,
		Status:      task.Status,
		Priority:    task.Priority,
		Creator:     newTaskUserSummary(task.CreatedByUser),
		StartDate:   formatTaskDate(task.StartDate),
		DueDate:     formatTaskDate(task.DueDate),
		Labels:      labels,
		CompletedAt: task.CompletedAt,
		CanceledAt:  task.CanceledAt,
		CreatedAt:   task.CreatedAt,
		UpdatedAt:   task.UpdatedAt,
	}
	if task.AssigneeUser != nil {
		summary := newTaskUserSummary(*task.AssigneeUser)
		response.Assignee = &summary
	}
	return response
}

func newTaskUserSummary(user store.User) projectUserSummary {
	return projectUserSummary{
		ID:       user.ID,
		Name:     user.Name,
		Nickname: user.Nickname,
		Avatar:   user.Avatar,
	}
}

func formatTaskDate(value *time.Time) *string {
	if value == nil {
		return nil
	}
	formatted := value.Format(taskDateLayout)
	return &formatted
}

func taskInvalidRequest(c echo.Context, message string) error {
	return failure(c, http.StatusBadRequest, "invalid_request", message)
}

func taskNotFound(c echo.Context) error {
	return failure(c, http.StatusNotFound, "not_found", "任务不存在")
}

func taskInternalError(c echo.Context) error {
	return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
}
