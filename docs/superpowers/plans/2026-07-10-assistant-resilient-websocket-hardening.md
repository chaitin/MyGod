# Assistant Resilient WebSocket Hardening Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Completed steps use checkbox (`- [x]`) syntax for tracking.

**Goal:** Close the remaining ordering, memory-bound, permanent-authentication, and exact-message-size gaps in the resilient assistant WebSocket implementation.

**Architecture:** Create app event outbox rows inside the same conversation-row-locked transaction that allocates message seq, and hold the existing app-event ordering mutex through commit and live delivery. Replay server rows with keyset pagination, cap the assistant FIFO event queue, and invalidate only the current WebSocket generation on overflow. Preserve both job-local and process-level sequence dedupe, surface permanent authentication failures, and write the exact bytes used for the 1 MiB size check.

**Tech Stack:** Go 1.25, Gorilla WebSocket, Echo, GORM/PostgreSQL with SQLite integration tests, Go testing and race detector.

---

### Task 1: Restore job-local sequence deduplication

**Files:**
- Modify: `assistant/internal/appclient/runner.go:86-129`
- Modify: `assistant/internal/appclient/runner_test.go`

- [x] **Step 1: Write the failing watermark-eviction test**

Add `fmt` to `runner_test.go` and add:

```go
func TestConversationAgentRunnerUsesJobWatermarkAfterGlobalEviction(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runner := newConversationAgentRunner(ctx)
	outputs := make(chan struct{}, 2)
	assistantAgent := agent.New(llmModelFunc(func(context.Context, llm.Request) (llm.Response, error) {
		return llm.Response{Blocks: []llm.Block{{Type: llm.BlockTypeText, Text: "完成"}}}, nil
	}))
	sink := agent.OutputSinkFunc(func(context.Context, string) error {
		outputs <- struct{}{}
		return nil
	})
	prepared := preparedTextRun("conversation-1", "message-1", 7, "第一条")
	runner.Start(ctx, "conversation-1", sink, assistantAgent, prepared)
	waitForSignal(t, outputs, "first response")

	runner.mu.Lock()
	for i := 0; i <= maxConversationSequenceWatermarks; i++ {
		runner.recordSequenceLocked(fmt.Sprintf("other-%d", i), 1)
	}
	_, stillCached := runner.lastSeenSeq["conversation-1"]
	runner.mu.Unlock()
	if stillCached {
		t.Fatal("conversation watermark was not evicted")
	}

	runner.Start(ctx, "conversation-1", sink, assistantAgent, prepared)
	select {
	case <-outputs:
		t.Fatal("duplicate sequence appended after global watermark eviction")
	case <-time.After(50 * time.Millisecond):
	}
}
```

- [x] **Step 2: Run the test and verify RED**

```bash
cd assistant
go test ./internal/appclient -run TestConversationAgentRunnerUsesJobWatermarkAfterGlobalEviction -count=1
```

Expected: FAIL because the existing-job branch appends seq 7 after the process watermark is evicted.

- [x] **Step 3: Add the job-local check before `Session.Append`**

Inside `if job, ok := r.jobs[key]`, before stopping the timer, add:

```go
if prepared.MessageSeq > 0 && prepared.MessageSeq <= job.lastSeenSeq {
	r.mu.Unlock()
	return true
}
```

Keep the process-level check at the top of `Start`; it covers jobs already removed by idle cleanup.

- [x] **Step 4: Run focused tests and verify GREEN**

```bash
cd assistant
go test ./internal/appclient -run 'TestConversationAgentRunner(UsesJobWatermarkAfterGlobalEviction|IgnoresDuplicateSequence|KeepsSequenceWatermarkAfterIdleCleanup)$' -count=1
```

Expected: PASS.

- [x] **Step 5: Commit**

```bash
git add assistant/internal/appclient/runner.go assistant/internal/appclient/runner_test.go
git commit -m "fix: retain session-local sequence dedupe"
```

### Task 2: Return permanent authentication failures

**Files:**
- Modify: `assistant/internal/appclient/requester.go:1-15`
- Modify: `assistant/internal/appclient/transport.go:74-93`
- Modify: `assistant/internal/appclient/client.go:223-246`
- Modify: `assistant/internal/appclient/transport_test.go`

- [x] **Step 1: Write the failing transport authentication test**

```go
func TestWebSocketManagerReturnsPermanentAuthenticationError(t *testing.T) {
	attempts := 0
	manager := newWebSocketManager(config.Config{WebSocketURL: "ws://server/ws"}, webSocketManagerOptions{
		Dial: func(context.Context, string, http.Header) (*websocket.Conn, *http.Response, error) {
			attempts++
			return nil, &http.Response{StatusCode: http.StatusUnauthorized}, errors.New("unauthorized")
		},
	})
	err := manager.Run(context.Background(), func(envelope) {})
	if !errors.Is(err, errWebSocketAuthentication) {
		t.Fatalf("Run() error = %v, want authentication error", err)
	}
	if attempts != 1 {
		t.Fatalf("dial attempts = %d, want 1", attempts)
	}
}
```

- [x] **Step 2: Run the transport test and verify RED**

```bash
cd assistant
go test ./internal/appclient -run TestWebSocketManagerReturnsPermanentAuthenticationError -count=1
```

Expected: build failure because `errWebSocketAuthentication` is undefined.

- [x] **Step 3: Introduce and return the permanent sentinel**

Define beside `errWebSocketUnavailable`:

```go
var errWebSocketAuthentication = errors.New("websocket authentication failed")
```

Wrap 401/403 in `webSocketManager.Run`:

```go
return fmt.Errorf("%w: dial %s failed: %v, status=%d", errWebSocketAuthentication, m.cfg.WebSocketURL, err, resp.StatusCode)
```

- [x] **Step 4: Write the failing `Client.Run` propagation test**

```go
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
```

- [x] **Step 5: Run the propagation test and verify RED**

```bash
cd assistant
go test ./internal/appclient -run TestClientRunReturnsPermanentAuthenticationError -count=1
```

Expected: FAIL because `Client.Run` waits for context cancellation and returns nil.

- [x] **Step 6: Propagate permanent errors from `Client.Run`**

Immediately after the context check following `transport.Run`, add:

```go
if errors.Is(err, errWebSocketAuthentication) {
	return err
}
```

- [x] **Step 7: Verify and commit**

```bash
cd assistant
go test ./internal/appclient -run 'Test(WebSocketManagerReturnsPermanentAuthenticationError|ClientRunReturnsPermanentAuthenticationError)$' -count=1
cd ..
git add assistant/internal/appclient/client.go assistant/internal/appclient/requester.go assistant/internal/appclient/transport.go assistant/internal/appclient/transport_test.go
git commit -m "fix: stop reconnecting after app authentication failure"
```

Expected: PASS before commit.

### Task 3: Send the exact bytes checked by the 1 MiB limit

**Files:**
- Modify: `server/internal/appconnection/connection.go:78-123`
- Modify: `server/internal/appconnection/manager_test.go`

- [x] **Step 1: Write the exact-boundary WebSocket test**

```go
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
		t.Fatalf("write request: %v", err)
	}
	messageType, encoded, err := client.ReadMessage()
	if err != nil {
		t.Fatalf("read exact-limit response: %v", err)
	}
	if messageType != websocket.TextMessage || len(encoded) != 1<<20 {
		t.Fatalf("response type/bytes = %d/%d, want text/%d", messageType, len(encoded), 1<<20)
	}
}
```

- [x] **Step 2: Run the test and verify RED**

```bash
cd server
go test ./internal/appconnection -run TestConnectionWritesEnvelopeAtExactOneMiBBoundary -count=1
```

Expected: FAIL with `websocket: read limit exceeded` because `WriteJSON` appends a newline.

- [x] **Step 3: Encode once and write raw text bytes**

Replace the envelope-returning limit helper with:

```go
func encodeOutboundEnvelope(message realtime.Envelope, maxMessageBytes int64) ([]byte, bool) {
	encoded, err := json.Marshal(message)
	if err == nil && int64(len(encoded)) <= maxMessageBytes {
		return encoded, true
	}
	if message.Kind != realtime.KindResponse {
		return nil, false
	}
	replacement, err := json.Marshal(realtime.NewErrorResponse(message.ReplyTo, "response_too_large", "应用响应超过 1MiB 限制"))
	if err != nil || int64(len(replacement)) > maxMessageBytes {
		return nil, false
	}
	return replacement, true
}
```

Replace the `writeLoop` message case with:

```go
case message := <-c.send:
	encoded, ok := encodeOutboundEnvelope(message, c.manager.maxMessageBytes)
	if !ok {
		log.Printf("skip oversized app websocket event: app_id=%s event=%s", c.appID, message.Event)
		continue
	}
	if err := c.writeMessage(encoded); err != nil {
		return
	}
```

Replace `writeJSON` with:

```go
func (c *Connection) writeMessage(encoded []byte) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	_ = c.socket.SetWriteDeadline(time.Now().Add(c.manager.writeWait))
	return c.socket.WriteMessage(websocket.TextMessage, encoded)
}
```

Replace the two existing outbound-limit tests with:

```go
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
```

- [x] **Step 4: Verify and commit**

```bash
cd server
go test ./internal/appconnection -run 'Test(ConnectionWritesEnvelopeAtExactOneMiBBoundary|EncodeOutboundEnvelope)' -count=1
cd ..
git add server/internal/appconnection/connection.go server/internal/appconnection/manager_test.go
git commit -m "fix: enforce exact app websocket write limit"
```

Expected: PASS before commit.

### Task 4: Bound server replay and assistant event admission

**Files:**
- Modify: `server/internal/httpserver/app_message_events.go:122-145`
- Modify: `server/internal/httpserver/server_test.go`
- Modify: `assistant/internal/appclient/client.go:20-358`
- Modify: `assistant/internal/appclient/transport.go:74-235`
- Modify: `assistant/internal/appclient/transport_test.go`

- [x] **Step 1: Write the failing server pagination test**

Add `gorm.io/gorm/clause` to `server_test.go`, then add:

```go
func TestAppWebSocketReplaysOutboxInBoundedPages(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()
	now := time.Now().UTC()
	app := insertTestApp(t, db, store.App{
		Name:             "Echo App",
		Enabled:          true,
		Visibility:       store.AppVisibilityPublic,
		ConnectionSecret: "echo-app-secret",
		CreatedAt:        now,
		UpdatedAt:        now,
	})
	events := make([]store.AppEventOutbox, 205)
	for i := range events {
		events[i] = store.AppEventOutbox{
			AppID:     app.ID,
			Event:     realtime.EventMessageCreated,
			Payload:   json.RawMessage(fmt.Sprintf(`{"index":%d}`, i)),
			CreatedAt: now.Add(time.Duration(i) * time.Millisecond),
		}
	}
	if err := db.Create(&events).Error; err != nil {
		t.Fatalf("create app events: %v", err)
	}

	queryLimits := make(chan int, 8)
	callbackName := "test:capture_app_event_replay_limits"
	if err := db.Callback().Query().Before("gorm:query").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Table != "app_event_outbox" {
			return
		}
		limit := -1
		if limitClause, ok := tx.Statement.Clauses["LIMIT"].Expression.(clause.Limit); ok && limitClause.Limit != nil {
			limit = *limitClause.Limit
		}
		queryLimits <- limit
	}); err != nil {
		t.Fatalf("register query callback: %v", err)
	}
	t.Cleanup(func() { _ = db.Callback().Query().Remove(callbackName) })

	conn := dialAppWebSocket(t, server, app.ID, app.ConnectionSecret)
	for i, stored := range events {
		replayed := readRealtimeEvent(t, conn)
		if replayed.Cursor != stored.ID {
			t.Fatalf("replayed cursor %d = %d, want %d", i, replayed.Cursor, stored.ID)
		}
	}
	limits := make([]int, 0, len(queryLimits))
	for len(queryLimits) > 0 {
		limits = append(limits, <-queryLimits)
	}
	if !slices.Equal(limits, []int{100, 100, 100}) {
		t.Fatalf("outbox query limits = %v, want [100 100 100]", limits)
	}
}
```

- [x] **Step 2: Run the pagination test and verify RED**

```bash
cd server
go test ./internal/httpserver -run TestAppWebSocketReplaysOutboxInBoundedPages -count=1
```

Expected: FAIL because current replay performs one query with no limit.

- [x] **Step 3: Implement keyset pagination**

Add:

```go
const appEventReplayPageSize = 100
```

Replace the unbounded outbox query in `replayAppEvents` with:

```go
nextCursor := lastAckedCursor
for {
	var events []store.AppEventOutbox
	if err := s.db.
		Where("app_id = ? AND id > ?", appID, nextCursor).
		Order("id ASC").
		Limit(appEventReplayPageSize).
		Find(&events).Error; err != nil {
		return err
	}
	for _, event := range events {
		if !conn.EnqueueReliable(realtime.NewCursorEvent(event.ID, event.Event, event.Payload)) {
			return errors.New("app connection closed during event replay")
		}
	}
	if len(events) < appEventReplayPageSize {
		return nil
	}
	nextCursor = events[len(events)-1].ID
}
```

- [x] **Step 4: Write the failing assistant admission tests**

Add:

```go
func TestClientRejectsNewCursorWhenEventQueueIsFull(t *testing.T) {
	client := &Client{eventCursors: make(map[int64]struct{}), eventRunning: true}
	for i := int64(1); i <= maxQueuedAppEvents; i++ {
		if !client.enqueueAppEvent(context.Background(), envelope{Kind: kindEvent, Cursor: i}) {
			t.Fatalf("cursor %d rejected before capacity", i)
		}
	}
	if client.enqueueAppEvent(context.Background(), envelope{Kind: kindEvent, Cursor: maxQueuedAppEvents + 1}) {
		t.Fatal("new cursor above queue capacity was accepted")
	}
	if !client.enqueueAppEvent(context.Background(), envelope{Kind: kindEvent, Cursor: 1}) {
		t.Fatal("duplicate cursor should remain an accepted wake signal")
	}
}
```

Add coverage proving a full event queue does not interfere with response routing:

```go
func TestClientRoutesResponseWhenEventQueueIsFull(t *testing.T) {
	responseC := make(chan envelope, 1)
	requester := &reliableRequester{pending: map[string]chan envelope{"request-1": responseC}}
	client := &Client{requester: requester, eventCursors: make(map[int64]struct{}), eventRunning: true}
	for i := int64(1); i <= maxQueuedAppEvents; i++ {
		if !client.enqueueAppEvent(context.Background(), envelope{Kind: kindEvent, Cursor: i}) {
			t.Fatalf("cursor %d rejected before capacity", i)
		}
	}
	ok := true
	response := envelope{Kind: kindResponse, ReplyTo: "request-1", OK: &ok}
	if !client.handleTransportMessage(context.Background(), response) {
		t.Fatal("response was rejected while event queue was full")
	}
	select {
	case got := <-responseC:
		if got.ReplyTo != response.ReplyTo {
			t.Fatalf("response ReplyTo = %q, want %q", got.ReplyTo, response.ReplyTo)
		}
	default:
		t.Fatal("response was not routed to reliable requester")
	}
}
```

Add coverage proving a cursor rejected at capacity can be accepted after an earlier cursor is completed, and that its replay remains deduplicated:

```go
func TestClientAcceptsReplayedCursorAfterQueueCapacityReturns(t *testing.T) {
	client := &Client{eventCursors: make(map[int64]struct{}), eventRunning: true}
	for i := int64(1); i <= maxQueuedAppEvents; i++ {
		if !client.enqueueAppEvent(context.Background(), envelope{Kind: kindEvent, Cursor: i}) {
			t.Fatalf("cursor %d rejected before capacity", i)
		}
	}
	replayed := envelope{Kind: kindEvent, Cursor: maxQueuedAppEvents + 1}
	if client.enqueueAppEvent(context.Background(), replayed) {
		t.Fatal("overflow cursor was accepted before capacity returned")
	}
	client.eventMu.Lock()
	delete(client.eventCursors, 1)
	client.eventQueue = client.eventQueue[1:]
	client.lastAckedCursor = 1
	client.eventMu.Unlock()
	if !client.enqueueAppEvent(context.Background(), replayed) {
		t.Fatal("replayed cursor was rejected after capacity returned")
	}
	client.eventMu.Lock()
	queueLength := len(client.eventQueue)
	client.eventMu.Unlock()
	if !client.enqueueAppEvent(context.Background(), replayed) {
		t.Fatal("duplicate replay should remain an accepted wake signal")
	}
	client.eventMu.Lock()
	duplicateLength := len(client.eventQueue)
	client.eventMu.Unlock()
	if duplicateLength != queueLength {
		t.Fatalf("queue length after duplicate replay = %d, want %d", duplicateLength, queueLength)
	}
}
```

Also add this direct generation test; prebuild the event on the test goroutine so the HTTP handler never calls `t.Fatal`:

```go
func TestWebSocketManagerInvalidatesGenerationWhenHandlerRejectsEvent(t *testing.T) {
	event := testMessageCreatedEnvelope(t, "user-1", "message-1", 1, "第一条")
	event.Cursor = maxQueuedAppEvents + 1
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
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
				return
			}
		}
	}))
	defer server.Close()
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	generation := &connectionGeneration{id: 1, conn: conn, done: make(chan struct{})}
	defer generation.close()
	manager := newWebSocketManager(config.Config{}, webSocketManagerOptions{})
	ctx := context.Background()
	err = manager.serveGeneration(ctx, generation, func(envelope) bool { return false })
	if !errors.Is(err, errAppEventQueueFull) {
		t.Fatalf("serveGeneration() error = %v, want queue full", err)
	}
	if ctx.Err() != nil {
		t.Fatalf("process context error = %v, want active", ctx.Err())
	}
}
```

- [x] **Step 5: Run assistant admission tests and verify RED**

```bash
cd assistant
go test ./internal/appclient -run 'Test(ClientRejectsNewCursorWhenEventQueueIsFull|ClientRoutesResponseWhenEventQueueIsFull|ClientAcceptsReplayedCursorAfterQueueCapacityReturns|WebSocketManagerInvalidatesGenerationWhenHandlerRejectsEvent)$' -count=1
```

Expected: build failure because bounded admission and boolean callbacks do not exist.

- [x] **Step 6: Implement bounded admission and generation invalidation**

Add:

```go
const maxQueuedAppEvents = 256
var errAppEventQueueFull = errors.New("app event queue full")
```

Change these signatures consistently:

```go
func (c *Client) handleTransportMessage(context.Context, envelope) bool
func (c *Client) enqueueAppEvent(context.Context, envelope) bool
func (m *webSocketManager) Run(context.Context, func(envelope) bool) error
func (m *webSocketManager) serveGeneration(context.Context, *connectionGeneration, func(envelope) bool) error
```

Implement response routing and bounded event admission as:

```go
func (c *Client) handleTransportMessage(ctx context.Context, message envelope) bool {
	if message.Kind == kindResponse {
		c.requester.HandleResponse(message)
		return true
	}
	return c.enqueueAppEvent(ctx, message)
}

func (c *Client) enqueueAppEvent(ctx context.Context, message envelope) bool {
	c.eventMu.Lock()
	if message.Cursor > 0 {
		if message.Cursor <= c.lastAckedCursor {
			c.eventMu.Unlock()
			return true
		}
		if c.eventCursors == nil {
			c.eventCursors = make(map[int64]struct{})
		}
		if _, exists := c.eventCursors[message.Cursor]; exists {
			c.eventVersion++
			if !c.eventRunning {
				c.eventRunning = true
				c.eventMu.Unlock()
				go c.drainAppEvents()
				return true
			}
			c.eventMu.Unlock()
			return true
		}
	}
	if len(c.eventQueue) >= maxQueuedAppEvents {
		c.eventMu.Unlock()
		return false
	}
	if message.Cursor > 0 {
		c.eventCursors[message.Cursor] = struct{}{}
	}
	c.eventQueue = append(c.eventQueue, &queuedAppEvent{ctx: ctx, message: message})
	c.eventVersion++
	if c.eventRunning {
		c.eventMu.Unlock()
		return true
	}
	c.eventRunning = true
	c.eventMu.Unlock()
	go c.drainAppEvents()
	return true
}
```

Pass `c.handleTransportMessage` through the boolean callback in `Client.Run`. In the reader goroutine, invalidate only the current generation when admission fails:

```go
if ok && handle != nil && !handle(message) {
	readErr <- errAppEventQueueFull
	return
}
```

Update every existing manager callback in tests to return true.

- [x] **Step 7: Verify and commit bounded replay**

```bash
cd server
go test ./internal/httpserver -run TestAppWebSocketReplaysOutboxInBoundedPages -count=1
cd ../assistant
go test ./internal/appclient -run 'Test(ClientRejectsNewCursorWhenEventQueueIsFull|ClientRoutesResponseWhenEventQueueIsFull|ClientAcceptsReplayedCursorAfterQueueCapacityReturns|ClientReplayRetriesAcknowledgementWithoutReprocessingEvent|WebSocketManagerInvalidatesGenerationWhenHandlerRejectsEvent)$' -count=1
go test ./internal/appclient -count=1
go test -race -count=1 ./internal/appclient ./internal/agent
cd ..
git add server/internal/httpserver/app_message_events.go server/internal/httpserver/server_test.go assistant/internal/appclient/client.go assistant/internal/appclient/transport.go assistant/internal/appclient/transport_test.go
git commit -m "fix: bound app event replay memory"
```

Expected: PASS with no race report before commit.

#### Completed review follow-up: protect responses and bound writer fairness

- [x] `d704ddf` added an independent bounded response queue, routed every normal/error response through reliable admission, made the writer response-first, and added the real `Client.Run` overflow integration test.
- [x] `7224371` introduced a fairness point after at most 16 consecutive responses, separately guaranteeing bounded progress for event-only and ping-only readiness; the simultaneous-ready case remained open until `5fe51d0`.
- [x] `5fe51d0` made the fairness point service both a ready ping and one ready event when they are simultaneously ready, preventing event starvation.

Verification recorded for these review follow-ups:

- `TestConnectionPrioritizesRequestResponseOverQueuedEvent` was observed RED before the response-queue implementation and GREEN afterward.
- `TestConnectionServicesQueuedEventWithinResponseBurst`, `TestConnectionSendsPingWithinResponseBurst`, and `TestConnectionServicesPingAndEventAtResponseFairnessPoint` cover event, ping, and simultaneous-ready fairness.
- `TestClientRecoversFromEventQueueOverflowWithPrioritizedResponses` runs a real `Client.Run` through 257 events, overflow close, reconnect, stable history request ID, ACK 1..257, and duplicate-seq agent deduplication.

### Task 5: Create message events inside the message transaction

**Files:**
- Modify: `server/internal/httpserver/server.go:20-31`
- Modify: `server/internal/httpserver/message_handlers.go:278-325,552-675`
- Modify: `server/internal/httpserver/message_image_handlers.go:176-213`
- Modify: `server/internal/httpserver/message_file_handlers.go:158-195`
- Modify: `server/internal/httpserver/app_message_events.go`
- Modify: `server/internal/httpserver/server_test.go`

- [x] **Step 1: Write failing atomicity and ordering tests**

Add `EmitAppEvent: true` to the test calls and add two unexported ordering test seams to `Server`:

```go
beforeAppEventLock     func(store.Message)
afterUserMessageCommit func(store.Message)
```

Use this helper in both tests:

```go
func insertTestAppConversationLink(t *testing.T, db *gorm.DB, appID string, userID string, conversationID string, now time.Time) {
	t.Helper()
	member := store.ConversationMember{
		ConversationID:        conversationID,
		MemberType:            store.ConversationMemberTypeApp,
		MemberID:              appID,
		Role:                  store.ConversationMemberRoleMember,
		JoinedAt:              now,
		HistoryVisibleFromSeq: 1,
	}
	if err := db.Create(&member).Error; err != nil {
		t.Fatalf("create app conversation member: %v", err)
	}
	link := store.AppConversation{AppID: appID, UserID: userID, ConversationID: conversationID, CreatedAt: now}
	if err := db.Create(&link).Error; err != nil {
		t.Fatalf("create app conversation link: %v", err)
	}
}
```

Add the rollback test:

```go
func TestUserMessageRollsBackWhenAppEventOutboxInsertFails(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()
	now := time.Now().UTC()
	user := insertTestUser(t, db, "alice@example.com", "Alice", store.UserStatusActive, now)
	app := insertTestApp(t, db, store.App{Name: "Echo", Enabled: true, Visibility: store.AppVisibilityPublic, ConnectionSecret: "secret", CreatedAt: now, UpdatedAt: now})
	conversation := insertTestConversation(t, db, testConversationInput{createdByUserID: user.ID, kind: store.ConversationKindApp, memberIDs: []string{user.ID}, name: app.Name, now: now})
	insertTestAppConversationLink(t, db, app.ID, user.ID, conversation.ID, now)
	if err := db.Callback().Create().Before("gorm:create").Register("test:fail_outbox_create", func(tx *gorm.DB) {
		if tx.Statement.Table == "app_event_outbox" {
			tx.AddError(errors.New("forced outbox failure"))
		}
	}); err != nil {
		t.Fatalf("register callback: %v", err)
	}
	subject := &Server{db: db}
	_, _, _, _, err := subject.createUserMessageWithMetadata(context.Background(), user.ID, conversation.ID, "atomic-1", json.RawMessage(`{"type":"text","content":"hello"}`), staticMessageBodyFinalizer("hello"), createMessageMetadata{EmitAppEvent: true})
	if err == nil {
		t.Fatal("createUserMessageWithMetadata() error = nil, want outbox failure")
	}
	var messages, events int64
	_ = db.Model(&store.Message{}).Where("conversation_id = ?", conversation.ID).Count(&messages).Error
	_ = db.Model(&store.AppEventOutbox{}).Where("app_id = ?", app.ID).Count(&events).Error
	if messages != 0 || events != 0 {
		t.Fatalf("messages/events = %d/%d, want 0/0", messages, events)
	}
}
```

Add the deterministic ordering test:

```go
func TestConcurrentAppMessagesPersistOutboxInSequenceOrder(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()
	now := time.Now().UTC()
	user := insertTestUser(t, db, "alice@example.com", "Alice", store.UserStatusActive, now)
	app := insertTestApp(t, db, store.App{Name: "Echo", Enabled: true, Visibility: store.AppVisibilityPublic, ConnectionSecret: "secret", CreatedAt: now, UpdatedAt: now})
	conversation := insertTestConversation(t, db, testConversationInput{createdByUserID: user.ID, kind: store.ConversationKindApp, memberIDs: []string{user.ID}, name: app.Name, now: now})
	insertTestAppConversationLink(t, db, app.ID, user.ID, conversation.ID, now)
	firstCommitted := make(chan struct{})
	secondReachedEventLock := make(chan struct{})
	releaseFirst := make(chan struct{})
	subject := &Server{db: db}
	subject.beforeAppEventLock = func(message store.Message) {
		if message.Seq == 2 {
			close(secondReachedEventLock)
		}
	}
	subject.afterUserMessageCommit = func(message store.Message) {
		if message.Seq == 1 {
			close(firstCommitted)
			<-releaseFirst
		}
	}
	errs := make(chan error, 2)
	create := func(id string, content string) {
		_, _, _, _, err := subject.createUserMessageWithMetadata(context.Background(), user.ID, conversation.ID, id, json.RawMessage(fmt.Sprintf(`{"type":"text","content":%q}`, content)), staticMessageBodyFinalizer(content), createMessageMetadata{EmitAppEvent: true})
		errs <- err
	}
	go create("message-1", "first")
	<-firstCommitted
	go create("message-2", "second")
	<-secondReachedEventLock
	close(releaseFirst)
	for range 2 {
		if err := <-errs; err != nil {
			t.Fatalf("create message: %v", err)
		}
	}
	var rows []store.AppEventOutbox
	if err := db.Where("app_id = ?", app.ID).Order("id ASC").Find(&rows).Error; err != nil {
		t.Fatalf("load outbox: %v", err)
	}
	seqs := make([]int64, len(rows))
	for i, row := range rows {
		var payload appMessageCreatedPayload
		if err := json.Unmarshal(row.Payload, &payload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		seqs[i] = payload.Message.Seq
	}
	if !slices.Equal(seqs, []int64{1, 2}) {
		t.Fatalf("outbox seqs = %v, want [1 2]", seqs)
	}
}
```

- [x] **Step 2: Run the tests and verify RED**

```bash
cd server
go test ./internal/httpserver -run 'Test(UserMessageRollsBackWhenAppEventOutboxInsertFails|ConcurrentAppMessagesPersistOutboxInSequenceOrder)$' -count=1
```

Expected: build failure because `EmitAppEvent` and `afterUserMessageCommit` are undefined.

- [x] **Step 3: Add explicit event-emission metadata**

Extend `createMessageMetadata`:

```go
EmitAppEvent bool
```

Set the text, image, and file user HTTP handler metadata literals to:

```go
createMessageMetadata{
	EmitAppEvent:     true,
	ReplyToMessageID: replyToMessageID,
}
```

Do not set `EmitAppEvent` in `handleAppSendMessageAsUser`; delegated app output must not recursively trigger apps.

- [x] **Step 4: Split event creation from live delivery**

Replace post-commit discovery with these transaction-aware helpers in `app_message_events.go`:

```go
func (s *Server) createAppMessageEventOutbox(tx *gorm.DB, conversation store.Conversation, sender store.User, message store.Message) ([]store.AppEventOutbox, error) {
	var appIDs []string
	switch conversation.Kind {
	case store.ConversationKindApp:
		appID, ok, err := findMessageConversationAppID(tx, conversation.ID)
		if err != nil {
			return nil, err
		}
		if ok {
			appIDs = []string{appID}
		}
	case store.ConversationKindGroup:
		var err error
		appIDs, err = findMentionedGroupAppIDs(tx, conversation.ID, message.Body)
		if err != nil {
			return nil, err
		}
	}

	payload := appMessageCreatedPayload{
		Conversation: appMessageConversationPayload{ID: conversation.ID, Name: conversation.Name, Type: conversation.Kind},
		Message: appMessagePayload{
			Body: message.Body, CreatedAt: message.CreatedAt, ID: message.ID,
			Seq: message.Seq, Summary: message.Summary,
		},
		Sender: appMessageSenderPayload{
			Email: sender.Email, ID: sender.ID, Name: sender.Name,
			Nickname: sender.Nickname, Type: store.MessageSenderTypeUser,
		},
	}
	events := make([]store.AppEventOutbox, 0, len(appIDs))
	for _, appID := range appIDs {
		stored, err := createStoredAppEvent(tx, appID, realtime.EventMessageCreated, payload)
		if err != nil {
			return nil, err
		}
		events = append(events, stored)
	}
	return events, nil
}

func findMessageConversationAppID(db *gorm.DB, conversationID string) (string, bool, error) {
	var member store.ConversationMember
	err := db.First(
		&member,
		"conversation_id = ? AND member_type = ? AND left_at IS NULL",
		conversationID,
		store.ConversationMemberTypeApp,
	).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return member.MemberID, true, nil
}

func findMentionedGroupAppIDs(db *gorm.DB, conversationID string, body json.RawMessage) ([]string, error) {
	targets := parseMessageMentionTargets(body)
	targetSet := make(map[string]struct{}, len(targets))
	targetIDs := make([]string, 0, len(targets))
	for _, target := range targets {
		if target.All || target.MemberType != store.ConversationMemberTypeApp {
			continue
		}
		if _, ok := targetSet[target.MemberID]; ok {
			continue
		}
		targetSet[target.MemberID] = struct{}{}
		targetIDs = append(targetIDs, target.MemberID)
	}
	if len(targetIDs) == 0 {
		return nil, nil
	}
	var members []store.ConversationMember
	if err := db.Where(
		"conversation_id = ? AND member_type = ? AND member_id IN ? AND left_at IS NULL",
		conversationID, store.ConversationMemberTypeApp, targetIDs,
	).Find(&members).Error; err != nil {
		return nil, err
	}
	memberSet := make(map[string]struct{}, len(members))
	for _, member := range members {
		memberSet[member.MemberID] = struct{}{}
	}
	appIDs := make([]string, 0, len(targetIDs))
	for _, targetID := range targetIDs {
		if _, ok := memberSet[targetID]; ok {
			appIDs = append(appIDs, targetID)
		}
	}
	return appIDs, nil
}

func createStoredAppEvent(db *gorm.DB, appID string, event string, payload any) (store.AppEventOutbox, error) {
	rawPayload, err := json.Marshal(payload)
	if err != nil {
		return store.AppEventOutbox{}, err
	}
	stored := store.AppEventOutbox{
		AppID: appID, Event: event, Payload: rawPayload, CreatedAt: time.Now().UTC(),
	}
	if err := db.Create(&stored).Error; err != nil {
		return store.AppEventOutbox{}, err
	}
	return stored, nil
}

func (s *Server) deliverStoredAppEvents(events []store.AppEventOutbox) {
	if s.appConnections == nil {
		return
	}
	for _, event := range events {
		s.appConnections.SendToApp(event.AppID, realtime.NewCursorEvent(event.ID, event.Event, event.Payload))
	}
}
```

Delete `dispatchAppMessageCreatedEvent`, `sendAppMessageCreatedEvent`, `enqueueAppEvent`, and the receiver-based lookup helpers they replace.

- [x] **Step 5: Insert outbox rows before the message transaction commits**

Add `outboxEvents` and `appEventLockHeld` beside the existing return state at the top of `createUserMessageWithMetadata`. After message and mention metadata are ready inside the new-message branch, move `created = true` before event creation and add:

```go
created = true
if metadata.EmitAppEvent {
	var sender store.User
	if err := tx.First(&sender, "id = ?", userID).Error; err != nil {
		return err
	}
	if s.beforeAppEventLock != nil {
		s.beforeAppEventLock(message)
	}
	s.appEventMu.Lock()
	appEventLockHeld = true
	outboxEvents, err = s.createAppMessageEventOutbox(tx, conversation, sender, message)
	if err != nil {
		return err
	}
}
```

Immediately after `Transaction` returns, keep the lock through commit and live delivery with:

```go
if appEventLockHeld {
	defer s.appEventMu.Unlock()
}
if err != nil {
	return store.Message{}, false, nil, nil, err
}
if appEventLockHeld {
	if s.afterUserMessageCommit != nil {
		s.afterUserMessageCommit(message)
	}
	s.deliverStoredAppEvents(outboxEvents)
}
return message, created, memberUserIDs, mentionedUserIDs, nil
```

Remove the old transaction-error/return block it replaces, and remove post-handler calls to `dispatchAppMessageCreatedEvent` from the text, image, and file handlers.

- [x] **Step 6: Add duplicate-message outbox coverage**

Immediately after reading the first event in `TestAppWebSocketReceivesTextMessageEvents`, resend the same request and add:

```go
duplicateResp, duplicateBody := postJSON(t, server, "/api/client/conversations/"+conversationID+"/messages", map[string]any{
	"client_message_id": "client-message-1",
	"body": map[string]any{"type": "text", "content": "你好，应用"},
}, userCookie)
if duplicateResp.StatusCode != http.StatusOK {
	t.Fatalf("duplicate status = %d, want 200, body = %#v", duplicateResp.StatusCode, duplicateBody)
}
var outboxCount int64
if err := db.Model(&store.AppEventOutbox{}).Where("app_id = ?", app.ID).Count(&outboxCount).Error; err != nil {
	t.Fatalf("count app outbox: %v", err)
}
if outboxCount != 1 {
	t.Fatalf("outbox rows after duplicate message = %d, want 1", outboxCount)
}
```

- [x] **Step 7: Verify and commit transactional ordering**

```bash
cd server
go test ./internal/httpserver -run 'Test(UserMessageRollsBackWhenAppEventOutboxInsertFails|ConcurrentAppMessagesPersistOutboxInSequenceOrder|AppWebSocketReceivesTextMessageEvents)$' -count=1
go test ./internal/httpserver -count=1
cd ..
git add server/internal/httpserver
git commit -m "fix: order app events with message transactions"
```

Expected: PASS before commit.

### Task 6: Full verification, review, and documentation sync

**Files:**
- Modify: `docs/superpowers/specs/2026-07-10-assistant-resilient-websocket-hardening-design.md`
- Modify: `docs/superpowers/plans/2026-07-10-assistant-resilient-websocket-hardening.md`
- Inspect only if a commit hook generates a real diff: `api-docs/swagger.json`, `api-docs/swagger.yaml`

- [x] **Step 1: Format changed Go files**

```bash
git diff --name-only f4a9660..HEAD -- '*.go' | xargs gofmt -w
```

Completed: all changed Go files were formatted and `gofmt` produced no diff.

- [x] **Step 2: Run full tests without cache**

```bash
cd assistant && go test ./... -count=1
cd ../server && go test ./... -count=1
```

Completed: both module-wide uncached test commands exited successfully.

- [x] **Step 3: Run race tests without cache**

```bash
cd assistant && go test -race -count=1 ./internal/appclient ./internal/agent
cd ../server && go test -race -count=1 ./internal/appconnection ./internal/httpserver
```

Completed: both uncached race commands exited successfully with no race report.

- [x] **Step 4: Run deployment and diff checks**

```bash
./scripts/verify-deploy-config.sh
git diff --check
git status --short
git log --oneline --decorate -12
```

Completed: deploy config verification and diff/worktree checks exited successfully.

- [x] **Step 5: Request final code review**

Review `f4a9660..HEAD` against `docs/superpowers/specs/2026-07-10-assistant-resilient-websocket-hardening-design.md`. Require explicit checks for seq/cursor ordering, message/outbox rollback, bounded replay/admission, permanent auth propagation, exact 1 MiB writes, goroutine lifetime, and race safety. Fix every Critical or Important finding with a new failing test.

Completed at `5fe51d0`: final reviewer reported `Ready: Yes` with no findings.

- [x] **Step 6: Mark the plan complete and commit docs**

Change completed checkboxes in this file to `[x]`, then:

```bash
git add docs/superpowers/specs/2026-07-10-assistant-resilient-websocket-hardening-design.md \
  docs/superpowers/plans/2026-07-10-assistant-resilient-websocket-hardening.md
git commit -m "docs: sync resilient websocket hardening"
```

- [x] **Step 7: Verify the final worktree**

```bash
git status --short
git diff --check
git log --oneline --decorate -12
```

Expected: clean worktree on `feature/resilient-assistant-websocket`.
