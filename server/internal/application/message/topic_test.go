package message

import (
	"context"
	"encoding/json"
	"sort"
	"testing"
	"time"

	"app/internal/store"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func TestTopicMessageAutomaticallyJoinsSenderAndExplicitMentions(t *testing.T) {
	db := openMessageTestDB(t)
	fixture := insertMessageTestFixture(t, db)
	now := time.Date(2026, 7, 20, 6, 0, 0, 0, time.UTC)
	mentioned := insertTopicMessageTestUser(t, db, "mentioned@example.com", now)
	unmentioned := insertTopicMessageTestUser(t, db, "unmentioned@example.com", now)
	parent := store.Conversation{
		ID: uuid.NewString(), Kind: store.ConversationKindGroup, Name: "Parent",
		CreatedByUserID: fixture.user.ID, Status: store.ConversationStatusActive,
		PostingPolicy: store.ConversationPostingPolicyOpen, Visibility: store.ConversationVisibilityPrivate,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := db.Create(&parent).Error; err != nil {
		t.Fatalf("create parent: %v", err)
	}
	members := []store.ConversationMember{
		{ConversationID: parent.ID, MemberType: store.ConversationMemberTypeUser, MemberID: fixture.user.ID, Role: store.ConversationMemberRoleOwner, JoinedAt: now, HistoryVisibleFromSeq: 1},
		{ConversationID: parent.ID, MemberType: store.ConversationMemberTypeUser, MemberID: mentioned.ID, Role: store.ConversationMemberRoleMember, JoinedAt: now, HistoryVisibleFromSeq: 1},
		{ConversationID: parent.ID, MemberType: store.ConversationMemberTypeUser, MemberID: unmentioned.ID, Role: store.ConversationMemberRoleMember, JoinedAt: now, HistoryVisibleFromSeq: 1},
		{ConversationID: parent.ID, MemberType: store.ConversationMemberTypeApp, MemberID: fixture.app.ID, Role: store.ConversationMemberRoleMember, JoinedAt: now, HistoryVisibleFromSeq: 1},
	}
	if err := db.Create(&members).Error; err != nil {
		t.Fatalf("create parent members: %v", err)
	}
	topic := store.Conversation{
		ID: uuid.NewString(), Kind: store.ConversationKindTopic, Name: "Topic",
		CreatedByUserID: fixture.user.ID, Status: store.ConversationStatusActive,
		PostingPolicy: store.ConversationPostingPolicyOpen, Visibility: store.ConversationVisibilityPrivate,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := db.Create(&topic).Error; err != nil {
		t.Fatalf("create topic conversation: %v", err)
	}
	sourceID := uuid.NewString()
	sourceSenderID := fixture.user.ID
	if err := db.Create(&store.ConversationTopic{
		ConversationID: topic.ID, ParentConversationID: parent.ID,
		SourceMessageID: sourceID, SourceMessageSeq: 1,
		SourceMessageBody:    json.RawMessage(`{"type":"text","content":"source"}`),
		SourceMessageSummary: "source", SourceSenderType: store.MessageSenderTypeUser,
		SourceSenderID: &sourceSenderID, SourceSenderName: fixture.user.Name,
		SourceMessageCreatedAt: now, CreatedByUserID: fixture.user.ID,
		CreatedAt: now, UpdatedAt: now,
	}).Error; err != nil {
		t.Fatalf("create topic metadata: %v", err)
	}

	notifications := &topicMessageNotificationRecorder{}
	events := &topicMessageAppEventRecorder{}
	service := NewService(Dependencies{
		DB: db, Bodies: fixedMessageBodyProcessor{}, Notifications: notifications, AppEvents: events,
	})
	body := json.RawMessage(`{"type":"text","content":"hello {(@user/` + mentioned.ID + `)} {(@app/` + fixture.app.ID + `)} {(@user/all)}"}`)
	result, err := service.Create(context.Background(), CreateCommand{
		AccountID: fixture.user.ID, ConversationID: topic.ID,
		ClientMessageID: "topic-message", Body: body,
	})
	if err != nil || !result.Created {
		t.Fatalf("create topic message = %#v, err = %v", result, err)
	}

	var participants []store.ConversationTopicParticipant
	if err := db.Where("conversation_id = ?", topic.ID).Order("participant_type, participant_id").Find(&participants).Error; err != nil {
		t.Fatalf("load participants: %v", err)
	}
	participantKeys := make([]string, 0, len(participants))
	for _, participant := range participants {
		participantKeys = append(participantKeys, participant.ParticipantType+"/"+participant.ParticipantID)
	}
	sort.Strings(participantKeys)
	wantParticipants := []string{
		store.ConversationMemberTypeApp + "/" + fixture.app.ID,
		store.ConversationMemberTypeUser + "/" + fixture.user.ID,
		store.ConversationMemberTypeUser + "/" + mentioned.ID,
	}
	sort.Strings(wantParticipants)
	if !equalStrings(participantKeys, wantParticipants) {
		t.Fatalf("participants = %v, want %v", participantKeys, wantParticipants)
	}

	sort.Strings(notifications.createdUserIDs)
	wantUsers := []string{fixture.user.ID, mentioned.ID}
	sort.Strings(wantUsers)
	if !equalStrings(notifications.createdUserIDs, wantUsers) {
		t.Fatalf("message recipients = %v, want %v", notifications.createdUserIDs, wantUsers)
	}
	if len(events.events) != 1 || events.events[0].AppID != fixture.app.ID {
		t.Fatalf("app events = %#v", events.events)
	}
	var payload struct {
		Conversation struct {
			ID     string `json:"id"`
			Parent *struct {
				ID string `json:"id"`
			} `json:"parent"`
			Source *struct {
				ID string `json:"id"`
			} `json:"source_message"`
			Type string `json:"type"`
		} `json:"conversation"`
	}
	if err := json.Unmarshal(events.events[0].Payload, &payload); err != nil {
		t.Fatalf("unmarshal app event: %v", err)
	}
	if payload.Conversation.ID != topic.ID || payload.Conversation.Type != store.ConversationKindTopic || payload.Conversation.Parent == nil || payload.Conversation.Parent.ID != parent.ID || payload.Conversation.Source == nil || payload.Conversation.Source.ID != sourceID {
		t.Fatalf("topic app payload = %#v", payload)
	}
}

func TestTopicHistoryUsesTopicSequenceSpaceForVisibleParentMember(t *testing.T) {
	db := openMessageTestDB(t)
	fixture := insertMessageTestFixture(t, db)
	now := time.Date(2026, 7, 20, 6, 30, 0, 0, time.UTC)
	parent := store.Conversation{
		ID: uuid.NewString(), Kind: store.ConversationKindGroup, Name: "Parent",
		CreatedByUserID: fixture.user.ID, Status: store.ConversationStatusActive,
		PostingPolicy: store.ConversationPostingPolicyOpen, Visibility: store.ConversationVisibilityPrivate,
		CreatedAt: now, UpdatedAt: now, LastMessageSeq: 100,
	}
	if err := db.Create(&parent).Error; err != nil {
		t.Fatalf("create parent: %v", err)
	}
	if err := db.Create(&store.ConversationMember{
		ConversationID: parent.ID, MemberType: store.ConversationMemberTypeUser,
		MemberID: fixture.user.ID, Role: store.ConversationMemberRoleMember,
		JoinedAt: now, HistoryVisibleFromSeq: 100,
	}).Error; err != nil {
		t.Fatalf("create parent member: %v", err)
	}
	topic := store.Conversation{
		ID: uuid.NewString(), Kind: store.ConversationKindTopic, Name: "Topic",
		CreatedByUserID: fixture.user.ID, Status: store.ConversationStatusActive,
		PostingPolicy: store.ConversationPostingPolicyOpen, Visibility: store.ConversationVisibilityPrivate,
		CreatedAt: now, UpdatedAt: now, LastMessageSeq: 1,
	}
	if err := db.Create(&topic).Error; err != nil {
		t.Fatalf("create topic: %v", err)
	}
	sourceSenderID := fixture.user.ID
	if err := db.Create(&store.ConversationTopic{
		ConversationID: topic.ID, ParentConversationID: parent.ID,
		SourceMessageID: uuid.NewString(), SourceMessageSeq: 100,
		SourceMessageBody:    json.RawMessage(`{"type":"text","content":"source"}`),
		SourceMessageSummary: "source", SourceSenderType: store.MessageSenderTypeUser,
		SourceSenderID: &sourceSenderID, SourceSenderName: fixture.user.Name,
		SourceMessageCreatedAt: now, CreatedByUserID: fixture.user.ID,
		CreatedAt: now, UpdatedAt: now,
	}).Error; err != nil {
		t.Fatalf("create topic metadata: %v", err)
	}
	message := store.Message{
		ID: uuid.NewString(), ConversationID: topic.ID, Seq: 1,
		SenderType: store.MessageSenderTypeUser, SenderID: &sourceSenderID,
		Body: json.RawMessage(`{"type":"text","content":"topic message"}`), Summary: "topic message",
		CreatedAt: now, UpdatedAt: now,
	}
	if err := db.Create(&message).Error; err != nil {
		t.Fatalf("create topic message: %v", err)
	}

	service := NewService(Dependencies{DB: db})
	listed, err := service.List(context.Background(), ListCommand{
		AccountID: fixture.user.ID, ConversationID: topic.ID,
	})
	if err != nil {
		t.Fatalf("list topic messages: %v", err)
	}
	if len(listed.Messages) != 1 || listed.Messages[0].ID != message.ID {
		t.Fatalf("messages = %#v", listed.Messages)
	}
}

func TestParentMessageHistoryIncludesThreeRecentTopicReplies(t *testing.T) {
	db := openMessageTestDB(t)
	fixture := insertMessageTestFixture(t, db)
	now := time.Date(2026, 7, 20, 7, 0, 0, 0, time.UTC)
	source := store.Message{
		ID: uuid.NewString(), ConversationID: fixture.conversation.ID, Seq: 1,
		SenderType: store.MessageSenderTypeUser, SenderID: &fixture.user.ID,
		Body: json.RawMessage(`{"type":"text","content":"source"}`), Summary: "source",
		CreatedAt: now, UpdatedAt: now,
	}
	if err := db.Create(&source).Error; err != nil {
		t.Fatalf("create source message: %v", err)
	}
	topic := store.Conversation{
		ID: uuid.NewString(), Kind: store.ConversationKindTopic, Name: "Topic",
		CreatedByUserID: fixture.user.ID, Status: store.ConversationStatusActive,
		PostingPolicy: store.ConversationPostingPolicyOpen, Visibility: store.ConversationVisibilityPrivate,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := db.Create(&topic).Error; err != nil {
		t.Fatalf("create topic: %v", err)
	}
	if err := db.Create(&store.ConversationTopic{
		ConversationID: topic.ID, ParentConversationID: fixture.conversation.ID,
		SourceMessageID: source.ID, SourceMessageSeq: source.Seq,
		SourceMessageBody: source.Body, SourceMessageSummary: source.Summary,
		SourceSenderType: source.SenderType, SourceSenderID: source.SenderID,
		SourceSenderName: fixture.user.Name, SourceMessageCreatedAt: now,
		CreatedByUserID: fixture.user.ID, CreatedAt: now, UpdatedAt: now,
	}).Error; err != nil {
		t.Fatalf("create topic metadata: %v", err)
	}
	revokedAt := now.Add(4 * time.Minute)
	replies := []store.Message{
		{ID: uuid.NewString(), ConversationID: topic.ID, Seq: 1, SenderType: store.MessageSenderTypeUser, SenderID: &fixture.user.ID, Body: json.RawMessage(`{"type":"text","content":"first"}`), Summary: "first", CreatedAt: now.Add(time.Minute), UpdatedAt: now.Add(time.Minute)},
		{ID: uuid.NewString(), ConversationID: topic.ID, Seq: 2, SenderType: store.MessageSenderTypeApp, SenderID: &fixture.app.ID, Body: json.RawMessage(`{"type":"text","content":"second"}`), Summary: "second", CreatedAt: now.Add(2 * time.Minute), UpdatedAt: now.Add(2 * time.Minute)},
		{ID: uuid.NewString(), ConversationID: topic.ID, Seq: 3, SenderType: store.MessageSenderTypeUser, SenderID: &fixture.user.ID, Body: json.RawMessage(`{"type":"text","content":"third"}`), Summary: "third", CreatedAt: now.Add(3 * time.Minute), UpdatedAt: now.Add(3 * time.Minute)},
		{ID: uuid.NewString(), ConversationID: topic.ID, Seq: 4, SenderType: store.MessageSenderTypeUser, SenderID: &fixture.user.ID, Body: json.RawMessage(`{"type":"text","content":"revoked"}`), Summary: "revoked", RevokedAt: &revokedAt, CreatedAt: revokedAt, UpdatedAt: revokedAt},
		{ID: uuid.NewString(), ConversationID: topic.ID, Seq: 5, SenderType: store.MessageSenderTypeSystem, Body: json.RawMessage(`{"type":"system_event"}`), Summary: "closed", CreatedAt: now.Add(5 * time.Minute), UpdatedAt: now.Add(5 * time.Minute)},
		{ID: uuid.NewString(), ConversationID: topic.ID, Seq: 6, SenderType: store.MessageSenderTypeApp, SenderID: &fixture.app.ID, Body: json.RawMessage(`{"type":"text","content":"latest"}`), Summary: "latest", CreatedAt: now.Add(6 * time.Minute), UpdatedAt: now.Add(6 * time.Minute)},
	}
	if err := db.Create(&replies).Error; err != nil {
		t.Fatalf("create topic replies: %v", err)
	}

	listed, err := NewService(Dependencies{DB: db}).List(context.Background(), ListCommand{
		AccountID: fixture.user.ID, ConversationID: fixture.conversation.ID,
	})
	if err != nil {
		t.Fatalf("list parent messages: %v", err)
	}
	if len(listed.Messages) != 1 || listed.Messages[0].Topic == nil {
		t.Fatalf("listed messages = %#v", listed.Messages)
	}
	got := listed.Messages[0].Topic.RecentReplies
	if len(got) != 3 || got[0].Summary != "second" || got[1].Summary != "third" || got[2].Summary != "latest" {
		t.Fatalf("recent replies = %#v", got)
	}
	if !got[2].CreatedAt.Equal(replies[5].CreatedAt) {
		t.Fatalf("latest reply time = %v, want %v", got[2].CreatedAt, replies[5].CreatedAt)
	}
	if got[0].Sender.Type != store.MessageSenderTypeApp || got[1].Sender.Type != store.MessageSenderTypeUser {
		t.Fatalf("recent reply senders = %#v", got)
	}
}

func TestAppTopicMessageDeliversToNewlyMentionedParticipant(t *testing.T) {
	db := openMessageTestDB(t)
	fixture := insertMessageTestFixture(t, db)
	now := time.Date(2026, 7, 20, 6, 45, 0, 0, time.UTC)
	mentioned := insertTopicMessageTestUser(t, db, "app-mentioned@example.com", now)
	parent := store.Conversation{
		ID: uuid.NewString(), Kind: store.ConversationKindGroup, Name: "Parent",
		CreatedByUserID: fixture.user.ID, Status: store.ConversationStatusActive,
		PostingPolicy: store.ConversationPostingPolicyOpen, Visibility: store.ConversationVisibilityPrivate,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := db.Create(&parent).Error; err != nil {
		t.Fatalf("create parent: %v", err)
	}
	members := []store.ConversationMember{
		{ConversationID: parent.ID, MemberType: store.ConversationMemberTypeUser, MemberID: fixture.user.ID, Role: store.ConversationMemberRoleOwner, JoinedAt: now, HistoryVisibleFromSeq: 1},
		{ConversationID: parent.ID, MemberType: store.ConversationMemberTypeUser, MemberID: mentioned.ID, Role: store.ConversationMemberRoleMember, JoinedAt: now, HistoryVisibleFromSeq: 1},
		{ConversationID: parent.ID, MemberType: store.ConversationMemberTypeApp, MemberID: fixture.app.ID, Role: store.ConversationMemberRoleMember, JoinedAt: now, HistoryVisibleFromSeq: 1},
	}
	if err := db.Create(&members).Error; err != nil {
		t.Fatalf("create parent members: %v", err)
	}
	topic := store.Conversation{
		ID: uuid.NewString(), Kind: store.ConversationKindTopic, Name: "Topic",
		CreatedByUserID: fixture.user.ID, Status: store.ConversationStatusActive,
		PostingPolicy: store.ConversationPostingPolicyOpen, Visibility: store.ConversationVisibilityPrivate,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := db.Create(&topic).Error; err != nil {
		t.Fatalf("create topic: %v", err)
	}
	sourceSenderID := fixture.user.ID
	if err := db.Create(&store.ConversationTopic{
		ConversationID: topic.ID, ParentConversationID: parent.ID,
		SourceMessageID: uuid.NewString(), SourceMessageSeq: 1,
		SourceMessageBody:    json.RawMessage(`{"type":"text","content":"source"}`),
		SourceMessageSummary: "source", SourceSenderType: store.MessageSenderTypeUser,
		SourceSenderID: &sourceSenderID, SourceSenderName: fixture.user.Name,
		SourceMessageCreatedAt: now, CreatedByUserID: fixture.user.ID,
		CreatedAt: now, UpdatedAt: now,
	}).Error; err != nil {
		t.Fatalf("create topic metadata: %v", err)
	}
	if err := db.Create(&store.ConversationTopicParticipant{
		ConversationID: topic.ID, ParticipantType: store.ConversationMemberTypeUser,
		ParticipantID: fixture.user.ID, JoinedReason: store.TopicParticipantReasonCreator,
		JoinedAt: now, HistoryVisibleFromSeq: 1, CreatedAt: now, UpdatedAt: now,
	}).Error; err != nil {
		t.Fatalf("create creator participant: %v", err)
	}

	notifications := &topicMessageNotificationRecorder{}
	service := NewService(Dependencies{DB: db, Notifications: notifications})
	body := json.RawMessage(`{"type":"text","content":"hello {(@user/` + mentioned.ID + `)}"}`)
	result, err := service.CreateAsApp(context.Background(), CreateAsAppCommand{
		AppID: fixture.app.ID, ConversationID: topic.ID, ClientMessageID: "app-topic-message", Body: body,
		Finalize: func(context.Context, json.RawMessage) (json.RawMessage, string, error) {
			return body, "hello", nil
		},
	})
	if err != nil || !result.Created {
		t.Fatalf("create app topic message = %#v, err = %v", result, err)
	}
	sort.Strings(notifications.createdUserIDs)
	want := []string{fixture.user.ID, mentioned.ID}
	sort.Strings(want)
	if !equalStrings(notifications.createdUserIDs, want) {
		t.Fatalf("message recipients = %v, want %v", notifications.createdUserIDs, want)
	}
}

func TestTopicMentionDoesNotBypassParentHistoryWindow(t *testing.T) {
	db := openMessageTestDB(t)
	fixture := insertMessageTestFixture(t, db)
	now := time.Date(2026, 7, 20, 7, 0, 0, 0, time.UTC)
	hidden := insertTopicMessageTestUser(t, db, "hidden-topic-mention@example.com", now)
	parent := store.Conversation{
		ID: uuid.NewString(), Kind: store.ConversationKindGroup, Name: "Parent",
		CreatedByUserID: fixture.user.ID, Status: store.ConversationStatusActive,
		PostingPolicy: store.ConversationPostingPolicyOpen, Visibility: store.ConversationVisibilityPrivate,
		CreatedAt: now, UpdatedAt: now, LastMessageSeq: 2,
	}
	if err := db.Create(&parent).Error; err != nil {
		t.Fatalf("create parent: %v", err)
	}
	members := []store.ConversationMember{
		{ConversationID: parent.ID, MemberType: store.ConversationMemberTypeUser, MemberID: fixture.user.ID, Role: store.ConversationMemberRoleOwner, JoinedAt: now, HistoryVisibleFromSeq: 1},
		{ConversationID: parent.ID, MemberType: store.ConversationMemberTypeUser, MemberID: hidden.ID, Role: store.ConversationMemberRoleMember, JoinedAt: now, HistoryVisibleFromSeq: 2},
	}
	if err := db.Create(&members).Error; err != nil {
		t.Fatalf("create parent members: %v", err)
	}
	topic := store.Conversation{
		ID: uuid.NewString(), Kind: store.ConversationKindTopic, Name: "Topic",
		CreatedByUserID: fixture.user.ID, Status: store.ConversationStatusActive,
		PostingPolicy: store.ConversationPostingPolicyOpen, Visibility: store.ConversationVisibilityPrivate,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := db.Create(&topic).Error; err != nil {
		t.Fatalf("create topic: %v", err)
	}
	sourceSenderID := fixture.user.ID
	if err := db.Create(&store.ConversationTopic{
		ConversationID: topic.ID, ParentConversationID: parent.ID,
		SourceMessageID: uuid.NewString(), SourceMessageSeq: 1,
		SourceMessageBody:    json.RawMessage(`{"type":"text","content":"source"}`),
		SourceMessageSummary: "source", SourceSenderType: store.MessageSenderTypeUser,
		SourceSenderID: &sourceSenderID, SourceSenderName: fixture.user.Name,
		SourceMessageCreatedAt: now, CreatedByUserID: fixture.user.ID,
		CreatedAt: now, UpdatedAt: now,
	}).Error; err != nil {
		t.Fatalf("create topic metadata: %v", err)
	}
	if err := db.Create(&store.ConversationTopicParticipant{
		ConversationID: topic.ID, ParticipantType: store.ConversationMemberTypeUser,
		ParticipantID: fixture.user.ID, JoinedReason: store.TopicParticipantReasonCreator,
		JoinedAt: now, HistoryVisibleFromSeq: 1, CreatedAt: now, UpdatedAt: now,
	}).Error; err != nil {
		t.Fatalf("create creator participant: %v", err)
	}

	notifications := &topicMessageNotificationRecorder{}
	service := NewService(Dependencies{DB: db, Bodies: fixedMessageBodyProcessor{}, Notifications: notifications})
	body := json.RawMessage(`{"type":"text","content":"hello {(@user/` + hidden.ID + `)}"}`)
	if _, err := service.Create(context.Background(), CreateCommand{
		AccountID: fixture.user.ID, ConversationID: topic.ID,
		ClientMessageID: "hidden-topic-mention", Body: body,
	}); err != nil {
		t.Fatalf("create topic message: %v", err)
	}
	var hiddenParticipantCount int64
	if err := db.Model(&store.ConversationTopicParticipant{}).Where(
		"conversation_id = ? AND participant_type = ? AND participant_id = ?",
		topic.ID, store.ConversationMemberTypeUser, hidden.ID,
	).Count(&hiddenParticipantCount).Error; err != nil {
		t.Fatalf("count hidden participant: %v", err)
	}
	if hiddenParticipantCount != 0 {
		t.Fatalf("hidden participant count = %d, want 0", hiddenParticipantCount)
	}
	if len(notifications.createdUserIDs) != 1 || notifications.createdUserIDs[0] != fixture.user.ID {
		t.Fatalf("message recipients = %v, want only sender", notifications.createdUserIDs)
	}
}

type topicMessageNotificationRecorder struct {
	createdUserIDs []string
}

func (r *topicMessageNotificationRecorder) PublishMessageCreated(_ context.Context, deliveries []Delivery) {
	for _, delivery := range deliveries {
		r.createdUserIDs = append(r.createdUserIDs, delivery.UserID)
	}
}

func (r *topicMessageNotificationRecorder) PublishSharedMessageCreated(_ context.Context, userIDs []string, _ Message) {
	r.createdUserIDs = append(r.createdUserIDs, userIDs...)
}

func (*topicMessageNotificationRecorder) PublishMessageUpdated(context.Context, []Delivery) {}
func (*topicMessageNotificationRecorder) PublishMembersMentioned(context.Context, []string, string, int64) {
}
func (*topicMessageNotificationRecorder) PublishMembersChoiceReceived(context.Context, []string, string, int64) {
}
func (*topicMessageNotificationRecorder) PublishMessageChoiceUpdated(context.Context, []string, ChoiceUpdatedEvent) {
}

type topicMessageAppEventRecorder struct {
	events []AppEvent
}

func (r *topicMessageAppEventRecorder) DeliverAppEvents(_ context.Context, events []AppEvent) {
	r.events = append(r.events, events...)
}

func insertTopicMessageTestUser(t *testing.T, db *gorm.DB, email string, now time.Time) store.User {
	t.Helper()
	user := store.User{
		ID: uuid.NewString(), Email: email, Name: email, PasswordHash: "hash",
		Status: store.UserStatusActive, CreatedAt: now, UpdatedAt: now,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create topic test user: %v", err)
	}
	return user
}

func equalStrings(left, right []string) bool {
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
