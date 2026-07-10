package appclient

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"assistant/internal/agent"
	"assistant/internal/config"

	"github.com/gorilla/websocket"
)

func TestWebSocketManagerStopsAfterTenRetries(t *testing.T) {
	attempts := 0
	delays := make([]time.Duration, 0)
	manager := newWebSocketManager(config.Config{WebSocketURL: "ws://server/ws"}, webSocketManagerOptions{
		MaxRetries: 10,
		Dial: func(ctx context.Context, url string, header http.Header) (*websocket.Conn, *http.Response, error) {
			attempts++
			return nil, nil, errors.New("offline")
		},
		Sleep: func(ctx context.Context, delay time.Duration) error {
			delays = append(delays, delay)
			return nil
		},
	})

	err := manager.Run(context.Background(), func(envelope) {})
	if err == nil || !errors.Is(err, errWebSocketUnavailable) {
		t.Fatalf("Run() error = %v, want websocket unavailable", err)
	}
	if attempts != 11 {
		t.Fatalf("dial attempts = %d, want initial attempt plus 10 retries", attempts)
	}
	want := []time.Duration{
		time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second,
		30 * time.Second, 30 * time.Second, 30 * time.Second, 30 * time.Second, 30 * time.Second,
	}
	if len(delays) != len(want) {
		t.Fatalf("retry delays = %v, want %v", delays, want)
	}
	for i := range want {
		if delays[i] != want[i] {
			t.Fatalf("retry delay %d = %s, want %s", i, delays[i], want[i])
		}
	}
}

func TestClientRetriesInFlightRequestAcrossReconnect(t *testing.T) {
	var connections atomic.Int32
	historyRequestIDs := make(chan string, 2)
	replyReceived := make(chan struct{})
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		connectionNumber := connections.Add(1)
		if connectionNumber == 1 {
			if err := conn.WriteJSON(testMessageCreatedEnvelope(t, "user-1", "message-1", 1, "第一条")); err != nil {
				return
			}
			var historyRequest envelope
			if err := conn.ReadJSON(&historyRequest); err != nil {
				return
			}
			historyRequestIDs <- historyRequest.ID
			return
		}

		var historyRequest envelope
		if err := conn.ReadJSON(&historyRequest); err != nil {
			return
		}
		historyRequestIDs <- historyRequest.ID
		ok := true
		historyPayload, _ := json.Marshal(appListConversationMessagesResponsePayload{})
		if err := conn.WriteJSON(envelope{V: protocolVersion, Kind: kindResponse, ReplyTo: historyRequest.ID, OK: &ok, Payload: historyPayload}); err != nil {
			return
		}

		var replyRequest envelope
		if err := conn.ReadJSON(&replyRequest); err != nil {
			return
		}
		if replyRequest.Method != methodMessageSend {
			return
		}
		if err := conn.WriteJSON(envelope{V: protocolVersion, Kind: kindResponse, ReplyTo: replyRequest.ID, OK: &ok, Payload: json.RawMessage(`{}`)}); err != nil {
			return
		}
		close(replyReceived)
		_, _, _ = conn.ReadMessage()
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cfg := config.Config{
		AppID:        "app-1",
		AppSecret:    "secret",
		WebSocketURL: "ws" + strings.TrimPrefix(server.URL, "http"),
	}
	transport := newWebSocketManager(cfg, webSocketManagerOptions{})
	requester := newReliableRequester(transport, reliableRequesterOptions{
		MaxRetries:   10,
		ResponseWait: time.Second,
		Sleep:        func(context.Context, time.Duration) error { return nil },
	})
	client := &Client{
		cfg:       cfg,
		dialer:    websocket.DefaultDialer,
		runner:    newConversationAgentRunner(ctx),
		transport: transport,
		requester: requester,
		assistantAgent: replyAgentFunc(func(ctx context.Context, request agent.Request, sink agent.OutputSink) error {
			return sink.SendMarkdown(ctx, "完成")
		}),
	}
	runDone := make(chan error, 1)
	go func() { runDone <- client.Run(ctx) }()

	select {
	case <-replyReceived:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for reply after reconnect")
	}
	firstID := <-historyRequestIDs
	secondID := <-historyRequestIDs
	if firstID == "" || firstID != secondID {
		t.Fatalf("history request IDs = %q and %q, want stable ID", firstID, secondID)
	}

	cancel()
	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("Client.Run() error = %v, want nil", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Client.Run() did not stop")
	}
}

func TestWebSocketManagerSendsAndRoutesEnvelope(t *testing.T) {
	received := make(chan envelope, 1)
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		var request envelope
		if err := conn.ReadJSON(&request); err != nil {
			return
		}
		ok := true
		_ = conn.WriteJSON(envelope{V: protocolVersion, Kind: kindResponse, ReplyTo: request.ID, OK: &ok})
		<-r.Context().Done()
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	manager := newWebSocketManager(config.Config{
		AppID:        "app-1",
		AppSecret:    "secret",
		WebSocketURL: "ws" + strings.TrimPrefix(server.URL, "http"),
	}, webSocketManagerOptions{})
	runDone := make(chan error, 1)
	go func() {
		runDone <- manager.Run(ctx, func(message envelope) { received <- message })
	}()

	sendCtx, cancelSend := context.WithTimeout(context.Background(), time.Second)
	defer cancelSend()
	generationDone, err := manager.Send(sendCtx, envelope{V: protocolVersion, Kind: kindRequest, ID: "request-1", Method: "test"})
	if err != nil {
		t.Fatalf("Send() error = %v, want nil", err)
	}
	if generationDone == nil {
		t.Fatal("Send() generation done channel = nil")
	}

	select {
	case response := <-received:
		if response.ReplyTo != "request-1" {
			t.Fatalf("response.ReplyTo = %q, want request-1", response.ReplyTo)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for routed response")
	}

	cancel()
	select {
	case <-runDone:
	case <-time.After(time.Second):
		t.Fatal("Run() did not stop after cancellation")
	}
}
