package usermanagement

import (
	"context"
	"testing"
	"time"

	"app/internal/store"

	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func TestServiceCreatesListsAndManagesUsers(t *testing.T) {
	db := openUserManagementTestDB(t)
	now := time.Date(2026, 7, 15, 13, 0, 0, 0, time.UTC)
	userID := "10000000-0000-0000-0000-000000000001"
	passwords := []string{"initial-password", "new-password"}
	presence := &fakeUserPresence{online: map[string]bool{}}
	service := NewService(Dependencies{
		DB: db, Presence: presence, Now: func() time.Time { return now }, NewID: func() string { return userID },
		GenerateInitialPassword: func(int) (string, error) {
			value := passwords[0]
			passwords = passwords[1:]
			return value, nil
		},
		HashPassword:   func(value string) (string, error) { return "hash:" + value, nil },
		GenerateAvatar: func() string { return "/assets/avatars/builtin/07.webp" },
	})

	created, err := service.Create(context.Background(), CreateCommand{
		Email: " Alice@Example.com ", Name: " Alice ", Phone: "138 1234 5678",
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if created.InitialPassword != "initial-password" || created.User.ID != userID ||
		created.User.Email != "alice@example.com" || created.User.Name != "Alice" ||
		created.User.Phone != "+8613812345678" || created.User.Online {
		t.Fatalf("created result = %#v", created)
	}
	var storedUser store.User
	if err := db.First(&storedUser, "id = ?", userID).Error; err != nil {
		t.Fatalf("load user: %v", err)
	}
	if storedUser.PasswordHash != "hash:initial-password" || storedUser.Avatar != "/assets/avatars/builtin/07.webp" {
		t.Fatalf("stored user = %#v", storedUser)
	}
	var personalProject store.Project
	if err := db.First(&personalProject, "owner_user_id = ? AND is_personal = ?", userID, true).Error; err != nil {
		t.Fatalf("load personal project: %v", err)
	}
	if personalProject.Name != "个人工作区" || !personalProject.CreatedAt.Equal(now) {
		t.Fatalf("personal project = %#v", personalProject)
	}

	presence.online[userID] = true
	listed, err := service.List(context.Background(), ListCommand{
		Keyword: "ALICE", Page: "1", PageSize: "20", Sort: "email", Order: "asc",
	})
	if err != nil {
		t.Fatalf("list users: %v", err)
	}
	if listed.Total != 1 || len(listed.Users) != 1 || !listed.Users[0].Online ||
		listed.Page != 1 || listed.PageSize != 20 || listed.Sort != "email" || listed.Order != "asc" {
		t.Fatalf("list result = %#v", listed)
	}

	createUserManagementSession(t, db, storedUser, "before-disable")
	disabled, err := service.SetStatus(context.Background(), SetStatusCommand{UserID: userID, Status: StatusDisabled})
	if err != nil {
		t.Fatalf("disable user: %v", err)
	}
	if disabled.Status != StatusDisabled || disabled.Online || presence.closeCalls != 1 {
		t.Fatalf("disabled user = %#v, close calls = %d", disabled, presence.closeCalls)
	}
	var sessionCount int64
	if err := db.Model(&store.UserSession{}).Where("user_id = ?", userID).Count(&sessionCount).Error; err != nil || sessionCount != 0 {
		t.Fatalf("sessions after disable = %d, err = %v", sessionCount, err)
	}

	enabled, err := service.SetStatus(context.Background(), SetStatusCommand{UserID: userID, Status: StatusActive})
	if err != nil || enabled.Status != StatusActive {
		t.Fatalf("enable user = %#v, err = %v", enabled, err)
	}
	presence.online[userID] = true
	createUserManagementSession(t, db, storedUser, "before-reset")
	reset, err := service.ResetPassword(context.Background(), userID)
	if err != nil {
		t.Fatalf("reset password: %v", err)
	}
	if reset.NewPassword != "new-password" || !reset.User.Online || presence.closeCalls != 1 {
		t.Fatalf("reset result = %#v, close calls = %d", reset, presence.closeCalls)
	}
	if err := db.First(&storedUser, "id = ?", userID).Error; err != nil {
		t.Fatalf("reload user: %v", err)
	}
	if storedUser.PasswordHash != "hash:new-password" {
		t.Fatalf("password hash = %q", storedUser.PasswordHash)
	}
	if err := db.Model(&store.UserSession{}).Where("user_id = ?", userID).Count(&sessionCount).Error; err != nil || sessionCount != 0 {
		t.Fatalf("sessions after reset = %d, err = %v", sessionCount, err)
	}
}

func TestServiceValidatesUserManagementInput(t *testing.T) {
	db := openUserManagementTestDB(t)
	service := NewService(Dependencies{
		DB: db, GenerateInitialPassword: func(int) (string, error) { return "password", nil },
		HashPassword: func(value string) (string, error) { return value, nil },
	})
	for name, cmd := range map[string]CreateCommand{
		"invalid email": {Email: "invalid", Name: "Alice"},
		"empty name":    {Email: "alice@example.com"},
		"invalid phone": {Email: "alice@example.com", Name: "Alice", Phone: "123"},
	} {
		if _, err := service.Create(context.Background(), cmd); ErrorCodeOf(err) != CodeInvalidRequest {
			t.Fatalf("%s err = %v, code = %q", name, err, ErrorCodeOf(err))
		}
	}
	if _, err := service.List(context.Background(), ListCommand{Sort: "password_hash"}); ErrorCodeOf(err) != CodeInvalidRequest || ErrorMessage(err) != "排序字段不支持" {
		t.Fatalf("invalid sort err = %v", err)
	}
	if _, err := service.List(context.Background(), ListCommand{Page: "0"}); ErrorCodeOf(err) != CodeInvalidRequest || ErrorMessage(err) != "页码必须是正整数" {
		t.Fatalf("invalid page err = %v", err)
	}
	if _, err := service.SetStatus(context.Background(), SetStatusCommand{UserID: "invalid", Status: StatusDisabled}); ErrorCodeOf(err) != CodeInvalidRequest {
		t.Fatalf("invalid user ID err = %v", err)
	}
	if _, err := service.ResetPassword(context.Background(), uuid.NewString()); ErrorCodeOf(err) != CodeNotFound {
		t.Fatalf("missing user err = %v", err)
	}
}

type fakeUserPresence struct {
	online     map[string]bool
	closeCalls int
}

func (p *fakeUserPresence) OnlineStatus(userIDs []string) map[string]bool {
	result := make(map[string]bool, len(userIDs))
	for _, userID := range userIDs {
		result[userID] = p.online[userID]
	}
	return result
}

func (p *fakeUserPresence) IsOnline(userID string) bool {
	return p.online[userID]
}

func (p *fakeUserPresence) CloseUser(userID string) int {
	p.closeCalls++
	wasOnline := p.online[userID]
	delete(p.online, userID)
	if wasOnline {
		return 1
	}
	return 0
}

func openUserManagementTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if err := db.AutoMigrate(&store.User{}, &store.UserSession{}, &store.Project{}); err != nil {
		t.Fatalf("migrate database: %v", err)
	}
	return db
}

func createUserManagementSession(t *testing.T, db *gorm.DB, user store.User, suffix string) {
	t.Helper()
	now := time.Now().UTC()
	session := store.UserSession{
		ID: uuid.NewString(), TokenHash: "token-" + suffix, UserID: user.ID,
		ExpiresAt: now.Add(time.Hour), CreatedAt: now, LastSeenAt: now,
	}
	if err := db.Create(&session).Error; err != nil {
		t.Fatalf("create session: %v", err)
	}
}
