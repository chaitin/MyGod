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
	sourceName                   = "builtin"
	sleepToolName                = "sleep"
	contactsToolName             = "contacts"
	myGroupsToolName             = "my_groups"
	replyToolName                = "reply"
	sendAsUserToolName           = "send_as_user"
	createGroupToolName          = "create_group"
	addGroupMembersToolName      = "add_group_members"
	methodContactsUsersList      = "contacts.users.list"
	methodGroupConversationsList = "group_conversations.list"
	methodCreateGroup            = "group_conversations.create"
	methodAddGroupMembers        = "group_conversations.members.add"
	methodMessageSend            = "message.send"
	methodMessageSendAsUser      = "message.send_as_user"
	minSleepSeconds              = 1
	maxSleepSeconds              = 60
	defaultSleepUnit             = time.Second
	messageTypeText              = "text"
	messageTypeMarkdown          = "markdown"
	messageTypeImage             = "image"
	messageTypeFile              = "file"
)

type sleepFunc func(context.Context, time.Duration) error
type scopeContextKey struct{}

type AppRequester interface {
	Request(context.Context, string, any) (json.RawMessage, error)
}

type Scope struct {
	ConversationID   string
	ConversationType string
	CurrentUserID    string
	Requester        AppRequester
	TriggerMessageID string
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

type myGroupsInput struct {
	Keyword string `json:"keyword"`
}

type messageInput struct {
	ContactID      string `json:"contact_id"`
	Content        string `json:"content"`
	ConversationID string `json:"conversation_id"`
	TargetType     string `json:"target_type"`
	Type           string `json:"type"`
}

type createGroupInput struct {
	MemberIDs []string `json:"member_ids"`
	Name      string   `json:"name"`
}

type addGroupMembersInput struct {
	ConversationID string   `json:"conversation_id"`
	MemberIDs      []string `json:"member_ids"`
}

type scopedMessagePayload struct {
	Content string `json:"content"`
	Type    string `json:"type"`
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
	ActorUserID      string               `json:"actor_user_id"`
	Message          scopedMessagePayload `json:"message"`
	Target           sendAsUserTarget     `json:"target"`
	TargetUserID     string               `json:"target_user_id,omitempty"`
	TriggerMessageID string               `json:"trigger_message_id"`
}

type sendAsUserTarget struct {
	ConversationID string `json:"conversation_id,omitempty"`
	Type           string `json:"type"`
	UserID         string `json:"user_id,omitempty"`
}

type myGroupsPayload struct {
	ActorUserID      string `json:"actor_user_id"`
	Keyword          string `json:"keyword"`
	TriggerMessageID string `json:"trigger_message_id"`
}

type createGroupPayload struct {
	ActorUserID      string   `json:"actor_user_id"`
	MemberIDs        []string `json:"member_ids"`
	Name             string   `json:"name"`
	TriggerMessageID string   `json:"trigger_message_id"`
}

type addGroupMembersPayload struct {
	ActorUserID      string   `json:"actor_user_id"`
	ConversationID   string   `json:"conversation_id"`
	MemberIDs        []string `json:"member_ids"`
	TriggerMessageID string   `json:"trigger_message_id"`
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
			Description: "等待指定秒数，常用于等待异步任务完成或协调多个工具调用。seconds 小于 1 时按 1 秒处理，大于 60 时按 60 秒处理。",
			InputSchema: map[string]any{
				"type":     "object",
				"required": []string{"seconds"},
				"properties": map[string]any{
					"seconds": map[string]any{
						"type":        "number",
						"description": "等待秒数。最小 1，最大 60，超出范围会被自动截断。",
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
			Name:        myGroupsToolName,
			Description: "查询当前触发用户已加入的已有群聊，只返回 active 群聊。用户要求把消息发到某个群、给已有群加人或需要确认群聊 ID 时使用；目标群聊不明确、查到多个相似群或没有查到时先追问，不要猜 conversation_id。不要用它创建群聊。",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"keyword": map[string]any{
						"type":        "string",
						"description": "可选搜索词，按群聊名称搜索；为空返回当前用户已加入的最近群聊。",
					},
				},
			},
		},
		{
			Name:        replyToolName,
			Description: "回复当前触发 assistant 的会话。只能发回当前会话，不能指定联系人；需要回复当前用户或当前群时使用。支持 text、markdown、image、file；image/file 的 content 必须是可下载 URL。",
			InputSchema: messageInputSchema(false),
		},
		{
			Name:        sendAsUserToolName,
			Description: "以当前触发用户的身份发送到私聊或已有群聊。只有用户明确要求“替我发给某人/以我的身份发给某人”或“替我发到某个已有群聊”时才能使用；target_type=user 时 contact_id 必须来自 contacts，target_type=group 时 conversation_id 必须来自 my_groups。目标群聊不明确、查到多个相似群或没有查到时先追问，不要猜 conversation_id。不要用它回复当前会话；回复当前会话必须用 reply。不要用它创建群聊或拉人进群。",
			InputSchema: messageInputSchema(true),
		},
		{
			Name:        createGroupToolName,
			Description: "以当前触发用户的身份创建新群聊。只在用户明确要求创建新群聊、建群、拉一个新群时使用。不要用它发送消息、不要用它回复当前会话、不要用它总结或查询已有群聊、不要用它给已有群聊加人；已有群聊加人应使用 add_group_members。成员 ID 必须来自 contacts 工具返回的用户联系人；联系人身份不明确、群名不明确、成员列表不明确时先追问。当前触发用户会自动成为群主，不要把当前用户放进 member_ids。",
			InputSchema: createGroupInputSchema(),
		},
		{
			Name:        addGroupMembersToolName,
			Description: "以当前触发用户的身份把成员加入已有群聊。只在用户明确要求把人加入已有群聊、拉人进群、邀请成员进群时使用。不要用它创建群聊、不要用它发送消息、不要用它回复当前会话；创建新群聊应使用 create_group。成员 ID 必须来自 contacts 工具返回的用户联系人。当前会话是目标群聊时可以省略 conversation_id；如果当前会话不是目标群聊，或目标群聊不明确、成员身份不明确时先追问。",
			InputSchema: addGroupMembersInputSchema(),
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
	case myGroupsToolName:
		return callMyGroups(ctx, input)
	case replyToolName:
		return callReply(ctx, input)
	case sendAsUserToolName:
		return callSendAsUser(ctx, input)
	case createGroupToolName:
		return callCreateGroup(ctx, input)
	case addGroupMembersToolName:
		return callAddGroupMembers(ctx, input)
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

func callMyGroups(ctx context.Context, input json.RawMessage) (mcpclient.ToolResult, error) {
	scope, err := requireScope(ctx)
	if err != nil {
		return mcpclient.ToolResult{}, err
	}
	actorUserID, triggerMessageID, err := requireActorTriggerScope(scope)
	if err != nil {
		return mcpclient.ToolResult{}, err
	}

	var parsed myGroupsInput
	if len(input) > 0 {
		if err := json.Unmarshal(input, &parsed); err != nil {
			return mcpclient.ToolResult{}, fmt.Errorf("parse my_groups input: %w", err)
		}
	}

	return requestTool(ctx, scope.Requester, methodGroupConversationsList, myGroupsPayload{
		ActorUserID:      actorUserID,
		Keyword:          strings.TrimSpace(parsed.Keyword),
		TriggerMessageID: triggerMessageID,
	})
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
	if strings.TrimSpace(scope.CurrentUserID) == "" || strings.TrimSpace(scope.TriggerMessageID) == "" {
		return mcpclient.ToolResult{}, fmt.Errorf("current user trigger scope is missing")
	}
	parsed, target, targetUserID, err := parseSendAsUserInput(input)
	if err != nil {
		return mcpclient.ToolResult{}, err
	}

	return requestTool(ctx, scope.Requester, methodMessageSendAsUser, sendAsUserPayload{
		ActorUserID:      strings.TrimSpace(scope.CurrentUserID),
		Target:           target,
		TargetUserID:     targetUserID,
		TriggerMessageID: strings.TrimSpace(scope.TriggerMessageID),
		Message:          parsed,
	})
}

func callCreateGroup(ctx context.Context, input json.RawMessage) (mcpclient.ToolResult, error) {
	scope, err := requireScope(ctx)
	if err != nil {
		return mcpclient.ToolResult{}, err
	}
	actorUserID, triggerMessageID, err := requireActorTriggerScope(scope)
	if err != nil {
		return mcpclient.ToolResult{}, err
	}

	var parsed createGroupInput
	if err := json.Unmarshal(input, &parsed); err != nil {
		return mcpclient.ToolResult{}, fmt.Errorf("parse create_group input: %w", err)
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
		ActorUserID:      actorUserID,
		TriggerMessageID: triggerMessageID,
		Name:             name,
		MemberIDs:        memberIDs,
	})
}

func callAddGroupMembers(ctx context.Context, input json.RawMessage) (mcpclient.ToolResult, error) {
	scope, err := requireScope(ctx)
	if err != nil {
		return mcpclient.ToolResult{}, err
	}
	actorUserID, triggerMessageID, err := requireActorTriggerScope(scope)
	if err != nil {
		return mcpclient.ToolResult{}, err
	}

	var parsed addGroupMembersInput
	if err := json.Unmarshal(input, &parsed); err != nil {
		return mcpclient.ToolResult{}, fmt.Errorf("parse add_group_members input: %w", err)
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
		ActorUserID:      actorUserID,
		ConversationID:   conversationID,
		TriggerMessageID: triggerMessageID,
		MemberIDs:        memberIDs,
	})
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
	content := strings.TrimSpace(parsed.Content)
	if content == "" {
		return scopedMessagePayload{}, fmt.Errorf("content is required")
	}

	return scopedMessagePayload{
		Type:    messageType,
		Content: content,
	}, nil
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
	required := []string{"type", "content"}
	properties := map[string]any{
		"type": map[string]any{
			"type":        "string",
			"enum":        []string{messageTypeText, messageTypeMarkdown, messageTypeImage, messageTypeFile},
			"description": "消息类型。text/markdown 的 content 是文本；image/file 的 content 是可下载 URL。",
		},
		"content": map[string]any{
			"type":        "string",
			"description": "text/markdown 时为消息内容；image/file 时为可下载 URL。",
		},
	}
	if requireContact {
		required = append([]string{"target_type"}, required...)
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
			"description": "target_type=group 时必填，目标已有群聊 ID，必须来自 my_groups 工具返回的群聊。",
		}
	}

	return map[string]any{
		"type":       "object",
		"required":   required,
		"properties": properties,
	}
}

func createGroupInputSchema() map[string]any {
	return map[string]any{
		"type":     "object",
		"required": []string{"name", "member_ids"},
		"properties": map[string]any{
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
		"required": []string{"member_ids"},
		"properties": map[string]any{
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

func sleepDuration(input json.RawMessage) (time.Duration, float64, error) {
	var parsed sleepInput
	if len(input) > 0 {
		if err := json.Unmarshal(input, &parsed); err != nil {
			return 0, 0, fmt.Errorf("parse sleep input: %w", err)
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
