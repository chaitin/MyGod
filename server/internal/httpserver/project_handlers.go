package httpserver

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"app/internal/store"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	defaultProjectPageLimit = 50
	maxProjectPageLimit     = 100
)

var errInvalidProjectGroup = errors.New("invalid project group")

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

type projectListResponse struct {
	PersonalProject *projectResponse  `json:"personal_project"`
	Projects        []projectResponse `json:"projects"`
	NextCursor      *string           `json:"next_cursor"`
}

type projectListCursor struct {
	UpdatedAt string `json:"updated_at"`
	ID        string `json:"id"`
}

type projectGroupResponse struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Avatar      string    `json:"avatar"`
	Status      string    `json:"status"`
	MemberCount int64     `json:"member_count"`
	CreatedAt   time.Time `json:"created_at"`
}

type projectGroupListResponse struct {
	Groups     []projectGroupResponse `json:"groups"`
	NextCursor *string                `json:"next_cursor"`
}

type projectGroupListCursor struct {
	CreatedAt      string `json:"created_at"`
	ConversationID string `json:"conversation_id"`
}

type projectGroupRow struct {
	ConversationID string    `gorm:"column:conversation_id"`
	Name           string    `gorm:"column:name"`
	Avatar         string    `gorm:"column:avatar"`
	Status         string    `gorm:"column:status"`
	CreatedAt      time.Time `gorm:"column:relation_created_at"`
}

type projectMemberResponse struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	Nickname       string   `json:"nickname"`
	Avatar         string   `json:"avatar"`
	Status         string   `json:"status"`
	DisplayName    string   `json:"display_name"`
	Role           string   `json:"role"`
	SourceGroupIDs []string `json:"source_group_ids"`
}

type projectMemberListResponse struct {
	Members    []projectMemberResponse `json:"members"`
	NextCursor *string                 `json:"next_cursor"`
}

type projectMemberListCursor struct {
	DisplayName string `json:"display_name"`
	ID          string `json:"id"`
}

type projectMemberSourceRow struct {
	ID            string `gorm:"column:id"`
	Name          string `gorm:"column:name"`
	Nickname      string `gorm:"column:nickname"`
	Avatar        string `gorm:"column:avatar"`
	Status        string `gorm:"column:status"`
	SourceGroupID string `gorm:"column:source_group_id"`
}

type createProjectRequest struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Avatar      string   `json:"avatar"`
	GroupIDs    []string `json:"group_ids"`
}

type updateProjectRequest struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
	Avatar      *string `json:"avatar"`
}

type projectTaskStatusCount struct {
	Status string
	Count  int64
}

func createPersonalProject(db *gorm.DB, user store.User, now time.Time) error {
	project := store.Project{
		ID:              uuid.NewString(),
		Name:            "个人工作区",
		Description:     "",
		Avatar:          "",
		OwnerUserID:     user.ID,
		CreatedByUserID: user.ID,
		IsPersonal:      true,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	return db.Create(&project).Error
}

func (s *Server) listProjects(c echo.Context) error {
	user, ok := currentUser(c)
	if !ok {
		return projectInternalError(c)
	}
	limit, err := parseProjectPageLimit(c.QueryParam("limit"))
	if err != nil {
		return projectInvalidRequest(c, err.Error())
	}
	cursor, err := decodeProjectListCursor(c.QueryParam("cursor"))
	if err != nil {
		return projectInvalidRequest(c, "项目游标格式错误")
	}

	query := s.db.WithContext(c.Request().Context()).
		Preload("OwnerUser").
		Where("is_personal = ?", false).
		Where(projectAccessSQL(), projectAccessArgs(user.ID)...)
	if cursor != nil {
		query = query.Where(
			"(updated_at < ?) OR (updated_at = ? AND id < ?)",
			cursor.UpdatedAt,
			cursor.UpdatedAt,
			cursor.ID,
		)
	}
	var projects []store.Project
	if err := query.Order("updated_at DESC").Order("id DESC").Limit(limit + 1).Find(&projects).Error; err != nil {
		return projectInternalError(c)
	}

	var nextCursor *string
	if len(projects) > limit {
		projects = projects[:limit]
		encoded, err := encodeProjectListCursor(projects[len(projects)-1])
		if err != nil {
			return projectInternalError(c)
		}
		nextCursor = &encoded
	}
	responses := make([]projectResponse, 0, len(projects))
	for _, project := range projects {
		role := store.ProjectRoleMember
		if project.OwnerUserID == user.ID {
			role = store.ProjectRoleOwner
		}
		response, err := s.newProjectResponse(c.Request().Context(), project, role)
		if err != nil {
			return projectInternalError(c)
		}
		responses = append(responses, response)
	}

	var personalResponse *projectResponse
	var personal store.Project
	err = s.db.WithContext(c.Request().Context()).
		Preload("OwnerUser").
		Where("owner_user_id = ? AND is_personal = ?", user.ID, true).
		First(&personal).Error
	if err == nil {
		response, responseErr := s.newProjectResponse(c.Request().Context(), personal, store.ProjectRoleOwner)
		if responseErr != nil {
			return projectInternalError(c)
		}
		personalResponse = &response
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return projectInternalError(c)
	}

	return success(c, http.StatusOK, projectListResponse{
		PersonalProject: personalResponse,
		Projects:        responses,
		NextCursor:      nextCursor,
	})
}

func (s *Server) createProject(c echo.Context) error {
	user, ok := currentUser(c)
	if !ok {
		return projectInternalError(c)
	}
	var req createProjectRequest
	if err := decodeProjectRequest(c, &req); err != nil {
		return projectInvalidRequest(c, "请求格式错误")
	}
	name, err := normalizeProjectName(req.Name)
	if err != nil {
		return projectInvalidRequest(c, err.Error())
	}
	groupIDs, err := normalizeProjectGroupIDs(req.GroupIDs)
	if err != nil {
		return projectInvalidRequest(c, err.Error())
	}

	now := time.Now().UTC()
	project := store.Project{
		ID:              uuid.NewString(),
		Name:            name,
		Description:     req.Description,
		Avatar:          req.Avatar,
		OwnerUserID:     user.ID,
		CreatedByUserID: user.ID,
		IsPersonal:      false,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	err = s.db.WithContext(c.Request().Context()).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&project).Error; err != nil {
			return err
		}
		for _, groupID := range groupIDs {
			if err := requireActiveGroupConversation(tx, groupID); err != nil {
				return err
			}
			link := store.ProjectGroup{
				ProjectID:      project.ID,
				ConversationID: groupID,
				LinkedByUserID: user.ID,
				CreatedAt:      now,
			}
			if err := tx.Create(&link).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if errors.Is(err, errInvalidProjectGroup) {
		return projectInvalidRequest(c, "群聊不存在或不可用")
	}
	if err != nil {
		return projectInternalError(c)
	}
	project.OwnerUser = user
	response, err := s.newProjectResponse(c.Request().Context(), project, store.ProjectRoleOwner)
	if err != nil {
		return projectInternalError(c)
	}
	return success(c, http.StatusCreated, response)
}

func (s *Server) getProject(c echo.Context) error {
	user, ok := currentUser(c)
	if !ok {
		return projectInternalError(c)
	}
	projectID, err := parseProjectID(c.Param("project_id"))
	if err != nil {
		return projectInvalidRequest(c, err.Error())
	}
	project, role, err := s.findAccessibleProject(c.Request().Context(), projectID, user.ID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return projectNotFound(c)
	}
	if err != nil {
		return projectInternalError(c)
	}
	response, err := s.newProjectResponse(c.Request().Context(), project, role)
	if err != nil {
		return projectInternalError(c)
	}
	return success(c, http.StatusOK, response)
}

func (s *Server) updateProject(c echo.Context) error {
	user, ok := currentUser(c)
	if !ok {
		return projectInternalError(c)
	}
	projectID, err := parseProjectID(c.Param("project_id"))
	if err != nil {
		return projectInvalidRequest(c, err.Error())
	}
	var req updateProjectRequest
	if err := decodeProjectRequest(c, &req); err != nil {
		return projectInvalidRequest(c, "请求格式错误")
	}
	updates := make(map[string]any, 3)
	if req.Name != nil {
		name, err := normalizeProjectName(*req.Name)
		if err != nil {
			return projectInvalidRequest(c, err.Error())
		}
		updates["name"] = name
	}
	if req.Description != nil {
		updates["description"] = *req.Description
	}
	if req.Avatar != nil {
		updates["avatar"] = *req.Avatar
	}

	project, role, err := s.findAccessibleProject(c.Request().Context(), projectID, user.ID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return projectNotFound(c)
	}
	if err != nil {
		return projectInternalError(c)
	}
	if role != store.ProjectRoleOwner {
		return projectForbidden(c)
	}
	if len(updates) > 0 {
		if err := s.db.WithContext(c.Request().Context()).Model(&project).Updates(updates).Error; err != nil {
			return projectInternalError(c)
		}
		project, _, err = s.findAccessibleProject(c.Request().Context(), projectID, user.ID)
		if err != nil {
			return projectInternalError(c)
		}
	}
	response, err := s.newProjectResponse(c.Request().Context(), project, store.ProjectRoleOwner)
	if err != nil {
		return projectInternalError(c)
	}
	return success(c, http.StatusOK, response)
}

func (s *Server) deleteProject(c echo.Context) error {
	user, ok := currentUser(c)
	if !ok {
		return projectInternalError(c)
	}
	projectID, err := parseProjectID(c.Param("project_id"))
	if err != nil {
		return projectInvalidRequest(c, err.Error())
	}
	project, role, err := s.findAccessibleProject(c.Request().Context(), projectID, user.ID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return projectNotFound(c)
	}
	if err != nil {
		return projectInternalError(c)
	}
	if role != store.ProjectRoleOwner {
		return projectForbidden(c)
	}
	if project.IsPersonal {
		return projectInvalidRequest(c, "个人项目不能删除")
	}
	response, err := s.newProjectResponse(c.Request().Context(), project, store.ProjectRoleOwner)
	if err != nil {
		return projectInternalError(c)
	}
	if err := s.db.WithContext(c.Request().Context()).Delete(&project).Error; err != nil {
		return projectInternalError(c)
	}
	return success(c, http.StatusOK, response)
}

func (s *Server) listProjectGroups(c echo.Context) error {
	user, ok := currentUser(c)
	if !ok {
		return projectInternalError(c)
	}
	projectID, err := parseProjectID(c.Param("project_id"))
	if err != nil {
		return projectInvalidRequest(c, err.Error())
	}
	limit, err := parseProjectPageLimit(c.QueryParam("limit"))
	if err != nil {
		return projectInvalidRequest(c, err.Error())
	}
	cursor, err := decodeProjectGroupListCursor(c.QueryParam("cursor"))
	if err != nil {
		return projectInvalidRequest(c, "群聊游标格式错误")
	}
	if _, _, err := s.findAccessibleProject(c.Request().Context(), projectID, user.ID); errors.Is(err, gorm.ErrRecordNotFound) {
		return projectNotFound(c)
	} else if err != nil {
		return projectInternalError(c)
	}

	query := s.db.WithContext(c.Request().Context()).
		Table("project_groups pg").
		Select(`
			pg.conversation_id,
			c.name,
			c.avatar,
			c.status,
			pg.created_at AS relation_created_at
		`).
		Joins("JOIN conversations c ON c.id = pg.conversation_id").
		Where("pg.project_id = ?", projectID).
		Where("c.kind = ? AND c.status = ?", store.ConversationKindGroup, store.ConversationStatusActive)
	if cursor != nil {
		query = query.Where(
			"(pg.created_at < ?) OR (pg.created_at = ? AND pg.conversation_id < ?)",
			cursor.CreatedAt,
			cursor.CreatedAt,
			cursor.ConversationID,
		)
	}
	var rows []projectGroupRow
	if err := query.
		Order("pg.created_at DESC").
		Order("pg.conversation_id DESC").
		Limit(limit + 1).
		Scan(&rows).Error; err != nil {
		return projectInternalError(c)
	}

	var nextCursor *string
	if len(rows) > limit {
		rows = rows[:limit]
		encoded, err := encodeProjectGroupListCursor(rows[len(rows)-1])
		if err != nil {
			return projectInternalError(c)
		}
		nextCursor = &encoded
	}
	groups := make([]projectGroupResponse, 0, len(rows))
	for _, row := range rows {
		memberCount, err := s.activeConversationMemberCount(c.Request().Context(), row.ConversationID)
		if err != nil {
			return projectInternalError(c)
		}
		groups = append(groups, projectGroupResponse{
			ID:          row.ConversationID,
			Name:        row.Name,
			Avatar:      row.Avatar,
			Status:      row.Status,
			MemberCount: memberCount,
			CreatedAt:   row.CreatedAt,
		})
	}
	return success(c, http.StatusOK, projectGroupListResponse{Groups: groups, NextCursor: nextCursor})
}

func (s *Server) bindProjectGroup(c echo.Context) error {
	user, ok := currentUser(c)
	if !ok {
		return projectInternalError(c)
	}
	projectID, err := parseProjectID(c.Param("project_id"))
	if err != nil {
		return projectInvalidRequest(c, err.Error())
	}
	groupID, err := parseProjectUUID(c.Param("group_id"), "群聊 ID 格式错误")
	if err != nil {
		return projectInvalidRequest(c, err.Error())
	}
	project, role, err := s.findAccessibleProject(c.Request().Context(), projectID, user.ID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return projectNotFound(c)
	}
	if err != nil {
		return projectInternalError(c)
	}
	if role != store.ProjectRoleOwner {
		return projectForbidden(c)
	}
	if project.IsPersonal {
		return projectInvalidRequest(c, "个人项目不能关联群聊")
	}

	err = s.db.WithContext(c.Request().Context()).Transaction(func(tx *gorm.DB) error {
		if err := requireActiveGroupConversation(tx, groupID); err != nil {
			return err
		}
		now := time.Now().UTC()
		link := store.ProjectGroup{
			ProjectID:      project.ID,
			ConversationID: groupID,
			LinkedByUserID: user.ID,
			CreatedAt:      now,
		}
		result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&link)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return nil
		}
		return tx.Model(&store.Project{}).
			Where("id = ?", project.ID).
			Update("updated_at", now).Error
	})
	if errors.Is(err, errInvalidProjectGroup) {
		return projectInvalidRequest(c, "群聊不存在或不可用")
	}
	if err != nil {
		return projectInternalError(c)
	}
	return success(c, http.StatusOK, map[string]any{})
}

func (s *Server) unbindProjectGroup(c echo.Context) error {
	user, ok := currentUser(c)
	if !ok {
		return projectInternalError(c)
	}
	projectID, err := parseProjectID(c.Param("project_id"))
	if err != nil {
		return projectInvalidRequest(c, err.Error())
	}
	groupID, err := parseProjectUUID(c.Param("group_id"), "群聊 ID 格式错误")
	if err != nil {
		return projectInvalidRequest(c, err.Error())
	}
	project, role, err := s.findAccessibleProject(c.Request().Context(), projectID, user.ID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return projectNotFound(c)
	}
	if err != nil {
		return projectInternalError(c)
	}
	if role != store.ProjectRoleOwner {
		return projectForbidden(c)
	}
	if project.IsPersonal {
		return projectInvalidRequest(c, "个人项目不能关联群聊")
	}

	err = s.db.WithContext(c.Request().Context()).Transaction(func(tx *gorm.DB) error {
		result := tx.Where(
			"project_id = ? AND conversation_id = ?",
			project.ID,
			groupID,
		).Delete(&store.ProjectGroup{})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return nil
		}
		return tx.Model(&store.Project{}).
			Where("id = ?", project.ID).
			Update("updated_at", time.Now().UTC()).Error
	})
	if err != nil {
		return projectInternalError(c)
	}
	return success(c, http.StatusOK, map[string]any{})
}

func (s *Server) listProjectMembers(c echo.Context) error {
	user, ok := currentUser(c)
	if !ok {
		return projectInternalError(c)
	}
	projectID, err := parseProjectID(c.Param("project_id"))
	if err != nil {
		return projectInvalidRequest(c, err.Error())
	}
	limit, err := parseProjectPageLimit(c.QueryParam("limit"))
	if err != nil {
		return projectInvalidRequest(c, err.Error())
	}
	cursor, err := decodeProjectMemberListCursor(c.QueryParam("cursor"))
	if err != nil {
		return projectInvalidRequest(c, "成员游标格式错误")
	}
	project, _, err := s.findAccessibleProject(c.Request().Context(), projectID, user.ID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return projectNotFound(c)
	}
	if err != nil {
		return projectInternalError(c)
	}

	members, err := s.loadProjectMembers(c.Request().Context(), project)
	if err != nil {
		return projectInternalError(c)
	}
	if cursor != nil {
		firstAfterCursor := sort.Search(len(members), func(index int) bool {
			member := members[index]
			return member.DisplayName > cursor.DisplayName ||
				(member.DisplayName == cursor.DisplayName && member.ID > cursor.ID)
		})
		members = members[firstAfterCursor:]
	}
	var nextCursor *string
	if len(members) > limit {
		members = members[:limit]
		encoded, err := encodeProjectMemberListCursor(members[len(members)-1])
		if err != nil {
			return projectInternalError(c)
		}
		nextCursor = &encoded
	}
	return success(c, http.StatusOK, projectMemberListResponse{Members: members, NextCursor: nextCursor})
}

func (s *Server) activeConversationMemberCount(ctx context.Context, conversationID string) (int64, error) {
	var count int64
	err := s.db.WithContext(ctx).
		Model(&store.ConversationMember{}).
		Where("conversation_id = ? AND left_at IS NULL", conversationID).
		Count(&count).Error
	return count, err
}

func (s *Server) loadProjectMembers(ctx context.Context, project store.Project) ([]projectMemberResponse, error) {
	var rows []projectMemberSourceRow
	err := s.db.WithContext(ctx).
		Table("conversation_members cm").
		Select(`
			u.id,
			u.name,
			u.nickname,
			u.avatar,
			u.status,
			cm.conversation_id AS source_group_id
		`).
		Joins("JOIN users u ON u.id = cm.member_id").
		Joins("JOIN conversations c ON c.id = cm.conversation_id").
		Joins("JOIN project_groups pg ON pg.conversation_id = c.id").
		Where("pg.project_id = ?", project.ID).
		Where("c.kind = ? AND c.status = ?", store.ConversationKindGroup, store.ConversationStatusActive).
		Where("cm.member_type = ? AND cm.left_at IS NULL", store.ConversationMemberTypeUser).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	type memberAccumulator struct {
		member  projectMemberResponse
		sources map[string]struct{}
	}
	byID := make(map[string]*memberAccumulator, len(rows)+1)
	byID[project.OwnerUserID] = &memberAccumulator{
		member: projectMemberResponse{
			ID:             project.OwnerUser.ID,
			Name:           project.OwnerUser.Name,
			Nickname:       project.OwnerUser.Nickname,
			Avatar:         project.OwnerUser.Avatar,
			Status:         project.OwnerUser.Status,
			DisplayName:    projectMemberDisplayName(project.OwnerUser.Name, project.OwnerUser.Nickname),
			Role:           store.ProjectRoleOwner,
			SourceGroupIDs: []string{},
		},
		sources: map[string]struct{}{},
	}
	for _, row := range rows {
		if row.ID == project.OwnerUserID {
			continue
		}
		accumulator, exists := byID[row.ID]
		if !exists {
			accumulator = &memberAccumulator{
				member: projectMemberResponse{
					ID:          row.ID,
					Name:        row.Name,
					Nickname:    row.Nickname,
					Avatar:      row.Avatar,
					Status:      row.Status,
					DisplayName: projectMemberDisplayName(row.Name, row.Nickname),
					Role:        store.ProjectRoleMember,
				},
				sources: make(map[string]struct{}),
			}
			byID[row.ID] = accumulator
		}
		accumulator.sources[row.SourceGroupID] = struct{}{}
	}

	members := make([]projectMemberResponse, 0, len(byID))
	for _, accumulator := range byID {
		if accumulator.member.Role != store.ProjectRoleOwner {
			accumulator.member.SourceGroupIDs = make([]string, 0, len(accumulator.sources))
			for groupID := range accumulator.sources {
				accumulator.member.SourceGroupIDs = append(accumulator.member.SourceGroupIDs, groupID)
			}
			sort.Strings(accumulator.member.SourceGroupIDs)
		}
		members = append(members, accumulator.member)
	}
	sort.Slice(members, func(i int, j int) bool {
		if members[i].DisplayName == members[j].DisplayName {
			return members[i].ID < members[j].ID
		}
		return members[i].DisplayName < members[j].DisplayName
	})
	return members, nil
}

func projectMemberDisplayName(name string, nickname string) string {
	if strings.TrimSpace(nickname) != "" {
		return nickname
	}
	return name
}

func decodeProjectRequest(c echo.Context, destination any) error {
	decoder := json.NewDecoder(c.Request().Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("请求只能包含一个 JSON 对象")
		}
		return err
	}
	return nil
}

func (s *Server) findAccessibleProject(ctx context.Context, projectID string, userID string) (store.Project, string, error) {
	var project store.Project
	err := s.db.WithContext(ctx).
		Preload("OwnerUser").
		Where("id = ?", projectID).
		Where(projectAccessSQL(), projectAccessArgs(userID)...).
		First(&project).Error
	if err != nil {
		return store.Project{}, "", err
	}
	role := store.ProjectRoleMember
	if project.OwnerUserID == userID {
		role = store.ProjectRoleOwner
	}
	return project, role, nil
}

func projectAccessSQL() string {
	return `(
		owner_user_id = ? OR EXISTS (
			SELECT 1
			FROM project_groups pg
			JOIN conversations c ON c.id = pg.conversation_id
			JOIN conversation_members cm ON cm.conversation_id = c.id
			WHERE pg.project_id = projects.id
				AND c.kind = ?
				AND c.status = ?
				AND cm.member_type = ?
				AND cm.member_id = ?
				AND cm.left_at IS NULL
		)
	)`
}

func projectAccessArgs(userID string) []any {
	return []any{
		userID,
		store.ConversationKindGroup,
		store.ConversationStatusActive,
		store.ConversationMemberTypeUser,
		userID,
	}
}

func (s *Server) newProjectResponse(ctx context.Context, project store.Project, role string) (projectResponse, error) {
	groupCount, err := s.projectGroupCount(ctx, project.ID)
	if err != nil {
		return projectResponse{}, err
	}
	memberCount, err := s.projectMemberCount(ctx, project)
	if err != nil {
		return projectResponse{}, err
	}
	taskCounts, err := s.projectTaskCounts(ctx, project.ID)
	if err != nil {
		return projectResponse{}, err
	}
	avatar := project.Avatar
	if project.IsPersonal {
		avatar = project.OwnerUser.Avatar
	}
	return projectResponse{
		ID:          project.ID,
		Name:        project.Name,
		Description: project.Description,
		Avatar:      avatar,
		IsPersonal:  project.IsPersonal,
		Owner: projectUserSummary{
			ID:       project.OwnerUser.ID,
			Name:     project.OwnerUser.Name,
			Nickname: project.OwnerUser.Nickname,
			Avatar:   project.OwnerUser.Avatar,
		},
		CurrentUserRole: role,
		GroupCount:      groupCount,
		MemberCount:     memberCount,
		TaskCounts:      taskCounts,
		CreatedAt:       project.CreatedAt,
		UpdatedAt:       project.UpdatedAt,
	}, nil
}

func (s *Server) projectGroupCount(ctx context.Context, projectID string) (int64, error) {
	var count int64
	err := s.db.WithContext(ctx).
		Table("project_groups pg").
		Joins("JOIN conversations c ON c.id = pg.conversation_id").
		Where("pg.project_id = ? AND c.kind = ? AND c.status = ?", projectID, store.ConversationKindGroup, store.ConversationStatusActive).
		Distinct("pg.conversation_id").
		Count(&count).Error
	return count, err
}

func (s *Server) projectMemberCount(ctx context.Context, project store.Project) (int64, error) {
	var memberIDs []string
	err := s.db.WithContext(ctx).
		Table("conversation_members cm").
		Select("DISTINCT cm.member_id").
		Joins("JOIN conversations c ON c.id = cm.conversation_id").
		Joins("JOIN project_groups pg ON pg.conversation_id = c.id").
		Where("pg.project_id = ?", project.ID).
		Where("c.kind = ? AND c.status = ?", store.ConversationKindGroup, store.ConversationStatusActive).
		Where("cm.member_type = ? AND cm.left_at IS NULL", store.ConversationMemberTypeUser).
		Pluck("cm.member_id", &memberIDs).Error
	if err != nil {
		return 0, err
	}
	members := map[string]struct{}{project.OwnerUserID: {}}
	for _, memberID := range memberIDs {
		members[memberID] = struct{}{}
	}
	return int64(len(members)), nil
}

func (s *Server) projectTaskCounts(ctx context.Context, projectID string) (projectTaskCountsResponse, error) {
	var rows []projectTaskStatusCount
	err := s.db.WithContext(ctx).
		Model(&store.Task{}).
		Select("status, COUNT(*) AS count").
		Where("project_id = ?", projectID).
		Group("status").
		Scan(&rows).Error
	if err != nil {
		return projectTaskCountsResponse{}, err
	}
	var counts projectTaskCountsResponse
	for _, row := range rows {
		counts.Total += row.Count
		switch row.Status {
		case store.TaskStatusTodo:
			counts.Todo = row.Count
		case store.TaskStatusInProgress:
			counts.InProgress = row.Count
		case store.TaskStatusDone:
			counts.Done = row.Count
		case store.TaskStatusCanceled:
			counts.Canceled = row.Count
		}
	}
	return counts, nil
}

func normalizeProjectName(value string) (string, error) {
	name := strings.TrimSpace(value)
	if count := utf8.RuneCountInString(name); count < 1 || count > 120 {
		return "", errors.New("项目名称长度必须为 1 到 120 个字符")
	}
	return name, nil
}

func normalizeProjectGroupIDs(values []string) ([]string, error) {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		id, err := parseProjectUUID(value, "群聊 ID 格式错误")
		if err != nil {
			return nil, err
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	return result, nil
}

func requireActiveGroupConversation(db *gorm.DB, groupID string) error {
	var count int64
	err := db.Model(&store.Conversation{}).
		Where("id = ? AND kind = ? AND status = ?", groupID, store.ConversationKindGroup, store.ConversationStatusActive).
		Count(&count).Error
	if err != nil {
		return err
	}
	if count != 1 {
		return errInvalidProjectGroup
	}
	return nil
}

func parseProjectID(value string) (string, error) {
	return parseProjectUUID(value, "项目 ID 格式错误")
}

func parseProjectUUID(value string, message string) (string, error) {
	trimmed := strings.TrimSpace(value)
	id, err := uuid.Parse(trimmed)
	if err != nil {
		return "", errors.New(message)
	}
	return id.String(), nil
}

func parseProjectPageLimit(value string) (int, error) {
	if value == "" {
		return defaultProjectPageLimit, nil
	}
	limit, err := strconv.Atoi(value)
	if err != nil || limit < 1 || limit > maxProjectPageLimit {
		return 0, errors.New("limit 必须为 1 到 100 的整数")
	}
	return limit, nil
}

func decodeProjectListCursor(value string) (*struct {
	UpdatedAt time.Time
	ID        string
}, error) {
	if value == "" {
		return nil, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return nil, err
	}
	var cursor projectListCursor
	if err := json.Unmarshal(raw, &cursor); err != nil {
		return nil, err
	}
	updatedAt, err := time.Parse(time.RFC3339Nano, cursor.UpdatedAt)
	if err != nil {
		return nil, err
	}
	id, err := parseProjectUUID(cursor.ID, "项目游标格式错误")
	if err != nil {
		return nil, err
	}
	return &struct {
		UpdatedAt time.Time
		ID        string
	}{UpdatedAt: updatedAt, ID: id}, nil
}

func encodeProjectListCursor(project store.Project) (string, error) {
	raw, err := json.Marshal(projectListCursor{
		UpdatedAt: project.UpdatedAt.Format(time.RFC3339Nano),
		ID:        project.ID,
	})
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func decodeProjectGroupListCursor(value string) (*struct {
	CreatedAt      time.Time
	ConversationID string
}, error) {
	if value == "" {
		return nil, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return nil, err
	}
	var cursor projectGroupListCursor
	if err := json.Unmarshal(raw, &cursor); err != nil {
		return nil, err
	}
	createdAt, err := time.Parse(time.RFC3339Nano, cursor.CreatedAt)
	if err != nil {
		return nil, err
	}
	conversationID, err := parseProjectUUID(cursor.ConversationID, "群聊游标格式错误")
	if err != nil {
		return nil, err
	}
	return &struct {
		CreatedAt      time.Time
		ConversationID string
	}{CreatedAt: createdAt, ConversationID: conversationID}, nil
}

func encodeProjectGroupListCursor(group projectGroupRow) (string, error) {
	raw, err := json.Marshal(projectGroupListCursor{
		CreatedAt:      group.CreatedAt.Format(time.RFC3339Nano),
		ConversationID: group.ConversationID,
	})
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func decodeProjectMemberListCursor(value string) (*projectMemberListCursor, error) {
	if value == "" {
		return nil, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return nil, err
	}
	var cursor projectMemberListCursor
	if err := json.Unmarshal(raw, &cursor); err != nil {
		return nil, err
	}
	id, err := parseProjectUUID(cursor.ID, "成员游标格式错误")
	if err != nil || cursor.DisplayName == "" {
		return nil, errors.New("成员游标格式错误")
	}
	cursor.ID = id
	return &cursor, nil
}

func encodeProjectMemberListCursor(member projectMemberResponse) (string, error) {
	raw, err := json.Marshal(projectMemberListCursor{DisplayName: member.DisplayName, ID: member.ID})
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func projectInvalidRequest(c echo.Context, message string) error {
	return failure(c, http.StatusBadRequest, "invalid_request", message)
}

func projectNotFound(c echo.Context) error {
	return failure(c, http.StatusNotFound, "not_found", "项目不存在")
}

func projectForbidden(c echo.Context) error {
	return failure(c, http.StatusForbidden, "forbidden", "无权操作项目")
}

func projectInternalError(c echo.Context) error {
	return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
}
