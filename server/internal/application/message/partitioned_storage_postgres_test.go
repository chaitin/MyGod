package message

import (
	"context"
	"encoding/json"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"app/internal/store"

	"github.com/google/uuid"
)

func TestPostgresPartitionedMessageServiceEnforcesOnlineWindowAndPreservesCurrentBehavior(t *testing.T) {
	baseDSN := strings.TrimSpace(os.Getenv("POSTGRES_TEST_DSN"))
	if baseDSN == "" {
		t.Skip("POSTGRES_TEST_DSN is not configured")
	}
	baseDB, err := store.OpenPostgres(baseDSN)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}

	schema := "message_service_partition_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if err := baseDB.Exec(`CREATE SCHEMA "` + schema + `"`).Error; err != nil {
		t.Fatalf("create test schema: %v", err)
	}
	t.Cleanup(func() {
		_ = baseDB.Exec(`DROP SCHEMA IF EXISTS "` + schema + `" CASCADE`).Error
	})

	parsedDSN, err := url.Parse(baseDSN)
	if err != nil {
		t.Fatalf("parse postgres dsn: %v", err)
	}
	query := parsedDSN.Query()
	query.Set("search_path", schema)
	parsedDSN.RawQuery = query.Encode()
	db, err := store.OpenPostgres(parsedDSN.String())
	if err != nil {
		t.Fatalf("open schema postgres: %v", err)
	}
	if err := store.RunPostgresMigrations(db, "../../../migrations"); err != nil {
		t.Fatalf("migrate postgres test schema: %v", err)
	}

	fixture := insertMessageTestFixture(t, db)
	now := time.Now().UTC()
	oldYear := store.MessageMinimumOnlineYear(now) - 1
	previousYear := now.Year() - 1
	if err := store.EnsureMessageYearPartitions(t.Context(), db, oldYear); err != nil {
		t.Fatalf("ensure old partitions: %v", err)
	}
	oldTime := time.Date(oldYear, time.December, 31, 23, 59, 0, 0, time.UTC)
	oldClientMessageID := "old-partitioned-message"
	oldMessage := store.Message{
		ID: uuid.NewString(), ConversationID: fixture.conversation.ID, Seq: 1,
		SenderType: store.MessageSenderTypeUser, SenderID: &fixture.user.ID, ClientMessageID: &oldClientMessageID,
		Body: json.RawMessage(`{"type":"text","content":"old"}`), Summary: "old",
		CreatedAt: oldTime, UpdatedAt: oldTime,
	}
	if err := db.Create(&oldMessage).Error; err != nil {
		t.Fatalf("create retained old message: %v", err)
	}

	previousTime := time.Date(previousYear, time.December, 31, 23, 59, 0, 0, time.UTC)
	previousClientMessageID := "previous-year-partitioned-message"
	previousMessage := store.Message{
		ID: uuid.NewString(), ConversationID: fixture.conversation.ID, Seq: 2,
		SenderType: store.MessageSenderTypeUser, SenderID: &fixture.user.ID, ClientMessageID: &previousClientMessageID,
		Body: json.RawMessage(`{"type":"text","content":"previous"}`), Summary: "previous",
		CreatedAt: previousTime, UpdatedAt: previousTime,
	}
	if err := db.Create(&previousMessage).Error; err != nil {
		t.Fatalf("create previous-year message: %v", err)
	}
	if err := db.Model(&store.Conversation{}).Where("id = ?", fixture.conversation.ID).Updates(map[string]any{
		"last_message_id": previousMessage.ID, "last_message_seq": previousMessage.Seq,
		"last_message_summary": previousMessage.Summary, "last_message_at": previousTime,
	}).Error; err != nil {
		t.Fatalf("update conversation cursor: %v", err)
	}

	service := NewService(Dependencies{
		DB: db, Bodies: &messageBodyProcessorRecorder{}, AppEventLocker: &sync.Mutex{},
	})
	created, err := service.Create(context.Background(), CreateCommand{
		AccountID: fixture.user.ID, ConversationID: fixture.conversation.ID,
		ClientMessageID: "current-partitioned-message", ReplyToMessageID: previousMessage.ID,
		Body: json.RawMessage(`{"type":"text","content":"current"}`),
	})
	if err != nil {
		t.Fatalf("create current message: %v", err)
	}
	if !created.Created || created.Message.Seq != 3 || created.Message.ReplyTo == nil || created.Message.ReplyTo.ID != previousMessage.ID {
		t.Fatalf("created message = %#v", created)
	}

	duplicate, err := service.Create(context.Background(), CreateCommand{
		AccountID: fixture.user.ID, ConversationID: fixture.conversation.ID,
		ClientMessageID: "current-partitioned-message",
		Body:            json.RawMessage(`{"type":"text","content":"ignored"}`),
	})
	if err != nil {
		t.Fatalf("retry current message: %v", err)
	}
	if duplicate.Created || duplicate.Message.ID != created.Message.ID {
		t.Fatalf("duplicate message = %#v", duplicate)
	}
	if err := service.AuthorizeRunAsTrigger(context.Background(), RunAsTriggerCommand{
		ActorID: fixture.user.ID, ActorType: store.MessageSenderTypeUser, AppID: fixture.app.ID,
		AuthorizationConversationID: fixture.conversation.ID, TriggerMessageID: created.Message.ID,
	}); err != nil {
		t.Fatalf("authorize current run-as trigger: %v", err)
	}
	if err := service.AuthorizeRunAsTrigger(context.Background(), RunAsTriggerCommand{
		ActorID: fixture.user.ID, ActorType: store.MessageSenderTypeUser, AppID: fixture.app.ID,
		AuthorizationConversationID: fixture.conversation.ID, TriggerMessageID: oldMessage.ID,
	}); ErrorCodeOf(err) != CodeForbidden {
		t.Fatalf("old run-as trigger error = %v, want forbidden", err)
	}

	listed, err := service.List(context.Background(), ListCommand{
		AccountID: fixture.user.ID, ConversationID: fixture.conversation.ID, Limit: 20,
	})
	if err != nil {
		t.Fatalf("list online messages: %v", err)
	}
	if len(listed.Messages) != 2 || listed.Messages[0].ID != previousMessage.ID || listed.Messages[1].ID != created.Message.ID {
		t.Fatalf("listed messages = %#v", listed.Messages)
	}
	if listed.Page.HasMoreBefore {
		t.Fatalf("window-excluded old message affected page bounds: %#v", listed.Page)
	}
	listedReply := listed.Messages[1].ReplyTo
	if listedReply == nil || listedReply.ID != previousMessage.ID || listedReply.Summary != previousMessage.Summary || listedReply.Sender.Name != fixture.user.Name {
		t.Fatalf("listed previous-year reply = %#v", listedReply)
	}

	_, err = service.Create(context.Background(), CreateCommand{
		AccountID: fixture.user.ID, ConversationID: fixture.conversation.ID,
		ClientMessageID: "reply-to-window-excluded-message", ReplyToMessageID: oldMessage.ID,
		Body: json.RawMessage(`{"type":"text","content":"invalid reply"}`),
	})
	if ErrorCodeOf(err) != CodeInvalidRequest {
		t.Fatalf("reply to old message error = %v, want invalid_request", err)
	}

	_, err = service.Revoke(context.Background(), RevokeCommand{
		AccountID: fixture.user.ID, ConversationID: fixture.conversation.ID, MessageID: oldMessage.ID,
	})
	if ErrorCodeOf(err) != CodeNotFound {
		t.Fatalf("revoke old message error = %v, want not_found", err)
	}

	revoked, err := service.Revoke(context.Background(), RevokeCommand{
		AccountID: fixture.user.ID, ConversationID: fixture.conversation.ID, MessageID: previousMessage.ID,
	})
	if err != nil {
		t.Fatalf("revoke previous-year message: %v", err)
	}
	if revoked.Message.RevokedAt == nil || revoked.Message.Body != nil || revoked.SystemMessage.Seq != 4 {
		t.Fatalf("revoke result = %#v", revoked)
	}
	var registry store.MessageRegistry
	if err := db.First(&registry, "id = ?", previousMessage.ID).Error; err != nil {
		t.Fatalf("load revoked registry: %v", err)
	}
	if registry.RevokedAt == nil {
		t.Fatal("previous-year registry revoke was not synchronized")
	}

	var retainedCount int64
	if err := db.Model(&store.Message{}).Where("id = ?", oldMessage.ID).Count(&retainedCount).Error; err != nil {
		t.Fatalf("count retained old message: %v", err)
	}
	if retainedCount != 1 {
		t.Fatalf("retained old message count = %d", retainedCount)
	}
}
