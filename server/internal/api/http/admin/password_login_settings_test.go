package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	settingsapp "app/internal/application/settings"

	"github.com/labstack/echo/v4"
)

func TestPasswordLoginSettingsAPIRoutesUseApplicationService(t *testing.T) {
	service := &fakePasswordLoginSettingsService{value: settingsapp.PasswordLoginSettings{Enabled: true}}
	api := NewPasswordLoginSettingsAPI(service)
	router := echo.New()
	api.RegisterRoutes(router.Group("/api/admin"))

	request := httptest.NewRequest(http.MethodGet, "/api/admin/settings/password-login", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	assertPasswordLoginSettingsResponse(t, recorder, true)

	request = httptest.NewRequest(http.MethodPut, "/api/admin/settings/password-login", bytes.NewBufferString(`{"enabled":false}`))
	request.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	assertPasswordLoginSettingsResponse(t, recorder, false)
	if service.command.Enabled {
		t.Fatalf("update command = %#v", service.command)
	}

	request = httptest.NewRequest(http.MethodPut, "/api/admin/settings/password-login", bytes.NewBufferString(`{}`))
	request.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("empty update status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
}

func assertPasswordLoginSettingsResponse(t *testing.T, recorder *httptest.ResponseRecorder, enabled bool) {
	t.Helper()
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload["data"].(map[string]any)["enabled"] != enabled {
		t.Fatalf("response = %#v", payload)
	}
}

type fakePasswordLoginSettingsService struct {
	value   settingsapp.PasswordLoginSettings
	command settingsapp.UpdatePasswordLoginCommand
}

func (s *fakePasswordLoginSettingsService) GetPasswordLogin(context.Context) (settingsapp.PasswordLoginSettings, error) {
	return s.value, nil
}

func (s *fakePasswordLoginSettingsService) UpdatePasswordLogin(_ context.Context, cmd settingsapp.UpdatePasswordLoginCommand) (settingsapp.PasswordLoginSettings, error) {
	s.command = cmd
	s.value = settingsapp.PasswordLoginSettings{Enabled: cmd.Enabled}
	return s.value, nil
}
