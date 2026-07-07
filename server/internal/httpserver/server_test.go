package httpserver

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"app/internal/appregistry"
	"app/internal/auth"
	"app/internal/config"
	"app/internal/realtime"
	"app/internal/store"

	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"gorm.io/gorm"
)

func newTestRouter(t *testing.T) (*httptest.Server, *gorm.DB) {
	t.Helper()

	return newTestRouterWithRealtimeOptions(t, realtime.Options{})
}

func newTestRouterWithRealtimeOptions(t *testing.T, options realtime.Options) (*httptest.Server, *gorm.DB) {
	t.Helper()

	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite db: %v", err)
	}
	if err := migrateTestSchema(db); err != nil {
		t.Fatalf("migrate test schema: %v", err)
	}

	router := NewRouterWithRealtimeOptions(db, config.Config{
		Server: config.ServerConfig{
			Addr:           ":20080",
			ClientHostname: "client.example.test",
			AdminHostname:  "admin.example.test",
		},
		Database: config.DatabaseConfig{DSN: "sqlite-test"},
		Admin:    config.AdminConfig{Password: "admin-secret"},
		Apps:     config.AppsConfig{GoddessSecret: "test-goddess-secret"},
	}, options)

	return httptest.NewServer(router), db
}

func migrateTestSchema(db *gorm.DB) error {
	return db.AutoMigrate(
		&store.User{},
		&store.AdminSession{},
		&store.UserSession{},
		&store.Conversation{},
		&store.ConversationMember{},
		&store.Message{},
		&store.DirectConversation{},
		&store.TemporaryFile{},
		&store.App{},
		&store.AppConversation{},
		&store.AppSettings{},
		&store.ThirdPartyLoginProvider{},
		&store.ThirdPartyLoginState{},
		&store.ThirdPartyAccount{},
		&store.LLMModel{},
	)
}

func postJSON(t *testing.T, server *httptest.Server, path string, body map[string]any, cookies ...*http.Cookie) (*http.Response, map[string]any) {
	t.Helper()

	return requestJSON(t, server, http.MethodPost, path, body, cookies...)
}

func putJSON(t *testing.T, server *httptest.Server, path string, body map[string]any, cookies ...*http.Cookie) (*http.Response, map[string]any) {
	t.Helper()

	return requestJSON(t, server, http.MethodPut, path, body, cookies...)
}

func patchJSON(t *testing.T, server *httptest.Server, path string, body map[string]any, cookies ...*http.Cookie) (*http.Response, map[string]any) {
	t.Helper()

	return requestJSON(t, server, http.MethodPatch, path, body, cookies...)
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

func getResponseWithClient(t *testing.T, client *http.Client, server *httptest.Server, path string, cookies ...*http.Cookie) *http.Response {
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

	return resp
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
		Avatar:       store.DefaultUserAvatar,
		Email:        email,
		Name:         name,
		Nickname:     "",
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

func insertTestThirdPartyLoginProvider(t *testing.T, db *gorm.DB, input store.ThirdPartyLoginProvider) store.ThirdPartyLoginProvider {
	t.Helper()

	if input.ID == "" {
		input.ID = uuid.NewString()
	}
	if input.Type == "" {
		input.Type = store.ThirdPartyLoginProviderTypeOIDC
	}
	if len(input.Scopes) == 0 {
		input.Scopes = json.RawMessage(`["openid","email","profile"]`)
	}
	if len(input.Config) == 0 {
		input.Config = json.RawMessage(`{
			"authorize_url":"https://sso.example.com/authorize",
			"token_url":"https://sso.example.com/token",
			"userinfo_url":"https://sso.example.com/userinfo",
			"external_id_field":"sub",
			"email_field":"email",
			"name_field":"name"
		}`)
	}
	if err := db.Create(&input).Error; err != nil {
		t.Fatalf("create test third-party provider: %v", err)
	}

	return input
}

func thirdPartyProviderConfig(t *testing.T, value map[string]any) json.RawMessage {
	t.Helper()

	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal third-party provider config: %v", err)
	}

	return raw
}

type testConversationInput struct {
	createdByUserID    string
	kind               string
	lastMessageAt      *time.Time
	lastMessageSeq     int64
	lastMessageSummary string
	memberIDs          []string
	memberLeftAtByID   map[string]*time.Time
	name               string
	now                time.Time
}

func insertTestConversation(t *testing.T, db *gorm.DB, input testConversationInput) store.Conversation {
	t.Helper()

	conversation := store.Conversation{
		ID:                 uuid.NewString(),
		Kind:               input.kind,
		Name:               input.name,
		CreatedByUserID:    input.createdByUserID,
		Status:             store.ConversationStatusActive,
		PostingPolicy:      store.ConversationPostingPolicyOpen,
		CreatedAt:          input.now,
		UpdatedAt:          input.now,
		LastMessageSeq:     input.lastMessageSeq,
		LastMessageSummary: input.lastMessageSummary,
		LastMessageAt:      input.lastMessageAt,
	}
	if err := db.Create(&conversation).Error; err != nil {
		t.Fatalf("create test conversation: %v", err)
	}

	members := make([]store.ConversationMember, 0, len(input.memberIDs))
	for _, memberID := range input.memberIDs {
		role := store.ConversationMemberRoleMember
		if memberID == input.createdByUserID {
			role = store.ConversationMemberRoleOwner
		}
		members = append(members, store.ConversationMember{
			ConversationID:        conversation.ID,
			MemberType:            store.ConversationMemberTypeUser,
			MemberID:              memberID,
			Role:                  role,
			JoinedAt:              input.now,
			HistoryVisibleFromSeq: 1,
			LeftAt:                input.memberLeftAtByID[memberID],
		})
	}
	if err := db.Create(&members).Error; err != nil {
		t.Fatalf("create test conversation members: %v", err)
	}

	if input.kind == store.ConversationKindDirect && len(input.memberIDs) == 2 {
		userLowID, userHighID := orderTestUserIDs(input.memberIDs[0], input.memberIDs[1])
		direct := store.DirectConversation{
			ConversationID: conversation.ID,
			UserLowID:      userLowID,
			UserHighID:     userHighID,
			CreatedAt:      input.now,
		}
		if err := db.Create(&direct).Error; err != nil {
			t.Fatalf("create test direct conversation: %v", err)
		}
	}

	return conversation
}

func setTestConversationMemberLastReadSeq(t *testing.T, db *gorm.DB, conversationID string, memberID string, lastReadSeq int64) {
	t.Helper()

	if err := db.Model(&store.ConversationMember{}).
		Where("conversation_id = ? AND member_type = ? AND member_id = ?", conversationID, store.ConversationMemberTypeUser, memberID).
		Update("last_read_seq", lastReadSeq).Error; err != nil {
		t.Fatalf("set conversation member last_read_seq: %v", err)
	}
}

func getTestConversationMemberLastReadSeq(t *testing.T, db *gorm.DB, conversationID string, memberID string) int64 {
	t.Helper()

	var member store.ConversationMember
	if err := db.First(
		&member,
		"conversation_id = ? AND member_type = ? AND member_id = ?",
		conversationID,
		store.ConversationMemberTypeUser,
		memberID,
	).Error; err != nil {
		t.Fatalf("find conversation member: %v", err)
	}

	return member.LastReadSeq
}

func insertTestMessage(t *testing.T, db *gorm.DB, conversationID string, senderID string, seq int64, content string, createdAt time.Time) store.Message {
	t.Helper()

	clientMessageID := fmt.Sprintf("client-message-%03d", seq)
	body := json.RawMessage(fmt.Sprintf(`{"type":"text","content":%q}`, content))
	message := store.Message{
		ID:              uuid.NewString(),
		ConversationID:  conversationID,
		Seq:             seq,
		SenderType:      store.MessageSenderTypeUser,
		SenderID:        &senderID,
		ClientMessageID: &clientMessageID,
		Body:            body,
		Summary:         content,
		CreatedAt:       createdAt,
		UpdatedAt:       createdAt,
	}
	if err := db.Create(&message).Error; err != nil {
		t.Fatalf("create test message: %v", err)
	}

	return message
}

func orderTestUserIDs(first string, second string) (string, string) {
	if first < second {
		return first, second
	}

	return second, first
}

func requireUsers(t *testing.T, data map[string]any) []any {
	t.Helper()

	users, ok := data["users"].([]any)
	if !ok {
		t.Fatalf("users = %#v, want array", data["users"])
	}

	return users
}

func requireContacts(t *testing.T, data map[string]any) []any {
	t.Helper()

	contacts, ok := data["contacts"].([]any)
	if !ok {
		t.Fatalf("contacts = %#v, want array", data["contacts"])
	}

	return contacts
}

func requireThirdPartyLoginProviders(t *testing.T, data map[string]any) []any {
	t.Helper()

	providers, ok := data["providers"].([]any)
	if !ok {
		t.Fatalf("providers = %#v, want array", data["providers"])
	}

	return providers
}

func requireStringSliceField(t *testing.T, object map[string]any, field string, expected []string) {
	t.Helper()

	rawValues, ok := object[field].([]any)
	if !ok {
		t.Fatalf("%s = %#v, want array", field, object[field])
	}
	values := make([]string, 0, len(rawValues))
	for _, rawValue := range rawValues {
		value, ok := rawValue.(string)
		if !ok {
			t.Fatalf("%s value = %#v, want string", field, rawValue)
		}
		values = append(values, value)
	}
	if !slices.Equal(values, expected) {
		t.Fatalf("%s = %#v, want %#v", field, values, expected)
	}
}

func requireConversations(t *testing.T, data map[string]any) []any {
	t.Helper()

	conversations, ok := data["conversations"].([]any)
	if !ok {
		t.Fatalf("conversations = %#v, want array", data["conversations"])
	}

	return conversations
}

func requireMessages(t *testing.T, data map[string]any) []any {
	t.Helper()

	messages, ok := data["messages"].([]any)
	if !ok {
		t.Fatalf("messages = %#v, want array", data["messages"])
	}

	return messages
}

func requireGroupMembersInvitedBody(t *testing.T, rawBody json.RawMessage, inviterID string, inviterDisplayName string, inviteeIDs []string, inviteeDisplayNames []string) {
	t.Helper()

	var body map[string]any
	if err := json.Unmarshal(rawBody, &body); err != nil {
		t.Fatalf("unmarshal system body: %v", err)
	}
	if body["type"] != "system_event" {
		t.Fatalf("body.type = %v, want system_event", body["type"])
	}
	if body["event"] != "group_members_invited" {
		t.Fatalf("body.event = %v, want group_members_invited", body["event"])
	}
	inviter, ok := body["inviter"].(map[string]any)
	if !ok {
		t.Fatalf("body.inviter = %#v, want object", body["inviter"])
	}
	if inviter["id"] != inviterID {
		t.Fatalf("inviter.id = %v, want %s", inviter["id"], inviterID)
	}
	if inviter["display_name"] != inviterDisplayName {
		t.Fatalf("inviter.display_name = %v, want %s", inviter["display_name"], inviterDisplayName)
	}
	invitees, ok := body["invitees"].([]any)
	if !ok {
		t.Fatalf("body.invitees = %#v, want array", body["invitees"])
	}
	if len(invitees) != len(inviteeIDs) {
		t.Fatalf("invitee count = %d, want %d", len(invitees), len(inviteeIDs))
	}
	for i, rawInvitee := range invitees {
		invitee, ok := rawInvitee.(map[string]any)
		if !ok {
			t.Fatalf("invitees[%d] = %#v, want object", i, rawInvitee)
		}
		if invitee["id"] != inviteeIDs[i] {
			t.Fatalf("invitees[%d].id = %v, want %s", i, invitee["id"], inviteeIDs[i])
		}
		if invitee["display_name"] != inviteeDisplayNames[i] {
			t.Fatalf("invitees[%d].display_name = %v, want %s", i, invitee["display_name"], inviteeDisplayNames[i])
		}
	}
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

func requireThirdPartyStateCookie(t *testing.T, resp *http.Response) *http.Cookie {
	t.Helper()

	cookie := requireCookieNamed(t, resp, "third_party_login_state")
	if cookie.SameSite != http.SameSiteLaxMode {
		t.Fatalf("third_party_login_state SameSite = %v, want Lax", cookie.SameSite)
	}

	return cookie
}

func dialClientWebSocket(t *testing.T, server *httptest.Server, cookie *http.Cookie) *websocket.Conn {
	t.Helper()

	header := http.Header{}
	if cookie != nil {
		header.Add("Cookie", cookie.String())
	}
	conn, resp, err := websocket.DefaultDialer.Dial(clientWebSocketURL(server), header)
	if err != nil {
		status := 0
		if resp != nil {
			status = resp.StatusCode
		}
		t.Fatalf("dial client websocket: %v, status = %d", err, status)
	}
	t.Cleanup(func() {
		_ = conn.Close()
	})

	return conn
}

func clientWebSocketURL(server *httptest.Server) string {
	return "ws" + strings.TrimPrefix(server.URL, "http") + "/api/client/ws"
}

func readRealtimeEvent(t *testing.T, conn *websocket.Conn) realtime.Envelope {
	t.Helper()

	_ = conn.SetReadDeadline(time.Now().Add(time.Second))
	var envelope realtime.Envelope
	if err := conn.ReadJSON(&envelope); err != nil {
		t.Fatalf("ReadJSON() error = %v", err)
	}

	return envelope
}

func readMessageCreatedEvent(t *testing.T, conn *websocket.Conn) map[string]any {
	t.Helper()

	envelope := readRealtimeEvent(t, conn)
	if envelope.Kind != realtime.KindEvent || envelope.Event != "message.created" {
		t.Fatalf("envelope = %#v, want message.created event", envelope)
	}
	var payload map[string]any
	if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
		t.Fatalf("unmarshal message.created payload: %v", err)
	}
	message, ok := payload["message"].(map[string]any)
	if !ok {
		t.Fatalf("payload.message = %#v, want object", payload["message"])
	}

	return message
}

func requireCookieNamed(t *testing.T, resp *http.Response, name string) *http.Cookie {
	t.Helper()

	cookie := findCookieNamed(t, resp, name)
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

func findCookieNamed(t *testing.T, resp *http.Response, name string) *http.Cookie {
	t.Helper()

	for _, cookie := range resp.Cookies() {
		if cookie.Name == name {
			return cookie
		}
	}

	t.Fatalf("response did not set %s cookie", name)
	return nil
}

func TestClientWebSocketRequiresUserSession(t *testing.T) {
	server, _ := newTestRouter(t)
	defer server.Close()

	conn, resp, err := websocket.DefaultDialer.Dial(clientWebSocketURL(server), nil)
	if err == nil {
		_ = conn.Close()
		t.Fatal("Dial() error = nil, want unauthorized error")
	}
	if resp == nil {
		t.Fatal("Dial() response = nil, want 401 response")
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("Dial() status = %d, want 401", resp.StatusCode)
	}
}

func TestClientWebSocketSendsSystemReadyAfterLogin(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()
	alice := insertTestUser(t, db, "alice@example.com", "Alice", store.UserStatusActive, time.Now().UTC())
	cookie := loginAsUser(t, server, alice.Email)
	conn := dialClientWebSocket(t, server, cookie)
	_ = conn.SetReadDeadline(time.Now().Add(time.Second))

	var envelope realtime.Envelope
	if err := conn.ReadJSON(&envelope); err != nil {
		t.Fatalf("ReadJSON() error = %v", err)
	}
	if envelope.Kind != realtime.KindEvent || envelope.Event != realtime.EventSystemReady {
		t.Fatalf("envelope = %#v, want system.ready event", envelope)
	}
}

func TestClientContactsIncludeRealtimePresence(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()
	alice := insertTestUser(t, db, "alice@example.com", "Alice", store.UserStatusActive, time.Now().UTC())
	bob := insertTestUser(t, db, "bob@example.com", "Bob", store.UserStatusActive, time.Now().UTC())
	lastOnlineAt := time.Date(2026, 7, 3, 1, 45, 0, 0, time.UTC)
	if err := db.Model(&store.User{}).Where("id = ?", bob.ID).Update("last_online_at", lastOnlineAt).Error; err != nil {
		t.Fatalf("set last_online_at: %v", err)
	}

	bobCookie := loginAsUser(t, server, bob.Email)
	_ = dialClientWebSocket(t, server, bobCookie)
	aliceCookie := loginAsUser(t, server, alice.Email)

	resp, body := getJSON(t, server, "/api/client/contacts/users", aliceCookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	data := requireSuccess(t, body)
	contacts := requireContacts(t, data)
	bobContactIndex := slices.IndexFunc(contacts, func(contact any) bool {
		contactMap, ok := contact.(map[string]any)
		return ok && contactMap["id"] == bob.ID
	})
	if bobContactIndex < 0 {
		t.Fatalf("contacts = %#v, want bob contact", contacts)
	}
	bobContact := contacts[bobContactIndex].(map[string]any)
	if bobContact["online"] != true {
		t.Fatalf("bob online = %#v, want true", bobContact["online"])
	}
	if bobContact["last_online_at"] != lastOnlineAt.Format(time.RFC3339) {
		t.Fatalf("bob last_online_at = %#v, want %s", bobContact["last_online_at"], lastOnlineAt.Format(time.RFC3339))
	}
}

func TestAdminListUsersIncludesRealtimePresence(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()
	adminCookie := loginAsAdmin(t, server)
	alice := insertTestUser(t, db, "alice@example.com", "Alice", store.UserStatusActive, time.Now().UTC())
	bob := insertTestUser(t, db, "bob@example.com", "Bob", store.UserStatusActive, time.Now().UTC())
	lastOnlineAt := time.Date(2026, 7, 3, 1, 45, 0, 0, time.UTC)
	if err := db.Model(&store.User{}).Where("id = ?", bob.ID).Update("last_online_at", lastOnlineAt).Error; err != nil {
		t.Fatalf("set last_online_at: %v", err)
	}

	bobCookie := loginAsUser(t, server, bob.Email)
	bobConn := dialClientWebSocket(t, server, bobCookie)
	defer bobConn.Close()

	resp, body := getJSON(t, server, "/api/admin/users?sort=email&order=asc", adminCookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	users := requireUsers(t, requireSuccess(t, body))
	aliceUser := users[0].(map[string]any)
	bobUser := users[1].(map[string]any)
	if aliceUser["id"] != alice.ID {
		t.Fatalf("first user id = %v, want %s", aliceUser["id"], alice.ID)
	}
	if aliceUser["online"] != false {
		t.Fatalf("alice online = %#v, want false", aliceUser["online"])
	}
	if bobUser["id"] != bob.ID {
		t.Fatalf("second user id = %v, want %s", bobUser["id"], bob.ID)
	}
	if bobUser["online"] != true {
		t.Fatalf("bob online = %#v, want true", bobUser["online"])
	}
	if bobUser["last_online_at"] != lastOnlineAt.Format(time.RFC3339) {
		t.Fatalf("bob last_online_at = %#v, want %s", bobUser["last_online_at"], lastOnlineAt.Format(time.RFC3339))
	}
}

func TestListClientConversationsReturnsRecentCurrentUserConversations(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()
	now := time.Date(2026, 7, 3, 9, 0, 0, 0, time.UTC)
	alice := insertTestUser(t, db, "alice@example.com", "Alice", store.UserStatusActive, now)
	bob := insertTestUser(t, db, "bob@example.com", "Bob", store.UserStatusActive, now)
	carol := insertTestUser(t, db, "carol@example.com", "Carol", store.UserStatusActive, now)
	dave := insertTestUser(t, db, "dave@example.com", "Dave", store.UserStatusActive, now)

	oldGroupLastAt := now.Add(-2 * time.Hour)
	newDirectLastAt := now.Add(-1 * time.Hour)
	otherDirectLastAt := now
	leftGroupLastAt := now.Add(time.Hour)
	leftAt := now.Add(-30 * time.Minute)
	oldGroup := insertTestConversation(t, db, testConversationInput{
		createdByUserID:    alice.ID,
		kind:               store.ConversationKindGroup,
		lastMessageAt:      &oldGroupLastAt,
		lastMessageSeq:     3,
		lastMessageSummary: "older group",
		memberIDs:          []string{alice.ID, carol.ID},
		name:               "Launch",
		now:                now.Add(-3 * time.Hour),
	})
	newDirect := insertTestConversation(t, db, testConversationInput{
		createdByUserID:    alice.ID,
		kind:               store.ConversationKindDirect,
		lastMessageAt:      &newDirectLastAt,
		lastMessageSeq:     5,
		lastMessageSummary: "newer direct",
		memberIDs:          []string{alice.ID, bob.ID},
		now:                now.Add(-4 * time.Hour),
	})
	setTestConversationMemberLastReadSeq(t, db, newDirect.ID, alice.ID, 2)
	insertTestConversation(t, db, testConversationInput{
		createdByUserID:    bob.ID,
		kind:               store.ConversationKindDirect,
		lastMessageAt:      &otherDirectLastAt,
		lastMessageSeq:     7,
		lastMessageSummary: "not alice",
		memberIDs:          []string{bob.ID, dave.ID},
		now:                now.Add(-5 * time.Hour),
	})
	insertTestConversation(t, db, testConversationInput{
		createdByUserID:    alice.ID,
		kind:               store.ConversationKindGroup,
		lastMessageAt:      &leftGroupLastAt,
		lastMessageSeq:     9,
		lastMessageSummary: "left group",
		memberIDs:          []string{alice.ID, dave.ID},
		memberLeftAtByID:   map[string]*time.Time{alice.ID: &leftAt},
		name:               "Left Group",
		now:                now.Add(-6 * time.Hour),
	})

	resp, body := getJSON(t, server, "/api/client/conversations", loginAsUser(t, server, alice.Email))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	conversations := requireConversations(t, requireSuccess(t, body))
	if len(conversations) != 3 {
		t.Fatalf("conversation count = %d, want 3: %#v", len(conversations), conversations)
	}

	first := conversations[0].(map[string]any)
	if first["type"] != store.ConversationKindApp {
		t.Fatalf("first type = %v, want app", first["type"])
	}
	if first["name"] != builtinAssistantConversationName {
		t.Fatalf("assistant name = %v, want %s", first["name"], builtinAssistantConversationName)
	}
	if first["avatar"] != builtinAssistantAvatar {
		t.Fatalf("assistant avatar = %v, want %s", first["avatar"], builtinAssistantAvatar)
	}
	if first["member_count"] != float64(1) {
		t.Fatalf("assistant member_count = %v, want 1", first["member_count"])
	}

	second := conversations[1].(map[string]any)
	if second["id"] != newDirect.ID {
		t.Fatalf("second id = %v, want new direct %s", second["id"], newDirect.ID)
	}
	if second["type"] != store.ConversationKindDirect {
		t.Fatalf("second type = %v, want direct", second["type"])
	}
	if second["name"] != bob.Name {
		t.Fatalf("direct name = %v, want %s", second["name"], bob.Name)
	}
	if second["avatar"] != bob.Avatar {
		t.Fatalf("direct avatar = %v, want %s", second["avatar"], bob.Avatar)
	}
	if second["member_count"] != float64(2) {
		t.Fatalf("direct member_count = %v, want 2", second["member_count"])
	}
	if second["last_message_summary"] != "newer direct" {
		t.Fatalf("direct last_message_summary = %v, want newer direct", second["last_message_summary"])
	}
	if second["last_message_seq"] != float64(5) {
		t.Fatalf("direct last_message_seq = %v, want 5", second["last_message_seq"])
	}
	if second["last_read_seq"] != float64(2) {
		t.Fatalf("direct last_read_seq = %v, want 2", second["last_read_seq"])
	}
	if second["unread_count"] != float64(3) {
		t.Fatalf("direct unread_count = %v, want 3", second["unread_count"])
	}
	if second["last_message_at"] != newDirectLastAt.Format(time.RFC3339) {
		t.Fatalf("direct last_message_at = %v, want %s", second["last_message_at"], newDirectLastAt.Format(time.RFC3339))
	}

	third := conversations[2].(map[string]any)
	if third["id"] != oldGroup.ID {
		t.Fatalf("third id = %v, want old group %s", third["id"], oldGroup.ID)
	}
	if third["type"] != store.ConversationKindGroup {
		t.Fatalf("third type = %v, want group", third["type"])
	}
	if third["name"] != oldGroup.Name {
		t.Fatalf("group name = %v, want %s", third["name"], oldGroup.Name)
	}
	if third["member_count"] != float64(2) {
		t.Fatalf("group member_count = %v, want 2", third["member_count"])
	}
}

func TestListClientConversationsCreatesBuiltinAssistantConversationOnce(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()
	now := time.Date(2026, 7, 3, 9, 0, 0, 0, time.UTC)
	alice := insertTestUser(t, db, "alice@example.com", "Alice", store.UserStatusActive, now)
	userCookie := loginAsUser(t, server, alice.Email)

	for i := 0; i < 2; i++ {
		resp, body := getJSON(t, server, "/api/client/conversations", userCookie)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200", resp.StatusCode)
		}
		conversations := requireConversations(t, requireSuccess(t, body))
		if len(conversations) != 1 {
			t.Fatalf("conversation count = %d, want 1: %#v", len(conversations), conversations)
		}
		assistant := conversations[0].(map[string]any)
		if assistant["id"] != builtinAssistantConversationID(alice.ID) {
			t.Fatalf("assistant id = %v, want deterministic id", assistant["id"])
		}
		if assistant["type"] != store.ConversationKindApp {
			t.Fatalf("assistant type = %v, want app", assistant["type"])
		}
	}

	var conversationCount int64
	if err := db.Model(&store.Conversation{}).
		Where("kind = ? AND created_by_user_id = ?", store.ConversationKindApp, alice.ID).
		Count(&conversationCount).Error; err != nil {
		t.Fatalf("count assistant conversations: %v", err)
	}
	if conversationCount != 1 {
		t.Fatalf("assistant conversation count = %d, want 1", conversationCount)
	}

	var memberCount int64
	if err := db.Model(&store.ConversationMember{}).
		Where("conversation_id = ?", builtinAssistantConversationID(alice.ID)).
		Count(&memberCount).Error; err != nil {
		t.Fatalf("count assistant members: %v", err)
	}
	if memberCount != 2 {
		t.Fatalf("assistant member count = %d, want user and app members", memberCount)
	}

	var appMember store.ConversationMember
	if err := db.First(
		&appMember,
		"conversation_id = ? AND member_type = ?",
		builtinAssistantConversationID(alice.ID),
		store.ConversationMemberTypeApp,
	).Error; err != nil {
		t.Fatalf("find assistant app member: %v", err)
	}
	if appMember.MemberID != appregistry.GoddessAppID {
		t.Fatalf("assistant app member id = %s, want goddess app id", appMember.MemberID)
	}

	var appConversation store.AppConversation
	if err := db.First(&appConversation, "app_id = ? AND user_id = ?", appregistry.GoddessAppID, alice.ID).Error; err != nil {
		t.Fatalf("find assistant app conversation: %v", err)
	}
	if appConversation.ConversationID != builtinAssistantConversationID(alice.ID) {
		t.Fatalf("app conversation id = %s, want assistant conversation", appConversation.ConversationID)
	}
}

func TestListClientConversationsLimitsToRecent100(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()
	now := time.Date(2026, 7, 3, 9, 0, 0, 0, time.UTC)
	alice := insertTestUser(t, db, "alice@example.com", "Alice", store.UserStatusActive, now)
	bob := insertTestUser(t, db, "bob@example.com", "Bob", store.UserStatusActive, now)

	for i := 0; i < 101; i++ {
		lastMessageAt := now.Add(-time.Duration(i) * time.Minute)
		insertTestConversation(t, db, testConversationInput{
			createdByUserID:    alice.ID,
			kind:               store.ConversationKindGroup,
			lastMessageAt:      &lastMessageAt,
			lastMessageSeq:     int64(i + 1),
			lastMessageSummary: fmt.Sprintf("summary %03d", i),
			memberIDs:          []string{alice.ID, bob.ID},
			name:               fmt.Sprintf("Group %03d", i),
			now:                now.Add(-time.Hour),
		})
	}

	resp, body := getJSON(t, server, "/api/client/conversations", loginAsUser(t, server, alice.Email))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	conversations := requireConversations(t, requireSuccess(t, body))
	if len(conversations) != 100 {
		t.Fatalf("conversation count = %d, want 100", len(conversations))
	}
	first := conversations[0].(map[string]any)
	second := conversations[1].(map[string]any)
	last := conversations[99].(map[string]any)
	if first["type"] != store.ConversationKindApp {
		t.Fatalf("first type = %v, want app", first["type"])
	}
	if second["name"] != "Group 000" {
		t.Fatalf("second name = %v, want Group 000", second["name"])
	}
	if last["name"] != "Group 098" {
		t.Fatalf("last name = %v, want Group 098", last["name"])
	}
}

func TestCreateDirectConversationRequiresUserSession(t *testing.T) {
	server, _ := newTestRouter(t)
	defer server.Close()

	resp, body := postJSON(t, server, "/api/client/conversations/direct", map[string]any{
		"user_id": uuid.NewString(),
	})

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
	requireError(t, body, "unauthorized")
}

func TestCreateDirectConversationCreatesConversationAndReturnsExisting(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()
	now := time.Date(2026, 7, 3, 9, 0, 0, 0, time.UTC)
	alice := insertTestUser(t, db, "alice@example.com", "Alice", store.UserStatusActive, now)
	bob := insertTestUser(t, db, "bob@example.com", "Bob", store.UserStatusActive, now)
	userCookie := loginAsUser(t, server, alice.Email)

	createResp, createBody := postJSON(t, server, "/api/client/conversations/direct", map[string]any{
		"user_id": bob.ID,
	}, userCookie)
	if createResp.StatusCode != http.StatusCreated {
		t.Fatalf("create status = %d, want 201, body = %#v", createResp.StatusCode, createBody)
	}
	createData := requireSuccess(t, createBody)
	if createData["created"] != true {
		t.Fatalf("created = %v, want true", createData["created"])
	}
	conversation := createData["conversation"].(map[string]any)
	if conversation["type"] != store.ConversationKindDirect {
		t.Fatalf("conversation.type = %v, want direct", conversation["type"])
	}
	if conversation["name"] != bob.Name {
		t.Fatalf("conversation.name = %v, want %s", conversation["name"], bob.Name)
	}
	if conversation["avatar"] != bob.Avatar {
		t.Fatalf("conversation.avatar = %v, want %s", conversation["avatar"], bob.Avatar)
	}
	if conversation["member_count"] != float64(2) {
		t.Fatalf("conversation.member_count = %v, want 2", conversation["member_count"])
	}
	if conversation["last_message_summary"] != "" {
		t.Fatalf("conversation.last_message_summary = %v, want empty", conversation["last_message_summary"])
	}

	conversationID := conversation["id"].(string)
	var storedConversation store.Conversation
	if err := db.First(&storedConversation, "id = ?", conversationID).Error; err != nil {
		t.Fatalf("find stored conversation: %v", err)
	}
	if storedConversation.Kind != store.ConversationKindDirect {
		t.Fatalf("stored kind = %v, want direct", storedConversation.Kind)
	}
	if storedConversation.CreatedByUserID != alice.ID {
		t.Fatalf("stored created_by_user_id = %v, want %s", storedConversation.CreatedByUserID, alice.ID)
	}

	var storedMembers []store.ConversationMember
	if err := db.Where("conversation_id = ?", conversationID).Find(&storedMembers).Error; err != nil {
		t.Fatalf("find stored members: %v", err)
	}
	if len(storedMembers) != 2 {
		t.Fatalf("stored member count = %d, want 2", len(storedMembers))
	}
	rolesByID := map[string]string{}
	for _, member := range storedMembers {
		rolesByID[member.MemberID] = member.Role
	}
	if rolesByID[alice.ID] != store.ConversationMemberRoleOwner {
		t.Fatalf("alice role = %v, want owner", rolesByID[alice.ID])
	}
	if rolesByID[bob.ID] != store.ConversationMemberRoleMember {
		t.Fatalf("bob role = %v, want member", rolesByID[bob.ID])
	}

	userLowID, userHighID := orderTestUserIDs(alice.ID, bob.ID)
	var storedDirect store.DirectConversation
	if err := db.First(&storedDirect, "user_low_id = ? AND user_high_id = ?", userLowID, userHighID).Error; err != nil {
		t.Fatalf("find direct conversation: %v", err)
	}
	if storedDirect.ConversationID != conversationID {
		t.Fatalf("direct conversation_id = %v, want %s", storedDirect.ConversationID, conversationID)
	}

	existingResp, existingBody := postJSON(t, server, "/api/client/conversations/direct", map[string]any{
		"user_id": strings.ToUpper(bob.ID),
	}, userCookie)
	if existingResp.StatusCode != http.StatusOK {
		t.Fatalf("existing status = %d, want 200, body = %#v", existingResp.StatusCode, existingBody)
	}
	existingData := requireSuccess(t, existingBody)
	if existingData["created"] != false {
		t.Fatalf("existing created = %v, want false", existingData["created"])
	}
	existingConversation := existingData["conversation"].(map[string]any)
	if existingConversation["id"] != conversationID {
		t.Fatalf("existing conversation id = %v, want %s", existingConversation["id"], conversationID)
	}

	var conversationCount int64
	if err := db.Model(&store.Conversation{}).Where("kind = ?", store.ConversationKindDirect).Count(&conversationCount).Error; err != nil {
		t.Fatalf("count direct conversations: %v", err)
	}
	if conversationCount != 1 {
		t.Fatalf("direct conversation count = %d, want 1", conversationCount)
	}
}

func TestCreateDirectConversationRejectsInvalidTargets(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()
	now := time.Date(2026, 7, 3, 9, 0, 0, 0, time.UTC)
	alice := insertTestUser(t, db, "alice@example.com", "Alice", store.UserStatusActive, now)
	disabled := insertTestUser(t, db, "disabled@example.com", "Disabled", store.UserStatusDisabled, now)
	userCookie := loginAsUser(t, server, alice.Email)

	for _, tc := range []struct {
		name   string
		userID string
	}{
		{name: "self", userID: alice.ID},
		{name: "disabled", userID: disabled.ID},
		{name: "missing", userID: uuid.NewString()},
		{name: "invalid uuid", userID: "not-a-uuid"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp, body := postJSON(t, server, "/api/client/conversations/direct", map[string]any{
				"user_id": tc.userID,
			}, userCookie)
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400, body = %#v", resp.StatusCode, body)
			}
			requireError(t, body, "invalid_request")
		})
	}
}

func TestMarkConversationReadAdvancesCurrentUserReadSeq(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()
	now := time.Date(2026, 7, 3, 9, 0, 0, 0, time.UTC)
	alice := insertTestUser(t, db, "alice@example.com", "Alice", store.UserStatusActive, now)
	bob := insertTestUser(t, db, "bob@example.com", "Bob", store.UserStatusActive, now)
	conversation := insertTestConversation(t, db, testConversationInput{
		createdByUserID:    alice.ID,
		kind:               store.ConversationKindDirect,
		lastMessageSeq:     8,
		lastMessageSummary: "latest",
		memberIDs:          []string{alice.ID, bob.ID},
		now:                now,
	})
	setTestConversationMemberLastReadSeq(t, db, conversation.ID, alice.ID, 2)
	setTestConversationMemberLastReadSeq(t, db, conversation.ID, bob.ID, 1)

	resp, body := postJSON(t, server, "/api/client/conversations/"+conversation.ID+"/read", map[string]any{}, loginAsUser(t, server, alice.Email))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %#v", resp.StatusCode, body)
	}
	data := requireSuccess(t, body)
	if data["conversation_id"] != conversation.ID {
		t.Fatalf("conversation_id = %v, want %s", data["conversation_id"], conversation.ID)
	}
	if data["last_read_seq"] != float64(8) {
		t.Fatalf("last_read_seq = %v, want 8", data["last_read_seq"])
	}
	if data["unread_count"] != float64(0) {
		t.Fatalf("unread_count = %v, want 0", data["unread_count"])
	}

	if got := getTestConversationMemberLastReadSeq(t, db, conversation.ID, alice.ID); got != 8 {
		t.Fatalf("alice last_read_seq = %d, want 8", got)
	}
	if got := getTestConversationMemberLastReadSeq(t, db, conversation.ID, bob.ID); got != 1 {
		t.Fatalf("bob last_read_seq = %d, want 1", got)
	}
}

func TestMarkConversationReadDoesNotRegressReadSeq(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()
	now := time.Date(2026, 7, 3, 9, 0, 0, 0, time.UTC)
	alice := insertTestUser(t, db, "alice@example.com", "Alice", store.UserStatusActive, now)
	bob := insertTestUser(t, db, "bob@example.com", "Bob", store.UserStatusActive, now)
	conversation := insertTestConversation(t, db, testConversationInput{
		createdByUserID:    alice.ID,
		kind:               store.ConversationKindDirect,
		lastMessageSeq:     10,
		lastMessageSummary: "latest",
		memberIDs:          []string{alice.ID, bob.ID},
		now:                now,
	})
	setTestConversationMemberLastReadSeq(t, db, conversation.ID, alice.ID, 7)

	resp, body := postJSON(t, server, "/api/client/conversations/"+conversation.ID+"/read", map[string]any{
		"up_to_seq": 3,
	}, loginAsUser(t, server, alice.Email))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %#v", resp.StatusCode, body)
	}
	data := requireSuccess(t, body)
	if data["last_read_seq"] != float64(7) {
		t.Fatalf("last_read_seq = %v, want 7", data["last_read_seq"])
	}
	if data["unread_count"] != float64(3) {
		t.Fatalf("unread_count = %v, want 3", data["unread_count"])
	}
	if got := getTestConversationMemberLastReadSeq(t, db, conversation.ID, alice.ID); got != 7 {
		t.Fatalf("alice last_read_seq = %d, want 7", got)
	}
}

func TestMarkConversationReadRejectsInvalidOrUnauthorizedRequests(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()
	now := time.Date(2026, 7, 3, 9, 0, 0, 0, time.UTC)
	alice := insertTestUser(t, db, "alice@example.com", "Alice", store.UserStatusActive, now)
	bob := insertTestUser(t, db, "bob@example.com", "Bob", store.UserStatusActive, now)
	dave := insertTestUser(t, db, "dave@example.com", "Dave", store.UserStatusActive, now)
	conversation := insertTestConversation(t, db, testConversationInput{
		createdByUserID: alice.ID,
		kind:            store.ConversationKindDirect,
		lastMessageSeq:  5,
		memberIDs:       []string{alice.ID, bob.ID},
		now:             now,
	})

	resp, body := postJSON(t, server, "/api/client/conversations/"+conversation.ID+"/read", map[string]any{})
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("missing session status = %d, want 401", resp.StatusCode)
	}
	requireError(t, body, "unauthorized")

	aliceCookie := loginAsUser(t, server, alice.Email)
	resp, body = postJSON(t, server, "/api/client/conversations/not-a-uuid/read", map[string]any{}, aliceCookie)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid uuid status = %d, want 400", resp.StatusCode)
	}
	requireError(t, body, "invalid_request")

	resp, body = postJSON(t, server, "/api/client/conversations/"+conversation.ID+"/read", map[string]any{}, loginAsUser(t, server, dave.Email))
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("non-member status = %d, want 403", resp.StatusCode)
	}
	requireError(t, body, "forbidden")

	resp, body = postJSON(t, server, "/api/client/conversations/"+conversation.ID+"/read", map[string]any{
		"up_to_seq": -1,
	}, aliceCookie)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid up_to_seq status = %d, want 400", resp.StatusCode)
	}
	requireError(t, body, "invalid_request")
}

func TestCreateConversationTextMessageRequiresUserSession(t *testing.T) {
	server, _ := newTestRouter(t)
	defer server.Close()

	resp, body := postJSON(t, server, "/api/client/conversations/"+uuid.NewString()+"/messages", map[string]any{
		"client_message_id": "client-message-1",
		"body": map[string]any{
			"type":    "text",
			"content": "你好",
		},
	})

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
	requireError(t, body, "unauthorized")
}

func TestCreateConversationTextMessageStoresSummaryAndUpdatesConversation(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()
	now := time.Date(2026, 7, 3, 9, 0, 0, 0, time.UTC)
	alice := insertTestUser(t, db, "alice@example.com", "Alice", store.UserStatusActive, now)
	bob := insertTestUser(t, db, "bob@example.com", "Bob", store.UserStatusActive, now)
	lastMessageAt := now.Add(-time.Hour)
	conversation := insertTestConversation(t, db, testConversationInput{
		createdByUserID:    alice.ID,
		kind:               store.ConversationKindDirect,
		lastMessageAt:      &lastMessageAt,
		lastMessageSeq:     2,
		lastMessageSummary: "上一条",
		memberIDs:          []string{alice.ID, bob.ID},
		now:                now.Add(-2 * time.Hour),
	})

	resp, body := postJSON(t, server, "/api/client/conversations/"+conversation.ID+"/messages", map[string]any{
		"client_message_id": "client-message-1",
		"body": map[string]any{
			"type":    "text",
			"content": "  你好，Bob  ",
		},
	}, loginAsUser(t, server, alice.Email))
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201, body = %#v", resp.StatusCode, body)
	}
	data := requireSuccess(t, body)
	message := data["message"].(map[string]any)
	if message["conversation_id"] != conversation.ID {
		t.Fatalf("message.conversation_id = %v, want %s", message["conversation_id"], conversation.ID)
	}
	if message["seq"] != float64(3) {
		t.Fatalf("message.seq = %v, want 3", message["seq"])
	}
	sender := message["sender"].(map[string]any)
	if sender["type"] != store.MessageSenderTypeUser {
		t.Fatalf("message.sender.type = %v, want user", sender["type"])
	}
	if sender["id"] != alice.ID {
		t.Fatalf("message.sender.id = %v, want %s", sender["id"], alice.ID)
	}
	messageBody := message["body"].(map[string]any)
	if messageBody["type"] != "text" {
		t.Fatalf("message.body.type = %v, want text", messageBody["type"])
	}
	if messageBody["content"] != "你好，Bob" {
		t.Fatalf("message.body.content = %v, want normalized text content", messageBody["content"])
	}
	if message["client_message_id"] != "client-message-1" {
		t.Fatalf("message.client_message_id = %v, want client-message-1", message["client_message_id"])
	}
	createdAt, ok := message["created_at"].(string)
	if !ok || createdAt == "" {
		t.Fatalf("message.created_at = %#v, want non-empty string", message["created_at"])
	}

	var storedMessage store.Message
	if err := db.First(&storedMessage, "id = ?", message["id"]).Error; err != nil {
		t.Fatalf("find stored message: %v", err)
	}
	if storedMessage.ConversationID != conversation.ID {
		t.Fatalf("stored conversation_id = %v, want %s", storedMessage.ConversationID, conversation.ID)
	}
	if storedMessage.Seq != 3 {
		t.Fatalf("stored seq = %d, want 3", storedMessage.Seq)
	}
	if storedMessage.SenderType != store.MessageSenderTypeUser {
		t.Fatalf("stored sender_type = %v, want user", storedMessage.SenderType)
	}
	if storedMessage.SenderID == nil || *storedMessage.SenderID != alice.ID {
		t.Fatalf("stored sender_id = %v, want %s", storedMessage.SenderID, alice.ID)
	}
	if storedMessage.ClientMessageID == nil || *storedMessage.ClientMessageID != "client-message-1" {
		t.Fatalf("stored client_message_id = %v, want client-message-1", storedMessage.ClientMessageID)
	}
	if storedMessage.Summary != "你好，Bob" {
		t.Fatalf("stored summary = %v, want text content", storedMessage.Summary)
	}
	var storedBody map[string]any
	if err := json.Unmarshal(storedMessage.Body, &storedBody); err != nil {
		t.Fatalf("unmarshal stored body: %v", err)
	}
	if storedBody["type"] != "text" {
		t.Fatalf("stored body.type = %v, want text", storedBody["type"])
	}
	if storedBody["content"] != "你好，Bob" {
		t.Fatalf("stored body.content = %v, want text content", storedBody["content"])
	}

	var storedConversation store.Conversation
	if err := db.First(&storedConversation, "id = ?", conversation.ID).Error; err != nil {
		t.Fatalf("find stored conversation: %v", err)
	}
	if storedConversation.LastMessageID == nil || *storedConversation.LastMessageID != storedMessage.ID {
		t.Fatalf("last_message_id = %v, want %s", storedConversation.LastMessageID, storedMessage.ID)
	}
	if storedConversation.LastMessageSeq != storedMessage.Seq {
		t.Fatalf("last_message_seq = %d, want %d", storedConversation.LastMessageSeq, storedMessage.Seq)
	}
	if storedConversation.LastMessageSummary != storedMessage.Summary {
		t.Fatalf("last_message_summary = %v, want %s", storedConversation.LastMessageSummary, storedMessage.Summary)
	}
	if storedConversation.LastMessageAt == nil || !storedConversation.LastMessageAt.Equal(storedMessage.CreatedAt) {
		t.Fatalf("last_message_at = %v, want %s", storedConversation.LastMessageAt, storedMessage.CreatedAt)
	}

	if got := getTestConversationMemberLastReadSeq(t, db, conversation.ID, alice.ID); got != 3 {
		t.Fatalf("alice last_read_seq = %d, want 3", got)
	}
	if got := getTestConversationMemberLastReadSeq(t, db, conversation.ID, bob.ID); got != 0 {
		t.Fatalf("bob last_read_seq = %d, want 0", got)
	}
}

func TestCreateConversationTextMessageReturnsExistingForSameClientMessageID(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()
	now := time.Date(2026, 7, 3, 9, 0, 0, 0, time.UTC)
	alice := insertTestUser(t, db, "alice@example.com", "Alice", store.UserStatusActive, now)
	bob := insertTestUser(t, db, "bob@example.com", "Bob", store.UserStatusActive, now)
	conversation := insertTestConversation(t, db, testConversationInput{
		createdByUserID: alice.ID,
		kind:            store.ConversationKindDirect,
		memberIDs:       []string{alice.ID, bob.ID},
		now:             now,
	})
	cookie := loginAsUser(t, server, alice.Email)
	path := "/api/client/conversations/" + conversation.ID + "/messages"

	firstResp, firstBody := postJSON(t, server, path, map[string]any{
		"client_message_id": "client-message-1",
		"body": map[string]any{
			"type":    "text",
			"content": "第一条",
		},
	}, cookie)
	if firstResp.StatusCode != http.StatusCreated {
		t.Fatalf("first status = %d, want 201, body = %#v", firstResp.StatusCode, firstBody)
	}
	firstMessage := requireSuccess(t, firstBody)["message"].(map[string]any)

	secondResp, secondBody := postJSON(t, server, path, map[string]any{
		"client_message_id": "client-message-1",
		"body": map[string]any{
			"type":    "text",
			"content": "重复提交的不同内容",
		},
	}, cookie)
	if secondResp.StatusCode != http.StatusOK {
		t.Fatalf("second status = %d, want 200, body = %#v", secondResp.StatusCode, secondBody)
	}
	secondMessage := requireSuccess(t, secondBody)["message"].(map[string]any)
	if secondMessage["id"] != firstMessage["id"] {
		t.Fatalf("second message id = %v, want existing %v", secondMessage["id"], firstMessage["id"])
	}
	secondMessageBody := secondMessage["body"].(map[string]any)
	if secondMessageBody["content"] != "第一条" {
		t.Fatalf("second message body content = %v, want original content", secondMessageBody["content"])
	}

	var messageCount int64
	if err := db.Model(&store.Message{}).Where("conversation_id = ?", conversation.ID).Count(&messageCount).Error; err != nil {
		t.Fatalf("count messages: %v", err)
	}
	if messageCount != 1 {
		t.Fatalf("message count = %d, want 1", messageCount)
	}
}

func TestCreateUserMessageTimestampsNewMessageAfterTransactionReads(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()
	now := time.Date(2026, 7, 3, 9, 0, 0, 0, time.UTC)
	alice := insertTestUser(t, db, "alice@example.com", "Alice", store.UserStatusActive, now)
	bob := insertTestUser(t, db, "bob@example.com", "Bob", store.UserStatusActive, now)
	conversation := insertTestConversation(t, db, testConversationInput{
		createdByUserID: alice.ID,
		kind:            store.ConversationKindDirect,
		memberIDs:       []string{alice.ID, bob.ID},
		now:             now,
	})

	var delayed bool
	var delayedUntil time.Time
	if err := db.Callback().Query().Before("gorm:query").Register("test:delay_first_message_create_query", func(tx *gorm.DB) {
		if delayed {
			return
		}
		delayed = true
		time.Sleep(20 * time.Millisecond)
		delayedUntil = time.Now().UTC()
	}); err != nil {
		t.Fatalf("register query callback: %v", err)
	}

	message, created, _, err := (&Server{db: db}).createUserMessage(
		alice.ID,
		conversation.ID,
		"client-message-1",
		json.RawMessage(`{"type":"text","content":"hello"}`),
		"hello",
	)
	if err != nil {
		t.Fatalf("createUserMessage() error = %v", err)
	}
	if !created {
		t.Fatal("created = false, want true")
	}
	if !delayed {
		t.Fatal("query delay callback did not run")
	}
	if message.CreatedAt.Before(delayedUntil) {
		t.Fatalf("message.CreatedAt = %s, want >= delayedUntil %s", message.CreatedAt, delayedUntil)
	}
}

func TestListConversationMessagesReturnsRecentMessagesInSeqOrder(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()
	now := time.Date(2026, 7, 3, 9, 0, 0, 0, time.UTC)
	alice := insertTestUser(t, db, "alice@example.com", "Alice", store.UserStatusActive, now)
	bob := insertTestUser(t, db, "bob@example.com", "Bob", store.UserStatusActive, now)
	conversation := insertTestConversation(t, db, testConversationInput{
		createdByUserID: alice.ID,
		kind:            store.ConversationKindDirect,
		memberIDs:       []string{alice.ID, bob.ID},
		now:             now,
	})
	for seq := int64(1); seq <= 25; seq++ {
		insertTestMessage(t, db, conversation.ID, alice.ID, seq, fmt.Sprintf("message %02d", seq), now.Add(time.Duration(seq)*time.Minute))
	}
	deletedAt := now.Add(time.Hour)
	if err := db.Model(&store.Message{}).Where("conversation_id = ? AND seq = ?", conversation.ID, 24).Update("deleted_at", deletedAt).Error; err != nil {
		t.Fatalf("delete test message: %v", err)
	}

	resp, body := getJSON(t, server, "/api/client/conversations/"+conversation.ID+"/messages?limit=100", loginAsUser(t, server, alice.Email))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %#v", resp.StatusCode, body)
	}
	data := requireSuccess(t, body)
	messages := requireMessages(t, data)
	if len(messages) != 20 {
		t.Fatalf("message count = %d, want 20", len(messages))
	}

	gotSeqs := make([]int64, 0, len(messages))
	for _, rawMessage := range messages {
		message := rawMessage.(map[string]any)
		gotSeqs = append(gotSeqs, int64(message["seq"].(float64)))
	}
	wantSeqs := []int64{5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 25}
	if !slices.Equal(gotSeqs, wantSeqs) {
		t.Fatalf("message seqs = %v, want %v", gotSeqs, wantSeqs)
	}

	first := messages[0].(map[string]any)
	firstBody := first["body"].(map[string]any)
	if firstBody["content"] != "message 05" {
		t.Fatalf("first body content = %v, want message 05", firstBody["content"])
	}
	last := messages[len(messages)-1].(map[string]any)
	if last["seq"] != float64(25) {
		t.Fatalf("last seq = %v, want 25", last["seq"])
	}
	page := data["page"].(map[string]any)
	if page["limit"] != float64(20) {
		t.Fatalf("page.limit = %v, want 20", page["limit"])
	}
	if page["oldest_seq"] != float64(5) || page["newest_seq"] != float64(25) {
		t.Fatalf("page seq range = %v-%v, want 5-25", page["oldest_seq"], page["newest_seq"])
	}
	if page["has_more_before"] != true {
		t.Fatalf("has_more_before = %v, want true", page["has_more_before"])
	}
	if page["has_more_after"] != false {
		t.Fatalf("has_more_after = %v, want false", page["has_more_after"])
	}
}

func TestListConversationMessagesSupportsBeforeAndAfterSeq(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()
	now := time.Date(2026, 7, 3, 9, 0, 0, 0, time.UTC)
	alice := insertTestUser(t, db, "alice@example.com", "Alice", store.UserStatusActive, now)
	bob := insertTestUser(t, db, "bob@example.com", "Bob", store.UserStatusActive, now)
	conversation := insertTestConversation(t, db, testConversationInput{
		createdByUserID: alice.ID,
		kind:            store.ConversationKindDirect,
		memberIDs:       []string{alice.ID, bob.ID},
		now:             now,
	})
	for seq := int64(1); seq <= 10; seq++ {
		insertTestMessage(t, db, conversation.ID, alice.ID, seq, fmt.Sprintf("message %02d", seq), now.Add(time.Duration(seq)*time.Minute))
	}
	cookie := loginAsUser(t, server, alice.Email)

	beforeResp, beforeBody := getJSON(t, server, "/api/client/conversations/"+conversation.ID+"/messages?before_seq=8&limit=3", cookie)
	if beforeResp.StatusCode != http.StatusOK {
		t.Fatalf("before status = %d, want 200, body = %#v", beforeResp.StatusCode, beforeBody)
	}
	beforeMessages := requireMessages(t, requireSuccess(t, beforeBody))
	beforeSeqs := make([]int64, 0, len(beforeMessages))
	for _, rawMessage := range beforeMessages {
		beforeSeqs = append(beforeSeqs, int64(rawMessage.(map[string]any)["seq"].(float64)))
	}
	if !slices.Equal(beforeSeqs, []int64{5, 6, 7}) {
		t.Fatalf("before seqs = %v, want [5 6 7]", beforeSeqs)
	}
	beforePage := requireSuccess(t, beforeBody)["page"].(map[string]any)
	if beforePage["has_more_before"] != true || beforePage["has_more_after"] != true {
		t.Fatalf("before page = %#v, want more before and after", beforePage)
	}

	afterResp, afterBody := getJSON(t, server, "/api/client/conversations/"+conversation.ID+"/messages?after_seq=7&limit=3", cookie)
	if afterResp.StatusCode != http.StatusOK {
		t.Fatalf("after status = %d, want 200, body = %#v", afterResp.StatusCode, afterBody)
	}
	afterMessages := requireMessages(t, requireSuccess(t, afterBody))
	afterSeqs := make([]int64, 0, len(afterMessages))
	for _, rawMessage := range afterMessages {
		afterSeqs = append(afterSeqs, int64(rawMessage.(map[string]any)["seq"].(float64)))
	}
	if !slices.Equal(afterSeqs, []int64{8, 9, 10}) {
		t.Fatalf("after seqs = %v, want [8 9 10]", afterSeqs)
	}
	afterPage := requireSuccess(t, afterBody)["page"].(map[string]any)
	if afterPage["has_more_before"] != true || afterPage["has_more_after"] != false {
		t.Fatalf("after page = %#v, want more before only", afterPage)
	}
}

func TestListConversationMessagesLimitsPreJoinHistoryTo100Messages(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()
	now := time.Date(2026, 7, 3, 9, 0, 0, 0, time.UTC)
	alice := insertTestUser(t, db, "alice@example.com", "Alice", store.UserStatusActive, now)
	bob := insertTestUser(t, db, "bob@example.com", "Bob", store.UserStatusActive, now)
	conversation := insertTestConversation(t, db, testConversationInput{
		createdByUserID: alice.ID,
		kind:            store.ConversationKindDirect,
		memberIDs:       []string{alice.ID, bob.ID},
		now:             now,
	})
	for seq := int64(1); seq <= 150; seq++ {
		insertTestMessage(t, db, conversation.ID, alice.ID, seq, fmt.Sprintf("message %03d", seq), now.Add(time.Duration(seq)*time.Minute))
	}
	joinAt := now.Add(151 * time.Minute)
	if err := db.Model(&store.ConversationMember{}).
		Where("conversation_id = ? AND member_type = ? AND member_id = ?", conversation.ID, store.ConversationMemberTypeUser, bob.ID).
		Updates(map[string]any{
			"joined_at":                joinAt,
			"history_visible_from_seq": int64(51),
		}).Error; err != nil {
		t.Fatalf("set member visible history: %v", err)
	}
	cookie := loginAsUser(t, server, bob.Email)

	beforeResp, beforeBody := getJSON(t, server, "/api/client/conversations/"+conversation.ID+"/messages?before_seq=55&limit=20", cookie)
	if beforeResp.StatusCode != http.StatusOK {
		t.Fatalf("before status = %d, want 200, body = %#v", beforeResp.StatusCode, beforeBody)
	}
	beforeMessages := requireMessages(t, requireSuccess(t, beforeBody))
	beforeSeqs := make([]int64, 0, len(beforeMessages))
	for _, rawMessage := range beforeMessages {
		beforeSeqs = append(beforeSeqs, int64(rawMessage.(map[string]any)["seq"].(float64)))
	}
	if !slices.Equal(beforeSeqs, []int64{51, 52, 53, 54}) {
		t.Fatalf("before seqs = %v, want [51 52 53 54]", beforeSeqs)
	}
	beforePage := requireSuccess(t, beforeBody)["page"].(map[string]any)
	if beforePage["has_more_before"] != false || beforePage["has_more_after"] != true {
		t.Fatalf("before page = %#v, want no more before and more after", beforePage)
	}

	afterResp, afterBody := getJSON(t, server, "/api/client/conversations/"+conversation.ID+"/messages?after_seq=1&limit=20", cookie)
	if afterResp.StatusCode != http.StatusOK {
		t.Fatalf("after status = %d, want 200, body = %#v", afterResp.StatusCode, afterBody)
	}
	afterMessages := requireMessages(t, requireSuccess(t, afterBody))
	if len(afterMessages) != 20 {
		t.Fatalf("after message count = %d, want 20", len(afterMessages))
	}
	firstAfter := afterMessages[0].(map[string]any)
	if firstAfter["seq"] != float64(51) {
		t.Fatalf("first after seq = %v, want 51", firstAfter["seq"])
	}
	afterPage := requireSuccess(t, afterBody)["page"].(map[string]any)
	if afterPage["has_more_before"] != false || afterPage["has_more_after"] != true {
		t.Fatalf("after page = %#v, want no more before and more after", afterPage)
	}
}

func TestListConversationMessagesReportsAccuratePageBoundaries(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()
	now := time.Date(2026, 7, 3, 9, 0, 0, 0, time.UTC)
	alice := insertTestUser(t, db, "alice@example.com", "Alice", store.UserStatusActive, now)
	bob := insertTestUser(t, db, "bob@example.com", "Bob", store.UserStatusActive, now)
	conversation := insertTestConversation(t, db, testConversationInput{
		createdByUserID: alice.ID,
		kind:            store.ConversationKindDirect,
		memberIDs:       []string{alice.ID, bob.ID},
		now:             now,
	})
	for seq := int64(1); seq <= 10; seq++ {
		insertTestMessage(t, db, conversation.ID, alice.ID, seq, fmt.Sprintf("message %02d", seq), now.Add(time.Duration(seq)*time.Minute))
	}

	resp, body := getJSON(t, server, "/api/client/conversations/"+conversation.ID+"/messages?before_seq=999&limit=20", loginAsUser(t, server, alice.Email))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %#v", resp.StatusCode, body)
	}
	page := requireSuccess(t, body)["page"].(map[string]any)
	if page["has_more_before"] != false || page["has_more_after"] != false {
		t.Fatalf("page = %#v, want no more before or after", page)
	}
}

func TestListConversationMessagesRejectsInvalidAccessAndQuery(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()
	now := time.Date(2026, 7, 3, 9, 0, 0, 0, time.UTC)
	alice := insertTestUser(t, db, "alice@example.com", "Alice", store.UserStatusActive, now)
	bob := insertTestUser(t, db, "bob@example.com", "Bob", store.UserStatusActive, now)
	carol := insertTestUser(t, db, "carol@example.com", "Carol", store.UserStatusActive, now)
	conversation := insertTestConversation(t, db, testConversationInput{
		createdByUserID: alice.ID,
		kind:            store.ConversationKindDirect,
		memberIDs:       []string{alice.ID, bob.ID},
		now:             now,
	})
	cookie := loginAsUser(t, server, alice.Email)

	for _, tc := range []struct {
		name       string
		path       string
		cookie     *http.Cookie
		statusCode int
		errorCode  string
	}{
		{
			name:       "unauthorized",
			path:       "/api/client/conversations/" + conversation.ID + "/messages",
			statusCode: http.StatusUnauthorized,
			errorCode:  "unauthorized",
		},
		{
			name:       "invalid conversation id",
			path:       "/api/client/conversations/not-a-uuid/messages",
			cookie:     cookie,
			statusCode: http.StatusBadRequest,
			errorCode:  "invalid_request",
		},
		{
			name:       "before and after seq together",
			path:       "/api/client/conversations/" + conversation.ID + "/messages?before_seq=8&after_seq=2",
			cookie:     cookie,
			statusCode: http.StatusBadRequest,
			errorCode:  "invalid_request",
		},
		{
			name:       "invalid before seq",
			path:       "/api/client/conversations/" + conversation.ID + "/messages?before_seq=0",
			cookie:     cookie,
			statusCode: http.StatusBadRequest,
			errorCode:  "invalid_request",
		},
		{
			name:       "non member",
			path:       "/api/client/conversations/" + conversation.ID + "/messages",
			cookie:     loginAsUser(t, server, carol.Email),
			statusCode: http.StatusForbidden,
			errorCode:  "forbidden",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var resp *http.Response
			var body map[string]any
			if tc.cookie == nil {
				resp, body = getJSON(t, server, tc.path)
			} else {
				resp, body = getJSON(t, server, tc.path, tc.cookie)
			}
			if resp.StatusCode != tc.statusCode {
				t.Fatalf("status = %d, want %d, body = %#v", resp.StatusCode, tc.statusCode, body)
			}
			requireError(t, body, tc.errorCode)
		})
	}
}

func TestListConversationMessagesRejectsDissolvedConversation(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()
	now := time.Date(2026, 7, 3, 9, 0, 0, 0, time.UTC)
	alice := insertTestUser(t, db, "alice@example.com", "Alice", store.UserStatusActive, now)
	bob := insertTestUser(t, db, "bob@example.com", "Bob", store.UserStatusActive, now)
	dissolvedAt := now.Add(time.Hour)
	conversation := insertTestConversation(t, db, testConversationInput{
		createdByUserID: alice.ID,
		kind:            store.ConversationKindDirect,
		memberIDs:       []string{alice.ID, bob.ID},
		now:             now,
	})
	if err := db.Model(&store.Conversation{}).Where("id = ?", conversation.ID).Updates(map[string]any{
		"status":       store.ConversationStatusDissolved,
		"dissolved_at": dissolvedAt,
	}).Error; err != nil {
		t.Fatalf("dissolve conversation: %v", err)
	}

	resp, body := getJSON(t, server, "/api/client/conversations/"+conversation.ID+"/messages", loginAsUser(t, server, alice.Email))
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403, body = %#v", resp.StatusCode, body)
	}
	requireError(t, body, "forbidden")
}

func TestCreateConversationTextMessagePushesMessageCreatedToConversationMembers(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()
	now := time.Date(2026, 7, 3, 9, 0, 0, 0, time.UTC)
	alice := insertTestUser(t, db, "alice@example.com", "Alice", store.UserStatusActive, now)
	bob := insertTestUser(t, db, "bob@example.com", "Bob", store.UserStatusActive, now)
	conversation := insertTestConversation(t, db, testConversationInput{
		createdByUserID: alice.ID,
		kind:            store.ConversationKindDirect,
		memberIDs:       []string{alice.ID, bob.ID},
		now:             now,
	})
	aliceCookie := loginAsUser(t, server, alice.Email)
	bobCookie := loginAsUser(t, server, bob.Email)
	aliceConn := dialClientWebSocket(t, server, aliceCookie)
	bobConn := dialClientWebSocket(t, server, bobCookie)
	if ready := readRealtimeEvent(t, aliceConn); ready.Kind != realtime.KindEvent || ready.Event != realtime.EventSystemReady {
		t.Fatalf("alice ready envelope = %#v, want system.ready", ready)
	}
	if ready := readRealtimeEvent(t, bobConn); ready.Kind != realtime.KindEvent || ready.Event != realtime.EventSystemReady {
		t.Fatalf("bob ready envelope = %#v, want system.ready", ready)
	}

	resp, body := postJSON(t, server, "/api/client/conversations/"+conversation.ID+"/messages", map[string]any{
		"client_message_id": "client-message-1",
		"body": map[string]any{
			"type":    "text",
			"content": "你好，Bob",
		},
	}, aliceCookie)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201, body = %#v", resp.StatusCode, body)
	}
	createdMessage := requireSuccess(t, body)["message"].(map[string]any)

	for name, conn := range map[string]*websocket.Conn{
		"alice": aliceConn,
		"bob":   bobConn,
	} {
		pushedMessage := readMessageCreatedEvent(t, conn)
		if pushedMessage["id"] != createdMessage["id"] {
			t.Fatalf("%s pushed message id = %v, want %v", name, pushedMessage["id"], createdMessage["id"])
		}
		if pushedMessage["conversation_id"] != conversation.ID {
			t.Fatalf("%s conversation_id = %v, want %s", name, pushedMessage["conversation_id"], conversation.ID)
		}
		pushedBody := pushedMessage["body"].(map[string]any)
		if pushedBody["type"] != "text" || pushedBody["content"] != "你好，Bob" {
			t.Fatalf("%s pushed body = %#v, want text body", name, pushedBody)
		}
	}
}

func TestCreateConversationTextMessageRejectsInvalidRequests(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()
	now := time.Date(2026, 7, 3, 9, 0, 0, 0, time.UTC)
	alice := insertTestUser(t, db, "alice@example.com", "Alice", store.UserStatusActive, now)
	bob := insertTestUser(t, db, "bob@example.com", "Bob", store.UserStatusActive, now)
	carol := insertTestUser(t, db, "carol@example.com", "Carol", store.UserStatusActive, now)
	conversation := insertTestConversation(t, db, testConversationInput{
		createdByUserID: alice.ID,
		kind:            store.ConversationKindDirect,
		memberIDs:       []string{alice.ID, bob.ID},
		now:             now,
	})
	cookie := loginAsUser(t, server, alice.Email)

	for _, tc := range []struct {
		name       string
		path       string
		body       map[string]any
		cookie     *http.Cookie
		statusCode int
		errorCode  string
	}{
		{
			name:       "invalid conversation id",
			path:       "/api/client/conversations/not-a-uuid/messages",
			body:       map[string]any{"client_message_id": "client-message-1", "body": map[string]any{"type": "text", "content": "你好"}},
			cookie:     cookie,
			statusCode: http.StatusBadRequest,
			errorCode:  "invalid_request",
		},
		{
			name:       "missing client message id",
			path:       "/api/client/conversations/" + conversation.ID + "/messages",
			body:       map[string]any{"body": map[string]any{"type": "text", "content": "你好"}},
			cookie:     cookie,
			statusCode: http.StatusBadRequest,
			errorCode:  "invalid_request",
		},
		{
			name:       "empty content",
			path:       "/api/client/conversations/" + conversation.ID + "/messages",
			body:       map[string]any{"client_message_id": "client-message-1", "body": map[string]any{"type": "text", "content": "   \n\t"}},
			cookie:     cookie,
			statusCode: http.StatusBadRequest,
			errorCode:  "invalid_request",
		},
		{
			name:       "unsupported body type",
			path:       "/api/client/conversations/" + conversation.ID + "/messages",
			body:       map[string]any{"client_message_id": "client-message-1", "body": map[string]any{"type": "image", "url": "https://example.com/a.png"}},
			cookie:     cookie,
			statusCode: http.StatusBadRequest,
			errorCode:  "invalid_request",
		},
		{
			name:       "non member",
			path:       "/api/client/conversations/" + conversation.ID + "/messages",
			body:       map[string]any{"client_message_id": "client-message-1", "body": map[string]any{"type": "text", "content": "你好"}},
			cookie:     loginAsUser(t, server, carol.Email),
			statusCode: http.StatusForbidden,
			errorCode:  "forbidden",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp, body := postJSON(t, server, tc.path, tc.body, tc.cookie)
			if resp.StatusCode != tc.statusCode {
				t.Fatalf("status = %d, want %d, body = %#v", resp.StatusCode, tc.statusCode, body)
			}
			requireError(t, body, tc.errorCode)
		})
	}
}

func TestCreateConversationTextMessageRejectsMutedConversation(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()
	now := time.Date(2026, 7, 3, 9, 0, 0, 0, time.UTC)
	alice := insertTestUser(t, db, "alice@example.com", "Alice", store.UserStatusActive, now)
	bob := insertTestUser(t, db, "bob@example.com", "Bob", store.UserStatusActive, now)
	conversation := insertTestConversation(t, db, testConversationInput{
		createdByUserID: alice.ID,
		kind:            store.ConversationKindDirect,
		memberIDs:       []string{alice.ID, bob.ID},
		now:             now,
	})
	if err := db.Model(&store.Conversation{}).Where("id = ?", conversation.ID).Update("posting_policy", store.ConversationPostingPolicyMuted).Error; err != nil {
		t.Fatalf("mute conversation: %v", err)
	}

	resp, body := postJSON(t, server, "/api/client/conversations/"+conversation.ID+"/messages", map[string]any{
		"client_message_id": "client-message-1",
		"body": map[string]any{
			"type":    "text",
			"content": "你好",
		},
	}, loginAsUser(t, server, alice.Email))
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403, body = %#v", resp.StatusCode, body)
	}
	requireError(t, body, "forbidden")
}

func TestRecordUserPongUpdatesLastOnlineAt(t *testing.T) {
	_, db := newTestRouter(t)
	user := insertTestUser(t, db, "alice@example.com", "Alice", store.UserStatusActive, time.Now().UTC())
	server := &Server{db: db}
	pongAt := time.Date(2026, 7, 3, 2, 0, 0, 0, time.UTC)

	server.recordUserPong(user.ID, pongAt)

	var stored store.User
	if err := db.First(&stored, "id = ?", user.ID).Error; err != nil {
		t.Fatalf("load user: %v", err)
	}
	if stored.LastOnlineAt == nil {
		t.Fatal("LastOnlineAt = nil, want pong time")
	}
	if !stored.LastOnlineAt.Equal(pongAt) {
		t.Fatalf("LastOnlineAt = %s, want %s", stored.LastOnlineAt, pongAt)
	}
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
		"/api/admin/assistant/models",
		"/api/admin/assistant/models/discover",
		"/api/admin/assistant/models/{id}/health-check",
		"/api/client/auth/login",
		"/api/client/auth/logout",
		"/api/client/me",
		"/api/client/contacts/users",
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
	if data["authenticated"] != false {
		t.Fatalf("authenticated = %v, want false", data["authenticated"])
	}
	if _, ok := data["version"]; ok {
		t.Fatalf("version = %v, want omitted", data["version"])
	}
}

func TestClientInfoReturnsAuthenticationState(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	insertTestUser(t, db, "alice@example.com", "Alice", store.UserStatusActive, time.Now().UTC())
	userCookie := loginAsUser(t, server, "alice@example.com")

	resp, body := getJSON(t, server, "/api/client/info", userCookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	data := requireSuccess(t, body)
	if data["authenticated"] != true {
		t.Fatalf("authenticated = %v, want true", data["authenticated"])
	}
}

func TestClientInfoReturnsEnabledThirdPartyLoginProviders(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	insertTestThirdPartyLoginProvider(t, db, store.ThirdPartyLoginProvider{
		Name:         "Disabled SSO",
		Key:          "disabled-sso",
		Type:         store.ThirdPartyLoginProviderTypeOIDC,
		Enabled:      false,
		ClientID:     "disabled-client",
		ClientSecret: "disabled-secret",
		Scopes:       json.RawMessage(`["openid","email"]`),
		Config: thirdPartyProviderConfig(t, map[string]any{
			"authorize_url":     "https://disabled.example.com/authorize",
			"token_url":         "https://disabled.example.com/token",
			"userinfo_url":      "https://disabled.example.com/userinfo",
			"external_id_field": "sub",
			"email_field":       "email",
			"name_field":        "name",
		}),
		SortOrder: 1,
	})
	insertTestThirdPartyLoginProvider(t, db, store.ThirdPartyLoginProvider{
		Name:         "Enterprise SSO",
		Key:          "enterprise",
		Type:         store.ThirdPartyLoginProviderTypeOIDC,
		Enabled:      true,
		ClientID:     "client-id",
		ClientSecret: "client-secret",
		Scopes:       json.RawMessage(`["openid","email","profile"]`),
		Config: thirdPartyProviderConfig(t, map[string]any{
			"authorize_url":     "https://sso.example.com/authorize",
			"token_url":         "https://sso.example.com/token",
			"userinfo_url":      "https://sso.example.com/userinfo",
			"external_id_field": "sub",
			"email_field":       "email",
			"name_field":        "name",
		}),
		SortOrder: 2,
	})

	resp, body := getJSON(t, server, "/api/client/info")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	data := requireSuccess(t, body)
	providers, ok := data["third_party_providers"].([]any)
	if !ok {
		t.Fatalf("third_party_providers = %#v, want array", data["third_party_providers"])
	}
	if len(providers) != 1 {
		t.Fatalf("third-party provider count = %d, want 1", len(providers))
	}
	publicProvider := providers[0].(map[string]any)
	if publicProvider["key"] != "enterprise" {
		t.Fatalf("provider key = %#v, want enterprise", publicProvider["key"])
	}
	if publicProvider["name"] != "Enterprise SSO" {
		t.Fatalf("provider name = %#v, want Enterprise SSO", publicProvider["name"])
	}
	if _, ok := publicProvider["client_secret"]; ok {
		t.Fatalf("public third-party provider leaks client_secret: %#v", publicProvider)
	}
}

func TestThirdPartyLoginCreatesUserSessionAndRedirectsToInit(t *testing.T) {
	var tokenCalled bool
	var userinfoCalled bool
	thirdPartyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			tokenCalled = true
			if err := r.ParseForm(); err != nil {
				t.Fatalf("parse token form: %v", err)
			}
			if r.Form.Get("grant_type") != "authorization_code" {
				t.Fatalf("grant_type = %q, want authorization_code", r.Form.Get("grant_type"))
			}
			if r.Form.Get("code") != "callback-code" {
				t.Fatalf("code = %q, want callback-code", r.Form.Get("code"))
			}
			if r.Form.Get("client_id") != "client-id" {
				t.Fatalf("client_id = %q, want client-id", r.Form.Get("client_id"))
			}
			if r.Form.Get("client_secret") != "client-secret" {
				t.Fatalf("client_secret = %q, want client-secret", r.Form.Get("client_secret"))
			}
			if r.Form.Get("code_verifier") == "" {
				t.Fatal("code_verifier is empty")
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"access-token","token_type":"Bearer"}`))
		case "/userinfo":
			userinfoCalled = true
			if r.Header.Get("Authorization") != "Bearer access-token" {
				t.Fatalf("Authorization = %q, want Bearer access-token", r.Header.Get("Authorization"))
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"sub": "alice-external-id",
				"mail": "Alice.ThirdParty@example.com",
				"mobile": "13812345678",
				"real_name": "Alice ThirdParty",
				"nick": "Ali",
				"picture": "https://sso.example.com/alice.webp"
			}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer thirdPartyServer.Close()

	server, db := newTestRouter(t)
	defer server.Close()
	provider := insertTestThirdPartyLoginProvider(t, db, store.ThirdPartyLoginProvider{
		Name:         "Enterprise SSO",
		Key:          "enterprise",
		Type:         store.ThirdPartyLoginProviderTypeOIDC,
		Enabled:      true,
		ClientID:     "client-id",
		ClientSecret: "client-secret",
		Scopes:       json.RawMessage(`["openid","email","profile"]`),
		Config: thirdPartyProviderConfig(t, map[string]any{
			"authorize_url":     thirdPartyServer.URL + "/authorize",
			"token_url":         thirdPartyServer.URL + "/token",
			"userinfo_url":      thirdPartyServer.URL + "/userinfo",
			"external_id_field": "sub",
			"email_field":       "mail",
			"phone_field":       "mobile",
			"name_field":        "real_name",
			"nickname_field":    "nick",
			"avatar_field":      "picture",
		}),
	})
	noRedirectClient := server.Client()
	noRedirectClient.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}

	startResp := getResponseWithClient(t, noRedirectClient, server, "/api/client/auth/third-party/enterprise/start?redirect=/init")
	if startResp.StatusCode != http.StatusFound {
		t.Fatalf("start status = %d, want 302", startResp.StatusCode)
	}
	authorizeLocation := startResp.Header.Get("Location")
	parsedAuthorizeURL, err := url.Parse(authorizeLocation)
	if err != nil {
		t.Fatalf("parse authorize location: %v", err)
	}
	if parsedAuthorizeURL.Path != "/authorize" {
		t.Fatalf("authorize path = %q, want /authorize", parsedAuthorizeURL.Path)
	}
	query := parsedAuthorizeURL.Query()
	if query.Get("response_type") != "code" {
		t.Fatalf("response_type = %q, want code", query.Get("response_type"))
	}
	if query.Get("client_id") != "client-id" {
		t.Fatalf("client_id = %q, want client-id", query.Get("client_id"))
	}
	if query.Get("scope") != "openid email profile" {
		t.Fatalf("scope = %q, want openid email profile", query.Get("scope"))
	}
	if query.Get("code_challenge_method") != "S256" {
		t.Fatalf("code_challenge_method = %q, want S256", query.Get("code_challenge_method"))
	}
	state := query.Get("state")
	if state == "" {
		t.Fatal("state is empty")
	}
	if query.Get("code_challenge") == "" {
		t.Fatal("code_challenge is empty")
	}
	stateCookie := requireThirdPartyStateCookie(t, startResp)

	callbackResp := getResponseWithClient(t, noRedirectClient, server, "/api/client/auth/third-party/enterprise/callback?code=callback-code&state="+url.QueryEscape(state), stateCookie)
	if callbackResp.StatusCode != http.StatusFound {
		t.Fatalf("callback status = %d, want 302", callbackResp.StatusCode)
	}
	if callbackResp.Header.Get("Location") != "/init" {
		t.Fatalf("callback location = %q, want /init", callbackResp.Header.Get("Location"))
	}
	requireUserSessionCookie(t, callbackResp)
	if !tokenCalled {
		t.Fatal("token endpoint was not called")
	}
	if !userinfoCalled {
		t.Fatal("userinfo endpoint was not called")
	}

	var user store.User
	if err := db.First(&user, "email = ?", "alice.thirdparty@example.com").Error; err != nil {
		t.Fatalf("find third-party user: %v", err)
	}
	if user.Name != "Alice ThirdParty" {
		t.Fatalf("user name = %q, want Alice ThirdParty", user.Name)
	}
	if user.Nickname != "Ali" {
		t.Fatalf("user nickname = %q, want Ali", user.Nickname)
	}
	if user.Phone == nil || *user.Phone != "+8613812345678" {
		t.Fatalf("user phone = %#v, want +8613812345678", user.Phone)
	}
	if user.Avatar != "https://sso.example.com/alice.webp" {
		t.Fatalf("user avatar = %q, want third-party avatar", user.Avatar)
	}
	var account store.ThirdPartyAccount
	if err := db.First(&account, "provider_id = ? AND external_user_id = ?", provider.ID, "alice-external-id").Error; err != nil {
		t.Fatalf("find third-party account binding: %v", err)
	}
	if account.UserID != user.ID {
		t.Fatalf("third-party account user_id = %q, want %q", account.UserID, user.ID)
	}

	var userSessionCount int64
	if err := db.Model(&store.UserSession{}).Where("user_id = ?", user.ID).Count(&userSessionCount).Error; err != nil {
		t.Fatalf("count user sessions: %v", err)
	}
	if userSessionCount != 1 {
		t.Fatalf("user session count = %d, want 1", userSessionCount)
	}

	var stateRecord store.ThirdPartyLoginState
	if err := db.First(&stateRecord).Error; err != nil {
		t.Fatalf("find third-party state: %v", err)
	}
	if stateRecord.ConsumedAt == nil {
		t.Fatal("state consumed_at = nil, want consumed timestamp")
	}
}

func TestThirdPartyLoginReusesExternalAccountWhenEmailIsMissing(t *testing.T) {
	var userinfoCalls int
	identityServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"access-token","token_type":"Bearer"}`))
		case "/userinfo":
			userinfoCalls++
			if r.Header.Get("Authorization") != "Bearer access-token" {
				t.Fatalf("Authorization = %q, want Bearer access-token", r.Header.Get("Authorization"))
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"id": 42,
				"login": "octocat",
				"name": "Octo Cat",
				"avatar_url": "https://example.com/octocat.webp"
			}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer identityServer.Close()

	server, db := newTestRouter(t)
	defer server.Close()
	provider := insertTestThirdPartyLoginProvider(t, db, store.ThirdPartyLoginProvider{
		Name:         "GitHub",
		Key:          "github",
		Type:         store.ThirdPartyLoginProviderTypeOIDC,
		Enabled:      true,
		ClientID:     "client-id",
		ClientSecret: "client-secret",
		Scopes:       json.RawMessage(`["read:user"]`),
		Config: thirdPartyProviderConfig(t, map[string]any{
			"authorize_url":     identityServer.URL + "/authorize",
			"token_url":         identityServer.URL + "/token",
			"userinfo_url":      identityServer.URL + "/userinfo",
			"external_id_field": "id",
			"name_field":        "name",
			"nickname_field":    "login",
			"avatar_field":      "avatar_url",
		}),
	})
	noRedirectClient := server.Client()
	noRedirectClient.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}

	for attempt := 0; attempt < 2; attempt++ {
		startResp := getResponseWithClient(t, noRedirectClient, server, "/api/client/auth/third-party/github/start?redirect=/init")
		if startResp.StatusCode != http.StatusFound {
			t.Fatalf("start status = %d, want 302", startResp.StatusCode)
		}
		parsedAuthorizeURL, err := url.Parse(startResp.Header.Get("Location"))
		if err != nil {
			t.Fatalf("parse authorize location: %v", err)
		}
		state := parsedAuthorizeURL.Query().Get("state")
		if state == "" {
			t.Fatal("state is empty")
		}

		callbackResp := getResponseWithClient(
			t,
			noRedirectClient,
			server,
			"/api/client/auth/third-party/github/callback?code=callback-code&state="+url.QueryEscape(state),
			requireThirdPartyStateCookie(t, startResp),
		)
		if callbackResp.StatusCode != http.StatusFound {
			t.Fatalf("callback status = %d, want 302", callbackResp.StatusCode)
		}
		requireUserSessionCookie(t, callbackResp)
	}

	if userinfoCalls != 2 {
		t.Fatalf("userinfo calls = %d, want 2", userinfoCalls)
	}
	var userCount int64
	if err := db.Model(&store.User{}).Count(&userCount).Error; err != nil {
		t.Fatalf("count users: %v", err)
	}
	if userCount != 1 {
		t.Fatalf("user count = %d, want 1", userCount)
	}
	var account store.ThirdPartyAccount
	if err := db.First(&account, "provider_id = ? AND external_user_id = ?", provider.ID, "42").Error; err != nil {
		t.Fatalf("find third-party account: %v", err)
	}
	var user store.User
	if err := db.First(&user, "id = ?", account.UserID).Error; err != nil {
		t.Fatalf("find bound user: %v", err)
	}
	if user.Email == "" || !strings.HasSuffix(user.Email, "@third-party.local") {
		t.Fatalf("generated user email = %q, want third-party.local address", user.Email)
	}
	if user.Name != "Octo Cat" {
		t.Fatalf("user name = %q, want Octo Cat", user.Name)
	}
}

func TestDingTalkLoginUsesUserAccessTokenHeaderForUserInfo(t *testing.T) {
	var tokenCalled bool
	var userinfoCalled bool
	var appTokenCalled bool
	var useridByUnionIDCalled bool
	var userdetailCalled bool
	dingTalkServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1.0/oauth2/userAccessToken":
			tokenCalled = true
			if r.Method != http.MethodPost {
				t.Fatalf("token method = %q, want POST", r.Method)
			}
			if !strings.Contains(r.Header.Get("Content-Type"), "application/json") {
				t.Fatalf("token content-type = %q, want json", r.Header.Get("Content-Type"))
			}
			var payload map[string]string
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode token payload: %v", err)
			}
			if payload["clientId"] != "ding-client-id" {
				t.Fatalf("clientId = %q, want ding-client-id", payload["clientId"])
			}
			if payload["clientSecret"] != "ding-client-secret" {
				t.Fatalf("clientSecret = %q, want ding-client-secret", payload["clientSecret"])
			}
			if payload["code"] != "callback-code" {
				t.Fatalf("code = %q, want callback-code", payload["code"])
			}
			if payload["grantType"] != "authorization_code" {
				t.Fatalf("grantType = %q, want authorization_code", payload["grantType"])
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"accessToken":"dingtalk-token","expireIn":7200}`))
		case "/v1.0/contact/users/me":
			userinfoCalled = true
			if r.Header.Get("Authorization") != "" {
				t.Fatalf("Authorization = %q, want empty", r.Header.Get("Authorization"))
			}
			if r.Header.Get("x-acs-dingtalk-access-token") != "dingtalk-token" {
				t.Fatalf("x-acs-dingtalk-access-token = %q, want dingtalk-token", r.Header.Get("x-acs-dingtalk-access-token"))
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
					"unionId": "ding-union-id",
					"openId": "ding-open-id",
					"email": "Ding.User@example.com",
					"mobile": "13900000000",
					"nick": "Ding Nick",
					"avatarUrl": "https://example.com/ding.webp"
				}`))
		case "/v1.0/oauth2/accessToken":
			appTokenCalled = true
			if r.Method != http.MethodPost {
				t.Fatalf("app token method = %q, want POST", r.Method)
			}
			if !strings.Contains(r.Header.Get("Content-Type"), "application/json") {
				t.Fatalf("app token content-type = %q, want json", r.Header.Get("Content-Type"))
			}
			var payload map[string]string
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode app token payload: %v", err)
			}
			if payload["appKey"] != "ding-client-id" {
				t.Fatalf("appKey = %q, want ding-client-id", payload["appKey"])
			}
			if payload["appSecret"] != "ding-client-secret" {
				t.Fatalf("appSecret = %q, want ding-client-secret", payload["appSecret"])
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"accessToken":"dingtalk-app-token","expireIn":7200}`))
		case "/user/getUseridByUnionid":
			useridByUnionIDCalled = true
			if r.Method != http.MethodGet {
				t.Fatalf("userid method = %q, want GET", r.Method)
			}
			if r.URL.Query().Get("access_token") != "dingtalk-app-token" {
				t.Fatalf("userid access_token = %q, want dingtalk-app-token", r.URL.Query().Get("access_token"))
			}
			if r.URL.Query().Get("unionid") != "ding-union-id" {
				t.Fatalf("unionid = %q, want ding-union-id", r.URL.Query().Get("unionid"))
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"errcode":0,"errmsg":"ok","userid":"ding-user-id"}`))
		case "/topapi/v2/user/get":
			userdetailCalled = true
			if r.Method != http.MethodPost {
				t.Fatalf("userdetail method = %q, want POST", r.Method)
			}
			if r.URL.Query().Get("access_token") != "dingtalk-app-token" {
				t.Fatalf("userdetail access_token = %q, want dingtalk-app-token", r.URL.Query().Get("access_token"))
			}
			var payload map[string]string
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode userdetail payload: %v", err)
			}
			if payload["userid"] != "ding-user-id" {
				t.Fatalf("userdetail userid = %q, want ding-user-id", payload["userid"])
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
					"errcode": 0,
					"errmsg": "ok",
					"result": {
						"userid": "ding-user-id",
						"unionid": "ding-union-id",
						"name": "Ding Real Name"
					}
				}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer dingTalkServer.Close()

	server, db := newTestRouter(t)
	defer server.Close()
	provider := insertTestThirdPartyLoginProvider(t, db, store.ThirdPartyLoginProvider{
		Name:         "钉钉",
		Key:          "dingtalk",
		Type:         store.ThirdPartyLoginProviderTypeDingTalk,
		Enabled:      true,
		ClientID:     "ding-client-id",
		ClientSecret: "ding-client-secret",
		Scopes:       json.RawMessage(`["openid"]`),
		Config: thirdPartyProviderConfig(t, map[string]any{
			"authorize_url":         dingTalkServer.URL + "/oauth2/auth",
			"token_url":             dingTalkServer.URL + "/v1.0/oauth2/userAccessToken",
			"userinfo_url":          dingTalkServer.URL + "/v1.0/contact/users/me",
			"app_token_url":         dingTalkServer.URL + "/v1.0/oauth2/accessToken",
			"userid_by_unionid_url": dingTalkServer.URL + "/user/getUseridByUnionid",
			"userdetail_url":        dingTalkServer.URL + "/topapi/v2/user/get",
			"external_id_field":     "unionId",
			"email_field":           "email",
			"phone_field":           "mobile",
			"name_field":            "nick",
			"nickname_field":        "nick",
			"avatar_field":          "avatarUrl",
		}),
	})
	noRedirectClient := server.Client()
	noRedirectClient.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}

	startResp := getResponseWithClient(t, noRedirectClient, server, "/api/client/auth/third-party/dingtalk/start?redirect=/init")
	if startResp.StatusCode != http.StatusFound {
		t.Fatalf("start status = %d, want 302", startResp.StatusCode)
	}
	parsedAuthorizeURL, err := url.Parse(startResp.Header.Get("Location"))
	if err != nil {
		t.Fatalf("parse authorize location: %v", err)
	}
	if parsedAuthorizeURL.Query().Get("client_id") != "ding-client-id" {
		t.Fatalf("client_id = %q, want ding-client-id", parsedAuthorizeURL.Query().Get("client_id"))
	}
	if parsedAuthorizeURL.Query().Get("scope") != "openid" {
		t.Fatalf("scope = %q, want openid", parsedAuthorizeURL.Query().Get("scope"))
	}
	if parsedAuthorizeURL.Query().Get("code_challenge") != "" {
		t.Fatalf("code_challenge = %q, want empty", parsedAuthorizeURL.Query().Get("code_challenge"))
	}
	state := parsedAuthorizeURL.Query().Get("state")
	if state == "" {
		t.Fatal("state is empty")
	}

	callbackResp := getResponseWithClient(
		t,
		noRedirectClient,
		server,
		"/api/client/auth/third-party/dingtalk/callback?authCode=callback-code&state="+url.QueryEscape(state),
		requireThirdPartyStateCookie(t, startResp),
	)
	if callbackResp.StatusCode != http.StatusFound {
		t.Fatalf("callback status = %d, want 302", callbackResp.StatusCode)
	}
	requireUserSessionCookie(t, callbackResp)
	if !tokenCalled {
		t.Fatal("dingtalk token endpoint was not called")
	}
	if !userinfoCalled {
		t.Fatal("dingtalk userinfo endpoint was not called")
	}
	if !appTokenCalled {
		t.Fatal("dingtalk app token endpoint was not called")
	}
	if !useridByUnionIDCalled {
		t.Fatal("dingtalk userid by unionid endpoint was not called")
	}
	if !userdetailCalled {
		t.Fatal("dingtalk userdetail endpoint was not called")
	}

	var account store.ThirdPartyAccount
	if err := db.First(&account, "provider_id = ? AND external_user_id = ?", provider.ID, "ding-union-id").Error; err != nil {
		t.Fatalf("find dingtalk account: %v", err)
	}
	var user store.User
	if err := db.First(&user, "id = ?", account.UserID).Error; err != nil {
		t.Fatalf("find dingtalk user: %v", err)
	}
	if user.Email != "ding.user@example.com" {
		t.Fatalf("user email = %q, want ding.user@example.com", user.Email)
	}
	if user.Name != "Ding Real Name" {
		t.Fatalf("user name = %q, want Ding Real Name", user.Name)
	}
	if user.Nickname != "Ding Nick" {
		t.Fatalf("user nickname = %q, want Ding Nick", user.Nickname)
	}
	if user.Phone == nil || *user.Phone != "+8613900000000" {
		t.Fatalf("user phone = %#v, want +8613900000000", user.Phone)
	}
	if user.Avatar != "https://example.com/ding.webp" {
		t.Fatalf("user avatar = %q, want dingtalk avatar", user.Avatar)
	}
}

func TestDingTalkLoginUpdatesExistingBoundUserName(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()
	now := time.Date(2026, 7, 7, 10, 0, 0, 0, time.UTC)
	provider := insertTestThirdPartyLoginProvider(t, db, store.ThirdPartyLoginProvider{
		Name:         "钉钉",
		Key:          "dingtalk",
		Type:         store.ThirdPartyLoginProviderTypeDingTalk,
		Enabled:      true,
		ClientID:     "ding-client-id",
		ClientSecret: "ding-client-secret",
		Scopes:       json.RawMessage(`["openid"]`),
		Config: thirdPartyProviderConfig(t, map[string]any{
			"authorize_url":     "https://login.example.com/oauth2/auth",
			"token_url":         "https://api.example.com/v1.0/oauth2/userAccessToken",
			"userinfo_url":      "https://api.example.com/v1.0/contact/users/me",
			"external_id_field": "unionId",
			"email_field":       "email",
			"phone_field":       "mobile",
			"name_field":        "nick",
			"nickname_field":    "nick",
			"avatar_field":      "avatarUrl",
		}),
	})
	user := insertTestUser(t, db, "ding.user@example.com", "Ding Nick", store.UserStatusActive, now)
	if err := db.Model(&user).Update("nickname", "Ding Nick").Error; err != nil {
		t.Fatalf("set existing nickname: %v", err)
	}
	account := store.ThirdPartyAccount{
		ID:             uuid.NewString(),
		ProviderID:     provider.ID,
		ExternalUserID: "ding-union-id",
		UserID:         user.ID,
		Profile:        json.RawMessage(`{"unionId":"ding-union-id","nick":"Ding Nick"}`),
	}
	if err := db.Create(&account).Error; err != nil {
		t.Fatalf("create existing dingtalk account: %v", err)
	}

	resolvedUser, err := (&Server{db: db}).findOrCreateThirdPartyUser(provider, externalUserProfile{
		ExternalUserID: "ding-union-id",
		Email:          "Ding.User@example.com",
		Name:           "Ding Real Name",
		Nickname:       "Ding Nick",
		Raw:            json.RawMessage(`{"unionId":"ding-union-id","nick":"Ding Nick","name":"Ding Real Name"}`),
	})
	if err != nil {
		t.Fatalf("find or create dingtalk user: %v", err)
	}
	if resolvedUser.ID != user.ID {
		t.Fatalf("resolved user id = %q, want %q", resolvedUser.ID, user.ID)
	}
	if resolvedUser.Name != "Ding Real Name" {
		t.Fatalf("resolved user name = %q, want Ding Real Name", resolvedUser.Name)
	}

	var storedUser store.User
	if err := db.First(&storedUser, "id = ?", user.ID).Error; err != nil {
		t.Fatalf("find stored user: %v", err)
	}
	if storedUser.Name != "Ding Real Name" {
		t.Fatalf("stored user name = %q, want Ding Real Name", storedUser.Name)
	}
	if storedUser.Nickname != "Ding Nick" {
		t.Fatalf("stored user nickname = %q, want Ding Nick", storedUser.Nickname)
	}
}

func TestFeishuLoginExchangesCodeWithJSONBody(t *testing.T) {
	var tokenCalled bool
	var userinfoCalled bool
	feishuServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth/v3/token":
			tokenCalled = true
			if r.Method != http.MethodPost {
				t.Fatalf("token method = %q, want POST", r.Method)
			}
			if !strings.Contains(r.Header.Get("Content-Type"), "application/json") {
				t.Fatalf("token content-type = %q, want json", r.Header.Get("Content-Type"))
			}
			var payload map[string]string
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode token payload: %v", err)
			}
			if payload["grant_type"] != "authorization_code" {
				t.Fatalf("grant_type = %q, want authorization_code", payload["grant_type"])
			}
			if payload["client_id"] != "feishu-client-id" {
				t.Fatalf("client_id = %q, want feishu-client-id", payload["client_id"])
			}
			if payload["client_secret"] != "feishu-client-secret" {
				t.Fatalf("client_secret = %q, want feishu-client-secret", payload["client_secret"])
			}
			if payload["code"] != "callback-code" {
				t.Fatalf("code = %q, want callback-code", payload["code"])
			}
			if payload["redirect_uri"] == "" {
				t.Fatal("redirect_uri is empty")
			}
			if payload["code_verifier"] == "" {
				t.Fatal("code_verifier is empty")
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"feishu-user-token","token_type":"Bearer"}`))
		case "/open-apis/authen/v1/user_info":
			userinfoCalled = true
			if r.Header.Get("Authorization") != "Bearer feishu-user-token" {
				t.Fatalf("Authorization = %q, want Bearer feishu-user-token", r.Header.Get("Authorization"))
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"code": 0,
				"msg": "success",
				"data": {
					"union_id": "feishu-union-id",
					"open_id": "feishu-open-id",
					"email": "Feishu.User@example.com",
					"mobile": "+8613800000000",
					"name": "Feishu User",
					"en_name": "Feishu",
					"avatar_url": "https://example.com/feishu.webp"
				}
			}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer feishuServer.Close()

	server, db := newTestRouter(t)
	defer server.Close()
	provider := insertTestThirdPartyLoginProvider(t, db, store.ThirdPartyLoginProvider{
		Name:         "飞书",
		Key:          "feishu",
		Type:         store.ThirdPartyLoginProviderTypeFeishu,
		Enabled:      true,
		ClientID:     "feishu-client-id",
		ClientSecret: "feishu-client-secret",
		Scopes:       json.RawMessage(`["contact:user.base:readonly"]`),
		Config: thirdPartyProviderConfig(t, map[string]any{
			"authorize_url":     feishuServer.URL + "/open-apis/authen/v1/authorize",
			"token_url":         feishuServer.URL + "/oauth/v3/token",
			"userinfo_url":      feishuServer.URL + "/open-apis/authen/v1/user_info",
			"external_id_field": "union_id",
			"email_field":       "email",
			"phone_field":       "mobile",
			"name_field":        "name",
			"nickname_field":    "en_name",
			"avatar_field":      "avatar_url",
		}),
	})
	noRedirectClient := server.Client()
	noRedirectClient.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}

	startResp := getResponseWithClient(t, noRedirectClient, server, "/api/client/auth/third-party/feishu/start?redirect=/init")
	if startResp.StatusCode != http.StatusFound {
		t.Fatalf("start status = %d, want 302", startResp.StatusCode)
	}
	parsedAuthorizeURL, err := url.Parse(startResp.Header.Get("Location"))
	if err != nil {
		t.Fatalf("parse authorize location: %v", err)
	}
	if parsedAuthorizeURL.Query().Get("client_id") != "feishu-client-id" {
		t.Fatalf("client_id = %q, want feishu-client-id", parsedAuthorizeURL.Query().Get("client_id"))
	}
	if parsedAuthorizeURL.Query().Get("code_challenge") == "" {
		t.Fatal("code_challenge is empty")
	}
	state := parsedAuthorizeURL.Query().Get("state")
	if state == "" {
		t.Fatal("state is empty")
	}

	callbackResp := getResponseWithClient(
		t,
		noRedirectClient,
		server,
		"/api/client/auth/third-party/feishu/callback?code=callback-code&state="+url.QueryEscape(state),
		requireThirdPartyStateCookie(t, startResp),
	)
	if callbackResp.StatusCode != http.StatusFound {
		t.Fatalf("callback status = %d, want 302", callbackResp.StatusCode)
	}
	requireUserSessionCookie(t, callbackResp)
	if !tokenCalled {
		t.Fatal("feishu token endpoint was not called")
	}
	if !userinfoCalled {
		t.Fatal("feishu userinfo endpoint was not called")
	}

	var account store.ThirdPartyAccount
	if err := db.First(&account, "provider_id = ? AND external_user_id = ?", provider.ID, "feishu-union-id").Error; err != nil {
		t.Fatalf("find feishu account: %v", err)
	}
	var user store.User
	if err := db.First(&user, "id = ?", account.UserID).Error; err != nil {
		t.Fatalf("find feishu user: %v", err)
	}
	if user.Email != "feishu.user@example.com" {
		t.Fatalf("user email = %q, want feishu.user@example.com", user.Email)
	}
	if user.Name != "Feishu User" {
		t.Fatalf("user name = %q, want Feishu User", user.Name)
	}
	if user.Nickname != "Feishu" {
		t.Fatalf("user nickname = %q, want Feishu", user.Nickname)
	}
	if user.Phone == nil || *user.Phone != "+8613800000000" {
		t.Fatalf("user phone = %#v, want +8613800000000", user.Phone)
	}
	if user.Avatar != "https://example.com/feishu.webp" {
		t.Fatalf("user avatar = %q, want feishu avatar", user.Avatar)
	}
}

func TestGitHubLoginFetchesPrimaryVerifiedEmailWhenUserEmailIsMissing(t *testing.T) {
	var userinfoCalled bool
	var emailsCalled bool
	gitHubServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/login/oauth/access_token":
			if err := r.ParseForm(); err != nil {
				t.Fatalf("parse token form: %v", err)
			}
			if r.Form.Get("client_id") != "github-client-id" {
				t.Fatalf("client_id = %q, want github-client-id", r.Form.Get("client_id"))
			}
			if r.Form.Get("client_secret") != "github-client-secret" {
				t.Fatalf("client_secret = %q, want github-client-secret", r.Form.Get("client_secret"))
			}
			if r.Form.Get("code") != "callback-code" {
				t.Fatalf("code = %q, want callback-code", r.Form.Get("code"))
			}
			if r.Form.Get("code_verifier") != "" {
				t.Fatalf("code_verifier = %q, want empty", r.Form.Get("code_verifier"))
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"github-token","token_type":"bearer"}`))
		case "/user":
			userinfoCalled = true
			if r.Header.Get("Authorization") != "Bearer github-token" {
				t.Fatalf("Authorization = %q, want Bearer github-token", r.Header.Get("Authorization"))
			}
			if r.Header.Get("X-GitHub-Api-Version") != "2022-11-28" {
				t.Fatalf("X-GitHub-Api-Version = %q, want 2022-11-28", r.Header.Get("X-GitHub-Api-Version"))
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"id": 42,
				"login": "octocat",
				"name": "Octo Cat",
				"avatar_url": "https://example.com/octocat.webp"
			}`))
		case "/user/emails":
			emailsCalled = true
			if r.Header.Get("Authorization") != "Bearer github-token" {
				t.Fatalf("Authorization = %q, want Bearer github-token", r.Header.Get("Authorization"))
			}
			if r.Header.Get("X-GitHub-Api-Version") != "2022-11-28" {
				t.Fatalf("X-GitHub-Api-Version = %q, want 2022-11-28", r.Header.Get("X-GitHub-Api-Version"))
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[
				{"email":"secondary@example.com","primary":false,"verified":true},
				{"email":"octocat@example.com","primary":true,"verified":true}
			]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer gitHubServer.Close()

	server, db := newTestRouter(t)
	defer server.Close()
	provider := insertTestThirdPartyLoginProvider(t, db, store.ThirdPartyLoginProvider{
		Name:         "GitHub",
		Key:          "github",
		Type:         store.ThirdPartyLoginProviderTypeGitHub,
		Enabled:      true,
		ClientID:     "github-client-id",
		ClientSecret: "github-client-secret",
		Scopes:       json.RawMessage(`["read:user","user:email"]`),
		Config: thirdPartyProviderConfig(t, map[string]any{
			"authorize_url":     gitHubServer.URL + "/login/oauth/authorize",
			"token_url":         gitHubServer.URL + "/login/oauth/access_token",
			"userinfo_url":      gitHubServer.URL + "/user",
			"emails_url":        gitHubServer.URL + "/user/emails",
			"external_id_field": "id",
			"email_field":       "email",
			"name_field":        "name",
			"nickname_field":    "login",
			"avatar_field":      "avatar_url",
		}),
	})
	noRedirectClient := server.Client()
	noRedirectClient.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}

	startResp := getResponseWithClient(t, noRedirectClient, server, "/api/client/auth/third-party/github/start?redirect=/init")
	if startResp.StatusCode != http.StatusFound {
		t.Fatalf("start status = %d, want 302", startResp.StatusCode)
	}
	parsedAuthorizeURL, err := url.Parse(startResp.Header.Get("Location"))
	if err != nil {
		t.Fatalf("parse authorize location: %v", err)
	}
	if parsedAuthorizeURL.Query().Get("client_id") != "github-client-id" {
		t.Fatalf("client_id = %q, want github-client-id", parsedAuthorizeURL.Query().Get("client_id"))
	}
	if parsedAuthorizeURL.Query().Get("scope") != "read:user user:email" {
		t.Fatalf("scope = %q, want read:user user:email", parsedAuthorizeURL.Query().Get("scope"))
	}
	if parsedAuthorizeURL.Query().Get("code_challenge") != "" {
		t.Fatalf("code_challenge = %q, want empty", parsedAuthorizeURL.Query().Get("code_challenge"))
	}
	state := parsedAuthorizeURL.Query().Get("state")
	if state == "" {
		t.Fatal("state is empty")
	}

	callbackResp := getResponseWithClient(
		t,
		noRedirectClient,
		server,
		"/api/client/auth/third-party/github/callback?code=callback-code&state="+url.QueryEscape(state),
		requireThirdPartyStateCookie(t, startResp),
	)
	if callbackResp.StatusCode != http.StatusFound {
		t.Fatalf("callback status = %d, want 302", callbackResp.StatusCode)
	}
	requireUserSessionCookie(t, callbackResp)
	if !userinfoCalled {
		t.Fatal("github user endpoint was not called")
	}
	if !emailsCalled {
		t.Fatal("github emails endpoint was not called")
	}

	var account store.ThirdPartyAccount
	if err := db.First(&account, "provider_id = ? AND external_user_id = ?", provider.ID, "42").Error; err != nil {
		t.Fatalf("find github account: %v", err)
	}
	var user store.User
	if err := db.First(&user, "id = ?", account.UserID).Error; err != nil {
		t.Fatalf("find github user: %v", err)
	}
	if user.Email != "octocat@example.com" {
		t.Fatalf("user email = %q, want octocat@example.com", user.Email)
	}
	if user.Name != "Octo Cat" {
		t.Fatalf("user name = %q, want Octo Cat", user.Name)
	}
}

func TestWeComLoginUsesEnterpriseAccessTokenQueryForUserInfo(t *testing.T) {
	var tokenCalled bool
	var userinfoCalled bool
	var userdetailCalled bool
	wecomServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/cgi-bin/gettoken":
			tokenCalled = true
			if r.URL.Query().Get("corpid") != "corp-id" {
				t.Fatalf("corpid = %q, want corp-id", r.URL.Query().Get("corpid"))
			}
			if r.URL.Query().Get("corpsecret") != "corp-secret" {
				t.Fatalf("corpsecret = %q, want corp-secret", r.URL.Query().Get("corpsecret"))
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"errcode":0,"errmsg":"ok","access_token":"enterprise-token","expires_in":7200}`))
		case "/cgi-bin/auth/getuserinfo":
			userinfoCalled = true
			if r.Header.Get("Authorization") != "" {
				t.Fatalf("Authorization = %q, want empty", r.Header.Get("Authorization"))
			}
			if r.URL.Query().Get("access_token") != "enterprise-token" {
				t.Fatalf("access_token = %q, want enterprise-token", r.URL.Query().Get("access_token"))
			}
			if r.URL.Query().Get("code") != "callback-code" {
				t.Fatalf("code = %q, want callback-code", r.URL.Query().Get("code"))
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"errcode":0,"errmsg":"ok","userid":"zhangsan","user_ticket":"user-ticket"}`))
		case "/cgi-bin/auth/getuserdetail":
			userdetailCalled = true
			if r.Method != http.MethodPost {
				t.Fatalf("userdetail method = %q, want POST", r.Method)
			}
			if r.URL.Query().Get("access_token") != "enterprise-token" {
				t.Fatalf("access_token = %q, want enterprise-token", r.URL.Query().Get("access_token"))
			}
			if !strings.Contains(r.Header.Get("Content-Type"), "application/json") {
				t.Fatalf("userdetail content-type = %q, want json", r.Header.Get("Content-Type"))
			}
			var payload map[string]string
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode userdetail payload: %v", err)
			}
			if payload["user_ticket"] != "user-ticket" {
				t.Fatalf("user_ticket = %q, want user-ticket", payload["user_ticket"])
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"errcode": 0,
				"errmsg": "ok",
				"userid": "zhangsan",
				"name": "张三",
				"mobile": "13800000000",
				"email": "zhangsan@example.com",
				"avatar": "https://example.com/wecom.webp"
			}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer wecomServer.Close()

	server, db := newTestRouter(t)
	defer server.Close()
	provider := insertTestThirdPartyLoginProvider(t, db, store.ThirdPartyLoginProvider{
		Name:         "企业微信",
		Key:          "wecom",
		Type:         store.ThirdPartyLoginProviderTypeWeCom,
		Enabled:      true,
		ClientID:     "corp-id",
		ClientSecret: "corp-secret",
		Scopes:       json.RawMessage(`["snsapi_base"]`),
		Config: thirdPartyProviderConfig(t, map[string]any{
			"agent_id":       "agent-id",
			"authorize_url":  wecomServer.URL + "/wwlogin/sso/login",
			"token_url":      wecomServer.URL + "/cgi-bin/gettoken",
			"userinfo_url":   wecomServer.URL + "/cgi-bin/auth/getuserinfo",
			"userdetail_url": wecomServer.URL + "/cgi-bin/auth/getuserdetail",
		}),
	})
	noRedirectClient := server.Client()
	noRedirectClient.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}

	startResp := getResponseWithClient(t, noRedirectClient, server, "/api/client/auth/third-party/wecom/start?redirect=/init")
	if startResp.StatusCode != http.StatusFound {
		t.Fatalf("start status = %d, want 302", startResp.StatusCode)
	}
	parsedAuthorizeURL, err := url.Parse(startResp.Header.Get("Location"))
	if err != nil {
		t.Fatalf("parse authorize location: %v", err)
	}
	if parsedAuthorizeURL.Path != "/wwlogin/sso/login" {
		t.Fatalf("authorize path = %q, want /wwlogin/sso/login", parsedAuthorizeURL.Path)
	}
	if parsedAuthorizeURL.Query().Get("login_type") != "CorpApp" {
		t.Fatalf("login_type = %q, want CorpApp", parsedAuthorizeURL.Query().Get("login_type"))
	}
	if parsedAuthorizeURL.Query().Get("appid") != "corp-id" {
		t.Fatalf("appid = %q, want corp-id", parsedAuthorizeURL.Query().Get("appid"))
	}
	if parsedAuthorizeURL.Query().Get("agentid") != "agent-id" {
		t.Fatalf("agentid = %q, want agent-id", parsedAuthorizeURL.Query().Get("agentid"))
	}
	if parsedAuthorizeURL.Query().Get("scope") != "" {
		t.Fatalf("scope = %q, want empty", parsedAuthorizeURL.Query().Get("scope"))
	}
	if parsedAuthorizeURL.Fragment != "" {
		t.Fatalf("fragment = %q, want empty", parsedAuthorizeURL.Fragment)
	}
	state := parsedAuthorizeURL.Query().Get("state")
	if state == "" {
		t.Fatal("state is empty")
	}

	callbackResp := getResponseWithClient(
		t,
		noRedirectClient,
		server,
		"/api/client/auth/third-party/wecom/callback?code=callback-code&state="+url.QueryEscape(state),
		requireThirdPartyStateCookie(t, startResp),
	)
	if callbackResp.StatusCode != http.StatusFound {
		t.Fatalf("callback status = %d, want 302", callbackResp.StatusCode)
	}
	requireUserSessionCookie(t, callbackResp)
	if !tokenCalled {
		t.Fatal("enterprise token endpoint was not called")
	}
	if !userinfoCalled {
		t.Fatal("userinfo endpoint was not called")
	}
	if !userdetailCalled {
		t.Fatal("userdetail endpoint was not called")
	}

	var account store.ThirdPartyAccount
	if err := db.First(&account, "provider_id = ? AND external_user_id = ?", provider.ID, "zhangsan").Error; err != nil {
		t.Fatalf("find wecom account: %v", err)
	}
	var user store.User
	if err := db.First(&user, "id = ?", account.UserID).Error; err != nil {
		t.Fatalf("find wecom user: %v", err)
	}
	if user.Email != "zhangsan@example.com" {
		t.Fatalf("user email = %q, want zhangsan@example.com", user.Email)
	}
	if user.Name != "张三" {
		t.Fatalf("user name = %q, want 张三", user.Name)
	}
	if user.Phone == nil || *user.Phone != "+8613800000000" {
		t.Fatalf("user phone = %#v, want +8613800000000", user.Phone)
	}
	if user.Avatar != "https://example.com/wecom.webp" {
		t.Fatalf("user avatar = %q, want wecom avatar", user.Avatar)
	}
}

func TestThirdPartyCallbackRequiresStateCookie(t *testing.T) {
	thirdPartyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"access-token","token_type":"Bearer"}`))
		case "/userinfo":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"email":"mallory@example.com","name":"Mallory"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer thirdPartyServer.Close()

	server, db := newTestRouter(t)
	defer server.Close()
	insertTestThirdPartyLoginProvider(t, db, store.ThirdPartyLoginProvider{
		Name:         "Enterprise SSO",
		Key:          "enterprise",
		Type:         store.ThirdPartyLoginProviderTypeOIDC,
		Enabled:      true,
		ClientID:     "client-id",
		ClientSecret: "client-secret",
		Scopes:       json.RawMessage(`["openid","email","profile"]`),
		Config: thirdPartyProviderConfig(t, map[string]any{
			"authorize_url":     thirdPartyServer.URL + "/authorize",
			"token_url":         thirdPartyServer.URL + "/token",
			"userinfo_url":      thirdPartyServer.URL + "/userinfo",
			"external_id_field": "sub",
			"email_field":       "email",
			"name_field":        "name",
		}),
	})
	noRedirectClient := server.Client()
	noRedirectClient.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}

	startResp := getResponseWithClient(t, noRedirectClient, server, "/api/client/auth/third-party/enterprise/start?redirect=/init")
	authorizeLocation := startResp.Header.Get("Location")
	parsedAuthorizeURL, err := url.Parse(authorizeLocation)
	if err != nil {
		t.Fatalf("parse authorize location: %v", err)
	}
	state := parsedAuthorizeURL.Query().Get("state")
	if state == "" {
		t.Fatal("state is empty")
	}
	requireThirdPartyStateCookie(t, startResp)

	callbackResp := getResponseWithClient(t, noRedirectClient, server, "/api/client/auth/third-party/enterprise/callback?code=callback-code&state="+url.QueryEscape(state))
	if callbackResp.StatusCode != http.StatusBadRequest {
		t.Fatalf("callback status = %d, want 400", callbackResp.StatusCode)
	}

	var userCount int64
	if err := db.Model(&store.User{}).Where("email = ?", "mallory@example.com").Count(&userCount).Error; err != nil {
		t.Fatalf("count third-party user: %v", err)
	}
	if userCount != 0 {
		t.Fatalf("third-party user count = %d, want 0", userCount)
	}
}

func TestThirdPartyStartCleansExpiredLoginStates(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()
	provider := insertTestThirdPartyLoginProvider(t, db, store.ThirdPartyLoginProvider{
		Name:         "Enterprise SSO",
		Key:          "enterprise",
		Type:         store.ThirdPartyLoginProviderTypeOIDC,
		Enabled:      true,
		ClientID:     "client-id",
		ClientSecret: "client-secret",
		Scopes:       json.RawMessage(`["openid","email","profile"]`),
		Config: thirdPartyProviderConfig(t, map[string]any{
			"authorize_url":     "https://sso.example.com/authorize",
			"token_url":         "https://sso.example.com/token",
			"userinfo_url":      "https://sso.example.com/userinfo",
			"external_id_field": "sub",
			"email_field":       "email",
			"name_field":        "name",
		}),
	})
	now := time.Now().UTC()
	expiredState := store.ThirdPartyLoginState{
		StateHash:    auth.HashSessionToken("expired-state"),
		ProviderID:   provider.ID,
		CodeVerifier: "expired-verifier",
		RedirectPath: "/init",
		ExpiresAt:    now.Add(-time.Minute),
		IP:           "127.0.0.1",
		UserAgent:    "test",
	}
	if err := db.Create(&expiredState).Error; err != nil {
		t.Fatalf("create expired third-party state: %v", err)
	}
	noRedirectClient := server.Client()
	noRedirectClient.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}

	resp := getResponseWithClient(t, noRedirectClient, server, "/api/client/auth/third-party/enterprise/start?redirect=/init")
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("start status = %d, want 302", resp.StatusCode)
	}

	var expiredCount int64
	if err := db.Model(&store.ThirdPartyLoginState{}).Where("state_hash = ?", expiredState.StateHash).Count(&expiredCount).Error; err != nil {
		t.Fatalf("count expired third-party states: %v", err)
	}
	if expiredCount != 0 {
		t.Fatalf("expired third-party state count = %d, want 0", expiredCount)
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

func TestAdminCanManageApps(t *testing.T) {
	server, _ := newTestRouter(t)
	defer server.Close()
	adminCookie := loginAsAdmin(t, server)

	listResp, listBody := getJSON(t, server, "/api/admin/apps", adminCookie)
	if listResp.StatusCode != http.StatusOK {
		t.Fatalf("list status = %d, want 200", listResp.StatusCode)
	}
	apps := requireSuccess(t, listBody)["apps"].([]any)
	if len(apps) != 1 {
		t.Fatalf("app count = %d, want goddess app", len(apps))
	}
	goddessApp := apps[0].(map[string]any)
	if goddessApp["id"] != appregistry.GoddessAppID {
		t.Fatalf("goddess id = %v, want %s", goddessApp["id"], appregistry.GoddessAppID)
	}
	if goddessApp["name"] != appregistry.GoddessDefaultName {
		t.Fatalf("goddess name = %v, want %s", goddessApp["name"], appregistry.GoddessDefaultName)
	}
	if goddessApp["callback_secret"] != "test-goddess-secret" {
		t.Fatalf("goddess secret = %v, want configured secret", goddessApp["callback_secret"])
	}
	if goddessApp["system"] != true {
		t.Fatalf("goddess system = %v, want true", goddessApp["system"])
	}
	if goddessApp["visibility"] != store.AppVisibilityPublic {
		t.Fatalf("goddess visibility = %v, want public", goddessApp["visibility"])
	}

	updateGoddessResp, updateGoddessBody := putJSON(t, server, "/api/admin/apps/"+appregistry.GoddessAppID, map[string]any{
		"name":         "女菩萨 Pro",
		"avatar":       "/assets/apps/goddess.webp",
		"description":  "AI Agent",
		"callback_url": "https://agent.example.com/webhook",
		"visibility":   "public",
	}, adminCookie)
	if updateGoddessResp.StatusCode != http.StatusOK {
		t.Fatalf("update goddess status = %d, want 200, body = %#v", updateGoddessResp.StatusCode, updateGoddessBody)
	}
	updatedGoddess := requireSuccess(t, updateGoddessBody)["app"].(map[string]any)
	if updatedGoddess["name"] != "女菩萨 Pro" {
		t.Fatalf("updated goddess name = %v", updatedGoddess["name"])
	}
	if updatedGoddess["callback_url"] != "https://agent.example.com/webhook" {
		t.Fatalf("updated goddess callback_url = %v", updatedGoddess["callback_url"])
	}
	if updatedGoddess["callback_secret"] != "test-goddess-secret" {
		t.Fatalf("updated goddess secret = %v, want configured secret", updatedGoddess["callback_secret"])
	}

	regenerateGoddessResp, _ := postJSON(t, server, "/api/admin/apps/"+appregistry.GoddessAppID+"/secret/regenerate", map[string]any{}, adminCookie)
	if regenerateGoddessResp.StatusCode != http.StatusForbidden {
		t.Fatalf("regenerate goddess status = %d, want 403", regenerateGoddessResp.StatusCode)
	}
	deleteGoddessResp, _ := requestJSON(t, server, http.MethodDelete, "/api/admin/apps/"+appregistry.GoddessAppID, map[string]any{}, adminCookie)
	if deleteGoddessResp.StatusCode != http.StatusForbidden {
		t.Fatalf("delete goddess status = %d, want 403", deleteGoddessResp.StatusCode)
	}

	createResp, createBody := postJSON(t, server, "/api/admin/apps", map[string]any{
		"name":         "知识库助手",
		"avatar":       "/assets/apps/kb.webp",
		"description":  "回答知识库问题",
		"visibility":   "public",
		"callback_url": "https://kb.example.com/webhook",
	}, adminCookie)
	if createResp.StatusCode != http.StatusCreated {
		t.Fatalf("create status = %d, want 201, body = %#v", createResp.StatusCode, createBody)
	}
	createdApp := requireSuccess(t, createBody)["app"].(map[string]any)
	appID := createdApp["id"].(string)
	firstSecret := createdApp["callback_secret"].(string)
	if appID == "" {
		t.Fatal("created app id is empty")
	}
	if firstSecret == "" || firstSecret == "test-goddess-secret" {
		t.Fatalf("created app secret = %q, want generated unique secret", firstSecret)
	}
	if createdApp["system"] != false {
		t.Fatalf("created app system = %v, want false", createdApp["system"])
	}
	if createdApp["creator_user_id"] != nil {
		t.Fatalf("created app creator_user_id = %v, want nil", createdApp["creator_user_id"])
	}

	updateResp, updateBody := putJSON(t, server, "/api/admin/apps/"+appID, map[string]any{
		"name":         "知识库 Agent",
		"avatar":       "",
		"description":  "更新后的介绍",
		"visibility":   "public",
		"callback_url": "",
	}, adminCookie)
	if updateResp.StatusCode != http.StatusOK {
		t.Fatalf("update status = %d, want 200, body = %#v", updateResp.StatusCode, updateBody)
	}
	updatedApp := requireSuccess(t, updateBody)["app"].(map[string]any)
	if updatedApp["name"] != "知识库 Agent" {
		t.Fatalf("updated app name = %v", updatedApp["name"])
	}
	if updatedApp["callback_url"] != "" {
		t.Fatalf("updated callback_url = %v, want empty", updatedApp["callback_url"])
	}

	disableResp, disableBody := postJSON(t, server, "/api/admin/apps/"+appID+"/disable", map[string]any{}, adminCookie)
	if disableResp.StatusCode != http.StatusOK {
		t.Fatalf("disable status = %d, want 200, body = %#v", disableResp.StatusCode, disableBody)
	}
	disabledApp := requireSuccess(t, disableBody)["app"].(map[string]any)
	if disabledApp["enabled"] != false {
		t.Fatalf("disabled enabled = %v, want false", disabledApp["enabled"])
	}

	enableResp, enableBody := postJSON(t, server, "/api/admin/apps/"+appID+"/enable", map[string]any{}, adminCookie)
	if enableResp.StatusCode != http.StatusOK {
		t.Fatalf("enable status = %d, want 200, body = %#v", enableResp.StatusCode, enableBody)
	}
	enabledApp := requireSuccess(t, enableBody)["app"].(map[string]any)
	if enabledApp["enabled"] != true {
		t.Fatalf("enabled enabled = %v, want true", enabledApp["enabled"])
	}

	regenerateResp, regenerateBody := postJSON(t, server, "/api/admin/apps/"+appID+"/secret/regenerate", map[string]any{}, adminCookie)
	if regenerateResp.StatusCode != http.StatusOK {
		t.Fatalf("regenerate status = %d, want 200, body = %#v", regenerateResp.StatusCode, regenerateBody)
	}
	regeneratedApp := requireSuccess(t, regenerateBody)["app"].(map[string]any)
	secondSecret := regeneratedApp["callback_secret"].(string)
	if secondSecret == "" || secondSecret == firstSecret {
		t.Fatalf("regenerated secret = %q, want changed from %q", secondSecret, firstSecret)
	}

	deleteResp, deleteBody := requestJSON(t, server, http.MethodDelete, "/api/admin/apps/"+appID, map[string]any{}, adminCookie)
	if deleteResp.StatusCode != http.StatusOK {
		t.Fatalf("delete status = %d, want 200, body = %#v", deleteResp.StatusCode, deleteBody)
	}
	requireSuccess(t, deleteBody)
}

func TestAdminCanManageThirdPartyLoginProviders(t *testing.T) {
	server, _ := newTestRouter(t)
	defer server.Close()
	adminCookie := loginAsAdmin(t, server)

	createResp, createBody := postJSON(t, server, "/api/admin/third-party/providers", map[string]any{
		"name":          "企业 SSO",
		"type":          "oidc",
		"client_id":     "client-id",
		"client_secret": "client-secret",
		"scopes":        []any{"email", "profile"},
		"config": map[string]any{
			"authorize_url":     "https://sso.example.com/oauth/authorize",
			"token_url":         "https://sso.example.com/oauth/token",
			"userinfo_url":      "https://sso.example.com/oauth/userinfo",
			"external_id_field": "sub",
			"email_field":       "email",
			"phone_field":       "mobile",
			"name_field":        "real_name",
			"nickname_field":    "nickname",
			"avatar_field":      "avatar_url",
		},
	}, adminCookie)
	if createResp.StatusCode != http.StatusCreated {
		t.Fatalf("create status = %d, want 201, body = %#v", createResp.StatusCode, createBody)
	}
	createdProvider := requireSuccess(t, createBody)["provider"].(map[string]any)
	providerID, ok := createdProvider["id"].(string)
	if !ok || providerID == "" {
		t.Fatalf("created provider id = %#v, want string", createdProvider["id"])
	}
	if createdProvider["client_secret"] != "client-secret" {
		t.Fatalf("created client_secret = %#v, want client-secret", createdProvider["client_secret"])
	}
	if createdProvider["callback_url"] != "https://client.example.test/api/client/auth/third-party/sso/callback" {
		t.Fatalf("created callback_url = %#v, want client callback url", createdProvider["callback_url"])
	}
	if createdProvider["type"] != "oidc" {
		t.Fatalf("created type = %#v, want oidc", createdProvider["type"])
	}
	createdConfig := createdProvider["config"].(map[string]any)
	if createdConfig["phone_field"] != "mobile" {
		t.Fatalf("created config.phone_field = %#v, want mobile", createdConfig["phone_field"])
	}
	if createdProvider["key"] != "sso" {
		t.Fatalf("created key = %#v, want generated sso", createdProvider["key"])
	}
	if createdProvider["enabled"] != true {
		t.Fatalf("created enabled = %#v, want true", createdProvider["enabled"])
	}
	if createdProvider["sort_order"] != float64(10) {
		t.Fatalf("created sort_order = %#v, want 10", createdProvider["sort_order"])
	}
	requireStringSliceField(t, createdProvider, "scopes", []string{"email", "profile"})

	listResp, listBody := getJSON(t, server, "/api/admin/third-party/providers", adminCookie)
	if listResp.StatusCode != http.StatusOK {
		t.Fatalf("list status = %d, want 200", listResp.StatusCode)
	}
	providers := requireThirdPartyLoginProviders(t, requireSuccess(t, listBody))
	if len(providers) != 1 {
		t.Fatalf("provider count = %d, want 1", len(providers))
	}
	listedProvider := providers[0].(map[string]any)
	if listedProvider["client_secret"] != "client-secret" {
		t.Fatalf("listed client_secret = %#v, want client-secret", listedProvider["client_secret"])
	}
	if listedProvider["callback_url"] != "https://client.example.test/api/client/auth/third-party/sso/callback" {
		t.Fatalf("listed callback_url = %#v, want client callback url", listedProvider["callback_url"])
	}

	updateResp, updateBody := putJSON(t, server, "/api/admin/third-party/providers/"+providerID, map[string]any{
		"name":          "企业统一身份",
		"type":          "oidc",
		"client_id":     "updated-client-id",
		"client_secret": "updated-secret",
		"scopes":        []any{"email"},
		"config": map[string]any{
			"authorize_url":     "https://idp.example.com/oauth/authorize",
			"token_url":         "https://idp.example.com/oauth/token",
			"userinfo_url":      "https://idp.example.com/oauth/userinfo",
			"external_id_field": "sub",
			"email_field":       "mail",
			"phone_field":       "phone",
			"name_field":        "name",
			"nickname_field":    "nick",
			"avatar_field":      "picture",
		},
	}, adminCookie)
	if updateResp.StatusCode != http.StatusOK {
		t.Fatalf("update status = %d, want 200, body = %#v", updateResp.StatusCode, updateBody)
	}
	updatedProvider := requireSuccess(t, updateBody)["provider"].(map[string]any)
	if updatedProvider["key"] != createdProvider["key"] {
		t.Fatalf("updated key = %#v, want preserved %v", updatedProvider["key"], createdProvider["key"])
	}
	if updatedProvider["client_secret"] != "updated-secret" {
		t.Fatalf("updated client_secret = %#v, want updated-secret", updatedProvider["client_secret"])
	}
	if updatedProvider["enabled"] != true {
		t.Fatalf("updated enabled = %#v, want preserved true", updatedProvider["enabled"])
	}
	if updatedProvider["sort_order"] != float64(10) {
		t.Fatalf("updated sort_order = %#v, want preserved 10", updatedProvider["sort_order"])
	}

	disableResp, disableBody := postJSON(t, server, "/api/admin/third-party/providers/"+providerID+"/disable", map[string]any{}, adminCookie)
	if disableResp.StatusCode != http.StatusOK {
		t.Fatalf("disable status = %d, want 200, body = %#v", disableResp.StatusCode, disableBody)
	}
	disabledProvider := requireSuccess(t, disableBody)["provider"].(map[string]any)
	if disabledProvider["enabled"] != false {
		t.Fatalf("disabled enabled = %#v, want false", disabledProvider["enabled"])
	}

	deleteResp, deleteBody := requestJSON(t, server, http.MethodDelete, "/api/admin/third-party/providers/"+providerID, map[string]any{}, adminCookie)
	if deleteResp.StatusCode != http.StatusOK {
		t.Fatalf("delete status = %d, want 200, body = %#v", deleteResp.StatusCode, deleteBody)
	}
	requireSuccess(t, deleteBody)

	finalListResp, finalListBody := getJSON(t, server, "/api/admin/third-party/providers", adminCookie)
	if finalListResp.StatusCode != http.StatusOK {
		t.Fatalf("final list status = %d, want 200", finalListResp.StatusCode)
	}
	finalProviders := requireThirdPartyLoginProviders(t, requireSuccess(t, finalListBody))
	if len(finalProviders) != 0 {
		t.Fatalf("final provider count = %d, want 0", len(finalProviders))
	}
}

func TestDingTalkProviderStartsWithConsentPromptByDefault(t *testing.T) {
	server, _ := newTestRouter(t)
	defer server.Close()
	adminCookie := loginAsAdmin(t, server)

	createResp, createBody := postJSON(t, server, "/api/admin/third-party/providers", map[string]any{
		"name":          "DingTalk",
		"type":          "dingtalk",
		"client_id":     "ding-client-id",
		"client_secret": "ding-client-secret",
	}, adminCookie)
	if createResp.StatusCode != http.StatusCreated {
		t.Fatalf("create status = %d, want 201, body = %#v", createResp.StatusCode, createBody)
	}
	createdProvider := requireSuccess(t, createBody)["provider"].(map[string]any)
	providerKey := createdProvider["key"].(string)

	noRedirectClient := server.Client()
	noRedirectClient.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	startResp := getResponseWithClient(t, noRedirectClient, server, "/api/client/auth/third-party/"+providerKey+"/start?redirect=/init")
	if startResp.StatusCode != http.StatusFound {
		t.Fatalf("start status = %d, want 302", startResp.StatusCode)
	}
	authorizeLocation := startResp.Header.Get("Location")
	parsedAuthorizeURL, err := url.Parse(authorizeLocation)
	if err != nil {
		t.Fatalf("parse authorize location: %v", err)
	}
	if prompt := parsedAuthorizeURL.Query().Get("prompt"); prompt != "consent" {
		t.Fatalf("prompt = %q, want consent", prompt)
	}
}

func TestAdminCreatesDistinctThirdPartyLoginProviderKeysFromSameName(t *testing.T) {
	server, _ := newTestRouter(t)
	defer server.Close()
	adminCookie := loginAsAdmin(t, server)

	firstResp, firstBody := postJSON(t, server, "/api/admin/third-party/providers", validThirdPartyLoginProviderPayload("企业 SSO"), adminCookie)
	if firstResp.StatusCode != http.StatusCreated {
		t.Fatalf("first create status = %d, want 201, body = %#v", firstResp.StatusCode, firstBody)
	}
	secondResp, secondBody := postJSON(t, server, "/api/admin/third-party/providers", validThirdPartyLoginProviderPayload("企业 SSO"), adminCookie)
	if secondResp.StatusCode != http.StatusCreated {
		t.Fatalf("second create status = %d, want 201, body = %#v", secondResp.StatusCode, secondBody)
	}

	firstProvider := requireSuccess(t, firstBody)["provider"].(map[string]any)
	secondProvider := requireSuccess(t, secondBody)["provider"].(map[string]any)
	if firstProvider["key"] == secondProvider["key"] {
		t.Fatalf("generated keys should be distinct, got %q", firstProvider["key"])
	}
	if secondProvider["sort_order"] != float64(20) {
		t.Fatalf("second sort_order = %#v, want 20", secondProvider["sort_order"])
	}
}

func validThirdPartyLoginProviderPayload(name string) map[string]any {
	return map[string]any{
		"name":          name,
		"type":          "oidc",
		"client_id":     "client-id",
		"client_secret": "client-secret",
		"scopes":        []any{"email", "profile"},
		"config": map[string]any{
			"authorize_url":     "https://sso.example.com/oauth/authorize",
			"token_url":         "https://sso.example.com/oauth/token",
			"userinfo_url":      "https://sso.example.com/oauth/userinfo",
			"external_id_field": "sub",
			"email_field":       "email",
			"phone_field":       "mobile",
			"name_field":        "real_name",
			"nickname_field":    "nickname",
			"avatar_field":      "avatar_url",
		},
	}
}

func TestAdminMovesThirdPartyLoginProvidersAndNormalizesSortOrder(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()
	adminCookie := loginAsAdmin(t, server)

	first := insertTestThirdPartyLoginProvider(t, db, store.ThirdPartyLoginProvider{
		Name:         "Alpha",
		Key:          "alpha",
		Type:         store.ThirdPartyLoginProviderTypeOIDC,
		Enabled:      true,
		ClientID:     "alpha-client",
		ClientSecret: "alpha-secret",
		Scopes:       json.RawMessage(`["email"]`),
		Config: thirdPartyProviderConfig(t, map[string]any{
			"authorize_url":     "https://alpha.example.com/authorize",
			"token_url":         "https://alpha.example.com/token",
			"userinfo_url":      "https://alpha.example.com/userinfo",
			"external_id_field": "sub",
			"email_field":       "email",
			"name_field":        "name",
		}),
		SortOrder: 5,
	})
	second := insertTestThirdPartyLoginProvider(t, db, store.ThirdPartyLoginProvider{
		Name:         "Beta",
		Key:          "beta",
		Type:         store.ThirdPartyLoginProviderTypeOIDC,
		Enabled:      true,
		ClientID:     "beta-client",
		ClientSecret: "beta-secret",
		Scopes:       json.RawMessage(`["email"]`),
		Config: thirdPartyProviderConfig(t, map[string]any{
			"authorize_url":     "https://beta.example.com/authorize",
			"token_url":         "https://beta.example.com/token",
			"userinfo_url":      "https://beta.example.com/userinfo",
			"external_id_field": "sub",
			"email_field":       "email",
			"name_field":        "name",
		}),
		SortOrder: 5,
	})
	third := insertTestThirdPartyLoginProvider(t, db, store.ThirdPartyLoginProvider{
		Name:         "Gamma",
		Key:          "gamma",
		Type:         store.ThirdPartyLoginProviderTypeOIDC,
		Enabled:      true,
		ClientID:     "gamma-client",
		ClientSecret: "gamma-secret",
		Scopes:       json.RawMessage(`["email"]`),
		Config: thirdPartyProviderConfig(t, map[string]any{
			"authorize_url":     "https://gamma.example.com/authorize",
			"token_url":         "https://gamma.example.com/token",
			"userinfo_url":      "https://gamma.example.com/userinfo",
			"external_id_field": "sub",
			"email_field":       "email",
			"name_field":        "name",
		}),
		SortOrder: 5,
	})

	moveResp, moveBody := postJSON(t, server, "/api/admin/third-party/providers/"+third.ID+"/move", map[string]any{
		"direction": "up",
	}, adminCookie)
	if moveResp.StatusCode != http.StatusOK {
		t.Fatalf("move status = %d, want 200, body = %#v", moveResp.StatusCode, moveBody)
	}

	providers := requireThirdPartyLoginProviders(t, requireSuccess(t, moveBody))
	if got := []string{
		providers[0].(map[string]any)["id"].(string),
		providers[1].(map[string]any)["id"].(string),
		providers[2].(map[string]any)["id"].(string),
	}; got[0] != first.ID || got[1] != third.ID || got[2] != second.ID {
		t.Fatalf("provider order = %#v, want first, third, second", got)
	}
	for index, provider := range providers {
		wantSortOrder := float64((index + 1) * 10)
		if provider.(map[string]any)["sort_order"] != wantSortOrder {
			t.Fatalf("provider %d sort_order = %#v, want %v", index, provider.(map[string]any)["sort_order"], wantSortOrder)
		}
	}
}

func TestThirdPartyLoginProviderAdminAPIRequiresAdminSession(t *testing.T) {
	server, _ := newTestRouter(t)
	defer server.Close()

	resp, body := getJSON(t, server, "/api/admin/third-party/providers")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
	requireError(t, body, "unauthorized")
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

func TestGetCurrentUserRequiresUserSession(t *testing.T) {
	server, _ := newTestRouter(t)
	defer server.Close()

	resp, body := getJSON(t, server, "/api/client/me")

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
	requireError(t, body, "unauthorized")
}

func TestGetCurrentUserReturnsSessionUser(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	phone := "+8613912345678"
	user := insertTestUser(t, db, "alice@example.com", "Alice Zhang", store.UserStatusActive, time.Now().UTC())
	user.Avatar = "/assets/avatars/builtin/17.webp"
	user.Nickname = "Al"
	user.Phone = &phone
	if err := db.Model(&store.User{}).Where("id = ?", user.ID).Updates(map[string]any{
		"avatar":   user.Avatar,
		"nickname": user.Nickname,
		"phone":    phone,
	}).Error; err != nil {
		t.Fatalf("update user profile: %v", err)
	}

	userCookie := loginAsUser(t, server, user.Email)

	resp, body := getJSON(t, server, "/api/client/me", userCookie)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %#v", resp.StatusCode, body)
	}
	data := requireSuccess(t, body)
	currentUser := data["user"].(map[string]any)
	if currentUser["id"] != user.ID {
		t.Fatalf("user.id = %v, want %s", currentUser["id"], user.ID)
	}
	if currentUser["email"] != user.Email {
		t.Fatalf("user.email = %v, want %s", currentUser["email"], user.Email)
	}
	if currentUser["name"] != user.Name {
		t.Fatalf("user.name = %v, want %s", currentUser["name"], user.Name)
	}
	if currentUser["nickname"] != user.Nickname {
		t.Fatalf("user.nickname = %v, want %s", currentUser["nickname"], user.Nickname)
	}
	if currentUser["phone"] != phone {
		t.Fatalf("user.phone = %v, want %s", currentUser["phone"], phone)
	}
	if currentUser["avatar"] != user.Avatar {
		t.Fatalf("user.avatar = %v, want %s", currentUser["avatar"], user.Avatar)
	}
	if currentUser["status"] != store.UserStatusActive {
		t.Fatalf("user.status = %v, want active", currentUser["status"])
	}
	if createdAt, ok := currentUser["created_at"].(string); !ok || createdAt == "" {
		t.Fatalf("user.created_at = %#v, want non-empty string", currentUser["created_at"])
	}
}

func TestUpdateCurrentUserRequiresUserSession(t *testing.T) {
	server, _ := newTestRouter(t)
	defer server.Close()

	resp, body := patchJSON(t, server, "/api/client/me", map[string]any{
		"avatar": "/assets/avatars/builtin/03.webp",
	})

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
	requireError(t, body, "unauthorized")
}

func TestUpdateCurrentUserCanUpdateAvatarOnly(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	user := insertTestUser(t, db, "alice@example.com", "Alice Zhang", store.UserStatusActive, time.Now().UTC())
	user.Nickname = "Al"
	if err := db.Model(&store.User{}).Where("id = ?", user.ID).Update("nickname", user.Nickname).Error; err != nil {
		t.Fatalf("set nickname: %v", err)
	}
	userCookie := loginAsUser(t, server, user.Email)

	resp, body := patchJSON(t, server, "/api/client/me", map[string]any{
		"avatar": "/assets/avatars/builtin/03.webp",
	}, userCookie)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %#v", resp.StatusCode, body)
	}
	data := requireSuccess(t, body)
	currentUser := data["user"].(map[string]any)
	if currentUser["avatar"] != "/assets/avatars/builtin/03.webp" {
		t.Fatalf("user.avatar = %v, want updated avatar", currentUser["avatar"])
	}
	if currentUser["nickname"] != "Al" {
		t.Fatalf("user.nickname = %v, want unchanged nickname", currentUser["nickname"])
	}

	var stored store.User
	if err := db.First(&stored, "id = ?", user.ID).Error; err != nil {
		t.Fatalf("load stored user: %v", err)
	}
	if stored.Avatar != "/assets/avatars/builtin/03.webp" {
		t.Fatalf("stored avatar = %q, want updated avatar", stored.Avatar)
	}
	if stored.Nickname != "Al" {
		t.Fatalf("stored nickname = %q, want unchanged nickname", stored.Nickname)
	}
}

func TestUpdateCurrentUserCanUpdateNicknameOnly(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	user := insertTestUser(t, db, "alice@example.com", "Alice Zhang", store.UserStatusActive, time.Now().UTC())
	user.Avatar = "/assets/avatars/builtin/17.webp"
	if err := db.Model(&store.User{}).Where("id = ?", user.ID).Update("avatar", user.Avatar).Error; err != nil {
		t.Fatalf("set avatar: %v", err)
	}
	userCookie := loginAsUser(t, server, user.Email)

	resp, body := patchJSON(t, server, "/api/client/me", map[string]any{
		"nickname": "Alice A",
	}, userCookie)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %#v", resp.StatusCode, body)
	}
	data := requireSuccess(t, body)
	currentUser := data["user"].(map[string]any)
	if currentUser["nickname"] != "Alice A" {
		t.Fatalf("user.nickname = %v, want updated nickname", currentUser["nickname"])
	}
	if currentUser["avatar"] != "/assets/avatars/builtin/17.webp" {
		t.Fatalf("user.avatar = %v, want unchanged avatar", currentUser["avatar"])
	}
}

func TestClientLogoutClearsUserSession(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	user := insertTestUser(t, db, "alice@example.com", "Alice", store.UserStatusActive, time.Now().UTC())
	userCookie := loginAsUser(t, server, user.Email)

	resp, body := postJSON(t, server, "/api/client/auth/logout", map[string]any{}, userCookie)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("logout status = %d, want 200, body = %#v", resp.StatusCode, body)
	}
	requireSuccess(t, body)
	expiredCookie := findCookieNamed(t, resp, "user_session")
	if expiredCookie.Value != "" {
		t.Fatalf("logout cookie value = %q, want empty", expiredCookie.Value)
	}
	if expiredCookie.MaxAge >= 0 {
		t.Fatalf("logout cookie MaxAge = %d, want negative", expiredCookie.MaxAge)
	}

	var userSessionCount int64
	if err := db.Model(&store.UserSession{}).Where("user_id = ?", user.ID).Count(&userSessionCount).Error; err != nil {
		t.Fatalf("count user sessions: %v", err)
	}
	if userSessionCount != 0 {
		t.Fatalf("user session count after logout = %d, want 0", userSessionCount)
	}

	meResp, meBody := getJSON(t, server, "/api/client/me", userCookie)
	if meResp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("me status after logout = %d, want 401", meResp.StatusCode)
	}
	requireError(t, meBody, "unauthorized")
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

func TestCreateGroupConversationCreatesSystemInviteMessage(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	now := time.Now().UTC()
	creator := insertTestUser(t, db, "creator@example.com", "Creator", store.UserStatusActive, now)
	alice := insertTestUser(t, db, "alice@example.com", "Alice", store.UserStatusActive, now)
	bob := insertTestUser(t, db, "bob@example.com", "Bob", store.UserStatusActive, now)
	userCookie := loginAsUser(t, server, creator.Email)

	resp, body := postJSON(t, server, "/api/client/conversations/groups", map[string]any{
		"name":       "产品讨论组",
		"member_ids": []string{alice.ID, bob.ID},
	}, userCookie)

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201, body = %#v", resp.StatusCode, body)
	}
	conversation := requireSuccess(t, body)["conversation"].(map[string]any)
	conversationID := conversation["id"].(string)
	summary := "Creator 邀请 Alice,Bob 加入群聊"
	if conversation["last_message_seq"] != float64(1) {
		t.Fatalf("conversation.last_message_seq = %v, want 1", conversation["last_message_seq"])
	}
	if conversation["last_message_summary"] != summary {
		t.Fatalf("conversation.last_message_summary = %v, want %s", conversation["last_message_summary"], summary)
	}
	if conversation["last_read_seq"] != float64(1) {
		t.Fatalf("conversation.last_read_seq = %v, want 1", conversation["last_read_seq"])
	}
	if conversation["unread_count"] != float64(0) {
		t.Fatalf("conversation.unread_count = %v, want 0", conversation["unread_count"])
	}

	var storedMessage store.Message
	if err := db.First(&storedMessage, "conversation_id = ? AND seq = ?", conversationID, int64(1)).Error; err != nil {
		t.Fatalf("find stored system message: %v", err)
	}
	if storedMessage.SenderType != store.MessageSenderTypeSystem {
		t.Fatalf("stored sender_type = %v, want system", storedMessage.SenderType)
	}
	if storedMessage.SenderID != nil {
		t.Fatalf("stored sender_id = %v, want nil", storedMessage.SenderID)
	}
	if storedMessage.ClientMessageID != nil {
		t.Fatalf("stored client_message_id = %v, want nil", storedMessage.ClientMessageID)
	}
	if storedMessage.Summary != summary {
		t.Fatalf("stored summary = %v, want %s", storedMessage.Summary, summary)
	}
	requireGroupMembersInvitedBody(t, storedMessage.Body, creator.ID, "Creator", []string{alice.ID, bob.ID}, []string{"Alice", "Bob"})

	var storedConversation store.Conversation
	if err := db.First(&storedConversation, "id = ?", conversationID).Error; err != nil {
		t.Fatalf("find stored conversation: %v", err)
	}
	if storedConversation.LastMessageID == nil || *storedConversation.LastMessageID != storedMessage.ID {
		t.Fatalf("last_message_id = %v, want %s", storedConversation.LastMessageID, storedMessage.ID)
	}
	if storedConversation.LastMessageSeq != 1 {
		t.Fatalf("last_message_seq = %d, want 1", storedConversation.LastMessageSeq)
	}
	if storedConversation.LastMessageSummary != summary {
		t.Fatalf("last_message_summary = %v, want %s", storedConversation.LastMessageSummary, summary)
	}
	if got := getTestConversationMemberLastReadSeq(t, db, conversationID, creator.ID); got != 1 {
		t.Fatalf("creator last_read_seq = %d, want 1", got)
	}
	if got := getTestConversationMemberLastReadSeq(t, db, conversationID, alice.ID); got != 0 {
		t.Fatalf("alice last_read_seq = %d, want 0", got)
	}
	if got := getTestConversationMemberLastReadSeq(t, db, conversationID, bob.ID); got != 0 {
		t.Fatalf("bob last_read_seq = %d, want 0", got)
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

func TestAddGroupConversationMembersCreatesMembersAndSystemMessage(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	now := time.Date(2026, 7, 3, 9, 0, 0, 0, time.UTC)
	alice := insertTestUser(t, db, "alice@example.com", "Alice", store.UserStatusActive, now)
	bob := insertTestUser(t, db, "bob@example.com", "Bob", store.UserStatusActive, now)
	carol := insertTestUser(t, db, "carol@example.com", "Carol", store.UserStatusActive, now)
	dave := insertTestUser(t, db, "dave@example.com", "Dave", store.UserStatusActive, now)
	lastMessageAt := now.Add(-time.Hour)
	conversation := insertTestConversation(t, db, testConversationInput{
		createdByUserID:    alice.ID,
		kind:               store.ConversationKindGroup,
		lastMessageAt:      &lastMessageAt,
		lastMessageSeq:     2,
		lastMessageSummary: "旧消息",
		memberIDs:          []string{alice.ID, bob.ID},
		name:               "产品讨论组",
		now:                now.Add(-2 * time.Hour),
	})
	setTestConversationMemberLastReadSeq(t, db, conversation.ID, alice.ID, 2)
	setTestConversationMemberLastReadSeq(t, db, conversation.ID, bob.ID, 1)
	userCookie := loginAsUser(t, server, alice.Email)

	resp, body := postJSON(t, server, "/api/client/conversations/"+conversation.ID+"/members", map[string]any{
		"member_ids": []string{carol.ID, dave.ID, carol.ID},
	}, userCookie)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %#v", resp.StatusCode, body)
	}
	data := requireSuccess(t, body)
	updatedConversation := data["conversation"].(map[string]any)
	createdMessage := data["message"].(map[string]any)
	summary := "Alice 邀请 Carol,Dave 加入群聊"
	if updatedConversation["id"] != conversation.ID {
		t.Fatalf("conversation.id = %v, want %s", updatedConversation["id"], conversation.ID)
	}
	if updatedConversation["member_count"] != float64(4) {
		t.Fatalf("conversation.member_count = %v, want 4", updatedConversation["member_count"])
	}
	if updatedConversation["last_message_seq"] != float64(3) {
		t.Fatalf("conversation.last_message_seq = %v, want 3", updatedConversation["last_message_seq"])
	}
	if updatedConversation["last_message_summary"] != summary {
		t.Fatalf("conversation.last_message_summary = %v, want %s", updatedConversation["last_message_summary"], summary)
	}
	if updatedConversation["last_read_seq"] != float64(3) {
		t.Fatalf("conversation.last_read_seq = %v, want 3", updatedConversation["last_read_seq"])
	}
	if updatedConversation["unread_count"] != float64(0) {
		t.Fatalf("conversation.unread_count = %v, want 0", updatedConversation["unread_count"])
	}
	if createdMessage["conversation_id"] != conversation.ID {
		t.Fatalf("message.conversation_id = %v, want %s", createdMessage["conversation_id"], conversation.ID)
	}
	if createdMessage["seq"] != float64(3) {
		t.Fatalf("message.seq = %v, want 3", createdMessage["seq"])
	}
	sender := createdMessage["sender"].(map[string]any)
	if sender["type"] != store.MessageSenderTypeSystem {
		t.Fatalf("message.sender.type = %v, want system", sender["type"])
	}
	if _, ok := sender["id"]; ok {
		t.Fatalf("message.sender.id = %v, want omitted", sender["id"])
	}
	messageBody := createdMessage["body"].(map[string]any)
	if messageBody["type"] != "system_event" {
		t.Fatalf("message.body.type = %v, want system_event", messageBody["type"])
	}
	if messageBody["event"] != "group_members_invited" {
		t.Fatalf("message.body.event = %v, want group_members_invited", messageBody["event"])
	}

	var storedMembers []store.ConversationMember
	if err := db.Where("conversation_id = ?", conversation.ID).Find(&storedMembers).Error; err != nil {
		t.Fatalf("find stored members: %v", err)
	}
	if len(storedMembers) != 4 {
		t.Fatalf("stored member count = %d, want 4", len(storedMembers))
	}
	membersByID := map[string]store.ConversationMember{}
	for _, member := range storedMembers {
		membersByID[member.MemberID] = member
	}
	for _, memberID := range []string{carol.ID, dave.ID} {
		member, ok := membersByID[memberID]
		if !ok {
			t.Fatalf("missing stored member %s", memberID)
		}
		if member.Role != store.ConversationMemberRoleMember {
			t.Fatalf("member %s role = %v, want member", memberID, member.Role)
		}
		if member.HistoryVisibleFromSeq != 3 {
			t.Fatalf("member %s history_visible_from_seq = %d, want 3", memberID, member.HistoryVisibleFromSeq)
		}
		if member.LastReadSeq != 2 {
			t.Fatalf("member %s last_read_seq = %d, want 2", memberID, member.LastReadSeq)
		}
	}
	if got := getTestConversationMemberLastReadSeq(t, db, conversation.ID, alice.ID); got != 3 {
		t.Fatalf("alice last_read_seq = %d, want 3", got)
	}
	if got := getTestConversationMemberLastReadSeq(t, db, conversation.ID, bob.ID); got != 1 {
		t.Fatalf("bob last_read_seq = %d, want 1", got)
	}

	var storedMessage store.Message
	if err := db.First(&storedMessage, "id = ?", createdMessage["id"]).Error; err != nil {
		t.Fatalf("find stored system message: %v", err)
	}
	if storedMessage.Summary != summary {
		t.Fatalf("stored summary = %v, want %s", storedMessage.Summary, summary)
	}
	requireGroupMembersInvitedBody(t, storedMessage.Body, alice.ID, "Alice", []string{carol.ID, dave.ID}, []string{"Carol", "Dave"})
}

func TestAddGroupConversationMembersNoopsWhenMembersAlreadyExist(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	now := time.Date(2026, 7, 3, 9, 0, 0, 0, time.UTC)
	alice := insertTestUser(t, db, "alice@example.com", "Alice", store.UserStatusActive, now)
	bob := insertTestUser(t, db, "bob@example.com", "Bob", store.UserStatusActive, now)
	conversation := insertTestConversation(t, db, testConversationInput{
		createdByUserID:    alice.ID,
		kind:               store.ConversationKindGroup,
		lastMessageSeq:     4,
		lastMessageSummary: "旧消息",
		memberIDs:          []string{alice.ID, bob.ID},
		name:               "产品讨论组",
		now:                now,
	})
	setTestConversationMemberLastReadSeq(t, db, conversation.ID, alice.ID, 4)

	resp, body := postJSON(t, server, "/api/client/conversations/"+conversation.ID+"/members", map[string]any{
		"member_ids": []string{bob.ID},
	}, loginAsUser(t, server, alice.Email))

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %#v", resp.StatusCode, body)
	}
	data := requireSuccess(t, body)
	if data["message"] != nil {
		t.Fatalf("message = %#v, want nil", data["message"])
	}
	updatedConversation := data["conversation"].(map[string]any)
	if updatedConversation["last_message_seq"] != float64(4) {
		t.Fatalf("last_message_seq = %v, want 4", updatedConversation["last_message_seq"])
	}

	var messageCount int64
	if err := db.Model(&store.Message{}).Where("conversation_id = ?", conversation.ID).Count(&messageCount).Error; err != nil {
		t.Fatalf("count messages: %v", err)
	}
	if messageCount != 0 {
		t.Fatalf("message count = %d, want 0", messageCount)
	}
}

func TestAddGroupConversationMembersReactivatesLeftMember(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	now := time.Date(2026, 7, 3, 9, 0, 0, 0, time.UTC)
	alice := insertTestUser(t, db, "alice@example.com", "Alice", store.UserStatusActive, now)
	bob := insertTestUser(t, db, "bob@example.com", "Bob", store.UserStatusActive, now)
	carol := insertTestUser(t, db, "carol@example.com", "Carol", store.UserStatusActive, now)
	leftAt := now.Add(-30 * time.Minute)
	conversation := insertTestConversation(t, db, testConversationInput{
		createdByUserID:    alice.ID,
		kind:               store.ConversationKindGroup,
		lastMessageSeq:     5,
		lastMessageSummary: "旧消息",
		memberIDs:          []string{alice.ID, bob.ID, carol.ID},
		memberLeftAtByID:   map[string]*time.Time{carol.ID: &leftAt},
		name:               "产品讨论组",
		now:                now.Add(-2 * time.Hour),
	})
	setTestConversationMemberLastReadSeq(t, db, conversation.ID, alice.ID, 5)
	setTestConversationMemberLastReadSeq(t, db, conversation.ID, carol.ID, 2)

	resp, body := postJSON(t, server, "/api/client/conversations/"+conversation.ID+"/members", map[string]any{
		"member_ids": []string{carol.ID},
	}, loginAsUser(t, server, alice.Email))

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %#v", resp.StatusCode, body)
	}
	data := requireSuccess(t, body)
	updatedConversation := data["conversation"].(map[string]any)
	createdMessage := data["message"].(map[string]any)
	summary := "Alice 邀请 Carol 加入群聊"
	if updatedConversation["member_count"] != float64(3) {
		t.Fatalf("conversation.member_count = %v, want 3", updatedConversation["member_count"])
	}
	if updatedConversation["last_message_seq"] != float64(6) {
		t.Fatalf("conversation.last_message_seq = %v, want 6", updatedConversation["last_message_seq"])
	}
	if updatedConversation["last_message_summary"] != summary {
		t.Fatalf("conversation.last_message_summary = %v, want %s", updatedConversation["last_message_summary"], summary)
	}
	if createdMessage["seq"] != float64(6) {
		t.Fatalf("message.seq = %v, want 6", createdMessage["seq"])
	}

	var reactivatedMember store.ConversationMember
	if err := db.First(
		&reactivatedMember,
		"conversation_id = ? AND member_type = ? AND member_id = ?",
		conversation.ID,
		store.ConversationMemberTypeUser,
		carol.ID,
	).Error; err != nil {
		t.Fatalf("find reactivated member: %v", err)
	}
	if reactivatedMember.LeftAt != nil {
		t.Fatalf("left_at = %v, want nil", reactivatedMember.LeftAt)
	}
	if reactivatedMember.HistoryVisibleFromSeq != 6 {
		t.Fatalf("history_visible_from_seq = %d, want 6", reactivatedMember.HistoryVisibleFromSeq)
	}
	if reactivatedMember.LastReadSeq != 5 {
		t.Fatalf("last_read_seq = %d, want 5", reactivatedMember.LastReadSeq)
	}
	if reactivatedMember.Role != store.ConversationMemberRoleMember {
		t.Fatalf("role = %v, want member", reactivatedMember.Role)
	}

	var memberCount int64
	if err := db.Model(&store.ConversationMember{}).Where("conversation_id = ?", conversation.ID).Count(&memberCount).Error; err != nil {
		t.Fatalf("count members: %v", err)
	}
	if memberCount != 3 {
		t.Fatalf("stored member row count = %d, want 3", memberCount)
	}

	var storedMessage store.Message
	if err := db.First(&storedMessage, "id = ?", createdMessage["id"]).Error; err != nil {
		t.Fatalf("find stored system message: %v", err)
	}
	if storedMessage.Summary != summary {
		t.Fatalf("stored summary = %v, want %s", storedMessage.Summary, summary)
	}
	requireGroupMembersInvitedBody(t, storedMessage.Body, alice.ID, "Alice", []string{carol.ID}, []string{"Carol"})
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
	if user["nickname"] != "" {
		t.Fatalf("user.nickname = %v, want empty string", user["nickname"])
	}
	if user["phone"] != nil && user["phone"] != "" {
		t.Fatalf("user.phone = %v, want empty string or null", user["phone"])
	}
	avatar, ok := user["avatar"].(string)
	if !ok {
		t.Fatalf("user.avatar = %#v, want string", user["avatar"])
	}
	if !strings.HasPrefix(avatar, "/assets/avatars/builtin/") || !strings.HasSuffix(avatar, ".webp") {
		t.Fatalf("user.avatar = %q, want builtin webp path", avatar)
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

func TestAdminCreatesUserWithNormalizedOptionalPhone(t *testing.T) {
	server, _ := newTestRouter(t)
	defer server.Close()

	adminCookie := loginAsAdmin(t, server)

	resp, body := postJSON(t, server, "/api/admin/users", map[string]any{
		"email": "alice@example.com",
		"name":  "Alice Zhang",
		"phone": "+86 138-1234-5678",
	}, adminCookie)

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201, body = %#v", resp.StatusCode, body)
	}
	user := requireSuccess(t, body)["user"].(map[string]any)
	if user["phone"] != "+8613812345678" {
		t.Fatalf("user.phone = %v, want normalized phone", user["phone"])
	}
	if user["nickname"] != "" {
		t.Fatalf("user.nickname = %v, want empty string", user["nickname"])
	}
	if avatar := user["avatar"].(string); !strings.HasPrefix(avatar, "/assets/avatars/builtin/") {
		t.Fatalf("user.avatar = %q, want builtin path", avatar)
	}
}

func TestDuplicateUserPhoneReturnsConflict(t *testing.T) {
	server, _ := newTestRouter(t)
	defer server.Close()

	adminCookie := loginAsAdmin(t, server)

	firstResp, firstBody := postJSON(t, server, "/api/admin/users", map[string]any{
		"email": "alice@example.com",
		"name":  "Alice",
		"phone": "13812345678",
	}, adminCookie)
	if firstResp.StatusCode != http.StatusCreated {
		t.Fatalf("first status = %d, want 201", firstResp.StatusCode)
	}
	requireSuccess(t, firstBody)

	duplicateResp, duplicateBody := postJSON(t, server, "/api/admin/users", map[string]any{
		"email": "bob@example.com",
		"name":  "Bob",
		"phone": "+8613812345678",
	}, adminCookie)
	if duplicateResp.StatusCode != http.StatusConflict {
		t.Fatalf("duplicate status = %d, want 409", duplicateResp.StatusCode)
	}
	requireError(t, duplicateBody, "conflict")
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
	carol := insertTestUser(t, db, "carol@company.io", "Carol Wang", store.UserStatusActive, jan3)
	if err := db.Model(&store.User{}).Where("id = ?", carol.ID).Update("phone", "+8613900000003").Error; err != nil {
		t.Fatalf("set carol phone: %v", err)
	}

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

	phoneResp, phoneBody := getJSON(t, server, "/api/admin/users?keyword="+url.QueryEscape("13900000003"), adminCookie)
	if phoneResp.StatusCode != http.StatusOK {
		t.Fatalf("phone search status = %d, want 200", phoneResp.StatusCode)
	}
	phoneUsers := requireUsers(t, requireSuccess(t, phoneBody))
	if len(phoneUsers) != 1 {
		t.Fatalf("phone search user count = %d, want 1", len(phoneUsers))
	}
	if got := phoneUsers[0].(map[string]any)["email"]; got != "carol@company.io" {
		t.Fatalf("phone search result = %v, want carol@company.io", got)
	}
}

func TestListContactsReturnsActiveUsersIncludingSelf(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	now := time.Now().UTC()
	alice := insertTestUser(t, db, "alice@example.com", "Alice Zhang", store.UserStatusActive, now)
	bob := insertTestUser(t, db, "bob@example.com", "Bob Li", store.UserStatusActive, now)
	disabled := insertTestUser(t, db, "disabled@example.com", "Disabled User", store.UserStatusDisabled, now)
	userCookie := loginAsUser(t, server, alice.Email)

	resp, body := getJSON(t, server, "/api/client/contacts/users", userCookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %#v", resp.StatusCode, body)
	}

	contacts := requireContacts(t, requireSuccess(t, body))
	ids := map[string]bool{}
	for _, rawContact := range contacts {
		contact := rawContact.(map[string]any)
		ids[contact["id"].(string)] = true
		if contact["type"] != "user" {
			t.Fatalf("contact.type = %v, want user", contact["type"])
		}
		if _, ok := contact["nickname"].(string); !ok {
			t.Fatalf("contact.nickname = %#v, want string", contact["nickname"])
		}
		if _, ok := contact["avatar"].(string); !ok {
			t.Fatalf("contact.avatar = %#v, want string", contact["avatar"])
		}
	}
	if !ids[alice.ID] {
		t.Fatal("contacts did not include current user")
	}
	if !ids[bob.ID] {
		t.Fatal("contacts did not include active user")
	}
	if ids[disabled.ID] {
		t.Fatal("contacts included disabled user")
	}
}

func TestListContactsSearchesNameEmailNicknameAndPhone(t *testing.T) {
	server, _ := newTestRouter(t)
	defer server.Close()

	adminCookie := loginAsAdmin(t, server)
	aliceResp, aliceBody := postJSON(t, server, "/api/admin/users", map[string]any{
		"email": "alice@example.com",
		"name":  "Alice Zhang",
		"phone": "13900000001",
	}, adminCookie)
	if aliceResp.StatusCode != http.StatusCreated {
		t.Fatalf("alice status = %d, want 201", aliceResp.StatusCode)
	}
	aliceData := requireSuccess(t, aliceBody)
	alice := aliceData["user"].(map[string]any)
	alicePassword := aliceData["initial_password"].(string)

	bobResp, bobBody := postJSON(t, server, "/api/admin/users", map[string]any{
		"email": "bob@example.com",
		"name":  "Bob Li",
		"phone": "13900000002",
	}, adminCookie)
	if bobResp.StatusCode != http.StatusCreated {
		t.Fatalf("bob status = %d, want 201", bobResp.StatusCode)
	}
	requireSuccess(t, bobBody)

	loginResp, loginBody := postJSON(t, server, "/api/client/auth/login", map[string]any{
		"email":    "alice@example.com",
		"password": alicePassword,
	})
	if loginResp.StatusCode != http.StatusOK {
		t.Fatalf("login status = %d, want 200", loginResp.StatusCode)
	}
	requireSuccess(t, loginBody)
	userCookie := requireUserSessionCookie(t, loginResp)

	resp, body := getJSON(t, server, "/api/client/contacts/users?keyword="+url.QueryEscape("13900000002"), userCookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %#v", resp.StatusCode, body)
	}
	contacts := requireContacts(t, requireSuccess(t, body))
	if len(contacts) != 1 {
		t.Fatalf("contact count = %d, want 1", len(contacts))
	}
	contact := contacts[0].(map[string]any)
	if contact["email"] != "bob@example.com" {
		t.Fatalf("contact.email = %v, want bob@example.com", contact["email"])
	}
	if contact["phone"] != "+8613900000002" {
		t.Fatalf("contact.phone = %v, want normalized phone", contact["phone"])
	}
	if contact["id"] == alice["id"] {
		t.Fatal("phone keyword matched the current user unexpectedly")
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
	userCookie := loginAsUser(t, server, user.Email)
	conn := dialClientWebSocket(t, server, userCookie)
	_ = conn.SetReadDeadline(time.Now().Add(time.Second))
	var ready realtime.Envelope
	if err := conn.ReadJSON(&ready); err != nil {
		t.Fatalf("ReadJSON() ready error = %v", err)
	}
	if ready.Kind != realtime.KindEvent || ready.Event != realtime.EventSystemReady {
		t.Fatalf("ready envelope = %#v, want system.ready event", ready)
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
	_ = conn.SetReadDeadline(time.Now().Add(time.Second))
	if _, _, err := conn.ReadMessage(); err == nil {
		t.Fatal("disabled user's websocket read error = nil, want closed connection")
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
