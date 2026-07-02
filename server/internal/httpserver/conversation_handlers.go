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

type createGroupConversationRequest struct {
	MemberIDs []string `json:"member_ids" example:"7f8d8b84-6d2c-4b12-9a8a-019a7e2787d4"`
	Name      string   `json:"name" example:"产品讨论组"`
}

type conversationMemberResponse struct {
	Email string `json:"email" example:"user@example.com"`
	ID    string `json:"id" example:"7f8d8b84-6d2c-4b12-9a8a-019a7e2787d4"`
	Name  string `json:"name" example:"张三"`
	Role  string `json:"role" example:"member"`
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

type conversationMemberCandidate struct {
	role string
	user store.User
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
				ConversationID: conversation.ID,
				MemberType:     store.ConversationMemberTypeUser,
				MemberID:       candidate.user.ID,
				Role:           candidate.role,
				JoinedAt:       now,
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

func newGroupConversationResponse(
	conversation store.Conversation,
	members []conversationMemberCandidate,
) groupConversationResponse {
	responses := make([]conversationMemberResponse, 0, len(members))
	for _, member := range members {
		responses = append(responses, conversationMemberResponse{
			Email: member.user.Email,
			ID:    member.user.ID,
			Name:  member.user.Name,
			Role:  member.role,
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
