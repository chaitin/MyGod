package appclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

	err := manager.Run(context.Background(), func(envelope) bool { return true })
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

	err := manager.Run(context.Background(), func(envelope) bool { return true })
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

func TestClientRejectsNewCursorWhenEventQueueIsFull(t *testing.T) {
	client := &Client{
		eventCursors: make(map[int64]struct{}),
		eventRunning: true,
	}
	ctx := context.Background()
	for cursor := int64(1); cursor <= maxQueuedAppEvents; cursor++ {
		if !client.enqueueAppEvent(ctx, envelope{Kind: kindEvent, Cursor: cursor}) {
			t.Fatalf("enqueueAppEvent() cursor %d = false, want true", cursor)
		}
	}
	if client.enqueueAppEvent(ctx, envelope{Kind: kindEvent, Cursor: maxQueuedAppEvents + 1}) {
		t.Fatal("enqueueAppEvent() accepted a new cursor when the event queue was full")
	}
	if !client.enqueueAppEvent(ctx, envelope{Kind: kindEvent, Cursor: 1}) {
		t.Fatal("enqueueAppEvent() duplicate cursor = false, want true")
	}
}

func TestClientRoutesResponseWhenEventQueueIsFull(t *testing.T) {
	responseC := make(chan envelope, 1)
	requester := &reliableRequester{
		pending: map[string]chan envelope{"request-1": responseC},
	}
	client := &Client{
		requester:    requester,
		eventCursors: make(map[int64]struct{}),
		eventRunning: true,
	}
	ctx := context.Background()
	for cursor := int64(1); cursor <= maxQueuedAppEvents; cursor++ {
		if !client.enqueueAppEvent(ctx, envelope{Kind: kindEvent, Cursor: cursor}) {
			t.Fatalf("enqueueAppEvent() cursor %d = false, want true", cursor)
		}
	}

	ok := true
	if !client.handleTransportMessage(ctx, envelope{Kind: kindResponse, ReplyTo: "request-1", OK: &ok}) {
		t.Fatal("handleTransportMessage() response = false, want true")
	}
	select {
	case response := <-responseC:
		if response.ReplyTo != "request-1" {
			t.Fatalf("routed response.ReplyTo = %q, want request-1", response.ReplyTo)
		}
	default:
		t.Fatal("response was not routed immediately while the event queue was full")
	}
}

func TestClientAcceptsReplayedCursorAfterQueueCapacityReturns(t *testing.T) {
	client := &Client{
		eventCursors: make(map[int64]struct{}),
		eventRunning: true,
	}
	ctx := context.Background()
	for cursor := int64(1); cursor <= maxQueuedAppEvents; cursor++ {
		if !client.enqueueAppEvent(ctx, envelope{Kind: kindEvent, Cursor: cursor}) {
			t.Fatalf("enqueueAppEvent() cursor %d = false, want true", cursor)
		}
	}
	replayedCursor := int64(maxQueuedAppEvents + 1)
	if client.enqueueAppEvent(ctx, envelope{Kind: kindEvent, Cursor: replayedCursor}) {
		t.Fatal("enqueueAppEvent() accepted a new cursor before queue capacity returned")
	}

	client.eventMu.Lock()
	delete(client.eventCursors, 1)
	client.eventQueue = client.eventQueue[1:]
	client.lastAckedCursor = 1
	client.eventMu.Unlock()

	if !client.enqueueAppEvent(ctx, envelope{Kind: kindEvent, Cursor: replayedCursor}) {
		t.Fatal("enqueueAppEvent() replayed cursor = false after queue capacity returned")
	}
	queueLength := len(client.eventQueue)
	if !client.enqueueAppEvent(ctx, envelope{Kind: kindEvent, Cursor: replayedCursor}) {
		t.Fatal("enqueueAppEvent() duplicate replayed cursor = false, want true")
	}
	if len(client.eventQueue) != queueLength {
		t.Fatalf("event queue length after duplicate replay = %d, want %d", len(client.eventQueue), queueLength)
	}
}

func TestWebSocketManagerInvalidatesGenerationWhenHandlerRejectsEvent(t *testing.T) {
	event := testMessageCreatedEnvelope(t, "user-1", "message-1", 1, "first")
	event.Cursor = maxQueuedAppEvents + 1

	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	clientClosed := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		if err := conn.WriteJSON(event); err != nil {
			return
		}
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				close(clientClosed)
				return
			}
		}
	}))
	t.Cleanup(server.Close)

	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	generation := &connectionGeneration{id: 1, conn: conn, done: make(chan struct{})}
	t.Cleanup(generation.close)
	manager := newWebSocketManager(config.Config{}, webSocketManagerOptions{})
	ctx := context.Background()
	err = manager.serveGeneration(ctx, generation, func(envelope) bool { return false })
	if !errors.Is(err, errAppEventQueueFull) {
		t.Fatalf("serveGeneration() error = %v, want app event queue full", err)
	}
	if ctx.Err() != nil {
		t.Fatalf("serveGeneration() canceled process context: %v", ctx.Err())
	}

	generation.close()
	select {
	case <-clientClosed:
	case <-time.After(time.Second):
		t.Fatal("server did not observe rejected generation closing")
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
		done <- manager.Run(ctx, func(envelope) bool { return true })
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

		var noticeRequest envelope
		if err := conn.ReadJSON(&noticeRequest); err != nil {
			return
		}
		if noticeRequest.Method != methodMessageSend {
			return
		}
		if err := conn.WriteJSON(envelope{V: protocolVersion, Kind: kindResponse, ReplyTo: noticeRequest.ID, OK: &ok, Payload: json.RawMessage(`{}`)}); err != nil {
			return
		}

		var topicRequest envelope
		if err := conn.ReadJSON(&topicRequest); err != nil {
			return
		}
		if topicRequest.Method != methodConversationTopicCreate {
			return
		}
		topicPayload := json.RawMessage(`{"conversation":{"id":"topic-1","name":"第一条","type":"topic"},"created":true}`)
		if err := conn.WriteJSON(envelope{V: protocolVersion, Kind: kindResponse, ReplyTo: topicRequest.ID, OK: &ok, Payload: topicPayload}); err != nil {
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

func TestClientRecoversFromEventQueueOverflowWithPrioritizedResponses(t *testing.T) {
	events := make([]envelope, maxQueuedAppEvents+1)
	for i := range events {
		events[i] = testMessageCreatedEnvelope(t, "user-1", "message-1", 1, "第一条")
		events[i].Cursor = int64(i + 1)
	}

	var connections atomic.Int32
	var finished atomic.Bool
	historyRequestIDs := make(chan string, 2)
	firstGenerationClosed := make(chan struct{})
	acknowledged := make(chan int64, len(events))
	finalAcknowledged := make(chan struct{})
	serverErrors := make(chan error, 1)
	reportServerError := func(err error) {
		select {
		case serverErrors <- err:
		default:
		}
	}

	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			reportServerError(fmt.Errorf("upgrade websocket: %w", err))
			return
		}
		defer conn.Close()

		generation := connections.Add(1)
		if generation > 2 {
			reportServerError(fmt.Errorf("unexpected websocket generation %d", generation))
			return
		}

		writeResponse := func(request envelope) (int64, error) {
			ok := true
			payload := json.RawMessage(`{}`)
			ackCursor := int64(0)
			switch request.Method {
			case methodConversationMessagesList:
				payload = json.RawMessage(`{"messages":[]}`)
			case methodConversationTopicCreate:
				payload = json.RawMessage(`{"conversation":{"id":"topic-1","name":"第一条","type":"topic"},"created":true}`)
			case methodEventsAck:
				var ack struct {
					Cursor int64 `json:"cursor"`
				}
				if err := json.Unmarshal(request.Payload, &ack); err != nil {
					return 0, fmt.Errorf("unmarshal event ack: %w", err)
				}
				ackCursor = ack.Cursor
			}
			if err := conn.WriteJSON(envelope{
				V:       protocolVersion,
				Kind:    kindResponse,
				ReplyTo: request.ID,
				OK:      &ok,
				Payload: payload,
			}); err != nil {
				return 0, fmt.Errorf("write response to %s: %w", request.Method, err)
			}
			return ackCursor, nil
		}

		if generation == 1 {
			if err := conn.WriteJSON(events[0]); err != nil {
				reportServerError(fmt.Errorf("write first cursor event: %w", err))
				return
			}
			var historyRequest envelope
			if err := conn.ReadJSON(&historyRequest); err != nil {
				reportServerError(fmt.Errorf("read first history request: %w", err))
				return
			}
			if historyRequest.Method != methodConversationMessagesList {
				reportServerError(fmt.Errorf("first request method = %q, want %q", historyRequest.Method, methodConversationMessagesList))
				return
			}
			historyRequestIDs <- historyRequest.ID

			for _, event := range events[1:] {
				if err := conn.WriteJSON(event); err != nil {
					reportServerError(fmt.Errorf("write first-generation cursor %d: %w", event.Cursor, err))
					return
				}
			}
			for {
				if _, _, err := conn.ReadMessage(); err != nil {
					close(firstGenerationClosed)
					return
				}
			}
		}

		if err := conn.WriteJSON(events[0]); err != nil {
			reportServerError(fmt.Errorf("replay first cursor event: %w", err))
			return
		}
		var replayedHistoryRequest envelope
		if err := conn.ReadJSON(&replayedHistoryRequest); err != nil {
			reportServerError(fmt.Errorf("read replayed history request: %w", err))
			return
		}
		if replayedHistoryRequest.Method != methodConversationMessagesList {
			reportServerError(fmt.Errorf("replayed request method = %q, want %q", replayedHistoryRequest.Method, methodConversationMessagesList))
			return
		}
		historyRequestIDs <- replayedHistoryRequest.ID
		if _, err := writeResponse(replayedHistoryRequest); err != nil {
			reportServerError(err)
			return
		}

		var noticeRequest envelope
		if err := conn.ReadJSON(&noticeRequest); err != nil {
			reportServerError(fmt.Errorf("read replayed topic notice: %w", err))
			return
		}
		if noticeRequest.Method != methodMessageSend {
			reportServerError(fmt.Errorf("replayed request method = %q, want %q", noticeRequest.Method, methodMessageSend))
			return
		}
		if _, err := writeResponse(noticeRequest); err != nil {
			reportServerError(err)
			return
		}

		var topicRequest envelope
		if err := conn.ReadJSON(&topicRequest); err != nil {
			reportServerError(fmt.Errorf("read replayed topic request: %w", err))
			return
		}
		if topicRequest.Method != methodConversationTopicCreate {
			reportServerError(fmt.Errorf("replayed request method = %q, want %q", topicRequest.Method, methodConversationTopicCreate))
			return
		}
		if _, err := writeResponse(topicRequest); err != nil {
			reportServerError(err)
			return
		}

		var firstAckRequest envelope
		if err := conn.ReadJSON(&firstAckRequest); err != nil {
			reportServerError(fmt.Errorf("read first event ack: %w", err))
			return
		}
		firstAckCursor, err := writeResponse(firstAckRequest)
		if err != nil {
			reportServerError(err)
			return
		}
		if firstAckCursor != 1 {
			reportServerError(fmt.Errorf("first acknowledged cursor = %d, want 1", firstAckCursor))
			return
		}
		acknowledged <- firstAckCursor

		for _, event := range events[1:] {
			if err := conn.WriteJSON(event); err != nil {
				reportServerError(fmt.Errorf("replay cursor %d: %w", event.Cursor, err))
				return
			}
		}
		for {
			var request envelope
			if err := conn.ReadJSON(&request); err != nil {
				if !finished.Load() {
					reportServerError(fmt.Errorf("read replay request: %w", err))
				}
				return
			}
			ackCursor, err := writeResponse(request)
			if err != nil {
				if !finished.Load() {
					reportServerError(err)
				}
				return
			}
			if ackCursor == 0 {
				continue
			}
			acknowledged <- ackCursor
			if ackCursor == int64(len(events)) && finished.CompareAndSwap(false, true) {
				close(finalAcknowledged)
			}
		}
	}))
	t.Cleanup(server.Close)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	cfg := config.Config{
		AppID:        "app-1",
		AppSecret:    "secret",
		WebSocketURL: "ws" + strings.TrimPrefix(server.URL, "http"),
	}
	noWait := func(context.Context, time.Duration) error { return nil }
	transport := newWebSocketManager(cfg, webSocketManagerOptions{Sleep: noWait})
	requester := newReliableRequester(transport, reliableRequesterOptions{
		MaxRetries:   20,
		ResponseWait: 2 * time.Second,
		Sleep:        noWait,
	})
	var agentCalls atomic.Int32
	agentMessageIDs := make(chan string, 2)
	client := &Client{
		cfg:       cfg,
		transport: transport,
		requester: requester,
		runner:    newConversationAgentRunner(ctx),
		assistantAgent: replyAgentFunc(func(_ context.Context, request agent.Request, _ agent.OutputSink) error {
			agentCalls.Add(1)
			agentMessageIDs <- request.MessageID
			return nil
		}),
	}
	t.Cleanup(client.Close)
	runDone := make(chan error, 1)
	go func() { runDone <- client.Run(ctx) }()

	select {
	case <-firstGenerationClosed:
	case err := <-serverErrors:
		t.Fatalf("first generation server error: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for event queue overflow to close the first generation")
	}
	if ctx.Err() != nil {
		t.Fatalf("event queue overflow canceled process context: %v", ctx.Err())
	}

	select {
	case <-finalAcknowledged:
	case err := <-serverErrors:
		t.Fatalf("replay server error: %v", err)
	case <-time.After(10 * time.Second):
		t.Fatalf("timed out waiting for cursor %d acknowledgement after reconnect", len(events))
	}
	select {
	case err := <-serverErrors:
		t.Fatalf("replay server error after final acknowledgement: %v", err)
	default:
	}
	if ctx.Err() != nil {
		t.Fatalf("replay completion canceled process context: %v", ctx.Err())
	}
	if got := connections.Load(); got < 2 {
		t.Fatalf("websocket generations = %d, want at least 2", got)
	}

	firstHistoryID := <-historyRequestIDs
	replayedHistoryID := <-historyRequestIDs
	if firstHistoryID == "" || replayedHistoryID != firstHistoryID {
		t.Fatalf("history request IDs = %q and %q, want one stable non-empty ID", firstHistoryID, replayedHistoryID)
	}
	for want := int64(1); want <= int64(len(events)); want++ {
		select {
		case got := <-acknowledged:
			if got != want {
				t.Fatalf("acknowledged cursor %d = %d, want %d", want, got, want)
			}
		default:
			t.Fatalf("missing acknowledgement for cursor %d", want)
		}
	}
	if got := agentCalls.Load(); got != 1 {
		t.Fatalf("agent calls = %d, want 1 for replayed sequence", got)
	}
	select {
	case messageID := <-agentMessageIDs:
		if messageID != "message-1" {
			t.Fatalf("agent message ID = %q, want message-1", messageID)
		}
	default:
		t.Fatal("agent did not process the durable event")
	}
	select {
	case duplicate := <-agentMessageIDs:
		t.Fatalf("agent processed replayed event again: message ID %q", duplicate)
	default:
	}

	cancel()
	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("Client.Run() error = %v, want nil after cancellation", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Client.Run() did not stop after cancellation")
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
		case methodConversationTopicCreate:
			payload = json.RawMessage(`{"conversation":{"id":"topic-1","name":"第一条","type":"topic"},"created":true}`)
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
		case methodConversationTopicCreate:
			payload = json.RawMessage(`{"conversation":{"id":"topic-1","name":"第一条","type":"topic"},"created":true}`)
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
		case methodConversationTopicCreate:
			payload = json.RawMessage(`{"conversation":{"id":"topic-1","name":"第一条","type":"topic"},"created":true}`)
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
	if calls := replyCalls.Load(); calls != 2 {
		t.Fatalf("message send calls = %d, want 2", calls)
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
		runDone <- manager.Run(ctx, func(message envelope) bool {
			received <- message
			return true
		})
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
