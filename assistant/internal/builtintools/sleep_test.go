package builtintools

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

type requestCall struct {
	method  string
	payload json.RawMessage
}

type fakeRequester struct {
	calls []requestCall
}

func (r *fakeRequester) Request(ctx context.Context, method string, payload any) (json.RawMessage, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	r.calls = append(r.calls, requestCall{
		method:  method,
		payload: raw,
	})

	return json.RawMessage(`{"ok":true}`), nil
}

func TestSleepToolClampsDuration(t *testing.T) {
	var durations []time.Duration
	source := newSourceWithSleeper(func(ctx context.Context, duration time.Duration) error {
		durations = append(durations, duration)
		return nil
	})

	if _, err := source.CallTool(context.Background(), "sleep", json.RawMessage(`{"seconds":0}`)); err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}
	if _, err := source.CallTool(context.Background(), "sleep", json.RawMessage(`{"seconds":100}`)); err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}

	want := []time.Duration{time.Second, 60 * time.Second}
	if len(durations) != len(want) {
		t.Fatalf("duration count = %d, want %d", len(durations), len(want))
	}
	for index := range want {
		if durations[index] != want[index] {
			t.Fatalf("duration[%d] = %s, want %s", index, durations[index], want[index])
		}
	}
}

func TestSleepToolReturnsCanceledContext(t *testing.T) {
	source := newSourceWithSleeper(func(ctx context.Context, duration time.Duration) error {
		<-ctx.Done()
		return ctx.Err()
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := source.CallTool(ctx, "sleep", json.RawMessage(`{"seconds":10}`))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("CallTool() error = %v, want context.Canceled", err)
	}
}

func TestSleepToolListMetadata(t *testing.T) {
	source := NewSource()

	tools, err := source.ListTools(context.Background())
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}
	toolNames := make(map[string]bool, len(tools))
	for _, tool := range tools {
		toolNames[tool.Name] = true
	}
	for _, name := range []string{"sleep", "contacts", "my_groups", "reply", "send_as_user", "create_group", "add_group_members"} {
		if !toolNames[name] {
			t.Fatalf("tools = %+v, want %s", tools, name)
		}
	}
	for _, tool := range tools {
		if tool.Description == "" {
			t.Fatalf("tool %s description is empty", tool.Name)
		}
		if tool.InputSchema == nil {
			t.Fatalf("tool %s input schema is nil", tool.Name)
		}
	}
}

func TestGroupToolMetadataClarifiesUsageScenarios(t *testing.T) {
	source := NewSource()
	tools, err := source.ListTools(context.Background())
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}
	toolsByName := map[string]string{}
	for _, tool := range tools {
		toolsByName[tool.Name] = tool.Description
	}

	for _, snippet := range []string{"明确要求创建新群聊", "不要用它发送消息", "不要用它回复", "已有群聊", "先追问"} {
		if !strings.Contains(toolsByName["create_group"], snippet) {
			t.Fatalf("create_group description = %q, want to contain %q", toolsByName["create_group"], snippet)
		}
	}
	for _, snippet := range []string{"明确要求把人加入已有群聊", "不要用它创建群聊", "目标群聊不明确", "先追问", "当前会话是目标群聊"} {
		if !strings.Contains(toolsByName["add_group_members"], snippet) {
			t.Fatalf("add_group_members description = %q, want to contain %q", toolsByName["add_group_members"], snippet)
		}
	}
}

func TestSendAsUserToolMetadataClarifiesGroupUsageScenarios(t *testing.T) {
	source := NewSource()
	tools, err := source.ListTools(context.Background())
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}
	toolsByName := map[string]string{}
	for _, tool := range tools {
		toolsByName[tool.Name] = tool.Description
	}

	for _, snippet := range []string{"私聊或已有群聊", "target_type", "my_groups", "目标群聊不明确", "不要用它回复当前会话"} {
		if !strings.Contains(toolsByName["send_as_user"], snippet) {
			t.Fatalf("send_as_user description = %q, want to contain %q", toolsByName["send_as_user"], snippet)
		}
	}
}

func TestMessageToolMetadataClarifiesFileUsageScenarios(t *testing.T) {
	source := NewSource()
	tools, err := source.ListTools(context.Background())
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}
	toolsByName := map[string]mcpToolForTest{}
	for _, tool := range tools {
		schema, ok := tool.InputSchema.(map[string]any)
		if !ok {
			t.Fatalf("%s schema = %#v, want object schema", tool.Name, tool.InputSchema)
		}
		toolsByName[tool.Name] = mcpToolForTest{
			Description: tool.Description,
			Schema:      schema,
		}
	}

	for _, toolName := range []string{"reply", "send_as_user"} {
		for _, snippet := range []string{"file", "name", "url", "content", "小文件", "不要猜文件名", "先追问"} {
			if !strings.Contains(toolsByName[toolName].Description, snippet) {
				t.Fatalf("%s description = %q, want to contain %q", toolName, toolsByName[toolName].Description, snippet)
			}
		}
		properties := toolsByName[toolName].Schema["properties"].(map[string]any)
		for _, property := range []string{"name", "url", "content"} {
			if _, ok := properties[property]; !ok {
				t.Fatalf("%s schema properties = %#v, want %s", toolName, properties, property)
			}
		}
	}
}

type mcpToolForTest struct {
	Description string
	Schema      map[string]any
}

func TestContactsToolCallsAppRequest(t *testing.T) {
	requester := &fakeRequester{}
	ctx := WithScope(context.Background(), Scope{Requester: requester})
	source := NewSource()

	result, err := source.CallTool(ctx, "contacts", json.RawMessage(`{"keyword":"ali"}`))
	if err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}
	if result.Content != `{"ok":true}` {
		t.Fatalf("result = %q, want app response JSON", result.Content)
	}
	if len(requester.calls) != 1 {
		t.Fatalf("request call count = %d, want 1", len(requester.calls))
	}
	if requester.calls[0].method != methodContactsUsersList {
		t.Fatalf("method = %q, want %s", requester.calls[0].method, methodContactsUsersList)
	}
	var payload map[string]any
	if err := json.Unmarshal(requester.calls[0].payload, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload["keyword"] != "ali" {
		t.Fatalf("keyword = %v, want ali", payload["keyword"])
	}
}

func TestMyGroupsToolCallsAppRequestWithTriggerContext(t *testing.T) {
	requester := &fakeRequester{}
	ctx := WithScope(context.Background(), Scope{
		CurrentUserID:    "user-1",
		Requester:        requester,
		TriggerMessageID: "message-1",
	})
	source := NewSource()

	result, err := source.CallTool(ctx, "my_groups", json.RawMessage(`{"keyword":" 项目 "}`))
	if err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}
	if result.Content != `{"ok":true}` {
		t.Fatalf("result = %q, want app response JSON", result.Content)
	}
	if len(requester.calls) != 1 {
		t.Fatalf("request call count = %d, want 1", len(requester.calls))
	}
	if requester.calls[0].method != methodGroupConversationsList {
		t.Fatalf("method = %q, want %s", requester.calls[0].method, methodGroupConversationsList)
	}
	var payload struct {
		ActorUserID      string `json:"actor_user_id"`
		Keyword          string `json:"keyword"`
		TriggerMessageID string `json:"trigger_message_id"`
	}
	if err := json.Unmarshal(requester.calls[0].payload, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload.ActorUserID != "user-1" || payload.TriggerMessageID != "message-1" || payload.Keyword != "项目" {
		t.Fatalf("payload = %#v, want scoped actor/trigger and trimmed keyword", payload)
	}
}

func TestReplyToolCallsMessageSendForCurrentConversation(t *testing.T) {
	requester := &fakeRequester{}
	ctx := WithScope(context.Background(), Scope{
		ConversationID:   "conversation-1",
		ConversationType: "app",
		Requester:        requester,
	})
	source := NewSource()

	_, err := source.CallTool(ctx, "reply", json.RawMessage(`{"type":"image","content":"https://example.com/a.png"}`))
	if err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}
	if len(requester.calls) != 1 {
		t.Fatalf("request call count = %d, want 1", len(requester.calls))
	}
	if requester.calls[0].method != methodMessageSend {
		t.Fatalf("method = %q, want %s", requester.calls[0].method, methodMessageSend)
	}
	var payload struct {
		Target struct {
			Type           string `json:"type"`
			ConversationID string `json:"conversation_id"`
		} `json:"target"`
		Message struct {
			Type    string `json:"type"`
			Content string `json:"content"`
		} `json:"message"`
	}
	if err := json.Unmarshal(requester.calls[0].payload, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload.Target.Type != "app" || payload.Target.ConversationID != "conversation-1" {
		t.Fatalf("target = %#v, want current app conversation", payload.Target)
	}
	if payload.Message.Type != "image" || payload.Message.Content != "https://example.com/a.png" {
		t.Fatalf("message = %#v, want image URL", payload.Message)
	}
}

func TestReplyToolCallsMessageSendForFileURLWithSpecifiedName(t *testing.T) {
	requester := &fakeRequester{}
	ctx := WithScope(context.Background(), Scope{
		ConversationID:   "conversation-1",
		ConversationType: "app",
		Requester:        requester,
	})
	source := NewSource()

	_, err := source.CallTool(ctx, "reply", json.RawMessage(`{"type":"file","name":"report.md","url":"https://example.com/report.md"}`))
	if err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}
	if len(requester.calls) != 1 {
		t.Fatalf("request call count = %d, want 1", len(requester.calls))
	}
	var payload struct {
		Message struct {
			Type    string `json:"type"`
			Name    string `json:"name"`
			URL     string `json:"url"`
			Content string `json:"content"`
		} `json:"message"`
	}
	if err := json.Unmarshal(requester.calls[0].payload, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload.Message.Type != "file" || payload.Message.Name != "report.md" || payload.Message.URL != "https://example.com/report.md" {
		t.Fatalf("message = %#v, want file URL with specified name", payload.Message)
	}
	if payload.Message.Content != "" {
		t.Fatalf("message content = %q, want empty for URL file", payload.Message.Content)
	}
}

func TestReplyToolCallsMessageSendForInlineFileContentWithSpecifiedName(t *testing.T) {
	requester := &fakeRequester{}
	ctx := WithScope(context.Background(), Scope{
		ConversationID:   "conversation-1",
		ConversationType: "app",
		Requester:        requester,
	})
	source := NewSource()

	fileContent := "  # 报告\n\n正文\n"
	input, err := json.Marshal(map[string]any{
		"type":    "file",
		"name":    "assistant-report.md",
		"content": fileContent,
	})
	if err != nil {
		t.Fatalf("marshal input: %v", err)
	}
	_, err = source.CallTool(ctx, "reply", input)
	if err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}
	var payload struct {
		Message struct {
			Type    string `json:"type"`
			Name    string `json:"name"`
			URL     string `json:"url"`
			Content string `json:"content"`
		} `json:"message"`
	}
	if err := json.Unmarshal(requester.calls[0].payload, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload.Message.Type != "file" || payload.Message.Name != "assistant-report.md" || payload.Message.Content != fileContent {
		t.Fatalf("message = %#v, want inline file content with specified name", payload.Message)
	}
	if payload.Message.URL != "" {
		t.Fatalf("message url = %q, want empty for inline file", payload.Message.URL)
	}
}

func TestReplyToolRejectsInvalidFileInputs(t *testing.T) {
	source := NewSource()
	ctx := WithScope(context.Background(), Scope{
		ConversationID:   "conversation-1",
		ConversationType: "app",
		Requester:        &fakeRequester{},
	})

	for _, tt := range []struct {
		name  string
		input string
	}{
		{
			name:  "missing file name",
			input: `{"type":"file","content":"hello"}`,
		},
		{
			name:  "path file name",
			input: `{"type":"file","name":"reports/report.md","content":"hello"}`,
		},
		{
			name:  "url and content",
			input: `{"type":"file","name":"report.md","url":"https://example.com/report.md","content":"hello"}`,
		},
		{
			name:  "missing source",
			input: `{"type":"file","name":"report.md"}`,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := source.CallTool(ctx, "reply", json.RawMessage(tt.input)); err == nil {
				t.Fatal("CallTool() error = nil, want invalid file input error")
			}
		})
	}
}

func TestSendAsUserToolCallsMessageSendAsUserWithTriggerContext(t *testing.T) {
	requester := &fakeRequester{}
	ctx := WithScope(context.Background(), Scope{
		CurrentUserID:    "user-1",
		Requester:        requester,
		TriggerMessageID: "message-1",
	})
	source := NewSource()

	_, err := source.CallTool(ctx, "send_as_user", json.RawMessage(`{"contact_id":"user-2","type":"markdown","content":"**收到**"}`))
	if err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}
	if len(requester.calls) != 1 {
		t.Fatalf("request call count = %d, want 1", len(requester.calls))
	}
	if requester.calls[0].method != methodMessageSendAsUser {
		t.Fatalf("method = %q, want %s", requester.calls[0].method, methodMessageSendAsUser)
	}
	var payload struct {
		ActorUserID      string `json:"actor_user_id"`
		TargetUserID     string `json:"target_user_id"`
		TriggerMessageID string `json:"trigger_message_id"`
		Message          struct {
			Type    string `json:"type"`
			Content string `json:"content"`
		} `json:"message"`
	}
	if err := json.Unmarshal(requester.calls[0].payload, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload.ActorUserID != "user-1" || payload.TargetUserID != "user-2" || payload.TriggerMessageID != "message-1" {
		t.Fatalf("payload context = %#v, want scoped actor/target/trigger", payload)
	}
	if payload.Message.Type != "markdown" || payload.Message.Content != "**收到**" {
		t.Fatalf("message = %#v, want markdown content", payload.Message)
	}
}

func TestSendAsUserToolCallsMessageSendAsUserForGroupConversation(t *testing.T) {
	requester := &fakeRequester{}
	ctx := WithScope(context.Background(), Scope{
		CurrentUserID:    "user-1",
		Requester:        requester,
		TriggerMessageID: "message-1",
	})
	source := NewSource()

	_, err := source.CallTool(ctx, "send_as_user", json.RawMessage(`{"target_type":"group","conversation_id":"group-1","type":"text","content":"群里同步一下"}`))
	if err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}
	if len(requester.calls) != 1 {
		t.Fatalf("request call count = %d, want 1", len(requester.calls))
	}
	if requester.calls[0].method != methodMessageSendAsUser {
		t.Fatalf("method = %q, want %s", requester.calls[0].method, methodMessageSendAsUser)
	}
	var payload struct {
		ActorUserID      string `json:"actor_user_id"`
		TriggerMessageID string `json:"trigger_message_id"`
		Target           struct {
			ConversationID string `json:"conversation_id"`
			Type           string `json:"type"`
		} `json:"target"`
		Message struct {
			Type    string `json:"type"`
			Content string `json:"content"`
		} `json:"message"`
	}
	if err := json.Unmarshal(requester.calls[0].payload, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload.ActorUserID != "user-1" || payload.TriggerMessageID != "message-1" {
		t.Fatalf("payload context = %#v, want scoped actor/trigger", payload)
	}
	if payload.Target.Type != "group" || payload.Target.ConversationID != "group-1" {
		t.Fatalf("target = %#v, want group conversation", payload.Target)
	}
	if payload.Message.Type != "text" || payload.Message.Content != "群里同步一下" {
		t.Fatalf("message = %#v, want text content", payload.Message)
	}
}

func TestCreateGroupToolCallsAppRequestWithTriggerContext(t *testing.T) {
	requester := &fakeRequester{}
	ctx := WithScope(context.Background(), Scope{
		CurrentUserID:    "user-1",
		Requester:        requester,
		TriggerMessageID: "message-1",
	})
	source := NewSource()

	_, err := source.CallTool(ctx, "create_group", json.RawMessage(`{"name":"项目讨论组","member_ids":["user-2","user-3"]}`))
	if err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}
	if len(requester.calls) != 1 {
		t.Fatalf("request call count = %d, want 1", len(requester.calls))
	}
	if requester.calls[0].method != "group_conversations.create" {
		t.Fatalf("method = %q, want group_conversations.create", requester.calls[0].method)
	}
	var payload struct {
		ActorUserID      string   `json:"actor_user_id"`
		TriggerMessageID string   `json:"trigger_message_id"`
		Name             string   `json:"name"`
		MemberIDs        []string `json:"member_ids"`
	}
	if err := json.Unmarshal(requester.calls[0].payload, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload.ActorUserID != "user-1" || payload.TriggerMessageID != "message-1" {
		t.Fatalf("payload context = %#v, want scoped actor/trigger", payload)
	}
	if payload.Name != "项目讨论组" {
		t.Fatalf("payload name = %q, want 项目讨论组", payload.Name)
	}
	if len(payload.MemberIDs) != 2 || payload.MemberIDs[0] != "user-2" || payload.MemberIDs[1] != "user-3" {
		t.Fatalf("payload member_ids = %#v, want user-2,user-3", payload.MemberIDs)
	}
}

func TestAddGroupMembersToolDefaultsToCurrentGroupConversation(t *testing.T) {
	requester := &fakeRequester{}
	ctx := WithScope(context.Background(), Scope{
		ConversationID:   "group-1",
		ConversationType: "group",
		CurrentUserID:    "user-1",
		Requester:        requester,
		TriggerMessageID: "message-1",
	})
	source := NewSource()

	_, err := source.CallTool(ctx, "add_group_members", json.RawMessage(`{"member_ids":["user-2","user-3"]}`))
	if err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}
	if len(requester.calls) != 1 {
		t.Fatalf("request call count = %d, want 1", len(requester.calls))
	}
	if requester.calls[0].method != "group_conversations.members.add" {
		t.Fatalf("method = %q, want group_conversations.members.add", requester.calls[0].method)
	}
	var payload struct {
		ActorUserID      string   `json:"actor_user_id"`
		ConversationID   string   `json:"conversation_id"`
		TriggerMessageID string   `json:"trigger_message_id"`
		MemberIDs        []string `json:"member_ids"`
	}
	if err := json.Unmarshal(requester.calls[0].payload, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload.ActorUserID != "user-1" || payload.TriggerMessageID != "message-1" || payload.ConversationID != "group-1" {
		t.Fatalf("payload context = %#v, want scoped actor/trigger/current group", payload)
	}
	if len(payload.MemberIDs) != 2 || payload.MemberIDs[0] != "user-2" || payload.MemberIDs[1] != "user-3" {
		t.Fatalf("payload member_ids = %#v, want user-2,user-3", payload.MemberIDs)
	}
}

func TestScopedToolsRequireScope(t *testing.T) {
	source := NewSource()

	_, err := source.CallTool(context.Background(), "reply", json.RawMessage(`{"type":"text","content":"hi"}`))
	if err == nil {
		t.Fatal("CallTool() error = nil, want missing scope error")
	}
}
