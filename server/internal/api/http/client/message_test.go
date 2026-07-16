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
	listCommand           messageapp.ListCommand
	listResult            messageapp.ListResult
	listErr               error
	createCommand         messageapp.CreateCommand
	createResult          messageapp.CreateResult
	createErr             error
	prepareUploadCommand  messageapp.PrepareUploadCommand
	prepareUploadResult   messageapp.PrepareUploadResult
	prepareUploadErr      error
	createPreparedCommand messageapp.CreatePreparedCommand
	createPreparedCalled  bool
	forwardCalled         bool
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
