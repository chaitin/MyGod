package builtintools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"assistant/internal/mcpclient"
)

type requestCall struct {
	method  string
	payload json.RawMessage
}

type fakeRequester struct {
	calls  []requestCall
	handle func(context.Context, string, any) (json.RawMessage, error)
}

type fakeConversationWaitRegistrar struct {
	actorID        string
	actorType      string
	afterSeq       int64
	closed         bool
	conversationID string
}

func (r *fakeConversationWaitRegistrar) RegisterConversationWait(conversationID string, afterSeq int64, actorType string, actorID string) (ConversationWaitRegistration, error) {
	r.conversationID = conversationID
	r.afterSeq = afterSeq
	r.actorType = actorType
	r.actorID = actorID
	return fakeConversationWaitRegistration{registrar: r}, nil
}

type fakeConversationWaitRegistration struct {
	registrar *fakeConversationWaitRegistrar
}

func (r fakeConversationWaitRegistration) Close() {
	r.registrar.closed = true
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
	if r.handle != nil {
		return r.handle(ctx, method, payload)
	}

	return json.RawMessage(`{"ok":true}`), nil
}

func TestSleepToolClampsDuration(t *testing.T) {
	var durations []time.Duration
	source := newSourceWithSleeper(func(ctx context.Context, duration time.Duration) error {
		durations = append(durations, duration)
		return nil
	})

	for _, input := range []json.RawMessage{
		json.RawMessage(`{"seconds":0}`),
		json.RawMessage(`{"seconds":3}`),
		json.RawMessage(`{"seconds":5}`),
		json.RawMessage(`{"seconds":30}`),
		json.RawMessage(`{"seconds":100}`),
		json.RawMessage(`{"seconds":"bad"}`),
		json.RawMessage(`not-json`),
		nil,
	} {
		if _, err := source.CallTool(context.Background(), "sleep", input); err != nil {
			t.Fatalf("CallTool(%s) error = %v", input, err)
		}
	}

	want := []time.Duration{
		5 * time.Second,
		5 * time.Second,
		5 * time.Second,
		30 * time.Second,
		30 * time.Second,
		5 * time.Second,
		5 * time.Second,
		5 * time.Second,
	}
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
	for _, name := range []string{"help", "sleep", "get_attachments", "end_conversation", "contacts", "conversations", "projects"} {
		if !toolNames[name] {
			t.Fatalf("tools = %+v, want %s", tools, name)
		}
	}
	for _, name := range []string{"wait", "recent_conversations", "read_history", "reply", "send_as_user", "create_group", "add_group_members", "read_file_urls"} {
		if toolNames[name] {
			t.Fatalf("tools = %+v, legacy tool %s should no longer be exposed", tools, name)
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

func TestEndConversationToolEndsCurrentCycleWithoutVisibleOutput(t *testing.T) {
	source := NewSource()

	result, err := source.CallTool(context.Background(), endConversationToolName, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}
	if result.Content != "当前处理已结束" || !result.Final || result.IsError {
		t.Fatalf("result = %#v", result)
	}
}

func TestEndConversationToolRejectsArguments(t *testing.T) {
	source := NewSource()
	if _, err := source.CallTool(context.Background(), endConversationToolName, json.RawMessage(`{"unexpected":true}`)); err == nil || !strings.Contains(err.Error(), "does not accept arguments") {
		t.Fatalf("CallTool() error = %v", err)
	}
}

func TestLegacyBuiltinToolNamesAreRejected(t *testing.T) {
	source := NewSource()
	for _, name := range []string{"recent_conversations", "read_history", "reply", "send_as_user", "create_group", "add_group_members", "read_file_urls", "wait", "reset_context"} {
		if _, err := source.CallTool(context.Background(), name, json.RawMessage(`{}`)); err == nil || !strings.Contains(err.Error(), "unknown builtin tool") {
			t.Fatalf("CallTool(%q) error = %v, want unknown builtin tool", name, err)
		}
	}
}

func TestSleepToolMetadataIsDirectAndSelfContained(t *testing.T) {
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

	tool := toolsByName["sleep"]
	for _, snippet := range []string{"5", "30", "异步任务", "不要用于普通回复"} {
		if !strings.Contains(tool.Description, snippet) {
			t.Fatalf("sleep description = %q, want to contain %q", tool.Description, snippet)
		}
	}
	properties := tool.Schema["properties"].(map[string]any)
	seconds, ok := properties["seconds"].(map[string]any)
	if !ok || seconds["minimum"] != minSleepSeconds || seconds["maximum"] != maxSleepSeconds {
		t.Fatalf("sleep schema properties = %#v, want bounded seconds", properties)
	}
	if _, ok := properties["operation"]; ok {
		t.Fatalf("sleep schema properties = %#v, operation should not be present", properties)
	}
}

func TestGroupToolMetadataClarifiesUsageScenarios(t *testing.T) {
	source := NewSource()
	for _, tt := range []struct {
		operation string
		snippets  []string
	}{
		{operation: conversationsOperationCreate, snippets: []string{"创建新群聊", "member_ids", "群主"}},
		{operation: conversationsOperationAdd, snippets: []string{"已有群聊", "member_ids", "conversation_id"}},
	} {
		description, schema := helpOperationForTest(t, source, tt.operation)
		for _, snippet := range tt.snippets {
			if !strings.Contains(description, snippet) {
				t.Fatalf("%s description = %q, want to contain %q", tt.operation, description, snippet)
			}
		}
		properties := schema["properties"].(map[string]any)
		if _, ok := properties["runas"]; !ok {
			t.Fatalf("%s schema = %#v, want top-level runas", tt.operation, schema)
		}
	}
}

func TestSendAsUserToolMetadataClarifiesGroupUsageScenarios(t *testing.T) {
	source := NewSource()
	description, schema := helpOperationForTest(t, source, conversationsOperationSend)
	for _, snippet := range []string{"私聊联系人", "已有群聊", "target_type", "contact_id", "conversation_id"} {
		if !strings.Contains(description, snippet) {
			t.Fatalf("send description = %q, want to contain %q", description, snippet)
		}
	}
	properties := schema["properties"].(map[string]any)
	arguments := properties["arguments"].(map[string]any)["properties"].(map[string]any)
	for _, property := range []string{"target_type", "contact_id", "conversation_id"} {
		if _, ok := arguments[property]; !ok {
			t.Fatalf("send arguments = %#v, want %s", arguments, property)
		}
	}
}

func TestConversationMessageHelpDocumentsMentionTokens(t *testing.T) {
	source := NewSource()
	for _, operation := range []string{conversationsOperationReply, conversationsOperationSend} {
		description, schema := helpOperationForTest(t, source, operation)
		arguments := schema["properties"].(map[string]any)["arguments"].(map[string]any)
		contentDescription := arguments["properties"].(map[string]any)["content"].(map[string]any)["description"].(string)
		for _, token := range []string{"{(@user/用户UUID)}", "{(@app/应用UUID)}", "{(@user/all)}"} {
			if !strings.Contains(description, token) {
				t.Fatalf("%s description = %q, want %s", operation, description, token)
			}
			if !strings.Contains(contentDescription, token) {
				t.Fatalf("%s content description = %q, want %s", operation, contentDescription, token)
			}
		}
	}
}

func TestMessageToolMetadataDocumentsSupportedMessageTypes(t *testing.T) {
	source := NewSource()
	for _, operation := range []string{conversationsOperationReply, conversationsOperationSend} {
		description, schema := helpOperationForTest(t, source, operation)
		for _, snippet := range []string{"text", "markdown", "choice", "image", "file", "card", "不要默认使用 text/markdown", "即使用户没有明确要求图表", "分析、对比、趋势、分布、占比、排名、统计或多维评分", "单个孤立数字"} {
			if !strings.Contains(description, snippet) {
				t.Fatalf("%s description = %q, want to contain %q", operation, description, snippet)
			}
		}
		properties := schema["properties"].(map[string]any)["arguments"].(map[string]any)["properties"].(map[string]any)
		for _, property := range []string{"name", "url", "content", "content_type", "selection", "options", "title", "description"} {
			if _, ok := properties[property]; !ok {
				t.Fatalf("%s arguments = %#v, want %s", operation, properties, property)
			}
		}
		if _, ok := properties["action"]; ok {
			t.Fatalf("%s arguments = %#v, action should not be exposed", operation, properties)
		}
		urlDescription := properties["url"].(map[string]any)["description"].(string)
		for _, snippet := range []string{"http://", "https://", "javascript:"} {
			if !strings.Contains(urlDescription, snippet) {
				t.Fatalf("%s url description = %q, want %q", operation, urlDescription, snippet)
			}
		}
		descriptionDescription := properties["description"].(map[string]any)["description"].(string)
		for _, snippet := range []string{"纯文本", "不支持 Markdown"} {
			if !strings.Contains(descriptionDescription, snippet) {
				t.Fatalf("%s description field = %q, want %q", operation, descriptionDescription, snippet)
			}
		}
	}
}

func TestEntityCardToolMetadataUsesObjectReferences(t *testing.T) {
	source := NewSource()
	for _, operation := range []string{conversationsOperationReplyEntityCard, conversationsOperationSendEntityCard} {
		description, schema := helpOperationForTest(t, source, operation)
		for _, snippet := range []string{"user", "app", "group", "project", "task", "Server", "尽量使用本操作", "不要把对象资料降级为 text/markdown"} {
			if !strings.Contains(description, snippet) {
				t.Fatalf("%s description = %q, want %q", operation, description, snippet)
			}
		}
		required := schema["required"].([]any)
		if !slices.Contains(required, any("runas")) {
			t.Fatalf("%s required = %#v, want runas", operation, required)
		}
		arguments := schema["properties"].(map[string]any)["arguments"].(map[string]any)
		properties := arguments["properties"].(map[string]any)
		for _, property := range []string{"entity_type", "entity_id"} {
			if _, ok := properties[property]; !ok {
				t.Fatalf("%s arguments = %#v, want %s", operation, properties, property)
			}
		}
		for _, property := range []string{"title", "description", "url"} {
			if _, ok := properties[property]; ok {
				t.Fatalf("%s arguments = %#v, should not expose %s", operation, properties, property)
			}
		}
	}
}

type mcpToolForTest struct {
	Description string
	Schema      map[string]any
}

func helpOperationForTest(t *testing.T, source *Source, operation string) (string, map[string]any) {
	t.Helper()
	result, err := source.CallTool(context.Background(), helpToolName, json.RawMessage(fmt.Sprintf(`{"capability":"conversations","operation":%q}`, operation)))
	if err != nil {
		t.Fatalf("CallTool(help %s) error = %v", operation, err)
	}
	var detail struct {
		Description string         `json:"description"`
		InputSchema map[string]any `json:"input_schema"`
		Tool        string         `json:"tool"`
	}
	if err := json.Unmarshal([]byte(result.Content), &detail); err != nil {
		t.Fatalf("unmarshal help %s: %v", operation, err)
	}
	if detail.Tool != "builtin__conversations" {
		t.Fatalf("help %s tool = %q, want builtin__conversations", operation, detail.Tool)
	}
	return detail.Description, detail.InputSchema
}

func TestReadFileURLsToolMetadataClarifiesOnDemandUsage(t *testing.T) {
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

	tool, ok := toolsByName["get_attachments"]
	if !ok {
		t.Fatalf("tools = %+v, want get_attachments", tools)
	}
	for _, snippet := range []string{"按需", "file_id", "历史消息", "当前消息", "部分失败", "不需要会话 ID"} {
		if !strings.Contains(tool.Description, snippet) {
			t.Fatalf("get_attachments description = %q, want to contain %q", tool.Description, snippet)
		}
	}
	properties := tool.Schema["properties"].(map[string]any)
	if _, ok := properties["file_ids"]; !ok {
		t.Fatalf("get_attachments schema properties = %#v, want file_ids", properties)
	}
}

func TestContactsToolCallsAppRequest(t *testing.T) {
	requester := &fakeRequester{}
	ctx := WithScope(context.Background(), Scope{
		AuthorizationResolver: AuthorizationResolverFunc(func(ref string) (Authorization, bool) {
			if ref != "auth-user-1" {
				return Authorization{}, false
			}
			return Authorization{ActorType: "user", ActorID: "user-1", TriggerMessageID: "message-1"}, true
		}),
		ConversationID: "conversation-1",
		Requester:      requester,
	})
	source := NewSource()

	result, err := source.CallTool(ctx, "contacts", json.RawMessage(`{"operation":"search_users","runas":{"type":" USER ","id":" user-1 ","authorization_ref":"auth-user-1"},"arguments":{"keyword":" ali "}}`))
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
	runAs := payload["runas"].(map[string]any)
	if runAs["type"] != "user" || runAs["id"] != "user-1" {
		t.Fatalf("runas = %#v, want normalized user identity", runAs)
	}
	if runAs["trigger_message_id"] != "message-1" || runAs["authorization_conversation_id"] != "conversation-1" {
		t.Fatalf("runas = %#v, want resolved authorization proof", runAs)
	}
}

func TestContactsToolSearchAppsCallsAppRequest(t *testing.T) {
	requester := &fakeRequester{}
	ctx := WithScope(context.Background(), Scope{
		AuthorizationResolver: AuthorizationResolverFunc(func(ref string) (Authorization, bool) {
			if ref != "auth-user-2" {
				return Authorization{}, false
			}
			return Authorization{ActorType: "user", ActorID: "user-2", TriggerMessageID: "message-2"}, true
		}),
		ConversationID: "conversation-2",
		Requester:      requester,
	})
	source := NewSource()

	result, err := source.CallTool(ctx, contactsToolName, json.RawMessage(`{"operation":"search_apps","runas":{"type":"user","id":"user-2","authorization_ref":"auth-user-2"},"arguments":{"keyword":" 助手 "}}`))
	if err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}
	if result.Content != `{"ok":true}` {
		t.Fatalf("result = %q, want app response JSON", result.Content)
	}
	if len(requester.calls) != 1 || requester.calls[0].method != methodContactsAppsList {
		t.Fatalf("request calls = %#v, want %s", requester.calls, methodContactsAppsList)
	}
	var payload map[string]any
	if err := json.Unmarshal(requester.calls[0].payload, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload["keyword"] != "助手" {
		t.Fatalf("keyword = %v, want 助手", payload["keyword"])
	}
	runAs := payload["runas"].(map[string]any)
	if runAs["type"] != "user" || runAs["id"] != "user-2" {
		t.Fatalf("runas = %#v, want user identity", runAs)
	}
}

func TestContactsToolSearchGroupsCallsAppRequest(t *testing.T) {
	requester := &fakeRequester{}
	ctx := WithScope(context.Background(), Scope{
		AuthorizationResolver: AuthorizationResolverFunc(func(ref string) (Authorization, bool) {
			if ref != "auth-user-1" {
				return Authorization{}, false
			}
			return Authorization{ActorType: "user", ActorID: "user-1", TriggerMessageID: "message-1"}, true
		}),
		ConversationID: "conversation-1",
		Requester:      requester,
	})
	source := NewSource()

	_, err := source.CallTool(ctx, contactsToolName, json.RawMessage(`{"operation":"search_groups","runas":{"type":"user","id":"user-1","authorization_ref":"auth-user-1"},"arguments":{"keyword":" 项目 "}}`))
	if err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}
	if len(requester.calls) != 1 || requester.calls[0].method != methodContactsGroupsList {
		t.Fatalf("request calls = %#v, want %s", requester.calls, methodContactsGroupsList)
	}
	var payload map[string]any
	if err := json.Unmarshal(requester.calls[0].payload, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload["keyword"] != "项目" {
		t.Fatalf("keyword = %v, want 项目", payload["keyword"])
	}
}

func TestContactsToolMetadataUsesGlobalHelpProtocol(t *testing.T) {
	source := NewSource()
	tools, err := source.ListTools(context.Background())
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}

	var contactsTool mcpclient.Tool
	for _, tool := range tools {
		if tool.Name == contactsToolName {
			contactsTool = tool
			break
		}
	}
	if contactsTool.Name == "" {
		t.Fatalf("tools = %+v, want contacts", tools)
	}
	for _, snippet := range []string{"全局 help", "runas", "authorization_ref"} {
		if !strings.Contains(contactsTool.Description, snippet) {
			t.Fatalf("contacts description = %q, want %q", contactsTool.Description, snippet)
		}
	}
	schema := contactsTool.InputSchema.(map[string]any)
	required := schema["required"].([]string)
	if !slices.Contains(required, "runas") {
		t.Fatalf("contacts required = %#v, want runas", required)
	}
	properties := schema["properties"].(map[string]any)
	for _, property := range []string{"operation", "arguments"} {
		if _, ok := properties[property]; !ok {
			t.Fatalf("contacts schema properties = %#v, want %s", properties, property)
		}
	}
	operationSchema := properties["operation"].(map[string]any)
	operations := operationSchema["enum"].([]string)
	for _, operation := range []string{contactsOperationSearchUsers, contactsOperationSearchApps, contactsOperationSearchGroups} {
		if !slices.Contains(operations, operation) {
			t.Fatalf("contacts operation enum = %#v, want %s", operations, operation)
		}
	}
	if _, ok := properties["keyword"]; ok {
		t.Fatalf("contacts schema properties = %#v, keyword should only be disclosed by help", properties)
	}
	runAsTypes := properties["runas"].(map[string]any)["properties"].(map[string]any)["type"].(map[string]any)["enum"].([]string)
	if !slices.Equal(runAsTypes, []string{"user"}) {
		t.Fatalf("contacts runas types = %#v, want user", runAsTypes)
	}
}

func TestHelpReturnsCapabilitiesAndContactsOperationSchema(t *testing.T) {
	source := NewSource()
	result, err := source.CallTool(context.Background(), helpToolName, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}
	var catalog struct {
		Kind         string `json:"kind"`
		Capabilities []struct {
			Name string `json:"name"`
		} `json:"capabilities"`
	}
	if err := json.Unmarshal([]byte(result.Content), &catalog); err != nil {
		t.Fatalf("unmarshal catalog: %v", err)
	}
	if catalog.Kind != "capability_list" || len(catalog.Capabilities) != 3 {
		t.Fatalf("catalog = %#v, want contacts, conversations, and projects capabilities", catalog)
	}
	for _, capability := range catalog.Capabilities {
		if capability.Name == "sleep" || capability.Name == "wait" {
			t.Fatalf("catalog = %#v, sleep should be a direct tool, not a help capability", catalog)
		}
	}

	result, err = source.CallTool(context.Background(), helpToolName, json.RawMessage(`{"capability":"contacts"}`))
	if err != nil {
		t.Fatalf("CallTool(capability) error = %v", err)
	}
	for _, operation := range []string{contactsOperationSearchUsers, contactsOperationSearchApps, contactsOperationSearchGroups} {
		if !strings.Contains(result.Content, operation) {
			t.Fatalf("contacts help = %s, want %s", result.Content, operation)
		}
	}
	if strings.Contains(result.Content, `"effect"`) || strings.Contains(result.Content, `"supports_runas"`) {
		t.Fatalf("contacts help = %s, contains removed metadata", result.Content)
	}
	if !strings.Contains(result.Content, `"description"`) {
		t.Fatalf("contacts help = %s, want descriptions", result.Content)
	}

	result, err = source.CallTool(context.Background(), helpToolName, json.RawMessage(`{"capability":"contacts","operation":"search_apps"}`))
	if err != nil {
		t.Fatalf("CallTool(operation) error = %v", err)
	}
	var operation struct {
		Description string         `json:"description"`
		Kind        string         `json:"kind"`
		Tool        string         `json:"tool"`
		InputSchema map[string]any `json:"input_schema"`
	}
	if err := json.Unmarshal([]byte(result.Content), &operation); err != nil {
		t.Fatalf("unmarshal operation help: %v", err)
	}
	if operation.Kind != "operation" || operation.Tool != "builtin__contacts" {
		t.Fatalf("operation help = %#v, want contacts tool", operation)
	}
	if operation.Description == "" {
		t.Fatal("operation help description is empty")
	}
	properties := operation.InputSchema["properties"].(map[string]any)
	operationRequired := operation.InputSchema["required"].([]any)
	if !slices.Contains(operationRequired, any("runas")) {
		t.Fatalf("operation required = %#v, want runas", operationRequired)
	}
	runAs := properties["runas"].(map[string]any)
	required := runAs["required"].([]any)
	if !slices.Contains(required, any("authorization_ref")) {
		t.Fatalf("runas required = %#v, want authorization_ref", required)
	}
}

func TestContactsToolRejectsLegacyKeywordOnlyInput(t *testing.T) {
	source := NewSource()
	if _, err := source.CallTool(context.Background(), contactsToolName, json.RawMessage(`{"keyword":"ali"}`)); err == nil || !strings.Contains(err.Error(), "operation is required") {
		t.Fatalf("CallTool() error = %v, want legacy input rejection", err)
	}
}

func TestContactsToolRejectsUnknownOperation(t *testing.T) {
	source := NewSource()
	_, err := source.CallTool(context.Background(), contactsToolName, json.RawMessage(`{"operation":"delete"}`))
	if err == nil || !strings.Contains(err.Error(), "use help") {
		t.Fatalf("CallTool() error = %v, want help guidance", err)
	}
}

func TestContactsToolRejectsInvalidRunAs(t *testing.T) {
	requester := &fakeRequester{}
	ctx := WithScope(context.Background(), Scope{Requester: requester})
	source := NewSource()

	for _, input := range []json.RawMessage{
		json.RawMessage(`{"operation":"search_users","runas":{"type":"system","id":"id-1"}}`),
		json.RawMessage(`{"operation":"search_apps","runas":{"type":"user","id":" "}}`),
	} {
		if _, err := source.CallTool(ctx, contactsToolName, input); err == nil {
			t.Fatalf("CallTool(%s) error = nil, want invalid runas error", input)
		}
	}
	if len(requester.calls) != 0 {
		t.Fatalf("request call count = %d, want 0", len(requester.calls))
	}
}

func TestContactsToolRunAsRequiresMatchingAuthorizationRef(t *testing.T) {
	requester := &fakeRequester{}
	ctx := WithScope(context.Background(), Scope{
		AuthorizationResolver: AuthorizationResolverFunc(func(ref string) (Authorization, bool) {
			if ref != "auth-user-1" {
				return Authorization{}, false
			}
			return Authorization{ActorType: "user", ActorID: "user-1", TriggerMessageID: "message-1"}, true
		}),
		ConversationID: "conversation-1",
		Requester:      requester,
	})
	source := NewSource()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "missing ref", input: `{"operation":"search_users","runas":{"type":"user","id":"user-1"}}`, want: "authorization_ref is required"},
		{name: "unknown ref", input: `{"operation":"search_users","runas":{"type":"user","id":"user-1","authorization_ref":"auth-missing"}}`, want: "authorization_ref is invalid"},
		{name: "app identity", input: `{"operation":"search_apps","runas":{"type":"app","id":"app-1","authorization_ref":"auth-user-1"}}`, want: "runas.type must be user"},
		{name: "identity mismatch", input: `{"operation":"search_apps","runas":{"type":"user","id":"user-2","authorization_ref":"auth-user-1"}}`, want: "does not authorize runas identity"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := source.CallTool(ctx, contactsToolName, json.RawMessage(tt.input))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("CallTool() error = %v, want %q", err, tt.want)
			}
		})
	}
	if len(requester.calls) != 0 {
		t.Fatalf("request call count = %d, want 0", len(requester.calls))
	}
}

func TestHelpRejectsInvalidQueries(t *testing.T) {
	source := NewSource()
	for _, input := range []json.RawMessage{
		json.RawMessage(`{"operation":"search_users"}`),
		json.RawMessage(`{"capability":"missing"}`),
		json.RawMessage(`{"capability":"contacts","operation":"missing"}`),
	} {
		if _, err := source.CallTool(context.Background(), helpToolName, input); err == nil {
			t.Fatalf("CallTool(%s) error = nil, want invalid help query", input)
		}
	}
}

func TestRecentConversationsToolMetadataClarifiesResultShape(t *testing.T) {
	source := NewSource()
	description, schema := helpOperationForTest(t, source, conversationsOperationSearch)
	for _, snippet := range []string{"最近使用的会话", "私聊", "群聊", "应用", "会话名称", "私聊对象", "姓名", "昵称", "成员数量", "最近活动时间"} {
		if !strings.Contains(description, snippet) {
			t.Fatalf("search description = %q, want to contain %q", description, snippet)
		}
	}
	properties := schema["properties"].(map[string]any)["arguments"].(map[string]any)["properties"].(map[string]any)
	for _, property := range []string{"keyword", "limit"} {
		if _, ok := properties[property]; !ok {
			t.Fatalf("search arguments = %#v, want %s", properties, property)
		}
	}
}

func TestRecentConversationsToolCallsAppRequestWithTriggerContext(t *testing.T) {
	requester := &fakeRequester{}
	ctx := WithScope(context.Background(), Scope{
		CurrentUserID:    "user-1",
		Requester:        requester,
		TriggerMessageID: "message-1",
	})

	result, err := callRecentConversations(ctx, json.RawMessage(`{"keyword":" 项目 ","limit":200}`))
	if err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}
	if result.Content != `{"ok":true}` {
		t.Fatalf("result = %q, want app response JSON", result.Content)
	}
	if len(requester.calls) != 1 {
		t.Fatalf("request call count = %d, want 1", len(requester.calls))
	}
	if requester.calls[0].method != methodConversationsList {
		t.Fatalf("method = %q, want %s", requester.calls[0].method, methodConversationsList)
	}
	var payload struct {
		ActorUserID      string `json:"actor_user_id"`
		Keyword          string `json:"keyword"`
		Limit            int    `json:"limit"`
		TriggerMessageID string `json:"trigger_message_id"`
	}
	if err := json.Unmarshal(requester.calls[0].payload, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload.ActorUserID != "user-1" || payload.TriggerMessageID != "message-1" || payload.Keyword != "项目" || payload.Limit != 200 {
		t.Fatalf("payload = %#v, want scoped actor/trigger, trimmed keyword, and limit", payload)
	}
}

func TestConversationsSearchUsesTopLevelRunAs(t *testing.T) {
	requester := &fakeRequester{}
	ctx := WithScope(context.Background(), Scope{
		AuthorizationResolver: AuthorizationResolverFunc(func(ref string) (Authorization, bool) {
			if ref != "auth-user-1" {
				return Authorization{}, false
			}
			return Authorization{ActorType: "user", ActorID: "user-1", TriggerMessageID: "message-1"}, true
		}),
		ConversationID: "authorization-conversation-1",
		Requester:      requester,
	})
	source := NewSource()

	result, err := source.CallTool(ctx, conversationsToolName, json.RawMessage(`{
		"operation":"search",
		"runas":{"type":"user","id":"user-1","authorization_ref":"auth-user-1"},
		"arguments":{"keyword":" 项目 ","limit":12}
	}`))
	if err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}
	if result.Content != `{"ok":true}` || len(requester.calls) != 1 || requester.calls[0].method != methodConversationsList {
		t.Fatalf("result = %#v, calls = %#v", result, requester.calls)
	}
	var payload recentConversationsPayload
	if err := json.Unmarshal(requester.calls[0].payload, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload.ActorUserID != "user-1" || payload.TriggerMessageID != "message-1" || payload.AuthorizationConversationID != "authorization-conversation-1" || payload.Keyword != "项目" || payload.Limit != 12 {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestConversationsWaitForReplyRequiresUserRunAs(t *testing.T) {
	requester := &fakeRequester{}
	ctx := WithScope(context.Background(), Scope{
		AuthorizationResolver: AuthorizationResolverFunc(func(ref string) (Authorization, bool) {
			return Authorization{ActorType: "app", ActorID: "app-1", TriggerMessageID: "message-1"}, ref == "auth-app-1"
		}),
		Requester: requester,
	})
	source := NewSource()

	for _, input := range []json.RawMessage{
		json.RawMessage(`{"operation":"wait_for_reply","arguments":{"conversation_id":"conversation-1","after_seq":10,"timeout_seconds":60}}`),
		json.RawMessage(`{"operation":"wait_for_reply","runas":{"type":"app","id":"app-1","authorization_ref":"auth-app-1"},"arguments":{"conversation_id":"conversation-1","after_seq":10,"timeout_seconds":60}}`),
	} {
		if _, err := source.CallTool(ctx, conversationsToolName, input); err == nil {
			t.Fatalf("CallTool(%s) error = nil, want user runas rejection", input)
		}
	}
	if len(requester.calls) != 0 {
		t.Fatalf("request calls = %#v, want none", requester.calls)
	}
}

func TestConversationsWaitForReplyFiltersRunAsMessages(t *testing.T) {
	call := 0
	requester := &fakeRequester{handle: func(_ context.Context, method string, payload any) (json.RawMessage, error) {
		if method != methodConversationHistoryRead {
			t.Fatalf("method = %q, want %s", method, methodConversationHistoryRead)
		}
		call++
		if call == 1 {
			return json.RawMessage(`{"messages":[{"id":"message-20","seq":20,"sender":{"type":"user","id":"user-1"}}]}`), nil
		}
		return json.RawMessage(`{"messages":[{"id":"message-20","seq":20,"sender":{"type":"user","id":"user-1"}},{"id":"message-21","seq":21,"sender":{"type":"user","id":"user-1"}},{"id":"message-22","seq":22,"sender":{"type":"app","id":"app-2"}}]}`), nil
	}}
	ctx := WithScope(context.Background(), Scope{
		AuthorizationResolver: AuthorizationResolverFunc(func(ref string) (Authorization, bool) {
			return Authorization{ActorType: "user", ActorID: "user-1", TriggerMessageID: "message-1"}, ref == "auth-user-1"
		}),
		ConversationID: "authorization-conversation-1",
		Requester:      requester,
	})
	source := newSourceWithSleeper(func(context.Context, time.Duration) error { return nil })

	result, err := source.CallTool(ctx, conversationsToolName, json.RawMessage(`{
		"operation":"wait_for_reply",
		"runas":{"type":"user","id":"user-1","authorization_ref":"auth-user-1"},
		"arguments":{"conversation_id":"conversation-1","after_seq":20,"timeout_seconds":5}
	}`))
	if err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}
	var response struct {
		Status   string `json:"status"`
		Messages []struct {
			Seq int64 `json:"seq"`
		} `json:"messages"`
	}
	if err := json.Unmarshal([]byte(result.Content), &response); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if response.Status != "replied" || len(response.Messages) != 1 || response.Messages[0].Seq != 22 {
		t.Fatalf("response = %#v, want only the other identity's message", response)
	}
	var payload readHistoryPayload
	if err := json.Unmarshal(requester.calls[1].payload, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload.ActorUserID != "user-1" || payload.TriggerMessageID != "message-1" || payload.Limit != waitForReplyMessageLimit {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestConversationsWaitForReplyReturnsTimeoutAtRequestedDeadline(t *testing.T) {
	var sleeps []time.Duration
	requester := &fakeRequester{handle: func(_ context.Context, _ string, _ any) (json.RawMessage, error) {
		return json.RawMessage(`{"messages":[{"id":"message-10","seq":10,"sender":{"type":"user","id":"user-1"}}]}`), nil
	}}
	ctx := WithScope(context.Background(), Scope{
		AuthorizationResolver: AuthorizationResolverFunc(func(ref string) (Authorization, bool) {
			return Authorization{ActorType: "user", ActorID: "user-1", TriggerMessageID: "message-1"}, ref == "auth-user-1"
		}),
		ConversationID: "authorization-conversation-1",
		Requester:      requester,
	})
	source := newSourceWithSleeper(func(_ context.Context, duration time.Duration) error {
		sleeps = append(sleeps, duration)
		return nil
	})

	result, err := source.CallTool(ctx, conversationsToolName, json.RawMessage(`{"operation":"wait_for_reply","runas":{"type":"user","id":"user-1","authorization_ref":"auth-user-1"},"arguments":{"conversation_id":"conversation-1","after_seq":10,"timeout_seconds":12}}`))
	if err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}
	var response struct {
		Status        string            `json:"status"`
		WaitedSeconds int               `json:"waited_seconds"`
		Messages      []json.RawMessage `json:"messages"`
	}
	if err := json.Unmarshal([]byte(result.Content), &response); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if response.Status != "timeout" || response.WaitedSeconds != 12 || len(response.Messages) != 0 {
		t.Fatalf("response = %#v", response)
	}
	wantSleeps := []time.Duration{5 * time.Second, 5 * time.Second, 2 * time.Second}
	if !slices.Equal(sleeps, wantSleeps) || len(requester.calls) != 4 {
		t.Fatalf("sleeps = %v, calls = %d", sleeps, len(requester.calls))
	}
}

func TestConversationRepliesAfterReturnsAtMostLatestThirty(t *testing.T) {
	messages := make([]json.RawMessage, 0, 35)
	for seq := 1; seq <= 35; seq++ {
		messages = append(messages, json.RawMessage(fmt.Sprintf(`{"id":"message-%d","seq":%d,"sender":{"type":"user","id":"user-2"}}`, seq, seq)))
	}
	replies, err := conversationRepliesAfter(messages, 0, Authorization{})
	if err != nil {
		t.Fatalf("conversationRepliesAfter() error = %v", err)
	}
	if len(replies) != waitForReplyMessageLimit {
		t.Fatalf("reply count = %d, want %d", len(replies), waitForReplyMessageLimit)
	}
	var first, last conversationHistoryMessageMetadata
	if err := json.Unmarshal(replies[0], &first); err != nil {
		t.Fatalf("unmarshal first reply: %v", err)
	}
	if err := json.Unmarshal(replies[len(replies)-1], &last); err != nil {
		t.Fatalf("unmarshal last reply: %v", err)
	}
	if first.Seq != 6 || last.Seq != 35 {
		t.Fatalf("reply seq range = %d..%d, want 6..35", first.Seq, last.Seq)
	}
}

func TestWaitForReplyHelpSchemaLimitsTimeoutToSixtySeconds(t *testing.T) {
	source := NewSource()
	description, schema := helpOperationForTest(t, source, conversationsOperationWait)
	for _, snippet := range []string{"立即查询一次", "每 5 秒", "最新 30 条", "5 到 60", "status=timeout", "after_seq"} {
		if !strings.Contains(description, snippet) {
			t.Fatalf("wait_for_reply description = %q, want %q", description, snippet)
		}
	}
	properties := schema["properties"].(map[string]any)
	required := schema["required"].([]any)
	if !slices.Contains(required, any("runas")) {
		t.Fatalf("wait_for_reply required = %#v, want runas", required)
	}
	arguments := properties["arguments"].(map[string]any)["properties"].(map[string]any)
	timeout := arguments["timeout_seconds"].(map[string]any)
	if timeout["minimum"] != float64(minWaitForReplySeconds) || timeout["maximum"] != float64(maxWaitForReplySeconds) {
		t.Fatalf("timeout schema = %#v", timeout)
	}
	if _, ok := arguments["after_seq"]; !ok {
		t.Fatalf("wait_for_reply arguments = %#v, want after_seq", arguments)
	}
	runAs := properties["runas"].(map[string]any)["properties"].(map[string]any)
	types := runAs["type"].(map[string]any)["enum"].([]any)
	if len(types) != 1 || types[0] != "user" {
		t.Fatalf("wait_for_reply runas types = %#v, want user", types)
	}
}

func TestConversationSchemasExposeCurrentRunAsAndConditionalConstraints(t *testing.T) {
	source := NewSource()
	tools, err := source.ListTools(context.Background())
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}
	var publicSchema map[string]any
	for _, tool := range tools {
		if tool.Name == conversationsToolName {
			publicSchema = tool.InputSchema.(map[string]any)
			break
		}
	}
	publicRunAs := publicSchema["properties"].(map[string]any)["runas"].(map[string]any)
	publicTypes := publicRunAs["properties"].(map[string]any)["type"].(map[string]any)["enum"].([]string)
	if !slices.Equal(publicTypes, []string{"user"}) {
		t.Fatalf("public conversations runas types = %#v, want user", publicTypes)
	}

	_, searchSchema := helpOperationForTest(t, source, conversationsOperationSearch)
	searchRequired := searchSchema["required"].([]any)
	if !slices.Contains(searchRequired, any("runas")) {
		t.Fatalf("search required = %#v, want runas", searchRequired)
	}
	searchRunAs := searchSchema["properties"].(map[string]any)["runas"].(map[string]any)
	if strings.Contains(searchRunAs["description"].(string), "可选") {
		t.Fatalf("required search runas description = %q", searchRunAs["description"])
	}

	_, readSchema := helpOperationForTest(t, source, conversationsOperationRead)
	readArguments := readSchema["properties"].(map[string]any)["arguments"].(map[string]any)
	if len(readArguments["oneOf"].([]any)) != 3 || readArguments["additionalProperties"] != false {
		t.Fatalf("read_history arguments = %#v, want three exclusive selectors and strict properties", readArguments)
	}

	_, sendSchema := helpOperationForTest(t, source, conversationsOperationSend)
	sendArguments := sendSchema["properties"].(map[string]any)["arguments"].(map[string]any)
	if len(sendArguments["allOf"].([]any)) != 2 || sendArguments["additionalProperties"] != false {
		t.Fatalf("send arguments = %#v, want message and target constraints", sendArguments)
	}
}

func TestAuthorizedConversationHelpDoesNotSuggestFakeAuthorizationRef(t *testing.T) {
	source := NewSource()
	result, err := source.CallTool(context.Background(), helpToolName, json.RawMessage(`{"capability":"conversations","operation":"search"}`))
	if err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}
	if strings.Contains(result.Content, "auth_12") || strings.Contains(result.Content, `"examples"`) {
		t.Fatalf("search help = %s, should not contain a copyable authorization example", result.Content)
	}
}

func TestReadHistoryToolCallsAppRequestWithConversationID(t *testing.T) {
	requester := &fakeRequester{}
	ctx := WithScope(context.Background(), Scope{
		CurrentUserID:    "user-1",
		Requester:        requester,
		TriggerMessageID: "message-1",
	})

	result, err := callReadHistory(ctx, json.RawMessage(`{"conversation_id":" conversation-1 ","before_seq":9,"limit":200}`))
	if err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}
	if result.Content != `{"ok":true}` {
		t.Fatalf("result = %q, want app response JSON", result.Content)
	}
	if len(requester.calls) != 1 {
		t.Fatalf("request call count = %d, want 1", len(requester.calls))
	}
	if requester.calls[0].method != methodConversationHistoryRead {
		t.Fatalf("method = %q, want %s", requester.calls[0].method, methodConversationHistoryRead)
	}
	var payload struct {
		ActorUserID      string `json:"actor_user_id"`
		BeforeSeq        int64  `json:"before_seq"`
		ConversationID   string `json:"conversation_id"`
		Limit            int    `json:"limit"`
		TriggerMessageID string `json:"trigger_message_id"`
	}
	if err := json.Unmarshal(requester.calls[0].payload, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload.ActorUserID != "user-1" ||
		payload.TriggerMessageID != "message-1" ||
		payload.ConversationID != "conversation-1" ||
		payload.BeforeSeq != 9 ||
		payload.Limit != 200 {
		t.Fatalf("payload = %#v, want conversation history request", payload)
	}
}

func TestReadHistoryToolCallsAppRequestWithUserIDOrAppID(t *testing.T) {
	for _, tt := range []struct {
		name     string
		input    string
		wantUser string
		wantApp  string
	}{
		{
			name:     "user id",
			input:    `{"user_id":" user-2 "}`,
			wantUser: "user-2",
		},
		{
			name:    "app id",
			input:   `{"app_id":" app-1 "}`,
			wantApp: "app-1",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			requester := &fakeRequester{}
			ctx := WithScope(context.Background(), Scope{
				CurrentUserID:    "user-1",
				Requester:        requester,
				TriggerMessageID: "message-1",
			})

			if _, err := callReadHistory(ctx, json.RawMessage(tt.input)); err != nil {
				t.Fatalf("CallTool() error = %v", err)
			}
			if len(requester.calls) != 1 {
				t.Fatalf("request call count = %d, want 1", len(requester.calls))
			}
			if requester.calls[0].method != methodConversationHistoryRead {
				t.Fatalf("method = %q, want %s", requester.calls[0].method, methodConversationHistoryRead)
			}
			var payload struct {
				AppID            string `json:"app_id"`
				ActorUserID      string `json:"actor_user_id"`
				Limit            int    `json:"limit"`
				TriggerMessageID string `json:"trigger_message_id"`
				UserID           string `json:"user_id"`
			}
			if err := json.Unmarshal(requester.calls[0].payload, &payload); err != nil {
				t.Fatalf("unmarshal payload: %v", err)
			}
			if payload.ActorUserID != "user-1" || payload.TriggerMessageID != "message-1" {
				t.Fatalf("payload context = %#v, want actor and trigger", payload)
			}
			if payload.UserID != tt.wantUser || payload.AppID != tt.wantApp {
				t.Fatalf("payload target = %#v, want user=%q app=%q", payload, tt.wantUser, tt.wantApp)
			}
			if payload.Limit != 0 {
				t.Fatalf("limit = %d, want omitted default 0", payload.Limit)
			}
		})
	}
}

func TestReadHistoryToolRejectsAmbiguousSelector(t *testing.T) {
	ctx := WithScope(context.Background(), Scope{
		CurrentUserID:    "user-1",
		Requester:        &fakeRequester{},
		TriggerMessageID: "message-1",
	})

	if _, err := callReadHistory(ctx, json.RawMessage(`{"conversation_id":"conversation-1","user_id":"user-2"}`)); err == nil {
		t.Fatal("CallTool() error = nil, want ambiguous selector error")
	}
}

func TestReadFileURLsToolCallsAppRequestWithFileIDsOnly(t *testing.T) {
	requester := &fakeRequester{
		handle: func(ctx context.Context, method string, payload any) (json.RawMessage, error) {
			var readPayload struct {
				FileIDs []string `json:"file_ids"`
			}
			rawPayload, err := json.Marshal(payload)
			if err != nil {
				t.Fatalf("marshal payload: %v", err)
			}
			var payloadMap map[string]any
			if err := json.Unmarshal(rawPayload, &payloadMap); err != nil {
				t.Fatalf("unmarshal payload map: %v", err)
			}
			if _, ok := payloadMap["conversation_id"]; ok {
				t.Fatalf("payload = %s, want file_ids only", rawPayload)
			}
			if err := json.Unmarshal(rawPayload, &readPayload); err != nil {
				t.Fatalf("unmarshal payload: %v", err)
			}
			if len(readPayload.FileIDs) > 1 {
				return nil, errors.New("not_found: temporary file not found")
			}
			if readPayload.FileIDs[0] == "file-missing" {
				return nil, errors.New("not_found: temporary file not found")
			}
			return json.Marshal(map[string]any{
				"urls": []map[string]any{
					{
						"file_id":    readPayload.FileIDs[0],
						"url":        "https://assets.example.test/" + readPayload.FileIDs[0],
						"expires_at": "2026-07-08T12:00:00Z",
					},
				},
			})
		},
	}
	ctx := WithScope(context.Background(), Scope{
		Requester: requester,
	})
	source := NewSource()

	result, err := source.CallTool(ctx, getAttachmentsToolName, json.RawMessage(`{"file_ids":[" file-ok ","file-missing","file-ok"]}`))
	if err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}
	if len(requester.calls) != 3 {
		t.Fatalf("request call count = %d, want batch plus individual fallbacks", len(requester.calls))
	}
	for _, call := range requester.calls {
		if call.method != methodTemporaryFilesReadURLs {
			t.Fatalf("method = %q, want %s", call.method, methodTemporaryFilesReadURLs)
		}
		var payload struct {
			FileIDs []string `json:"file_ids"`
		}
		if err := json.Unmarshal(call.payload, &payload); err != nil {
			t.Fatalf("unmarshal payload: %v", err)
		}
		var payloadMap map[string]any
		if err := json.Unmarshal(call.payload, &payloadMap); err != nil {
			t.Fatalf("unmarshal payload map: %v", err)
		}
		if _, ok := payloadMap["conversation_id"]; ok {
			t.Fatalf("payload = %s, want file_ids only", call.payload)
		}
	}

	var payload struct {
		URLs []struct {
			FileID string `json:"file_id"`
			URL    string `json:"url"`
		} `json:"urls"`
		Errors []struct {
			FileID string `json:"file_id"`
			Error  string `json:"error"`
		} `json:"errors"`
	}
	if err := json.Unmarshal([]byte(result.Content), &payload); err != nil {
		t.Fatalf("unmarshal result: %v; content=%q", err, result.Content)
	}
	if len(payload.URLs) != 1 || payload.URLs[0].FileID != "file-ok" || payload.URLs[0].URL != "https://assets.example.test/file-ok" {
		t.Fatalf("urls = %#v, want resolved file-ok", payload.URLs)
	}
	if len(payload.Errors) != 1 || payload.Errors[0].FileID != "file-missing" || payload.Errors[0].Error == "" {
		t.Fatalf("errors = %#v, want file-missing error", payload.Errors)
	}
}

func TestReplyToolCallsMessageSendForCurrentConversation(t *testing.T) {
	requester := &fakeRequester{}
	ctx := WithScope(context.Background(), Scope{
		ConversationID:   "conversation-1",
		ConversationType: "app",
		Requester:        requester,
	})

	_, err := callReply(ctx, json.RawMessage(`{"type":"image","content":"https://example.com/a.png"}`))
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

	_, err := callReply(ctx, json.RawMessage(`{"type":"file","name":"report.md","url":"https://example.com/report.md"}`))
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

	fileContent := "  # 报告\n\n正文\n"
	input, err := json.Marshal(map[string]any{
		"type":    "file",
		"name":    "assistant-report.md",
		"content": fileContent,
	})
	if err != nil {
		t.Fatalf("marshal input: %v", err)
	}
	_, err = callReply(ctx, input)
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

func TestReplyToolCallsMessageSendForCard(t *testing.T) {
	requester := &fakeRequester{}
	ctx := WithScope(context.Background(), Scope{
		ConversationID:   "conversation-1",
		ConversationType: "app",
		Requester:        requester,
	})

	result, err := callReply(ctx, json.RawMessage(`{
		"type":"card",
		"title":"任务标题",
		"description":"任务说明",
		"url":"/projects/project-1?taskId=task-1"
	}`))
	if err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}
	if result.Final {
		t.Fatal("card reply result.Final = true, want false so multiple cards can be sent")
	}
	var payload struct {
		Message scopedMessagePayload `json:"message"`
	}
	if err := json.Unmarshal(requester.calls[0].payload, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload.Message.Type != messageTypeCard || payload.Message.Title != "任务标题" || payload.Message.Description != "任务说明" {
		t.Fatalf("message = %#v, want card message", payload.Message)
	}
	if payload.Message.URL != "/projects/project-1?taskId=task-1" {
		t.Fatalf("message url = %q, want card URL", payload.Message.URL)
	}
}

func TestReplyToolSendsChoiceAndEndsTheCurrentCycle(t *testing.T) {
	requester := &fakeRequester{}
	ctx := WithScope(context.Background(), Scope{
		ConversationID: "conversation-1", ConversationType: "app", Requester: requester,
	})
	result, err := callReply(ctx, json.RawMessage(`{
		"type":"choice",
		"content_type":"markdown",
		"content":"**请选择项目**",
		"selection":"multiple",
		"options":[
			{"id":"project-a","label":"项目 A"},
			{"id":"project_b","label":"项目 B"}
		]
	}`))
	if err != nil {
		t.Fatalf("callReply() error = %v", err)
	}
	if !result.Final {
		t.Fatal("choice reply result.Final = false, want true")
	}
	if len(requester.calls) != 1 || requester.calls[0].method != methodMessageSend {
		t.Fatalf("request calls = %#v", requester.calls)
	}
	var payload sendMessagePayload
	if err := json.Unmarshal(requester.calls[0].payload, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload.Message.Type != messageTypeChoice || payload.Message.ContentType != messageTypeMarkdown ||
		payload.Message.Selection != "multiple" || len(payload.Message.Options) != 2 || payload.Message.Options[1].ID != "project_b" {
		t.Fatalf("choice payload = %#v", payload.Message)
	}
}

func TestReplyToolRejectsInvalidChoiceMessages(t *testing.T) {
	ctx := WithScope(context.Background(), Scope{
		ConversationID: "conversation-1", ConversationType: "app", Requester: &fakeRequester{},
	})
	for _, input := range []string{
		`{"type":"choice","content_type":"html","content":"请选择","selection":"single","options":[{"id":"a","label":"A"},{"id":"b","label":"B"}]}`,
		`{"type":"choice","content_type":"text","content":"请选择","selection":"any","options":[{"id":"a","label":"A"},{"id":"b","label":"B"}]}`,
		`{"type":"choice","content_type":"text","content":"请选择","selection":"single","options":[{"id":"a","label":"A"}]}`,
		`{"type":"choice","content_type":"text","content":"请选择","selection":"single","options":[{"id":"a b","label":"A"},{"id":"b","label":"B"}]}`,
		`{"type":"choice","content_type":"text","content":"请选择","selection":"single","options":[{"id":"a","label":"A"},{"id":"a","label":"B"}]}`,
	} {
		if _, err := callReply(ctx, json.RawMessage(input)); err == nil {
			t.Fatalf("callReply(%s) error = nil, want choice validation error", input)
		}
	}
}

func TestReplyToolRejectsIncompleteCard(t *testing.T) {
	ctx := WithScope(context.Background(), Scope{
		ConversationID:   "conversation-1",
		ConversationType: "app",
		Requester:        &fakeRequester{},
	})

	for _, input := range []string{
		`{"type":"card","description":"说明","url":"/projects/1"}`,
		`{"type":"card","title":"标题","url":"/projects/1"}`,
		`{"type":"card","title":"标题","description":"说明"}`,
	} {
		if _, err := callReply(ctx, json.RawMessage(input)); err == nil {
			t.Fatalf("CallTool(%s) error = nil, want invalid card message error", input)
		}
	}
}

func TestReplyToolCallsMessageSendForSupportedCharts(t *testing.T) {
	testCases := []struct {
		chartType string
		data      any
	}{
		{"line", map[string]any{"labels": []string{"周一", "周二"}, "series": []any{map[string]any{"name": "数量", "values": []any{12, 18}}}}},
		{"bar", map[string]any{"direction": "vertical", "mode": "grouped", "labels": []string{"一月"}, "series": []any{map[string]any{"name": "数量", "values": []any{12}}}}},
		{"pie", map[string]any{"items": []any{map[string]any{"name": "待办", "value": 12}, map[string]any{"name": "完成", "value": 8}}}},
		{"radar", map[string]any{"axes": []any{map[string]any{"name": "进度", "max": 100}, map[string]any{"name": "质量", "max": 100}, map[string]any{"name": "协作", "max": 100}}, "series": []any{map[string]any{"name": "本周", "values": []any{80, 92, 76}}}}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.chartType, func(t *testing.T) {
			requester := &fakeRequester{}
			ctx := WithScope(context.Background(), Scope{
				ConversationID:   "conversation-1",
				ConversationType: "app",
				Requester:        requester,
			})
			input, err := json.Marshal(map[string]any{
				"type":        messageTypeChart,
				"chart_type":  testCase.chartType,
				"title":       "项目趋势",
				"data":        testCase.data,
				"description": "单位：个，按自然日统计",
			})
			if err != nil {
				t.Fatalf("marshal chart input: %v", err)
			}
			result, err := callReply(ctx, input)
			if err != nil {
				t.Fatalf("CallTool() error = %v", err)
			}
			if result.Final {
				t.Fatal("chart reply result.Final = true, want false so analysis can continue")
			}
			var payload struct {
				Message scopedMessagePayload `json:"message"`
			}
			if err := json.Unmarshal(requester.calls[0].payload, &payload); err != nil {
				t.Fatalf("unmarshal payload: %v", err)
			}
			if payload.Message.Type != messageTypeChart || payload.Message.ChartType != testCase.chartType || payload.Message.Title != "项目趋势" || len(payload.Message.Data) == 0 {
				t.Fatalf("message = %#v, want %s chart", payload.Message, testCase.chartType)
			}
		})
	}
}

func TestReplyToolKeepsTextNonFinal(t *testing.T) {
	ctx := WithScope(context.Background(), Scope{
		ConversationID:   "conversation-1",
		ConversationType: "app",
		Requester:        &fakeRequester{},
	})
	result, err := callReply(ctx, json.RawMessage(`{"type":"text","content":"综合结论"}`))
	if err != nil {
		t.Fatalf("callReply() error = %v", err)
	}
	if result.Final {
		t.Fatal("text reply result.Final = true, want false so the model controls completion")
	}
}

func TestChartMessageDoesNotRequireExplicitUnit(t *testing.T) {
	_, err := parseChartMessageInput(messageInput{
		ChartType:   "line",
		Data:        json.RawMessage(`{"labels":["周一","周二"],"series":[{"name":"数量","values":[12,18]}]}`),
		Description: "按自然日统计",
		Title:       "消息趋势",
	})
	if err != nil {
		t.Fatalf("parseChartMessageInput() error = %v, unit guidance must not be enforced", err)
	}
}

func TestReplyToolRejectsInvalidChartEnvelope(t *testing.T) {
	ctx := WithScope(context.Background(), Scope{
		ConversationID:   "conversation-1",
		ConversationType: "app",
		Requester:        &fakeRequester{},
	})
	for _, input := range []string{
		`{"type":"chart","chart_type":"area","title":"趋势","description":"说明","data":{"labels":[]}}`,
		`{"type":"chart","chart_type":"line","description":"说明","data":{"labels":[]}}`,
		`{"type":"chart","chart_type":"line","title":"趋势","data":{"labels":[]}}`,
		`{"type":"chart","chart_type":"line","title":"这是一个超过十六个字符的图表消息标题","description":"说明","data":{"labels":[]}}`,
		`{"type":"chart","chart_type":"line","title":"趋势","description":"说明","data":null}`,
	} {
		if _, err := callReply(ctx, json.RawMessage(input)); err == nil {
			t.Fatalf("CallTool(%s) error = nil, want invalid chart error", input)
		}
	}
}

func TestReplyEntityCardUsesAuthorizedUserForResolutionAndAgentForSending(t *testing.T) {
	requester := &fakeRequester{}
	source := NewSource()
	ctx := WithScope(context.Background(), Scope{
		AuthorizationResolver: AuthorizationResolverFunc(func(ref string) (Authorization, bool) {
			if ref != "auth-1" {
				return Authorization{}, false
			}
			return Authorization{ActorID: "user-1", ActorType: "user", TriggerMessageID: "message-1"}, true
		}),
		ConversationID:   "conversation-1",
		ConversationType: "app",
		Requester:        requester,
	})

	result, err := source.CallTool(ctx, conversationsToolName, json.RawMessage(`{
		"operation":"reply_entity_card",
		"runas":{"type":"user","id":"user-1","authorization_ref":"auth-1"},
		"arguments":{"entity_type":"task","entity_id":"task-1"}
	}`))
	if err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}
	if result.Final {
		t.Fatal("result.Final = true, want false so multiple entity cards can be sent")
	}
	if len(requester.calls) != 1 || requester.calls[0].method != methodMessageSend {
		t.Fatalf("request calls = %#v", requester.calls)
	}
	var payload sendMessagePayload
	if err := json.Unmarshal(requester.calls[0].payload, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload.ActorUserID != "user-1" || payload.TriggerMessageID != "message-1" || payload.AuthorizationConversationID != "conversation-1" {
		t.Fatalf("payload authorization = %#v", payload)
	}
	if payload.Message.Type != messageTypeEntityCard || payload.Message.EntityType != "task" || payload.Message.EntityID != "task-1" {
		t.Fatalf("payload message = %#v", payload.Message)
	}
	if payload.Target.Type != "app" || payload.Target.ConversationID != "conversation-1" {
		t.Fatalf("payload target = %#v", payload.Target)
	}
}

func TestSendEntityCardUsesDelegatedSendingTarget(t *testing.T) {
	requester := &fakeRequester{}
	source := NewSource()
	ctx := WithScope(context.Background(), Scope{
		AuthorizationResolver: AuthorizationResolverFunc(func(ref string) (Authorization, bool) {
			return Authorization{ActorID: "user-1", ActorType: "user", TriggerMessageID: "message-1"}, ref == "auth-1"
		}),
		ConversationID: "conversation-1",
		Requester:      requester,
	})

	_, err := source.CallTool(ctx, conversationsToolName, json.RawMessage(`{
		"operation":"send_entity_card",
		"runas":{"type":"user","id":"user-1","authorization_ref":"auth-1"},
		"arguments":{
			"target_type":"group",
			"conversation_id":"conversation-2",
			"entity_type":"project",
			"entity_id":"project-1"
		}
	}`))
	if err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}
	if len(requester.calls) != 1 || requester.calls[0].method != methodMessageSendAsUser {
		t.Fatalf("request calls = %#v", requester.calls)
	}
	var payload sendAsUserPayload
	if err := json.Unmarshal(requester.calls[0].payload, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload.Target.Type != "group" || payload.Target.ConversationID != "conversation-2" {
		t.Fatalf("payload target = %#v", payload.Target)
	}
	if payload.Message.Type != messageTypeEntityCard || payload.Message.EntityType != "project" || payload.Message.EntityID != "project-1" {
		t.Fatalf("payload message = %#v", payload.Message)
	}
}

func TestReplyToolRejectsInvalidFileInputs(t *testing.T) {
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
			if _, err := callReply(ctx, json.RawMessage(tt.input)); err == nil {
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

	_, err := callSendAsUser(ctx, json.RawMessage(`{"contact_id":"user-2","type":"markdown","content":"**收到**"}`))
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

func TestSendAsUserToolCallsMessageSendAsUserForCard(t *testing.T) {
	requester := &fakeRequester{}
	ctx := WithScope(context.Background(), Scope{
		CurrentUserID:    "user-1",
		Requester:        requester,
		TriggerMessageID: "message-1",
	})

	_, err := callSendAsUser(ctx, json.RawMessage(`{
		"target_type":"group",
		"conversation_id":"conversation-2",
		"type":"card",
		"title":"任务标题",
		"description":"任务说明",
		"url":"/projects/project-1?taskId=task-1"
	}`))
	if err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}
	var payload struct {
		Message scopedMessagePayload `json:"message"`
		Target  sendAsUserTarget     `json:"target"`
	}
	if err := json.Unmarshal(requester.calls[0].payload, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload.Target.Type != "group" || payload.Target.ConversationID != "conversation-2" {
		t.Fatalf("target = %#v, want group conversation", payload.Target)
	}
	if payload.Message.Type != messageTypeCard || payload.Message.URL != "/projects/project-1?taskId=task-1" {
		t.Fatalf("message = %#v, want card message", payload.Message)
	}
}

func TestSendAsUserToolCallsMessageSendAsUserForChart(t *testing.T) {
	requester := &fakeRequester{}
	ctx := WithScope(context.Background(), Scope{
		CurrentUserID:    "user-1",
		Requester:        requester,
		TriggerMessageID: "message-1",
	})

	_, err := callSendAsUser(ctx, json.RawMessage(`{
		"target_type":"group",
		"conversation_id":"conversation-2",
		"type":"chart",
		"chart_type":"bar",
		"title":"任务对比",
		"description":"单位：个，按月份统计",
		"data":{"direction":"horizontal","mode":"stacked","labels":["一月"],"series":[{"name":"新增","values":[12]}]}
	}`))
	if err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}
	var payload struct {
		Message scopedMessagePayload `json:"message"`
		Target  sendAsUserTarget     `json:"target"`
	}
	if err := json.Unmarshal(requester.calls[0].payload, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload.Target.Type != "group" || payload.Target.ConversationID != "conversation-2" {
		t.Fatalf("target = %#v, want group conversation", payload.Target)
	}
	if payload.Message.Type != messageTypeChart || payload.Message.ChartType != "bar" || len(payload.Message.Data) == 0 {
		t.Fatalf("message = %#v, want bar chart", payload.Message)
	}
}

func TestConversationMessageHelpDescribesChartSchema(t *testing.T) {
	source := NewSource()
	for _, operation := range []string{conversationsOperationReply, conversationsOperationSend} {
		description, schema := helpOperationForTest(t, source, operation)
		for _, snippet := range []string{"chart", "line", "bar", "pie", "radar", "16", "128", "至少两项可信且可比较的数字", "多个互补图表", "建议在 description 中自然说明", "没有单位时无需机械填写"} {
			if !strings.Contains(description, snippet) {
				t.Fatalf("%s description = %q, want %q", operation, description, snippet)
			}
		}
		if operation == conversationsOperationReply {
			for _, snippet := range []string{"图表只是分析过程中的普通消息", "不代表回复结束", "最后再用 text/markdown 给出综合结论"} {
				if !strings.Contains(description, snippet) {
					t.Fatalf("%s description = %q, want %q", operation, description, snippet)
				}
			}
		}
		arguments := schema["properties"].(map[string]any)["arguments"].(map[string]any)
		properties := arguments["properties"].(map[string]any)
		for _, property := range []string{"chart_type", "title", "data", "description"} {
			if _, ok := properties[property]; !ok {
				t.Fatalf("%s chart schema missing %s", operation, property)
			}
		}
		descriptionField := properties["description"].(map[string]any)["description"].(string)
		for _, snippet := range []string{"建议在 description 中自然说明", "没有单位时无需机械填写"} {
			if !strings.Contains(descriptionField, snippet) {
				t.Fatalf("%s description field = %q, want %q", operation, descriptionField, snippet)
			}
		}
	}
}

func TestChartMessageSchemaLimitsNumericValues(t *testing.T) {
	pieItems := chartPieDataSchema()["properties"].(map[string]any)["items"].(map[string]any)
	pieValue := pieItems["items"].(map[string]any)["properties"].(map[string]any)["value"].(map[string]any)
	if pieValue["maximum"] != maxChartMessageValue {
		t.Fatalf("pie value schema = %#v", pieValue)
	}

	lineSeries := chartSeriesListSchema(2, 100, true)
	lineValues := lineSeries["items"].(map[string]any)["properties"].(map[string]any)["values"].(map[string]any)
	lineNumber := lineValues["items"].(map[string]any)["anyOf"].([]any)[0].(map[string]any)
	if lineNumber["minimum"] != -maxChartMessageValue || lineNumber["maximum"] != maxChartMessageValue {
		t.Fatalf("line value schema = %#v", lineNumber)
	}

	radarSeries := chartSeriesListSchema(3, 12, false)
	radarValues := radarSeries["items"].(map[string]any)["properties"].(map[string]any)["values"].(map[string]any)
	radarNumber := radarValues["items"].(map[string]any)
	if radarNumber["minimum"] != 0 || radarNumber["maximum"] != maxChartMessageValue {
		t.Fatalf("radar value schema = %#v", radarNumber)
	}
}

func TestSendAsUserToolResolvesAuthorizationRef(t *testing.T) {
	requester := &fakeRequester{}
	ctx := WithScope(context.Background(), Scope{
		AuthorizationResolver: AuthorizationResolverFunc(func(ref string) (Authorization, bool) {
			if ref != "auth_2" {
				return Authorization{}, false
			}
			return Authorization{
				ActorID:          "user-2",
				ActorType:        "user",
				TriggerMessageID: "message-2",
			}, true
		}),
		Requester: requester,
	})

	_, err := callSendAsUser(ctx, json.RawMessage(`{"authorization_ref":"auth_2","contact_id":"user-3","type":"markdown","content":"**收到**"}`))
	if err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}
	if len(requester.calls) != 1 {
		t.Fatalf("request call count = %d, want 1", len(requester.calls))
	}
	var payload struct {
		ActorUserID      string `json:"actor_user_id"`
		TargetUserID     string `json:"target_user_id"`
		TriggerMessageID string `json:"trigger_message_id"`
	}
	if err := json.Unmarshal(requester.calls[0].payload, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload.ActorUserID != "user-2" || payload.TriggerMessageID != "message-2" || payload.TargetUserID != "user-3" {
		t.Fatalf("payload = %#v, want authorization ref actor/trigger and target", payload)
	}
}

func TestAuthorizationRefRequiredWhenResolverConfigured(t *testing.T) {
	ctx := WithScope(context.Background(), Scope{
		AuthorizationResolver: AuthorizationResolverFunc(func(ref string) (Authorization, bool) {
			return Authorization{}, false
		}),
		Requester: &fakeRequester{},
	})

	if _, err := callSendAsUser(ctx, json.RawMessage(`{"contact_id":"user-2","type":"text","content":"hello"}`)); err == nil {
		t.Fatal("CallTool() error = nil, want missing authorization_ref error")
	}
	if _, err := callSendAsUser(ctx, json.RawMessage(`{"authorization_ref":"auth-missing","contact_id":"user-2","type":"text","content":"hello"}`)); err == nil {
		t.Fatal("CallTool() error = nil, want invalid authorization_ref error")
	}
}

func TestUserOnlyToolRejectsAppAuthorizationRef(t *testing.T) {
	requester := &fakeRequester{}
	ctx := WithScope(context.Background(), Scope{
		AuthorizationResolver: AuthorizationResolverFunc(func(ref string) (Authorization, bool) {
			return Authorization{ActorType: "app", ActorID: "app-1", TriggerMessageID: "message-1"}, true
		}),
		Requester: requester,
	})

	_, err := callRecentConversations(ctx, json.RawMessage(`{"authorization_ref":"auth-app"}`))
	if err == nil || !strings.Contains(err.Error(), "does not authorize a user") {
		t.Fatalf("CallTool() error = %v, want user-only authorization rejection", err)
	}
	if len(requester.calls) != 0 {
		t.Fatalf("request call count = %d, want 0", len(requester.calls))
	}
}

func TestSendAsUserToolCallsMessageSendAsUserForGroupConversation(t *testing.T) {
	requester := &fakeRequester{}
	ctx := WithScope(context.Background(), Scope{
		CurrentUserID:    "user-1",
		Requester:        requester,
		TriggerMessageID: "message-1",
	})

	_, err := callSendAsUser(ctx, json.RawMessage(`{"target_type":"group","conversation_id":"group-1","type":"text","content":"群里同步一下"}`))
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

	_, err := callCreateGroup(ctx, json.RawMessage(`{"name":"项目讨论组","member_ids":["user-2","user-3"]}`))
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

	_, err := callAddGroupMembers(ctx, json.RawMessage(`{"member_ids":["user-2","user-3"]}`))
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

func TestAddGroupMembersToolDefaultsToTopicParentGroup(t *testing.T) {
	requester := &fakeRequester{}
	ctx := WithScope(context.Background(), Scope{
		AuthorizationConversationID: "topic-1",
		AuthorizationResolver: AuthorizationResolverFunc(func(ref string) (Authorization, bool) {
			return Authorization{ActorID: "user-1", ActorType: "user", TriggerMessageID: "topic-message-1"}, ref == "auth_1"
		}),
		ConversationID:         "topic-1",
		ConversationType:       "topic",
		ParentConversationID:   "group-1",
		ParentConversationType: "group",
		Requester:              requester,
	})

	_, err := callAddGroupMembers(ctx, json.RawMessage(`{"authorization_ref":"auth_1","member_ids":["user-2"]}`))
	if err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}
	if len(requester.calls) != 1 {
		t.Fatalf("request call count = %d, want 1", len(requester.calls))
	}
	var payload struct {
		AuthorizationConversationID string `json:"authorization_conversation_id"`
		ConversationID              string `json:"conversation_id"`
	}
	if err := json.Unmarshal(requester.calls[0].payload, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload.ConversationID != "group-1" || payload.AuthorizationConversationID != "topic-1" {
		t.Fatalf("topic parent group payload = %#v", payload)
	}
}

func TestScopedToolsRequireScope(t *testing.T) {
	_, err := callReply(context.Background(), json.RawMessage(`{"type":"text","content":"hi"}`))
	if err == nil {
		t.Fatal("CallTool() error = nil, want missing scope error")
	}
}
