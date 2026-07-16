package builtintools

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"assistant/internal/mcpclient"
)

const (
	sourceName                            = "builtin"
	sleepToolName                         = "sleep"
	getAttachmentsToolName                = "get_attachments"
	endConversationToolName               = "end_conversation"
	contactsToolName                      = "contacts"
	conversationsToolName                 = "conversations"
	contactsOperationSearchUsers          = "search_users"
	contactsOperationSearchApps           = "search_apps"
	contactsOperationSearchGroups         = "search_groups"
	conversationsOperationSearch          = "search"
	conversationsOperationRead            = "read_history"
	conversationsOperationReply           = "reply"
	conversationsOperationSend            = "send"
	conversationsOperationReplyEntityCard = "reply_entity_card"
	conversationsOperationSendEntityCard  = "send_entity_card"
	conversationsOperationWait            = "wait_for_reply"
	conversationsOperationCreate          = "create_group"
	conversationsOperationAdd             = "add_members"
	methodContactsUsersList               = "contacts.users.list"
	methodContactsAppsList                = "contacts.apps.list"
	methodContactsGroupsList              = "contacts.groups.list"
	methodConversationsList               = "conversations.list"
	methodConversationHistoryRead         = "conversation.history.read"
	methodGroupConversationsList          = "group_conversations.list"
	methodCreateGroup                     = "group_conversations.create"
	methodAddGroupMembers                 = "group_conversations.members.add"
	methodMessageSend                     = "message.send"
	methodMessageSendAsUser               = "message.send_as_user"
	methodTemporaryFilesReadURLs          = "temporary_files.read_urls"
	minSleepSeconds                       = 5
	maxSleepSeconds                       = 30
	minWaitForReplySeconds                = 5
	maxWaitForReplySeconds                = 60
	waitForReplyPollSeconds               = 5
	waitForReplyMessageLimit              = 30
	defaultSleepUnit                      = time.Second
	messageTypeText                       = "text"
	messageTypeMarkdown                   = "markdown"
	messageTypeImage                      = "image"
	messageTypeFile                       = "file"
	messageTypeCard                       = "card"
	messageTypeChart                      = "chart"
	messageTypeEntityCard                 = "entity_card"
	maxChartMessageBodyBytes              = 64 * 1024
	maxChartMessageTitleRunes             = 16
	maxChartMessageDescriptionRunes       = 128
	maxChartMessageValue                  = 1_000_000_000_000_000
)

type sleepFunc func(context.Context, time.Duration) error
type scopeContextKey struct{}

type AppRequester interface {
	Request(context.Context, string, any) (json.RawMessage, error)
}

type Authorization struct {
	ActorID          string
	ActorType        string
	TriggerMessageID string
}

type AuthorizationResolver interface {
	ResolveAuthorization(ref string) (Authorization, bool)
}

type AuthorizationResolverFunc func(ref string) (Authorization, bool)

func (f AuthorizationResolverFunc) ResolveAuthorization(ref string) (Authorization, bool) {
	return f(ref)
}

type ConversationWaitRegistration interface {
	Close()
}

type ConversationWaitRegistrar interface {
	RegisterConversationWait(conversationID string, afterSeq int64, actorType string, actorID string) (ConversationWaitRegistration, error)
}

type Scope struct {
	AuthorizationResolver AuthorizationResolver
	ConversationWaiter    ConversationWaitRegistrar
	ConversationID        string
	ConversationType      string
	CurrentAppID          string
	CurrentUserID         string
	Requester             AppRequester
	TriggerMessageID      string
}

type Source struct {
	sleep sleepFunc
}

type sleepInput struct {
	Seconds float64 `json:"seconds"`
}

type contactsInput struct {
	Operation string          `json:"operation"`
	Arguments json.RawMessage `json:"arguments"`
	RunAs     *runAsInput     `json:"runas,omitempty"`
}

type conversationsInput struct {
	Operation string          `json:"operation"`
	Arguments json.RawMessage `json:"arguments"`
	RunAs     *runAsInput     `json:"runas,omitempty"`
}

type waitForReplyArguments struct {
	ConversationID string `json:"conversation_id"`
	TimeoutSeconds int    `json:"timeout_seconds"`
	AfterSeq       int64  `json:"after_seq"`
}

type contactsSearchArguments struct {
	Keyword string `json:"keyword"`
}

type contactsSearchPayload struct {
	Keyword string        `json:"keyword"`
	RunAs   *runAsPayload `json:"runas,omitempty"`
}

type runAsInput struct {
	AuthorizationRef string `json:"authorization_ref"`
	ID               string `json:"id"`
	Type             string `json:"type"`
}

type runAsPayload struct {
	AuthorizationConversationID string `json:"authorization_conversation_id"`
	ID                          string `json:"id"`
	TriggerMessageID            string `json:"trigger_message_id"`
	Type                        string `json:"type"`
}

type recentConversationsInput struct {
	AuthorizationRef string `json:"authorization_ref"`
	Keyword          string `json:"keyword"`
	Limit            int    `json:"limit"`
}

type readHistoryInput struct {
	AppID            string `json:"app_id"`
	AuthorizationRef string `json:"authorization_ref"`
	BeforeSeq        int64  `json:"before_seq"`
	ConversationID   string `json:"conversation_id"`
	Limit            int    `json:"limit"`
	UserID           string `json:"user_id"`
}

type messageInput struct {
	AuthorizationRef string          `json:"authorization_ref"`
	ChartType        string          `json:"chart_type"`
	ContactID        string          `json:"contact_id"`
	Content          string          `json:"content"`
	ConversationID   string          `json:"conversation_id"`
	Data             json.RawMessage `json:"data"`
	Description      string          `json:"description"`
	Name             string          `json:"name"`
	TargetType       string          `json:"target_type"`
	Title            string          `json:"title"`
	Type             string          `json:"type"`
	URL              string          `json:"url"`
}

type entityCardInput struct {
	AuthorizationRef string `json:"authorization_ref"`
	ContactID        string `json:"contact_id"`
	ConversationID   string `json:"conversation_id"`
	EntityID         string `json:"entity_id"`
	EntityType       string `json:"entity_type"`
	TargetType       string `json:"target_type"`
}

type createGroupInput struct {
	AuthorizationRef string   `json:"authorization_ref"`
	MemberIDs        []string `json:"member_ids"`
	Name             string   `json:"name"`
}

type addGroupMembersInput struct {
	AuthorizationRef string   `json:"authorization_ref"`
	ConversationID   string   `json:"conversation_id"`
	MemberIDs        []string `json:"member_ids"`
}

type readFileURLsInput struct {
	FileIDs []string `json:"file_ids"`
}

type scopedMessagePayload struct {
	AuthorizationRef string          `json:"-"`
	ChartType        string          `json:"chart_type,omitempty"`
	Content          string          `json:"content,omitempty"`
	Data             json.RawMessage `json:"data,omitempty"`
	Description      string          `json:"description,omitempty"`
	EntityID         string          `json:"entity_id,omitempty"`
	EntityType       string          `json:"entity_type,omitempty"`
	Name             string          `json:"name,omitempty"`
	Title            string          `json:"title,omitempty"`
	Type             string          `json:"type"`
	URL              string          `json:"url,omitempty"`
}

type sendMessageTargetPayload struct {
	ConversationID string `json:"conversation_id,omitempty"`
	Type           string `json:"type"`
}

type sendMessagePayload struct {
	ActorUserID                 string                   `json:"actor_user_id,omitempty"`
	AuthorizationConversationID string                   `json:"authorization_conversation_id,omitempty"`
	Message                     scopedMessagePayload     `json:"message"`
	Target                      sendMessageTargetPayload `json:"target"`
	TriggerMessageID            string                   `json:"trigger_message_id,omitempty"`
}

type sendAsUserPayload struct {
	ActorUserID                 string               `json:"actor_user_id"`
	AuthorizationConversationID string               `json:"authorization_conversation_id,omitempty"`
	Message                     scopedMessagePayload `json:"message"`
	Target                      sendAsUserTarget     `json:"target"`
	TargetUserID                string               `json:"target_user_id,omitempty"`
	TriggerMessageID            string               `json:"trigger_message_id"`
}

type sendAsUserTarget struct {
	ConversationID string `json:"conversation_id,omitempty"`
	Type           string `json:"type"`
	UserID         string `json:"user_id,omitempty"`
}

type recentConversationsPayload struct {
	ActorUserID                 string `json:"actor_user_id"`
	AuthorizationConversationID string `json:"authorization_conversation_id,omitempty"`
	Keyword                     string `json:"keyword"`
	Limit                       int    `json:"limit"`
	TriggerMessageID            string `json:"trigger_message_id"`
}

type readHistoryPayload struct {
	AppID                       string `json:"app_id,omitempty"`
	ActorUserID                 string `json:"actor_user_id"`
	AuthorizationConversationID string `json:"authorization_conversation_id,omitempty"`
	BeforeSeq                   int64  `json:"before_seq,omitempty"`
	ConversationID              string `json:"conversation_id,omitempty"`
	Limit                       int    `json:"limit,omitempty"`
	TriggerMessageID            string `json:"trigger_message_id"`
	UserID                      string `json:"user_id,omitempty"`
}

type conversationHistoryEnvelope struct {
	Messages []json.RawMessage `json:"messages"`
}

type conversationHistoryMessageMetadata struct {
	Sender struct {
		ID   string `json:"id"`
		Type string `json:"type"`
	} `json:"sender"`
	Seq int64 `json:"seq"`
}

type createGroupPayload struct {
	ActorUserID                 string   `json:"actor_user_id"`
	AuthorizationConversationID string   `json:"authorization_conversation_id,omitempty"`
	MemberIDs                   []string `json:"member_ids"`
	Name                        string   `json:"name"`
	TriggerMessageID            string   `json:"trigger_message_id"`
}

type addGroupMembersPayload struct {
	ActorUserID                 string   `json:"actor_user_id"`
	AuthorizationConversationID string   `json:"authorization_conversation_id,omitempty"`
	ConversationID              string   `json:"conversation_id"`
	MemberIDs                   []string `json:"member_ids"`
	TriggerMessageID            string   `json:"trigger_message_id"`
}

type readTemporaryFileURLsPayload struct {
	FileIDs []string `json:"file_ids"`
}

type readTemporaryFileURLsResponse struct {
	URLs []temporaryFileReadURL `json:"urls"`
}

type temporaryFileReadURL struct {
	ExpiresAt string `json:"expires_at"`
	FileID    string `json:"file_id"`
	URL       string `json:"url"`
}

type readFileURLsToolResult struct {
	URLs   []temporaryFileReadURL `json:"urls"`
	Errors []readFileURLError     `json:"errors,omitempty"`
}

type readFileURLError struct {
	Error  string `json:"error"`
	FileID string `json:"file_id"`
}

func WithScope(ctx context.Context, scope Scope) context.Context {
	return context.WithValue(ctx, scopeContextKey{}, scope)
}

func NewSource() *Source {
	return newSourceWithSleeper(realSleep)
}

func newSourceWithSleeper(sleep sleepFunc) *Source {
	if sleep == nil {
		sleep = realSleep
	}

	return &Source{sleep: sleep}
}

func (s *Source) SourceName() string {
	return sourceName
}

func (s *Source) ListTools(ctx context.Context) ([]mcpclient.Tool, error) {
	return s.listedTools(), nil
}

func (s *Source) CallTool(ctx context.Context, name string, input json.RawMessage) (mcpclient.ToolResult, error) {
	if err := ctx.Err(); err != nil {
		return mcpclient.ToolResult{}, err
	}

	switch name {
	case helpToolName:
		return s.callHelp(ctx, input)
	case sleepToolName:
		return s.callSleep(ctx, input)
	case getAttachmentsToolName:
		return callReadFileURLs(ctx, input)
	case endConversationToolName:
		return callEndConversation(ctx, input)
	case contactsToolName:
		return callContacts(ctx, input)
	case conversationsToolName:
		return s.callConversations(ctx, input)
	case projectsToolName:
		return callProjects(ctx, input)
	default:
		return mcpclient.ToolResult{}, fmt.Errorf("unknown builtin tool %q", name)
	}
}

func callEndConversation(ctx context.Context, input json.RawMessage) (mcpclient.ToolResult, error) {
	if len(input) > 0 && string(input) != "null" {
		var properties map[string]json.RawMessage
		if err := json.Unmarshal(input, &properties); err != nil {
			return mcpclient.ToolResult{}, fmt.Errorf("parse end_conversation input: %w", err)
		}
		if len(properties) > 0 {
			return mcpclient.ToolResult{}, fmt.Errorf("end_conversation does not accept arguments")
		}
	}
	return mcpclient.ToolResult{Content: "当前处理已结束", Final: true}, nil
}

func (s *Source) callSleep(ctx context.Context, input json.RawMessage) (mcpclient.ToolResult, error) {
	duration, seconds, err := sleepDuration(input)
	if err != nil {
		return mcpclient.ToolResult{}, err
	}
	if err := s.sleep(ctx, duration); err != nil {
		return mcpclient.ToolResult{}, err
	}

	return mcpclient.ToolResult{Content: fmt.Sprintf("slept %s", formatSeconds(seconds))}, nil
}

func (s *Source) callConversations(ctx context.Context, input json.RawMessage) (mcpclient.ToolResult, error) {
	var parsed conversationsInput
	if err := json.Unmarshal(input, &parsed); err != nil {
		return mcpclient.ToolResult{}, fmt.Errorf("parse conversations input: %w", err)
	}
	parsed.Operation = strings.ToLower(strings.TrimSpace(parsed.Operation))
	if parsed.Operation == "" {
		return mcpclient.ToolResult{}, fmt.Errorf("operation is required; use help to inspect supported operations")
	}

	switch parsed.Operation {
	case conversationsOperationReply:
		if parsed.RunAs != nil {
			return mcpclient.ToolResult{}, fmt.Errorf("runas is not supported for reply; use send for delegated sending")
		}
		return callReply(ctx, parsed.Arguments)
	case conversationsOperationWait:
		return s.callWaitForReply(ctx, parsed)
	case conversationsOperationSearch, conversationsOperationRead, conversationsOperationSend, conversationsOperationReplyEntityCard, conversationsOperationSendEntityCard, conversationsOperationCreate, conversationsOperationAdd:
		legacyInput, err := conversationLegacyUserInput(ctx, parsed)
		if err != nil {
			return mcpclient.ToolResult{}, err
		}
		switch parsed.Operation {
		case conversationsOperationSearch:
			return callRecentConversations(ctx, legacyInput)
		case conversationsOperationRead:
			return callReadHistory(ctx, legacyInput)
		case conversationsOperationSend:
			return callSendAsUser(ctx, legacyInput)
		case conversationsOperationReplyEntityCard:
			return callReplyEntityCard(ctx, legacyInput)
		case conversationsOperationSendEntityCard:
			return callSendEntityCard(ctx, legacyInput)
		case conversationsOperationCreate:
			return callCreateGroup(ctx, legacyInput)
		default:
			return callAddGroupMembers(ctx, legacyInput)
		}
	default:
		return mcpclient.ToolResult{}, fmt.Errorf("unsupported conversations operation %q; use help to inspect supported operations", parsed.Operation)
	}
}

func conversationLegacyUserInput(ctx context.Context, input conversationsInput) (json.RawMessage, error) {
	scope, err := requireScope(ctx)
	if err != nil {
		return nil, err
	}
	if input.RunAs == nil {
		return nil, fmt.Errorf("runas is required for conversations operation %q", input.Operation)
	}
	authorization, err := authorizedRunAs(scope, input.RunAs)
	if err != nil {
		return nil, err
	}
	if authorization.ActorType != "user" {
		return nil, fmt.Errorf("conversations operation %q currently requires a user runas identity", input.Operation)
	}

	arguments := map[string]any{}
	if len(input.Arguments) > 0 && string(input.Arguments) != "null" {
		if err := json.Unmarshal(input.Arguments, &arguments); err != nil {
			return nil, fmt.Errorf("parse conversations %s arguments: %w", input.Operation, err)
		}
	}
	arguments["authorization_ref"] = strings.TrimSpace(input.RunAs.AuthorizationRef)
	raw, err := json.Marshal(arguments)
	if err != nil {
		return nil, err
	}
	return raw, nil
}

func authorizedRunAs(scope Scope, input *runAsInput) (Authorization, error) {
	if input == nil {
		return Authorization{}, fmt.Errorf("runas is required")
	}
	runAsType := strings.ToLower(strings.TrimSpace(input.Type))
	runAsID := strings.TrimSpace(input.ID)
	if runAsType != "user" {
		return Authorization{}, fmt.Errorf("runas.type must be user")
	}
	if runAsID == "" {
		return Authorization{}, fmt.Errorf("runas.id is required")
	}
	authorization, err := requireAuthorization(scope, input.AuthorizationRef)
	if err != nil {
		return Authorization{}, err
	}
	if authorization.ActorType != runAsType || authorization.ActorID != runAsID {
		return Authorization{}, fmt.Errorf("authorization_ref does not authorize runas identity")
	}
	return authorization, nil
}

func (s *Source) callWaitForReply(ctx context.Context, input conversationsInput) (mcpclient.ToolResult, error) {
	scope, err := requireScope(ctx)
	if err != nil {
		return mcpclient.ToolResult{}, err
	}
	var arguments waitForReplyArguments
	if err := json.Unmarshal(input.Arguments, &arguments); err != nil {
		return mcpclient.ToolResult{}, fmt.Errorf("parse conversations wait_for_reply arguments: %w", err)
	}
	arguments.ConversationID = strings.TrimSpace(arguments.ConversationID)
	if arguments.ConversationID == "" {
		return mcpclient.ToolResult{}, fmt.Errorf("conversation_id is required")
	}
	if arguments.AfterSeq <= 0 {
		return mcpclient.ToolResult{}, fmt.Errorf("after_seq must be a positive integer")
	}
	if arguments.TimeoutSeconds < minWaitForReplySeconds || arguments.TimeoutSeconds > maxWaitForReplySeconds {
		return mcpclient.ToolResult{}, fmt.Errorf("timeout_seconds must be between %d and %d", minWaitForReplySeconds, maxWaitForReplySeconds)
	}

	actor, err := authorizedRunAs(scope, input.RunAs)
	if err != nil {
		return mcpclient.ToolResult{}, err
	}

	if scope.ConversationWaiter != nil {
		registration, err := scope.ConversationWaiter.RegisterConversationWait(
			arguments.ConversationID,
			arguments.AfterSeq,
			actor.ActorType,
			actor.ActorID,
		)
		if err != nil {
			return mcpclient.ToolResult{}, err
		}
		defer registration.Close()
	}

	waitedSeconds := 0
	latestSeq := arguments.AfterSeq
	for {
		messages, err := latestConversationMessages(ctx, scope, input.RunAs, actor, arguments.ConversationID, waitForReplyMessageLimit)
		if err != nil {
			return mcpclient.ToolResult{}, err
		}
		latestSeq, err = latestMessageSequence(messages)
		if err != nil {
			return mcpclient.ToolResult{}, err
		}
		replies, err := conversationRepliesAfter(messages, arguments.AfterSeq, actor)
		if err != nil {
			return mcpclient.ToolResult{}, err
		}
		if len(replies) > 0 {
			return jsonToolResult(map[string]any{
				"status":          "replied",
				"replied":         true,
				"conversation_id": arguments.ConversationID,
				"after_seq":       arguments.AfterSeq,
				"latest_seq":      latestSeq,
				"waited_seconds":  waitedSeconds,
				"messages":        replies,
			})
		}
		if waitedSeconds >= arguments.TimeoutSeconds {
			break
		}

		waitSeconds := waitForReplyPollSeconds
		if remaining := arguments.TimeoutSeconds - waitedSeconds; remaining < waitSeconds {
			waitSeconds = remaining
		}
		if err := s.sleep(ctx, time.Duration(waitSeconds)*time.Second); err != nil {
			return mcpclient.ToolResult{}, err
		}
		waitedSeconds += waitSeconds
	}

	return jsonToolResult(map[string]any{
		"status":          "timeout",
		"replied":         false,
		"conversation_id": arguments.ConversationID,
		"after_seq":       arguments.AfterSeq,
		"latest_seq":      latestSeq,
		"waited_seconds":  waitedSeconds,
		"messages":        []json.RawMessage{},
		"message":         "等待超时，会话中没有检测到新的回复。",
	})
}

func latestConversationMessages(ctx context.Context, scope Scope, _ *runAsInput, actor Authorization, conversationID string, limit int) ([]json.RawMessage, error) {
	raw, err := scope.Requester.Request(ctx, methodConversationHistoryRead, readHistoryPayload{
		ActorUserID:                 actor.ActorID,
		AuthorizationConversationID: strings.TrimSpace(scope.ConversationID),
		ConversationID:              conversationID,
		Limit:                       limit,
		TriggerMessageID:            actor.TriggerMessageID,
	})
	if err != nil {
		return nil, err
	}
	var response conversationHistoryEnvelope
	if err := json.Unmarshal(raw, &response); err != nil {
		return nil, fmt.Errorf("parse conversation history response: %w", err)
	}
	return response.Messages, nil
}

func latestMessageSequence(messages []json.RawMessage) (int64, error) {
	var latest int64
	for _, raw := range messages {
		var message conversationHistoryMessageMetadata
		if err := json.Unmarshal(raw, &message); err != nil {
			return 0, fmt.Errorf("parse conversation message: %w", err)
		}
		if message.Seq > latest {
			latest = message.Seq
		}
	}
	return latest, nil
}

func conversationRepliesAfter(messages []json.RawMessage, baselineSeq int64, actor Authorization) ([]json.RawMessage, error) {
	replies := make([]json.RawMessage, 0, len(messages))
	for _, raw := range messages {
		var message conversationHistoryMessageMetadata
		if err := json.Unmarshal(raw, &message); err != nil {
			return nil, fmt.Errorf("parse conversation message: %w", err)
		}
		if message.Seq <= baselineSeq {
			continue
		}
		senderType := strings.ToLower(strings.TrimSpace(message.Sender.Type))
		senderID := strings.TrimSpace(message.Sender.ID)
		if senderType != "user" && senderType != "app" {
			continue
		}
		if actor.ActorType != "" && senderType == actor.ActorType && senderID == actor.ActorID {
			continue
		}
		replies = append(replies, raw)
	}
	if len(replies) > waitForReplyMessageLimit {
		replies = replies[len(replies)-waitForReplyMessageLimit:]
	}
	return replies, nil
}

func callContacts(ctx context.Context, input json.RawMessage) (mcpclient.ToolResult, error) {
	var parsed contactsInput
	if len(input) > 0 {
		if err := json.Unmarshal(input, &parsed); err != nil {
			return mcpclient.ToolResult{}, fmt.Errorf("parse contacts input: %w", err)
		}
	}
	operation := strings.ToLower(strings.TrimSpace(parsed.Operation))
	if operation == "" {
		return mcpclient.ToolResult{}, fmt.Errorf("operation is required; use help to inspect supported operations")
	}

	switch operation {
	case contactsOperationSearchUsers:
		return searchContacts(ctx, parsed, methodContactsUsersList)
	case contactsOperationSearchApps:
		return searchContacts(ctx, parsed, methodContactsAppsList)
	case contactsOperationSearchGroups:
		return searchContacts(ctx, parsed, methodContactsGroupsList)
	default:
		return mcpclient.ToolResult{}, fmt.Errorf("unsupported contacts operation %q; use help to inspect supported operations", parsed.Operation)
	}
}

func searchContacts(ctx context.Context, input contactsInput, method string) (mcpclient.ToolResult, error) {
	arguments := contactsSearchArguments{}
	if len(input.Arguments) > 0 && string(input.Arguments) != "null" {
		if err := json.Unmarshal(input.Arguments, &arguments); err != nil {
			return mcpclient.ToolResult{}, fmt.Errorf("parse contacts search arguments: %w", err)
		}
	}
	scope, err := requireScope(ctx)
	if err != nil {
		return mcpclient.ToolResult{}, err
	}
	runAs, err := resolveRunAs(scope, input.RunAs)
	if err != nil {
		return mcpclient.ToolResult{}, err
	}
	return requestTool(ctx, scope.Requester, method, contactsSearchPayload{
		Keyword: strings.TrimSpace(arguments.Keyword),
		RunAs:   runAs,
	})
}

func resolveRunAs(scope Scope, input *runAsInput) (*runAsPayload, error) {
	if input == nil {
		return nil, fmt.Errorf("runas is required")
	}

	runAs := runAsInput{
		AuthorizationRef: strings.TrimSpace(input.AuthorizationRef),
		ID:               strings.TrimSpace(input.ID),
		Type:             strings.ToLower(strings.TrimSpace(input.Type)),
	}
	if runAs.Type != "user" {
		return nil, fmt.Errorf("runas.type must be user")
	}
	if runAs.ID == "" {
		return nil, fmt.Errorf("runas.id is required")
	}
	authorization, err := requireAuthorization(scope, runAs.AuthorizationRef)
	if err != nil {
		return nil, err
	}
	if authorization.ActorType != runAs.Type || authorization.ActorID != runAs.ID {
		return nil, fmt.Errorf("authorization_ref does not authorize runas identity")
	}

	return &runAsPayload{
		AuthorizationConversationID: strings.TrimSpace(scope.ConversationID),
		ID:                          runAs.ID,
		TriggerMessageID:            authorization.TriggerMessageID,
		Type:                        runAs.Type,
	}, nil
}

func runAsInputSchema() map[string]any {
	return map[string]any{
		"type":        "object",
		"description": "必填授权用户执行身份；type 必须为 user，id 和 authorization_ref 必须与当前授权候选完全匹配。",
		"required":    []string{"type", "id", "authorization_ref"},
		"properties": map[string]any{
			"type": map[string]any{
				"type":        "string",
				"enum":        []string{"user"},
				"description": "执行身份类型，固定为 user。",
			},
			"id": map[string]any{
				"type":        "string",
				"description": "用户 ID；必须来自可信上下文，不要猜测。",
			},
			"authorization_ref": map[string]any{
				"type":        "string",
				"description": "授权引用，只能从当前上下文 authorization_candidates 中选择；对应候选的 sender_type 和 sender_id 必须分别匹配 runas.type 和 runas.id。",
			},
		},
		"additionalProperties": false,
	}
}

func callRecentConversations(ctx context.Context, input json.RawMessage) (mcpclient.ToolResult, error) {
	scope, err := requireScope(ctx)
	if err != nil {
		return mcpclient.ToolResult{}, err
	}

	var parsed recentConversationsInput
	if len(input) > 0 {
		if err := json.Unmarshal(input, &parsed); err != nil {
			return mcpclient.ToolResult{}, fmt.Errorf("parse conversations search input: %w", err)
		}
	}
	auth, err := requireUserAuthorization(scope, parsed.AuthorizationRef)
	if err != nil {
		return mcpclient.ToolResult{}, err
	}

	return requestTool(ctx, scope.Requester, methodConversationsList, recentConversationsPayload{
		ActorUserID:                 auth.ActorID,
		AuthorizationConversationID: strings.TrimSpace(scope.ConversationID),
		Keyword:                     strings.TrimSpace(parsed.Keyword),
		Limit:                       parsed.Limit,
		TriggerMessageID:            auth.TriggerMessageID,
	})
}

func callReadHistory(ctx context.Context, input json.RawMessage) (mcpclient.ToolResult, error) {
	scope, err := requireScope(ctx)
	if err != nil {
		return mcpclient.ToolResult{}, err
	}

	var parsed readHistoryInput
	if len(input) > 0 {
		if err := json.Unmarshal(input, &parsed); err != nil {
			return mcpclient.ToolResult{}, fmt.Errorf("parse conversations read_history input: %w", err)
		}
	}
	auth, err := requireUserAuthorization(scope, parsed.AuthorizationRef)
	if err != nil {
		return mcpclient.ToolResult{}, err
	}
	payload := readHistoryPayload{
		ActorUserID:                 auth.ActorID,
		AppID:                       strings.TrimSpace(parsed.AppID),
		AuthorizationConversationID: strings.TrimSpace(scope.ConversationID),
		BeforeSeq:                   parsed.BeforeSeq,
		ConversationID:              strings.TrimSpace(parsed.ConversationID),
		Limit:                       parsed.Limit,
		TriggerMessageID:            auth.TriggerMessageID,
		UserID:                      strings.TrimSpace(parsed.UserID),
	}
	if countReadHistorySelectors(payload) != 1 {
		return mcpclient.ToolResult{}, fmt.Errorf("exactly one of conversation_id, user_id, app_id is required")
	}

	return requestTool(ctx, scope.Requester, methodConversationHistoryRead, payload)
}

func countReadHistorySelectors(payload readHistoryPayload) int {
	count := 0
	if payload.ConversationID != "" {
		count++
	}
	if payload.UserID != "" {
		count++
	}
	if payload.AppID != "" {
		count++
	}
	return count
}

func callReply(ctx context.Context, input json.RawMessage) (mcpclient.ToolResult, error) {
	scope, err := requireScope(ctx)
	if err != nil {
		return mcpclient.ToolResult{}, err
	}
	if strings.TrimSpace(scope.ConversationID) == "" || strings.TrimSpace(scope.ConversationType) == "" {
		return mcpclient.ToolResult{}, fmt.Errorf("current conversation scope is missing")
	}
	message, err := parseMessageInput(input, false)
	if err != nil {
		return mcpclient.ToolResult{}, err
	}

	result, err := requestTool(ctx, scope.Requester, methodMessageSend, sendMessagePayload{
		Target: sendMessageTargetPayload{
			Type:           strings.TrimSpace(scope.ConversationType),
			ConversationID: strings.TrimSpace(scope.ConversationID),
		},
		Message: message,
	})
	if err != nil {
		return mcpclient.ToolResult{}, err
	}
	return result, nil
}

func callSendAsUser(ctx context.Context, input json.RawMessage) (mcpclient.ToolResult, error) {
	scope, err := requireScope(ctx)
	if err != nil {
		return mcpclient.ToolResult{}, err
	}
	parsed, target, targetUserID, err := parseSendAsUserInput(input)
	if err != nil {
		return mcpclient.ToolResult{}, err
	}
	auth, err := requireUserAuthorization(scope, parsed.AuthorizationRef)
	if err != nil {
		return mcpclient.ToolResult{}, err
	}

	return requestTool(ctx, scope.Requester, methodMessageSendAsUser, sendAsUserPayload{
		ActorUserID:                 auth.ActorID,
		AuthorizationConversationID: strings.TrimSpace(scope.ConversationID),
		Target:                      target,
		TargetUserID:                targetUserID,
		TriggerMessageID:            auth.TriggerMessageID,
		Message:                     parsed,
	})
}

func callReplyEntityCard(ctx context.Context, input json.RawMessage) (mcpclient.ToolResult, error) {
	scope, err := requireScope(ctx)
	if err != nil {
		return mcpclient.ToolResult{}, err
	}
	if strings.TrimSpace(scope.ConversationID) == "" || strings.TrimSpace(scope.ConversationType) == "" {
		return mcpclient.ToolResult{}, fmt.Errorf("current conversation scope is missing")
	}
	message, parsed, err := parseEntityCardInput(input)
	if err != nil {
		return mcpclient.ToolResult{}, err
	}
	auth, err := requireUserAuthorization(scope, parsed.AuthorizationRef)
	if err != nil {
		return mcpclient.ToolResult{}, err
	}

	result, err := requestTool(ctx, scope.Requester, methodMessageSend, sendMessagePayload{
		ActorUserID:                 auth.ActorID,
		AuthorizationConversationID: strings.TrimSpace(scope.ConversationID),
		Message:                     message,
		Target: sendMessageTargetPayload{
			Type:           strings.TrimSpace(scope.ConversationType),
			ConversationID: strings.TrimSpace(scope.ConversationID),
		},
		TriggerMessageID: auth.TriggerMessageID,
	})
	if err != nil {
		return mcpclient.ToolResult{}, err
	}
	return result, nil
}

func callSendEntityCard(ctx context.Context, input json.RawMessage) (mcpclient.ToolResult, error) {
	scope, err := requireScope(ctx)
	if err != nil {
		return mcpclient.ToolResult{}, err
	}
	message, parsed, err := parseEntityCardInput(input)
	if err != nil {
		return mcpclient.ToolResult{}, err
	}
	target, targetUserID, err := parseEntityCardTarget(parsed)
	if err != nil {
		return mcpclient.ToolResult{}, err
	}
	auth, err := requireUserAuthorization(scope, parsed.AuthorizationRef)
	if err != nil {
		return mcpclient.ToolResult{}, err
	}

	return requestTool(ctx, scope.Requester, methodMessageSendAsUser, sendAsUserPayload{
		ActorUserID:                 auth.ActorID,
		AuthorizationConversationID: strings.TrimSpace(scope.ConversationID),
		Message:                     message,
		Target:                      target,
		TargetUserID:                targetUserID,
		TriggerMessageID:            auth.TriggerMessageID,
	})
}

func callCreateGroup(ctx context.Context, input json.RawMessage) (mcpclient.ToolResult, error) {
	scope, err := requireScope(ctx)
	if err != nil {
		return mcpclient.ToolResult{}, err
	}

	var parsed createGroupInput
	if err := json.Unmarshal(input, &parsed); err != nil {
		return mcpclient.ToolResult{}, fmt.Errorf("parse conversations create_group input: %w", err)
	}
	auth, err := requireUserAuthorization(scope, parsed.AuthorizationRef)
	if err != nil {
		return mcpclient.ToolResult{}, err
	}
	name := strings.TrimSpace(parsed.Name)
	if name == "" {
		return mcpclient.ToolResult{}, fmt.Errorf("name is required")
	}
	memberIDs, err := normalizeToolMemberIDs(parsed.MemberIDs)
	if err != nil {
		return mcpclient.ToolResult{}, err
	}

	return requestTool(ctx, scope.Requester, methodCreateGroup, createGroupPayload{
		ActorUserID:                 auth.ActorID,
		AuthorizationConversationID: strings.TrimSpace(scope.ConversationID),
		TriggerMessageID:            auth.TriggerMessageID,
		Name:                        name,
		MemberIDs:                   memberIDs,
	})
}

func callAddGroupMembers(ctx context.Context, input json.RawMessage) (mcpclient.ToolResult, error) {
	scope, err := requireScope(ctx)
	if err != nil {
		return mcpclient.ToolResult{}, err
	}

	var parsed addGroupMembersInput
	if err := json.Unmarshal(input, &parsed); err != nil {
		return mcpclient.ToolResult{}, fmt.Errorf("parse conversations add_members input: %w", err)
	}
	auth, err := requireUserAuthorization(scope, parsed.AuthorizationRef)
	if err != nil {
		return mcpclient.ToolResult{}, err
	}
	conversationID := strings.TrimSpace(parsed.ConversationID)
	if conversationID == "" {
		if strings.TrimSpace(scope.ConversationType) != "group" || strings.TrimSpace(scope.ConversationID) == "" {
			return mcpclient.ToolResult{}, fmt.Errorf("conversation_id is required outside a group conversation")
		}
		conversationID = strings.TrimSpace(scope.ConversationID)
	}
	memberIDs, err := normalizeToolMemberIDs(parsed.MemberIDs)
	if err != nil {
		return mcpclient.ToolResult{}, err
	}

	return requestTool(ctx, scope.Requester, methodAddGroupMembers, addGroupMembersPayload{
		ActorUserID:                 auth.ActorID,
		AuthorizationConversationID: strings.TrimSpace(scope.ConversationID),
		ConversationID:              conversationID,
		TriggerMessageID:            auth.TriggerMessageID,
		MemberIDs:                   memberIDs,
	})
}

func callReadFileURLs(ctx context.Context, input json.RawMessage) (mcpclient.ToolResult, error) {
	scope, err := requireScope(ctx)
	if err != nil {
		return mcpclient.ToolResult{}, err
	}

	fileIDs, err := parseReadFileURLsInput(input)
	if err != nil {
		return mcpclient.ToolResult{}, err
	}

	result, err := readFileURLsBestEffort(ctx, scope.Requester, fileIDs)
	if err != nil {
		return mcpclient.ToolResult{}, err
	}
	content, err := json.Marshal(result)
	if err != nil {
		return mcpclient.ToolResult{}, err
	}

	return mcpclient.ToolResult{Content: string(content)}, nil
}

func parseReadFileURLsInput(input json.RawMessage) ([]string, error) {
	var parsed readFileURLsInput
	if err := json.Unmarshal(input, &parsed); err != nil {
		return nil, fmt.Errorf("parse get_attachments input: %w", err)
	}
	fileIDs := make([]string, 0, len(parsed.FileIDs))
	seen := map[string]struct{}{}
	for _, rawFileID := range parsed.FileIDs {
		fileID := strings.TrimSpace(rawFileID)
		if fileID == "" {
			continue
		}
		if _, ok := seen[fileID]; ok {
			continue
		}
		seen[fileID] = struct{}{}
		fileIDs = append(fileIDs, fileID)
	}
	if len(fileIDs) == 0 {
		return nil, fmt.Errorf("file_ids is required")
	}

	return fileIDs, nil
}

func readFileURLsBestEffort(ctx context.Context, requester AppRequester, fileIDs []string) (readFileURLsToolResult, error) {
	urls, err := requestTemporaryFileURLs(ctx, requester, fileIDs)
	if err == nil {
		return readFileURLsToolResult{URLs: urls}, nil
	}
	if err := ctx.Err(); err != nil {
		return readFileURLsToolResult{}, err
	}

	result := readFileURLsToolResult{
		URLs:   make([]temporaryFileReadURL, 0, len(fileIDs)),
		Errors: make([]readFileURLError, 0),
	}
	for _, fileID := range fileIDs {
		if err := ctx.Err(); err != nil {
			return readFileURLsToolResult{}, err
		}
		urls, err := requestTemporaryFileURLs(ctx, requester, []string{fileID})
		if err != nil {
			result.Errors = append(result.Errors, readFileURLError{FileID: fileID, Error: err.Error()})
			continue
		}
		if len(urls) == 0 {
			result.Errors = append(result.Errors, readFileURLError{FileID: fileID, Error: "temporary file read URL not found"})
			continue
		}
		result.URLs = append(result.URLs, urls...)
	}

	return result, nil
}

func requestTemporaryFileURLs(ctx context.Context, requester AppRequester, fileIDs []string) ([]temporaryFileReadURL, error) {
	raw, err := requester.Request(ctx, methodTemporaryFilesReadURLs, readTemporaryFileURLsPayload{
		FileIDs: fileIDs,
	})
	if err != nil {
		return nil, err
	}

	var response readTemporaryFileURLsResponse
	if err := json.Unmarshal(raw, &response); err != nil {
		return nil, err
	}

	return response.URLs, nil
}

func requireAuthorization(scope Scope, authorizationRef string) (Authorization, error) {
	if scope.AuthorizationResolver == nil {
		actorUserID, triggerMessageID, err := requireActorTriggerScope(scope)
		if err != nil {
			return Authorization{}, err
		}
		return Authorization{
			ActorID:          actorUserID,
			ActorType:        "user",
			TriggerMessageID: triggerMessageID,
		}, nil
	}

	authorizationRef = strings.TrimSpace(authorizationRef)
	if authorizationRef == "" {
		return Authorization{}, fmt.Errorf("authorization_ref is required")
	}
	authorization, ok := scope.AuthorizationResolver.ResolveAuthorization(authorizationRef)
	if !ok {
		return Authorization{}, fmt.Errorf("authorization_ref is invalid")
	}
	authorization.ActorID = strings.TrimSpace(authorization.ActorID)
	authorization.ActorType = strings.ToLower(strings.TrimSpace(authorization.ActorType))
	authorization.TriggerMessageID = strings.TrimSpace(authorization.TriggerMessageID)
	if (authorization.ActorType != "user" && authorization.ActorType != "app") || authorization.ActorID == "" || authorization.TriggerMessageID == "" {
		return Authorization{}, fmt.Errorf("authorization_ref is invalid")
	}

	return authorization, nil
}

func requireUserAuthorization(scope Scope, authorizationRef string) (Authorization, error) {
	authorization, err := requireAuthorization(scope, authorizationRef)
	if err != nil {
		return Authorization{}, err
	}
	if authorization.ActorType != "user" {
		return Authorization{}, fmt.Errorf("authorization_ref does not authorize a user")
	}

	return authorization, nil
}

func requireActorTriggerScope(scope Scope) (string, string, error) {
	actorUserID := strings.TrimSpace(scope.CurrentUserID)
	triggerMessageID := strings.TrimSpace(scope.TriggerMessageID)
	if actorUserID == "" || triggerMessageID == "" {
		return "", "", fmt.Errorf("current user trigger scope is missing")
	}

	return actorUserID, triggerMessageID, nil
}

func normalizeToolMemberIDs(rawMemberIDs []string) ([]string, error) {
	memberIDs := make([]string, 0, len(rawMemberIDs))
	for _, rawMemberID := range rawMemberIDs {
		memberID := strings.TrimSpace(rawMemberID)
		if memberID == "" {
			continue
		}
		memberIDs = append(memberIDs, memberID)
	}
	if len(memberIDs) == 0 {
		return nil, fmt.Errorf("member_ids is required")
	}

	return memberIDs, nil
}

func requireScope(ctx context.Context) (Scope, error) {
	scope, ok := ctx.Value(scopeContextKey{}).(Scope)
	if !ok || scope.Requester == nil {
		return Scope{}, fmt.Errorf("builtin tool scope is not configured")
	}
	return scope, nil
}

func requestTool(ctx context.Context, requester AppRequester, method string, payload any) (mcpclient.ToolResult, error) {
	raw, err := requester.Request(ctx, method, payload)
	if err != nil {
		return mcpclient.ToolResult{}, err
	}

	return mcpclient.ToolResult{Content: strings.TrimSpace(string(raw))}, nil
}

func parseMessageInput(input json.RawMessage, requireContact bool) (scopedMessagePayload, error) {
	var parsed messageInput
	if err := json.Unmarshal(input, &parsed); err != nil {
		return scopedMessagePayload{}, fmt.Errorf("parse message input: %w", err)
	}
	if requireContact && strings.TrimSpace(parsed.ContactID) == "" {
		return scopedMessagePayload{}, fmt.Errorf("contact_id is required")
	}
	messageType := strings.TrimSpace(parsed.Type)
	switch messageType {
	case messageTypeText, messageTypeMarkdown, messageTypeImage, messageTypeFile, messageTypeCard, messageTypeChart:
	default:
		return scopedMessagePayload{}, fmt.Errorf("unsupported message type %q", parsed.Type)
	}
	if messageType == messageTypeFile {
		return parseFileMessageInput(parsed)
	}
	if messageType == messageTypeCard {
		return parseCardMessageInput(parsed)
	}
	if messageType == messageTypeChart {
		return parseChartMessageInput(parsed)
	}
	content := strings.TrimSpace(parsed.Content)
	if content == "" {
		return scopedMessagePayload{}, fmt.Errorf("content is required")
	}

	return scopedMessagePayload{
		AuthorizationRef: strings.TrimSpace(parsed.AuthorizationRef),
		Type:             messageType,
		Content:          content,
	}, nil
}

func parseChartMessageInput(parsed messageInput) (scopedMessagePayload, error) {
	chartType := strings.TrimSpace(parsed.ChartType)
	switch chartType {
	case "line", "bar", "pie", "radar":
	default:
		return scopedMessagePayload{}, fmt.Errorf("chart_type must be line, bar, pie, or radar")
	}
	title := strings.TrimSpace(parsed.Title)
	if title == "" {
		return scopedMessagePayload{}, fmt.Errorf("chart message title is required")
	}
	if len([]rune(title)) > maxChartMessageTitleRunes {
		return scopedMessagePayload{}, fmt.Errorf("chart message title must be at most 16 characters")
	}
	description := strings.TrimSpace(parsed.Description)
	if description == "" {
		return scopedMessagePayload{}, fmt.Errorf("chart message description is required")
	}
	if len([]rune(description)) > maxChartMessageDescriptionRunes {
		return scopedMessagePayload{}, fmt.Errorf("chart message description must be at most 128 characters")
	}
	if len(parsed.Data) == 0 || string(parsed.Data) == "null" {
		return scopedMessagePayload{}, fmt.Errorf("chart message data is required")
	}
	var data map[string]json.RawMessage
	if err := json.Unmarshal(parsed.Data, &data); err != nil || len(data) == 0 {
		return scopedMessagePayload{}, fmt.Errorf("chart message data must be an object")
	}

	message := scopedMessagePayload{
		AuthorizationRef: strings.TrimSpace(parsed.AuthorizationRef),
		ChartType:        chartType,
		Data:             parsed.Data,
		Description:      description,
		Title:            title,
		Type:             messageTypeChart,
	}
	encoded, err := json.Marshal(message)
	if err != nil {
		return scopedMessagePayload{}, fmt.Errorf("marshal chart message: %w", err)
	}
	if len(encoded) > maxChartMessageBodyBytes {
		return scopedMessagePayload{}, fmt.Errorf("chart message must be at most 64 KiB")
	}
	return message, nil
}

func parseCardMessageInput(parsed messageInput) (scopedMessagePayload, error) {
	title := strings.TrimSpace(parsed.Title)
	if title == "" {
		return scopedMessagePayload{}, fmt.Errorf("card message title is required")
	}
	description := strings.TrimSpace(parsed.Description)
	if description == "" {
		return scopedMessagePayload{}, fmt.Errorf("card message description is required")
	}
	url := strings.TrimSpace(parsed.URL)
	if url == "" {
		return scopedMessagePayload{}, fmt.Errorf("card message url is required")
	}

	return scopedMessagePayload{
		AuthorizationRef: strings.TrimSpace(parsed.AuthorizationRef),
		Description:      description,
		Title:            title,
		Type:             messageTypeCard,
		URL:              url,
	}, nil
}

func parseFileMessageInput(parsed messageInput) (scopedMessagePayload, error) {
	name, err := normalizeSpecifiedMessageFileName(parsed.Name)
	if err != nil {
		return scopedMessagePayload{}, err
	}
	url := strings.TrimSpace(parsed.URL)
	hasURL := url != ""
	hasContent := parsed.Content != ""
	switch {
	case hasURL && hasContent:
		return scopedMessagePayload{}, fmt.Errorf("file url and content are mutually exclusive")
	case hasURL:
		return scopedMessagePayload{
			AuthorizationRef: strings.TrimSpace(parsed.AuthorizationRef),
			Type:             messageTypeFile,
			Name:             name,
			URL:              url,
		}, nil
	case hasContent:
		return scopedMessagePayload{
			AuthorizationRef: strings.TrimSpace(parsed.AuthorizationRef),
			Type:             messageTypeFile,
			Name:             name,
			Content:          parsed.Content,
		}, nil
	default:
		return scopedMessagePayload{}, fmt.Errorf("file url or content is required")
	}
}

func normalizeSpecifiedMessageFileName(rawName string) (string, error) {
	name := strings.TrimSpace(rawName)
	if name == "" || name == "." || name == "/" {
		return "", fmt.Errorf("file name is required")
	}
	if strings.ContainsAny(name, `/\`) {
		return "", fmt.Errorf("file name must not contain a path")
	}
	if len([]rune(name)) > 255 {
		return "", fmt.Errorf("file name must be at most 255 characters")
	}

	return name, nil
}

func parseSendAsUserInput(input json.RawMessage) (scopedMessagePayload, sendAsUserTarget, string, error) {
	message, err := parseMessageInput(input, false)
	if err != nil {
		return scopedMessagePayload{}, sendAsUserTarget{}, "", err
	}

	var parsed messageInput
	if err := json.Unmarshal(input, &parsed); err != nil {
		return scopedMessagePayload{}, sendAsUserTarget{}, "", fmt.Errorf("parse conversations send input: %w", err)
	}
	targetType := strings.TrimSpace(parsed.TargetType)
	contactID := strings.TrimSpace(parsed.ContactID)
	conversationID := strings.TrimSpace(parsed.ConversationID)
	if targetType == "" && contactID != "" {
		targetType = "user"
	}

	switch targetType {
	case "user":
		if contactID == "" {
			return scopedMessagePayload{}, sendAsUserTarget{}, "", fmt.Errorf("contact_id is required for user target")
		}
		return message, sendAsUserTarget{
			Type:   "user",
			UserID: contactID,
		}, contactID, nil
	case "group":
		if conversationID == "" {
			return scopedMessagePayload{}, sendAsUserTarget{}, "", fmt.Errorf("conversation_id is required for group target")
		}
		return message, sendAsUserTarget{
			ConversationID: conversationID,
			Type:           "group",
		}, "", nil
	default:
		return scopedMessagePayload{}, sendAsUserTarget{}, "", fmt.Errorf("unsupported target_type %q", parsed.TargetType)
	}
}

func parseEntityCardInput(input json.RawMessage) (scopedMessagePayload, entityCardInput, error) {
	var parsed entityCardInput
	if err := json.Unmarshal(input, &parsed); err != nil {
		return scopedMessagePayload{}, entityCardInput{}, fmt.Errorf("parse entity card input: %w", err)
	}
	entityType := strings.ToLower(strings.TrimSpace(parsed.EntityType))
	switch entityType {
	case "user", "app", "group", "project", "task":
	default:
		return scopedMessagePayload{}, entityCardInput{}, fmt.Errorf("unsupported entity_type %q", parsed.EntityType)
	}
	entityID := strings.TrimSpace(parsed.EntityID)
	if entityID == "" {
		return scopedMessagePayload{}, entityCardInput{}, fmt.Errorf("entity_id is required")
	}
	parsed.AuthorizationRef = strings.TrimSpace(parsed.AuthorizationRef)
	parsed.EntityID = entityID
	parsed.EntityType = entityType

	return scopedMessagePayload{
		EntityID:   entityID,
		EntityType: entityType,
		Type:       messageTypeEntityCard,
	}, parsed, nil
}

func parseEntityCardTarget(parsed entityCardInput) (sendAsUserTarget, string, error) {
	targetType := strings.TrimSpace(parsed.TargetType)
	contactID := strings.TrimSpace(parsed.ContactID)
	conversationID := strings.TrimSpace(parsed.ConversationID)
	if targetType == "" && contactID != "" {
		targetType = "user"
	}

	switch targetType {
	case "user":
		if contactID == "" {
			return sendAsUserTarget{}, "", fmt.Errorf("contact_id is required for user target")
		}
		return sendAsUserTarget{Type: "user", UserID: contactID}, contactID, nil
	case "group":
		if conversationID == "" {
			return sendAsUserTarget{}, "", fmt.Errorf("conversation_id is required for group target")
		}
		return sendAsUserTarget{ConversationID: conversationID, Type: "group"}, "", nil
	default:
		return sendAsUserTarget{}, "", fmt.Errorf("unsupported target_type %q", parsed.TargetType)
	}
}

func readFileURLsInputSchema() map[string]any {
	return map[string]any{
		"type":     "object",
		"required": []string{"file_ids"},
		"properties": map[string]any{
			"file_ids": map[string]any{
				"type":        "array",
				"minItems":    1,
				"uniqueItems": true,
				"description": "当前消息或历史消息里的 file_id 列表。只有需要查看图片或附件内容时传入；可一次传多个。",
				"items": map[string]any{
					"type":      "string",
					"minLength": 1,
				},
			},
		},
		"additionalProperties": false,
	}
}

func sleepDuration(input json.RawMessage) (time.Duration, float64, error) {
	var parsed sleepInput
	if len(input) > 0 {
		if err := json.Unmarshal(input, &parsed); err != nil {
			seconds := float64(minSleepSeconds)
			return time.Duration(seconds * float64(defaultSleepUnit)), seconds, nil
		}
	}

	seconds := clampSeconds(parsed.Seconds)
	duration := time.Duration(seconds * float64(defaultSleepUnit))
	return duration, seconds, nil
}

func clampSeconds(seconds float64) float64 {
	if math.IsNaN(seconds) || seconds < minSleepSeconds {
		return minSleepSeconds
	}
	if seconds > maxSleepSeconds {
		return maxSleepSeconds
	}

	return seconds
}

func realSleep(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func formatSeconds(seconds float64) string {
	if seconds == math.Trunc(seconds) {
		if seconds == 1 {
			return "1 second"
		}
		return fmt.Sprintf("%.0f seconds", seconds)
	}

	return fmt.Sprintf("%g seconds", seconds)
}
