package httpserver

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"app/internal/appregistry"
	"app/internal/store"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const maxGroupConversationMembers = 200
const maxClientConversationListItems = 100

const (
	builtinAssistantAvatar              = appregistry.GoddessDefaultAvatar
	builtinAssistantConversationName    = appregistry.GoddessDefaultName
	messageTypeSystemEvent              = "system_event"
	systemEventGroupMembersInvited      = "group_members_invited"
	systemEventGroupAvatarUpdated       = "group_avatar_updated"
	groupMembersInvitedSummarySeparator = ","
)

var (
	errConversationNotGroup             = errors.New("conversation is not group")
	errGroupConversationMemberCap       = errors.New("group conversation member cap exceeded")
	errGroupConversationMemberMiss      = errors.New("group conversation member missing")
	errGroupConversationAvatarForbidden = errors.New("group conversation avatar forbidden")
)

type createGroupConversationRequest struct {
	MemberIDs []string `json:"member_ids" example:"7f8d8b84-6d2c-4b12-9a8a-019a7e2787d4"`
	Name      string   `json:"name" example:"产品讨论组"`
}

type addGroupConversationMembersRequest struct {
	MemberIDs []string `json:"member_ids" example:"7f8d8b84-6d2c-4b12-9a8a-019a7e2787d4"`
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
	LastReadSeq        int64                        `json:"last_read_seq" example:"12"`
	MemberCount        int                          `json:"member_count" example:"3"`
	Members            []conversationMemberResponse `json:"members"`
	Name               string                       `json:"name" example:"产品讨论组"`
	PostingPolicy      string                       `json:"posting_policy" example:"open"`
	Status             string                       `json:"status" example:"active"`
	Type               string                       `json:"type" example:"group"`
	UnreadCount        int64                        `json:"unread_count" example:"0"`
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
	LastReadSeq        int64                        `json:"last_read_seq" example:"9"`
	MemberCount        int                          `json:"member_count" example:"2"`
	Members            []conversationMemberResponse `json:"members"`
	Name               string                       `json:"name" example:"张三"`
	Type               string                       `json:"type" example:"direct"`
	UnreadCount        int64                        `json:"unread_count" example:"3"`
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
	role string
	user store.User
}

type systemEventUserRef struct {
	DisplayName string `json:"display_name"`
	ID          string `json:"id"`
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

	membersByConversationID, usersByID, err := s.loadConversationListMembers(conversationIDs)
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
		))
	}
	for _, conversation := range conversations {
		responses = append(responses, newConversationListItemResponse(
			conversation,
			user.ID,
			membersByConversationID[conversation.ID],
			usersByID,
		))
	}

	return success(c, http.StatusOK, listClientConversationsResponse{
		Conversations: responses,
	})
}

func (s *Server) ensureBuiltinAssistantConversation(user store.User) (store.Conversation, bool, error) {
	assistantApp, err := appregistry.EnsureGoddessApp(s.db, s.cfg.Apps)
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
		),
		Created: created,
	})
}

// createGroupConversation godoc
//
// @Summary 创建群聊
// @Description 普通用户创建群聊。当前登录用户会自动成为群主，member_ids 只需要传其他成员。
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

	name := strings.TrimSpace(req.Name)
	if name == "" {
		return failure(c, http.StatusBadRequest, "invalid_request", "群聊名称不能为空")
	}
	if len([]rune(name)) > 120 {
		return failure(c, http.StatusBadRequest, "invalid_request", "群聊名称不能超过 120 个字符")
	}

	memberIDs, err := normalizeGroupMemberIDs(req.MemberIDs, user.ID)
	if err != nil {
		return failure(c, http.StatusBadRequest, "invalid_request", err.Error())
	}
	if len(memberIDs) == 0 {
		return failure(c, http.StatusBadRequest, "invalid_request", "至少选择一名成员")
	}
	if len(memberIDs)+1 > maxGroupConversationMembers {
		return failure(c, http.StatusBadRequest, "invalid_request", "群聊成员不能超过 200 人")
	}

	members, err := s.loadActiveGroupMembers(memberIDs)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return failure(c, http.StatusBadRequest, "invalid_request", "成员不存在或已禁用")
		}
		return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
	}

	now := time.Now().UTC()
	conversation := store.Conversation{
		ID:              uuid.NewString(),
		Kind:            store.ConversationKindGroup,
		Name:            name,
		CreatedByUserID: user.ID,
		Status:          store.ConversationStatusActive,
		PostingPolicy:   store.ConversationPostingPolicyOpen,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	candidates := make([]conversationMemberCandidate, 0, len(members)+1)
	candidates = append(candidates, conversationMemberCandidate{
		role: store.ConversationMemberRoleOwner,
		user: user,
	})
	for _, member := range members {
		candidates = append(candidates, conversationMemberCandidate{
			role: store.ConversationMemberRoleMember,
			user: member,
		})
	}

	var createdMessage store.Message
	memberUserIDs := make([]string, 0, len(candidates))
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&conversation).Error; err != nil {
			return err
		}

		systemMessageSeq := conversation.LastMessageSeq + 1
		conversationMembers := make([]store.ConversationMember, 0, len(candidates))
		for _, candidate := range candidates {
			lastReadSeq := int64(0)
			if candidate.user.ID == user.ID {
				lastReadSeq = systemMessageSeq
			}
			conversationMembers = append(conversationMembers, store.ConversationMember{
				ConversationID:        conversation.ID,
				MemberType:            store.ConversationMemberTypeUser,
				MemberID:              candidate.user.ID,
				Role:                  candidate.role,
				JoinedAt:              now,
				HistoryVisibleFromSeq: 1,
				LastReadSeq:           lastReadSeq,
			})
			memberUserIDs = append(memberUserIDs, candidate.user.ID)
		}

		if err := tx.Create(&conversationMembers).Error; err != nil {
			return err
		}

		message, err := createGroupMembersInvitedSystemMessage(tx, &conversation, user, members, now)
		if err != nil {
			return err
		}
		createdMessage = message
		return nil
	}); err != nil {
		return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
	}

	s.realtime.SendToUsers(memberUserIDs, realtimeMessageCreatedEvent(newMessageResponse(createdMessage)))

	return success(c, http.StatusCreated, createGroupConversationResponse{
		Conversation: newGroupConversationResponse(conversation, candidates, user.ID),
	})
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
	if len(memberIDs) == 0 {
		return failure(c, http.StatusBadRequest, "invalid_request", "至少选择一名成员")
	}

	conversation, message, memberUserIDs, err := s.addUserGroupConversationMembers(user, conversationID, memberIDs)
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
			return failure(c, http.StatusBadRequest, "invalid_request", "群聊成员不能超过 200 人")
		}
		if errors.Is(err, errGroupConversationMemberMiss) {
			return failure(c, http.StatusBadRequest, "invalid_request", "成员不存在或已禁用")
		}

		return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
	}

	membersByConversationID, usersByID, err := s.loadConversationListMembers([]string{conversation.ID})
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
		),
		Message: messageResponse,
	})
}

func canManageGroupConversation(role string) bool {
	return role == store.ConversationMemberRoleOwner || role == store.ConversationMemberRoleAdmin
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
			Where("conversation_id = ? AND member_type = ?", conversationID, store.ConversationMemberTypeUser).
			Find(&existingMembers).Error; err != nil {
			return err
		}

		activeMemberCount := 0
		membersByID := make(map[string]store.ConversationMember, len(existingMembers))
		for _, member := range existingMembers {
			membersByID[member.MemberID] = member
			if member.LeftAt == nil {
				activeMemberCount++
			}
		}

		newMemberIDs := make([]string, 0, len(memberIDs))
		reactivatedMemberIDs := make([]string, 0, len(memberIDs))
		addedMemberIDs := make([]string, 0, len(memberIDs))
		for _, memberID := range memberIDs {
			existingMember, ok := membersByID[memberID]
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
		if len(addedMemberIDs) == 0 {
			return nil
		}
		if activeMemberCount+len(newMemberIDs)+len(reactivatedMemberIDs) > maxGroupConversationMembers {
			return errGroupConversationMemberCap
		}

		addedUsers, err := loadActiveGroupMembers(tx, addedMemberIDs)
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

		newUsersByID := make(map[string]store.User, len(addedUsers))
		for _, addedUser := range addedUsers {
			newUsersByID[addedUser.ID] = addedUser
		}
		conversationMembers := make([]store.ConversationMember, 0, len(newMemberIDs))
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
		if len(conversationMembers) > 0 {
			if err := tx.Create(&conversationMembers).Error; err != nil {
				return err
			}
		}

		createdMessage, err := createGroupMembersInvitedSystemMessage(tx, &conversation, currentUser, addedUsers, now)
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

func createGroupMembersInvitedSystemMessage(db *gorm.DB, conversation *store.Conversation, inviter store.User, invitees []store.User, now time.Time) (store.Message, error) {
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

func newGroupMembersInvitedSystemEventBody(inviter store.User, invitees []store.User) (json.RawMessage, string, error) {
	inviteeRefs := make([]systemEventUserRef, 0, len(invitees))
	inviteeNames := make([]string, 0, len(invitees))
	for _, invitee := range invitees {
		displayName := userDisplayName(invitee)
		inviteeRefs = append(inviteeRefs, systemEventUserRef{
			DisplayName: displayName,
			ID:          invitee.ID,
		})
		inviteeNames = append(inviteeNames, displayName)
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

func orderDirectConversationUserIDs(first string, second string) (string, string) {
	if first < second {
		return first, second
	}

	return second, first
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

func (s *Server) loadConversationListMembers(conversationIDs []string) (map[string][]store.ConversationMember, map[string]store.User, error) {
	membersByConversationID := make(map[string][]store.ConversationMember, len(conversationIDs))
	usersByID := make(map[string]store.User)
	if len(conversationIDs) == 0 {
		return membersByConversationID, usersByID, nil
	}

	var members []store.ConversationMember
	if err := s.db.
		Where("conversation_id IN ? AND member_type = ? AND left_at IS NULL", conversationIDs, store.ConversationMemberTypeUser).
		Order("conversation_id ASC").
		Order("joined_at ASC").
		Find(&members).Error; err != nil {
		return nil, nil, err
	}

	userIDSet := make(map[string]struct{})
	for _, member := range members {
		membersByConversationID[member.ConversationID] = append(membersByConversationID[member.ConversationID], member)
		userIDSet[member.MemberID] = struct{}{}
	}

	userIDs := make([]string, 0, len(userIDSet))
	for userID := range userIDSet {
		userIDs = append(userIDs, userID)
	}
	if len(userIDs) == 0 {
		return membersByConversationID, usersByID, nil
	}

	var users []store.User
	if err := s.db.Where("id IN ?", userIDs).Find(&users).Error; err != nil {
		return nil, nil, err
	}
	for _, user := range users {
		usersByID[user.ID] = user
	}

	return membersByConversationID, usersByID, nil
}

func newConversationListItemResponse(
	conversation store.Conversation,
	currentUserID string,
	members []store.ConversationMember,
	usersByID map[string]store.User,
) conversationListItemResponse {
	name := conversation.Name
	avatar := conversation.Avatar
	lastReadSeq := currentMemberLastReadSeq(currentUserID, members)
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
		LastReadSeq:        lastReadSeq,
		MemberCount:        len(members),
		Members:            newConversationMemberResponses(members, usersByID),
		Name:               name,
		Type:               conversation.Kind,
		UnreadCount:        unreadCount(conversation.LastMessageSeq, lastReadSeq),
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

func unreadCount(lastMessageSeq int64, lastReadSeq int64) int64 {
	if lastReadSeq >= lastMessageSeq {
		return 0
	}

	return lastMessageSeq - lastReadSeq
}

func newConversationMemberResponses(
	members []store.ConversationMember,
	usersByID map[string]store.User,
) []conversationMemberResponse {
	responses := make([]conversationMemberResponse, 0, len(members))
	for _, member := range members {
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
	}
}
