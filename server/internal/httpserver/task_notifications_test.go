package httpserver

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"app/internal/appregistry"
	"app/internal/realtime"
	"app/internal/store"

	"gorm.io/gorm"
)

func TestHTTPTaskCreateAndUpdateSendAssistantNotifications(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	now := time.Now().UTC()
	owner := insertTestUser(t, db, "task-notification-owner@example.com", "通知创建人", store.UserStatusActive, now)
	assignee := insertTestUser(t, db, "task-notification-assignee@example.com", "通知负责人", store.UserStatusActive, now)
	project := insertProjectFixture(t, db, projectFixtureInput{Owner: owner, Name: "通知项目", UpdatedAt: now})
	grantTaskProjectAccess(t, db, project, owner, assignee, now)
	cookie := loginAsUser(t, server, owner.Email)

	resp, body := postJSON(t, server, "/api/client/projects/"+project.ID+"/tasks", map[string]any{
		"title":            "编写上线方案",
		"description":      "整理上线步骤与回滚方案",
		"assignee_user_id": assignee.ID,
	}, cookie)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create status = %d, want 201, body = %#v", resp.StatusCode, body)
	}
	task := requireTaskResponse(t, requireSuccess(t, body))
	taskID := task["id"].(string)

	messages := loadTaskNotificationMessages(t, db, assignee.ID)
	if len(messages) != 1 {
		t.Fatalf("create notification count = %d, want 1", len(messages))
	}
	assertTaskNotificationMessage(
		t,
		messages[0],
		"任务动态 - 编写上线方案",
		"状态: 待办\n负责人: 通知负责人",
		"/projects/"+project.ID+"?taskId="+taskID,
	)

	resp, body = patchJSON(t, server, taskPath(project.ID, taskID), map[string]any{
		"title":       "完成上线方案",
		"description": "",
	}, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("update status = %d, want 200, body = %#v", resp.StatusCode, body)
	}

	messages = loadTaskNotificationMessages(t, db, assignee.ID)
	if len(messages) != 2 {
		t.Fatalf("update notification count = %d, want 2", len(messages))
	}
	assertTaskNotificationMessage(
		t,
		messages[1],
		"任务动态 - 完成上线方案",
		"状态: 待办\n负责人: 通知负责人",
		"/projects/"+project.ID+"?taskId="+taskID,
	)

	resp, body = patchJSON(t, server, taskPath(project.ID, taskID), map[string]any{
		"title":       "完成上线方案",
		"description": "",
	}, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("noop status = %d, want 200, body = %#v", resp.StatusCode, body)
	}
	if count := len(loadTaskNotificationMessages(t, db, assignee.ID)); count != 2 {
		t.Fatalf("noop notification count = %d, want 2", count)
	}

	resp, body = patchJSON(t, server, taskPath(project.ID, taskID), map[string]any{
		"assignee_user_id": nil,
	}, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("clear assignee status = %d, want 200, body = %#v", resp.StatusCode, body)
	}
	if count := len(loadTaskNotificationMessages(t, db, assignee.ID)); count != 2 {
		t.Fatalf("clear assignee notification count = %d, want 2", count)
	}
}

func TestHTTPTaskNotificationTargetsNewAssigneeOnly(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	now := time.Now().UTC()
	owner := insertTestUser(t, db, "task-notification-reassign-owner@example.com", "改派创建人", store.UserStatusActive, now)
	oldAssignee := insertTestUser(t, db, "task-notification-reassign-old@example.com", "原负责人", store.UserStatusActive, now)
	newAssignee := insertTestUser(t, db, "task-notification-reassign-new@example.com", "新负责人", store.UserStatusActive, now)
	project := insertProjectFixture(t, db, projectFixtureInput{Owner: owner, Name: "改派项目", UpdatedAt: now})
	grantTaskProjectAccess(t, db, project, owner, oldAssignee, now)
	grantTaskProjectAccess(t, db, project, owner, newAssignee, now)
	task := insertTaskTestFixture(t, db, taskFixtureInput{
		ProjectID: project.ID,
		Creator:   owner,
		Assignee:  &oldAssignee,
		Title:     "待改派任务",
		UpdatedAt: now,
	})
	cookie := loginAsUser(t, server, owner.Email)

	resp, body := patchJSON(t, server, taskPath(project.ID, task.ID), map[string]any{
		"assignee_user_id": newAssignee.ID,
	}, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %#v", resp.StatusCode, body)
	}
	if count := len(loadTaskNotificationMessages(t, db, oldAssignee.ID)); count != 0 {
		t.Fatalf("old assignee notification count = %d, want 0", count)
	}
	if count := len(loadTaskNotificationMessages(t, db, newAssignee.ID)); count != 1 {
		t.Fatalf("new assignee notification count = %d, want 1", count)
	}
}

func TestHTTPTaskNotificationSkipsAssigneeWithoutCurrentProjectAccess(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	now := time.Now().UTC()
	owner := insertTestUser(t, db, "task-notification-access-owner@example.com", "权限项目创建人", store.UserStatusActive, now)
	assignee := insertTestUser(t, db, "task-notification-access-assignee@example.com", "失权负责人", store.UserStatusActive, now)
	project := insertProjectFixture(t, db, projectFixtureInput{Owner: owner, Name: "权限变化项目", UpdatedAt: now})
	accessGroup := insertProjectConversationFixture(t, db, projectConversationFixtureInput{
		Creator: owner,
		Kind:    store.ConversationKindGroup,
		Status:  store.ConversationStatusActive,
		Name:    "任务权限群",
		Now:     now,
		Members: []store.ConversationMember{{
			MemberType: store.ConversationMemberTypeUser,
			MemberID:   assignee.ID,
		}},
	})
	insertProjectGroupFixture(t, db, project.ID, accessGroup.ID, owner.ID, now)
	task := insertTaskTestFixture(t, db, taskFixtureInput{
		ProjectID: project.ID,
		Creator:   owner,
		Assignee:  &assignee,
		Title:     "失权前分配的任务",
		UpdatedAt: now,
	})
	if err := db.Where(
		"project_id = ? AND conversation_id = ?",
		project.ID,
		accessGroup.ID,
	).Delete(&store.ProjectGroup{}).Error; err != nil {
		t.Fatalf("remove project access: %v", err)
	}

	cookie := loginAsUser(t, server, owner.Email)
	resp, body := patchJSON(t, server, taskPath(project.ID, task.ID), map[string]any{
		"description": "不应泄露给失权负责人的描述",
	}, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %#v", resp.StatusCode, body)
	}
	if count := len(loadTaskNotificationMessages(t, db, assignee.ID)); count != 0 {
		t.Fatalf("notification count = %d, want 0", count)
	}
}

func TestAppTaskCreateAndUpdateSendAssistantNotifications(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	now := time.Now().UTC()
	owner := insertTestUser(t, db, "app-task-notification-owner@example.com", "应用任务负责人", store.UserStatusActive, now)
	project := insertProjectFixture(t, db, projectFixtureInput{Owner: owner, Name: "应用通知项目", UpdatedAt: now})
	authorizationConversation := insertTestConversation(t, db, testConversationInput{
		createdByUserID: owner.ID,
		kind:            store.ConversationKindGroup,
		memberIDs:       []string{owner.ID},
		name:            "应用任务授权群",
		now:             now,
	})
	insertTestAppConversationMember(t, db, appregistry.AIAssistantAppID, authorizationConversation.ID, now)
	trigger := insertTestMessageFromSender(t, db, authorizationConversation.ID, store.MessageSenderTypeUser, owner.ID, 1, "创建通知任务", now)
	runAs := map[string]any{
		"type":                          "user",
		"id":                            owner.ID,
		"trigger_message_id":            trigger.ID,
		"authorization_conversation_id": authorizationConversation.ID,
	}
	conn := dialAppWebSocket(t, server, appregistry.AIAssistantAppID, "test-ai-assistant-secret")

	createResponse := sendAppRequest(t, conn, realtime.Envelope{
		V:      realtime.ProtocolVersion,
		Kind:   realtime.KindRequest,
		ID:     "task-notification-app-create",
		Method: appMethodProjectTasksCreate,
		Payload: mustMarshalPayloadForTest(t, map[string]any{
			"runas":            runAs,
			"project_id":       project.ID,
			"title":            "应用创建的任务",
			"description":      "创建时的任务描述",
			"assignee_user_id": owner.ID,
		}),
	})
	var task taskResponse
	if err := json.Unmarshal(createResponse.Payload, &task); err != nil {
		t.Fatalf("unmarshal created task: %v", err)
	}
	if count := len(loadTaskNotificationMessages(t, db, owner.ID)); count != 1 {
		t.Fatalf("app create notification count = %d, want 1", count)
	}

	updateResponse := sendAppRequest(t, conn, realtime.Envelope{
		V:      realtime.ProtocolVersion,
		Kind:   realtime.KindRequest,
		ID:     "task-notification-app-update",
		Method: appMethodProjectTasksUpdate,
		Payload: mustMarshalPayloadForTest(t, map[string]any{
			"runas":               runAs,
			"project_id":          project.ID,
			"task_id":             task.ID,
			"expected_updated_at": task.UpdatedAt.Format(time.RFC3339Nano),
			"description":         "更新后的任务描述",
		}),
	})
	var updated taskResponse
	if err := json.Unmarshal(updateResponse.Payload, &updated); err != nil {
		t.Fatalf("unmarshal updated task: %v", err)
	}
	messages := loadTaskNotificationMessages(t, db, owner.ID)
	if len(messages) != 2 {
		t.Fatalf("app update notification count = %d, want 2", len(messages))
	}
	assertTaskNotificationMessage(
		t,
		messages[1],
		"任务动态 - 应用创建的任务",
		"状态: 待办\n负责人: 应用任务负责人",
		"/projects/"+project.ID+"?taskId="+task.ID,
	)
}

func TestTaskNotificationFailureRollsBackTaskCreation(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	now := time.Now().UTC()
	owner := insertTestUser(t, db, "task-notification-rollback-owner@example.com", "回滚创建人", store.UserStatusActive, now)
	assignee := insertTestUser(t, db, "task-notification-rollback-assignee@example.com", "回滚负责人", store.UserStatusActive, now)
	project := insertProjectFixture(t, db, projectFixtureInput{Owner: owner, Name: "回滚项目", UpdatedAt: now})
	grantTaskProjectAccess(t, db, project, owner, assignee, now)
	cookie := loginAsUser(t, server, owner.Email)

	const callbackName = "test:fail_task_notification_message"
	if err := db.Callback().Create().Before("gorm:create").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Schema != nil && tx.Statement.Schema.Table == "messages" {
			tx.AddError(errors.New("forced task notification failure"))
		}
	}); err != nil {
		t.Fatalf("register callback: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Callback().Create().Remove(callbackName); err != nil {
			t.Errorf("remove callback: %v", err)
		}
	})

	resp, _ := postJSON(t, server, "/api/client/projects/"+project.ID+"/tasks", map[string]any{
		"title":            "必须回滚的任务",
		"assignee_user_id": assignee.ID,
	}, cookie)
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", resp.StatusCode)
	}
	var taskCount int64
	if err := db.Model(&store.Task{}).Where("title = ?", "必须回滚的任务").Count(&taskCount).Error; err != nil {
		t.Fatalf("count tasks: %v", err)
	}
	if taskCount != 0 {
		t.Fatalf("task count = %d, want 0", taskCount)
	}
}

func TestTaskNotificationBodyIncludesKeyTaskFields(t *testing.T) {
	dueDate := time.Date(2026, 7, 20, 0, 0, 0, 0, time.UTC)
	task := store.Task{
		ID:        "11111111-1111-4111-8111-111111111111",
		ProjectID: "22222222-2222-4222-8222-222222222222",
		Title:     "关键字段任务",
		Status:    store.TaskStatusInProgress,
		DueDate:   &dueDate,
		AssigneeUser: &store.User{
			Name: "张三",
		},
	}
	body, _, err := buildTaskNotificationBody(t.Context(), task)
	if err != nil {
		t.Fatalf("build body: %v", err)
	}
	card, err := decodeCardMessageBody(body)
	if err != nil {
		t.Fatalf("decode card: %v", err)
	}
	if card.Description != "状态: 进行中\n负责人: 张三\n截止日期: 2026-07-20" {
		t.Fatalf("description = %q", card.Description)
	}
}

func TestTaskNotificationBodyKeepsMaximumLengthTaskTitle(t *testing.T) {
	task := store.Task{
		ID:        "11111111-1111-4111-8111-111111111111",
		ProjectID: "22222222-2222-4222-8222-222222222222",
		Title:     strings.Repeat("任", 240),
	}
	body, _, err := buildTaskNotificationBody(t.Context(), task)
	if err != nil {
		t.Fatalf("build body: %v", err)
	}
	card, err := decodeCardMessageBody(body)
	if err != nil {
		t.Fatalf("decode card: %v", err)
	}
	wantTitle := "任务动态 - " + task.Title
	if card.Title != wantTitle {
		t.Fatalf("title = %q, want %q", card.Title, wantTitle)
	}
}

func TestTaskNotificationClientMessageIDUsesDatabaseTimePrecision(t *testing.T) {
	task := store.Task{
		ID:        "11111111-1111-4111-8111-111111111111",
		UpdatedAt: time.Unix(1_700_000_000, 123_456_789).UTC(),
	}
	recipientUserID := "22222222-2222-4222-8222-222222222222"

	original := taskNotificationClientMessageID(task, recipientUserID)
	task.UpdatedAt = task.UpdatedAt.Truncate(time.Microsecond)
	afterDatabaseRoundTrip := taskNotificationClientMessageID(task, recipientUserID)
	if afterDatabaseRoundTrip != original {
		t.Fatalf(
			"client message ID changed after database precision normalization: %q != %q",
			afterDatabaseRoundTrip,
			original,
		)
	}
	if len(original) > maxClientMessageIDLength {
		t.Fatalf("client message ID length = %d, want <= %d", len(original), maxClientMessageIDLength)
	}
}

func loadTaskNotificationMessages(t *testing.T, db *gorm.DB, userID string) []store.Message {
	t.Helper()
	var messages []store.Message
	if err := db.
		Where("conversation_id = ?", builtinAssistantConversationID(userID)).
		Order("seq ASC").
		Find(&messages).Error; err != nil {
		t.Fatalf("load task notifications: %v", err)
	}
	return messages
}

func assertTaskNotificationMessage(
	t *testing.T,
	message store.Message,
	wantTitle string,
	wantDescription string,
	wantURL string,
) {
	t.Helper()
	if message.SenderType != store.MessageSenderTypeApp ||
		message.SenderID == nil ||
		*message.SenderID != appregistry.AIAssistantAppID {
		t.Fatalf("sender = %s/%v, want assistant app", message.SenderType, message.SenderID)
	}
	card, err := decodeCardMessageBody(message.Body)
	if err != nil {
		t.Fatalf("decode notification card: %v", err)
	}
	if card.Title != wantTitle || card.Description != wantDescription || card.URL != wantURL {
		t.Fatalf("card = %#v, want title=%q description=%q url=%q", card, wantTitle, wantDescription, wantURL)
	}
	if message.Summary != "[卡片] "+wantTitle {
		t.Fatalf("summary = %q, want %q", message.Summary, "[卡片] "+wantTitle)
	}
}
