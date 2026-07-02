package httpserver

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"app/internal/auth"
	"app/internal/config"
	"app/internal/store"

	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func newTestRouter(t *testing.T) (*httptest.Server, *gorm.DB) {
	t.Helper()

	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite db: %v", err)
	}
	if err := store.AutoMigrate(db); err != nil {
		t.Fatalf("AutoMigrate() error = %v", err)
	}

	router := NewRouter(db, config.Config{
		Server:   config.ServerConfig{Addr: ":20080"},
		Database: config.DatabaseConfig{DSN: "sqlite-test"},
		Admin:    config.AdminConfig{Password: "admin-secret"},
	})

	return httptest.NewServer(router), db
}

func postJSON(t *testing.T, server *httptest.Server, path string, body map[string]any, cookies ...*http.Cookie) (*http.Response, map[string]any) {
	t.Helper()

	return requestJSON(t, server, http.MethodPost, path, body, cookies...)
}

func putJSON(t *testing.T, server *httptest.Server, path string, body map[string]any, cookies ...*http.Cookie) (*http.Response, map[string]any) {
	t.Helper()

	return requestJSON(t, server, http.MethodPut, path, body, cookies...)
}

func requestJSON(t *testing.T, server *httptest.Server, method string, path string, body map[string]any, cookies ...*http.Cookie) (*http.Response, map[string]any) {
	t.Helper()

	return requestJSONWithClient(t, server.Client(), server, method, path, body, cookies...)
}

func requestJSONWithClient(t *testing.T, client *http.Client, server *httptest.Server, method string, path string, body map[string]any, cookies ...*http.Cookie) (*http.Response, map[string]any) {
	t.Helper()

	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	req, err := http.NewRequest(method, server.URL+path, bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	t.Cleanup(func() {
		_ = resp.Body.Close()
	})

	var decoded map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	return resp, decoded
}

func getJSON(t *testing.T, server *httptest.Server, path string, cookies ...*http.Cookie) (*http.Response, map[string]any) {
	t.Helper()

	return getJSONWithClient(t, server.Client(), server, path, cookies...)
}

func getJSONWithClient(t *testing.T, client *http.Client, server *httptest.Server, path string, cookies ...*http.Cookie) (*http.Response, map[string]any) {
	t.Helper()

	req, err := http.NewRequest(http.MethodGet, server.URL+path, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	t.Cleanup(func() {
		_ = resp.Body.Close()
	})

	var decoded map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	return resp, decoded
}

func postJSONWithClient(t *testing.T, client *http.Client, server *httptest.Server, path string, body map[string]any, cookies ...*http.Cookie) (*http.Response, map[string]any) {
	t.Helper()

	return requestJSONWithClient(t, client, server, http.MethodPost, path, body, cookies...)
}

func loginAsAdmin(t *testing.T, server *httptest.Server) *http.Cookie {
	t.Helper()

	resp, body := postJSON(t, server, "/api/admin/auth/login", map[string]any{
		"email":    "admin",
		"password": "admin-secret",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("admin login status = %d, want 200", resp.StatusCode)
	}
	requireSuccess(t, body)

	return requireAdminSessionCookie(t, resp)
}

func loginAsUser(t *testing.T, server *httptest.Server, email string) *http.Cookie {
	t.Helper()

	resp, body := postJSON(t, server, "/api/client/auth/login", map[string]any{
		"email":    email,
		"password": "test-password",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("user login status = %d, want 200", resp.StatusCode)
	}
	requireSuccess(t, body)

	return requireUserSessionCookie(t, resp)
}

func insertTestUser(t *testing.T, db *gorm.DB, email string, name string, status string, createdAt time.Time) store.User {
	t.Helper()

	passwordHash, err := auth.HashPassword("test-password")
	if err != nil {
		t.Fatalf("hash test password: %v", err)
	}
	user := store.User{
		ID:           uuid.NewString(),
		Email:        email,
		Name:         name,
		PasswordHash: passwordHash,
		Status:       status,
		CreatedAt:    createdAt,
		UpdatedAt:    createdAt,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create test user: %v", err)
	}

	return user
}

func requireUsers(t *testing.T, data map[string]any) []any {
	t.Helper()

	users, ok := data["users"].([]any)
	if !ok {
		t.Fatalf("users = %#v, want array", data["users"])
	}

	return users
}

func requireSuccess(t *testing.T, response map[string]any) map[string]any {
	t.Helper()

	if response["success"] != true {
		t.Fatalf("success = %v, want true, response = %#v", response["success"], response)
	}
	data, ok := response["data"].(map[string]any)
	if !ok {
		t.Fatalf("data = %#v, want object", response["data"])
	}

	return data
}

func requireError(t *testing.T, response map[string]any, code string) {
	t.Helper()

	if response["success"] != false {
		t.Fatalf("success = %v, want false, response = %#v", response["success"], response)
	}
	errObject, ok := response["error"].(map[string]any)
	if !ok {
		t.Fatalf("error = %#v, want object", response["error"])
	}
	if errObject["code"] != code {
		t.Fatalf("error.code = %v, want %s", errObject["code"], code)
	}
}

func requireAdminSessionCookie(t *testing.T, resp *http.Response) *http.Cookie {
	t.Helper()

	return requireCookieNamed(t, resp, "admin_session")
}

func requireUserSessionCookie(t *testing.T, resp *http.Response) *http.Cookie {
	t.Helper()

	return requireCookieNamed(t, resp, "user_session")
}

func requireCookieNamed(t *testing.T, resp *http.Response, name string) *http.Cookie {
	t.Helper()

	for _, cookie := range resp.Cookies() {
		if cookie.Name == name {
			if cookie.Value == "" {
				t.Fatalf("%s cookie value is empty", name)
			}
			if !cookie.HttpOnly {
				t.Fatalf("%s cookie HttpOnly = false, want true", name)
			}
			if cookie.Secure {
				t.Fatalf("%s cookie Secure = true, want false", name)
			}
			return cookie
		}
	}

	t.Fatalf("response did not set %s cookie", name)
	return nil
}

func TestGeneratedSwaggerSpecIsServed(t *testing.T) {
	server, _ := newTestRouter(t)
	defer server.Close()

	resp, err := server.Client().Get(server.URL + "/api-docs/swagger.json")
	if err != nil {
		t.Fatalf("get swagger spec: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if contentType := resp.Header.Get("Content-Type"); contentType != "application/json" && contentType != "application/json; charset=utf-8" {
		t.Fatalf("Content-Type = %q, want application/json", contentType)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read swagger spec: %v", err)
	}

	var spec map[string]any
	if err := json.Unmarshal(body, &spec); err != nil {
		t.Fatalf("decode swagger spec: %v", err)
	}
	paths, ok := spec["paths"].(map[string]any)
	if !ok {
		t.Fatalf("paths = %#v, want object", spec["paths"])
	}
	for _, path := range []string{
		"/api/admin/auth/login",
		"/api/admin/users",
		"/api/admin/users/{id}/disable",
		"/api/admin/users/{id}/enable",
		"/api/admin/users/{id}/reset-password",
		"/api/admin/settings/info",
		"/api/client/auth/login",
		"/api/client/conversations/groups",
		"/api/client/info",
	} {
		if _, ok := paths[path]; !ok {
			t.Fatalf("swagger paths missing %s", path)
		}
	}
}

func TestClientInfoIsPublicAndReturnsDefaultSettings(t *testing.T) {
	server, _ := newTestRouter(t)
	defer server.Close()

	resp, body := getJSON(t, server, "/api/client/info")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	data := requireSuccess(t, body)
	if data["app_name"] != "MyGod" {
		t.Fatalf("app_name = %v, want MyGod", data["app_name"])
	}
	if data["organization_name"] != "长亭科技" {
		t.Fatalf("organization_name = %v, want 长亭科技", data["organization_name"])
	}
	if _, ok := data["version"]; ok {
		t.Fatalf("version = %v, want omitted", data["version"])
	}
}

func TestAdminCanReadAndUpdateInfoSettings(t *testing.T) {
	server, _ := newTestRouter(t)
	defer server.Close()

	adminCookie := loginAsAdmin(t, server)

	readResp, readBody := getJSON(t, server, "/api/admin/settings/info", adminCookie)
	if readResp.StatusCode != http.StatusOK {
		t.Fatalf("read status = %d, want 200", readResp.StatusCode)
	}
	readData := requireSuccess(t, readBody)
	if readData["app_name"] != "MyGod" {
		t.Fatalf("read app_name = %v, want MyGod", readData["app_name"])
	}

	updateResp, updateBody := putJSON(t, server, "/api/admin/settings/info", map[string]any{
		"app_name":          "星环协作",
		"organization_name": "长亭科技企业安全",
	}, adminCookie)
	if updateResp.StatusCode != http.StatusOK {
		t.Fatalf("update status = %d, want 200", updateResp.StatusCode)
	}
	updateData := requireSuccess(t, updateBody)
	if updateData["app_name"] != "星环协作" {
		t.Fatalf("updated app_name = %v, want 星环协作", updateData["app_name"])
	}
	if updateData["organization_name"] != "长亭科技企业安全" {
		t.Fatalf("updated organization_name = %v, want 长亭科技企业安全", updateData["organization_name"])
	}
	if _, ok := updateData["version"]; ok {
		t.Fatalf("updated version = %v, want omitted", updateData["version"])
	}

	clientResp, clientBody := getJSON(t, server, "/api/client/info")
	if clientResp.StatusCode != http.StatusOK {
		t.Fatalf("client status = %d, want 200", clientResp.StatusCode)
	}
	clientData := requireSuccess(t, clientBody)
	if clientData["app_name"] != "星环协作" {
		t.Fatalf("client app_name = %v, want 星环协作", clientData["app_name"])
	}
}

func TestUpdateInfoSettingsRequiresAdminSession(t *testing.T) {
	server, _ := newTestRouter(t)
	defer server.Close()

	resp, body := putJSON(t, server, "/api/admin/settings/info", map[string]any{
		"app_name":          "星环协作",
		"organization_name": "长亭科技",
	})
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
	requireError(t, body, "unauthorized")
}

func TestUpdateInfoSettingsRejectsEmptyNames(t *testing.T) {
	server, _ := newTestRouter(t)
	defer server.Close()

	adminCookie := loginAsAdmin(t, server)

	resp, body := putJSON(t, server, "/api/admin/settings/info", map[string]any{
		"app_name":          " ",
		"organization_name": "长亭科技",
	}, adminCookie)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
	requireError(t, body, "invalid_request")
}

func TestAdminLoginCreatesAdminSession(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	resp, body := postJSON(t, server, "/api/admin/auth/login", map[string]any{
		"email":    "admin",
		"password": "admin-secret",
	})

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	data := requireSuccess(t, body)
	admin := data["admin"].(map[string]any)
	if admin["email"] != "admin" {
		t.Fatalf("admin.email = %v, want admin", admin["email"])
	}
	requireAdminSessionCookie(t, resp)

	var count int64
	if err := db.Model(&store.AdminSession{}).Count(&count).Error; err != nil {
		t.Fatalf("count admin sessions: %v", err)
	}
	if count != 1 {
		t.Fatalf("admin session count = %d, want 1", count)
	}
}

func TestAdminAndUserSessionsUseSeparateCookies(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("new cookie jar: %v", err)
	}
	client := server.Client()
	client.Jar = jar

	adminResp, adminBody := postJSONWithClient(t, client, server, "/api/admin/auth/login", map[string]any{
		"email":    "admin",
		"password": "admin-secret",
	})
	if adminResp.StatusCode != http.StatusOK {
		t.Fatalf("admin login status = %d, want 200", adminResp.StatusCode)
	}
	requireSuccess(t, adminBody)
	requireAdminSessionCookie(t, adminResp)

	insertTestUser(t, db, "alice@example.com", "Alice Zhang", store.UserStatusActive, time.Now().UTC())
	userResp, userBody := postJSONWithClient(t, client, server, "/api/client/auth/login", map[string]any{
		"email":    "alice@example.com",
		"password": "test-password",
	})
	if userResp.StatusCode != http.StatusOK {
		t.Fatalf("user login status = %d, want 200", userResp.StatusCode)
	}
	requireSuccess(t, userBody)
	requireUserSessionCookie(t, userResp)

	adminListResp, adminListBody := getJSONWithClient(t, client, server, "/api/admin/users")
	if adminListResp.StatusCode != http.StatusOK {
		t.Fatalf("admin list status after user login = %d, want 200", adminListResp.StatusCode)
	}
	requireSuccess(t, adminListBody)
}

func TestCreateGroupConversationRequiresUserSession(t *testing.T) {
	server, _ := newTestRouter(t)
	defer server.Close()

	resp, body := postJSON(t, server, "/api/client/conversations/groups", map[string]any{
		"name":       "产品讨论组",
		"member_ids": []string{uuid.NewString()},
	})

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
	requireError(t, body, "unauthorized")
}

func TestCreateGroupConversationCreatesConversationAndMembers(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	now := time.Now().UTC()
	creator := insertTestUser(t, db, "creator@example.com", "Creator", store.UserStatusActive, now)
	alice := insertTestUser(t, db, "alice@example.com", "Alice", store.UserStatusActive, now)
	bob := insertTestUser(t, db, "bob@example.com", "Bob", store.UserStatusActive, now)
	userCookie := loginAsUser(t, server, creator.Email)

	resp, body := postJSON(t, server, "/api/client/conversations/groups", map[string]any{
		"name":       " 产品讨论组 ",
		"member_ids": []string{alice.ID, bob.ID, alice.ID},
	}, userCookie)

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201, body = %#v", resp.StatusCode, body)
	}
	data := requireSuccess(t, body)
	conversation := data["conversation"].(map[string]any)
	if conversation["type"] != "group" {
		t.Fatalf("conversation.type = %v, want group", conversation["type"])
	}
	if conversation["name"] != "产品讨论组" {
		t.Fatalf("conversation.name = %v, want trimmed name", conversation["name"])
	}
	if conversation["status"] != "active" {
		t.Fatalf("conversation.status = %v, want active", conversation["status"])
	}
	if conversation["posting_policy"] != "open" {
		t.Fatalf("conversation.posting_policy = %v, want open", conversation["posting_policy"])
	}
	if conversation["created_by_user_id"] != creator.ID {
		t.Fatalf("conversation.created_by_user_id = %v, want %s", conversation["created_by_user_id"], creator.ID)
	}
	if conversation["member_count"] != float64(3) {
		t.Fatalf("conversation.member_count = %v, want 3", conversation["member_count"])
	}

	members := conversation["members"].([]any)
	if len(members) != 3 {
		t.Fatalf("member count = %d, want 3", len(members))
	}
	rolesByID := map[string]string{}
	for _, rawMember := range members {
		member := rawMember.(map[string]any)
		rolesByID[member["id"].(string)] = member["role"].(string)
	}
	if rolesByID[creator.ID] != "owner" {
		t.Fatalf("creator role = %v, want owner", rolesByID[creator.ID])
	}
	if rolesByID[alice.ID] != "member" {
		t.Fatalf("alice role = %v, want member", rolesByID[alice.ID])
	}
	if rolesByID[bob.ID] != "member" {
		t.Fatalf("bob role = %v, want member", rolesByID[bob.ID])
	}

	var storedConversation store.Conversation
	if err := db.First(&storedConversation, "id = ?", conversation["id"]).Error; err != nil {
		t.Fatalf("find stored conversation: %v", err)
	}
	if storedConversation.Kind != store.ConversationKindGroup {
		t.Fatalf("stored conversation kind = %v, want group", storedConversation.Kind)
	}
	if storedConversation.CreatedByUserID != creator.ID {
		t.Fatalf("stored created_by_user_id = %v, want %s", storedConversation.CreatedByUserID, creator.ID)
	}
	if storedConversation.Status != store.ConversationStatusActive {
		t.Fatalf("stored status = %v, want active", storedConversation.Status)
	}
	if storedConversation.PostingPolicy != store.ConversationPostingPolicyOpen {
		t.Fatalf("stored posting_policy = %v, want open", storedConversation.PostingPolicy)
	}

	var storedMembers []store.ConversationMember
	if err := db.Where("conversation_id = ?", storedConversation.ID).Find(&storedMembers).Error; err != nil {
		t.Fatalf("find stored members: %v", err)
	}
	if len(storedMembers) != 3 {
		t.Fatalf("stored member count = %d, want 3", len(storedMembers))
	}
	storedRolesByID := map[string]string{}
	for _, member := range storedMembers {
		storedRolesByID[member.MemberID] = member.Role
	}
	if storedRolesByID[creator.ID] != store.ConversationMemberRoleOwner {
		t.Fatalf("stored creator role = %v, want owner", storedRolesByID[creator.ID])
	}
	if storedRolesByID[alice.ID] != store.ConversationMemberRoleMember {
		t.Fatalf("stored alice role = %v, want member", storedRolesByID[alice.ID])
	}
	if storedRolesByID[bob.ID] != store.ConversationMemberRoleMember {
		t.Fatalf("stored bob role = %v, want member", storedRolesByID[bob.ID])
	}
}

func TestCreateGroupConversationIgnoresCreatorIDCaseInsensitively(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	now := time.Now().UTC()
	creator := insertTestUser(t, db, "creator@example.com", "Creator", store.UserStatusActive, now)
	alice := insertTestUser(t, db, "alice@example.com", "Alice", store.UserStatusActive, now)
	userCookie := loginAsUser(t, server, creator.Email)

	resp, body := postJSON(t, server, "/api/client/conversations/groups", map[string]any{
		"name":       "产品讨论组",
		"member_ids": []string{strings.ToUpper(creator.ID), alice.ID},
	}, userCookie)

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201, body = %#v", resp.StatusCode, body)
	}
	conversation := requireSuccess(t, body)["conversation"].(map[string]any)
	if conversation["member_count"] != float64(2) {
		t.Fatalf("member_count = %v, want 2", conversation["member_count"])
	}

	var storedMembers []store.ConversationMember
	if err := db.Where("conversation_id = ?", conversation["id"]).Find(&storedMembers).Error; err != nil {
		t.Fatalf("find stored members: %v", err)
	}
	if len(storedMembers) != 2 {
		t.Fatalf("stored member count = %d, want 2", len(storedMembers))
	}
}

func TestCreateGroupConversationRejectsDisabledMembers(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	now := time.Now().UTC()
	creator := insertTestUser(t, db, "creator@example.com", "Creator", store.UserStatusActive, now)
	disabled := insertTestUser(t, db, "disabled@example.com", "Disabled", store.UserStatusDisabled, now)
	userCookie := loginAsUser(t, server, creator.Email)

	resp, body := postJSON(t, server, "/api/client/conversations/groups", map[string]any{
		"name":       "产品讨论组",
		"member_ids": []string{disabled.ID},
	}, userCookie)

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
	requireError(t, body, "invalid_request")
}

func TestCreateUserRequiresAdminSession(t *testing.T) {
	server, _ := newTestRouter(t)
	defer server.Close()

	resp, body := postJSON(t, server, "/api/admin/users", map[string]any{
		"email": "wenlei@example.com",
		"name":  "Wenlei Zhu",
	})

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
	requireError(t, body, "unauthorized")
}

func TestAdminCreatesUserAndUserCanLogin(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	adminResp, adminBody := postJSON(t, server, "/api/admin/auth/login", map[string]any{
		"email":    "admin",
		"password": "admin-secret",
	})
	requireSuccess(t, adminBody)
	adminCookie := requireAdminSessionCookie(t, adminResp)

	createResp, createBody := postJSON(t, server, "/api/admin/users", map[string]any{
		"email": "WENLEI@EXAMPLE.COM",
		"name":  "Wenlei Zhu",
	}, adminCookie)

	if createResp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201", createResp.StatusCode)
	}
	createData := requireSuccess(t, createBody)
	user := createData["user"].(map[string]any)
	if user["email"] != "wenlei@example.com" {
		t.Fatalf("user.email = %v, want normalized email", user["email"])
	}
	if user["name"] != "Wenlei Zhu" {
		t.Fatalf("user.name = %v, want Wenlei Zhu", user["name"])
	}
	if user["status"] != store.UserStatusActive {
		t.Fatalf("user.status = %v, want active", user["status"])
	}
	if createdAt, ok := user["created_at"].(string); !ok || createdAt == "" {
		t.Fatalf("user.created_at = %#v, want non-empty string", user["created_at"])
	}

	initialPassword, ok := createData["initial_password"].(string)
	if !ok {
		t.Fatalf("initial_password = %#v, want string", createData["initial_password"])
	}
	if len(initialPassword) != 16 {
		t.Fatalf("initial_password length = %d, want 16", len(initialPassword))
	}

	var storedUser store.User
	if err := db.Where("email = ?", "wenlei@example.com").First(&storedUser).Error; err != nil {
		t.Fatalf("find stored user: %v", err)
	}
	if storedUser.PasswordHash == initialPassword {
		t.Fatal("stored password hash equals plaintext initial password")
	}

	loginResp, loginBody := postJSON(t, server, "/api/client/auth/login", map[string]any{
		"email":    "wenlei@example.com",
		"password": initialPassword,
	})
	if loginResp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", loginResp.StatusCode)
	}
	loginData := requireSuccess(t, loginBody)
	loginUser := loginData["user"].(map[string]any)
	if loginUser["id"] != storedUser.ID {
		t.Fatalf("login user id = %v, want %s", loginUser["id"], storedUser.ID)
	}
	requireUserSessionCookie(t, loginResp)

	var userSessionCount int64
	if err := db.Model(&store.UserSession{}).Count(&userSessionCount).Error; err != nil {
		t.Fatalf("count user sessions: %v", err)
	}
	if userSessionCount != 1 {
		t.Fatalf("user session count = %d, want 1", userSessionCount)
	}
}

func TestDuplicateUserEmailReturnsConflict(t *testing.T) {
	server, _ := newTestRouter(t)
	defer server.Close()

	adminResp, adminBody := postJSON(t, server, "/api/admin/auth/login", map[string]any{
		"email":    "admin",
		"password": "admin-secret",
	})
	requireSuccess(t, adminBody)
	adminCookie := requireAdminSessionCookie(t, adminResp)

	firstResp, firstBody := postJSON(t, server, "/api/admin/users", map[string]any{
		"email": "wenlei@example.com",
		"name":  "Wenlei Zhu",
	}, adminCookie)
	if firstResp.StatusCode != http.StatusCreated {
		t.Fatalf("first status = %d, want 201", firstResp.StatusCode)
	}
	requireSuccess(t, firstBody)

	duplicateResp, duplicateBody := postJSON(t, server, "/api/admin/users", map[string]any{
		"email": "WENLEI@EXAMPLE.COM",
		"name":  "Duplicate",
	}, adminCookie)
	if duplicateResp.StatusCode != http.StatusConflict {
		t.Fatalf("duplicate status = %d, want 409", duplicateResp.StatusCode)
	}
	requireError(t, duplicateBody, "conflict")
}

func TestListUsersSupportsKeywordSearchAndSorting(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	adminCookie := loginAsAdmin(t, server)
	jan1 := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	jan2 := time.Date(2026, 1, 2, 10, 0, 0, 0, time.UTC)
	jan3 := time.Date(2026, 1, 3, 10, 0, 0, 0, time.UTC)
	insertTestUser(t, db, "alice@example.com", "Alice Zhang", store.UserStatusActive, jan2)
	insertTestUser(t, db, "bob@example.net", "Bob Li", store.UserStatusDisabled, jan1)
	insertTestUser(t, db, "carol@company.io", "Carol Wang", store.UserStatusActive, jan3)

	emailResp, emailBody := getJSON(t, server, "/api/admin/users?sort=email&order=asc", adminCookie)
	if emailResp.StatusCode != http.StatusOK {
		t.Fatalf("email sort status = %d, want 200", emailResp.StatusCode)
	}
	emailUsers := requireUsers(t, requireSuccess(t, emailBody))
	if got := emailUsers[0].(map[string]any)["email"]; got != "alice@example.com" {
		t.Fatalf("first user email = %v, want alice@example.com", got)
	}
	if got := emailUsers[2].(map[string]any)["email"]; got != "carol@company.io" {
		t.Fatalf("last user email = %v, want carol@company.io", got)
	}

	createdResp, createdBody := getJSON(t, server, "/api/admin/users?keyword="+url.QueryEscape("example")+"&sort=created_at&order=desc", adminCookie)
	if createdResp.StatusCode != http.StatusOK {
		t.Fatalf("created_at sort status = %d, want 200", createdResp.StatusCode)
	}
	createdData := requireSuccess(t, createdBody)
	createdUsers := requireUsers(t, createdData)
	if total := createdData["total"]; total != float64(2) {
		t.Fatalf("total = %v, want 2", total)
	}
	if got := createdUsers[0].(map[string]any)["email"]; got != "alice@example.com" {
		t.Fatalf("first keyword user email = %v, want alice@example.com", got)
	}
	if got := createdUsers[1].(map[string]any)["email"]; got != "bob@example.net" {
		t.Fatalf("second keyword user email = %v, want bob@example.net", got)
	}

	nameResp, nameBody := getJSON(t, server, "/api/admin/users?keyword="+url.QueryEscape("wang")+"&sort=status&order=asc", adminCookie)
	if nameResp.StatusCode != http.StatusOK {
		t.Fatalf("name search status = %d, want 200", nameResp.StatusCode)
	}
	nameUsers := requireUsers(t, requireSuccess(t, nameBody))
	if len(nameUsers) != 1 {
		t.Fatalf("name search user count = %d, want 1", len(nameUsers))
	}
	if got := nameUsers[0].(map[string]any)["name"]; got != "Carol Wang" {
		t.Fatalf("name search result = %v, want Carol Wang", got)
	}
}

func TestListUsersSupportsPagination(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	adminCookie := loginAsAdmin(t, server)
	now := time.Now().UTC()
	insertTestUser(t, db, "alice@example.com", "Alice Zhang", store.UserStatusActive, now)
	insertTestUser(t, db, "bob@example.com", "Bob Li", store.UserStatusActive, now)
	insertTestUser(t, db, "carol@example.com", "Carol Wang", store.UserStatusActive, now)

	resp, body := getJSON(t, server, "/api/admin/users?sort=email&order=asc&page=2&page_size=1", adminCookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	data := requireSuccess(t, body)
	users := requireUsers(t, data)
	if len(users) != 1 {
		t.Fatalf("user count = %d, want 1", len(users))
	}
	if got := users[0].(map[string]any)["email"]; got != "bob@example.com" {
		t.Fatalf("paged user email = %v, want bob@example.com", got)
	}
	if total := data["total"]; total != float64(3) {
		t.Fatalf("total = %v, want 3", total)
	}
	if page := data["page"]; page != float64(2) {
		t.Fatalf("page = %v, want 2", page)
	}
	if pageSize := data["page_size"]; pageSize != float64(1) {
		t.Fatalf("page_size = %v, want 1", pageSize)
	}
}

func TestListUsersRejectsUnsupportedSorting(t *testing.T) {
	server, _ := newTestRouter(t)
	defer server.Close()

	adminCookie := loginAsAdmin(t, server)

	resp, body := getJSON(t, server, "/api/admin/users?sort=password_hash&order=asc", adminCookie)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
	requireError(t, body, "invalid_request")
}

func TestListUsersRejectsUnsupportedPagination(t *testing.T) {
	server, _ := newTestRouter(t)
	defer server.Close()

	adminCookie := loginAsAdmin(t, server)

	resp, body := getJSON(t, server, "/api/admin/users?page=0&page_size=1001", adminCookie)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
	requireError(t, body, "invalid_request")
}

func TestAdminCanDisableAndEnableUser(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	adminCookie := loginAsAdmin(t, server)
	user := insertTestUser(t, db, "alice@example.com", "Alice Zhang", store.UserStatusActive, time.Now().UTC())
	userSession := store.UserSession{
		ID:         uuid.NewString(),
		TokenHash:  "test-user-session-token-hash",
		UserID:     user.ID,
		ExpiresAt:  time.Now().UTC().Add(time.Hour),
		LastSeenAt: time.Now().UTC(),
	}
	if err := db.Create(&userSession).Error; err != nil {
		t.Fatalf("create user session: %v", err)
	}

	disableResp, disableBody := postJSON(t, server, "/api/admin/users/"+user.ID+"/disable", map[string]any{}, adminCookie)
	if disableResp.StatusCode != http.StatusOK {
		t.Fatalf("disable status = %d, want 200", disableResp.StatusCode)
	}
	disabledUser := requireSuccess(t, disableBody)["user"].(map[string]any)
	if disabledUser["status"] != store.UserStatusDisabled {
		t.Fatalf("disabled user status = %v, want disabled", disabledUser["status"])
	}

	var storedDisabled store.User
	if err := db.First(&storedDisabled, "id = ?", user.ID).Error; err != nil {
		t.Fatalf("find disabled user: %v", err)
	}
	if storedDisabled.Status != store.UserStatusDisabled {
		t.Fatalf("stored disabled status = %v, want disabled", storedDisabled.Status)
	}
	var userSessionCount int64
	if err := db.Model(&store.UserSession{}).Where("user_id = ?", user.ID).Count(&userSessionCount).Error; err != nil {
		t.Fatalf("count user sessions: %v", err)
	}
	if userSessionCount != 0 {
		t.Fatalf("user session count after disable = %d, want 0", userSessionCount)
	}

	enableResp, enableBody := postJSON(t, server, "/api/admin/users/"+user.ID+"/enable", map[string]any{}, adminCookie)
	if enableResp.StatusCode != http.StatusOK {
		t.Fatalf("enable status = %d, want 200", enableResp.StatusCode)
	}
	enabledUser := requireSuccess(t, enableBody)["user"].(map[string]any)
	if enabledUser["status"] != store.UserStatusActive {
		t.Fatalf("enabled user status = %v, want active", enabledUser["status"])
	}
}

func TestAdminCanResetUserPassword(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	adminCookie := loginAsAdmin(t, server)
	user := insertTestUser(t, db, "alice@example.com", "Alice Zhang", store.UserStatusActive, time.Now().UTC())
	userSession := store.UserSession{
		ID:         uuid.NewString(),
		TokenHash:  "reset-user-session-token-hash",
		UserID:     user.ID,
		ExpiresAt:  time.Now().UTC().Add(time.Hour),
		LastSeenAt: time.Now().UTC(),
	}
	if err := db.Create(&userSession).Error; err != nil {
		t.Fatalf("create user session: %v", err)
	}

	resetResp, resetBody := postJSON(t, server, "/api/admin/users/"+user.ID+"/reset-password", map[string]any{}, adminCookie)
	if resetResp.StatusCode != http.StatusOK {
		t.Fatalf("reset status = %d, want 200", resetResp.StatusCode)
	}
	resetData := requireSuccess(t, resetBody)
	resetUser := resetData["user"].(map[string]any)
	if resetUser["id"] != user.ID {
		t.Fatalf("reset user id = %v, want %s", resetUser["id"], user.ID)
	}
	newPassword, ok := resetData["new_password"].(string)
	if !ok {
		t.Fatalf("new_password = %#v, want string", resetData["new_password"])
	}
	if len(newPassword) != 16 {
		t.Fatalf("new_password length = %d, want 16", len(newPassword))
	}

	var storedUser store.User
	if err := db.First(&storedUser, "id = ?", user.ID).Error; err != nil {
		t.Fatalf("find reset user: %v", err)
	}
	oldPasswordOK, err := auth.VerifyPassword("test-password", storedUser.PasswordHash)
	if err != nil {
		t.Fatalf("verify old password: %v", err)
	}
	if oldPasswordOK {
		t.Fatal("old password still works after reset")
	}
	newPasswordOK, err := auth.VerifyPassword(newPassword, storedUser.PasswordHash)
	if err != nil {
		t.Fatalf("verify new password: %v", err)
	}
	if !newPasswordOK {
		t.Fatal("new password does not match stored hash")
	}
	var userSessionCount int64
	if err := db.Model(&store.UserSession{}).Where("user_id = ?", user.ID).Count(&userSessionCount).Error; err != nil {
		t.Fatalf("count user sessions: %v", err)
	}
	if userSessionCount != 0 {
		t.Fatalf("user session count after password reset = %d, want 0", userSessionCount)
	}

	loginResp, loginBody := postJSON(t, server, "/api/client/auth/login", map[string]any{
		"email":    "alice@example.com",
		"password": newPassword,
	})
	if loginResp.StatusCode != http.StatusOK {
		t.Fatalf("login with reset password status = %d, want 200", loginResp.StatusCode)
	}
	requireSuccess(t, loginBody)
}
