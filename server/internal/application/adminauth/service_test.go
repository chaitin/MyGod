package adminauth

import (
	"context"
	"testing"
	"time"

	"app/internal/auth"
	"app/internal/store"

	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func TestServiceLoginAndAuthenticateSession(t *testing.T) {
	db := openAdminAuthTestDB(t)
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	service := NewService(Dependencies{
		DB: db, Password: "admin-password", Now: func() time.Time { return now },
		GenerateSessionToken: func() (string, error) { return "admin-session-token", nil },
		NewID:                func() string { return "10000000-0000-0000-0000-000000000001" },
	})

	result, err := service.Login(context.Background(), LoginCommand{
		Email: " admin ", Password: "admin-password", UserAgent: "admin-test", IP: "127.0.0.1",
	})
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if result.Admin.Email != "admin" || result.Session.Token != "admin-session-token" ||
		!result.Session.ExpiresAt.Equal(now.Add(defaultSessionTTL)) {
		t.Fatalf("login result = %#v", result)
	}
	var stored store.AdminSession
	if err := db.First(&stored).Error; err != nil {
		t.Fatalf("load session: %v", err)
	}
	if stored.TokenHash != auth.HashSessionToken("admin-session-token") || stored.UserAgent != "admin-test" || stored.IP != "127.0.0.1" {
		t.Fatalf("stored session = %#v", stored)
	}

	now = now.Add(time.Minute)
	authenticated, err := service.AuthenticateSession(context.Background(), "admin-session-token")
	if err != nil || authenticated.ID != stored.ID {
		t.Fatalf("authenticate = %#v, err = %v", authenticated, err)
	}
	if err := db.First(&stored, "id = ?", stored.ID).Error; err != nil {
		t.Fatalf("reload session: %v", err)
	}
	if !stored.LastSeenAt.Equal(now) {
		t.Fatalf("last seen at = %v, want %v", stored.LastSeenAt, now)
	}

	now = result.Session.ExpiresAt
	if _, err := service.AuthenticateSession(context.Background(), "admin-session-token"); ErrorCodeOf(err) != CodeUnauthorized {
		t.Fatalf("authenticate expired session err = %v", err)
	}
}

func TestServiceRejectsInvalidAdminCredentials(t *testing.T) {
	service := NewService(Dependencies{DB: openAdminAuthTestDB(t), Password: "admin-password"})
	for _, cmd := range []LoginCommand{
		{Email: "other", Password: "admin-password"},
		{Email: "admin", Password: "wrong-password"},
	} {
		if _, err := service.Login(context.Background(), cmd); ErrorCodeOf(err) != CodeInvalidCredentials || ErrorMessage(err) != "邮箱或密码错误" {
			t.Fatalf("login %#v err = %v", cmd, err)
		}
	}
	if _, err := service.AuthenticateSession(context.Background(), ""); ErrorCodeOf(err) != CodeUnauthorized {
		t.Fatalf("authenticate empty token err = %v", err)
	}
}

func openAdminAuthTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if err := db.AutoMigrate(&store.AdminSession{}); err != nil {
		t.Fatalf("migrate database: %v", err)
	}
	return db
}
