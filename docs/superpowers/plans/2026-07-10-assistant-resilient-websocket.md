# Assistant Resilient WebSocket Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Decouple assistant sessions from WebSocket connections, raise the app protocol limit to 1 MiB, retry transient communication failures up to ten times, deduplicate retried server requests, and replay missed app events.

**Architecture:** Keep `conversationAgentRunner` at process scope, introduce a process-scoped WebSocket manager plus reliable request broker, and route every server-dependent action through it. Add server-side request response replay and a durable app-event outbox with acknowledgement cursors.

**Tech Stack:** Go 1.25, Gorilla WebSocket, Echo, GORM/PostgreSQL, Goose migrations, Go testing and race detector.

---

### Task 1: Raise and enforce the 1 MiB app protocol limit

**Files:**
- Modify: `assistant/internal/appclient/client.go`
- Modify: `assistant/internal/appclient/client_test.go`
- Modify: `server/internal/appconnection/manager.go`
- Modify: `server/internal/appconnection/connection.go`
- Create: `server/internal/appconnection/connection_test.go`
- Modify: `assistant/internal/builtintools/sleep.go`
- Modify: `assistant/internal/agent/agent.go`

- [x] **Step 1: Write failing limit tests**

Add tests proving an assistant request below 1 MiB is written, an envelope above 1 MiB is rejected, and a server connection configured with defaults accepts a message above 64 KiB but rejects one above 1 MiB. Use an `httptest.Server` WebSocket pair so the test exercises Gorilla's read limit.

```go
func TestConnectionRequesterUsesOneMiBMessageLimit(t *testing.T) {
    payload := map[string]any{"content": strings.Repeat("x", 128*1024)}
    // Request must reach the writer and complete through a synthetic response.
}

func TestDefaultMaxMessageBytesIsOneMiB(t *testing.T) {
    manager := NewManager(Options{})
    if manager.maxMessageBytes != 1<<20 {
        t.Fatalf("maxMessageBytes = %d, want %d", manager.maxMessageBytes, 1<<20)
    }
}
```

- [x] **Step 2: Run the new tests and confirm failure**

Run:

```bash
cd assistant && go test ./internal/appclient
cd ../server && go test ./internal/appconnection
```

Expected: failures report the existing 64 KiB limit.

- [x] **Step 3: Introduce the shared 1 MiB constants and outbound checks**

Use explicit constants in the two binaries:

```go
const maxMessageBytes = 1 << 20
const defaultMaxMessageBytes = 1 << 20
```

Marshal before every app envelope write and return `app websocket message exceeds 1MiB limit` locally. Server responses that exceed the limit must be replaced by a small `response_too_large` response; oversized events are logged and skipped without intentionally closing the connection.

- [x] **Step 4: Correct inline file descriptions**

Keep `maxAppInlineFileContentBytes` at 64 KiB but change prompt/tool wording from “WebSocket message limit” to “inline file content limit”.

- [x] **Step 5: Run focused tests**

```bash
cd assistant && go test ./internal/appclient ./internal/agent ./internal/builtintools
cd ../server && go test ./internal/appconnection
```

Expected: PASS.

- [x] **Step 6: Commit the limit change**

```bash
git add assistant/internal/appclient assistant/internal/agent/agent.go assistant/internal/builtintools/sleep.go server/internal/appconnection
git commit -m "fix: raise app websocket limit to 1 MiB"
```

### Task 2: Promote the conversation Session Manager to process scope

**Files:**
- Modify: `assistant/internal/appclient/client.go`
- Modify: `assistant/internal/appclient/runner.go`
- Modify: `assistant/internal/appclient/client_test.go`
- Create: `assistant/internal/appclient/runner_test.go`

- [x] **Step 1: Write failing lifecycle tests**

Move runner-specific tests into `runner_test.go` and add a test that simulates a connection handler returning while an agent model is blocked. Verify the agent context remains active and that a second message appends to the same `*agent.Session`.

```go
func TestConnectionExitDoesNotCancelConversationSession(t *testing.T) {
    manager := newConversationAgentRunner()
    // Start a blocked model cycle, end a synthetic connection, and assert
    // the job context has not closed before manager.Close().
}
```

- [x] **Step 2: Run the lifecycle test and confirm failure**

```bash
cd assistant && go test ./internal/appclient -run 'TestConnectionExitDoesNotCancelConversationSession'
```

Expected: the current connection-scoped `defer runner.CancelAll()` cancels the job.

- [x] **Step 3: Make the runner a `Client` field**

Initialize one runner in `New` and close it only from `Client.Close`/process shutdown:

```go
type Client struct {
    cfg            config.Config
    dialer         *websocket.Dialer
    assistantAgent replyAgent
    runner         *conversationAgentRunner
    mcpSources     []mcpclient.Source
}
```

Pass this runner into connection message dispatch. Remove connection-level `CancelAll`.

- [x] **Step 4: Rename shutdown semantics**

Expose `Close()` on the runner, keep the one-hour idle timeout, and ensure idle cleanup only removes non-running jobs with no pending instruction.

- [x] **Step 5: Run focused tests**

```bash
cd assistant && go test ./internal/appclient ./internal/agent
```

Expected: PASS, including existing append and idle reuse behavior.

- [x] **Step 6: Commit process-scoped sessions**

```bash
git add assistant/internal/appclient
git commit -m "refactor: keep assistant sessions across websocket reconnects"
```

### Task 3: Add the WebSocket manager and reliable request broker

**Files:**
- Create: `assistant/internal/appclient/transport.go`
- Create: `assistant/internal/appclient/transport_test.go`
- Create: `assistant/internal/appclient/requester.go`
- Create: `assistant/internal/appclient/requester_test.go`
- Modify: `assistant/internal/appclient/client.go`
- Modify: `assistant/internal/appclient/runner.go`
- Modify: `assistant/internal/appclient/client_test.go`

- [x] **Step 1: Write failing transport retry tests**

Use an injected dial function, fake clock/sleeper, and scripted connection server. Cover ten retries, capped exponential delays, jitter-free deterministic tests, reset after success, non-retryable authentication responses, one active generation, and shutdown cancellation.

```go
var expected = []time.Duration{
    time.Second, 2*time.Second, 4*time.Second, 8*time.Second,
    16*time.Second, 30*time.Second, 30*time.Second, 30*time.Second,
    30*time.Second, 30*time.Second,
}
```

- [x] **Step 2: Run transport tests and confirm failure**

```bash
cd assistant && go test ./internal/appclient -run 'TestWebSocketManager'
```

Expected: transport types are undefined.

- [x] **Step 3: Implement `webSocketManager`**

The manager owns the current generation and exposes:

```go
type connectionGeneration struct {
    id   uint64
    conn *websocket.Conn
    done chan struct{}
}

type webSocketManager struct {
    // mutex-protected current generation, wake channel, dialer and retry policy
}

func (m *webSocketManager) Run(ctx context.Context, handle func(envelope)) error
func (m *webSocketManager) Send(ctx context.Context, message envelope) (*connectionGeneration, error)
func (m *webSocketManager) Invalidate(generation *connectionGeneration, err error)
```

`Send` starts or wakes a bounded ten-retry connection cycle when disconnected. A stale generation may close itself but cannot clear a newer generation.

- [x] **Step 4: Write failing reliable requester tests**

Cover stable request IDs across reconnects, response routing by `reply_to`, generation failure while waiting, request timeout retries, protocol errors without retry, and retry exhaustion without canceling the caller's parent session.

```go
func TestReliableRequesterRetriesSameEnvelopeAfterDisconnect(t *testing.T) {
    // Capture every write and assert all retries use the same envelope.ID.
}
```

- [x] **Step 5: Implement `reliableRequester`**

Provide the existing interface while moving request state out of physical connections:

```go
type reliableRequester struct {
    transport appTransport
    pending   map[string]chan envelope
    // retry policy and mutex
}

func (r *reliableRequester) Request(ctx context.Context, method string, payload any) (json.RawMessage, error)
func (r *reliableRequester) HandleResponse(response envelope)
```

Create the request ID once, retry the same envelope at most ten times after the initial failure, and unregister pending state on completion.

- [x] **Step 6: Route all app-dependent actions through the broker**

Replace direct `writeJSON` use in final replies with a broker request. `prepareAgentRun`, built-in tools, file URL lookup, history lookup, and output sinks all receive the same process-scoped requester.

- [x] **Step 7: Run assistant tests and race detector**

```bash
cd assistant && go test ./...
go test -race ./internal/appclient ./internal/agent
```

Expected: PASS with no races.

- [x] **Step 8: Commit reliable transport**

```bash
git add assistant/internal/appclient
git commit -m "feat: add resilient assistant websocket transport"
```

### Task 4: Deduplicate retried app requests on the server

**Files:**
- Create: `server/internal/appconnection/request_cache.go`
- Create: `server/internal/appconnection/request_cache_test.go`
- Modify: `server/internal/appconnection/manager.go`
- Modify: `server/internal/appconnection/connection.go`
- Modify: `server/internal/httpserver/server_test.go`

- [x] **Step 1: Write failing request-cache tests**

Test a completed duplicate, a concurrent duplicate, request-ID conflict, ten-minute expiry, 1,000-entry eviction, and 64 MiB byte-budget eviction.

```go
func TestRequestCacheReplaysCompletedResponse(t *testing.T) {
    calls := 0
    // Execute the same app ID/request twice and require calls == 1.
}
```

- [x] **Step 2: Run request-cache tests and confirm failure**

```bash
cd server && go test ./internal/appconnection -run 'TestRequestCache'
```

Expected: cache type is undefined.

- [x] **Step 3: Implement bounded response replay**

Hash method and raw payload with SHA-256. Store running/completed entries keyed by app ID plus request ID. Concurrent duplicates wait on an entry completion channel. Completed entries expire after ten minutes and are LRU-evicted at 1,000 entries or 64 MiB.

```go
func (m *Manager) HandleRequest(appID string, request realtime.Envelope) realtime.Envelope
```

Return `request_id_conflict` if a duplicate ID has a different digest.

- [x] **Step 4: Route connection requests through the manager**

Change `Connection.handleAppMessage` to call `manager.HandleRequest`; keep the HTTP server's existing handler as the underlying executor.

- [x] **Step 5: Add side-effect integration tests**

Send duplicate `message.send`, `message.send_as_user`, group create, and add-member requests with the same ID. Assert each database mutation and emitted system message occurs once and both responses are equivalent.

- [x] **Step 6: Run server tests and race detector**

```bash
cd server && go test ./internal/appconnection ./internal/httpserver
go test -race ./internal/appconnection
```

Expected: PASS with no races.

- [x] **Step 7: Commit request deduplication**

```bash
git add server/internal/appconnection server/internal/httpserver/server_test.go
git commit -m "feat: replay duplicate app request responses"
```

### Task 5: Add durable app-event cursor replay

**Files:**
- Create: `server/migrations/00007_add_app_event_outbox.sql`
- Modify: `server/internal/store/models.go`
- Modify: `server/internal/store/migrations_test.go`
- Modify: `server/internal/httpserver/app_message_events.go`
- Modify: `server/internal/httpserver/app_request_handlers.go`
- Modify: `server/internal/httpserver/app_websocket_handlers.go`
- Modify: `server/internal/httpserver/server_test.go`
- Modify: `server/internal/realtime/protocol.go`
- Modify: `assistant/internal/appclient/client.go`
- Modify: `assistant/internal/appclient/runner.go`
- Modify: `assistant/internal/appclient/client_test.go`

- [x] **Step 1: Write the migration test and migration**

Create an `app_event_outbox` table with `id BIGSERIAL`, `app_id UUID`, event name, JSONB payload and timestamps, plus an `app_event_acks` table keyed by app ID with the last acknowledged cursor. Add indexes on `(app_id, id)`.

```sql
CREATE TABLE app_event_outbox (
    id BIGSERIAL PRIMARY KEY,
    app_id UUID NOT NULL REFERENCES apps(id) ON DELETE CASCADE,
    event VARCHAR(120) NOT NULL,
    payload JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL
);
```

- [x] **Step 2: Add cursor support to protocol envelopes**

```go
type Envelope struct {
    Cursor int64 `json:"cursor,omitempty"`
    // existing fields
}
```

Mirror the field in the assistant envelope.

- [x] **Step 3: Write failing outbox delivery tests**

Cover durable insertion before live delivery, reconnect replay after the stored ack, ordered cursor delivery, ack monotonicity, and duplicate delivery deduplication by message ID/seq.

- [x] **Step 4: Persist and deliver message events**

Replace direct `SendToApp(NewEvent(...))` with an outbox insert followed by ordered delivery. On app connection, replay rows after the stored ack before normal live delivery. Add app request method `events.ack` whose update only advances the cursor.

- [x] **Step 5: Ack accepted events from assistant**

After Message Router has accepted an event into Session Manager, send `events.ack` through the reliable requester. Keep an in-memory `(conversation ID, message ID/seq)` dedupe set so replay does not append the same instruction twice.

- [x] **Step 6: Run migration and integration tests**

```bash
cd server && go test ./internal/store ./internal/httpserver
cd ../assistant && go test ./internal/appclient
```

Expected: PASS.

- [x] **Step 7: Commit event replay**

```bash
git add server/migrations server/internal/store server/internal/httpserver server/internal/realtime assistant/internal/appclient
git commit -m "feat: replay unacknowledged app events"
```

### Task 6: Full verification and documentation sync

**Files:**
- Modify if generated: `api-docs/swagger.json`
- Modify if generated: `api-docs/swagger.yaml`
- Modify: `docs/superpowers/plans/2026-07-10-assistant-resilient-websocket.md`

- [x] **Step 1: Format all modified Go files**

```bash
gofmt -w assistant/internal server/internal
```

- [x] **Step 2: Run all backend tests**

```bash
cd assistant && go test ./...
cd ../server && go test ./...
```

Expected: PASS.

- [x] **Step 3: Run race-sensitive packages**

```bash
cd assistant && go test -race ./internal/appclient ./internal/agent
cd ../server && go test -race ./internal/appconnection
```

Expected: PASS with no race reports.

- [x] **Step 4: Run deployment verification**

```bash
./scripts/verify-deploy-config.sh
```

Expected: all assertions pass.

- [x] **Step 5: Check the final diff**

```bash
git status --short
git diff --check
git log --oneline --decorate -8
```

Expected: only intended implementation files remain and `git diff --check` is silent.

- [x] **Step 6: Commit final generated or documentation updates**

```bash
git add api-docs docs/superpowers/plans/2026-07-10-assistant-resilient-websocket.md
git commit -m "docs: sync resilient websocket implementation"
```
