package task

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"app/internal/store"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	errInvalidAssignee = errors.New("invalid task assignee")
	errInvalidPatch    = errors.New("任务字段组合无效")
	errVersionConflict = errors.New("task version conflict")
)

type Dependencies struct {
	DB            *gorm.DB
	Notifications NotificationPort
	Now           func() time.Time
}

type Service struct {
	db            *gorm.DB
	notifications NotificationPort
	now           func() time.Time
}

type listCursor struct {
	UpdatedAt string `json:"updated_at"`
	ID        string `json:"id"`
}

type listFilters struct {
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

type normalizedPatch struct {
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
	Reminder        Field[ReminderInput]
}

func NewService(deps Dependencies) *Service {
	now := deps.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &Service{db: deps.DB, notifications: deps.Notifications, now: now}
}

func (s *Service) List(ctx context.Context, cmd ListCommand) (ListResult, error) {
	projectID, err := parseUUID(cmd.ProjectID, "项目 ID 格式错误")
	if err != nil {
		return ListResult{}, err
	}
	filters, err := normalizeListFilters(cmd)
	if err != nil {
		return ListResult{}, err
	}
	if err := s.requireProjectAccess(ctx, projectID, strings.TrimSpace(cmd.AccountID)); err != nil {
		return ListResult{}, mapNotFound(err)
	}
	query := s.db.WithContext(ctx).Preload("AssigneeUser").Preload("CreatedByUser").Preload("Reminder").Where("project_id = ?", projectID)
	query = applyListFilters(query, s.db.Dialector.Name(), filters)
	if filters.Cursor != nil {
		query = query.Where("(updated_at < ?) OR (updated_at = ? AND id < ?)", filters.Cursor.UpdatedAt, filters.Cursor.UpdatedAt, filters.Cursor.ID)
	}
	var tasks []store.Task
	if err := query.Order("updated_at DESC").Order("id DESC").Limit(filters.Limit + 1).Find(&tasks).Error; err != nil {
		return ListResult{}, internalError(err)
	}
	var nextCursor *string
	if len(tasks) > filters.Limit {
		tasks = tasks[:filters.Limit]
		encoded, err := encodeCursor(tasks[len(tasks)-1])
		if err != nil {
			return ListResult{}, internalError(err)
		}
		nextCursor = &encoded
	}
	result := make([]Task, 0, len(tasks))
	for _, value := range tasks {
		result = append(result, newTask(value))
	}
	return ListResult{Tasks: result, NextCursor: nextCursor}, nil
}

func (s *Service) Create(ctx context.Context, cmd CreateCommand) (Task, error) {
	projectID, err := parseUUID(cmd.ProjectID, "项目 ID 格式错误")
	if err != nil {
		return Task{}, err
	}
	accountID := strings.TrimSpace(cmd.AccountID)
	value, err := normalizeCreate(cmd, projectID, accountID)
	if err != nil {
		return Task{}, err
	}
	var assignee *store.User
	var reminder *store.TaskReminder
	var notification any
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireProjectAccessForUpdate(tx, projectID, accountID); err != nil {
			return err
		}
		if value.AssigneeUserID != nil {
			validated, err := validateAssignee(tx, projectID, *value.AssigneeUserID)
			if err != nil {
				return err
			}
			assignee = &validated
		}
		now := s.now().UTC()
		if cmd.Reminder.Present && !cmd.Reminder.Null {
			reminder, err = normalizeReminder(cmd.Reminder.Value, now, value.Status)
			if err != nil {
				return err
			}
		}
		value.ID = uuid.NewString()
		value.CreatedAt = now
		value.UpdatedAt = now
		setTerminalTimestamps(&value, now)
		if err := tx.Create(&value).Error; err != nil {
			return err
		}
		if reminder != nil {
			reminder.TaskID = value.ID
			if err := tx.Create(reminder).Error; err != nil {
				return err
			}
			value.Reminder = reminder
		}
		if err := updateProjectTimestamp(tx, projectID, now); err != nil {
			return err
		}
		if s.notifications != nil {
			prepared, err := s.notifications.PrepareTaskNotification(ctx, tx, value)
			if err != nil {
				return err
			}
			notification = prepared
		}
		return nil
	})
	if err != nil {
		return Task{}, mapMutationError(err)
	}
	var creator store.User
	if err := s.db.WithContext(ctx).First(&creator, "id = ?", accountID).Error; err != nil {
		return Task{}, internalError(err)
	}
	value.CreatedByUser = creator
	value.AssigneeUser = assignee
	if s.notifications != nil {
		s.notifications.PublishTaskNotification(ctx, notification)
	}
	return newTask(value), nil
}

func (s *Service) Get(ctx context.Context, cmd GetCommand) (Task, error) {
	projectID, taskID, err := parseIDs(cmd.ProjectID, cmd.TaskID)
	if err != nil {
		return Task{}, err
	}
	if err := s.requireProjectAccess(ctx, projectID, strings.TrimSpace(cmd.AccountID)); err != nil {
		return Task{}, mapNotFound(err)
	}
	var value store.Task
	err = s.db.WithContext(ctx).Preload("AssigneeUser").Preload("CreatedByUser").Preload("Reminder").Where("id = ? AND project_id = ?", taskID, projectID).First(&value).Error
	if err != nil {
		return Task{}, mapNotFound(err)
	}
	return newTask(value), nil
}

func (s *Service) Update(ctx context.Context, cmd UpdateCommand) (Task, error) {
	projectID, taskID, err := parseIDs(cmd.ProjectID, cmd.TaskID)
	if err != nil {
		return Task{}, err
	}
	patch, err := normalizePatch(cmd)
	if err != nil {
		return Task{}, err
	}
	accountID := strings.TrimSpace(cmd.AccountID)
	var value store.Task
	var notification any
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireProjectAccessForUpdate(tx, projectID, accountID); err != nil {
			return err
		}
		value, err = findForUpdate(tx, projectID, taskID)
		if err != nil {
			return err
		}
		if cmd.ExpectedUpdatedAt != nil && !value.UpdatedAt.Equal(*cmd.ExpectedUpdatedAt) {
			return errVersionConflict
		}
		if err := validatePatchDates(value, patch); err != nil {
			return err
		}
		if patch.AssigneePresent && patch.AssigneeUserID != nil && (value.AssigneeUserID == nil || *value.AssigneeUserID != *patch.AssigneeUserID) {
			validated, err := validateAssignee(tx, projectID, *patch.AssigneeUserID)
			if err != nil {
				return err
			}
			value.AssigneeUser = &validated
		}
		previousStatus := value.Status
		now := s.now().UTC()
		updates := patchUpdates(&value, patch, now)
		taskChanged := len(updates) > 0
		reminderChanged, reminder, err := applyReminderMutation(tx, value, previousStatus, patch.Reminder, now)
		if err != nil {
			return err
		}
		value.Reminder = reminder
		if !taskChanged && !reminderChanged {
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
		if err := updateProjectTimestamp(tx, projectID, now); err != nil {
			return err
		}
		value.UpdatedAt = now
		if taskChanged && s.notifications != nil {
			prepared, err := s.notifications.PrepareTaskNotification(ctx, tx, value)
			if err != nil {
				return err
			}
			notification = prepared
		}
		return nil
	})
	if err != nil {
		return Task{}, mapMutationError(err)
	}
	if s.notifications != nil {
		s.notifications.PublishTaskNotification(ctx, notification)
	}
	return newTask(value), nil
}

func (s *Service) Delete(ctx context.Context, cmd GetCommand) (string, error) {
	projectID, taskID, err := parseIDs(cmd.ProjectID, cmd.TaskID)
	if err != nil {
		return "", err
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireProjectAccessForUpdate(tx, projectID, strings.TrimSpace(cmd.AccountID)); err != nil {
			return err
		}
		if _, err := findForUpdate(tx, projectID, taskID); err != nil {
			return err
		}
		if err := tx.Where("task_id = ?", taskID).Delete(&store.TaskReminder{}).Error; err != nil {
			return err
		}
		result := tx.Where("id = ? AND project_id = ? AND deleted_at IS NULL", taskID, projectID).Delete(&store.Task{})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return updateProjectTimestamp(tx, projectID, s.now().UTC())
	})
	if err != nil {
		return "", mapNotFoundOrInternal(err)
	}
	return taskID, nil
}

func normalizeListFilters(cmd ListCommand) (listFilters, error) {
	filters := listFilters{Keyword: cmd.Keyword, Limit: cmd.Limit}
	if filters.Limit == 0 {
		filters.Limit = DefaultPageLimit
	}
	if filters.Limit < 1 || filters.Limit > MaxPageLimit {
		return filters, invalid("limit 必须为 1 到 100 的整数", nil)
	}
	if err := validateText(filters.Keyword, "搜索关键词"); err != nil {
		return filters, err
	}
	var err error
	if cmd.Status.Present {
		filters.Statuses, err = parseStatuses(cmd.Status.Value)
		if err != nil {
			return filters, err
		}
	}
	if cmd.Priority.Present {
		filters.Priorities, err = parsePriorities(cmd.Priority.Value)
		if err != nil {
			return filters, err
		}
	}
	if cmd.Assignee.Present {
		filters.AssigneeUserIDs, err = parseAssignees(cmd.Assignee.Value)
		if err != nil {
			return filters, err
		}
	}
	if cmd.Label.Present {
		label := strings.TrimSpace(cmd.Label.Value)
		if err := validateText(label, "任务标签"); err != nil {
			return filters, err
		}
		if count := utf8.RuneCountInString(label); count < 1 || count > 32 {
			return filters, invalid("标签长度必须为 1 到 32 个字符", nil)
		}
		filters.Label = &label
	}
	if cmd.StartFrom.Present {
		filters.StartDateFrom, err = parseFilterDate(cmd.StartFrom.Value, "start_date_from")
		if err != nil {
			return filters, err
		}
	}
	if cmd.StartTo.Present {
		filters.StartDateTo, err = parseFilterDate(cmd.StartTo.Value, "start_date_to")
		if err != nil {
			return filters, err
		}
	}
	if cmd.DueFrom.Present {
		filters.DueDateFrom, err = parseFilterDate(cmd.DueFrom.Value, "due_date_from")
		if err != nil {
			return filters, err
		}
	}
	if cmd.DueTo.Present {
		filters.DueDateTo, err = parseFilterDate(cmd.DueTo.Value, "due_date_to")
		if err != nil {
			return filters, err
		}
	}
	if filters.StartDateFrom != nil && filters.StartDateTo != nil && filters.StartDateFrom.After(*filters.StartDateTo) {
		return filters, invalid("开始日期筛选范围无效", nil)
	}
	if filters.DueDateFrom != nil && filters.DueDateTo != nil && filters.DueDateFrom.After(*filters.DueDateTo) {
		return filters, invalid("截止日期筛选范围无效", nil)
	}
	if cmd.Cursor.Present {
		filters.Cursor, err = decodeCursor(cmd.Cursor.Value)
		if err != nil {
			return filters, invalid("任务游标格式错误", err)
		}
	}
	return filters, nil
}

func parseStatuses(raw string) ([]string, error) {
	parts := strings.Split(raw, ",")
	result := make([]string, 0, len(parts))
	seen := map[string]struct{}{}
	for _, part := range parts {
		value := strings.TrimSpace(part)
		if !validStatus(value) {
			return nil, invalid("任务状态筛选无效", nil)
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result, nil
}

func parsePriorities(raw string) ([]int16, error) {
	parts := strings.Split(raw, ",")
	result := make([]int16, 0, len(parts))
	seen := map[int16]struct{}{}
	for _, part := range parts {
		parsed, err := strconv.Atoi(strings.TrimSpace(part))
		value := int16(parsed)
		if err != nil || parsed < int(PriorityLow) || parsed > int(PriorityHigh) {
			return nil, invalid("任务优先级筛选无效", err)
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result, nil
}

func parseAssignees(raw string) ([]string, error) {
	parts := strings.Split(raw, ",")
	result := make([]string, 0, len(parts))
	seen := map[string]struct{}{}
	for _, part := range parts {
		value, err := parseUUID(part, "负责人 ID 格式错误")
		if err != nil {
			return nil, err
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result, nil
}

func parseFilterDate(raw, key string) (*time.Time, error) {
	value, err := parseDate(raw)
	if err != nil {
		return nil, invalid(key+" 格式必须为 YYYY-MM-DD", err)
	}
	return &value, nil
}

func applyListFilters(query *gorm.DB, dialect string, filters listFilters) *gorm.DB {
	if filters.Keyword != "" {
		pattern := "%" + escapeLikePattern(strings.ToLower(filters.Keyword)) + "%"
		query = query.Where("(LOWER(title) LIKE ? ESCAPE '\\' OR LOWER(description) LIKE ? ESCAPE '\\')", pattern, pattern)
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
			query = query.Where("EXISTS (SELECT 1 FROM json_each('[' || substr(tasks.labels, 2, length(tasks.labels) - 2) || ']') AS task_label WHERE task_label.value = ?)", *filters.Label)
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

func normalizeCreate(cmd CreateCommand, projectID, accountID string) (store.Task, error) {
	if !cmd.Title.Present || cmd.Title.Null {
		return store.Task{}, invalid("标题不能为空", nil)
	}
	title, err := normalizeTitle(cmd.Title.Value)
	if err != nil {
		return store.Task{}, err
	}
	description := ""
	if cmd.Description.Present {
		if cmd.Description.Null {
			return store.Task{}, invalid("描述不能为 null", nil)
		}
		if err := validateText(cmd.Description.Value, "任务描述"); err != nil {
			return store.Task{}, err
		}
		description = cmd.Description.Value
	}
	status := StatusTodo
	if cmd.Status.Present {
		if cmd.Status.Null || !validStatus(cmd.Status.Value) {
			return store.Task{}, invalid("任务状态无效", nil)
		}
		status = cmd.Status.Value
	}
	priority := PriorityMedium
	if cmd.Priority.Present {
		if cmd.Priority.Null || !validPriority(cmd.Priority.Value) {
			return store.Task{}, invalid("任务优先级无效", nil)
		}
		priority = cmd.Priority.Value
	}
	var assignee *string
	if cmd.AssigneeUserID.Present && !cmd.AssigneeUserID.Null {
		value, err := parseUUID(cmd.AssigneeUserID.Value, "负责人 ID 格式错误")
		if err != nil {
			return store.Task{}, err
		}
		assignee = &value
	}
	start, err := parseCreateDate(cmd.StartDate, "开始日期")
	if err != nil {
		return store.Task{}, err
	}
	due, err := parseCreateDate(cmd.DueDate, "截止日期")
	if err != nil {
		return store.Task{}, err
	}
	if start != nil && due != nil && start.After(*due) {
		return store.Task{}, invalid("开始日期不能晚于截止日期", nil)
	}
	labels := pq.StringArray{}
	if cmd.Labels.Present {
		if cmd.Labels.Null {
			return store.Task{}, invalid("标签不能为 null", nil)
		}
		labels, err = normalizeLabels(cmd.Labels.Value)
		if err != nil {
			return store.Task{}, err
		}
	}
	return store.Task{ProjectID: projectID, Title: title, Description: description, Status: status, Priority: priority, AssigneeUserID: assignee, StartDate: start, DueDate: due, Labels: labels, CreatedByUserID: accountID}, nil
}

func normalizePatch(cmd UpdateCommand) (normalizedPatch, error) {
	patch := normalizedPatch{Reminder: cmd.Reminder}
	var err error
	if cmd.Title.Present {
		if cmd.Title.Null {
			return patch, invalid("标题不能为 null", nil)
		}
		value, err := normalizeTitle(cmd.Title.Value)
		if err != nil {
			return patch, err
		}
		patch.Title = &value
	}
	if cmd.Description.Present {
		if cmd.Description.Null {
			return patch, invalid("描述不能为 null", nil)
		}
		if err := validateText(cmd.Description.Value, "任务描述"); err != nil {
			return patch, err
		}
		patch.Description = &cmd.Description.Value
	}
	if cmd.Status.Present {
		if cmd.Status.Null || !validStatus(cmd.Status.Value) {
			return patch, invalid("任务状态无效", nil)
		}
		patch.Status = &cmd.Status.Value
	}
	if cmd.Priority.Present {
		if cmd.Priority.Null || !validPriority(cmd.Priority.Value) {
			return patch, invalid("任务优先级无效", nil)
		}
		patch.Priority = &cmd.Priority.Value
	}
	if cmd.AssigneeUserID.Present {
		patch.AssigneePresent = true
		if !cmd.AssigneeUserID.Null {
			value, err := parseUUID(cmd.AssigneeUserID.Value, "负责人 ID 格式错误")
			if err != nil {
				return patch, err
			}
			patch.AssigneeUserID = &value
		}
	}
	if cmd.StartDate.Present {
		patch.StartPresent = true
		if !cmd.StartDate.Null {
			value, err := parseDate(cmd.StartDate.Value)
			if err != nil {
				return patch, invalid("开始日期格式必须为 YYYY-MM-DD", err)
			}
			patch.StartDate = &value
		}
	}
	if cmd.DueDate.Present {
		patch.DuePresent = true
		if !cmd.DueDate.Null {
			value, err := parseDate(cmd.DueDate.Value)
			if err != nil {
				return patch, invalid("截止日期格式必须为 YYYY-MM-DD", err)
			}
			patch.DueDate = &value
		}
	}
	if cmd.Labels.Present {
		if cmd.Labels.Null {
			return patch, invalid("标签不能为 null", nil)
		}
		labels, err := normalizeLabels(cmd.Labels.Value)
		if err != nil {
			return patch, err
		}
		patch.Labels = &labels
	}
	return patch, err
}

func validatePatchDates(value store.Task, patch normalizedPatch) error {
	start := value.StartDate
	if patch.StartPresent {
		start = patch.StartDate
	}
	due := value.DueDate
	if patch.DuePresent {
		due = patch.DueDate
	}
	if start != nil && due != nil && start.After(*due) {
		return errInvalidPatch
	}
	return nil
}

func patchUpdates(value *store.Task, patch normalizedPatch, now time.Time) map[string]any {
	updates := map[string]any{}
	if patch.Title != nil && value.Title != *patch.Title {
		value.Title = *patch.Title
		updates["title"] = value.Title
	}
	if patch.Description != nil && value.Description != *patch.Description {
		value.Description = *patch.Description
		updates["description"] = value.Description
	}
	if patch.Status != nil && value.Status != *patch.Status {
		value.Status = *patch.Status
		setTerminalTimestamps(value, now)
		updates["status"] = value.Status
		updates["completed_at"] = value.CompletedAt
		updates["canceled_at"] = value.CanceledAt
	}
	if patch.Priority != nil && value.Priority != *patch.Priority {
		value.Priority = *patch.Priority
		updates["priority"] = value.Priority
	}
	if patch.AssigneePresent && !equalStrings(value.AssigneeUserID, patch.AssigneeUserID) {
		value.AssigneeUserID = patch.AssigneeUserID
		updates["assignee_user_id"] = patch.AssigneeUserID
		if patch.AssigneeUserID == nil {
			value.AssigneeUser = nil
		}
	}
	if patch.StartPresent && !equalDates(value.StartDate, patch.StartDate) {
		value.StartDate = patch.StartDate
		updates["start_date"] = patch.StartDate
	}
	if patch.DuePresent && !equalDates(value.DueDate, patch.DueDate) {
		value.DueDate = patch.DueDate
		updates["due_date"] = patch.DueDate
	}
	if patch.Labels != nil && !slices.Equal(value.Labels, *patch.Labels) {
		value.Labels = append(pq.StringArray{}, (*patch.Labels)...)
		updates["labels"] = value.Labels
	}
	return updates
}

func (s *Service) requireProjectAccess(ctx context.Context, projectID, accountID string) error {
	var value store.Project
	return s.db.WithContext(ctx).Where("id = ?", projectID).Where(projectAccessSQL(), projectAccessArgs(accountID)...).First(&value).Error
}
func requireProjectAccessForUpdate(tx *gorm.DB, projectID, accountID string) error {
	var value store.Project
	return tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", projectID).Where(projectAccessSQL(), projectAccessArgs(accountID)...).First(&value).Error
}

func projectAccessSQL() string {
	return `(owner_user_id = ? OR EXISTS (SELECT 1 FROM project_groups pg JOIN conversations c ON c.id = pg.conversation_id JOIN conversation_members cm ON cm.conversation_id = c.id WHERE pg.project_id = projects.id AND c.kind = ? AND c.status = ? AND cm.member_type = ? AND cm.member_id = ? AND cm.left_at IS NULL))`
}
func projectAccessArgs(accountID string) []any {
	return []any{accountID, store.ConversationKindGroup, store.ConversationStatusActive, store.ConversationMemberTypeUser, accountID}
}

func validateAssignee(tx *gorm.DB, projectID, accountID string) (store.User, error) {
	var user store.User
	err := tx.Where("id = ? AND status = ?", accountID, store.UserStatusActive).First(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return store.User{}, errInvalidAssignee
	}
	if err != nil {
		return store.User{}, err
	}
	var value store.Project
	err = tx.Where("id = ?", projectID).Where(projectAccessSQL(), projectAccessArgs(accountID)...).First(&value).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return store.User{}, errInvalidAssignee
	}
	return user, err
}

func ResolveNotificationRecipient(tx *gorm.DB, projectID, accountID string) (store.User, bool, error) {
	user, err := validateAssignee(tx, strings.TrimSpace(projectID), strings.TrimSpace(accountID))
	if errors.Is(err, errInvalidAssignee) {
		return store.User{}, false, nil
	}
	if err != nil {
		return store.User{}, false, err
	}
	return user, true, nil
}

func findForUpdate(tx *gorm.DB, projectID, taskID string) (store.Task, error) {
	var value store.Task
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Preload("AssigneeUser").Preload("CreatedByUser").Preload("Reminder").Where("id = ? AND project_id = ?", taskID, projectID).First(&value).Error
	return value, err
}
func updateProjectTimestamp(tx *gorm.DB, projectID string, now time.Time) error {
	result := tx.Model(&store.Project{}).Where("id = ? AND deleted_at IS NULL", projectID).Update("updated_at", now)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func normalizeTitle(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if err := validateText(value, "任务标题"); err != nil {
		return "", err
	}
	if count := utf8.RuneCountInString(value); count < 1 || count > 240 {
		return "", invalid("标题长度必须为 1 到 240 个字符", nil)
	}
	return value, nil
}
func normalizeLabels(values []string) (pq.StringArray, error) {
	if len(values) > 20 {
		return nil, invalid("标签不能超过 20 个", nil)
	}
	result := make(pq.StringArray, 0, len(values))
	seen := map[string]struct{}{}
	for _, raw := range values {
		value := strings.TrimSpace(raw)
		if err := validateText(value, "任务标签"); err != nil {
			return nil, err
		}
		if count := utf8.RuneCountInString(value); count < 1 || count > 32 {
			return nil, invalid("标签长度必须为 1 到 32 个字符", nil)
		}
		key := strings.ToLower(value)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, value)
	}
	return result, nil
}
func validateText(value, field string) error {
	if strings.IndexByte(value, 0) >= 0 {
		return invalid(field+"不能包含空字符", nil)
	}
	return nil
}
func parseCreateDate(field Field[string], label string) (*time.Time, error) {
	if !field.Present || field.Null {
		return nil, nil
	}
	value, err := parseDate(field.Value)
	if err != nil {
		return nil, invalid(label+"格式必须为 YYYY-MM-DD", err)
	}
	return &value, nil
}
func parseDate(raw string) (time.Time, error) {
	if len(raw) != len(DateLayout) {
		return time.Time{}, errors.New("invalid date")
	}
	value, err := time.Parse(DateLayout, raw)
	if err != nil || value.Format(DateLayout) != raw {
		return time.Time{}, errors.New("invalid date")
	}
	return value, nil
}
func validStatus(value string) bool {
	return value == StatusTodo || value == StatusInProgress || value == StatusDone || value == StatusCanceled
}
func validPriority(value int16) bool { return value >= PriorityLow && value <= PriorityHigh }
func parseUUID(raw, message string) (string, error) {
	value, err := uuid.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", invalid(message, err)
	}
	return value.String(), nil
}
func parseIDs(projectRaw, taskRaw string) (string, string, error) {
	projectID, err := parseUUID(projectRaw, "项目 ID 格式错误")
	if err != nil {
		return "", "", err
	}
	taskID, err := parseUUID(taskRaw, "任务 ID 格式错误")
	return projectID, taskID, err
}

func setTerminalTimestamps(value *store.Task, now time.Time) {
	switch value.Status {
	case StatusDone:
		value.CompletedAt = &now
		value.CanceledAt = nil
	case StatusCanceled:
		value.CanceledAt = &now
		value.CompletedAt = nil
	default:
		value.CompletedAt = nil
		value.CanceledAt = nil
	}
}
func equalStrings(left, right *string) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}
func equalDates(left, right *time.Time) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return left.Format(DateLayout) == right.Format(DateLayout)
}
func newTask(value store.Task) Task {
	labels := append([]string{}, value.Labels...)
	result := Task{ID: value.ID, ProjectID: value.ProjectID, Title: value.Title, Description: value.Description, Status: value.Status, Priority: value.Priority, Creator: newUser(value.CreatedByUser), StartDate: formatDate(value.StartDate), DueDate: formatDate(value.DueDate), Labels: labels, CompletedAt: value.CompletedAt, CanceledAt: value.CanceledAt, CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt}
	if value.AssigneeUser != nil {
		user := newUser(*value.AssigneeUser)
		result.Assignee = &user
	}
	if value.Reminder != nil {
		result.Reminder = newReminder(*value.Reminder, value.Status)
	}
	return result
}
func newUser(value store.User) UserSummary {
	return UserSummary{ID: value.ID, Name: value.Name, Nickname: value.Nickname, Avatar: value.Avatar}
}
func formatDate(value *time.Time) *string {
	if value == nil {
		return nil
	}
	formatted := value.Format(DateLayout)
	return &formatted
}

func escapeLikePattern(value string) string {
	return strings.NewReplacer("\\", "\\\\", "%", "\\%", "_", "\\_").Replace(value)
}
func decodeCursor(raw string) (*struct {
	UpdatedAt time.Time
	ID        string
}, error) {
	if raw == "" {
		return nil, errors.New("empty cursor")
	}
	content, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	var cursor listCursor
	if err := decoder.Decode(&cursor); err != nil {
		return nil, err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, errors.New("trailing data")
		}
		return nil, err
	}
	at, err := time.Parse(time.RFC3339Nano, cursor.UpdatedAt)
	if err != nil {
		return nil, err
	}
	id, err := uuid.Parse(cursor.ID)
	if err != nil {
		return nil, err
	}
	return &struct {
		UpdatedAt time.Time
		ID        string
	}{at, id.String()}, nil
}
func encodeCursor(value store.Task) (string, error) {
	content, err := json.Marshal(listCursor{value.UpdatedAt.Format(time.RFC3339Nano), value.ID})
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(content), nil
}

func mapNotFound(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return newError(CodeNotFound, "任务不存在", err)
	}
	return internalError(err)
}
func mapNotFoundOrInternal(err error) error { return mapNotFound(err) }
func mapMutationError(err error) error {
	switch {
	case errors.Is(err, errInvalidAssignee):
		return invalid("负责人不存在、不可用或无项目访问权限", err)
	case errors.Is(err, errInvalidPatch):
		return invalid(err.Error(), err)
	case errors.Is(err, errVersionConflict):
		return newError(CodeConflict, "任务已被更新，请刷新后重试", err)
	case errors.Is(err, gorm.ErrRecordNotFound):
		return newError(CodeNotFound, "任务不存在", err)
	default:
		return internalError(err)
	}
}

var _ ClientService = (*Service)(nil)
