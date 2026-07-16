package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"app/internal/application/identityprovider"

	"github.com/labstack/echo/v4"
)

func TestIdentityProviderAPIRoutesUseApplicationService(t *testing.T) {
	provider := identityprovider.Provider{
		ID: "10000000-0000-0000-0000-000000000001", Name: "Enterprise SSO", Key: "enterprise/sso",
		Type: identityprovider.TypeOIDC, Enabled: true, ClientID: "client-id", ClientSecret: "client-secret",
		Scopes: []string{"openid"}, Config: map[string]any{"authorize_url": "https://sso.test/authorize"}, SortOrder: 10,
	}
	service := &identityProviderServiceStub{provider: provider, providers: []identityprovider.Provider{provider}}
	router := echo.New()
	NewIdentityProviderAPI(service, "client.example.com").RegisterRoutes(router.Group("/api/admin"))

	response := performIdentityProviderRequest(router, http.MethodGet, "/api/admin/third-party/providers", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("list status = %d, body = %s", response.Code, response.Body.String())
	}
	var payload struct {
		Data struct {
			Providers []struct {
				CallbackURL string `json:"callback_url"`
			} `json:"providers"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil || len(payload.Data.Providers) != 1 ||
		payload.Data.Providers[0].CallbackURL != "https://client.example.com/api/client/auth/third-party/enterprise%2Fsso/callback" {
		t.Fatalf("list response = %s, err = %v", response.Body.String(), err)
	}

	response = performIdentityProviderRequest(router, http.MethodPost, "/api/admin/third-party/providers", bytes.NewBufferString(
		`{"name":"Enterprise SSO","type":"oidc","client_id":"id","client_secret":"secret","scopes":["openid"],"config":{"authorize_url":"https://sso.test/authorize"}}`,
	))
	if response.Code != http.StatusCreated || service.writeCommand.Name != "Enterprise SSO" || service.writeCommand.ClientID != "id" {
		t.Fatalf("create status = %d, command = %#v, body = %s", response.Code, service.writeCommand, response.Body.String())
	}

	response = performIdentityProviderRequest(router, http.MethodPost, "/api/admin/third-party/providers/"+provider.ID+"/disable", bytes.NewBufferString(`{}`))
	if response.Code != http.StatusOK || service.enabledCommand.Enabled || service.enabledCommand.ProviderID != provider.ID {
		t.Fatalf("disable status = %d, command = %#v", response.Code, service.enabledCommand)
	}
	response = performIdentityProviderRequest(router, http.MethodPost, "/api/admin/third-party/providers/"+provider.ID+"/move", bytes.NewBufferString(`{"direction":"up"}`))
	if response.Code != http.StatusOK || service.moveCommand.Direction != "up" {
		t.Fatalf("move status = %d, command = %#v", response.Code, service.moveCommand)
	}
	response = performIdentityProviderRequest(router, http.MethodDelete, "/api/admin/third-party/providers/"+provider.ID, nil)
	if response.Code != http.StatusOK || service.deletedID != provider.ID {
		t.Fatalf("delete status = %d, id = %q", response.Code, service.deletedID)
	}

	service.listErr = &identityprovider.Error{Code: identityprovider.CodeInternal, Message: "服务端错误"}
	response = performIdentityProviderRequest(router, http.MethodGet, "/api/admin/third-party/providers", nil)
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("list error status = %d, body = %s", response.Code, response.Body.String())
	}
}

func performIdentityProviderRequest(router *echo.Echo, method, path string, body *bytes.Buffer) *httptest.ResponseRecorder {
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

type identityProviderServiceStub struct {
	provider       identityprovider.Provider
	providers      []identityprovider.Provider
	writeCommand   identityprovider.WriteCommand
	enabledCommand identityprovider.SetEnabledCommand
	moveCommand    identityprovider.MoveCommand
	deletedID      string
	listErr        error
}

func (s *identityProviderServiceStub) List(context.Context) ([]identityprovider.Provider, error) {
	return s.providers, s.listErr
}

func (s *identityProviderServiceStub) Get(context.Context, string) (identityprovider.Provider, error) {
	return s.provider, nil
}

func (s *identityProviderServiceStub) Create(_ context.Context, cmd identityprovider.WriteCommand) (identityprovider.Provider, error) {
	s.writeCommand = cmd
	return s.provider, nil
}

func (s *identityProviderServiceStub) Update(_ context.Context, cmd identityprovider.UpdateCommand) (identityprovider.Provider, error) {
	s.writeCommand = cmd.WriteCommand
	return s.provider, nil
}

func (s *identityProviderServiceStub) SetEnabled(_ context.Context, cmd identityprovider.SetEnabledCommand) (identityprovider.Provider, error) {
	s.enabledCommand = cmd
	s.provider.Enabled = cmd.Enabled
	return s.provider, nil
}

func (s *identityProviderServiceStub) Move(_ context.Context, cmd identityprovider.MoveCommand) ([]identityprovider.Provider, error) {
	s.moveCommand = cmd
	return s.providers, nil
}

func (s *identityProviderServiceStub) Delete(_ context.Context, providerID string) error {
	s.deletedID = providerID
	return nil
}
