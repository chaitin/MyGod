package httpserver

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"app/internal/config"
	"app/internal/realtime"
	"app/internal/store"

	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func TestClientCanUploadTemporaryFile(t *testing.T) {
	s3Server, uploadedObjects := newFakeS3Server(t)
	defer s3Server.Close()

	server, db := newTemporaryFileTestRouter(t, s3Server.URL, "assets.example.test")
	defer server.Close()

	user := insertTestUser(t, db, "alice@example.com", "Alice", store.UserStatusActive, time.Now().UTC())
	userCookie := loginAsUser(t, server, user.Email)

	resp, body := postMultipartFile(t, server, "/api/client/temporary-files", "file", "hello.txt", "hello temporary file", userCookie)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("upload status = %d, want 201: %#v", resp.StatusCode, body)
	}
	requireSuccess(t, body)

	file := body["data"].(map[string]any)["file"].(map[string]any)
	fileID := file["id"].(string)
	if _, err := uuid.Parse(fileID); err != nil {
		t.Fatalf("file.id = %q, want uuid", fileID)
	}
	if file["size_bytes"] != float64(len("hello temporary file")) {
		t.Fatalf("file.size_bytes = %#v", file["size_bytes"])
	}

	var stored store.TemporaryFile
	if err := db.First(&stored, "id = ?", fileID).Error; err != nil {
		t.Fatalf("find temporary file: %v", err)
	}
	if !strings.HasPrefix(stored.ObjectKey, "temporary-files/") {
		t.Fatalf("object_key = %q, want temporary-files prefix", stored.ObjectKey)
	}

	uploadedObjects.mu.Lock()
	uploadedBody := uploadedObjects.objects["/mygod-temporary/"+stored.ObjectKey]
	uploadedObjects.mu.Unlock()
	if string(uploadedBody) != "hello temporary file" {
		t.Fatalf("uploaded object body = %q", string(uploadedBody))
	}
}

func TestClientCanReadTemporaryFileURLs(t *testing.T) {
	s3Server, _ := newFakeS3Server(t)
	defer s3Server.Close()

	server, db := newTemporaryFileTestRouter(t, s3Server.URL, "assets.example.test")
	defer server.Close()

	user := insertTestUser(t, db, "alice@example.com", "Alice", store.UserStatusActive, time.Now().UTC())
	userCookie := loginAsUser(t, server, user.Email)
	temporaryFile := store.TemporaryFile{
		ID:        uuid.NewString(),
		ObjectKey: "temporary-files/2026/07/07/example",
		SizeBytes: 123,
		CreatedAt: time.Now().UTC(),
	}
	if err := db.Create(&temporaryFile).Error; err != nil {
		t.Fatalf("create temporary file: %v", err)
	}

	resp, body := postJSON(t, server, "/api/client/temporary-files/read-urls", map[string]any{
		"file_ids": []string{temporaryFile.ID},
	}, userCookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("read urls status = %d, want 200: %#v", resp.StatusCode, body)
	}
	requireSuccess(t, body)

	urls := body["data"].(map[string]any)["urls"].([]any)
	if len(urls) != 1 {
		t.Fatalf("urls length = %d, want 1", len(urls))
	}
	item := urls[0].(map[string]any)
	if item["file_id"] != temporaryFile.ID {
		t.Fatalf("file_id = %v, want %s", item["file_id"], temporaryFile.ID)
	}
	readURL, err := url.Parse(item["url"].(string))
	if err != nil {
		t.Fatalf("parse read url: %v", err)
	}
	if readURL.Scheme != "https" || readURL.Host != "assets.example.test" {
		t.Fatalf("read URL = %s, want https assets host", readURL.String())
	}
	if readURL.Path != "/mygod-temporary/"+temporaryFile.ObjectKey {
		t.Fatalf("read URL path = %q", readURL.Path)
	}
	if readURL.Query().Get("X-Amz-Algorithm") == "" {
		t.Fatalf("read URL query missing X-Amz-Algorithm: %s", readURL.RawQuery)
	}
}

func TestAppWebSocketTemporaryFilesReadURLsReturnsConversationFileURLs(t *testing.T) {
	s3Server, _ := newFakeS3Server(t)
	defer s3Server.Close()

	server, db := newTemporaryFileTestRouter(t, s3Server.URL, "assets.example.test")
	defer server.Close()

	now := time.Date(2026, 7, 8, 11, 30, 0, 0, time.UTC)
	alice := insertTestUser(t, db, "alice@example.com", "Alice", store.UserStatusActive, now)
	app := insertTestApp(t, db, store.App{
		Name:             "Echo App",
		Enabled:          true,
		Visibility:       store.AppVisibilityPublic,
		ConnectionSecret: "echo-app-secret",
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
	conversation := requireSuccess(t, createConversationBody)["conversation"].(map[string]any)
	conversationID := conversation["id"].(string)
	temporaryFile := store.TemporaryFile{
		ID:        uuid.NewString(),
		ObjectKey: "temporary-files/2026/07/08/app-file",
		SizeBytes: 123,
		CreatedAt: now,
	}
	if err := db.Create(&temporaryFile).Error; err != nil {
		t.Fatalf("create temporary file: %v", err)
	}
	body, err := json.Marshal(fileMessageBody{
		Type:      messageTypeFile,
		FileID:    temporaryFile.ID,
		Name:      "report.txt",
		SizeBytes: temporaryFile.SizeBytes,
	})
	if err != nil {
		t.Fatalf("marshal file body: %v", err)
	}
	senderID := alice.ID
	if err := db.Create(&store.Message{
		ID:             uuid.NewString(),
		ConversationID: conversationID,
		Seq:            1,
		SenderType:     store.MessageSenderTypeUser,
		SenderID:       &senderID,
		Body:           body,
		Summary:        "[文件] report.txt",
		CreatedAt:      now,
		UpdatedAt:      now,
	}).Error; err != nil {
		t.Fatalf("create file message: %v", err)
	}

	response := sendAppRequest(t, appConn, realtime.Envelope{
		V:      realtime.ProtocolVersion,
		Kind:   realtime.KindRequest,
		ID:     "app-read-file-url",
		Method: "temporary_files.read_urls",
		Payload: mustMarshalPayloadForTest(t, map[string]any{
			"conversation_id": conversationID,
			"file_ids":        []string{temporaryFile.ID},
		}),
	})
	var payload map[string]any
	if err := json.Unmarshal(response.Payload, &payload); err != nil {
		t.Fatalf("unmarshal response payload: %v", err)
	}
	urls := payload["urls"].([]any)
	if len(urls) != 1 {
		t.Fatalf("url count = %d, want 1", len(urls))
	}
	item := urls[0].(map[string]any)
	if item["file_id"] != temporaryFile.ID {
		t.Fatalf("file_id = %v, want %s", item["file_id"], temporaryFile.ID)
	}
	readURL, err := url.Parse(item["url"].(string))
	if err != nil {
		t.Fatalf("parse read URL: %v", err)
	}
	if readURL.Scheme != "https" || readURL.Host != "assets.example.test" {
		t.Fatalf("read URL = %s, want https assets host", readURL.String())
	}
	if readURL.Path != "/mygod-temporary/"+temporaryFile.ObjectKey {
		t.Fatalf("read URL path = %q, want temporary file path", readURL.Path)
	}
	if readURL.Query().Get("X-Amz-Algorithm") == "" {
		t.Fatalf("read URL query missing X-Amz-Algorithm: %s", readURL.RawQuery)
	}
	if item["expires_at"] == "" {
		t.Fatalf("expires_at = %#v, want non-empty", item["expires_at"])
	}
}

func TestAppWebSocketTemporaryFilesReadURLsReturnsConversationImageURLs(t *testing.T) {
	s3Server, _ := newFakeS3Server(t)
	defer s3Server.Close()

	server, db := newTemporaryFileTestRouter(t, s3Server.URL, "assets.example.test")
	defer server.Close()

	now := time.Date(2026, 7, 8, 11, 30, 0, 0, time.UTC)
	alice := insertTestUser(t, db, "alice@example.com", "Alice", store.UserStatusActive, now)
	app := insertTestApp(t, db, store.App{
		Name:             "Echo App",
		Enabled:          true,
		Visibility:       store.AppVisibilityPublic,
		ConnectionSecret: "echo-app-secret",
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
	conversation := requireSuccess(t, createConversationBody)["conversation"].(map[string]any)
	conversationID := conversation["id"].(string)
	temporaryFile := store.TemporaryFile{
		ID:        uuid.NewString(),
		ObjectKey: "temporary-files/2026/07/08/app-image",
		SizeBytes: 456,
		CreatedAt: now,
	}
	if err := db.Create(&temporaryFile).Error; err != nil {
		t.Fatalf("create temporary file: %v", err)
	}
	body, err := json.Marshal(imageMessageBody{
		Type:   messageTypeImage,
		FileID: temporaryFile.ID,
	})
	if err != nil {
		t.Fatalf("marshal image body: %v", err)
	}
	senderID := alice.ID
	if err := db.Create(&store.Message{
		ID:             uuid.NewString(),
		ConversationID: conversationID,
		Seq:            1,
		SenderType:     store.MessageSenderTypeUser,
		SenderID:       &senderID,
		Body:           body,
		Summary:        "[图片]",
		CreatedAt:      now,
		UpdatedAt:      now,
	}).Error; err != nil {
		t.Fatalf("create image message: %v", err)
	}

	response := sendAppRequest(t, appConn, realtime.Envelope{
		V:      realtime.ProtocolVersion,
		Kind:   realtime.KindRequest,
		ID:     "app-read-image-url",
		Method: "temporary_files.read_urls",
		Payload: mustMarshalPayloadForTest(t, map[string]any{
			"conversation_id": conversationID,
			"file_ids":        []string{temporaryFile.ID},
		}),
	})
	var payload map[string]any
	if err := json.Unmarshal(response.Payload, &payload); err != nil {
		t.Fatalf("unmarshal response payload: %v", err)
	}
	urls := payload["urls"].([]any)
	if len(urls) != 1 {
		t.Fatalf("url count = %d, want 1", len(urls))
	}
	item := urls[0].(map[string]any)
	if item["file_id"] != temporaryFile.ID {
		t.Fatalf("file_id = %v, want %s", item["file_id"], temporaryFile.ID)
	}
	readURL, err := url.Parse(item["url"].(string))
	if err != nil {
		t.Fatalf("parse read URL: %v", err)
	}
	if readURL.Path != "/mygod-temporary/"+temporaryFile.ObjectKey {
		t.Fatalf("read URL path = %q, want temporary image path", readURL.Path)
	}
}

func TestAppWebSocketTemporaryFilesReadURLsRejectsUnreferencedFile(t *testing.T) {
	s3Server, _ := newFakeS3Server(t)
	defer s3Server.Close()

	server, db := newTemporaryFileTestRouter(t, s3Server.URL, "assets.example.test")
	defer server.Close()

	now := time.Date(2026, 7, 8, 11, 30, 0, 0, time.UTC)
	alice := insertTestUser(t, db, "alice@example.com", "Alice", store.UserStatusActive, now)
	app := insertTestApp(t, db, store.App{
		Name:             "Echo App",
		Enabled:          true,
		Visibility:       store.AppVisibilityPublic,
		ConnectionSecret: "echo-app-secret",
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
	conversation := requireSuccess(t, createConversationBody)["conversation"].(map[string]any)
	temporaryFile := store.TemporaryFile{
		ID:        uuid.NewString(),
		ObjectKey: "temporary-files/2026/07/08/unreferenced",
		SizeBytes: 123,
		CreatedAt: now,
	}
	if err := db.Create(&temporaryFile).Error; err != nil {
		t.Fatalf("create temporary file: %v", err)
	}

	request := realtime.Envelope{
		V:      realtime.ProtocolVersion,
		Kind:   realtime.KindRequest,
		ID:     "app-read-unreferenced-file-url",
		Method: "temporary_files.read_urls",
		Payload: mustMarshalPayloadForTest(t, map[string]any{
			"conversation_id": conversation["id"].(string),
			"file_ids":        []string{temporaryFile.ID},
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

func TestReadTemporaryFileURLsRejectsExpiredFiles(t *testing.T) {
	s3Server, _ := newFakeS3Server(t)
	defer s3Server.Close()

	server, db := newTemporaryFileTestRouter(t, s3Server.URL, "assets.example.test")
	defer server.Close()

	user := insertTestUser(t, db, "alice@example.com", "Alice", store.UserStatusActive, time.Now().UTC())
	userCookie := loginAsUser(t, server, user.Email)
	temporaryFile := store.TemporaryFile{
		ID:        uuid.NewString(),
		ObjectKey: "temporary-files/2026/01/01/expired",
		SizeBytes: 123,
		CreatedAt: time.Now().UTC().AddDate(0, 0, -181),
	}
	if err := db.Create(&temporaryFile).Error; err != nil {
		t.Fatalf("create temporary file: %v", err)
	}

	resp, body := postJSON(t, server, "/api/client/temporary-files/read-urls", map[string]any{
		"file_ids": []string{temporaryFile.ID},
	}, userCookie)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("read urls status = %d, want 404: %#v", resp.StatusCode, body)
	}
	requireError(t, body, "not_found")
}

type fakeS3Uploads struct {
	mu      sync.Mutex
	objects map[string][]byte
}

func newFakeS3Server(t *testing.T) (*httptest.Server, *fakeS3Uploads) {
	t.Helper()

	uploads := &fakeS3Uploads{
		objects: map[string][]byte{},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			http.NotFound(w, r)
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "read body failed", http.StatusInternalServerError)
			return
		}
		uploads.mu.Lock()
		uploads.objects[r.URL.Path] = body
		uploads.mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))

	return server, uploads
}

func newTemporaryFileTestRouter(t *testing.T, s3Endpoint string, assetsHostname string) (*httptest.Server, *gorm.DB) {
	t.Helper()

	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite db: %v", err)
	}
	if err := migrateTestSchema(db); err != nil {
		t.Fatalf("migrate test schema: %v", err)
	}

	router := NewRouter(db, config.Config{
		Server:   config.ServerConfig{Addr: ":20080"},
		Database: config.DatabaseConfig{DSN: "sqlite-test"},
		Admin:    config.AdminConfig{Password: "admin-secret"},
		Storage: config.StorageConfig{
			Provider:        "s3",
			Endpoint:        s3Endpoint,
			Region:          "us-east-1",
			AccessKeyID:     "mygod",
			SecretAccessKey: "storage-secret",
			ForcePathStyle:  true,
			AssetsHostname:  assetsHostname,
			Buckets: config.StorageBucketsConfig{
				Public:    "mygod-public",
				Private:   "mygod-private",
				Temporary: "mygod-temporary",
			},
		},
	})

	return httptest.NewServer(router), db
}

func postMultipartFile(t *testing.T, server *httptest.Server, path string, fieldName string, filename string, content string, cookies ...*http.Cookie) (*http.Response, map[string]any) {
	t.Helper()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile(fieldName, filename)
	if err != nil {
		t.Fatalf("create multipart file: %v", err)
	}
	if _, err := part.Write([]byte(content)); err != nil {
		t.Fatalf("write multipart file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	req, err := http.NewRequest(http.MethodPost, server.URL+path, &body)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}

	resp, err := server.Client().Do(req)
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
