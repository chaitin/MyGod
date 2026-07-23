package client

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"app/internal/application/account"
	fileapp "app/internal/application/file"
	messageapp "app/internal/application/message"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

func TestMessageAPIListsMessages(t *testing.T) {
	conversationID := uuid.NewString()
	stub := &messageServiceStub{listResult: messageapp.ListResult{
		Messages: []messageapp.Message{{ID: "message-id", ConversationID: conversationID, Sender: messageapp.Identity{ID: "user-id", Type: "user"}, Seq: 3}},
		Page:     messageapp.Page{Limit: 20, OldestSeq: 3, NewestSeq: 3},
	}}
	api := NewMessageAPI(stub, nil)
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/conversations/"+conversationID+"/messages?before_seq=5&limit=100", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPath("/conversations/:conversation_id/messages")
	c.SetParamNames("conversation_id")
	c.SetParamValues(conversationID)
	c.Set(currentAccountKey, account.Account{ID: "account-id"})

	if err := api.list(c); err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if rec.Code != http.StatusOK || stub.listCommand.AccountID != "account-id" || stub.listCommand.Limit != 20 || stub.listCommand.BeforeSeq == nil || *stub.listCommand.BeforeSeq != 5 {
		t.Fatalf("status = %d, command = %#v", rec.Code, stub.listCommand)
	}
}

func TestMessageAPIListsMessagesEncodesEmptyChoiceOptionIDsAsArray(t *testing.T) {
	conversationID := uuid.NewString()
	stub := &messageServiceStub{listResult: messageapp.ListResult{
		Messages: []messageapp.Message{{
			ID: "message-id", ConversationID: conversationID,
			Body: json.RawMessage(`{"type":"choice","content_type":"text","content":"请选择","selection":"single","options":[{"id":"yes","label":"是"},{"id":"no","label":"否"}]}`),
			Choice: &messageapp.ChoiceState{
				Options: []messageapp.ChoiceOptionState{{ID: "yes"}, {ID: "no"}},
			},
			Sender: messageapp.Identity{ID: "app-id", Type: "app"}, Seq: 1,
		}},
		Page: messageapp.Page{Limit: 20, OldestSeq: 1, NewestSeq: 1},
	}}
	api := NewMessageAPI(stub, nil)
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/conversations/"+conversationID+"/messages", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPath("/conversations/:conversation_id/messages")
	c.SetParamNames("conversation_id")
	c.SetParamValues(conversationID)
	c.Set(currentAccountKey, account.Account{ID: "account-id"})

	if err := api.list(c); err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"my_option_ids":[]`)) {
		t.Fatalf("expected empty choice option ids to be encoded as an array, body = %s", rec.Body.String())
	}
}

func TestMessageAPICreatesMessage(t *testing.T) {
	conversationID := uuid.NewString()
	createdAt := time.Date(2026, 7, 15, 14, 0, 0, 0, time.UTC)
	stub := &messageServiceStub{createResult: messageapp.CreateResult{Created: true, Message: messageapp.Message{
		ID: "message-id", ConversationID: conversationID, CreatedAt: createdAt,
		Body:   json.RawMessage(`{"type":"text","content":"hello"}`),
		Sender: messageapp.Identity{ID: "account-id", Type: "user"}, Seq: 1,
	}}}
	api := NewMessageAPI(stub, nil)
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/conversations/"+conversationID+"/messages", strings.NewReader(`{
		"client_message_id":"client-id",
		"body":{"type":"text","content":"hello"}
	}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPath("/conversations/:conversation_id/messages")
	c.SetParamNames("conversation_id")
	c.SetParamValues(conversationID)
	c.Set(currentAccountKey, account.Account{ID: "account-id"})

	if err := api.create(c); err != nil {
		t.Fatalf("create message: %v", err)
	}
	if rec.Code != http.StatusCreated || stub.createCommand.ClientMessageID != "client-id" || stub.createCommand.AccountID != "account-id" {
		t.Fatalf("status = %d, command = %#v", rec.Code, stub.createCommand)
	}
	var response struct {
		Success bool                  `json:"success"`
		Data    createMessageResponse `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !response.Success || response.Data.Message.ID != "message-id" || response.Data.Message.CreatedAt != createdAt {
		t.Fatalf("response = %#v", response)
	}
}

func TestMessageAPISetsReaction(t *testing.T) {
	conversationID := uuid.NewString()
	messageID := uuid.NewString()
	stub := &messageServiceStub{setReactionResult: messageapp.SetReactionResult{
		ConversationID: conversationID, MessageID: messageID, ReactionVersion: 2,
		Reactions: []messageapp.ReactionSummary{{
			Count: 3, ReactedByMe: true, Text: "👍", Users: []messageapp.ReactionUser{
				{ID: "user-1", Name: "Alice"}, {ID: "user-2", Name: "Bob"}, {ID: "user-3", Name: "Carol"},
			},
		}},
	}}
	api := NewMessageAPI(stub, nil)
	e := echo.New()
	req := httptest.NewRequest(http.MethodPut, "/conversations/"+conversationID+"/messages/"+messageID+"/reactions", strings.NewReader(`{"text":"👍","reacted":true}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPath("/conversations/:conversation_id/messages/:message_id/reactions")
	c.SetParamNames("conversation_id", "message_id")
	c.SetParamValues(conversationID, messageID)
	c.Set(currentAccountKey, account.Account{ID: "account-id"})

	if err := api.setReaction(c); err != nil {
		t.Fatalf("set reaction: %v", err)
	}
	if rec.Code != http.StatusOK || stub.setReactionCommand != (messageapp.SetReactionCommand{
		AccountID: "account-id", ConversationID: conversationID, MessageID: messageID,
		Reacted: true, Text: "👍",
	}) {
		t.Fatalf("status = %d, command = %#v", rec.Code, stub.setReactionCommand)
	}
	var response struct {
		Success bool                       `json:"success"`
		Data    setMessageReactionResponse `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !response.Success || response.Data.ReactionVersion != 2 || len(response.Data.Reactions) != 1 ||
		!response.Data.Reactions[0].ReactedByMe || len(response.Data.Reactions[0].Users) != 3 ||
		response.Data.Reactions[0].Users[0] != (messageReactionUserResponse{ID: "user-1", Name: "Alice"}) {
		t.Fatalf("response = %#v", response)
	}
}

func TestMessageAPIListsReactionSnapshots(t *testing.T) {
	conversationID := uuid.NewString()
	messageID := uuid.NewString()
	stub := &messageServiceStub{listReactionSnapshotsResult: messageapp.ListReactionSnapshotsResult{
		ConversationID: conversationID,
		Snapshots: []messageapp.ReactionSnapshot{{
			MessageID: messageID, ReactionVersion: 4,
			Reactions: []messageapp.ReactionSummary{{
				Count: 2, ReactedByMe: true, Text: "👍", Users: []messageapp.ReactionUser{
					{ID: "user-1", Name: "Alice"}, {ID: "user-2", Name: "Bob"},
				},
			}},
		}},
	}}
	api := NewMessageAPI(stub, nil)
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/conversations/"+conversationID+"/messages/reactions/query", strings.NewReader(`{"message_ids":["`+messageID+`"]}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPath("/conversations/:conversation_id/messages/reactions/query")
	c.SetParamNames("conversation_id")
	c.SetParamValues(conversationID)
	c.Set(currentAccountKey, account.Account{ID: "account-id"})

	if err := api.listReactionSnapshots(c); err != nil {
		t.Fatalf("list reaction snapshots: %v", err)
	}
	if rec.Code != http.StatusOK || stub.listReactionSnapshotsCommand.AccountID != "account-id" ||
		stub.listReactionSnapshotsCommand.ConversationID != conversationID ||
		len(stub.listReactionSnapshotsCommand.MessageIDs) != 1 || stub.listReactionSnapshotsCommand.MessageIDs[0] != messageID {
		t.Fatalf("status = %d, command = %#v", rec.Code, stub.listReactionSnapshotsCommand)
	}
	var response struct {
		Success bool                                 `json:"success"`
		Data    listMessageReactionSnapshotsResponse `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !response.Success || len(response.Data.Snapshots) != 1 || response.Data.Snapshots[0].ReactionVersion != 4 ||
		len(response.Data.Snapshots[0].Reactions) != 1 || !response.Data.Snapshots[0].Reactions[0].ReactedByMe ||
		len(response.Data.Snapshots[0].Reactions[0].Users) != 2 {
		t.Fatalf("response = %#v", response)
	}
}

func TestMessageAPISubmitsChoiceResponse(t *testing.T) {
	conversationID := uuid.NewString()
	messageID := uuid.NewString()
	responseID := uuid.NewString()
	createdAt := time.Date(2026, 7, 23, 10, 0, 0, 0, time.UTC)
	stub := &messageServiceStub{submitChoiceResponseResult: messageapp.SubmitChoiceResponseResult{
		ConversationID: conversationID, MessageID: messageID, Created: true,
		Response: messageapp.ChoiceResponse{
			ID: responseID, UserID: "account-id", OptionIDs: []string{"a", "c"}, CreatedAt: createdAt,
		},
		Choice: messageapp.ChoiceState{
			MyOptionIDs: []string{"a", "c"}, ResponseCount: 2,
			Options: []messageapp.ChoiceOptionState{{ID: "a", ResponseCount: 2}, {ID: "b"}, {ID: "c", ResponseCount: 1}},
		},
	}}
	api := NewMessageAPI(stub, nil)
	e := echo.New()
	req := httptest.NewRequest(http.MethodPut,
		"/conversations/"+conversationID+"/messages/"+messageID+"/choice-response",
		strings.NewReader(`{"option_ids":["a","c"]}`),
	)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPath("/conversations/:conversation_id/messages/:message_id/choice-response")
	c.SetParamNames("conversation_id", "message_id")
	c.SetParamValues(conversationID, messageID)
	c.Set(currentAccountKey, account.Account{ID: "account-id"})

	if err := api.submitChoiceResponse(c); err != nil {
		t.Fatalf("submit choice response: %v", err)
	}
	if rec.Code != http.StatusCreated || stub.submitChoiceResponseCommand.AccountID != "account-id" ||
		stub.submitChoiceResponseCommand.ConversationID != conversationID || stub.submitChoiceResponseCommand.MessageID != messageID ||
		len(stub.submitChoiceResponseCommand.OptionIDs) != 2 {
		t.Fatalf("status = %d, command = %#v", rec.Code, stub.submitChoiceResponseCommand)
	}
	var response struct {
		Success bool                         `json:"success"`
		Data    submitChoiceResponseResponse `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !response.Success || response.Data.Response.ID != responseID || response.Data.Response.CreatedAt != createdAt ||
		response.Data.Choice.ResponseCount != 2 || len(response.Data.Choice.MyOptionIDs) != 2 {
		t.Fatalf("response = %#v", response)
	}
}

func TestMessageAPIListsChoiceSnapshots(t *testing.T) {
	conversationID := uuid.NewString()
	messageID := uuid.NewString()
	choice := messageapp.ChoiceState{
		MyOptionIDs: []string{"yes"}, ResponseCount: 3,
		Options: []messageapp.ChoiceOptionState{{ID: "yes", ResponseCount: 2}, {ID: "no", ResponseCount: 1}},
	}
	stub := &messageServiceStub{listChoiceSnapshotsResult: messageapp.ListChoiceSnapshotsResult{
		ConversationID: conversationID,
		Snapshots: []messageapp.ChoiceSnapshot{{
			MessageID: messageID, Choice: &choice, Status: "active",
		}},
	}}
	api := NewMessageAPI(stub, nil)
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost,
		"/conversations/"+conversationID+"/messages/choices/query",
		strings.NewReader(`{"message_ids":["`+messageID+`"]}`),
	)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPath("/conversations/:conversation_id/messages/choices/query")
	c.SetParamNames("conversation_id")
	c.SetParamValues(conversationID)
	c.Set(currentAccountKey, account.Account{ID: "account-id"})

	if err := api.listChoiceSnapshots(c); err != nil {
		t.Fatalf("list choice snapshots: %v", err)
	}
	if rec.Code != http.StatusOK || stub.listChoiceSnapshotsCommand.AccountID != "account-id" ||
		stub.listChoiceSnapshotsCommand.ConversationID != conversationID || len(stub.listChoiceSnapshotsCommand.MessageIDs) != 1 {
		t.Fatalf("status = %d, command = %#v", rec.Code, stub.listChoiceSnapshotsCommand)
	}
	var response struct {
		Success bool                        `json:"success"`
		Data    listChoiceSnapshotsResponse `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !response.Success || len(response.Data.Snapshots) != 1 ||
		response.Data.Snapshots[0].Status != "active" || response.Data.Snapshots[0].Choice == nil ||
		response.Data.Snapshots[0].Choice.ResponseCount != 3 || response.Data.Snapshots[0].Choice.MyOptionIDs[0] != "yes" {
		t.Fatalf("response = %#v", response)
	}
}

func TestMessageAPIListsReactionUsers(t *testing.T) {
	conversationID := uuid.NewString()
	messageID := uuid.NewString()
	stub := &messageServiceStub{listReactionUsersResult: messageapp.ListReactionUsersResult{
		ConversationID: conversationID, MessageID: messageID, Text: "👍",
		Users: []messageapp.ReactionUser{{ID: "user-1", Name: "Alice"}, {ID: "user-2", Name: "Bob"}},
	}}
	api := NewMessageAPI(stub, nil)
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/conversations/"+conversationID+"/messages/"+messageID+"/reactions/users?text=%F0%9F%91%8D", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPath("/conversations/:conversation_id/messages/:message_id/reactions/users")
	c.SetParamNames("conversation_id", "message_id")
	c.SetParamValues(conversationID, messageID)
	c.Set(currentAccountKey, account.Account{ID: "account-id"})

	if err := api.listReactionUsers(c); err != nil {
		t.Fatalf("list reaction users: %v", err)
	}
	if rec.Code != http.StatusOK || stub.listReactionUsersCommand != (messageapp.ListReactionUsersCommand{
		AccountID: "account-id", ConversationID: conversationID, MessageID: messageID, Text: "👍",
	}) {
		t.Fatalf("status = %d, command = %#v", rec.Code, stub.listReactionUsersCommand)
	}
	var response struct {
		Success bool                             `json:"success"`
		Data    listMessageReactionUsersResponse `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !response.Success || response.Data.Text != "👍" || len(response.Data.Users) != 2 ||
		response.Data.Users[1] != (messageReactionUserResponse{ID: "user-2", Name: "Bob"}) {
		t.Fatalf("response = %#v", response)
	}
}

func TestMessageAPICreatesFileMessageAfterPreparingUpload(t *testing.T) {
	conversationID := uuid.NewString()
	fileID := uuid.NewString()
	stub := &messageServiceStub{createResult: messageapp.CreateResult{Created: true, Message: messageapp.Message{ID: uuid.NewString()}}}
	files := &fakeFileService{uploaded: fileapp.TemporaryFile{ID: fileID, SizeBytes: 5}}
	api := NewMessageAPI(stub, files)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("client_message_id", "client-id"); err != nil {
		t.Fatalf("write client message id: %v", err)
	}
	part, err := writer.CreateFormFile("file", "hello.txt")
	if err != nil {
		t.Fatalf("create file part: %v", err)
	}
	if _, err := part.Write([]byte("hello")); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart body: %v", err)
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/conversations/"+conversationID+"/messages/files", &body)
	req.Header.Set(echo.HeaderContentType, writer.FormDataContentType())
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPath("/conversations/:conversation_id/messages/files")
	c.SetParamNames("conversation_id")
	c.SetParamValues(conversationID)
	c.Set(currentAccountKey, account.Account{ID: "account-id"})

	if err := api.createFile(c); err != nil {
		t.Fatalf("create file message: %v", err)
	}
	if rec.Code != http.StatusCreated || stub.prepareUploadCommand.ClientMessageID != "client-id" {
		t.Fatalf("status = %d, prepare command = %#v", rec.Code, stub.prepareUploadCommand)
	}
	if string(files.uploadContent) != "hello" || files.uploadCommand.ContentType != "application/octet-stream" {
		t.Fatalf("upload command = %#v, content = %q", files.uploadCommand, files.uploadContent)
	}
	if stub.createPreparedCommand.Summary != "[文件] hello.txt" {
		t.Fatalf("create prepared command = %#v", stub.createPreparedCommand)
	}
	var messageBody fileMessageBody
	if err := json.Unmarshal(stub.createPreparedCommand.Body, &messageBody); err != nil {
		t.Fatalf("decode message body: %v", err)
	}
	if messageBody.FileID != fileID || messageBody.Name != "hello.txt" || messageBody.SizeBytes != 5 {
		t.Fatalf("message body = %#v", messageBody)
	}
}

func TestMessageAPIAttachmentRetryDoesNotUploadAgain(t *testing.T) {
	conversationID := uuid.NewString()
	existing := messageapp.Message{ID: uuid.NewString(), ConversationID: conversationID}
	stub := &messageServiceStub{prepareUploadResult: messageapp.PrepareUploadResult{Existing: &existing}}
	api := NewMessageAPI(stub, nil)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("client_message_id", "client-id"); err != nil {
		t.Fatalf("write client message id: %v", err)
	}
	part, err := writer.CreateFormFile("file", "ignored.txt")
	if err != nil {
		t.Fatalf("create file part: %v", err)
	}
	if _, err := part.Write([]byte("ignored")); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart body: %v", err)
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/conversations/"+conversationID+"/messages/files", &body)
	req.Header.Set(echo.HeaderContentType, writer.FormDataContentType())
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPath("/conversations/:conversation_id/messages/files")
	c.SetParamNames("conversation_id")
	c.SetParamValues(conversationID)
	c.Set(currentAccountKey, account.Account{ID: "account-id"})

	if err := api.createFile(c); err != nil {
		t.Fatalf("retry file message: %v", err)
	}
	if rec.Code != http.StatusOK || stub.createPreparedCalled {
		t.Fatalf("status = %d, create prepared called = %t", rec.Code, stub.createPreparedCalled)
	}
}

func TestMessageAPIForwardValidatesConversationBeforeBindingBody(t *testing.T) {
	stub := &messageServiceStub{}
	api := NewMessageAPI(stub, nil)
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/conversations/not-a-uuid/messages/forward", strings.NewReader(`{`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPath("/conversations/:conversation_id/messages/forward")
	c.SetParamNames("conversation_id")
	c.SetParamValues("not-a-uuid")
	c.Set(currentAccountKey, account.Account{ID: "account-id"})

	if err := api.forward(c); err != nil {
		t.Fatalf("forward message: %v", err)
	}
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "会话 ID 格式错误") {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if stub.forwardCalled {
		t.Fatal("message service called before path validation")
	}
}

type messageServiceStub struct {
	listCommand                  messageapp.ListCommand
	listResult                   messageapp.ListResult
	listErr                      error
	createCommand                messageapp.CreateCommand
	createResult                 messageapp.CreateResult
	createErr                    error
	prepareUploadCommand         messageapp.PrepareUploadCommand
	prepareUploadResult          messageapp.PrepareUploadResult
	prepareUploadErr             error
	createPreparedCommand        messageapp.CreatePreparedCommand
	createPreparedCalled         bool
	forwardCalled                bool
	setReactionCommand           messageapp.SetReactionCommand
	setReactionResult            messageapp.SetReactionResult
	setReactionErr               error
	listReactionSnapshotsCommand messageapp.ListReactionSnapshotsCommand
	listReactionSnapshotsResult  messageapp.ListReactionSnapshotsResult
	listReactionSnapshotsErr     error
	listReactionUsersCommand     messageapp.ListReactionUsersCommand
	listReactionUsersResult      messageapp.ListReactionUsersResult
	listReactionUsersErr         error
	submitChoiceResponseCommand  messageapp.SubmitChoiceResponseCommand
	submitChoiceResponseResult   messageapp.SubmitChoiceResponseResult
	submitChoiceResponseErr      error
	listChoiceSnapshotsCommand   messageapp.ListChoiceSnapshotsCommand
	listChoiceSnapshotsResult    messageapp.ListChoiceSnapshotsResult
	listChoiceSnapshotsErr       error
}

func (s *messageServiceStub) List(_ context.Context, command messageapp.ListCommand) (messageapp.ListResult, error) {
	s.listCommand = command
	return s.listResult, s.listErr
}

func (s *messageServiceStub) Create(_ context.Context, command messageapp.CreateCommand) (messageapp.CreateResult, error) {
	s.createCommand = command
	return s.createResult, s.createErr
}

func (s *messageServiceStub) PrepareUpload(_ context.Context, command messageapp.PrepareUploadCommand) (messageapp.PrepareUploadResult, error) {
	s.prepareUploadCommand = command
	return s.prepareUploadResult, s.prepareUploadErr
}

func (s *messageServiceStub) CreatePrepared(_ context.Context, command messageapp.CreatePreparedCommand) (messageapp.CreateResult, error) {
	s.createPreparedCalled = true
	s.createPreparedCommand = command
	return s.createResult, s.createErr
}

func (s *messageServiceStub) Revoke(context.Context, messageapp.RevokeCommand) (messageapp.RevokeResult, error) {
	return messageapp.RevokeResult{}, nil
}

func (s *messageServiceStub) Forward(context.Context, messageapp.ForwardCommand) (messageapp.ForwardResult, error) {
	s.forwardCalled = true
	return messageapp.ForwardResult{}, nil
}

func (s *messageServiceStub) SetReaction(_ context.Context, command messageapp.SetReactionCommand) (messageapp.SetReactionResult, error) {
	s.setReactionCommand = command
	return s.setReactionResult, s.setReactionErr
}

func (s *messageServiceStub) ListReactionSnapshots(_ context.Context, command messageapp.ListReactionSnapshotsCommand) (messageapp.ListReactionSnapshotsResult, error) {
	s.listReactionSnapshotsCommand = command
	return s.listReactionSnapshotsResult, s.listReactionSnapshotsErr
}

func (s *messageServiceStub) ListReactionUsers(_ context.Context, command messageapp.ListReactionUsersCommand) (messageapp.ListReactionUsersResult, error) {
	s.listReactionUsersCommand = command
	return s.listReactionUsersResult, s.listReactionUsersErr
}

func (s *messageServiceStub) SubmitChoiceResponse(_ context.Context, command messageapp.SubmitChoiceResponseCommand) (messageapp.SubmitChoiceResponseResult, error) {
	s.submitChoiceResponseCommand = command
	return s.submitChoiceResponseResult, s.submitChoiceResponseErr
}

func (s *messageServiceStub) ListChoiceSnapshots(_ context.Context, command messageapp.ListChoiceSnapshotsCommand) (messageapp.ListChoiceSnapshotsResult, error) {
	s.listChoiceSnapshotsCommand = command
	return s.listChoiceSnapshotsResult, s.listChoiceSnapshotsErr
}
