package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	contactapp "app/internal/application/contact"
	conversationapp "app/internal/application/conversation"
	fileapp "app/internal/application/file"
	messageapp "app/internal/application/message"
	"app/internal/appregistry"
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
	appMethodContactsAppsList          = "contacts.apps.list"
	appMethodContactsGroupsList        = "contacts.groups.list"
	appMethodConversationsList         = "conversations.list"
	appMethodConversationMessagesList  = "conversation.messages.list"
	appMethodConversationHistoryRead   = "conversation.history.read"
	appMethodGroupConversationsList    = "group_conversations.list"
	appMethodGroupConversationsCreate  = "group_conversations.create"
	appMethodGroupMembersAdd           = "group_conversations.members.add"
	appMethodProjectsList              = "projects.list"
	appMethodProjectsCreate            = "projects.create"
	appMethodProjectGroupsGrant        = "projects.groups.grant"
	appMethodProjectTasksList          = "projects.tasks.list"
	appMethodProjectTasksCreate        = "projects.tasks.create"
	appMethodProjectTasksUpdate        = "projects.tasks.update"
	appMethodTemporaryFilesReadURLs    = "temporary_files.read_urls"
	appMethodEventsAck                 = "events.ack"
	defaultAppConversationHistoryLimit = 30
	maxAppConversationHistoryLimit     = 100
	defaultAppScopedHistoryReadLimit   = 20
	defaultAppConversationListLimit    = 20
	defaultAppGroupConversationLimit   = 30

	appMessageTargetUser  = "user"
	appMessageTargetGroup = "group"
	appMessageTargetApp   = "app"
)

type appSendMessageRequest struct {
	ActorUserID                 string               `json:"actor_user_id"`
	AuthorizationConversationID string               `json:"authorization_conversation_id"`
	Message                     json.RawMessage      `json:"message"`
	Target                      appSendMessageTarget `json:"target"`
	TriggerMessageID            string               `json:"trigger_message_id"`
	UserID                      string               `json:"user_id"`
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
	Message      *appMessagePayload        `json:"message"`
}

type appAddGroupConversationMembersResponse struct {
	Conversation conversationListItemResponse `json:"conversation"`
	Message      *appMessagePayload           `json:"message"`
}

type appSendAsUserRequest struct {
	ActorUserID                 string              `json:"actor_user_id"`
	AuthorizationConversationID string              `json:"authorization_conversation_id"`
	Message                     json.RawMessage     `json:"message"`
	Target                      appSendAsUserTarget `json:"target"`
	TargetUserID                string              `json:"target_user_id"`
	TriggerMessageID            string              `json:"trigger_message_id"`
}

type appSendAsUserTarget struct {
	ConversationID string `json:"conversation_id"`
	Type           string `json:"type"`
	UserID         string `json:"user_id"`
}

type appCreateGroupConversationRequest struct {
	ActorUserID                 string   `json:"actor_user_id"`
	AuthorizationConversationID string   `json:"authorization_conversation_id"`
	MemberIDs                   []string `json:"member_ids"`
	Name                        string   `json:"name"`
	TriggerMessageID            string   `json:"trigger_message_id"`
}

type appAddGroupConversationMembersRequest struct {
	ActorUserID                 string   `json:"actor_user_id"`
	AuthorizationConversationID string   `json:"authorization_conversation_id"`
	ConversationID              string   `json:"conversation_id"`
	MemberIDs                   []string `json:"member_ids"`
	TriggerMessageID            string   `json:"trigger_message_id"`
}

type appListContactUsersRequest struct {
	Keyword string    `json:"keyword"`
	RunAs   *appRunAs `json:"runas"`
}

type appListContactUsersResponse struct {
	Contacts []contactUserResponse `json:"contacts"`
	RunAs    appRunAsIdentity      `json:"runas"`
}

type appListContactAppsResponse struct {
	Apps  []contactAppResponse `json:"apps"`
	RunAs appRunAsIdentity     `json:"runas"`
}

type appListContactGroupsResponse struct {
	Groups []contactGroupResponse `json:"groups"`
	RunAs  appRunAsIdentity       `json:"runas"`
}

type appRunAs struct {
	AuthorizationConversationID string `json:"authorization_conversation_id"`
	ID                          string `json:"id"`
	TriggerMessageID            string `json:"trigger_message_id"`
	Type                        string `json:"type"`
}

type appRunAsIdentity struct {
	ID   string `json:"id"`
	Type string `json:"type"`
}

type appListConversationsRequest struct {
	ActorUserID                 string `json:"actor_user_id"`
	AuthorizationConversationID string `json:"authorization_conversation_id"`
	Keyword                     string `json:"keyword"`
	Limit                       int    `json:"limit"`
	TriggerMessageID            string `json:"trigger_message_id"`
}

type appListConversationsResponse struct {
	Conversations []appConversationSummaryPayload `json:"conversations"`
	Limit         int                             `json:"limit"`
}

type appListConversationMessagesRequest struct {
	BeforeOrEqualSeq int64     `json:"before_or_equal_seq"`
	ConversationID   string    `json:"conversation_id"`
	Limit            int       `json:"limit"`
	RunAs            *appRunAs `json:"runas"`
}

type appListConversationMessagesResponse struct {
	Limit          int                                    `json:"limit"`
	Messages       []appConversationHistoryMessagePayload `json:"messages"`
	ProjectContext *appConversationProjectContext         `json:"project_context,omitempty"`
}

type appConversationProjectContext struct {
	ConversationProjects []appConversationProjectContextProject `json:"conversation_projects"`
	PersonalProject      *appConversationProjectContextProject  `json:"personal_project"`
}

type appConversationProjectContextProject struct {
	Description string `json:"description"`
	ID          string `json:"id"`
	Name        string `json:"name"`
}

type appReadConversationHistoryRequest struct {
	ActorUserID                 string `json:"actor_user_id"`
	AppID                       string `json:"app_id"`
	AuthorizationConversationID string `json:"authorization_conversation_id"`
	BeforeSeq                   int64  `json:"before_seq"`
	ConversationID              string `json:"conversation_id"`
	Limit                       int    `json:"limit"`
	TriggerMessageID            string `json:"trigger_message_id"`
	UserID                      string `json:"user_id"`
}

type appReadConversationHistoryResponse struct {
	Conversation appConversationSummaryPayload          `json:"conversation"`
	Limit        int                                    `json:"limit"`
	Messages     []appConversationHistoryMessagePayload `json:"messages"`
}

type appListGroupConversationsRequest struct {
	ActorUserID                 string `json:"actor_user_id"`
	AuthorizationConversationID string `json:"authorization_conversation_id"`
	Keyword                     string `json:"keyword"`
	Limit                       int    `json:"limit"`
	TriggerMessageID            string `json:"trigger_message_id"`
}

type appListGroupConversationsResponse struct {
	Groups []conversationListItemResponse `json:"groups"`
	Limit  int                            `json:"limit"`
}

type appReadTemporaryFileURLsRequest struct {
	FileIDs []string `json:"file_ids"`
}

type appAckEventsRequest struct {
	Cursor int64 `json:"cursor"`
}

type appAckEventsResponse struct {
	Cursor int64 `json:"cursor"`
}

type appConversationHistoryMessagePayload struct {
	Body      json.RawMessage         `json:"body,omitempty"`
	CreatedAt time.Time               `json:"created_at"`
	ID        string                  `json:"id"`
	Sender    appMessageSenderPayload `json:"sender"`
	Seq       int64                   `json:"seq"`
	Summary   string                  `json:"summary"`
}

type appConversationSummaryPayload struct {
	ConversationID string    `json:"conversation_id"`
	LastActiveAt   time.Time `json:"last_active_at"`
	MemberCount    int       `json:"member_count"`
	Name           string    `json:"name"`
	Type           string    `json:"type"`
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
		response, err := s.handleAppListContactUsers(appID, request)
		if err != nil {
			return appRequestErrorResponse(request.ID, err)
		}
		return realtime.NewResponse(request.ID, response)
	case appMethodContactsAppsList:
		response, err := s.handleAppListContactApps(appID, request)
		if err != nil {
			return appRequestErrorResponse(request.ID, err)
		}
		return realtime.NewResponse(request.ID, response)
	case appMethodContactsGroupsList:
		response, err := s.handleAppListContactGroups(appID, request)
		if err != nil {
			return appRequestErrorResponse(request.ID, err)
		}
		return realtime.NewResponse(request.ID, response)
	case appMethodConversationsList:
		response, err := s.handleAppListConversations(appID, request)
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
	case appMethodConversationHistoryRead:
		response, err := s.handleAppReadConversationHistory(appID, request)
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
	case appMethodProjectsList:
		response, err := s.handleAppListProjects(appID, request)
		if err != nil {
			return appRequestErrorResponse(request.ID, err)
		}
		return realtime.NewResponse(request.ID, response)
	case appMethodProjectsCreate:
		response, err := s.handleAppCreateProject(appID, request)
		if err != nil {
			return appRequestErrorResponse(request.ID, err)
		}
		return realtime.NewResponse(request.ID, response)
	case appMethodProjectGroupsGrant:
		response, err := s.handleAppGrantProjectGroup(appID, request)
		if err != nil {
			return appRequestErrorResponse(request.ID, err)
		}
		return realtime.NewResponse(request.ID, response)
	case appMethodProjectTasksList:
		response, err := s.handleAppListProjectTasks(appID, request)
		if err != nil {
			return appRequestErrorResponse(request.ID, err)
		}
		return realtime.NewResponse(request.ID, response)
	case appMethodProjectTasksCreate:
		response, err := s.handleAppCreateProjectTask(appID, request)
		if err != nil {
			return appRequestErrorResponse(request.ID, err)
		}
		return realtime.NewResponse(request.ID, response)
	case appMethodProjectTasksUpdate:
		response, err := s.handleAppUpdateProjectTask(appID, request)
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
	case appMethodEventsAck:
		response, err := s.handleAppAckEvents(appID, request)
		if err != nil {
			return appRequestErrorResponse(request.ID, err)
		}
		return realtime.NewResponse(request.ID, response)
	default:
		return realtime.NewErrorResponse(request.ID, "unknown_method", "未知应用方法")
	}
}

func (s *Server) handleAppAckEvents(appID string, request realtime.Envelope) (appAckEventsResponse, error) {
	var req appAckEventsRequest
	if err := json.Unmarshal(request.Payload, &req); err != nil || req.Cursor <= 0 {
		return appAckEventsResponse{}, newAppRequestFailure("invalid_request", "事件游标格式错误")
	}

	err := s.db.Transaction(func(tx *gorm.DB) error {
		var currentAck store.AppEventAck
		err := tx.First(&currentAck, "app_id = ?", appID).Error
		if err == nil && req.Cursor <= currentAck.LastAckedCursor {
			return nil
		}
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		var eventCount int64
		if err := tx.Model(&store.AppEventOutbox{}).
			Where("app_id = ? AND id = ?", appID, req.Cursor).
			Count(&eventCount).Error; err != nil {
			return err
		}
		if eventCount == 0 {
			return newAppRequestFailure("invalid_request", "事件游标不存在")
		}

		now := time.Now().UTC()
		result := tx.Model(&store.AppEventAck{}).
			Where("app_id = ? AND last_acked_cursor < ?", appID, req.Cursor).
			Updates(map[string]any{"last_acked_cursor": req.Cursor, "updated_at": now})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			ack := store.AppEventAck{AppID: appID, LastAckedCursor: req.Cursor, UpdatedAt: now}
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&ack).Error; err != nil {
				return err
			}
			if err := tx.Model(&store.AppEventAck{}).
				Where("app_id = ? AND last_acked_cursor < ?", appID, req.Cursor).
				Updates(map[string]any{"last_acked_cursor": req.Cursor, "updated_at": now}).Error; err != nil {
				return err
			}
		}
		return tx.Where("app_id = ? AND id <= ?", appID, req.Cursor).Delete(&store.AppEventOutbox{}).Error
	})
	if err != nil {
		return appAckEventsResponse{}, err
	}
	return appAckEventsResponse{Cursor: req.Cursor}, nil
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
	entityCardUserID := ""
	if isEntityCardMessageBody(req.Message) {
		actorUserID, triggerMessageID, err := normalizeAppActorTrigger(req.ActorUserID, req.TriggerMessageID)
		if err != nil {
			return appSendMessageResponse{}, err
		}
		if err := s.requireAppSendAsUserTrigger(appID, actorUserID, triggerMessageID, req.AuthorizationConversationID); err != nil {
			return appSendMessageResponse{}, err
		}
		if strings.TrimSpace(req.AuthorizationConversationID) != conversation.ID {
			return appSendMessageResponse{}, newAppRequestFailure("forbidden", "对象卡片只能回复到授权会话")
		}
		if _, err := s.findActiveAppActor(actorUserID); err != nil {
			return appSendMessageResponse{}, err
		}
		entityCardUserID = actorUserID
	}
	prepared, err := s.prepareAppSendMessageBodyForUser(context.Background(), entityCardUserID, req.Message)
	if err != nil {
		return appSendMessageResponse{}, err
	}

	createdMessage, err := s.messages.CreateAsApp(context.Background(), messageapp.CreateAsAppCommand{
		AppID: appID, Body: prepared.Body, ClientMessageID: request.ID, ConversationID: conversation.ID,
		Finalize: messageapp.FinalizeBody(prepared.Finalize),
	})
	if err != nil {
		return appSendMessageResponse{}, mapMessageApplicationErrorForApp(err)
	}
	message := createdMessage.Message

	return appSendMessageResponse{
		Conversation: appMessageConversationPayload{
			ID:   conversation.ID,
			Name: conversation.Name,
			Type: conversation.Kind,
		},
		Created: createdMessage.Created,
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
	if err := s.requireAppSendAsUserTrigger(appID, req.ActorUserID, req.TriggerMessageID, req.AuthorizationConversationID); err != nil {
		return appSendMessageResponse{}, err
	}

	actor, conversation, err := s.findAppSendAsUserConversation(req)
	if err != nil {
		return appSendMessageResponse{}, err
	}
	prepared, err := s.prepareAppSendMessageBodyForUser(context.Background(), actor.ID, req.Message)
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
	createdMessage, err := s.messages.CreateDelegated(context.Background(), messageapp.CreateDelegatedCommand{
		AccountID: actor.ID, Body: prepared.Body, ClientMessageID: request.ID, ConversationID: conversation.ID,
		DelegatedBy: messageapp.Identity{ID: delegatedByID, Name: app.Name, Type: delegatedByType},
		Finalize:    messageapp.FinalizeBody(prepared.Finalize),
	})
	if err != nil {
		return appSendMessageResponse{}, mapMessageApplicationErrorForApp(err)
	}
	message := createdMessage.Message

	return appSendMessageResponse{
		Conversation: appMessageConversationPayload{
			ID:   conversation.ID,
			Name: conversation.Name,
			Type: conversation.Kind,
		},
		Created: createdMessage.Created,
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

func (s *Server) handleAppListConversations(appID string, request realtime.Envelope) (appListConversationsResponse, error) {
	var req appListConversationsRequest
	if len(request.Payload) > 0 {
		if err := json.Unmarshal(request.Payload, &req); err != nil {
			return appListConversationsResponse{}, newAppRequestFailure("invalid_request", "请求格式错误")
		}
	}

	actorUserID, triggerMessageID, err := normalizeAppActorTrigger(req.ActorUserID, req.TriggerMessageID)
	if err != nil {
		return appListConversationsResponse{}, err
	}
	if err := s.requireAppSendAsUserTrigger(appID, actorUserID, triggerMessageID, req.AuthorizationConversationID); err != nil {
		return appListConversationsResponse{}, err
	}
	actor, err := s.findActiveAppActor(actorUserID)
	if err != nil {
		return appListConversationsResponse{}, err
	}

	limit := normalizeAppScopedLimit(req.Limit, defaultAppConversationListLimit)
	result, err := s.conversations.ListForActor(context.Background(), conversationapp.AppListCommand{
		ActorID: actor.ID,
		Keyword: strings.ToLower(strings.TrimSpace(req.Keyword)),
		Limit:   limit,
	})
	if err != nil {
		return appListConversationsResponse{}, err
	}
	payloads := make([]appConversationSummaryPayload, 0, len(result.Conversations))
	for _, conversation := range result.Conversations {
		payloads = append(payloads, appConversationSummaryPayload{
			ConversationID: conversation.ConversationID,
			LastActiveAt:   conversation.LastActiveAt,
			MemberCount:    conversation.MemberCount,
			Name:           conversation.Name,
			Type:           conversation.Type,
		})
	}

	return appListConversationsResponse{
		Conversations: payloads,
		Limit:         limit,
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
	if err := s.requireAppSendAsUserTrigger(appID, actorUserID, triggerMessageID, req.AuthorizationConversationID); err != nil {
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
	result, err := s.conversations.ListGroupsForActor(context.Background(), conversationapp.AppListCommand{
		ActorID: actor.ID,
		Keyword: strings.ToLower(strings.TrimSpace(req.Keyword)),
		Limit:   limit,
	})
	if err != nil {
		return appListGroupConversationsResponse{}, err
	}
	groups := make([]conversationListItemResponse, 0, len(result.Groups))
	for _, conversation := range result.Groups {
		groups = append(groups, legacyConversationItem(conversation))
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

	if err := s.requireAppSendAsUserTrigger(appID, actorUserID, triggerMessageID, req.AuthorizationConversationID); err != nil {
		return appCreateGroupConversationResponse{}, err
	}
	actor, err := s.findActiveAppActor(actorUserID)
	if err != nil {
		return appCreateGroupConversationResponse{}, err
	}

	result, err := s.conversations.CreateGroup(context.Background(), conversationapp.CreateGroupCommand{
		Actor:     conversationActorFromUser(actor),
		Name:      name,
		MemberIDs: memberIDs,
	})
	if err != nil {
		return appCreateGroupConversationResponse{}, mapAppGroupConversationError(err)
	}
	var messageResponse *appMessagePayload
	if result.Message != nil {
		response := newAppSystemMessagePayload(*result.Message)
		messageResponse = &response
	}

	return appCreateGroupConversationResponse{
		Conversation: legacyGroupConversation(result.Conversation),
		Message:      messageResponse,
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

	if err := s.requireAppSendAsUserTrigger(appID, actorUserID, triggerMessageID, req.AuthorizationConversationID); err != nil {
		return appAddGroupConversationMembersResponse{}, err
	}
	actor, err := s.findActiveAppActor(actorUserID)
	if err != nil {
		return appAddGroupConversationMembersResponse{}, err
	}

	result, err := s.conversations.AddMembers(context.Background(), conversationapp.AddMembersCommand{
		Actor:          conversationActorFromUser(actor),
		ConversationID: conversationID,
		MemberIDs:      memberIDs,
	})
	if err != nil {
		return appAddGroupConversationMembersResponse{}, mapAppGroupConversationError(err)
	}

	var messageResponse *appMessagePayload
	if result.Message != nil {
		response := newAppSystemMessagePayload(*result.Message)
		messageResponse = &response
	}

	return appAddGroupConversationMembersResponse{
		Conversation: legacyConversationItem(result.Conversation),
		Message:      messageResponse,
	}, nil
}

func (s *Server) handleAppListContactUsers(appID string, request realtime.Envelope) (appListContactUsersResponse, error) {
	var req appListContactUsersRequest
	if len(request.Payload) > 0 {
		if err := json.Unmarshal(request.Payload, &req); err != nil {
			return appListContactUsersResponse{}, newAppRequestFailure("invalid_request", "请求格式错误")
		}
	}
	runAs, err := s.resolveAppRunAs(appID, req.RunAs)
	if err != nil {
		return appListContactUsersResponse{}, err
	}

	result, err := s.contacts.ListUsers(context.Background(), contactapp.ListUsersCommand{Keyword: req.Keyword})
	if err != nil {
		return appListContactUsersResponse{}, err
	}

	return appListContactUsersResponse{Contacts: legacyContactUsers(result.Users), RunAs: runAs.identity()}, nil
}

func (s *Server) handleAppListContactApps(appID string, request realtime.Envelope) (appListContactAppsResponse, error) {
	var req appListContactUsersRequest
	if len(request.Payload) > 0 {
		if err := json.Unmarshal(request.Payload, &req); err != nil {
			return appListContactAppsResponse{}, newAppRequestFailure("invalid_request", "请求格式错误")
		}
	}
	runAs, err := s.resolveAppRunAs(appID, req.RunAs)
	if err != nil {
		return appListContactAppsResponse{}, err
	}

	result, err := s.contacts.ListAppsForIdentity(context.Background(), contactapp.ListForIdentityCommand{
		Identity: contactapp.Identity{Type: runAs.Type, ID: runAs.ID},
		Keyword:  req.Keyword,
	})
	if err != nil {
		return appListContactAppsResponse{}, err
	}

	return appListContactAppsResponse{Apps: legacyContactApps(result.Apps), RunAs: runAs.identity()}, nil
}

func (s *Server) handleAppListContactGroups(appID string, request realtime.Envelope) (appListContactGroupsResponse, error) {
	var req appListContactUsersRequest
	if len(request.Payload) > 0 {
		if err := json.Unmarshal(request.Payload, &req); err != nil {
			return appListContactGroupsResponse{}, newAppRequestFailure("invalid_request", "请求格式错误")
		}
	}
	runAs, err := s.resolveAppRunAs(appID, req.RunAs)
	if err != nil {
		return appListContactGroupsResponse{}, err
	}

	result, err := s.contacts.ListGroupsForIdentity(context.Background(), contactapp.ListForIdentityCommand{
		Identity: contactapp.Identity{Type: runAs.Type, ID: runAs.ID},
		Keyword:  req.Keyword,
	})
	if err != nil {
		return appListContactGroupsResponse{}, err
	}

	return appListContactGroupsResponse{Groups: legacyContactGroups(result.Groups), RunAs: runAs.identity()}, nil
}

func (r appRunAs) identity() appRunAsIdentity {
	return appRunAsIdentity{Type: r.Type, ID: r.ID}
}

func (s *Server) resolveAppRunAs(appID string, requested *appRunAs) (appRunAs, error) {
	if requested == nil {
		return appRunAs{}, newAppRequestFailure("invalid_request", "runas 用户身份不能为空")
	}
	if !appregistry.IsAIAssistantAppID(appID) {
		return appRunAs{}, newAppRequestFailure("forbidden", "当前应用无权指定 runas 身份")
	}

	runAs := appRunAs{
		AuthorizationConversationID: strings.TrimSpace(requested.AuthorizationConversationID),
		ID:                          strings.TrimSpace(requested.ID),
		TriggerMessageID:            strings.TrimSpace(requested.TriggerMessageID),
		Type:                        strings.ToLower(strings.TrimSpace(requested.Type)),
	}
	if runAs.Type != store.MessageSenderTypeUser {
		return appRunAs{}, newAppRequestFailure("invalid_request", "runas.type 必须是 user")
	}
	if _, err := uuid.Parse(runAs.ID); err != nil {
		return appRunAs{}, newAppRequestFailure("invalid_request", "runas 身份 ID 格式错误")
	}
	if runAs.AuthorizationConversationID == "" {
		return appRunAs{}, newAppRequestFailure("invalid_request", "runas 授权会话 ID 不能为空")
	}
	if err := s.requireAppRunAsTrigger(
		appID,
		runAs.Type,
		runAs.ID,
		runAs.TriggerMessageID,
		runAs.AuthorizationConversationID,
	); err != nil {
		return appRunAs{}, err
	}
	var user store.User
	err := s.db.First(&user, "id = ? AND status = ?", runAs.ID, store.UserStatusActive).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return appRunAs{}, newAppRequestFailure("not_found", "runas 用户不存在或不可用")
	}
	if err != nil {
		return appRunAs{}, err
	}
	return appRunAs{Type: store.MessageSenderTypeUser, ID: user.ID}, nil
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

	access, err := s.messages.AuthorizeAppConversation(context.Background(), messageapp.AppConversationAccessCommand{
		AppID: appID, ConversationID: req.ConversationID,
	})
	if err != nil {
		return appListConversationMessagesResponse{}, mapMessageApplicationErrorForApp(err)
	}
	var projectContext *appConversationProjectContext
	if req.RunAs != nil {
		if strings.TrimSpace(req.RunAs.AuthorizationConversationID) != req.ConversationID {
			return appListConversationMessagesResponse{}, newAppRequestFailure("forbidden", "项目上下文授权会话不匹配")
		}
		runAs, err := s.resolveAppRunAs(appID, req.RunAs)
		if err != nil {
			return appListConversationMessagesResponse{}, err
		}
		loaded, err := s.loadAppConversationProjectContext(req.ConversationID, runAs.ID)
		if err != nil {
			return appListConversationMessagesResponse{}, err
		}
		projectContext = &loaded
	}
	listed, err := s.messages.ListForApp(context.Background(), messageapp.ListForAppCommand{
		BeforeOrEqualSeq: req.BeforeOrEqualSeq, ConversationID: req.ConversationID,
		HistoryVisibleFromSeq: access.HistoryVisibleFromSeq, Limit: req.Limit,
	})
	if err != nil {
		return appListConversationMessagesResponse{}, mapMessageApplicationErrorForApp(err)
	}

	return appListConversationMessagesResponse{
		Limit:          req.Limit,
		Messages:       legacyAppHistoryMessages(listed.Messages),
		ProjectContext: projectContext,
	}, nil
}

func (s *Server) loadAppConversationProjectContext(conversationID string, userID string) (appConversationProjectContext, error) {
	result := appConversationProjectContext{
		ConversationProjects: []appConversationProjectContextProject{},
	}

	var personal store.Project
	err := s.db.
		Select("id", "name", "description").
		Where("owner_user_id = ? AND is_personal = ?", userID, true).
		First(&personal).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return appConversationProjectContext{}, err
	}
	if err == nil {
		result.PersonalProject = &appConversationProjectContextProject{
			Description: personal.Description,
			ID:          personal.ID,
			Name:        personal.Name,
		}
	}

	var projects []store.Project
	if err := s.db.
		Model(&store.Project{}).
		Select("projects.id", "projects.name", "projects.description").
		Joins("JOIN project_groups pg ON pg.project_id = projects.id").
		Where("pg.conversation_id = ?", conversationID).
		Order("projects.updated_at DESC").
		Order("projects.id DESC").
		Find(&projects).Error; err != nil {
		return appConversationProjectContext{}, err
	}
	for _, project := range projects {
		result.ConversationProjects = append(result.ConversationProjects, appConversationProjectContextProject{
			Description: project.Description,
			ID:          project.ID,
			Name:        project.Name,
		})
	}

	return result, nil
}

func (s *Server) handleAppReadConversationHistory(appID string, request realtime.Envelope) (appReadConversationHistoryResponse, error) {
	var req appReadConversationHistoryRequest
	if err := json.Unmarshal(request.Payload, &req); err != nil {
		return appReadConversationHistoryResponse{}, newAppRequestFailure("invalid_request", "请求格式错误")
	}

	req, err := normalizeAppReadConversationHistoryRequest(req)
	if err != nil {
		return appReadConversationHistoryResponse{}, err
	}
	if err := s.requireAppSendAsUserTrigger(appID, req.ActorUserID, req.TriggerMessageID, req.AuthorizationConversationID); err != nil {
		return appReadConversationHistoryResponse{}, err
	}
	actor, err := s.findActiveAppActor(req.ActorUserID)
	if err != nil {
		return appReadConversationHistoryResponse{}, err
	}
	read, err := s.messages.ReadForUser(context.Background(), messageapp.ReadForUserCommand{
		AccountID: actor.ID, AppID: req.AppID, BeforeSeq: req.BeforeSeq,
		ConversationID: req.ConversationID, Limit: req.Limit, UserID: req.UserID,
	})
	if err != nil {
		return appReadConversationHistoryResponse{}, mapMessageApplicationErrorForApp(err)
	}

	return appReadConversationHistoryResponse{
		Conversation: legacyAppConversationSummary(read.Conversation),
		Limit:        req.Limit,
		Messages:     legacyAppHistoryMessages(read.Messages),
	}, nil
}

func legacyAppHistoryMessages(messages []messageapp.AppHistoryMessage) []appConversationHistoryMessagePayload {
	result := make([]appConversationHistoryMessagePayload, 0, len(messages))
	for _, message := range messages {
		result = append(result, appConversationHistoryMessagePayload{
			Body: message.Body, CreatedAt: message.CreatedAt, ID: message.ID,
			Sender: appMessageSenderPayload{
				Email: message.Sender.Email, ID: message.Sender.ID, Name: message.Sender.Name,
				Nickname: message.Sender.Nickname, Type: message.Sender.Type,
			},
			Seq: message.Seq, Summary: message.Summary,
		})
	}
	return result
}

func legacyAppConversationSummary(value messageapp.AppConversationSummary) appConversationSummaryPayload {
	return appConversationSummaryPayload{
		ConversationID: value.ConversationID, LastActiveAt: value.LastActiveAt,
		MemberCount: value.MemberCount, Name: value.Name, Type: value.Type,
	}
}

func (s *Server) handleAppReadTemporaryFileURLs(_ string, request realtime.Envelope) (readTemporaryFileURLsResponse, error) {
	var req appReadTemporaryFileURLsRequest
	if err := json.Unmarshal(request.Payload, &req); err != nil {
		return readTemporaryFileURLsResponse{}, newAppRequestFailure("invalid_request", "请求格式错误")
	}

	urls, err := s.presignTemporaryFileReadURLsForApp(context.Background(), req.FileIDs)
	if err != nil {
		return readTemporaryFileURLsResponse{}, err
	}

	return readTemporaryFileURLsResponse{URLs: urls}, nil
}

func (s *Server) presignTemporaryFileReadURLsForApp(ctx context.Context, fileIDs []string) ([]temporaryFileReadURLResponse, error) {
	values, err := s.files.ResolveTemporaryURLs(ctx, fileIDs)
	if err != nil {
		switch fileapp.ErrorCodeOf(err) {
		case fileapp.CodeInvalidRequest:
			return nil, newAppRequestFailure("invalid_request", fileapp.ErrorMessage(err))
		case fileapp.CodeNotFound:
			return nil, newAppRequestFailure("not_found", "临时文件不存在")
		case fileapp.CodeStorageUnavailable:
			return nil, newAppRequestFailure("internal_error", "临时文件存储未配置")
		default:
			return nil, newAppRequestFailure("internal_error", fileapp.ErrorMessage(err))
		}
	}
	return newTemporaryFileReadURLResponses(values), nil
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

func normalizeAppReadConversationHistoryRequest(req appReadConversationHistoryRequest) (appReadConversationHistoryRequest, error) {
	actorUserID, triggerMessageID, err := normalizeAppActorTrigger(req.ActorUserID, req.TriggerMessageID)
	if err != nil {
		return appReadConversationHistoryRequest{}, err
	}
	req.ActorUserID = actorUserID
	req.TriggerMessageID = triggerMessageID
	req.ConversationID = strings.TrimSpace(req.ConversationID)
	req.UserID = strings.TrimSpace(req.UserID)
	req.AppID = strings.TrimSpace(req.AppID)

	selectorCount := 0
	if req.ConversationID != "" {
		if _, err := uuid.Parse(req.ConversationID); err != nil {
			return appReadConversationHistoryRequest{}, newAppRequestFailure("invalid_request", "会话 ID 格式错误")
		}
		selectorCount++
	}
	if req.UserID != "" {
		if _, err := uuid.Parse(req.UserID); err != nil {
			return appReadConversationHistoryRequest{}, newAppRequestFailure("invalid_request", "用户 ID 格式错误")
		}
		selectorCount++
	}
	if req.AppID != "" {
		if _, err := uuid.Parse(req.AppID); err != nil {
			return appReadConversationHistoryRequest{}, newAppRequestFailure("invalid_request", "应用 ID 格式错误")
		}
		selectorCount++
	}
	if selectorCount != 1 {
		return appReadConversationHistoryRequest{}, newAppRequestFailure("invalid_request", "conversation_id、user_id、app_id 必须三选一")
	}
	if req.BeforeSeq < 0 {
		return appReadConversationHistoryRequest{}, newAppRequestFailure("invalid_request", "before_seq 必须是正整数")
	}
	req.Limit = normalizeAppScopedLimit(req.Limit, defaultAppScopedHistoryReadLimit)

	return req, nil
}

func normalizeAppScopedLimit(limit int, defaultLimit int) int {
	if limit <= 0 {
		limit = defaultLimit
	}
	if limit > maxAppConversationHistoryLimit {
		limit = maxAppConversationHistoryLimit
	}

	return limit
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

func (s *Server) requireAppSendAsUserTrigger(appID string, actorUserID string, triggerMessageID string, authorizationConversationID string) error {
	return s.requireAppRunAsTrigger(appID, store.MessageSenderTypeUser, actorUserID, triggerMessageID, authorizationConversationID)
}

func (s *Server) requireAppRunAsTrigger(appID string, actorType string, actorID string, triggerMessageID string, authorizationConversationID string) error {
	actorType = strings.ToLower(strings.TrimSpace(actorType))
	actorID = strings.TrimSpace(actorID)
	triggerMessageID = strings.TrimSpace(triggerMessageID)
	if actorType != store.MessageSenderTypeUser && actorType != store.MessageSenderTypeApp {
		return newAppRequestFailure("invalid_request", "runas.type 必须是 user 或 app")
	}
	if _, err := uuid.Parse(actorID); err != nil {
		return newAppRequestFailure("invalid_request", "runas 身份 ID 格式错误")
	}
	if _, err := uuid.Parse(triggerMessageID); err != nil {
		return newAppRequestFailure("invalid_request", "runas 授权触发消息 ID 格式错误")
	}
	authorizationConversationID = strings.TrimSpace(authorizationConversationID)
	if authorizationConversationID != "" {
		if _, err := uuid.Parse(authorizationConversationID); err != nil {
			return newAppRequestFailure("invalid_request", "授权会话 ID 格式错误")
		}
	}
	err := s.messages.AuthorizeRunAsTrigger(context.Background(), messageapp.RunAsTriggerCommand{
		ActorID: actorID, ActorType: actorType, AppID: appID,
		AuthorizationConversationID: authorizationConversationID, TriggerMessageID: triggerMessageID,
	})
	if err != nil {
		return mapMessageApplicationErrorForApp(err)
	}
	return nil
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
		opened, _, err := s.conversations.OpenDirectForUsers(
			context.Background(),
			conversationActorFromUser(actor),
			conversationActorFromUser(target),
		)
		return actor, store.Conversation{ID: opened.ID, Name: opened.Name, Kind: opened.Type}, err
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

func (s *Server) newAppConversationSummaryPayloads(conversations []store.Conversation, currentUserID string) ([]appConversationSummaryPayload, error) {
	conversationIDs := make([]string, 0, len(conversations))
	for _, conversation := range conversations {
		conversationIDs = append(conversationIDs, conversation.ID)
	}
	membersByConversationID, usersByID, appsByID, err := s.loadConversationListMembers(conversationIDs)
	if err != nil {
		return nil, err
	}
	memberCountsByConversationID, err := s.loadConversationActiveMemberCounts(conversationIDs)
	if err != nil {
		return nil, err
	}

	payloads := make([]appConversationSummaryPayload, 0, len(conversations))
	for _, conversation := range conversations {
		item := newConversationListItemResponse(
			conversation,
			currentUserID,
			membersByConversationID[conversation.ID],
			usersByID,
			appsByID,
		)
		lastActiveAt := conversation.CreatedAt
		if conversation.LastMessageAt != nil {
			lastActiveAt = *conversation.LastMessageAt
		}
		memberCount := memberCountsByConversationID[conversation.ID]
		if memberCount == 0 {
			memberCount = item.MemberCount
		}
		payloads = append(payloads, appConversationSummaryPayload{
			ConversationID: conversation.ID,
			LastActiveAt:   lastActiveAt,
			MemberCount:    memberCount,
			Name:           item.Name,
			Type:           item.Type,
		})
	}

	return payloads, nil
}

func (s *Server) loadConversationActiveMemberCounts(conversationIDs []string) (map[string]int, error) {
	counts := make(map[string]int, len(conversationIDs))
	if len(conversationIDs) == 0 {
		return counts, nil
	}

	type countRow struct {
		ConversationID string
		Count          int
	}
	var rows []countRow
	if err := s.db.Model(&store.ConversationMember{}).
		Select("conversation_id, COUNT(*) AS count").
		Where("conversation_id IN ? AND left_at IS NULL", conversationIDs).
		Group("conversation_id").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		counts[row.ConversationID] = row.Count
	}

	return counts, nil
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

func newAppSystemMessagePayload(message conversationapp.Message) appMessagePayload {
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

func (s *Server) normalizeAppSendMessageBody(ctx context.Context, raw json.RawMessage) (json.RawMessage, error) {
	body, err := s.messageContentService().Normalize(ctx, raw)
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
	return s.prepareAppSendMessageBodyForUser(ctx, "", raw)
}

func (s *Server) prepareAppSendMessageBodyForUser(ctx context.Context, userID string, raw json.RawMessage) (preparedAppSendMessageBody, error) {
	var envelope appSendMessageBodyEnvelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return preparedAppSendMessageBody{}, newAppRequestFailure("invalid_request", "消息体格式错误")
	}
	switch strings.TrimSpace(envelope.Type) {
	case messageTypeEntityCard:
		contents := s.messageContentService()
		body, err := contents.Prepare(ctx, userID, raw)
		if err != nil {
			switch messageapp.ErrorCodeOf(err) {
			case messageapp.CodeNotFound:
				return preparedAppSendMessageBody{}, newAppRequestFailure("not_found", messageapp.ErrorMessage(err))
			case messageapp.CodeInvalidRequest:
				return preparedAppSendMessageBody{}, newAppRequestFailure("invalid_request", messageapp.ErrorMessage(err))
			default:
				return preparedAppSendMessageBody{}, newAppRequestFailure("internal_error", "生成对象卡片失败")
			}
		}
		return preparedAppSendMessageBody{
			Body:     body,
			Finalize: contents.Finalize,
		}, nil
	case messageTypeText, messageTypeMarkdown, messageTypeLink, messageTypeCard, messageTypeChart:
		body, err := s.normalizeAppSendMessageBody(ctx, raw)
		if err != nil {
			return preparedAppSendMessageBody{}, err
		}
		return preparedAppSendMessageBody{
			Body:     body,
			Finalize: s.messageContentService().Finalize,
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

		opened, _, err := s.conversations.OpenAppForUser(
			context.Background(),
			conversationActorFromUser(user),
			conversationapp.AppIdentity{ID: app.ID, Name: app.Name, Avatar: app.Avatar},
		)
		return store.Conversation{ID: opened.ID, Name: opened.Name, Kind: opened.Type}, err
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

func mapMessageApplicationErrorForApp(err error) error {
	var messageErr *messageapp.Error
	if !errors.As(err, &messageErr) {
		return mapAppMessageError(err)
	}
	switch messageErr.Code {
	case messageapp.CodeNotFound:
		return newAppRequestFailure("not_found", messageErr.Message)
	case messageapp.CodeForbidden:
		return newAppRequestFailure("forbidden", messageErr.Message)
	case messageapp.CodeInvalidRequest:
		return newAppRequestFailure("invalid_request", messageErr.Message)
	default:
		return newAppRequestFailure("internal_error", "服务端错误")
	}
}

func mapAppHistoryReadError(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return newAppRequestFailure("not_found", "会话不存在")
	}
	if errors.Is(err, errConversationAccessDenied) {
		return newAppRequestFailure("forbidden", "无权访问会话")
	}

	return err
}

func mapAppGroupConversationError(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return newAppRequestFailure("not_found", "会话不存在")
	}
	if errors.Is(err, errConversationAccessDenied) || errors.Is(err, conversationapp.ErrAccessDenied) {
		return newAppRequestFailure("forbidden", "无权访问会话")
	}
	if errors.Is(err, conversationapp.ErrNotGroup) {
		return newAppRequestFailure("invalid_request", "只能向群聊添加成员")
	}
	if errors.Is(err, conversationapp.ErrMemberCap) {
		return newAppRequestFailure("invalid_request", "群聊成员不能超过 500 人")
	}
	if errors.Is(err, conversationapp.ErrMemberMissing) {
		return newAppRequestFailure("invalid_request", "成员不存在或已禁用")
	}

	return err
}

func conversationActorFromUser(user store.User) conversationapp.Actor {
	phone := ""
	if user.Phone != nil {
		phone = *user.Phone
	}
	return conversationapp.Actor{
		ID: user.ID, Email: user.Email, Name: user.Name, Nickname: user.Nickname,
		Phone: phone, Avatar: user.Avatar,
	}
}

func legacyConversationItem(value conversationapp.Item) conversationListItemResponse {
	members := make([]conversationMemberResponse, 0, len(value.Members))
	for _, member := range value.Members {
		members = append(members, conversationMemberResponse{
			Avatar: member.Avatar, Email: member.Email, ID: member.ID, Name: member.Name,
			Nickname: member.Nickname, Phone: member.Phone, Role: member.Role, Type: member.Type,
		})
	}
	result := conversationListItemResponse{
		Avatar: value.Avatar, CreatedAt: value.CreatedAt, ID: value.ID,
		LastMessageAt: value.LastMessageAt, LastMessageID: value.LastMessageID,
		LastMessageSeq: value.LastMessageSeq, LastMessageSummary: value.LastMessageSummary,
		LastMentionedSeq: value.LastMentionedSeq, LastReadSeq: value.LastReadSeq,
		MemberCount: value.MemberCount, Members: members, Name: value.Name, Type: value.Type,
		UnreadCount: value.UnreadCount, Visibility: value.Visibility,
	}
	if value.Projects != nil {
		projects := make([]conversationProjectResponse, 0, len(*value.Projects))
		for _, project := range *value.Projects {
			projects = append(projects, conversationProjectResponse{
				Avatar: project.Avatar, Description: project.Description, ID: project.ID, Name: project.Name,
			})
		}
		result.Projects = &projects
	}
	return result
}

func legacyGroupConversation(value conversationapp.Group) groupConversationResponse {
	members := make([]conversationMemberResponse, 0, len(value.Members))
	for _, member := range value.Members {
		members = append(members, conversationMemberResponse{
			Avatar: member.Avatar, Email: member.Email, ID: member.ID, Name: member.Name,
			Nickname: member.Nickname, Phone: member.Phone, Role: member.Role, Type: member.Type,
		})
	}
	return groupConversationResponse{
		Avatar: value.Avatar, CreatedAt: value.CreatedAt, CreatedByUserID: value.CreatedByUserID,
		ID: value.ID, LastMessageAt: value.LastMessageAt, LastMessageID: value.LastMessageID,
		LastMessageSeq: value.LastMessageSeq, LastMessageSummary: value.LastMessageSummary,
		LastMentionedSeq: value.LastMentionedSeq, LastReadSeq: value.LastReadSeq,
		MemberCount: value.MemberCount, Members: members, Name: value.Name,
		PostingPolicy: value.PostingPolicy, Status: value.Status, Type: value.Type,
		UnreadCount: value.UnreadCount, Visibility: value.Visibility,
	}
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
