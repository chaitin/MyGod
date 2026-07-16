package admin

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"app/internal/application/adminauth"

	"github.com/labstack/echo/v4"
)

func TestAuthAPILoginAndSessionMiddlewareUseApplicationService(t *testing.T) {
	expiresAt := time.Now().UTC().Add(time.Hour)
	service := &fakeAdminAuthService{loginResult: adminauth.LoginResult{
		Admin:   adminauth.Admin{Email: "admin"},
		Session: adminauth.SessionCredential{Token: "admin-token", ExpiresAt: expiresAt},
	}}
	api := NewAuthAPI(service, service)
	router := echo.New()
	api.RegisterPublicRoutes(router)
	router.GET("/api/admin/protected", func(c echo.Context) error {
		return c.NoContent(http.StatusNoContent)
	}, api.RequireSession)

	request := httptest.NewRequest(http.MethodPost, "/api/admin/auth/login", bytes.NewBufferString(`{"email":"admin","password":"password"}`))
	request.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || service.loginCommand.Email != "admin" || service.loginCommand.Password != "password" {
		t.Fatalf("login status = %d, command = %#v, body = %s", recorder.Code, service.loginCommand, recorder.Body.String())
	}
	cookies := recorder.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != AdminSessionCookieName || cookies[0].Value != "admin-token" ||
		!cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteLaxMode {
		t.Fatalf("login cookies = %#v", cookies)
	}

	request = httptest.NewRequest(http.MethodGet, "/api/admin/protected", nil)
	request.AddCookie(cookies[0])
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNoContent || service.sessionToken != "admin-token" {
		t.Fatalf("protected status = %d, token = %q, body = %s", recorder.Code, service.sessionToken, recorder.Body.String())
	}

	request = httptest.NewRequest(http.MethodGet, "/api/admin/protected", nil)
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("missing session status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
}

type fakeAdminAuthService struct {
	loginResult  adminauth.LoginResult
	loginCommand adminauth.LoginCommand
	loginErr     error
	sessionToken string
	sessionErr   error
}

func (s *fakeAdminAuthService) Login(_ context.Context, cmd adminauth.LoginCommand) (adminauth.LoginResult, error) {
	s.loginCommand = cmd
	return s.loginResult, s.loginErr
}

func (s *fakeAdminAuthService) AuthenticateSession(_ context.Context, token string) (adminauth.AuthenticatedSession, error) {
	s.sessionToken = token
	return adminauth.AuthenticatedSession{ID: "session-id"}, s.sessionErr
}
