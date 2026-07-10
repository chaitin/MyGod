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
	sourceName                    = "builtin"
	sleepToolName                 = "sleep"
	contactsToolName              = "contacts"
	recentConversationsToolName   = "recent_conversations"
	readHistoryToolName           = "read_history"
	replyToolName                 = "reply"
	sendAsUserToolName            = "send_as_user"
	createGroupToolName           = "create_group"
	addGroupMembersToolName       = "add_group_members"
	readFileURLsToolName          = "read_file_urls"
	methodContactsUsersList       = "contacts.users.list"
	methodConversationsList       = "conversations.list"
	methodConversationHistoryRead = "conversation.history.read"
	methodGroupConversationsList  = "group_conversations.list"
	methodCreateGroup             = "group_conversations.create"
	methodAddGroupMembers         = "group_conversations.members.add"
	methodMessageSend             = "message.send"
	methodMessageSendAsUser       = "message.send_as_user"
	methodTemporaryFilesReadURLs  = "temporary_files.read_urls"
	minSleepSeconds               = 5
	maxSleepSeconds               = 30
	defaultSleepUnit              = time.Second
	messageTypeText               = "text"
	messageTypeMarkdown           = "markdown"
	messageTypeImage              = "image"
	messageTypeFile               = "file"
)

type sleepFunc func(context.Context, time.Duration) error
type scopeContextKey struct{}

type AppRequester interface {
	Request(context.Context, string, any) (json.RawMessage, error)
}

type Authorization struct {
	ActorUserID      string
	TriggerMessageID string
}

type AuthorizationResolver interface {
	ResolveAuthorization(ref string) (Authorization, bool)
}

type AuthorizationResolverFunc func(ref string) (Authorization, bool)

func (f AuthorizationResolverFunc) ResolveAuthorization(ref string) (Authorization, bool) {
	return f(ref)
}

type Scope struct {
	AuthorizationResolver AuthorizationResolver
	ConversationID        string
	ConversationType      string
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
	Keyword string `json:"keyword"`
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
	AuthorizationRef string `json:"authorization_ref"`
	ContactID        string `json:"contact_id"`
	Content          string `json:"content"`
	ConversationID   string `json:"conversation_id"`
	Name             string `json:"name"`
	TargetType       string `json:"target_type"`
	Type             string `json:"type"`
	URL              string `json:"url"`
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
	AuthorizationRef string `json:"-"`
	Content          string `json:"content,omitempty"`
	Name             string `json:"name,omitempty"`
	Type             string `json:"type"`
	URL              string `json:"url,omitempty"`
}

type sendMessageTargetPayload struct {
	ConversationID string `json:"conversation_id,omitempty"`
	Type           string `json:"type"`
}

type sendMessagePayload struct {
	Message scopedMessagePayload     `json:"message"`
	Target  sendMessageTargetPayload `json:"target"`
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
	return []mcpclient.Tool{
		{
			Name:        sleepToolName,
			Description: "等待 5 到 30 秒，常用于等待异步任务完成、外部工具结果同步、文件处理、后台状态变化，或配合查询工具轮询。seconds 小于 5、缺失或不合法按 5 秒处理，大于 30 时按 30 秒处理。不用于普通回复、拖延回复或无目的等待。",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"seconds": map[string]any{
						"type":        "number",
						"description": "等待秒数。最小 5，最大 30；缺失或不合法按 5 秒处理，超出范围会被自动截断。",
						"minimum":     minSleepSeconds,
						"maximum":     maxSleepSeconds,
					},
				},
			},
		},
		{
			Name:        contactsToolName,
			Description: "查询可发送消息的联系人，只返回 active 用户，不包含 app 和群。需要找人或确认联系人 ID 时使用。",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"keyword": map[string]any{
						"type":        "string",
						"description": "可选搜索词，按姓名、昵称、邮箱、手机号搜索；为空返回全部联系人。",
					},
				},
			},
		},
		{
			Name:        recentConversationsToolName,
			Description: "查询授权用户最近使用的会话，返回私聊、群聊、应用会话。需要 authorization_ref，且只能使用当前上下文 authorization_candidates 提供的 ref。可用于确认会话 ID、会话类型、成员数量和最近活动时间；返回字段包括 type、conversation_id、name、member_count、last_active_at。keyword 按会话名称，或私聊对象的姓名、昵称搜索，不查消息内容；limit 默认 20，最大 100。",
			InputSchema: map[string]any{
				"type":     "object",
				"required": []string{"authorization_ref"},
				"properties": map[string]any{
					"authorization_ref": authorizationRefSchema(),
					"keyword": map[string]any{
						"type":        "string",
						"description": "可选搜索词，按会话名称，或私聊对象的姓名、昵称搜索；不传或为空返回所有最近会话。",
					},
					"limit": map[string]any{
						"type":        "integer",
						"description": "可选返回数量。不传默认 20，最大 100。",
						"minimum":     1,
						"maximum":     100,
					},
				},
			},
		},
		{
			Name:        readHistoryToolName,
			Description: "读取授权用户有权限访问的聊天记录。需要 authorization_ref，且只能使用当前上下文 authorization_candidates 提供的 ref。conversation_id、user_id、app_id 三选一：conversation_id 读取指定会话，user_id 读取和该用户的私聊，app_id 读取和该应用的会话。before_seq 不传表示读取最新消息，传入时读取 seq 小于 before_seq 的更早消息；limit 默认 20，最大 100。图片和附件只返回 file_id，需要真实 URL 时再调用 read_file_urls。",
			InputSchema: readHistoryInputSchema(),
		},
		{
			Name:        replyToolName,
			Description: "回复当前触发 assistant 的会话。只能发回当前会话，不能指定联系人；需要回复当前用户或当前群时使用。支持 text、markdown、image、file。image 的 content 必须是可下载 URL。file 必须提供 name，不要猜文件名；已有可下载文件用 url，assistant 生成的小文件用 content，url/content 只能二选一；没有明确文件名时先追问。",
			InputSchema: messageInputSchema(false),
		},
		{
			Name:        sendAsUserToolName,
			Description: "以授权用户的身份发送到私聊或已有群聊。需要 authorization_ref，且只能使用当前上下文 authorization_candidates 提供的 ref；只有对应授权消息明确要求“替我发给某人/以我的身份发给某人”或“替我发到某个已有群聊”时才能使用。target_type=user 时 contact_id 必须来自 contacts，target_type=group 时 conversation_id 必须来自 recent_conversations。目标群聊不明确、查到多个相似群或没有查到时先追问，不要猜 conversation_id。不要用它回复当前会话；回复当前会话必须用 reply。不要用它创建群聊或拉人进群。发送 file 时必须提供 name，不要猜文件名；已有可下载文件用 url，assistant 生成的小文件用 content；没有明确文件名时先追问。",
			InputSchema: messageInputSchema(true),
		},
		{
			Name:        createGroupToolName,
			Description: "以授权用户的身份创建新群聊。需要 authorization_ref，且只能使用当前上下文 authorization_candidates 提供的 ref；只在对应授权消息明确要求创建新群聊、建群、拉一个新群时使用。不要用它发送消息、不要用它回复当前会话、不要用它总结或查询已有群聊、不要用它给已有群聊加人；已有群聊加人应使用 add_group_members。成员 ID 必须来自 contacts 工具返回的用户联系人；联系人身份不明确、群名不明确、成员列表不明确时先追问。授权用户会自动成为群主，不要把授权用户放进 member_ids。",
			InputSchema: createGroupInputSchema(),
		},
		{
			Name:        addGroupMembersToolName,
			Description: "以授权用户的身份把成员加入已有群聊。需要 authorization_ref，且只能使用当前上下文 authorization_candidates 提供的 ref；只在对应授权消息明确要求把人加入已有群聊、拉人进群、邀请成员进群时使用。不要用它创建群聊、不要用它发送消息、不要用它回复当前会话；创建新群聊应使用 create_group。成员 ID 必须来自 contacts 工具返回的用户联系人。当前会话是目标群聊时可以省略 conversation_id；如果当前会话不是目标群聊，或目标群聊不明确、成员身份不明确时先追问。",
			InputSchema: addGroupMembersInputSchema(),
		},
		{
			Name:        readFileURLsToolName,
			Description: "按需把当前消息或历史消息里的 file_id 换成临时可访问 URL。只需要 file_id，不需要会话 ID；只有确实需要读取图片或附件内容时使用。历史消息默认只提供 file_id。支持一次传多个 file_id，部分失败时会在 errors 返回，不影响已成功获取的 URL。",
			InputSchema: readFileURLsInputSchema(),
		},
	}, nil
}

func (s *Source) CallTool(ctx context.Context, name string, input json.RawMessage) (mcpclient.ToolResult, error) {
	if err := ctx.Err(); err != nil {
		return mcpclient.ToolResult{}, err
	}

	switch name {
	case sleepToolName:
		return s.callSleep(ctx, input)
	case contactsToolName:
		return callContacts(ctx, input)
	case recentConversationsToolName:
		return callRecentConversations(ctx, input)
	case readHistoryToolName:
		return callReadHistory(ctx, input)
	case replyToolName:
		return callReply(ctx, input)
	case sendAsUserToolName:
		return callSendAsUser(ctx, input)
	case createGroupToolName:
		return callCreateGroup(ctx, input)
	case addGroupMembersToolName:
		return callAddGroupMembers(ctx, input)
	case readFileURLsToolName:
		return callReadFileURLs(ctx, input)
	default:
		return mcpclient.ToolResult{}, fmt.Errorf("unknown builtin tool %q", name)
	}
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

func callContacts(ctx context.Context, input json.RawMessage) (mcpclient.ToolResult, error) {
	scope, err := requireScope(ctx)
	if err != nil {
		return mcpclient.ToolResult{}, err
	}

	var parsed contactsInput
	if len(input) > 0 {
		if err := json.Unmarshal(input, &parsed); err != nil {
			return mcpclient.ToolResult{}, fmt.Errorf("parse contacts input: %w", err)
		}
	}
	return requestTool(ctx, scope.Requester, methodContactsUsersList, contactsInput{
		Keyword: strings.TrimSpace(parsed.Keyword),
	})
}

func callRecentConversations(ctx context.Context, input json.RawMessage) (mcpclient.ToolResult, error) {
	scope, err := requireScope(ctx)
	if err != nil {
		return mcpclient.ToolResult{}, err
	}

	var parsed recentConversationsInput
	if len(input) > 0 {
		if err := json.Unmarshal(input, &parsed); err != nil {
			return mcpclient.ToolResult{}, fmt.Errorf("parse recent_conversations input: %w", err)
		}
	}
	auth, err := requireAuthorization(scope, parsed.AuthorizationRef)
	if err != nil {
		return mcpclient.ToolResult{}, err
	}

	return requestTool(ctx, scope.Requester, methodConversationsList, recentConversationsPayload{
		ActorUserID:                 auth.ActorUserID,
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
			return mcpclient.ToolResult{}, fmt.Errorf("parse read_history input: %w", err)
		}
	}
	auth, err := requireAuthorization(scope, parsed.AuthorizationRef)
	if err != nil {
		return mcpclient.ToolResult{}, err
	}
	payload := readHistoryPayload{
		ActorUserID:                 auth.ActorUserID,
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
	result.Final = true

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
	auth, err := requireAuthorization(scope, parsed.AuthorizationRef)
	if err != nil {
		return mcpclient.ToolResult{}, err
	}

	return requestTool(ctx, scope.Requester, methodMessageSendAsUser, sendAsUserPayload{
		ActorUserID:                 auth.ActorUserID,
		AuthorizationConversationID: strings.TrimSpace(scope.ConversationID),
		Target:                      target,
		TargetUserID:                targetUserID,
		TriggerMessageID:            auth.TriggerMessageID,
		Message:                     parsed,
	})
}

func callCreateGroup(ctx context.Context, input json.RawMessage) (mcpclient.ToolResult, error) {
	scope, err := requireScope(ctx)
	if err != nil {
		return mcpclient.ToolResult{}, err
	}

	var parsed createGroupInput
	if err := json.Unmarshal(input, &parsed); err != nil {
		return mcpclient.ToolResult{}, fmt.Errorf("parse create_group input: %w", err)
	}
	auth, err := requireAuthorization(scope, parsed.AuthorizationRef)
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
		ActorUserID:                 auth.ActorUserID,
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
		return mcpclient.ToolResult{}, fmt.Errorf("parse add_group_members input: %w", err)
	}
	auth, err := requireAuthorization(scope, parsed.AuthorizationRef)
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
		ActorUserID:                 auth.ActorUserID,
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
		return nil, fmt.Errorf("parse read_file_urls input: %w", err)
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
			ActorUserID:      actorUserID,
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
	authorization.ActorUserID = strings.TrimSpace(authorization.ActorUserID)
	authorization.TriggerMessageID = strings.TrimSpace(authorization.TriggerMessageID)
	if authorization.ActorUserID == "" || authorization.TriggerMessageID == "" {
		return Authorization{}, fmt.Errorf("authorization_ref is invalid")
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
	case messageTypeText, messageTypeMarkdown, messageTypeImage, messageTypeFile:
	default:
		return scopedMessagePayload{}, fmt.Errorf("unsupported message type %q", parsed.Type)
	}
	if messageType == messageTypeFile {
		return parseFileMessageInput(parsed)
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
		return scopedMessagePayload{}, sendAsUserTarget{}, "", fmt.Errorf("parse send_as_user input: %w", err)
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

func messageInputSchema(requireContact bool) map[string]any {
	required := []string{"type"}
	properties := map[string]any{
		"type": map[string]any{
			"type":        "string",
			"enum":        []string{messageTypeText, messageTypeMarkdown, messageTypeImage, messageTypeFile},
			"description": "消息类型。text/markdown 的 content 是文本；image 的 content 是可下载 URL；file 必须显式提供 name，并在 url 或 content 中二选一。",
		},
		"content": map[string]any{
			"type":        "string",
			"description": "text/markdown 时为消息内容；image 时为可下载 URL；file 且没有 url 时为 assistant 生成的小文件内容，受 64KiB 内联文件内容上限约束。",
		},
		"name": map[string]any{
			"type":        "string",
			"description": "type=file 时必填，必须是用户明确指定的文件名，不能包含路径；没有明确文件名时先追问，不要猜文件名。",
		},
		"url": map[string]any{
			"type":        "string",
			"description": "type=file 且已有可下载文件时使用；与 content 只能二选一。",
		},
	}
	if requireContact {
		required = append([]string{"authorization_ref", "target_type"}, required...)
		properties["authorization_ref"] = authorizationRefSchema()
		properties["target_type"] = map[string]any{
			"type":        "string",
			"enum":        []string{"user", "group"},
			"description": "发送目标类型。user 表示私聊联系人；group 表示已有群聊。",
		}
		properties["contact_id"] = map[string]any{
			"type":        "string",
			"description": "target_type=user 时必填，目标联系人用户 ID，必须来自 contacts 工具返回的用户联系人。",
		}
		properties["conversation_id"] = map[string]any{
			"type":        "string",
			"description": "target_type=group 时必填，目标已有群聊 ID，必须来自 recent_conversations 工具返回的群聊。",
		}
	}

	return map[string]any{
		"type":       "object",
		"required":   required,
		"properties": properties,
	}
}

func readHistoryInputSchema() map[string]any {
	return map[string]any{
		"type":     "object",
		"required": []string{"authorization_ref"},
		"properties": map[string]any{
			"authorization_ref": authorizationRefSchema(),
			"conversation_id": map[string]any{
				"type":        "string",
				"description": "可选，会话 ID。与 user_id、app_id 三选一；用于读取指定会话的历史。",
			},
			"user_id": map[string]any{
				"type":        "string",
				"description": "可选，联系人用户 ID。与 conversation_id、app_id 三选一；用于读取当前触发用户和该用户的私聊历史。",
			},
			"app_id": map[string]any{
				"type":        "string",
				"description": "可选，应用 ID。与 conversation_id、user_id 三选一；用于读取当前触发用户和该应用的会话历史。",
			},
			"before_seq": map[string]any{
				"type":        "integer",
				"description": "可选。读取 seq 小于 before_seq 的更早消息；不传表示读取最新消息。",
				"minimum":     1,
			},
			"limit": map[string]any{
				"type":        "integer",
				"description": "可选返回数量。不传默认 20，最大 100。",
				"minimum":     1,
				"maximum":     100,
			},
		},
	}
}

func createGroupInputSchema() map[string]any {
	return map[string]any{
		"type":     "object",
		"required": []string{"authorization_ref", "name", "member_ids"},
		"properties": map[string]any{
			"authorization_ref": authorizationRefSchema(),
			"name": map[string]any{
				"type":        "string",
				"description": "新群聊名称。只有用户明确给出或能从请求中明确推断群名时填写；不明确时先追问。",
			},
			"member_ids": map[string]any{
				"type":        "array",
				"description": "新群聊成员用户 ID 列表，必须来自 contacts 工具返回的联系人。不要包含当前触发用户；联系人重名、身份不明确或没有查到时先追问，不要猜 ID。",
				"items": map[string]any{
					"type": "string",
				},
			},
		},
	}
}

func addGroupMembersInputSchema() map[string]any {
	return map[string]any{
		"type":     "object",
		"required": []string{"authorization_ref", "member_ids"},
		"properties": map[string]any{
			"authorization_ref": authorizationRefSchema(),
			"conversation_id": map[string]any{
				"type":        "string",
				"description": "目标已有群聊 ID。当前会话是目标群聊时可省略；当前会话不是目标群聊或目标群聊不明确时先追问，不要猜测。",
			},
			"member_ids": map[string]any{
				"type":        "array",
				"description": "要拉入已有群聊的用户 ID 列表，必须来自 contacts 工具返回的联系人。联系人重名、身份不明确或没有查到时先追问，不要猜 ID。",
				"items": map[string]any{
					"type": "string",
				},
			},
		},
	}
}

func authorizationRefSchema() map[string]any {
	return map[string]any{
		"type":        "string",
		"description": "必填，授权来源，只能填写当前上下文 authorization_candidates 中提供的 authorization_ref，不能编造或填写真实消息 ID。",
	}
}

func readFileURLsInputSchema() map[string]any {
	return map[string]any{
		"type":     "object",
		"required": []string{"file_ids"},
		"properties": map[string]any{
			"file_ids": map[string]any{
				"type":        "array",
				"description": "当前消息或历史消息里的 file_id 列表。只有需要查看图片或附件内容时传入；可一次传多个。",
				"items": map[string]any{
					"type": "string",
				},
			},
		},
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
