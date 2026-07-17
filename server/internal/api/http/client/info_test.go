package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"app/internal/application/account"
	settingsapp "app/internal/application/settings"

	"github.com/labstack/echo/v4"
)

func TestInfoAPIReturnsPublicSettingsAndAuthenticationState(t *testing.T) {
	settings := &fakePublicSettings{info: settingsapp.PublicInfo{
		Settings:              settingsapp.Settings{AppName: "即应", OrganizationName: "长亭科技"},
		Providers:             []settingsapp.PublicProvider{{Key: "company-sso", Name: "企业 SSO"}},
		EmailCodeLoginEnabled: true,
		PasswordLoginEnabled:  true,
	}}
	sessions := &fakeInfoSessions{}
	api := NewInfoAPI(settings, sessions)
	router := echo.New()
	api.RegisterPublicRoutes(router)

	request := httptest.NewRequest(http.MethodGet, "/api/client/info", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	assertInfoResponse(t, recorder, false)
	if sessions.token != "" {
		t.Fatalf("session token = %q, want empty", sessions.token)
	}

	request = httptest.NewRequest(http.MethodGet, "/api/client/info", nil)
	request.AddCookie(&http.Cookie{Name: UserSessionCookieName, Value: "valid-session"})
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	assertInfoResponse(t, recorder, true)
	if sessions.token != "valid-session" {
		t.Fatalf("session token = %q", sessions.token)
	}
}

func assertInfoResponse(t *testing.T, recorder *httptest.ResponseRecorder, authenticated bool) {
	t.Helper()
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	data := payload["data"].(map[string]any)
	if data["app_name"] != "即应" || data["authenticated"] != authenticated ||
		data["email_code_login_enabled"] != true || data["password_login_enabled"] != true {
		t.Fatalf("response = %#v", payload)
	}
	providers := data["third_party_providers"].([]any)
	if len(providers) != 1 || providers[0].(map[string]any)["key"] != "company-sso" {
		t.Fatalf("providers = %#v", providers)
	}
}

type fakePublicSettings struct {
	info settingsapp.PublicInfo
}

func (s *fakePublicSettings) GetPublicInfo(context.Context) (settingsapp.PublicInfo, error) {
	return s.info, nil
}

type fakeInfoSessions struct {
	token string
}

func (s *fakeInfoSessions) AuthenticateSession(_ context.Context, token string) (account.AuthenticatedSession, error) {
	s.token = token
	return account.AuthenticatedSession{ID: "session-1"}, nil
}
