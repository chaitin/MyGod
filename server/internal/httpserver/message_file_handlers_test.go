package httpserver

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"app/internal/realtime"
	"app/internal/store"

	"github.com/google/uuid"
)

func TestClientCanSendConversationFileMessage(t *testing.T) {
	s3Server, uploadedObjects := newFakeS3Server(t)
	defer s3Server.Close()

	server, db := newTemporaryFileTestRouter(t, s3Server.URL, "assets.example.test")
	defer server.Close()

	now := time.Date(2026, 7, 7, 9, 0, 0, 0, time.UTC)
	alice := insertTestUser(t, db, "alice@example.com", "Alice", store.UserStatusActive, now)
	bob := insertTestUser(t, db, "bob@example.com", "Bob", store.UserStatusActive, now)
	conversation := insertTestConversation(t, db, testConversationInput{
		createdByUserID: alice.ID,
		kind:            store.ConversationKindDirect,
		memberIDs:       []string{alice.ID, bob.ID},
		now:             now,
	})

	bobCookie := loginAsUser(t, server, bob.Email)
	bobConn := dialClientWebSocket(t, server, bobCookie)
	ready := readRealtimeEvent(t, bobConn)
	if ready.Kind != realtime.KindEvent || ready.Event != realtime.EventSystemReady {
		t.Fatalf("ready envelope = %#v, want system.ready event", ready)
	}

	content := []byte("type MessageRenderer = () => JSX.Element\n")
	resp, body := postMultipartFileMessage(
		t,
		server,
		"/api/client/conversations/"+conversation.ID+"/messages/files",
		"client-file-message-1",
		"message-renderer.tsx",
		content,
		loginAsUser(t, server, alice.Email),
	)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("send file message status = %d, want 201: %#v", resp.StatusCode, body)
	}
	data := requireSuccess(t, body)
	message := data["message"].(map[string]any)

	if message["conversation_id"] != conversation.ID {
		t.Fatalf("message.conversation_id = %v, want %s", message["conversation_id"], conversation.ID)
	}
	if message["client_message_id"] != "client-file-message-1" {
		t.Fatalf("message.client_message_id = %v", message["client_message_id"])
	}
	if message["seq"] != float64(1) {
		t.Fatalf("message.seq = %v, want 1", message["seq"])
	}
	sender := message["sender"].(map[string]any)
	if sender["type"] != store.MessageSenderTypeUser || sender["id"] != alice.ID {
		t.Fatalf("message.sender = %#v, want Alice user sender", sender)
	}
	messageBody := message["body"].(map[string]any)
	if messageBody["type"] != "file" {
		t.Fatalf("message.body.type = %v, want file", messageBody["type"])
	}
	if messageBody["name"] != "message-renderer.tsx" {
		t.Fatalf("message.body.name = %v", messageBody["name"])
	}
	if messageBody["size_bytes"] != float64(len(content)) {
		t.Fatalf("message.body.size_bytes = %v, want %d", messageBody["size_bytes"], len(content))
	}
	fileID, ok := messageBody["file_id"].(string)
	if !ok {
		t.Fatalf("message.body.file_id = %#v, want string", messageBody["file_id"])
	}
	if _, err := uuid.Parse(fileID); err != nil {
		t.Fatalf("message.body.file_id = %q, want uuid", fileID)
	}
	if _, ok := messageBody["content_type"]; ok {
		t.Fatalf("message.body.content_type = %#v, want omitted", messageBody["content_type"])
	}

	var storedFile store.TemporaryFile
	if err := db.First(&storedFile, "id = ?", fileID).Error; err != nil {
		t.Fatalf("find temporary file: %v", err)
	}
	if storedFile.SizeBytes != int64(len(content)) {
		t.Fatalf("stored temporary file size = %d, want %d", storedFile.SizeBytes, len(content))
	}
	if !strings.HasPrefix(storedFile.ObjectKey, "temporary-files/") {
		t.Fatalf("stored temporary object key = %q, want temporary-files prefix", storedFile.ObjectKey)
	}
	uploadedObjects.mu.Lock()
	uploadedBody := uploadedObjects.objects["/mygod-temporary/"+storedFile.ObjectKey]
	uploadedObjects.mu.Unlock()
	if !bytes.Equal(uploadedBody, content) {
		t.Fatalf("uploaded object body = %q, want %q", string(uploadedBody), string(content))
	}

	var storedMessage store.Message
	if err := db.First(&storedMessage, "id = ?", message["id"]).Error; err != nil {
		t.Fatalf("find stored message: %v", err)
	}
	if storedMessage.Summary != "[文件] message-renderer.tsx" {
		t.Fatalf("stored message summary = %q", storedMessage.Summary)
	}
	var storedConversation store.Conversation
	if err := db.First(&storedConversation, "id = ?", conversation.ID).Error; err != nil {
		t.Fatalf("find stored conversation: %v", err)
	}
	if storedConversation.LastMessageSummary != "[文件] message-renderer.tsx" {
		t.Fatalf("conversation last_message_summary = %q", storedConversation.LastMessageSummary)
	}
	if got := getTestConversationMemberLastReadSeq(t, db, conversation.ID, alice.ID); got != 1 {
		t.Fatalf("alice last_read_seq = %d, want 1", got)
	}
	if got := getTestConversationMemberLastReadSeq(t, db, conversation.ID, bob.ID); got != 0 {
		t.Fatalf("bob last_read_seq = %d, want 0", got)
	}

	pushedMessage := readMessageCreatedEvent(t, bobConn)
	pushedBody := pushedMessage["body"].(map[string]any)
	if pushedBody["type"] != "file" || pushedBody["file_id"] != fileID || pushedBody["name"] != "message-renderer.tsx" {
		t.Fatalf("pushed message body = %#v, want file body", pushedBody)
	}
}

func TestCreateConversationFileMessageIsIdempotent(t *testing.T) {
	s3Server, uploadedObjects := newFakeS3Server(t)
	defer s3Server.Close()

	server, db := newTemporaryFileTestRouter(t, s3Server.URL, "assets.example.test")
	defer server.Close()

	now := time.Date(2026, 7, 7, 9, 0, 0, 0, time.UTC)
	alice := insertTestUser(t, db, "alice@example.com", "Alice", store.UserStatusActive, now)
	bob := insertTestUser(t, db, "bob@example.com", "Bob", store.UserStatusActive, now)
	conversation := insertTestConversation(t, db, testConversationInput{
		createdByUserID: alice.ID,
		kind:            store.ConversationKindDirect,
		memberIDs:       []string{alice.ID, bob.ID},
		now:             now,
	})
	cookie := loginAsUser(t, server, alice.Email)

	firstResp, firstBody := postMultipartFileMessage(
		t,
		server,
		"/api/client/conversations/"+conversation.ID+"/messages/files",
		"client-file-message-1",
		"first.txt",
		[]byte("first"),
		cookie,
	)
	if firstResp.StatusCode != http.StatusCreated {
		t.Fatalf("first send status = %d, want 201: %#v", firstResp.StatusCode, firstBody)
	}
	firstMessage := requireSuccess(t, firstBody)["message"].(map[string]any)
	firstFileID := firstMessage["body"].(map[string]any)["file_id"].(string)

	secondResp, secondBody := postMultipartFileMessage(
		t,
		server,
		"/api/client/conversations/"+conversation.ID+"/messages/files",
		"client-file-message-1",
		"second.txt",
		[]byte("second"),
		cookie,
	)
	if secondResp.StatusCode != http.StatusOK {
		t.Fatalf("second send status = %d, want 200: %#v", secondResp.StatusCode, secondBody)
	}
	secondMessage := requireSuccess(t, secondBody)["message"].(map[string]any)
	if secondMessage["id"] != firstMessage["id"] {
		t.Fatalf("second message id = %v, want original %v", secondMessage["id"], firstMessage["id"])
	}
	secondFileID := secondMessage["body"].(map[string]any)["file_id"].(string)
	if secondFileID != firstFileID {
		t.Fatalf("second file id = %s, want original %s", secondFileID, firstFileID)
	}

	var temporaryFileCount int64
	if err := db.Model(&store.TemporaryFile{}).Count(&temporaryFileCount).Error; err != nil {
		t.Fatalf("count temporary files: %v", err)
	}
	if temporaryFileCount != 1 {
		t.Fatalf("temporary file count = %d, want 1", temporaryFileCount)
	}
	uploadedObjects.mu.Lock()
	uploadedCount := len(uploadedObjects.objects)
	uploadedObjects.mu.Unlock()
	if uploadedCount != 1 {
		t.Fatalf("uploaded object count = %d, want 1", uploadedCount)
	}
}

func postMultipartFileMessage(t *testing.T, server *httptest.Server, path string, clientMessageID string, filename string, content []byte, cookies ...*http.Cookie) (*http.Response, map[string]any) {
	t.Helper()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("client_message_id", clientMessageID); err != nil {
		t.Fatalf("write client_message_id: %v", err)
	}
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		t.Fatalf("create multipart file: %v", err)
	}
	if _, err := part.Write(content); err != nil {
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
