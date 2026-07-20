package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	appapp "app/internal/application/app"
	conversationapp "app/internal/application/conversation"
	"app/internal/realtime"
	"app/internal/store"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type appGetUserRequest struct {
	UserID string `json:"user_id"`
}

type appUserResourceResponse struct {
	Avatar   string `json:"avatar"`
	Email    string `json:"email"`
	ID       string `json:"id"`
	Name     string `json:"name"`
	Nickname string `json:"nickname"`
}

type appGetUserResponse struct {
	User appUserResourceResponse `json:"user"`
}

type appGetApplicationRequest struct {
	AppID string `json:"app_id"`
}

type appApplicationResourceResponse struct {
	Avatar      string `json:"avatar"`
	Description string `json:"description"`
	Enabled     bool   `json:"enabled"`
	ID          string `json:"id"`
	Name        string `json:"name"`
	Online      bool   `json:"online"`
	Visibility  string `json:"visibility"`
}

type appGetApplicationResponse struct {
	App appApplicationResourceResponse `json:"app"`
}

type appGroupIdentityResponse struct {
	Avatar   string `json:"avatar"`
	Email    string `json:"email,omitempty"`
	ID       string `json:"id"`
	Name     string `json:"name"`
	Nickname string `json:"nickname,omitempty"`
	Type     string `json:"type"`
}

type appGroupDetailResponse struct {
	Avatar         string                   `json:"avatar"`
	CreatedAt      string                   `json:"created_at"`
	CreatedBy      appGroupIdentityResponse `json:"created_by"`
	CurrentAppRole string                   `json:"current_app_role"`
	ID             string                   `json:"id"`
	MemberCount    int64                    `json:"member_count"`
	Name           string                   `json:"name"`
	Owner          appGroupIdentityResponse `json:"owner"`
	PostingPolicy  string                   `json:"posting_policy"`
	Status         string                   `json:"status"`
	UpdatedAt      string                   `json:"updated_at"`
	Visibility     string                   `json:"visibility"`
}

type appGetGroupRequest struct {
	ConversationID string `json:"conversation_id"`
}

type appGetGroupResponse struct {
	Conversation appGroupDetailResponse `json:"conversation"`
}

type appListGroupMembersRequest struct {
	ConversationID string `json:"conversation_id"`
	Page           int    `json:"page"`
	PageSize       int    `json:"page_size"`
}

type appGroupMemberResponse struct {
	appGroupIdentityResponse
	JoinedAt string `json:"joined_at"`
	Role     string `json:"role"`
}

type appListGroupMembersResponse struct {
	Members  []appGroupMemberResponse `json:"members"`
	Page     int                      `json:"page"`
	PageSize int                      `json:"page_size"`
	Total    int64                    `json:"total"`
}

type appRemoveGroupMemberRequest struct {
	ConversationID string `json:"conversation_id"`
	MemberID       string `json:"member_id"`
	MemberType     string `json:"member_type"`
}

type appSetGroupMemberRoleRequest struct {
	ConversationID string `json:"conversation_id"`
	MemberID       string `json:"member_id"`
	MemberType     string `json:"member_type"`
	Role           string `json:"role"`
}

type appUpdateGroupRequest struct {
	ConversationID string `json:"conversation_id"`
	Name           string `json:"name"`
}

type appDissolveGroupRequest struct {
	ConversationID string `json:"conversation_id"`
}

type appGroupMutationResponse struct {
	Conversation appGroupDetailResponse `json:"conversation"`
	Message      *appMessagePayload     `json:"message"`
}

type appDissolveGroupResponse struct {
	ConversationID string `json:"conversation_id"`
}

func (s *Server) handleAppGetUser(appID string, request realtime.Envelope) (appGetUserResponse, error) {
	var req appGetUserRequest
	if err := json.Unmarshal(request.Payload, &req); err != nil {
		return appGetUserResponse{}, newAppRequestFailure("invalid_request", "请求格式错误")
	}
	userID := strings.TrimSpace(req.UserID)
	if _, err := uuid.Parse(userID); err != nil {
		return appGetUserResponse{}, newAppRequestFailure("invalid_request", "用户 ID 格式错误")
	}
	allowed, err := s.apps.CanUserAccess(context.Background(), appID, userID)
	if err != nil {
		return appGetUserResponse{}, err
	}
	if !allowed {
		return appGetUserResponse{}, newAppRequestFailure("not_found", "用户不存在")
	}
	var user store.User
	if err := s.db.First(&user, "id = ? AND status = ?", userID, store.UserStatusActive).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return appGetUserResponse{}, newAppRequestFailure("not_found", "用户不存在")
		}
		return appGetUserResponse{}, err
	}
	return appGetUserResponse{User: appUserResourceResponse{
		Avatar: user.Avatar, Email: user.Email, ID: user.ID, Name: user.Name, Nickname: user.Nickname,
	}}, nil
}

func (s *Server) handleAppListOwnConversations(appID string, req appListConversationsRequest) (appListConversationsResponse, error) {
	limit := normalizeAppScopedLimit(req.Limit, defaultAppConversationListLimit)
	keyword := strings.ToLower(strings.TrimSpace(req.Keyword))
	db := s.db.Model(&store.Conversation{})
	var conversations []store.Conversation
	query := db.
		Joins("JOIN conversation_members cm ON cm.conversation_id = conversations.id").
		Where("cm.member_type = ? AND cm.member_id = ? AND cm.left_at IS NULL", store.ConversationMemberTypeApp, appID).
		Where("conversations.kind <> ? AND conversations.status = ?", store.ConversationKindTopic, store.ConversationStatusActive)
	if keyword != "" {
		query = query.Where("LOWER(conversations.name) LIKE ?", "%"+keyword+"%")
	}
	if err := query.
		Order("COALESCE(conversations.last_message_at, conversations.created_at) DESC").
		Order("conversations.id ASC").Limit(limit).Find(&conversations).Error; err != nil {
		return appListConversationsResponse{}, err
	}
	var topics []store.Conversation
	topicQuery := s.db.Model(&store.Conversation{}).
		Joins("JOIN conversation_topic_participants ctp ON ctp.conversation_id = conversations.id").
		Joins("JOIN conversation_topics ct ON ct.conversation_id = conversations.id").
		Joins("JOIN conversation_members parent_cm ON parent_cm.conversation_id = ct.parent_conversation_id").
		Where("ctp.participant_type = ? AND ctp.participant_id = ?", store.ConversationMemberTypeApp, appID).
		Where("parent_cm.member_type = ? AND parent_cm.member_id = ? AND parent_cm.left_at IS NULL", store.ConversationMemberTypeApp, appID).
		Where("ct.archived_at IS NULL").
		Where("conversations.status = ?", store.ConversationStatusActive)
	if keyword != "" {
		topicQuery = topicQuery.Where("LOWER(conversations.name) LIKE ?", "%"+keyword+"%")
	}
	if err := topicQuery.
		Order("COALESCE(conversations.last_message_at, conversations.created_at) DESC").
		Order("conversations.id ASC").Limit(limit).Find(&topics).Error; err != nil {
		return appListConversationsResponse{}, err
	}
	conversations = append(conversations, topics...)
	sort.Slice(conversations, func(left, right int) bool {
		leftAt := conversations[left].CreatedAt
		if conversations[left].LastMessageAt != nil {
			leftAt = *conversations[left].LastMessageAt
		}
		rightAt := conversations[right].CreatedAt
		if conversations[right].LastMessageAt != nil {
			rightAt = *conversations[right].LastMessageAt
		}
		if !leftAt.Equal(rightAt) {
			return leftAt.After(rightAt)
		}
		return conversations[left].ID < conversations[right].ID
	})
	if len(conversations) > limit {
		conversations = conversations[:limit]
	}
	responses := make([]appConversationSummaryPayload, 0, len(conversations))
	for _, conversation := range conversations {
		lastActiveAt := conversation.CreatedAt
		if conversation.LastMessageAt != nil {
			lastActiveAt = *conversation.LastMessageAt
		}
		var memberCount int64
		memberQuery := s.db.Model(&store.ConversationMember{}).
			Where("conversation_id = ? AND left_at IS NULL", conversation.ID)
		if conversation.Kind == store.ConversationKindTopic {
			memberQuery = s.db.Model(&store.ConversationTopicParticipant{}).
				Where("conversation_id = ?", conversation.ID)
		}
		if err := memberQuery.Count(&memberCount).Error; err != nil {
			return appListConversationsResponse{}, err
		}
		responses = append(responses, appConversationSummaryPayload{
			ConversationID: conversation.ID, LastActiveAt: lastActiveAt,
			MemberCount: int(memberCount), Name: conversation.Name, Type: conversation.Kind,
		})
	}
	return appListConversationsResponse{Conversations: responses, Limit: limit}, nil
}

func (s *Server) handleAppGetApplication(callerAppID string, request realtime.Envelope) (appGetApplicationResponse, error) {
	var req appGetApplicationRequest
	if err := json.Unmarshal(request.Payload, &req); err != nil {
		return appGetApplicationResponse{}, newAppRequestFailure("invalid_request", "请求格式错误")
	}
	targetAppID := strings.TrimSpace(req.AppID)
	if _, err := uuid.Parse(targetAppID); err != nil {
		return appGetApplicationResponse{}, newAppRequestFailure("invalid_request", "应用 ID 格式错误")
	}
	var target store.App
	if err := appapp.ApplyUsableScope(s.db.Model(&store.App{})).First(&target, "apps.id = ?", targetAppID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return appGetApplicationResponse{}, newAppRequestFailure("not_found", "应用不存在")
		}
		return appGetApplicationResponse{}, err
	}
	allowed := targetAppID == callerAppID || target.Visibility == store.AppVisibilityPublic
	if !allowed {
		var sharedCount int64
		err := s.db.Model(&store.ConversationMember{}).Table("conversation_members caller").
			Joins("JOIN conversation_members target ON target.conversation_id = caller.conversation_id").
			Joins("JOIN conversations c ON c.id = caller.conversation_id").
			Where("caller.member_type = ? AND caller.member_id = ? AND caller.left_at IS NULL", store.ConversationMemberTypeApp, callerAppID).
			Where("target.member_type = ? AND target.member_id = ? AND target.left_at IS NULL", store.ConversationMemberTypeApp, targetAppID).
			Where("c.kind = ? AND c.status = ?", store.ConversationKindGroup, store.ConversationStatusActive).
			Count(&sharedCount).Error
		if err != nil {
			return appGetApplicationResponse{}, err
		}
		allowed = sharedCount > 0
	}
	if !allowed {
		return appGetApplicationResponse{}, newAppRequestFailure("not_found", "应用不存在")
	}
	return appGetApplicationResponse{App: appApplicationResourceResponse{
		Avatar: target.Avatar, Description: target.Description, Enabled: target.Enabled,
		ID: target.ID, Name: target.Name, Online: s.appConnections.IsOnline(target.ID), Visibility: target.Visibility,
	}}, nil
}

func (s *Server) handleAppGetGroup(appID string, request realtime.Envelope) (appGetGroupResponse, error) {
	var req appGetGroupRequest
	if err := json.Unmarshal(request.Payload, &req); err != nil {
		return appGetGroupResponse{}, newAppRequestFailure("invalid_request", "请求格式错误")
	}
	result, err := s.conversations.GetGroupForApplication(context.Background(), conversationapp.GetGroupForApplicationCommand{
		AppID: appID, ConversationID: req.ConversationID,
	})
	if err != nil {
		return appGetGroupResponse{}, mapConversationApplicationErrorForApp(err)
	}
	return appGetGroupResponse{Conversation: newAppGroupDetailResponse(result)}, nil
}

func (s *Server) handleAppListGroupMembers(appID string, request realtime.Envelope) (appListGroupMembersResponse, error) {
	var req appListGroupMembersRequest
	if err := json.Unmarshal(request.Payload, &req); err != nil {
		return appListGroupMembersResponse{}, newAppRequestFailure("invalid_request", "请求格式错误")
	}
	result, err := s.conversations.ListGroupMembersForApplication(context.Background(), conversationapp.ListGroupMembersForApplicationCommand{
		AppID: appID, ConversationID: req.ConversationID, Page: req.Page, PageSize: req.PageSize,
	})
	if err != nil {
		return appListGroupMembersResponse{}, mapConversationApplicationErrorForApp(err)
	}
	members := make([]appGroupMemberResponse, 0, len(result.Members))
	for _, member := range result.Members {
		members = append(members, appGroupMemberResponse{
			appGroupIdentityResponse: newAppGroupIdentityResponse(member.ApplicationGroupIdentity),
			JoinedAt:                 member.JoinedAt.UTC().Format(time.RFC3339Nano), Role: member.Role,
		})
	}
	return appListGroupMembersResponse{
		Members: members, Page: result.Page, PageSize: result.PageSize, Total: result.Total,
	}, nil
}

func (s *Server) handleAppRemoveGroupMember(appID string, request realtime.Envelope) (appGroupMutationResponse, error) {
	var req appRemoveGroupMemberRequest
	if err := json.Unmarshal(request.Payload, &req); err != nil {
		return appGroupMutationResponse{}, newAppRequestFailure("invalid_request", "请求格式错误")
	}
	result, err := s.conversations.RemoveGroupMemberAsApplication(context.Background(), conversationapp.RemoveGroupMemberAsApplicationCommand{
		AppID: appID, ConversationID: req.ConversationID,
		MemberID: req.MemberID, MemberType: req.MemberType,
	})
	if err != nil {
		return appGroupMutationResponse{}, mapConversationApplicationErrorForApp(err)
	}
	return newAppGroupMutationResponse(result), nil
}

func (s *Server) handleAppSetGroupMemberRole(appID string, request realtime.Envelope) (appGroupMutationResponse, error) {
	var req appSetGroupMemberRoleRequest
	if err := json.Unmarshal(request.Payload, &req); err != nil {
		return appGroupMutationResponse{}, newAppRequestFailure("invalid_request", "请求格式错误")
	}
	result, err := s.conversations.SetGroupMemberRoleAsApplication(context.Background(), conversationapp.SetGroupMemberRoleAsApplicationCommand{
		AppID: appID, ConversationID: req.ConversationID,
		MemberID: req.MemberID, MemberType: req.MemberType, Role: req.Role,
	})
	if err != nil {
		return appGroupMutationResponse{}, mapConversationApplicationErrorForApp(err)
	}
	return newAppGroupMutationResponse(result), nil
}

func (s *Server) handleAppUpdateGroup(appID string, request realtime.Envelope) (appGroupMutationResponse, error) {
	var req appUpdateGroupRequest
	if err := json.Unmarshal(request.Payload, &req); err != nil {
		return appGroupMutationResponse{}, newAppRequestFailure("invalid_request", "请求格式错误")
	}
	result, err := s.conversations.UpdateGroupNameAsApplication(context.Background(), conversationapp.UpdateGroupNameAsApplicationCommand{
		AppID: appID, ConversationID: req.ConversationID, Name: req.Name,
	})
	if err != nil {
		return appGroupMutationResponse{}, mapConversationApplicationErrorForApp(err)
	}
	return newAppGroupMutationResponse(result), nil
}

func (s *Server) handleAppDissolveGroup(appID string, request realtime.Envelope) (appDissolveGroupResponse, error) {
	var req appDissolveGroupRequest
	if err := json.Unmarshal(request.Payload, &req); err != nil {
		return appDissolveGroupResponse{}, newAppRequestFailure("invalid_request", "请求格式错误")
	}
	result, err := s.conversations.DissolveGroupAsApplication(context.Background(), conversationapp.DissolveGroupAsApplicationCommand{
		AppID: appID, ConversationID: req.ConversationID,
	})
	if err != nil {
		return appDissolveGroupResponse{}, mapConversationApplicationErrorForApp(err)
	}
	return appDissolveGroupResponse{ConversationID: result.ConversationID}, nil
}

func newAppGroupIdentityResponse(value conversationapp.ApplicationGroupIdentity) appGroupIdentityResponse {
	return appGroupIdentityResponse{
		Avatar: value.Avatar, Email: value.Email, ID: value.ID, Name: value.Name,
		Nickname: value.Nickname, Type: value.Type,
	}
}

func newAppGroupDetailResponse(value conversationapp.ApplicationGroupDetail) appGroupDetailResponse {
	return appGroupDetailResponse{
		Avatar: value.Avatar, CreatedAt: value.CreatedAt.UTC().Format(time.RFC3339Nano),
		CreatedBy: newAppGroupIdentityResponse(value.CreatedBy), CurrentAppRole: value.CurrentAppRole,
		ID: value.ID, MemberCount: value.MemberCount, Name: value.Name,
		Owner: newAppGroupIdentityResponse(value.Owner), PostingPolicy: value.PostingPolicy,
		Status: value.Status, UpdatedAt: value.UpdatedAt.UTC().Format(time.RFC3339Nano), Visibility: value.Visibility,
	}
}

func newAppGroupMutationResponse(value conversationapp.ApplicationGroupMutationResult) appGroupMutationResponse {
	var message *appMessagePayload
	if value.Message != nil {
		converted := newAppSystemMessagePayload(*value.Message)
		message = &converted
	}
	return appGroupMutationResponse{
		Conversation: newAppGroupDetailResponse(value.Conversation), Message: message,
	}
}

func mapConversationApplicationErrorForApp(err error) error {
	code := conversationapp.ErrorCodeOf(err)
	switch code {
	case conversationapp.CodeInvalidRequest:
		return newAppRequestFailure("invalid_request", conversationapp.ErrorMessage(err))
	case conversationapp.CodeForbidden:
		return newAppRequestFailure("forbidden", conversationapp.ErrorMessage(err))
	case conversationapp.CodeNotFound:
		return newAppRequestFailure("not_found", conversationapp.ErrorMessage(err))
	case conversationapp.CodeConflict:
		return newAppRequestFailure("conflict", conversationapp.ErrorMessage(err))
	default:
		return newAppRequestFailure("internal_error", "服务端错误")
	}
}
