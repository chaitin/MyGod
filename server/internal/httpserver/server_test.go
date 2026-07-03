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
		Server:   config.ServerConfig{Addr: ":20080"},
		Database: config.DatabaseConfig{DSN: "sqlite-test"},
		Admin:    config.AdminConfig{Password: "admin-secret"},
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
		&store.AppSettings{},
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
	if len(conversations) != 2 {
		t.Fatalf("conversation count = %d, want 2: %#v", len(conversations), conversations)
	}

	first := conversations[0].(map[string]any)
	if first["id"] != newDirect.ID {
		t.Fatalf("first id = %v, want new direct %s", first["id"], newDirect.ID)
	}
	if first["type"] != store.ConversationKindDirect {
		t.Fatalf("first type = %v, want direct", first["type"])
	}
	if first["name"] != bob.Name {
		t.Fatalf("direct name = %v, want %s", first["name"], bob.Name)
	}
	if first["avatar"] != bob.Avatar {
		t.Fatalf("direct avatar = %v, want %s", first["avatar"], bob.Avatar)
	}
	if first["member_count"] != float64(2) {
		t.Fatalf("direct member_count = %v, want 2", first["member_count"])
	}
	if first["last_message_summary"] != "newer direct" {
		t.Fatalf("direct last_message_summary = %v, want newer direct", first["last_message_summary"])
	}
	if first["last_message_seq"] != float64(5) {
		t.Fatalf("direct last_message_seq = %v, want 5", first["last_message_seq"])
	}
	if first["last_message_at"] != newDirectLastAt.Format(time.RFC3339) {
		t.Fatalf("direct last_message_at = %v, want %s", first["last_message_at"], newDirectLastAt.Format(time.RFC3339))
	}

	second := conversations[1].(map[string]any)
	if second["id"] != oldGroup.ID {
		t.Fatalf("second id = %v, want old group %s", second["id"], oldGroup.ID)
	}
	if second["type"] != store.ConversationKindGroup {
		t.Fatalf("second type = %v, want group", second["type"])
	}
	if second["name"] != oldGroup.Name {
		t.Fatalf("group name = %v, want %s", second["name"], oldGroup.Name)
	}
	if second["member_count"] != float64(2) {
		t.Fatalf("group member_count = %v, want 2", second["member_count"])
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
	last := conversations[99].(map[string]any)
	if first["name"] != "Group 000" {
		t.Fatalf("first name = %v, want Group 000", first["name"])
	}
	if last["name"] != "Group 099" {
		t.Fatalf("last name = %v, want Group 099", last["name"])
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
