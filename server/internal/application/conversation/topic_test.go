package conversation

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"app/internal/config"
	"app/internal/store"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func TestTopicLifecycleKeepsGroupVisibilityParticipantScoped(t *testing.T) {
	db := openConversationTestDB(t)
	now := time.Date(2026, 7, 20, 4, 0, 0, 0, time.UTC)
	owner := insertConversationTestUser(t, db, "topic-owner@example.com", "Owner", now)
	member := insertConversationTestUser(t, db, "topic-member@example.com", "Member", now)
	parent, source := insertConversationTopicFixture(t, db, owner, member, now)
	service := NewService(Dependencies{
		DB: db, Apps: config.AppsConfig{AIAssistantSecret: "secret"}, Now: func() time.Time { return now },
	})

	created, err := service.CreateTopic(context.Background(), CreateTopicCommand{
		Actor: actorFromTestUser(owner), ParentConversationID: parent.ID, SourceMessageID: source.ID,
	})
	if err != nil || !created.Created || created.Conversation.Type != store.ConversationKindTopic {
		t.Fatalf("create topic = %#v, err = %v", created, err)
	}
	if created.Conversation.Name != source.Summary || created.Conversation.Topic == nil || !created.Conversation.Topic.Participating {
		t.Fatalf("created topic conversation = %#v", created.Conversation)
	}
	if source.SenderID == nil || created.Conversation.Topic.SourceSender.ID != *source.SenderID || created.Conversation.Topic.SourceSender.Type != source.SenderType {
		t.Fatalf("topic source sender = %#v", created.Conversation.Topic.SourceSender)
	}

	var participantCount int64
	if err := db.Model(&store.ConversationTopicParticipant{}).Where("conversation_id = ?", created.Conversation.ID).Count(&participantCount).Error; err != nil {
		t.Fatalf("count participants: %v", err)
	}
	if participantCount != 1 {
		t.Fatalf("participant count = %d, want 1", participantCount)
	}

	memberList, err := service.List(context.Background(), ListCommand{AccountID: member.ID})
	if err != nil {
		t.Fatalf("list member conversations: %v", err)
	}
	if containsConversation(memberList.Conversations, created.Conversation.ID) {
		t.Fatal("non-participant topic unexpectedly appeared in member list")
	}

	detail, err := service.GetTopic(context.Background(), GetTopicCommand{
		Actor: actorFromTestUser(member), TopicConversationID: created.Conversation.ID,
	})
	if err != nil || !detail.CanParticipate || detail.Conversation.Topic == nil || detail.Conversation.Topic.Participating {
		t.Fatalf("member topic detail = %#v, err = %v", detail, err)
	}
	if detail.SourceMessage.Sender.Avatar != owner.Avatar {
		t.Fatalf("source sender avatar = %q, want %q", detail.SourceMessage.Sender.Avatar, owner.Avatar)
	}

	participated, err := service.ParticipateTopic(context.Background(), ParticipateTopicCommand{
		Actor: actorFromTestUser(member), TopicConversationID: created.Conversation.ID,
	})
	if err != nil || participated.Topic == nil || !participated.Topic.Participating {
		t.Fatalf("participate topic = %#v, err = %v", participated, err)
	}
	memberList, err = service.List(context.Background(), ListCommand{AccountID: member.ID})
	if err != nil || !containsConversation(memberList.Conversations, created.Conversation.ID) {
		t.Fatalf("participating member list = %#v, err = %v", memberList, err)
	}
	actorList, err := service.ListForActor(context.Background(), AppListCommand{ActorID: member.ID, Limit: 100})
	if err != nil || !containsAppConversation(actorList.Conversations, created.Conversation.ID) {
		t.Fatalf("participating actor list = %#v, err = %v", actorList, err)
	}

	archived, err := service.ArchiveTopic(context.Background(), ArchiveTopicCommand{
		Actor: actorFromTestUser(owner), TopicConversationID: created.Conversation.ID,
	})
	if err != nil || archived.Topic == nil || !archived.Topic.Archived {
		t.Fatalf("archive topic = %#v, err = %v", archived, err)
	}
	memberList, err = service.List(context.Background(), ListCommand{AccountID: member.ID})
	if err != nil || containsConversation(memberList.Conversations, created.Conversation.ID) {
		t.Fatalf("archived topic list = %#v, err = %v", memberList, err)
	}
	actorList, err = service.ListForActor(context.Background(), AppListCommand{ActorID: member.ID, Limit: 100})
	if err != nil || containsAppConversation(actorList.Conversations, created.Conversation.ID) {
		t.Fatalf("archived topic actor list = %#v, err = %v", actorList, err)
	}
	if _, err := service.ParticipateTopic(context.Background(), ParticipateTopicCommand{
		Actor: actorFromTestUser(member), TopicConversationID: created.Conversation.ID,
	}); err == nil || ErrorCodeOf(err) != CodeConflict {
		t.Fatalf("participate archived topic error = %v, want conflict", err)
	}
}

func TestCreateTopicIsIdempotentAndRejectsNesting(t *testing.T) {
	db := openConversationTestDB(t)
	now := time.Date(2026, 7, 20, 5, 0, 0, 0, time.UTC)
	owner := insertConversationTestUser(t, db, "topic-idempotent@example.com", "Owner", now)
	member := insertConversationTestUser(t, db, "topic-idempotent-member@example.com", "Member", now)
	parent, source := insertConversationTopicFixture(t, db, owner, member, now)
	service := NewService(Dependencies{
		DB: db, Apps: config.AppsConfig{AIAssistantSecret: "secret"}, Now: func() time.Time { return now },
	})

	first, err := service.CreateTopic(context.Background(), CreateTopicCommand{
		Actor: actorFromTestUser(owner), ParentConversationID: parent.ID, SourceMessageID: source.ID,
	})
	if err != nil || !first.Created {
		t.Fatalf("first create = %#v, err = %v", first, err)
	}
	second, err := service.CreateTopic(context.Background(), CreateTopicCommand{
		Actor: actorFromTestUser(owner), ParentConversationID: parent.ID, SourceMessageID: source.ID,
	})
	if err != nil || second.Created || second.Conversation.ID != first.Conversation.ID {
		t.Fatalf("second create = %#v, err = %v", second, err)
	}

	ownerID := owner.ID
	nestedSource := store.Message{
		ID: uuid.NewString(), ConversationID: first.Conversation.ID, Seq: 1,
		SenderType: store.MessageSenderTypeUser, SenderID: &ownerID,
		Body: json.RawMessage(`{"type":"text","content":"nested"}`), Summary: "nested",
		CreatedAt: now.Add(time.Minute), UpdatedAt: now.Add(time.Minute),
	}
	if err := db.Create(&nestedSource).Error; err != nil {
		t.Fatalf("create nested source: %v", err)
	}
	if err := db.Model(&store.Conversation{}).Where("id = ?", first.Conversation.ID).Updates(map[string]any{
		"last_message_seq": int64(1), "last_message_id": nestedSource.ID, "last_message_at": nestedSource.CreatedAt,
	}).Error; err != nil {
		t.Fatalf("update topic conversation: %v", err)
	}
	_, err = service.CreateTopic(context.Background(), CreateTopicCommand{
		Actor: actorFromTestUser(owner), ParentConversationID: first.Conversation.ID, SourceMessageID: nestedSource.ID,
	})
	if err == nil || ErrorCodeOf(err) != CodeInvalidRequest {
		t.Fatalf("nested topic error = %v, want invalid_request", err)
	}
}

func TestTopicAccessHonorsParentHistoryWindow(t *testing.T) {
	db := openConversationTestDB(t)
	now := time.Date(2026, 7, 20, 5, 30, 0, 0, time.UTC)
	owner := insertConversationTestUser(t, db, "topic-history-owner@example.com", "Owner", now)
	member := insertConversationTestUser(t, db, "topic-history-member@example.com", "Member", now)
	parent, source := insertConversationTopicFixture(t, db, owner, member, now)
	service := NewService(Dependencies{
		DB: db, Apps: config.AppsConfig{AIAssistantSecret: "secret"}, Now: func() time.Time { return now },
	})

	created, err := service.CreateTopic(context.Background(), CreateTopicCommand{
		Actor: actorFromTestUser(owner), ParentConversationID: parent.ID, SourceMessageID: source.ID,
	})
	if err != nil {
		t.Fatalf("create topic: %v", err)
	}
	if err := db.Create(&store.ConversationTopicParticipant{
		ConversationID: created.Conversation.ID, ParticipantType: store.ConversationMemberTypeUser,
		ParticipantID: member.ID, JoinedReason: store.TopicParticipantReasonMention,
		JoinedAt: now, HistoryVisibleFromSeq: 1, CreatedAt: now, UpdatedAt: now,
	}).Error; err != nil {
		t.Fatalf("create stale participant: %v", err)
	}
	if err := db.Model(&store.ConversationMember{}).Where(
		"conversation_id = ? AND member_type = ? AND member_id = ?",
		parent.ID, store.ConversationMemberTypeUser, member.ID,
	).Update("history_visible_from_seq", int64(2)).Error; err != nil {
		t.Fatalf("advance parent history window: %v", err)
	}

	if _, err := service.GetTopic(context.Background(), GetTopicCommand{
		Actor: actorFromTestUser(member), TopicConversationID: created.Conversation.ID,
	}); err == nil || ErrorCodeOf(err) != CodeForbidden {
		t.Fatalf("get hidden topic error = %v, want forbidden", err)
	}
	if _, err := service.ParticipateTopic(context.Background(), ParticipateTopicCommand{
		Actor: actorFromTestUser(member), TopicConversationID: created.Conversation.ID,
	}); err == nil || ErrorCodeOf(err) != CodeForbidden {
		t.Fatalf("participate hidden topic error = %v, want forbidden", err)
	}
	listed, err := service.List(context.Background(), ListCommand{AccountID: member.ID})
	if err != nil {
		t.Fatalf("list conversations: %v", err)
	}
	if containsConversation(listed.Conversations, created.Conversation.ID) {
		t.Fatal("topic before the current parent history window appeared in the list")
	}
}

func TestTopicEventsExcludeMembersOutsideParentHistoryWindow(t *testing.T) {
	db := openConversationTestDB(t)
	now := time.Date(2026, 7, 20, 5, 40, 0, 0, time.UTC)
	owner := insertConversationTestUser(t, db, "topic-event-owner@example.com", "Owner", now)
	member := insertConversationTestUser(t, db, "topic-event-member@example.com", "Member", now)
	parent, source := insertConversationTopicFixture(t, db, owner, member, now)
	if err := db.Model(&store.ConversationMember{}).Where(
		"conversation_id = ? AND member_type = ? AND member_id = ?",
		parent.ID, store.ConversationMemberTypeUser, member.ID,
	).Update("history_visible_from_seq", int64(2)).Error; err != nil {
		t.Fatalf("advance parent history window: %v", err)
	}
	notifications := &topicConversationNotificationRecorder{}
	service := NewService(Dependencies{
		DB: db, Notifications: notifications, Now: func() time.Time { return now },
	})

	created, err := service.CreateTopic(context.Background(), CreateTopicCommand{
		Actor: actorFromTestUser(owner), ParentConversationID: parent.ID, SourceMessageID: source.ID,
	})
	if err != nil {
		t.Fatalf("create topic: %v", err)
	}
	if len(notifications.events) != 1 || !equalConversationTestStrings(notifications.events[0].userIDs, []string{owner.ID}) {
		t.Fatalf("created topic recipients = %#v", notifications.events)
	}

	if err := db.Create(&store.ConversationTopicParticipant{
		ConversationID: created.Conversation.ID, ParticipantType: store.ConversationMemberTypeUser,
		ParticipantID: member.ID, JoinedReason: store.TopicParticipantReasonMention,
		JoinedAt: now, HistoryVisibleFromSeq: 1, CreatedAt: now, UpdatedAt: now,
	}).Error; err != nil {
		t.Fatalf("create stale participant: %v", err)
	}
	if _, err := service.ArchiveTopic(context.Background(), ArchiveTopicCommand{
		Actor: actorFromTestUser(owner), TopicConversationID: created.Conversation.ID,
	}); err != nil {
		t.Fatalf("archive topic: %v", err)
	}
	if len(notifications.events) != 2 || !equalConversationTestStrings(notifications.events[1].userIDs, []string{owner.ID}) {
		t.Fatalf("archived topic recipients = %#v", notifications.events)
	}
	if len(notifications.messages) != 1 || !equalConversationTestStrings(notifications.messages[0].userIDs, []string{owner.ID}) {
		t.Fatalf("closed topic message recipients = %#v", notifications.messages)
	}
	closedMessage := notifications.messages[0].message
	if closedMessage.Sender.Type != store.MessageSenderTypeSystem || closedMessage.Summary != "Owner 已将话题关闭" {
		t.Fatalf("closed topic message = %#v", closedMessage)
	}
	var body topicClosedSystemEventBody
	if err := json.Unmarshal(closedMessage.Body, &body); err != nil {
		t.Fatalf("decode closed topic message: %v", err)
	}
	if body.Type != messageTypeSystemEvent || body.Event != systemEventTopicClosed || body.Actor.ID != owner.ID || body.Actor.DisplayName != "Owner" {
		t.Fatalf("closed topic message body = %#v", body)
	}
	var stored store.Message
	if err := db.First(&stored, "id = ?", closedMessage.ID).Error; err != nil {
		t.Fatalf("find closed topic system message: %v", err)
	}
	if stored.Seq != 1 || stored.Summary != closedMessage.Summary {
		t.Fatalf("stored closed topic message = %#v", stored)
	}
}

func TestTopicOperationsRejectDissolvedParentConversation(t *testing.T) {
	db := openConversationTestDB(t)
	now := time.Date(2026, 7, 20, 5, 42, 0, 0, time.UTC)
	owner := insertConversationTestUser(t, db, "topic-dissolved-owner@example.com", "Owner", now)
	member := insertConversationTestUser(t, db, "topic-dissolved-member@example.com", "Member", now)
	parent, source := insertConversationTopicFixture(t, db, owner, member, now)
	service := NewService(Dependencies{DB: db, Now: func() time.Time { return now }})
	created, err := service.CreateTopic(context.Background(), CreateTopicCommand{
		Actor: actorFromTestUser(owner), ParentConversationID: parent.ID, SourceMessageID: source.ID,
	})
	if err != nil {
		t.Fatalf("create topic: %v", err)
	}
	if err := db.Model(&store.Conversation{}).Where("id = ?", parent.ID).Update("status", store.ConversationStatusDissolved).Error; err != nil {
		t.Fatalf("dissolve parent: %v", err)
	}

	if _, err := service.CreateTopic(context.Background(), CreateTopicCommand{
		Actor: actorFromTestUser(owner), ParentConversationID: parent.ID, SourceMessageID: source.ID,
	}); err == nil || ErrorCodeOf(err) != CodeForbidden {
		t.Fatalf("reopen dissolved parent topic error = %v, want forbidden", err)
	}
	if _, err := service.GetTopic(context.Background(), GetTopicCommand{
		Actor: actorFromTestUser(owner), TopicConversationID: created.Conversation.ID,
	}); err == nil || ErrorCodeOf(err) != CodeForbidden {
		t.Fatalf("get dissolved parent topic error = %v, want forbidden", err)
	}
	if _, err := service.ParticipateTopic(context.Background(), ParticipateTopicCommand{
		Actor: actorFromTestUser(member), TopicConversationID: created.Conversation.ID,
	}); err == nil || ErrorCodeOf(err) != CodeForbidden {
		t.Fatalf("participate dissolved parent topic error = %v, want forbidden", err)
	}
	if _, err := service.ArchiveTopic(context.Background(), ArchiveTopicCommand{
		Actor: actorFromTestUser(owner), TopicConversationID: created.Conversation.ID,
	}); err == nil || ErrorCodeOf(err) != CodeForbidden {
		t.Fatalf("archive dissolved parent topic error = %v, want forbidden", err)
	}
}

func TestTopicNameResolvesMentionTemplates(t *testing.T) {
	db := openConversationTestDB(t)
	now := time.Date(2026, 7, 20, 5, 45, 0, 0, time.UTC)
	owner := insertConversationTestUser(t, db, "topic-name-owner@example.com", "Owner", now)
	member := insertConversationTestUser(t, db, "topic-name-member@example.com", "Member", now)
	parent, source := insertConversationTopicFixture(t, db, owner, member, now)
	summary := "请 {(@user/" + member.ID + ")} 和 {(@user/all)} 查看"
	if err := db.Model(&store.Message{}).Where("id = ?", source.ID).Updates(map[string]any{
		"body": json.RawMessage(`{"type":"text","content":"` + summary + `"}`), "summary": summary,
	}).Error; err != nil {
		t.Fatalf("update source: %v", err)
	}
	service := NewService(Dependencies{DB: db, Now: func() time.Time { return now }})

	created, err := service.CreateTopic(context.Background(), CreateTopicCommand{
		Actor: actorFromTestUser(owner), ParentConversationID: parent.ID, SourceMessageID: source.ID,
	})
	if err != nil {
		t.Fatalf("create topic: %v", err)
	}
	if created.Conversation.Name != "请 @Member 和 @所有人 查看" {
		t.Fatalf("topic name = %q", created.Conversation.Name)
	}
}

func TestRemovingParentMemberClearsTopicParticipation(t *testing.T) {
	db := openConversationTestDB(t)
	now := time.Date(2026, 7, 20, 5, 50, 0, 0, time.UTC)
	owner := insertConversationTestUser(t, db, "topic-remove-owner@example.com", "Owner", now)
	member := insertConversationTestUser(t, db, "topic-remove-member@example.com", "Member", now)
	parent, source := insertConversationTopicFixture(t, db, owner, member, now)
	service := NewService(Dependencies{DB: db, Now: func() time.Time { return now }})
	created, err := service.CreateTopic(context.Background(), CreateTopicCommand{
		Actor: actorFromTestUser(owner), ParentConversationID: parent.ID, SourceMessageID: source.ID,
	})
	if err != nil {
		t.Fatalf("create topic: %v", err)
	}
	if _, err := service.ParticipateTopic(context.Background(), ParticipateTopicCommand{
		Actor: actorFromTestUser(member), TopicConversationID: created.Conversation.ID,
	}); err != nil {
		t.Fatalf("participate topic: %v", err)
	}
	if _, err := service.RemoveMember(context.Background(), RemoveMemberCommand{
		Actor: actorFromTestUser(owner), ConversationID: parent.ID,
		MemberType: store.ConversationMemberTypeUser, MemberID: member.ID,
	}); err != nil {
		t.Fatalf("remove member: %v", err)
	}
	var count int64
	if err := db.Model(&store.ConversationTopicParticipant{}).Where(
		"conversation_id = ? AND participant_type = ? AND participant_id = ?",
		created.Conversation.ID, store.ConversationMemberTypeUser, member.ID,
	).Count(&count).Error; err != nil {
		t.Fatalf("count participant: %v", err)
	}
	if count != 0 {
		t.Fatalf("participant count = %d, want 0", count)
	}
}

func TestAppTopicLifecycleIsIdempotentParticipantScopedAndSequenceSafe(t *testing.T) {
	db := openConversationTestDB(t)
	now := time.Date(2026, 7, 20, 8, 0, 0, 0, time.UTC)
	owner := insertConversationTestUser(t, db, "app-topic-owner@example.com", "Owner", now)
	mentioned := insertConversationTestUser(t, db, "app-topic-mentioned@example.com", "Mentioned", now)
	parent, source := insertConversationTopicFixture(t, db, owner, mentioned, now)
	app := store.App{
		ID: uuid.NewString(), Name: "Topic App", Enabled: true,
		Visibility: store.AppVisibilityPublic, ConnectionSecret: "topic-app-secret",
		CreatedAt: now, UpdatedAt: now,
	}
	otherApp := store.App{
		ID: uuid.NewString(), Name: "Other App", Enabled: true,
		Visibility: store.AppVisibilityPublic, ConnectionSecret: "other-app-secret",
		CreatedAt: now, UpdatedAt: now,
	}
	if err := db.Create(&[]store.App{app, otherApp}).Error; err != nil {
		t.Fatalf("create apps: %v", err)
	}
	if err := db.Create(&store.ConversationMember{
		ConversationID: parent.ID, MemberType: store.ConversationMemberTypeApp,
		MemberID: app.ID, Role: store.ConversationMemberRoleMember,
		JoinedAt: now, HistoryVisibleFromSeq: 1,
	}).Error; err != nil {
		t.Fatalf("create app parent member: %v", err)
	}
	summary := "请 {(@user/" + mentioned.ID + ")} 跟进"
	body, err := json.Marshal(map[string]any{"type": "text", "content": summary})
	if err != nil {
		t.Fatalf("marshal source body: %v", err)
	}
	if err := db.Model(&store.Message{}).Where("id = ?", source.ID).Updates(map[string]any{
		"body": body, "summary": summary,
	}).Error; err != nil {
		t.Fatalf("update source mention: %v", err)
	}

	appEvents := &topicAppEventRecorder{}
	service := NewService(Dependencies{
		AppEvents: appEvents, AppEventLocker: &sync.Mutex{}, DB: db, Now: func() time.Time { return now },
	})
	created, err := service.CreateTopicAsApp(context.Background(), AppCreateTopicCommand{
		AppID: app.ID, ParentConversationID: parent.ID, SourceMessageID: source.ID,
	})
	if err != nil || !created.Created || created.Type != store.ConversationKindTopic || created.Archived {
		t.Fatalf("create app topic = %#v, err = %v", created, err)
	}
	if created.ParentConversationID != parent.ID || created.SourceMessageID != source.ID || created.LastMessageSeq != 0 {
		t.Fatalf("created app topic metadata = %#v", created)
	}
	if err := db.Model(&store.ConversationMember{}).Where(
		"conversation_id = ? AND member_type = ? AND member_id = ?",
		parent.ID, store.ConversationMemberTypeUser, owner.ID,
	).Update("role", store.ConversationMemberRoleMember).Error; err != nil {
		t.Fatalf("demote source user for permission check: %v", err)
	}
	userDetail, err := service.GetTopic(context.Background(), GetTopicCommand{
		Actor: actorFromTestUser(owner), TopicConversationID: created.ConversationID,
	})
	if err != nil || !userDetail.CanArchive {
		t.Fatalf("source user topic detail = %#v, err = %v", userDetail, err)
	}

	for _, participant := range []struct {
		memberType string
		memberID   string
	}{
		{store.ConversationMemberTypeApp, app.ID},
		{store.ConversationMemberTypeUser, owner.ID},
		{store.ConversationMemberTypeUser, mentioned.ID},
	} {
		var count int64
		if err := db.Model(&store.ConversationTopicParticipant{}).Where(
			"conversation_id = ? AND participant_type = ? AND participant_id = ?",
			created.ConversationID, participant.memberType, participant.memberID,
		).Count(&count).Error; err != nil {
			t.Fatalf("count topic participant: %v", err)
		}
		if count != 1 {
			t.Fatalf("participant %s/%s count = %d, want 1", participant.memberType, participant.memberID, count)
		}
	}

	retried, err := service.CreateTopicAsApp(context.Background(), AppCreateTopicCommand{
		AppID: app.ID, ParentConversationID: parent.ID, SourceMessageID: source.ID,
	})
	if err != nil || retried.Created || retried.ConversationID != created.ConversationID {
		t.Fatalf("retry app topic = %#v, err = %v", retried, err)
	}
	loaded, err := service.GetTopicAsApp(context.Background(), AppGetTopicCommand{
		AppID: app.ID, TopicConversationID: created.ConversationID,
	})
	if err != nil || loaded.ConversationID != created.ConversationID || loaded.Archived {
		t.Fatalf("get app topic = %#v, err = %v", loaded, err)
	}

	if _, err := service.CreateTopicAsApp(context.Background(), AppCreateTopicCommand{
		AppID: otherApp.ID, ParentConversationID: parent.ID, SourceMessageID: source.ID,
	}); err == nil || ErrorCodeOf(err) != CodeForbidden {
		t.Fatalf("unauthorized app create error = %v, want forbidden", err)
	}
	if _, err := service.GetTopicAsApp(context.Background(), AppGetTopicCommand{
		AppID: otherApp.ID, TopicConversationID: created.ConversationID,
	}); err == nil || ErrorCodeOf(err) != CodeForbidden {
		t.Fatalf("unauthorized app get error = %v, want forbidden", err)
	}
	if _, err := service.CloseTopicAsApp(context.Background(), AppCloseTopicCommand{
		AppID: otherApp.ID, TopicConversationID: created.ConversationID,
		ExpectedLastMessageSeq: 0,
	}); err == nil || ErrorCodeOf(err) != CodeForbidden {
		t.Fatalf("unauthorized app close error = %v, want forbidden", err)
	}
	if err := db.Create(&store.ConversationMember{
		ConversationID: parent.ID, MemberType: store.ConversationMemberTypeApp,
		MemberID: otherApp.ID, Role: store.ConversationMemberRoleMember,
		JoinedAt: now, HistoryVisibleFromSeq: 1,
	}).Error; err != nil {
		t.Fatalf("create non-participant app parent member: %v", err)
	}
	if _, err := service.GetTopicAsApp(context.Background(), AppGetTopicCommand{
		AppID: otherApp.ID, TopicConversationID: created.ConversationID,
	}); err == nil || ErrorCodeOf(err) != CodeForbidden {
		t.Fatalf("non-participant app get error = %v, want forbidden", err)
	}
	if _, err := service.CloseTopicAsApp(context.Background(), AppCloseTopicCommand{
		AppID: app.ID, TopicConversationID: created.ConversationID,
		ExpectedLastMessageSeq: 1,
	}); err == nil || ErrorCodeOf(err) != CodeConflict {
		t.Fatalf("stale app close error = %v, want conflict", err)
	}

	closed, err := service.CloseTopicAsApp(context.Background(), AppCloseTopicCommand{
		AppID: app.ID, TopicConversationID: created.ConversationID,
		ExpectedLastMessageSeq: 0,
	})
	if err != nil || !closed.Archived || closed.LastMessageSeq != 1 {
		t.Fatalf("close app topic = %#v, err = %v", closed, err)
	}
	var closedMessage store.Message
	if err := db.First(&closedMessage, "conversation_id = ? AND seq = ?", created.ConversationID, 1).Error; err != nil {
		t.Fatalf("load app close message: %v", err)
	}
	if closedMessage.SenderType != store.MessageSenderTypeSystem || closedMessage.SenderID != nil || closedMessage.Summary != "Topic App 已将话题关闭" {
		t.Fatalf("app close message = %#v", closedMessage)
	}
	var closedBody topicClosedSystemEventBody
	if err := json.Unmarshal(closedMessage.Body, &closedBody); err != nil {
		t.Fatalf("decode app close message: %v", err)
	}
	if closedBody.Type != messageTypeSystemEvent || closedBody.Event != systemEventTopicClosed ||
		closedBody.Actor.ID != app.ID || closedBody.Actor.Type != store.MessageSenderTypeApp || closedBody.Actor.DisplayName != app.Name {
		t.Fatalf("app close message body = %#v", closedBody)
	}
	if len(appEvents.events) != 1 || appEvents.events[0].AppID != app.ID || appEvents.events[0].Cursor <= 0 || appEvents.events[0].Event != appEventTopicClosed {
		t.Fatalf("topic close app events = %#v", appEvents.events)
	}
	var eventPayload topicClosedAppEventPayload
	if err := json.Unmarshal(appEvents.events[0].Payload, &eventPayload); err != nil {
		t.Fatalf("decode topic close app event: %v", err)
	}
	if !eventPayload.Archived || eventPayload.ConversationID != created.ConversationID ||
		eventPayload.ParentConversationID != parent.ID || eventPayload.SourceMessageID != source.ID {
		t.Fatalf("topic close app event payload = %#v", eventPayload)
	}
	if _, err := service.CloseTopicAsApp(context.Background(), AppCloseTopicCommand{
		AppID: app.ID, TopicConversationID: created.ConversationID,
		ExpectedLastMessageSeq: closed.LastMessageSeq,
	}); err != nil {
		t.Fatalf("repeat close app topic: %v", err)
	}
	if len(appEvents.events) != 1 {
		t.Fatalf("repeat close delivered %d app events, want 1 total", len(appEvents.events))
	}
	var outboxCount int64
	if err := db.Model(&store.AppEventOutbox{}).Where("app_id = ?", app.ID).Count(&outboxCount).Error; err != nil || outboxCount != 1 {
		t.Fatalf("topic close outbox count = %d, err = %v", outboxCount, err)
	}
}

func TestClosingTopicRollsBackWhenAppEventOutboxInsertFails(t *testing.T) {
	db := openConversationTestDB(t)
	now := time.Date(2026, 7, 20, 8, 15, 0, 0, time.UTC)
	owner := insertConversationTestUser(t, db, "topic-outbox-owner@example.com", "Owner", now)
	member := insertConversationTestUser(t, db, "topic-outbox-member@example.com", "Member", now)
	parent, source := insertConversationTopicFixture(t, db, owner, member, now)
	app := store.App{
		ID: uuid.NewString(), Name: "Topic App", Enabled: true,
		Visibility: store.AppVisibilityPublic, ConnectionSecret: "topic-app-secret",
		CreatedAt: now, UpdatedAt: now,
	}
	if err := db.Create(&app).Error; err != nil {
		t.Fatalf("create app: %v", err)
	}
	if err := db.Create(&store.ConversationMember{
		ConversationID: parent.ID, MemberType: store.ConversationMemberTypeApp,
		MemberID: app.ID, Role: store.ConversationMemberRoleMember,
		JoinedAt: now, HistoryVisibleFromSeq: 1,
	}).Error; err != nil {
		t.Fatalf("create app parent member: %v", err)
	}
	service := NewService(Dependencies{
		AppEvents: &topicAppEventRecorder{}, AppEventLocker: &sync.Mutex{}, DB: db, Now: func() time.Time { return now },
	})
	created, err := service.CreateTopicAsApp(context.Background(), AppCreateTopicCommand{
		AppID: app.ID, ParentConversationID: parent.ID, SourceMessageID: source.ID,
	})
	if err != nil {
		t.Fatalf("create topic: %v", err)
	}

	const callbackName = "test:fail_topic_closed_app_event_outbox_create"
	if err := db.Callback().Create().Before("gorm:create").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Table == "app_event_outbox" {
			tx.AddError(errors.New("forced topic outbox failure"))
		}
	}); err != nil {
		t.Fatalf("register create callback: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Callback().Create().Remove(callbackName); err != nil {
			t.Errorf("remove create callback: %v", err)
		}
	})

	if _, err := service.CloseTopicAsApp(context.Background(), AppCloseTopicCommand{
		AppID: app.ID, TopicConversationID: created.ConversationID, ExpectedLastMessageSeq: 0,
	}); err == nil || ErrorCodeOf(err) != CodeInternal {
		t.Fatalf("close topic error = %v, want internal", err)
	}
	var topic store.ConversationTopic
	if err := db.First(&topic, "conversation_id = ?", created.ConversationID).Error; err != nil {
		t.Fatalf("load topic: %v", err)
	}
	var conversation store.Conversation
	if err := db.First(&conversation, "id = ?", created.ConversationID).Error; err != nil {
		t.Fatalf("load topic conversation: %v", err)
	}
	var messageCount, outboxCount int64
	if err := db.Model(&store.Message{}).Where("conversation_id = ?", created.ConversationID).Count(&messageCount).Error; err != nil {
		t.Fatalf("count topic messages: %v", err)
	}
	if err := db.Model(&store.AppEventOutbox{}).Count(&outboxCount).Error; err != nil {
		t.Fatalf("count app events: %v", err)
	}
	if topic.ArchivedAt != nil || conversation.PostingPolicy != store.ConversationPostingPolicyOpen || messageCount != 0 || outboxCount != 0 {
		t.Fatalf("close transaction was not rolled back: topic=%#v conversation=%#v messages=%d outbox=%d", topic, conversation, messageCount, outboxCount)
	}
}

func TestAppCreatedTopicDoesNotGrantLegacyCreatorUserPermission(t *testing.T) {
	db := openConversationTestDB(t)
	now := time.Date(2026, 7, 20, 8, 30, 0, 0, time.UTC)
	legacyUser := insertConversationTestUser(t, db, "app-topic-legacy@example.com", "Legacy", now)
	app := store.App{
		ID: uuid.NewString(), Name: "Creator App", Enabled: true,
		Visibility: store.AppVisibilityPublic, ConnectionSecret: "creator-app-secret",
		CreatedAt: now, UpdatedAt: now,
	}
	if err := db.Create(&app).Error; err != nil {
		t.Fatalf("create app: %v", err)
	}
	parent := store.Conversation{
		ID: uuid.NewString(), Kind: store.ConversationKindGroup, Name: "App source parent",
		CreatedByUserID: legacyUser.ID, Status: store.ConversationStatusActive,
		PostingPolicy: store.ConversationPostingPolicyOpen, Visibility: store.ConversationVisibilityPrivate,
		CreatedAt: now, UpdatedAt: now, LastMessageSeq: 1,
	}
	appID := app.ID
	source := store.Message{
		ID: uuid.NewString(), ConversationID: parent.ID, Seq: 1,
		SenderType: store.MessageSenderTypeApp, SenderID: &appID,
		Body: json.RawMessage(`{"type":"text","content":"应用发起"}`), Summary: "应用发起",
		CreatedAt: now, UpdatedAt: now,
	}
	parent.LastMessageID = &source.ID
	parent.LastMessageAt = &source.CreatedAt
	parent.LastMessageSummary = source.Summary
	if err := db.Create(&parent).Error; err != nil {
		t.Fatalf("create parent: %v", err)
	}
	members := []store.ConversationMember{
		{ConversationID: parent.ID, MemberType: store.ConversationMemberTypeUser, MemberID: legacyUser.ID, Role: store.ConversationMemberRoleMember, JoinedAt: now, HistoryVisibleFromSeq: 1},
		{ConversationID: parent.ID, MemberType: store.ConversationMemberTypeApp, MemberID: app.ID, Role: store.ConversationMemberRoleMember, JoinedAt: now, HistoryVisibleFromSeq: 1},
	}
	if err := db.Create(&members).Error; err != nil {
		t.Fatalf("create parent members: %v", err)
	}
	if err := db.Create(&source).Error; err != nil {
		t.Fatalf("create source: %v", err)
	}
	service := NewService(Dependencies{DB: db, Now: func() time.Time { return now }})
	created, err := service.CreateTopicAsApp(context.Background(), AppCreateTopicCommand{
		AppID: app.ID, ParentConversationID: parent.ID, SourceMessageID: source.ID,
	})
	if err != nil {
		t.Fatalf("create app topic: %v", err)
	}
	detail, err := service.GetTopic(context.Background(), GetTopicCommand{
		Actor: actorFromTestUser(legacyUser), TopicConversationID: created.ConversationID,
	})
	if err != nil {
		t.Fatalf("get app topic as legacy user: %v", err)
	}
	if detail.CanArchive {
		t.Fatal("legacy created_by_user_id unexpectedly grants close permission")
	}
	if _, err := service.ArchiveTopic(context.Background(), ArchiveTopicCommand{
		Actor: actorFromTestUser(legacyUser), TopicConversationID: created.ConversationID,
	}); err == nil || ErrorCodeOf(err) != CodeForbidden {
		t.Fatalf("legacy user close error = %v, want forbidden", err)
	}
}

func TestAppDirectTopicWritesRequireCurrentUserAccess(t *testing.T) {
	db := openConversationTestDB(t)
	now := time.Date(2026, 7, 20, 9, 0, 0, 0, time.UTC)
	creator := insertConversationTestUser(t, db, "direct-topic-app-owner@example.com", "App Owner", now)
	user := insertConversationTestUser(t, db, "direct-topic-user@example.com", "User", now)
	app := store.App{
		ID: uuid.NewString(), Name: "Direct Topic App", CreatorUserID: &creator.ID,
		Enabled: true, Visibility: store.AppVisibilityPublic, ConnectionSecret: "direct-topic-secret",
		CreatedAt: now, UpdatedAt: now,
	}
	if err := db.Create(&app).Error; err != nil {
		t.Fatalf("create app: %v", err)
	}
	service := NewService(Dependencies{
		DB: db, Apps: config.AppsConfig{AIAssistantSecret: "secret"}, Now: func() time.Time { return now },
	})
	opened, err := service.CreateApp(context.Background(), CreateAppCommand{
		Actor: actorFromTestUser(user), AppID: app.ID,
	})
	if err != nil {
		t.Fatalf("open app conversation: %v", err)
	}
	conversationID := opened.Conversation.ID
	userID := user.ID
	appID := app.ID
	sources := []store.Message{
		{ID: uuid.NewString(), ConversationID: conversationID, Seq: 1, SenderType: store.MessageSenderTypeUser, SenderID: &userID, Body: json.RawMessage(`{"type":"text","content":"user topic"}`), Summary: "user topic", CreatedAt: now, UpdatedAt: now},
		{ID: uuid.NewString(), ConversationID: conversationID, Seq: 2, SenderType: store.MessageSenderTypeUser, SenderID: &userID, Body: json.RawMessage(`{"type":"text","content":"denied topic"}`), Summary: "denied topic", CreatedAt: now.Add(time.Minute), UpdatedAt: now.Add(time.Minute)},
		{ID: uuid.NewString(), ConversationID: conversationID, Seq: 3, SenderType: store.MessageSenderTypeApp, SenderID: &appID, Body: json.RawMessage(`{"type":"text","content":"app topic"}`), Summary: "app topic", CreatedAt: now.Add(2 * time.Minute), UpdatedAt: now.Add(2 * time.Minute)},
	}
	if err := db.Create(&sources).Error; err != nil {
		t.Fatalf("create source messages: %v", err)
	}
	if err := db.Model(&store.Conversation{}).Where("id = ?", conversationID).Updates(map[string]any{
		"last_message_seq": int64(3), "last_message_id": sources[2].ID,
		"last_message_at": sources[2].CreatedAt, "last_message_summary": sources[2].Summary,
	}).Error; err != nil {
		t.Fatalf("update app conversation: %v", err)
	}
	userTopic, err := service.CreateTopic(context.Background(), CreateTopicCommand{
		Actor: actorFromTestUser(user), ParentConversationID: conversationID, SourceMessageID: sources[0].ID,
	})
	if err != nil {
		t.Fatalf("create user topic: %v", err)
	}
	appTopic, err := service.CreateTopicAsApp(context.Background(), AppCreateTopicCommand{
		AppID: app.ID, ParentConversationID: conversationID, SourceMessageID: sources[2].ID,
	})
	if err != nil {
		t.Fatalf("create app topic: %v", err)
	}
	if err := db.Model(&store.App{}).Where("id = ?", app.ID).
		Update("visibility", store.AppVisibilityCreator).Error; err != nil {
		t.Fatalf("restrict app visibility: %v", err)
	}

	detail, err := service.GetTopic(context.Background(), GetTopicCommand{
		Actor: actorFromTestUser(user), TopicConversationID: userTopic.Conversation.ID,
	})
	if err != nil {
		t.Fatalf("read retained topic history: %v", err)
	}
	if detail.Conversation.CanSend || detail.CanArchive || detail.CanParticipate {
		t.Fatalf("restricted topic detail = %#v", detail)
	}
	listed, err := service.List(context.Background(), ListCommand{AccountID: user.ID})
	if err != nil {
		t.Fatalf("list retained app conversations: %v", err)
	}
	for _, item := range listed.Conversations {
		if (item.ID == conversationID || item.ID == userTopic.Conversation.ID) && item.CanSend {
			t.Fatalf("restricted conversation remains sendable: %#v", item)
		}
	}
	if _, err := service.CreateTopic(context.Background(), CreateTopicCommand{
		Actor: actorFromTestUser(user), ParentConversationID: conversationID, SourceMessageID: sources[1].ID,
	}); ErrorCodeOf(err) != CodeForbidden {
		t.Fatalf("user create topic error = %v, want forbidden", err)
	}
	if _, err := service.CreateTopic(context.Background(), CreateTopicCommand{
		Actor: actorFromTestUser(user), ParentConversationID: conversationID, SourceMessageID: sources[0].ID,
	}); ErrorCodeOf(err) != CodeForbidden {
		t.Fatalf("user reopen topic error = %v, want forbidden", err)
	}
	if _, err := service.CreateTopicAsApp(context.Background(), AppCreateTopicCommand{
		AppID: app.ID, ParentConversationID: conversationID, SourceMessageID: sources[1].ID,
	}); ErrorCodeOf(err) != CodeForbidden {
		t.Fatalf("app create topic error = %v, want forbidden", err)
	}
	if _, err := service.CreateTopicAsApp(context.Background(), AppCreateTopicCommand{
		AppID: app.ID, ParentConversationID: conversationID, SourceMessageID: sources[2].ID,
	}); ErrorCodeOf(err) != CodeForbidden {
		t.Fatalf("app reopen topic error = %v, want forbidden", err)
	}
	if _, err := service.ParticipateTopic(context.Background(), ParticipateTopicCommand{
		Actor: actorFromTestUser(user), TopicConversationID: userTopic.Conversation.ID,
	}); ErrorCodeOf(err) != CodeForbidden {
		t.Fatalf("participate topic error = %v, want forbidden", err)
	}
	if _, err := service.ArchiveTopic(context.Background(), ArchiveTopicCommand{
		Actor: actorFromTestUser(user), TopicConversationID: userTopic.Conversation.ID,
	}); ErrorCodeOf(err) != CodeForbidden {
		t.Fatalf("archive topic error = %v, want forbidden", err)
	}
	if _, err := service.CloseTopicAsApp(context.Background(), AppCloseTopicCommand{
		AppID: app.ID, TopicConversationID: appTopic.ConversationID,
		ExpectedLastMessageSeq: appTopic.LastMessageSeq,
	}); ErrorCodeOf(err) != CodeForbidden {
		t.Fatalf("app close topic error = %v, want forbidden", err)
	}
}

func insertConversationTopicFixture(t *testing.T, db *gorm.DB, owner, member store.User, now time.Time) (store.Conversation, store.Message) {
	t.Helper()
	parent := store.Conversation{
		ID: uuid.NewString(), Kind: store.ConversationKindGroup, Name: "Topic parent",
		CreatedByUserID: owner.ID, Status: store.ConversationStatusActive,
		PostingPolicy: store.ConversationPostingPolicyOpen, Visibility: store.ConversationVisibilityPrivate,
		CreatedAt: now, UpdatedAt: now, LastMessageSeq: 1,
	}
	ownerID := owner.ID
	source := store.Message{
		ID: uuid.NewString(), ConversationID: parent.ID, Seq: 1,
		SenderType: store.MessageSenderTypeUser, SenderID: &ownerID,
		Body:    json.RawMessage(`{"type":"text","content":"Discuss the rollout"}`),
		Summary: "Discuss the rollout", CreatedAt: now, UpdatedAt: now,
	}
	parent.LastMessageID = &source.ID
	parent.LastMessageAt = &source.CreatedAt
	parent.LastMessageSummary = source.Summary
	if err := db.Create(&parent).Error; err != nil {
		t.Fatalf("create parent: %v", err)
	}
	members := []store.ConversationMember{
		{ConversationID: parent.ID, MemberType: store.ConversationMemberTypeUser, MemberID: owner.ID, Role: store.ConversationMemberRoleOwner, JoinedAt: now, HistoryVisibleFromSeq: 1},
		{ConversationID: parent.ID, MemberType: store.ConversationMemberTypeUser, MemberID: member.ID, Role: store.ConversationMemberRoleMember, JoinedAt: now, HistoryVisibleFromSeq: 1},
	}
	if err := db.Create(&members).Error; err != nil {
		t.Fatalf("create parent members: %v", err)
	}
	if err := db.Create(&source).Error; err != nil {
		t.Fatalf("create source: %v", err)
	}
	return parent, source
}

func containsConversation(items []Item, conversationID string) bool {
	for _, item := range items {
		if item.ID == conversationID {
			return true
		}
	}
	return false
}

func containsAppConversation(items []AppSummary, conversationID string) bool {
	for _, item := range items {
		if item.ConversationID == conversationID {
			return true
		}
	}
	return false
}

type recordedTopicConversationEvent struct {
	event   TopicEvent
	userIDs []string
}

type recordedTopicConversationMessage struct {
	message Message
	userIDs []string
}

type topicConversationNotificationRecorder struct {
	events   []recordedTopicConversationEvent
	messages []recordedTopicConversationMessage
}

type topicAppEventRecorder struct {
	events []AppEvent
}

func (r *topicAppEventRecorder) DeliverConversationAppEvents(_ context.Context, events []AppEvent) {
	r.events = append(r.events, events...)
}

func (r *topicConversationNotificationRecorder) PublishConversationMessage(_ context.Context, userIDs []string, message Message) {
	r.messages = append(r.messages, recordedTopicConversationMessage{message: message, userIDs: append([]string(nil), userIDs...)})
}

func (*topicConversationNotificationRecorder) PublishConversationRemoved(context.Context, []string, string) {
}

func (*topicConversationNotificationRecorder) PublishConversationPinUpdated(context.Context, []string, ConversationPinEvent) {
}

func (r *topicConversationNotificationRecorder) PublishTopicEvent(_ context.Context, userIDs []string, event TopicEvent) {
	r.events = append(r.events, recordedTopicConversationEvent{event: event, userIDs: append([]string(nil), userIDs...)})
}

func equalConversationTestStrings(left, right []string) bool {
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
