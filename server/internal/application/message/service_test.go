package message

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"app/internal/appregistry"
	"app/internal/store"

	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func TestServiceCreateAndListPreserveIdempotencyOutboxAndReply(t *testing.T) {
	db := openMessageTestDB(t)
	fixture := insertMessageTestFixture(t, db)
	processor := &messageBodyProcessorRecorder{}
	order := []string{}
	notifications := &messageNotificationRecorder{db: db, order: &order}
	appEvents := &messageAppEventRecorder{db: db, order: &order}
	service := NewService(Dependencies{
		DB: db, Bodies: processor, Notifications: notifications, AppEvents: appEvents,
		AppEventLocker:         &sync.Mutex{},
		AfterUserMessageCommit: func(Message) { order = append(order, "commit-hook") },
	})

	first, err := service.Create(context.Background(), CreateCommand{
		AccountID: fixture.user.ID, ConversationID: fixture.conversation.ID,
		ClientMessageID: "message-1", Body: json.RawMessage(`{"type":"text","content":"hello"}`),
	})
	if err != nil {
		t.Fatalf("create first message: %v", err)
	}
	if !first.Created || first.Message.Seq != 1 || first.Message.Summary != "hello" {
		t.Fatalf("first = %#v", first)
	}
	if processor.prepareCalls != 1 || processor.finalizeCalls != 1 {
		t.Fatalf("processor calls = prepare %d, finalize %d", processor.prepareCalls, processor.finalizeCalls)
	}
	if len(notifications.created) != 1 || len(appEvents.events) != 1 {
		t.Fatalf("notifications = %#v, events = %#v", notifications.created, appEvents.events)
	}
	wantOrder := []string{"realtime", "commit-hook", "app-delivery"}
	if !equalMessageTestStrings(order, wantOrder) {
		t.Fatalf("order = %v, want %v", order, wantOrder)
	}

	duplicate, err := service.Create(context.Background(), CreateCommand{
		AccountID: fixture.user.ID, ConversationID: fixture.conversation.ID,
		ClientMessageID: "message-1", Body: json.RawMessage(`{"type":"text","content":"ignored"}`),
	})
	if err != nil {
		t.Fatalf("create duplicate: %v", err)
	}
	if duplicate.Created || duplicate.Message.ID != first.Message.ID {
		t.Fatalf("duplicate = %#v", duplicate)
	}
	if processor.prepareCalls != 2 || processor.finalizeCalls != 1 || len(appEvents.events) != 1 || len(notifications.created) != 1 {
		t.Fatalf("duplicate side effects: processor=%#v events=%d notifications=%d", processor, len(appEvents.events), len(notifications.created))
	}

	reply, err := service.Create(context.Background(), CreateCommand{
		AccountID: fixture.user.ID, ConversationID: fixture.conversation.ID,
		ClientMessageID: "message-2", ReplyToMessageID: first.Message.ID,
		Body: json.RawMessage(`{"type":"text","content":"reply"}`),
	})
	if err != nil {
		t.Fatalf("create reply: %v", err)
	}
	if reply.Message.ReplyTo == nil || reply.Message.ReplyTo.ID != first.Message.ID || reply.Message.ReplyTo.Sender.Name != fixture.user.Name {
		t.Fatalf("reply = %#v", reply)
	}

	listed, err := service.List(context.Background(), ListCommand{
		AccountID: fixture.user.ID, ConversationID: fixture.conversation.ID, Limit: 20,
	})
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(listed.Messages) != 2 || listed.Messages[1].ReplyTo == nil || listed.Page.OldestSeq != 1 || listed.Page.NewestSeq != 2 {
		t.Fatalf("listed = %#v", listed)
	}
}

func TestDirectAppMessagesRequireCurrentAccessWhileGroupMessagesUseMembership(t *testing.T) {
	db := openMessageTestDB(t)
	fixture := insertMessageTestFixture(t, db)
	if err := db.Model(&store.App{}).Where("id = ?", fixture.app.ID).
		Update("visibility", store.AppVisibilityRestricted).Error; err != nil {
		t.Fatalf("restrict app: %v", err)
	}
	service := NewService(Dependencies{DB: db, Bodies: &messageBodyProcessorRecorder{}})

	_, err := service.Create(context.Background(), CreateCommand{
		AccountID: fixture.user.ID, ConversationID: fixture.conversation.ID,
		ClientMessageID: "denied-user-direct", Body: json.RawMessage(`{"type":"text","content":"hello"}`),
	})
	if ErrorCodeOf(err) != CodeForbidden || ErrorMessage(err) != "你当前无权直接使用此应用" {
		t.Fatalf("user direct error = %v", err)
	}
	_, err = service.CreateAsApp(context.Background(), CreateAsAppCommand{
		AppID: fixture.app.ID, ConversationID: fixture.conversation.ID,
		ClientMessageID: "denied-app-direct", Body: json.RawMessage(`{"type":"text","content":"hello"}`),
		Finalize: func(_ context.Context, body json.RawMessage) (json.RawMessage, string, error) {
			return body, "hello", nil
		},
	})
	if ErrorCodeOf(err) != CodeForbidden || ErrorMessage(err) != "对方当前无权直接使用此应用" {
		t.Fatalf("app direct error = %v", err)
	}
	if _, err := service.List(context.Background(), ListCommand{
		AccountID: fixture.user.ID, ConversationID: fixture.conversation.ID, Limit: 20,
	}); err != nil {
		t.Fatalf("retained direct history: %v", err)
	}

	now := time.Date(2026, 7, 15, 14, 0, 0, 0, time.UTC)
	group := store.Conversation{
		ID: uuid.NewString(), Kind: store.ConversationKindGroup, Name: "Group",
		CreatedByUserID: fixture.user.ID, Status: store.ConversationStatusActive,
		PostingPolicy: store.ConversationPostingPolicyOpen, Visibility: store.ConversationVisibilityPrivate,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := db.Create(&group).Error; err != nil {
		t.Fatalf("create group: %v", err)
	}
	groupMembers := []store.ConversationMember{
		{ConversationID: group.ID, MemberType: store.ConversationMemberTypeUser, MemberID: fixture.user.ID, Role: store.ConversationMemberRoleOwner, JoinedAt: now, HistoryVisibleFromSeq: 1},
		{ConversationID: group.ID, MemberType: store.ConversationMemberTypeApp, MemberID: fixture.app.ID, Role: store.ConversationMemberRoleMember, JoinedAt: now, HistoryVisibleFromSeq: 1},
	}
	if err := db.Create(&groupMembers).Error; err != nil {
		t.Fatalf("create group members: %v", err)
	}
	if _, err := service.Create(context.Background(), CreateCommand{
		AccountID: fixture.user.ID, ConversationID: group.ID,
		ClientMessageID: "allowed-user-group", Body: json.RawMessage(`{"type":"text","content":"group hello"}`),
	}); err != nil {
		t.Fatalf("user group message: %v", err)
	}
	if _, err := service.CreateAsApp(context.Background(), CreateAsAppCommand{
		AppID: fixture.app.ID, ConversationID: group.ID,
		ClientMessageID: "allowed-app-group", Body: json.RawMessage(`{"type":"text","content":"group reply"}`),
		Finalize: func(_ context.Context, body json.RawMessage) (json.RawMessage, string, error) {
			return body, "group reply", nil
		},
	}); err != nil {
		t.Fatalf("app group message: %v", err)
	}
}

func TestServiceChoiceResponsesAggregateAndRemainIdempotent(t *testing.T) {
	db := openMessageTestDB(t)
	fixture := insertMessageTestFixture(t, db)
	order := []string{}
	appEvents := &messageAppEventRecorder{db: db, order: &order}
	service := NewService(Dependencies{DB: db, AppEvents: appEvents, AppEventLocker: &sync.Mutex{}})
	now := time.Now().UTC()
	appID := fixture.app.ID
	clientMessageID := "choice-1"
	body := json.RawMessage(`{"type":"choice","content_type":"text","content":"请选择","selection":"multiple","options":[{"id":"a","label":"选项 A"},{"id":"b","label":"选项 B"},{"id":"c","label":"选项 C"}]}`)
	message := store.Message{
		ID: uuid.NewString(), ConversationID: fixture.conversation.ID, Seq: 1,
		SenderType: store.MessageSenderTypeApp, SenderID: &appID, ClientMessageID: &clientMessageID,
		Body: body, Summary: "[选择] 请选择", CreatedAt: now, UpdatedAt: now,
	}
	if err := db.Create(&message).Error; err != nil {
		t.Fatalf("create choice message: %v", err)
	}
	if err := db.Model(&store.Conversation{}).Where("id = ?", fixture.conversation.ID).Updates(map[string]any{
		"last_message_id": message.ID, "last_message_seq": message.Seq,
	}).Error; err != nil {
		t.Fatalf("advance conversation: %v", err)
	}

	first, err := service.SubmitChoiceResponse(context.Background(), SubmitChoiceResponseCommand{
		AccountID: fixture.user.ID, ConversationID: fixture.conversation.ID,
		MessageID: message.ID, OptionIDs: []string{"c", "a"},
	})
	if err != nil {
		t.Fatalf("submit choice response: %v", err)
	}
	if !first.Created || !equalMessageTestStrings(first.Response.OptionIDs, []string{"a", "c"}) ||
		first.Choice.ResponseCount != 1 || !equalMessageTestStrings(first.Choice.MyOptionIDs, []string{"a", "c"}) ||
		first.Choice.Options[0].ResponseCount != 1 || first.Choice.Options[1].ResponseCount != 0 || first.Choice.Options[2].ResponseCount != 1 {
		t.Fatalf("first response = %#v", first)
	}
	if len(appEvents.events) != 1 || appEvents.events[0].Event != "choice.response_created" || appEvents.events[0].AppID != fixture.app.ID {
		t.Fatalf("choice app events = %#v", appEvents.events)
	}
	var eventPayload struct {
		ChoiceMessage appMessagePayload `json:"choice_message"`
		Response      struct {
			ID        string   `json:"id"`
			OptionIDs []string `json:"option_ids"`
		} `json:"response"`
		Sender appMessageSenderPayload `json:"sender"`
	}
	if err := json.Unmarshal(appEvents.events[0].Payload, &eventPayload); err != nil ||
		eventPayload.ChoiceMessage.ID != message.ID || eventPayload.Response.ID != first.Response.ID ||
		!equalMessageTestStrings(eventPayload.Response.OptionIDs, []string{"a", "c"}) || eventPayload.Sender.ID != fixture.user.ID {
		t.Fatalf("choice app event payload = %#v, err = %v", eventPayload, err)
	}
	if err := service.AuthorizeRunAsTrigger(context.Background(), RunAsTriggerCommand{
		ActorID: fixture.user.ID, ActorType: store.MessageSenderTypeUser,
		AppID: fixture.app.ID, AuthorizationConversationID: fixture.conversation.ID,
		TriggerMessageID: first.Response.ID,
	}); err != nil {
		t.Fatalf("authorize choice response trigger: %v", err)
	}

	duplicate, err := service.SubmitChoiceResponse(context.Background(), SubmitChoiceResponseCommand{
		AccountID: fixture.user.ID, ConversationID: fixture.conversation.ID,
		MessageID: message.ID, OptionIDs: []string{"a", "c"},
	})
	if err != nil || duplicate.Created || duplicate.Response.ID != first.Response.ID {
		t.Fatalf("duplicate response = %#v, err = %v", duplicate, err)
	}
	if len(appEvents.events) != 1 {
		t.Fatalf("duplicate delivered %d app events, want 1", len(appEvents.events))
	}
	_, err = service.SubmitChoiceResponse(context.Background(), SubmitChoiceResponseCommand{
		AccountID: fixture.user.ID, ConversationID: fixture.conversation.ID,
		MessageID: message.ID, OptionIDs: []string{"b"},
	})
	if ErrorCodeOf(err) != CodeConflict {
		t.Fatalf("changed response error = %v, want conflict", err)
	}

	snapshots, err := service.ListChoiceSnapshots(context.Background(), ListChoiceSnapshotsCommand{
		AccountID: fixture.user.ID, ConversationID: fixture.conversation.ID,
		MessageIDs: []string{message.ID},
	})
	if err != nil || len(snapshots.Snapshots) != 1 || snapshots.Snapshots[0].Status != choiceSnapshotStatusActive ||
		snapshots.Snapshots[0].Choice == nil || snapshots.Snapshots[0].Choice.ResponseCount != 1 {
		t.Fatalf("choice snapshots = %#v, err = %v", snapshots, err)
	}
	listed, err := service.List(context.Background(), ListCommand{
		AccountID: fixture.user.ID, ConversationID: fixture.conversation.ID, Limit: 20,
	})
	if err != nil || len(listed.Messages) != 1 || listed.Messages[0].Choice == nil || listed.Messages[0].Choice.ResponseCount != 1 {
		t.Fatalf("listed choices = %#v, err = %v", listed, err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get choice test sql db: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	concurrentClientMessageID := "choice-concurrent"
	concurrentMessage := store.Message{
		ID: uuid.NewString(), ConversationID: fixture.conversation.ID, Seq: 2,
		SenderType: store.MessageSenderTypeApp, SenderID: &appID, ClientMessageID: &concurrentClientMessageID,
		Body: body, Summary: "[选择] 请选择", CreatedAt: now.Add(time.Second), UpdatedAt: now.Add(time.Second),
	}
	if err := db.Create(&concurrentMessage).Error; err != nil {
		t.Fatalf("create concurrent choice message: %v", err)
	}
	concurrentService := NewService(Dependencies{DB: db, AppEventLocker: &sync.Mutex{}})
	results := make(chan SubmitChoiceResponseResult, 2)
	errors := make(chan error, 2)
	for range 2 {
		go func() {
			result, submitErr := concurrentService.SubmitChoiceResponse(context.Background(), SubmitChoiceResponseCommand{
				AccountID: fixture.user.ID, ConversationID: fixture.conversation.ID,
				MessageID: concurrentMessage.ID, OptionIDs: []string{"b"},
			})
			results <- result
			errors <- submitErr
		}()
	}
	createdCount := 0
	responseIDs := map[string]struct{}{}
	for range 2 {
		if submitErr := <-errors; submitErr != nil {
			t.Fatalf("concurrent choice response: %v", submitErr)
		}
		result := <-results
		if result.Created {
			createdCount++
		}
		responseIDs[result.Response.ID] = struct{}{}
	}
	if createdCount != 1 || len(responseIDs) != 1 {
		t.Fatalf("concurrent created count = %d, response ids = %v", createdCount, responseIDs)
	}
}

func TestServiceAppChoiceUpdatesUnreadChoiceSequence(t *testing.T) {
	db := openMessageTestDB(t)
	fixture := insertMessageTestFixture(t, db)
	order := []string{}
	notifications := &messageNotificationRecorder{db: db, order: &order}
	service := NewService(Dependencies{DB: db, Notifications: notifications})
	body := json.RawMessage(`{"type":"choice","content_type":"text","content":"请选择","selection":"single","options":[{"id":"a","label":"A"},{"id":"b","label":"B"}]}`)
	result, err := service.CreateAsApp(context.Background(), CreateAsAppCommand{
		AppID: fixture.app.ID, ConversationID: fixture.conversation.ID,
		ClientMessageID: "app-choice-sequence", Body: body,
		Finalize: func(_ context.Context, body json.RawMessage) (json.RawMessage, string, error) {
			return body, "[选择] 请选择", nil
		},
	})
	if err != nil || !result.Created || result.Message.Seq != 1 {
		t.Fatalf("create app choice = %#v, err = %v", result, err)
	}
	var member store.ConversationMember
	if err := db.First(&member,
		"conversation_id = ? AND member_type = ? AND member_id = ?",
		fixture.conversation.ID, store.ConversationMemberTypeUser, fixture.user.ID,
	).Error; err != nil {
		t.Fatalf("load choice recipient member: %v", err)
	}
	if member.LastChoiceSeq != result.Message.Seq {
		t.Fatalf("last choice seq = %d, want %d", member.LastChoiceSeq, result.Message.Seq)
	}
}

func TestServiceChoiceResponseRejectsInvalidUnavailableAndSingleSelection(t *testing.T) {
	db := openMessageTestDB(t)
	fixture := insertMessageTestFixture(t, db)
	service := NewService(Dependencies{DB: db})
	now := time.Now().UTC()
	appID := fixture.app.ID
	clientMessageID := "single-choice"
	message := store.Message{
		ID: uuid.NewString(), ConversationID: fixture.conversation.ID, Seq: 1,
		SenderType: store.MessageSenderTypeApp, SenderID: &appID, ClientMessageID: &clientMessageID,
		Body:    json.RawMessage(`{"type":"choice","content_type":"text","content":"请选择","selection":"single","options":[{"id":"a","label":"A"},{"id":"b","label":"B"}]}`),
		Summary: "[选择] 请选择", CreatedAt: now, UpdatedAt: now,
	}
	if err := db.Create(&message).Error; err != nil {
		t.Fatalf("create choice message: %v", err)
	}
	command := SubmitChoiceResponseCommand{
		AccountID: fixture.user.ID, ConversationID: fixture.conversation.ID, MessageID: message.ID,
	}
	outsider := store.User{
		ID: uuid.NewString(), Email: "choice-outsider@example.com", Name: "Outsider", PasswordHash: "hash",
		Status: store.UserStatusActive, CreatedAt: now, UpdatedAt: now,
	}
	if err := db.Create(&outsider).Error; err != nil {
		t.Fatalf("create choice outsider: %v", err)
	}
	command.AccountID = outsider.ID
	command.OptionIDs = []string{"a"}
	if _, err := service.SubmitChoiceResponse(context.Background(), command); ErrorCodeOf(err) != CodeForbidden {
		t.Fatalf("outsider choice response error = %v, want forbidden", err)
	}
	command.AccountID = fixture.user.ID
	for _, options := range [][]string{{"missing"}, {"a", "b"}, {"a", "a"}} {
		command.OptionIDs = options
		if _, err := service.SubmitChoiceResponse(context.Background(), command); ErrorCodeOf(err) != CodeInvalidRequest {
			t.Fatalf("options %v error = %v, want invalid_request", options, err)
		}
	}
	revokedAt := now.Add(time.Minute)
	if err := db.Model(&store.Message{}).Where("id = ?", message.ID).Update("revoked_at", revokedAt).Error; err != nil {
		t.Fatalf("revoke choice message: %v", err)
	}
	revokedSnapshots, err := service.ListChoiceSnapshots(context.Background(), ListChoiceSnapshotsCommand{
		AccountID: fixture.user.ID, ConversationID: fixture.conversation.ID, MessageIDs: []string{message.ID},
	})
	if err != nil || len(revokedSnapshots.Snapshots) != 1 ||
		revokedSnapshots.Snapshots[0].Status != choiceSnapshotStatusRevoked || revokedSnapshots.Snapshots[0].Choice != nil {
		t.Fatalf("revoked choice snapshots = %#v, err = %v", revokedSnapshots, err)
	}
	command.OptionIDs = []string{"a"}
	if _, err := service.SubmitChoiceResponse(context.Background(), command); ErrorCodeOf(err) != CodeConflict {
		t.Fatalf("revoked choice response error = %v, want conflict", err)
	}
	deletedAt := revokedAt.Add(time.Minute)
	if err := db.Model(&store.Message{}).Where("id = ?", message.ID).Update("deleted_at", deletedAt).Error; err != nil {
		t.Fatalf("delete choice message: %v", err)
	}
	deletedSnapshots, err := service.ListChoiceSnapshots(context.Background(), ListChoiceSnapshotsCommand{
		AccountID: fixture.user.ID, ConversationID: fixture.conversation.ID, MessageIDs: []string{message.ID},
	})
	if err != nil || len(deletedSnapshots.Snapshots) != 1 ||
		deletedSnapshots.Snapshots[0].Status != choiceSnapshotStatusDeleted || deletedSnapshots.Snapshots[0].Choice != nil {
		t.Fatalf("deleted choice snapshots = %#v, err = %v", deletedSnapshots, err)
	}
	if _, err := service.SubmitChoiceResponse(context.Background(), command); ErrorCodeOf(err) != CodeNotFound {
		t.Fatalf("deleted choice response error = %v, want not_found", err)
	}
}

func TestServiceRevokeChoiceRewindsUnreadChoiceSequence(t *testing.T) {
	db := openMessageTestDB(t)
	fixture := insertMessageTestFixture(t, db)
	service := NewService(Dependencies{DB: db, AppEventLocker: &sync.Mutex{}})
	body := json.RawMessage(`{"type":"choice","content_type":"text","content":"请选择","selection":"single","options":[{"id":"a","label":"A"},{"id":"b","label":"B"}]}`)
	createChoice := func(clientMessageID string) CreateResult {
		t.Helper()
		result, err := service.createFinalizedMessage(
			context.Background(), fixture.user.ID, fixture.conversation.ID,
			clientMessageID, nil, body,
			func(_ context.Context, body json.RawMessage) (json.RawMessage, string, error) {
				return body, "[选择] 请选择", nil
			},
		)
		if err != nil {
			t.Fatalf("create choice %s: %v", clientMessageID, err)
		}
		return result
	}
	first := createChoice("rewind-choice-1")
	second := createChoice("rewind-choice-2")

	if _, err := service.Revoke(context.Background(), RevokeCommand{
		AccountID: fixture.user.ID, ConversationID: fixture.conversation.ID, MessageID: second.Message.ID,
	}); err != nil {
		t.Fatalf("revoke latest choice: %v", err)
	}
	var member store.ConversationMember
	if err := db.First(&member,
		"conversation_id = ? AND member_type = ? AND member_id = ?",
		fixture.conversation.ID, store.ConversationMemberTypeUser, fixture.user.ID,
	).Error; err != nil {
		t.Fatalf("load member after latest revoke: %v", err)
	}
	if member.LastChoiceSeq != first.Message.Seq {
		t.Fatalf("last choice seq after latest revoke = %d, want %d", member.LastChoiceSeq, first.Message.Seq)
	}

	if _, err := service.Revoke(context.Background(), RevokeCommand{
		AccountID: fixture.user.ID, ConversationID: fixture.conversation.ID, MessageID: first.Message.ID,
	}); err != nil {
		t.Fatalf("revoke remaining choice: %v", err)
	}
	if err := db.First(&member,
		"conversation_id = ? AND member_type = ? AND member_id = ?",
		fixture.conversation.ID, store.ConversationMemberTypeUser, fixture.user.ID,
	).Error; err != nil {
		t.Fatalf("load member after remaining revoke: %v", err)
	}
	if member.LastChoiceSeq != 0 {
		t.Fatalf("last choice seq after all choices revoked = %d, want 0", member.LastChoiceSeq)
	}
}

func TestParseMessageMentionTargetsSupportsChoiceContent(t *testing.T) {
	userID := uuid.NewString()
	targets := parseMessageMentionTargets(json.RawMessage(`{"type":"choice","content_type":"markdown","content":"{(@user/` + userID + `)} 请选择","selection":"single","options":[{"id":"a","label":"A"},{"id":"b","label":"B"}]}`))
	if len(targets) != 1 || targets[0].MemberType != store.ConversationMemberTypeUser || targets[0].MemberID != userID {
		t.Fatalf("choice mention targets = %#v", targets)
	}
}

func TestServiceListAndRevokeHideMessagesOutsideOnlineWindow(t *testing.T) {
	db := openMessageTestDB(t)
	fixture := insertMessageTestFixture(t, db)
	now := time.Now().UTC()
	cutoff := store.MessageOnlineCutoff(now)
	oldClientMessageID := "outside-window"
	previousClientMessageID := "inside-window"
	currentClientMessageID := "current-window"
	oldMessage := store.Message{
		ID: uuid.NewString(), ConversationID: fixture.conversation.ID, Seq: 1,
		SenderType: store.MessageSenderTypeUser, SenderID: &fixture.user.ID, ClientMessageID: &oldClientMessageID,
		Body: json.RawMessage(`{"type":"text","content":"old"}`), Summary: "old",
		CreatedAt: cutoff.Add(-time.Second), UpdatedAt: cutoff.Add(-time.Second),
	}
	previousMessage := store.Message{
		ID: uuid.NewString(), ConversationID: fixture.conversation.ID, Seq: 2,
		SenderType: store.MessageSenderTypeUser, SenderID: &fixture.user.ID, ClientMessageID: &previousClientMessageID,
		Body: json.RawMessage(`{"type":"text","content":"previous"}`), Summary: "previous",
		CreatedAt: cutoff, UpdatedAt: cutoff,
	}
	currentMessage := store.Message{
		ID: uuid.NewString(), ConversationID: fixture.conversation.ID, Seq: 3,
		SenderType: store.MessageSenderTypeUser, SenderID: &fixture.user.ID, ClientMessageID: &currentClientMessageID,
		ReplyToMessageID: &oldMessage.ID,
		Body:             json.RawMessage(`{"type":"text","content":"current"}`), Summary: "current",
		CreatedAt: now, UpdatedAt: now,
	}
	if err := db.Create(&[]store.Message{oldMessage, previousMessage, currentMessage}).Error; err != nil {
		t.Fatalf("create window messages: %v", err)
	}

	service := NewService(Dependencies{DB: db})
	listed, err := service.List(context.Background(), ListCommand{
		AccountID: fixture.user.ID, ConversationID: fixture.conversation.ID, Limit: 20,
	})
	if err != nil {
		t.Fatalf("list online window: %v", err)
	}
	if len(listed.Messages) != 2 || listed.Messages[0].ID != previousMessage.ID || listed.Messages[1].ID != currentMessage.ID {
		t.Fatalf("listed messages = %#v", listed.Messages)
	}
	if listed.Messages[1].ReplyTo != nil || listed.Page.HasMoreBefore {
		t.Fatalf("outside-window message leaked into history metadata: %#v", listed)
	}

	_, err = service.Revoke(context.Background(), RevokeCommand{
		AccountID: fixture.user.ID, ConversationID: fixture.conversation.ID, MessageID: oldMessage.ID,
	})
	if ErrorCodeOf(err) != CodeNotFound {
		t.Fatalf("revoke outside-window message error = %v, want not_found", err)
	}
}

func TestServiceCreateRollsBackWhenOutboxInsertFails(t *testing.T) {
	db := openMessageTestDB(t)
	fixture := insertMessageTestFixture(t, db)
	const callbackName = "test:message-application-outbox-failure"
	if err := db.Callback().Create().Before("gorm:create").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Table == "app_event_outbox" {
			tx.AddError(errors.New("forced outbox failure"))
		}
	}); err != nil {
		t.Fatalf("register callback: %v", err)
	}
	t.Cleanup(func() { _ = db.Callback().Create().Remove(callbackName) })

	service := NewService(Dependencies{DB: db, Bodies: &messageBodyProcessorRecorder{}, AppEventLocker: &sync.Mutex{}})
	_, err := service.Create(context.Background(), CreateCommand{
		AccountID: fixture.user.ID, ConversationID: fixture.conversation.ID,
		ClientMessageID: "message-1", Body: json.RawMessage(`{"type":"text","content":"hello"}`),
	})
	if err == nil || ErrorCodeOf(err) != CodeInternal {
		t.Fatalf("error = %v, want internal error", err)
	}
	var messageCount, outboxCount int64
	if err := db.Model(&store.Message{}).Count(&messageCount).Error; err != nil {
		t.Fatalf("count messages: %v", err)
	}
	if err := db.Model(&store.AppEventOutbox{}).Count(&outboxCount).Error; err != nil {
		t.Fatalf("count outbox: %v", err)
	}
	if messageCount != 0 || outboxCount != 0 {
		t.Fatalf("message count = %d, outbox count = %d", messageCount, outboxCount)
	}
}

func TestServiceAppCreateRetriesPreserveStoredBodyAfterRevoke(t *testing.T) {
	db := openMessageTestDB(t)
	fixture := insertMessageTestFixture(t, db)
	now := time.Date(2026, 7, 15, 15, 0, 0, 0, time.UTC)
	revokedAt := now.Add(time.Minute)
	body := json.RawMessage(`{"type":"text","content":"revoked body"}`)

	appClientMessageID := "app-retry"
	appID := fixture.app.ID
	appMessage := store.Message{
		ID: uuid.NewString(), ConversationID: fixture.conversation.ID, Seq: 1,
		SenderType: store.MessageSenderTypeApp, SenderID: &appID, ClientMessageID: &appClientMessageID,
		Body: body, Summary: "revoked body", RevokedAt: &revokedAt, CreatedAt: now, UpdatedAt: revokedAt,
	}
	if err := db.Create(&appMessage).Error; err != nil {
		t.Fatalf("create revoked app message: %v", err)
	}

	service := NewService(Dependencies{DB: db})
	finalizeCalls := 0
	appResult, err := service.CreateAsApp(context.Background(), CreateAsAppCommand{
		AppID: fixture.app.ID, ConversationID: fixture.conversation.ID, ClientMessageID: appClientMessageID,
		Body: json.RawMessage(`{"type":"text","content":"ignored"}`),
		Finalize: func(context.Context, json.RawMessage) (json.RawMessage, string, error) {
			finalizeCalls++
			return nil, "", nil
		},
	})
	if err != nil {
		t.Fatalf("retry app message: %v", err)
	}
	if appResult.Created || string(appResult.Message.Body) != string(body) || finalizeCalls != 0 {
		t.Fatalf("app retry = %#v, finalize calls = %d", appResult, finalizeCalls)
	}

	delegatedClientMessageID := "delegated-retry"
	userID := fixture.user.ID
	delegatedType := store.MessageSenderTypeApp
	delegatedID := fixture.app.ID
	delegatedMessage := store.Message{
		ID: uuid.NewString(), ConversationID: fixture.conversation.ID, Seq: 2,
		SenderType: store.MessageSenderTypeUser, SenderID: &userID, ClientMessageID: &delegatedClientMessageID,
		DelegatedByType: &delegatedType, DelegatedByID: &delegatedID, DelegatedByName: fixture.app.Name,
		Body: body, Summary: "revoked body", RevokedAt: &revokedAt, CreatedAt: now, UpdatedAt: revokedAt,
	}
	if err := db.Create(&delegatedMessage).Error; err != nil {
		t.Fatalf("create revoked delegated message: %v", err)
	}
	delegatedResult, err := service.CreateDelegated(context.Background(), CreateDelegatedCommand{
		AccountID: fixture.user.ID, ConversationID: fixture.conversation.ID, ClientMessageID: delegatedClientMessageID,
		DelegatedBy: Identity{ID: fixture.app.ID, Name: fixture.app.Name, Type: store.MessageSenderTypeApp},
		Body:        json.RawMessage(`{"type":"text","content":"ignored"}`),
		Finalize: func(context.Context, json.RawMessage) (json.RawMessage, string, error) {
			finalizeCalls++
			return nil, "", nil
		},
	})
	if err != nil {
		t.Fatalf("retry delegated message: %v", err)
	}
	if delegatedResult.Created || string(delegatedResult.Message.Body) != string(body) || finalizeCalls != 0 {
		t.Fatalf("delegated retry = %#v, finalize calls = %d", delegatedResult, finalizeCalls)
	}
}

func TestServiceTimestampsMessageAfterTransactionReads(t *testing.T) {
	db := openMessageTestDB(t)
	fixture := insertMessageTestFixture(t, db)
	var delayed bool
	var delayedUntil time.Time
	const callbackName = "test:message-application-delay-first-query"
	if err := db.Callback().Query().Before("gorm:query").Register(callbackName, func(*gorm.DB) {
		if delayed {
			return
		}
		delayed = true
		time.Sleep(20 * time.Millisecond)
		delayedUntil = time.Now().UTC()
	}); err != nil {
		t.Fatalf("register query callback: %v", err)
	}
	t.Cleanup(func() { _ = db.Callback().Query().Remove(callbackName) })

	service := NewService(Dependencies{
		DB: db, Bodies: fixedMessageBodyProcessor{}, AppEventLocker: &sync.Mutex{},
	})
	result, err := service.Create(context.Background(), CreateCommand{
		AccountID: fixture.user.ID, ConversationID: fixture.conversation.ID,
		ClientMessageID: "timestamp-message", Body: json.RawMessage(`{"type":"text","content":"hello"}`),
	})
	if err != nil {
		t.Fatalf("create message: %v", err)
	}
	if !result.Created || !delayed || result.Message.CreatedAt.Before(delayedUntil) {
		t.Fatalf("result = %#v, delayed = %t, delayed until = %s", result, delayed, delayedUntil)
	}
}

func TestServiceConcurrentMessagesPersistOutboxInSequenceOrder(t *testing.T) {
	db := openMessageTestDB(t)
	fixture := insertMessageTestFixture(t, db)
	locker := &sync.Mutex{}
	firstCommitted := make(chan struct{})
	releaseFirst := make(chan struct{})
	secondReachedEventLock := make(chan struct{})
	lockErrors := make(chan error, 1)
	defer func() {
		select {
		case <-releaseFirst:
		default:
			close(releaseFirst)
		}
	}()

	service := NewService(Dependencies{
		DB: db, Bodies: fixedMessageBodyProcessor{}, AppEventLocker: locker,
		BeforeAppEventLock: func(message Message) {
			if message.Seq == 2 {
				close(secondReachedEventLock)
			}
		},
		AfterUserMessageCommit: func(message Message) {
			if message.Seq != 1 {
				return
			}
			if locker.TryLock() {
				locker.Unlock()
				lockErrors <- errors.New("app event lock was not held after commit")
			} else {
				lockErrors <- nil
			}
			close(firstCommitted)
			<-releaseFirst
		},
	})
	create := func(clientMessageID string) error {
		_, err := service.Create(context.Background(), CreateCommand{
			AccountID: fixture.user.ID, ConversationID: fixture.conversation.ID,
			ClientMessageID: clientMessageID, Body: json.RawMessage(`{"type":"text","content":"hello"}`),
		})
		return err
	}

	errs := make(chan error, 2)
	go func() { errs <- create("concurrent-message-1") }()
	select {
	case <-firstCommitted:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for first message commit")
	}
	if err := <-lockErrors; err != nil {
		t.Fatal(err)
	}
	go func() { errs <- create("concurrent-message-2") }()
	select {
	case <-secondReachedEventLock:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for second message to reach app event lock")
	}
	close(releaseFirst)
	for range 2 {
		select {
		case err := <-errs:
			if err != nil {
				t.Fatalf("create message: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for message creation")
		}
	}

	var events []store.AppEventOutbox
	if err := db.Order("id ASC").Find(&events).Error; err != nil {
		t.Fatalf("load app event outbox: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("app event count = %d, want 2", len(events))
	}
	for index, event := range events {
		var payload appMessageCreatedPayload
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			t.Fatalf("decode event payload: %v", err)
		}
		if payload.Message.Seq != int64(index+1) {
			t.Fatalf("event %d message seq = %d, want %d", index, payload.Message.Seq, index+1)
		}
	}
}

func TestServiceTaskNotificationCreationIsIdempotent(t *testing.T) {
	db := openMessageTestDB(t)
	fixture := insertMessageTestFixture(t, db)
	now := time.Date(2026, 7, 15, 16, 0, 0, 0, time.UTC)
	project := store.Project{
		ID: uuid.NewString(), Name: "Notification Project", OwnerUserID: fixture.user.ID,
		CreatedByUserID: fixture.user.ID, CreatedAt: now, UpdatedAt: now,
	}
	if err := db.Create(&project).Error; err != nil {
		t.Fatalf("create project: %v", err)
	}
	assistant := store.App{
		ID: appregistry.AIAssistantAppID, Name: "Assistant", Enabled: true,
		Visibility: store.AppVisibilityPublic, ConnectionSecret: "assistant-secret", CreatedAt: now, UpdatedAt: now,
	}
	if err := db.Create(&assistant).Error; err != nil {
		t.Fatalf("create assistant app: %v", err)
	}
	assigneeID := fixture.user.ID
	command := TaskNotificationCommand{
		AssigneeUserID: &assigneeID, CreatedByUserID: fixture.user.ID, ID: uuid.NewString(),
		ProjectID: project.ID, Title: "Task", UpdatedAt: now,
	}
	service := NewService(Dependencies{DB: db, TaskNotificationBodies: fixedTaskNotificationBodyBuilder{}})
	var first *TaskNotificationBatchResult
	if err := db.Transaction(func(tx *gorm.DB) error {
		var err error
		first, err = service.PrepareTaskNotification(context.Background(), tx, command)
		return err
	}); err != nil {
		t.Fatalf("create first notification: %v", err)
	}
	var second *TaskNotificationBatchResult
	if err := db.Transaction(func(tx *gorm.DB) error {
		var err error
		second, err = service.PrepareTaskNotification(context.Background(), tx, command)
		return err
	}); err != nil {
		t.Fatalf("create duplicate notification: %v", err)
	}
	if first == nil || len(first.Notifications) != 1 || !first.Notifications[0].Created ||
		second == nil || len(second.Notifications) != 1 || second.Notifications[0].Created ||
		second.Notifications[0].Message.ID != first.Notifications[0].Message.ID {
		t.Fatalf("first = %#v, second = %#v", first, second)
	}
	var count int64
	if err := db.Model(&store.Message{}).Where("client_message_id = ?", first.Notifications[0].Message.ClientMessageID).Count(&count).Error; err != nil {
		t.Fatalf("count notifications: %v", err)
	}
	if count != 1 {
		t.Fatalf("notification count = %d, want 1", count)
	}
}

func TestServiceTaskReminderNotificationCreationIsIdempotent(t *testing.T) {
	db := openMessageTestDB(t)
	fixture := insertMessageTestFixture(t, db)
	now := time.Date(2026, 7, 15, 16, 0, 0, 0, time.UTC)
	project := store.Project{
		ID: uuid.NewString(), Name: "Reminder Project", OwnerUserID: fixture.user.ID,
		CreatedByUserID: fixture.user.ID, CreatedAt: now, UpdatedAt: now,
	}
	if err := db.Create(&project).Error; err != nil {
		t.Fatalf("create project: %v", err)
	}
	assistant := store.App{
		ID: appregistry.AIAssistantAppID, Name: "Assistant", Enabled: true,
		Visibility: store.AppVisibilityPublic, ConnectionSecret: "assistant-secret", CreatedAt: now, UpdatedAt: now,
	}
	if err := db.Create(&assistant).Error; err != nil {
		t.Fatalf("create assistant app: %v", err)
	}
	assigneeID := fixture.user.ID
	command := TaskReminderNotificationCommand{
		AssigneeUserID: &assigneeID, Description: "description", ID: uuid.NewString(),
		ProjectID: project.ID, Title: "Task", OccurrenceAt: now, Timezone: "UTC",
	}
	service := NewService(Dependencies{DB: db, TaskReminderBodies: fixedTaskReminderBodyBuilder{}})
	var first, second *TaskNotificationResult
	for _, target := range []**TaskNotificationResult{&first, &second} {
		if err := db.Transaction(func(tx *gorm.DB) error {
			var err error
			*target, err = service.PrepareTaskReminderNotification(context.Background(), tx, command)
			return err
		}); err != nil {
			t.Fatalf("create reminder notification: %v", err)
		}
	}
	if first == nil || !first.Created || second == nil || second.Created || second.Message.ID != first.Message.ID {
		t.Fatalf("first = %#v, second = %#v", first, second)
	}
}

type messageBodyProcessorRecorder struct {
	prepareCalls  int
	finalizeCalls int
}

type fixedMessageBodyProcessor struct{}

func (fixedMessageBodyProcessor) Prepare(_ context.Context, _ string, body json.RawMessage) (json.RawMessage, error) {
	return body, nil
}

func (fixedMessageBodyProcessor) Finalize(_ context.Context, body json.RawMessage) (json.RawMessage, string, error) {
	return body, "hello", nil
}

type fixedTaskNotificationBodyBuilder struct{}

func (fixedTaskNotificationBodyBuilder) BuildTaskNotificationBody(_ context.Context, _ TaskNotificationCommand) (json.RawMessage, string, error) {
	return json.RawMessage(`{"type":"card","title":"Task","description":"description","url":"/task"}`), "[卡片] Task", nil
}

type fixedTaskReminderBodyBuilder struct{}

func (fixedTaskReminderBodyBuilder) BuildTaskReminderBody(_ context.Context, _ TaskReminderNotificationCommand) (json.RawMessage, string, error) {
	return json.RawMessage(`{"type":"card","title":"Reminder","description":"description","url":"/task"}`), "[卡片] Reminder", nil
}

func (p *messageBodyProcessorRecorder) Prepare(_ context.Context, _ string, body json.RawMessage) (json.RawMessage, error) {
	p.prepareCalls++
	return body, nil
}

func (p *messageBodyProcessorRecorder) Finalize(_ context.Context, body json.RawMessage) (json.RawMessage, string, error) {
	p.finalizeCalls++
	var value struct {
		Content string `json:"content"`
	}
	if err := json.Unmarshal(body, &value); err != nil {
		return nil, "", err
	}
	return body, value.Content, nil
}

type messageNotificationRecorder struct {
	db      *gorm.DB
	created []Delivery
	order   *[]string
}

func (r *messageNotificationRecorder) PublishMessageCreated(_ context.Context, deliveries []Delivery) {
	var count int64
	if len(deliveries) > 0 {
		_ = r.db.Model(&store.Message{}).Where("id = ?", deliveries[0].Message.ID).Count(&count).Error
	}
	if count != 1 {
		return
	}
	r.created = append(r.created, deliveries...)
	*r.order = append(*r.order, "realtime")
}

func (r *messageNotificationRecorder) PublishSharedMessageCreated(_ context.Context, userIDs []string, message Message) {
	deliveries := make([]Delivery, 0, len(userIDs))
	for _, userID := range userIDs {
		deliveries = append(deliveries, Delivery{Message: message, UserID: userID})
	}
	r.PublishMessageCreated(context.Background(), deliveries)
}

func (*messageNotificationRecorder) PublishMessageUpdated(context.Context, []Delivery) {
}

func (*messageNotificationRecorder) PublishMembersMentioned(context.Context, []string, string, int64) {
}

func (*messageNotificationRecorder) PublishMembersChoiceReceived(context.Context, []string, string, int64) {
}
func (*messageNotificationRecorder) PublishMessageChoiceUpdated(context.Context, []string, ChoiceUpdatedEvent) {
}

type messageAppEventRecorder struct {
	db     *gorm.DB
	events []AppEvent
	order  *[]string
}

func (r *messageAppEventRecorder) DeliverAppEvents(_ context.Context, events []AppEvent) {
	var count int64
	if len(events) > 0 {
		_ = r.db.Model(&store.AppEventOutbox{}).Where("id = ?", events[0].Cursor).Count(&count).Error
	}
	if count != 1 {
		return
	}
	r.events = append(r.events, events...)
	*r.order = append(*r.order, "app-delivery")
}

type messageTestFixture struct {
	user         store.User
	app          store.App
	conversation store.Conversation
}

func openMessageTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if err := db.AutoMigrate(
		&store.User{}, &store.App{}, &store.Conversation{}, &store.ConversationMember{},
		&store.Message{}, &store.AppEventOutbox{}, &store.AppConversation{}, &store.AppUserGrant{},
		&store.ConversationTopic{}, &store.ConversationTopicParticipant{},
		&store.MessageReaction{}, &store.MessageReactionState{},
		&store.MessageChoiceResponse{},
		&store.Project{}, &store.ProjectGroup{},
	); err != nil {
		t.Fatalf("migrate database: %v", err)
	}
	return db
}

func insertMessageTestFixture(t *testing.T, db *gorm.DB) messageTestFixture {
	t.Helper()
	now := time.Date(2026, 7, 15, 13, 0, 0, 0, time.UTC)
	user := store.User{
		ID: uuid.NewString(), Email: "user@example.com", Name: "User", PasswordHash: "hash",
		Status: store.UserStatusActive, CreatedAt: now, UpdatedAt: now,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	app := store.App{
		ID: uuid.NewString(), Name: "App", Enabled: true, Visibility: store.AppVisibilityPublic,
		ConnectionSecret: "secret", CreatedAt: now, UpdatedAt: now,
	}
	if err := db.Create(&app).Error; err != nil {
		t.Fatalf("create app: %v", err)
	}
	conversation := store.Conversation{
		ID: uuid.NewString(), Kind: store.ConversationKindApp, Name: app.Name,
		CreatedByUserID: user.ID, Status: store.ConversationStatusActive,
		PostingPolicy: store.ConversationPostingPolicyOpen, Visibility: store.ConversationVisibilityPrivate,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := db.Create(&conversation).Error; err != nil {
		t.Fatalf("create conversation: %v", err)
	}
	members := []store.ConversationMember{
		{ConversationID: conversation.ID, MemberType: store.ConversationMemberTypeUser, MemberID: user.ID, Role: store.ConversationMemberRoleOwner, JoinedAt: now, HistoryVisibleFromSeq: 1},
		{ConversationID: conversation.ID, MemberType: store.ConversationMemberTypeApp, MemberID: app.ID, Role: store.ConversationMemberRoleMember, JoinedAt: now, HistoryVisibleFromSeq: 1},
	}
	if err := db.Create(&members).Error; err != nil {
		t.Fatalf("create members: %v", err)
	}
	if err := db.Create(&store.AppConversation{
		AppID: app.ID, UserID: user.ID, ConversationID: conversation.ID, CreatedAt: now,
	}).Error; err != nil {
		t.Fatalf("create app conversation relation: %v", err)
	}
	return messageTestFixture{user: user, app: app, conversation: conversation}
}

func equalMessageTestStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
