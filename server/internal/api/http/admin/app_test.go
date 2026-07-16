package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	appapp "app/internal/application/app"

	"github.com/labstack/echo/v4"
)

func TestAppAPIRoutesUseApplicationService(t *testing.T) {
	value := appapp.App{
		ID: "10000000-0000-0000-0000-000000000001", Name: "知识库助手", Enabled: true,
		Visibility: appapp.VisibilityPublic, ConnectionSecret: "secret",
		ConnectionStatus: appapp.ConnectionStatusOffline, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	service := &fakeAdminAppService{value: value}
	router := echo.New()
	NewAppAPI(service).RegisterRoutes(router.Group("/api/admin"))

	response := performAdminAppRequest(router, http.MethodGet, "/api/admin/apps", nil, "")
	if response.Code != http.StatusOK || service.listCalls != 1 {
		t.Fatalf("list status = %d, calls = %d, body = %s", response.Code, service.listCalls, response.Body.String())
	}

	response = performAdminAppRequest(router, http.MethodPost, "/api/admin/apps", bytes.NewBufferString(`{"name":"知识库助手","description":"回答问题","visibility":"public"}`), echo.MIMEApplicationJSON)
	if response.Code != http.StatusCreated || service.createCommand.Name != "知识库助手" || service.createCommand.Description != "回答问题" {
		t.Fatalf("create status = %d, command = %#v, body = %s", response.Code, service.createCommand, response.Body.String())
	}

	service.callOrder = nil
	response = performAdminAppRequest(router, http.MethodPut, "/api/admin/apps/"+value.ID, bytes.NewBufferString(`{"name":"知识库 Agent","description":"更新","visibility":"public"}`), echo.MIMEApplicationJSON)
	if response.Code != http.StatusOK || service.updateCommand.AppID != value.ID || service.updateCommand.Name != "知识库 Agent" {
		t.Fatalf("update status = %d, command = %#v, body = %s", response.Code, service.updateCommand, response.Body.String())
	}
	if len(service.callOrder) != 2 || service.callOrder[0] != "get" || service.callOrder[1] != "update" {
		t.Fatalf("update call order = %#v", service.callOrder)
	}

	response = performAdminAppRequest(router, http.MethodPost, "/api/admin/apps/"+value.ID+"/disable", nil, "")
	if response.Code != http.StatusOK || service.enabledCommand.AppID != value.ID || service.enabledCommand.Enabled {
		t.Fatalf("disable status = %d, command = %#v", response.Code, service.enabledCommand)
	}
	response = performAdminAppRequest(router, http.MethodPost, "/api/admin/apps/"+value.ID+"/enable", nil, "")
	if response.Code != http.StatusOK || !service.enabledCommand.Enabled {
		t.Fatalf("enable status = %d, command = %#v", response.Code, service.enabledCommand)
	}

	response = performAdminAppRequest(router, http.MethodPost, "/api/admin/apps/"+value.ID+"/secret/regenerate", nil, "")
	if response.Code != http.StatusOK || service.regenerateID != value.ID {
		t.Fatalf("regenerate status = %d, id = %q", response.Code, service.regenerateID)
	}
	response = performAdminAppRequest(router, http.MethodDelete, "/api/admin/apps/"+value.ID, nil, "")
	if response.Code != http.StatusOK || service.deleteID != value.ID {
		t.Fatalf("delete status = %d, id = %q", response.Code, service.deleteID)
	}

	var payload map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil || payload["success"] != true {
		t.Fatalf("delete response = %s, err = %v", response.Body.String(), err)
	}
}

func TestAppAPIUploadsAvatarAndMapsApplicationErrors(t *testing.T) {
	value := appapp.App{ID: "10000000-0000-0000-0000-000000000001", Name: "知识库助手"}
	service := &fakeAdminAppService{value: value}
	router := echo.New()
	NewAppAPI(service).RegisterRoutes(router.Group("/api/admin"))

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "avatar.webp")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	content := []byte("webp-content")
	if _, err := part.Write(content); err != nil {
		t.Fatalf("write form file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}
	response := performAdminAppRequest(router, http.MethodPost, "/api/admin/apps/"+value.ID+"/avatar", &body, writer.FormDataContentType())
	if response.Code != http.StatusOK || service.uploadCommand.AppID != value.ID || !bytes.Equal(service.uploadedContent, content) {
		t.Fatalf("upload status = %d, command = %#v, content = %q, body = %s", response.Code, service.uploadCommand, service.uploadedContent, response.Body.String())
	}

	service.getErr = &appapp.Error{Code: appapp.CodeNotFound, Message: "应用不存在"}
	response = performAdminAppRequest(router, http.MethodPut, "/api/admin/apps/"+value.ID, bytes.NewBufferString(`{"name":"不会更新"}`), echo.MIMEApplicationJSON)
	if response.Code != http.StatusNotFound {
		t.Fatalf("not found status = %d, body = %s", response.Code, response.Body.String())
	}
	service.getErr = nil
	service.regenerateErr = &appapp.Error{Code: appapp.CodeForbidden, Message: "茉莉密钥由配置管理"}
	response = performAdminAppRequest(router, http.MethodPost, "/api/admin/apps/"+value.ID+"/secret/regenerate", nil, "")
	if response.Code != http.StatusForbidden {
		t.Fatalf("forbidden status = %d, body = %s", response.Code, response.Body.String())
	}
}

func performAdminAppRequest(router *echo.Echo, method string, path string, body io.Reader, contentType string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, body)
	if contentType != "" {
		request.Header.Set(echo.HeaderContentType, contentType)
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}

type fakeAdminAppService struct {
	value           appapp.App
	listCalls       int
	createCommand   appapp.CreateCommand
	updateCommand   appapp.UpdateCommand
	enabledCommand  appapp.SetEnabledCommand
	uploadCommand   appapp.UploadAvatarCommand
	uploadedContent []byte
	regenerateID    string
	deleteID        string
	getErr          error
	regenerateErr   error
	callOrder       []string
}

func (s *fakeAdminAppService) List(context.Context) ([]appapp.App, error) {
	s.listCalls++
	return []appapp.App{s.value}, nil
}

func (s *fakeAdminAppService) Get(_ context.Context, _ string) (appapp.App, error) {
	s.callOrder = append(s.callOrder, "get")
	return s.value, s.getErr
}

func (s *fakeAdminAppService) Create(_ context.Context, cmd appapp.CreateCommand) (appapp.App, error) {
	s.createCommand = cmd
	return s.value, nil
}

func (s *fakeAdminAppService) Update(_ context.Context, cmd appapp.UpdateCommand) (appapp.App, error) {
	s.callOrder = append(s.callOrder, "update")
	s.updateCommand = cmd
	s.value.Name = cmd.Name
	return s.value, nil
}

func (s *fakeAdminAppService) SetEnabled(_ context.Context, cmd appapp.SetEnabledCommand) (appapp.App, error) {
	s.enabledCommand = cmd
	s.value.Enabled = cmd.Enabled
	return s.value, nil
}

func (s *fakeAdminAppService) RegenerateSecret(_ context.Context, appID string) (appapp.App, error) {
	s.regenerateID = appID
	return s.value, s.regenerateErr
}

func (s *fakeAdminAppService) Delete(_ context.Context, appID string) error {
	s.deleteID = appID
	return nil
}

func (s *fakeAdminAppService) UploadAvatar(_ context.Context, cmd appapp.UploadAvatarCommand) (appapp.App, error) {
	s.uploadCommand = cmd
	s.uploadedContent, _ = io.ReadAll(cmd.Content)
	return s.value, nil
}
