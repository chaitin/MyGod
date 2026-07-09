package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"app/internal/realtime"
	"app/internal/store"

	"github.com/google/uuid"
)

func TestAppMessageSendConvertsImageURLToWebP(t *testing.T) {
	s3Server, uploadedObjects := newFakeS3Server(t)
	defer s3Server.Close()

	sourceImage := testPNGImage(t, 64, 32)
	sourceServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(sourceImage)
	}))
	defer sourceServer.Close()

	previousHTTPClient := remoteMessageFetchHTTPClient
	previousValidator := validateRemoteMessageFetchURL
	remoteMessageFetchHTTPClient = sourceServer.Client()
	validateRemoteMessageFetchURL = func(context.Context, *url.URL) error {
		return nil
	}
	t.Cleanup(func() {
		remoteMessageFetchHTTPClient = previousHTTPClient
		validateRemoteMessageFetchURL = previousValidator
	})

	server, db := newTemporaryFileTestRouter(t, s3Server.URL, "assets.example.test")
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
	aliceCookie := loginAsUser(t, server, alice.Email)
	aliceConn := dialClientWebSocket(t, server, aliceCookie)
	if ready := readRealtimeEvent(t, aliceConn); ready.Kind != realtime.KindEvent || ready.Event != realtime.EventSystemReady {
		t.Fatalf("ready envelope = %#v, want system.ready", ready)
	}
	appConn := dialAppWebSocket(t, server, app.ID, app.ConnectionSecret)

	createConversationResp, createConversationBody := postJSON(t, server, "/api/client/conversations/apps", map[string]any{
		"app_id": app.ID,
	}, aliceCookie)
	if createConversationResp.StatusCode != http.StatusCreated {
		t.Fatalf("create app conversation status = %d, want 201, body = %#v", createConversationResp.StatusCode, createConversationBody)
	}
	conversation := requireSuccess(t, createConversationBody)["conversation"].(map[string]any)
	conversationID := conversation["id"].(string)

	response := sendAppRequest(t, appConn, realtime.Envelope{
		V:      realtime.ProtocolVersion,
		Kind:   realtime.KindRequest,
		ID:     "app-image-url-request",
		Method: appMethodMessageSend,
		Payload: mustMarshalPayloadForTest(t, map[string]any{
			"target": map[string]any{
				"type":            "app",
				"conversation_id": conversationID,
			},
			"message": map[string]any{
				"type":    "image",
				"content": sourceServer.URL + "/photo.png",
			},
		}),
	})
	payload := requireAppSendMessageResponsePayload(t, response)
	message := payload["message"].(map[string]any)
	body := message["body"].(map[string]any)
	if body["type"] != messageTypeImage {
		t.Fatalf("message.body.type = %v, want image", body["type"])
	}
	fileID := body["file_id"].(string)
	if body["width"] != float64(64) || body["height"] != float64(32) {
		t.Fatalf("message.body dimensions = %vx%v, want 64x32", body["width"], body["height"])
	}

	var storedFile store.TemporaryFile
	if err := db.First(&storedFile, "id = ?", fileID).Error; err != nil {
		t.Fatalf("find temporary file: %v", err)
	}
	uploadedObjects.mu.Lock()
	uploadedBody := uploadedObjects.objects["/mygod-temporary/"+storedFile.ObjectKey]
	uploadedObjects.mu.Unlock()
	if bytes.Equal(uploadedBody, sourceImage) {
		t.Fatalf("uploaded image equals source PNG, want converted WebP")
	}
	width, height, err := parseWebPDimensions(uploadedBody)
	if err != nil {
		t.Fatalf("parse uploaded WebP dimensions: %v", err)
	}
	if width != 64 || height != 32 {
		t.Fatalf("uploaded WebP dimensions = %dx%d, want 64x32", width, height)
	}
	if !webpHasChunk(uploadedBody, "VP8 ") {
		t.Fatalf("uploaded WebP chunks do not include VP8 lossy chunk")
	}
	if webpHasChunk(uploadedBody, "VP8L") {
		t.Fatalf("uploaded WebP chunks include VP8L lossless chunk, want lossy WebP")
	}
	if storedFile.SizeBytes != int64(len(uploadedBody)) {
		t.Fatalf("stored file size = %d, want uploaded size %d", storedFile.SizeBytes, len(uploadedBody))
	}

	pushedMessage := readMessageCreatedEvent(t, aliceConn)
	pushedBody := pushedMessage["body"].(map[string]any)
	if pushedBody["type"] != messageTypeImage ||
		pushedBody["file_id"] != fileID ||
		pushedBody["width"] != float64(64) ||
		pushedBody["height"] != float64(32) {
		t.Fatalf("pushed body = %#v, want image file id", pushedBody)
	}
}

func TestClientCanSendConversationImageMessage(t *testing.T) {
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

	content := testWebPVP8X(1024, 768)
	resp, body := postMultipartImageMessage(
		t,
		server,
		"/api/client/conversations/"+conversation.ID+"/messages/images",
		"client-image-message-1",
		"photo.webp",
		content,
		loginAsUser(t, server, alice.Email),
	)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("send image message status = %d, want 201: %#v", resp.StatusCode, body)
	}
	data := requireSuccess(t, body)
	message := data["message"].(map[string]any)

	messageBody := message["body"].(map[string]any)
	if messageBody["type"] != "image" {
		t.Fatalf("message.body.type = %v, want image", messageBody["type"])
	}
	fileID, ok := messageBody["file_id"].(string)
	if !ok {
		t.Fatalf("message.body.file_id = %#v, want string", messageBody["file_id"])
	}
	if _, err := uuid.Parse(fileID); err != nil {
		t.Fatalf("message.body.file_id = %q, want uuid", fileID)
	}
	if messageBody["width"] != float64(1024) {
		t.Fatalf("message.body.width = %#v, want 1024", messageBody["width"])
	}
	if messageBody["height"] != float64(768) {
		t.Fatalf("message.body.height = %#v, want 768", messageBody["height"])
	}
	if _, ok := messageBody["name"]; ok {
		t.Fatalf("message.body.name = %#v, want omitted", messageBody["name"])
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
		t.Fatalf("uploaded object body = %#v, want image content", uploadedBody)
	}

	var storedMessage store.Message
	if err := db.First(&storedMessage, "id = ?", message["id"]).Error; err != nil {
		t.Fatalf("find stored message: %v", err)
	}
	if storedMessage.Summary != "[图片]" {
		t.Fatalf("stored message summary = %q", storedMessage.Summary)
	}
	var storedConversation store.Conversation
	if err := db.First(&storedConversation, "id = ?", conversation.ID).Error; err != nil {
		t.Fatalf("find stored conversation: %v", err)
	}
	if storedConversation.LastMessageSummary != "[图片]" {
		t.Fatalf("conversation last_message_summary = %q", storedConversation.LastMessageSummary)
	}

	pushedMessage := readMessageCreatedEvent(t, bobConn)
	pushedBody := pushedMessage["body"].(map[string]any)
	if pushedBody["type"] != "image" ||
		pushedBody["file_id"] != fileID ||
		pushedBody["width"] != float64(1024) ||
		pushedBody["height"] != float64(768) {
		t.Fatalf("pushed message body = %#v, want image body", pushedBody)
	}
}

func TestClientCanSendConversationImageMessageToAppConversationNotifiesApp(t *testing.T) {
	s3Server, _ := newFakeS3Server(t)
	defer s3Server.Close()

	server, db := newTemporaryFileTestRouter(t, s3Server.URL, "assets.example.test")
	defer server.Close()

	now := time.Date(2026, 7, 8, 11, 0, 0, 0, time.UTC)
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

	resp, body := postMultipartImageMessage(
		t,
		server,
		"/api/client/conversations/"+conversationID+"/messages/images",
		"client-app-image-message-1",
		"photo.webp",
		testWebPVP8X(320, 240),
		aliceCookie,
	)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("send app image message status = %d, want 201: %#v", resp.StatusCode, body)
	}
	createdMessage := requireSuccess(t, body)["message"].(map[string]any)
	createdBody := createdMessage["body"].(map[string]any)

	event := readRealtimeEvent(t, appConn)
	if event.Kind != realtime.KindEvent || event.Event != realtime.EventMessageCreated {
		t.Fatalf("app event = %#v, want message.created event", event)
	}
	var payload map[string]any
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		t.Fatalf("unmarshal app event payload: %v", err)
	}
	eventConversation := payload["conversation"].(map[string]any)
	if eventConversation["id"] != conversationID {
		t.Fatalf("conversation.id = %v, want %s", eventConversation["id"], conversationID)
	}
	sender := payload["sender"].(map[string]any)
	if sender["id"] != alice.ID || sender["type"] != store.MessageSenderTypeUser {
		t.Fatalf("sender = %#v, want Alice user sender", sender)
	}
	message := payload["message"].(map[string]any)
	if message["id"] != createdMessage["id"] {
		t.Fatalf("message.id = %v, want %v", message["id"], createdMessage["id"])
	}
	messageBody := message["body"].(map[string]any)
	if messageBody["type"] != messageTypeImage ||
		messageBody["file_id"] != createdBody["file_id"] ||
		messageBody["width"] != float64(320) ||
		messageBody["height"] != float64(240) {
		t.Fatalf("message.body = %#v, want image body with created file id", messageBody)
	}
	if message["summary"] != imageMessageSummary() {
		t.Fatalf("message.summary = %v, want image summary", message["summary"])
	}
}

func TestClientCanSendConversationImageMessageWithReplyReference(t *testing.T) {
	s3Server, _ := newFakeS3Server(t)
	defer s3Server.Close()

	server, db := newTemporaryFileTestRouter(t, s3Server.URL, "assets.example.test")
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
	quotedMessage := insertTestMessage(t, db, conversation.ID, bob.ID, 1, "请看这张图", now)
	if err := db.Model(&store.Conversation{}).Where("id = ?", conversation.ID).Update("last_message_seq", int64(1)).Error; err != nil {
		t.Fatalf("update conversation seq: %v", err)
	}

	resp, body := postMultipartImageMessage(
		t,
		server,
		"/api/client/conversations/"+conversation.ID+"/messages/images",
		"client-image-message-reply",
		"photo.webp",
		testWebPVP8X(1024, 768),
		loginAsUser(t, server, alice.Email),
		quotedMessage.ID,
	)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("send image message status = %d, want 201: %#v", resp.StatusCode, body)
	}
	message := requireSuccess(t, body)["message"].(map[string]any)
	if message["reply_to_message_id"] != quotedMessage.ID {
		t.Fatalf("message.reply_to_message_id = %v, want %s", message["reply_to_message_id"], quotedMessage.ID)
	}
	replyTo := message["reply_to"].(map[string]any)
	if replyTo["summary"] != "请看这张图" {
		t.Fatalf("reply_to.summary = %v, want quoted summary", replyTo["summary"])
	}

	var storedMessage store.Message
	if err := db.First(&storedMessage, "id = ?", message["id"]).Error; err != nil {
		t.Fatalf("find stored message: %v", err)
	}
	if storedMessage.ReplyToMessageID == nil || *storedMessage.ReplyToMessageID != quotedMessage.ID {
		t.Fatalf("stored reply_to_message_id = %v, want %s", storedMessage.ReplyToMessageID, quotedMessage.ID)
	}
}

func TestCreateConversationImageMessageRejectsInvalidReplyReferenceBeforeUpload(t *testing.T) {
	s3Server, uploadedObjects := newFakeS3Server(t)
	defer s3Server.Close()

	server, db := newTemporaryFileTestRouter(t, s3Server.URL, "assets.example.test")
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

	resp, body := postMultipartImageMessage(
		t,
		server,
		"/api/client/conversations/"+conversation.ID+"/messages/images",
		"client-image-message-invalid-reply",
		"photo.webp",
		testWebPVP8X(1024, 768),
		loginAsUser(t, server, alice.Email),
		uuid.NewString(),
	)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("send image message status = %d, want 400: %#v", resp.StatusCode, body)
	}
	requireError(t, body, "invalid_request")

	var temporaryFileCount int64
	if err := db.Model(&store.TemporaryFile{}).Count(&temporaryFileCount).Error; err != nil {
		t.Fatalf("count temporary files: %v", err)
	}
	if temporaryFileCount != 0 {
		t.Fatalf("temporary file count = %d, want 0", temporaryFileCount)
	}
	uploadedObjects.mu.Lock()
	uploadedCount := len(uploadedObjects.objects)
	uploadedObjects.mu.Unlock()
	if uploadedCount != 0 {
		t.Fatalf("uploaded object count = %d, want 0", uploadedCount)
	}
}

func TestCreateConversationImageMessageIsIdempotent(t *testing.T) {
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

	firstResp, firstBody := postMultipartImageMessage(
		t,
		server,
		"/api/client/conversations/"+conversation.ID+"/messages/images",
		"client-image-message-1",
		"first.webp",
		testWebPVP8X(1024, 768),
		cookie,
	)
	if firstResp.StatusCode != http.StatusCreated {
		t.Fatalf("first send status = %d, want 201: %#v", firstResp.StatusCode, firstBody)
	}
	firstMessage := requireSuccess(t, firstBody)["message"].(map[string]any)
	firstFileID := firstMessage["body"].(map[string]any)["file_id"].(string)

	secondResp, secondBody := postMultipartImageMessage(
		t,
		server,
		"/api/client/conversations/"+conversation.ID+"/messages/images",
		"client-image-message-1",
		"second.webp",
		testWebPVP8X(800, 600),
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

func TestCreateConversationImageMessageRejectsNonWebP(t *testing.T) {
	s3Server, _ := newFakeS3Server(t)
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

	resp, body := postMultipartImageMessage(
		t,
		server,
		"/api/client/conversations/"+conversation.ID+"/messages/images",
		"client-image-message-1",
		"image.txt",
		[]byte("not a webp"),
		loginAsUser(t, server, alice.Email),
	)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("send image status = %d, want 400: %#v", resp.StatusCode, body)
	}
	requireError(t, body, "invalid_request")
}

func TestCreateConversationImageMessageRejectsLargeDimensions(t *testing.T) {
	s3Server, _ := newFakeS3Server(t)
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

	resp, body := postMultipartImageMessage(
		t,
		server,
		"/api/client/conversations/"+conversation.ID+"/messages/images",
		"client-image-message-1",
		"image.webp",
		testWebPVP8X(1921, 1080),
		loginAsUser(t, server, alice.Email),
	)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("send image status = %d, want 400: %#v", resp.StatusCode, body)
	}
	requireError(t, body, "invalid_request")
}

func TestCreateConversationImageMessageAcceptsImageUpToFiveMiB(t *testing.T) {
	s3Server, _ := newFakeS3Server(t)
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

	content := testSizedWebPVP8X(1024, 768, 3*1024*1024)
	resp, body := postMultipartImageMessage(
		t,
		server,
		"/api/client/conversations/"+conversation.ID+"/messages/images",
		"client-image-message-1",
		"image.webp",
		content,
		loginAsUser(t, server, alice.Email),
	)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("send image status = %d, want 201: %#v", resp.StatusCode, body)
	}
	message := requireSuccess(t, body)["message"].(map[string]any)
	messageBody := message["body"].(map[string]any)
	if messageBody["type"] != messageTypeImage {
		t.Fatalf("message.body.type = %v, want image", messageBody["type"])
	}
}

func TestCreateConversationImageMessageRejectsLargeFile(t *testing.T) {
	s3Server, _ := newFakeS3Server(t)
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

	content := testSizedWebPVP8X(1024, 768, 5*1024*1024+1)
	resp, body := postMultipartImageMessage(
		t,
		server,
		"/api/client/conversations/"+conversation.ID+"/messages/images",
		"client-image-message-1",
		"image.webp",
		content,
		loginAsUser(t, server, alice.Email),
	)
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("send image status = %d, want 413: %#v", resp.StatusCode, body)
	}
	requireError(t, body, "request_too_large")
}

func postMultipartImageMessage(t *testing.T, server *httptest.Server, path string, clientMessageID string, filename string, content []byte, cookiesAndReplyTo ...any) (*http.Response, map[string]any) {
	t.Helper()

	var cookies []*http.Cookie
	var replyToMessageID string
	for _, value := range cookiesAndReplyTo {
		switch typedValue := value.(type) {
		case *http.Cookie:
			cookies = append(cookies, typedValue)
		case string:
			replyToMessageID = typedValue
		default:
			t.Fatalf("unsupported multipart image message option %T", value)
		}
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("client_message_id", clientMessageID); err != nil {
		t.Fatalf("write client_message_id: %v", err)
	}
	if replyToMessageID != "" {
		if err := writer.WriteField("reply_to_message_id", replyToMessageID); err != nil {
			t.Fatalf("write reply_to_message_id: %v", err)
		}
	}
	part, err := writer.CreateFormFile("image", filename)
	if err != nil {
		t.Fatalf("create multipart image: %v", err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatalf("write multipart image: %v", err)
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

func testSizedWebPVP8X(width int, height int, size int) []byte {
	content := make([]byte, size)
	copy(content, testWebPVP8X(width, height))

	return content
}

func webpHasChunk(content []byte, chunkType string) bool {
	if len(content) < 12 || string(content[0:4]) != "RIFF" || string(content[8:12]) != "WEBP" {
		return false
	}

	for offset := 12; offset+8 <= len(content); {
		currentChunkType := string(content[offset : offset+4])
		chunkSize := int(uint32(content[offset+4]) | uint32(content[offset+5])<<8 | uint32(content[offset+6])<<16 | uint32(content[offset+7])<<24)
		payloadEnd := offset + 8 + chunkSize
		if chunkSize < 0 || payloadEnd > len(content) {
			return false
		}
		if currentChunkType == chunkType {
			return true
		}
		offset = payloadEnd
		if chunkSize%2 == 1 {
			offset++
		}
	}

	return false
}

func testPNGImage(t *testing.T, width int, height int) []byte {
	t.Helper()

	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{
				R: uint8((x * 255) / width),
				G: uint8((y * 255) / height),
				B: 128,
				A: 255,
			})
		}
	}
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}

	return buffer.Bytes()
}
