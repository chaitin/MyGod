package httpserver

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
	Reminder       taskOptionalReminder    `json:"reminder"`
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
	Reminder        taskOptionalReminder    `json:"reminder"`
}
