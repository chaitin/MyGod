package httpserver

import (
	"bytes"
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
	personalProjectName     = "个人工作区"
)

var (
	errInvalidProjectGroup         = errors.New("invalid project group")
	errProjectOwnerRequired        = errors.New("project owner required")
	errPersonalProjectDelete       = errors.New("personal project cannot be deleted")
	errPersonalProjectGroup        = errors.New("personal project cannot link groups")
	errPersonalProjectImmutable    = errors.New("personal project name and avatar are immutable")
	errGroupConversationProjectCap = errors.New("group conversation project cap exceeded")
)

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
	PersonalProject projectResponse   `json:"personal_project"`
	Projects        []projectResponse `json:"projects"`
	NextCursor      *string           `json:"next_cursor" extensions:"x-nullable"`
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
	NextCursor *string                `json:"next_cursor" extensions:"x-nullable"`
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
	NextCursor *string                 `json:"next_cursor" extensions:"x-nullable"`
}

// projectGroupMutationResponse documents the empty data object returned by group link mutations.
type projectGroupMutationResponse struct{}

type projectMemberListCursor struct {
	DisplayName string `json:"display_name"`
	ID          string `json:"id"`
}

type projectMemberSourceRow struct {
	MemberID      string `gorm:"column:member_id"`
	SourceGroupID string `gorm:"column:source_group_id"`
}

type projectCountRow struct {
	ProjectID string `gorm:"column:project_id"`
	Count     int64  `gorm:"column:count"`
}

type conversationCountRow struct {
	ConversationID string `gorm:"column:conversation_id"`
	Count          int64  `gorm:"column:count"`
}

type createProjectRequest struct {
	Name        projectOptionalString      `json:"name" swaggertype:"string" binding:"required" example:"新版发布"`
	Description projectOptionalString      `json:"description" swaggertype:"string" example:"协调新版发布工作"`
	Avatar      projectOptionalString      `json:"avatar" swaggertype:"string" example:"/assets/avatars/projects/release.webp"`
	GroupIDs    projectOptionalStringSlice `json:"group_ids" swaggertype:"array,string" example:"7f8d8b84-6d2c-4b12-9a8a-019a7e2787d4"`
}

type updateProjectRequest struct {
	Name        projectOptionalString `json:"name" swaggertype:"string" example:"新版发布"`
	Description projectOptionalString `json:"description" swaggertype:"string" example:"协调新版发布工作"`
	Avatar      projectOptionalString `json:"avatar" swaggertype:"string" example:"/assets/avatars/projects/release.webp"`
}

type projectOptionalString struct {
	Present bool
	Value   string
}

type projectOptionalStringSlice struct {
	Present bool
	Value   []string
}

type projectTaskStatusCount struct {
	ProjectID string `gorm:"column:project_id"`
	Status    string `gorm:"column:status"`
	Count     int64  `gorm:"column:count"`
}

func createPersonalProject(db *gorm.DB, user store.User, now time.Time) error {
	project := store.Project{
		ID:              uuid.NewString(),
		Name:            personalProjectName,
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

// listProjects godoc
//
// @Summary 列出项目
// @Description 获取当前用户可访问的项目及个人项目，按更新时间倒序分页。
// @Tags 客户端项目
// @Produce json
// @Param limit query int false "每页数量，默认 50，最大 100"
// @Param cursor query string false "项目分页游标"
// @Success 200 {object} successEnvelope{data=projectListResponse}
// @Failure 400 {object} errorEnvelope
// @Failure 401 {object} errorEnvelope
// @Failure 500 {object} errorEnvelope
// @Security UserSession
// @Router /api/client/projects [get]
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
	roles := make(map[string]string, len(projects)+1)
	for _, project := range projects {
		role := store.ProjectRoleMember
		if project.OwnerUserID == user.ID {
			role = store.ProjectRoleOwner
		}
		roles[project.ID] = role
	}

	allProjects := append([]store.Project(nil), projects...)
	var personal store.Project
	err = s.db.WithContext(c.Request().Context()).
		Preload("OwnerUser").
		Where("owner_user_id = ? AND is_personal = ?", user.ID, true).
		First(&personal).Error
	if err != nil {
		return projectInternalError(c)
	}
	roles[personal.ID] = store.ProjectRoleOwner
	allProjects = append(allProjects, personal)
	allResponses, err := s.newProjectResponses(c.Request().Context(), allProjects, roles)
	if err != nil {
		return projectInternalError(c)
	}
	responses := allResponses[:len(projects)]
	personalResponse := allResponses[len(projects)]

	return success(c, http.StatusOK, projectListResponse{
		PersonalProject: personalResponse,
		Projects:        responses,
		NextCursor:      nextCursor,
	})
}

// createProject godoc
//
// @Summary 创建项目
// @Description 创建普通项目，可同时关联当前可用的群聊。
// @Tags 客户端项目
// @Accept json
// @Produce json
// @Param body body createProjectRequest true "项目信息"
// @Success 201 {object} successEnvelope{data=projectResponse}
// @Failure 400 {object} errorEnvelope
// @Failure 401 {object} errorEnvelope
// @Failure 500 {object} errorEnvelope
// @Security UserSession
// @Router /api/client/projects [post]
func (s *Server) createProject(c echo.Context) error {
	user, ok := currentUser(c)
	if !ok {
		return projectInternalError(c)
	}
	var req createProjectRequest
	if err := decodeProjectRequest(c, &req); err != nil {
		return projectInvalidRequest(c, "请求格式错误")
	}
	if len(req.GroupIDs.Value) > maxGroupConversationProjects {
		return projectInvalidRequest(c, "项目最多关联 100 个群聊")
	}
	name, err := normalizeProjectName(req.Name.Value)
	if err != nil {
		return projectInvalidRequest(c, err.Error())
	}
	groupIDs, err := normalizeProjectGroupIDs(req.GroupIDs.Value)
	if err != nil {
		return projectInvalidRequest(c, err.Error())
	}

	now := time.Now().UTC()
	project := store.Project{
		ID:              uuid.NewString(),
		Name:            name,
		Description:     req.Description.Value,
		Avatar:          req.Avatar.Value,
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
			if err := requireActiveGroupConversationForUpdate(tx, groupID); err != nil {
				return err
			}
			if err := requireGroupConversationProjectCapacity(tx, groupID); err != nil {
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
		return projectMutationFailure(c, err)
	}
	project.OwnerUser = user
	response, err := s.newProjectResponse(c.Request().Context(), project, store.ProjectRoleOwner)
	if err != nil {
		return projectInternalError(c)
	}
	return success(c, http.StatusCreated, response)
}

// getProject godoc
//
// @Summary 获取项目
// @Description 获取当前用户可访问的项目详情和任务统计。
// @Tags 客户端项目
// @Produce json
// @Param project_id path string true "项目 ID"
// @Success 200 {object} successEnvelope{data=projectResponse}
// @Failure 400 {object} errorEnvelope
// @Failure 401 {object} errorEnvelope
// @Failure 404 {object} errorEnvelope
// @Failure 500 {object} errorEnvelope
// @Security UserSession
// @Router /api/client/projects/{project_id} [get]
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

// updateProject godoc
//
// @Summary 更新项目
// @Description 项目所有者更新项目名称、描述或头像。
// @Tags 客户端项目
// @Accept json
// @Produce json
// @Param project_id path string true "项目 ID"
// @Param body body updateProjectRequest true "项目更新信息"
// @Success 200 {object} successEnvelope{data=projectResponse}
// @Failure 400 {object} errorEnvelope
// @Failure 401 {object} errorEnvelope
// @Failure 403 {object} errorEnvelope
// @Failure 404 {object} errorEnvelope
// @Failure 500 {object} errorEnvelope
// @Security UserSession
// @Router /api/client/projects/{project_id} [patch]
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
	if req.Name.Present {
		name, err := normalizeProjectName(req.Name.Value)
		if err != nil {
			return projectInvalidRequest(c, err.Error())
		}
		updates["name"] = name
	}
	if req.Description.Present {
		updates["description"] = req.Description.Value
	}
	if req.Avatar.Present {
		updates["avatar"] = req.Avatar.Value
	}

	var project store.Project
	err = s.db.WithContext(c.Request().Context()).Transaction(func(tx *gorm.DB) error {
		var role string
		var err error
		project, role, err = findAccessibleProjectForUpdate(tx, projectID, user.ID)
		if err != nil {
			return err
		}
		if role != store.ProjectRoleOwner {
			return errProjectOwnerRequired
		}
		if project.IsPersonal {
			if name, exists := updates["name"].(string); exists {
				if name != personalProjectName {
					return errPersonalProjectImmutable
				}
				delete(updates, "name")
			}
			if avatar, exists := updates["avatar"].(string); exists {
				if avatar != "" {
					return errPersonalProjectImmutable
				}
				delete(updates, "avatar")
			}
		}
		if len(updates) == 0 {
			return nil
		}
		now := time.Now().UTC()
		updates["updated_at"] = now
		result := tx.Model(&store.Project{}).
			Where("id = ? AND deleted_at IS NULL", project.ID).
			Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		if name, exists := updates["name"].(string); exists {
			project.Name = name
		}
		if description, exists := updates["description"].(string); exists {
			project.Description = description
		}
		if avatar, exists := updates["avatar"].(string); exists {
			project.Avatar = avatar
		}
		project.UpdatedAt = now
		return nil
	})
	if err != nil {
		return projectMutationFailure(c, err)
	}
	response, err := s.newProjectResponse(c.Request().Context(), project, store.ProjectRoleOwner)
	if err != nil {
		return projectInternalError(c)
	}
	return success(c, http.StatusOK, response)
}

// deleteProject godoc
//
// @Summary 删除项目
// @Description 项目所有者删除普通项目；个人项目不能删除。
// @Tags 客户端项目
// @Produce json
// @Param project_id path string true "项目 ID"
// @Success 200 {object} successEnvelope{data=projectResponse}
// @Failure 400 {object} errorEnvelope
// @Failure 401 {object} errorEnvelope
// @Failure 403 {object} errorEnvelope
// @Failure 404 {object} errorEnvelope
// @Failure 500 {object} errorEnvelope
// @Security UserSession
// @Router /api/client/projects/{project_id} [delete]
func (s *Server) deleteProject(c echo.Context) error {
	user, ok := currentUser(c)
	if !ok {
		return projectInternalError(c)
	}
	projectID, err := parseProjectID(c.Param("project_id"))
	if err != nil {
		return projectInvalidRequest(c, err.Error())
	}
	var project store.Project
	err = s.db.WithContext(c.Request().Context()).Transaction(func(tx *gorm.DB) error {
		var role string
		var err error
		project, role, err = findAccessibleProjectForUpdate(tx, projectID, user.ID)
		if err != nil {
			return err
		}
		if role != store.ProjectRoleOwner {
			return errProjectOwnerRequired
		}
		if project.IsPersonal {
			return errPersonalProjectDelete
		}
		result := tx.Where("id = ?", project.ID).Delete(&store.Project{})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, errPersonalProjectDelete) {
			return projectInvalidRequest(c, "个人项目不能删除")
		}
		return projectMutationFailure(c, err)
	}
	response, err := s.newProjectResponse(c.Request().Context(), project, store.ProjectRoleOwner)
	if err != nil {
		return projectInternalError(c)
	}
	return success(c, http.StatusOK, response)
}

// listProjectGroups godoc
//
// @Summary 列出项目群聊
// @Description 获取项目关联的可用群聊，按关联时间倒序分页。
// @Tags 客户端项目
// @Produce json
// @Param project_id path string true "项目 ID"
// @Param limit query int false "每页数量，默认 50，最大 100"
// @Param cursor query string false "群聊分页游标"
// @Success 200 {object} successEnvelope{data=projectGroupListResponse}
// @Failure 400 {object} errorEnvelope
// @Failure 401 {object} errorEnvelope
// @Failure 404 {object} errorEnvelope
// @Failure 500 {object} errorEnvelope
// @Security UserSession
// @Router /api/client/projects/{project_id}/groups [get]
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
	groupIDs := make([]string, 0, len(rows))
	for _, row := range rows {
		groupIDs = append(groupIDs, row.ConversationID)
	}
	memberCounts, err := s.activeConversationMemberCounts(c.Request().Context(), groupIDs)
	if err != nil {
		return projectInternalError(c)
	}
	groups := make([]projectGroupResponse, 0, len(rows))
	for _, row := range rows {
		groups = append(groups, projectGroupResponse{
			ID:          row.ConversationID,
			Name:        row.Name,
			Avatar:      row.Avatar,
			Status:      row.Status,
			MemberCount: memberCounts[row.ConversationID],
			CreatedAt:   row.CreatedAt,
		})
	}
	return success(c, http.StatusOK, projectGroupListResponse{Groups: groups, NextCursor: nextCursor})
}

// bindProjectGroup godoc
//
// @Summary 关联项目群聊
// @Description 项目所有者将可用群聊关联到普通项目；重复关联保持成功。
// @Tags 客户端项目
// @Accept json
// @Produce json
// @Param project_id path string true "项目 ID"
// @Param group_id path string true "群聊 ID"
// @Success 200 {object} successEnvelope{data=projectGroupMutationResponse}
// @Failure 400 {object} errorEnvelope
// @Failure 401 {object} errorEnvelope
// @Failure 403 {object} errorEnvelope
// @Failure 404 {object} errorEnvelope
// @Failure 500 {object} errorEnvelope
// @Security UserSession
// @Router /api/client/projects/{project_id}/groups/{group_id} [put]
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
	err = s.db.WithContext(c.Request().Context()).Transaction(func(tx *gorm.DB) error {
		project, role, err := findAccessibleProjectForUpdate(tx, projectID, user.ID)
		if err != nil {
			return err
		}
		if role != store.ProjectRoleOwner {
			return errProjectOwnerRequired
		}
		if project.IsPersonal {
			return errPersonalProjectGroup
		}
		if err := requireActiveGroupConversationForUpdate(tx, groupID); err != nil {
			return err
		}
		var existingCount int64
		if err := tx.Model(&store.ProjectGroup{}).
			Where("project_id = ? AND conversation_id = ?", project.ID, groupID).
			Count(&existingCount).Error; err != nil {
			return err
		}
		if existingCount > 0 {
			return nil
		}
		if err := requireGroupConversationProjectCapacity(tx, groupID); err != nil {
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
		updateResult := tx.Model(&store.Project{}).
			Where("id = ?", project.ID).
			Update("updated_at", now)
		if updateResult.Error != nil {
			return updateResult.Error
		}
		if updateResult.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return nil
	})
	if errors.Is(err, errInvalidProjectGroup) {
		return projectInvalidRequest(c, "群聊不存在或不可用")
	}
	if errors.Is(err, errPersonalProjectGroup) {
		return projectInvalidRequest(c, "个人项目不能关联群聊")
	}
	if err != nil {
		return projectMutationFailure(c, err)
	}
	return success(c, http.StatusOK, map[string]any{})
}

// unbindProjectGroup godoc
//
// @Summary 解除项目群聊
// @Description 项目所有者解除普通项目与群聊的关联；未关联时保持成功。
// @Tags 客户端项目
// @Produce json
// @Param project_id path string true "项目 ID"
// @Param group_id path string true "群聊 ID"
// @Success 200 {object} successEnvelope{data=projectGroupMutationResponse}
// @Failure 400 {object} errorEnvelope
// @Failure 401 {object} errorEnvelope
// @Failure 403 {object} errorEnvelope
// @Failure 404 {object} errorEnvelope
// @Failure 500 {object} errorEnvelope
// @Security UserSession
// @Router /api/client/projects/{project_id}/groups/{group_id} [delete]
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
	err = s.db.WithContext(c.Request().Context()).Transaction(func(tx *gorm.DB) error {
		project, role, err := findAccessibleProjectForUpdate(tx, projectID, user.ID)
		if err != nil {
			return err
		}
		if role != store.ProjectRoleOwner {
			return errProjectOwnerRequired
		}
		if project.IsPersonal {
			return errPersonalProjectGroup
		}
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
		updateResult := tx.Model(&store.Project{}).
			Where("id = ?", project.ID).
			Update("updated_at", time.Now().UTC())
		if updateResult.Error != nil {
			return updateResult.Error
		}
		if updateResult.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return nil
	})
	if errors.Is(err, errPersonalProjectGroup) {
		return projectInvalidRequest(c, "个人项目不能关联群聊")
	}
	if err != nil {
		return projectMutationFailure(c, err)
	}
	return success(c, http.StatusOK, map[string]any{})
}

// listProjectMembers godoc
//
// @Summary 列出项目成员
// @Description 获取项目成员及其来源群聊，按显示名称分页。
// @Tags 客户端项目
// @Produce json
// @Param project_id path string true "项目 ID"
// @Param limit query int false "每页数量，默认 50，最大 100"
// @Param cursor query string false "成员分页游标"
// @Success 200 {object} successEnvelope{data=projectMemberListResponse}
// @Failure 400 {object} errorEnvelope
// @Failure 401 {object} errorEnvelope
// @Failure 404 {object} errorEnvelope
// @Failure 500 {object} errorEnvelope
// @Security UserSession
// @Router /api/client/projects/{project_id}/members [get]
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

	members, err := s.loadProjectMemberPage(c.Request().Context(), project, cursor, limit+1)
	if err != nil {
		return projectInternalError(c)
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
	if err := s.loadProjectMemberSources(c.Request().Context(), project, members); err != nil {
		return projectInternalError(c)
	}
	return success(c, http.StatusOK, projectMemberListResponse{Members: members, NextCursor: nextCursor})
}

func (s *Server) activeConversationMemberCounts(ctx context.Context, conversationIDs []string) (map[string]int64, error) {
	counts := make(map[string]int64, len(conversationIDs))
	if len(conversationIDs) == 0 {
		return counts, nil
	}
	var rows []conversationCountRow
	err := s.db.WithContext(ctx).
		Model(&store.ConversationMember{}).
		Select("conversation_id, COUNT(*) AS count").
		Where("conversation_id IN ? AND left_at IS NULL", conversationIDs).
		Group("conversation_id").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		counts[row.ConversationID] = row.Count
	}
	return counts, nil
}

func (s *Server) loadProjectMemberPage(ctx context.Context, project store.Project, cursor *projectMemberListCursor, limit int) ([]projectMemberResponse, error) {
	query := `
		SELECT
			member_page.id,
			member_page.name,
			member_page.nickname,
			member_page.avatar,
			member_page.status,
			member_page.display_name
		FROM (
			SELECT
				member_base.id,
				member_base.name,
				member_base.nickname,
				member_base.avatar,
				member_base.status,
				CASE
					WHEN TRIM(member_base.nickname) <> '' THEN member_base.nickname
					ELSE member_base.name
				END AS display_name
			FROM (
				SELECT u.id, u.name, u.nickname, u.avatar, u.status
				FROM users u
				WHERE u.id = ?
				UNION
				SELECT u.id, u.name, u.nickname, u.avatar, u.status
				FROM users u
				JOIN conversation_members cm ON cm.member_id = u.id
				JOIN conversations c ON c.id = cm.conversation_id
				JOIN project_groups pg ON pg.conversation_id = c.id
				WHERE pg.project_id = ?
					AND c.kind = ?
					AND c.status = ?
					AND cm.member_type = ?
					AND cm.left_at IS NULL
			) member_base
		) member_page`
	args := []any{
		project.OwnerUserID,
		project.ID,
		store.ConversationKindGroup,
		store.ConversationStatusActive,
		store.ConversationMemberTypeUser,
	}
	if cursor != nil {
		query += `
			WHERE member_page.display_name > ?
				OR (member_page.display_name = ? AND member_page.id > ?)`
		args = append(args, cursor.DisplayName, cursor.DisplayName, cursor.ID)
	}
	query += `
		ORDER BY member_page.display_name ASC, member_page.id ASC
		LIMIT ?`
	args = append(args, limit)

	var rows []struct {
		ID          string `gorm:"column:id"`
		Name        string `gorm:"column:name"`
		Nickname    string `gorm:"column:nickname"`
		Avatar      string `gorm:"column:avatar"`
		Status      string `gorm:"column:status"`
		DisplayName string `gorm:"column:display_name"`
	}
	if err := s.db.WithContext(ctx).Raw(query, args...).Scan(&rows).Error; err != nil {
		return nil, err
	}
	members := make([]projectMemberResponse, 0, len(rows))
	for _, row := range rows {
		role := store.ProjectRoleMember
		if row.ID == project.OwnerUserID {
			role = store.ProjectRoleOwner
		}
		members = append(members, projectMemberResponse{
			ID:             row.ID,
			Name:           row.Name,
			Nickname:       row.Nickname,
			Avatar:         row.Avatar,
			Status:         row.Status,
			DisplayName:    row.DisplayName,
			Role:           role,
			SourceGroupIDs: []string{},
		})
	}
	return members, nil
}

func (s *Server) loadProjectMemberSources(ctx context.Context, project store.Project, members []projectMemberResponse) error {
	memberIDs := make([]string, 0, len(members))
	byID := make(map[string]*projectMemberResponse, len(members))
	for index := range members {
		if members[index].ID == project.OwnerUserID {
			continue
		}
		memberIDs = append(memberIDs, members[index].ID)
		byID[members[index].ID] = &members[index]
	}
	if len(memberIDs) == 0 {
		return nil
	}
	var rows []projectMemberSourceRow
	err := s.db.WithContext(ctx).
		Table("conversation_members cm").
		Select("cm.member_id, cm.conversation_id AS source_group_id").
		Joins("JOIN conversations c ON c.id = cm.conversation_id").
		Joins("JOIN project_groups pg ON pg.conversation_id = c.id").
		Where("pg.project_id = ?", project.ID).
		Where("c.kind = ? AND c.status = ?", store.ConversationKindGroup, store.ConversationStatusActive).
		Where("cm.member_type = ? AND cm.left_at IS NULL", store.ConversationMemberTypeUser).
		Where("cm.member_id IN ?", memberIDs).
		Scan(&rows).Error
	if err != nil {
		return err
	}
	sourceSets := make(map[string]map[string]struct{}, len(memberIDs))
	for _, row := range rows {
		if sourceSets[row.MemberID] == nil {
			sourceSets[row.MemberID] = make(map[string]struct{})
		}
		sourceSets[row.MemberID][row.SourceGroupID] = struct{}{}
	}
	for memberID, sources := range sourceSets {
		member := byID[memberID]
		for sourceGroupID := range sources {
			member.SourceGroupIDs = append(member.SourceGroupIDs, sourceGroupID)
		}
		sort.Strings(member.SourceGroupIDs)
	}
	return nil
}

func decodeProjectRequest(c echo.Context, destination any) error {
	decoder := json.NewDecoder(c.Request().Body)
	var raw json.RawMessage
	if err := decoder.Decode(&raw); err != nil {
		return err
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return errors.New("请求不能为 null")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("请求只能包含一个 JSON 对象")
		}
		return err
	}
	strictDecoder := json.NewDecoder(bytes.NewReader(raw))
	strictDecoder.DisallowUnknownFields()
	return strictDecoder.Decode(destination)
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

func findAccessibleProjectForUpdate(tx *gorm.DB, projectID string, userID string) (store.Project, string, error) {
	var project store.Project
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ?", projectID).
		Where(projectAccessSQL(), projectAccessArgs(userID)...).
		First(&project).Error
	if err != nil {
		return store.Project{}, "", err
	}
	if err := tx.First(&project.OwnerUser, "id = ?", project.OwnerUserID).Error; err != nil {
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
	responses, err := s.newProjectResponses(ctx, []store.Project{project}, map[string]string{project.ID: role})
	if err != nil {
		return projectResponse{}, err
	}
	return responses[0], nil
}

func (s *Server) newProjectResponses(ctx context.Context, projects []store.Project, roles map[string]string) ([]projectResponse, error) {
	responses := make([]projectResponse, 0, len(projects))
	if len(projects) == 0 {
		return responses, nil
	}
	projectIDs := make([]string, 0, len(projects))
	for _, project := range projects {
		projectIDs = append(projectIDs, project.ID)
	}
	groupCounts, err := s.loadProjectGroupCounts(ctx, projectIDs)
	if err != nil {
		return nil, err
	}
	memberCounts, err := s.loadProjectMemberCounts(ctx, projectIDs)
	if err != nil {
		return nil, err
	}
	taskCounts, err := s.loadProjectTaskCounts(ctx, projectIDs)
	if err != nil {
		return nil, err
	}
	for _, project := range projects {
		avatar := project.Avatar
		if project.IsPersonal {
			avatar = project.OwnerUser.Avatar
		}
		responses = append(responses, projectResponse{
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
			CurrentUserRole: roles[project.ID],
			GroupCount:      groupCounts[project.ID],
			MemberCount:     memberCounts[project.ID] + 1,
			TaskCounts:      taskCounts[project.ID],
			CreatedAt:       project.CreatedAt,
			UpdatedAt:       project.UpdatedAt,
		})
	}
	return responses, nil
}

func (s *Server) loadProjectGroupCounts(ctx context.Context, projectIDs []string) (map[string]int64, error) {
	counts := make(map[string]int64, len(projectIDs))
	var rows []projectCountRow
	err := s.db.WithContext(ctx).
		Table("project_groups pg").
		Select("pg.project_id, COUNT(DISTINCT pg.conversation_id) AS count").
		Joins("JOIN conversations c ON c.id = pg.conversation_id").
		Where("pg.project_id IN ?", projectIDs).
		Where("c.kind = ? AND c.status = ?", store.ConversationKindGroup, store.ConversationStatusActive).
		Group("pg.project_id").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		counts[row.ProjectID] = row.Count
	}
	return counts, nil
}

func (s *Server) loadProjectMemberCounts(ctx context.Context, projectIDs []string) (map[string]int64, error) {
	counts := make(map[string]int64, len(projectIDs))
	var rows []projectCountRow
	err := s.db.WithContext(ctx).
		Table("conversation_members cm").
		Select("pg.project_id, COUNT(DISTINCT cm.member_id) AS count").
		Joins("JOIN conversations c ON c.id = cm.conversation_id").
		Joins("JOIN project_groups pg ON pg.conversation_id = c.id").
		Joins("JOIN projects p ON p.id = pg.project_id").
		Where("pg.project_id IN ?", projectIDs).
		Where("c.kind = ? AND c.status = ?", store.ConversationKindGroup, store.ConversationStatusActive).
		Where("cm.member_type = ? AND cm.left_at IS NULL", store.ConversationMemberTypeUser).
		Where("cm.member_id <> p.owner_user_id").
		Group("pg.project_id").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		counts[row.ProjectID] = row.Count
	}
	return counts, nil
}

func (s *Server) loadProjectTaskCounts(ctx context.Context, projectIDs []string) (map[string]projectTaskCountsResponse, error) {
	counts := make(map[string]projectTaskCountsResponse, len(projectIDs))
	var rows []projectTaskStatusCount
	err := s.db.WithContext(ctx).
		Model(&store.Task{}).
		Select("project_id, status, COUNT(*) AS count").
		Where("project_id IN ?", projectIDs).
		Group("project_id, status").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		projectCounts := counts[row.ProjectID]
		projectCounts.Total += row.Count
		switch row.Status {
		case store.TaskStatusTodo:
			projectCounts.Todo = row.Count
		case store.TaskStatusInProgress:
			projectCounts.InProgress = row.Count
		case store.TaskStatusDone:
			projectCounts.Done = row.Count
		case store.TaskStatusCanceled:
			projectCounts.Canceled = row.Count
		}
		counts[row.ProjectID] = projectCounts
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
	sort.Strings(result)
	return result, nil
}

func requireActiveGroupConversationForUpdate(tx *gorm.DB, groupID string) error {
	var conversation store.Conversation
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Select("id", "kind", "status").
		First(&conversation, "id = ?", groupID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return errInvalidProjectGroup
	}
	if err != nil {
		return err
	}
	if conversation.Kind != store.ConversationKindGroup || conversation.Status != store.ConversationStatusActive {
		return errInvalidProjectGroup
	}
	return nil
}

func requireGroupConversationProjectCapacity(tx *gorm.DB, groupID string) error {
	var count int64
	if err := tx.Model(&store.ProjectGroup{}).
		Where("conversation_id = ?", groupID).
		Count(&count).Error; err != nil {
		return err
	}
	if count >= maxGroupConversationProjects {
		return errGroupConversationProjectCap
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

func projectMutationFailure(c echo.Context, err error) error {
	if errors.Is(err, errPersonalProjectImmutable) {
		return projectInvalidRequest(c, "个人工作区名称和头像不能修改")
	}
	if errors.Is(err, errGroupConversationProjectCap) {
		return failure(c, http.StatusConflict, "conflict", "群聊关联项目数量已达上限")
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return projectNotFound(c)
	}
	if errors.Is(err, errProjectOwnerRequired) {
		return projectForbidden(c)
	}
	return projectInternalError(c)
}

func projectInternalError(c echo.Context) error {
	return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
}
