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
	appMethodMessageSendAsUser         = "message.send_as_user"
	appMethodContactsUsersList         = "contacts.users.list"
	appMethodConversationMessagesList  = "conversation.messages.list"
	appMethodGroupConversationsList    = "group_conversations.list"
	appMethodGroupConversationsCreate  = "group_conversations.create"
	appMethodGroupMembersAdd           = "group_conversations.members.add"
	appMethodTemporaryFilesReadURLs    = "temporary_files.read_urls"
	defaultAppConversationHistoryLimit = 30
	maxAppConversationHistoryLimit     = 100
	defaultAppGroupConversationLimit   = 30

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

type appCreateGroupConversationResponse struct {
	Conversation groupConversationResponse `json:"conversation"`
	Message      appMessagePayload         `json:"message"`
}

type appAddGroupConversationMembersResponse struct {
	Conversation conversationListItemResponse `json:"conversation"`
	Message      *appMessagePayload           `json:"message"`
}

type appSendAsUserRequest struct {
	ActorUserID      string              `json:"actor_user_id"`
	Message          json.RawMessage     `json:"message"`
	Target           appSendAsUserTarget `json:"target"`
	TargetUserID     string              `json:"target_user_id"`
	TriggerMessageID string              `json:"trigger_message_id"`
}

type appSendAsUserTarget struct {
	ConversationID string `json:"conversation_id"`
	Type           string `json:"type"`
	UserID         string `json:"user_id"`
}

type appCreateGroupConversationRequest struct {
	ActorUserID      string   `json:"actor_user_id"`
	MemberIDs        []string `json:"member_ids"`
	Name             string   `json:"name"`
	TriggerMessageID string   `json:"trigger_message_id"`
}

type appAddGroupConversationMembersRequest struct {
	ActorUserID      string   `json:"actor_user_id"`
	ConversationID   string   `json:"conversation_id"`
	MemberIDs        []string `json:"member_ids"`
	TriggerMessageID string   `json:"trigger_message_id"`
}

type appListContactUsersRequest struct {
	Keyword string `json:"keyword"`
}

type appListContactUsersResponse struct {
	Contacts []contactUserResponse `json:"contacts"`
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

type appListGroupConversationsRequest struct {
	ActorUserID      string `json:"actor_user_id"`
	Keyword          string `json:"keyword"`
	Limit            int    `json:"limit"`
	TriggerMessageID string `json:"trigger_message_id"`
}

type appListGroupConversationsResponse struct {
	Groups []conversationListItemResponse `json:"groups"`
	Limit  int                            `json:"limit"`
}

type appReadTemporaryFileURLsRequest struct {
	ConversationID string   `json:"conversation_id"`
	FileIDs        []string `json:"file_ids"`
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
	case appMethodMessageSendAsUser:
		response, err := s.handleAppSendMessageAsUser(appID, request)
		if err != nil {
			return appRequestErrorResponse(request.ID, err)
		}
		return realtime.NewResponse(request.ID, response)
	case appMethodContactsUsersList:
		response, err := s.handleAppListContactUsers(request)
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
	case appMethodGroupConversationsList:
		response, err := s.handleAppListGroupConversations(appID, request)
		if err != nil {
			return appRequestErrorResponse(request.ID, err)
		}
		return realtime.NewResponse(request.ID, response)
	case appMethodGroupConversationsCreate:
		response, err := s.handleAppCreateGroupConversation(appID, request)
		if err != nil {
			return appRequestErrorResponse(request.ID, err)
		}
		return realtime.NewResponse(request.ID, response)
	case appMethodGroupMembersAdd:
		response, err := s.handleAppAddGroupConversationMembers(appID, request)
		if err != nil {
			return appRequestErrorResponse(request.ID, err)
		}
		return realtime.NewResponse(request.ID, response)
	case appMethodTemporaryFilesReadURLs:
		response, err := s.handleAppReadTemporaryFileURLs(appID, request)
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

	conversation, err := s.findAppSendMessageConversation(appID, target)
	if err != nil {
		return appSendMessageResponse{}, err
	}
	prepared, err := s.prepareAppSendMessageBody(context.Background(), req.Message)
	if err != nil {
		return appSendMessageResponse{}, err
	}

	message, created, memberUserIDs, err := s.createAppMessage(appID, conversation.ID, request.ID, prepared.Body, prepared.Finalize)
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
			Sender: &appMessageSenderPayload{
				ID:   appID,
				Type: store.MessageSenderTypeApp,
			},
			Summary: message.Summary,
		},
	}, nil
}

func (s *Server) handleAppSendMessageAsUser(appID string, request realtime.Envelope) (appSendMessageResponse, error) {
	var req appSendAsUserRequest
	if err := json.Unmarshal(request.Payload, &req); err != nil {
		return appSendMessageResponse{}, newAppRequestFailure("invalid_request", "请求格式错误")
	}

	req, err := normalizeAppSendAsUserRequest(req)
	if err != nil {
		return appSendMessageResponse{}, err
	}
	if err := s.requireAppSendAsUserTrigger(appID, req.ActorUserID, req.TriggerMessageID); err != nil {
		return appSendMessageResponse{}, err
	}

	actor, conversation, err := s.findAppSendAsUserConversation(req)
	if err != nil {
		return appSendMessageResponse{}, err
	}
	prepared, err := s.prepareAppSendMessageBody(context.Background(), req.Message)
	if err != nil {
		return appSendMessageResponse{}, err
	}
	app, ok, err := s.findAppForConnection(appID)
	if err != nil {
		return appSendMessageResponse{}, err
	}
	if !ok || !app.Enabled {
		return appSendMessageResponse{}, newAppRequestFailure("forbidden", "应用不可用")
	}

	delegatedByType := store.MessageSenderTypeApp
	delegatedByID := app.ID
	message, created, memberUserIDs, err := s.createUserMessageWithMetadata(
		context.Background(),
		actor.ID,
		conversation.ID,
		request.ID,
		prepared.Body,
		prepared.Finalize,
		createMessageMetadata{
			DelegatedByType: &delegatedByType,
			DelegatedByID:   &delegatedByID,
			DelegatedByName: app.Name,
		},
	)
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
			DelegatedBy: &appMessageSenderPayload{
				ID:   app.ID,
				Name: app.Name,
				Type: store.MessageSenderTypeApp,
			},
			ID:  message.ID,
			Seq: message.Seq,
			Sender: &appMessageSenderPayload{
				Email: actor.Email,
				ID:    actor.ID,
				Name:  actor.Name,
				Type:  store.MessageSenderTypeUser,
			},
			Summary: message.Summary,
		},
	}, nil
}

func (s *Server) handleAppListGroupConversations(appID string, request realtime.Envelope) (appListGroupConversationsResponse, error) {
	var req appListGroupConversationsRequest
	if len(request.Payload) > 0 {
		if err := json.Unmarshal(request.Payload, &req); err != nil {
			return appListGroupConversationsResponse{}, newAppRequestFailure("invalid_request", "请求格式错误")
		}
	}

	actorUserID, triggerMessageID, err := normalizeAppActorTrigger(req.ActorUserID, req.TriggerMessageID)
	if err != nil {
		return appListGroupConversationsResponse{}, err
	}
	if err := s.requireAppSendAsUserTrigger(appID, actorUserID, triggerMessageID); err != nil {
		return appListGroupConversationsResponse{}, err
	}
	actor, err := s.findActiveAppActor(actorUserID)
	if err != nil {
		return appListGroupConversationsResponse{}, err
	}

	limit := req.Limit
	if limit <= 0 {
		limit = defaultAppGroupConversationLimit
	}
	if limit > maxClientConversationListItems {
		limit = maxClientConversationListItems
	}
	conversations, err := s.loadAppUserGroupConversations(actor.ID, strings.ToLower(strings.TrimSpace(req.Keyword)), limit)
	if err != nil {
		return appListGroupConversationsResponse{}, err
	}
	conversationIDs := make([]string, 0, len(conversations))
	for _, conversation := range conversations {
		conversationIDs = append(conversationIDs, conversation.ID)
	}
	membersByConversationID, usersByID, err := s.loadConversationListMembers(conversationIDs)
	if err != nil {
		return appListGroupConversationsResponse{}, err
	}

	groups := make([]conversationListItemResponse, 0, len(conversations))
	for _, conversation := range conversations {
		groups = append(groups, newConversationListItemResponse(
			conversation,
			actor.ID,
			membersByConversationID[conversation.ID],
			usersByID,
		))
	}

	return appListGroupConversationsResponse{
		Groups: groups,
		Limit:  limit,
	}, nil
}

func (s *Server) handleAppCreateGroupConversation(appID string, request realtime.Envelope) (appCreateGroupConversationResponse, error) {
	var req appCreateGroupConversationRequest
	if err := json.Unmarshal(request.Payload, &req); err != nil {
		return appCreateGroupConversationResponse{}, newAppRequestFailure("invalid_request", "请求格式错误")
	}

	actorUserID, triggerMessageID, err := normalizeAppActorTrigger(req.ActorUserID, req.TriggerMessageID)
	if err != nil {
		return appCreateGroupConversationResponse{}, err
	}
	name, err := normalizeGroupConversationName(req.Name)
	if err != nil {
		return appCreateGroupConversationResponse{}, newAppRequestFailure("invalid_request", err.Error())
	}
	memberIDs, err := normalizeGroupMemberIDs(req.MemberIDs, actorUserID)
	if err != nil {
		return appCreateGroupConversationResponse{}, newAppRequestFailure("invalid_request", err.Error())
	}
	if len(memberIDs) == 0 {
		return appCreateGroupConversationResponse{}, newAppRequestFailure("invalid_request", "至少选择一名成员")
	}

	if err := s.requireAppSendAsUserTrigger(appID, actorUserID, triggerMessageID); err != nil {
		return appCreateGroupConversationResponse{}, err
	}
	actor, err := s.findActiveAppActor(actorUserID)
	if err != nil {
		return appCreateGroupConversationResponse{}, err
	}

	conversation, message, candidates, memberUserIDs, err := s.createUserGroupConversation(actor, name, memberIDs)
	if err != nil {
		return appCreateGroupConversationResponse{}, mapAppGroupConversationError(err)
	}
	s.realtime.SendToUsers(memberUserIDs, realtimeMessageCreatedEvent(newMessageResponse(message)))

	return appCreateGroupConversationResponse{
		Conversation: newGroupConversationResponse(conversation, candidates, actor.ID),
		Message:      newAppSystemMessagePayload(message),
	}, nil
}

func (s *Server) handleAppAddGroupConversationMembers(appID string, request realtime.Envelope) (appAddGroupConversationMembersResponse, error) {
	var req appAddGroupConversationMembersRequest
	if err := json.Unmarshal(request.Payload, &req); err != nil {
		return appAddGroupConversationMembersResponse{}, newAppRequestFailure("invalid_request", "请求格式错误")
	}

	actorUserID, triggerMessageID, err := normalizeAppActorTrigger(req.ActorUserID, req.TriggerMessageID)
	if err != nil {
		return appAddGroupConversationMembersResponse{}, err
	}
	conversationID := strings.TrimSpace(req.ConversationID)
	if _, err := uuid.Parse(conversationID); err != nil {
		return appAddGroupConversationMembersResponse{}, newAppRequestFailure("invalid_request", "会话 ID 格式错误")
	}
	memberIDs, err := normalizeGroupMemberIDs(req.MemberIDs, actorUserID)
	if err != nil {
		return appAddGroupConversationMembersResponse{}, newAppRequestFailure("invalid_request", err.Error())
	}
	if len(memberIDs) == 0 {
		return appAddGroupConversationMembersResponse{}, newAppRequestFailure("invalid_request", "至少选择一名成员")
	}

	if err := s.requireAppSendAsUserTrigger(appID, actorUserID, triggerMessageID); err != nil {
		return appAddGroupConversationMembersResponse{}, err
	}
	actor, err := s.findActiveAppActor(actorUserID)
	if err != nil {
		return appAddGroupConversationMembersResponse{}, err
	}

	conversation, message, memberUserIDs, err := s.addUserGroupConversationMembers(actor, conversationID, memberIDs)
	if err != nil {
		return appAddGroupConversationMembersResponse{}, mapAppGroupConversationError(err)
	}
	membersByConversationID, usersByID, err := s.loadConversationListMembers([]string{conversation.ID})
	if err != nil {
		return appAddGroupConversationMembersResponse{}, err
	}

	var messageResponse *appMessagePayload
	if message != nil {
		response := newAppSystemMessagePayload(*message)
		messageResponse = &response
		s.realtime.SendToUsers(memberUserIDs, realtimeMessageCreatedEvent(newMessageResponse(*message)))
	}

	return appAddGroupConversationMembersResponse{
		Conversation: newConversationListItemResponse(
			conversation,
			actor.ID,
			membersByConversationID[conversation.ID],
			usersByID,
		),
		Message: messageResponse,
	}, nil
}

func (s *Server) handleAppListContactUsers(request realtime.Envelope) (appListContactUsersResponse, error) {
	var req appListContactUsersRequest
	if len(request.Payload) > 0 {
		if err := json.Unmarshal(request.Payload, &req); err != nil {
			return appListContactUsersResponse{}, newAppRequestFailure("invalid_request", "请求格式错误")
		}
	}

	contacts, err := s.loadContactUsers(strings.ToLower(strings.TrimSpace(req.Keyword)))
	if err != nil {
		return appListContactUsersResponse{}, err
	}

	return appListContactUsersResponse{Contacts: contacts}, nil
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

func (s *Server) handleAppReadTemporaryFileURLs(appID string, request realtime.Envelope) (readTemporaryFileURLsResponse, error) {
	var req appReadTemporaryFileURLsRequest
	if err := json.Unmarshal(request.Payload, &req); err != nil {
		return readTemporaryFileURLsResponse{}, newAppRequestFailure("invalid_request", "请求格式错误")
	}

	conversationID := strings.TrimSpace(req.ConversationID)
	if _, err := uuid.Parse(conversationID); err != nil {
		return readTemporaryFileURLsResponse{}, newAppRequestFailure("invalid_request", "会话 ID 格式错误")
	}
	fileIDs, err := normalizeTemporaryFileIDs(req.FileIDs)
	if err != nil {
		return readTemporaryFileURLsResponse{}, newAppRequestFailure("invalid_request", err.Error())
	}

	member, err := s.requireReadableAppConversationMember(appID, conversationID)
	if err != nil {
		return readTemporaryFileURLsResponse{}, err
	}
	readableFileIDs, err := s.loadAppReadableTemporaryFileIDs(conversationID, member.HistoryVisibleFromSeq, fileIDs)
	if err != nil {
		return readTemporaryFileURLsResponse{}, err
	}
	for _, fileID := range fileIDs {
		if _, ok := readableFileIDs[fileID]; !ok {
			return readTemporaryFileURLsResponse{}, newAppRequestFailure("forbidden", "无权访问临时文件")
		}
	}

	urls, err := s.presignTemporaryFileReadURLsForApp(context.Background(), fileIDs)
	if err != nil {
		return readTemporaryFileURLsResponse{}, err
	}

	return readTemporaryFileURLsResponse{URLs: urls}, nil
}

func (s *Server) loadAppReadableTemporaryFileIDs(conversationID string, historyVisibleFromSeq int64, fileIDs []string) (map[string]struct{}, error) {
	requested := make(map[string]struct{}, len(fileIDs))
	for _, fileID := range fileIDs {
		requested[fileID] = struct{}{}
	}
	if historyVisibleFromSeq < 1 {
		historyVisibleFromSeq = 1
	}

	likeClauses := make([]string, 0, len(fileIDs))
	likeArgs := make([]any, 0, len(fileIDs))
	for _, fileID := range fileIDs {
		likeClauses = append(likeClauses, "body LIKE ?")
		likeArgs = append(likeArgs, "%"+fileID+"%")
	}

	var messages []store.Message
	if err := s.db.
		Select("body").
		Where("conversation_id = ? AND deleted_at IS NULL AND seq >= ?", conversationID, historyVisibleFromSeq).
		Where("("+strings.Join(likeClauses, " OR ")+")", likeArgs...).
		Find(&messages).Error; err != nil {
		return nil, err
	}

	readable := make(map[string]struct{}, len(fileIDs))
	for _, message := range messages {
		var envelope messageBodyEnvelope
		if err := json.Unmarshal(message.Body, &envelope); err != nil {
			continue
		}
		switch envelope.Type {
		case messageTypeImage:
			var body imageMessageBody
			if err := json.Unmarshal(message.Body, &body); err != nil {
				continue
			}
			if _, ok := requested[body.FileID]; ok {
				readable[body.FileID] = struct{}{}
			}
		case messageTypeFile:
			var body fileMessageBody
			if err := json.Unmarshal(message.Body, &body); err != nil {
				continue
			}
			if _, ok := requested[body.FileID]; ok {
				readable[body.FileID] = struct{}{}
			}
		}
	}

	return readable, nil
}

func (s *Server) presignTemporaryFileReadURLsForApp(ctx context.Context, fileIDs []string) ([]temporaryFileReadURLResponse, error) {
	var files []store.TemporaryFile
	if err := s.db.Where("id IN ?", fileIDs).Find(&files).Error; err != nil {
		return nil, err
	}
	if len(files) != len(fileIDs) {
		return nil, newAppRequestFailure("not_found", "临时文件不存在")
	}

	storageClient, err := s.newObjectStoreClient(ctx)
	if err != nil {
		return nil, newAppRequestFailure("internal_error", "临时文件存储未配置")
	}

	filesByID := make(map[string]store.TemporaryFile, len(files))
	for _, file := range files {
		filesByID[file.ID] = file
	}

	urls := make([]temporaryFileReadURLResponse, 0, len(fileIDs))
	now := time.Now().UTC()
	for _, fileID := range fileIDs {
		file := filesByID[fileID]
		if isTemporaryFileExpired(file, s.cfg.Storage.Lifecycle.TemporaryExpireDays, now) {
			return nil, newAppRequestFailure("not_found", "临时文件不存在")
		}
		url, expiresAt, err := storageClient.PresignTemporaryReadURL(ctx, file.ObjectKey)
		if err != nil {
			return nil, newAppRequestFailure("internal_error", "生成临时文件访问地址失败")
		}
		urls = append(urls, temporaryFileReadURLResponse{
			ExpiresAt: expiresAt,
			FileID:    file.ID,
			URL:       url,
		})
	}

	return urls, nil
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

func normalizeAppSendAsUserRequest(req appSendAsUserRequest) (appSendAsUserRequest, error) {
	req.ActorUserID = strings.TrimSpace(req.ActorUserID)
	req.Target.Type = strings.TrimSpace(req.Target.Type)
	req.Target.UserID = strings.TrimSpace(firstNonEmptyAppString(req.Target.UserID, req.TargetUserID))
	req.Target.ConversationID = strings.TrimSpace(req.Target.ConversationID)
	req.TargetUserID = strings.TrimSpace(req.TargetUserID)
	req.TriggerMessageID = strings.TrimSpace(req.TriggerMessageID)
	if _, err := uuid.Parse(req.ActorUserID); err != nil {
		return appSendAsUserRequest{}, newAppRequestFailure("invalid_request", "发送人用户 ID 格式错误")
	}
	if _, err := uuid.Parse(req.TriggerMessageID); err != nil {
		return appSendAsUserRequest{}, newAppRequestFailure("invalid_request", "触发消息 ID 格式错误")
	}

	if req.Target.Type == "" && req.Target.UserID != "" {
		req.Target.Type = appMessageTargetUser
	}

	switch req.Target.Type {
	case appMessageTargetUser:
		if _, err := uuid.Parse(req.Target.UserID); err != nil {
			return appSendAsUserRequest{}, newAppRequestFailure("invalid_request", "目标用户 ID 格式错误")
		}
		if req.ActorUserID == req.Target.UserID {
			return appSendAsUserRequest{}, newAppRequestFailure("invalid_request", "不能给自己发送代发消息")
		}
		req.TargetUserID = req.Target.UserID
	case appMessageTargetGroup:
		if _, err := uuid.Parse(req.Target.ConversationID); err != nil {
			return appSendAsUserRequest{}, newAppRequestFailure("invalid_request", "会话 ID 格式错误")
		}
	default:
		return appSendAsUserRequest{}, newAppRequestFailure("invalid_request", "消息目标类型不支持")
	}

	return req, nil
}

func normalizeAppActorTrigger(rawActorUserID string, rawTriggerMessageID string) (string, string, error) {
	actorUserID := strings.TrimSpace(rawActorUserID)
	triggerMessageID := strings.TrimSpace(rawTriggerMessageID)
	if _, err := uuid.Parse(actorUserID); err != nil {
		return "", "", newAppRequestFailure("invalid_request", "发送人用户 ID 格式错误")
	}
	if _, err := uuid.Parse(triggerMessageID); err != nil {
		return "", "", newAppRequestFailure("invalid_request", "触发消息 ID 格式错误")
	}

	return actorUserID, triggerMessageID, nil
}

func (s *Server) findActiveAppActor(userID string) (store.User, error) {
	var user store.User
	err := s.db.First(&user, "id = ? AND status = ?", userID, store.UserStatusActive).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return store.User{}, newAppRequestFailure("not_found", "发送人用户不存在")
	}
	if err != nil {
		return store.User{}, err
	}

	return user, nil
}

func (s *Server) requireAppSendAsUserTrigger(appID string, actorUserID string, triggerMessageID string) error {
	var trigger store.Message
	err := s.db.First(
		&trigger,
		"id = ? AND sender_type = ? AND sender_id = ? AND deleted_at IS NULL",
		triggerMessageID,
		store.MessageSenderTypeUser,
		actorUserID,
	).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return newAppRequestFailure("forbidden", "触发消息无效")
	}
	if err != nil {
		return err
	}

	_, err = s.requireReadableAppConversationMember(appID, trigger.ConversationID)
	return err
}

func (s *Server) findAppSendAsUserUsers(actorUserID string, targetUserID string) (store.User, store.User, error) {
	var users []store.User
	if err := s.db.Find(&users, "id IN ? AND status = ?", []string{actorUserID, targetUserID}, store.UserStatusActive).Error; err != nil {
		return store.User{}, store.User{}, err
	}

	var actor store.User
	var target store.User
	for _, user := range users {
		switch user.ID {
		case actorUserID:
			actor = user
		case targetUserID:
			target = user
		}
	}
	if actor.ID == "" {
		return store.User{}, store.User{}, newAppRequestFailure("not_found", "发送人用户不存在")
	}
	if target.ID == "" {
		return store.User{}, store.User{}, newAppRequestFailure("not_found", "目标用户不存在")
	}

	return actor, target, nil
}

func (s *Server) findAppSendAsUserConversation(req appSendAsUserRequest) (store.User, store.Conversation, error) {
	switch req.Target.Type {
	case appMessageTargetUser:
		actor, target, err := s.findAppSendAsUserUsers(req.ActorUserID, req.Target.UserID)
		if err != nil {
			return store.User{}, store.Conversation{}, err
		}
		conversation, _, err := s.getOrCreateDirectConversation(actor, target)
		return actor, conversation, err
	case appMessageTargetGroup:
		actor, err := s.findActiveAppActor(req.ActorUserID)
		if err != nil {
			return store.User{}, store.Conversation{}, err
		}
		conversation, err := s.findAppSendAsUserGroupConversation(actor.ID, req.Target.ConversationID)
		return actor, conversation, err
	default:
		return store.User{}, store.Conversation{}, newAppRequestFailure("invalid_request", "消息目标类型不支持")
	}
}

func (s *Server) findAppSendAsUserGroupConversation(actorUserID string, conversationID string) (store.Conversation, error) {
	var conversation store.Conversation
	err := s.db.First(&conversation, "id = ?", conversationID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return store.Conversation{}, newAppRequestFailure("not_found", "群聊不存在")
	}
	if err != nil {
		return store.Conversation{}, err
	}
	if conversation.Kind != store.ConversationKindGroup {
		return store.Conversation{}, newAppRequestFailure("invalid_request", "会话类型不匹配")
	}
	if conversation.Status != store.ConversationStatusActive ||
		conversation.PostingPolicy != store.ConversationPostingPolicyOpen {
		return store.Conversation{}, newAppRequestFailure("forbidden", "当前会话不能发送消息")
	}

	var member store.ConversationMember
	err = s.db.First(
		&member,
		"conversation_id = ? AND member_type = ? AND member_id = ? AND left_at IS NULL",
		conversationID,
		store.ConversationMemberTypeUser,
		actorUserID,
	).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return store.Conversation{}, newAppRequestFailure("forbidden", "无权访问群聊")
	}
	if err != nil {
		return store.Conversation{}, err
	}

	return conversation, nil
}

func (s *Server) loadAppUserGroupConversations(actorUserID string, keyword string, limit int) ([]store.Conversation, error) {
	query := s.db.Model(&store.Conversation{}).
		Joins("JOIN conversation_members cm ON cm.conversation_id = conversations.id").
		Where("cm.member_type = ? AND cm.member_id = ? AND cm.left_at IS NULL", store.ConversationMemberTypeUser, actorUserID).
		Where("conversations.kind = ? AND conversations.status = ?", store.ConversationKindGroup, store.ConversationStatusActive)
	if keyword != "" {
		query = query.Where("LOWER(conversations.name) LIKE ?", "%"+keyword+"%")
	}

	var conversations []store.Conversation
	if err := query.
		Order("COALESCE(conversations.last_message_at, conversations.created_at) DESC").
		Order("conversations.id ASC").
		Limit(limit).
		Find(&conversations).Error; err != nil {
		return nil, err
	}

	return conversations, nil
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
		summary := message.Summary
		if message.RevokedAt != nil {
			summary = revokedMessageSummary()
		}
		payloads = append(payloads, appConversationHistoryMessagePayload{
			CreatedAt: message.CreatedAt,
			ID:        message.ID,
			Sender:    newAppHistoryMessageSenderPayload(message, usersByID, appsByID),
			Seq:       message.Seq,
			Summary:   summary,
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
			sender.Email = user.Email
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

func newAppSystemMessagePayload(message store.Message) appMessagePayload {
	return appMessagePayload{
		Body:      message.Body,
		CreatedAt: message.CreatedAt,
		ID:        message.ID,
		Seq:       message.Seq,
		Summary:   message.Summary,
	}
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

type preparedAppSendMessageBody struct {
	Body     json.RawMessage
	Finalize finalizeMessageBodyFunc
}

type appSendMessageBodyEnvelope struct {
	Content string `json:"content"`
	Name    string `json:"name"`
	Type    string `json:"type"`
	URL     string `json:"url"`
}

func (s *Server) prepareAppSendMessageBody(ctx context.Context, raw json.RawMessage) (preparedAppSendMessageBody, error) {
	var envelope appSendMessageBodyEnvelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return preparedAppSendMessageBody{}, newAppRequestFailure("invalid_request", "消息体格式错误")
	}
	switch strings.TrimSpace(envelope.Type) {
	case messageTypeText, messageTypeMarkdown, messageTypeLink:
		body, err := normalizeAppSendMessageBody(ctx, raw)
		if err != nil {
			return preparedAppSendMessageBody{}, err
		}
		return preparedAppSendMessageBody{
			Body:     body,
			Finalize: finalizeNormalizedMessageBody,
		}, nil
	case messageTypeImage:
		body, err := s.createRemoteImageMessageBody(ctx, firstNonEmptyAppString(envelope.Content, envelope.URL))
		if err != nil {
			return preparedAppSendMessageBody{}, err
		}
		return preparedAppSendMessageBody{
			Body:     body,
			Finalize: staticMessageBodyFinalizer(imageMessageSummary()),
		}, nil
	case messageTypeFile:
		body, name, err := s.prepareAppSendFileMessageBody(ctx, envelope)
		if err != nil {
			return preparedAppSendMessageBody{}, err
		}
		return preparedAppSendMessageBody{
			Body:     body,
			Finalize: staticMessageBodyFinalizer(fileMessageSummary(name)),
		}, nil
	default:
		return preparedAppSendMessageBody{}, newAppRequestFailure("invalid_request", "不支持的消息类型")
	}
}

func (s *Server) prepareAppSendFileMessageBody(ctx context.Context, envelope appSendMessageBodyEnvelope) (json.RawMessage, string, error) {
	url := strings.TrimSpace(envelope.URL)
	hasURL := url != ""
	hasContent := envelope.Content != ""
	switch {
	case hasURL && hasContent:
		return nil, "", newAppRequestFailure("invalid_request", "文件 URL 和内容只能二选一")
	case hasURL:
		return s.createRemoteFileMessageBody(ctx, url, envelope.Name)
	case hasContent:
		return s.createInlineFileMessageBody(ctx, envelope.Content, envelope.Name)
	default:
		return nil, "", newAppRequestFailure("invalid_request", "文件 URL 或内容不能为空")
	}
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

		existing, ok, err := findExistingMessageByClientMessageID(
			tx,
			conversationID,
			store.MessageSenderTypeApp,
			appID,
			clientMessageID,
		)
		if err != nil {
			return err
		}
		if ok {
			message = existing
			created = false
			return nil
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

func mapAppGroupConversationError(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return newAppRequestFailure("not_found", "会话不存在")
	}
	if errors.Is(err, errConversationAccessDenied) {
		return newAppRequestFailure("forbidden", "无权访问会话")
	}
	if errors.Is(err, errConversationNotGroup) {
		return newAppRequestFailure("invalid_request", "只能向群聊添加成员")
	}
	if errors.Is(err, errGroupConversationMemberCap) {
		return newAppRequestFailure("invalid_request", "群聊成员不能超过 100 人")
	}
	if errors.Is(err, errGroupConversationMemberMiss) {
		return newAppRequestFailure("invalid_request", "成员不存在或已禁用")
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
