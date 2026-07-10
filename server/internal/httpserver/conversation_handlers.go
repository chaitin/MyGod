package httpserver

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"app/internal/appregistry"
	"app/internal/store"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const maxGroupConversationMembers = 500
const maxGroupConversationNameLength = 120
const maxClientConversationListItems = 100

const (
	builtinAssistantAvatar              = appregistry.AIAssistantDefaultAvatar
	builtinAssistantConversationName    = appregistry.AIAssistantDefaultName
	messageTypeSystemEvent              = "system_event"
	systemEventGroupMembersInvited      = "group_members_invited"
	systemEventGroupAvatarUpdated       = "group_avatar_updated"
	systemEventGroupVisibilityChanged   = "group_visibility_changed"
	systemEventGroupMemberJoined        = "group_member_joined"
	systemEventGroupMemberLeft          = "group_member_left"
	systemEventGroupMemberRemoved       = "group_member_removed"
	systemEventGroupNameUpdated         = "group_name_updated"
	systemEventMessageRevoked           = "message_revoked"
	groupMembersInvitedSummarySeparator = ","
)

var (
	errConversationNotGroup               = errors.New("conversation is not group")
	errGroupConversationMemberCap         = errors.New("group conversation member cap exceeded")
	errGroupConversationMemberMiss        = errors.New("group conversation member missing")
	errGroupConversationAvatarForbidden   = errors.New("group conversation avatar forbidden")
	errGroupConversationOwnerCannotLeave  = errors.New("group conversation owner cannot leave")
	errGroupConversationOwnerCannotRemove = errors.New("group conversation owner cannot be removed")
	errGroupConversationCannotRemoveSelf  = errors.New("group conversation member cannot remove self")
	errGroupConversationProjectInvalid    = errors.New("invalid group conversation project")
	errGroupConversationProjectPersonal   = errors.New("personal project cannot link group conversation")
	errGroupConversationProjectUnowned    = errors.New("group conversation project is not owned by user")
	errGroupConversationProjectMissing    = errors.New("group conversation project missing")
	errGroupConversationProjectMutation   = errors.New("group conversation project mutation failed")
	errGroupConversationProjectLockChange = errors.New("group conversation project lock set changed")
)

type createGroupConversationRequest struct {
	AppIDs     []string                   `json:"app_ids" example:"7f8d8b84-6d2c-4b12-9a8a-019a7e2787d4"`
	MemberIDs  []string                   `json:"member_ids" example:"7f8d8b84-6d2c-4b12-9a8a-019a7e2787d4"`
	Name       string                     `json:"name" example:"产品讨论组"`
	ProjectIDs projectOptionalStringSlice `json:"project_ids" swaggertype:"array,string" example:"7f8d8b84-6d2c-4b12-9a8a-019a7e2787d4"`
}

type addGroupConversationMembersRequest struct {
	AppIDs    []string `json:"app_ids" example:"7f8d8b84-6d2c-4b12-9a8a-019a7e2787d4"`
	MemberIDs []string `json:"member_ids" example:"7f8d8b84-6d2c-4b12-9a8a-019a7e2787d4"`
}

type updateGroupConversationNameRequest struct {
	Name string `json:"name" example:"产品讨论组"`
}

type createAppConversationRequest struct {
	AppID string `json:"app_id" example:"7f8d8b84-6d2c-4b12-9a8a-019a7e2787d4"`
}

type createDirectConversationRequest struct {
	UserID string `json:"user_id" example:"7f8d8b84-6d2c-4b12-9a8a-019a7e2787d4"`
}

type conversationMemberResponse struct {
	Avatar   string `json:"avatar" example:"/assets/avatars/builtin/07.webp"`
	Email    string `json:"email" example:"user@example.com"`
	ID       string `json:"id" example:"7f8d8b84-6d2c-4b12-9a8a-019a7e2787d4"`
	Name     string `json:"name" example:"张三"`
	Nickname string `json:"nickname" example:"小张"`
	Phone    string `json:"phone" example:"+8613812345678"`
	Role     string `json:"role" example:"member"`
	Type     string `json:"type" example:"user"`
}

type groupConversationResponse struct {
	Avatar             string                       `json:"avatar" example:"/assets/avatars/groups/07.webp"`
	CreatedAt          time.Time                    `json:"created_at" format:"date-time"`
	CreatedByUserID    string                       `json:"created_by_user_id" example:"7f8d8b84-6d2c-4b12-9a8a-019a7e2787d4"`
	ID                 string                       `json:"id" example:"7f8d8b84-6d2c-4b12-9a8a-019a7e2787d4"`
	LastMessageAt      *time.Time                   `json:"last_message_at" format:"date-time"`
	LastMessageID      *string                      `json:"last_message_id" example:"7f8d8b84-6d2c-4b12-9a8a-019a7e2787d4"`
	LastMessageSeq     int64                        `json:"last_message_seq" example:"12"`
	LastMessageSummary string                       `json:"last_message_summary" example:"张三 邀请 李四 加入群聊"`
	LastMentionedSeq   int64                        `json:"last_mentioned_seq" example:"0"`
	LastReadSeq        int64                        `json:"last_read_seq" example:"12"`
	MemberCount        int                          `json:"member_count" example:"3"`
	Members            []conversationMemberResponse `json:"members"`
	Name               string                       `json:"name" example:"产品讨论组"`
	PostingPolicy      string                       `json:"posting_policy" example:"open"`
	Status             string                       `json:"status" example:"active"`
	Type               string                       `json:"type" example:"group"`
	UnreadCount        int64                        `json:"unread_count" example:"0"`
	Visibility         string                       `json:"visibility" example:"private"`
}

type createGroupConversationResponse struct {
	Conversation groupConversationResponse `json:"conversation"`
}

type addGroupConversationMembersResponse struct {
	Conversation conversationListItemResponse `json:"conversation"`
	Message      *messageResponse             `json:"message"`
}

type updateGroupConversationAvatarResponse struct {
	Conversation conversationListItemResponse `json:"conversation"`
	Message      messageResponse              `json:"message"`
}

type leaveGroupConversationResponse struct {
	ConversationID string          `json:"conversation_id" example:"7f8d8b84-6d2c-4b12-9a8a-019a7e2787d4"`
	Message        messageResponse `json:"message"`
}

type dissolveGroupConversationResponse struct {
	ConversationID string `json:"conversation_id" example:"7f8d8b84-6d2c-4b12-9a8a-019a7e2787d4"`
}

type createDirectConversationResponse struct {
	Conversation conversationListItemResponse `json:"conversation"`
	Created      bool                         `json:"created" example:"true"`
}

type conversationListItemResponse struct {
	Avatar             string                       `json:"avatar" example:"/assets/avatars/builtin/07.webp"`
	CreatedAt          time.Time                    `json:"created_at" format:"date-time"`
	ID                 string                       `json:"id" example:"7f8d8b84-6d2c-4b12-9a8a-019a7e2787d4"`
	LastMessageAt      *time.Time                   `json:"last_message_at" format:"date-time"`
	LastMessageID      *string                      `json:"last_message_id" example:"7f8d8b84-6d2c-4b12-9a8a-019a7e2787d4"`
	LastMessageSeq     int64                        `json:"last_message_seq" example:"12"`
	LastMessageSummary string                       `json:"last_message_summary" example:"好的，我看一下"`
	LastMentionedSeq   int64                        `json:"last_mentioned_seq" example:"0"`
	LastReadSeq        int64                        `json:"last_read_seq" example:"9"`
	MemberCount        int                          `json:"member_count" example:"2"`
	Members            []conversationMemberResponse `json:"members"`
	Name               string                       `json:"name" example:"张三"`
	Type               string                       `json:"type" example:"direct"`
	UnreadCount        int64                        `json:"unread_count" example:"3"`
	Visibility         string                       `json:"visibility" example:"private"`
}

type listClientConversationsResponse struct {
	Conversations []conversationListItemResponse `json:"conversations"`
}

type markConversationReadRequest struct {
	UpToSeq *int64 `json:"up_to_seq" example:"123"`
}

type markConversationReadResponse struct {
	ConversationID string `json:"conversation_id" example:"7f8d8b84-6d2c-4b12-9a8a-019a7e2787d4"`
	LastReadSeq    int64  `json:"last_read_seq" example:"123"`
	UnreadCount    int64  `json:"unread_count" example:"0"`
}

type conversationMemberCandidate struct {
	app        store.App
	memberType string
	role       string
	user       store.User
}

type systemEventUserRef struct {
	DisplayName string `json:"display_name"`
	ID          string `json:"id"`
	Type        string `json:"type,omitempty"`
}

type groupMembersInvitedSystemEventBody struct {
	Event    string               `json:"event"`
	Invitees []systemEventUserRef `json:"invitees"`
	Inviter  systemEventUserRef   `json:"inviter"`
	Type     string               `json:"type"`
}

type groupAvatarUpdatedSystemEventBody struct {
	Actor systemEventUserRef `json:"actor"`
	Event string             `json:"event"`
	Type  string             `json:"type"`
}

type groupVisibilityChangedSystemEventBody struct {
	Actor      systemEventUserRef `json:"actor"`
	Event      string             `json:"event"`
	Type       string             `json:"type"`
	Visibility string             `json:"visibility"`
}

type groupMemberJoinedSystemEventBody struct {
	Actor systemEventUserRef `json:"actor"`
	Event string             `json:"event"`
	Type  string             `json:"type"`
}

type groupMemberLeftSystemEventBody struct {
	Actor systemEventUserRef `json:"actor"`
	Event string             `json:"event"`
	Type  string             `json:"type"`
}

type groupMemberRemovedSystemEventBody struct {
	Actor  systemEventUserRef `json:"actor"`
	Event  string             `json:"event"`
	Target systemEventUserRef `json:"target"`
	Type   string             `json:"type"`
}

type groupNameUpdatedSystemEventBody struct {
	Actor systemEventUserRef `json:"actor"`
	Event string             `json:"event"`
	Name  string             `json:"name"`
	Type  string             `json:"type"`
}

type messageRevokedSystemEventBody struct {
	Actor systemEventUserRef `json:"actor"`
	Event string             `json:"event"`
	Type  string             `json:"type"`
}

// listClientConversations godoc
//
// @Summary 列出当前用户会话
// @Description 普通用户获取自己参与的最近 100 个会话，按照最后消息时间倒序排列。
// @Tags 客户端会话
// @Produce json
// @Success 200 {object} successEnvelope{data=listClientConversationsResponse}
// @Failure 401 {object} errorEnvelope
// @Failure 500 {object} errorEnvelope
// @Router /api/client/conversations [get]
func (s *Server) listClientConversations(c echo.Context) error {
	user, ok := currentUser(c)
	if !ok {
		return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
	}

	assistantConversationID := builtinAssistantConversationID(user.ID)
	assistantConversation, hasAssistantConversation, err := s.ensureBuiltinAssistantConversation(user)
	if err != nil {
		return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
	}
	listLimit := maxClientConversationListItems
	if hasAssistantConversation {
		listLimit--
	}

	var conversations []store.Conversation
	if err := s.db.Model(&store.Conversation{}).
		Joins("JOIN conversation_members cm ON cm.conversation_id = conversations.id").
		Where("cm.member_type = ? AND cm.member_id = ? AND cm.left_at IS NULL", store.ConversationMemberTypeUser, user.ID).
		Where("conversations.id <> ?", assistantConversationID).
		Where("conversations.status = ?", store.ConversationStatusActive).
		Order("COALESCE(conversations.last_message_at, conversations.created_at) DESC").
		Order("conversations.id ASC").
		Limit(listLimit).
		Find(&conversations).Error; err != nil {
		return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
	}

	conversationIDs := make([]string, 0, len(conversations)+1)
	if hasAssistantConversation {
		conversationIDs = append(conversationIDs, assistantConversation.ID)
	}
	for _, conversation := range conversations {
		conversationIDs = append(conversationIDs, conversation.ID)
	}

	membersByConversationID, usersByID, appsByID, err := s.loadConversationListMembers(conversationIDs)
	if err != nil {
		return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
	}

	responses := make([]conversationListItemResponse, 0, len(conversations)+1)
	if hasAssistantConversation {
		responses = append(responses, newConversationListItemResponse(
			assistantConversation,
			user.ID,
			membersByConversationID[assistantConversation.ID],
			usersByID,
			appsByID,
		))
	}
	for _, conversation := range conversations {
		responses = append(responses, newConversationListItemResponse(
			conversation,
			user.ID,
			membersByConversationID[conversation.ID],
			usersByID,
			appsByID,
		))
	}

	return success(c, http.StatusOK, listClientConversationsResponse{
		Conversations: responses,
	})
}

func (s *Server) ensureBuiltinAssistantConversation(user store.User) (store.Conversation, bool, error) {
	assistantApp, err := appregistry.EnsureAIAssistantApp(s.db, s.cfg.Apps)
	if err != nil {
		return store.Conversation{}, false, err
	}
	if !assistantApp.Enabled {
		return store.Conversation{}, false, nil
	}

	conversationID := builtinAssistantConversationID(user.ID)
	now := time.Now().UTC()
	conversation := store.Conversation{}

	err = s.db.Transaction(func(tx *gorm.DB) error {
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&conversation, "id = ?", conversationID).Error
		if err == nil {
			if err := ensureBuiltinAssistantConversationFields(tx, &conversation, assistantApp, user.ID, now); err != nil {
				return err
			}
			if err := ensureBuiltinAssistantConversationMembers(tx, conversation.ID, assistantApp.ID, user.ID, now); err != nil {
				return err
			}
			return ensureBuiltinAssistantAppConversation(tx, assistantApp.ID, conversation.ID, user.ID, now)
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		conversation = store.Conversation{
			ID:              conversationID,
			Kind:            store.ConversationKindApp,
			Name:            assistantApp.Name,
			Avatar:          assistantApp.Avatar,
			CreatedByUserID: user.ID,
			Status:          store.ConversationStatusActive,
			PostingPolicy:   store.ConversationPostingPolicyOpen,
			Visibility:      store.ConversationVisibilityPrivate,
			CreatedAt:       now,
			UpdatedAt:       now,
		}
		if err := tx.Create(&conversation).Error; err != nil {
			return err
		}

		if err := ensureBuiltinAssistantConversationMembers(tx, conversation.ID, assistantApp.ID, user.ID, now); err != nil {
			return err
		}
		return ensureBuiltinAssistantAppConversation(tx, assistantApp.ID, conversation.ID, user.ID, now)
	})
	if err != nil {
		if isUniqueConstraintError(err) {
			if findErr := s.db.First(&conversation, "id = ?", conversationID).Error; findErr == nil {
				return conversation, true, nil
			}
		}
		return store.Conversation{}, false, err
	}

	return conversation, true, nil
}

func ensureBuiltinAssistantConversationFields(db *gorm.DB, conversation *store.Conversation, assistantApp store.App, userID string, now time.Time) error {
	updates := map[string]any{}
	if conversation.Kind != store.ConversationKindApp {
		updates["kind"] = store.ConversationKindApp
	}
	if conversation.Name != assistantApp.Name {
		updates["name"] = assistantApp.Name
	}
	if conversation.Avatar != assistantApp.Avatar {
		updates["avatar"] = assistantApp.Avatar
	}
	if conversation.CreatedByUserID != userID {
		updates["created_by_user_id"] = userID
	}
	if conversation.Status != store.ConversationStatusActive {
		updates["status"] = store.ConversationStatusActive
	}
	if conversation.PostingPolicy != store.ConversationPostingPolicyOpen {
		updates["posting_policy"] = store.ConversationPostingPolicyOpen
	}
	if conversation.Visibility == "" {
		updates["visibility"] = store.ConversationVisibilityPrivate
	}
	if len(updates) == 0 {
		return nil
	}

	updates["updated_at"] = now
	if err := db.Model(&store.Conversation{}).Where("id = ?", conversation.ID).Updates(updates).Error; err != nil {
		return err
	}
	for field, value := range updates {
		switch field {
		case "kind":
			conversation.Kind = value.(string)
		case "name":
			conversation.Name = value.(string)
		case "avatar":
			conversation.Avatar = value.(string)
		case "created_by_user_id":
			conversation.CreatedByUserID = value.(string)
		case "status":
			conversation.Status = value.(string)
		case "posting_policy":
			conversation.PostingPolicy = value.(string)
		case "visibility":
			conversation.Visibility = value.(string)
		case "updated_at":
			conversation.UpdatedAt = value.(time.Time)
		}
	}

	return nil
}

func ensureBuiltinAssistantConversationMembers(db *gorm.DB, conversationID string, appID string, userID string, now time.Time) error {
	if err := ensureBuiltinAssistantConversationMember(db, store.ConversationMember{
		ConversationID:        conversationID,
		MemberType:            store.ConversationMemberTypeUser,
		MemberID:              userID,
		Role:                  store.ConversationMemberRoleOwner,
		JoinedAt:              now,
		HistoryVisibleFromSeq: 1,
	}); err != nil {
		return err
	}

	return ensureBuiltinAssistantConversationMember(db, store.ConversationMember{
		ConversationID:        conversationID,
		MemberType:            store.ConversationMemberTypeApp,
		MemberID:              appID,
		Role:                  store.ConversationMemberRoleMember,
		JoinedAt:              now,
		HistoryVisibleFromSeq: 1,
	})
}

func ensureBuiltinAssistantConversationMember(db *gorm.DB, member store.ConversationMember) error {
	var existing store.ConversationMember
	err := db.First(
		&existing,
		"conversation_id = ? AND member_type = ? AND member_id = ?",
		member.ConversationID,
		member.MemberType,
		member.MemberID,
	).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return db.Create(&member).Error
	}
	if err != nil {
		return err
	}

	updates := map[string]any{}
	if existing.Role != member.Role {
		updates["role"] = member.Role
	}
	if existing.HistoryVisibleFromSeq < 1 {
		updates["history_visible_from_seq"] = int64(1)
	}
	if existing.LeftAt != nil {
		updates["left_at"] = nil
	}
	if len(updates) == 0 {
		return nil
	}

	return db.Model(&store.ConversationMember{}).
		Where("conversation_id = ? AND member_type = ? AND member_id = ?", member.ConversationID, member.MemberType, member.MemberID).
		Updates(updates).Error
}

func ensureBuiltinAssistantAppConversation(db *gorm.DB, appID string, conversationID string, userID string, now time.Time) error {
	var existing store.AppConversation
	err := db.First(&existing, "app_id = ? AND user_id = ?", appID, userID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return db.Create(&store.AppConversation{
			AppID:          appID,
			ConversationID: conversationID,
			UserID:         userID,
			CreatedAt:      now,
		}).Error
	}
	if err != nil {
		return err
	}
	if existing.ConversationID == conversationID {
		return nil
	}

	return db.Model(&store.AppConversation{}).
		Where("app_id = ? AND user_id = ?", appID, userID).
		Update("conversation_id", conversationID).Error
}

func builtinAssistantConversationID(userID string) string {
	namespace := uuid.NewSHA1(uuid.NameSpaceURL, []byte("mygod:builtin-assistant-conversation"))

	return uuid.NewSHA1(namespace, []byte(strings.ToLower(strings.TrimSpace(userID)))).String()
}

// markConversationRead godoc
//
// @Summary 标记会话已读
// @Description 普通用户把自己在指定会话中的已读位置推进到指定 seq，未指定时推进到会话当前最新消息。
// @Tags 客户端会话
// @Accept json
// @Produce json
// @Param conversation_id path string true "会话 ID"
// @Param body body markConversationReadRequest false "已读位置"
// @Success 200 {object} successEnvelope{data=markConversationReadResponse}
// @Failure 400 {object} errorEnvelope
// @Failure 401 {object} errorEnvelope
// @Failure 403 {object} errorEnvelope
// @Failure 404 {object} errorEnvelope
// @Failure 500 {object} errorEnvelope
// @Router /api/client/conversations/{conversation_id}/read [post]
func (s *Server) markConversationRead(c echo.Context) error {
	user, ok := currentUser(c)
	if !ok {
		return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
	}

	conversationID, err := normalizeMessageConversationID(c.Param("conversation_id"))
	if err != nil {
		return failure(c, http.StatusBadRequest, "invalid_request", err.Error())
	}

	var req markConversationReadRequest
	if err := c.Bind(&req); err != nil && !errors.Is(err, io.EOF) {
		return failure(c, http.StatusBadRequest, "invalid_request", "请求格式错误")
	}
	if req.UpToSeq != nil && *req.UpToSeq <= 0 {
		return failure(c, http.StatusBadRequest, "invalid_request", "up_to_seq 必须是正整数")
	}

	response, err := s.markUserConversationRead(user.ID, conversationID, req.UpToSeq)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return failure(c, http.StatusNotFound, "not_found", "会话不存在")
		}
		if errors.Is(err, errConversationAccessDenied) {
			return failure(c, http.StatusForbidden, "forbidden", "无权访问会话")
		}

		return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
	}

	return success(c, http.StatusOK, response)
}

// createDirectConversation godoc
//
// @Summary 创建或打开一对一会话
// @Description 普通用户创建或打开和指定用户的一对一会话。重复调用会返回已有会话，不会创建重复私聊。
// @Tags 客户端会话
// @Accept json
// @Produce json
// @Param body body createDirectConversationRequest true "一对一会话目标用户"
// @Success 200 {object} successEnvelope{data=createDirectConversationResponse}
// @Success 201 {object} successEnvelope{data=createDirectConversationResponse}
// @Failure 400 {object} errorEnvelope
// @Failure 401 {object} errorEnvelope
// @Failure 500 {object} errorEnvelope
// @Router /api/client/conversations/direct [post]
func (s *Server) createDirectConversation(c echo.Context) error {
	user, ok := currentUser(c)
	if !ok {
		return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
	}

	var req createDirectConversationRequest
	if err := c.Bind(&req); err != nil {
		return failure(c, http.StatusBadRequest, "invalid_request", "请求格式错误")
	}

	targetID, err := normalizeDirectConversationUserID(req.UserID, user.ID)
	if err != nil {
		return failure(c, http.StatusBadRequest, "invalid_request", err.Error())
	}

	var target store.User
	err = s.db.First(&target, "id = ? AND status = ?", targetID, store.UserStatusActive).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return failure(c, http.StatusBadRequest, "invalid_request", "用户不存在或已禁用")
	}
	if err != nil {
		return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
	}

	conversation, created, err := s.getOrCreateDirectConversation(user, target)
	if err != nil {
		return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
	}

	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}

	return success(c, status, createDirectConversationResponse{
		Conversation: newConversationListItemResponse(
			conversation,
			user.ID,
			[]store.ConversationMember{
				{ConversationID: conversation.ID, MemberType: store.ConversationMemberTypeUser, MemberID: user.ID},
				{ConversationID: conversation.ID, MemberType: store.ConversationMemberTypeUser, MemberID: target.ID},
			},
			map[string]store.User{
				user.ID:   user,
				target.ID: target,
			},
			nil,
		),
		Created: created,
	})
}

// createAppConversation godoc
//
// @Summary 创建或打开应用会话
// @Description 普通用户创建或打开和指定应用的会话。应用必须启用且对当前用户可见。
// @Tags 客户端会话
// @Accept json
// @Produce json
// @Param body body createAppConversationRequest true "应用会话目标应用"
// @Success 200 {object} successEnvelope{data=createDirectConversationResponse}
// @Success 201 {object} successEnvelope{data=createDirectConversationResponse}
// @Failure 400 {object} errorEnvelope
// @Failure 401 {object} errorEnvelope
// @Failure 404 {object} errorEnvelope
// @Failure 500 {object} errorEnvelope
// @Router /api/client/conversations/apps [post]
func (s *Server) createAppConversation(c echo.Context) error {
	user, ok := currentUser(c)
	if !ok {
		return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
	}

	var req createAppConversationRequest
	if err := c.Bind(&req); err != nil {
		return failure(c, http.StatusBadRequest, "invalid_request", "请求格式错误")
	}
	appID, err := normalizeUUIDString(req.AppID, "应用 ID 格式错误")
	if err != nil {
		return failure(c, http.StatusBadRequest, "invalid_request", err.Error())
	}

	app, ok, err := s.findVisibleClientApp(appID, user.ID)
	if err != nil {
		return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
	}
	if !ok {
		return failure(c, http.StatusNotFound, "not_found", "应用不存在")
	}

	conversation, created, err := s.getOrCreateAppConversation(user, app)
	if err != nil {
		return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
	}

	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}

	return success(c, status, createDirectConversationResponse{
		Conversation: newConversationListItemResponse(
			conversation,
			user.ID,
			[]store.ConversationMember{
				{ConversationID: conversation.ID, MemberType: store.ConversationMemberTypeUser, MemberID: user.ID},
				{ConversationID: conversation.ID, MemberType: store.ConversationMemberTypeApp, MemberID: app.ID},
			},
			map[string]store.User{
				user.ID: user,
			},
			map[string]store.App{
				app.ID: app,
			},
		),
		Created: created,
	})
}

// createGroupConversation godoc
//
// @Summary 创建群聊
// @Description 普通用户创建群聊。当前登录用户会自动成为群主，member_ids 和 app_ids 可选择其他成员或应用，也可以都为空。
// @Tags 客户端会话
// @Accept json
// @Produce json
// @Param body body createGroupConversationRequest true "群聊信息"
// @Success 201 {object} successEnvelope{data=createGroupConversationResponse}
// @Failure 400 {object} errorEnvelope
// @Failure 401 {object} errorEnvelope
// @Failure 500 {object} errorEnvelope
// @Router /api/client/conversations/groups [post]
func (s *Server) createGroupConversation(c echo.Context) error {
	user, ok := currentUser(c)
	if !ok {
		return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
	}

	var req createGroupConversationRequest
	if err := c.Bind(&req); err != nil {
		return failure(c, http.StatusBadRequest, "invalid_request", "请求格式错误")
	}

	name, err := normalizeGroupConversationName(req.Name)
	if err != nil {
		return failure(c, http.StatusBadRequest, "invalid_request", err.Error())
	}

	memberIDs, err := normalizeGroupMemberIDs(req.MemberIDs, user.ID)
	if err != nil {
		return failure(c, http.StatusBadRequest, "invalid_request", err.Error())
	}
	appIDs, err := normalizeGroupAppIDs(req.AppIDs)
	if err != nil {
		return failure(c, http.StatusBadRequest, "invalid_request", err.Error())
	}
	projectIDs, err := normalizeGroupProjectIDs(req.ProjectIDs.Value)
	if err != nil {
		return failure(c, http.StatusBadRequest, "invalid_request", "项目 ID 格式错误")
	}
	if len(memberIDs)+len(appIDs)+1 > maxGroupConversationMembers {
		return failure(c, http.StatusBadRequest, "invalid_request", "群聊成员不能超过 500 人")
	}

	conversation, createdMessage, candidates, memberUserIDs, err := s.createUserGroupConversationWithProjects(user, name, memberIDs, appIDs, projectIDs)
	if err != nil {
		if errors.Is(err, errGroupConversationMemberMiss) {
			return failure(c, http.StatusBadRequest, "invalid_request", "成员或应用不存在或不可用")
		}
		if errors.Is(err, errGroupConversationMemberCap) {
			return failure(c, http.StatusBadRequest, "invalid_request", "群聊成员不能超过 500 人")
		}
		if errors.Is(err, errGroupConversationProjectInvalid) {
			return failure(c, http.StatusBadRequest, "invalid_request", "项目 ID 格式错误")
		}
		if errors.Is(err, errGroupConversationProjectPersonal) {
			return failure(c, http.StatusBadRequest, "invalid_request", "个人项目不能关联群聊")
		}
		if errors.Is(err, errGroupConversationProjectUnowned) || errors.Is(err, errGroupConversationProjectMissing) {
			return failure(c, http.StatusNotFound, "not_found", "项目不存在")
		}
		return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
	}

	if createdMessage != nil {
		s.realtime.SendToUsers(memberUserIDs, realtimeMessageCreatedEvent(newMessageResponse(*createdMessage)))
	}

	return success(c, http.StatusCreated, createGroupConversationResponse{
		Conversation: newGroupConversationResponse(conversation, candidates, user.ID),
	})
}

func (s *Server) createUserGroupConversation(user store.User, name string, memberIDs []string, appIDs []string) (store.Conversation, *store.Message, []conversationMemberCandidate, []string, error) {
	return s.createUserGroupConversationWithProjects(user, name, memberIDs, appIDs, nil)
}

func (s *Server) createUserGroupConversationWithProjects(user store.User, name string, memberIDs []string, appIDs []string, projectIDs []string) (store.Conversation, *store.Message, []conversationMemberCandidate, []string, error) {
	if len(memberIDs)+len(appIDs)+1 > maxGroupConversationMembers {
		return store.Conversation{}, nil, nil, nil, errGroupConversationMemberCap
	}

	members, err := s.loadActiveGroupMembers(memberIDs)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return store.Conversation{}, nil, nil, nil, errGroupConversationMemberMiss
		}
		return store.Conversation{}, nil, nil, nil, err
	}
	apps, err := loadVisibleGroupApps(s.db, user.ID, appIDs)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return store.Conversation{}, nil, nil, nil, errGroupConversationMemberMiss
		}
		return store.Conversation{}, nil, nil, nil, err
	}

	now := time.Now().UTC()
	conversation := store.Conversation{
		ID:              uuid.NewString(),
		Kind:            store.ConversationKindGroup,
		Name:            name,
		CreatedByUserID: user.ID,
		Status:          store.ConversationStatusActive,
		PostingPolicy:   store.ConversationPostingPolicyOpen,
		Visibility:      store.ConversationVisibilityPrivate,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	candidates := make([]conversationMemberCandidate, 0, len(members)+len(apps)+1)
	candidates = append(candidates, conversationMemberCandidate{
		memberType: store.ConversationMemberTypeUser,
		role:       store.ConversationMemberRoleOwner,
		user:       user,
	})
	for _, member := range members {
		candidates = append(candidates, conversationMemberCandidate{
			memberType: store.ConversationMemberTypeUser,
			role:       store.ConversationMemberRoleMember,
			user:       member,
		})
	}
	for _, app := range apps {
		candidates = append(candidates, conversationMemberCandidate{
			app:        app,
			memberType: store.ConversationMemberTypeApp,
			role:       store.ConversationMemberRoleMember,
		})
	}

	var createdMessage *store.Message
	memberUserIDs := make([]string, 0, len(candidates))
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		projects, err := lockOwnedGroupConversationProjects(tx, projectIDs, user.ID)
		if err != nil {
			return err
		}

		if err := tx.Create(&conversation).Error; err != nil {
			return err
		}

		systemMessageSeq := conversation.LastMessageSeq
		if len(members)+len(apps) > 0 {
			systemMessageSeq++
		}
		conversationMembers := make([]store.ConversationMember, 0, len(candidates))
		for _, candidate := range candidates {
			memberID := candidate.user.ID
			if candidate.memberType == store.ConversationMemberTypeApp {
				memberID = candidate.app.ID
			}
			lastReadSeq := int64(0)
			if candidate.memberType == store.ConversationMemberTypeUser && memberID == user.ID {
				lastReadSeq = systemMessageSeq
			}
			conversationMembers = append(conversationMembers, store.ConversationMember{
				ConversationID:        conversation.ID,
				MemberType:            candidate.memberType,
				MemberID:              memberID,
				Role:                  candidate.role,
				JoinedAt:              now,
				HistoryVisibleFromSeq: 1,
				LastReadSeq:           lastReadSeq,
			})
			if candidate.memberType == store.ConversationMemberTypeUser {
				memberUserIDs = append(memberUserIDs, memberID)
			}
		}

		if err := tx.Create(&conversationMembers).Error; err != nil {
			return err
		}

		if len(members)+len(apps) > 0 {
			message, err := createGroupMembersInvitedSystemMessage(tx, &conversation, user, makeGroupMemberInviteeRefs(members, apps), now)
			if err != nil {
				return err
			}
			createdMessage = &message
		}

		if len(projects) > 0 {
			links := make([]store.ProjectGroup, 0, len(projects))
			for _, project := range projects {
				links = append(links, store.ProjectGroup{
					ProjectID:      project.ID,
					ConversationID: conversation.ID,
					LinkedByUserID: user.ID,
					CreatedAt:      now,
				})
			}
			if err := tx.Create(&links).Error; err != nil {
				return err
			}
			for _, project := range projects {
				result := tx.Model(&store.Project{}).
					Where("id = ?", project.ID).
					Update("updated_at", now)
				if result.Error != nil {
					return result.Error
				}
				if result.RowsAffected != 1 {
					return errGroupConversationProjectMutation
				}
			}
		}
		return nil
	}); err != nil {
		return store.Conversation{}, nil, nil, nil, err
	}

	return conversation, createdMessage, candidates, memberUserIDs, nil
}

func lockOwnedGroupConversationProjects(tx *gorm.DB, projectIDs []string, userID string) ([]store.Project, error) {
	projects := make([]store.Project, 0, len(projectIDs))
	for _, projectID := range projectIDs {
		var project store.Project
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Select("id", "owner_user_id", "is_personal").
			First(&project, "id = ?", projectID).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errGroupConversationProjectMissing
		}
		if err != nil {
			return nil, err
		}
		if project.OwnerUserID != userID {
			return nil, errGroupConversationProjectUnowned
		}
		if project.IsPersonal {
			return nil, errGroupConversationProjectPersonal
		}
		projects = append(projects, project)
	}
	return projects, nil
}

// addGroupConversationMembers godoc
//
// @Summary 添加群聊成员
// @Description 普通用户向自己参与的 active 群聊添加成员，并生成一条系统邀请消息。
// @Tags 客户端会话
// @Accept json
// @Produce json
// @Param conversation_id path string true "会话 ID"
// @Param body body addGroupConversationMembersRequest true "成员信息"
// @Success 200 {object} successEnvelope{data=addGroupConversationMembersResponse}
// @Failure 400 {object} errorEnvelope
// @Failure 401 {object} errorEnvelope
// @Failure 403 {object} errorEnvelope
// @Failure 404 {object} errorEnvelope
// @Failure 500 {object} errorEnvelope
// @Router /api/client/conversations/{conversation_id}/members [post]
func (s *Server) addGroupConversationMembers(c echo.Context) error {
	user, ok := currentUser(c)
	if !ok {
		return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
	}

	conversationID, err := normalizeMessageConversationID(c.Param("conversation_id"))
	if err != nil {
		return failure(c, http.StatusBadRequest, "invalid_request", err.Error())
	}

	var req addGroupConversationMembersRequest
	if err := c.Bind(&req); err != nil {
		return failure(c, http.StatusBadRequest, "invalid_request", "请求格式错误")
	}

	memberIDs, err := normalizeGroupMemberIDs(req.MemberIDs, user.ID)
	if err != nil {
		return failure(c, http.StatusBadRequest, "invalid_request", err.Error())
	}
	appIDs, err := normalizeGroupAppIDs(req.AppIDs)
	if err != nil {
		return failure(c, http.StatusBadRequest, "invalid_request", err.Error())
	}
	if len(memberIDs)+len(appIDs) == 0 {
		return failure(c, http.StatusBadRequest, "invalid_request", "至少选择一名成员或应用")
	}

	conversation, message, memberUserIDs, err := s.addGroupConversationMemberTargets(user, conversationID, memberIDs, appIDs)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return failure(c, http.StatusNotFound, "not_found", "会话不存在")
		}
		if errors.Is(err, errConversationAccessDenied) {
			return failure(c, http.StatusForbidden, "forbidden", "无权访问会话")
		}
		if errors.Is(err, errConversationNotGroup) {
			return failure(c, http.StatusBadRequest, "invalid_request", "只能向群聊添加成员")
		}
		if errors.Is(err, errGroupConversationMemberCap) {
			return failure(c, http.StatusBadRequest, "invalid_request", "群聊成员不能超过 500 人")
		}
		if errors.Is(err, errGroupConversationMemberMiss) {
			return failure(c, http.StatusBadRequest, "invalid_request", "成员或应用不存在或不可用")
		}

		return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
	}

	membersByConversationID, usersByID, appsByID, err := s.loadConversationListMembers([]string{conversation.ID})
	if err != nil {
		return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
	}

	var messageResponse *messageResponse
	if message != nil {
		response := newMessageResponse(*message)
		messageResponse = &response
		s.realtime.SendToUsers(memberUserIDs, realtimeMessageCreatedEvent(response))
	}

	return success(c, http.StatusOK, addGroupConversationMembersResponse{
		Conversation: newConversationListItemResponse(
			conversation,
			user.ID,
			membersByConversationID[conversation.ID],
			usersByID,
			appsByID,
		),
		Message: messageResponse,
	})
}

// removeGroupConversationMember godoc
//
// @Summary 移出群聊成员
// @Description 群主或管理员将成员移出 active 群聊，并生成系统消息。群主不能被移出。
// @Tags 客户端会话
// @Produce json
// @Param conversation_id path string true "会话 ID"
// @Param member_id path string true "成员用户 ID"
// @Success 200 {object} successEnvelope{data=addGroupConversationMembersResponse}
// @Failure 400 {object} errorEnvelope
// @Failure 401 {object} errorEnvelope
// @Failure 403 {object} errorEnvelope
// @Failure 404 {object} errorEnvelope
// @Failure 500 {object} errorEnvelope
// @Router /api/client/conversations/groups/{conversation_id}/members/{member_id} [delete]
func (s *Server) removeGroupConversationMember(c echo.Context) error {
	return s.removeGroupConversationMemberByType(c, store.ConversationMemberTypeUser)
}

// removeTypedGroupConversationMember godoc
//
// @Summary 移出群聊成员或应用
// @Description 群主或管理员将用户成员或应用成员移出 active 群聊，并生成系统消息。群主不能被移出。
// @Tags 客户端会话
// @Produce json
// @Param conversation_id path string true "会话 ID"
// @Param member_type path string true "成员类型 user|app"
// @Param member_id path string true "成员 ID"
// @Success 200 {object} successEnvelope{data=addGroupConversationMembersResponse}
// @Failure 400 {object} errorEnvelope
// @Failure 401 {object} errorEnvelope
// @Failure 403 {object} errorEnvelope
// @Failure 404 {object} errorEnvelope
// @Failure 500 {object} errorEnvelope
// @Router /api/client/conversations/groups/{conversation_id}/members/{member_type}/{member_id} [delete]
func (s *Server) removeTypedGroupConversationMember(c echo.Context) error {
	memberType := strings.TrimSpace(c.Param("member_type"))
	switch memberType {
	case store.ConversationMemberTypeUser, store.ConversationMemberTypeApp:
	default:
		return failure(c, http.StatusBadRequest, "invalid_request", "成员类型格式错误")
	}

	return s.removeGroupConversationMemberByType(c, memberType)
}

func (s *Server) removeGroupConversationMemberByType(c echo.Context, memberType string) error {
	user, ok := currentUser(c)
	if !ok {
		return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
	}
	conversationID, err := normalizeMessageConversationID(c.Param("conversation_id"))
	if err != nil {
		return failure(c, http.StatusBadRequest, "invalid_request", err.Error())
	}
	memberID, err := normalizeUUIDString(c.Param("member_id"), "成员 ID 格式错误")
	if err != nil {
		return failure(c, http.StatusBadRequest, "invalid_request", err.Error())
	}

	conversation, message, memberUserIDs, removedUserID, err := s.removeGroupConversationMemberTarget(user, conversationID, memberType, memberID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return failure(c, http.StatusNotFound, "not_found", "会话不存在或成员不在群聊中")
		}
		if errors.Is(err, errGroupConversationCannotRemoveSelf) {
			return failure(c, http.StatusForbidden, "forbidden", "不能移出自己")
		}
		if errors.Is(err, errGroupConversationOwnerCannotRemove) {
			return failure(c, http.StatusForbidden, "forbidden", "群主不能被移出群聊")
		}
		if errors.Is(err, errConversationAccessDenied) || errors.Is(err, errConversationNotGroup) {
			return failure(c, http.StatusForbidden, "forbidden", "无权操作群聊")
		}
		return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
	}

	membersByConversationID, usersByID, appsByID, err := s.loadConversationListMembers([]string{conversation.ID})
	if err != nil {
		return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
	}

	var messageResponse *messageResponse
	if message != nil {
		response := newMessageResponse(*message)
		messageResponse = &response
		s.realtime.SendToUsers(memberUserIDs, realtimeMessageCreatedEvent(response))
	}
	if removedUserID != "" {
		s.realtime.SendToUsers([]string{removedUserID}, realtimeConversationRemovedEvent(conversation.ID))
	}

	return success(c, http.StatusOK, addGroupConversationMembersResponse{
		Conversation: newConversationListItemResponse(
			conversation,
			user.ID,
			membersByConversationID[conversation.ID],
			usersByID,
			appsByID,
		),
		Message: messageResponse,
	})
}

// updateGroupConversationName godoc
//
// @Summary 修改群聊名称
// @Description 群主或管理员修改 active 群聊名称，并生成系统消息。
// @Tags 客户端会话
// @Accept json
// @Produce json
// @Param conversation_id path string true "会话 ID"
// @Param body body updateGroupConversationNameRequest true "群聊名称"
// @Success 200 {object} successEnvelope{data=addGroupConversationMembersResponse}
// @Failure 400 {object} errorEnvelope
// @Failure 401 {object} errorEnvelope
// @Failure 403 {object} errorEnvelope
// @Failure 404 {object} errorEnvelope
// @Failure 500 {object} errorEnvelope
// @Router /api/client/conversations/groups/{conversation_id}/name [patch]
func (s *Server) updateGroupConversationName(c echo.Context) error {
	user, ok := currentUser(c)
	if !ok {
		return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
	}
	conversationID, err := normalizeMessageConversationID(c.Param("conversation_id"))
	if err != nil {
		return failure(c, http.StatusBadRequest, "invalid_request", err.Error())
	}

	var req updateGroupConversationNameRequest
	if err := c.Bind(&req); err != nil {
		return failure(c, http.StatusBadRequest, "invalid_request", "请求格式错误")
	}
	name, err := normalizeGroupConversationName(req.Name)
	if err != nil {
		return failure(c, http.StatusBadRequest, "invalid_request", err.Error())
	}

	conversation, message, memberUserIDs, err := s.updateUserGroupConversationName(user, conversationID, name)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return failure(c, http.StatusNotFound, "not_found", "会话不存在")
		}
		if errors.Is(err, errConversationAccessDenied) || errors.Is(err, errConversationNotGroup) {
			return failure(c, http.StatusForbidden, "forbidden", "无权操作群聊")
		}
		return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
	}

	membersByConversationID, usersByID, appsByID, err := s.loadConversationListMembers([]string{conversation.ID})
	if err != nil {
		return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
	}

	var messageResponse *messageResponse
	if message != nil {
		response := newMessageResponse(*message)
		messageResponse = &response
		s.realtime.SendToUsers(memberUserIDs, realtimeMessageCreatedEvent(response))
	}

	return success(c, http.StatusOK, addGroupConversationMembersResponse{
		Conversation: newConversationListItemResponse(
			conversation,
			user.ID,
			membersByConversationID[conversation.ID],
			usersByID,
			appsByID,
		),
		Message: messageResponse,
	})
}

// setGroupConversationPublic godoc
//
// @Summary 设置公开群
// @Description 群主将 active 群聊设置为公开群，并生成系统消息。
// @Tags 客户端会话
// @Produce json
// @Param conversation_id path string true "会话 ID"
// @Success 200 {object} successEnvelope{data=addGroupConversationMembersResponse}
// @Failure 401 {object} errorEnvelope
// @Failure 403 {object} errorEnvelope
// @Failure 404 {object} errorEnvelope
// @Failure 500 {object} errorEnvelope
// @Router /api/client/conversations/groups/{conversation_id}/public [post]
func (s *Server) setGroupConversationPublic(c echo.Context) error {
	return s.setGroupConversationVisibility(c, store.ConversationVisibilityPublic)
}

// setGroupConversationPrivate godoc
//
// @Summary 取消公开群
// @Description 群主将 active 群聊设置为私有群，并生成系统消息。
// @Tags 客户端会话
// @Produce json
// @Param conversation_id path string true "会话 ID"
// @Success 200 {object} successEnvelope{data=addGroupConversationMembersResponse}
// @Failure 401 {object} errorEnvelope
// @Failure 403 {object} errorEnvelope
// @Failure 404 {object} errorEnvelope
// @Failure 500 {object} errorEnvelope
// @Router /api/client/conversations/groups/{conversation_id}/private [post]
func (s *Server) setGroupConversationPrivate(c echo.Context) error {
	return s.setGroupConversationVisibility(c, store.ConversationVisibilityPrivate)
}

func (s *Server) setGroupConversationVisibility(c echo.Context, visibility string) error {
	user, ok := currentUser(c)
	if !ok {
		return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
	}
	conversationID, err := normalizeMessageConversationID(c.Param("conversation_id"))
	if err != nil {
		return failure(c, http.StatusBadRequest, "invalid_request", err.Error())
	}

	conversation, message, memberUserIDs, err := s.updateUserGroupConversationVisibility(user, conversationID, visibility)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return failure(c, http.StatusNotFound, "not_found", "会话不存在")
		}
		if errors.Is(err, errConversationAccessDenied) || errors.Is(err, errConversationNotGroup) {
			return failure(c, http.StatusForbidden, "forbidden", "无权操作群聊")
		}
		return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
	}

	membersByConversationID, usersByID, appsByID, err := s.loadConversationListMembers([]string{conversation.ID})
	if err != nil {
		return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
	}

	var messageResponse *messageResponse
	if message != nil {
		response := newMessageResponse(*message)
		messageResponse = &response
		s.realtime.SendToUsers(memberUserIDs, realtimeMessageCreatedEvent(response))
	}

	return success(c, http.StatusOK, addGroupConversationMembersResponse{
		Conversation: newConversationListItemResponse(
			conversation,
			user.ID,
			membersByConversationID[conversation.ID],
			usersByID,
			appsByID,
		),
		Message: messageResponse,
	})
}

// joinPublicGroupConversation godoc
//
// @Summary 加入公开群
// @Description 普通用户加入 active 公开群，并生成系统消息。
// @Tags 客户端会话
// @Produce json
// @Param conversation_id path string true "会话 ID"
// @Success 200 {object} successEnvelope{data=addGroupConversationMembersResponse}
// @Failure 401 {object} errorEnvelope
// @Failure 403 {object} errorEnvelope
// @Failure 404 {object} errorEnvelope
// @Failure 500 {object} errorEnvelope
// @Router /api/client/conversations/groups/{conversation_id}/join [post]
func (s *Server) joinPublicGroupConversation(c echo.Context) error {
	user, ok := currentUser(c)
	if !ok {
		return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
	}
	conversationID, err := normalizeMessageConversationID(c.Param("conversation_id"))
	if err != nil {
		return failure(c, http.StatusBadRequest, "invalid_request", err.Error())
	}

	conversation, message, memberUserIDs, err := s.joinUserPublicGroupConversation(user, conversationID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return failure(c, http.StatusNotFound, "not_found", "会话不存在")
		}
		if errors.Is(err, errConversationAccessDenied) || errors.Is(err, errConversationNotGroup) {
			return failure(c, http.StatusForbidden, "forbidden", "无权加入群聊")
		}
		if errors.Is(err, errGroupConversationMemberCap) {
			return failure(c, http.StatusBadRequest, "invalid_request", "群聊成员不能超过 500 人")
		}
		return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
	}

	membersByConversationID, usersByID, appsByID, err := s.loadConversationListMembers([]string{conversation.ID})
	if err != nil {
		return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
	}

	var messageResponse *messageResponse
	if message != nil {
		response := newMessageResponse(*message)
		messageResponse = &response
		s.realtime.SendToUsers(memberUserIDs, realtimeMessageCreatedEvent(response))
	}

	return success(c, http.StatusOK, addGroupConversationMembersResponse{
		Conversation: newConversationListItemResponse(
			conversation,
			user.ID,
			membersByConversationID[conversation.ID],
			usersByID,
			appsByID,
		),
		Message: messageResponse,
	})
}

// leaveGroupConversation godoc
//
// @Summary 退出群聊
// @Description 当前用户退出 active 群聊，并生成系统消息。群主不能退出群聊。
// @Tags 客户端会话
// @Produce json
// @Param conversation_id path string true "会话 ID"
// @Success 200 {object} successEnvelope{data=leaveGroupConversationResponse}
// @Failure 400 {object} errorEnvelope
// @Failure 401 {object} errorEnvelope
// @Failure 403 {object} errorEnvelope
// @Failure 404 {object} errorEnvelope
// @Failure 500 {object} errorEnvelope
// @Router /api/client/conversations/groups/{conversation_id}/leave [post]
func (s *Server) leaveGroupConversation(c echo.Context) error {
	user, ok := currentUser(c)
	if !ok {
		return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
	}
	conversationID, err := normalizeMessageConversationID(c.Param("conversation_id"))
	if err != nil {
		return failure(c, http.StatusBadRequest, "invalid_request", err.Error())
	}

	message, memberUserIDs, err := s.leaveUserGroupConversation(user, conversationID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return failure(c, http.StatusNotFound, "not_found", "会话不存在")
		}
		if errors.Is(err, errGroupConversationOwnerCannotLeave) {
			return failure(c, http.StatusForbidden, "forbidden", "群主不能退出群聊")
		}
		if errors.Is(err, errConversationAccessDenied) || errors.Is(err, errConversationNotGroup) {
			return failure(c, http.StatusForbidden, "forbidden", "无权操作群聊")
		}
		return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
	}

	response := newMessageResponse(message)
	s.realtime.SendToUsers(memberUserIDs, realtimeMessageCreatedEvent(response))

	return success(c, http.StatusOK, leaveGroupConversationResponse{
		ConversationID: conversationID,
		Message:        response,
	})
}

// dissolveGroupConversation godoc
//
// @Summary 解散群聊
// @Description 群主解散 active 群聊。解散后所有成员将不再看到该群聊。
// @Tags 客户端会话
// @Produce json
// @Param conversation_id path string true "会话 ID"
// @Success 200 {object} successEnvelope{data=dissolveGroupConversationResponse}
// @Failure 400 {object} errorEnvelope
// @Failure 401 {object} errorEnvelope
// @Failure 403 {object} errorEnvelope
// @Failure 404 {object} errorEnvelope
// @Failure 500 {object} errorEnvelope
// @Router /api/client/conversations/groups/{conversation_id} [delete]
func (s *Server) dissolveGroupConversation(c echo.Context) error {
	user, ok := currentUser(c)
	if !ok {
		return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
	}
	conversationID, err := normalizeMessageConversationID(c.Param("conversation_id"))
	if err != nil {
		return failure(c, http.StatusBadRequest, "invalid_request", err.Error())
	}

	memberUserIDs, err := s.dissolveUserGroupConversation(user, conversationID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return failure(c, http.StatusNotFound, "not_found", "会话不存在")
		}
		if errors.Is(err, errConversationAccessDenied) || errors.Is(err, errConversationNotGroup) {
			return failure(c, http.StatusForbidden, "forbidden", "无权操作群聊")
		}
		return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
	}

	s.realtime.SendToUsers(memberUserIDs, realtimeConversationRemovedEvent(conversationID))

	return success(c, http.StatusOK, dissolveGroupConversationResponse{
		ConversationID: conversationID,
	})
}

func canManageGroupConversation(role string) bool {
	return role == store.ConversationMemberRoleOwner || role == store.ConversationMemberRoleAdmin
}

func canSetGroupVisibility(role string) bool {
	return role == store.ConversationMemberRoleOwner
}

func normalizeUUIDString(rawID string, message string) (string, error) {
	id := strings.TrimSpace(rawID)
	if id == "" {
		return "", errors.New(message)
	}
	parsedID, err := uuid.Parse(id)
	if err != nil {
		return "", errors.New(message)
	}

	return parsedID.String(), nil
}

func normalizeDirectConversationUserID(rawID string, currentUserID string) (string, error) {
	id := strings.TrimSpace(rawID)
	if id == "" {
		return "", errors.New("用户 ID 不能为空")
	}

	parsedID, err := uuid.Parse(id)
	if err != nil {
		return "", errors.New("用户 ID 格式错误")
	}
	parsedCurrentUserID, err := uuid.Parse(currentUserID)
	if err != nil {
		return "", errors.New("当前用户 ID 格式错误")
	}
	if parsedID == parsedCurrentUserID {
		return "", errors.New("不能和自己创建私聊")
	}

	return parsedID.String(), nil
}

func (s *Server) getOrCreateDirectConversation(currentUser store.User, targetUser store.User) (store.Conversation, bool, error) {
	userLowID, userHighID := orderDirectConversationUserIDs(currentUser.ID, targetUser.ID)

	existing, err := findDirectConversationByUserPair(s.db, userLowID, userHighID)
	if err == nil {
		return existing, false, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return store.Conversation{}, false, err
	}

	now := time.Now().UTC()
	conversation := store.Conversation{
		ID:              uuid.NewString(),
		Kind:            store.ConversationKindDirect,
		Name:            "",
		CreatedByUserID: currentUser.ID,
		Status:          store.ConversationStatusActive,
		PostingPolicy:   store.ConversationPostingPolicyOpen,
		Visibility:      store.ConversationVisibilityPrivate,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	var created bool
	err = s.db.Transaction(func(tx *gorm.DB) error {
		existing, err := findDirectConversationByUserPair(tx, userLowID, userHighID)
		if err == nil {
			conversation = existing
			created = false
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		if err := tx.Create(&conversation).Error; err != nil {
			return err
		}

		members := []store.ConversationMember{
			{
				ConversationID:        conversation.ID,
				MemberType:            store.ConversationMemberTypeUser,
				MemberID:              currentUser.ID,
				Role:                  store.ConversationMemberRoleOwner,
				JoinedAt:              now,
				HistoryVisibleFromSeq: 1,
			},
			{
				ConversationID:        conversation.ID,
				MemberType:            store.ConversationMemberTypeUser,
				MemberID:              targetUser.ID,
				Role:                  store.ConversationMemberRoleMember,
				JoinedAt:              now,
				HistoryVisibleFromSeq: 1,
			},
		}
		if err := tx.Create(&members).Error; err != nil {
			return err
		}

		direct := store.DirectConversation{
			ConversationID: conversation.ID,
			UserLowID:      userLowID,
			UserHighID:     userHighID,
			CreatedAt:      now,
		}
		if err := tx.Create(&direct).Error; err != nil {
			return err
		}

		created = true
		return nil
	})
	if err != nil {
		if isUniqueConstraintError(err) {
			existing, findErr := findDirectConversationByUserPair(s.db, userLowID, userHighID)
			if findErr == nil {
				return existing, false, nil
			}
		}
		return store.Conversation{}, false, err
	}

	return conversation, created, nil
}

func findDirectConversationByUserPair(db *gorm.DB, userLowID string, userHighID string) (store.Conversation, error) {
	var direct store.DirectConversation
	if err := db.First(&direct, "user_low_id = ? AND user_high_id = ?", userLowID, userHighID).Error; err != nil {
		return store.Conversation{}, err
	}

	var conversation store.Conversation
	if err := db.First(&conversation, "id = ?", direct.ConversationID).Error; err != nil {
		return store.Conversation{}, err
	}

	return conversation, nil
}

func (s *Server) findVisibleClientApp(appID string, currentUserID string) (store.App, bool, error) {
	if appregistry.IsAIAssistantAppID(appID) {
		if _, err := appregistry.EnsureAIAssistantApp(s.db, s.cfg.Apps); err != nil {
			return store.App{}, false, err
		}
	}

	var app store.App
	err := s.db.
		Where("id = ? AND enabled = ?", appID, true).
		Where(
			"visibility = ? OR (visibility = ? AND creator_user_id = ?)",
			store.AppVisibilityPublic,
			store.AppVisibilityCreator,
			currentUserID,
		).
		First(&app).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return store.App{}, false, nil
	}
	if err != nil {
		return store.App{}, false, err
	}

	return app, true, nil
}

func (s *Server) getOrCreateAppConversation(currentUser store.User, app store.App) (store.Conversation, bool, error) {
	existing, err := findAppConversationByUserAndApp(s.db, app.ID, currentUser.ID)
	if err == nil {
		return existing, false, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return store.Conversation{}, false, err
	}

	now := time.Now().UTC()
	conversation := store.Conversation{
		ID:              uuid.NewString(),
		Kind:            store.ConversationKindApp,
		Name:            app.Name,
		Avatar:          app.Avatar,
		CreatedByUserID: currentUser.ID,
		Status:          store.ConversationStatusActive,
		PostingPolicy:   store.ConversationPostingPolicyOpen,
		Visibility:      store.ConversationVisibilityPrivate,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	var created bool
	err = s.db.Transaction(func(tx *gorm.DB) error {
		existing, err := findAppConversationByUserAndApp(tx, app.ID, currentUser.ID)
		if err == nil {
			conversation = existing
			created = false
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		if err := tx.Create(&conversation).Error; err != nil {
			return err
		}

		members := []store.ConversationMember{
			{
				ConversationID:        conversation.ID,
				MemberType:            store.ConversationMemberTypeUser,
				MemberID:              currentUser.ID,
				Role:                  store.ConversationMemberRoleOwner,
				JoinedAt:              now,
				HistoryVisibleFromSeq: 1,
			},
			{
				ConversationID:        conversation.ID,
				MemberType:            store.ConversationMemberTypeApp,
				MemberID:              app.ID,
				Role:                  store.ConversationMemberRoleMember,
				JoinedAt:              now,
				HistoryVisibleFromSeq: 1,
			},
		}
		if err := tx.Create(&members).Error; err != nil {
			return err
		}

		appConversation := store.AppConversation{
			AppID:          app.ID,
			UserID:         currentUser.ID,
			ConversationID: conversation.ID,
			CreatedAt:      now,
		}
		if err := tx.Create(&appConversation).Error; err != nil {
			return err
		}

		created = true
		return nil
	})
	if err != nil {
		if isUniqueConstraintError(err) {
			existing, findErr := findAppConversationByUserAndApp(s.db, app.ID, currentUser.ID)
			if findErr == nil {
				return existing, false, nil
			}
		}
		return store.Conversation{}, false, err
	}

	return conversation, created, nil
}

func findAppConversationByUserAndApp(db *gorm.DB, appID string, userID string) (store.Conversation, error) {
	var appConversation store.AppConversation
	if err := db.First(&appConversation, "app_id = ? AND user_id = ?", appID, userID).Error; err != nil {
		return store.Conversation{}, err
	}

	var conversation store.Conversation
	if err := db.First(&conversation, "id = ?", appConversation.ConversationID).Error; err != nil {
		return store.Conversation{}, err
	}

	return conversation, nil
}

func (s *Server) markUserConversationRead(userID string, conversationID string, upToSeq *int64) (markConversationReadResponse, error) {
	var response markConversationReadResponse

	err := s.db.Transaction(func(tx *gorm.DB) error {
		var conversation store.Conversation
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&conversation, "id = ?", conversationID).Error; err != nil {
			return err
		}
		if conversation.Status != store.ConversationStatusActive {
			return errConversationAccessDenied
		}

		var member store.ConversationMember
		if err := tx.First(
			&member,
			"conversation_id = ? AND member_type = ? AND member_id = ? AND left_at IS NULL",
			conversationID,
			store.ConversationMemberTypeUser,
			userID,
		).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errConversationAccessDenied
			}
			return err
		}

		targetSeq := conversation.LastMessageSeq
		if upToSeq != nil && *upToSeq < targetSeq {
			targetSeq = *upToSeq
		}
		if err := advanceConversationMemberReadSeq(tx, conversationID, userID, targetSeq); err != nil {
			return err
		}
		if targetSeq > member.LastReadSeq {
			member.LastReadSeq = targetSeq
		}

		response = markConversationReadResponse{
			ConversationID: conversationID,
			LastReadSeq:    member.LastReadSeq,
			UnreadCount:    unreadCount(conversation.LastMessageSeq, member.LastReadSeq),
		}
		return nil
	})
	if err != nil {
		return markConversationReadResponse{}, err
	}

	return response, nil
}

func (s *Server) addUserGroupConversationMembers(currentUser store.User, conversationID string, memberIDs []string) (store.Conversation, *store.Message, []string, error) {
	return s.addGroupConversationMemberTargets(currentUser, conversationID, memberIDs, nil)
}

func (s *Server) addGroupConversationMemberTargets(currentUser store.User, conversationID string, memberIDs []string, appIDs []string) (store.Conversation, *store.Message, []string, error) {
	var conversation store.Conversation
	var message *store.Message
	memberUserIDs := []string{}

	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&conversation, "id = ?", conversationID).Error; err != nil {
			return err
		}
		if conversation.Status != store.ConversationStatusActive {
			return errConversationAccessDenied
		}
		if conversation.Kind != store.ConversationKindGroup {
			return errConversationNotGroup
		}

		var currentMember store.ConversationMember
		if err := tx.First(
			&currentMember,
			"conversation_id = ? AND member_type = ? AND member_id = ? AND left_at IS NULL",
			conversationID,
			store.ConversationMemberTypeUser,
			currentUser.ID,
		).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errConversationAccessDenied
			}
			return err
		}

		var existingMembers []store.ConversationMember
		if err := tx.
			Where("conversation_id = ?", conversationID).
			Find(&existingMembers).Error; err != nil {
			return err
		}

		activeMemberCount := 0
		membersByID := make(map[string]store.ConversationMember, len(existingMembers))
		for _, member := range existingMembers {
			if member.LeftAt == nil {
				activeMemberCount++
			}
			membersByID[conversationMemberMentionKey(member.MemberType, member.MemberID)] = member
		}

		newMemberIDs := make([]string, 0, len(memberIDs))
		reactivatedMemberIDs := make([]string, 0, len(memberIDs))
		addedMemberIDs := make([]string, 0, len(memberIDs))
		for _, memberID := range memberIDs {
			existingMember, ok := membersByID[conversationMemberMentionKey(store.ConversationMemberTypeUser, memberID)]
			if ok && existingMember.LeftAt == nil {
				continue
			}
			addedMemberIDs = append(addedMemberIDs, memberID)
			if ok {
				reactivatedMemberIDs = append(reactivatedMemberIDs, memberID)
				continue
			}
			newMemberIDs = append(newMemberIDs, memberID)
		}

		newAppIDs := make([]string, 0, len(appIDs))
		reactivatedAppIDs := make([]string, 0, len(appIDs))
		addedAppIDs := make([]string, 0, len(appIDs))
		for _, appID := range appIDs {
			existingMember, ok := membersByID[conversationMemberMentionKey(store.ConversationMemberTypeApp, appID)]
			if ok && existingMember.LeftAt == nil {
				continue
			}
			addedAppIDs = append(addedAppIDs, appID)
			if ok {
				reactivatedAppIDs = append(reactivatedAppIDs, appID)
				continue
			}
			newAppIDs = append(newAppIDs, appID)
		}
		if len(addedMemberIDs)+len(addedAppIDs) == 0 {
			return nil
		}
		if activeMemberCount+len(newMemberIDs)+len(reactivatedMemberIDs)+len(newAppIDs)+len(reactivatedAppIDs) > maxGroupConversationMembers {
			return errGroupConversationMemberCap
		}

		addedUsers, err := loadActiveGroupMembers(tx, addedMemberIDs)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errGroupConversationMemberMiss
			}
			return err
		}
		addedApps, err := loadVisibleGroupApps(tx, currentUser.ID, addedAppIDs)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errGroupConversationMemberMiss
			}
			return err
		}

		now := time.Now().UTC()
		systemMessageSeq := conversation.LastMessageSeq + 1
		if len(reactivatedMemberIDs) > 0 {
			if err := tx.Model(&store.ConversationMember{}).
				Where("conversation_id = ? AND member_type = ? AND member_id IN ?", conversationID, store.ConversationMemberTypeUser, reactivatedMemberIDs).
				Updates(map[string]any{
					"role":                     store.ConversationMemberRoleMember,
					"joined_at":                now,
					"history_visible_from_seq": systemMessageSeq,
					"left_at":                  nil,
					"last_read_seq":            conversation.LastMessageSeq,
				}).Error; err != nil {
				return err
			}
		}
		if len(reactivatedAppIDs) > 0 {
			if err := tx.Model(&store.ConversationMember{}).
				Where("conversation_id = ? AND member_type = ? AND member_id IN ?", conversationID, store.ConversationMemberTypeApp, reactivatedAppIDs).
				Updates(map[string]any{
					"role":                     store.ConversationMemberRoleMember,
					"joined_at":                now,
					"history_visible_from_seq": systemMessageSeq,
					"left_at":                  nil,
					"last_read_seq":            conversation.LastMessageSeq,
				}).Error; err != nil {
				return err
			}
		}

		newUsersByID := make(map[string]store.User, len(addedUsers))
		for _, addedUser := range addedUsers {
			newUsersByID[addedUser.ID] = addedUser
		}
		newAppsByID := make(map[string]store.App, len(addedApps))
		for _, addedApp := range addedApps {
			newAppsByID[addedApp.ID] = addedApp
		}
		conversationMembers := make([]store.ConversationMember, 0, len(newMemberIDs)+len(newAppIDs))
		for _, newMemberID := range newMemberIDs {
			newUser := newUsersByID[newMemberID]
			conversationMembers = append(conversationMembers, store.ConversationMember{
				ConversationID:        conversationID,
				MemberType:            store.ConversationMemberTypeUser,
				MemberID:              newUser.ID,
				Role:                  store.ConversationMemberRoleMember,
				JoinedAt:              now,
				HistoryVisibleFromSeq: systemMessageSeq,
				LastReadSeq:           conversation.LastMessageSeq,
			})
		}
		for _, newAppID := range newAppIDs {
			newApp := newAppsByID[newAppID]
			conversationMembers = append(conversationMembers, store.ConversationMember{
				ConversationID:        conversationID,
				MemberType:            store.ConversationMemberTypeApp,
				MemberID:              newApp.ID,
				Role:                  store.ConversationMemberRoleMember,
				JoinedAt:              now,
				HistoryVisibleFromSeq: systemMessageSeq,
				LastReadSeq:           conversation.LastMessageSeq,
			})
		}
		if len(conversationMembers) > 0 {
			if err := tx.Create(&conversationMembers).Error; err != nil {
				return err
			}
		}

		invitees := makeGroupMemberInviteeRefs(addedUsers, addedApps)
		createdMessage, err := createGroupMembersInvitedSystemMessage(tx, &conversation, currentUser, invitees, now)
		if err != nil {
			return err
		}
		message = &createdMessage

		if err := advanceConversationMemberReadSeq(tx, conversationID, currentUser.ID, createdMessage.Seq); err != nil {
			return err
		}

		ids, err := loadActiveConversationUserIDs(tx, conversationID)
		if err != nil {
			return err
		}
		memberUserIDs = ids
		return nil
	})
	if err != nil {
		return store.Conversation{}, nil, nil, err
	}

	return conversation, message, memberUserIDs, nil
}

func (s *Server) updateUserGroupConversationVisibility(currentUser store.User, conversationID string, visibility string) (store.Conversation, *store.Message, []string, error) {
	var conversation store.Conversation
	var message *store.Message
	memberUserIDs := []string{}

	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&conversation, "id = ?", conversationID).Error; err != nil {
			return err
		}
		if conversation.Status != store.ConversationStatusActive {
			return errConversationAccessDenied
		}
		if conversation.Kind != store.ConversationKindGroup {
			return errConversationNotGroup
		}

		var currentMember store.ConversationMember
		if err := tx.First(
			&currentMember,
			"conversation_id = ? AND member_type = ? AND member_id = ? AND left_at IS NULL",
			conversationID,
			store.ConversationMemberTypeUser,
			currentUser.ID,
		).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errConversationAccessDenied
			}
			return err
		}
		if !canSetGroupVisibility(currentMember.Role) {
			return errConversationAccessDenied
		}
		if conversation.Visibility == visibility {
			ids, err := loadActiveConversationUserIDs(tx, conversationID)
			if err != nil {
				return err
			}
			memberUserIDs = ids
			return nil
		}

		now := time.Now().UTC()
		if err := tx.Model(&store.Conversation{}).
			Where("id = ?", conversationID).
			Updates(map[string]any{
				"visibility": visibility,
				"updated_at": now,
			}).Error; err != nil {
			return err
		}
		conversation.Visibility = visibility
		conversation.UpdatedAt = now

		createdMessage, err := createGroupVisibilityChangedSystemMessage(tx, &conversation, currentUser, visibility, now)
		if err != nil {
			return err
		}
		message = &createdMessage
		if err := advanceConversationMemberReadSeq(tx, conversationID, currentUser.ID, createdMessage.Seq); err != nil {
			return err
		}

		ids, err := loadActiveConversationUserIDs(tx, conversationID)
		if err != nil {
			return err
		}
		memberUserIDs = ids
		return nil
	})
	if err != nil {
		return store.Conversation{}, nil, nil, err
	}

	return conversation, message, memberUserIDs, nil
}

func (s *Server) updateUserGroupConversationName(currentUser store.User, conversationID string, name string) (store.Conversation, *store.Message, []string, error) {
	var conversation store.Conversation
	var message *store.Message
	memberUserIDs := []string{}

	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&conversation, "id = ?", conversationID).Error; err != nil {
			return err
		}
		if conversation.Status != store.ConversationStatusActive {
			return errConversationAccessDenied
		}
		if conversation.Kind != store.ConversationKindGroup {
			return errConversationNotGroup
		}

		var currentMember store.ConversationMember
		if err := tx.First(
			&currentMember,
			"conversation_id = ? AND member_type = ? AND member_id = ? AND left_at IS NULL",
			conversationID,
			store.ConversationMemberTypeUser,
			currentUser.ID,
		).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errConversationAccessDenied
			}
			return err
		}
		if !canManageGroupConversation(currentMember.Role) {
			return errConversationAccessDenied
		}
		if conversation.Name == name {
			ids, err := loadActiveConversationUserIDs(tx, conversationID)
			if err != nil {
				return err
			}
			memberUserIDs = ids
			return nil
		}

		now := time.Now().UTC()
		if err := tx.Model(&store.Conversation{}).
			Where("id = ?", conversationID).
			Updates(map[string]any{
				"name":       name,
				"updated_at": now,
			}).Error; err != nil {
			return err
		}
		conversation.Name = name
		conversation.UpdatedAt = now

		createdMessage, err := createGroupNameUpdatedSystemMessage(tx, &conversation, currentUser, name, now)
		if err != nil {
			return err
		}
		message = &createdMessage
		if err := advanceConversationMemberReadSeq(tx, conversationID, currentUser.ID, createdMessage.Seq); err != nil {
			return err
		}

		ids, err := loadActiveConversationUserIDs(tx, conversationID)
		if err != nil {
			return err
		}
		memberUserIDs = ids
		return nil
	})
	if err != nil {
		return store.Conversation{}, nil, nil, err
	}

	return conversation, message, memberUserIDs, nil
}

func (s *Server) joinUserPublicGroupConversation(currentUser store.User, conversationID string) (store.Conversation, *store.Message, []string, error) {
	var conversation store.Conversation
	var message *store.Message
	memberUserIDs := []string{}

	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&conversation, "id = ?", conversationID).Error; err != nil {
			return err
		}
		if conversation.Status != store.ConversationStatusActive {
			return errConversationAccessDenied
		}
		if conversation.Kind != store.ConversationKindGroup {
			return errConversationNotGroup
		}
		if conversation.Visibility != store.ConversationVisibilityPublic {
			return errConversationAccessDenied
		}

		var existingMembers []store.ConversationMember
		if err := tx.
			Where("conversation_id = ? AND member_type = ?", conversationID, store.ConversationMemberTypeUser).
			Find(&existingMembers).Error; err != nil {
			return err
		}

		activeMemberCount := 0
		var existingCurrentMember *store.ConversationMember
		for index := range existingMembers {
			member := &existingMembers[index]
			if member.LeftAt == nil {
				activeMemberCount++
			}
			if member.MemberID == currentUser.ID {
				existingCurrentMember = member
			}
		}

		if existingCurrentMember != nil && existingCurrentMember.LeftAt == nil {
			ids, err := loadActiveConversationUserIDs(tx, conversationID)
			if err != nil {
				return err
			}
			memberUserIDs = ids
			return nil
		}
		if activeMemberCount >= maxGroupConversationMembers {
			return errGroupConversationMemberCap
		}

		now := time.Now().UTC()
		systemMessageSeq := conversation.LastMessageSeq + 1
		if existingCurrentMember != nil {
			if err := tx.Model(&store.ConversationMember{}).
				Where("conversation_id = ? AND member_type = ? AND member_id = ?", conversationID, store.ConversationMemberTypeUser, currentUser.ID).
				Updates(map[string]any{
					"role":                     store.ConversationMemberRoleMember,
					"joined_at":                now,
					"history_visible_from_seq": systemMessageSeq,
					"left_at":                  nil,
					"last_read_seq":            systemMessageSeq,
				}).Error; err != nil {
				return err
			}
		} else {
			member := store.ConversationMember{
				ConversationID:        conversationID,
				MemberType:            store.ConversationMemberTypeUser,
				MemberID:              currentUser.ID,
				Role:                  store.ConversationMemberRoleMember,
				JoinedAt:              now,
				HistoryVisibleFromSeq: systemMessageSeq,
				LastReadSeq:           systemMessageSeq,
			}
			if err := tx.Create(&member).Error; err != nil {
				return err
			}
		}

		createdMessage, err := createGroupMemberJoinedSystemMessage(tx, &conversation, currentUser, now)
		if err != nil {
			return err
		}
		message = &createdMessage

		ids, err := loadActiveConversationUserIDs(tx, conversationID)
		if err != nil {
			return err
		}
		memberUserIDs = ids
		return nil
	})
	if err != nil {
		return store.Conversation{}, nil, nil, err
	}

	return conversation, message, memberUserIDs, nil
}

func (s *Server) removeUserGroupConversationMember(currentUser store.User, conversationID string, targetUserID string) (store.Conversation, *store.Message, []string, string, error) {
	return s.removeGroupConversationMemberTarget(currentUser, conversationID, store.ConversationMemberTypeUser, targetUserID)
}

func (s *Server) removeGroupConversationMemberTarget(currentUser store.User, conversationID string, targetMemberType string, targetMemberID string) (store.Conversation, *store.Message, []string, string, error) {
	var conversation store.Conversation
	var message *store.Message
	memberUserIDs := []string{}
	removedUserID := ""

	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&conversation, "id = ?", conversationID).Error; err != nil {
			return err
		}
		if conversation.Status != store.ConversationStatusActive {
			return errConversationAccessDenied
		}
		if conversation.Kind != store.ConversationKindGroup {
			return errConversationNotGroup
		}
		if targetMemberType == store.ConversationMemberTypeUser && currentUser.ID == targetMemberID {
			return errGroupConversationCannotRemoveSelf
		}

		var currentMember store.ConversationMember
		if err := tx.First(
			&currentMember,
			"conversation_id = ? AND member_type = ? AND member_id = ? AND left_at IS NULL",
			conversationID,
			store.ConversationMemberTypeUser,
			currentUser.ID,
		).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errConversationAccessDenied
			}
			return err
		}
		if !canManageGroupConversation(currentMember.Role) {
			return errConversationAccessDenied
		}

		var targetMember store.ConversationMember
		if err := tx.First(
			&targetMember,
			"conversation_id = ? AND member_type = ? AND member_id = ? AND left_at IS NULL",
			conversationID,
			targetMemberType,
			targetMemberID,
		).Error; err != nil {
			return err
		}
		if targetMember.Role == store.ConversationMemberRoleOwner {
			return errGroupConversationOwnerCannotRemove
		}

		targetRef, err := loadGroupMemberSystemRef(tx, targetMemberType, targetMemberID)
		if err != nil {
			return err
		}

		now := time.Now().UTC()
		if err := tx.Model(&store.ConversationMember{}).
			Where("conversation_id = ? AND member_type = ? AND member_id = ?", conversationID, targetMemberType, targetMemberID).
			Updates(map[string]any{
				"left_at": now,
			}).Error; err != nil {
			return err
		}

		createdMessage, err := createGroupMemberRemovedSystemMessage(tx, &conversation, currentUser, targetRef, now)
		if err != nil {
			return err
		}
		message = &createdMessage
		if err := advanceConversationMemberReadSeq(tx, conversationID, currentUser.ID, createdMessage.Seq); err != nil {
			return err
		}

		ids, err := loadActiveConversationUserIDs(tx, conversationID)
		if err != nil {
			return err
		}
		memberUserIDs = ids
		if targetMemberType == store.ConversationMemberTypeUser {
			removedUserID = targetMemberID
		}
		return nil
	})
	if err != nil {
		return store.Conversation{}, nil, nil, "", err
	}

	return conversation, message, memberUserIDs, removedUserID, nil
}

func (s *Server) leaveUserGroupConversation(currentUser store.User, conversationID string) (store.Message, []string, error) {
	var message store.Message
	memberUserIDs := []string{}

	err := s.db.Transaction(func(tx *gorm.DB) error {
		var conversation store.Conversation
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&conversation, "id = ?", conversationID).Error; err != nil {
			return err
		}
		if conversation.Status != store.ConversationStatusActive {
			return errConversationAccessDenied
		}
		if conversation.Kind != store.ConversationKindGroup {
			return errConversationNotGroup
		}

		var currentMember store.ConversationMember
		if err := tx.First(
			&currentMember,
			"conversation_id = ? AND member_type = ? AND member_id = ? AND left_at IS NULL",
			conversationID,
			store.ConversationMemberTypeUser,
			currentUser.ID,
		).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errConversationAccessDenied
			}
			return err
		}
		if currentMember.Role == store.ConversationMemberRoleOwner {
			return errGroupConversationOwnerCannotLeave
		}

		now := time.Now().UTC()
		if err := tx.Model(&store.ConversationMember{}).
			Where("conversation_id = ? AND member_type = ? AND member_id = ?", conversationID, store.ConversationMemberTypeUser, currentUser.ID).
			Updates(map[string]any{
				"left_at": now,
			}).Error; err != nil {
			return err
		}

		createdMessage, err := createGroupMemberLeftSystemMessage(tx, &conversation, currentUser, now)
		if err != nil {
			return err
		}
		message = createdMessage

		ids, err := loadActiveConversationUserIDs(tx, conversationID)
		if err != nil {
			return err
		}
		memberUserIDs = ids
		return nil
	})
	if err != nil {
		return store.Message{}, nil, err
	}

	return message, memberUserIDs, nil
}

func (s *Server) dissolveUserGroupConversation(currentUser store.User, conversationID string) ([]string, error) {
	memberUserIDs := []string{}

	for {
		err := s.db.Transaction(func(tx *gorm.DB) error {
			projectIDs, err := loadGroupConversationProjectIDs(tx, conversationID)
			if err != nil {
				return err
			}
			projectsByID, err := lockGroupConversationProjectsForDissolution(tx, projectIDs)
			if err != nil {
				return err
			}

			var conversation store.Conversation
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&conversation, "id = ?", conversationID).Error; err != nil {
				return err
			}

			currentProjectIDs, err := loadGroupConversationProjectIDs(tx, conversationID)
			if err != nil {
				return err
			}
			if containsAdditionalProjectID(projectIDs, currentProjectIDs) {
				return errGroupConversationProjectLockChange
			}

			if conversation.Status != store.ConversationStatusActive {
				return errConversationAccessDenied
			}
			if conversation.Kind != store.ConversationKindGroup {
				return errConversationNotGroup
			}

			var currentMember store.ConversationMember
			if err := tx.First(
				&currentMember,
				"conversation_id = ? AND member_type = ? AND member_id = ? AND left_at IS NULL",
				conversationID,
				store.ConversationMemberTypeUser,
				currentUser.ID,
			).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return errConversationAccessDenied
				}
				return err
			}
			if currentMember.Role != store.ConversationMemberRoleOwner {
				return errConversationAccessDenied
			}

			ids, err := loadActiveConversationUserIDs(tx, conversationID)
			if err != nil {
				return err
			}
			memberUserIDs = ids

			now := time.Now().UTC()
			deleteResult := tx.Where("conversation_id = ?", conversationID).Delete(&store.ProjectGroup{})
			if deleteResult.Error != nil {
				return deleteResult.Error
			}
			if deleteResult.RowsAffected != int64(len(currentProjectIDs)) {
				return errGroupConversationProjectMutation
			}

			for _, projectID := range currentProjectIDs {
				project, exists := projectsByID[projectID]
				if !exists || project.DeletedAt.Valid {
					continue
				}
				updateResult := tx.Model(&store.Project{}).
					Where("id = ?", projectID).
					Update("updated_at", now)
				if updateResult.Error != nil {
					return updateResult.Error
				}
				if updateResult.RowsAffected != 1 {
					return errGroupConversationProjectMutation
				}
			}

			updateResult := tx.Model(&store.Conversation{}).
				Where("id = ?", conversationID).
				Updates(map[string]any{
					"dissolved_at": now,
					"status":       store.ConversationStatusDissolved,
					"updated_at":   now,
				})
			if updateResult.Error != nil {
				return updateResult.Error
			}
			if updateResult.RowsAffected != 1 {
				return errGroupConversationProjectMutation
			}
			return nil
		})
		if errors.Is(err, errGroupConversationProjectLockChange) {
			continue
		}
		if err != nil {
			return nil, err
		}
		return memberUserIDs, nil
	}
}

func loadGroupConversationProjectIDs(tx *gorm.DB, conversationID string) ([]string, error) {
	projectIDs := []string{}
	if err := tx.Model(&store.ProjectGroup{}).
		Where("conversation_id = ?", conversationID).
		Pluck("project_id", &projectIDs).Error; err != nil {
		return nil, err
	}
	sort.Strings(projectIDs)
	return projectIDs, nil
}

func lockGroupConversationProjectsForDissolution(tx *gorm.DB, projectIDs []string) (map[string]store.Project, error) {
	projectsByID := make(map[string]store.Project, len(projectIDs))
	for _, projectID := range projectIDs {
		var project store.Project
		err := tx.Unscoped().Clauses(clause.Locking{Strength: "UPDATE"}).
			Select("id", "deleted_at").
			First(&project, "id = ?", projectID).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		projectsByID[project.ID] = project
	}
	return projectsByID, nil
}

func containsAdditionalProjectID(lockedProjectIDs []string, currentProjectIDs []string) bool {
	locked := make(map[string]struct{}, len(lockedProjectIDs))
	for _, projectID := range lockedProjectIDs {
		locked[projectID] = struct{}{}
	}
	for _, projectID := range currentProjectIDs {
		if _, exists := locked[projectID]; !exists {
			return true
		}
	}
	return false
}

func (s *Server) updateUserGroupConversationAvatar(currentUser store.User, conversationID string, avatarURL string) (store.Conversation, store.Message, []string, error) {
	var conversation store.Conversation
	var message store.Message
	memberUserIDs := []string{}

	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&conversation, "id = ?", conversationID).Error; err != nil {
			return err
		}
		if conversation.Status != store.ConversationStatusActive {
			return errConversationAccessDenied
		}
		if conversation.Kind != store.ConversationKindGroup {
			return errConversationNotGroup
		}

		var currentMember store.ConversationMember
		if err := tx.First(
			&currentMember,
			"conversation_id = ? AND member_type = ? AND member_id = ? AND left_at IS NULL",
			conversationID,
			store.ConversationMemberTypeUser,
			currentUser.ID,
		).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errConversationAccessDenied
			}
			return err
		}
		if !canManageGroupConversation(currentMember.Role) {
			return errGroupConversationAvatarForbidden
		}

		now := time.Now().UTC()
		if err := tx.Model(&store.Conversation{}).
			Where("id = ?", conversationID).
			Updates(map[string]any{
				"avatar":     avatarURL,
				"updated_at": now,
			}).Error; err != nil {
			return err
		}
		conversation.Avatar = avatarURL
		conversation.UpdatedAt = now

		createdMessage, err := createGroupAvatarUpdatedSystemMessage(tx, &conversation, currentUser, now)
		if err != nil {
			return err
		}
		message = createdMessage

		if err := advanceConversationMemberReadSeq(tx, conversationID, currentUser.ID, createdMessage.Seq); err != nil {
			return err
		}

		ids, err := loadActiveConversationUserIDs(tx, conversationID)
		if err != nil {
			return err
		}
		memberUserIDs = ids
		return nil
	})
	if err != nil {
		return store.Conversation{}, store.Message{}, nil, err
	}

	return conversation, message, memberUserIDs, nil
}

func createGroupMembersInvitedSystemMessage(db *gorm.DB, conversation *store.Conversation, inviter store.User, invitees []systemEventUserRef, now time.Time) (store.Message, error) {
	body, summary, err := newGroupMembersInvitedSystemEventBody(inviter, invitees)
	if err != nil {
		return store.Message{}, err
	}

	message := store.Message{
		ID:             uuid.NewString(),
		ConversationID: conversation.ID,
		Seq:            conversation.LastMessageSeq + 1,
		SenderType:     store.MessageSenderTypeSystem,
		Body:           body,
		Summary:        summary,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := db.Create(&message).Error; err != nil {
		return store.Message{}, err
	}

	if err := db.Model(&store.Conversation{}).
		Where("id = ?", conversation.ID).
		Updates(map[string]any{
			"last_message_at":      message.CreatedAt,
			"last_message_id":      message.ID,
			"last_message_seq":     message.Seq,
			"last_message_summary": message.Summary,
			"updated_at":           now,
		}).Error; err != nil {
		return store.Message{}, err
	}

	lastMessageAt := message.CreatedAt
	conversation.LastMessageAt = &lastMessageAt
	conversation.LastMessageID = &message.ID
	conversation.LastMessageSeq = message.Seq
	conversation.LastMessageSummary = message.Summary
	conversation.UpdatedAt = now

	return message, nil
}

func createGroupAvatarUpdatedSystemMessage(db *gorm.DB, conversation *store.Conversation, actor store.User, now time.Time) (store.Message, error) {
	body, summary, err := newGroupAvatarUpdatedSystemEventBody(actor)
	if err != nil {
		return store.Message{}, err
	}

	message := store.Message{
		ID:             uuid.NewString(),
		ConversationID: conversation.ID,
		Seq:            conversation.LastMessageSeq + 1,
		SenderType:     store.MessageSenderTypeSystem,
		Body:           body,
		Summary:        summary,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := db.Create(&message).Error; err != nil {
		return store.Message{}, err
	}

	if err := db.Model(&store.Conversation{}).
		Where("id = ?", conversation.ID).
		Updates(map[string]any{
			"last_message_at":      message.CreatedAt,
			"last_message_id":      message.ID,
			"last_message_seq":     message.Seq,
			"last_message_summary": message.Summary,
			"updated_at":           now,
		}).Error; err != nil {
		return store.Message{}, err
	}

	lastMessageAt := message.CreatedAt
	conversation.LastMessageAt = &lastMessageAt
	conversation.LastMessageID = &message.ID
	conversation.LastMessageSeq = message.Seq
	conversation.LastMessageSummary = message.Summary
	conversation.UpdatedAt = now

	return message, nil
}

func createGroupVisibilityChangedSystemMessage(db *gorm.DB, conversation *store.Conversation, actor store.User, visibility string, now time.Time) (store.Message, error) {
	body, summary, err := newGroupVisibilityChangedSystemEventBody(actor, visibility)
	if err != nil {
		return store.Message{}, err
	}

	message := store.Message{
		ID:             uuid.NewString(),
		ConversationID: conversation.ID,
		Seq:            conversation.LastMessageSeq + 1,
		SenderType:     store.MessageSenderTypeSystem,
		Body:           body,
		Summary:        summary,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := db.Create(&message).Error; err != nil {
		return store.Message{}, err
	}

	if err := db.Model(&store.Conversation{}).
		Where("id = ?", conversation.ID).
		Updates(map[string]any{
			"last_message_at":      message.CreatedAt,
			"last_message_id":      message.ID,
			"last_message_seq":     message.Seq,
			"last_message_summary": message.Summary,
			"updated_at":           now,
		}).Error; err != nil {
		return store.Message{}, err
	}

	lastMessageAt := message.CreatedAt
	conversation.LastMessageAt = &lastMessageAt
	conversation.LastMessageID = &message.ID
	conversation.LastMessageSeq = message.Seq
	conversation.LastMessageSummary = message.Summary
	conversation.UpdatedAt = now

	return message, nil
}

func createGroupMemberJoinedSystemMessage(db *gorm.DB, conversation *store.Conversation, actor store.User, now time.Time) (store.Message, error) {
	body, summary, err := newGroupMemberJoinedSystemEventBody(actor)
	if err != nil {
		return store.Message{}, err
	}

	message := store.Message{
		ID:             uuid.NewString(),
		ConversationID: conversation.ID,
		Seq:            conversation.LastMessageSeq + 1,
		SenderType:     store.MessageSenderTypeSystem,
		Body:           body,
		Summary:        summary,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := db.Create(&message).Error; err != nil {
		return store.Message{}, err
	}

	if err := db.Model(&store.Conversation{}).
		Where("id = ?", conversation.ID).
		Updates(map[string]any{
			"last_message_at":      message.CreatedAt,
			"last_message_id":      message.ID,
			"last_message_seq":     message.Seq,
			"last_message_summary": message.Summary,
			"updated_at":           now,
		}).Error; err != nil {
		return store.Message{}, err
	}

	lastMessageAt := message.CreatedAt
	conversation.LastMessageAt = &lastMessageAt
	conversation.LastMessageID = &message.ID
	conversation.LastMessageSeq = message.Seq
	conversation.LastMessageSummary = message.Summary
	conversation.UpdatedAt = now

	return message, nil
}

func createGroupMemberLeftSystemMessage(db *gorm.DB, conversation *store.Conversation, actor store.User, now time.Time) (store.Message, error) {
	body, summary, err := newGroupMemberLeftSystemEventBody(actor)
	if err != nil {
		return store.Message{}, err
	}

	message := store.Message{
		ID:             uuid.NewString(),
		ConversationID: conversation.ID,
		Seq:            conversation.LastMessageSeq + 1,
		SenderType:     store.MessageSenderTypeSystem,
		Body:           body,
		Summary:        summary,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := db.Create(&message).Error; err != nil {
		return store.Message{}, err
	}

	if err := db.Model(&store.Conversation{}).
		Where("id = ?", conversation.ID).
		Updates(map[string]any{
			"last_message_at":      message.CreatedAt,
			"last_message_id":      message.ID,
			"last_message_seq":     message.Seq,
			"last_message_summary": message.Summary,
			"updated_at":           now,
		}).Error; err != nil {
		return store.Message{}, err
	}

	lastMessageAt := message.CreatedAt
	conversation.LastMessageAt = &lastMessageAt
	conversation.LastMessageID = &message.ID
	conversation.LastMessageSeq = message.Seq
	conversation.LastMessageSummary = message.Summary
	conversation.UpdatedAt = now

	return message, nil
}

func createGroupMemberRemovedSystemMessage(db *gorm.DB, conversation *store.Conversation, actor store.User, target systemEventUserRef, now time.Time) (store.Message, error) {
	body, summary, err := newGroupMemberRemovedSystemEventBody(actor, target)
	if err != nil {
		return store.Message{}, err
	}

	message := store.Message{
		ID:             uuid.NewString(),
		ConversationID: conversation.ID,
		Seq:            conversation.LastMessageSeq + 1,
		SenderType:     store.MessageSenderTypeSystem,
		Body:           body,
		Summary:        summary,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := db.Create(&message).Error; err != nil {
		return store.Message{}, err
	}

	if err := db.Model(&store.Conversation{}).
		Where("id = ?", conversation.ID).
		Updates(map[string]any{
			"last_message_at":      message.CreatedAt,
			"last_message_id":      message.ID,
			"last_message_seq":     message.Seq,
			"last_message_summary": message.Summary,
			"updated_at":           now,
		}).Error; err != nil {
		return store.Message{}, err
	}

	lastMessageAt := message.CreatedAt
	conversation.LastMessageAt = &lastMessageAt
	conversation.LastMessageID = &message.ID
	conversation.LastMessageSeq = message.Seq
	conversation.LastMessageSummary = message.Summary
	conversation.UpdatedAt = now

	return message, nil
}

func createGroupNameUpdatedSystemMessage(db *gorm.DB, conversation *store.Conversation, actor store.User, name string, now time.Time) (store.Message, error) {
	body, summary, err := newGroupNameUpdatedSystemEventBody(actor, name)
	if err != nil {
		return store.Message{}, err
	}

	message := store.Message{
		ID:             uuid.NewString(),
		ConversationID: conversation.ID,
		Seq:            conversation.LastMessageSeq + 1,
		SenderType:     store.MessageSenderTypeSystem,
		Body:           body,
		Summary:        summary,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := db.Create(&message).Error; err != nil {
		return store.Message{}, err
	}

	if err := db.Model(&store.Conversation{}).
		Where("id = ?", conversation.ID).
		Updates(map[string]any{
			"last_message_at":      message.CreatedAt,
			"last_message_id":      message.ID,
			"last_message_seq":     message.Seq,
			"last_message_summary": message.Summary,
			"updated_at":           now,
		}).Error; err != nil {
		return store.Message{}, err
	}

	lastMessageAt := message.CreatedAt
	conversation.LastMessageAt = &lastMessageAt
	conversation.LastMessageID = &message.ID
	conversation.LastMessageSeq = message.Seq
	conversation.LastMessageSummary = message.Summary
	conversation.UpdatedAt = now

	return message, nil
}

func newGroupMembersInvitedSystemEventBody(inviter store.User, inviteeRefs []systemEventUserRef) (json.RawMessage, string, error) {
	inviteeNames := make([]string, 0, len(inviteeRefs))
	for _, invitee := range inviteeRefs {
		inviteeNames = append(inviteeNames, invitee.DisplayName)
	}

	inviterDisplayName := userDisplayName(inviter)
	summary := inviterDisplayName + " 邀请 " + strings.Join(inviteeNames, groupMembersInvitedSummarySeparator) + " 加入群聊"
	body, err := json.Marshal(groupMembersInvitedSystemEventBody{
		Event:    systemEventGroupMembersInvited,
		Invitees: inviteeRefs,
		Inviter: systemEventUserRef{
			DisplayName: inviterDisplayName,
			ID:          inviter.ID,
		},
		Type: messageTypeSystemEvent,
	})
	if err != nil {
		return nil, "", err
	}

	return body, summary, nil
}

func makeGroupMemberInviteeRefs(users []store.User, apps []store.App) []systemEventUserRef {
	refs := make([]systemEventUserRef, 0, len(users)+len(apps))
	for _, user := range users {
		refs = append(refs, systemEventUserRef{
			DisplayName: userDisplayName(user),
			ID:          user.ID,
		})
	}
	for _, app := range apps {
		refs = append(refs, systemEventUserRef{
			DisplayName: app.Name,
			ID:          app.ID,
			Type:        store.ConversationMemberTypeApp,
		})
	}

	return refs
}

func loadGroupMemberSystemRef(db *gorm.DB, memberType string, memberID string) (systemEventUserRef, error) {
	switch memberType {
	case store.ConversationMemberTypeUser:
		var user store.User
		if err := db.First(&user, "id = ?", memberID).Error; err != nil {
			return systemEventUserRef{}, err
		}
		return systemEventUserRef{
			DisplayName: userDisplayName(user),
			ID:          user.ID,
		}, nil
	case store.ConversationMemberTypeApp:
		var app store.App
		if err := db.Unscoped().First(&app, "id = ?", memberID).Error; err != nil {
			return systemEventUserRef{}, err
		}
		return systemEventUserRef{
			DisplayName: app.Name,
			ID:          app.ID,
			Type:        store.ConversationMemberTypeApp,
		}, nil
	default:
		return systemEventUserRef{}, gorm.ErrRecordNotFound
	}
}

func newGroupAvatarUpdatedSystemEventBody(actor store.User) (json.RawMessage, string, error) {
	actorDisplayName := userDisplayName(actor)
	summary := actorDisplayName + " 修改了群头像"
	body, err := json.Marshal(groupAvatarUpdatedSystemEventBody{
		Actor: systemEventUserRef{
			DisplayName: actorDisplayName,
			ID:          actor.ID,
		},
		Event: systemEventGroupAvatarUpdated,
		Type:  messageTypeSystemEvent,
	})
	if err != nil {
		return nil, "", err
	}

	return body, summary, nil
}

func newGroupVisibilityChangedSystemEventBody(actor store.User, visibility string) (json.RawMessage, string, error) {
	actorDisplayName := userDisplayName(actor)
	summary := actorDisplayName + " 将当前群设置为公开群"
	if visibility == store.ConversationVisibilityPrivate {
		summary = actorDisplayName + " 将当前群设为私有群"
	}
	body, err := json.Marshal(groupVisibilityChangedSystemEventBody{
		Actor: systemEventUserRef{
			DisplayName: actorDisplayName,
			ID:          actor.ID,
		},
		Event:      systemEventGroupVisibilityChanged,
		Type:       messageTypeSystemEvent,
		Visibility: visibility,
	})
	if err != nil {
		return nil, "", err
	}

	return body, summary, nil
}

func newGroupMemberJoinedSystemEventBody(actor store.User) (json.RawMessage, string, error) {
	actorDisplayName := userDisplayName(actor)
	summary := actorDisplayName + " 加入群聊"
	body, err := json.Marshal(groupMemberJoinedSystemEventBody{
		Actor: systemEventUserRef{
			DisplayName: actorDisplayName,
			ID:          actor.ID,
		},
		Event: systemEventGroupMemberJoined,
		Type:  messageTypeSystemEvent,
	})
	if err != nil {
		return nil, "", err
	}

	return body, summary, nil
}

func newGroupMemberLeftSystemEventBody(actor store.User) (json.RawMessage, string, error) {
	actorDisplayName := userDisplayName(actor)
	summary := actorDisplayName + " 已退出群聊"
	body, err := json.Marshal(groupMemberLeftSystemEventBody{
		Actor: systemEventUserRef{
			DisplayName: actorDisplayName,
			ID:          actor.ID,
		},
		Event: systemEventGroupMemberLeft,
		Type:  messageTypeSystemEvent,
	})
	if err != nil {
		return nil, "", err
	}

	return body, summary, nil
}

func newGroupMemberRemovedSystemEventBody(actor store.User, target systemEventUserRef) (json.RawMessage, string, error) {
	actorDisplayName := userDisplayName(actor)
	summary := actorDisplayName + " 已将 " + target.DisplayName + " 移出群聊"
	body, err := json.Marshal(groupMemberRemovedSystemEventBody{
		Actor: systemEventUserRef{
			DisplayName: actorDisplayName,
			ID:          actor.ID,
		},
		Event:  systemEventGroupMemberRemoved,
		Target: target,
		Type:   messageTypeSystemEvent,
	})
	if err != nil {
		return nil, "", err
	}

	return body, summary, nil
}

func newGroupNameUpdatedSystemEventBody(actor store.User, name string) (json.RawMessage, string, error) {
	actorDisplayName := userDisplayName(actor)
	summary := actorDisplayName + " 修改群聊名称为 " + name
	body, err := json.Marshal(groupNameUpdatedSystemEventBody{
		Actor: systemEventUserRef{
			DisplayName: actorDisplayName,
			ID:          actor.ID,
		},
		Event: systemEventGroupNameUpdated,
		Name:  name,
		Type:  messageTypeSystemEvent,
	})
	if err != nil {
		return nil, "", err
	}

	return body, summary, nil
}

func newMessageRevokedSystemEventBody(actor store.User) (json.RawMessage, string, error) {
	actorDisplayName := userDisplayName(actor)
	summary := actorDisplayName + " 撤回了一条消息"
	body, err := json.Marshal(messageRevokedSystemEventBody{
		Actor: systemEventUserRef{
			DisplayName: actorDisplayName,
			ID:          actor.ID,
		},
		Event: systemEventMessageRevoked,
		Type:  messageTypeSystemEvent,
	})
	if err != nil {
		return nil, "", err
	}

	return body, summary, nil
}

func revokedMessageSummary() string {
	return "该消息已被撤回"
}

func orderDirectConversationUserIDs(first string, second string) (string, string) {
	if first < second {
		return first, second
	}

	return second, first
}

func normalizeGroupConversationName(rawName string) (string, error) {
	name := strings.TrimSpace(rawName)
	if name == "" {
		return "", errors.New("群聊名称不能为空")
	}
	if len([]rune(name)) > maxGroupConversationNameLength {
		return "", errors.New("群聊名称不能超过 120 个字符")
	}

	return name, nil
}

func normalizeGroupMemberIDs(rawIDs []string, creatorID string) ([]string, error) {
	parsedCreatorID, err := uuid.Parse(creatorID)
	if err != nil {
		return nil, errors.New("当前用户 ID 格式错误")
	}

	seen := map[string]struct{}{parsedCreatorID.String(): {}}
	memberIDs := make([]string, 0, len(rawIDs))

	for _, rawID := range rawIDs {
		id := strings.TrimSpace(rawID)
		if id == "" {
			return nil, errors.New("成员 ID 不能为空")
		}
		parsedID, err := uuid.Parse(id)
		if err != nil {
			return nil, errors.New("成员 ID 格式错误")
		}
		id = parsedID.String()
		if _, ok := seen[id]; ok {
			continue
		}

		seen[id] = struct{}{}
		memberIDs = append(memberIDs, id)
	}

	return memberIDs, nil
}

func normalizeGroupAppIDs(rawIDs []string) ([]string, error) {
	seen := map[string]struct{}{}
	appIDs := make([]string, 0, len(rawIDs))

	for _, rawID := range rawIDs {
		id := strings.TrimSpace(rawID)
		if id == "" {
			return nil, errors.New("应用 ID 不能为空")
		}
		parsedID, err := uuid.Parse(id)
		if err != nil {
			return nil, errors.New("应用 ID 格式错误")
		}
		id = parsedID.String()
		if _, ok := seen[id]; ok {
			continue
		}

		seen[id] = struct{}{}
		appIDs = append(appIDs, id)
	}

	return appIDs, nil
}

func normalizeGroupProjectIDs(rawIDs []string) ([]string, error) {
	seen := make(map[string]struct{}, len(rawIDs))
	projectIDs := make([]string, 0, len(rawIDs))
	for _, rawID := range rawIDs {
		parsedID, err := uuid.Parse(strings.TrimSpace(rawID))
		if err != nil {
			return nil, errGroupConversationProjectInvalid
		}
		projectID := parsedID.String()
		if _, exists := seen[projectID]; exists {
			continue
		}
		seen[projectID] = struct{}{}
		projectIDs = append(projectIDs, projectID)
	}
	sort.Strings(projectIDs)
	return projectIDs, nil
}

func (s *Server) loadActiveGroupMembers(memberIDs []string) ([]store.User, error) {
	return loadActiveGroupMembers(s.db, memberIDs)
}

func loadActiveGroupMembers(db *gorm.DB, memberIDs []string) ([]store.User, error) {
	var users []store.User
	if err := db.Where("id IN ? AND status = ?", memberIDs, store.UserStatusActive).Find(&users).Error; err != nil {
		return nil, err
	}
	if len(users) != len(memberIDs) {
		return nil, gorm.ErrRecordNotFound
	}

	usersByID := make(map[string]store.User, len(users))
	for _, user := range users {
		usersByID[user.ID] = user
	}

	orderedUsers := make([]store.User, 0, len(memberIDs))
	for _, memberID := range memberIDs {
		user, ok := usersByID[memberID]
		if !ok {
			return nil, gorm.ErrRecordNotFound
		}
		orderedUsers = append(orderedUsers, user)
	}

	return orderedUsers, nil
}

func loadVisibleGroupApps(db *gorm.DB, currentUserID string, appIDs []string) ([]store.App, error) {
	if len(appIDs) == 0 {
		return nil, nil
	}

	var apps []store.App
	if err := db.
		Where("id IN ? AND enabled = ?", appIDs, true).
		Where(
			"visibility = ? OR (visibility = ? AND creator_user_id = ?)",
			store.AppVisibilityPublic,
			store.AppVisibilityCreator,
			currentUserID,
		).
		Find(&apps).Error; err != nil {
		return nil, err
	}
	if len(apps) != len(appIDs) {
		return nil, gorm.ErrRecordNotFound
	}

	appsByID := make(map[string]store.App, len(apps))
	for _, app := range apps {
		appsByID[app.ID] = app
	}

	orderedApps := make([]store.App, 0, len(appIDs))
	for _, appID := range appIDs {
		app, ok := appsByID[appID]
		if !ok {
			return nil, gorm.ErrRecordNotFound
		}
		orderedApps = append(orderedApps, app)
	}

	return orderedApps, nil
}

func (s *Server) loadConversationListMembers(conversationIDs []string) (map[string][]store.ConversationMember, map[string]store.User, map[string]store.App, error) {
	membersByConversationID := make(map[string][]store.ConversationMember, len(conversationIDs))
	usersByID := make(map[string]store.User)
	appsByID := make(map[string]store.App)
	if len(conversationIDs) == 0 {
		return membersByConversationID, usersByID, appsByID, nil
	}

	var members []store.ConversationMember
	if err := s.db.
		Where("conversation_id IN ? AND left_at IS NULL", conversationIDs).
		Order("conversation_id ASC").
		Order("joined_at ASC").
		Find(&members).Error; err != nil {
		return nil, nil, nil, err
	}

	userIDSet := make(map[string]struct{})
	appIDSet := make(map[string]struct{})
	for _, member := range members {
		membersByConversationID[member.ConversationID] = append(membersByConversationID[member.ConversationID], member)
		switch member.MemberType {
		case store.ConversationMemberTypeUser:
			userIDSet[member.MemberID] = struct{}{}
		case store.ConversationMemberTypeApp:
			appIDSet[member.MemberID] = struct{}{}
		}
	}

	userIDs := make([]string, 0, len(userIDSet))
	for userID := range userIDSet {
		userIDs = append(userIDs, userID)
	}
	if len(userIDs) > 0 {
		var users []store.User
		if err := s.db.Where("id IN ?", userIDs).Find(&users).Error; err != nil {
			return nil, nil, nil, err
		}
		for _, user := range users {
			usersByID[user.ID] = user
		}
	}

	appIDs := make([]string, 0, len(appIDSet))
	for appID := range appIDSet {
		appIDs = append(appIDs, appID)
	}
	if len(appIDs) > 0 {
		var apps []store.App
		if err := s.db.Unscoped().Where("id IN ?", appIDs).Find(&apps).Error; err != nil {
			return nil, nil, nil, err
		}
		for _, app := range apps {
			appsByID[app.ID] = app
		}
	}

	return membersByConversationID, usersByID, appsByID, nil
}

func newConversationListItemResponse(
	conversation store.Conversation,
	currentUserID string,
	members []store.ConversationMember,
	usersByID map[string]store.User,
	appsByID map[string]store.App,
) conversationListItemResponse {
	name := conversation.Name
	avatar := conversation.Avatar
	lastReadSeq := currentMemberLastReadSeq(currentUserID, members)
	lastMentionedSeq := currentMemberLastMentionedSeq(currentUserID, members)
	if conversation.Kind == store.ConversationKindDirect {
		for _, member := range members {
			if member.MemberID == currentUserID {
				continue
			}
			otherUser, ok := usersByID[member.MemberID]
			if !ok {
				continue
			}
			name = userDisplayName(otherUser)
			avatar = otherUser.Avatar
			if avatar == "" {
				avatar = store.DefaultUserAvatar
			}
			break
		}
		if strings.TrimSpace(name) == "" {
			name = "私聊"
		}
	} else if strings.TrimSpace(name) == "" {
		name = "群聊"
	}

	return conversationListItemResponse{
		Avatar:             avatar,
		CreatedAt:          conversation.CreatedAt,
		ID:                 conversation.ID,
		LastMessageAt:      conversation.LastMessageAt,
		LastMessageID:      conversation.LastMessageID,
		LastMessageSeq:     conversation.LastMessageSeq,
		LastMessageSummary: conversation.LastMessageSummary,
		LastMentionedSeq:   lastMentionedSeq,
		LastReadSeq:        lastReadSeq,
		MemberCount:        conversationListMemberCount(conversation.Kind, members),
		Members:            newConversationMemberResponses(members, usersByID, appsByID),
		Name:               name,
		Type:               conversation.Kind,
		UnreadCount:        unreadCount(conversation.LastMessageSeq, lastReadSeq),
		Visibility:         conversation.Visibility,
	}
}

func currentMemberLastReadSeq(currentUserID string, members []store.ConversationMember) int64 {
	for _, member := range members {
		if member.MemberType == store.ConversationMemberTypeUser && member.MemberID == currentUserID {
			return member.LastReadSeq
		}
	}

	return 0
}

func currentMemberLastMentionedSeq(currentUserID string, members []store.ConversationMember) int64 {
	for _, member := range members {
		if member.MemberType == store.ConversationMemberTypeUser && member.MemberID == currentUserID {
			return member.LastMentionedSeq
		}
	}

	return 0
}

func conversationListMemberCount(conversationKind string, members []store.ConversationMember) int {
	if conversationKind != store.ConversationKindApp {
		return len(members)
	}

	count := 0
	for _, member := range members {
		if member.MemberType == store.ConversationMemberTypeUser {
			count++
		}
	}

	return count
}

func unreadCount(lastMessageSeq int64, lastReadSeq int64) int64 {
	if lastReadSeq >= lastMessageSeq {
		return 0
	}

	return lastMessageSeq - lastReadSeq
}

func newConversationMemberResponses(
	members []store.ConversationMember,
	usersByID map[string]store.User,
	appsByID map[string]store.App,
) []conversationMemberResponse {
	responses := make([]conversationMemberResponse, 0, len(members))
	for _, member := range members {
		if member.MemberType == store.ConversationMemberTypeApp {
			app, ok := appsByID[member.MemberID]
			if !ok {
				continue
			}
			responses = append(responses, conversationMemberResponse{
				Avatar: app.Avatar,
				ID:     app.ID,
				Name:   app.Name,
				Role:   member.Role,
				Type:   store.ConversationMemberTypeApp,
			})
			continue
		}

		user, ok := usersByID[member.MemberID]
		if !ok {
			continue
		}
		phone := ""
		if user.Phone != nil {
			phone = *user.Phone
		}
		avatar := user.Avatar
		if avatar == "" {
			avatar = store.DefaultUserAvatar
		}
		responses = append(responses, conversationMemberResponse{
			Avatar:   avatar,
			Email:    user.Email,
			ID:       user.ID,
			Name:     user.Name,
			Nickname: user.Nickname,
			Phone:    phone,
			Role:     member.Role,
			Type:     store.ConversationMemberTypeUser,
		})
	}

	return responses
}

func userDisplayName(user store.User) string {
	if nickname := strings.TrimSpace(user.Nickname); nickname != "" {
		return nickname
	}

	return strings.TrimSpace(user.Name)
}

func newGroupConversationResponse(
	conversation store.Conversation,
	members []conversationMemberCandidate,
	currentUserID string,
) groupConversationResponse {
	responses := make([]conversationMemberResponse, 0, len(members))
	for _, member := range members {
		if member.memberType == store.ConversationMemberTypeApp {
			responses = append(responses, conversationMemberResponse{
				Avatar: member.app.Avatar,
				ID:     member.app.ID,
				Name:   member.app.Name,
				Role:   member.role,
				Type:   store.ConversationMemberTypeApp,
			})
			continue
		}
		phone := ""
		if member.user.Phone != nil {
			phone = *member.user.Phone
		}
		avatar := member.user.Avatar
		if avatar == "" {
			avatar = store.DefaultUserAvatar
		}
		responses = append(responses, conversationMemberResponse{
			Avatar:   avatar,
			Email:    member.user.Email,
			ID:       member.user.ID,
			Name:     member.user.Name,
			Nickname: member.user.Nickname,
			Phone:    phone,
			Role:     member.role,
			Type:     store.ConversationMemberTypeUser,
		})
	}

	lastReadSeq := int64(0)
	if currentUserID == conversation.CreatedByUserID {
		lastReadSeq = conversation.LastMessageSeq
	}

	return groupConversationResponse{
		Avatar:             conversation.Avatar,
		CreatedAt:          conversation.CreatedAt,
		CreatedByUserID:    conversation.CreatedByUserID,
		ID:                 conversation.ID,
		LastMessageAt:      conversation.LastMessageAt,
		LastMessageID:      conversation.LastMessageID,
		LastMessageSeq:     conversation.LastMessageSeq,
		LastMessageSummary: conversation.LastMessageSummary,
		LastReadSeq:        lastReadSeq,
		MemberCount:        len(responses),
		Members:            responses,
		Name:               conversation.Name,
		PostingPolicy:      conversation.PostingPolicy,
		Status:             conversation.Status,
		Type:               conversation.Kind,
		UnreadCount:        unreadCount(conversation.LastMessageSeq, lastReadSeq),
		Visibility:         conversation.Visibility,
	}
}
