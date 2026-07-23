package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"app/internal/application/usermanagement"

	"github.com/labstack/echo/v4"
)

func TestUserAPIRoutesUseApplicationService(t *testing.T) {
	value := usermanagement.User{
		ID: "10000000-0000-0000-0000-000000000001", Email: "alice@example.com", Name: "Alice",
		Avatar: "/assets/avatars/builtin/07.webp", Status: usermanagement.StatusActive,
		CreatedAt: time.Now().UTC(), Online: true,
	}
	service := &fakeUserManagementService{value: value}
	router := echo.New()
	NewUserAPI(service).RegisterRoutes(router.Group("/api/admin"))

	response := performAdminUserRequest(router, http.MethodGet, "/api/admin/users?keyword=alice&online=true&page=2&page_size=10&sort=email&order=asc", nil)
	if response.Code != http.StatusOK || service.listCommand.Keyword != "alice" || service.listCommand.Page != "2" ||
		service.listCommand.Online != "true" || service.listCommand.PageSize != "10" || service.listCommand.Sort != "email" || service.listCommand.Order != "asc" {
		t.Fatalf("list status = %d, command = %#v, body = %s", response.Code, service.listCommand, response.Body.String())
	}

	response = performAdminUserRequest(router, http.MethodPost, "/api/admin/users", bytes.NewBufferString(`{"email":"alice@example.com","name":"Alice","phone":"13812345678"}`))
	if response.Code != http.StatusCreated || service.createCommand.Email != "alice@example.com" || service.createCommand.Phone != "13812345678" {
		t.Fatalf("create status = %d, command = %#v, body = %s", response.Code, service.createCommand, response.Body.String())
	}

	response = performAdminUserRequest(router, http.MethodPost, "/api/admin/users/"+value.ID+"/disable", bytes.NewBufferString(`{}`))
	if response.Code != http.StatusOK || service.statusCommand.UserID != value.ID || service.statusCommand.Status != usermanagement.StatusDisabled {
		t.Fatalf("disable status = %d, command = %#v", response.Code, service.statusCommand)
	}
	response = performAdminUserRequest(router, http.MethodPost, "/api/admin/users/"+value.ID+"/enable", bytes.NewBufferString(`{}`))
	if response.Code != http.StatusOK || service.statusCommand.Status != usermanagement.StatusActive {
		t.Fatalf("enable status = %d, command = %#v", response.Code, service.statusCommand)
	}
	response = performAdminUserRequest(router, http.MethodPost, "/api/admin/users/"+value.ID+"/reset-password", bytes.NewBufferString(`{}`))
	if response.Code != http.StatusOK || service.resetUserID != value.ID {
		t.Fatalf("reset status = %d, id = %q", response.Code, service.resetUserID)
	}
	var payload map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil || payload["success"] != true {
		t.Fatalf("reset response = %s, err = %v", response.Body.String(), err)
	}

	service.createErr = &usermanagement.Error{Code: usermanagement.CodeConflict, Message: "邮箱已存在"}
	response = performAdminUserRequest(router, http.MethodPost, "/api/admin/users", bytes.NewBufferString(`{"email":"alice@example.com","name":"Alice"}`))
	if response.Code != http.StatusConflict {
		t.Fatalf("conflict status = %d, body = %s", response.Code, response.Body.String())
	}
}

func performAdminUserRequest(router *echo.Echo, method string, path string, body *bytes.Buffer) *httptest.ResponseRecorder {
	var request *http.Request
	if body == nil {
		request = httptest.NewRequest(method, path, nil)
	} else {
		request = httptest.NewRequest(method, path, body)
		request.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}

type fakeUserManagementService struct {
	value         usermanagement.User
	listCommand   usermanagement.ListCommand
	createCommand usermanagement.CreateCommand
	statusCommand usermanagement.SetStatusCommand
	resetUserID   string
	createErr     error
}

func (s *fakeUserManagementService) List(_ context.Context, cmd usermanagement.ListCommand) (usermanagement.ListResult, error) {
	s.listCommand = cmd
	return usermanagement.ListResult{
		Users: []usermanagement.User{s.value}, Total: 1, Page: 2, PageSize: 10, Sort: "email", Order: "asc",
	}, nil
}

func (s *fakeUserManagementService) Create(_ context.Context, cmd usermanagement.CreateCommand) (usermanagement.CreateResult, error) {
	s.createCommand = cmd
	return usermanagement.CreateResult{User: s.value, InitialPassword: "initial-password"}, s.createErr
}

func (s *fakeUserManagementService) SetStatus(_ context.Context, cmd usermanagement.SetStatusCommand) (usermanagement.User, error) {
	s.statusCommand = cmd
	s.value.Status = cmd.Status
	return s.value, nil
}

func (s *fakeUserManagementService) ResetPassword(_ context.Context, userID string) (usermanagement.ResetPasswordResult, error) {
	s.resetUserID = userID
	return usermanagement.ResetPasswordResult{User: s.value, NewPassword: "new-password"}, nil
}
