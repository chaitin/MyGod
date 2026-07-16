package conversation

import (
	"context"
	"testing"
	"time"

	"app/internal/store"

	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func TestServiceGroupLifecyclePublishesAfterCommit(t *testing.T) {
	db := openConversationTestDB(t)
	now := time.Date(2026, 7, 15, 6, 0, 0, 0, time.UTC)
	owner := insertConversationTestUser(t, db, "owner@example.com", "Owner", now)
	member := insertConversationTestUser(t, db, "member@example.com", "Member", now)
	project := store.Project{ID: uuid.NewString(), Name: "Release", OwnerUserID: owner.ID, CreatedByUserID: owner.ID, CreatedAt: now, UpdatedAt: now}
	if err := db.Create(&project).Error; err != nil {
		t.Fatalf("create project: %v", err)
	}
	notifications := &conversationNotificationRecorder{db: db}
	service := NewService(Dependencies{DB: db, Notifications: notifications, Now: func() time.Time { return now }})

	created, err := service.CreateGroup(context.Background(), CreateGroupCommand{
		Actor: actorFromTestUser(owner), Name: " Release group ",
		MemberIDs: []string{member.ID}, ProjectIDs: []string{project.ID},
	})
	if err != nil {
		t.Fatalf("create group: %v", err)
	}
	if created.Conversation.Name != "Release group" || created.Message == nil || created.Message.Seq != 1 {
		t.Fatalf("created = %#v", created)
	}
	if notifications.messages != 1 || !notifications.sawCommittedMessage {
		t.Fatalf("notifications = %#v", notifications)
	}
	var links int64
	if err := db.Model(&store.ProjectGroup{}).Where("conversation_id = ? AND project_id = ?", created.Conversation.ID, project.ID).Count(&links).Error; err != nil || links != 1 {
		t.Fatalf("project links = %d, err = %v", links, err)
	}

	unchanged, err := service.AddMembers(context.Background(), AddMembersCommand{
		Actor: actorFromTestUser(owner), ConversationID: created.Conversation.ID, MemberIDs: []string{member.ID},
	})
	if err != nil {
		t.Fatalf("add existing member: %v", err)
	}
	if unchanged.Message != nil || notifications.messages != 1 {
		t.Fatalf("unchanged mutation = %#v, notifications = %d", unchanged, notifications.messages)
	}

	now = now.Add(time.Minute)
	updated, err := service.UpdateName(context.Background(), UpdateNameCommand{Actor: actorFromTestUser(owner), ConversationID: created.Conversation.ID, Name: "Renamed"})
	if err != nil {
		t.Fatalf("update name: %v", err)
	}
	if updated.Message == nil || updated.Message.Seq != 2 || notifications.messages != 2 {
		t.Fatalf("updated = %#v, notifications = %d", updated, notifications.messages)
	}

	now = now.Add(time.Minute)
	if _, err := service.Dissolve(context.Background(), DissolveCommand{Actor: actorFromTestUser(owner), ConversationID: created.Conversation.ID}); err != nil {
		t.Fatalf("dissolve: %v", err)
	}
	if notifications.removals != 1 || !notifications.sawCommittedRemoval {
		t.Fatalf("removal notifications = %#v", notifications)
	}
	if err := db.Model(&store.ProjectGroup{}).Where("conversation_id = ?", created.Conversation.ID).Count(&links).Error; err != nil || links != 0 {
		t.Fatalf("remaining links = %d, err = %v", links, err)
	}
}

func TestServiceDirectAndAppConversationsRemainIdempotent(t *testing.T) {
	db := openConversationTestDB(t)
	now := time.Date(2026, 7, 15, 6, 0, 0, 0, time.UTC)
	owner := insertConversationTestUser(t, db, "owner@example.com", "Owner", now)
	member := insertConversationTestUser(t, db, "member@example.com", "Member", now)
	service := NewService(Dependencies{DB: db, Now: func() time.Time { return now }})

	first, err := service.CreateDirect(context.Background(), CreateDirectCommand{Actor: actorFromTestUser(owner), UserID: member.ID})
	if err != nil || !first.Created {
		t.Fatalf("first direct = %#v, err = %v", first, err)
	}
	second, err := service.CreateDirect(context.Background(), CreateDirectCommand{Actor: actorFromTestUser(owner), UserID: member.ID})
	if err != nil || second.Created || second.Conversation.ID != first.Conversation.ID {
		t.Fatalf("second direct = %#v, err = %v", second, err)
	}

	app := store.App{ID: uuid.NewString(), Name: "Assistant", Enabled: true, Visibility: store.AppVisibilityPublic, ConnectionSecret: "secret", CreatedAt: now, UpdatedAt: now}
	if err := db.Create(&app).Error; err != nil {
		t.Fatalf("create app: %v", err)
	}
	appFirst, err := service.CreateApp(context.Background(), CreateAppCommand{Actor: actorFromTestUser(owner), AppID: app.ID})
	if err != nil || !appFirst.Created {
		t.Fatalf("first app = %#v, err = %v", appFirst, err)
	}
	appSecond, err := service.CreateApp(context.Background(), CreateAppCommand{Actor: actorFromTestUser(owner), AppID: app.ID})
	if err != nil || appSecond.Created || appSecond.Conversation.ID != appFirst.Conversation.ID {
		t.Fatalf("second app = %#v, err = %v", appSecond, err)
	}
}

type conversationNotificationRecorder struct {
	db                  *gorm.DB
	messages            int
	removals            int
	sawCommittedMessage bool
	sawCommittedRemoval bool
}

func (r *conversationNotificationRecorder) PublishConversationMessage(_ context.Context, _ []string, message Message) {
	r.messages++
	var count int64
	if err := r.db.Model(&store.Message{}).Where("id = ?", message.ID).Count(&count).Error; err == nil && count == 1 {
		r.sawCommittedMessage = true
	}
}

func (r *conversationNotificationRecorder) PublishConversationRemoved(_ context.Context, _ []string, conversationID string) {
	r.removals++
	var conversation store.Conversation
	if err := r.db.First(&conversation, "id = ?", conversationID).Error; err == nil && conversation.Status == store.ConversationStatusDissolved {
		r.sawCommittedRemoval = true
	}
}

func openConversationTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if err := db.AutoMigrate(
		&store.User{}, &store.App{}, &store.Conversation{}, &store.ConversationMember{},
		&store.DirectConversation{}, &store.AppConversation{}, &store.Message{},
		&store.Project{}, &store.ProjectGroup{},
	); err != nil {
		t.Fatalf("migrate database: %v", err)
	}
	return db
}

func insertConversationTestUser(t *testing.T, db *gorm.DB, email, name string, now time.Time) store.User {
	t.Helper()
	user := store.User{ID: uuid.NewString(), Email: email, Name: name, Avatar: store.DefaultUserAvatar, PasswordHash: "hash", Status: store.UserStatusActive, CreatedAt: now, UpdatedAt: now}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	return user
}

func actorFromTestUser(user store.User) Actor {
	return Actor{ID: user.ID, Email: user.Email, Name: user.Name, Nickname: user.Nickname, Avatar: user.Avatar}
}
