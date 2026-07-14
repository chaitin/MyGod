package networktools

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"assistant/internal/publicnet"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestHTTPClientNormalizesRequestAndContentLength(t *testing.T) {
	var captured *http.Request
	source := &Source{
		guard: publicnet.NewGuard(),
		httpClient: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			captured = request
			return &http.Response{
				Body:       io.NopCloser(strings.NewReader(`{"ok":true}`)),
				Header:     http.Header{"X-Result": []string{"created"}},
				StatusCode: http.StatusCreated,
			}, nil
		})},
	}
	body := `{"name":"demo"}`
	input, err := json.Marshal(map[string]any{
		"method": " post ",
		"url":    "https://8.8.8.8/submit#ignored",
		"headers": map[string]string{
			"content-length": "999",
			"content-type":   "application/json",
			"x-demo":         "value",
		},
		"body": body,
	})
	if err != nil {
		t.Fatalf("marshal input: %v", err)
	}

	result, err := source.CallTool(context.Background(), httpClientToolName, input)
	if err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}
	if captured == nil {
		t.Fatal("HTTP request was not sent")
	}
	if captured.Method != http.MethodPost || captured.URL.Fragment != "" {
		t.Fatalf("request method/url = %s/%s", captured.Method, captured.URL.String())
	}
	if captured.ContentLength != int64(len([]byte(body))) {
		t.Fatalf("ContentLength = %d, want %d", captured.ContentLength, len([]byte(body)))
	}
	if captured.Header.Get("Content-Length") != "" || captured.Header.Get("Content-Type") != "application/json" || captured.Header.Get("X-Demo") != "value" {
		t.Fatalf("headers = %#v", captured.Header)
	}
	requestBody, err := io.ReadAll(captured.Body)
	if err != nil {
		t.Fatalf("read request body: %v", err)
	}
	if string(requestBody) != body {
		t.Fatalf("body = %q, want %q", requestBody, body)
	}

	var response httpClientResult
	if err := json.Unmarshal([]byte(result.Content), &response); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if response.StatusCode != http.StatusCreated || response.Body != `{"ok":true}` || response.Headers["X-Result"][0] != "created" || response.Truncated {
		t.Fatalf("response = %#v", response)
	}
}

func TestHTTPClientRejectsPrivateTargetsBeforeSending(t *testing.T) {
	called := false
	source := &Source{
		guard: publicnet.NewGuard(),
		httpClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			called = true
			return nil, nil
		})},
	}
	_, err := source.CallTool(context.Background(), httpClientToolName, json.RawMessage(`{
		"method":"GET",
		"url":"http://127.0.0.1/admin"
	}`))
	if err == nil || !strings.Contains(err.Error(), "non-public") {
		t.Fatalf("CallTool() error = %v, want non-public rejection", err)
	}
	if called {
		t.Fatal("HTTP transport was called for a private target")
	}
}

func TestHTTPClientRejectsManagedAndInjectedHeaders(t *testing.T) {
	for _, headers := range []map[string]string{
		{"Host": "example.com"},
		{"X-Test": "safe\r\ninjected: true"},
	} {
		input, err := json.Marshal(map[string]any{
			"method":  "GET",
			"url":     "https://8.8.8.8/",
			"headers": headers,
		})
		if err != nil {
			t.Fatalf("marshal input: %v", err)
		}
		source := &Source{guard: publicnet.NewGuard(), httpClient: &http.Client{}}
		if _, err := source.CallTool(context.Background(), httpClientToolName, input); err == nil {
			t.Fatalf("CallTool(%#v) error = nil", headers)
		}
	}
}

func TestHTTPClientTruncatesLargeResponse(t *testing.T) {
	source := &Source{
		guard: publicnet.NewGuard(),
		httpClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{
				Body:       io.NopCloser(strings.NewReader(strings.Repeat("x", maxHTTPResponseBodyBytes+1))),
				Header:     http.Header{},
				StatusCode: http.StatusOK,
			}, nil
		})},
	}
	result, err := source.CallTool(context.Background(), httpClientToolName, json.RawMessage(`{"method":"GET","url":"https://8.8.8.8/"}`))
	if err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}
	var response httpClientResult
	if err := json.Unmarshal([]byte(result.Content), &response); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if !response.Truncated || len(response.Body) != maxHTTPResponseBodyBytes {
		t.Fatalf("response length/truncated = %d/%v", len(response.Body), response.Truncated)
	}
}
