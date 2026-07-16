package builtintools

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

func TestProjectsOperationsUseAuthorizedUserRunAs(t *testing.T) {
	tests := []struct {
		operation string
		method    string
		arguments string
	}{
		{projectsOperationSearchProjects, methodProjectsList, `{}`},
		{projectsOperationCreateProject, methodProjectsCreate, `{"name":"发布项目"}`},
		{projectsOperationGrantGroupAccess, methodProjectGroupsGrant, `{"project_id":"project-1","conversation_id":"group-1"}`},
		{projectsOperationSearchTasks, methodProjectTasksList, `{"project_id":"project-1"}`},
		{projectsOperationCreateTask, methodProjectTasksCreate, `{"project_id":"project-1","title":"检查发布"}`},
		{projectsOperationUpdateTask, methodProjectTasksUpdate, `{"project_id":"project-1","task_id":"task-1","status":"done"}`},
	}
	for _, tt := range tests {
		t.Run(tt.operation, func(t *testing.T) {
			requester := &fakeRequester{}
			ctx := WithScope(context.Background(), Scope{
				AuthorizationResolver: AuthorizationResolverFunc(func(ref string) (Authorization, bool) {
					return Authorization{ActorType: "user", ActorID: "user-1", TriggerMessageID: "message-1"}, ref == "auth-user-1"
				}),
				ConversationID: "authorization-conversation-1",
				Requester:      requester,
			})
			input := `{"operation":"` + tt.operation + `","runas":{"type":"user","id":"user-1","authorization_ref":"auth-user-1"},"arguments":` + tt.arguments + `}`
			result, err := NewSource().CallTool(ctx, projectsToolName, json.RawMessage(input))
			if err != nil {
				t.Fatalf("CallTool() error = %v", err)
			}
			if result.Content != `{"ok":true}` || len(requester.calls) != 1 || requester.calls[0].method != tt.method {
				t.Fatalf("result = %#v, calls = %#v", result, requester.calls)
			}
			var payload map[string]any
			if err := json.Unmarshal(requester.calls[0].payload, &payload); err != nil {
				t.Fatalf("unmarshal payload: %v", err)
			}
			runAs := payload["runas"].(map[string]any)
			if runAs["type"] != "user" || runAs["id"] != "user-1" || runAs["trigger_message_id"] != "message-1" || runAs["authorization_conversation_id"] != "authorization-conversation-1" {
				t.Fatalf("runas = %#v", runAs)
			}
		})
	}
}

func TestProjectsRejectsMissingOrAppRunAs(t *testing.T) {
	requester := &fakeRequester{}
	ctx := WithScope(context.Background(), Scope{
		AuthorizationResolver: AuthorizationResolverFunc(func(ref string) (Authorization, bool) {
			return Authorization{ActorType: "app", ActorID: "app-1", TriggerMessageID: "message-1"}, ref == "auth-app-1"
		}),
		Requester: requester,
	})
	for _, input := range []json.RawMessage{
		json.RawMessage(`{"operation":"search_projects","arguments":{}}`),
		json.RawMessage(`{"operation":"search_projects","runas":{"type":"app","id":"app-1","authorization_ref":"auth-app-1"},"arguments":{}}`),
	} {
		if _, err := NewSource().CallTool(ctx, projectsToolName, input); err == nil {
			t.Fatalf("CallTool(%s) error = nil", input)
		}
	}
	if len(requester.calls) != 0 {
		t.Fatalf("calls = %#v, want none", requester.calls)
	}
}

func TestProjectsHelpExposesSixStrictUserOperations(t *testing.T) {
	source := NewSource()
	tools, err := source.ListTools(context.Background())
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}
	var schema map[string]any
	for _, tool := range tools {
		if tool.Name == projectsToolName {
			schema = tool.InputSchema.(map[string]any)
		}
	}
	if schema == nil {
		t.Fatal("projects tool missing")
	}
	required := schema["required"].([]string)
	for _, field := range []string{"operation", "runas", "arguments"} {
		if !slices.Contains(required, field) {
			t.Fatalf("required = %#v, want %s", required, field)
		}
	}
	properties := schema["properties"].(map[string]any)
	operations := properties["operation"].(map[string]any)["enum"].([]string)
	if len(operations) != 6 {
		t.Fatalf("operations = %#v, want six", operations)
	}
	types := properties["runas"].(map[string]any)["properties"].(map[string]any)["type"].(map[string]any)["enum"].([]string)
	if !slices.Equal(types, []string{"user"}) {
		t.Fatalf("runas types = %#v, want user", types)
	}

	result, err := source.CallTool(context.Background(), helpToolName, json.RawMessage(`{"capability":"projects","operation":"update_task"}`))
	if err != nil {
		t.Fatalf("help error = %v", err)
	}
	for _, snippet := range []string{"expected_updated_at", "assignee_user_id", "reminder", "day_of_month", "weekdays", "Asia/Shanghai", "additionalProperties", "authorization_ref"} {
		if !strings.Contains(result.Content, snippet) {
			t.Fatalf("help = %s, want %s", result.Content, snippet)
		}
	}

	result, err = source.CallTool(context.Background(), helpToolName, json.RawMessage(`{"capability":"projects","operation":"create_task"}`))
	if err != nil {
		t.Fatalf("create task help error = %v", err)
	}
	for _, snippet := range []string{"search_tasks", "相同或同义任务", "只适用于任务创建", "不得调用本操作", "update_task", "填写简洁、真实的 description"} {
		if !strings.Contains(result.Content, snippet) {
			t.Fatalf("create task help = %s, want %s", result.Content, snippet)
		}
	}
}
