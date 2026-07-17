package account

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"strings"
	"testing"
	"time"

	fileapp "app/internal/application/file"
	"app/internal/auth"
	"app/internal/store"

	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func TestServiceLoginAuthenticateAndLogout(t *testing.T) {
	db := openAccountTestDB(t)
	now := time.Date(2026, 7, 15, 4, 0, 0, 0, time.UTC)
	user := insertAccountTestUser(t, db, "alice@example.com", "test-password", now)
	service := NewService(Dependencies{
		DB:                   db,
		Now:                  func() time.Time { return now },
		GenerateSessionToken: func() (string, error) { return "session-token", nil },
	})

	result, err := service.Login(context.Background(), LoginCommand{
		Email:     " Alice@Example.com ",
		Password:  "test-password",
		UserAgent: "account-test",
		IP:        "127.0.0.1",
	})
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if result.Account.ID != user.ID || result.Session.Token != "session-token" {
		t.Fatalf("login result = %#v", result)
	}
	if want := now.Add(defaultSessionTTL); !result.Session.ExpiresAt.Equal(want) {
		t.Fatalf("expires at = %v, want %v", result.Session.ExpiresAt, want)
	}

	var storedSession store.UserSession
	if err := db.First(&storedSession, "user_id = ?", user.ID).Error; err != nil {
		t.Fatalf("load session: %v", err)
	}
	if storedSession.TokenHash != auth.HashSessionToken("session-token") {
		t.Fatalf("stored token hash = %q", storedSession.TokenHash)
	}

	authenticated, err := service.AuthenticateSession(context.Background(), "session-token")
	if err != nil {
		t.Fatalf("authenticate: %v", err)
	}
	if authenticated.ID != storedSession.ID || authenticated.Account.ID != user.ID {
		t.Fatalf("authenticated session = %#v", authenticated)
	}

	if err := service.Logout(context.Background(), LogoutCommand{Token: "session-token"}); err != nil {
		t.Fatalf("logout: %v", err)
	}
	if _, err := service.AuthenticateSession(context.Background(), "session-token"); ErrorCodeOf(err) != CodeUnauthorized {
		t.Fatalf("authenticate after logout error = %v, code = %q", err, ErrorCodeOf(err))
	}
}

func TestServiceLoginDoesNotRevealDisabledOrMissingAccount(t *testing.T) {
	db := openAccountTestDB(t)
	now := time.Date(2026, 7, 15, 4, 0, 0, 0, time.UTC)
	user := insertAccountTestUser(t, db, "disabled@example.com", "test-password", now)
	if err := db.Model(&user).Update("status", store.UserStatusDisabled).Error; err != nil {
		t.Fatalf("disable user: %v", err)
	}
	service := NewService(Dependencies{DB: db, Now: func() time.Time { return now }})

	for _, email := range []string{"disabled@example.com", "missing@example.com", "not-an-email"} {
		_, err := service.Login(context.Background(), LoginCommand{Email: email, Password: "test-password"})
		if ErrorCodeOf(err) != CodeInvalidCredentials || ErrorMessage(err) != "邮箱或密码错误" {
			t.Fatalf("login %q error = %v, code = %q", email, err, ErrorCodeOf(err))
		}
	}
}

func TestServiceRejectsPasswordLoginWhenDisabled(t *testing.T) {
	policy := &fakePasswordLoginPolicy{enabled: false}
	service := NewService(Dependencies{
		DB: openAccountTestDB(t), PasswordLoginPolicy: policy,
	})

	_, err := service.Login(context.Background(), LoginCommand{
		Email: "alice@example.com", Password: "test-password",
	})
	if ErrorCodeOf(err) != CodeLoginUnavailable || ErrorMessage(err) != "密码登录未启用" {
		t.Fatalf("login error = %v, code = %q", err, ErrorCodeOf(err))
	}
	if policy.calls != 1 {
		t.Fatalf("password login policy calls = %d", policy.calls)
	}
}

func TestServiceIssuesSessionOnlyForVerifiedActiveEmail(t *testing.T) {
	db := openAccountTestDB(t)
	now := time.Date(2026, 7, 15, 4, 0, 0, 0, time.UTC)
	user := insertAccountTestUser(t, db, "alice@example.com", "test-password", now)
	service := NewService(Dependencies{
		DB:                   db,
		Now:                  func() time.Time { return now },
		GenerateSessionToken: func() (string, error) { return "verified-session-token", nil },
	})

	allowed, err := service.CanLoginWithEmail(context.Background(), " Alice@Example.com ")
	if err != nil || !allowed {
		t.Fatalf("active email allowed = %t, error = %v", allowed, err)
	}
	result, err := service.LoginWithVerifiedEmail(context.Background(), VerifiedEmailLoginCommand{
		Email: "Alice@example.com", UserAgent: "verified-email-test", IP: "127.0.0.1",
	})
	if err != nil {
		t.Fatalf("verified login: %v", err)
	}
	if result.Account.ID != user.ID || result.Session.Token != "verified-session-token" {
		t.Fatalf("verified login result = %#v", result)
	}

	if err := db.Model(&user).Update("status", store.UserStatusDisabled).Error; err != nil {
		t.Fatalf("disable user: %v", err)
	}
	allowed, err = service.CanLoginWithEmail(context.Background(), user.Email)
	if err != nil || allowed {
		t.Fatalf("disabled email allowed = %t, error = %v", allowed, err)
	}
	if _, err := service.LoginWithVerifiedEmail(context.Background(), VerifiedEmailLoginCommand{Email: user.Email}); ErrorCodeOf(err) != CodeInvalidCredentials {
		t.Fatalf("disabled verified login error = %v", err)
	}
}

func TestServiceUpdatesProfileAndOnlineActivity(t *testing.T) {
	db := openAccountTestDB(t)
	now := time.Date(2026, 7, 15, 4, 0, 0, 0, time.UTC)
	user := insertAccountTestUser(t, db, "alice@example.com", "test-password", now)
	service := NewService(Dependencies{DB: db})
	nickname := " Alice A "
	avatar := "/assets/avatars/builtin/03.webp"

	updated, err := service.UpdateProfile(context.Background(), UpdateProfileCommand{
		AccountID: user.ID,
		Avatar:    &avatar,
		Nickname:  &nickname,
	})
	if err != nil {
		t.Fatalf("update profile: %v", err)
	}
	if updated.Avatar != avatar || updated.Nickname != "Alice A" {
		t.Fatalf("updated account = %#v", updated)
	}

	activityAt := now.Add(time.Hour)
	if err := service.RecordOnlineActivity(context.Background(), user.ID, activityAt); err != nil {
		t.Fatalf("record activity: %v", err)
	}
	profile, err := service.GetProfile(context.Background(), user.ID)
	if err != nil {
		t.Fatalf("get profile: %v", err)
	}
	if profile.LastOnlineAt == nil || !profile.LastOnlineAt.Equal(activityAt) {
		t.Fatalf("last online at = %v, want %v", profile.LastOnlineAt, activityAt)
	}

	invalidAvatar := "https://example.com/avatar.webp"
	if _, err := service.UpdateProfile(context.Background(), UpdateProfileCommand{AccountID: user.ID, Avatar: &invalidAvatar}); ErrorCodeOf(err) != CodeInvalidRequest {
		t.Fatalf("invalid avatar error = %v, code = %q", err, ErrorCodeOf(err))
	}
}

func TestServiceUploadsAvatarThroughStoragePort(t *testing.T) {
	db := openAccountTestDB(t)
	now := time.Date(2026, 7, 15, 4, 0, 0, 0, time.UTC)
	user := insertAccountTestUser(t, db, "alice@example.com", "test-password", now)
	storage := &recordingAvatarStorage{}
	service := NewService(Dependencies{DB: db, Files: storage})
	content := accountTestWebP(256, 256)

	updated, err := service.UploadAvatar(context.Background(), UploadAvatarCommand{
		AccountID: user.ID,
		Size:      int64(len(content)),
		Content:   bytes.NewReader(content),
	})
	if err != nil {
		t.Fatalf("upload avatar: %v", err)
	}
	if !strings.HasPrefix(storage.key, "avatars/users/"+user.ID+"/") || !strings.HasSuffix(storage.key, ".webp") {
		t.Fatalf("avatar key = %q", storage.key)
	}
	if !bytes.Equal(storage.content, content) || storage.contentType != avatarContentType {
		t.Fatalf("stored avatar = %#v, type = %q", storage.content, storage.contentType)
	}
	if updated.Avatar != storage.url {
		t.Fatalf("updated avatar = %q, want %q", updated.Avatar, storage.url)
	}

	wrongSize := accountTestWebP(128, 128)
	if _, err := service.UploadAvatar(context.Background(), UploadAvatarCommand{
		AccountID: user.ID,
		Size:      int64(len(wrongSize)),
		Content:   bytes.NewReader(wrongSize),
	}); ErrorCodeOf(err) != CodeInvalidRequest {
		t.Fatalf("wrong dimension error = %v, code = %q", err, ErrorCodeOf(err))
	}
}

type recordingAvatarStorage struct {
	key         string
	content     []byte
	contentType string
	url         string
}

type fakePasswordLoginPolicy struct {
	enabled bool
	calls   int
}

func (p *fakePasswordLoginPolicy) PasswordLoginEnabled(context.Context) (bool, error) {
	p.calls++
	return p.enabled, nil
}

func (s *recordingAvatarStorage) UploadPublic(_ context.Context, cmd fileapp.UploadPublicCommand) (fileapp.PublicFile, error) {
	s.key = cmd.ObjectKey
	s.content, _ = io.ReadAll(cmd.Content)
	s.contentType = cmd.ContentType
	s.url = "https://assets.example.test/public/" + cmd.ObjectKey
	return fileapp.PublicFile{ObjectKey: cmd.ObjectKey, URL: s.url, ContentType: cmd.ContentType, SizeBytes: cmd.SizeBytes}, nil
}

func openAccountTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if err := db.AutoMigrate(&store.User{}, &store.UserSession{}); err != nil {
		t.Fatalf("migrate database: %v", err)
	}
	return db
}

func insertAccountTestUser(t *testing.T, db *gorm.DB, email string, password string, now time.Time) store.User {
	t.Helper()
	passwordHash, err := auth.HashPassword(password)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	user := store.User{
		ID:           uuid.NewString(),
		Email:        email,
		Name:         "Alice",
		Avatar:       store.DefaultUserAvatar,
		PasswordHash: passwordHash,
		Status:       store.UserStatusActive,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	return user
}

func accountTestWebP(width int, height int) []byte {
	content := make([]byte, 30)
	copy(content[0:4], "RIFF")
	binary.LittleEndian.PutUint32(content[4:8], uint32(len(content)-8))
	copy(content[8:12], "WEBP")
	copy(content[12:16], "VP8X")
	binary.LittleEndian.PutUint32(content[16:20], 10)
	w := width - 1
	h := height - 1
	content[24] = byte(w)
	content[25] = byte(w >> 8)
	content[26] = byte(w >> 16)
	content[27] = byte(h)
	content[28] = byte(h >> 8)
	content[29] = byte(h >> 16)
	return content
}
