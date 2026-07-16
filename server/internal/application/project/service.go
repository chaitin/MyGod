package project

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	fileapp "app/internal/application/file"
	"app/internal/media"
	"app/internal/store"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	DefaultPageLimit      = 50
	MaxPageLimit          = 100
	MaxGroupsPerProject   = 100
	PersonalWorkspaceName = "个人工作区"
	avatarContentType     = "image/webp"
	avatarSize            = 256
	maxAvatarBytes        = 1 * 1024 * 1024
)

var (
	errInvalidGroup      = errors.New("invalid project group")
	errOwnerRequired     = errors.New("project owner required")
	errPersonalDelete    = errors.New("personal project cannot be deleted")
	errPersonalGroup     = errors.New("personal project cannot link groups")
	errPersonalImmutable = errors.New("personal project name and avatar are immutable")
	errGroupCapacity     = errors.New("group project capacity exceeded")
	errGroupMembership   = errors.New("group membership required")
	errGroupAccess       = errors.New("group access denied")
	errGroupManage       = errors.New("group manager required")
	errConversationGroup = errors.New("conversation is not a group")
)

type Dependencies struct {
	DB    *gorm.DB
	Files fileapp.PublicUploader
	Now   func() time.Time
}

type Service struct {
	db    *gorm.DB
	files fileapp.PublicUploader
	now   func() time.Time
}

type projectListCursor struct {
	UpdatedAt string `json:"updated_at"`
	ID        string `json:"id"`
}

type groupListCursor struct {
	CreatedAt string `json:"created_at"`
	ID        string `json:"conversation_id"`
}

type memberListCursor struct {
	DisplayName string `json:"display_name"`
	ID          string `json:"id"`
}

type groupRow struct {
	ConversationID string    `gorm:"column:conversation_id"`
	Name           string    `gorm:"column:name"`
	Avatar         string    `gorm:"column:avatar"`
	Status         string    `gorm:"column:status"`
	CreatedAt      time.Time `gorm:"column:relation_created_at"`
}

type countRow struct {
	ID    string `gorm:"column:id"`
	Count int64  `gorm:"column:count"`
}

type taskStatusCountRow struct {
	ProjectID string `gorm:"column:project_id"`
	Status    string `gorm:"column:status"`
	Count     int64  `gorm:"column:count"`
}

func NewService(deps Dependencies) *Service {
	now := deps.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &Service{db: deps.DB, files: deps.Files, now: now}
}

func ProvisionPersonalWorkspace(tx *gorm.DB, accountID string, now time.Time) error {
	accountID = strings.TrimSpace(accountID)
	return tx.Create(&store.Project{
		ID:              uuid.NewString(),
		Name:            PersonalWorkspaceName,
		Description:     "",
		Avatar:          "",
		OwnerUserID:     accountID,
		CreatedByUserID: accountID,
		IsPersonal:      true,
		CreatedAt:       now.UTC(),
		UpdatedAt:       now.UTC(),
	}).Error
}

func (s *Service) List(ctx context.Context, cmd ListCommand) (ListResult, error) {
	accountID := strings.TrimSpace(cmd.AccountID)
	limit, err := normalizeLimit(cmd.Limit)
	if err != nil {
		return ListResult{}, err
	}
	cursor, err := decodeProjectCursor(cmd.Cursor)
	if err != nil {
		return ListResult{}, newError(CodeInvalidRequest, "项目游标格式错误", err)
	}
	keyword := strings.TrimSpace(cmd.Keyword)
	if err := validateText(keyword, "项目搜索关键词"); err != nil {
		return ListResult{}, err
	}

	query := s.db.WithContext(ctx).Where(projectAccessSQL(), projectAccessArgs(accountID)...)
	if cmd.IncludePersonal {
		query = query.Where("is_personal = ?", false)
	} else {
		query = query.Preload("OwnerUser")
	}
	if keyword != "" {
		pattern := "%" + escapeLikePattern(strings.ToLower(keyword)) + "%"
		query = query.Where("(LOWER(name) LIKE ? ESCAPE '\\' OR LOWER(description) LIKE ? ESCAPE '\\')", pattern, pattern)
	}
	if cursor != nil {
		query = query.Where("(updated_at < ?) OR (updated_at = ? AND id < ?)", cursor.UpdatedAt, cursor.UpdatedAt, cursor.ID)
	}
	var projects []store.Project
	if err := query.Order("updated_at DESC").Order("id DESC").Limit(limit + 1).Find(&projects).Error; err != nil {
		return ListResult{}, internalError(err)
	}
	var nextCursor *string
	if len(projects) > limit {
		projects = projects[:limit]
		encoded, err := encodeProjectCursor(projects[len(projects)-1])
		if err != nil {
			return ListResult{}, internalError(err)
		}
		nextCursor = &encoded
	}

	result := ListResult{Projects: make([]Summary, 0, len(projects)), NextCursor: nextCursor}
	for _, value := range projects {
		result.Projects = append(result.Projects, newSummary(value))
	}
	if !cmd.IncludePersonal {
		roles := make(map[string]string, len(projects))
		for _, value := range projects {
			roles[value.ID] = RoleMember
			if value.OwnerUserID == accountID {
				roles[value.ID] = RoleOwner
			}
		}
		details, err := s.enrich(ctx, projects, roles)
		if err != nil {
			return ListResult{}, internalError(err)
		}
		result.DetailedProjects = details
		return result, nil
	}

	var personal store.Project
	if err := s.db.WithContext(ctx).
		Preload("OwnerUser").
		Where("owner_user_id = ? AND is_personal = ?", accountID, true).
		First(&personal).Error; err != nil {
		return ListResult{}, internalError(err)
	}
	personalSummary := newSummary(personal)
	result.PersonalProject = &personalSummary
	return result, nil
}

func (s *Service) ListForConversations(ctx context.Context, conversationIDs []string) (map[string][]ConversationProject, error) {
	projects := make(map[string][]ConversationProject, len(conversationIDs))
	for _, conversationID := range conversationIDs {
		projects[conversationID] = []ConversationProject{}
	}
	if len(conversationIDs) == 0 {
		return projects, nil
	}
	var rows []struct {
		Avatar         string `gorm:"column:avatar"`
		ConversationID string `gorm:"column:conversation_id"`
		Description    string `gorm:"column:description"`
		Name           string `gorm:"column:name"`
		ProjectID      string `gorm:"column:project_id"`
	}
	if err := s.db.WithContext(ctx).
		Table("project_groups pg").
		Select("pg.conversation_id, p.id AS project_id, p.name, p.avatar, p.description").
		Joins("JOIN projects p ON p.id = pg.project_id AND p.deleted_at IS NULL").
		Where("pg.conversation_id IN ?", conversationIDs).
		Where("p.is_personal = ?", false).
		Order("pg.conversation_id ASC").
		Order("pg.created_at DESC").
		Order("p.id DESC").
		Scan(&rows).Error; err != nil {
		return nil, internalError(err)
	}
	for _, row := range rows {
		projects[row.ConversationID] = append(projects[row.ConversationID], ConversationProject{
			Avatar: row.Avatar, Description: row.Description, ID: row.ProjectID, Name: row.Name,
		})
	}
	return projects, nil
}

func (s *Service) Create(ctx context.Context, cmd CreateCommand) (Project, error) {
	accountID, err := parseUUID(cmd.AccountID, "用户 ID 格式错误")
	if err != nil {
		return Project{}, err
	}
	name, err := normalizeName(cmd.Name)
	if err != nil {
		return Project{}, err
	}
	if err := validateText(cmd.Description, "项目描述"); err != nil {
		return Project{}, err
	}
	if err := validateText(cmd.Avatar, "项目头像"); err != nil {
		return Project{}, err
	}
	if len(cmd.GroupIDs) > MaxGroupsPerProject {
		return Project{}, newError(CodeInvalidRequest, "项目最多关联 100 个群聊", nil)
	}
	groupIDs, err := normalizeGroupIDs(cmd.GroupIDs)
	if err != nil {
		return Project{}, err
	}

	var owner store.User
	if err := s.db.WithContext(ctx).First(&owner, "id = ? AND status = ?", accountID, store.UserStatusActive).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return Project{}, newError(CodeNotFound, "用户不存在", err)
		}
		return Project{}, internalError(err)
	}
	now := s.now().UTC()
	value := store.Project{
		ID:              uuid.NewString(),
		Name:            name,
		Description:     cmd.Description,
		Avatar:          cmd.Avatar,
		OwnerUserID:     accountID,
		OwnerUser:       owner,
		CreatedByUserID: accountID,
		IsPersonal:      false,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&value).Error; err != nil {
			return err
		}
		for _, groupID := range groupIDs {
			if err := requireActiveGroup(tx, groupID); err != nil {
				return err
			}
			if err := requireGroupCapacity(tx, groupID); err != nil {
				return err
			}
			if err := tx.Create(&store.ProjectGroup{
				ProjectID: value.ID, ConversationID: groupID, LinkedByUserID: accountID, CreatedAt: now,
			}).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return Project{}, mapMutationError(err)
	}
	return s.enrichOne(ctx, value, RoleOwner)
}

func (s *Service) Get(ctx context.Context, cmd ProjectCommand) (Project, error) {
	projectID, err := parseUUID(cmd.ProjectID, "项目 ID 格式错误")
	if err != nil {
		return Project{}, err
	}
	value, role, err := s.findAccessible(ctx, projectID, strings.TrimSpace(cmd.AccountID))
	if err != nil {
		return Project{}, mapLookupError(err)
	}
	return s.enrichOne(ctx, value, role)
}

func (s *Service) Update(ctx context.Context, cmd UpdateCommand) (Project, error) {
	projectID, err := parseUUID(cmd.ProjectID, "项目 ID 格式错误")
	if err != nil {
		return Project{}, err
	}
	updates := make(map[string]any, 3)
	if cmd.Name != nil {
		name, err := normalizeName(*cmd.Name)
		if err != nil {
			return Project{}, err
		}
		updates["name"] = name
	}
	if cmd.Description != nil {
		if err := validateText(*cmd.Description, "项目描述"); err != nil {
			return Project{}, err
		}
		updates["description"] = *cmd.Description
	}
	if cmd.Avatar != nil {
		if err := validateText(*cmd.Avatar, "项目头像"); err != nil {
			return Project{}, err
		}
		updates["avatar"] = *cmd.Avatar
	}

	var value store.Project
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var role string
		var err error
		value, role, err = findAccessibleForUpdate(tx, projectID, strings.TrimSpace(cmd.AccountID))
		if err != nil {
			return err
		}
		if role != RoleOwner {
			return errOwnerRequired
		}
		if value.IsPersonal {
			if name, exists := updates["name"].(string); exists {
				if name != PersonalWorkspaceName {
					return errPersonalImmutable
				}
				delete(updates, "name")
			}
			if avatar, exists := updates["avatar"].(string); exists {
				if avatar != "" {
					return errPersonalImmutable
				}
				delete(updates, "avatar")
			}
		}
		if len(updates) == 0 {
			return nil
		}
		now := s.now().UTC()
		updates["updated_at"] = now
		result := tx.Model(&store.Project{}).Where("id = ? AND deleted_at IS NULL", value.ID).Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		if field, ok := updates["name"].(string); ok {
			value.Name = field
		}
		if field, ok := updates["description"].(string); ok {
			value.Description = field
		}
		if field, ok := updates["avatar"].(string); ok {
			value.Avatar = field
		}
		value.UpdatedAt = now
		return nil
	})
	if err != nil {
		return Project{}, mapMutationError(err)
	}
	return s.enrichOne(ctx, value, RoleOwner)
}

func (s *Service) Delete(ctx context.Context, cmd ProjectCommand) (Project, error) {
	projectID, err := parseUUID(cmd.ProjectID, "项目 ID 格式错误")
	if err != nil {
		return Project{}, err
	}
	var value store.Project
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var role string
		var err error
		value, role, err = findAccessibleForUpdate(tx, projectID, strings.TrimSpace(cmd.AccountID))
		if err != nil {
			return err
		}
		if role != RoleOwner {
			return errOwnerRequired
		}
		if value.IsPersonal {
			return errPersonalDelete
		}
		taskIDs := tx.Unscoped().Model(&store.Task{}).Select("id").Where("project_id = ?", value.ID)
		if err := tx.Where("task_id IN (?)", taskIDs).Delete(&store.TaskReminder{}).Error; err != nil {
			return err
		}
		if err := tx.Where("project_id = ?", value.ID).Delete(&store.ProjectGroup{}).Error; err != nil {
			return err
		}
		result := tx.Where("id = ?", value.ID).Delete(&store.Project{})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, errPersonalDelete) {
			return Project{}, newError(CodeInvalidRequest, "个人项目不能删除", err)
		}
		return Project{}, mapMutationError(err)
	}
	return s.enrichOne(ctx, value, RoleOwner)
}

func (s *Service) ListGroups(ctx context.Context, cmd ListGroupsCommand) (ListGroupsResult, error) {
	projectID, err := parseUUID(cmd.ProjectID, "项目 ID 格式错误")
	if err != nil {
		return ListGroupsResult{}, err
	}
	limit, err := normalizeLimit(cmd.Limit)
	if err != nil {
		return ListGroupsResult{}, err
	}
	cursor, err := decodeGroupCursor(cmd.Cursor)
	if err != nil {
		return ListGroupsResult{}, newError(CodeInvalidRequest, "群聊游标格式错误", err)
	}
	if _, _, err := s.findAccessible(ctx, projectID, strings.TrimSpace(cmd.AccountID)); err != nil {
		return ListGroupsResult{}, mapLookupError(err)
	}
	query := s.db.WithContext(ctx).Table("project_groups pg").Select(`
		pg.conversation_id, c.name, c.avatar, c.status, pg.created_at AS relation_created_at
	`).Joins("JOIN conversations c ON c.id = pg.conversation_id").
		Where("pg.project_id = ?", projectID).
		Where("c.kind = ? AND c.status = ?", store.ConversationKindGroup, store.ConversationStatusActive)
	if cursor != nil {
		query = query.Where("(pg.created_at < ?) OR (pg.created_at = ? AND pg.conversation_id < ?)", cursor.CreatedAt, cursor.CreatedAt, cursor.ID)
	}
	var rows []groupRow
	if err := query.Order("pg.created_at DESC").Order("pg.conversation_id DESC").Limit(limit + 1).Scan(&rows).Error; err != nil {
		return ListGroupsResult{}, internalError(err)
	}
	var nextCursor *string
	if len(rows) > limit {
		rows = rows[:limit]
		encoded, err := encodeGroupCursor(rows[len(rows)-1])
		if err != nil {
			return ListGroupsResult{}, internalError(err)
		}
		nextCursor = &encoded
	}
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ConversationID)
	}
	counts, err := s.conversationMemberCounts(ctx, ids)
	if err != nil {
		return ListGroupsResult{}, internalError(err)
	}
	groups := make([]Group, 0, len(rows))
	for _, row := range rows {
		groups = append(groups, Group{ID: row.ConversationID, Name: row.Name, Avatar: row.Avatar, Status: row.Status, MemberCount: counts[row.ConversationID], CreatedAt: row.CreatedAt})
	}
	return ListGroupsResult{Groups: groups, NextCursor: nextCursor}, nil
}

func (s *Service) BindGroup(ctx context.Context, cmd MutateGroupCommand) (GroupMutationResult, error) {
	projectID, err := parseUUID(cmd.ProjectID, "项目 ID 格式错误")
	if err != nil {
		return GroupMutationResult{}, err
	}
	groupID, err := parseUUID(cmd.GroupID, "群聊 ID 格式错误")
	if err != nil {
		return GroupMutationResult{}, err
	}
	result := GroupMutationResult{ProjectID: projectID, GroupID: groupID}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		value, role, err := findAccessibleForUpdate(tx, projectID, strings.TrimSpace(cmd.AccountID))
		if err != nil {
			return err
		}
		if role != RoleOwner && !cmd.AllowProjectMember {
			return errOwnerRequired
		}
		if value.IsPersonal {
			return errPersonalGroup
		}
		if cmd.RequireGroupManager {
			if err := requireGroupActor(tx, groupID, strings.TrimSpace(cmd.AccountID), true); err != nil {
				return err
			}
		} else {
			if err := requireActiveGroup(tx, groupID); err != nil {
				return err
			}
			if cmd.RequireGroupMembership {
				if err := requireGroupMember(tx, groupID, strings.TrimSpace(cmd.AccountID)); err != nil {
					return err
				}
			}
		}
		var count int64
		if err := tx.Model(&store.ProjectGroup{}).Where("project_id = ? AND conversation_id = ?", value.ID, groupID).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			result.AlreadyLinked = true
			return nil
		}
		if err := requireGroupCapacity(tx, groupID); err != nil {
			return err
		}
		now := s.now().UTC()
		created := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&store.ProjectGroup{ProjectID: value.ID, ConversationID: groupID, LinkedByUserID: strings.TrimSpace(cmd.AccountID), CreatedAt: now})
		if created.Error != nil {
			return created.Error
		}
		if created.RowsAffected == 0 {
			result.AlreadyLinked = true
			return nil
		}
		updated := tx.Model(&store.Project{}).Where("id = ?", value.ID).Update("updated_at", now)
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return nil
	})
	if err != nil {
		return GroupMutationResult{}, mapGroupMutationError(err)
	}
	return result, nil
}

func (s *Service) UnbindGroup(ctx context.Context, cmd MutateGroupCommand) error {
	projectID, err := parseUUID(cmd.ProjectID, "项目 ID 格式错误")
	if err != nil {
		return err
	}
	groupID, err := parseUUID(cmd.GroupID, "群聊 ID 格式错误")
	if err != nil {
		return err
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		value, role, err := findAccessibleForUpdate(tx, projectID, strings.TrimSpace(cmd.AccountID))
		if err != nil {
			return err
		}
		if role != RoleOwner && !cmd.AllowProjectMember {
			return errOwnerRequired
		}
		if value.IsPersonal {
			return errPersonalGroup
		}
		if cmd.RequireGroupManager {
			if err := requireGroupActor(tx, groupID, strings.TrimSpace(cmd.AccountID), true); err != nil {
				return err
			}
		} else if cmd.RequireGroupMembership {
			if err := requireGroupMember(tx, groupID, strings.TrimSpace(cmd.AccountID)); err != nil {
				return err
			}
		}
		deleted := tx.Where("project_id = ? AND conversation_id = ?", value.ID, groupID).Delete(&store.ProjectGroup{})
		if deleted.Error != nil {
			return deleted.Error
		}
		if deleted.RowsAffected == 0 {
			return nil
		}
		updated := tx.Model(&store.Project{}).Where("id = ?", value.ID).Update("updated_at", s.now().UTC())
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return nil
	})
	if err != nil {
		return mapGroupMutationError(err)
	}
	return nil
}

func (s *Service) ListMembers(ctx context.Context, cmd ListMembersCommand) (ListMembersResult, error) {
	projectID, err := parseUUID(cmd.ProjectID, "项目 ID 格式错误")
	if err != nil {
		return ListMembersResult{}, err
	}
	limit, err := normalizeLimit(cmd.Limit)
	if err != nil {
		return ListMembersResult{}, err
	}
	cursor, err := decodeMemberCursor(cmd.Cursor)
	if err != nil {
		return ListMembersResult{}, newError(CodeInvalidRequest, "成员游标格式错误", err)
	}
	value, _, err := s.findAccessible(ctx, projectID, strings.TrimSpace(cmd.AccountID))
	if err != nil {
		return ListMembersResult{}, mapLookupError(err)
	}
	members, err := s.loadMemberPage(ctx, value, cursor, limit+1)
	if err != nil {
		return ListMembersResult{}, internalError(err)
	}
	var nextCursor *string
	if len(members) > limit {
		members = members[:limit]
		encoded, err := encodeMemberCursor(members[len(members)-1])
		if err != nil {
			return ListMembersResult{}, internalError(err)
		}
		nextCursor = &encoded
	}
	if err := s.loadMemberSources(ctx, value, members); err != nil {
		return ListMembersResult{}, internalError(err)
	}
	return ListMembersResult{Members: members, NextCursor: nextCursor}, nil
}

func (s *Service) UploadAvatar(ctx context.Context, cmd UploadAvatarCommand) (Project, error) {
	projectID, err := parseUUID(cmd.ProjectID, "项目 ID 格式错误")
	if err != nil {
		return Project{}, err
	}
	value, role, err := s.findAccessible(ctx, projectID, strings.TrimSpace(cmd.AccountID))
	if err != nil {
		return Project{}, mapLookupError(err)
	}
	if role != RoleOwner {
		return Project{}, newError(CodeForbidden, "无权执行此操作", errOwnerRequired)
	}
	if value.IsPersonal {
		return Project{}, newError(CodeInvalidRequest, "个人工作区头像不能修改", errPersonalImmutable)
	}
	if cmd.Size > maxAvatarBytes {
		return Project{}, newError(CodeRequestTooLarge, "项目头像文件不能超过 1MiB", nil)
	}
	if cmd.Size == 0 || cmd.Content == nil {
		return Project{}, newError(CodeInvalidRequest, "项目头像文件不能为空", nil)
	}
	content, err := io.ReadAll(io.LimitReader(cmd.Content, maxAvatarBytes+1))
	if err != nil {
		return Project{}, newError(CodeInvalidRequest, "读取项目头像失败", err)
	}
	if len(content) > maxAvatarBytes {
		return Project{}, newError(CodeRequestTooLarge, "项目头像文件不能超过 1MiB", nil)
	}
	if len(content) == 0 {
		return Project{}, newError(CodeInvalidRequest, "项目头像文件不能为空", nil)
	}
	width, height, err := media.WebPDimensions(content)
	if err != nil || width != avatarSize || height != avatarSize {
		return Project{}, newError(CodeInvalidRequest, "项目头像必须是 256x256 的 WebP 图片", err)
	}
	if s.files == nil {
		return Project{}, newError(CodeInternal, "项目头像存储未配置", nil)
	}
	key := fmt.Sprintf("avatars/projects/%s/%s.webp", value.ID, uuid.NewString())
	uploaded, err := s.files.UploadPublic(ctx, fileapp.UploadPublicCommand{
		ObjectKey:   key,
		Content:     bytes.NewReader(content),
		ContentType: avatarContentType,
		SizeBytes:   int64(len(content)),
	})
	if err != nil {
		if fileapp.ErrorCodeOf(err) == fileapp.CodeStorageUnavailable {
			return Project{}, newError(CodeInternal, "项目头像存储未配置", err)
		}
		return Project{}, newError(CodeInternal, "上传项目头像失败", err)
	}
	avatarURL := uploaded.URL
	if strings.TrimSpace(avatarURL) == "" {
		return Project{}, newError(CodeInternal, "项目头像存储未配置", nil)
	}
	now := s.now().UTC()
	updated := s.db.WithContext(ctx).Model(&store.Project{}).
		Where("id = ? AND owner_user_id = ? AND is_personal = ?", value.ID, strings.TrimSpace(cmd.AccountID), false).
		Updates(map[string]any{"avatar": avatarURL, "updated_at": now})
	if updated.Error != nil {
		return Project{}, internalError(updated.Error)
	}
	if updated.RowsAffected == 0 {
		return Project{}, newError(CodeNotFound, "项目不存在", gorm.ErrRecordNotFound)
	}
	value.Avatar = avatarURL
	value.UpdatedAt = now
	return s.enrichOne(ctx, value, RoleOwner)
}

func (s *Service) findAccessible(ctx context.Context, projectID, accountID string) (store.Project, string, error) {
	var value store.Project
	err := s.db.WithContext(ctx).Preload("OwnerUser").Where("id = ?", projectID).
		Where(projectAccessSQL(), projectAccessArgs(accountID)...).First(&value).Error
	if err != nil {
		return store.Project{}, "", err
	}
	role := RoleMember
	if value.OwnerUserID == accountID {
		role = RoleOwner
	}
	return value, role, nil
}

func findAccessibleForUpdate(tx *gorm.DB, projectID, accountID string) (store.Project, string, error) {
	var value store.Project
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", projectID).
		Where(projectAccessSQL(), projectAccessArgs(accountID)...).First(&value).Error
	if err != nil {
		return store.Project{}, "", err
	}
	if err := tx.First(&value.OwnerUser, "id = ?", value.OwnerUserID).Error; err != nil {
		return store.Project{}, "", err
	}
	role := RoleMember
	if value.OwnerUserID == accountID {
		role = RoleOwner
	}
	return value, role, nil
}

func projectAccessSQL() string {
	return `(owner_user_id = ? OR EXISTS (
		SELECT 1 FROM project_groups pg
		JOIN conversations c ON c.id = pg.conversation_id
		JOIN conversation_members cm ON cm.conversation_id = c.id
		WHERE pg.project_id = projects.id AND c.kind = ? AND c.status = ?
			AND cm.member_type = ? AND cm.member_id = ? AND cm.left_at IS NULL
	))`
}

func projectAccessArgs(accountID string) []any {
	return []any{accountID, store.ConversationKindGroup, store.ConversationStatusActive, store.ConversationMemberTypeUser, accountID}
}

func (s *Service) enrichOne(ctx context.Context, value store.Project, role string) (Project, error) {
	values, err := s.enrich(ctx, []store.Project{value}, map[string]string{value.ID: role})
	if err != nil {
		return Project{}, internalError(err)
	}
	return values[0], nil
}

func (s *Service) enrich(ctx context.Context, projects []store.Project, roles map[string]string) ([]Project, error) {
	result := make([]Project, 0, len(projects))
	if len(projects) == 0 {
		return result, nil
	}
	ids := make([]string, 0, len(projects))
	for _, value := range projects {
		ids = append(ids, value.ID)
	}
	groupCounts, err := s.groupCounts(ctx, ids)
	if err != nil {
		return nil, err
	}
	memberCounts, err := s.memberCounts(ctx, ids)
	if err != nil {
		return nil, err
	}
	taskCounts, err := s.taskCounts(ctx, ids)
	if err != nil {
		return nil, err
	}
	for _, value := range projects {
		avatar := value.Avatar
		if value.IsPersonal {
			avatar = value.OwnerUser.Avatar
		}
		result = append(result, Project{
			ID: value.ID, Name: value.Name, Description: value.Description, Avatar: avatar, IsPersonal: value.IsPersonal,
			Owner:           UserSummary{ID: value.OwnerUser.ID, Name: value.OwnerUser.Name, Nickname: value.OwnerUser.Nickname, Avatar: value.OwnerUser.Avatar},
			CurrentUserRole: roles[value.ID], GroupCount: groupCounts[value.ID], MemberCount: memberCounts[value.ID] + 1,
			TaskCounts: taskCounts[value.ID], CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt,
		})
	}
	return result, nil
}

func newSummary(value store.Project) Summary {
	avatar := value.Avatar
	if value.IsPersonal {
		avatar = value.OwnerUser.Avatar
	}
	return Summary{ID: value.ID, Name: value.Name, Description: value.Description, Avatar: avatar, IsPersonal: value.IsPersonal, UpdatedAt: value.UpdatedAt}
}

func (s *Service) groupCounts(ctx context.Context, ids []string) (map[string]int64, error) {
	counts := make(map[string]int64, len(ids))
	var rows []countRow
	err := s.db.WithContext(ctx).Table("project_groups pg").Select("pg.project_id AS id, COUNT(DISTINCT pg.conversation_id) AS count").
		Joins("JOIN conversations c ON c.id = pg.conversation_id").Where("pg.project_id IN ?", ids).
		Where("c.kind = ? AND c.status = ?", store.ConversationKindGroup, store.ConversationStatusActive).Group("pg.project_id").Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		counts[row.ID] = row.Count
	}
	return counts, nil
}

func (s *Service) memberCounts(ctx context.Context, ids []string) (map[string]int64, error) {
	counts := make(map[string]int64, len(ids))
	var rows []countRow
	err := s.db.WithContext(ctx).Table("conversation_members cm").Select("pg.project_id AS id, COUNT(DISTINCT cm.member_id) AS count").
		Joins("JOIN conversations c ON c.id = cm.conversation_id").Joins("JOIN project_groups pg ON pg.conversation_id = c.id").
		Joins("JOIN projects p ON p.id = pg.project_id").Where("pg.project_id IN ?", ids).
		Where("c.kind = ? AND c.status = ?", store.ConversationKindGroup, store.ConversationStatusActive).
		Where("cm.member_type = ? AND cm.left_at IS NULL", store.ConversationMemberTypeUser).Where("cm.member_id <> p.owner_user_id").
		Group("pg.project_id").Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		counts[row.ID] = row.Count
	}
	return counts, nil
}

func (s *Service) taskCounts(ctx context.Context, ids []string) (map[string]TaskCounts, error) {
	counts := make(map[string]TaskCounts, len(ids))
	var rows []taskStatusCountRow
	err := s.db.WithContext(ctx).Model(&store.Task{}).Select("project_id, status, COUNT(*) AS count").Where("project_id IN ?", ids).
		Group("project_id, status").Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		value := counts[row.ProjectID]
		value.Total += row.Count
		switch row.Status {
		case store.TaskStatusTodo:
			value.Todo = row.Count
		case store.TaskStatusInProgress:
			value.InProgress = row.Count
		case store.TaskStatusDone:
			value.Done = row.Count
		case store.TaskStatusCanceled:
			value.Canceled = row.Count
		}
		counts[row.ProjectID] = value
	}
	return counts, nil
}

func (s *Service) conversationMemberCounts(ctx context.Context, ids []string) (map[string]int64, error) {
	counts := make(map[string]int64, len(ids))
	if len(ids) == 0 {
		return counts, nil
	}
	var rows []countRow
	err := s.db.WithContext(ctx).Model(&store.ConversationMember{}).Select("conversation_id AS id, COUNT(*) AS count").
		Where("conversation_id IN ? AND left_at IS NULL", ids).Group("conversation_id").Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		counts[row.ID] = row.Count
	}
	return counts, nil
}

func (s *Service) loadMemberPage(ctx context.Context, value store.Project, cursor *memberListCursor, limit int) ([]Member, error) {
	query := `SELECT member_page.id, member_page.name, member_page.nickname, member_page.email, member_page.avatar, member_page.status, member_page.display_name
		FROM (SELECT member_base.id, member_base.name, member_base.nickname, member_base.email, member_base.avatar, member_base.status,
		CASE WHEN TRIM(member_base.nickname) <> '' THEN member_base.nickname ELSE member_base.name END AS display_name
		FROM (SELECT u.id, u.name, u.nickname, u.email, u.avatar, u.status FROM users u WHERE u.id = ?
		UNION SELECT u.id, u.name, u.nickname, u.email, u.avatar, u.status FROM users u
		JOIN conversation_members cm ON cm.member_id = u.id JOIN conversations c ON c.id = cm.conversation_id
		JOIN project_groups pg ON pg.conversation_id = c.id WHERE pg.project_id = ? AND c.kind = ? AND c.status = ?
		AND cm.member_type = ? AND cm.left_at IS NULL) member_base) member_page`
	args := []any{value.OwnerUserID, value.ID, store.ConversationKindGroup, store.ConversationStatusActive, store.ConversationMemberTypeUser}
	if cursor != nil {
		query += ` WHERE member_page.display_name > ? OR (member_page.display_name = ? AND member_page.id > ?)`
		args = append(args, cursor.DisplayName, cursor.DisplayName, cursor.ID)
	}
	query += ` ORDER BY member_page.display_name ASC, member_page.id ASC LIMIT ?`
	args = append(args, limit)
	var rows []struct{ ID, Name, Nickname, Email, Avatar, Status, DisplayName string }
	if err := s.db.WithContext(ctx).Raw(query, args...).Scan(&rows).Error; err != nil {
		return nil, err
	}
	members := make([]Member, 0, len(rows))
	for _, row := range rows {
		role := RoleMember
		if row.ID == value.OwnerUserID {
			role = RoleOwner
		}
		members = append(members, Member{ID: row.ID, Name: row.Name, Nickname: row.Nickname, Email: row.Email, Avatar: row.Avatar, Status: row.Status, DisplayName: row.DisplayName, Role: role, SourceGroupIDs: []string{}})
	}
	return members, nil
}

func (s *Service) loadMemberSources(ctx context.Context, value store.Project, members []Member) error {
	ids := make([]string, 0, len(members))
	byID := make(map[string]*Member, len(members))
	for index := range members {
		if members[index].ID == value.OwnerUserID {
			continue
		}
		ids = append(ids, members[index].ID)
		byID[members[index].ID] = &members[index]
	}
	if len(ids) == 0 {
		return nil
	}
	var rows []struct {
		MemberID string `gorm:"column:member_id"`
		GroupID  string `gorm:"column:source_group_id"`
	}
	err := s.db.WithContext(ctx).Table("conversation_members cm").Select("cm.member_id, cm.conversation_id AS source_group_id").
		Joins("JOIN conversations c ON c.id = cm.conversation_id").Joins("JOIN project_groups pg ON pg.conversation_id = c.id").
		Where("pg.project_id = ?", value.ID).Where("c.kind = ? AND c.status = ?", store.ConversationKindGroup, store.ConversationStatusActive).
		Where("cm.member_type = ? AND cm.left_at IS NULL", store.ConversationMemberTypeUser).Where("cm.member_id IN ?", ids).Scan(&rows).Error
	if err != nil {
		return err
	}
	sets := make(map[string]map[string]struct{}, len(ids))
	for _, row := range rows {
		if sets[row.MemberID] == nil {
			sets[row.MemberID] = map[string]struct{}{}
		}
		sets[row.MemberID][row.GroupID] = struct{}{}
	}
	for id, groups := range sets {
		for groupID := range groups {
			byID[id].SourceGroupIDs = append(byID[id].SourceGroupIDs, groupID)
		}
		sort.Strings(byID[id].SourceGroupIDs)
	}
	return nil
}

func requireActiveGroup(tx *gorm.DB, groupID string) error {
	var value store.Conversation
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id", "kind", "status").First(&value, "id = ?", groupID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return errInvalidGroup
	}
	if err != nil {
		return err
	}
	if value.Kind != store.ConversationKindGroup || value.Status != store.ConversationStatusActive {
		return errInvalidGroup
	}
	return nil
}

func requireGroupMember(tx *gorm.DB, groupID, accountID string) error {
	var count int64
	err := tx.Model(&store.ConversationMember{}).Where("conversation_id = ? AND member_type = ? AND member_id = ? AND left_at IS NULL", groupID, store.ConversationMemberTypeUser, accountID).Count(&count).Error
	if err != nil {
		return err
	}
	if count == 0 {
		return errGroupMembership
	}
	return nil
}

func requireGroupActor(tx *gorm.DB, groupID, accountID string, manager bool) error {
	var conversation store.Conversation
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id", "kind", "status").First(&conversation, "id = ?", groupID).Error; err != nil {
		return err
	}
	if conversation.Kind != store.ConversationKindGroup {
		return errConversationGroup
	}
	if conversation.Status != store.ConversationStatusActive {
		return errGroupAccess
	}
	var member store.ConversationMember
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where(
		"conversation_id = ? AND member_type = ? AND member_id = ? AND left_at IS NULL",
		groupID, store.ConversationMemberTypeUser, accountID,
	).First(&member).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return errGroupAccess
	}
	if err != nil {
		return err
	}
	if manager && member.Role != store.ConversationMemberRoleOwner && member.Role != store.ConversationMemberRoleAdmin {
		return errGroupManage
	}
	return nil
}

func requireGroupCapacity(tx *gorm.DB, groupID string) error {
	var count int64
	if err := tx.Model(&store.ProjectGroup{}).Where("conversation_id = ?", groupID).Count(&count).Error; err != nil {
		return err
	}
	if count >= MaxGroupsPerProject {
		return errGroupCapacity
	}
	return nil
}

func normalizeName(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if err := validateText(value, "项目名称"); err != nil {
		return "", err
	}
	if count := utf8.RuneCountInString(value); count < 1 || count > 120 {
		return "", newError(CodeInvalidRequest, "项目名称长度必须为 1 到 120 个字符", nil)
	}
	return value, nil
}

func validateText(value, field string) error {
	if strings.IndexByte(value, 0) >= 0 {
		return newError(CodeInvalidRequest, field+"不能包含空字符", nil)
	}
	return nil
}

func normalizeGroupIDs(values []string) ([]string, error) {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, raw := range values {
		id, err := parseUUID(raw, "群聊 ID 格式错误")
		if err != nil {
			return nil, err
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	sort.Strings(result)
	return result, nil
}

func parseUUID(raw, message string) (string, error) {
	id, err := uuid.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", newError(CodeInvalidRequest, message, err)
	}
	return id.String(), nil
}

func normalizeLimit(limit int) (int, error) {
	if limit == 0 {
		return DefaultPageLimit, nil
	}
	if limit < 1 || limit > MaxPageLimit {
		return 0, newError(CodeInvalidRequest, "limit 必须为 1 到 100 的整数", nil)
	}
	return limit, nil
}

func decodeProjectCursor(raw string) (*struct {
	UpdatedAt time.Time
	ID        string
}, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	content, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return nil, err
	}
	var cursor projectListCursor
	if err := json.Unmarshal(content, &cursor); err != nil {
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

func encodeProjectCursor(value store.Project) (string, error) {
	content, err := json.Marshal(projectListCursor{value.UpdatedAt.Format(time.RFC3339Nano), value.ID})
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(content), nil
}

func decodeGroupCursor(raw string) (*struct {
	CreatedAt time.Time
	ID        string
}, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	content, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return nil, err
	}
	var cursor groupListCursor
	if err := json.Unmarshal(content, &cursor); err != nil {
		return nil, err
	}
	at, err := time.Parse(time.RFC3339Nano, cursor.CreatedAt)
	if err != nil {
		return nil, err
	}
	id, err := uuid.Parse(cursor.ID)
	if err != nil {
		return nil, err
	}
	return &struct {
		CreatedAt time.Time
		ID        string
	}{at, id.String()}, nil
}

func encodeGroupCursor(value groupRow) (string, error) {
	content, err := json.Marshal(groupListCursor{value.CreatedAt.Format(time.RFC3339Nano), value.ConversationID})
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(content), nil
}

func decodeMemberCursor(raw string) (*memberListCursor, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	content, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	var cursor memberListCursor
	if err := decoder.Decode(&cursor); err != nil {
		return nil, err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, errors.New("member cursor contains trailing data")
		}
		return nil, err
	}
	if cursor.DisplayName == "" {
		return nil, errors.New("empty display name")
	}
	if strings.IndexByte(cursor.DisplayName, 0) >= 0 {
		return nil, errors.New("invalid display name")
	}
	id, err := uuid.Parse(cursor.ID)
	if err != nil {
		return nil, err
	}
	cursor.ID = id.String()
	return &cursor, nil
}

func encodeMemberCursor(value Member) (string, error) {
	content, err := json.Marshal(memberListCursor{value.DisplayName, value.ID})
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(content), nil
}

func escapeLikePattern(value string) string {
	value = strings.ReplaceAll(value, "\\", "\\\\")
	value = strings.ReplaceAll(value, "%", "\\%")
	return strings.ReplaceAll(value, "_", "\\_")
}

func mapLookupError(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return newError(CodeNotFound, "项目不存在", err)
	}
	return internalError(err)
}

func mapMutationError(err error) error {
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return newError(CodeNotFound, "项目不存在", err)
	case errors.Is(err, errOwnerRequired):
		return newError(CodeForbidden, "无权执行此操作", err)
	case errors.Is(err, errPersonalImmutable):
		return newError(CodeInvalidRequest, "个人项目名称和头像不能修改", err)
	case errors.Is(err, errInvalidGroup):
		return newError(CodeInvalidRequest, "群聊不存在或不可用", err)
	case errors.Is(err, errGroupCapacity):
		return newError(CodeConflict, "群聊关联项目数量已达上限", err)
	default:
		return internalError(err)
	}
}

func mapGroupMutationError(err error) error {
	if errors.Is(err, errPersonalGroup) {
		return newError(CodeInvalidRequest, "个人项目不能关联群聊", err)
	}
	if errors.Is(err, errGroupMembership) {
		return newError(CodeForbidden, "授权用户不是目标群聊的有效成员", err)
	}
	if errors.Is(err, errConversationGroup) {
		return newError(CodeInvalidRequest, "只能关联群聊", err)
	}
	if errors.Is(err, errGroupAccess) {
		return newError(CodeForbidden, "无权访问会话", err)
	}
	if errors.Is(err, errGroupManage) {
		return newError(CodeForbidden, "只有群主或群管理员可以管理群聊项目", err)
	}
	return mapMutationError(err)
}

var _ ClientService = (*Service)(nil)
var _ ConversationReader = (*Service)(nil)
