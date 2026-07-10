package appconnection

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"app/internal/realtime"

	"github.com/gorilla/websocket"
)

func TestDefaultMaxMessageBytesIsOneMiB(t *testing.T) {
	manager := NewManager(Options{})
	if manager.maxMessageBytes != 1<<20 {
		t.Fatalf("maxMessageBytes = %d, want %d", manager.maxMessageBytes, 1<<20)
	}
}

func TestConnectionAcceptsRequestLargerThan64KiB(t *testing.T) {
	received := make(chan int, 1)
	manager := NewManager(Options{RequestHandler: func(appID string, request realtime.Envelope) realtime.Envelope {
		received <- len(request.Payload)
		return realtime.NewResponse(request.ID, map[string]any{"received": true})
	}})
	client := dialManagedWebSocket(t, manager)
	request := testAppRequest("large-request", "test.large", map[string]any{
		"content": strings.Repeat("x", 128<<10),
	})
	if err := client.WriteJSON(request); err != nil {
		t.Fatalf("write request: %v", err)
	}
	var response realtime.Envelope
	if err := client.ReadJSON(&response); err != nil {
		t.Fatalf("read response: %v", err)
	}
	if response.OK == nil || !*response.OK {
		t.Fatalf("response = %#v, want success", response)
	}
	select {
	case payloadBytes := <-received:
		if payloadBytes <= 64<<10 {
			t.Fatalf("received payload bytes = %d, want above 64 KiB", payloadBytes)
		}
	case <-time.After(time.Second):
		t.Fatal("large request did not reach handler")
	}
}

func TestConnectionRejectsRequestLargerThanOneMiB(t *testing.T) {
	called := make(chan struct{}, 1)
	manager := NewManager(Options{RequestHandler: func(appID string, request realtime.Envelope) realtime.Envelope {
		called <- struct{}{}
		return realtime.NewResponse(request.ID, nil)
	}})
	client := dialManagedWebSocket(t, manager)
	request := testAppRequest("oversized-request", "test.large", map[string]any{
		"content": strings.Repeat("x", (1<<20)+1),
	})
	if err := client.WriteJSON(request); err != nil {
		t.Fatalf("write oversized request: %v", err)
	}
	_ = client.SetReadDeadline(time.Now().Add(time.Second))
	var response realtime.Envelope
	if err := client.ReadJSON(&response); err == nil {
		t.Fatalf("ReadJSON() error = nil, response = %#v; want read-limit disconnect", response)
	}
	select {
	case <-called:
		t.Fatal("oversized request reached handler")
	default:
	}
}

func dialManagedWebSocket(t *testing.T, manager *Manager) *websocket.Conn {
	t.Helper()

	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		managed := manager.NewConnection("app-1", conn)
		manager.Register(managed)
		managed.Serve()
		manager.Unregister(managed)
	}))
	t.Cleanup(server.Close)
	client, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func exactSizeResponse(t *testing.T, replyTo string, size int) realtime.Envelope {
	t.Helper()
	base := realtime.NewResponse(replyTo, map[string]any{"content": ""})
	encoded, err := json.Marshal(base)
	if err != nil {
		t.Fatalf("marshal base response: %v", err)
	}
	response := realtime.NewResponse(replyTo, map[string]any{"content": strings.Repeat("x", size-len(encoded))})
	encoded, err = json.Marshal(response)
	if err != nil || len(encoded) != size {
		t.Fatalf("encoded response bytes/error = %d/%v, want %d/nil", len(encoded), err, size)
	}
	return response
}

func TestConnectionWritesEnvelopeAtExactOneMiBBoundary(t *testing.T) {
	response := exactSizeResponse(t, "exact-limit", 1<<20)
	manager := NewManager(Options{RequestHandler: func(string, realtime.Envelope) realtime.Envelope {
		return response
	}})
	client := dialManagedWebSocket(t, manager)
	client.SetReadLimit(1 << 20)
	if err := client.WriteJSON(testAppRequest("exact-limit", "test.limit", nil)); err != nil {
		t.Fatalf("write exact-limit request: %v", err)
	}
	_ = client.SetReadDeadline(time.Now().Add(time.Second))
	messageType, encoded, err := client.ReadMessage()
	if err != nil {
		t.Fatalf("read exact-limit response: %v", err)
	}
	if messageType != websocket.TextMessage || len(encoded) != 1<<20 {
		t.Fatalf("message type/bytes = %d/%d, want %d/%d", messageType, len(encoded), websocket.TextMessage, 1<<20)
	}
}

func TestLogSkippedOutboundEnvelopeIncludesResponseIdentity(t *testing.T) {
	var output bytes.Buffer
	previousWriter := log.Writer()
	log.SetOutput(&output)
	t.Cleanup(func() { log.SetOutput(previousWriter) })

	logSkippedOutboundEnvelope("app-1", realtime.NewResponse("request-1", nil))

	for _, want := range []string{"app_id=app-1", "kind=response", "reply_to=request-1"} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("log output = %q, want %q", output.String(), want)
		}
	}
}

func TestLogSkippedOutboundEnvelopeIncludesEventIdentity(t *testing.T) {
	var output bytes.Buffer
	previousWriter := log.Writer()
	log.SetOutput(&output)
	t.Cleanup(func() { log.SetOutput(previousWriter) })

	logSkippedOutboundEnvelope("app-1", realtime.NewEvent("large.event", nil))

	for _, want := range []string{"app_id=app-1", "kind=event", "event=large.event"} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("log output = %q, want %q", output.String(), want)
		}
	}
}

func TestEncodeOutboundEnvelopeReplacesOversizedResponse(t *testing.T) {
	payload, err := json.Marshal(map[string]any{"content": strings.Repeat("x", 1<<20)})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	response := realtime.NewResponse("request-1", nil)
	response.Payload = payload

	encoded, ok := encodeOutboundEnvelope(response, 1<<20)
	if !ok {
		t.Fatal("encodeOutboundEnvelope() ok = false, want replacement response")
	}
	var limited realtime.Envelope
	if err := json.Unmarshal(encoded, &limited); err != nil {
		t.Fatalf("unmarshal limited response: %v", err)
	}
	if limited.Error == nil || limited.Error.Code != "response_too_large" {
		t.Fatalf("limited response = %#v, want response_too_large", limited)
	}
	if limited.ReplyTo != "request-1" {
		t.Fatalf("limited.ReplyTo = %q, want request-1", limited.ReplyTo)
	}
}

func TestEncodeOutboundEnvelopeSkipsOversizedEvent(t *testing.T) {
	event := realtime.NewEvent("large.event", map[string]any{"content": strings.Repeat("x", 1<<20)})
	if _, ok := encodeOutboundEnvelope(event, 1<<20); ok {
		t.Fatal("encodeOutboundEnvelope() ok = true, want oversized event skipped")
	}
}

func TestManagerHandleRequestReplaysDuplicateResponse(t *testing.T) {
	calls := 0
	manager := NewManager(Options{RequestHandler: func(appID string, request realtime.Envelope) realtime.Envelope {
		calls++
		return realtime.NewResponse(request.ID, map[string]any{"calls": calls})
	}})
	request := testAppRequest("request-1", "method.one", map[string]any{"value": 1})

	first := manager.HandleRequest("app-1", request)
	second := manager.HandleRequest("app-1", request)
	if calls != 1 {
		t.Fatalf("handler calls = %d, want 1", calls)
	}
	if string(first.Payload) != string(second.Payload) {
		t.Fatalf("responses differ: %s != %s", first.Payload, second.Payload)
	}
}

func TestManagerClosesLaggingConnectionInsteadOfSkippingCursor(t *testing.T) {
	serverSocket := make(chan *websocket.Conn, 1)
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		serverSocket <- conn
		<-r.Context().Done()
	}))
	defer server.Close()
	client, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	defer client.Close()

	manager := NewManager(Options{SendBuffer: 1})
	conn := manager.NewConnection("app-1", <-serverSocket)
	manager.Register(conn)
	if !conn.Enqueue(realtime.NewCursorEvent(1, "test.event", map[string]any{"value": 1})) {
		t.Fatal("failed to fill connection send queue")
	}
	manager.SendToApp("app-1", realtime.NewCursorEvent(2, "test.event", map[string]any{"value": 2}))

	select {
	case <-conn.done:
	case <-time.After(time.Second):
		t.Fatal("lagging connection remained open after cursor enqueue failed")
	}
}
