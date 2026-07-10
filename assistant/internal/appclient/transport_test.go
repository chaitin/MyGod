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

func TestWebSocketManagerReturnsPermanentAuthenticationError(t *testing.T) {
	attempts := 0
	manager := newWebSocketManager(config.Config{WebSocketURL: "ws://server/ws"}, webSocketManagerOptions{
		Dial: func(context.Context, string, http.Header) (*websocket.Conn, *http.Response, error) {
			attempts++
			return nil, &http.Response{StatusCode: http.StatusUnauthorized}, errors.New("unauthorized")
		},
		Sleep: func(context.Context, time.Duration) error { return nil },
	})

	err := manager.Run(context.Background(), func(envelope) {})
	if !errors.Is(err, errWebSocketAuthentication) {
		t.Fatalf("Run() error = %v, want authentication error", err)
	}
	if attempts != 1 {
		t.Fatalf("dial attempts = %d, want 1", attempts)
	}
}

func TestClientRunReturnsPermanentAuthenticationError(t *testing.T) {
	transport := newWebSocketManager(config.Config{WebSocketURL: "ws://server/ws"}, webSocketManagerOptions{
		Dial: func(context.Context, string, http.Header) (*websocket.Conn, *http.Response, error) {
			return nil, &http.Response{StatusCode: http.StatusForbidden}, errors.New("forbidden")
		},
	})
	client := &Client{transport: transport, requester: newReliableRequester(transport, reliableRequesterOptions{})}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := client.Run(ctx)
	if !errors.Is(err, errWebSocketAuthentication) {
		t.Fatalf("Client.Run() error = %v, want authentication error", err)
	}
}

func TestWebSocketManagerBacksOffAfterConnectedGenerationDrops(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		_ = conn.Close()
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	delays := make(chan time.Duration, 1)
	manager := newWebSocketManager(config.Config{WebSocketURL: "ws" + strings.TrimPrefix(server.URL, "http")}, webSocketManagerOptions{
		Sleep: func(ctx context.Context, delay time.Duration) error {
			delays <- delay
			cancel()
			return ctx.Err()
		},
	})
	done := make(chan error, 1)
	go func() {
		done <- manager.Run(ctx, func(envelope) {})
	}()

	select {
	case delay := <-delays:
		if delay != time.Second {
			t.Fatalf("disconnect retry delay = %s, want 1s", delay)
		}
	case <-time.After(250 * time.Millisecond):
		cancel()
		t.Fatal("connected generations retried without backoff")
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run() error = %v, want nil after cancellation", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Run() did not stop after cancellation")
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

func TestClientAcknowledgesAcceptedCursorEvent(t *testing.T) {
	transport := &scriptedRequestTransport{}
	acknowledged := make(chan int64, 1)
	var requester *reliableRequester
	transport.onSend = func(message envelope, attempt int) (<-chan struct{}, error) {
		done := make(chan struct{})
		payload := json.RawMessage(`{}`)
		switch message.Method {
		case methodConversationMessagesList:
			payload = json.RawMessage(`{"messages":[]}`)
		case methodEventsAck:
			var ack struct {
				Cursor int64 `json:"cursor"`
			}
			if err := json.Unmarshal(message.Payload, &ack); err != nil {
				t.Fatalf("unmarshal ack payload: %v", err)
			}
			acknowledged <- ack.Cursor
		}
		ok := true
		requester.HandleResponse(envelope{V: protocolVersion, Kind: kindResponse, ReplyTo: message.ID, OK: &ok, Payload: payload})
		return done, nil
	}
	requester = newReliableRequester(transport, reliableRequesterOptions{
		Sleep: func(context.Context, time.Duration) error { return nil },
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client := &Client{
		cfg:       config.Config{AppID: "app-1"},
		requester: requester,
		runner:    newConversationAgentRunner(ctx),
		assistantAgent: replyAgentFunc(func(ctx context.Context, request agent.Request, sink agent.OutputSink) error {
			return sink.SendMarkdown(ctx, "完成")
		}),
	}
	event := testMessageCreatedEnvelope(t, "user-1", "message-1", 1, "第一条")
	event.Cursor = 42
	client.handleTransportMessage(ctx, event)

	select {
	case cursor := <-acknowledged:
		if cursor != 42 {
			t.Fatalf("acknowledged cursor = %d, want 42", cursor)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event acknowledgement")
	}
}

func TestClientRoutesCursorEventsInArrivalOrder(t *testing.T) {
	firstHistoryStarted := make(chan struct{})
	secondHistoryStarted := make(chan struct{})
	releaseFirstHistory := make(chan struct{})
	acknowledged := make(chan int64, 2)
	var historyCalls atomic.Int32
	var requester *reliableRequester
	transport := requestTransportFunc(func(ctx context.Context, message envelope) (<-chan struct{}, error) {
		done := make(chan struct{})
		payload := json.RawMessage(`{}`)
		switch message.Method {
		case methodConversationMessagesList:
			call := historyCalls.Add(1)
			if call == 1 {
				close(firstHistoryStarted)
				<-releaseFirstHistory
			} else if call == 2 {
				close(secondHistoryStarted)
			}
			payload = json.RawMessage(`{"messages":[]}`)
		case methodEventsAck:
			var ack struct {
				Cursor int64 `json:"cursor"`
			}
			if err := json.Unmarshal(message.Payload, &ack); err != nil {
				t.Errorf("unmarshal ack payload: %v", err)
			} else {
				acknowledged <- ack.Cursor
			}
		}
		ok := true
		requester.HandleResponse(envelope{V: protocolVersion, Kind: kindResponse, ReplyTo: message.ID, OK: &ok, Payload: payload})
		return done, nil
	})
	requester = newReliableRequester(transport, reliableRequesterOptions{
		Sleep: func(context.Context, time.Duration) error { return nil },
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client := &Client{
		cfg:       config.Config{AppID: "app-1"},
		requester: requester,
		runner:    newConversationAgentRunner(ctx),
		assistantAgent: replyAgentFunc(func(ctx context.Context, request agent.Request, sink agent.OutputSink) error {
			return sink.SendMarkdown(ctx, "完成")
		}),
	}
	first := testMessageCreatedEnvelope(t, "user-1", "message-1", 1, "第一条")
	first.Cursor = 10
	second := testMessageCreatedEnvelope(t, "user-1", "message-2", 2, "第二条")
	second.Cursor = 20
	client.handleTransportMessage(ctx, first)
	waitForSignal(t, firstHistoryStarted, "first history request")
	client.handleTransportMessage(ctx, second)

	select {
	case <-secondHistoryStarted:
		close(releaseFirstHistory)
		t.Fatal("second cursor started before the first cursor was accepted")
	case <-time.After(50 * time.Millisecond):
	}
	close(releaseFirstHistory)
	waitForSignal(t, secondHistoryStarted, "second history request")

	for _, want := range []int64{10, 20} {
		select {
		case cursor := <-acknowledged:
			if cursor != want {
				t.Fatalf("acknowledged cursor = %d, want %d", cursor, want)
			}
		case <-time.After(time.Second):
			t.Fatalf("timed out waiting for cursor %d acknowledgement", want)
		}
	}
}

func TestClientDoesNotAcknowledgeEventThatWasNotAcceptedOrDelivered(t *testing.T) {
	acknowledged := make(chan struct{}, 1)
	var requester *reliableRequester
	transport := requestTransportFunc(func(ctx context.Context, message envelope) (<-chan struct{}, error) {
		done := make(chan struct{})
		if message.Method == methodEventsAck {
			acknowledged <- struct{}{}
		}
		ok := false
		requester.HandleResponse(envelope{
			V:       protocolVersion,
			Kind:    kindResponse,
			ReplyTo: message.ID,
			OK:      &ok,
			Error:   &errorPayload{Code: "unavailable", Message: "try later"},
		})
		return done, nil
	})
	requester = newReliableRequester(transport, reliableRequesterOptions{
		Sleep: func(context.Context, time.Duration) error { return nil },
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client := &Client{
		cfg:       config.Config{AppID: "app-1"},
		requester: requester,
		runner:    newConversationAgentRunner(ctx),
		assistantAgent: replyAgentFunc(func(ctx context.Context, request agent.Request, sink agent.OutputSink) error {
			return sink.SendMarkdown(ctx, "完成")
		}),
	}
	event := testMessageCreatedEnvelope(t, "user-1", "message-1", 1, "第一条")
	event.Cursor = 42
	client.handleTransportMessage(ctx, event)

	select {
	case <-acknowledged:
		t.Fatal("unaccepted cursor event was acknowledged")
	case <-time.After(100 * time.Millisecond):
	}
}

func TestClientReplayRetriesAcknowledgementWithoutReprocessingEvent(t *testing.T) {
	firstAckFailed := make(chan struct{})
	acknowledged := make(chan struct{})
	var ackAttempts atomic.Int32
	var replyCalls atomic.Int32
	var requester *reliableRequester
	transport := requestTransportFunc(func(ctx context.Context, message envelope) (<-chan struct{}, error) {
		done := make(chan struct{})
		ok := true
		payload := json.RawMessage(`{}`)
		switch message.Method {
		case methodConversationMessagesList:
			payload = json.RawMessage(`{"messages":[]}`)
		case methodMessageSend:
			replyCalls.Add(1)
		case methodEventsAck:
			if ackAttempts.Add(1) == 1 {
				ok = false
				close(firstAckFailed)
			} else {
				close(acknowledged)
			}
		}
		response := envelope{V: protocolVersion, Kind: kindResponse, ReplyTo: message.ID, OK: &ok, Payload: payload}
		if !ok {
			response.Error = &errorPayload{Code: "unavailable", Message: "try later"}
		}
		requester.HandleResponse(response)
		return done, nil
	})
	requester = newReliableRequester(transport, reliableRequesterOptions{
		Sleep: func(context.Context, time.Duration) error { return nil },
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client := &Client{
		cfg:       config.Config{AppID: "app-1"},
		requester: requester,
		runner:    newConversationAgentRunner(ctx),
		assistantAgent: replyAgentFunc(func(ctx context.Context, request agent.Request, sink agent.OutputSink) error {
			return sink.SendMarkdown(ctx, "完成")
		}),
	}
	event := testMessageCreatedEnvelope(t, "user-1", "message-1", 1, "第一条")
	event.Cursor = 42
	client.handleTransportMessage(ctx, event)
	waitForSignal(t, firstAckFailed, "first acknowledgement failure")

	client.handleTransportMessage(ctx, event)
	waitForSignal(t, acknowledged, "replayed event acknowledgement")
	if calls := replyCalls.Load(); calls != 1 {
		t.Fatalf("agent reply calls = %d, want 1", calls)
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
