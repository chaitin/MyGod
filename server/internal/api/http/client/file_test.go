package client

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	fileapp "app/internal/application/file"

	"github.com/labstack/echo/v4"
)

func TestFileAPIRoutesUseFileService(t *testing.T) {
	now := time.Date(2026, 7, 15, 7, 0, 0, 0, time.UTC)
	service := &fakeFileService{
		uploaded: fileapp.TemporaryFile{ID: "7f8d8b84-6d2c-4b12-9a8a-019a7e2787d4", SizeBytes: 5, CreatedAt: now},
		resolved: []fileapp.ResolvedTemporaryURL{{
			FileID:    "7f8d8b84-6d2c-4b12-9a8a-019a7e2787d4",
			URL:       "https://assets.example.test/temporary/file",
			ExpiresAt: now.Add(24 * time.Hour),
		}},
	}
	api := NewFileAPI(service)
	router := echo.New()
	api.RegisterRoutes(router.Group("/api/client"))

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "hello.txt")
	if err != nil {
		t.Fatalf("create multipart file: %v", err)
	}
	if _, err := part.Write([]byte("hello")); err != nil {
		t.Fatalf("write multipart file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart body: %v", err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/client/temporary-files", &body)
	request.Header.Set(echo.HeaderContentType, writer.FormDataContentType())
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("upload status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if string(service.uploadContent) != "hello" || service.uploadCommand.SizeBytes != 5 {
		t.Fatalf("upload command = %#v, content = %q", service.uploadCommand, service.uploadContent)
	}

	request = httptest.NewRequest(http.MethodPost, "/api/client/temporary-files/read-urls", bytes.NewBufferString(`{"file_ids":["7f8d8b84-6d2c-4b12-9a8a-019a7e2787d4"]}`))
	request.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || len(service.resolveIDs) != 1 {
		t.Fatalf("read URLs status = %d, ids = %#v, body = %s", recorder.Code, service.resolveIDs, recorder.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	urls := payload["data"].(map[string]any)["urls"].([]any)
	if urls[0].(map[string]any)["url"] != service.resolved[0].URL {
		t.Fatalf("read URLs response = %#v", payload)
	}

	request = httptest.NewRequest(http.MethodGet, "/api/client/temporary-files/7f8d8b84-6d2c-4b12-9a8a-019a7e2787d4/content", nil)
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusTemporaryRedirect || recorder.Header().Get("Location") != service.resolved[0].URL {
		t.Fatalf("redirect status = %d, location = %q", recorder.Code, recorder.Header().Get("Location"))
	}
}

type fakeFileService struct {
	uploadCommand fileapp.UploadTemporaryCommand
	uploadContent []byte
	uploaded      fileapp.TemporaryFile
	resolveIDs    []string
	resolved      []fileapp.ResolvedTemporaryURL
}

func (s *fakeFileService) ValidateTemporaryFiles(context.Context, []string) error {
	return nil
}

func (s *fakeFileService) UploadTemporary(_ context.Context, cmd fileapp.UploadTemporaryCommand) (fileapp.TemporaryFile, error) {
	s.uploadCommand = cmd
	s.uploadContent, _ = io.ReadAll(cmd.Content)
	return s.uploaded, nil
}

func (s *fakeFileService) ResolveTemporaryURL(_ context.Context, fileID string) (fileapp.ResolvedTemporaryURL, error) {
	s.resolveIDs = []string{fileID}
	return s.resolved[0], nil
}

func (s *fakeFileService) ResolveTemporaryURLs(_ context.Context, fileIDs []string) ([]fileapp.ResolvedTemporaryURL, error) {
	s.resolveIDs = append([]string(nil), fileIDs...)
	return s.resolved, nil
}
