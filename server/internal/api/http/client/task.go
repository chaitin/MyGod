package client

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"app/internal/application/task"

	"github.com/labstack/echo/v4"
)

type TaskAPI struct{ tasks task.ClientService }

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
type taskReminderRequest struct {
	Mode       string  `json:"mode" enums:"once,recurring"`
	Frequency  string  `json:"frequency,omitempty" enums:"daily,weekly,monthly"`
	Timezone   string  `json:"timezone" example:"Asia/Shanghai"`
	At         string  `json:"at,omitempty" format:"date-time"`
	Time       string  `json:"time,omitempty" example:"09:30"`
	Weekdays   []int16 `json:"weekdays,omitempty"`
	DayOfMonth int16   `json:"day_of_month,omitempty" example:"15"`
}
type taskOptionalReminder struct {
	Present bool
	Null    bool
	Value   taskReminderRequest
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
	Reminder       taskOptionalReminder    `json:"reminder" swaggertype:"object" extensions:"x-nullable"`
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
	Reminder       taskOptionalReminder    `json:"reminder" swaggertype:"object" extensions:"x-nullable"`
}

type taskReminderResponse struct {
	Mode            string     `json:"mode"`
	Frequency       string     `json:"frequency,omitempty"`
	Timezone        string     `json:"timezone"`
	At              *time.Time `json:"at,omitempty"`
	Time            string     `json:"time,omitempty"`
	Weekdays        []int16    `json:"weekdays,omitempty"`
	DayOfMonth      *int16     `json:"day_of_month,omitempty"`
	NextTriggerAt   *time.Time `json:"next_trigger_at" extensions:"x-nullable"`
	LastProcessedAt *time.Time `json:"last_processed_at" extensions:"x-nullable"`
	State           string     `json:"state"`
}

type taskResponse struct {
	ID          string                `json:"id"`
	ProjectID   string                `json:"project_id"`
	Title       string                `json:"title"`
	Description string                `json:"description"`
	Status      string                `json:"status"`
	Priority    int16                 `json:"priority"`
	Assignee    *projectUserResponse  `json:"assignee" extensions:"x-nullable"`
	Creator     projectUserResponse   `json:"creator"`
	StartDate   *string               `json:"start_date" extensions:"x-nullable"`
	DueDate     *string               `json:"due_date" extensions:"x-nullable"`
	Labels      []string              `json:"labels"`
	CompletedAt *time.Time            `json:"completed_at" extensions:"x-nullable"`
	CanceledAt  *time.Time            `json:"canceled_at" extensions:"x-nullable"`
	CreatedAt   time.Time             `json:"created_at"`
	UpdatedAt   time.Time             `json:"updated_at"`
	Reminder    *taskReminderResponse `json:"reminder" extensions:"x-nullable"`
}

type taskListResponse struct {
	Tasks      []taskResponse `json:"tasks"`
	NextCursor *string        `json:"next_cursor" extensions:"x-nullable"`
}
type deleteTaskResponse struct {
	TaskID string `json:"task_id" example:"7f8d8b84-6d2c-4b12-9a8a-019a7e2787d4"`
}

func NewTaskAPI(tasks task.ClientService) *TaskAPI { return &TaskAPI{tasks: tasks} }

func (a *TaskAPI) RegisterRoutes(group *echo.Group) {
	group.GET("/projects/:project_id/tasks", a.list)
	group.POST("/projects/:project_id/tasks", a.create)
	group.GET("/projects/:project_id/tasks/:task_id", a.get)
	group.PATCH("/projects/:project_id/tasks/:task_id", a.update)
	group.DELETE("/projects/:project_id/tasks/:task_id", a.delete)
}

// list godoc
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
func (a *TaskAPI) list(c echo.Context) error {
	current, ok := CurrentAccount(c)
	if !ok {
		return writeFailure(c, 500, string(task.CodeInternal), "服务端错误")
	}
	limit := 0
	if raw, present := c.QueryParams()["limit"]; present {
		value := ""
		if len(raw) > 0 {
			value = raw[0]
		}
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 1 || parsed > task.MaxPageLimit {
			return writeFailure(c, 400, string(task.CodeInvalidRequest), "limit 必须为 1 到 100 的整数")
		}
		limit = parsed
	}
	cmd := task.ListCommand{AccountID: current.ID, ProjectID: c.Param("project_id"), Keyword: c.QueryParam("keyword"), Limit: limit}
	params := c.QueryParams()
	cmd.Status = queryStringField(params, "status")
	cmd.Priority = queryStringField(params, "priority")
	cmd.Assignee = queryStringField(params, "assignee_user_id")
	cmd.Label = queryStringField(params, "label")
	cmd.StartFrom = queryStringField(params, "start_date_from")
	cmd.StartTo = queryStringField(params, "start_date_to")
	cmd.DueFrom = queryStringField(params, "due_date_from")
	cmd.DueTo = queryStringField(params, "due_date_to")
	cmd.Cursor = queryStringField(params, "cursor")
	result, err := a.tasks.List(c.Request().Context(), cmd)
	if err != nil {
		return writeTaskError(c, err)
	}
	values := make([]taskResponse, 0, len(result.Tasks))
	for _, value := range result.Tasks {
		values = append(values, newTaskResponse(value))
	}
	return writeSuccess(c, 200, taskListResponse{Tasks: values, NextCursor: result.NextCursor})
}

// create godoc
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
func (a *TaskAPI) create(c echo.Context) error {
	current, ok := CurrentAccount(c)
	if !ok {
		return writeFailure(c, 500, string(task.CodeInternal), "服务端错误")
	}
	var req createTaskRequest
	if err := decodeStrictJSON(c, &req); err != nil {
		return writeFailure(c, 400, string(task.CodeInvalidRequest), "请求格式错误")
	}
	value, err := a.tasks.Create(c.Request().Context(), task.CreateCommand{AccountID: current.ID, ProjectID: c.Param("project_id"), Title: stringField(req.Title), Description: stringField(req.Description), Status: stringField(req.Status), Priority: int16Field(req.Priority), AssigneeUserID: stringField(req.AssigneeUserID), StartDate: stringField(req.StartDate), DueDate: stringField(req.DueDate), Labels: stringSliceField(req.Labels), Reminder: reminderField(req.Reminder)})
	if err != nil {
		return writeTaskError(c, err)
	}
	return writeSuccess(c, http.StatusCreated, newTaskResponse(value))
}

// get godoc
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
func (a *TaskAPI) get(c echo.Context) error {
	current, ok := CurrentAccount(c)
	if !ok {
		return writeFailure(c, 500, string(task.CodeInternal), "服务端错误")
	}
	value, err := a.tasks.Get(c.Request().Context(), task.GetCommand{AccountID: current.ID, ProjectID: c.Param("project_id"), TaskID: c.Param("task_id")})
	if err != nil {
		return writeTaskError(c, err)
	}
	return writeSuccess(c, 200, newTaskResponse(value))
}

// update godoc
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
func (a *TaskAPI) update(c echo.Context) error {
	current, ok := CurrentAccount(c)
	if !ok {
		return writeFailure(c, 500, string(task.CodeInternal), "服务端错误")
	}
	var req updateTaskRequest
	if err := decodeStrictJSON(c, &req); err != nil {
		return writeFailure(c, 400, string(task.CodeInvalidRequest), "请求格式错误")
	}
	value, err := a.tasks.Update(c.Request().Context(), task.UpdateCommand{AccountID: current.ID, ProjectID: c.Param("project_id"), TaskID: c.Param("task_id"), Title: stringField(req.Title), Description: stringField(req.Description), Status: stringField(req.Status), Priority: int16Field(req.Priority), AssigneeUserID: stringField(req.AssigneeUserID), StartDate: stringField(req.StartDate), DueDate: stringField(req.DueDate), Labels: stringSliceField(req.Labels), Reminder: reminderField(req.Reminder)})
	if err != nil {
		return writeTaskError(c, err)
	}
	return writeSuccess(c, 200, newTaskResponse(value))
}

// delete godoc
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
func (a *TaskAPI) delete(c echo.Context) error {
	current, ok := CurrentAccount(c)
	if !ok {
		return writeFailure(c, 500, string(task.CodeInternal), "服务端错误")
	}
	id, err := a.tasks.Delete(c.Request().Context(), task.GetCommand{AccountID: current.ID, ProjectID: c.Param("project_id"), TaskID: c.Param("task_id")})
	if err != nil {
		return writeTaskError(c, err)
	}
	return writeSuccess(c, 200, deleteTaskResponse{TaskID: id})
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
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	return decoder.Decode(&value.Value)
}

func queryStringField(values map[string][]string, key string) task.Field[string] {
	raw, present := values[key]
	value := ""
	if len(raw) > 0 {
		value = raw[0]
	}
	return task.Field[string]{Present: present, Value: value}
}
func stringField(value taskOptionalString) task.Field[string] {
	return task.Field[string]{Present: value.Present, Null: value.Null, Value: value.Value}
}
func int16Field(value taskOptionalInt16) task.Field[int16] {
	return task.Field[int16]{Present: value.Present, Null: value.Null, Value: value.Value}
}
func stringSliceField(value taskOptionalStringSlice) task.Field[[]string] {
	return task.Field[[]string]{Present: value.Present, Null: value.Null, Value: value.Value}
}
func reminderField(value taskOptionalReminder) task.Field[task.ReminderInput] {
	return task.Field[task.ReminderInput]{Present: value.Present, Null: value.Null, Value: task.ReminderInput{
		Mode: value.Value.Mode, Frequency: value.Value.Frequency, Timezone: value.Value.Timezone,
		At: value.Value.At, Time: value.Value.Time, Weekdays: value.Value.Weekdays, DayOfMonth: value.Value.DayOfMonth,
	}}
}

func newTaskResponse(value task.Task) taskResponse {
	result := taskResponse{ID: value.ID, ProjectID: value.ProjectID, Title: value.Title, Description: value.Description, Status: value.Status, Priority: value.Priority, Creator: projectUserResponse{ID: value.Creator.ID, Name: value.Creator.Name, Nickname: value.Creator.Nickname, Avatar: value.Creator.Avatar}, StartDate: value.StartDate, DueDate: value.DueDate, Labels: value.Labels, CompletedAt: value.CompletedAt, CanceledAt: value.CanceledAt, CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt}
	if value.Assignee != nil {
		result.Assignee = &projectUserResponse{ID: value.Assignee.ID, Name: value.Assignee.Name, Nickname: value.Assignee.Nickname, Avatar: value.Assignee.Avatar}
	}
	if value.Reminder != nil {
		result.Reminder = &taskReminderResponse{
			Mode: value.Reminder.Mode, Frequency: value.Reminder.Frequency, Timezone: value.Reminder.Timezone,
			At: value.Reminder.At, Time: value.Reminder.Time, Weekdays: value.Reminder.Weekdays,
			DayOfMonth: value.Reminder.DayOfMonth, NextTriggerAt: value.Reminder.NextTriggerAt,
			LastProcessedAt: value.Reminder.LastProcessedAt, State: value.Reminder.State,
		}
	}
	return result
}

func writeTaskError(c echo.Context, err error) error {
	status := http.StatusInternalServerError
	switch task.ErrorCodeOf(err) {
	case task.CodeInvalidRequest:
		status = 400
	case task.CodeNotFound:
		status = 404
	case task.CodeConflict:
		status = 409
	}
	return writeFailure(c, status, string(task.ErrorCodeOf(err)), task.ErrorMessage(err))
}
