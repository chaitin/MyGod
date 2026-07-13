package builtintools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"assistant/internal/mcpclient"
)

const (
	helpToolName            = "help"
	capabilityContacts      = "contacts"
	capabilityConversations = "conversations"
	capabilityProjects      = "projects"
	capabilitySchemaVersion = "1"
)

type capabilitySpec struct {
	Description string
	Name        string
	Summary     string
	Operations  []operationSpec
}

type operationSpec struct {
	Description     string
	Example         any
	InputSchema     map[string]any
	Name            string
	ToolDescription string
	ToolInputSchema map[string]any
	ToolName        string
}

type helpInput struct {
	Capability string `json:"capability"`
	Operation  string `json:"operation"`
}

func (s *Source) capabilitySpecs() []capabilitySpec {
	return []capabilitySpec{
		contactsCapabilitySpec(),
		conversationsCapabilitySpec(),
		projectsCapabilitySpec(),
	}
}

func contactsCapabilitySpec() capabilitySpec {
	toolDescription := "统一通讯录管理能力。所有操作都必须使用授权用户身份；runas.type 固定为 user，并携带与该用户匹配的 authorization_ref。具体操作和参数通过全局 help 查询。"
	toolSchema := capabilityToolInputSchema([]string{
		contactsOperationSearchUsers,
		contactsOperationSearchApps,
		contactsOperationSearchGroups,
	}, runAsInputSchema())
	toolSchema["required"] = []string{"operation", "runas"}
	return capabilitySpec{
		Description: "提供用户联系人、应用和群聊查询。所有查询都按授权用户的可见范围执行，必须传 type=user 的 runas 以及与该用户匹配的 authorization_ref。",
		Name:        capabilityContacts,
		Summary:     "查询通讯录中的用户、应用和群聊。",
		Operations: []operationSpec{
			{
				Description:     "查询 active 用户。keyword 按姓名、昵称、邮箱和手机号模糊匹配，为空时返回全部 active 用户。返回 id、type、name、nickname、email、phone、avatar、online 和 last_online_at。",
				Example:         map[string]any{"operation": contactsOperationSearchUsers, "arguments": map[string]any{"keyword": "张三"}},
				InputSchema:     contactsSearchOperationSchema(contactsOperationSearchUsers, "按姓名、昵称、邮箱或手机号搜索；为空返回全部 active 用户。"),
				Name:            contactsOperationSearchUsers,
				ToolDescription: toolDescription,
				ToolInputSchema: toolSchema,
				ToolName:        contactsToolName,
			},
			{
				Description:     "查询授权用户可见的 enabled 应用。keyword 按应用名称和描述模糊匹配，为空时返回全部可见应用。用户可见 public 应用和自己创建的 creator 应用。返回 id、type、name、description、avatar 和 online。",
				Example:         map[string]any{"operation": contactsOperationSearchApps, "arguments": map[string]any{"keyword": "助手"}},
				InputSchema:     contactsSearchOperationSchema(contactsOperationSearchApps, "按应用名称或描述搜索；为空返回当前执行身份可见的全部 enabled 应用。"),
				Name:            contactsOperationSearchApps,
				ToolDescription: toolDescription,
				ToolInputSchema: toolSchema,
				ToolName:        contactsToolName,
			},
			{
				Description:     "查询授权用户可见的 active 群聊。keyword 按群聊名称模糊匹配，为空时返回全部可见群聊。结果包括 public 群聊和该用户仍是成员的 private 群聊，不包括已退出的 private 群聊和 dissolved 群聊。返回 id、type、name、avatar、visibility、joined 和 member_count。",
				Example:         map[string]any{"operation": contactsOperationSearchGroups, "arguments": map[string]any{"keyword": "项目"}},
				InputSchema:     contactsSearchOperationSchema(contactsOperationSearchGroups, "按群聊名称搜索；为空返回当前执行身份可见的全部 active 群聊。"),
				Name:            contactsOperationSearchGroups,
				ToolDescription: toolDescription,
				ToolInputSchema: toolSchema,
				ToolName:        contactsToolName,
			},
		},
	}
}

func conversationsCapabilitySpec() capabilitySpec {
	toolDescription := "统一会话管理能力。支持查询会话、读取历史、回复当前会话、授权用户代发、等待新回复、创建群聊和添加成员；具体 operation 和参数通过全局 help 查询。"
	toolSchema := capabilityToolInputSchema([]string{
		conversationsOperationSearch,
		conversationsOperationRead,
		conversationsOperationReply,
		conversationsOperationSend,
		conversationsOperationWait,
		conversationsOperationCreate,
		conversationsOperationAdd,
	}, conversationPublicRunAsInputSchema())
	return capabilitySpec{
		Description: "提供最近会话查询、聊天历史读取、当前会话回复、授权用户代发、等待会话新回复，以及群聊创建和成员添加。操作统一通过 conversations 工具执行；需要授权用户身份的操作在顶层传 runas。",
		Name:        capabilityConversations,
		Summary:     "管理会话、消息和群聊，并等待新回复。",
		Operations: []operationSpec{
			{
				Description:     "查询授权用户最近使用的会话，包括私聊、群聊和应用会话。keyword 按会话名称或私聊对象的姓名、昵称匹配；limit 默认 20，最大 100。返回会话 ID、类型、名称、成员数量 member_count 和最近活动时间 last_active_at。",
				InputSchema:     conversationUserOperationInputSchema(conversationsOperationSearch, recentConversationsArgumentsSchema(), true),
				Name:            conversationsOperationSearch,
				ToolDescription: toolDescription,
				ToolInputSchema: toolSchema,
				ToolName:        conversationsToolName,
			},
			{
				Description:     "读取授权用户有权访问的聊天记录。conversation_id、user_id 和 app_id 三选一；before_seq 用于读取更早消息，limit 默认 20，最大 100。返回会话信息和消息列表；图片及附件消息只返回 file_id。",
				InputSchema:     conversationUserOperationInputSchema(conversationsOperationRead, readHistoryArgumentsSchema(), true),
				Name:            conversationsOperationRead,
				ToolDescription: toolDescription,
				ToolInputSchema: toolSchema,
				ToolName:        conversationsToolName,
			},
			{
				Description:     "回复当前触发 Assistant 的会话。支持 text、markdown、image 和 file；text/markdown 的 content 可使用 {(@user/用户UUID)} @ 用户、{(@app/应用UUID)} @ 应用、{(@user/all)} @ 全体用户，指定对象必须是当前会话成员；image 使用可下载 URL，file 使用显式文件名以及 url 或小文本 content。返回消息发送结果。",
				Example:         conversationExample(conversationsOperationReply, map[string]any{"type": "text", "content": "收到"}),
				InputSchema:     conversationOperationInputSchema(conversationsOperationReply, messageArgumentsSchema(false), false, false),
				Name:            conversationsOperationReply,
				ToolDescription: toolDescription,
				ToolInputSchema: toolSchema,
				ToolName:        conversationsToolName,
			},
			{
				Description:     "以授权用户身份向私聊联系人或已有群聊发送消息。target_type=user 时使用 contact_id，target_type=group 时使用 conversation_id；支持 text、markdown、image 和 file。text/markdown 的 content 可使用 {(@user/用户UUID)} @ 用户、{(@app/应用UUID)} @ 应用、{(@user/all)} @ 全体用户，UUID 必须可信且指定对象必须是目标会话成员。返回消息发送结果。",
				InputSchema:     conversationUserOperationInputSchema(conversationsOperationSend, messageArgumentsSchema(true), true),
				Name:            conversationsOperationSend,
				ToolDescription: toolDescription,
				ToolInputSchema: toolSchema,
				ToolName:        conversationsToolName,
			},
			{
				Description:     "以授权用户身份等待指定会话在 after_seq 之后出现新的用户或应用消息。Agent 内部立即查询一次，之后每 5 秒读取该用户有权访问的最新 30 条消息；timeout_seconds 范围为 5 到 60。匹配回复由当前代聊工作流认领，不再作为独立 Agent 消息触发。收到回复时最多返回 30 条，超时时返回 status=timeout。",
				Example:         conversationExample(conversationsOperationWait, map[string]any{"conversation_id": "conversation-id", "after_seq": 128, "timeout_seconds": 60}),
				InputSchema:     conversationUserOperationInputSchema(conversationsOperationWait, waitForReplyArgumentsSchema(), true),
				Name:            conversationsOperationWait,
				ToolDescription: toolDescription,
				ToolInputSchema: toolSchema,
				ToolName:        conversationsToolName,
			},
			{
				Description:     "以授权用户身份创建新群聊。name 为群名，member_ids 为联系人用户 ID；授权用户自动成为群主。返回创建后的群聊信息。",
				InputSchema:     conversationUserOperationInputSchema(conversationsOperationCreate, createGroupArgumentsSchema(), true),
				Name:            conversationsOperationCreate,
				ToolDescription: toolDescription,
				ToolInputSchema: toolSchema,
				ToolName:        conversationsToolName,
			},
			{
				Description:     "以授权用户身份向已有群聊添加用户成员。member_ids 为联系人用户 ID；当前会话是目标群聊时 conversation_id 可以省略。返回更新后的群聊信息。",
				InputSchema:     conversationUserOperationInputSchema(conversationsOperationAdd, addMembersArgumentsSchema(), true),
				Name:            conversationsOperationAdd,
				ToolDescription: toolDescription,
				ToolInputSchema: toolSchema,
				ToolName:        conversationsToolName,
			},
		},
	}
}

func projectsCapabilitySpec() capabilitySpec {
	operations := []string{
		projectsOperationSearchProjects,
		projectsOperationCreateProject,
		projectsOperationGrantGroupAccess,
		projectsOperationSearchTasks,
		projectsOperationCreateTask,
		projectsOperationUpdateTask,
	}
	toolDescription := "统一项目管理能力。支持查询和创建项目、将项目授权给群聊，以及查询、创建和修改任务。所有操作都必须使用授权用户身份；具体 operation 和参数通过全局 help 查询。"
	toolSchema := capabilityToolInputSchema(operations, runAsInputSchema())
	toolSchema["required"] = []string{"operation", "runas", "arguments"}
	return capabilitySpec{
		Name:        capabilityProjects,
		Summary:     "按授权用户权限管理项目、群授权和任务。",
		Description: "提供项目和任务的查询及写入能力。六个操作全部按授权用户权限执行，必须传 type=user 的 runas 和匹配的 authorization_ref；Agent 不使用自身身份访问项目数据。",
		Operations: []operationSpec{
			{
				Name:            projectsOperationSearchProjects,
				Description:     "查询授权用户可访问的个人项目和协作项目。conversation_context.project_context 只是当前语境的优先候选，不是完整清单；需要查找其中未列出的项目时使用本操作。keyword 按项目名称和描述匹配；limit 默认 50，最大 100；cursor 用于继续分页。返回项目 ID、名称、描述、所有者、当前用户角色、群数、成员数、任务状态统计和更新时间。",
				InputSchema:     projectOperationInputSchema(projectsOperationSearchProjects, searchProjectsArgumentsSchema()),
				ToolName:        projectsToolName,
				ToolDescription: toolDescription,
				ToolInputSchema: toolSchema,
			},
			{
				Name:            projectsOperationCreateProject,
				Description:     "以授权用户身份创建普通项目，该用户成为项目所有者。name 必填，description 和 avatar 可选；群授权通过 grant_group_access 单独执行。返回完整项目详情。",
				InputSchema:     projectOperationInputSchema(projectsOperationCreateProject, createProjectArgumentsSchema()),
				ToolName:        projectsToolName,
				ToolDescription: toolDescription,
				ToolInputSchema: toolSchema,
			},
			{
				Name:            projectsOperationGrantGroupAccess,
				Description:     "将授权用户拥有的普通项目授权给一个 active 群聊。授权用户必须是项目所有者且仍是目标群成员；个人项目不能授权。当前可信会话是目标群时可直接使用 conversation.id，其他群使用 contacts.search_groups 返回的群聊 ID。重复授权保持成功，并返回 already_granted。",
				InputSchema:     projectOperationInputSchema(projectsOperationGrantGroupAccess, grantGroupAccessArgumentsSchema()),
				ToolName:        projectsToolName,
				ToolDescription: toolDescription,
				ToolInputSchema: toolSchema,
			},
			{
				Name:            projectsOperationSearchTasks,
				Description:     "查询授权用户有权访问的指定项目任务，也用于创建任务前检查相同或同义的已有任务。project_id 可直接使用可信 project_context 中的候选；否则先查询项目。支持标题/描述关键词、状态、优先级、负责人、标签、开始日期和截止日期范围筛选；limit 默认 50，最大 100。返回任务详情、负责人、创建人、updated_at 和分页游标。",
				InputSchema:     projectOperationInputSchema(projectsOperationSearchTasks, searchTasksArgumentsSchema()),
				ToolName:        projectsToolName,
				ToolDescription: toolDescription,
				ToolInputSchema: toolSchema,
			},
			{
				Name:            projectsOperationCreateTask,
				Description:     "以授权用户身份在其有权访问的项目中创建任务。本查重要求只适用于任务创建：调用前必须先在同一项目使用 search_tasks 检查相同或同义任务；确认重复时不得调用本操作，应使用已有任务的 task_id 和 updated_at 调用 update_task，只在没有重复任务时创建。创建时应尽量根据用户请求和必要聊天背景填写简洁、真实的 description，不复制整段聊天或编造信息。project_id 可直接使用按系统项目选择规则确定的可信 project_context 候选；否则先查询项目。title 必填；status 默认 todo，priority 默认 2；负责人必须是有项目访问权的 active 用户。返回创建后的完整任务。",
				InputSchema:     projectOperationInputSchema(projectsOperationCreateTask, createTaskArgumentsSchema()),
				ToolName:        projectsToolName,
				ToolDescription: toolDescription,
				ToolInputSchema: toolSchema,
			},
			{
				Name:            projectsOperationUpdateTask,
				Description:     "以授权用户身份修改其有权访问的项目任务，只更新明确传入的字段。assignee_user_id、start_date、due_date 传 null 可清除，labels 传空数组可清空；至少提供一个修改字段。expected_updated_at 可用于并发保护，冲突时需重新查询。返回更新后的完整任务。",
				InputSchema:     projectOperationInputSchema(projectsOperationUpdateTask, updateTaskArgumentsSchema()),
				ToolName:        projectsToolName,
				ToolDescription: toolDescription,
				ToolInputSchema: toolSchema,
			},
		},
	}
}

func (s *Source) listedTools() []mcpclient.Tool {
	tools := []mcpclient.Tool{{
		Name:        helpToolName,
		Description: "查询内置能力、支持的操作及具体调用参数。本工具只返回说明，不执行业务操作。不传参数列出全部能力；传 capability 查看该能力；同时传 capability 和 operation 查看完整调用 schema。",
		InputSchema: helpInputSchema(s.capabilitySpecs()),
	}, {
		Name:        sleepToolName,
		Description: "等待指定秒数，用于等待异步任务完成或状态更新后继续处理。每次可等待 5 到 30 秒；不要用于普通回复或无目的等待。",
		InputSchema: sleepInputSchema(),
	}, {
		Name:        getAttachmentsToolName,
		Description: "按需将当前消息或历史消息中的 file_id 转换为临时可访问 URL，不需要会话 ID。一次可处理多个 file_id；部分失败时仍返回成功生成的 URL。",
		InputSchema: readFileURLsInputSchema(),
	}, {
		Name:        endConversationToolName,
		Description: "结束当前 Agent 对话。仅在用户明确要求结束当前对话时调用；调用后回复“已结束”、立即结束当前处理并清除当前持久上下文，下一条消息将开启新对话。",
		InputSchema: map[string]any{
			"type":                 "object",
			"properties":           map[string]any{},
			"additionalProperties": false,
		},
	}}
	seen := map[string]bool{helpToolName: true, sleepToolName: true, getAttachmentsToolName: true, endConversationToolName: true}
	for _, capability := range s.capabilitySpecs() {
		for _, operation := range capability.Operations {
			if seen[operation.ToolName] {
				continue
			}
			seen[operation.ToolName] = true
			tools = append(tools, mcpclient.Tool{
				Name:        operation.ToolName,
				Description: operation.ToolDescription,
				InputSchema: operation.ToolInputSchema,
			})
		}
	}
	return tools
}

func (s *Source) callHelp(ctx context.Context, input json.RawMessage) (mcpclient.ToolResult, error) {
	if err := ctx.Err(); err != nil {
		return mcpclient.ToolResult{}, err
	}
	var parsed helpInput
	if len(input) > 0 {
		if err := json.Unmarshal(input, &parsed); err != nil {
			return mcpclient.ToolResult{}, fmt.Errorf("parse help input: %w", err)
		}
	}
	parsed.Capability = strings.ToLower(strings.TrimSpace(parsed.Capability))
	parsed.Operation = strings.ToLower(strings.TrimSpace(parsed.Operation))
	if parsed.Capability == "" && parsed.Operation != "" {
		return mcpclient.ToolResult{}, fmt.Errorf("capability is required when operation is provided")
	}

	specs := s.capabilitySpecs()
	if parsed.Capability == "" {
		capabilities := make([]map[string]any, 0, len(specs))
		for _, spec := range specs {
			capabilities = append(capabilities, map[string]any{"name": spec.Name, "summary": spec.Summary})
		}
		return jsonToolResult(map[string]any{
			"kind":           "capability_list",
			"schema_version": capabilitySchemaVersion,
			"capabilities":   capabilities,
		})
	}

	capability, ok := findCapabilitySpec(specs, parsed.Capability)
	if !ok {
		return mcpclient.ToolResult{}, fmt.Errorf("unknown capability %q", parsed.Capability)
	}
	if parsed.Operation == "" {
		operations := make([]map[string]any, 0, len(capability.Operations))
		for _, operation := range capability.Operations {
			operations = append(operations, map[string]any{
				"name":        operation.Name,
				"description": operation.Description,
			})
		}
		return jsonToolResult(map[string]any{
			"kind":           "capability",
			"schema_version": capabilitySchemaVersion,
			"capability": map[string]any{
				"name":        capability.Name,
				"summary":     capability.Summary,
				"description": capability.Description,
				"operations":  operations,
			},
		})
	}

	operation, ok := findOperationSpec(capability, parsed.Operation)
	if !ok {
		return mcpclient.ToolResult{}, fmt.Errorf("unknown operation %q for capability %q", parsed.Operation, parsed.Capability)
	}
	result := map[string]any{
		"kind":           "operation",
		"schema_version": capabilitySchemaVersion,
		"capability":     capability.Name,
		"operation":      operation.Name,
		"description":    operation.Description,
		"tool":           sourceName + "__" + operation.ToolName,
		"input_schema":   operation.InputSchema,
	}
	if operation.Example != nil {
		result["examples"] = []any{operation.Example}
	}
	return jsonToolResult(result)
}

func helpInputSchema(specs []capabilitySpec) map[string]any {
	capabilities := make([]string, 0, len(specs))
	for _, spec := range specs {
		capabilities = append(capabilities, spec.Name)
	}
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"capability": map[string]any{
				"type":        "string",
				"enum":        capabilities,
				"description": "可选能力名称；省略时列出全部能力。",
			},
			"operation": map[string]any{
				"type":        "string",
				"description": "可选具体操作名称；必须和 capability 一起使用。",
			},
		},
		"additionalProperties": false,
	}
}

func capabilityToolInputSchema(operations []string, runAsSchema map[string]any) map[string]any {
	properties := map[string]any{
		"operation": map[string]any{"type": "string", "enum": operations, "description": "要执行的操作；通过全局 help 查询具体参数。"},
		"arguments": map[string]any{"type": "object", "description": "当前 operation 的业务参数；具体字段通过 operation 级 help 查询。", "additionalProperties": true},
	}
	if runAsSchema != nil {
		properties["runas"] = runAsSchema
	}
	return map[string]any{
		"type":                 "object",
		"required":             []string{"operation"},
		"properties":           properties,
		"additionalProperties": false,
	}
}

func contactsSearchOperationSchema(operation string, keywordDescription string) map[string]any {
	return map[string]any{
		"type":     "object",
		"required": []string{"operation", "runas"},
		"properties": map[string]any{
			"operation": map[string]any{"type": "string", "enum": []string{operation}},
			"runas":     runAsInputSchema(),
			"arguments": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"keyword": map[string]any{"type": "string", "description": keywordDescription},
				},
				"additionalProperties": false,
			},
		},
		"additionalProperties": false,
	}
}

func conversationOperationInputSchema(operation string, argumentsSchema map[string]any, requireRunAs bool, allowRunAs bool) map[string]any {
	required := []string{"operation", "arguments"}
	properties := map[string]any{
		"operation": map[string]any{"type": "string", "enum": []string{operation}},
		"arguments": argumentsSchema,
	}
	if allowRunAs {
		properties["runas"] = runAsInputSchema()
		if requireRunAs {
			required = append(required, "runas")
		}
	}
	return map[string]any{
		"type":                 "object",
		"required":             required,
		"properties":           properties,
		"additionalProperties": false,
	}
}

func conversationUserOperationInputSchema(operation string, argumentsSchema map[string]any, requireRunAs bool) map[string]any {
	schema := conversationOperationInputSchema(operation, argumentsSchema, requireRunAs, true)
	properties := schema["properties"].(map[string]any)
	runAs := runAsInputSchema()
	if requireRunAs {
		runAs["description"] = "必填授权用户执行身份；type 必须为 user，id 和 authorization_ref 必须与当前授权候选完全匹配。"
	}
	runAsProperties := runAs["properties"].(map[string]any)
	runAsProperties["type"].(map[string]any)["enum"] = []string{"user"}
	properties["runas"] = runAs
	return schema
}

func conversationPublicRunAsInputSchema() map[string]any {
	runAs := runAsInputSchema()
	runAs["description"] = "除 reply 外，会话操作都必须提供授权用户执行身份；type 固定为 user。reply 不接受 runas。"
	properties := runAs["properties"].(map[string]any)
	properties["type"].(map[string]any)["enum"] = []string{"user"}
	return runAs
}

func conversationExample(operation string, arguments map[string]any) map[string]any {
	return map[string]any{
		"operation": operation,
		"arguments": arguments,
	}
}

func recentConversationsArgumentsSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"keyword": map[string]any{"type": "string", "description": "按会话名称，或私聊对象的姓名、昵称搜索。"},
			"limit":   map[string]any{"type": "integer", "minimum": 1, "maximum": 100},
		},
		"additionalProperties": false,
	}
}

func readHistoryArgumentsSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"conversation_id": map[string]any{"type": "string", "minLength": 1, "description": "指定会话 ID。与 user_id、app_id 三选一。"},
			"user_id":         map[string]any{"type": "string", "minLength": 1, "description": "联系人用户 ID。与 conversation_id、app_id 三选一。"},
			"app_id":          map[string]any{"type": "string", "minLength": 1, "description": "应用 ID。与 conversation_id、user_id 三选一。"},
			"before_seq":      map[string]any{"type": "integer", "minimum": 1, "description": "读取 seq 小于该值的更早消息；省略时读取最新消息。"},
			"limit":           map[string]any{"type": "integer", "minimum": 1, "maximum": 100},
		},
		"oneOf": []any{
			map[string]any{"required": []string{"conversation_id"}},
			map[string]any{"required": []string{"user_id"}},
			map[string]any{"required": []string{"app_id"}},
		},
		"additionalProperties": false,
	}
}

func messageArgumentsSchema(withTarget bool) map[string]any {
	properties := map[string]any{
		"type": map[string]any{"type": "string", "enum": []string{messageTypeText, messageTypeMarkdown, messageTypeImage, messageTypeFile}},
		"content": map[string]any{
			"type":        "string",
			"minLength":   1,
			"description": "消息内容。text/markdown 中可嵌入精确 @ token：{(@user/用户UUID)}、{(@app/应用UUID)} 或 {(@user/all)}；UUID 必须来自可信上下文，指定对象必须是目标会话成员。image 时为可下载 URL；file 且没有 url 时为小文本文件内容。",
		},
		"name": map[string]any{"type": "string", "minLength": 1, "maxLength": 255},
		"url":  map[string]any{"type": "string", "minLength": 1},
	}
	messageConstraint := map[string]any{
		"oneOf": []any{
			map[string]any{
				"properties": map[string]any{"type": map[string]any{"enum": []string{messageTypeText, messageTypeMarkdown, messageTypeImage}}},
				"required":   []string{"type", "content"},
				"not": map[string]any{"anyOf": []any{
					map[string]any{"required": []string{"name"}},
					map[string]any{"required": []string{"url"}},
				}},
			},
			map[string]any{
				"properties": map[string]any{"type": map[string]any{"enum": []string{messageTypeFile}}},
				"required":   []string{"type", "name"},
				"oneOf": []any{
					map[string]any{"required": []string{"url"}, "not": map[string]any{"required": []string{"content"}}},
					map[string]any{"required": []string{"content"}, "not": map[string]any{"required": []string{"url"}}},
				},
			},
		},
	}
	constraints := []any{messageConstraint}
	if withTarget {
		properties["target_type"] = map[string]any{"type": "string", "enum": []string{"user", "group"}}
		properties["contact_id"] = map[string]any{"type": "string", "minLength": 1}
		properties["conversation_id"] = map[string]any{"type": "string", "minLength": 1}
		constraints = append(constraints, map[string]any{
			"oneOf": []any{
				map[string]any{
					"properties": map[string]any{"target_type": map[string]any{"enum": []string{"user"}}},
					"required":   []string{"target_type", "contact_id"},
					"not":        map[string]any{"required": []string{"conversation_id"}},
				},
				map[string]any{
					"properties": map[string]any{"target_type": map[string]any{"enum": []string{"group"}}},
					"required":   []string{"target_type", "conversation_id"},
					"not":        map[string]any{"required": []string{"contact_id"}},
				},
			},
		})
	}
	return map[string]any{
		"type":                 "object",
		"properties":           properties,
		"allOf":                constraints,
		"additionalProperties": false,
	}
}

func createGroupArgumentsSchema() map[string]any {
	return map[string]any{
		"type":     "object",
		"required": []string{"name", "member_ids"},
		"properties": map[string]any{
			"name": map[string]any{"type": "string", "minLength": 1},
			"member_ids": map[string]any{
				"type": "array", "minItems": 1, "uniqueItems": true,
				"items": map[string]any{"type": "string", "minLength": 1},
			},
		},
		"additionalProperties": false,
	}
}

func addMembersArgumentsSchema() map[string]any {
	return map[string]any{
		"type":     "object",
		"required": []string{"member_ids"},
		"properties": map[string]any{
			"conversation_id": map[string]any{"type": "string", "minLength": 1},
			"member_ids": map[string]any{
				"type": "array", "minItems": 1, "uniqueItems": true,
				"items": map[string]any{"type": "string", "minLength": 1},
			},
		},
		"additionalProperties": false,
	}
}

func waitForReplyArgumentsSchema() map[string]any {
	return map[string]any{
		"type":     "object",
		"required": []string{"conversation_id", "after_seq", "timeout_seconds"},
		"properties": map[string]any{
			"conversation_id": map[string]any{
				"type":        "string",
				"minLength":   1,
				"description": "等待新回复的会话 ID。",
			},
			"after_seq": map[string]any{
				"type":        "integer",
				"minimum":     1,
				"description": "只接收 seq 大于该值的新回复；通常传 conversations.send 返回的消息 seq。",
			},
			"timeout_seconds": map[string]any{
				"type":        "integer",
				"minimum":     minWaitForReplySeconds,
				"maximum":     maxWaitForReplySeconds,
				"description": "最长等待秒数，范围为 5 到 60。",
			},
		},
		"additionalProperties": false,
	}
}

func projectOperationInputSchema(operation string, argumentsSchema map[string]any) map[string]any {
	return conversationUserOperationInputSchema(operation, argumentsSchema, true)
}

func searchProjectsArgumentsSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"keyword": map[string]any{"type": "string", "description": "按项目名称或描述搜索；省略时返回全部可访问项目。"},
			"limit":   map[string]any{"type": "integer", "minimum": 1, "maximum": 100},
			"cursor":  map[string]any{"type": "string", "minLength": 1, "description": "上一页返回的 next_cursor。"},
		},
		"additionalProperties": false,
	}
}

func createProjectArgumentsSchema() map[string]any {
	return map[string]any{
		"type":     "object",
		"required": []string{"name"},
		"properties": map[string]any{
			"name":        map[string]any{"type": "string", "minLength": 1, "maxLength": 120},
			"description": map[string]any{"type": "string"},
			"avatar":      map[string]any{"type": "string", "maxLength": 512},
		},
		"additionalProperties": false,
	}
}

func grantGroupAccessArgumentsSchema() map[string]any {
	return map[string]any{
		"type":     "object",
		"required": []string{"project_id", "conversation_id"},
		"properties": map[string]any{
			"project_id":      map[string]any{"type": "string", "minLength": 1},
			"conversation_id": map[string]any{"type": "string", "minLength": 1, "description": "目标群聊 ID。"},
		},
		"additionalProperties": false,
	}
}

func searchTasksArgumentsSchema() map[string]any {
	return map[string]any{
		"type":     "object",
		"required": []string{"project_id"},
		"properties": map[string]any{
			"project_id": map[string]any{"type": "string", "minLength": 1},
			"keyword":    map[string]any{"type": "string", "description": "按任务标题或描述搜索。"},
			"statuses":   stringArraySchema([]string{"todo", "in_progress", "done", "canceled"}),
			"priorities": map[string]any{"type": "array", "uniqueItems": true, "items": map[string]any{"type": "integer", "enum": []int{1, 2, 3}}},
			"assignee_user_ids": map[string]any{
				"type": "array", "uniqueItems": true, "items": map[string]any{"type": "string", "minLength": 1},
			},
			"label":           map[string]any{"type": "string", "minLength": 1, "maxLength": 32},
			"start_date_from": dateStringSchema("开始日期下限。"),
			"start_date_to":   dateStringSchema("开始日期上限。"),
			"due_date_from":   dateStringSchema("截止日期下限。"),
			"due_date_to":     dateStringSchema("截止日期上限。"),
			"limit":           map[string]any{"type": "integer", "minimum": 1, "maximum": 100},
			"cursor":          map[string]any{"type": "string", "minLength": 1, "description": "上一页返回的 next_cursor。"},
		},
		"additionalProperties": false,
	}
}

func createTaskArgumentsSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"required":             []string{"project_id", "title"},
		"properties":           taskMutationProperties(true),
		"additionalProperties": false,
	}
}

func updateTaskArgumentsSchema() map[string]any {
	properties := taskMutationProperties(false)
	properties["task_id"] = map[string]any{"type": "string", "minLength": 1}
	properties["expected_updated_at"] = map[string]any{"type": "string", "format": "date-time", "description": "可选并发校验值，通常使用 search_tasks 返回的 updated_at。"}
	mutable := []string{"title", "description", "status", "priority", "assignee_user_id", "start_date", "due_date", "labels"}
	anyOf := make([]any, 0, len(mutable))
	for _, field := range mutable {
		anyOf = append(anyOf, map[string]any{"required": []string{field}})
	}
	return map[string]any{
		"type":                 "object",
		"required":             []string{"project_id", "task_id"},
		"properties":           properties,
		"anyOf":                anyOf,
		"additionalProperties": false,
	}
}

func taskMutationProperties(create bool) map[string]any {
	properties := map[string]any{
		"project_id":       map[string]any{"type": "string", "minLength": 1},
		"title":            map[string]any{"type": "string", "minLength": 1, "maxLength": 240},
		"description":      map[string]any{"type": "string"},
		"status":           map[string]any{"type": "string", "enum": []string{"todo", "in_progress", "done", "canceled"}},
		"priority":         map[string]any{"type": "integer", "enum": []int{1, 2, 3}},
		"assignee_user_id": nullableStringSchema("负责人用户 ID；null 表示不指定或清除负责人。"),
		"start_date":       nullableDateSchema("开始日期；null 表示不指定或清除。"),
		"due_date":         nullableDateSchema("截止日期；null 表示不指定或清除。"),
		"labels": map[string]any{
			"type": "array", "maxItems": 20, "uniqueItems": true,
			"items": map[string]any{"type": "string", "minLength": 1, "maxLength": 32},
		},
	}
	if create {
		properties["status"].(map[string]any)["description"] = "默认 todo。"
		properties["priority"].(map[string]any)["description"] = "1=低、2=中、3=高，默认 2。"
	}
	return properties
}

func stringArraySchema(values []string) map[string]any {
	return map[string]any{"type": "array", "uniqueItems": true, "items": map[string]any{"type": "string", "enum": values}}
}

func dateStringSchema(description string) map[string]any {
	return map[string]any{"type": "string", "format": "date", "pattern": `^\d{4}-\d{2}-\d{2}$`, "description": description}
}

func nullableStringSchema(description string) map[string]any {
	return map[string]any{"type": []string{"string", "null"}, "description": description}
}

func nullableDateSchema(description string) map[string]any {
	schema := nullableStringSchema(description)
	schema["format"] = "date"
	schema["pattern"] = `^\d{4}-\d{2}-\d{2}$`
	return schema
}

func sleepInputSchema() map[string]any {
	return map[string]any{
		"type":     "object",
		"required": []string{"seconds"},
		"properties": map[string]any{
			"seconds": map[string]any{
				"type":        "number",
				"minimum":     minSleepSeconds,
				"maximum":     maxSleepSeconds,
				"description": "等待秒数，范围为 5 到 30。",
			},
		},
		"additionalProperties": false,
	}
}

func findCapabilitySpec(specs []capabilitySpec, name string) (capabilitySpec, bool) {
	for _, spec := range specs {
		if spec.Name == name {
			return spec, true
		}
	}
	return capabilitySpec{}, false
}

func findOperationSpec(capability capabilitySpec, name string) (operationSpec, bool) {
	for _, operation := range capability.Operations {
		if operation.Name == name {
			return operation, true
		}
	}
	return operationSpec{}, false
}

func jsonToolResult(value any) (mcpclient.ToolResult, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return mcpclient.ToolResult{}, err
	}
	return mcpclient.ToolResult{Content: string(raw)}, nil
}
