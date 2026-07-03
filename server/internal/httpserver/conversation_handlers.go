package httpserver

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"app/internal/store"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
)

const maxGroupConversationMembers = 200
const maxClientConversationListItems = 100

type createGroupConversationRequest struct {
	MemberIDs []string `json:"member_ids" example:"7f8d8b84-6d2c-4b12-9a8a-019a7e2787d4"`
	Name      string   `json:"name" example:"产品讨论组"`
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
	CreatedAt       time.Time                    `json:"created_at" format:"date-time"`
	CreatedByUserID string                       `json:"created_by_user_id" example:"7f8d8b84-6d2c-4b12-9a8a-019a7e2787d4"`
	ID              string                       `json:"id" example:"7f8d8b84-6d2c-4b12-9a8a-019a7e2787d4"`
	MemberCount     int                          `json:"member_count" example:"3"`
	Members         []conversationMemberResponse `json:"members"`
	Name            string                       `json:"name" example:"产品讨论组"`
	PostingPolicy   string                       `json:"posting_policy" example:"open"`
	Status          string                       `json:"status" example:"active"`
	Type            string                       `json:"type" example:"group"`
}

type createGroupConversationResponse struct {
	Conversation groupConversationResponse `json:"conversation"`
}

type createDirectConversationResponse struct {
	Conversation conversationListItemResponse `json:"conversation"`
	Created      bool                         `json:"created" example:"true"`
}

type conversationListItemResponse struct {
	Avatar             string     `json:"avatar" example:"/assets/avatars/builtin/07.webp"`
	CreatedAt          time.Time  `json:"created_at" format:"date-time"`
	ID                 string     `json:"id" example:"7f8d8b84-6d2c-4b12-9a8a-019a7e2787d4"`
	LastMessageAt      *time.Time `json:"last_message_at" format:"date-time"`
	LastMessageID      *string    `json:"last_message_id" example:"7f8d8b84-6d2c-4b12-9a8a-019a7e2787d4"`
	LastMessageSeq     int64      `json:"last_message_seq" example:"12"`
	LastMessageSummary string     `json:"last_message_summary" example:"好的，我看一下"`
	MemberCount        int        `json:"member_count" example:"2"`
	Name               string     `json:"name" example:"张三"`
	Type               string     `json:"type" example:"direct"`
}

type listClientConversationsResponse struct {
	Conversations []conversationListItemResponse `json:"conversations"`
}

type conversationMemberCandidate struct {
	role string
	user store.User
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

	var conversations []store.Conversation
	if err := s.db.Model(&store.Conversation{}).
		Joins("JOIN conversation_members cm ON cm.conversation_id = conversations.id").
		Where("cm.member_type = ? AND cm.member_id = ? AND cm.left_at IS NULL", store.ConversationMemberTypeUser, user.ID).
		Where("conversations.status = ?", store.ConversationStatusActive).
		Order("COALESCE(conversations.last_message_at, conversations.created_at) DESC").
		Order("conversations.id ASC").
		Limit(maxClientConversationListItems).
		Find(&conversations).Error; err != nil {
		return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
	}

	conversationIDs := make([]string, 0, len(conversations))
	for _, conversation := range conversations {
		conversationIDs = append(conversationIDs, conversation.ID)
	}

	membersByConversationID, usersByID, err := s.loadConversationListMembers(conversationIDs)
	if err != nil {
		return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
	}

	responses := make([]conversationListItemResponse, 0, len(conversations))
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

	if err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&conversation).Error; err != nil {
			return err
		}

		conversationMembers := make([]store.ConversationMember, 0, len(candidates))
		for _, candidate := range candidates {
			conversationMembers = append(conversationMembers, store.ConversationMember{
				ConversationID:        conversation.ID,
				MemberType:            store.ConversationMemberTypeUser,
				MemberID:              candidate.user.ID,
				Role:                  candidate.role,
				JoinedAt:              now,
				HistoryVisibleFromSeq: 1,
			})
		}

		return tx.Create(&conversationMembers).Error
	}); err != nil {
		return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
	}

	return success(c, http.StatusCreated, createGroupConversationResponse{
		Conversation: newGroupConversationResponse(conversation, candidates),
	})
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
	var users []store.User
	if err := s.db.Where("id IN ? AND status = ?", memberIDs, store.UserStatusActive).Find(&users).Error; err != nil {
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
	avatar := ""
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
			name = "单聊"
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
		MemberCount:        len(members),
		Name:               name,
		Type:               conversation.Kind,
	}
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

	return groupConversationResponse{
		CreatedAt:       conversation.CreatedAt,
		CreatedByUserID: conversation.CreatedByUserID,
		ID:              conversation.ID,
		MemberCount:     len(responses),
		Members:         responses,
		Name:            conversation.Name,
		PostingPolicy:   conversation.PostingPolicy,
		Status:          conversation.Status,
		Type:            conversation.Kind,
	}
}
