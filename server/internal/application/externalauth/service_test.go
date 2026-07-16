package externalauth

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"app/internal/application/identityprovider"
	"app/internal/auth"
	"app/internal/store"

	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func TestServiceStartsAndFinishesLoginWithoutChangingLegacyFlow(t *testing.T) {
	db := openExternalAuthTestDB(t)
	now := time.Date(2026, 7, 15, 8, 0, 0, 0, time.UTC)
	provider := externalAuthTestProvider(t, db)
	oauthPort := &externalAuthOAuthStub{profile: Profile{
		ExternalUserID: "external-alice", Email: " Alice@Example.com ", Name: "Alice",
		Nickname: "Ali", Phone: "13812345678", Avatar: "https://sso.test/alice.webp",
		Raw: json.RawMessage(`{"sub":"external-alice"}`),
	}}
	randomValues := []string{"login-state", "pkce-verifier"}
	service := NewService(Dependencies{
		DB: db, Providers: externalAuthProviderStub{provider: provider}, OAuth: oauthPort,
		Now: func() time.Time { return now },
		GenerateRandomValue: func(int) (string, error) {
			value := randomValues[0]
			randomValues = randomValues[1:]
			return value, nil
		},
		GenerateSessionToken: func() (string, error) { return "user-session-token", nil },
		RandomAvatar:         func() string { return "/assets/avatars/builtin/01.webp" },
	})

	started, err := service.Start(context.Background(), StartCommand{
		ProviderKey: provider.Key, Redirect: "/projects?tab=mine",
		CallbackURLForProvider: func(string) string { return "https://client.test/callback" },
		IP:                     "127.0.0.1", UserAgent: "external-auth-test",
	})
	if err != nil {
		t.Fatalf("start login: %v", err)
	}
	if started.State != "login-state" || started.AuthorizeURL != "https://sso.test/authorize-result" ||
		!started.ExpiresAt.Equal(now.Add(defaultStateTTL)) {
		t.Fatalf("start result = %#v", started)
	}
	if oauthPort.buildVerifier != "pkce-verifier" || oauthPort.buildRedirectURI != "https://client.test/callback" {
		t.Fatalf("build authorize call = %#v", oauthPort)
	}
	var loginState store.ThirdPartyLoginState
	if err := db.First(&loginState, "state_hash = ?", auth.HashSessionToken("login-state")).Error; err != nil {
		t.Fatalf("load login state: %v", err)
	}
	if loginState.RedirectPath != "/projects?tab=mine" || loginState.CodeVerifier != "pkce-verifier" {
		t.Fatalf("stored login state = %#v", loginState)
	}

	finished, err := service.Finish(context.Background(), FinishCommand{
		ProviderKey: provider.Key, Code: "callback-code", State: "login-state", CookieState: "login-state",
		CallbackURLForProvider: func(string) string { return "https://client.test/callback" },
		IP:                     "127.0.0.1", UserAgent: "external-auth-test",
	})
	if err != nil {
		t.Fatalf("finish login: %v", err)
	}
	if finished.RedirectPath != "/projects?tab=mine" || finished.Session.Token != "user-session-token" ||
		!finished.Session.ExpiresAt.Equal(now.Add(defaultSessionTTL)) {
		t.Fatalf("finish result = %#v", finished)
	}
	if oauthPort.fetchCode != "callback-code" || oauthPort.fetchVerifier != "pkce-verifier" {
		t.Fatalf("fetch profile call = %#v", oauthPort)
	}

	var user store.User
	if err := db.First(&user, "email = ?", "alice@example.com").Error; err != nil {
		t.Fatalf("load created user: %v", err)
	}
	if user.Name != "Alice" || user.Nickname != "Ali" || user.Phone == nil || *user.Phone != "+8613812345678" ||
		user.Avatar != "https://sso.test/alice.webp" {
		t.Fatalf("created user = %#v", user)
	}
	var account store.ThirdPartyAccount
	if err := db.First(&account, "provider_id = ? AND external_user_id = ?", provider.ID, "external-alice").Error; err != nil {
		t.Fatalf("load external account: %v", err)
	}
	if account.UserID != user.ID {
		t.Fatalf("external account = %#v, user = %#v", account, user)
	}
	var projectCount int64
	if err := db.Model(&store.Project{}).Where("owner_user_id = ? AND is_personal = ?", user.ID, true).Count(&projectCount).Error; err != nil || projectCount != 1 {
		t.Fatalf("personal project count = %d, err = %v", projectCount, err)
	}
	var session store.UserSession
	if err := db.First(&session, "user_id = ?", user.ID).Error; err != nil {
		t.Fatalf("load user session: %v", err)
	}
	if session.TokenHash != auth.HashSessionToken("user-session-token") {
		t.Fatalf("session token hash = %q", session.TokenHash)
	}
	if err := db.First(&loginState, "state_hash = ?", auth.HashSessionToken("login-state")).Error; err != nil || loginState.ConsumedAt == nil {
		t.Fatalf("consumed login state = %#v, err = %v", loginState, err)
	}
}

func TestServiceValidatesStateAndProfileBeforeCreatingSession(t *testing.T) {
	db := openExternalAuthTestDB(t)
	now := time.Date(2026, 7, 15, 9, 0, 0, 0, time.UTC)
	provider := externalAuthTestProvider(t, db)
	oauthPort := &externalAuthOAuthStub{profile: Profile{ExternalUserID: "missing-email"}}
	randomValues := []string{"state", "verifier"}
	service := NewService(Dependencies{
		DB: db, Providers: externalAuthProviderStub{provider: provider}, OAuth: oauthPort,
		Now: func() time.Time { return now },
		GenerateRandomValue: func(int) (string, error) {
			value := randomValues[0]
			randomValues = randomValues[1:]
			return value, nil
		},
	})
	if _, err := service.Start(context.Background(), StartCommand{ProviderKey: provider.Key}); err != nil {
		t.Fatalf("start login: %v", err)
	}
	if _, err := service.Finish(context.Background(), FinishCommand{
		ProviderKey: provider.Key, Code: "code", State: "state", CookieState: "different",
	}); ErrorCodeOf(err) != CodeInvalidRequest || ErrorMessage(err) != "第三方登录状态已失效" {
		t.Fatalf("mismatched state error = %v, code = %q", err, ErrorCodeOf(err))
	}
	if _, err := service.Finish(context.Background(), FinishCommand{
		ProviderKey: provider.Key, Code: "code", State: "state", CookieState: "state",
	}); ErrorCodeOf(err) != CodeInvalidThirdPartyLogin || ErrorMessage(err) != "第三方邮箱为空" || IsOAuthFailure(err) {
		t.Fatalf("missing email error = %v, code = %q", err, ErrorCodeOf(err))
	}
	var sessionCount int64
	if err := db.Model(&store.UserSession{}).Count(&sessionCount).Error; err != nil || sessionCount != 0 {
		t.Fatalf("session count = %d, err = %v", sessionCount, err)
	}
}

type externalAuthProviderStub struct {
	provider identityprovider.Provider
	err      error
}

func (s externalAuthProviderStub) GetEnabledByKey(context.Context, string) (identityprovider.Provider, error) {
	return s.provider, s.err
}

type externalAuthOAuthStub struct {
	profile          Profile
	buildVerifier    string
	buildRedirectURI string
	fetchCode        string
	fetchVerifier    string
}

func (s *externalAuthOAuthStub) BuildAuthorizeURL(_ identityprovider.Provider, _ string, redirectURI string, verifier string) (string, error) {
	s.buildRedirectURI = redirectURI
	s.buildVerifier = verifier
	return "https://sso.test/authorize-result", nil
}

func (s *externalAuthOAuthStub) FetchProfile(_ context.Context, _ identityprovider.Provider, code, _ string, verifier string) (Profile, error) {
	s.fetchCode = code
	s.fetchVerifier = verifier
	return s.profile, nil
}

func externalAuthTestProvider(t *testing.T, db *gorm.DB) identityprovider.Provider {
	t.Helper()
	value := store.ThirdPartyLoginProvider{
		ID: uuid.NewString(), Name: "Enterprise SSO", Key: "enterprise", Type: identityprovider.TypeOIDC,
		Enabled: true, ClientID: "client-id", ClientSecret: "client-secret",
		Scopes: json.RawMessage(`[]`), Config: json.RawMessage(`{}`),
	}
	if err := db.Create(&value).Error; err != nil {
		t.Fatalf("create provider: %v", err)
	}
	return identityprovider.Provider{
		ID: value.ID, Name: value.Name, Key: value.Key, Type: value.Type, Enabled: true,
		ClientID: value.ClientID, ClientSecret: value.ClientSecret, Scopes: []string{}, Config: map[string]any{},
	}
}

func openExternalAuthTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:external-auth-%p?mode=memory&cache=shared", t)), &gorm.Config{})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if err := db.AutoMigrate(
		&store.User{}, &store.ThirdPartyLoginProvider{}, &store.ThirdPartyLoginState{},
		&store.ThirdPartyAccount{}, &store.UserSession{}, &store.Project{},
	); err != nil {
		t.Fatalf("migrate database: %v", err)
	}
	return db
}
