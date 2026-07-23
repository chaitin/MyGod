package message

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"app/internal/store"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func TestLoadForwardSourceMessagesAllowsOnlyTopicSourceFromParent(t *testing.T) {
	db := openMessageTestDB(t)
	fixture := insertMessageTestFixture(t, db)
	now := time.Date(2026, 7, 24, 4, 0, 0, 0, time.UTC)
	parent := createForwardTopicConversation(t, db, fixture.user.ID, store.ConversationKindGroup, "Parent", now)
	topic := createForwardTopicConversation(t, db, fixture.user.ID, store.ConversationKindTopic, "Topic", now)
	if err := db.Create(&store.ConversationMember{
		ConversationID: parent.ID, MemberType: store.ConversationMemberTypeUser,
		MemberID: fixture.user.ID, Role: store.ConversationMemberRoleOwner,
		JoinedAt: now, HistoryVisibleFromSeq: 1,
	}).Error; err != nil {
		t.Fatalf("create parent member: %v", err)
	}
	source := createForwardTopicMessage(t, db, parent.ID, fixture.user.ID, 10, "source", now)
	otherParentMessage := createForwardTopicMessage(t, db, parent.ID, fixture.user.ID, 11, "other parent", now.Add(time.Minute))
	topicReply := createForwardTopicMessage(t, db, topic.ID, fixture.user.ID, 1, "reply", now.Add(2*time.Minute))
	if err := db.Create(&store.ConversationTopic{
		ConversationID: topic.ID, ParentConversationID: parent.ID,
		SourceMessageID: source.ID, SourceMessageSeq: source.Seq,
		SourceMessageBody: source.Body, SourceMessageSummary: source.Summary,
		SourceSenderType: source.SenderType, SourceSenderID: source.SenderID,
		SourceSenderName: fixture.user.Name, SourceMessageCreatedAt: source.CreatedAt,
		CreatedByUserID: fixture.user.ID, CreatedAt: now, UpdatedAt: now,
	}).Error; err != nil {
		t.Fatalf("create topic metadata: %v", err)
	}

	service := NewService(Dependencies{DB: db})
	messages, err := service.loadForwardSourceMessages(
		context.Background(), fixture.user.ID, topic.ID, []string{topicReply.ID, source.ID},
	)
	if err != nil {
		t.Fatalf("load topic forward sources: %v", err)
	}
	if len(messages) != 2 || messages[0].ID != source.ID || messages[1].ID != topicReply.ID {
		t.Fatalf("topic forward sources = %#v", messages)
	}

	_, err = service.loadForwardSourceMessages(
		context.Background(), fixture.user.ID, topic.ID, []string{topicReply.ID, otherParentMessage.ID},
	)
	if !errors.Is(err, errForwardSourceUnavailable) {
		t.Fatalf("other parent message error = %v, want unavailable", err)
	}
}

func TestLoadForwardSourceMessagesRejectsHiddenDeletedOrRevokedTopicSource(t *testing.T) {
	db := openMessageTestDB(t)
	fixture := insertMessageTestFixture(t, db)
	now := time.Date(2026, 7, 24, 4, 0, 0, 0, time.UTC)
	parent := createForwardTopicConversation(t, db, fixture.user.ID, store.ConversationKindGroup, "Parent", now)
	topic := createForwardTopicConversation(t, db, fixture.user.ID, store.ConversationKindTopic, "Topic", now)
	if err := db.Create(&store.ConversationMember{
		ConversationID: parent.ID, MemberType: store.ConversationMemberTypeUser,
		MemberID: fixture.user.ID, Role: store.ConversationMemberRoleOwner,
		JoinedAt: now, HistoryVisibleFromSeq: 2,
	}).Error; err != nil {
		t.Fatalf("create parent member: %v", err)
	}
	source := createForwardTopicMessage(t, db, parent.ID, fixture.user.ID, 1, "source", now)
	if err := db.Create(&store.ConversationTopic{
		ConversationID: topic.ID, ParentConversationID: parent.ID,
		SourceMessageID: source.ID, SourceMessageSeq: source.Seq,
		SourceMessageBody: source.Body, SourceMessageSummary: source.Summary,
		SourceSenderType: source.SenderType, SourceSenderID: source.SenderID,
		SourceSenderName: fixture.user.Name, SourceMessageCreatedAt: source.CreatedAt,
		CreatedByUserID: fixture.user.ID, CreatedAt: now, UpdatedAt: now,
	}).Error; err != nil {
		t.Fatalf("create topic metadata: %v", err)
	}

	service := NewService(Dependencies{DB: db})
	if _, err := service.loadForwardSourceMessages(
		context.Background(), fixture.user.ID, topic.ID, []string{source.ID},
	); err == nil {
		t.Fatal("hidden topic source should be rejected")
	}

	if err := db.Model(&store.ConversationMember{}).Where(
		"conversation_id = ? AND member_type = ? AND member_id = ?",
		parent.ID, store.ConversationMemberTypeUser, fixture.user.ID,
	).Update("history_visible_from_seq", 1).Error; err != nil {
		t.Fatalf("restore source visibility: %v", err)
	}
	if err := db.Model(&store.Message{}).Where("id = ?", source.ID).Update("deleted_at", now).Error; err != nil {
		t.Fatalf("delete source: %v", err)
	}
	if _, err := service.loadForwardSourceMessages(
		context.Background(), fixture.user.ID, topic.ID, []string{source.ID},
	); !errors.Is(err, errForwardSourceUnavailable) {
		t.Fatalf("deleted source error = %v, want unavailable", err)
	}
	if err := db.Model(&store.Message{}).Where("id = ?", source.ID).Update("deleted_at", nil).Error; err != nil {
		t.Fatalf("restore deleted source: %v", err)
	}
	revokedAt := now.Add(time.Minute)
	if err := db.Model(&store.Message{}).Where("id = ?", source.ID).Update("revoked_at", revokedAt).Error; err != nil {
		t.Fatalf("revoke source: %v", err)
	}
	if _, err := service.loadForwardSourceMessages(
		context.Background(), fixture.user.ID, topic.ID, []string{source.ID},
	); !errors.Is(err, errForwardSourceUnavailable) {
		t.Fatalf("revoked source error = %v, want unavailable", err)
	}
}

func createForwardTopicConversation(t *testing.T, db *gorm.DB, creatorID, kind, name string, now time.Time) store.Conversation {
	t.Helper()
	conversation := store.Conversation{
		ID: uuid.NewString(), Kind: kind, Name: name, CreatedByUserID: creatorID,
		Status: store.ConversationStatusActive, PostingPolicy: store.ConversationPostingPolicyOpen,
		Visibility: store.ConversationVisibilityPrivate, CreatedAt: now, UpdatedAt: now,
	}
	if err := db.Create(&conversation).Error; err != nil {
		t.Fatalf("create %s conversation: %v", kind, err)
	}
	return conversation
}

func createForwardTopicMessage(t *testing.T, db *gorm.DB, conversationID, senderID string, seq int64, content string, now time.Time) store.Message {
	t.Helper()
	message := store.Message{
		ID: uuid.NewString(), ConversationID: conversationID, Seq: seq,
		SenderType: store.MessageSenderTypeUser, SenderID: &senderID,
		Body:    json.RawMessage(`{"type":"text","content":"` + content + `"}`),
		Summary: content, CreatedAt: now, UpdatedAt: now,
	}
	if err := db.Create(&message).Error; err != nil {
		t.Fatalf("create message: %v", err)
	}
	return message
}
