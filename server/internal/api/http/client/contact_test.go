package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"app/internal/application/account"
	contactapp "app/internal/application/contact"

	"github.com/labstack/echo/v4"
)

func TestContactAPIListsUnifiedContacts(t *testing.T) {
	lastOnlineAt := time.Date(2026, 7, 15, 9, 0, 0, 0, time.UTC)
	stub := &contactServiceStub{
		listResult: contactapp.ListResult{
			Apps:   []contactapp.App{{ID: "app-id", Name: "App", Type: contactapp.ContactTypeApp}},
			Groups: []contactapp.Group{{ID: "group-id", Name: "Group", Type: contactapp.ContactTypeGroup}},
			Users:  []contactapp.User{{ID: "user-id", Name: "User", Type: contactapp.ContactTypeUser, LastOnlineAt: &lastOnlineAt}},
		},
	}
	api := NewContactAPI(stub)
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/contacts?keyword=Team", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set(currentAccountKey, account.Account{ID: "account-id"})

	if err := api.list(c); err != nil {
		t.Fatalf("list contacts: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if stub.listCommand.AccountID != "account-id" || stub.listCommand.Keyword != "Team" {
		t.Fatalf("command = %#v", stub.listCommand)
	}
	var response struct {
		Success bool                       `json:"success"`
		Data    listClientContactsResponse `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !response.Success || len(response.Data.Apps) != 1 || len(response.Data.Groups) != 1 || len(response.Data.Users) != 1 {
		t.Fatalf("response = %#v", response)
	}
	if response.Data.Users[0].LastOnlineAt == nil || *response.Data.Users[0].LastOnlineAt != lastOnlineAt.Format(time.RFC3339) {
		t.Fatalf("user = %#v", response.Data.Users[0])
	}
}

func TestContactAPIListsUsers(t *testing.T) {
	stub := &contactServiceStub{usersResult: contactapp.ListUsersResult{Users: []contactapp.User{{ID: "user-id", Type: contactapp.ContactTypeUser}}}}
	api := NewContactAPI(stub)
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/contacts/users?keyword=alice", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := api.listUsers(c); err != nil {
		t.Fatalf("list users: %v", err)
	}
	if rec.Code != http.StatusOK || stub.usersCommand.Keyword != "alice" {
		t.Fatalf("status = %d, command = %#v", rec.Code, stub.usersCommand)
	}
	var response struct {
		Success bool                     `json:"success"`
		Data    listContactUsersResponse `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !response.Success || len(response.Data.Contacts) != 1 || response.Data.Contacts[0].ID != "user-id" {
		t.Fatalf("response = %#v", response)
	}
}

type contactServiceStub struct {
	listCommand  contactapp.ListCommand
	listResult   contactapp.ListResult
	listErr      error
	usersCommand contactapp.ListUsersCommand
	usersResult  contactapp.ListUsersResult
	usersErr     error
}

func (s *contactServiceStub) List(_ context.Context, command contactapp.ListCommand) (contactapp.ListResult, error) {
	s.listCommand = command
	return s.listResult, s.listErr
}

func (s *contactServiceStub) ListUsers(_ context.Context, command contactapp.ListUsersCommand) (contactapp.ListUsersResult, error) {
	s.usersCommand = command
	return s.usersResult, s.usersErr
}
