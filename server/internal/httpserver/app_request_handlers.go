package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"app/internal/realtime"
	"app/internal/store"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	appMethodMessageSend               = "message.send"
	appMethodConversationMessagesList  = "conversation.messages.list"
	defaultAppConversationHistoryLimit = 30
	maxAppConversationHistoryLimit     = 100

	appMessageTargetUser  = "user"
	appMessageTargetGroup = "group"
	appMessageTargetApp   = "app"
)

type appSendMessageRequest struct {
	Message json.RawMessage      `json:"message"`
	Target  appSendMessageTarget `json:"target"`
	UserID  string               `json:"user_id"`
}

type appSendMessageTarget struct {
	Type           string `json:"type"`
	UserID         string `json:"user_id"`
	ConversationID string `json:"conversation_id"`
}

type appSendMessageResponse struct {
	Conversation appMessageConversationPayload `json:"conversation"`
	Created      bool                          `json:"created"`
	Message      appMessagePayload             `json:"message"`
}

type appListConversationMessagesRequest struct {
	BeforeOrEqualSeq int64  `json:"before_or_equal_seq"`
	ConversationID   string `json:"conversation_id"`
	Limit            int    `json:"limit"`
}

type appListConversationMessagesResponse struct {
	Limit    int                                    `json:"limit"`
	Messages []appConversationHistoryMessagePayload `json:"messages"`
}

type appConversationHistoryMessagePayload struct {
	CreatedAt time.Time               `json:"created_at"`
	ID        string                  `json:"id"`
	Sender    appMessageSenderPayload `json:"sender"`
	Seq       int64                   `json:"seq"`
	Summary   string                  `json:"summary"`
}

type appRequestFailure struct {
	Code    string
	Message string
}

func (e appRequestFailure) Error() string {
	return e.Message
}

func (s *Server) handleAppRequest(appID string, request realtime.Envelope) realtime.Envelope {
	switch strings.TrimSpace(request.Method) {
	case appMethodMessageSend:
		response, err := s.handleAppSendMessage(appID, request)
		if err != nil {
			return appRequestErrorResponse(request.ID, err)
		}
		return realtime.NewResponse(request.ID, response)
	case appMethodConversationMessagesList:
		response, err := s.handleAppListConversationMessages(appID, request)
		if err != nil {
			return appRequestErrorResponse(request.ID, err)
		}
		return realtime.NewResponse(request.ID, response)
	default:
		return realtime.NewErrorResponse(request.ID, "unknown_method", "未知应用方法")
	}
}

func (s *Server) handleAppSendMessage(appID string, request realtime.Envelope) (appSendMessageResponse, error) {
	var req appSendMessageRequest
	if err := json.Unmarshal(request.Payload, &req); err != nil {
		return appSendMessageResponse{}, newAppRequestFailure("invalid_request", "请求格式错误")
	}

	target, err := normalizeAppSendMessageTarget(req)
	if err != nil {
		return appSendMessageResponse{}, err
	}
	body, err := normalizeAppSendMessageBody(context.Background(), req.Message)
	if err != nil {
		return appSendMessageResponse{}, err
	}

	conversation, err := s.findAppSendMessageConversation(appID, target)
	if err != nil {
		return appSendMessageResponse{}, err
	}

	message, created, memberUserIDs, err := s.createAppMessage(appID, conversation.ID, request.ID, body, finalizeNormalizedMessageBody)
	if err != nil {
		return appSendMessageResponse{}, mapAppMessageError(err)
	}
	if created {
		s.realtime.SendToUsers(memberUserIDs, realtimeMessageCreatedEvent(newMessageResponse(message)))
	}

	return appSendMessageResponse{
		Conversation: appMessageConversationPayload{
			ID:   conversation.ID,
			Name: conversation.Name,
			Type: conversation.Kind,
		},
		Created: created,
		Message: appMessagePayload{
			Body:      message.Body,
			CreatedAt: message.CreatedAt,
			ID:        message.ID,
			Seq:       message.Seq,
			Summary:   message.Summary,
		},
	}, nil
}

func (s *Server) handleAppListConversationMessages(appID string, request realtime.Envelope) (appListConversationMessagesResponse, error) {
	var req appListConversationMessagesRequest
	if err := json.Unmarshal(request.Payload, &req); err != nil {
		return appListConversationMessagesResponse{}, newAppRequestFailure("invalid_request", "请求格式错误")
	}

	req, err := normalizeAppListConversationMessagesRequest(req)
	if err != nil {
		return appListConversationMessagesResponse{}, err
	}

	member, err := s.requireReadableAppConversationMember(appID, req.ConversationID)
	if err != nil {
		return appListConversationMessagesResponse{}, err
	}
	visibleFromSeq := member.HistoryVisibleFromSeq
	if visibleFromSeq < 1 {
		visibleFromSeq = 1
	}

	var messages []store.Message
	if err := s.db.
		Where("conversation_id = ? AND deleted_at IS NULL AND seq >= ? AND seq <= ?", req.ConversationID, visibleFromSeq, req.BeforeOrEqualSeq).
		Order("seq DESC").
		Limit(req.Limit).
		Find(&messages).Error; err != nil {
		return appListConversationMessagesResponse{}, err
	}
	reverseMessages(messages)

	payloads, err := s.newAppConversationHistoryMessagePayloads(messages)
	if err != nil {
		return appListConversationMessagesResponse{}, err
	}

	return appListConversationMessagesResponse{
		Limit:    req.Limit,
		Messages: payloads,
	}, nil
}

func normalizeAppListConversationMessagesRequest(req appListConversationMessagesRequest) (appListConversationMessagesRequest, error) {
	req.ConversationID = strings.TrimSpace(req.ConversationID)
	if _, err := uuid.Parse(req.ConversationID); err != nil {
		return appListConversationMessagesRequest{}, newAppRequestFailure("invalid_request", "会话 ID 格式错误")
	}
	if req.BeforeOrEqualSeq <= 0 {
		return appListConversationMessagesRequest{}, newAppRequestFailure("invalid_request", "before_or_equal_seq 必须是正整数")
	}
	if req.Limit <= 0 {
		req.Limit = defaultAppConversationHistoryLimit
	}
	if req.Limit > maxAppConversationHistoryLimit {
		req.Limit = maxAppConversationHistoryLimit
	}

	return req, nil
}

func (s *Server) requireReadableAppConversationMember(appID string, conversationID string) (store.ConversationMember, error) {
	var conversation store.Conversation
	if err := s.db.First(&conversation, "id = ?", conversationID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return store.ConversationMember{}, newAppRequestFailure("not_found", "会话不存在")
		}
		return store.ConversationMember{}, err
	}
	if conversation.Status != store.ConversationStatusActive {
		return store.ConversationMember{}, newAppRequestFailure("forbidden", "无权访问会话")
	}

	var member store.ConversationMember
	err := s.db.First(
		&member,
		"conversation_id = ? AND member_type = ? AND member_id = ? AND left_at IS NULL",
		conversationID,
		store.ConversationMemberTypeApp,
		appID,
	).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return store.ConversationMember{}, newAppRequestFailure("forbidden", "无权访问会话")
	}
	if err != nil {
		return store.ConversationMember{}, err
	}

	return member, nil
}

func (s *Server) newAppConversationHistoryMessagePayloads(messages []store.Message) ([]appConversationHistoryMessagePayload, error) {
	userIDs := make([]string, 0)
	appIDs := make([]string, 0)
	for _, message := range messages {
		if message.SenderID == nil {
			continue
		}
		switch message.SenderType {
		case store.MessageSenderTypeUser:
			userIDs = append(userIDs, *message.SenderID)
		case store.MessageSenderTypeApp:
			appIDs = append(appIDs, *message.SenderID)
		}
	}

	usersByID := map[string]store.User{}
	if len(userIDs) > 0 {
		var users []store.User
		if err := s.db.Find(&users, "id IN ?", userIDs).Error; err != nil {
			return nil, err
		}
		for _, user := range users {
			usersByID[user.ID] = user
		}
	}
	appsByID := map[string]store.App{}
	if len(appIDs) > 0 {
		var apps []store.App
		if err := s.db.Find(&apps, "id IN ?", appIDs).Error; err != nil {
			return nil, err
		}
		for _, app := range apps {
			appsByID[app.ID] = app
		}
	}

	payloads := make([]appConversationHistoryMessagePayload, 0, len(messages))
	for _, message := range messages {
		payloads = append(payloads, appConversationHistoryMessagePayload{
			CreatedAt: message.CreatedAt,
			ID:        message.ID,
			Sender:    newAppHistoryMessageSenderPayload(message, usersByID, appsByID),
			Seq:       message.Seq,
			Summary:   message.Summary,
		})
	}

	return payloads, nil
}

func newAppHistoryMessageSenderPayload(message store.Message, usersByID map[string]store.User, appsByID map[string]store.App) appMessageSenderPayload {
	sender := appMessageSenderPayload{Type: message.SenderType}
	if message.SenderID == nil {
		return sender
	}

	sender.ID = *message.SenderID
	switch message.SenderType {
	case store.MessageSenderTypeUser:
		if user, ok := usersByID[*message.SenderID]; ok {
			sender.Name = user.Name
			sender.Nickname = user.Nickname
		}
	case store.MessageSenderTypeApp:
		if app, ok := appsByID[*message.SenderID]; ok {
			sender.Name = app.Name
		}
	}

	return sender
}

func normalizeAppSendMessageTarget(req appSendMessageRequest) (appSendMessageTarget, error) {
	target := req.Target
	target.Type = strings.TrimSpace(target.Type)
	target.UserID = strings.TrimSpace(firstNonEmptyAppString(target.UserID, req.UserID))
	target.ConversationID = strings.TrimSpace(target.ConversationID)

	if target.Type == "" && target.UserID != "" {
		target.Type = appMessageTargetUser
	}

	switch target.Type {
	case appMessageTargetUser:
		if _, err := uuid.Parse(target.UserID); err != nil {
			return appSendMessageTarget{}, newAppRequestFailure("invalid_request", "用户 ID 格式错误")
		}
	case appMessageTargetGroup, appMessageTargetApp:
		if _, err := uuid.Parse(target.ConversationID); err != nil {
			return appSendMessageTarget{}, newAppRequestFailure("invalid_request", "会话 ID 格式错误")
		}
	default:
		return appSendMessageTarget{}, newAppRequestFailure("invalid_request", "消息目标类型不支持")
	}

	return target, nil
}

func normalizeAppSendMessageBody(ctx context.Context, raw json.RawMessage) (json.RawMessage, error) {
	handler, err := findMessageBodyHandler(raw)
	if err != nil {
		return nil, newAppRequestFailure("invalid_request", err.Error())
	}
	body, err := handler.Normalize(ctx, raw)
	if err != nil {
		return nil, newAppRequestFailure("invalid_request", err.Error())
	}

	return body, nil
}

func (s *Server) findAppSendMessageConversation(appID string, target appSendMessageTarget) (store.Conversation, error) {
	switch target.Type {
	case appMessageTargetUser:
		var user store.User
		err := s.db.First(&user, "id = ? AND status = ?", target.UserID, store.UserStatusActive).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return store.Conversation{}, newAppRequestFailure("not_found", "用户不存在")
		}
		if err != nil {
			return store.Conversation{}, err
		}

		app, ok, err := s.findAppForConnection(appID)
		if err != nil {
			return store.Conversation{}, err
		}
		if !ok || !app.Enabled {
			return store.Conversation{}, newAppRequestFailure("forbidden", "应用不可用")
		}

		conversation, _, err := s.getOrCreateAppConversation(user, app)
		return conversation, err
	case appMessageTargetGroup, appMessageTargetApp:
		return s.findAppWritableConversation(appID, target.ConversationID, target.Type)
	default:
		return store.Conversation{}, newAppRequestFailure("invalid_request", "消息目标类型不支持")
	}
}

func (s *Server) findAppWritableConversation(appID string, conversationID string, expectedKind string) (store.Conversation, error) {
	var conversation store.Conversation
	if err := s.db.First(&conversation, "id = ?", conversationID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return store.Conversation{}, newAppRequestFailure("not_found", "会话不存在")
		}
		return store.Conversation{}, err
	}
	if conversation.Kind != expectedKind {
		return store.Conversation{}, newAppRequestFailure("invalid_request", "会话类型不匹配")
	}
	if conversation.Status != store.ConversationStatusActive ||
		conversation.PostingPolicy != store.ConversationPostingPolicyOpen {
		return store.Conversation{}, newAppRequestFailure("forbidden", "当前会话不能发送消息")
	}

	var member store.ConversationMember
	err := s.db.First(
		&member,
		"conversation_id = ? AND member_type = ? AND member_id = ? AND left_at IS NULL",
		conversationID,
		store.ConversationMemberTypeApp,
		appID,
	).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return store.Conversation{}, newAppRequestFailure("forbidden", "应用不在当前会话中")
	}
	if err != nil {
		return store.Conversation{}, err
	}

	return conversation, nil
}

func (s *Server) createAppMessage(appID string, conversationID string, clientMessageID string, body json.RawMessage, finalizeBody finalizeMessageBodyFunc) (store.Message, bool, []string, error) {
	var created bool
	var message store.Message
	memberUserIDs := []string{}

	err := s.db.Transaction(func(tx *gorm.DB) error {
		var conversation store.Conversation
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&conversation, "id = ?", conversationID).Error; err != nil {
			return err
		}
		if conversation.Status != store.ConversationStatusActive ||
			conversation.PostingPolicy != store.ConversationPostingPolicyOpen {
			return errConversationNotSendable
		}

		var member store.ConversationMember
		if err := tx.First(
			&member,
			"conversation_id = ? AND member_type = ? AND member_id = ? AND left_at IS NULL",
			conversationID,
			store.ConversationMemberTypeApp,
			appID,
		).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errConversationAccessDenied
			}
			return err
		}

		var existing store.Message
		err := tx.First(
			&existing,
			"conversation_id = ? AND sender_type = ? AND sender_id = ? AND client_message_id = ?",
			conversationID,
			store.MessageSenderTypeApp,
			appID,
			clientMessageID,
		).Error
		if err == nil {
			message = existing
			created = false
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		finalBody, summary, err := finalizeBody(context.Background(), body)
		if err != nil {
			return err
		}

		now := time.Now().UTC()
		message = store.Message{
			ID:              uuid.NewString(),
			ConversationID:  conversationID,
			Seq:             conversation.LastMessageSeq + 1,
			SenderType:      store.MessageSenderTypeApp,
			SenderID:        &appID,
			ClientMessageID: &clientMessageID,
			Body:            finalBody,
			Summary:         summary,
			CreatedAt:       now,
			UpdatedAt:       now,
		}
		if err := tx.Create(&message).Error; err != nil {
			return err
		}

		if err := tx.Model(&store.Conversation{}).
			Where("id = ?", conversationID).
			Updates(map[string]any{
				"last_message_at":      message.CreatedAt,
				"last_message_id":      message.ID,
				"last_message_seq":     message.Seq,
				"last_message_summary": message.Summary,
				"updated_at":           now,
			}).Error; err != nil {
			return err
		}

		ids, err := loadActiveConversationUserIDs(tx, conversationID)
		if err != nil {
			return err
		}
		memberUserIDs = ids
		created = true
		return nil
	})
	if err != nil {
		return store.Message{}, false, nil, err
	}

	return message, created, memberUserIDs, nil
}

func appRequestErrorResponse(replyTo string, err error) realtime.Envelope {
	var requestErr appRequestFailure
	if errors.As(err, &requestErr) {
		return realtime.NewErrorResponse(replyTo, requestErr.Code, requestErr.Message)
	}

	return realtime.NewErrorResponse(replyTo, "internal_error", "服务端错误")
}

func mapAppMessageError(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return newAppRequestFailure("not_found", "会话不存在")
	}
	if errors.Is(err, errConversationAccessDenied) {
		return newAppRequestFailure("forbidden", "无权访问会话")
	}
	if errors.Is(err, errConversationNotSendable) {
		return newAppRequestFailure("forbidden", "当前会话不能发送消息")
	}

	return err
}

func newAppRequestFailure(code string, message string) appRequestFailure {
	return appRequestFailure{
		Code:    code,
		Message: message,
	}
}

func firstNonEmptyAppString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}

	return ""
}
