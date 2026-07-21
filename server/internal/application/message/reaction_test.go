package message

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"app/internal/store"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func TestSetReactionAddRemoveIdempotencyAndHistory(t *testing.T) {
	db := openMessageTestDB(t)
	fixture := insertMessageTestFixture(t, db)
	message := insertReactionTestMessage(t, db, fixture.conversation.ID, fixture.user.ID, 1, store.MessageSenderTypeUser)
	notifications := &reactionNotificationRecorder{}
	service := NewService(Dependencies{DB: db, ReactionNotifications: notifications})

	added, err := service.SetReaction(context.Background(), SetReactionCommand{
		AccountID: fixture.user.ID, ConversationID: fixture.conversation.ID,
		MessageID: message.ID, Reacted: true, Text: "  e\u0301  ",
	})
	if err != nil {
		t.Fatalf("add reaction: %v", err)
	}
	if !added.Changed || added.ReactionVersion != 1 || len(added.Reactions) != 1 ||
		added.Reactions[0].Text != "é" || added.Reactions[0].Count != 1 || !added.Reactions[0].ReactedByMe ||
		len(added.Reactions[0].Users) != 1 || added.Reactions[0].Users[0] != (ReactionUser{ID: fixture.user.ID, Name: fixture.user.Name}) {
		t.Fatalf("added = %#v", added)
	}
	if len(notifications.recipients) != 1 || len(notifications.recipients[0]) != 1 ||
		notifications.recipients[0][0] != fixture.user.ID {
		t.Fatalf("recipients = %#v", notifications.recipients)
	}
	if len(notifications.events[0].Reactions) != 1 ||
		len(notifications.events[0].Reactions[0].Users) != 1 ||
		notifications.events[0].Reactions[0].Users[0] != (ReactionUser{ID: fixture.user.ID, Name: fixture.user.Name}) {
		t.Fatalf("notification event = %#v", notifications.events[0])
	}

	duplicate, err := service.SetReaction(context.Background(), SetReactionCommand{
		AccountID: fixture.user.ID, ConversationID: fixture.conversation.ID,
		MessageID: message.ID, Reacted: true, Text: "é",
	})
	if err != nil || duplicate.Changed || duplicate.ReactionVersion != 1 || len(notifications.events) != 1 {
		t.Fatalf("duplicate = %#v, err = %v, events = %d", duplicate, err, len(notifications.events))
	}

	listed, err := service.List(context.Background(), ListCommand{
		AccountID: fixture.user.ID, ConversationID: fixture.conversation.ID,
	})
	if err != nil {
		t.Fatalf("list reactions: %v", err)
	}
	if len(listed.Messages) != 1 || listed.Messages[0].ReactionVersion != 1 ||
		len(listed.Messages[0].Reactions) != 1 || !listed.Messages[0].Reactions[0].ReactedByMe {
		t.Fatalf("listed = %#v", listed.Messages)
	}

	removed, err := service.SetReaction(context.Background(), SetReactionCommand{
		AccountID: fixture.user.ID, ConversationID: fixture.conversation.ID,
		MessageID: message.ID, Reacted: false, Text: "e\u0301",
	})
	if err != nil || !removed.Changed || removed.ReactionVersion != 2 || len(removed.Reactions) != 0 {
		t.Fatalf("removed = %#v, err = %v", removed, err)
	}
	removedAgain, err := service.SetReaction(context.Background(), SetReactionCommand{
		AccountID: fixture.user.ID, ConversationID: fixture.conversation.ID,
		MessageID: message.ID, Reacted: false, Text: "é",
	})
	if err != nil || removedAgain.Changed || removedAgain.ReactionVersion != 2 || len(notifications.events) != 2 {
		t.Fatalf("removed again = %#v, err = %v, events = %d", removedAgain, err, len(notifications.events))
	}
}

func TestSetReactionValidatesTextAndLimits(t *testing.T) {
	db := openMessageTestDB(t)
	fixture := insertMessageTestFixture(t, db)
	message := insertReactionTestMessage(t, db, fixture.conversation.ID, fixture.user.ID, 1, store.MessageSenderTypeApp)
	service := NewService(Dependencies{DB: db})

	for _, text := range []string{
		"", " \n ", "hello\nworld", "hello\u2028world", "hello\u2029world", string(make([]byte, 129)),
	} {
		_, err := service.SetReaction(context.Background(), SetReactionCommand{
			AccountID: fixture.user.ID, ConversationID: fixture.conversation.ID,
			MessageID: message.ID, Reacted: true, Text: text,
		})
		if ErrorCodeOf(err) != CodeInvalidRequest {
			t.Fatalf("text %q error = %v, want invalid_request", text, err)
		}
	}
	for index := 0; index < maxReactionsPerUserMessage; index++ {
		_, err := service.SetReaction(context.Background(), SetReactionCommand{
			AccountID: fixture.user.ID, ConversationID: fixture.conversation.ID,
			MessageID: message.ID, Reacted: true, Text: fmt.Sprintf("r%d", index),
		})
		if err != nil {
			t.Fatalf("add user reaction %d: %v", index, err)
		}
	}
	_, err := service.SetReaction(context.Background(), SetReactionCommand{
		AccountID: fixture.user.ID, ConversationID: fixture.conversation.ID,
		MessageID: message.ID, Reacted: true, Text: "ninth",
	})
	if ErrorCodeOf(err) != CodeConflict {
		t.Fatalf("ninth reaction error = %v, want conflict", err)
	}

	second := insertReactionTestUser(t, db, "reaction-second@example.com")
	convertReactionTestConversationToGroup(t, db, fixture.conversation.ID)
	if err := db.Create(&store.ConversationMember{
		ConversationID: fixture.conversation.ID, MemberType: store.ConversationMemberTypeUser,
		MemberID: second.ID, Role: store.ConversationMemberRoleMember,
		JoinedAt: time.Now().UTC(), HistoryVisibleFromSeq: 1,
	}).Error; err != nil {
		t.Fatalf("create second member: %v", err)
	}
	if err := db.Where("message_id = ?", message.ID).Delete(&store.MessageReaction{}).Error; err != nil {
		t.Fatalf("clear reactions: %v", err)
	}
	now := time.Now().UTC()
	values := make([]store.MessageReaction, maxDistinctReactionsPerMessage)
	for index := range values {
		values[index] = store.MessageReaction{
			MessageID: message.ID, UserID: fixture.user.ID, Text: fmt.Sprintf("distinct-%d", index),
			CreatedAt: now.Add(time.Duration(index) * time.Millisecond),
		}
	}
	if err := db.Create(&values).Error; err != nil {
		t.Fatalf("seed distinct reactions: %v", err)
	}
	_, err = service.SetReaction(context.Background(), SetReactionCommand{
		AccountID: second.ID, ConversationID: fixture.conversation.ID,
		MessageID: message.ID, Reacted: true, Text: "overflow",
	})
	if ErrorCodeOf(err) != CodeConflict {
		t.Fatalf("distinct overflow error = %v, want conflict", err)
	}
}

func TestSetReactionEnforcesMessageAndConversationRules(t *testing.T) {
	db := openMessageTestDB(t)
	fixture := insertMessageTestFixture(t, db)
	now := time.Now().UTC()
	revoked := insertReactionTestMessage(t, db, fixture.conversation.ID, fixture.user.ID, 1, store.MessageSenderTypeUser)
	revokedAt := now
	if err := db.Model(&store.Message{}).Where("id = ?", revoked.ID).Update("revoked_at", revokedAt).Error; err != nil {
		t.Fatalf("revoke seed message: %v", err)
	}
	system := insertReactionTestMessage(t, db, fixture.conversation.ID, "", 2, store.MessageSenderTypeSystem)
	deleted := insertReactionTestMessage(t, db, fixture.conversation.ID, fixture.user.ID, 3, store.MessageSenderTypeUser)
	if err := db.Model(&store.Message{}).Where("id = ?", deleted.ID).Update("deleted_at", now).Error; err != nil {
		t.Fatalf("delete seed message: %v", err)
	}
	service := NewService(Dependencies{DB: db})
	for _, messageID := range []string{revoked.ID, system.ID} {
		_, err := service.SetReaction(context.Background(), SetReactionCommand{
			AccountID: fixture.user.ID, ConversationID: fixture.conversation.ID,
			MessageID: messageID, Reacted: true, Text: "👍",
		})
		if ErrorCodeOf(err) != CodeConflict {
			t.Fatalf("unavailable message %s error = %v, want conflict", messageID, err)
		}
	}
	_, err := service.SetReaction(context.Background(), SetReactionCommand{
		AccountID: fixture.user.ID, ConversationID: fixture.conversation.ID,
		MessageID: deleted.ID, Reacted: true, Text: "👍",
	})
	if ErrorCodeOf(err) != CodeNotFound {
		t.Fatalf("deleted message error = %v, want not_found", err)
	}
	visibleMessage := insertReactionTestMessage(t, db, fixture.conversation.ID, fixture.user.ID, 4, store.MessageSenderTypeUser)
	if err := db.Model(&store.ConversationMember{}).Where(
		"conversation_id = ? AND member_type = ? AND member_id = ?",
		fixture.conversation.ID, store.ConversationMemberTypeUser, fixture.user.ID,
	).Update("history_visible_from_seq", 5).Error; err != nil {
		t.Fatalf("update history visibility: %v", err)
	}
	_, err = service.SetReaction(context.Background(), SetReactionCommand{
		AccountID: fixture.user.ID, ConversationID: fixture.conversation.ID,
		MessageID: visibleMessage.ID, Reacted: true, Text: "👍",
	})
	if ErrorCodeOf(err) != CodeForbidden {
		t.Fatalf("hidden history message error = %v, want forbidden", err)
	}

	outsider := insertReactionTestUser(t, db, "reaction-outsider@example.com")
	_, err = service.SetReaction(context.Background(), SetReactionCommand{
		AccountID: outsider.ID, ConversationID: fixture.conversation.ID,
		MessageID: revoked.ID, Reacted: false, Text: "👍",
	})
	if ErrorCodeOf(err) != CodeForbidden {
		t.Fatalf("outsider error = %v, want forbidden", err)
	}
}

func TestSetReactionRemovalRequiresCurrentDirectAppAccess(t *testing.T) {
	db := openMessageTestDB(t)
	fixture := insertMessageTestFixture(t, db)
	message := insertReactionTestMessage(t, db, fixture.conversation.ID, fixture.user.ID, 1, store.MessageSenderTypeUser)
	service := NewService(Dependencies{DB: db})
	command := SetReactionCommand{
		AccountID: fixture.user.ID, ConversationID: fixture.conversation.ID,
		MessageID: message.ID, Reacted: true, Text: "👍",
	}
	if _, err := service.SetReaction(context.Background(), command); err != nil {
		t.Fatalf("add reaction: %v", err)
	}
	if err := db.Model(&store.App{}).Where("id = ?", fixture.app.ID).
		Update("visibility", store.AppVisibilityRestricted).Error; err != nil {
		t.Fatalf("restrict app: %v", err)
	}

	command.Reacted = false
	if _, err := service.SetReaction(context.Background(), command); ErrorCodeOf(err) != CodeForbidden || ErrorMessage(err) != "你当前无权直接使用此应用" {
		t.Fatalf("remove reaction error = %v", err)
	}
	if _, err := service.List(context.Background(), ListCommand{
		AccountID: fixture.user.ID, ConversationID: fixture.conversation.ID,
	}); err != nil {
		t.Fatalf("retained direct history: %v", err)
	}
}

func TestSetReactionRequiresTopicParticipationToAddButAllowsRemovalAfterArchive(t *testing.T) {
	db := openMessageTestDB(t)
	fixture := insertMessageTestFixture(t, db)
	now := time.Now().UTC()
	parent := store.Conversation{
		ID: uuid.NewString(), Kind: store.ConversationKindGroup, Name: "Parent",
		CreatedByUserID: fixture.user.ID, Status: store.ConversationStatusActive,
		PostingPolicy: store.ConversationPostingPolicyOpen, Visibility: store.ConversationVisibilityPrivate,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := db.Create(&parent).Error; err != nil {
		t.Fatalf("create parent: %v", err)
	}
	if err := db.Create(&store.ConversationMember{
		ConversationID: parent.ID, MemberType: store.ConversationMemberTypeUser,
		MemberID: fixture.user.ID, Role: store.ConversationMemberRoleOwner,
		JoinedAt: now, HistoryVisibleFromSeq: 1,
	}).Error; err != nil {
		t.Fatalf("create parent member: %v", err)
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
	message := insertReactionTestMessage(t, db, topic.ID, fixture.user.ID, 1, store.MessageSenderTypeUser)
	notifications := &reactionNotificationRecorder{}
	service := NewService(Dependencies{DB: db, ReactionNotifications: notifications})
	command := SetReactionCommand{
		AccountID: fixture.user.ID, ConversationID: topic.ID,
		MessageID: message.ID, Reacted: true, Text: "🎉",
	}
	if _, err := service.SetReaction(context.Background(), command); ErrorCodeOf(err) != CodeForbidden {
		t.Fatalf("non-participant add error = %v, want forbidden", err)
	}
	if err := db.Create(&store.ConversationTopicParticipant{
		ConversationID: topic.ID, ParticipantType: store.ConversationMemberTypeUser,
		ParticipantID: fixture.user.ID, JoinedReason: store.TopicParticipantReasonManual,
		JoinedAt: now, HistoryVisibleFromSeq: 1, CreatedAt: now, UpdatedAt: now,
	}).Error; err != nil {
		t.Fatalf("create participant: %v", err)
	}
	if _, err := service.SetReaction(context.Background(), command); err != nil {
		t.Fatalf("participant add: %v", err)
	}
	if len(notifications.recipients) != 1 || len(notifications.recipients[0]) != 1 ||
		notifications.recipients[0][0] != fixture.user.ID {
		t.Fatalf("topic recipients = %#v", notifications.recipients)
	}
	if err := db.Model(&store.Conversation{}).Where("id = ?", topic.ID).
		Update("posting_policy", store.ConversationPostingPolicyMuted).Error; err != nil {
		t.Fatalf("archive topic conversation: %v", err)
	}
	command.Text = "👀"
	if _, err := service.SetReaction(context.Background(), command); ErrorCodeOf(err) != CodeForbidden {
		t.Fatalf("archived add error = %v, want forbidden", err)
	}
	command.Text = "🎉"
	command.Reacted = false
	if result, err := service.SetReaction(context.Background(), command); err != nil || !result.Changed {
		t.Fatalf("archived remove = %#v, err = %v", result, err)
	}
}

func TestRevokeClearsMessageReactionsAndKeepsTombstoneVersion(t *testing.T) {
	db := openMessageTestDB(t)
	fixture := insertMessageTestFixture(t, db)
	message := insertReactionTestMessage(t, db, fixture.conversation.ID, fixture.user.ID, 1, store.MessageSenderTypeUser)
	service := NewService(Dependencies{DB: db})
	if _, err := service.SetReaction(context.Background(), SetReactionCommand{
		AccountID: fixture.user.ID, ConversationID: fixture.conversation.ID,
		MessageID: message.ID, Reacted: true, Text: "👍",
	}); err != nil {
		t.Fatalf("add reaction: %v", err)
	}
	revoked, err := service.Revoke(context.Background(), RevokeCommand{
		AccountID: fixture.user.ID, ConversationID: fixture.conversation.ID, MessageID: message.ID,
	})
	if err != nil {
		t.Fatalf("revoke message: %v", err)
	}
	if revoked.Message.ReactionVersion != 2 || len(revoked.Message.Reactions) != 0 {
		t.Fatalf("revoked message = %#v", revoked.Message)
	}
	var reactionCount, stateCount int64
	db.Model(&store.MessageReaction{}).Where("message_id = ?", message.ID).Count(&reactionCount)
	db.Model(&store.MessageReactionState{}).Where("message_id = ?", message.ID).Count(&stateCount)
	var state store.MessageReactionState
	if err := db.First(&state, "message_id = ?", message.ID).Error; err != nil {
		t.Fatalf("load reaction state: %v", err)
	}
	if reactionCount != 0 || stateCount != 1 || state.Version != 2 {
		t.Fatalf("reaction rows = %d, state rows = %d, version = %d", reactionCount, stateCount, state.Version)
	}
	listed, err := service.List(context.Background(), ListCommand{
		AccountID: fixture.user.ID, ConversationID: fixture.conversation.ID,
	})
	if err != nil {
		t.Fatalf("list revoked message: %v", err)
	}
	var listedRevoked *Message
	for index := range listed.Messages {
		if listed.Messages[index].ID == message.ID {
			listedRevoked = &listed.Messages[index]
			break
		}
	}
	if listedRevoked == nil || listedRevoked.ReactionVersion != 2 || len(listedRevoked.Reactions) != 0 {
		t.Fatalf("listed revoked message = %#v", listedRevoked)
	}
}

func TestListReactionSnapshotsReturnsOrderedPerUserStateAndRevokedTombstone(t *testing.T) {
	db := openMessageTestDB(t)
	fixture := insertMessageTestFixture(t, db)
	convertReactionTestConversationToGroup(t, db, fixture.conversation.ID)
	second := insertReactionTestUser(t, db, "reaction-snapshot-second@example.com")
	if err := db.Create(&store.ConversationMember{
		ConversationID: fixture.conversation.ID, MemberType: store.ConversationMemberTypeUser,
		MemberID: second.ID, Role: store.ConversationMemberRoleMember,
		JoinedAt: time.Now().UTC(), HistoryVisibleFromSeq: 1,
	}).Error; err != nil {
		t.Fatalf("create second member: %v", err)
	}
	first := insertReactionTestMessage(t, db, fixture.conversation.ID, fixture.user.ID, 1, store.MessageSenderTypeUser)
	secondMessage := insertReactionTestMessage(t, db, fixture.conversation.ID, fixture.user.ID, 2, store.MessageSenderTypeUser)
	service := NewService(Dependencies{DB: db})
	if _, err := service.SetReaction(context.Background(), SetReactionCommand{
		AccountID: fixture.user.ID, ConversationID: fixture.conversation.ID,
		MessageID: first.ID, Reacted: true, Text: "👍",
	}); err != nil {
		t.Fatalf("add first reaction: %v", err)
	}
	if _, err := service.SetReaction(context.Background(), SetReactionCommand{
		AccountID: second.ID, ConversationID: fixture.conversation.ID,
		MessageID: secondMessage.ID, Reacted: true, Text: "🎉",
	}); err != nil {
		t.Fatalf("add second reaction: %v", err)
	}
	if _, err := service.Revoke(context.Background(), RevokeCommand{
		AccountID: fixture.user.ID, ConversationID: fixture.conversation.ID, MessageID: secondMessage.ID,
	}); err != nil {
		t.Fatalf("revoke second message: %v", err)
	}

	result, err := service.ListReactionSnapshots(context.Background(), ListReactionSnapshotsCommand{
		AccountID: second.ID, ConversationID: fixture.conversation.ID,
		MessageIDs: []string{secondMessage.ID, first.ID, first.ID},
	})
	if err != nil {
		t.Fatalf("list reaction snapshots: %v", err)
	}
	if result.ConversationID != fixture.conversation.ID || len(result.Snapshots) != 2 {
		t.Fatalf("result = %#v", result)
	}
	if result.Snapshots[0].MessageID != secondMessage.ID || result.Snapshots[0].ReactionVersion != 2 ||
		len(result.Snapshots[0].Reactions) != 0 {
		t.Fatalf("revoked snapshot = %#v", result.Snapshots[0])
	}
	if result.Snapshots[1].MessageID != first.ID || result.Snapshots[1].ReactionVersion != 1 ||
		len(result.Snapshots[1].Reactions) != 1 || result.Snapshots[1].Reactions[0].ReactedByMe {
		t.Fatalf("first snapshot = %#v", result.Snapshots[1])
	}

	ownerResult, err := service.ListReactionSnapshots(context.Background(), ListReactionSnapshotsCommand{
		AccountID: fixture.user.ID, ConversationID: fixture.conversation.ID, MessageIDs: []string{first.ID},
	})
	if err != nil || len(ownerResult.Snapshots) != 1 || !ownerResult.Snapshots[0].Reactions[0].ReactedByMe {
		t.Fatalf("owner result = %#v, err = %v", ownerResult, err)
	}
}

func TestReactionQueriesReturnTenUserSummaryAndCompleteUserList(t *testing.T) {
	db := openMessageTestDB(t)
	fixture := insertMessageTestFixture(t, db)
	convertReactionTestConversationToGroup(t, db, fixture.conversation.ID)
	if err := db.Model(&store.User{}).Where("id = ?", fixture.user.ID).
		Updates(map[string]any{"name": "原名", "nickname": "李昌志"}).Error; err != nil {
		t.Fatalf("update fixture display name: %v", err)
	}
	users := []store.User{fixture.user}
	for index, name := range []string{
		"朱文磊", "王彪", "赵一", "钱二", "孙三", "周四", "吴五", "郑六", "王七", "冯八", "陈九",
	} {
		user := insertReactionTestUser(t, db, fmt.Sprintf("reaction-name-%d@example.com", index))
		if err := db.Model(&store.User{}).Where("id = ?", user.ID).Update("name", name).Error; err != nil {
			t.Fatalf("update reaction user name: %v", err)
		}
		if err := db.Create(&store.ConversationMember{
			ConversationID: fixture.conversation.ID, MemberType: store.ConversationMemberTypeUser,
			MemberID: user.ID, Role: store.ConversationMemberRoleMember,
			JoinedAt: time.Now().UTC(), HistoryVisibleFromSeq: 1,
		}).Error; err != nil {
			t.Fatalf("create reaction user member: %v", err)
		}
		users = append(users, user)
	}
	message := insertReactionTestMessage(t, db, fixture.conversation.ID, fixture.user.ID, 1, store.MessageSenderTypeUser)
	createdAt := time.Now().UTC()
	for index, user := range users {
		if err := db.Create(&store.MessageReaction{
			MessageID: message.ID, UserID: user.ID, Text: "🏷",
			CreatedAt: createdAt.Add(time.Duration(index) * time.Second),
		}).Error; err != nil {
			t.Fatalf("create named reaction: %v", err)
		}
	}
	if err := db.Create(&store.MessageReactionState{
		MessageID: message.ID, Version: int64(len(users)), UpdatedAt: createdAt,
	}).Error; err != nil {
		t.Fatalf("create reaction state: %v", err)
	}

	result, err := NewService(Dependencies{DB: db}).ListReactionSnapshots(
		context.Background(),
		ListReactionSnapshotsCommand{
			AccountID: fixture.user.ID, ConversationID: fixture.conversation.ID,
			MessageIDs: []string{message.ID},
		},
	)
	if err != nil {
		t.Fatalf("list named reaction snapshot: %v", err)
	}
	reaction := result.Snapshots[0].Reactions[0]
	if reaction.Count != int64(len(users)) || !reaction.ReactedByMe || len(reaction.Users) != 10 ||
		reaction.Users[0] != (ReactionUser{ID: fixture.user.ID, Name: "李昌志"}) ||
		reaction.Users[9].Name != "王七" {
		t.Fatalf("named reaction = %#v", reaction)
	}

	userResult, err := NewService(Dependencies{DB: db}).ListReactionUsers(
		context.Background(),
		ListReactionUsersCommand{
			AccountID: fixture.user.ID, ConversationID: fixture.conversation.ID,
			MessageID: message.ID, Text: "  🏷  ",
		},
	)
	if err != nil {
		t.Fatalf("list reaction users: %v", err)
	}
	if userResult.Text != "🏷" || len(userResult.Users) != len(users) ||
		userResult.Users[0] != (ReactionUser{ID: fixture.user.ID, Name: "李昌志"}) ||
		userResult.Users[len(userResult.Users)-1].ID != users[len(users)-1].ID {
		t.Fatalf("reaction users = %#v", userResult)
	}
}

func TestListReactionSnapshotsValidatesIDsAndHistoryAccess(t *testing.T) {
	db := openMessageTestDB(t)
	fixture := insertMessageTestFixture(t, db)
	message := insertReactionTestMessage(t, db, fixture.conversation.ID, fixture.user.ID, 1, store.MessageSenderTypeUser)
	service := NewService(Dependencies{DB: db})

	for _, command := range []ListReactionSnapshotsCommand{
		{AccountID: fixture.user.ID, ConversationID: fixture.conversation.ID},
		{AccountID: fixture.user.ID, ConversationID: fixture.conversation.ID, MessageIDs: []string{"invalid"}},
		{AccountID: fixture.user.ID, ConversationID: fixture.conversation.ID, MessageIDs: makeReactionSnapshotTestIDs(MaxReactionSnapshotIDs + 1)},
	} {
		if _, err := service.ListReactionSnapshots(context.Background(), command); ErrorCodeOf(err) != CodeInvalidRequest {
			t.Fatalf("command = %#v, error = %v, want invalid_request", command, err)
		}
	}

	if err := db.Model(&store.ConversationMember{}).Where(
		"conversation_id = ? AND member_type = ? AND member_id = ?",
		fixture.conversation.ID, store.ConversationMemberTypeUser, fixture.user.ID,
	).Update("history_visible_from_seq", 2).Error; err != nil {
		t.Fatalf("update history visibility: %v", err)
	}
	_, err := service.ListReactionSnapshots(context.Background(), ListReactionSnapshotsCommand{
		AccountID: fixture.user.ID, ConversationID: fixture.conversation.ID, MessageIDs: []string{message.ID},
	})
	if ErrorCodeOf(err) != CodeForbidden {
		t.Fatalf("hidden history error = %v, want forbidden", err)
	}
	_, err = service.ListReactionUsers(context.Background(), ListReactionUsersCommand{
		AccountID: fixture.user.ID, ConversationID: fixture.conversation.ID,
		MessageID: message.ID, Text: "👍",
	})
	if ErrorCodeOf(err) != CodeForbidden {
		t.Fatalf("hidden reaction users error = %v, want forbidden", err)
	}

	outsider := insertReactionTestUser(t, db, "reaction-snapshot-outsider@example.com")
	_, err = service.ListReactionSnapshots(context.Background(), ListReactionSnapshotsCommand{
		AccountID: outsider.ID, ConversationID: fixture.conversation.ID, MessageIDs: []string{message.ID},
	})
	if ErrorCodeOf(err) != CodeForbidden {
		t.Fatalf("outsider error = %v, want forbidden", err)
	}
}

func TestListReactionUsersValidatesInputAndMissingMessage(t *testing.T) {
	db := openMessageTestDB(t)
	fixture := insertMessageTestFixture(t, db)
	service := NewService(Dependencies{DB: db})
	for _, command := range []ListReactionUsersCommand{
		{AccountID: fixture.user.ID, ConversationID: "invalid", MessageID: uuid.NewString(), Text: "👍"},
		{AccountID: fixture.user.ID, ConversationID: fixture.conversation.ID, MessageID: "invalid", Text: "👍"},
		{AccountID: fixture.user.ID, ConversationID: fixture.conversation.ID, MessageID: uuid.NewString()},
	} {
		if _, err := service.ListReactionUsers(context.Background(), command); ErrorCodeOf(err) != CodeInvalidRequest {
			t.Fatalf("command = %#v, error = %v, want invalid_request", command, err)
		}
	}
	_, err := service.ListReactionUsers(context.Background(), ListReactionUsersCommand{
		AccountID: fixture.user.ID, ConversationID: fixture.conversation.ID,
		MessageID: uuid.NewString(), Text: "👍",
	})
	if ErrorCodeOf(err) != CodeNotFound {
		t.Fatalf("missing message error = %v, want not_found", err)
	}
}

func makeReactionSnapshotTestIDs(count int) []string {
	result := make([]string, count)
	for index := range result {
		result[index] = uuid.NewString()
	}
	return result
}

func convertReactionTestConversationToGroup(t *testing.T, db *gorm.DB, conversationID string) {
	t.Helper()
	if err := db.Model(&store.Conversation{}).Where("id = ?", conversationID).
		Update("kind", store.ConversationKindGroup).Error; err != nil {
		t.Fatalf("convert reaction test conversation to group: %v", err)
	}
}

func insertReactionTestMessage(t *testing.T, db *gorm.DB, conversationID, senderID string, seq int64, senderType string) store.Message {
	t.Helper()
	now := time.Now().UTC().Add(time.Duration(seq) * time.Second)
	var storedSenderID *string
	if senderID != "" {
		storedSenderID = &senderID
	}
	message := store.Message{
		ID: uuid.NewString(), ConversationID: conversationID, Seq: seq,
		SenderType: senderType, SenderID: storedSenderID,
		Body: json.RawMessage(`{"type":"text","content":"message"}`), Summary: "message",
		CreatedAt: now, UpdatedAt: now,
	}
	if err := db.Create(&message).Error; err != nil {
		t.Fatalf("create message: %v", err)
	}
	if err := db.Model(&store.Conversation{}).Where("id = ? AND last_message_seq < ?", conversationID, seq).
		Updates(map[string]any{
			"last_message_id": message.ID, "last_message_seq": seq,
			"last_message_summary": message.Summary, "last_message_at": now,
		}).Error; err != nil {
		t.Fatalf("update conversation last message: %v", err)
	}
	return message
}

func insertReactionTestUser(t *testing.T, db *gorm.DB, email string) store.User {
	t.Helper()
	now := time.Now().UTC()
	user := store.User{
		ID: uuid.NewString(), Email: email, Name: "Reaction User", PasswordHash: "hash",
		Status: store.UserStatusActive, CreatedAt: now, UpdatedAt: now,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	return user
}

type reactionNotificationRecorder struct {
	events     []ReactionEvent
	recipients [][]string
}

func (r *reactionNotificationRecorder) PublishMessageReactionsUpdated(_ context.Context, userIDs []string, event ReactionEvent) {
	r.events = append(r.events, event)
	r.recipients = append(r.recipients, append([]string(nil), userIDs...))
}
