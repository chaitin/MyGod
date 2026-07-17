package client

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"app/internal/application/account"

	"github.com/labstack/echo/v4"
)

func TestAccountAPILoginMapsTransportDataAndSetsCookie(t *testing.T) {
	now := time.Date(2026, 7, 15, 4, 0, 0, 0, time.UTC)
	service := &fakeAccountService{
		loginResult: account.LoginResult{
			Account: account.Account{ID: "account-1", Email: "alice@example.com", Name: "Alice", Status: account.StatusActive, CreatedAt: now},
			Session: account.SessionCredential{Token: "session-token", ExpiresAt: now.Add(time.Hour)},
		},
	}
	api := NewAccountAPI(service, service, nil)
	router := echo.New()
	api.RegisterPublicRoutes(router)
	body := bytes.NewBufferString(`{"email":"alice@example.com","password":"secret"}`)
	request := httptest.NewRequest(http.MethodPost, "/api/client/auth/login", body)
	request.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	request.Header.Set("User-Agent", "account-api-test")
	request.RemoteAddr = "127.0.0.1:12345"
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if service.loginCommand.Email != "alice@example.com" || service.loginCommand.UserAgent != "account-api-test" || service.loginCommand.IP != "127.0.0.1" {
		t.Fatalf("login command = %#v", service.loginCommand)
	}
	response := recorder.Result()
	cookies := response.Cookies()
	if len(cookies) != 1 || cookies[0].Name != UserSessionCookieName || cookies[0].Value != "session-token" || !cookies[0].HttpOnly {
		t.Fatalf("cookies = %#v", cookies)
	}
}

func TestAccountAPIReturnsUnavailableForDisabledPasswordLogin(t *testing.T) {
	service := &fakeAccountService{loginErr: &account.Error{
		Code: account.CodeLoginUnavailable, Message: "密码登录未启用",
	}}
	api := NewAccountAPI(service, service, nil)
	router := echo.New()
	api.RegisterPublicRoutes(router)
	request := httptest.NewRequest(http.MethodPost, "/api/client/auth/login", bytes.NewBufferString(
		`{"email":"alice@example.com","password":"secret"}`,
	))
	request.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestAccountAPIProtectedRoutesUseSessionAuthenticator(t *testing.T) {
	now := time.Date(2026, 7, 15, 4, 0, 0, 0, time.UTC)
	service := &fakeAccountService{
		authenticated: account.AuthenticatedSession{
			ID: "session-1",
			Account: account.Account{
				ID:        "account-1",
				Email:     "alice@example.com",
				Name:      "Alice",
				Status:    account.StatusActive,
				CreatedAt: now,
			},
		},
	}
	hookCalled := false
	api := NewAccountAPI(service, service, func(_ echo.Context, session account.AuthenticatedSession) {
		hookCalled = session.ID == "session-1"
	})
	router := echo.New()
	group := router.Group("/api/client", api.RequireSession)
	api.RegisterProtectedRoutes(group)
	request := httptest.NewRequest(http.MethodGet, "/api/client/me", nil)
	request.AddCookie(&http.Cookie{Name: UserSessionCookieName, Value: "session-token"})
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || service.authToken != "session-token" || !hookCalled {
		t.Fatalf("status = %d, token = %q, hook = %t, body = %s", recorder.Code, service.authToken, hookCalled, recorder.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	data := payload["data"].(map[string]any)
	user := data["user"].(map[string]any)
	if user["id"] != "account-1" || user["email"] != "alice@example.com" {
		t.Fatalf("user response = %#v", user)
	}
}

type fakeAccountService struct {
	loginCommand  account.LoginCommand
	loginResult   account.LoginResult
	loginErr      error
	authToken     string
	authenticated account.AuthenticatedSession
}

func (s *fakeAccountService) Login(_ context.Context, cmd account.LoginCommand) (account.LoginResult, error) {
	s.loginCommand = cmd
	return s.loginResult, s.loginErr
}

func (s *fakeAccountService) Logout(context.Context, account.LogoutCommand) error {
	return nil
}

func (s *fakeAccountService) GetProfile(context.Context, string) (account.Account, error) {
	return s.authenticated.Account, nil
}

func (s *fakeAccountService) UpdateProfile(_ context.Context, cmd account.UpdateProfileCommand) (account.Account, error) {
	return s.authenticated.Account, nil
}

func (s *fakeAccountService) UploadAvatar(_ context.Context, cmd account.UploadAvatarCommand) (account.Account, error) {
	return s.authenticated.Account, nil
}

func (s *fakeAccountService) AuthenticateSession(_ context.Context, token string) (account.AuthenticatedSession, error) {
	s.authToken = token
	return s.authenticated, nil
}
