package entitycard

import (
	"context"
	"errors"
	"fmt"
	"strings"

	projectapp "app/internal/application/project"
	"app/internal/messageformat"
	"app/internal/store"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	maxTitleLength       = 256
	plainTextExcerptSize = 240
)

type Dependencies struct {
	DB       *gorm.DB
	Projects ProjectReader
}

type Service struct {
	db       *gorm.DB
	projects ProjectReader
}

func NewService(deps Dependencies) *Service {
	return &Service{db: deps.DB, projects: deps.Projects}
}

func (s *Service) Resolve(ctx context.Context, cmd ResolveCommand) (Card, error) {
	accountID := strings.TrimSpace(cmd.AccountID)
	if _, err := uuid.Parse(accountID); err != nil {
		return Card{}, newError(CodeInvalidRequest, "授权用户 ID 格式错误", err)
	}
	entityType := strings.ToLower(strings.TrimSpace(cmd.EntityType))
	if !supportedType(entityType) {
		return Card{}, newError(CodeInvalidRequest, "不支持的对象卡片类型", nil)
	}
	entityID := strings.TrimSpace(cmd.EntityID)
	if _, err := uuid.Parse(entityID); err != nil {
		return Card{}, newError(CodeInvalidRequest, "对象 ID 格式错误", err)
	}

	switch entityType {
	case TypeUser:
		return s.resolveUser(ctx, entityID)
	case TypeApp:
		return s.resolveApp(ctx, accountID, entityID)
	case TypeGroup:
		return s.resolveGroup(ctx, accountID, entityID)
	case TypeProject:
		return s.resolveProject(ctx, accountID, entityID)
	case TypeTask:
		return s.resolveTask(ctx, accountID, entityID)
	}
	return Card{}, internalError(errors.New("unreachable entity card type"))
}

func supportedType(entityType string) bool {
	switch entityType {
	case TypeUser, TypeApp, TypeGroup, TypeProject, TypeTask:
		return true
	default:
		return false
	}
}

func (s *Service) resolveUser(ctx context.Context, entityID string) (Card, error) {
	var user store.User
	if err := s.db.WithContext(ctx).First(&user, "id = ? AND status = ?", entityID, store.UserStatusActive).Error; err != nil {
		return Card{}, mapLookupError(err)
	}
	titleName := strings.TrimSpace(user.Nickname)
	if titleName == "" {
		titleName = strings.TrimSpace(user.Name)
	}
	if titleName == "" {
		titleName = strings.TrimSpace(user.Email)
	}
	return newCard(
		Title("联系人", titleName),
		Details(
			Detail{Label: "姓名", Value: user.Name},
			Detail{Label: "昵称", Value: user.Nickname},
			Detail{Label: "邮箱", Value: user.Email},
		),
		"/contacts/user/"+user.ID,
	), nil
}

func (s *Service) resolveApp(ctx context.Context, accountID, entityID string) (Card, error) {
	var app store.App
	err := s.db.WithContext(ctx).
		Where("id = ? AND enabled = ?", entityID, true).
		Where(
			"visibility = ? OR (visibility = ? AND creator_user_id = ?)",
			store.AppVisibilityPublic, store.AppVisibilityCreator, accountID,
		).
		First(&app).Error
	if err != nil {
		return Card{}, mapLookupError(err)
	}
	return newCard(
		Title("应用", app.Name),
		s.plainTextExcerpt(app.Description, plainTextExcerptSize),
		"/contacts/app/"+app.ID,
	), nil
}

func (s *Service) resolveGroup(ctx context.Context, accountID, entityID string) (Card, error) {
	const memberExistsSQL = "EXISTS (SELECT 1 FROM conversation_members cm WHERE cm.conversation_id = conversations.id AND cm.member_type = ? AND cm.member_id = ? AND cm.left_at IS NULL)"
	var group store.Conversation
	err := s.db.WithContext(ctx).
		Where("id = ? AND kind = ? AND status = ?", entityID, store.ConversationKindGroup, store.ConversationStatusActive).
		Where("visibility = ? OR "+memberExistsSQL, store.ConversationVisibilityPublic, store.ConversationMemberTypeUser, accountID).
		First(&group).Error
	if err != nil {
		return Card{}, mapLookupError(err)
	}
	var memberCount int64
	if err := s.db.WithContext(ctx).Model(&store.ConversationMember{}).
		Where("conversation_id = ? AND member_type = ? AND left_at IS NULL", group.ID, store.ConversationMemberTypeUser).
		Count(&memberCount).Error; err != nil {
		return Card{}, internalError(err)
	}
	return newCard(
		Title("群聊", group.Name),
		fmt.Sprintf("%d 位成员", memberCount),
		"/contacts/group/"+group.ID,
	), nil
}

func (s *Service) resolveProject(ctx context.Context, accountID, entityID string) (Card, error) {
	if s.projects == nil {
		return Card{}, internalError(errors.New("entity card project reader is not configured"))
	}
	project, err := s.projects.Get(ctx, projectapp.ProjectCommand{AccountID: accountID, ProjectID: entityID})
	if err != nil {
		return Card{}, mapProjectError(err)
	}
	description := s.plainTextExcerpt(project.Description, plainTextExcerptSize)
	if description == "" {
		description = "暂无描述"
	}
	return newCard(Title("项目", project.Name), description, "/projects/"+project.ID), nil
}

func (s *Service) resolveTask(ctx context.Context, accountID, entityID string) (Card, error) {
	var task store.Task
	if err := s.db.WithContext(ctx).Preload("AssigneeUser").First(&task, "id = ?", entityID).Error; err != nil {
		return Card{}, mapLookupError(err)
	}
	if s.projects == nil {
		return Card{}, internalError(errors.New("entity card project reader is not configured"))
	}
	project, err := s.projects.Get(ctx, projectapp.ProjectCommand{AccountID: accountID, ProjectID: task.ProjectID})
	if err != nil {
		return Card{}, mapProjectError(err)
	}
	assignee := ""
	if task.AssigneeUser != nil {
		assignee = displayName(*task.AssigneeUser)
	}
	dueDate := ""
	if task.DueDate != nil {
		dueDate = task.DueDate.Format("2006-01-02")
	}
	return newCard(
		Title("任务", task.Title),
		Details(
			Detail{Label: "状态", Value: taskStatusLabel(task.Status)},
			Detail{Label: "负责人", Value: assignee},
			Detail{Label: "截止日期", Value: dueDate},
		),
		fmt.Sprintf("/projects/%s?taskId=%s", project.ID, task.ID),
	), nil
}

func newCard(title, description, url string) Card {
	return Card{Description: strings.TrimSpace(description), Title: strings.TrimSpace(title), URL: url}
}

func Title(entityLabel, entityName string) string {
	prefix := strings.TrimSpace(entityLabel) + " - "
	name := []rune(strings.TrimSpace(entityName))
	remaining := maxTitleLength - len([]rune(prefix))
	if len(name) <= remaining {
		return prefix + string(name)
	}
	if remaining <= 1 {
		return string([]rune(prefix)[:maxTitleLength])
	}
	return prefix + string(name[:remaining-1]) + "…"
}

func Details(details ...Detail) string {
	lines := make([]string, 0, len(details))
	for _, detail := range details {
		label := strings.TrimSpace(detail.Label)
		value := strings.TrimSpace(detail.Value)
		if label == "" || value == "" {
			continue
		}
		lines = append(lines, label+": "+value)
	}
	return strings.Join(lines, "\n")
}

func (s *Service) plainTextExcerpt(source string, limit int) string {
	plainText, err := messageformat.MarkdownPlainText(strings.TrimSpace(source))
	if err != nil {
		plainText = source
	}
	plainText = strings.Join(strings.Fields(plainText), " ")
	characters := []rune(plainText)
	if len(characters) <= limit {
		return plainText
	}
	return strings.TrimSpace(string(characters[:limit])) + "…"
}

func taskStatusLabel(status string) string {
	switch status {
	case store.TaskStatusTodo:
		return "待办"
	case store.TaskStatusInProgress:
		return "进行中"
	case store.TaskStatusDone:
		return "已完成"
	case store.TaskStatusCanceled:
		return "已取消"
	default:
		return status
	}
}

func displayName(user store.User) string {
	if nickname := strings.TrimSpace(user.Nickname); nickname != "" {
		return nickname
	}
	return strings.TrimSpace(user.Name)
}

func mapLookupError(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return newError(CodeNotFound, "对象不存在或无权访问", err)
	}
	return internalError(err)
}

func mapProjectError(err error) error {
	if projectapp.ErrorCodeOf(err) == projectapp.CodeNotFound {
		return newError(CodeNotFound, "对象不存在或无权访问", err)
	}
	return internalError(err)
}

var _ Resolver = (*Service)(nil)
