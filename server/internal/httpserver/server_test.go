package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
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
	"gorm.io/gorm/clause"
	gormlogger "gorm.io/gorm/logger"
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
		Apps:     config.AppsConfig{AIAssistantSecret: "test-ai-assistant-secret"},
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
		&store.AppEventOutbox{},
		&store.AppEventAck{},
		&store.AppSettings{},
		&store.ThirdPartyLoginProvider{},
		&store.ThirdPartyLoginState{},
		&store.ThirdPartyAccount{},
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

func insertTestApp(t *testing.T, db *gorm.DB, input store.App) store.App {
	t.Helper()

	if input.ID == "" {
		input.ID = uuid.NewString()
	}
	if input.Visibility == "" {
		input.Visibility = store.AppVisibilityPublic
	}
	if input.ConnectionSecret == "" {
		input.ConnectionSecret = uuid.NewString()
	}
	if input.CreatedAt.IsZero() {
		input.CreatedAt = time.Now().UTC()
	}
	if input.UpdatedAt.IsZero() {
		input.UpdatedAt = input.CreatedAt
	}

	if err := db.Select("*").Create(&input).Error; err != nil {
		t.Fatalf("create test app: %v", err)
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
	visibility         string
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
		Visibility:         firstNonEmptyString(input.visibility, store.ConversationVisibilityPrivate),
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

func insertTestAppConversationLink(t *testing.T, db *gorm.DB, appID string, userID string, conversationID string, now time.Time) {
	t.Helper()

	member := store.ConversationMember{
		ConversationID:        conversationID,
		MemberType:            store.ConversationMemberTypeApp,
		MemberID:              appID,
		Role:                  store.ConversationMemberRoleMember,
		JoinedAt:              now,
		HistoryVisibleFromSeq: 1,
	}
	if err := db.Create(&member).Error; err != nil {
		t.Fatalf("create test app conversation member: %v", err)
	}

	link := store.AppConversation{
		AppID:          appID,
		UserID:         userID,
		ConversationID: conversationID,
		CreatedAt:      now,
	}
	if err := db.Create(&link).Error; err != nil {
		t.Fatalf("create test app conversation link: %v", err)
	}
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

func requireSystemEventActorBody(t *testing.T, rawBody json.RawMessage, event string, actorID string, actorDisplayName string) {
	t.Helper()

	var body map[string]any
	if err := json.Unmarshal(rawBody, &body); err != nil {
		t.Fatalf("unmarshal system body: %v", err)
	}
	if body["type"] != "system_event" {
		t.Fatalf("body.type = %v, want system_event", body["type"])
	}
	if body["event"] != event {
		t.Fatalf("body.event = %v, want %s", body["event"], event)
	}
	actor, ok := body["actor"].(map[string]any)
	if !ok {
		t.Fatalf("body.actor = %#v, want object", body["actor"])
	}
	if actor["id"] != actorID {
		t.Fatalf("actor.id = %v, want %s", actor["id"], actorID)
	}
	if actor["display_name"] != actorDisplayName {
		t.Fatalf("actor.display_name = %v, want %s", actor["display_name"], actorDisplayName)
	}
}

func requireGroupMemberRemovedBody(t *testing.T, rawBody json.RawMessage, actorID string, actorDisplayName string, targetID string, targetDisplayName string) {
	t.Helper()

	var body map[string]any
	if err := json.Unmarshal(rawBody, &body); err != nil {
		t.Fatalf("unmarshal system body: %v", err)
	}
	if body["type"] != "system_event" {
		t.Fatalf("body.type = %v, want system_event", body["type"])
	}
	if body["event"] != "group_member_removed" {
		t.Fatalf("body.event = %v, want group_member_removed", body["event"])
	}
	actor, ok := body["actor"].(map[string]any)
	if !ok {
		t.Fatalf("body.actor = %#v, want object", body["actor"])
	}
	if actor["id"] != actorID {
		t.Fatalf("actor.id = %v, want %s", actor["id"], actorID)
	}
	if actor["display_name"] != actorDisplayName {
		t.Fatalf("actor.display_name = %v, want %s", actor["display_name"], actorDisplayName)
	}
	target, ok := body["target"].(map[string]any)
	if !ok {
		t.Fatalf("body.target = %#v, want object", body["target"])
	}
	if target["id"] != targetID {
		t.Fatalf("target.id = %v, want %s", target["id"], targetID)
	}
	if target["display_name"] != targetDisplayName {
		t.Fatalf("target.display_name = %v, want %s", target["display_name"], targetDisplayName)
	}
}

func requireMessageRevokedBody(t *testing.T, rawBody json.RawMessage, actorID string, actorDisplayName string) {
	t.Helper()

	var body map[string]any
	if err := json.Unmarshal(rawBody, &body); err != nil {
		t.Fatalf("unmarshal system body: %v", err)
	}
	if body["type"] != "system_event" {
		t.Fatalf("body.type = %v, want system_event", body["type"])
	}
	if body["event"] != "message_revoked" {
		t.Fatalf("body.event = %v, want message_revoked", body["event"])
	}
	actor, ok := body["actor"].(map[string]any)
	if !ok {
		t.Fatalf("body.actor = %#v, want object", body["actor"])
	}
	if actor["id"] != actorID {
		t.Fatalf("actor.id = %v, want %s", actor["id"], actorID)
	}
	if actor["display_name"] != actorDisplayName {
		t.Fatalf("actor.display_name = %v, want %s", actor["display_name"], actorDisplayName)
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

func appWebSocketURL(server *httptest.Server) string {
	return "ws" + strings.TrimPrefix(server.URL, "http") + "/api/app/ws"
}

func dialAppWebSocket(t *testing.T, server *httptest.Server, appID string, secret string) *websocket.Conn {
	t.Helper()

	header := http.Header{}
	header.Set(appIDHeader, appID)
	header.Set("Authorization", "Bearer "+secret)
	conn, resp, err := websocket.DefaultDialer.Dial(appWebSocketURL(server), header)
	if err != nil {
		status := 0
		if resp != nil {
			status = resp.StatusCode
		}
		t.Fatalf("dial app websocket: %v, status = %d", err, status)
	}
	t.Cleanup(func() {
		_ = conn.Close()
	})

	return conn
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

func requireNoRealtimeEvent(t *testing.T, conn *websocket.Conn) {
	t.Helper()

	_ = conn.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
	var envelope realtime.Envelope
	if err := conn.ReadJSON(&envelope); err == nil {
		t.Fatalf("unexpected realtime envelope: %#v", envelope)
	}
}

func sendAppRequest(t *testing.T, conn *websocket.Conn, request realtime.Envelope) realtime.Envelope {
	t.Helper()

	response := sendRawAppRequest(t, conn, request)
	if response.OK == nil || !*response.OK {
		t.Fatalf("response ok = %#v, error = %#v", response.OK, response.Error)
	}

	return response
}

func requireReplayedAppResponse(t *testing.T, conn *websocket.Conn, request realtime.Envelope, first realtime.Envelope) {
	t.Helper()

	replayed := sendAppRequest(t, conn, request)
	firstJSON, err := json.Marshal(first)
	if err != nil {
		t.Fatalf("marshal first app response: %v", err)
	}
	replayedJSON, err := json.Marshal(replayed)
	if err != nil {
		t.Fatalf("marshal replayed app response: %v", err)
	}
	if !bytes.Equal(replayedJSON, firstJSON) {
		t.Fatalf("replayed response = %s, want %s", replayedJSON, firstJSON)
	}
}

func ackAppEvent(t *testing.T, conn *websocket.Conn, cursor int64) {
	t.Helper()

	payload, err := json.Marshal(map[string]any{"cursor": cursor})
	if err != nil {
		t.Fatalf("marshal app event ack: %v", err)
	}
	sendAppRequest(t, conn, realtime.Envelope{
		V:       realtime.ProtocolVersion,
		Kind:    realtime.KindRequest,
		ID:      "ack-event-" + uuid.NewString(),
		Method:  appMethodEventsAck,
		Payload: payload,
	})
}

func sendRawAppRequest(t *testing.T, conn *websocket.Conn, request realtime.Envelope) realtime.Envelope {
	t.Helper()

	if err := conn.WriteJSON(request); err != nil {
		t.Fatalf("WriteJSON() error = %v", err)
	}

	response := readRealtimeEvent(t, conn)
	if response.Kind != realtime.KindResponse {
		t.Fatalf("response kind = %v, want response: %#v", response.Kind, response)
	}
	if response.ReplyTo != request.ID {
		t.Fatalf("response reply_to = %v, want %s", response.ReplyTo, request.ID)
	}
	return response
}

func requireAppSendMessageResponsePayload(t *testing.T, response realtime.Envelope) map[string]any {
	t.Helper()

	var payload map[string]any
	if err := json.Unmarshal(response.Payload, &payload); err != nil {
		t.Fatalf("unmarshal app response payload: %v", err)
	}
	if payload["created"] != true {
		t.Fatalf("created = %v, want true", payload["created"])
	}
	if _, ok := payload["conversation"].(map[string]any); !ok {
		t.Fatalf("conversation = %#v, want object", payload["conversation"])
	}
	if _, ok := payload["message"].(map[string]any); !ok {
		t.Fatalf("message = %#v, want object", payload["message"])
	}

	return payload
}

func requireAppErrorResponse(t *testing.T, response realtime.Envelope, code string) {
	t.Helper()

	if response.OK == nil || *response.OK {
		t.Fatalf("response ok = %#v, want false", response.OK)
	}
	if response.Error == nil {
		t.Fatalf("response error = nil, want %s", code)
	}
	if response.Error.Code != code {
		t.Fatalf("response error code = %q, want %q: %s", response.Error.Code, code, response.Error.Message)
	}
}

func mustMarshalPayloadForTest(t *testing.T, payload any) json.RawMessage {
	t.Helper()

	content, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	return content
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

func readMessageUpdatedEvent(t *testing.T, conn *websocket.Conn) map[string]any {
	t.Helper()

	envelope := readRealtimeEvent(t, conn)
	if envelope.Kind != realtime.KindEvent || envelope.Event != "message.updated" {
		t.Fatalf("envelope = %#v, want message.updated event", envelope)
	}
	var payload map[string]any
	if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
		t.Fatalf("unmarshal message.updated payload: %v", err)
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

func TestAppWebSocketRequiresValidCredentials(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	_, resp, err := websocket.DefaultDialer.Dial(appWebSocketURL(server), nil)
	if err == nil {
		t.Fatal("Dial() error = nil, want missing app id error")
	}
	if resp == nil || resp.StatusCode != http.StatusBadRequest {
		status := 0
		if resp != nil {
			status = resp.StatusCode
		}
		t.Fatalf("missing app id status = %d, want 400", status)
	}

	header := http.Header{}
	header.Set(appIDHeader, appregistry.AIAssistantAppID)
	header.Set("Authorization", "Bearer wrong-secret")
	_, resp, err = websocket.DefaultDialer.Dial(appWebSocketURL(server), header)
	if err == nil {
		t.Fatal("Dial() error = nil, want unauthorized error")
	}
	if resp == nil || resp.StatusCode != http.StatusUnauthorized {
		status := 0
		if resp != nil {
			status = resp.StatusCode
		}
		t.Fatalf("invalid secret status = %d, want 401", status)
	}

	disabledApp := store.App{
		ID:               uuid.NewString(),
		Name:             "Disabled App",
		Enabled:          true,
		Visibility:       store.AppVisibilityPublic,
		ConnectionSecret: "disabled-secret",
		CreatedAt:        time.Now().UTC(),
		UpdatedAt:        time.Now().UTC(),
	}
	if err := db.Create(&disabledApp).Error; err != nil {
		t.Fatalf("create disabled app: %v", err)
	}
	if err := db.Model(&disabledApp).Update("enabled", false).Error; err != nil {
		t.Fatalf("disable app: %v", err)
	}
	header.Set(appIDHeader, disabledApp.ID)
	header.Set("Authorization", "Bearer disabled-secret")
	_, resp, err = websocket.DefaultDialer.Dial(appWebSocketURL(server), header)
	if err == nil {
		t.Fatal("Dial() error = nil, want forbidden error")
	}
	if resp == nil || resp.StatusCode != http.StatusForbidden {
		status := 0
		if resp != nil {
			status = resp.StatusCode
		}
		t.Fatalf("disabled app status = %d, want 403", status)
	}
}

func TestAppWebSocketTracksAdminConnectionStatus(t *testing.T) {
	server, _ := newTestRouter(t)
	defer server.Close()
	adminCookie := loginAsAdmin(t, server)

	if status := requireAdminAppConnectionStatus(t, server, adminCookie, appregistry.AIAssistantAppID); status != "offline" {
		t.Fatalf("initial connection_status = %v, want offline", status)
	}

	header := http.Header{}
	header.Set(appIDHeader, appregistry.AIAssistantAppID)
	header.Set("Authorization", "Bearer test-ai-assistant-secret")
	conn, resp, err := websocket.DefaultDialer.Dial(appWebSocketURL(server), header)
	if err != nil {
		status := 0
		if resp != nil {
			status = resp.StatusCode
		}
		t.Fatalf("Dial() error = %v, status = %d", err, status)
	}

	if status := requireAdminAppConnectionStatus(t, server, adminCookie, appregistry.AIAssistantAppID); status != "online" {
		t.Fatalf("connected connection_status = %v, want online", status)
	}

	_ = conn.Close()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if status := requireAdminAppConnectionStatus(t, server, adminCookie, appregistry.AIAssistantAppID); status == "offline" {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("connection_status did not become offline after websocket close")
}

func TestUserMessageRollsBackWhenAppEventOutboxInsertFails(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	now := time.Date(2026, 7, 10, 9, 0, 0, 0, time.UTC)
	alice := insertTestUser(t, db, "alice@example.com", "Alice", store.UserStatusActive, now)
	app := insertTestApp(t, db, store.App{
		Name:      "Echo App",
		Enabled:   true,
		CreatedAt: now,
		UpdatedAt: now,
	})
	conversation := insertTestConversation(t, db, testConversationInput{
		createdByUserID: alice.ID,
		kind:            store.ConversationKindApp,
		memberIDs:       []string{alice.ID},
		name:            app.Name,
		now:             now,
	})
	insertTestAppConversationLink(t, db, app.ID, alice.ID, conversation.ID, now)

	const callbackName = "test:fail_app_event_outbox_create"
	if err := db.Callback().Create().Before("gorm:create").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Table == "app_event_outbox" {
			tx.AddError(errors.New("forced outbox failure"))
		}
	}); err != nil {
		t.Fatalf("register create callback: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Callback().Create().Remove(callbackName); err != nil {
			t.Errorf("remove create callback: %v", err)
		}
	})

	subject := &Server{db: db}
	_, _, _, _, err := subject.createUserMessageWithMetadata(
		context.Background(),
		alice.ID,
		conversation.ID,
		"message-1",
		json.RawMessage(`{"type":"text","content":"hello"}`),
		staticMessageBodyFinalizer("hello"),
		createMessageMetadata{EmitAppEvent: true},
	)
	if err == nil {
		t.Fatal("createUserMessageWithMetadata() error = nil, want forced outbox failure")
	}

	var messageCount int64
	if err := db.Model(&store.Message{}).Count(&messageCount).Error; err != nil {
		t.Fatalf("count messages: %v", err)
	}
	if messageCount != 0 {
		t.Fatalf("message count = %d, want 0", messageCount)
	}
	var outboxCount int64
	if err := db.Model(&store.AppEventOutbox{}).Count(&outboxCount).Error; err != nil {
		t.Fatalf("count app event outbox: %v", err)
	}
	if outboxCount != 0 {
		t.Fatalf("app event outbox count = %d, want 0", outboxCount)
	}
}

func TestConcurrentAppMessagesPersistOutboxInSequenceOrder(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	now := time.Date(2026, 7, 10, 9, 0, 0, 0, time.UTC)
	alice := insertTestUser(t, db, "alice@example.com", "Alice", store.UserStatusActive, now)
	app := insertTestApp(t, db, store.App{
		Name:      "Echo App",
		Enabled:   true,
		CreatedAt: now,
		UpdatedAt: now,
	})
	conversation := insertTestConversation(t, db, testConversationInput{
		createdByUserID: alice.ID,
		kind:            store.ConversationKindApp,
		memberIDs:       []string{alice.ID},
		name:            app.Name,
		now:             now,
	})
	insertTestAppConversationLink(t, db, app.ID, alice.ID, conversation.ID, now)

	firstCommitted := make(chan struct{})
	releaseFirst := make(chan struct{})
	secondReachedEventLock := make(chan struct{})
	defer func() {
		select {
		case <-releaseFirst:
		default:
			close(releaseFirst)
		}
	}()

	subject := &Server{db: db}
	subject.afterUserMessageCommit = func(message store.Message) {
		if message.Seq == 1 {
			close(firstCommitted)
			<-releaseFirst
		}
	}
	subject.beforeAppEventLock = func(message store.Message) {
		if message.Seq == 2 {
			close(secondReachedEventLock)
		}
	}

	createMessage := func(clientMessageID string) error {
		_, _, _, _, err := subject.createUserMessageWithMetadata(
			context.Background(),
			alice.ID,
			conversation.ID,
			clientMessageID,
			json.RawMessage(`{"type":"text","content":"hello"}`),
			staticMessageBodyFinalizer("hello"),
			createMessageMetadata{EmitAppEvent: true},
		)
		return err
	}

	errs := make(chan error, 2)
	go func() { errs <- createMessage("message-1") }()
	select {
	case <-firstCommitted:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for first message commit")
	}

	go func() { errs <- createMessage("message-2") }()
	select {
	case <-secondReachedEventLock:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for second message to reach app event lock")
	}
	close(releaseFirst)

	for i := 0; i < 2; i++ {
		select {
		case err := <-errs:
			if err != nil {
				t.Fatalf("create message error = %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for message creation")
		}
	}

	var events []store.AppEventOutbox
	if err := db.Order("id ASC").Find(&events).Error; err != nil {
		t.Fatalf("load app event outbox: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("app event outbox count = %d, want 2", len(events))
	}
	seqs := make([]int64, 0, len(events))
	for _, event := range events {
		var payload appMessageCreatedPayload
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			t.Fatalf("unmarshal app event payload: %v", err)
		}
		seqs = append(seqs, payload.Message.Seq)
	}
	if !slices.Equal(seqs, []int64{1, 2}) {
		t.Fatalf("app event message seqs = %v, want [1 2]", seqs)
	}
}

func TestAppWebSocketReceivesTextMessageEvents(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	now := time.Date(2026, 7, 3, 9, 0, 0, 0, time.UTC)
	alice := insertTestUser(t, db, "alice@example.com", "Alice", store.UserStatusActive, now)
	if err := db.Model(&alice).Update("nickname", "Al").Error; err != nil {
		t.Fatalf("set alice nickname: %v", err)
	}
	alice.Nickname = "Al"
	app := insertTestApp(t, db, store.App{
		Name:             "Echo App",
		Enabled:          true,
		Visibility:       store.AppVisibilityPublic,
		ConnectionSecret: "echo-app-secret",
		CreatedAt:        now,
		UpdatedAt:        now,
	})
	userCookie := loginAsUser(t, server, alice.Email)
	appConn := dialAppWebSocket(t, server, app.ID, app.ConnectionSecret)

	createConversationResp, createConversationBody := postJSON(t, server, "/api/client/conversations/apps", map[string]any{
		"app_id": app.ID,
	}, userCookie)
	if createConversationResp.StatusCode != http.StatusCreated {
		t.Fatalf("create app conversation status = %d, want 201, body = %#v", createConversationResp.StatusCode, createConversationBody)
	}
	conversation := requireSuccess(t, createConversationBody)["conversation"].(map[string]any)
	conversationID := conversation["id"].(string)

	resp, body := postJSON(t, server, "/api/client/conversations/"+conversationID+"/messages", map[string]any{
		"client_message_id": "client-message-1",
		"body": map[string]any{
			"type":    "text",
			"content": "  你好，应用  ",
		},
	}, userCookie)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("send text status = %d, want 201, body = %#v", resp.StatusCode, body)
	}
	createdMessage := requireSuccess(t, body)["message"].(map[string]any)

	event := readRealtimeEvent(t, appConn)
	if event.Kind != realtime.KindEvent || event.Event != realtime.EventMessageCreated {
		t.Fatalf("app event = %#v, want message.created event", event)
	}
	if event.Cursor <= 0 {
		t.Fatalf("app event cursor = %d, want positive cursor", event.Cursor)
	}
	duplicateResp, duplicateBody := postJSON(t, server, "/api/client/conversations/"+conversationID+"/messages", map[string]any{
		"client_message_id": "client-message-1",
		"body": map[string]any{
			"type":    "text",
			"content": "  你好，应用  ",
		},
	}, userCookie)
	if duplicateResp.StatusCode != http.StatusOK {
		t.Fatalf("duplicate text status = %d, want 200, body = %#v", duplicateResp.StatusCode, duplicateBody)
	}
	var outboxCount int64
	if err := db.Model(&store.AppEventOutbox{}).Where("app_id = ?", app.ID).Count(&outboxCount).Error; err != nil {
		t.Fatalf("count app event outbox: %v", err)
	}
	if outboxCount != 1 {
		t.Fatalf("app event outbox count = %d, want 1", outboxCount)
	}
	var payload map[string]any
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		t.Fatalf("unmarshal app event payload: %v", err)
	}
	eventConversation := payload["conversation"].(map[string]any)
	if eventConversation["id"] != conversationID {
		t.Fatalf("conversation.id = %v, want %s", eventConversation["id"], conversationID)
	}
	if eventConversation["name"] != app.Name {
		t.Fatalf("conversation.name = %v, want %s", eventConversation["name"], app.Name)
	}
	if eventConversation["type"] != store.ConversationKindApp {
		t.Fatalf("conversation.type = %v, want app", eventConversation["type"])
	}

	sender := payload["sender"].(map[string]any)
	if _, ok := sender["avatar"]; ok {
		t.Fatalf("sender.avatar = %v, want omitted", sender["avatar"])
	}
	if sender["type"] != store.MessageSenderTypeUser {
		t.Fatalf("sender.type = %v, want user", sender["type"])
	}
	if sender["id"] != alice.ID {
		t.Fatalf("sender.id = %v, want %s", sender["id"], alice.ID)
	}
	if sender["name"] != alice.Name {
		t.Fatalf("sender.name = %v, want %s", sender["name"], alice.Name)
	}
	if sender["nickname"] != alice.Nickname {
		t.Fatalf("sender.nickname = %v, want %s", sender["nickname"], alice.Nickname)
	}

	message := payload["message"].(map[string]any)
	if message["id"] != createdMessage["id"] {
		t.Fatalf("message.id = %v, want %v", message["id"], createdMessage["id"])
	}

	replayConn := dialAppWebSocket(t, server, app.ID, app.ConnectionSecret)
	replayed := readRealtimeEvent(t, replayConn)
	if replayed.Cursor != event.Cursor || replayed.Event != event.Event || string(replayed.Payload) != string(event.Payload) {
		t.Fatalf("replayed event = %#v, want cursor %d payload %s", replayed, event.Cursor, event.Payload)
	}
	requireNoRealtimeEvent(t, appConn)
	_ = appConn.Close()
	ackAppEvent(t, replayConn, event.Cursor)
	_ = replayConn.Close()
	afterAckConn := dialAppWebSocket(t, server, app.ID, app.ConnectionSecret)
	requireNoRealtimeEvent(t, afterAckConn)
	if _, ok := message["conversation_id"]; ok {
		t.Fatalf("message.conversation_id = %v, want omitted", message["conversation_id"])
	}
	messageBody := message["body"].(map[string]any)
	if messageBody["type"] != messageTypeText {
		t.Fatalf("message.body.type = %v, want text", messageBody["type"])
	}
	if messageBody["content"] != "你好，应用" {
		t.Fatalf("message.body.content = %v, want normalized text", messageBody["content"])
	}
	if message["summary"] != "你好，应用" {
		t.Fatalf("message.summary = %v, want normalized summary", message["summary"])
	}
	if message["seq"] != float64(1) {
		t.Fatalf("message.seq = %v, want 1", message["seq"])
	}
	if message["created_at"] == "" {
		t.Fatalf("message.created_at = %#v, want non-empty", message["created_at"])
	}
}

func TestAppWebSocketReplaysOutboxInBoundedPages(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	now := time.Date(2026, 7, 10, 6, 0, 0, 0, time.UTC)
	app := insertTestApp(t, db, store.App{
		Name:             "Echo App",
		Enabled:          true,
		Visibility:       store.AppVisibilityPublic,
		ConnectionSecret: "echo-app-secret",
		CreatedAt:        now,
		UpdatedAt:        now,
	})
	events := make([]store.AppEventOutbox, 205)
	for i := range events {
		events[i] = store.AppEventOutbox{
			AppID:     app.ID,
			Event:     realtime.EventMessageCreated,
			Payload:   json.RawMessage(`{"message":"queued"}`),
			CreatedAt: now.Add(time.Duration(i) * time.Second),
		}
	}
	if err := db.Create(&events).Error; err != nil {
		t.Fatalf("create app events: %v", err)
	}

	const callbackName = "test:capture_app_event_outbox_query_limits"
	queryLimits := make(chan int, 8)
	if err := db.Callback().Query().Before("gorm:query").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Table != "app_event_outbox" {
			return
		}
		limit := -1
		if limitClause, ok := tx.Statement.Clauses["LIMIT"].Expression.(clause.Limit); ok && limitClause.Limit != nil {
			limit = *limitClause.Limit
		}
		queryLimits <- limit
	}); err != nil {
		t.Fatalf("register query callback: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Callback().Query().Remove(callbackName); err != nil {
			t.Errorf("remove query callback: %v", err)
		}
	})

	conn := dialAppWebSocket(t, server, app.ID, app.ConnectionSecret)
	for i := range events {
		replayed := readRealtimeEvent(t, conn)
		if replayed.Cursor != events[i].ID {
			t.Fatalf("replayed cursor %d = %d, want %d", i, replayed.Cursor, events[i].ID)
		}
	}

	limits := make([]int, 0, len(queryLimits))
	for len(queryLimits) > 0 {
		limits = append(limits, <-queryLimits)
	}
	if want := []int{100, 100, 100}; !slices.Equal(limits, want) {
		t.Fatalf("app event outbox query limits = %v, want %v", limits, want)
	}
}

func TestAppWebSocketAckOlderCursorIsIdempotent(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	now := time.Date(2026, 7, 10, 6, 0, 0, 0, time.UTC)
	app := insertTestApp(t, db, store.App{
		Name:             "Echo App",
		Enabled:          true,
		Visibility:       store.AppVisibilityPublic,
		ConnectionSecret: "echo-app-secret",
		CreatedAt:        now,
		UpdatedAt:        now,
	})
	events := []store.AppEventOutbox{
		{AppID: app.ID, Event: realtime.EventMessageCreated, Payload: json.RawMessage(`{"message":"first"}`), CreatedAt: now},
		{AppID: app.ID, Event: realtime.EventMessageCreated, Payload: json.RawMessage(`{"message":"second"}`), CreatedAt: now.Add(time.Second)},
	}
	if err := db.Create(&events).Error; err != nil {
		t.Fatalf("create app events: %v", err)
	}

	conn := dialAppWebSocket(t, server, app.ID, app.ConnectionSecret)
	first := readRealtimeEvent(t, conn)
	second := readRealtimeEvent(t, conn)
	if first.Cursor != events[0].ID || second.Cursor != events[1].ID {
		t.Fatalf("replayed cursors = [%d, %d], want [%d, %d]", first.Cursor, second.Cursor, events[0].ID, events[1].ID)
	}

	ackAppEvent(t, conn, second.Cursor)
	ackAppEvent(t, conn, first.Cursor)

	var ack store.AppEventAck
	if err := db.First(&ack, "app_id = ?", app.ID).Error; err != nil {
		t.Fatalf("load app event ack: %v", err)
	}
	if ack.LastAckedCursor != second.Cursor {
		t.Fatalf("last acked cursor = %d, want %d", ack.LastAckedCursor, second.Cursor)
	}
}

func TestAppWebSocketReceivesGroupMessageOnlyWhenMentionedDirectly(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	now := time.Date(2026, 7, 3, 9, 0, 0, 0, time.UTC)
	alice := insertTestUser(t, db, "alice@example.com", "Alice", store.UserStatusActive, now)
	bob := insertTestUser(t, db, "bob@example.com", "Bob", store.UserStatusActive, now)
	app := insertTestApp(t, db, store.App{
		Name:             "AI 女菩萨",
		Enabled:          true,
		Visibility:       store.AppVisibilityPublic,
		ConnectionSecret: "assistant-secret",
		CreatedAt:        now,
		UpdatedAt:        now,
	})
	otherApp := insertTestApp(t, db, store.App{
		Name:             "Other App",
		Enabled:          true,
		Visibility:       store.AppVisibilityPublic,
		ConnectionSecret: "other-secret",
		CreatedAt:        now,
		UpdatedAt:        now,
	})
	conversation := insertTestConversation(t, db, testConversationInput{
		createdByUserID: alice.ID,
		kind:            store.ConversationKindGroup,
		memberIDs:       []string{alice.ID, bob.ID},
		name:            "产品讨论组",
		now:             now,
	})
	appMembers := []store.ConversationMember{
		{
			ConversationID:        conversation.ID,
			MemberType:            store.ConversationMemberTypeApp,
			MemberID:              app.ID,
			Role:                  store.ConversationMemberRoleMember,
			JoinedAt:              now,
			HistoryVisibleFromSeq: 1,
		},
		{
			ConversationID:        conversation.ID,
			MemberType:            store.ConversationMemberTypeApp,
			MemberID:              otherApp.ID,
			Role:                  store.ConversationMemberRoleMember,
			JoinedAt:              now,
			HistoryVisibleFromSeq: 1,
		},
	}
	if err := db.Create(&appMembers).Error; err != nil {
		t.Fatalf("create app members: %v", err)
	}
	userCookie := loginAsUser(t, server, alice.Email)

	normalConn := dialAppWebSocket(t, server, app.ID, app.ConnectionSecret)
	normalResp, normalBody := postJSON(t, server, "/api/client/conversations/"+conversation.ID+"/messages", map[string]any{
		"client_message_id": "client-message-normal",
		"body": map[string]any{
			"type":    "text",
			"content": "普通群消息",
		},
	}, userCookie)
	if normalResp.StatusCode != http.StatusCreated {
		t.Fatalf("normal message status = %d, want 201, body = %#v", normalResp.StatusCode, normalBody)
	}
	requireNoRealtimeEvent(t, normalConn)

	mentionedConn := dialAppWebSocket(t, server, app.ID, app.ConnectionSecret)
	otherConn := dialAppWebSocket(t, server, otherApp.ID, otherApp.ConnectionSecret)
	mentionedContent := "请看一下 {(@app/" + app.ID + ")}"
	mentionedResp, mentionedBody := postJSON(t, server, "/api/client/conversations/"+conversation.ID+"/messages", map[string]any{
		"client_message_id": "client-message-mention",
		"body": map[string]any{
			"type":    "text",
			"content": mentionedContent,
		},
	}, userCookie)
	if mentionedResp.StatusCode != http.StatusCreated {
		t.Fatalf("mentioned message status = %d, want 201, body = %#v", mentionedResp.StatusCode, mentionedBody)
	}
	event := readRealtimeEvent(t, mentionedConn)
	if event.Kind != realtime.KindEvent || event.Event != realtime.EventMessageCreated {
		t.Fatalf("app event = %#v, want message.created event", event)
	}
	var payload map[string]any
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		t.Fatalf("unmarshal app event payload: %v", err)
	}
	eventConversation := payload["conversation"].(map[string]any)
	if eventConversation["id"] != conversation.ID {
		t.Fatalf("conversation.id = %v, want %s", eventConversation["id"], conversation.ID)
	}
	if eventConversation["type"] != store.ConversationKindGroup {
		t.Fatalf("conversation.type = %v, want group", eventConversation["type"])
	}
	message := payload["message"].(map[string]any)
	if message["summary"] != mentionedContent {
		t.Fatalf("message.summary = %v, want %s", message["summary"], mentionedContent)
	}
	ackAppEvent(t, mentionedConn, event.Cursor)
	requireNoRealtimeEvent(t, otherConn)

	allConn := dialAppWebSocket(t, server, app.ID, app.ConnectionSecret)
	allResp, allBody := postJSON(t, server, "/api/client/conversations/"+conversation.ID+"/messages", map[string]any{
		"client_message_id": "client-message-all",
		"body": map[string]any{
			"type":    "text",
			"content": "大家看一下 {(@user/all)}",
		},
	}, userCookie)
	if allResp.StatusCode != http.StatusCreated {
		t.Fatalf("all message status = %d, want 201, body = %#v", allResp.StatusCode, allBody)
	}
	requireNoRealtimeEvent(t, allConn)
}

func TestAppWebSocketConversationMessagesListReturnsAuthorizedSummaries(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	now := time.Date(2026, 7, 8, 10, 0, 0, 0, time.UTC)
	alice := insertTestUser(t, db, "alice@example.com", "Alice", store.UserStatusActive, now)
	app := insertTestApp(t, db, store.App{
		Name:             "Echo App",
		Enabled:          true,
		Visibility:       store.AppVisibilityPublic,
		ConnectionSecret: "echo-app-secret",
		CreatedAt:        now,
		UpdatedAt:        now,
	})
	userCookie := loginAsUser(t, server, alice.Email)
	appConn := dialAppWebSocket(t, server, app.ID, app.ConnectionSecret)

	createConversationResp, createConversationBody := postJSON(t, server, "/api/client/conversations/apps", map[string]any{
		"app_id": app.ID,
	}, userCookie)
	if createConversationResp.StatusCode != http.StatusCreated {
		t.Fatalf("create app conversation status = %d, want 201, body = %#v", createConversationResp.StatusCode, createConversationBody)
	}
	conversation := requireSuccess(t, createConversationBody)["conversation"].(map[string]any)
	conversationID := conversation["id"].(string)
	if err := db.Model(&store.ConversationMember{}).
		Where("conversation_id = ? AND member_type = ? AND member_id = ?", conversationID, store.ConversationMemberTypeApp, app.ID).
		Update("history_visible_from_seq", 20).Error; err != nil {
		t.Fatalf("set app member history_visible_from_seq: %v", err)
	}

	for seq := int64(1); seq <= 150; seq++ {
		createdAt := now.Add(time.Duration(seq) * time.Minute)
		if seq == 100 {
			clientMessageID := "app-history-message-100"
			senderID := app.ID
			message := store.Message{
				ID:              uuid.NewString(),
				ConversationID:  conversationID,
				Seq:             seq,
				SenderType:      store.MessageSenderTypeApp,
				SenderID:        &senderID,
				ClientMessageID: &clientMessageID,
				Body:            json.RawMessage(`{"type":"text","content":"assistant full body 100"}`),
				Summary:         "assistant summary 100",
				CreatedAt:       createdAt,
				UpdatedAt:       createdAt,
			}
			if err := db.Create(&message).Error; err != nil {
				t.Fatalf("create app history message: %v", err)
			}
			continue
		}
		if seq == 99 {
			revokedAt := createdAt.Add(time.Minute)
			senderID := alice.ID
			message := store.Message{
				ID:              uuid.NewString(),
				ConversationID:  conversationID,
				Seq:             seq,
				SenderType:      store.MessageSenderTypeUser,
				SenderID:        &senderID,
				Body:            json.RawMessage(`{"type":"text","content":"revoked secret body"}`),
				Summary:         "revoked secret body",
				RevokedAt:       &revokedAt,
				RevokedByUserID: &alice.ID,
				CreatedAt:       createdAt,
				UpdatedAt:       createdAt,
			}
			if err := db.Create(&message).Error; err != nil {
				t.Fatalf("create revoked app history message: %v", err)
			}
			continue
		}
		insertTestMessage(t, db, conversationID, alice.ID, seq, fmt.Sprintf("user summary %03d", seq), createdAt)
	}

	response := sendAppRequest(t, appConn, realtime.Envelope{
		V:      realtime.ProtocolVersion,
		Kind:   realtime.KindRequest,
		ID:     "app-history-request",
		Method: appMethodConversationMessagesList,
		Payload: mustMarshalPayloadForTest(t, map[string]any{
			"conversation_id":     conversationID,
			"before_or_equal_seq": 150,
			"limit":               200,
		}),
	})
	var payload map[string]any
	if err := json.Unmarshal(response.Payload, &payload); err != nil {
		t.Fatalf("unmarshal history response: %v", err)
	}
	if payload["limit"] != float64(100) {
		t.Fatalf("limit = %v, want 100", payload["limit"])
	}
	messages := payload["messages"].([]any)
	if len(messages) != 100 {
		t.Fatalf("message count = %d, want 100", len(messages))
	}
	first := messages[0].(map[string]any)
	if first["seq"] != float64(51) {
		t.Fatalf("first seq = %v, want 51", first["seq"])
	}
	last := messages[len(messages)-1].(map[string]any)
	if last["seq"] != float64(150) {
		t.Fatalf("last seq = %v, want 150", last["seq"])
	}
	appHistoryMessage := messages[49].(map[string]any)
	if appHistoryMessage["seq"] != float64(100) {
		t.Fatalf("app history seq = %v, want 100", appHistoryMessage["seq"])
	}
	if appHistoryMessage["summary"] != "assistant summary 100" {
		t.Fatalf("app history summary = %v, want summary", appHistoryMessage["summary"])
	}
	appHistoryBody := appHistoryMessage["body"].(map[string]any)
	if appHistoryBody["type"] != "text" || appHistoryBody["content"] != "assistant full body 100" {
		t.Fatalf("app history body = %#v, want full body", appHistoryBody)
	}
	appSender := appHistoryMessage["sender"].(map[string]any)
	if appSender["type"] != store.MessageSenderTypeApp {
		t.Fatalf("app sender type = %v, want app", appSender["type"])
	}
	if appSender["name"] != app.Name {
		t.Fatalf("app sender name = %v, want %s", appSender["name"], app.Name)
	}
	revokedHistoryMessage := messages[48].(map[string]any)
	if revokedHistoryMessage["seq"] != float64(99) {
		t.Fatalf("revoked history seq = %v, want 99", revokedHistoryMessage["seq"])
	}
	if revokedHistoryMessage["summary"] != "该消息已被撤回" {
		t.Fatalf("revoked history summary = %v, want revoked summary", revokedHistoryMessage["summary"])
	}
	if _, ok := revokedHistoryMessage["body"]; ok {
		t.Fatalf("revoked history body = %v, want omitted", revokedHistoryMessage["body"])
	}

	earlyResponse := sendAppRequest(t, appConn, realtime.Envelope{
		V:      realtime.ProtocolVersion,
		Kind:   realtime.KindRequest,
		ID:     "app-history-early-window-request",
		Method: appMethodConversationMessagesList,
		Payload: mustMarshalPayloadForTest(t, map[string]any{
			"conversation_id":     conversationID,
			"before_or_equal_seq": 25,
			"limit":               100,
		}),
	})
	var earlyPayload map[string]any
	if err := json.Unmarshal(earlyResponse.Payload, &earlyPayload); err != nil {
		t.Fatalf("unmarshal early history response: %v", err)
	}
	earlyMessages := earlyPayload["messages"].([]any)
	if len(earlyMessages) != 6 {
		t.Fatalf("early message count = %d, want 6", len(earlyMessages))
	}
	earlyFirst := earlyMessages[0].(map[string]any)
	if earlyFirst["seq"] != float64(20) {
		t.Fatalf("early first seq = %v, want 20", earlyFirst["seq"])
	}
	earlyLast := earlyMessages[len(earlyMessages)-1].(map[string]any)
	if earlyLast["seq"] != float64(25) {
		t.Fatalf("early last seq = %v, want 25", earlyLast["seq"])
	}
}

func TestAppWebSocketConversationMessagesListRejectsNonMemberApp(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	now := time.Date(2026, 7, 8, 10, 0, 0, 0, time.UTC)
	alice := insertTestUser(t, db, "alice@example.com", "Alice", store.UserStatusActive, now)
	app := insertTestApp(t, db, store.App{
		Name:             "Echo App",
		Enabled:          true,
		Visibility:       store.AppVisibilityPublic,
		ConnectionSecret: "echo-app-secret",
		CreatedAt:        now,
		UpdatedAt:        now,
	})
	appConn := dialAppWebSocket(t, server, app.ID, app.ConnectionSecret)
	group := insertTestConversation(t, db, testConversationInput{
		createdByUserID: alice.ID,
		kind:            store.ConversationKindGroup,
		memberIDs:       []string{alice.ID},
		name:            "产品讨论组",
		now:             now,
	})

	request := realtime.Envelope{
		V:      realtime.ProtocolVersion,
		Kind:   realtime.KindRequest,
		ID:     "app-history-forbidden",
		Method: appMethodConversationMessagesList,
		Payload: mustMarshalPayloadForTest(t, map[string]any{
			"conversation_id":     group.ID,
			"before_or_equal_seq": 1,
			"limit":               30,
		}),
	}
	if err := appConn.WriteJSON(request); err != nil {
		t.Fatalf("WriteJSON() error = %v", err)
	}
	response := readRealtimeEvent(t, appConn)
	if response.Kind != realtime.KindResponse || response.ReplyTo != request.ID {
		t.Fatalf("response = %#v, want matching response", response)
	}
	if response.OK == nil || *response.OK {
		t.Fatalf("response ok = %#v, want false", response.OK)
	}
	if response.Error == nil || response.Error.Code != "forbidden" {
		t.Fatalf("response error = %#v, want forbidden", response.Error)
	}
}

func TestAppWebSocketContactsUsersListReturnsActiveUsersOnly(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	now := time.Date(2026, 7, 8, 10, 0, 0, 0, time.UTC)
	alice := insertTestUser(t, db, "alice@example.com", "Alice", store.UserStatusActive, now)
	if err := db.Model(&alice).Update("nickname", "Al").Error; err != nil {
		t.Fatalf("set alice nickname: %v", err)
	}
	bob := insertTestUser(t, db, "bob@example.com", "Bob", store.UserStatusActive, now)
	_ = insertTestUser(t, db, "disabled@example.com", "Disabled", store.UserStatusDisabled, now)
	app := insertTestApp(t, db, store.App{
		Name:             "Echo App",
		Enabled:          true,
		Visibility:       store.AppVisibilityPublic,
		ConnectionSecret: "echo-app-secret",
		CreatedAt:        now,
		UpdatedAt:        now,
	})
	appConn := dialAppWebSocket(t, server, app.ID, app.ConnectionSecret)

	response := sendAppRequest(t, appConn, realtime.Envelope{
		V:      realtime.ProtocolVersion,
		Kind:   realtime.KindRequest,
		ID:     "app-contacts-request",
		Method: appMethodContactsUsersList,
		Payload: mustMarshalPayloadForTest(t, map[string]any{
			"keyword": "ali",
		}),
	})
	var payload map[string]any
	if err := json.Unmarshal(response.Payload, &payload); err != nil {
		t.Fatalf("unmarshal contacts response: %v", err)
	}
	contacts := payload["contacts"].([]any)
	if len(contacts) != 1 {
		t.Fatalf("contact count = %d, want 1", len(contacts))
	}
	contact := contacts[0].(map[string]any)
	if contact["id"] != alice.ID {
		t.Fatalf("contact.id = %v, want %s", contact["id"], alice.ID)
	}
	if contact["nickname"] != "Al" {
		t.Fatalf("contact.nickname = %v, want Al", contact["nickname"])
	}
	if contact["type"] != "user" {
		t.Fatalf("contact.type = %v, want user", contact["type"])
	}

	response = sendAppRequest(t, appConn, realtime.Envelope{
		V:       realtime.ProtocolVersion,
		Kind:    realtime.KindRequest,
		ID:      "app-contacts-all-request",
		Method:  appMethodContactsUsersList,
		Payload: mustMarshalPayloadForTest(t, map[string]any{}),
	})
	if err := json.Unmarshal(response.Payload, &payload); err != nil {
		t.Fatalf("unmarshal all contacts response: %v", err)
	}
	contacts = payload["contacts"].([]any)
	contactIDs := make(map[string]bool, len(contacts))
	for _, rawContact := range contacts {
		contact := rawContact.(map[string]any)
		contactIDs[contact["id"].(string)] = true
		if contact["type"] != "user" {
			t.Fatalf("contact.type = %v, want user", contact["type"])
		}
	}
	if !contactIDs[alice.ID] || !contactIDs[bob.ID] {
		t.Fatalf("contact ids = %#v, want alice and bob", contactIDs)
	}
}

func TestAppWebSocketMessageSendAsUserStoresDelegatedByAndPushesDirectMessage(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	now := time.Date(2026, 7, 8, 10, 0, 0, 0, time.UTC)
	alice := insertTestUser(t, db, "alice@example.com", "Alice", store.UserStatusActive, now)
	bob := insertTestUser(t, db, "bob@example.com", "Bob", store.UserStatusActive, now)
	app := insertTestApp(t, db, store.App{
		Name:             "女菩萨",
		Enabled:          true,
		Visibility:       store.AppVisibilityPublic,
		ConnectionSecret: "assistant-secret",
		CreatedAt:        now,
		UpdatedAt:        now,
	})
	aliceCookie := loginAsUser(t, server, alice.Email)
	bobCookie := loginAsUser(t, server, bob.Email)
	bobConn := dialClientWebSocket(t, server, bobCookie)
	if ready := readRealtimeEvent(t, bobConn); ready.Kind != realtime.KindEvent || ready.Event != realtime.EventSystemReady {
		t.Fatalf("ready envelope = %#v, want system.ready", ready)
	}
	appConn := dialAppWebSocket(t, server, app.ID, app.ConnectionSecret)

	createConversationResp, createConversationBody := postJSON(t, server, "/api/client/conversations/apps", map[string]any{
		"app_id": app.ID,
	}, aliceCookie)
	if createConversationResp.StatusCode != http.StatusCreated {
		t.Fatalf("create app conversation status = %d, want 201, body = %#v", createConversationResp.StatusCode, createConversationBody)
	}
	appConversation := requireSuccess(t, createConversationBody)["conversation"].(map[string]any)
	appConversationID := appConversation["id"].(string)

	triggerResp, triggerBody := postJSON(t, server, "/api/client/conversations/"+appConversationID+"/messages", map[string]any{
		"client_message_id": "trigger-message-1",
		"body": map[string]any{
			"type":    "text",
			"content": "帮我发给 Bob",
		},
	}, aliceCookie)
	if triggerResp.StatusCode != http.StatusCreated {
		t.Fatalf("trigger message status = %d, want 201, body = %#v", triggerResp.StatusCode, triggerBody)
	}
	triggerMessage := requireSuccess(t, triggerBody)["message"].(map[string]any)
	triggerEvent := readRealtimeEvent(t, appConn)
	if triggerEvent.Kind != realtime.KindEvent || triggerEvent.Event != realtime.EventMessageCreated {
		t.Fatalf("trigger app event = %#v, want message.created", triggerEvent)
	}
	var triggerAppPayload map[string]any
	if err := json.Unmarshal(triggerEvent.Payload, &triggerAppPayload); err != nil {
		t.Fatalf("unmarshal trigger app payload: %v", err)
	}
	triggerSender := triggerAppPayload["sender"].(map[string]any)
	if triggerSender["email"] != alice.Email {
		t.Fatalf("trigger sender email = %v, want %s", triggerSender["email"], alice.Email)
	}

	request := realtime.Envelope{
		V:      realtime.ProtocolVersion,
		Kind:   realtime.KindRequest,
		ID:     "app-send-as-user-request",
		Method: appMethodMessageSendAsUser,
		Payload: mustMarshalPayloadForTest(t, map[string]any{
			"actor_user_id":      alice.ID,
			"target_user_id":     bob.ID,
			"trigger_message_id": triggerMessage["id"],
			"message": map[string]any{
				"type":    "markdown",
				"content": "**请看这个报告**",
			},
		}),
	}
	response := sendAppRequest(t, appConn, request)
	payload := requireAppSendMessageResponsePayload(t, response)
	conversation := payload["conversation"].(map[string]any)
	if conversation["type"] != store.ConversationKindDirect {
		t.Fatalf("conversation.type = %v, want direct", conversation["type"])
	}
	message := payload["message"].(map[string]any)
	messageBody := message["body"].(map[string]any)
	if messageBody["type"] != messageTypeMarkdown {
		t.Fatalf("message.body.type = %v, want markdown", messageBody["type"])
	}
	sender := message["sender"].(map[string]any)
	if sender["type"] != store.MessageSenderTypeUser || sender["id"] != alice.ID {
		t.Fatalf("message.sender = %#v, want Alice user sender", sender)
	}
	delegatedBy := message["delegated_by"].(map[string]any)
	if delegatedBy["type"] != store.MessageSenderTypeApp || delegatedBy["id"] != app.ID || delegatedBy["name"] != app.Name {
		t.Fatalf("message.delegated_by = %#v, want app delegate", delegatedBy)
	}

	pushedMessage := readMessageCreatedEvent(t, bobConn)
	if pushedMessage["id"] != message["id"] {
		t.Fatalf("pushed message id = %v, want %v", pushedMessage["id"], message["id"])
	}
	pushedDelegatedBy := pushedMessage["delegated_by"].(map[string]any)
	if pushedDelegatedBy["type"] != store.MessageSenderTypeApp || pushedDelegatedBy["id"] != app.ID || pushedDelegatedBy["name"] != app.Name {
		t.Fatalf("pushed delegated_by = %#v, want app delegate", pushedDelegatedBy)
	}
	requireReplayedAppResponse(t, appConn, request, response)
	requireNoRealtimeEvent(t, bobConn)

	var storedMessage store.Message
	if err := db.First(&storedMessage, "id = ?", message["id"]).Error; err != nil {
		t.Fatalf("find stored message: %v", err)
	}
	if storedMessage.SenderType != store.MessageSenderTypeUser || storedMessage.SenderID == nil || *storedMessage.SenderID != alice.ID {
		t.Fatalf("stored sender = %s/%v, want Alice", storedMessage.SenderType, storedMessage.SenderID)
	}
	if storedMessage.DelegatedByType == nil || *storedMessage.DelegatedByType != store.MessageSenderTypeApp {
		t.Fatalf("stored delegated_by_type = %v, want app", storedMessage.DelegatedByType)
	}
	if storedMessage.DelegatedByID == nil || *storedMessage.DelegatedByID != app.ID {
		t.Fatalf("stored delegated_by_id = %v, want %s", storedMessage.DelegatedByID, app.ID)
	}

	historyResp, historyBody := getJSON(t, server, "/api/client/conversations/"+conversation["id"].(string)+"/messages", bobCookie)
	if historyResp.StatusCode != http.StatusOK {
		t.Fatalf("history status = %d, want 200, body = %#v", historyResp.StatusCode, historyBody)
	}
	messages := requireMessages(t, requireSuccess(t, historyBody))
	historyMessage := messages[len(messages)-1].(map[string]any)
	historyDelegatedBy := historyMessage["delegated_by"].(map[string]any)
	if historyDelegatedBy["type"] != store.MessageSenderTypeApp || historyDelegatedBy["name"] != app.Name {
		t.Fatalf("history delegated_by = %#v, want app delegate", historyDelegatedBy)
	}
}

func TestAppWebSocketMessageSendAsUserRejectsSpoofedActor(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	now := time.Date(2026, 7, 8, 10, 0, 0, 0, time.UTC)
	alice := insertTestUser(t, db, "alice@example.com", "Alice", store.UserStatusActive, now)
	bob := insertTestUser(t, db, "bob@example.com", "Bob", store.UserStatusActive, now)
	carol := insertTestUser(t, db, "carol@example.com", "Carol", store.UserStatusActive, now)
	app := insertTestApp(t, db, store.App{
		Name:             "女菩萨",
		Enabled:          true,
		Visibility:       store.AppVisibilityPublic,
		ConnectionSecret: "assistant-secret",
		CreatedAt:        now,
		UpdatedAt:        now,
	})
	aliceCookie := loginAsUser(t, server, alice.Email)
	appConn := dialAppWebSocket(t, server, app.ID, app.ConnectionSecret)

	createConversationResp, createConversationBody := postJSON(t, server, "/api/client/conversations/apps", map[string]any{
		"app_id": app.ID,
	}, aliceCookie)
	if createConversationResp.StatusCode != http.StatusCreated {
		t.Fatalf("create app conversation status = %d, want 201, body = %#v", createConversationResp.StatusCode, createConversationBody)
	}
	appConversation := requireSuccess(t, createConversationBody)["conversation"].(map[string]any)
	triggerResp, triggerBody := postJSON(t, server, "/api/client/conversations/"+appConversation["id"].(string)+"/messages", map[string]any{
		"client_message_id": "trigger-message-1",
		"body": map[string]any{
			"type":    "text",
			"content": "帮我发给 Bob",
		},
	}, aliceCookie)
	if triggerResp.StatusCode != http.StatusCreated {
		t.Fatalf("trigger message status = %d, want 201, body = %#v", triggerResp.StatusCode, triggerBody)
	}
	triggerMessage := requireSuccess(t, triggerBody)["message"].(map[string]any)
	triggerEvent := readRealtimeEvent(t, appConn)
	if triggerEvent.Kind != realtime.KindEvent || triggerEvent.Event != realtime.EventMessageCreated {
		t.Fatalf("trigger app event = %#v, want message.created", triggerEvent)
	}

	request := realtime.Envelope{
		V:      realtime.ProtocolVersion,
		Kind:   realtime.KindRequest,
		ID:     "app-send-as-user-spoofed",
		Method: appMethodMessageSendAsUser,
		Payload: mustMarshalPayloadForTest(t, map[string]any{
			"actor_user_id":      bob.ID,
			"target_user_id":     carol.ID,
			"trigger_message_id": triggerMessage["id"],
			"message": map[string]any{
				"type":    "text",
				"content": "伪造 Bob 发送",
			},
		}),
	}
	if err := appConn.WriteJSON(request); err != nil {
		t.Fatalf("WriteJSON() error = %v", err)
	}
	response := readRealtimeEvent(t, appConn)
	if response.Kind != realtime.KindResponse || response.ReplyTo != request.ID {
		t.Fatalf("response = %#v, want matching response", response)
	}
	if response.OK == nil || *response.OK {
		t.Fatalf("response ok = %#v, want false", response.OK)
	}
	if response.Error == nil || response.Error.Code != "forbidden" {
		t.Fatalf("response error = %#v, want forbidden", response.Error)
	}
}

func TestAppWebSocketMessageSendAsUserRejectsAuthorizationConversationMismatch(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	now := time.Date(2026, 7, 8, 10, 0, 0, 0, time.UTC)
	alice := insertTestUser(t, db, "alice@example.com", "Alice", store.UserStatusActive, now)
	bob := insertTestUser(t, db, "bob@example.com", "Bob", store.UserStatusActive, now)
	app := insertTestApp(t, db, store.App{
		Name:             "女菩萨",
		Enabled:          true,
		Visibility:       store.AppVisibilityPublic,
		ConnectionSecret: "assistant-secret",
		CreatedAt:        now,
		UpdatedAt:        now,
	})
	aliceCookie := loginAsUser(t, server, alice.Email)
	appConn := dialAppWebSocket(t, server, app.ID, app.ConnectionSecret)

	createConversationResp, createConversationBody := postJSON(t, server, "/api/client/conversations/apps", map[string]any{
		"app_id": app.ID,
	}, aliceCookie)
	if createConversationResp.StatusCode != http.StatusCreated {
		t.Fatalf("create app conversation status = %d, want 201, body = %#v", createConversationResp.StatusCode, createConversationBody)
	}
	appConversation := requireSuccess(t, createConversationBody)["conversation"].(map[string]any)
	triggerResp, triggerBody := postJSON(t, server, "/api/client/conversations/"+appConversation["id"].(string)+"/messages", map[string]any{
		"client_message_id": "trigger-mismatch-1",
		"body": map[string]any{
			"type":    "text",
			"content": "帮我发给 Bob",
		},
	}, aliceCookie)
	if triggerResp.StatusCode != http.StatusCreated {
		t.Fatalf("trigger message status = %d, want 201, body = %#v", triggerResp.StatusCode, triggerBody)
	}
	triggerMessage := requireSuccess(t, triggerBody)["message"].(map[string]any)
	triggerEvent := readRealtimeEvent(t, appConn)
	if triggerEvent.Kind != realtime.KindEvent || triggerEvent.Event != realtime.EventMessageCreated {
		t.Fatalf("trigger app event = %#v, want message.created", triggerEvent)
	}

	request := realtime.Envelope{
		V:      realtime.ProtocolVersion,
		Kind:   realtime.KindRequest,
		ID:     "app-send-as-user-auth-conversation-mismatch",
		Method: appMethodMessageSendAsUser,
		Payload: mustMarshalPayloadForTest(t, map[string]any{
			"actor_user_id":                 alice.ID,
			"authorization_conversation_id": uuid.NewString(),
			"target_user_id":                bob.ID,
			"trigger_message_id":            triggerMessage["id"],
			"message": map[string]any{
				"type":    "text",
				"content": "不应该发送",
			},
		}),
	}
	if err := appConn.WriteJSON(request); err != nil {
		t.Fatalf("WriteJSON() error = %v", err)
	}
	response := readRealtimeEvent(t, appConn)
	if response.Kind != realtime.KindResponse || response.ReplyTo != request.ID {
		t.Fatalf("response = %#v, want matching response", response)
	}
	if response.OK == nil || *response.OK {
		t.Fatalf("response ok = %#v, want false", response.OK)
	}
	if response.Error == nil || response.Error.Code != "forbidden" {
		t.Fatalf("response error = %#v, want forbidden", response.Error)
	}
}

func TestAppWebSocketGroupConversationCreateUsesTriggeringUser(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	now := time.Date(2026, 7, 8, 10, 0, 0, 0, time.UTC)
	alice := insertTestUser(t, db, "alice@example.com", "Alice", store.UserStatusActive, now)
	bob := insertTestUser(t, db, "bob@example.com", "Bob", store.UserStatusActive, now)
	carol := insertTestUser(t, db, "carol@example.com", "Carol", store.UserStatusActive, now)
	app := insertTestApp(t, db, store.App{
		Name:             "女菩萨",
		Enabled:          true,
		Visibility:       store.AppVisibilityPublic,
		ConnectionSecret: "assistant-secret",
		CreatedAt:        now,
		UpdatedAt:        now,
	})
	aliceCookie := loginAsUser(t, server, alice.Email)
	appConn := dialAppWebSocket(t, server, app.ID, app.ConnectionSecret)

	createConversationResp, createConversationBody := postJSON(t, server, "/api/client/conversations/apps", map[string]any{
		"app_id": app.ID,
	}, aliceCookie)
	if createConversationResp.StatusCode != http.StatusCreated {
		t.Fatalf("create app conversation status = %d, want 201, body = %#v", createConversationResp.StatusCode, createConversationBody)
	}
	appConversation := requireSuccess(t, createConversationBody)["conversation"].(map[string]any)
	triggerResp, triggerBody := postJSON(t, server, "/api/client/conversations/"+appConversation["id"].(string)+"/messages", map[string]any{
		"client_message_id": "trigger-create-group-1",
		"body": map[string]any{
			"type":    "text",
			"content": "帮我建一个项目讨论组，拉 Bob 和 Carol",
		},
	}, aliceCookie)
	if triggerResp.StatusCode != http.StatusCreated {
		t.Fatalf("trigger message status = %d, want 201, body = %#v", triggerResp.StatusCode, triggerBody)
	}
	triggerMessage := requireSuccess(t, triggerBody)["message"].(map[string]any)
	triggerEvent := readRealtimeEvent(t, appConn)
	if triggerEvent.Kind != realtime.KindEvent || triggerEvent.Event != realtime.EventMessageCreated {
		t.Fatalf("trigger app event = %#v, want message.created", triggerEvent)
	}

	request := realtime.Envelope{
		V:      realtime.ProtocolVersion,
		Kind:   realtime.KindRequest,
		ID:     "app-create-group-request",
		Method: "group_conversations.create",
		Payload: mustMarshalPayloadForTest(t, map[string]any{
			"actor_user_id":      alice.ID,
			"trigger_message_id": triggerMessage["id"],
			"name":               " 项目讨论组 ",
			"member_ids":         []string{bob.ID, carol.ID, bob.ID},
		}),
	}
	response := sendAppRequest(t, appConn, request)
	requireReplayedAppResponse(t, appConn, request, response)
	var payload map[string]any
	if err := json.Unmarshal(response.Payload, &payload); err != nil {
		t.Fatalf("unmarshal group create response: %v", err)
	}
	conversation := payload["conversation"].(map[string]any)
	if conversation["type"] != store.ConversationKindGroup {
		t.Fatalf("conversation.type = %v, want group", conversation["type"])
	}
	if conversation["name"] != "项目讨论组" {
		t.Fatalf("conversation.name = %v, want trimmed group name", conversation["name"])
	}
	if conversation["created_by_user_id"] != alice.ID {
		t.Fatalf("conversation.created_by_user_id = %v, want %s", conversation["created_by_user_id"], alice.ID)
	}
	if conversation["member_count"] != float64(3) {
		t.Fatalf("conversation.member_count = %v, want 3", conversation["member_count"])
	}
	message := payload["message"].(map[string]any)
	if message["summary"] != "Alice 邀请 Bob,Carol 加入群聊" {
		t.Fatalf("message.summary = %v, want invite summary", message["summary"])
	}

	var storedConversation store.Conversation
	if err := db.First(&storedConversation, "id = ?", conversation["id"]).Error; err != nil {
		t.Fatalf("find stored conversation: %v", err)
	}
	if storedConversation.CreatedByUserID != alice.ID || storedConversation.Kind != store.ConversationKindGroup {
		t.Fatalf("stored conversation = %#v, want Alice group", storedConversation)
	}
	var members []store.ConversationMember
	if err := db.Where("conversation_id = ?", storedConversation.ID).Find(&members).Error; err != nil {
		t.Fatalf("find group members: %v", err)
	}
	if len(members) != 3 {
		t.Fatalf("member count = %d, want 3", len(members))
	}
	var messageCount int64
	if err := db.Model(&store.Message{}).Where("conversation_id = ?", storedConversation.ID).Count(&messageCount).Error; err != nil {
		t.Fatalf("count group create messages: %v", err)
	}
	if messageCount != 1 {
		t.Fatalf("group create message count = %d, want 1", messageCount)
	}
}

func TestAppWebSocketGroupConversationMembersAddUsesTriggeringUser(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	now := time.Date(2026, 7, 8, 10, 0, 0, 0, time.UTC)
	alice := insertTestUser(t, db, "alice@example.com", "Alice", store.UserStatusActive, now)
	bob := insertTestUser(t, db, "bob@example.com", "Bob", store.UserStatusActive, now)
	carol := insertTestUser(t, db, "carol@example.com", "Carol", store.UserStatusActive, now)
	dave := insertTestUser(t, db, "dave@example.com", "Dave", store.UserStatusActive, now)
	group := insertTestConversation(t, db, testConversationInput{
		createdByUserID: alice.ID,
		kind:            store.ConversationKindGroup,
		lastMessageSeq:  2,
		memberIDs:       []string{alice.ID, bob.ID},
		name:            "项目讨论组",
		now:             now,
	})
	setTestConversationMemberLastReadSeq(t, db, group.ID, alice.ID, 2)
	app := insertTestApp(t, db, store.App{
		Name:             "女菩萨",
		Enabled:          true,
		Visibility:       store.AppVisibilityPublic,
		ConnectionSecret: "assistant-secret",
		CreatedAt:        now,
		UpdatedAt:        now,
	})
	aliceCookie := loginAsUser(t, server, alice.Email)
	appConn := dialAppWebSocket(t, server, app.ID, app.ConnectionSecret)

	createConversationResp, createConversationBody := postJSON(t, server, "/api/client/conversations/apps", map[string]any{
		"app_id": app.ID,
	}, aliceCookie)
	if createConversationResp.StatusCode != http.StatusCreated {
		t.Fatalf("create app conversation status = %d, want 201, body = %#v", createConversationResp.StatusCode, createConversationBody)
	}
	appConversation := requireSuccess(t, createConversationBody)["conversation"].(map[string]any)
	triggerResp, triggerBody := postJSON(t, server, "/api/client/conversations/"+appConversation["id"].(string)+"/messages", map[string]any{
		"client_message_id": "trigger-add-members-1",
		"body": map[string]any{
			"type":    "text",
			"content": "把 Carol 和 Dave 拉进项目讨论组",
		},
	}, aliceCookie)
	if triggerResp.StatusCode != http.StatusCreated {
		t.Fatalf("trigger message status = %d, want 201, body = %#v", triggerResp.StatusCode, triggerBody)
	}
	triggerMessage := requireSuccess(t, triggerBody)["message"].(map[string]any)
	triggerEvent := readRealtimeEvent(t, appConn)
	if triggerEvent.Kind != realtime.KindEvent || triggerEvent.Event != realtime.EventMessageCreated {
		t.Fatalf("trigger app event = %#v, want message.created", triggerEvent)
	}

	request := realtime.Envelope{
		V:      realtime.ProtocolVersion,
		Kind:   realtime.KindRequest,
		ID:     "app-add-group-members-request",
		Method: "group_conversations.members.add",
		Payload: mustMarshalPayloadForTest(t, map[string]any{
			"actor_user_id":      alice.ID,
			"trigger_message_id": triggerMessage["id"],
			"conversation_id":    group.ID,
			"member_ids":         []string{carol.ID, dave.ID},
		}),
	}
	response := sendAppRequest(t, appConn, request)
	requireReplayedAppResponse(t, appConn, request, response)
	var payload map[string]any
	if err := json.Unmarshal(response.Payload, &payload); err != nil {
		t.Fatalf("unmarshal members add response: %v", err)
	}
	conversation := payload["conversation"].(map[string]any)
	if conversation["id"] != group.ID {
		t.Fatalf("conversation.id = %v, want %s", conversation["id"], group.ID)
	}
	if conversation["member_count"] != float64(4) {
		t.Fatalf("conversation.member_count = %v, want 4", conversation["member_count"])
	}
	message := payload["message"].(map[string]any)
	if message["summary"] != "Alice 邀请 Carol,Dave 加入群聊" {
		t.Fatalf("message.summary = %v, want invite summary", message["summary"])
	}

	var members []store.ConversationMember
	if err := db.Where("conversation_id = ?", group.ID).Find(&members).Error; err != nil {
		t.Fatalf("find group members: %v", err)
	}
	if len(members) != 4 {
		t.Fatalf("member count = %d, want 4", len(members))
	}
	for _, member := range members {
		if member.MemberID == carol.ID || member.MemberID == dave.ID {
			if member.HistoryVisibleFromSeq != 3 {
				t.Fatalf("new member %s history_visible_from_seq = %d, want 3", member.MemberID, member.HistoryVisibleFromSeq)
			}
		}
	}
	var messageCount int64
	if err := db.Model(&store.Message{}).Where("conversation_id = ?", group.ID).Count(&messageCount).Error; err != nil {
		t.Fatalf("count group member messages: %v", err)
	}
	if messageCount != 1 {
		t.Fatalf("group member message count = %d, want 1", messageCount)
	}
}

func TestAppWebSocketGroupConversationsListReturnsTriggeringUserGroups(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	now := time.Date(2026, 7, 8, 10, 0, 0, 0, time.UTC)
	alice := insertTestUser(t, db, "alice@example.com", "Alice", store.UserStatusActive, now)
	bob := insertTestUser(t, db, "bob@example.com", "Bob", store.UserStatusActive, now)
	carol := insertTestUser(t, db, "carol@example.com", "Carol", store.UserStatusActive, now)
	matchedGroupLastMessageAt := now.Add(-10 * time.Minute)
	matchedGroup := insertTestConversation(t, db, testConversationInput{
		createdByUserID:    alice.ID,
		kind:               store.ConversationKindGroup,
		lastMessageAt:      &matchedGroupLastMessageAt,
		lastMessageSeq:     7,
		lastMessageSummary: "最近的项目消息",
		memberIDs:          []string{alice.ID, bob.ID},
		name:               "项目讨论组",
		now:                now.Add(-2 * time.Hour),
	})
	_ = insertTestConversation(t, db, testConversationInput{
		createdByUserID:    carol.ID,
		kind:               store.ConversationKindGroup,
		lastMessageSummary: "不应该看到",
		memberIDs:          []string{carol.ID},
		name:               "项目旁观组",
		now:                now.Add(-90 * time.Minute),
	})
	_ = insertTestConversation(t, db, testConversationInput{
		createdByUserID: alice.ID,
		kind:            store.ConversationKindGroup,
		memberIDs:       []string{alice.ID, carol.ID},
		name:            "其他小组",
		now:             now.Add(-80 * time.Minute),
	})
	app := insertTestApp(t, db, store.App{
		Name:             "女菩萨",
		Enabled:          true,
		Visibility:       store.AppVisibilityPublic,
		ConnectionSecret: "assistant-secret",
		CreatedAt:        now,
		UpdatedAt:        now,
	})
	aliceCookie := loginAsUser(t, server, alice.Email)
	appConn := dialAppWebSocket(t, server, app.ID, app.ConnectionSecret)

	createConversationResp, createConversationBody := postJSON(t, server, "/api/client/conversations/apps", map[string]any{
		"app_id": app.ID,
	}, aliceCookie)
	if createConversationResp.StatusCode != http.StatusCreated {
		t.Fatalf("create app conversation status = %d, want 201, body = %#v", createConversationResp.StatusCode, createConversationBody)
	}
	appConversation := requireSuccess(t, createConversationBody)["conversation"].(map[string]any)
	triggerResp, triggerBody := postJSON(t, server, "/api/client/conversations/"+appConversation["id"].(string)+"/messages", map[string]any{
		"client_message_id": "trigger-list-groups-1",
		"body": map[string]any{
			"type":    "text",
			"content": "看看我有哪些项目群",
		},
	}, aliceCookie)
	if triggerResp.StatusCode != http.StatusCreated {
		t.Fatalf("trigger message status = %d, want 201, body = %#v", triggerResp.StatusCode, triggerBody)
	}
	triggerMessage := requireSuccess(t, triggerBody)["message"].(map[string]any)
	triggerEvent := readRealtimeEvent(t, appConn)
	if triggerEvent.Kind != realtime.KindEvent || triggerEvent.Event != realtime.EventMessageCreated {
		t.Fatalf("trigger app event = %#v, want message.created", triggerEvent)
	}

	response := sendAppRequest(t, appConn, realtime.Envelope{
		V:      realtime.ProtocolVersion,
		Kind:   realtime.KindRequest,
		ID:     "app-list-groups-request",
		Method: "group_conversations.list",
		Payload: mustMarshalPayloadForTest(t, map[string]any{
			"actor_user_id":      alice.ID,
			"trigger_message_id": triggerMessage["id"],
			"keyword":            "项目",
		}),
	})
	var payload map[string]any
	if err := json.Unmarshal(response.Payload, &payload); err != nil {
		t.Fatalf("unmarshal groups list response: %v", err)
	}
	groups := payload["groups"].([]any)
	if len(groups) != 1 {
		t.Fatalf("group count = %d, want 1: %#v", len(groups), groups)
	}
	group := groups[0].(map[string]any)
	if group["id"] != matchedGroup.ID {
		t.Fatalf("group.id = %v, want %s", group["id"], matchedGroup.ID)
	}
	if group["type"] != store.ConversationKindGroup {
		t.Fatalf("group.type = %v, want group", group["type"])
	}
	if group["name"] != "项目讨论组" {
		t.Fatalf("group.name = %v, want 项目讨论组", group["name"])
	}
	if group["member_count"] != float64(2) {
		t.Fatalf("group.member_count = %v, want 2", group["member_count"])
	}
	if group["last_message_summary"] != "最近的项目消息" {
		t.Fatalf("group.last_message_summary = %v, want 最近的项目消息", group["last_message_summary"])
	}
}

func TestAppWebSocketRecentConversationsListUsesTriggeringUser(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	now := time.Date(2026, 7, 8, 10, 0, 0, 0, time.UTC)
	alice := insertTestUser(t, db, "alice@example.com", "Alice", store.UserStatusActive, now)
	bob := insertTestUser(t, db, "bob@example.com", "Bob", store.UserStatusActive, now)
	bob.Nickname = "鲍勃"
	if err := db.Model(&store.User{}).Where("id = ?", bob.ID).Update("nickname", bob.Nickname).Error; err != nil {
		t.Fatalf("update bob nickname: %v", err)
	}
	carol := insertTestUser(t, db, "carol@example.com", "Carol", store.UserStatusActive, now)
	directLastActiveAt := now.Add(-5 * time.Minute)
	direct := insertTestConversation(t, db, testConversationInput{
		createdByUserID:    alice.ID,
		kind:               store.ConversationKindDirect,
		lastMessageAt:      &directLastActiveAt,
		lastMessageSeq:     4,
		lastMessageSummary: "私聊消息",
		memberIDs:          []string{alice.ID, bob.ID},
		now:                now.Add(-2 * time.Hour),
	})
	groupLastActiveAt := now.Add(-10 * time.Minute)
	group := insertTestConversation(t, db, testConversationInput{
		createdByUserID:    alice.ID,
		kind:               store.ConversationKindGroup,
		lastMessageAt:      &groupLastActiveAt,
		lastMessageSeq:     7,
		lastMessageSummary: "群聊消息",
		memberIDs:          []string{alice.ID, bob.ID},
		name:               "项目讨论组",
		now:                now.Add(-90 * time.Minute),
	})
	_ = insertTestConversation(t, db, testConversationInput{
		createdByUserID:    carol.ID,
		kind:               store.ConversationKindGroup,
		lastMessageSummary: "不应该看到",
		memberIDs:          []string{carol.ID},
		name:               "项目旁观组",
		now:                now.Add(-80 * time.Minute),
	})
	app := insertTestApp(t, db, store.App{
		Name:             "女菩萨",
		Enabled:          true,
		Visibility:       store.AppVisibilityPublic,
		ConnectionSecret: "assistant-secret",
		CreatedAt:        now,
		UpdatedAt:        now,
	})
	aliceCookie := loginAsUser(t, server, alice.Email)
	appConn := dialAppWebSocket(t, server, app.ID, app.ConnectionSecret)

	createConversationResp, createConversationBody := postJSON(t, server, "/api/client/conversations/apps", map[string]any{
		"app_id": app.ID,
	}, aliceCookie)
	if createConversationResp.StatusCode != http.StatusCreated {
		t.Fatalf("create app conversation status = %d, want 201, body = %#v", createConversationResp.StatusCode, createConversationBody)
	}
	appConversation := requireSuccess(t, createConversationBody)["conversation"].(map[string]any)
	triggerResp, triggerBody := postJSON(t, server, "/api/client/conversations/"+appConversation["id"].(string)+"/messages", map[string]any{
		"client_message_id": "trigger-list-recent-conversations-1",
		"body": map[string]any{
			"type":    "text",
			"content": "看看最近会话",
		},
	}, aliceCookie)
	if triggerResp.StatusCode != http.StatusCreated {
		t.Fatalf("trigger message status = %d, want 201, body = %#v", triggerResp.StatusCode, triggerBody)
	}
	triggerMessage := requireSuccess(t, triggerBody)["message"].(map[string]any)
	triggerEvent := readRealtimeEvent(t, appConn)
	if triggerEvent.Kind != realtime.KindEvent || triggerEvent.Event != realtime.EventMessageCreated {
		t.Fatalf("trigger app event = %#v, want message.created", triggerEvent)
	}

	response := sendAppRequest(t, appConn, realtime.Envelope{
		V:      realtime.ProtocolVersion,
		Kind:   realtime.KindRequest,
		ID:     "app-list-recent-conversations-request",
		Method: appMethodConversationsList,
		Payload: mustMarshalPayloadForTest(t, map[string]any{
			"actor_user_id":      alice.ID,
			"trigger_message_id": triggerMessage["id"],
			"limit":              200,
		}),
	})
	var payload map[string]any
	if err := json.Unmarshal(response.Payload, &payload); err != nil {
		t.Fatalf("unmarshal recent conversations response: %v", err)
	}
	if payload["limit"] != float64(100) {
		t.Fatalf("limit = %v, want 100", payload["limit"])
	}
	conversations := payload["conversations"].([]any)
	byID := map[string]map[string]any{}
	for _, item := range conversations {
		conversation := item.(map[string]any)
		byID[conversation["conversation_id"].(string)] = conversation
	}
	for _, tt := range []struct {
		id          string
		kind        string
		memberCount float64
	}{
		{id: appConversation["id"].(string), kind: store.ConversationKindApp, memberCount: 2},
		{id: direct.ID, kind: store.ConversationKindDirect, memberCount: 2},
		{id: group.ID, kind: store.ConversationKindGroup, memberCount: 2},
	} {
		conversation, ok := byID[tt.id]
		if !ok {
			t.Fatalf("conversation %s missing from %#v", tt.id, conversations)
		}
		if conversation["type"] != tt.kind {
			t.Fatalf("conversation %s type = %v, want %s", tt.id, conversation["type"], tt.kind)
		}
		if conversation["member_count"] != tt.memberCount {
			t.Fatalf("conversation %s member_count = %v, want %v", tt.id, conversation["member_count"], tt.memberCount)
		}
		if conversation["last_active_at"] == "" {
			t.Fatalf("conversation %s last_active_at is empty", tt.id)
		}
	}

	keywordResponse := sendAppRequest(t, appConn, realtime.Envelope{
		V:      realtime.ProtocolVersion,
		Kind:   realtime.KindRequest,
		ID:     "app-list-recent-conversations-keyword-request",
		Method: appMethodConversationsList,
		Payload: mustMarshalPayloadForTest(t, map[string]any{
			"actor_user_id":      alice.ID,
			"trigger_message_id": triggerMessage["id"],
			"keyword":            "项目",
		}),
	})
	var keywordPayload map[string]any
	if err := json.Unmarshal(keywordResponse.Payload, &keywordPayload); err != nil {
		t.Fatalf("unmarshal keyword recent conversations response: %v", err)
	}
	if keywordPayload["limit"] != float64(20) {
		t.Fatalf("keyword limit = %v, want default 20", keywordPayload["limit"])
	}
	keywordConversations := keywordPayload["conversations"].([]any)
	if len(keywordConversations) != 1 {
		t.Fatalf("keyword conversation count = %d, want 1: %#v", len(keywordConversations), keywordConversations)
	}
	keywordConversation := keywordConversations[0].(map[string]any)
	if keywordConversation["conversation_id"] != group.ID {
		t.Fatalf("keyword conversation id = %v, want %s", keywordConversation["conversation_id"], group.ID)
	}

	directKeywordResponse := sendAppRequest(t, appConn, realtime.Envelope{
		V:      realtime.ProtocolVersion,
		Kind:   realtime.KindRequest,
		ID:     "app-list-recent-conversations-direct-keyword-request",
		Method: appMethodConversationsList,
		Payload: mustMarshalPayloadForTest(t, map[string]any{
			"actor_user_id":      alice.ID,
			"trigger_message_id": triggerMessage["id"],
			"keyword":            "鲍勃",
		}),
	})
	var directKeywordPayload map[string]any
	if err := json.Unmarshal(directKeywordResponse.Payload, &directKeywordPayload); err != nil {
		t.Fatalf("unmarshal direct keyword recent conversations response: %v", err)
	}
	directKeywordConversations := directKeywordPayload["conversations"].([]any)
	if len(directKeywordConversations) != 1 {
		t.Fatalf("direct keyword conversation count = %d, want 1: %#v", len(directKeywordConversations), directKeywordConversations)
	}
	directKeywordConversation := directKeywordConversations[0].(map[string]any)
	if directKeywordConversation["conversation_id"] != direct.ID {
		t.Fatalf("direct keyword conversation id = %v, want %s", directKeywordConversation["conversation_id"], direct.ID)
	}
}

func TestAppWebSocketConversationHistoryReadSupportsConversationUserAndAppSelectors(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	now := time.Date(2026, 7, 8, 10, 0, 0, 0, time.UTC)
	alice := insertTestUser(t, db, "alice@example.com", "Alice", store.UserStatusActive, now)
	bob := insertTestUser(t, db, "bob@example.com", "Bob", store.UserStatusActive, now)
	direct := insertTestConversation(t, db, testConversationInput{
		createdByUserID: alice.ID,
		kind:            store.ConversationKindDirect,
		memberIDs:       []string{alice.ID, bob.ID},
		now:             now.Add(-2 * time.Hour),
	})
	insertTestMessage(t, db, direct.ID, alice.ID, 1, "第一条私聊", now.Add(-50*time.Minute))
	insertTestMessage(t, db, direct.ID, bob.ID, 2, "第二条私聊", now.Add(-40*time.Minute))
	insertTestMessage(t, db, direct.ID, alice.ID, 3, "第三条私聊", now.Add(-30*time.Minute))
	group := insertTestConversation(t, db, testConversationInput{
		createdByUserID: alice.ID,
		kind:            store.ConversationKindGroup,
		memberIDs:       []string{alice.ID, bob.ID},
		name:            "项目讨论组",
		now:             now.Add(-90 * time.Minute),
	})
	insertTestMessage(t, db, group.ID, alice.ID, 1, "第一条群聊", now.Add(-20*time.Minute))
	insertTestMessage(t, db, group.ID, bob.ID, 2, "第二条群聊", now.Add(-10*time.Minute))
	app := insertTestApp(t, db, store.App{
		Name:             "女菩萨",
		Enabled:          true,
		Visibility:       store.AppVisibilityPublic,
		ConnectionSecret: "assistant-secret",
		CreatedAt:        now,
		UpdatedAt:        now,
	})
	aliceCookie := loginAsUser(t, server, alice.Email)
	appConn := dialAppWebSocket(t, server, app.ID, app.ConnectionSecret)

	createConversationResp, createConversationBody := postJSON(t, server, "/api/client/conversations/apps", map[string]any{
		"app_id": app.ID,
	}, aliceCookie)
	if createConversationResp.StatusCode != http.StatusCreated {
		t.Fatalf("create app conversation status = %d, want 201, body = %#v", createConversationResp.StatusCode, createConversationBody)
	}
	appConversation := requireSuccess(t, createConversationBody)["conversation"].(map[string]any)
	triggerResp, triggerBody := postJSON(t, server, "/api/client/conversations/"+appConversation["id"].(string)+"/messages", map[string]any{
		"client_message_id": "trigger-read-history-1",
		"body": map[string]any{
			"type":    "text",
			"content": "帮我读聊天记录",
		},
	}, aliceCookie)
	if triggerResp.StatusCode != http.StatusCreated {
		t.Fatalf("trigger message status = %d, want 201, body = %#v", triggerResp.StatusCode, triggerBody)
	}
	triggerMessage := requireSuccess(t, triggerBody)["message"].(map[string]any)
	triggerEvent := readRealtimeEvent(t, appConn)
	if triggerEvent.Kind != realtime.KindEvent || triggerEvent.Event != realtime.EventMessageCreated {
		t.Fatalf("trigger app event = %#v, want message.created", triggerEvent)
	}

	groupResponse := sendAppRequest(t, appConn, realtime.Envelope{
		V:      realtime.ProtocolVersion,
		Kind:   realtime.KindRequest,
		ID:     "app-read-history-conversation",
		Method: appMethodConversationHistoryRead,
		Payload: mustMarshalPayloadForTest(t, map[string]any{
			"actor_user_id":      alice.ID,
			"trigger_message_id": triggerMessage["id"],
			"conversation_id":    group.ID,
			"limit":              1,
		}),
	})
	var groupPayload map[string]any
	if err := json.Unmarshal(groupResponse.Payload, &groupPayload); err != nil {
		t.Fatalf("unmarshal group history response: %v", err)
	}
	groupMessages := groupPayload["messages"].([]any)
	if len(groupMessages) != 1 || groupMessages[0].(map[string]any)["seq"] != float64(2) {
		t.Fatalf("group messages = %#v, want latest seq 2", groupMessages)
	}

	directResponse := sendAppRequest(t, appConn, realtime.Envelope{
		V:      realtime.ProtocolVersion,
		Kind:   realtime.KindRequest,
		ID:     "app-read-history-user",
		Method: appMethodConversationHistoryRead,
		Payload: mustMarshalPayloadForTest(t, map[string]any{
			"actor_user_id":      alice.ID,
			"trigger_message_id": triggerMessage["id"],
			"user_id":            bob.ID,
			"before_seq":         3,
			"limit":              2,
		}),
	})
	var directPayload map[string]any
	if err := json.Unmarshal(directResponse.Payload, &directPayload); err != nil {
		t.Fatalf("unmarshal direct history response: %v", err)
	}
	if directPayload["limit"] != float64(2) {
		t.Fatalf("direct limit = %v, want 2", directPayload["limit"])
	}
	directConversation := directPayload["conversation"].(map[string]any)
	if directConversation["conversation_id"] != direct.ID || directConversation["type"] != store.ConversationKindDirect {
		t.Fatalf("direct conversation = %#v, want direct conversation", directConversation)
	}
	directMessages := directPayload["messages"].([]any)
	if len(directMessages) != 2 ||
		directMessages[0].(map[string]any)["seq"] != float64(1) ||
		directMessages[1].(map[string]any)["seq"] != float64(2) {
		t.Fatalf("direct messages = %#v, want seq 1 and 2", directMessages)
	}

	appResponse := sendAppRequest(t, appConn, realtime.Envelope{
		V:      realtime.ProtocolVersion,
		Kind:   realtime.KindRequest,
		ID:     "app-read-history-app",
		Method: appMethodConversationHistoryRead,
		Payload: mustMarshalPayloadForTest(t, map[string]any{
			"actor_user_id":      alice.ID,
			"trigger_message_id": triggerMessage["id"],
			"app_id":             app.ID,
		}),
	})
	var appPayload map[string]any
	if err := json.Unmarshal(appResponse.Payload, &appPayload); err != nil {
		t.Fatalf("unmarshal app history response: %v", err)
	}
	if appPayload["limit"] != float64(20) {
		t.Fatalf("app limit = %v, want default 20", appPayload["limit"])
	}
	appConversationPayload := appPayload["conversation"].(map[string]any)
	if appConversationPayload["conversation_id"] != appConversation["id"] || appConversationPayload["type"] != store.ConversationKindApp {
		t.Fatalf("app conversation = %#v, want app conversation", appConversationPayload)
	}
	appMessages := appPayload["messages"].([]any)
	if len(appMessages) != 1 || appMessages[0].(map[string]any)["id"] != triggerMessage["id"] {
		t.Fatalf("app messages = %#v, want trigger message", appMessages)
	}
}

func TestAppWebSocketMessageSendAsUserCanSendToGroupConversation(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	now := time.Date(2026, 7, 8, 10, 0, 0, 0, time.UTC)
	alice := insertTestUser(t, db, "alice@example.com", "Alice", store.UserStatusActive, now)
	bob := insertTestUser(t, db, "bob@example.com", "Bob", store.UserStatusActive, now)
	group := insertTestConversation(t, db, testConversationInput{
		createdByUserID:    alice.ID,
		kind:               store.ConversationKindGroup,
		lastMessageSeq:     3,
		lastMessageSummary: "旧群消息",
		memberIDs:          []string{alice.ID, bob.ID},
		name:               "项目讨论组",
		now:                now,
	})
	app := insertTestApp(t, db, store.App{
		Name:             "女菩萨",
		Enabled:          true,
		Visibility:       store.AppVisibilityPublic,
		ConnectionSecret: "assistant-secret",
		CreatedAt:        now,
		UpdatedAt:        now,
	})
	aliceCookie := loginAsUser(t, server, alice.Email)
	bobCookie := loginAsUser(t, server, bob.Email)
	bobConn := dialClientWebSocket(t, server, bobCookie)
	if ready := readRealtimeEvent(t, bobConn); ready.Kind != realtime.KindEvent || ready.Event != realtime.EventSystemReady {
		t.Fatalf("ready envelope = %#v, want system.ready", ready)
	}
	appConn := dialAppWebSocket(t, server, app.ID, app.ConnectionSecret)

	createConversationResp, createConversationBody := postJSON(t, server, "/api/client/conversations/apps", map[string]any{
		"app_id": app.ID,
	}, aliceCookie)
	if createConversationResp.StatusCode != http.StatusCreated {
		t.Fatalf("create app conversation status = %d, want 201, body = %#v", createConversationResp.StatusCode, createConversationBody)
	}
	appConversation := requireSuccess(t, createConversationBody)["conversation"].(map[string]any)
	triggerResp, triggerBody := postJSON(t, server, "/api/client/conversations/"+appConversation["id"].(string)+"/messages", map[string]any{
		"client_message_id": "trigger-send-group-1",
		"body": map[string]any{
			"type":    "text",
			"content": "替我发到项目讨论组",
		},
	}, aliceCookie)
	if triggerResp.StatusCode != http.StatusCreated {
		t.Fatalf("trigger message status = %d, want 201, body = %#v", triggerResp.StatusCode, triggerBody)
	}
	triggerMessage := requireSuccess(t, triggerBody)["message"].(map[string]any)
	triggerEvent := readRealtimeEvent(t, appConn)
	if triggerEvent.Kind != realtime.KindEvent || triggerEvent.Event != realtime.EventMessageCreated {
		t.Fatalf("trigger app event = %#v, want message.created", triggerEvent)
	}

	response := sendAppRequest(t, appConn, realtime.Envelope{
		V:      realtime.ProtocolVersion,
		Kind:   realtime.KindRequest,
		ID:     "app-send-as-user-group-request",
		Method: appMethodMessageSendAsUser,
		Payload: mustMarshalPayloadForTest(t, map[string]any{
			"actor_user_id":      alice.ID,
			"trigger_message_id": triggerMessage["id"],
			"target": map[string]any{
				"type":            "group",
				"conversation_id": group.ID,
			},
			"message": map[string]any{
				"type":    "markdown",
				"content": "**群里同步一下**",
			},
		}),
	})
	payload := requireAppSendMessageResponsePayload(t, response)
	conversation := payload["conversation"].(map[string]any)
	if conversation["id"] != group.ID || conversation["type"] != store.ConversationKindGroup {
		t.Fatalf("conversation = %#v, want group %s", conversation, group.ID)
	}
	message := payload["message"].(map[string]any)
	messageBody := message["body"].(map[string]any)
	if messageBody["type"] != messageTypeMarkdown || messageBody["content"] != "**群里同步一下**" {
		t.Fatalf("message.body = %#v, want markdown group content", messageBody)
	}
	sender := message["sender"].(map[string]any)
	if sender["type"] != store.MessageSenderTypeUser || sender["id"] != alice.ID {
		t.Fatalf("message.sender = %#v, want Alice user sender", sender)
	}
	delegatedBy := message["delegated_by"].(map[string]any)
	if delegatedBy["type"] != store.MessageSenderTypeApp || delegatedBy["id"] != app.ID || delegatedBy["name"] != app.Name {
		t.Fatalf("message.delegated_by = %#v, want app delegate", delegatedBy)
	}

	pushedMessage := readMessageCreatedEvent(t, bobConn)
	if pushedMessage["id"] != message["id"] {
		t.Fatalf("pushed message id = %v, want %v", pushedMessage["id"], message["id"])
	}
	pushedDelegatedBy := pushedMessage["delegated_by"].(map[string]any)
	if pushedDelegatedBy["type"] != store.MessageSenderTypeApp || pushedDelegatedBy["id"] != app.ID || pushedDelegatedBy["name"] != app.Name {
		t.Fatalf("pushed delegated_by = %#v, want app delegate", pushedDelegatedBy)
	}

	var storedMessage store.Message
	if err := db.First(&storedMessage, "id = ?", message["id"]).Error; err != nil {
		t.Fatalf("find stored message: %v", err)
	}
	if storedMessage.ConversationID != group.ID || storedMessage.Seq != 4 {
		t.Fatalf("stored message conversation/seq = %s/%d, want %s/4", storedMessage.ConversationID, storedMessage.Seq, group.ID)
	}
	if storedMessage.SenderType != store.MessageSenderTypeUser || storedMessage.SenderID == nil || *storedMessage.SenderID != alice.ID {
		t.Fatalf("stored sender = %s/%v, want Alice", storedMessage.SenderType, storedMessage.SenderID)
	}
	if storedMessage.DelegatedByType == nil || *storedMessage.DelegatedByType != store.MessageSenderTypeApp {
		t.Fatalf("stored delegated_by_type = %v, want app", storedMessage.DelegatedByType)
	}
	if storedMessage.DelegatedByID == nil || *storedMessage.DelegatedByID != app.ID {
		t.Fatalf("stored delegated_by_id = %v, want %s", storedMessage.DelegatedByID, app.ID)
	}
}

func TestAppWebSocketMessageSendSupportsUserGroupAndAppTargets(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	now := time.Date(2026, 7, 3, 9, 0, 0, 0, time.UTC)
	alice := insertTestUser(t, db, "alice@example.com", "Alice", store.UserStatusActive, now)
	app := insertTestApp(t, db, store.App{
		Name:             "Echo App",
		Enabled:          true,
		Visibility:       store.AppVisibilityPublic,
		ConnectionSecret: "echo-app-secret",
		CreatedAt:        now,
		UpdatedAt:        now,
	})
	userCookie := loginAsUser(t, server, alice.Email)
	userConn := dialClientWebSocket(t, server, userCookie)
	if ready := readRealtimeEvent(t, userConn); ready.Kind != realtime.KindEvent || ready.Event != realtime.EventSystemReady {
		t.Fatalf("ready envelope = %#v, want system.ready", ready)
	}
	appConn := dialAppWebSocket(t, server, app.ID, app.ConnectionSecret)

	userRequest := realtime.Envelope{
		V:      realtime.ProtocolVersion,
		Kind:   realtime.KindRequest,
		ID:     "app-request-user",
		Method: appMethodMessageSend,
		Payload: mustMarshalPayloadForTest(t, map[string]any{
			"target": map[string]any{
				"type":    "user",
				"user_id": alice.ID,
			},
			"message": map[string]any{
				"type":    "text",
				"content": "  给 Alice 的消息  ",
			},
		}),
	}
	userResponse := sendAppRequest(t, appConn, userRequest)
	userPayload := requireAppSendMessageResponsePayload(t, userResponse)
	userConversation := userPayload["conversation"].(map[string]any)
	if userConversation["type"] != store.ConversationKindApp {
		t.Fatalf("user target conversation.type = %v, want app", userConversation["type"])
	}
	userMessage := userPayload["message"].(map[string]any)
	userBody := userMessage["body"].(map[string]any)
	if userBody["content"] != "给 Alice 的消息" {
		t.Fatalf("user target body.content = %v, want normalized text", userBody["content"])
	}
	if userMessage["summary"] != "给 Alice 的消息" {
		t.Fatalf("user target summary = %v, want normalized text", userMessage["summary"])
	}
	pushedUserMessage := readMessageCreatedEvent(t, userConn)
	if pushedUserMessage["id"] != userMessage["id"] {
		t.Fatalf("user target pushed id = %v, want %v", pushedUserMessage["id"], userMessage["id"])
	}
	userSender := pushedUserMessage["sender"].(map[string]any)
	if userSender["type"] != store.MessageSenderTypeApp || userSender["id"] != app.ID {
		t.Fatalf("user target pushed sender = %#v, want app sender", userSender)
	}

	appConversationID := userConversation["id"].(string)
	appRequest := realtime.Envelope{
		V:      realtime.ProtocolVersion,
		Kind:   realtime.KindRequest,
		ID:     "app-request-app-conversation",
		Method: appMethodMessageSend,
		Payload: mustMarshalPayloadForTest(t, map[string]any{
			"target": map[string]any{
				"type":            "app",
				"conversation_id": appConversationID,
			},
			"message": map[string]any{
				"type":    "text",
				"content": "应用会话里的第二条",
			},
		}),
	}
	appResponse := sendAppRequest(t, appConn, appRequest)
	appPayload := requireAppSendMessageResponsePayload(t, appResponse)
	appConversation := appPayload["conversation"].(map[string]any)
	if appConversation["id"] != appConversationID {
		t.Fatalf("app target conversation.id = %v, want %s", appConversation["id"], appConversationID)
	}
	if appConversation["type"] != store.ConversationKindApp {
		t.Fatalf("app target conversation.type = %v, want app", appConversation["type"])
	}
	appMessage := appPayload["message"].(map[string]any)
	if appMessage["seq"] != float64(2) {
		t.Fatalf("app target message.seq = %v, want 2", appMessage["seq"])
	}
	pushedAppMessage := readMessageCreatedEvent(t, userConn)
	if pushedAppMessage["id"] != appMessage["id"] {
		t.Fatalf("app target pushed id = %v, want %v", pushedAppMessage["id"], appMessage["id"])
	}

	group := insertTestConversation(t, db, testConversationInput{
		createdByUserID: alice.ID,
		kind:            store.ConversationKindGroup,
		memberIDs:       []string{alice.ID},
		name:            "产品讨论组",
		now:             now,
	})
	if err := db.Create(&store.ConversationMember{
		ConversationID:        group.ID,
		MemberType:            store.ConversationMemberTypeApp,
		MemberID:              app.ID,
		Role:                  store.ConversationMemberRoleMember,
		JoinedAt:              now,
		HistoryVisibleFromSeq: 1,
	}).Error; err != nil {
		t.Fatalf("create app group member: %v", err)
	}

	groupRequest := realtime.Envelope{
		V:      realtime.ProtocolVersion,
		Kind:   realtime.KindRequest,
		ID:     "app-request-group",
		Method: appMethodMessageSend,
		Payload: mustMarshalPayloadForTest(t, map[string]any{
			"target": map[string]any{
				"type":            "group",
				"conversation_id": group.ID,
			},
			"message": map[string]any{
				"type":    "text",
				"content": "群消息",
			},
		}),
	}
	groupResponse := sendAppRequest(t, appConn, groupRequest)
	groupPayload := requireAppSendMessageResponsePayload(t, groupResponse)
	groupConversation := groupPayload["conversation"].(map[string]any)
	if groupConversation["id"] != group.ID {
		t.Fatalf("group target conversation.id = %v, want %s", groupConversation["id"], group.ID)
	}
	if groupConversation["type"] != store.ConversationKindGroup {
		t.Fatalf("group target conversation.type = %v, want group", groupConversation["type"])
	}
	groupMessage := groupPayload["message"].(map[string]any)
	pushedGroupMessage := readMessageCreatedEvent(t, userConn)
	if pushedGroupMessage["id"] != groupMessage["id"] {
		t.Fatalf("group target pushed id = %v, want %v", pushedGroupMessage["id"], groupMessage["id"])
	}
	requireReplayedAppResponse(t, appConn, groupRequest, groupResponse)
	requireNoRealtimeEvent(t, userConn)

	var storedMessages []store.Message
	if err := db.Order("created_at ASC").Find(&storedMessages, "sender_type = ? AND sender_id = ?", store.MessageSenderTypeApp, app.ID).Error; err != nil {
		t.Fatalf("find app messages: %v", err)
	}
	if len(storedMessages) != 3 {
		t.Fatalf("stored app message count = %d, want 3", len(storedMessages))
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
	if appMember.MemberID != appregistry.AIAssistantAppID {
		t.Fatalf("assistant app member id = %s, want AI assistant app id", appMember.MemberID)
	}

	var appConversation store.AppConversation
	if err := db.First(&appConversation, "app_id = ? AND user_id = ?", appregistry.AIAssistantAppID, alice.ID).Error; err != nil {
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

func TestClientAppConversationCreatesAndReturnsExistingForVisibleApp(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	now := time.Now().UTC()
	alice := insertTestUser(t, db, "alice@example.com", "Alice", store.UserStatusActive, now)
	aliceCreatorID := alice.ID
	visibleApp := insertTestApp(t, db, store.App{
		Name:             "Alice Agent",
		CreatorUserID:    &aliceCreatorID,
		Enabled:          true,
		Visibility:       store.AppVisibilityCreator,
		ConnectionSecret: "alice-agent-secret",
		CreatedAt:        now,
		UpdatedAt:        now,
	})
	cookie := loginAsUser(t, server, alice.Email)

	createResp, createBody := postJSON(t, server, "/api/client/conversations/apps", map[string]any{
		"app_id": visibleApp.ID,
	}, cookie)
	if createResp.StatusCode != http.StatusCreated {
		t.Fatalf("create status = %d, want 201, body = %#v", createResp.StatusCode, createBody)
	}
	createdConversation := requireSuccess(t, createBody)["conversation"].(map[string]any)
	conversationID := createdConversation["id"].(string)
	if createdConversation["type"] != store.ConversationKindApp {
		t.Fatalf("conversation.type = %v, want app", createdConversation["type"])
	}
	if createdConversation["name"] != visibleApp.Name {
		t.Fatalf("conversation.name = %v, want %s", createdConversation["name"], visibleApp.Name)
	}

	var storedConversation store.Conversation
	if err := db.First(&storedConversation, "id = ?", conversationID).Error; err != nil {
		t.Fatalf("find app conversation: %v", err)
	}
	if storedConversation.Kind != store.ConversationKindApp {
		t.Fatalf("stored kind = %v, want app", storedConversation.Kind)
	}
	if storedConversation.CreatedByUserID != alice.ID {
		t.Fatalf("stored created_by_user_id = %v, want %s", storedConversation.CreatedByUserID, alice.ID)
	}
	var appConversation store.AppConversation
	if err := db.First(&appConversation, "app_id = ? AND user_id = ?", visibleApp.ID, alice.ID).Error; err != nil {
		t.Fatalf("find app_conversation: %v", err)
	}
	if appConversation.ConversationID != conversationID {
		t.Fatalf("app conversation id = %s, want %s", appConversation.ConversationID, conversationID)
	}

	var members []store.ConversationMember
	if err := db.Where("conversation_id = ?", conversationID).Find(&members).Error; err != nil {
		t.Fatalf("find app conversation members: %v", err)
	}
	if len(members) != 2 {
		t.Fatalf("member count = %d, want 2", len(members))
	}

	existingResp, existingBody := postJSON(t, server, "/api/client/conversations/apps", map[string]any{
		"app_id": visibleApp.ID,
	}, cookie)
	if existingResp.StatusCode != http.StatusOK {
		t.Fatalf("existing status = %d, want 200, body = %#v", existingResp.StatusCode, existingBody)
	}
	existingConversation := requireSuccess(t, existingBody)["conversation"].(map[string]any)
	if existingConversation["id"] != conversationID {
		t.Fatalf("existing conversation id = %v, want %s", existingConversation["id"], conversationID)
	}
}

func TestClientAppConversationRejectsHiddenOrDisabledApp(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	now := time.Now().UTC()
	alice := insertTestUser(t, db, "alice@example.com", "Alice", store.UserStatusActive, now)
	bob := insertTestUser(t, db, "bob@example.com", "Bob", store.UserStatusActive, now)
	bobCreatorID := bob.ID
	hiddenApp := insertTestApp(t, db, store.App{
		Name:             "Bob Agent",
		CreatorUserID:    &bobCreatorID,
		Enabled:          true,
		Visibility:       store.AppVisibilityCreator,
		ConnectionSecret: "bob-agent-secret",
		CreatedAt:        now,
		UpdatedAt:        now,
	})
	disabledApp := insertTestApp(t, db, store.App{
		Name:             "Disabled Agent",
		Enabled:          true,
		Visibility:       store.AppVisibilityPublic,
		ConnectionSecret: "disabled-agent-secret",
		CreatedAt:        now,
		UpdatedAt:        now,
	})
	if err := db.Model(&store.App{}).Where("id = ?", disabledApp.ID).Update("enabled", false).Error; err != nil {
		t.Fatalf("disable test app: %v", err)
	}
	cookie := loginAsUser(t, server, alice.Email)

	for _, appID := range []string{hiddenApp.ID, disabledApp.ID} {
		resp, body := postJSON(t, server, "/api/client/conversations/apps", map[string]any{
			"app_id": appID,
		}, cookie)
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("status = %d, want 404, body = %#v", resp.StatusCode, body)
		}
		requireError(t, body, "not_found")
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

func TestCreateConversationTextMessageDoesNotLogRecordNotFoundForFreshClientMessageID(t *testing.T) {
	var dbLogs bytes.Buffer
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{
		Logger: gormlogger.New(log.New(&dbLogs, "", 0), gormlogger.Config{LogLevel: gormlogger.Info}),
	})
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
		Apps:     config.AppsConfig{AIAssistantSecret: "test-ai-assistant-secret"},
	}, realtime.Options{})
	server := httptest.NewServer(router)
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
	dbLogs.Reset()

	resp, body := postJSON(t, server, "/api/client/conversations/"+conversation.ID+"/messages", map[string]any{
		"client_message_id": "fresh-client-message-1",
		"body": map[string]any{
			"type":    "text",
			"content": "第一条",
		},
	}, cookie)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201, body = %#v", resp.StatusCode, body)
	}
	if strings.Contains(dbLogs.String(), "record not found") {
		t.Fatalf("db logs contain record not found for fresh message:\n%s", dbLogs.String())
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
		context.Background(),
		alice.ID,
		conversation.ID,
		"client-message-1",
		json.RawMessage(`{"type":"text","content":"hello"}`),
		staticMessageBodyFinalizer("hello"),
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

func TestCreateConversationTextMessageUpdatesMentionedMemberAndPushesMentionEvent(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()
	now := time.Date(2026, 7, 3, 9, 0, 0, 0, time.UTC)
	alice := insertTestUser(t, db, "alice@example.com", "Alice", store.UserStatusActive, now)
	bob := insertTestUser(t, db, "bob@example.com", "Bob", store.UserStatusActive, now)
	conversation := insertTestConversation(t, db, testConversationInput{
		createdByUserID: alice.ID,
		kind:            store.ConversationKindGroup,
		memberIDs:       []string{alice.ID, bob.ID},
		now:             now,
	})
	aliceCookie := loginAsUser(t, server, alice.Email)
	bobConn := dialClientWebSocket(t, server, loginAsUser(t, server, bob.Email))
	if ready := readRealtimeEvent(t, bobConn); ready.Kind != realtime.KindEvent || ready.Event != realtime.EventSystemReady {
		t.Fatalf("bob ready envelope = %#v, want system.ready", ready)
	}

	resp, body := postJSON(t, server, "/api/client/conversations/"+conversation.ID+"/messages", map[string]any{
		"client_message_id": "client-message-mention-1",
		"body": map[string]any{
			"type":    "text",
			"content": "你好，{(@user/" + bob.ID + ")}",
		},
	}, aliceCookie)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201, body = %#v", resp.StatusCode, body)
	}
	createdMessage := requireSuccess(t, body)["message"].(map[string]any)

	pushedMessage := readMessageCreatedEvent(t, bobConn)
	if pushedMessage["id"] != createdMessage["id"] {
		t.Fatalf("pushed message id = %v, want %v", pushedMessage["id"], createdMessage["id"])
	}

	mentionEvent := readRealtimeEvent(t, bobConn)
	if mentionEvent.Kind != realtime.KindEvent || mentionEvent.Event != realtime.EventMemberMentioned {
		t.Fatalf("mention event = %#v, want conversation.member_mentioned", mentionEvent)
	}
	var mentionPayload map[string]any
	if err := json.Unmarshal(mentionEvent.Payload, &mentionPayload); err != nil {
		t.Fatalf("unmarshal mention payload: %v", err)
	}
	if mentionPayload["conversation_id"] != conversation.ID {
		t.Fatalf("mention conversation_id = %v, want %s", mentionPayload["conversation_id"], conversation.ID)
	}
	if mentionPayload["last_mentioned_seq"] != float64(1) {
		t.Fatalf("mention last_mentioned_seq = %v, want 1", mentionPayload["last_mentioned_seq"])
	}

	var bobMember store.ConversationMember
	if err := db.First(
		&bobMember,
		"conversation_id = ? AND member_type = ? AND member_id = ?",
		conversation.ID,
		store.ConversationMemberTypeUser,
		bob.ID,
	).Error; err != nil {
		t.Fatalf("find bob conversation member: %v", err)
	}
	if bobMember.LastMentionedSeq != 1 {
		t.Fatalf("bob last_mentioned_seq = %d, want 1", bobMember.LastMentionedSeq)
	}
}

func TestCreateConversationTextMessageMentionAllUpdatesCurrentUserMembers(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()
	now := time.Date(2026, 7, 3, 9, 0, 0, 0, time.UTC)
	alice := insertTestUser(t, db, "alice@example.com", "Alice", store.UserStatusActive, now)
	bob := insertTestUser(t, db, "bob@example.com", "Bob", store.UserStatusActive, now)
	conversation := insertTestConversation(t, db, testConversationInput{
		createdByUserID: alice.ID,
		kind:            store.ConversationKindGroup,
		memberIDs:       []string{alice.ID, bob.ID},
		now:             now,
	})
	aliceCookie := loginAsUser(t, server, alice.Email)
	bobConn := dialClientWebSocket(t, server, loginAsUser(t, server, bob.Email))
	if ready := readRealtimeEvent(t, bobConn); ready.Kind != realtime.KindEvent || ready.Event != realtime.EventSystemReady {
		t.Fatalf("bob ready envelope = %#v, want system.ready", ready)
	}

	resp, body := postJSON(t, server, "/api/client/conversations/"+conversation.ID+"/messages", map[string]any{
		"client_message_id": "client-message-mention-all-1",
		"body": map[string]any{
			"type":    "text",
			"content": "大家看一下，{(@user/all)}",
		},
	}, aliceCookie)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201, body = %#v", resp.StatusCode, body)
	}
	createdMessage := requireSuccess(t, body)["message"].(map[string]any)

	pushedMessage := readMessageCreatedEvent(t, bobConn)
	if pushedMessage["id"] != createdMessage["id"] {
		t.Fatalf("pushed message id = %v, want %v", pushedMessage["id"], createdMessage["id"])
	}

	mentionEvent := readRealtimeEvent(t, bobConn)
	if mentionEvent.Kind != realtime.KindEvent || mentionEvent.Event != realtime.EventMemberMentioned {
		t.Fatalf("mention event = %#v, want conversation.member_mentioned", mentionEvent)
	}

	for _, user := range []store.User{alice, bob} {
		var member store.ConversationMember
		if err := db.First(
			&member,
			"conversation_id = ? AND member_type = ? AND member_id = ?",
			conversation.ID,
			store.ConversationMemberTypeUser,
			user.ID,
		).Error; err != nil {
			t.Fatalf("find %s conversation member: %v", user.Email, err)
		}
		if member.LastMentionedSeq != 1 {
			t.Fatalf("%s last_mentioned_seq = %d, want 1", user.Email, member.LastMentionedSeq)
		}
	}
}

func TestRevokeOwnConversationMessageMarksOriginalAndCreatesSystemMessage(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()
	now := time.Date(2026, 7, 8, 10, 0, 0, 0, time.UTC)
	alice := insertTestUser(t, db, "alice@example.com", "Alice", store.UserStatusActive, now)
	bob := insertTestUser(t, db, "bob@example.com", "Bob", store.UserStatusActive, now)
	conversation := insertTestConversation(t, db, testConversationInput{
		createdByUserID: alice.ID,
		kind:            store.ConversationKindDirect,
		memberIDs:       []string{alice.ID, bob.ID},
		now:             now,
	})
	original := insertTestMessage(t, db, conversation.ID, alice.ID, 1, "这条消息稍后撤回", now)
	if err := db.Model(&store.Conversation{}).Where("id = ?", conversation.ID).Updates(map[string]any{
		"last_message_at":      original.CreatedAt,
		"last_message_id":      original.ID,
		"last_message_seq":     original.Seq,
		"last_message_summary": original.Summary,
	}).Error; err != nil {
		t.Fatalf("update conversation last message: %v", err)
	}

	bobCookie := loginAsUser(t, server, bob.Email)
	bobConn := dialClientWebSocket(t, server, bobCookie)
	if ready := readRealtimeEvent(t, bobConn); ready.Kind != realtime.KindEvent || ready.Event != realtime.EventSystemReady {
		t.Fatalf("ready envelope = %#v, want system.ready", ready)
	}

	resp, body := postJSON(t, server, "/api/client/conversations/"+conversation.ID+"/messages/"+original.ID+"/revoke", map[string]any{}, loginAsUser(t, server, alice.Email))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("revoke status = %d, want 200, body = %#v", resp.StatusCode, body)
	}
	data := requireSuccess(t, body)
	revokedMessage := data["message"].(map[string]any)
	if revokedMessage["id"] != original.ID {
		t.Fatalf("revoked message id = %v, want %s", revokedMessage["id"], original.ID)
	}
	if _, ok := revokedMessage["body"]; ok {
		t.Fatalf("revoked message body = %#v, want omitted", revokedMessage["body"])
	}
	if revokedMessage["revoked_at"] == "" || revokedMessage["revoked_at"] == nil {
		t.Fatalf("revoked_at = %#v, want set", revokedMessage["revoked_at"])
	}
	if revokedMessage["revoked_by_user_id"] != alice.ID {
		t.Fatalf("revoked_by_user_id = %v, want %s", revokedMessage["revoked_by_user_id"], alice.ID)
	}
	systemMessage := data["system_message"].(map[string]any)
	if systemMessage["seq"] != float64(2) {
		t.Fatalf("system message seq = %v, want 2", systemMessage["seq"])
	}
	if systemMessage["sender"].(map[string]any)["type"] != store.MessageSenderTypeSystem {
		t.Fatalf("system message sender = %#v, want system", systemMessage["sender"])
	}
	systemBody := systemMessage["body"].(map[string]any)
	if systemBody["type"] != "system_event" || systemBody["event"] != "message_revoked" {
		t.Fatalf("system message body = %#v, want message_revoked event", systemBody)
	}
	if systemBody["actor"].(map[string]any)["display_name"] != "Alice" {
		t.Fatalf("system actor = %#v, want Alice", systemBody["actor"])
	}

	var revokedRow struct {
		RevokedAt       *time.Time
		RevokedByUserID *string
	}
	if err := db.Raw("SELECT revoked_at, revoked_by_user_id FROM messages WHERE id = ?", original.ID).Scan(&revokedRow).Error; err != nil {
		t.Fatalf("find revoked message row: %v", err)
	}
	if revokedRow.RevokedAt == nil {
		t.Fatal("stored revoked_at = nil, want set")
	}
	if revokedRow.RevokedByUserID == nil || *revokedRow.RevokedByUserID != alice.ID {
		t.Fatalf("stored revoked_by_user_id = %v, want %s", revokedRow.RevokedByUserID, alice.ID)
	}

	var storedSystemMessage store.Message
	if err := db.First(&storedSystemMessage, "conversation_id = ? AND seq = ?", conversation.ID, int64(2)).Error; err != nil {
		t.Fatalf("find stored system message: %v", err)
	}
	if storedSystemMessage.Summary != "Alice 撤回了一条消息" {
		t.Fatalf("stored system summary = %v, want Alice 撤回了一条消息", storedSystemMessage.Summary)
	}
	requireMessageRevokedBody(t, storedSystemMessage.Body, alice.ID, "Alice")

	updatedEventMessage := readMessageUpdatedEvent(t, bobConn)
	if updatedEventMessage["id"] != original.ID {
		t.Fatalf("updated event message id = %v, want %s", updatedEventMessage["id"], original.ID)
	}
	if _, ok := updatedEventMessage["body"]; ok {
		t.Fatalf("updated event body = %#v, want omitted", updatedEventMessage["body"])
	}
	createdEventMessage := readMessageCreatedEvent(t, bobConn)
	if createdEventMessage["id"] != storedSystemMessage.ID {
		t.Fatalf("created event message id = %v, want %s", createdEventMessage["id"], storedSystemMessage.ID)
	}

	historyResp, historyBody := getJSON(t, server, "/api/client/conversations/"+conversation.ID+"/messages", loginAsUser(t, server, alice.Email))
	if historyResp.StatusCode != http.StatusOK {
		t.Fatalf("history status = %d, want 200, body = %#v", historyResp.StatusCode, historyBody)
	}
	messages := requireMessages(t, requireSuccess(t, historyBody))
	if len(messages) != 2 {
		t.Fatalf("history message count = %d, want 2", len(messages))
	}
	historyOriginal := messages[0].(map[string]any)
	if historyOriginal["id"] != original.ID {
		t.Fatalf("history original id = %v, want %s", historyOriginal["id"], original.ID)
	}
	if _, ok := historyOriginal["body"]; ok {
		t.Fatalf("history original body = %#v, want omitted", historyOriginal["body"])
	}
	if historyOriginal["revoked_by_user_id"] != alice.ID {
		t.Fatalf("history revoked_by_user_id = %v, want %s", historyOriginal["revoked_by_user_id"], alice.ID)
	}
}

func TestGroupAdminCanRevokeAnotherMembersMessage(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()
	now := time.Date(2026, 7, 8, 10, 0, 0, 0, time.UTC)
	alice := insertTestUser(t, db, "alice@example.com", "Alice", store.UserStatusActive, now)
	bob := insertTestUser(t, db, "bob@example.com", "Bob", store.UserStatusActive, now)
	carol := insertTestUser(t, db, "carol@example.com", "Carol", store.UserStatusActive, now)
	conversation := insertTestConversation(t, db, testConversationInput{
		createdByUserID: alice.ID,
		kind:            store.ConversationKindGroup,
		memberIDs:       []string{alice.ID, bob.ID, carol.ID},
		name:            "产品讨论组",
		now:             now,
	})
	if err := db.Model(&store.ConversationMember{}).
		Where("conversation_id = ? AND member_type = ? AND member_id = ?", conversation.ID, store.ConversationMemberTypeUser, bob.ID).
		Update("role", store.ConversationMemberRoleAdmin).Error; err != nil {
		t.Fatalf("set bob admin: %v", err)
	}
	original := insertTestMessage(t, db, conversation.ID, carol.ID, 1, "需要管理员撤回", now)
	if err := db.Model(&store.Conversation{}).Where("id = ?", conversation.ID).Updates(map[string]any{
		"last_message_at":      original.CreatedAt,
		"last_message_id":      original.ID,
		"last_message_seq":     original.Seq,
		"last_message_summary": original.Summary,
	}).Error; err != nil {
		t.Fatalf("update conversation last message: %v", err)
	}

	resp, body := postJSON(t, server, "/api/client/conversations/"+conversation.ID+"/messages/"+original.ID+"/revoke", map[string]any{}, loginAsUser(t, server, bob.Email))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("revoke status = %d, want 200, body = %#v", resp.StatusCode, body)
	}
	data := requireSuccess(t, body)
	revokedMessage := data["message"].(map[string]any)
	if revokedMessage["revoked_by_user_id"] != bob.ID {
		t.Fatalf("revoked_by_user_id = %v, want %s", revokedMessage["revoked_by_user_id"], bob.ID)
	}
	systemMessage := data["system_message"].(map[string]any)
	systemBody := systemMessage["body"].(map[string]any)
	if systemBody["actor"].(map[string]any)["display_name"] != "Bob" {
		t.Fatalf("system actor = %#v, want Bob", systemBody["actor"])
	}

	var storedSystemMessage store.Message
	if err := db.First(&storedSystemMessage, "conversation_id = ? AND seq = ?", conversation.ID, int64(2)).Error; err != nil {
		t.Fatalf("find stored system message: %v", err)
	}
	if storedSystemMessage.Summary != "Bob 撤回了一条消息" {
		t.Fatalf("stored system summary = %v, want Bob 撤回了一条消息", storedSystemMessage.Summary)
	}
	requireMessageRevokedBody(t, storedSystemMessage.Body, bob.ID, "Bob")
}

func TestRevokeConversationMessageRejectsUnauthorizedUsers(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()
	now := time.Date(2026, 7, 8, 10, 0, 0, 0, time.UTC)
	alice := insertTestUser(t, db, "alice@example.com", "Alice", store.UserStatusActive, now)
	bob := insertTestUser(t, db, "bob@example.com", "Bob", store.UserStatusActive, now)
	carol := insertTestUser(t, db, "carol@example.com", "Carol", store.UserStatusActive, now)
	direct := insertTestConversation(t, db, testConversationInput{
		createdByUserID: alice.ID,
		kind:            store.ConversationKindDirect,
		memberIDs:       []string{alice.ID, bob.ID},
		now:             now,
	})
	bobDirectMessage := insertTestMessage(t, db, direct.ID, bob.ID, 1, "Alice 不能撤回", now)

	directResp, directBody := postJSON(t, server, "/api/client/conversations/"+direct.ID+"/messages/"+bobDirectMessage.ID+"/revoke", map[string]any{}, loginAsUser(t, server, alice.Email))
	if directResp.StatusCode != http.StatusForbidden {
		t.Fatalf("direct revoke status = %d, want 403, body = %#v", directResp.StatusCode, directBody)
	}
	requireError(t, directBody, "forbidden")

	group := insertTestConversation(t, db, testConversationInput{
		createdByUserID: alice.ID,
		kind:            store.ConversationKindGroup,
		memberIDs:       []string{alice.ID, bob.ID, carol.ID},
		name:            "产品讨论组",
		now:             now,
	})
	bobGroupMessage := insertTestMessage(t, db, group.ID, bob.ID, 1, "Carol 不能撤回", now)

	groupResp, groupBody := postJSON(t, server, "/api/client/conversations/"+group.ID+"/messages/"+bobGroupMessage.ID+"/revoke", map[string]any{}, loginAsUser(t, server, carol.Email))
	if groupResp.StatusCode != http.StatusForbidden {
		t.Fatalf("group revoke status = %d, want 403, body = %#v", groupResp.StatusCode, groupBody)
	}
	requireError(t, groupBody, "forbidden")
}

func TestCreateConversationMessageStoresReplyToAndReturnsReference(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()
	now := time.Date(2026, 7, 8, 9, 0, 0, 0, time.UTC)
	alice := insertTestUser(t, db, "alice@example.com", "Alice", store.UserStatusActive, now)
	bob := insertTestUser(t, db, "bob@example.com", "Bob", store.UserStatusActive, now)
	conversation := insertTestConversation(t, db, testConversationInput{
		createdByUserID: alice.ID,
		kind:            store.ConversationKindDirect,
		memberIDs:       []string{alice.ID, bob.ID},
		now:             now,
	})

	quotedResp, quotedBody := postJSON(t, server, "/api/client/conversations/"+conversation.ID+"/messages", map[string]any{
		"client_message_id": "client-message-quoted",
		"body": map[string]any{
			"type":    "text",
			"content": "这条消息需要被引用",
		},
	}, loginAsUser(t, server, bob.Email))
	if quotedResp.StatusCode != http.StatusCreated {
		t.Fatalf("quoted status = %d, want 201, body = %#v", quotedResp.StatusCode, quotedBody)
	}
	quotedMessage := requireSuccess(t, quotedBody)["message"].(map[string]any)
	quotedMessageID := quotedMessage["id"].(string)

	resp, body := postJSON(t, server, "/api/client/conversations/"+conversation.ID+"/messages", map[string]any{
		"client_message_id":   "client-message-reply",
		"reply_to_message_id": quotedMessageID,
		"body": map[string]any{
			"type":    "markdown",
			"content": "收到，我来处理",
		},
	}, loginAsUser(t, server, alice.Email))
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201, body = %#v", resp.StatusCode, body)
	}
	message := requireSuccess(t, body)["message"].(map[string]any)
	if message["reply_to_message_id"] != quotedMessageID {
		t.Fatalf("message.reply_to_message_id = %v, want %s", message["reply_to_message_id"], quotedMessageID)
	}
	replyTo := message["reply_to"].(map[string]any)
	if replyTo["id"] != quotedMessageID {
		t.Fatalf("reply_to.id = %v, want %s", replyTo["id"], quotedMessageID)
	}
	if replyTo["summary"] != "这条消息需要被引用" {
		t.Fatalf("reply_to.summary = %v, want quoted summary", replyTo["summary"])
	}
	sender := replyTo["sender"].(map[string]any)
	if sender["type"] != store.MessageSenderTypeUser || sender["id"] != bob.ID || sender["name"] != "Bob" {
		t.Fatalf("reply_to.sender = %#v, want Bob user sender", sender)
	}

	var storedMessage store.Message
	if err := db.First(&storedMessage, "id = ?", message["id"]).Error; err != nil {
		t.Fatalf("find stored message: %v", err)
	}
	if storedMessage.ReplyToMessageID == nil || *storedMessage.ReplyToMessageID != quotedMessageID {
		t.Fatalf("stored reply_to_message_id = %v, want %s", storedMessage.ReplyToMessageID, quotedMessageID)
	}

	historyResp, historyBody := getJSON(t, server, "/api/client/conversations/"+conversation.ID+"/messages", loginAsUser(t, server, alice.Email))
	if historyResp.StatusCode != http.StatusOK {
		t.Fatalf("history status = %d, want 200, body = %#v", historyResp.StatusCode, historyBody)
	}
	messages := requireMessages(t, requireSuccess(t, historyBody))
	historyReply := messages[len(messages)-1].(map[string]any)
	if historyReply["reply_to_message_id"] != quotedMessageID {
		t.Fatalf("history reply_to_message_id = %v, want %s", historyReply["reply_to_message_id"], quotedMessageID)
	}
	if historyReply["reply_to"].(map[string]any)["summary"] != "这条消息需要被引用" {
		t.Fatalf("history reply_to = %#v, want quoted summary", historyReply["reply_to"])
	}
}

func TestListConversationMessagesKeepsReplySenderNameForDeletedApp(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()
	now := time.Date(2026, 7, 8, 9, 0, 0, 0, time.UTC)
	alice := insertTestUser(t, db, "alice@example.com", "Alice", store.UserStatusActive, now)
	app := insertTestApp(t, db, store.App{
		Name:             "知识库助手",
		Enabled:          true,
		Visibility:       store.AppVisibilityPublic,
		ConnectionSecret: "kb-app-secret",
		CreatedAt:        now,
		UpdatedAt:        now,
	})
	aliceCookie := loginAsUser(t, server, alice.Email)
	adminCookie := loginAsAdmin(t, server)

	createConversationResp, createConversationBody := postJSON(t, server, "/api/client/conversations/apps", map[string]any{
		"app_id": app.ID,
	}, aliceCookie)
	if createConversationResp.StatusCode != http.StatusCreated {
		t.Fatalf("create app conversation status = %d, want 201, body = %#v", createConversationResp.StatusCode, createConversationBody)
	}
	conversation := requireSuccess(t, createConversationBody)["conversation"].(map[string]any)
	conversationID := conversation["id"].(string)

	appClientMessageID := "app-message-quoted"
	appSenderID := app.ID
	appMessage := store.Message{
		ID:              uuid.NewString(),
		ConversationID:  conversationID,
		Seq:             1,
		SenderType:      store.MessageSenderTypeApp,
		SenderID:        &appSenderID,
		ClientMessageID: &appClientMessageID,
		Body:            json.RawMessage(`{"type":"text","content":"引用这条应用消息"}`),
		Summary:         "引用这条应用消息",
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := db.Create(&appMessage).Error; err != nil {
		t.Fatalf("create app message: %v", err)
	}
	if err := db.Model(&store.Conversation{}).Where("id = ?", conversationID).Update("last_message_seq", int64(1)).Error; err != nil {
		t.Fatalf("update conversation seq: %v", err)
	}

	replyResp, replyBody := postJSON(t, server, "/api/client/conversations/"+conversationID+"/messages", map[string]any{
		"client_message_id":   "client-message-reply-to-app",
		"reply_to_message_id": appMessage.ID,
		"body": map[string]any{
			"type":    "text",
			"content": "收到",
		},
	}, aliceCookie)
	if replyResp.StatusCode != http.StatusCreated {
		t.Fatalf("reply status = %d, want 201, body = %#v", replyResp.StatusCode, replyBody)
	}

	deleteResp, deleteBody := requestJSON(t, server, http.MethodDelete, "/api/admin/apps/"+app.ID, map[string]any{}, adminCookie)
	if deleteResp.StatusCode != http.StatusOK {
		t.Fatalf("delete app status = %d, want 200, body = %#v", deleteResp.StatusCode, deleteBody)
	}
	var rawAppCount int64
	if err := db.Raw("SELECT count(*) FROM apps WHERE id = ?", app.ID).Scan(&rawAppCount).Error; err != nil {
		t.Fatalf("count raw app rows: %v", err)
	}
	if rawAppCount != 1 {
		t.Fatalf("raw app row count = %d, want 1", rawAppCount)
	}

	historyResp, historyBody := getJSON(t, server, "/api/client/conversations/"+conversationID+"/messages", aliceCookie)
	if historyResp.StatusCode != http.StatusOK {
		t.Fatalf("history status = %d, want 200, body = %#v", historyResp.StatusCode, historyBody)
	}
	messages := requireMessages(t, requireSuccess(t, historyBody))
	historyReply := messages[len(messages)-1].(map[string]any)
	replyTo := historyReply["reply_to"].(map[string]any)
	sender := replyTo["sender"].(map[string]any)
	if sender["type"] != store.MessageSenderTypeApp || sender["id"] != app.ID || sender["name"] != app.Name {
		t.Fatalf("reply_to.sender = %#v, want deleted app sender", sender)
	}
}

func TestCreateConversationMessageRejectsReplyToOutsideCurrentConversation(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()
	now := time.Date(2026, 7, 8, 9, 0, 0, 0, time.UTC)
	alice := insertTestUser(t, db, "alice@example.com", "Alice", store.UserStatusActive, now)
	bob := insertTestUser(t, db, "bob@example.com", "Bob", store.UserStatusActive, now)
	carol := insertTestUser(t, db, "carol@example.com", "Carol", store.UserStatusActive, now)
	currentConversation := insertTestConversation(t, db, testConversationInput{
		createdByUserID: alice.ID,
		kind:            store.ConversationKindDirect,
		memberIDs:       []string{alice.ID, bob.ID},
		now:             now,
	})
	otherConversation := insertTestConversation(t, db, testConversationInput{
		createdByUserID: alice.ID,
		kind:            store.ConversationKindDirect,
		memberIDs:       []string{alice.ID, carol.ID},
		now:             now,
	})
	otherMessage := insertTestMessage(t, db, otherConversation.ID, carol.ID, 1, "其他会话消息", now)

	resp, body := postJSON(t, server, "/api/client/conversations/"+currentConversation.ID+"/messages", map[string]any{
		"client_message_id":   "client-message-invalid-reply",
		"reply_to_message_id": otherMessage.ID,
		"body": map[string]any{
			"type":    "text",
			"content": "不能引用其他会话",
		},
	}, loginAsUser(t, server, alice.Email))
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %#v", resp.StatusCode, body)
	}
	requireError(t, body, "invalid_request")
}

func TestCreateConversationMessageRejectsReplyToHiddenByHistoryVisibility(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()
	now := time.Date(2026, 7, 8, 9, 0, 0, 0, time.UTC)
	alice := insertTestUser(t, db, "alice@example.com", "Alice", store.UserStatusActive, now)
	bob := insertTestUser(t, db, "bob@example.com", "Bob", store.UserStatusActive, now)
	conversation := insertTestConversation(t, db, testConversationInput{
		createdByUserID: alice.ID,
		kind:            store.ConversationKindDirect,
		memberIDs:       []string{alice.ID, bob.ID},
		now:             now,
	})
	hiddenMessage := insertTestMessage(t, db, conversation.ID, bob.ID, 1, "Alice 看不到这条", now)
	if err := db.Model(&store.Conversation{}).Where("id = ?", conversation.ID).Update("last_message_seq", int64(1)).Error; err != nil {
		t.Fatalf("update conversation seq: %v", err)
	}
	if err := db.Model(&store.ConversationMember{}).
		Where("conversation_id = ? AND member_type = ? AND member_id = ?", conversation.ID, store.ConversationMemberTypeUser, alice.ID).
		Update("history_visible_from_seq", int64(2)).Error; err != nil {
		t.Fatalf("set history_visible_from_seq: %v", err)
	}

	resp, body := postJSON(t, server, "/api/client/conversations/"+conversation.ID+"/messages", map[string]any{
		"client_message_id":   "client-message-hidden-reply",
		"reply_to_message_id": hiddenMessage.ID,
		"body": map[string]any{
			"type":    "text",
			"content": "不能引用不可见历史",
		},
	}, loginAsUser(t, server, alice.Email))
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %#v", resp.StatusCode, body)
	}
	requireError(t, body, "invalid_request")
}

func TestCreateConversationMarkdownMessageNormalizesAndSummarizes(t *testing.T) {
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

	content := `
# 今日总结

你好 **Bob**

- 第一项
- 第二项

> 保持关注
`
	resp, body := postJSON(t, server, "/api/client/conversations/"+conversation.ID+"/messages", map[string]any{
		"client_message_id": "client-markdown-message-1",
		"body": map[string]any{
			"type":    "markdown",
			"content": content,
		},
	}, loginAsUser(t, server, alice.Email))
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201, body = %#v", resp.StatusCode, body)
	}
	createdMessage := requireSuccess(t, body)["message"].(map[string]any)
	messageBody := createdMessage["body"].(map[string]any)
	if messageBody["type"] != "markdown" {
		t.Fatalf("message.body.type = %v, want markdown", messageBody["type"])
	}
	normalizedContent := strings.TrimSpace(content)
	if messageBody["content"] != normalizedContent {
		t.Fatalf("message.body.content = %q, want normalized markdown", messageBody["content"])
	}

	var storedMessage store.Message
	if err := db.First(&storedMessage, "id = ?", createdMessage["id"]).Error; err != nil {
		t.Fatalf("find stored message: %v", err)
	}
	wantSummary := strings.Join([]string{
		"今日总结",
		"你好 Bob",
		"第一项",
		"第二项",
		"保持关注",
	}, "\n")
	if storedMessage.Summary != wantSummary {
		t.Fatalf("stored summary = %q, want %q", storedMessage.Summary, wantSummary)
	}
}

func TestCreateConversationMarkdownMessageSummarizesTable(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()
	now := time.Date(2026, 7, 8, 9, 0, 0, 0, time.UTC)
	alice := insertTestUser(t, db, "alice@example.com", "Alice", store.UserStatusActive, now)
	bob := insertTestUser(t, db, "bob@example.com", "Bob", store.UserStatusActive, now)
	conversation := insertTestConversation(t, db, testConversationInput{
		createdByUserID: alice.ID,
		kind:            store.ConversationKindDirect,
		memberIDs:       []string{alice.ID, bob.ID},
		now:             now,
	})

	content := `
| 姓名 | 年龄 |
| --- | ---: |
| 张三 | 18 |
| 李四 | 20 |
`
	resp, body := postJSON(t, server, "/api/client/conversations/"+conversation.ID+"/messages", map[string]any{
		"client_message_id": "client-markdown-table-message-1",
		"body": map[string]any{
			"type":    "markdown",
			"content": content,
		},
	}, loginAsUser(t, server, alice.Email))
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201, body = %#v", resp.StatusCode, body)
	}
	createdMessage := requireSuccess(t, body)["message"].(map[string]any)

	var storedMessage store.Message
	if err := db.First(&storedMessage, "id = ?", createdMessage["id"]).Error; err != nil {
		t.Fatalf("find stored message: %v", err)
	}
	wantSummary := strings.Join([]string{
		"姓名 年龄",
		"张三 18",
		"李四 20",
	}, "\n")
	if storedMessage.Summary != wantSummary {
		t.Fatalf("stored summary = %q, want %q", storedMessage.Summary, wantSummary)
	}
}

func TestCreateConversationMarkdownMessageSummarizesStrikethroughAndThematicBreak(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()
	now := time.Date(2026, 7, 8, 9, 0, 0, 0, time.UTC)
	alice := insertTestUser(t, db, "alice@example.com", "Alice", store.UserStatusActive, now)
	bob := insertTestUser(t, db, "bob@example.com", "Bob", store.UserStatusActive, now)
	conversation := insertTestConversation(t, db, testConversationInput{
		createdByUserID: alice.ID,
		kind:            store.ConversationKindDirect,
		memberIDs:       []string{alice.ID, bob.ID},
		now:             now,
	})

	content := `
这个计划~~废弃~~保留

---

继续推进
`
	resp, body := postJSON(t, server, "/api/client/conversations/"+conversation.ID+"/messages", map[string]any{
		"client_message_id": "client-markdown-strike-hr-message-1",
		"body": map[string]any{
			"type":    "markdown",
			"content": content,
		},
	}, loginAsUser(t, server, alice.Email))
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201, body = %#v", resp.StatusCode, body)
	}
	createdMessage := requireSuccess(t, body)["message"].(map[string]any)

	var storedMessage store.Message
	if err := db.First(&storedMessage, "id = ?", createdMessage["id"]).Error; err != nil {
		t.Fatalf("find stored message: %v", err)
	}
	wantSummary := strings.Join([]string{
		"这个计划废弃保留",
		"继续推进",
	}, "\n")
	if storedMessage.Summary != wantSummary {
		t.Fatalf("stored summary = %q, want %q", storedMessage.Summary, wantSummary)
	}
}

func TestCreateConversationMarkdownMessageAllowsUnsupportedRenderSyntax(t *testing.T) {
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

	content := `
[官网](https://example.com)

<https://docs.example.com>

<strong>HTML 由客户端决定是否渲染</strong>

![图片](https://example.com/a.png)
`
	resp, body := postJSON(t, server, "/api/client/conversations/"+conversation.ID+"/messages", map[string]any{
		"client_message_id": "client-markdown-message-1",
		"body": map[string]any{
			"type":    "markdown",
			"content": content,
		},
	}, loginAsUser(t, server, alice.Email))
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201, body = %#v", resp.StatusCode, body)
	}
	createdMessage := requireSuccess(t, body)["message"].(map[string]any)
	messageBody := createdMessage["body"].(map[string]any)
	if messageBody["content"] != strings.TrimSpace(content) {
		t.Fatalf("message.body.content = %q, want original markdown", messageBody["content"])
	}

	var storedMessage store.Message
	if err := db.First(&storedMessage, "id = ?", createdMessage["id"]).Error; err != nil {
		t.Fatalf("find stored message: %v", err)
	}
	if !strings.Contains(storedMessage.Summary, "官网") {
		t.Fatalf("stored summary = %q, want link label", storedMessage.Summary)
	}
}

func TestCreateConversationLinkMessageNormalizesAndSummarizes(t *testing.T) {
	previousFetch := fetchLinkPreviewTitle
	fetchCount := 0
	fetchLinkPreviewTitle = func(_ context.Context, linkURL string) (string, error) {
		fetchCount++
		if linkURL != "https://example.com/docs?tab=api" {
			t.Fatalf("linkURL = %q, want normalized URL", linkURL)
		}
		return "  Example Docs  ", nil
	}
	defer func() {
		fetchLinkPreviewTitle = previousFetch
	}()

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

	resp, body := postJSON(t, server, "/api/client/conversations/"+conversation.ID+"/messages", map[string]any{
		"client_message_id": "client-link-message-1",
		"body": map[string]any{
			"type": "link",
			"url":  " https://example.com/docs?tab=api ",
		},
	}, loginAsUser(t, server, alice.Email))
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201, body = %#v", resp.StatusCode, body)
	}
	createdMessage := requireSuccess(t, body)["message"].(map[string]any)
	messageBody := createdMessage["body"].(map[string]any)
	if messageBody["type"] != "link" {
		t.Fatalf("message.body.type = %v, want link", messageBody["type"])
	}
	if messageBody["url"] != "https://example.com/docs?tab=api" {
		t.Fatalf("message.body.url = %v, want normalized URL", messageBody["url"])
	}
	if messageBody["title"] != "Example Docs" {
		t.Fatalf("message.body.title = %v, want Example Docs", messageBody["title"])
	}

	var storedMessage store.Message
	if err := db.First(&storedMessage, "id = ?", createdMessage["id"]).Error; err != nil {
		t.Fatalf("find stored message: %v", err)
	}
	if storedMessage.Summary != "[链接] Example Docs" {
		t.Fatalf("stored summary = %q, want link summary", storedMessage.Summary)
	}

	duplicateResp, duplicateBody := postJSON(t, server, "/api/client/conversations/"+conversation.ID+"/messages", map[string]any{
		"client_message_id": "client-link-message-1",
		"body": map[string]any{
			"type": "link",
			"url":  "https://example.com/docs?tab=api",
		},
	}, loginAsUser(t, server, alice.Email))
	if duplicateResp.StatusCode != http.StatusOK {
		t.Fatalf("duplicate status = %d, want 200, body = %#v", duplicateResp.StatusCode, duplicateBody)
	}
	duplicateMessage := requireSuccess(t, duplicateBody)["message"].(map[string]any)
	if duplicateMessage["id"] != createdMessage["id"] {
		t.Fatalf("duplicate message id = %v, want %v", duplicateMessage["id"], createdMessage["id"])
	}
	if fetchCount != 1 {
		t.Fatalf("fetch count = %d, want 1", fetchCount)
	}
}

func TestCreateConversationLinkMessageFallsBackToDomain(t *testing.T) {
	previousFetch := fetchLinkPreviewTitle
	fetchLinkPreviewTitle = func(_ context.Context, _ string) (string, error) {
		return "", fmt.Errorf("preview failed")
	}
	defer func() {
		fetchLinkPreviewTitle = previousFetch
	}()

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

	resp, body := postJSON(t, server, "/api/client/conversations/"+conversation.ID+"/messages", map[string]any{
		"client_message_id": "client-link-message-1",
		"body": map[string]any{
			"type": "link",
			"url":  "https://docs.example.com/guide",
		},
	}, loginAsUser(t, server, alice.Email))
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201, body = %#v", resp.StatusCode, body)
	}
	createdMessage := requireSuccess(t, body)["message"].(map[string]any)
	messageBody := createdMessage["body"].(map[string]any)
	if messageBody["title"] != "docs.example.com" {
		t.Fatalf("message.body.title = %v, want docs.example.com", messageBody["title"])
	}
}

func TestCreateConversationLinkMessageDoesNotFetchPreviewBeforeAccessCheck(t *testing.T) {
	previousFetch := fetchLinkPreviewTitle
	fetchLinkPreviewTitle = func(_ context.Context, linkURL string) (string, error) {
		t.Fatalf("fetchLinkPreviewTitle(%q) should not be called before access check", linkURL)
		return "", nil
	}
	defer func() {
		fetchLinkPreviewTitle = previousFetch
	}()

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

	resp, body := postJSON(t, server, "/api/client/conversations/"+conversation.ID+"/messages", map[string]any{
		"client_message_id": "client-link-message-1",
		"body": map[string]any{
			"type": "link",
			"url":  "https://example.com/docs",
		},
	}, loginAsUser(t, server, carol.Email))
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403, body = %#v", resp.StatusCode, body)
	}
	requireError(t, body, "forbidden")
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
	for _, path := range []string{
		"/api/admin/assistant/models",
		"/api/admin/assistant/models/discover",
		"/api/admin/assistant/models/{id}",
		"/api/admin/assistant/models/{id}/health-check",
		"/api/client/assistant/models",
	} {
		if _, ok := paths[path]; ok {
			t.Fatalf("swagger paths include removed assistant model API %s", path)
		}
	}
}

func TestAssistantModelAPIRoutesAreRemoved(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	adminCookie := loginAsAdmin(t, server)
	user := insertTestUser(t, db, "assistant-route-user@example.com", "Assistant Route User", store.UserStatusActive, time.Now().UTC())
	userCookie := loginAsUser(t, server, user.Email)

	for _, tc := range []struct {
		method string
		path   string
		cookie *http.Cookie
	}{
		{method: http.MethodGet, path: "/api/admin/assistant/models", cookie: adminCookie},
		{method: http.MethodPost, path: "/api/admin/assistant/models/discover", cookie: adminCookie},
		{method: http.MethodGet, path: "/api/client/assistant/models", cookie: userCookie},
	} {
		req, err := http.NewRequest(tc.method, server.URL+tc.path, strings.NewReader("{}"))
		if err != nil {
			t.Fatalf("new request %s %s: %v", tc.method, tc.path, err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(tc.cookie)

		resp, err := server.Client().Do(req)
		if err != nil {
			t.Fatalf("%s %s: %v", tc.method, tc.path, err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("%s %s status = %d, want 404", tc.method, tc.path, resp.StatusCode)
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

func TestThirdPartyLoginRejectsProfileWithoutEmail(t *testing.T) {
	var userinfoCalled bool
	identityServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"access-token","token_type":"Bearer"}`))
		case "/userinfo":
			userinfoCalled = true
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
	if callbackResp.StatusCode != http.StatusBadRequest {
		t.Fatalf("callback status = %d, want 400", callbackResp.StatusCode)
	}
	var callbackBody map[string]any
	if err := json.NewDecoder(callbackResp.Body).Decode(&callbackBody); err != nil {
		t.Fatalf("decode callback response: %v", err)
	}
	requireError(t, callbackBody, "invalid_third_party_login")
	if !userinfoCalled {
		t.Fatal("userinfo endpoint was not called")
	}

	var userCount int64
	if err := db.Model(&store.User{}).Count(&userCount).Error; err != nil {
		t.Fatalf("count users: %v", err)
	}
	if userCount != 0 {
		t.Fatalf("user count = %d, want 0", userCount)
	}
	var accountCount int64
	if err := db.Model(&store.ThirdPartyAccount{}).Where("provider_id = ? AND external_user_id = ?", provider.ID, "42").Count(&accountCount).Error; err != nil {
		t.Fatalf("count third-party accounts: %v", err)
	}
	if accountCount != 0 {
		t.Fatalf("third-party account count = %d, want 0", accountCount)
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
					"email": "Personal.User@example.com",
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
						"name": "Ding Real Name",
						"org_email": "Org.User@example.com"
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
	if user.Email != "org.user@example.com" {
		t.Fatalf("user email = %q, want org.user@example.com", user.Email)
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

func TestDingTalkLoginUpdatesLegacySyntheticEmailForBoundUser(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()
	now := time.Date(2026, 7, 8, 9, 0, 0, 0, time.UTC)
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
			"name_field":        "name",
		}),
	})
	user := insertTestUser(t, db, "dingtalk.65e27bd85b300672@third-party.local", "Old Name", store.UserStatusActive, now)
	account := store.ThirdPartyAccount{
		ID:             uuid.NewString(),
		ProviderID:     provider.ID,
		ExternalUserID: "ding-union-id",
		UserID:         user.ID,
		Profile:        json.RawMessage(`{"unionId":"ding-union-id"}`),
	}
	if err := db.Create(&account).Error; err != nil {
		t.Fatalf("create existing dingtalk account: %v", err)
	}

	resolvedUser, err := (&Server{db: db}).findOrCreateThirdPartyUser(provider, externalUserProfile{
		ExternalUserID: "ding-union-id",
		Email:          "Boyang.Lai@Chaitin.com",
		Name:           "Boyang Lai",
		Raw:            json.RawMessage(`{"unionId":"ding-union-id","org_email":"Boyang.Lai@Chaitin.com","name":"Boyang Lai"}`),
	})
	if err != nil {
		t.Fatalf("find or create dingtalk user: %v", err)
	}
	if resolvedUser.ID != user.ID {
		t.Fatalf("resolved user id = %q, want %q", resolvedUser.ID, user.ID)
	}
	if resolvedUser.Email != "boyang.lai@chaitin.com" {
		t.Fatalf("resolved user email = %q, want boyang.lai@chaitin.com", resolvedUser.Email)
	}

	var storedUser store.User
	if err := db.First(&storedUser, "id = ?", user.ID).Error; err != nil {
		t.Fatalf("find stored user: %v", err)
	}
	if storedUser.Email != "boyang.lai@chaitin.com" {
		t.Fatalf("stored user email = %q, want boyang.lai@chaitin.com", storedUser.Email)
	}
	if storedUser.Name != "Boyang Lai" {
		t.Fatalf("stored user name = %q, want Boyang Lai", storedUser.Name)
	}
}

func TestThirdPartyLoginRejectsBoundAccountWithoutEmail(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()
	now := time.Date(2026, 7, 8, 10, 0, 0, 0, time.UTC)
	provider := insertTestThirdPartyLoginProvider(t, db, store.ThirdPartyLoginProvider{
		Name:         "Enterprise SSO",
		Key:          "enterprise",
		Type:         store.ThirdPartyLoginProviderTypeOIDC,
		Enabled:      true,
		ClientID:     "client-id",
		ClientSecret: "client-secret",
		Scopes:       json.RawMessage(`["openid","profile"]`),
		Config: thirdPartyProviderConfig(t, map[string]any{
			"authorize_url":     "https://sso.example.com/authorize",
			"token_url":         "https://sso.example.com/token",
			"userinfo_url":      "https://sso.example.com/userinfo",
			"external_id_field": "sub",
			"name_field":        "name",
		}),
	})
	user := insertTestUser(t, db, "alice@example.com", "Old Name", store.UserStatusActive, now)
	account := store.ThirdPartyAccount{
		ID:             uuid.NewString(),
		ProviderID:     provider.ID,
		ExternalUserID: "alice-external-id",
		UserID:         user.ID,
		Profile:        json.RawMessage(`{"sub":"alice-external-id","name":"Old Name"}`),
	}
	if err := db.Create(&account).Error; err != nil {
		t.Fatalf("create existing third-party account: %v", err)
	}

	_, err := (&Server{db: db}).findOrCreateThirdPartyUser(provider, externalUserProfile{
		ExternalUserID: "alice-external-id",
		Name:           "New Name",
		Raw:            json.RawMessage(`{"sub":"alice-external-id","name":"New Name"}`),
	})
	if err == nil {
		t.Fatal("find or create third-party user error = nil, want invalid_third_party_login")
	}
	userErr, ok := err.(thirdPartyUserError)
	if !ok {
		t.Fatalf("find or create third-party user error = %T %v, want thirdPartyUserError", err, err)
	}
	if userErr.status != http.StatusBadRequest || userErr.code != "invalid_third_party_login" {
		t.Fatalf("third-party user error = status %d code %q, want 400 invalid_third_party_login", userErr.status, userErr.code)
	}

	var storedUser store.User
	if err := db.First(&storedUser, "id = ?", user.ID).Error; err != nil {
		t.Fatalf("find stored user: %v", err)
	}
	if storedUser.Name != "Old Name" {
		t.Fatalf("stored user name = %q, want Old Name", storedUser.Name)
	}
	var storedAccount store.ThirdPartyAccount
	if err := db.First(&storedAccount, "id = ?", account.ID).Error; err != nil {
		t.Fatalf("find stored third-party account: %v", err)
	}
	if string(storedAccount.Profile) != string(account.Profile) {
		t.Fatalf("stored account profile = %s, want %s", storedAccount.Profile, account.Profile)
	}
}

func TestThirdPartyLoginUpdatesExistingEmailUserNameAndPhoneOnly(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()
	now := time.Date(2026, 7, 8, 10, 0, 0, 0, time.UTC)
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
			"phone_field":       "mobile",
			"name_field":        "name",
			"nickname_field":    "nickname",
			"avatar_field":      "picture",
		}),
	})
	user := insertTestUser(t, db, "alice@example.com", "Old Name", store.UserStatusActive, now)
	oldPhone := "+8613800000000"
	if err := db.Model(&user).Updates(map[string]any{
		"avatar":   "https://example.com/old.webp",
		"nickname": "Keep Nick",
		"phone":    oldPhone,
	}).Error; err != nil {
		t.Fatalf("update existing user profile: %v", err)
	}

	resolvedUser, err := (&Server{db: db}).findOrCreateThirdPartyUser(provider, externalUserProfile{
		ExternalUserID: "alice-external-id",
		Email:          "ALICE@example.com",
		Name:           "Alice Real Name",
		Nickname:       "Third Party Nick",
		Phone:          "13900000000",
		Avatar:         "https://example.com/new.webp",
		Raw:            json.RawMessage(`{"sub":"alice-external-id","email":"ALICE@example.com","name":"Alice Real Name","nickname":"Third Party Nick","mobile":"13900000000","picture":"https://example.com/new.webp"}`),
	})
	if err != nil {
		t.Fatalf("find or create third-party user: %v", err)
	}
	if resolvedUser.ID != user.ID {
		t.Fatalf("resolved user id = %q, want %q", resolvedUser.ID, user.ID)
	}

	var storedUser store.User
	if err := db.First(&storedUser, "id = ?", user.ID).Error; err != nil {
		t.Fatalf("find stored user: %v", err)
	}
	if storedUser.Name != "Alice Real Name" {
		t.Fatalf("stored user name = %q, want Alice Real Name", storedUser.Name)
	}
	if storedUser.Phone == nil || *storedUser.Phone != "+8613900000000" {
		t.Fatalf("stored user phone = %#v, want +8613900000000", storedUser.Phone)
	}
	if storedUser.Nickname != "Keep Nick" {
		t.Fatalf("stored user nickname = %q, want Keep Nick", storedUser.Nickname)
	}
	if storedUser.Avatar != "https://example.com/old.webp" {
		t.Fatalf("stored user avatar = %q, want old avatar", storedUser.Avatar)
	}

	var account store.ThirdPartyAccount
	if err := db.First(&account, "provider_id = ? AND external_user_id = ?", provider.ID, "alice-external-id").Error; err != nil {
		t.Fatalf("find third-party account: %v", err)
	}
	if account.UserID != user.ID {
		t.Fatalf("third-party account user_id = %q, want %q", account.UserID, user.ID)
	}
}

func TestThirdPartyLoginRejectsPhoneAlreadyOwnedByAnotherUser(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()
	now := time.Date(2026, 7, 8, 11, 0, 0, 0, time.UTC)
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
			"phone_field":       "mobile",
			"name_field":        "name",
		}),
	})
	alice := insertTestUser(t, db, "alice@example.com", "Old Name", store.UserStatusActive, now)
	bob := insertTestUser(t, db, "bob@example.com", "Bob", store.UserStatusActive, now)
	if err := db.Model(&bob).Update("phone", "+8613900000000").Error; err != nil {
		t.Fatalf("set bob phone: %v", err)
	}

	_, err := (&Server{db: db}).findOrCreateThirdPartyUser(provider, externalUserProfile{
		ExternalUserID: "alice-external-id",
		Email:          "ALICE@example.com",
		Name:           "Alice Real Name",
		Phone:          "13900000000",
		Raw:            json.RawMessage(`{"sub":"alice-external-id","email":"ALICE@example.com","name":"Alice Real Name","mobile":"13900000000"}`),
	})
	if err == nil {
		t.Fatal("find or create third-party user error = nil, want conflict")
	}
	userErr, ok := err.(thirdPartyUserError)
	if !ok {
		t.Fatalf("find or create third-party user error = %T %v, want thirdPartyUserError", err, err)
	}
	if userErr.status != http.StatusConflict || userErr.code != "conflict" {
		t.Fatalf("third-party user error = status %d code %q, want 409 conflict", userErr.status, userErr.code)
	}

	var storedAlice store.User
	if err := db.First(&storedAlice, "id = ?", alice.ID).Error; err != nil {
		t.Fatalf("find stored alice: %v", err)
	}
	if storedAlice.Phone != nil {
		t.Fatalf("stored alice phone = %#v, want nil", storedAlice.Phone)
	}
	if storedAlice.Name != "Old Name" {
		t.Fatalf("stored alice name = %q, want Old Name", storedAlice.Name)
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
					"email": "Personal.Feishu@example.com",
					"enterprise_email": "Enterprise.Feishu@example.com",
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
	if user.Email != "enterprise.feishu@example.com" {
		t.Fatalf("user email = %q, want enterprise.feishu@example.com", user.Email)
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
					"email": "personal.zhangsan@example.com",
					"biz_mail": "zhangsan@example.com",
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
		t.Fatalf("app count = %d, want AI assistant app", len(apps))
	}
	aiAssistantApp := apps[0].(map[string]any)
	if aiAssistantApp["id"] != appregistry.AIAssistantAppID {
		t.Fatalf("AI assistant id = %v, want %s", aiAssistantApp["id"], appregistry.AIAssistantAppID)
	}
	if aiAssistantApp["name"] != appregistry.AIAssistantDefaultName {
		t.Fatalf("AI assistant name = %v, want %s", aiAssistantApp["name"], appregistry.AIAssistantDefaultName)
	}
	if aiAssistantApp["connection_secret"] != "test-ai-assistant-secret" {
		t.Fatalf("AI assistant secret = %v, want configured secret", aiAssistantApp["connection_secret"])
	}
	if aiAssistantApp["connection_status"] != "offline" {
		t.Fatalf("AI assistant connection_status = %v, want offline", aiAssistantApp["connection_status"])
	}
	if aiAssistantApp["system"] != true {
		t.Fatalf("AI assistant system = %v, want true", aiAssistantApp["system"])
	}
	if aiAssistantApp["visibility"] != store.AppVisibilityPublic {
		t.Fatalf("AI assistant visibility = %v, want public", aiAssistantApp["visibility"])
	}

	updateAIAssistantResp, updateAIAssistantBody := putJSON(t, server, "/api/admin/apps/"+appregistry.AIAssistantAppID, map[string]any{
		"name":        "AI 女菩萨 Pro",
		"avatar":      "/assets/apps/assistant.webp",
		"description": "AI Agent",
		"visibility":  "public",
	}, adminCookie)
	if updateAIAssistantResp.StatusCode != http.StatusOK {
		t.Fatalf("update AI assistant status = %d, want 200, body = %#v", updateAIAssistantResp.StatusCode, updateAIAssistantBody)
	}
	updatedAIAssistant := requireSuccess(t, updateAIAssistantBody)["app"].(map[string]any)
	if updatedAIAssistant["name"] != "AI 女菩萨 Pro" {
		t.Fatalf("updated AI assistant name = %v", updatedAIAssistant["name"])
	}
	if updatedAIAssistant["connection_secret"] != "test-ai-assistant-secret" {
		t.Fatalf("updated AI assistant secret = %v, want configured secret", updatedAIAssistant["connection_secret"])
	}
	listAfterUpdateResp, listAfterUpdateBody := getJSON(t, server, "/api/admin/apps", adminCookie)
	if listAfterUpdateResp.StatusCode != http.StatusOK {
		t.Fatalf("list after update status = %d, want 200, body = %#v", listAfterUpdateResp.StatusCode, listAfterUpdateBody)
	}
	appsAfterUpdate := requireSuccess(t, listAfterUpdateBody)["apps"].([]any)
	aiAssistantAfterUpdate := appsAfterUpdate[0].(map[string]any)
	if aiAssistantAfterUpdate["name"] != "AI 女菩萨 Pro" {
		t.Fatalf("listed AI assistant name = %v, want AI 女菩萨 Pro", aiAssistantAfterUpdate["name"])
	}
	if aiAssistantAfterUpdate["connection_secret"] != "test-ai-assistant-secret" {
		t.Fatalf("listed AI assistant secret = %v, want configured secret", aiAssistantAfterUpdate["connection_secret"])
	}

	regenerateAIAssistantResp, _ := postJSON(t, server, "/api/admin/apps/"+appregistry.AIAssistantAppID+"/secret/regenerate", map[string]any{}, adminCookie)
	if regenerateAIAssistantResp.StatusCode != http.StatusForbidden {
		t.Fatalf("regenerate AI assistant status = %d, want 403", regenerateAIAssistantResp.StatusCode)
	}
	deleteAIAssistantResp, _ := requestJSON(t, server, http.MethodDelete, "/api/admin/apps/"+appregistry.AIAssistantAppID, map[string]any{}, adminCookie)
	if deleteAIAssistantResp.StatusCode != http.StatusForbidden {
		t.Fatalf("delete AI assistant status = %d, want 403", deleteAIAssistantResp.StatusCode)
	}

	createResp, createBody := postJSON(t, server, "/api/admin/apps", map[string]any{
		"name":        "知识库助手",
		"avatar":      "/assets/apps/kb.webp",
		"description": "回答知识库问题",
		"visibility":  "public",
	}, adminCookie)
	if createResp.StatusCode != http.StatusCreated {
		t.Fatalf("create status = %d, want 201, body = %#v", createResp.StatusCode, createBody)
	}
	createdApp := requireSuccess(t, createBody)["app"].(map[string]any)
	appID := createdApp["id"].(string)
	firstSecret := createdApp["connection_secret"].(string)
	if appID == "" {
		t.Fatal("created app id is empty")
	}
	if firstSecret == "" || firstSecret == "test-ai-assistant-secret" {
		t.Fatalf("created app secret = %q, want generated unique secret", firstSecret)
	}
	if createdApp["system"] != false {
		t.Fatalf("created app system = %v, want false", createdApp["system"])
	}
	if createdApp["creator_user_id"] != nil {
		t.Fatalf("created app creator_user_id = %v, want nil", createdApp["creator_user_id"])
	}
	if createdApp["connection_status"] != "offline" {
		t.Fatalf("created app connection_status = %v, want offline", createdApp["connection_status"])
	}

	updateResp, updateBody := putJSON(t, server, "/api/admin/apps/"+appID, map[string]any{
		"name":        "知识库 Agent",
		"avatar":      "",
		"description": "更新后的介绍",
		"visibility":  "public",
	}, adminCookie)
	if updateResp.StatusCode != http.StatusOK {
		t.Fatalf("update status = %d, want 200, body = %#v", updateResp.StatusCode, updateBody)
	}
	updatedApp := requireSuccess(t, updateBody)["app"].(map[string]any)
	if updatedApp["name"] != "知识库 Agent" {
		t.Fatalf("updated app name = %v", updatedApp["name"])
	}

	disableResp, disableBody := postJSON(t, server, "/api/admin/apps/"+appID+"/disable", map[string]any{}, adminCookie)
	if disableResp.StatusCode != http.StatusOK {
		t.Fatalf("disable status = %d, want 200, body = %#v", disableResp.StatusCode, disableBody)
	}
	disabledApp := requireSuccess(t, disableBody)["app"].(map[string]any)
	if disabledApp["enabled"] != false {
		t.Fatalf("disabled enabled = %v, want false", disabledApp["enabled"])
	}
	if disabledApp["connection_status"] != "disabled" {
		t.Fatalf("disabled connection_status = %v, want disabled", disabledApp["connection_status"])
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
	secondSecret := regeneratedApp["connection_secret"].(string)
	if secondSecret == "" || secondSecret == firstSecret {
		t.Fatalf("regenerated secret = %q, want changed from %q", secondSecret, firstSecret)
	}

	deleteResp, deleteBody := requestJSON(t, server, http.MethodDelete, "/api/admin/apps/"+appID, map[string]any{}, adminCookie)
	if deleteResp.StatusCode != http.StatusOK {
		t.Fatalf("delete status = %d, want 200, body = %#v", deleteResp.StatusCode, deleteBody)
	}
	requireSuccess(t, deleteBody)
}

func requireAdminAppConnectionStatus(t *testing.T, server *httptest.Server, adminCookie *http.Cookie, appID string) string {
	t.Helper()

	resp, body := getJSON(t, server, "/api/admin/apps", adminCookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list status = %d, want 200, body = %#v", resp.StatusCode, body)
	}
	apps := requireSuccess(t, body)["apps"].([]any)
	for _, rawApp := range apps {
		app := rawApp.(map[string]any)
		if app["id"] == appID {
			status, ok := app["connection_status"].(string)
			if !ok {
				t.Fatalf("connection_status = %#v, want string", app["connection_status"])
			}

			return status
		}
	}

	t.Fatalf("admin apps did not include app %s", appID)
	return ""
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
	createdConfig := createdProvider["config"].(map[string]any)
	if createdConfig["email_field"] != "org_email" {
		t.Fatalf("dingtalk config.email_field = %#v, want org_email", createdConfig["email_field"])
	}

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

func TestFeishuProviderDefaultsToEnterpriseEmail(t *testing.T) {
	server, _ := newTestRouter(t)
	defer server.Close()
	adminCookie := loginAsAdmin(t, server)

	createResp, createBody := postJSON(t, server, "/api/admin/third-party/providers", map[string]any{
		"name":          "Feishu",
		"type":          "feishu",
		"client_id":     "feishu-client-id",
		"client_secret": "feishu-client-secret",
	}, adminCookie)
	if createResp.StatusCode != http.StatusCreated {
		t.Fatalf("create status = %d, want 201, body = %#v", createResp.StatusCode, createBody)
	}
	createdProvider := requireSuccess(t, createBody)["provider"].(map[string]any)
	createdConfig := createdProvider["config"].(map[string]any)
	if createdConfig["email_field"] != "enterprise_email" {
		t.Fatalf("feishu config.email_field = %#v, want enterprise_email", createdConfig["email_field"])
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

func TestClientGroupVisibilityCanBeChangedByOwner(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	now := time.Now().UTC()
	alice := insertTestUser(t, db, "alice@example.com", "Alice", store.UserStatusActive, now)
	bob := insertTestUser(t, db, "bob@example.com", "Bob", store.UserStatusActive, now)
	conversation := insertTestConversation(t, db, testConversationInput{
		createdByUserID: alice.ID,
		kind:            store.ConversationKindGroup,
		memberIDs:       []string{alice.ID, bob.ID},
		name:            "产品讨论组",
		now:             now,
	})
	cookie := loginAsUser(t, server, alice.Email)

	publicResp, publicBody := postJSON(t, server, "/api/client/conversations/groups/"+conversation.ID+"/public", map[string]any{}, cookie)
	if publicResp.StatusCode != http.StatusOK {
		t.Fatalf("public status = %d, want 200, body = %#v", publicResp.StatusCode, publicBody)
	}
	publicConversation := requireSuccess(t, publicBody)["conversation"].(map[string]any)
	if publicConversation["visibility"] != store.ConversationVisibilityPublic {
		t.Fatalf("public conversation visibility = %v, want public", publicConversation["visibility"])
	}
	if publicConversation["last_message_summary"] != "Alice 将当前群设置为公开群" {
		t.Fatalf("public summary = %v", publicConversation["last_message_summary"])
	}

	var storedPublic store.Message
	if err := db.First(&storedPublic, "conversation_id = ? AND seq = ?", conversation.ID, int64(1)).Error; err != nil {
		t.Fatalf("find public system message: %v", err)
	}
	if storedPublic.Summary != "Alice 将当前群设置为公开群" {
		t.Fatalf("stored public summary = %v", storedPublic.Summary)
	}
	requireSystemEventActorBody(t, storedPublic.Body, "group_visibility_changed", alice.ID, "Alice")

	repeatResp, repeatBody := postJSON(t, server, "/api/client/conversations/groups/"+conversation.ID+"/public", map[string]any{}, cookie)
	if repeatResp.StatusCode != http.StatusOK {
		t.Fatalf("repeat public status = %d, want 200, body = %#v", repeatResp.StatusCode, repeatBody)
	}
	var messageCount int64
	if err := db.Model(&store.Message{}).Where("conversation_id = ?", conversation.ID).Count(&messageCount).Error; err != nil {
		t.Fatalf("count messages: %v", err)
	}
	if messageCount != 1 {
		t.Fatalf("message count after idempotent public = %d, want 1", messageCount)
	}

	privateResp, privateBody := postJSON(t, server, "/api/client/conversations/groups/"+conversation.ID+"/private", map[string]any{}, cookie)
	if privateResp.StatusCode != http.StatusOK {
		t.Fatalf("private status = %d, want 200, body = %#v", privateResp.StatusCode, privateBody)
	}
	privateConversation := requireSuccess(t, privateBody)["conversation"].(map[string]any)
	if privateConversation["visibility"] != store.ConversationVisibilityPrivate {
		t.Fatalf("private conversation visibility = %v, want private", privateConversation["visibility"])
	}
	if privateConversation["last_message_summary"] != "Alice 将当前群设为私有群" {
		t.Fatalf("private summary = %v", privateConversation["last_message_summary"])
	}

	var storedPrivate store.Message
	if err := db.First(&storedPrivate, "conversation_id = ? AND seq = ?", conversation.ID, int64(2)).Error; err != nil {
		t.Fatalf("find private system message: %v", err)
	}
	if storedPrivate.Summary != "Alice 将当前群设为私有群" {
		t.Fatalf("stored private summary = %v", storedPrivate.Summary)
	}
	requireSystemEventActorBody(t, storedPrivate.Body, "group_visibility_changed", alice.ID, "Alice")
}

func TestClientGroupVisibilityRejectsNonOwner(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	now := time.Now().UTC()
	alice := insertTestUser(t, db, "alice@example.com", "Alice", store.UserStatusActive, now)
	bob := insertTestUser(t, db, "bob@example.com", "Bob", store.UserStatusActive, now)
	conversation := insertTestConversation(t, db, testConversationInput{
		createdByUserID: alice.ID,
		kind:            store.ConversationKindGroup,
		memberIDs:       []string{alice.ID, bob.ID},
		name:            "产品讨论组",
		now:             now,
	})
	cookie := loginAsUser(t, server, bob.Email)

	for _, suffix := range []string{"public", "private"} {
		resp, body := postJSON(t, server, "/api/client/conversations/groups/"+conversation.ID+"/"+suffix, map[string]any{}, cookie)
		if resp.StatusCode != http.StatusForbidden {
			t.Fatalf("%s status = %d, want 403, body = %#v", suffix, resp.StatusCode, body)
		}
		requireError(t, body, "forbidden")
	}
}

func TestUpdateGroupConversationNameCreatesSystemMessage(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	now := time.Now().UTC()
	alice := insertTestUser(t, db, "alice@example.com", "Alice", store.UserStatusActive, now)
	bob := insertTestUser(t, db, "bob@example.com", "Bob", store.UserStatusActive, now)
	conversation := insertTestConversation(t, db, testConversationInput{
		createdByUserID: alice.ID,
		kind:            store.ConversationKindGroup,
		memberIDs:       []string{alice.ID, bob.ID},
		name:            "产品讨论组",
		now:             now,
	})
	cookie := loginAsUser(t, server, alice.Email)

	resp, body := patchJSON(t, server, "/api/client/conversations/groups/"+conversation.ID+"/name", map[string]any{
		"name": " 新产品讨论组 ",
	}, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %#v", resp.StatusCode, body)
	}

	data := requireSuccess(t, body)
	updatedConversation := data["conversation"].(map[string]any)
	createdMessage := data["message"].(map[string]any)
	summary := "Alice 修改群聊名称为 新产品讨论组"
	if updatedConversation["name"] != "新产品讨论组" {
		t.Fatalf("conversation.name = %v, want 新产品讨论组", updatedConversation["name"])
	}
	if updatedConversation["last_message_summary"] != summary {
		t.Fatalf("last_message_summary = %v, want %s", updatedConversation["last_message_summary"], summary)
	}
	if createdMessage["seq"] != float64(1) {
		t.Fatalf("message.seq = %v, want 1", createdMessage["seq"])
	}

	var storedConversation store.Conversation
	if err := db.First(&storedConversation, "id = ?", conversation.ID).Error; err != nil {
		t.Fatalf("find stored conversation: %v", err)
	}
	if storedConversation.Name != "新产品讨论组" {
		t.Fatalf("stored conversation name = %q, want 新产品讨论组", storedConversation.Name)
	}

	var storedMessage store.Message
	if err := db.First(&storedMessage, "conversation_id = ? AND seq = ?", conversation.ID, int64(1)).Error; err != nil {
		t.Fatalf("find stored system message: %v", err)
	}
	if storedMessage.Summary != summary {
		t.Fatalf("stored summary = %v, want %s", storedMessage.Summary, summary)
	}
	requireSystemEventActorBody(t, storedMessage.Body, "group_name_updated", alice.ID, "Alice")

	var systemBody map[string]any
	if err := json.Unmarshal(storedMessage.Body, &systemBody); err != nil {
		t.Fatalf("unmarshal system body: %v", err)
	}
	if systemBody["name"] != "新产品讨论组" {
		t.Fatalf("body.name = %v, want 新产品讨论组", systemBody["name"])
	}
}

func TestUpdateGroupConversationNameRejectsMember(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	now := time.Now().UTC()
	alice := insertTestUser(t, db, "alice@example.com", "Alice", store.UserStatusActive, now)
	bob := insertTestUser(t, db, "bob@example.com", "Bob", store.UserStatusActive, now)
	conversation := insertTestConversation(t, db, testConversationInput{
		createdByUserID: alice.ID,
		kind:            store.ConversationKindGroup,
		memberIDs:       []string{alice.ID, bob.ID},
		name:            "产品讨论组",
		now:             now,
	})

	resp, body := patchJSON(t, server, "/api/client/conversations/groups/"+conversation.ID+"/name", map[string]any{
		"name": "新产品讨论组",
	}, loginAsUser(t, server, bob.Email))
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403, body = %#v", resp.StatusCode, body)
	}
	requireError(t, body, "forbidden")
}

func TestLeaveGroupConversationCreatesSystemMessage(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	now := time.Now().UTC()
	alice := insertTestUser(t, db, "alice@example.com", "Alice", store.UserStatusActive, now)
	bob := insertTestUser(t, db, "bob@example.com", "Bob", store.UserStatusActive, now)
	conversation := insertTestConversation(t, db, testConversationInput{
		createdByUserID: alice.ID,
		kind:            store.ConversationKindGroup,
		memberIDs:       []string{alice.ID, bob.ID},
		name:            "产品讨论组",
		now:             now,
	})

	resp, body := postJSON(t, server, "/api/client/conversations/groups/"+conversation.ID+"/leave", map[string]any{}, loginAsUser(t, server, bob.Email))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %#v", resp.StatusCode, body)
	}
	data := requireSuccess(t, body)
	if data["conversation_id"] != conversation.ID {
		t.Fatalf("conversation_id = %v, want %s", data["conversation_id"], conversation.ID)
	}
	createdMessage := data["message"].(map[string]any)
	if createdMessage["seq"] != float64(1) {
		t.Fatalf("message.seq = %v, want 1", createdMessage["seq"])
	}

	var member store.ConversationMember
	if err := db.First(&member, "conversation_id = ? AND member_type = ? AND member_id = ?", conversation.ID, store.ConversationMemberTypeUser, bob.ID).Error; err != nil {
		t.Fatalf("find left member: %v", err)
	}
	if member.LeftAt == nil {
		t.Fatal("left_at = nil, want set")
	}

	var storedMessage store.Message
	if err := db.First(&storedMessage, "conversation_id = ? AND seq = ?", conversation.ID, int64(1)).Error; err != nil {
		t.Fatalf("find stored system message: %v", err)
	}
	if storedMessage.Summary != "Bob 已退出群聊" {
		t.Fatalf("stored summary = %v, want Bob 已退出群聊", storedMessage.Summary)
	}
	requireSystemEventActorBody(t, storedMessage.Body, "group_member_left", bob.ID, "Bob")

	var activeMembers int64
	if err := db.Model(&store.ConversationMember{}).
		Where("conversation_id = ? AND member_type = ? AND left_at IS NULL", conversation.ID, store.ConversationMemberTypeUser).
		Count(&activeMembers).Error; err != nil {
		t.Fatalf("count active members: %v", err)
	}
	if activeMembers != 1 {
		t.Fatalf("active members = %d, want 1", activeMembers)
	}
}

func TestLeaveGroupConversationRejectsOwner(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	now := time.Now().UTC()
	alice := insertTestUser(t, db, "alice@example.com", "Alice", store.UserStatusActive, now)
	bob := insertTestUser(t, db, "bob@example.com", "Bob", store.UserStatusActive, now)
	conversation := insertTestConversation(t, db, testConversationInput{
		createdByUserID: alice.ID,
		kind:            store.ConversationKindGroup,
		memberIDs:       []string{alice.ID, bob.ID},
		name:            "产品讨论组",
		now:             now,
	})

	resp, body := postJSON(t, server, "/api/client/conversations/groups/"+conversation.ID+"/leave", map[string]any{}, loginAsUser(t, server, alice.Email))
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403, body = %#v", resp.StatusCode, body)
	}
	requireError(t, body, "forbidden")

	var member store.ConversationMember
	if err := db.First(&member, "conversation_id = ? AND member_type = ? AND member_id = ?", conversation.ID, store.ConversationMemberTypeUser, alice.ID).Error; err != nil {
		t.Fatalf("find owner member: %v", err)
	}
	if member.LeftAt != nil {
		t.Fatalf("owner left_at = %v, want nil", member.LeftAt)
	}
}

func TestDissolveGroupConversationMarksConversationDissolved(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	now := time.Now().UTC()
	alice := insertTestUser(t, db, "alice@example.com", "Alice", store.UserStatusActive, now)
	bob := insertTestUser(t, db, "bob@example.com", "Bob", store.UserStatusActive, now)
	conversation := insertTestConversation(t, db, testConversationInput{
		createdByUserID: alice.ID,
		kind:            store.ConversationKindGroup,
		memberIDs:       []string{alice.ID, bob.ID},
		name:            "产品讨论组",
		now:             now,
	})

	resp, body := requestJSON(t, server, http.MethodDelete, "/api/client/conversations/groups/"+conversation.ID, map[string]any{}, loginAsUser(t, server, alice.Email))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %#v", resp.StatusCode, body)
	}
	data := requireSuccess(t, body)
	if data["conversation_id"] != conversation.ID {
		t.Fatalf("conversation_id = %v, want %s", data["conversation_id"], conversation.ID)
	}

	var storedConversation store.Conversation
	if err := db.First(&storedConversation, "id = ?", conversation.ID).Error; err != nil {
		t.Fatalf("find dissolved conversation: %v", err)
	}
	if storedConversation.Status != store.ConversationStatusDissolved {
		t.Fatalf("status = %s, want %s", storedConversation.Status, store.ConversationStatusDissolved)
	}
	if storedConversation.DissolvedAt == nil {
		t.Fatal("dissolved_at = nil, want set")
	}
}

func TestDissolveGroupConversationRejectsMember(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	now := time.Now().UTC()
	alice := insertTestUser(t, db, "alice@example.com", "Alice", store.UserStatusActive, now)
	bob := insertTestUser(t, db, "bob@example.com", "Bob", store.UserStatusActive, now)
	conversation := insertTestConversation(t, db, testConversationInput{
		createdByUserID: alice.ID,
		kind:            store.ConversationKindGroup,
		memberIDs:       []string{alice.ID, bob.ID},
		name:            "产品讨论组",
		now:             now,
	})

	resp, body := requestJSON(t, server, http.MethodDelete, "/api/client/conversations/groups/"+conversation.ID, map[string]any{}, loginAsUser(t, server, bob.Email))
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403, body = %#v", resp.StatusCode, body)
	}
	requireError(t, body, "forbidden")

	var storedConversation store.Conversation
	if err := db.First(&storedConversation, "id = ?", conversation.ID).Error; err != nil {
		t.Fatalf("find conversation: %v", err)
	}
	if storedConversation.Status != store.ConversationStatusActive {
		t.Fatalf("status = %s, want %s", storedConversation.Status, store.ConversationStatusActive)
	}
}

func TestRemoveGroupConversationMemberCreatesSystemMessage(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	now := time.Now().UTC()
	alice := insertTestUser(t, db, "alice@example.com", "Alice", store.UserStatusActive, now)
	bob := insertTestUser(t, db, "bob@example.com", "Bob", store.UserStatusActive, now)
	carol := insertTestUser(t, db, "carol@example.com", "Carol", store.UserStatusActive, now)
	dave := insertTestUser(t, db, "dave@example.com", "Dave", store.UserStatusActive, now)
	conversation := insertTestConversation(t, db, testConversationInput{
		createdByUserID: alice.ID,
		kind:            store.ConversationKindGroup,
		memberIDs:       []string{alice.ID, bob.ID, carol.ID, dave.ID},
		name:            "产品讨论组",
		now:             now,
	})
	if err := db.Model(&store.ConversationMember{}).
		Where("conversation_id = ? AND member_type = ? AND member_id IN ?", conversation.ID, store.ConversationMemberTypeUser, []string{bob.ID, carol.ID}).
		Update("role", store.ConversationMemberRoleAdmin).Error; err != nil {
		t.Fatalf("set admin roles: %v", err)
	}
	setTestConversationMemberLastReadSeq(t, db, conversation.ID, bob.ID, 0)
	carolCookie := loginAsUser(t, server, carol.Email)
	carolConn := dialClientWebSocket(t, server, carolCookie)
	if ready := readRealtimeEvent(t, carolConn); ready.Kind != realtime.KindEvent || ready.Event != realtime.EventSystemReady {
		t.Fatalf("ready event = %#v", ready)
	}

	resp, body := requestJSON(t, server, http.MethodDelete, "/api/client/conversations/groups/"+conversation.ID+"/members/"+carol.ID, map[string]any{}, loginAsUser(t, server, bob.Email))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %#v", resp.StatusCode, body)
	}
	data := requireSuccess(t, body)
	updatedConversation := data["conversation"].(map[string]any)
	createdMessage := data["message"].(map[string]any)
	summary := "Bob 已将 Carol 移出群聊"
	if updatedConversation["member_count"] != float64(3) {
		t.Fatalf("conversation.member_count = %v, want 3", updatedConversation["member_count"])
	}
	if updatedConversation["last_message_summary"] != summary {
		t.Fatalf("last_message_summary = %v, want %s", updatedConversation["last_message_summary"], summary)
	}
	if createdMessage["seq"] != float64(1) {
		t.Fatalf("message.seq = %v, want 1", createdMessage["seq"])
	}

	var member store.ConversationMember
	if err := db.First(&member, "conversation_id = ? AND member_type = ? AND member_id = ?", conversation.ID, store.ConversationMemberTypeUser, carol.ID).Error; err != nil {
		t.Fatalf("find removed member: %v", err)
	}
	if member.LeftAt == nil {
		t.Fatal("left_at = nil, want set")
	}
	if got := getTestConversationMemberLastReadSeq(t, db, conversation.ID, bob.ID); got != 1 {
		t.Fatalf("bob last_read_seq = %d, want 1", got)
	}

	var storedMessage store.Message
	if err := db.First(&storedMessage, "conversation_id = ? AND seq = ?", conversation.ID, int64(1)).Error; err != nil {
		t.Fatalf("find stored system message: %v", err)
	}
	if storedMessage.Summary != summary {
		t.Fatalf("stored summary = %v, want %s", storedMessage.Summary, summary)
	}
	requireGroupMemberRemovedBody(t, storedMessage.Body, bob.ID, "Bob", carol.ID, "Carol")

	removedEvent := readRealtimeEvent(t, carolConn)
	if removedEvent.Kind != realtime.KindEvent || removedEvent.Event != realtime.EventConversationRemoved {
		t.Fatalf("removed event = %#v", removedEvent)
	}
	var removedPayload map[string]any
	if err := json.Unmarshal(removedEvent.Payload, &removedPayload); err != nil {
		t.Fatalf("unmarshal removed payload: %v", err)
	}
	if removedPayload["conversation_id"] != conversation.ID {
		t.Fatalf("removed conversation_id = %v, want %s", removedPayload["conversation_id"], conversation.ID)
	}
}

func TestRemoveGroupConversationMemberRejectsOwnerTarget(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	now := time.Now().UTC()
	alice := insertTestUser(t, db, "alice@example.com", "Alice", store.UserStatusActive, now)
	bob := insertTestUser(t, db, "bob@example.com", "Bob", store.UserStatusActive, now)
	conversation := insertTestConversation(t, db, testConversationInput{
		createdByUserID: alice.ID,
		kind:            store.ConversationKindGroup,
		memberIDs:       []string{alice.ID, bob.ID},
		name:            "产品讨论组",
		now:             now,
	})
	if err := db.Model(&store.ConversationMember{}).
		Where("conversation_id = ? AND member_type = ? AND member_id = ?", conversation.ID, store.ConversationMemberTypeUser, bob.ID).
		Update("role", store.ConversationMemberRoleAdmin).Error; err != nil {
		t.Fatalf("set admin role: %v", err)
	}

	resp, body := requestJSON(t, server, http.MethodDelete, "/api/client/conversations/groups/"+conversation.ID+"/members/"+alice.ID, map[string]any{}, loginAsUser(t, server, bob.Email))
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403, body = %#v", resp.StatusCode, body)
	}
	requireError(t, body, "forbidden")

	var member store.ConversationMember
	if err := db.First(&member, "conversation_id = ? AND member_type = ? AND member_id = ?", conversation.ID, store.ConversationMemberTypeUser, alice.ID).Error; err != nil {
		t.Fatalf("find owner member: %v", err)
	}
	if member.LeftAt != nil {
		t.Fatalf("owner left_at = %v, want nil", member.LeftAt)
	}
}

func TestRemoveGroupConversationMemberRejectsRegularMember(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	now := time.Now().UTC()
	alice := insertTestUser(t, db, "alice@example.com", "Alice", store.UserStatusActive, now)
	bob := insertTestUser(t, db, "bob@example.com", "Bob", store.UserStatusActive, now)
	carol := insertTestUser(t, db, "carol@example.com", "Carol", store.UserStatusActive, now)
	conversation := insertTestConversation(t, db, testConversationInput{
		createdByUserID: alice.ID,
		kind:            store.ConversationKindGroup,
		memberIDs:       []string{alice.ID, bob.ID, carol.ID},
		name:            "产品讨论组",
		now:             now,
	})

	resp, body := requestJSON(t, server, http.MethodDelete, "/api/client/conversations/groups/"+conversation.ID+"/members/"+bob.ID, map[string]any{}, loginAsUser(t, server, carol.Email))
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403, body = %#v", resp.StatusCode, body)
	}
	requireError(t, body, "forbidden")

	var member store.ConversationMember
	if err := db.First(&member, "conversation_id = ? AND member_type = ? AND member_id = ?", conversation.ID, store.ConversationMemberTypeUser, bob.ID).Error; err != nil {
		t.Fatalf("find target member: %v", err)
	}
	if member.LeftAt != nil {
		t.Fatalf("target left_at = %v, want nil", member.LeftAt)
	}
}

func TestClientJoinPublicGroupCreatesMemberAndSystemMessage(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	now := time.Now().UTC()
	alice := insertTestUser(t, db, "alice@example.com", "Alice", store.UserStatusActive, now)
	bob := insertTestUser(t, db, "bob@example.com", "Bob", store.UserStatusActive, now)
	conversation := insertTestConversation(t, db, testConversationInput{
		createdByUserID: alice.ID,
		kind:            store.ConversationKindGroup,
		memberIDs:       []string{alice.ID},
		name:            "公开讨论组",
		now:             now,
		visibility:      store.ConversationVisibilityPublic,
	})
	cookie := loginAsUser(t, server, bob.Email)

	resp, body := postJSON(t, server, "/api/client/conversations/groups/"+conversation.ID+"/join", map[string]any{}, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("join status = %d, want 200, body = %#v", resp.StatusCode, body)
	}
	data := requireSuccess(t, body)
	joinedConversation := data["conversation"].(map[string]any)
	if joinedConversation["id"] != conversation.ID {
		t.Fatalf("conversation.id = %v, want %s", joinedConversation["id"], conversation.ID)
	}
	if joinedConversation["member_count"] != float64(2) {
		t.Fatalf("member_count = %v, want 2", joinedConversation["member_count"])
	}
	if joinedConversation["last_message_summary"] != "Bob 加入群聊" {
		t.Fatalf("last_message_summary = %v", joinedConversation["last_message_summary"])
	}
	if data["message"] == nil {
		t.Fatal("message = nil, want join system message")
	}

	var member store.ConversationMember
	if err := db.First(&member, "conversation_id = ? AND member_type = ? AND member_id = ?", conversation.ID, store.ConversationMemberTypeUser, bob.ID).Error; err != nil {
		t.Fatalf("find joined member: %v", err)
	}
	if member.LeftAt != nil {
		t.Fatalf("joined member left_at = %v, want nil", member.LeftAt)
	}
	if member.HistoryVisibleFromSeq != 1 {
		t.Fatalf("history_visible_from_seq = %d, want 1", member.HistoryVisibleFromSeq)
	}

	var storedMessage store.Message
	if err := db.First(&storedMessage, "conversation_id = ? AND seq = ?", conversation.ID, int64(1)).Error; err != nil {
		t.Fatalf("find join system message: %v", err)
	}
	if storedMessage.Summary != "Bob 加入群聊" {
		t.Fatalf("stored summary = %v", storedMessage.Summary)
	}
	requireSystemEventActorBody(t, storedMessage.Body, "group_member_joined", bob.ID, "Bob")
}

func TestClientJoinPublicGroupRejectsPrivateOrFullGroup(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	now := time.Now().UTC()
	owner := insertTestUser(t, db, "owner@example.com", "Owner", store.UserStatusActive, now)
	bob := insertTestUser(t, db, "bob@example.com", "Bob", store.UserStatusActive, now)
	privateGroup := insertTestConversation(t, db, testConversationInput{
		createdByUserID: owner.ID,
		kind:            store.ConversationKindGroup,
		memberIDs:       []string{owner.ID},
		name:            "私有群",
		now:             now,
	})
	cookie := loginAsUser(t, server, bob.Email)

	privateResp, privateBody := postJSON(t, server, "/api/client/conversations/groups/"+privateGroup.ID+"/join", map[string]any{}, cookie)
	if privateResp.StatusCode != http.StatusForbidden {
		t.Fatalf("private join status = %d, want 403, body = %#v", privateResp.StatusCode, privateBody)
	}
	requireError(t, privateBody, "forbidden")

	memberIDs := []string{owner.ID}
	for i := 0; i < 99; i++ {
		member := insertTestUser(t, db, fmt.Sprintf("member-%02d@example.com", i), fmt.Sprintf("Member %02d", i), store.UserStatusActive, now)
		memberIDs = append(memberIDs, member.ID)
	}
	fullGroup := insertTestConversation(t, db, testConversationInput{
		createdByUserID: owner.ID,
		kind:            store.ConversationKindGroup,
		memberIDs:       memberIDs,
		name:            "满员群",
		now:             now,
		visibility:      store.ConversationVisibilityPublic,
	})
	fullResp, fullBody := postJSON(t, server, "/api/client/conversations/groups/"+fullGroup.ID+"/join", map[string]any{}, cookie)
	if fullResp.StatusCode != http.StatusBadRequest {
		t.Fatalf("full join status = %d, want 400, body = %#v", fullResp.StatusCode, fullBody)
	}
	requireError(t, fullBody, "invalid_request")
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

func TestAddGroupConversationMembersCanAddApps(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	now := time.Date(2026, 7, 3, 9, 0, 0, 0, time.UTC)
	alice := insertTestUser(t, db, "alice@example.com", "Alice", store.UserStatusActive, now)
	bob := insertTestUser(t, db, "bob@example.com", "Bob", store.UserStatusActive, now)
	app := insertTestApp(t, db, store.App{
		Name:             "AI 女菩萨",
		Avatar:           "/assets/apps/assistant.webp",
		Description:      "AI 助手",
		Enabled:          true,
		Visibility:       store.AppVisibilityPublic,
		ConnectionSecret: "assistant-secret",
		CreatedAt:        now,
		UpdatedAt:        now,
	})
	conversation := insertTestConversation(t, db, testConversationInput{
		createdByUserID:    alice.ID,
		kind:               store.ConversationKindGroup,
		lastMessageSeq:     2,
		lastMessageSummary: "旧消息",
		memberIDs:          []string{alice.ID, bob.ID},
		name:               "产品讨论组",
		now:                now.Add(-2 * time.Hour),
	})
	setTestConversationMemberLastReadSeq(t, db, conversation.ID, alice.ID, 2)

	resp, body := postJSON(t, server, "/api/client/conversations/"+conversation.ID+"/members", map[string]any{
		"app_ids": []string{app.ID},
	}, loginAsUser(t, server, alice.Email))

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %#v", resp.StatusCode, body)
	}
	data := requireSuccess(t, body)
	updatedConversation := data["conversation"].(map[string]any)
	createdMessage := data["message"].(map[string]any)
	summary := "Alice 邀请 AI 女菩萨 加入群聊"
	if updatedConversation["member_count"] != float64(3) {
		t.Fatalf("conversation.member_count = %v, want 3", updatedConversation["member_count"])
	}
	if updatedConversation["last_message_seq"] != float64(3) {
		t.Fatalf("conversation.last_message_seq = %v, want 3", updatedConversation["last_message_seq"])
	}
	if updatedConversation["last_message_summary"] != summary {
		t.Fatalf("conversation.last_message_summary = %v, want %s", updatedConversation["last_message_summary"], summary)
	}
	if createdMessage["seq"] != float64(3) {
		t.Fatalf("message.seq = %v, want 3", createdMessage["seq"])
	}

	var appMember store.ConversationMember
	if err := db.First(
		&appMember,
		"conversation_id = ? AND member_type = ? AND member_id = ?",
		conversation.ID,
		store.ConversationMemberTypeApp,
		app.ID,
	).Error; err != nil {
		t.Fatalf("find app member: %v", err)
	}
	if appMember.LeftAt != nil {
		t.Fatalf("app member left_at = %v, want nil", appMember.LeftAt)
	}
	if appMember.Role != store.ConversationMemberRoleMember {
		t.Fatalf("app member role = %v, want member", appMember.Role)
	}
	if appMember.HistoryVisibleFromSeq != 3 {
		t.Fatalf("app member history_visible_from_seq = %d, want 3", appMember.HistoryVisibleFromSeq)
	}
	if appMember.LastReadSeq != 2 {
		t.Fatalf("app member last_read_seq = %d, want 2", appMember.LastReadSeq)
	}

	members := updatedConversation["members"].([]any)
	var appResponse map[string]any
	for _, rawMember := range members {
		member := rawMember.(map[string]any)
		if member["id"] == app.ID {
			appResponse = member
			break
		}
	}
	if appResponse == nil {
		t.Fatalf("conversation members missing app %s: %#v", app.ID, members)
	}
	if appResponse["type"] != store.ConversationMemberTypeApp {
		t.Fatalf("app member type = %v, want app", appResponse["type"])
	}
	if appResponse["name"] != app.Name {
		t.Fatalf("app member name = %v, want %s", appResponse["name"], app.Name)
	}

	var storedMessage store.Message
	if err := db.First(&storedMessage, "id = ?", createdMessage["id"]).Error; err != nil {
		t.Fatalf("find stored system message: %v", err)
	}
	if storedMessage.Summary != summary {
		t.Fatalf("stored summary = %v, want %s", storedMessage.Summary, summary)
	}
}

func TestRemoveGroupConversationMemberCanRemoveApp(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	now := time.Date(2026, 7, 3, 9, 0, 0, 0, time.UTC)
	alice := insertTestUser(t, db, "alice@example.com", "Alice", store.UserStatusActive, now)
	bob := insertTestUser(t, db, "bob@example.com", "Bob", store.UserStatusActive, now)
	app := insertTestApp(t, db, store.App{
		Name:             "AI 女菩萨",
		Enabled:          true,
		Visibility:       store.AppVisibilityPublic,
		ConnectionSecret: "assistant-secret",
		CreatedAt:        now,
		UpdatedAt:        now,
	})
	conversation := insertTestConversation(t, db, testConversationInput{
		createdByUserID: alice.ID,
		kind:            store.ConversationKindGroup,
		memberIDs:       []string{alice.ID, bob.ID},
		name:            "产品讨论组",
		now:             now,
	})
	if err := db.Create(&store.ConversationMember{
		ConversationID:        conversation.ID,
		MemberType:            store.ConversationMemberTypeApp,
		MemberID:              app.ID,
		Role:                  store.ConversationMemberRoleMember,
		JoinedAt:              now,
		HistoryVisibleFromSeq: 1,
	}).Error; err != nil {
		t.Fatalf("create app member: %v", err)
	}

	resp, body := requestJSON(t, server, http.MethodDelete, "/api/client/conversations/groups/"+conversation.ID+"/members/app/"+app.ID, map[string]any{}, loginAsUser(t, server, alice.Email))

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %#v", resp.StatusCode, body)
	}
	data := requireSuccess(t, body)
	updatedConversation := data["conversation"].(map[string]any)
	summary := "Alice 已将 AI 女菩萨 移出群聊"
	if updatedConversation["member_count"] != float64(2) {
		t.Fatalf("conversation.member_count = %v, want 2", updatedConversation["member_count"])
	}
	if updatedConversation["last_message_summary"] != summary {
		t.Fatalf("conversation.last_message_summary = %v, want %s", updatedConversation["last_message_summary"], summary)
	}

	var appMember store.ConversationMember
	if err := db.First(
		&appMember,
		"conversation_id = ? AND member_type = ? AND member_id = ?",
		conversation.ID,
		store.ConversationMemberTypeApp,
		app.ID,
	).Error; err != nil {
		t.Fatalf("find app member: %v", err)
	}
	if appMember.LeftAt == nil {
		t.Fatal("app member left_at = nil, want set")
	}

	var storedMessage store.Message
	if err := db.First(&storedMessage, "conversation_id = ? AND seq = ?", conversation.ID, int64(1)).Error; err != nil {
		t.Fatalf("find stored system message: %v", err)
	}
	if storedMessage.Summary != summary {
		t.Fatalf("stored summary = %v, want %s", storedMessage.Summary, summary)
	}
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
		t.Fatalf("duplicate status = %d, want 409, body = %#v", duplicateResp.StatusCode, duplicateBody)
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

func TestClientContactsReturnsVisibleAppsUsersAndGroups(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	now := time.Now().UTC()
	alice := insertTestUser(t, db, "alice@example.com", "Alice Zhang", store.UserStatusActive, now)
	bob := insertTestUser(t, db, "bob@example.com", "Bob Li", store.UserStatusActive, now)
	disabledUser := insertTestUser(t, db, "disabled@example.com", "Disabled User", store.UserStatusDisabled, now)

	aliceCreatorID := alice.ID
	bobCreatorID := bob.ID
	publicApp := insertTestApp(t, db, store.App{
		Name:             "Public Agent",
		Avatar:           "/assets/apps/public.webp",
		Description:      "Public app",
		Enabled:          true,
		Visibility:       store.AppVisibilityPublic,
		ConnectionSecret: "public-secret",
		CreatedAt:        now,
		UpdatedAt:        now,
	})
	aliceCreatorApp := insertTestApp(t, db, store.App{
		Name:             "Alice Agent",
		Avatar:           "/assets/apps/alice.webp",
		Description:      "Creator app",
		CreatorUserID:    &aliceCreatorID,
		Enabled:          true,
		Visibility:       store.AppVisibilityCreator,
		ConnectionSecret: "alice-secret",
		CreatedAt:        now,
		UpdatedAt:        now,
	})
	bobCreatorApp := insertTestApp(t, db, store.App{
		Name:             "Bob Agent",
		CreatorUserID:    &bobCreatorID,
		Enabled:          true,
		Visibility:       store.AppVisibilityCreator,
		ConnectionSecret: "bob-secret",
		CreatedAt:        now,
		UpdatedAt:        now,
	})
	disabledApp := insertTestApp(t, db, store.App{
		Name:             "Disabled Agent",
		Enabled:          false,
		Visibility:       store.AppVisibilityPublic,
		ConnectionSecret: "disabled-secret",
		CreatedAt:        now,
		UpdatedAt:        now,
	})
	if err := db.Model(&store.App{}).Where("id = ?", disabledApp.ID).Update("enabled", false).Error; err != nil {
		t.Fatalf("disable test app: %v", err)
	}

	joinedPrivateGroup := insertTestConversation(t, db, testConversationInput{
		createdByUserID: alice.ID,
		kind:            store.ConversationKindGroup,
		memberIDs:       []string{alice.ID},
		name:            "Joined Private",
		now:             now,
	})
	publicGroup := insertTestConversation(t, db, testConversationInput{
		createdByUserID: bob.ID,
		kind:            store.ConversationKindGroup,
		memberIDs:       []string{bob.ID},
		name:            "Open Group",
		now:             now,
	})
	privateUnjoinedGroup := insertTestConversation(t, db, testConversationInput{
		createdByUserID: bob.ID,
		kind:            store.ConversationKindGroup,
		memberIDs:       []string{bob.ID},
		name:            "Hidden Group",
		now:             now,
	})
	if err := db.Model(&store.Conversation{}).Where("id = ?", publicGroup.ID).Update("visibility", "public").Error; err != nil {
		t.Fatalf("set public group visibility: %v", err)
	}

	resp, body := getJSON(t, server, "/api/client/contacts", loginAsUser(t, server, alice.Email))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %#v", resp.StatusCode, body)
	}

	data := requireSuccess(t, body)
	apps := data["apps"].([]any)
	appIDs := map[string]bool{}
	for _, rawApp := range apps {
		app := rawApp.(map[string]any)
		appIDs[app["id"].(string)] = true
		if app["type"] != "app" {
			t.Fatalf("app.type = %v, want app", app["type"])
		}
		if app["online"] != false {
			t.Fatalf("app.online = %v, want false", app["online"])
		}
	}
	if !appIDs[publicApp.ID] {
		t.Fatal("contacts apps did not include public app")
	}
	if !appIDs[aliceCreatorApp.ID] {
		t.Fatal("contacts apps did not include current user's creator app")
	}
	if appIDs[bobCreatorApp.ID] {
		t.Fatal("contacts apps included another user's creator app")
	}
	if appIDs[disabledApp.ID] {
		t.Fatal("contacts apps included disabled app")
	}

	users := data["users"].([]any)
	userIDs := map[string]bool{}
	for _, rawUser := range users {
		user := rawUser.(map[string]any)
		userIDs[user["id"].(string)] = true
		if user["type"] != "user" {
			t.Fatalf("user.type = %v, want user", user["type"])
		}
	}
	if !userIDs[alice.ID] || !userIDs[bob.ID] {
		t.Fatalf("users = %#v, want alice and bob", users)
	}
	if userIDs[disabledUser.ID] {
		t.Fatal("contacts users included disabled user")
	}

	groups := data["groups"].([]any)
	groupsByID := map[string]map[string]any{}
	for _, rawGroup := range groups {
		group := rawGroup.(map[string]any)
		groupsByID[group["id"].(string)] = group
		if group["type"] != "group" {
			t.Fatalf("group.type = %v, want group", group["type"])
		}
	}
	if groupsByID[joinedPrivateGroup.ID] != nil {
		t.Fatal("contacts groups included joined private group")
	}
	if groupsByID[publicGroup.ID] == nil {
		t.Fatal("contacts groups did not include public group")
	}
	if groupsByID[publicGroup.ID]["joined"] != false {
		t.Fatalf("public group joined = %v, want false", groupsByID[publicGroup.ID]["joined"])
	}
	if groupsByID[publicGroup.ID]["visibility"] != "public" {
		t.Fatalf("public group visibility = %v, want public", groupsByID[publicGroup.ID]["visibility"])
	}
	if groupsByID[privateUnjoinedGroup.ID] != nil {
		t.Fatal("contacts groups included private unjoined group")
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
