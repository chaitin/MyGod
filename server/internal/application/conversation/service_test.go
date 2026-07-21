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

	memberID := member.ID
	restricted := store.App{ID: uuid.NewString(), Name: "Restricted", CreatorUserID: &memberID, Enabled: true, Visibility: store.AppVisibilityRestricted, ConnectionSecret: "restricted", CreatedAt: now, UpdatedAt: now}
	if err := db.Create(&restricted).Error; err != nil {
		t.Fatalf("create restricted app: %v", err)
	}
	if err := db.Create(&store.AppUserGrant{AppID: restricted.ID, UserID: owner.ID, GrantedByUserID: &memberID, CreatedAt: now}).Error; err != nil {
		t.Fatalf("create app user grant: %v", err)
	}
	if opened, err := service.CreateApp(context.Background(), CreateAppCommand{Actor: actorFromTestUser(owner), AppID: restricted.ID}); err != nil || !opened.Created {
		t.Fatalf("open granted app = %#v, err = %v", opened, err)
	}
}

func TestServiceAppConversationUsesCurrentAppProfile(t *testing.T) {
	db := openConversationTestDB(t)
	now := time.Date(2026, 7, 17, 6, 0, 0, 0, time.UTC)
	owner := insertConversationTestUser(t, db, "app-owner@example.com", "Owner", now)
	app := store.App{
		ID: uuid.NewString(), Name: "Old name", Avatar: "/old.webp", Enabled: true,
		Visibility: store.AppVisibilityPublic, ConnectionSecret: "secret", CreatedAt: now, UpdatedAt: now,
	}
	if err := db.Create(&app).Error; err != nil {
		t.Fatalf("create app: %v", err)
	}
	service := NewService(Dependencies{DB: db, Now: func() time.Time { return now }})
	created, err := service.CreateApp(context.Background(), CreateAppCommand{Actor: actorFromTestUser(owner), AppID: app.ID})
	if err != nil || !created.Created {
		t.Fatalf("create app conversation = %#v, err = %v", created, err)
	}

	if err := db.Model(&store.App{}).Where("id = ?", app.ID).Updates(map[string]any{
		"name": "New name", "avatar": "/new.webp", "updated_at": now.Add(time.Minute),
	}).Error; err != nil {
		t.Fatalf("update app profile: %v", err)
	}
	var stored store.Conversation
	if err := db.First(&stored, "id = ?", created.Conversation.ID).Error; err != nil {
		t.Fatalf("load stored conversation: %v", err)
	}
	listed, err := service.loadItem(db, stored, owner.ID)
	if err != nil {
		t.Fatalf("load app conversation item: %v", err)
	}
	if listed.Name != "New name" || listed.Avatar != "/new.webp" {
		t.Fatalf("listed app conversation profile = %#v", listed)
	}
	reopened, err := service.CreateApp(context.Background(), CreateAppCommand{Actor: actorFromTestUser(owner), AppID: app.ID})
	if err != nil || reopened.Created {
		t.Fatalf("reopen app conversation = %#v, err = %v", reopened, err)
	}
	if reopened.Conversation.Name != "New name" || reopened.Conversation.Avatar != "/new.webp" {
		t.Fatalf("app conversation profile = %#v", reopened.Conversation)
	}
}

func TestServiceUsesInvitingUsersAppAccessForGroupMembers(t *testing.T) {
	db := openConversationTestDB(t)
	now := time.Date(2026, 7, 16, 11, 0, 0, 0, time.UTC)
	owner := insertConversationTestUser(t, db, "group-owner@example.com", "Owner", now)
	other := insertConversationTestUser(t, db, "group-other@example.com", "Other", now)
	ownerID := owner.ID
	privateApp := store.App{
		ID: uuid.NewString(), Name: "Private App", CreatorUserID: &ownerID, Enabled: true,
		Visibility: store.AppVisibilityCreator, ConnectionSecret: "private", CreatedAt: now, UpdatedAt: now,
	}
	otherID := other.ID
	restrictedApp := store.App{
		ID: uuid.NewString(), Name: "Restricted App", CreatorUserID: &otherID, Enabled: true,
		Visibility: store.AppVisibilityRestricted, ConnectionSecret: "restricted", CreatedAt: now, UpdatedAt: now,
	}
	inaccessibleApp := store.App{
		ID: uuid.NewString(), Name: "Inaccessible App", CreatorUserID: &otherID, Enabled: true,
		Visibility: store.AppVisibilityCreator, ConnectionSecret: "inaccessible", CreatedAt: now, UpdatedAt: now,
	}
	publicApp := store.App{
		ID: uuid.NewString(), Name: "Public App", CreatorUserID: &ownerID, Enabled: true,
		Visibility: store.AppVisibilityPublic, ConnectionSecret: "public", CreatedAt: now, UpdatedAt: now,
	}
	disabledOwner := insertConversationTestUser(t, db, "disabled-app-owner@example.com", "Disabled", now)
	if err := db.Model(&store.User{}).Where("id = ?", disabledOwner.ID).Update("status", store.UserStatusDisabled).Error; err != nil {
		t.Fatalf("disable app owner: %v", err)
	}
	disabledOwnerApp := store.App{
		ID: uuid.NewString(), Name: "Disabled Owner App", CreatorUserID: &disabledOwner.ID, Enabled: true,
		Visibility: store.AppVisibilityPublic, ConnectionSecret: "disabled-owner-public", CreatedAt: now, UpdatedAt: now,
	}
	if err := db.Create(&[]store.App{privateApp, restrictedApp, inaccessibleApp, publicApp, disabledOwnerApp}).Error; err != nil {
		t.Fatalf("create apps: %v", err)
	}
	if err := db.Create(&store.AppUserGrant{AppID: restrictedApp.ID, UserID: owner.ID, GrantedByUserID: &otherID, CreatedAt: now}).Error; err != nil {
		t.Fatalf("create app grant: %v", err)
	}
	service := NewService(Dependencies{DB: db, Now: func() time.Time { return now }})
	created, err := service.CreateGroup(context.Background(), CreateGroupCommand{
		Actor: actorFromTestUser(owner), Name: "Private app group", AppIDs: []string{privateApp.ID},
	})
	if err != nil || created.Conversation.MemberCount != 2 {
		t.Fatalf("private app group = %#v, err = %v", created, err)
	}
	if _, err := service.CreateApp(context.Background(), CreateAppCommand{
		Actor: actorFromTestUser(owner), AppID: disabledOwnerApp.ID,
	}); ErrorCodeOf(err) != CodeNotFound {
		t.Fatalf("disabled owner app conversation error = %v", err)
	}
	if _, err := service.CreateGroup(context.Background(), CreateGroupCommand{
		Actor: actorFromTestUser(owner), Name: "Disabled owner app group", AppIDs: []string{disabledOwnerApp.ID},
	}); ErrorCodeOf(err) != CodeInvalidRequest || ErrorMessage(err) != "所选应用不存在、已停用或你无权访问" {
		t.Fatalf("disabled owner app group error = %v", err)
	}
	if _, err := service.CreateGroup(context.Background(), CreateGroupCommand{
		Actor: actorFromTestUser(owner), Name: "Inaccessible app group", AppIDs: []string{inaccessibleApp.ID},
	}); ErrorCodeOf(err) != CodeInvalidRequest || ErrorMessage(err) != "所选应用不存在、已停用或你无权访问" {
		t.Fatalf("inaccessible app group error = %v", err)
	}
	publicGroup, err := service.CreateGroup(context.Background(), CreateGroupCommand{
		Actor: actorFromTestUser(owner), Name: "Public app group", AppIDs: []string{publicApp.ID},
	})
	if err != nil || publicGroup.Conversation.MemberCount != 2 {
		t.Fatalf("public app group = %#v, err = %v", publicGroup, err)
	}
	if _, err := service.AddMembers(context.Background(), AddMembersCommand{
		Actor: actorFromTestUser(owner), ConversationID: publicGroup.Conversation.ID, AppIDs: []string{restrictedApp.ID},
	}); err != nil {
		t.Fatalf("add granted app: %v", err)
	}
	if _, err := service.AddMembers(context.Background(), AddMembersCommand{
		Actor: actorFromTestUser(owner), ConversationID: publicGroup.Conversation.ID, AppIDs: []string{inaccessibleApp.ID},
	}); ErrorCodeOf(err) != CodeInvalidRequest || ErrorMessage(err) != "所选应用不存在、已停用或你无权访问" {
		t.Fatalf("add inaccessible app error = %v", err)
	}
	if _, err := service.AddMembers(context.Background(), AddMembersCommand{
		Actor: actorFromTestUser(owner), ConversationID: publicGroup.Conversation.ID, MemberIDs: []string{uuid.NewString()},
	}); ErrorCodeOf(err) != CodeInvalidRequest || ErrorMessage(err) != "成员不存在或已禁用" {
		t.Fatalf("add missing member error = %v", err)
	}
	if _, err := service.CreateGroupAsApplication(context.Background(), CreateGroupAsApplicationCommand{
		AppID: publicApp.ID, Name: "Application group", MemberIDs: []string{owner.ID}, AppIDs: []string{privateApp.ID},
	}); ErrorCodeOf(err) != CodeInvalidRequest || ErrorMessage(err) != "只有已启用且所有人可见的应用才能加入群聊" {
		t.Fatalf("application adds private app error = %v", err)
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

func (*conversationNotificationRecorder) PublishConversationPinUpdated(context.Context, []string, ConversationPinEvent) {
}

func (*conversationNotificationRecorder) PublishTopicEvent(context.Context, []string, TopicEvent) {}

func openConversationTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if err := db.AutoMigrate(
		&store.User{}, &store.App{}, &store.AppUserGrant{}, &store.Conversation{}, &store.ConversationMember{},
		&store.ConversationPin{},
		&store.DirectConversation{}, &store.AppConversation{}, &store.Message{}, &store.MessageRegistry{},
		&store.ConversationTopic{}, &store.ConversationTopicParticipant{},
		&store.AppEventOutbox{},
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
