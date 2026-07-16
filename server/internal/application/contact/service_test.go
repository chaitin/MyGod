package contact

import (
	"context"
	"testing"
	"time"

	"app/internal/store"

	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func TestServiceListsActiveUsersWithPresence(t *testing.T) {
	db := openContactTestDB(t)
	now := time.Date(2026, 7, 15, 10, 0, 0, 0, time.UTC)
	active := insertContactTestUser(t, db, "active@example.com", "Active", store.UserStatusActive, now)
	active.Avatar = ""
	lastOnlineAt := now.Add(-time.Hour)
	active.LastOnlineAt = &lastOnlineAt
	phone := "+8613900000001"
	active.Phone = &phone
	if err := db.Save(&active).Error; err != nil {
		t.Fatalf("update active user: %v", err)
	}
	_ = insertContactTestUser(t, db, "disabled@example.com", "Disabled", store.UserStatusDisabled, now)

	service := NewService(Dependencies{DB: db, UserPresence: contactUserPresence{active.ID: true}})
	result, err := service.ListUsers(context.Background(), ListUsersCommand{Keyword: " ACTIVE "})
	if err != nil {
		t.Fatalf("list users: %v", err)
	}
	if len(result.Users) != 1 {
		t.Fatalf("users = %#v, want one active user", result.Users)
	}
	got := result.Users[0]
	if got.ID != active.ID || !got.Online || got.Phone != phone || got.LastOnlineAt == nil || !got.LastOnlineAt.Equal(lastOnlineAt) {
		t.Fatalf("user = %#v", got)
	}
	if got.Avatar != store.DefaultUserAvatar || got.Type != ContactTypeUser {
		t.Fatalf("user avatar/type = %#v", got)
	}
}

func TestServiceListsVisibleAppsAndGroups(t *testing.T) {
	db := openContactTestDB(t)
	now := time.Date(2026, 7, 15, 11, 0, 0, 0, time.UTC)
	owner := insertContactTestUser(t, db, "owner@example.com", "Owner", store.UserStatusActive, now)
	other := insertContactTestUser(t, db, "other@example.com", "Other", store.UserStatusActive, now)
	publicApp := insertContactTestApp(t, db, store.App{Name: "Public App", Enabled: true, Visibility: store.AppVisibilityPublic, ConnectionSecret: "public", CreatedAt: now, UpdatedAt: now})
	creatorID := owner.ID
	creatorApp := insertContactTestApp(t, db, store.App{Name: "Creator App", Enabled: true, Visibility: store.AppVisibilityCreator, CreatorUserID: &creatorID, ConnectionSecret: "creator", CreatedAt: now, UpdatedAt: now})
	otherID := other.ID
	_ = insertContactTestApp(t, db, store.App{Name: "Hidden App", Enabled: true, Visibility: store.AppVisibilityCreator, CreatorUserID: &otherID, ConnectionSecret: "hidden", CreatedAt: now, UpdatedAt: now})

	joined := insertContactTestGroup(t, db, owner.ID, "Joined Private", store.ConversationVisibilityPrivate, now)
	public := insertContactTestGroup(t, db, other.ID, "Open Group", store.ConversationVisibilityPublic, now)
	hidden := insertContactTestGroup(t, db, other.ID, "Hidden Group", store.ConversationVisibilityPrivate, now)
	insertContactTestMember(t, db, joined.ID, store.ConversationMemberTypeUser, owner.ID, store.ConversationMemberRoleOwner, now)
	insertContactTestMember(t, db, public.ID, store.ConversationMemberTypeUser, other.ID, store.ConversationMemberRoleOwner, now)
	insertContactTestMember(t, db, hidden.ID, store.ConversationMemberTypeUser, other.ID, store.ConversationMemberRoleOwner, now)

	service := NewService(Dependencies{DB: db, AppPresence: contactAppPresence{publicApp.ID: true}})
	identity := Identity{Type: IdentityTypeUser, ID: owner.ID}
	apps, err := service.ListAppsForIdentity(context.Background(), ListForIdentityCommand{Identity: identity})
	if err != nil {
		t.Fatalf("list apps: %v", err)
	}
	if len(apps.Apps) != 2 || apps.Apps[0].ID != creatorApp.ID || apps.Apps[1].ID != publicApp.ID || !apps.Apps[1].Online {
		t.Fatalf("apps = %#v", apps.Apps)
	}
	groups, err := service.ListGroupsForIdentity(context.Background(), ListForIdentityCommand{Identity: identity})
	if err != nil {
		t.Fatalf("list groups: %v", err)
	}
	if len(groups.Groups) != 2 || groups.Groups[0].ID != joined.ID || !groups.Groups[0].Joined || groups.Groups[1].ID != public.ID || groups.Groups[1].Joined {
		t.Fatalf("groups = %#v", groups.Groups)
	}
	for _, group := range groups.Groups {
		if group.ID == hidden.ID {
			t.Fatalf("hidden group leaked: %#v", groups.Groups)
		}
	}
}

func TestServiceGroupAvatarMembersAreRoleOrderedAndLimited(t *testing.T) {
	db := openContactTestDB(t)
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	users := []store.User{
		insertContactTestUser(t, db, "member-one@example.com", "Member One", store.UserStatusActive, now),
		insertContactTestUser(t, db, "member-two@example.com", "Member Two", store.UserStatusActive, now),
		insertContactTestUser(t, db, "owner@example.com", "Owner", store.UserStatusActive, now),
		insertContactTestUser(t, db, "admin@example.com", "Admin", store.UserStatusActive, now),
		insertContactTestUser(t, db, "member-three@example.com", "Member Three", store.UserStatusActive, now),
	}
	group := insertContactTestGroup(t, db, users[2].ID, "Group", store.ConversationVisibilityPrivate, now)
	roles := []string{
		store.ConversationMemberRoleMember,
		store.ConversationMemberRoleMember,
		store.ConversationMemberRoleOwner,
		store.ConversationMemberRoleAdmin,
		store.ConversationMemberRoleMember,
	}
	for index, user := range users {
		insertContactTestMember(t, db, group.ID, store.ConversationMemberTypeUser, user.ID, roles[index], now.Add(time.Duration(index)*time.Second))
	}

	service := NewService(Dependencies{DB: db})
	result, err := service.ListGroupsForIdentity(context.Background(), ListForIdentityCommand{
		Identity: Identity{Type: IdentityTypeUser, ID: users[2].ID},
	})
	if err != nil {
		t.Fatalf("list groups: %v", err)
	}
	if len(result.Groups) != 1 || len(result.Groups[0].AvatarMembers) != 4 {
		t.Fatalf("groups = %#v", result.Groups)
	}
	wantNames := []string{"Owner", "Admin", "Member One", "Member Two"}
	for index, want := range wantNames {
		if result.Groups[0].AvatarMembers[index].Name != want {
			t.Fatalf("avatar member %d = %#v, want %q", index, result.Groups[0].AvatarMembers[index], want)
		}
	}
}

type contactUserPresence map[string]bool

func (p contactUserPresence) OnlineStatus(ids []string) map[string]bool {
	result := make(map[string]bool, len(ids))
	for _, id := range ids {
		result[id] = p[id]
	}
	return result
}

type contactAppPresence map[string]bool

func (p contactAppPresence) IsOnline(id string) bool {
	return p[id]
}

func openContactTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if err := db.AutoMigrate(&store.User{}, &store.App{}, &store.Conversation{}, &store.ConversationMember{}); err != nil {
		t.Fatalf("migrate database: %v", err)
	}
	return db
}

func insertContactTestUser(t *testing.T, db *gorm.DB, email, name, status string, now time.Time) store.User {
	t.Helper()
	value := store.User{
		ID: uuid.NewString(), Email: email, Name: name, Avatar: store.DefaultUserAvatar,
		PasswordHash: "hash", Status: status, CreatedAt: now, UpdatedAt: now,
	}
	if err := db.Create(&value).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	return value
}

func insertContactTestApp(t *testing.T, db *gorm.DB, value store.App) store.App {
	t.Helper()
	value.ID = uuid.NewString()
	if err := db.Create(&value).Error; err != nil {
		t.Fatalf("create app: %v", err)
	}
	return value
}

func insertContactTestGroup(t *testing.T, db *gorm.DB, creatorID, name, visibility string, now time.Time) store.Conversation {
	t.Helper()
	value := store.Conversation{
		ID: uuid.NewString(), Kind: store.ConversationKindGroup, Name: name,
		CreatedByUserID: creatorID, Status: store.ConversationStatusActive,
		PostingPolicy: store.ConversationPostingPolicyOpen, Visibility: visibility,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := db.Create(&value).Error; err != nil {
		t.Fatalf("create group: %v", err)
	}
	return value
}

func insertContactTestMember(t *testing.T, db *gorm.DB, conversationID, memberType, memberID, role string, joinedAt time.Time) {
	t.Helper()
	value := store.ConversationMember{
		ConversationID: conversationID, MemberType: memberType, MemberID: memberID,
		Role: role, JoinedAt: joinedAt, HistoryVisibleFromSeq: 1,
	}
	if err := db.Create(&value).Error; err != nil {
		t.Fatalf("create member: %v", err)
	}
}
