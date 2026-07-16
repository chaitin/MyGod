package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	entitycardapp "app/internal/application/entitycard"
	projectapp "app/internal/application/project"
	"app/internal/realtime"
	"app/internal/store"
)

func TestResolveEntityCardMessageBodyUsesFixedTemplates(t *testing.T) {
	testServer, db := newTestRouter(t)
	defer testServer.Close()
	now := time.Date(2026, 7, 14, 9, 0, 0, 0, time.UTC)
	owner := insertTestUser(t, db, "owner@example.com", "项目负责人", store.UserStatusActive, now)
	owner.Nickname = "老板"
	if err := db.Model(&owner).Update("nickname", owner.Nickname).Error; err != nil {
		t.Fatalf("update owner nickname: %v", err)
	}
	assignee := insertTestUser(t, db, "assignee@example.com", "张三", store.UserStatusActive, now)
	app := insertTestApp(t, db, store.App{
		Name:        "设计助手",
		Description: "**智能** 设计应用",
		Enabled:     true,
		Visibility:  store.AppVisibilityPublic,
	})
	group := insertTestConversation(t, db, testConversationInput{
		createdByUserID: owner.ID,
		kind:            store.ConversationKindGroup,
		memberIDs:       []string{owner.ID, assignee.ID},
		name:            "设计群",
		now:             now,
		visibility:      store.ConversationVisibilityPublic,
	})
	project := insertProjectFixture(t, db, projectFixtureInput{
		Description: "**官网** 改版项目",
		Name:        "官网项目",
		Owner:       owner,
		UpdatedAt:   now,
	})
	emptyProject := insertProjectFixture(t, db, projectFixtureInput{
		Name:      "空项目",
		Owner:     owner,
		UpdatedAt: now,
	})
	dueDate := time.Date(2026, 7, 20, 0, 0, 0, 0, time.UTC)
	task := insertTaskTestFixture(t, db, taskFixtureInput{
		Assignee:  &assignee,
		Creator:   owner,
		DueDate:   &dueDate,
		ProjectID: project.ID,
		Status:    store.TaskStatusInProgress,
		Title:     "完成首页改版",
		UpdatedAt: now,
	})

	projects := projectapp.NewService(projectapp.Dependencies{DB: db})
	subject := &Server{
		db: db, projects: projects,
		entityCards: entitycardapp.NewService(entitycardapp.Dependencies{
			DB: db, Projects: projects,
		}),
	}
	for _, testCase := range []struct {
		entityID        string
		entityType      string
		wantDescription string
		wantTitle       string
		wantURL         string
	}{
		{owner.ID, entityCardTypeUser, "姓名: 项目负责人\n昵称: 老板\n邮箱: owner@example.com", "联系人 - 老板", "/contacts/user/" + owner.ID},
		{app.ID, entityCardTypeApp, "智能 设计应用", "应用 - 设计助手", "/contacts/app/" + app.ID},
		{group.ID, entityCardTypeGroup, "2 位成员", "群聊 - 设计群", "/contacts/group/" + group.ID},
		{project.ID, entityCardTypeProject, "官网 改版项目", "项目 - 官网项目", "/projects/" + project.ID},
		{emptyProject.ID, entityCardTypeProject, "暂无描述", "项目 - 空项目", "/projects/" + emptyProject.ID},
		{task.ID, entityCardTypeTask, "状态: 进行中\n负责人: 张三\n截止日期: 2026-07-20", "任务 - 完成首页改版", "/projects/" + project.ID + "?taskId=" + task.ID},
	} {
		t.Run(testCase.entityType, func(t *testing.T) {
			raw, err := json.Marshal(entityCardMessageBody{
				EntityID:   testCase.entityID,
				EntityType: testCase.entityType,
				Type:       messageTypeEntityCard,
			})
			if err != nil {
				t.Fatalf("marshal entity card request: %v", err)
			}
			resolved, err := subject.resolveEntityCardMessageBody(context.Background(), owner.ID, raw)
			if err != nil {
				t.Fatalf("resolve entity card: %v", err)
			}
			var card cardMessageBody
			if err := json.Unmarshal(resolved, &card); err != nil {
				t.Fatalf("unmarshal card: %v", err)
			}
			if card.Type != messageTypeCard || card.Title != testCase.wantTitle || card.Description != testCase.wantDescription || card.URL != testCase.wantURL {
				t.Fatalf("card = %#v", card)
			}
		})
	}
}

func TestEntityCardDetailsOmitsEmptyFields(t *testing.T) {
	got := entitycardapp.Details(
		entitycardapp.Detail{Label: "姓名", Value: "张三"},
		entitycardapp.Detail{Label: "昵称", Value: ""},
		entitycardapp.Detail{Label: "邮箱", Value: "zhangsan@example.com"},
	)
	if got != "姓名: 张三\n邮箱: zhangsan@example.com" {
		t.Fatalf("description = %q", got)
	}
}

func TestCardMessageAllowsTitleOnlyEntityCard(t *testing.T) {
	raw, err := json.Marshal(cardMessageBody{Title: "项目 - 空项目", Type: messageTypeCard, URL: "/projects/project-id"})
	if err != nil {
		t.Fatalf("marshal card: %v", err)
	}
	if err := (cardMessageBodyHandler{}).Validate(raw); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestEntityCardTitleUsesConsistentPrefixAndBoundsLongName(t *testing.T) {
	if got := entityCardTitle("任务", "完成首页改版"); got != "任务 - 完成首页改版" {
		t.Fatalf("title = %q", got)
	}
	got := entityCardTitle("任务", strings.Repeat("任务", 200))
	if len([]rune(got)) != maxCardTitleLength || !strings.HasSuffix(got, "…") {
		t.Fatalf("bounded title length = %d, title = %q", len([]rune(got)), got)
	}
}

func TestResolveEntityCardMessageBodyHidesInaccessibleTask(t *testing.T) {
	testServer, db := newTestRouter(t)
	defer testServer.Close()
	now := time.Now().UTC()
	owner := insertTestUser(t, db, "entity-owner@example.com", "Owner", store.UserStatusActive, now)
	outsider := insertTestUser(t, db, "entity-outsider@example.com", "Outsider", store.UserStatusActive, now)
	project := insertProjectFixture(t, db, projectFixtureInput{Name: "Private", Owner: owner, UpdatedAt: now})
	task := insertTaskTestFixture(t, db, taskFixtureInput{Creator: owner, ProjectID: project.ID, Title: "Private task", UpdatedAt: now})
	raw, err := json.Marshal(entityCardMessageBody{EntityID: task.ID, EntityType: entityCardTypeTask, Type: messageTypeEntityCard})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	projects := projectapp.NewService(projectapp.Dependencies{DB: db})
	subject := &Server{
		db: db, projects: projects,
		entityCards: entitycardapp.NewService(entitycardapp.Dependencies{
			DB: db, Projects: projects,
		}),
	}
	_, err = subject.resolveEntityCardMessageBody(context.Background(), outsider.ID, raw)
	if !errors.Is(err, errEntityCardNotFound) {
		t.Fatalf("resolve error = %v, want not found", err)
	}
}

func TestCreateConversationEntityCardStoresGeneratedTaskCard(t *testing.T) {
	testServer, db := newTestRouter(t)
	defer testServer.Close()
	now := time.Date(2026, 7, 14, 9, 0, 0, 0, time.UTC)
	alice := insertTestUser(t, db, "entity-alice@example.com", "Alice", store.UserStatusActive, now)
	bob := insertTestUser(t, db, "entity-bob@example.com", "Bob", store.UserStatusActive, now)
	project := insertProjectFixture(t, db, projectFixtureInput{Name: "产品项目", Owner: alice, UpdatedAt: now})
	task := insertTaskTestFixture(t, db, taskFixtureInput{Creator: alice, ProjectID: project.ID, Status: store.TaskStatusTodo, Title: "整理需求", UpdatedAt: now})
	conversation := insertTestConversation(t, db, testConversationInput{
		createdByUserID: alice.ID,
		kind:            store.ConversationKindDirect,
		memberIDs:       []string{alice.ID, bob.ID},
		now:             now,
	})

	resp, body := postJSON(t, testServer, "/api/client/conversations/"+conversation.ID+"/messages", map[string]any{
		"client_message_id": "client-entity-card-1",
		"body": map[string]any{
			"entity_id":   task.ID,
			"entity_type": "task",
			"type":        "entity_card",
		},
	}, loginAsUser(t, testServer, alice.Email))
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, body = %#v", resp.StatusCode, body)
	}
	message := requireSuccess(t, body)["message"].(map[string]any)
	card := message["body"].(map[string]any)
	if card["type"] != messageTypeCard || card["title"] != "任务 - "+task.Title || card["url"] != "/projects/"+project.ID+"?taskId="+task.ID {
		t.Fatalf("message body = %#v", card)
	}
	if card["description"] != "状态: 待办" {
		t.Fatalf("description = %q", card["description"])
	}

	var stored store.Message
	if err := db.First(&stored, "id = ?", message["id"]).Error; err != nil {
		t.Fatalf("load stored message: %v", err)
	}
	var storedBody cardMessageBody
	if err := json.Unmarshal(stored.Body, &storedBody); err != nil {
		t.Fatalf("unmarshal stored body: %v", err)
	}
	if storedBody.Type != messageTypeCard {
		t.Fatalf("stored body = %#v, want card", storedBody)
	}
}

func TestAppReplyEntityCardUsesAuthorizedUserAndStoresAppCard(t *testing.T) {
	testServer, db := newTestRouter(t)
	defer testServer.Close()
	now := time.Date(2026, 7, 14, 9, 0, 0, 0, time.UTC)
	alice := insertTestUser(t, db, "entity-app-alice@example.com", "Alice", store.UserStatusActive, now)
	project := insertProjectFixture(t, db, projectFixtureInput{Name: "设计项目", Owner: alice, UpdatedAt: now})
	task := insertTaskTestFixture(t, db, taskFixtureInput{Creator: alice, ProjectID: project.ID, Status: store.TaskStatusDone, Title: "确认设计稿", UpdatedAt: now})
	app := insertTestApp(t, db, store.App{
		Name:             "茉莉",
		Enabled:          true,
		Visibility:       store.AppVisibilityPublic,
		ConnectionSecret: "entity-card-app-secret",
		CreatedAt:        now,
		UpdatedAt:        now,
	})
	aliceCookie := loginAsUser(t, testServer, alice.Email)
	appConn := dialAppWebSocket(t, testServer, app.ID, app.ConnectionSecret)

	conversationResp, conversationBody := postJSON(t, testServer, "/api/client/conversations/apps", map[string]any{
		"app_id": app.ID,
	}, aliceCookie)
	if conversationResp.StatusCode != http.StatusCreated {
		t.Fatalf("create app conversation status = %d, body = %#v", conversationResp.StatusCode, conversationBody)
	}
	conversation := requireSuccess(t, conversationBody)["conversation"].(map[string]any)
	conversationID := conversation["id"].(string)
	triggerResp, triggerBody := postJSON(t, testServer, "/api/client/conversations/"+conversationID+"/messages", map[string]any{
		"client_message_id": "trigger-entity-card-1",
		"body":              map[string]any{"type": "text", "content": "把任务发给我"},
	}, aliceCookie)
	if triggerResp.StatusCode != http.StatusCreated {
		t.Fatalf("trigger status = %d, body = %#v", triggerResp.StatusCode, triggerBody)
	}
	triggerMessage := requireSuccess(t, triggerBody)["message"].(map[string]any)
	if event := readRealtimeEvent(t, appConn); event.Kind != realtime.KindEvent || event.Event != realtime.EventMessageCreated {
		t.Fatalf("event = %#v, want message.created", event)
	}

	response := sendAppRequest(t, appConn, realtime.Envelope{
		V:      realtime.ProtocolVersion,
		Kind:   realtime.KindRequest,
		ID:     "reply-entity-card-1",
		Method: appMethodMessageSend,
		Payload: mustMarshalPayloadForTest(t, map[string]any{
			"actor_user_id":                 alice.ID,
			"authorization_conversation_id": conversationID,
			"trigger_message_id":            triggerMessage["id"],
			"target": map[string]any{
				"type":            "app",
				"conversation_id": conversationID,
			},
			"message": map[string]any{
				"type":        "entity_card",
				"entity_type": "task",
				"entity_id":   task.ID,
			},
		}),
	})
	payload := requireAppSendMessageResponsePayload(t, response)
	message := payload["message"].(map[string]any)
	body := message["body"].(map[string]any)
	if body["type"] != messageTypeCard || body["title"] != "任务 - "+task.Title || body["description"] != "状态: 已完成" {
		t.Fatalf("message body = %#v", body)
	}
	sender := message["sender"].(map[string]any)
	if sender["type"] != store.MessageSenderTypeApp || sender["id"] != app.ID {
		t.Fatalf("sender = %#v, want app", sender)
	}

	otherGroup := insertTestConversation(t, db, testConversationInput{
		createdByUserID: alice.ID,
		kind:            store.ConversationKindGroup,
		memberIDs:       []string{alice.ID},
		name:            "其他群",
		now:             now,
	})
	insertTestAppConversationMember(t, db, app.ID, otherGroup.ID, now)
	wrongTargetResponse := sendRawAppRequest(t, appConn, realtime.Envelope{
		V:      realtime.ProtocolVersion,
		Kind:   realtime.KindRequest,
		ID:     "reply-entity-card-wrong-target",
		Method: appMethodMessageSend,
		Payload: mustMarshalPayloadForTest(t, map[string]any{
			"actor_user_id":                 alice.ID,
			"authorization_conversation_id": conversationID,
			"trigger_message_id":            triggerMessage["id"],
			"target": map[string]any{
				"type":            "group",
				"conversation_id": otherGroup.ID,
			},
			"message": map[string]any{
				"type":        "entity_card",
				"entity_type": "task",
				"entity_id":   task.ID,
			},
		}),
	})
	requireAppErrorResponse(t, wrongTargetResponse, "forbidden")
}
