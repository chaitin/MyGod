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

func TestSettingsAPIRoutesUseApplicationService(t *testing.T) {
	service := &fakeSettingsService{value: settingsapp.Settings{AppName: "即应", OrganizationName: "长亭科技"}}
	api := NewSettingsAPI(service)
	router := echo.New()
	api.RegisterRoutes(router.Group("/api/admin"))

	request := httptest.NewRequest(http.MethodGet, "/api/admin/settings/info", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || service.getCalls != 1 {
		t.Fatalf("get status = %d, calls = %d, body = %s", recorder.Code, service.getCalls, recorder.Body.String())
	}

	request = httptest.NewRequest(http.MethodPut, "/api/admin/settings/info", bytes.NewBufferString(`{"app_name":"星环协作","organization_name":"长亭科技企业安全"}`))
	request.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("update status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if service.updateCommand.AppName != "星环协作" || service.updateCommand.OrganizationName != "长亭科技企业安全" {
		t.Fatalf("update command = %#v", service.updateCommand)
	}
	var payload map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	data := payload["data"].(map[string]any)
	if data["app_name"] != "星环协作" {
		t.Fatalf("response = %#v", payload)
	}
	if _, exists := data["password_login_enabled"]; exists {
		t.Fatalf("basic info response contains password login setting: %#v", payload)
	}
}

type fakeSettingsService struct {
	value         settingsapp.Settings
	getCalls      int
	updateCommand settingsapp.UpdateCommand
}

func (s *fakeSettingsService) Get(context.Context) (settingsapp.Settings, error) {
	s.getCalls++
	return s.value, nil
}

func (s *fakeSettingsService) Update(_ context.Context, cmd settingsapp.UpdateCommand) (settingsapp.Settings, error) {
	s.updateCommand = cmd
	s.value = settingsapp.Settings{AppName: cmd.AppName, OrganizationName: cmd.OrganizationName}
	return s.value, nil
}
