package appclient

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"regexp"
	"strings"
	"sync"
	"time"

	"assistant/internal/agent"
	"assistant/internal/builtintools"
	"assistant/internal/config"
	"assistant/internal/llm"
	"assistant/internal/mcpclient"

	"github.com/gorilla/websocket"
)

const (
	pingInterval        = 30 * time.Second
	pongWait            = 60 * time.Second
	requestWait         = 30 * time.Second
	writeWait           = 10 * time.Second
	maxMessageBytes     = 1 << 20
	maxReconnectBackoff = 30 * time.Second
	maxQueuedAppEvents  = 256
)

const (
	protocolVersion         = 1
	kindRequest             = "request"
	kindResponse            = "response"
	kindEvent               = "event"
	eventMessageCreated     = "message.created"
	methodMessageSend       = "message.send"
	methodMessageSendAsUser = "message.send_as_user"

	methodConversationMessagesList = "conversation.messages.list"
	methodTemporaryFilesReadURLs   = "temporary_files.read_urls"
	methodEventsAck                = "events.ack"

	defaultConversationContextLimit = 30
)

var appMentionTokenPattern = regexp.MustCompile(`\{\(@app/([0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12})\)\}`)
var errAppEventQueueFull = errors.New("app event queue full")

type Client struct {
	cfg            config.Config
	dialer         *websocket.Dialer
	assistantAgent replyAgent
	mcpSources     []mcpclient.Source
	runner         *conversationAgentRunner
	transport      *webSocketManager
	requester      *reliableRequester

	eventMu         sync.Mutex
	eventQueue      []*queuedAppEvent
	eventCursors    map[int64]struct{}
	eventRunning    bool
	eventVersion    uint64
	lastAckedCursor int64
}

type queuedAppEvent struct {
	ctx     context.Context
	message envelope
	handled bool
}

type replyAgent interface {
	Run(ctx context.Context, request agent.Request, sink agent.OutputSink) error
}

type appRequester interface {
	Request(ctx context.Context, method string, payload any) (json.RawMessage, error)
}

type envelope struct {
	V       int             `json:"v"`
	Kind    string          `json:"kind"`
	Cursor  int64           `json:"cursor,omitempty"`
	ID      string          `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Event   string          `json:"event,omitempty"`
	ReplyTo string          `json:"reply_to,omitempty"`
	OK      *bool           `json:"ok,omitempty"`
	Payload json.RawMessage `json:"payload,omitempty"`
	Error   *errorPayload   `json:"error,omitempty"`
}

type errorPayload struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type messageCreatedPayload struct {
	Conversation conversationPayload `json:"conversation"`
	Message      messagePayload      `json:"message"`
	Sender       senderPayload       `json:"sender"`
}

type conversationPayload struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
}

type messagePayload struct {
	Body    json.RawMessage `json:"body"`
	ID      string          `json:"id"`
	Seq     int64           `json:"seq"`
	Summary string          `json:"summary"`
}

type messageBody struct {
	Content   string `json:"content"`
	FileID    string `json:"file_id"`
	Name      string `json:"name"`
	SizeBytes int64  `json:"size_bytes"`
	Type      string `json:"type"`
}

type sendMessageRequestPayload struct {
	Target  sendMessageTarget `json:"target"`
	Message messageBody       `json:"message"`
}

type sendMessageTarget struct {
	Type           string `json:"type"`
	ConversationID string `json:"conversation_id,omitempty"`
}

type senderPayload struct {
	Email    string `json:"email"`
	ID       string `json:"id"`
	Name     string `json:"name"`
	Nickname string `json:"nickname"`
	Type     string `json:"type"`
}

type appListConversationMessagesRequestPayload struct {
	BeforeOrEqualSeq int64  `json:"before_or_equal_seq"`
	ConversationID   string `json:"conversation_id"`
	Limit            int    `json:"limit"`
}

type appListConversationMessagesResponsePayload struct {
	Messages []historyMessagePayload `json:"messages"`
}

type readTemporaryFileURLsRequestPayload struct {
	FileIDs []string `json:"file_ids"`
}

type readTemporaryFileURLsResponsePayload struct {
	URLs []temporaryFileReadURLPayload `json:"urls"`
}

type temporaryFileReadURLPayload struct {
	ExpiresAt time.Time `json:"expires_at"`
	FileID    string    `json:"file_id"`
	URL       string    `json:"url"`
}

type historyMessagePayload struct {
	Body      json.RawMessage `json:"body,omitempty"`
	CreatedAt time.Time       `json:"created_at"`
	ID        string          `json:"id"`
	Seq       int64           `json:"seq"`
	Sender    senderPayload   `json:"sender"`
	Summary   string          `json:"summary"`
}

func New(ctx context.Context, cfg config.Config) (*Client, error) {
	registry, sources, err := newToolRegistry(ctx, cfg.MCP.Servers)
	if err != nil {
		return nil, err
	}

	transport := newWebSocketManager(cfg, webSocketManagerOptions{})
	return &Client{
		cfg:            cfg,
		dialer:         websocket.DefaultDialer,
		assistantAgent: agent.New(llm.NewAnthropicClient(cfg.LLM), agent.WithToolRegistry(registry), agent.WithMaxTurns(cfg.Agent.MaxTurns)),
		mcpSources:     sources,
		runner:         newConversationAgentRunner(ctx),
		transport:      transport,
		requester:      newReliableRequester(transport, reliableRequesterOptions{}),
	}, nil
}

func newToolRegistry(ctx context.Context, servers []config.MCPServerConfig) (*mcpclient.Registry, []mcpclient.Source, error) {
	mcpSources, err := mcpclient.NewSDKSources(ctx, servers)
	if err != nil {
		return nil, nil, err
	}

	sources := make([]mcpclient.Source, 0, len(mcpSources)+1)
	sources = append(sources, builtintools.NewSource())
	sources = append(sources, mcpSources...)

	registry, err := mcpclient.NewRegistry(ctx, sources)
	if err != nil {
		mcpclient.CloseSources(mcpSources)
		return nil, nil, err
	}

	return registry, mcpSources, nil
}

func (c *Client) Close() {
	if c.runner != nil {
		c.runner.CancelAll()
	}
	mcpclient.CloseSources(c.mcpSources)
}

func (c *Client) Run(ctx context.Context) error {
	for {
		if ctx.Err() != nil {
			return nil
		}
		if c.transport == nil {
			c.transport = newWebSocketManager(c.cfg, webSocketManagerOptions{})
		}
		if c.requester == nil {
			c.requester = newReliableRequester(c.transport, reliableRequesterOptions{})
		}
		err := c.transport.Run(ctx, func(message envelope) bool {
			return c.handleTransportMessage(ctx, message)
		})
		if ctx.Err() != nil {
			return nil
		}
		if errors.Is(err, errWebSocketAuthentication) {
			return err
		}
		if err != nil {
			log.Printf("app websocket connection cycle failed: %v", err)
		}
		if err := sleepContext(ctx, maxReconnectBackoff); err != nil {
			return nil
		}
	}
}

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
		if c.eventCursors == nil {
			c.eventCursors = make(map[int64]struct{})
		}
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

func (c *Client) drainAppEvents() {
	for {
		c.eventMu.Lock()
		if len(c.eventQueue) == 0 {
			c.eventRunning = false
			c.eventMu.Unlock()
			return
		}
		queued := c.eventQueue[0]
		version := c.eventVersion
		c.eventMu.Unlock()

		if !queued.handled {
			queued.handled = handleParsedServerMessage(
				queued.ctx,
				queued.message,
				c.cfg.AppID,
				c.requester,
				c.assistantAgent,
				c.runner,
				func(writeCtx context.Context, outgoing envelope) error {
					_, err := c.requester.RequestEnvelope(writeCtx, outgoing)
					return err
				},
			)
			if !queued.handled {
				if c.stopOrRetryAppEvent(version) {
					continue
				}
				return
			}
		}

		if queued.message.Cursor > 0 {
			if _, err := c.requester.Request(queued.ctx, methodEventsAck, struct {
				Cursor int64 `json:"cursor"`
			}{Cursor: queued.message.Cursor}); err != nil {
				if queued.ctx.Err() == nil {
					log.Printf("ack app event failed: cursor=%d error=%v", queued.message.Cursor, err)
				}
				if c.stopOrRetryAppEvent(version) {
					continue
				}
				return
			}
		}

		c.eventMu.Lock()
		if queued.message.Cursor > 0 {
			delete(c.eventCursors, queued.message.Cursor)
			if queued.message.Cursor > c.lastAckedCursor {
				c.lastAckedCursor = queued.message.Cursor
			}
		}
		c.eventQueue = c.eventQueue[1:]
		c.eventMu.Unlock()
	}
}

func (c *Client) stopOrRetryAppEvent(version uint64) bool {
	c.eventMu.Lock()
	defer c.eventMu.Unlock()
	if c.eventVersion != version {
		return true
	}
	c.eventRunning = false
	return false
}

type pendingResponse struct {
	ch chan envelope
}

type connectionRequester struct {
	mu      sync.Mutex
	pending map[string]pendingResponse
	write   func(envelope) error
}

func newConnectionRequester(writeJSON func(envelope) error) *connectionRequester {
	return &connectionRequester{
		pending: map[string]pendingResponse{},
		write:   writeJSON,
	}
}

func (r *connectionRequester) Request(ctx context.Context, method string, payload any) (json.RawMessage, error) {
	content, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	id := newRequestID()
	request := envelope{
		V:       protocolVersion,
		Kind:    kindRequest,
		ID:      id,
		Method:  method,
		Payload: content,
	}
	encodedRequest, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}
	if len(encodedRequest) > maxMessageBytes {
		return nil, fmt.Errorf("app websocket request exceeds 1MiB limit")
	}

	responseCh := make(chan envelope, 1)
	r.mu.Lock()
	r.pending[id] = pendingResponse{ch: responseCh}
	r.mu.Unlock()
	defer r.forget(id)

	if err := r.write(request); err != nil {
		return nil, err
	}

	requestCtx, cancel := context.WithTimeout(ctx, requestWait)
	defer cancel()
	select {
	case <-requestCtx.Done():
		return nil, requestCtx.Err()
	case response := <-responseCh:
		if response.OK != nil && !*response.OK {
			if response.Error != nil {
				return nil, fmt.Errorf("%s: %s", response.Error.Code, response.Error.Message)
			}
			return nil, fmt.Errorf("app request failed")
		}
		return response.Payload, nil
	}
}

func (r *connectionRequester) HandleResponse(response envelope) {
	r.mu.Lock()
	pending, ok := r.pending[response.ReplyTo]
	r.mu.Unlock()
	if !ok {
		if response.OK != nil && !*response.OK && response.Error != nil {
			log.Printf("app websocket request failed: reply_to=%s code=%s message=%s", response.ReplyTo, response.Error.Code, response.Error.Message)
		}
		return
	}

	select {
	case pending.ch <- response:
	default:
	}
}

func (r *connectionRequester) forget(id string) {
	r.mu.Lock()
	delete(r.pending, id)
	r.mu.Unlock()
}

func decodeServerMessage(messageType int, data []byte) (envelope, bool) {
	if messageType != websocket.TextMessage && messageType != websocket.BinaryMessage {
		return envelope{}, false
	}

	var message envelope
	if err := json.Unmarshal(data, &message); err != nil {
		log.Printf("ignore invalid app websocket message: %v", err)
		return envelope{}, false
	}
	if message.V != protocolVersion {
		return envelope{}, false
	}

	return message, true
}

func encodeEnvelope(message envelope) ([]byte, error) {
	encoded, err := json.Marshal(message)
	if err != nil {
		return nil, err
	}
	if len(encoded) > maxMessageBytes {
		return nil, fmt.Errorf("app websocket message exceeds 1MiB limit")
	}
	return encoded, nil
}

func handleServerMessage(ctx context.Context, messageType int, data []byte, requester appRequester, assistantAgent replyAgent, writeJSON func(context.Context, envelope) error) {
	message, ok := decodeServerMessage(messageType, data)
	if !ok {
		return
	}
	handleParsedServerMessage(ctx, message, "", requester, assistantAgent, directAgentRunner{}, writeJSON)
}

func handleParsedServerMessage(ctx context.Context, message envelope, appID string, requester appRequester, assistantAgent replyAgent, runner agentRunner, writeJSON func(context.Context, envelope) error) bool {
	if message.Kind == kindResponse {
		return true
	}
	if message.Kind != kindEvent || message.Event != eventMessageCreated {
		return true
	}

	var payload messageCreatedPayload
	if err := json.Unmarshal(message.Payload, &payload); err != nil {
		log.Printf("ignore invalid message.created payload: %v", err)
		return true
	}
	var body messageBody
	if err := json.Unmarshal(payload.Message.Body, &body); err != nil {
		log.Printf("ignore invalid message body: %v", err)
		return true
	}
	if !isSupportedIncomingMessageType(body.Type) {
		return true
	}
	if !shouldHandleIncomingMessage(appID, payload, body) {
		return true
	}

	senderName := payload.Sender.Name
	if payload.Sender.Nickname != "" {
		senderName = payload.Sender.Nickname
	}
	log.Printf(
		"received %s message from %s (%s) in conversation %s: %s",
		body.Type,
		senderName,
		payload.Sender.ID,
		payload.Conversation.ID,
		body.Content,
	)

	sink := agent.OutputSinkFunc(func(ctx context.Context, content string) error {
		return sendMarkdownReply(ctx, writeJSON, payload.Conversation, content)
	})
	prepared, err := prepareAgentRun(ctx, requester, payload, body, senderName)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return false
		}
		log.Printf("prepare agent run failed: %v", err)
		return sendAgentFallback(ctx, sink) == nil
	}
	return runner.Start(ctx, payload.Conversation.ID, sink, assistantAgent, prepared)
}

func prepareAgentRun(ctx context.Context, requester appRequester, payload messageCreatedPayload, body messageBody, senderName string) (preparedAgentRun, error) {
	historyMessages, err := loadConversationHistoryMessages(ctx, requester, payload)
	if err != nil {
		return preparedAgentRun{}, fmt.Errorf("load conversation history: %w", err)
	}
	fileURLs, err := readTemporaryFileURLsForMessage(ctx, requester, body)
	if err != nil {
		return preparedAgentRun{}, fmt.Errorf("read temporary file URLs: %w", err)
	}
	content, err := buildAgentMessageContent(body, fileURLs)
	if err != nil {
		return preparedAgentRun{}, fmt.Errorf("prepare agent message content: %w", err)
	}
	history, err := buildAgentHistory(payload.Message.ID, historyMessages)
	if err != nil {
		return preparedAgentRun{}, fmt.Errorf("prepare conversation history: %w", err)
	}
	authorization := authorizationForMessage(payload)

	return preparedAgentRun{
		Authorization: authorization,
		MessageSeq:    payload.Message.Seq,
		Request: agent.Request{
			AuthorizationRef: authorization.Ref,
			Conversation: agent.Conversation{
				ID:   payload.Conversation.ID,
				Name: payload.Conversation.Name,
				Type: payload.Conversation.Type,
			},
			Sender: agent.Sender{
				Email: payload.Sender.Email,
				ID:    payload.Sender.ID,
				Name:  senderName,
				Type:  payload.Sender.Type,
			},
			MessageID:   payload.Message.ID,
			Content:     content,
			CurrentTime: time.Now().UTC(),
			History:     history,
		},
		Scope: builtintools.Scope{
			ConversationID:   payload.Conversation.ID,
			ConversationType: payload.Conversation.Type,
			Requester:        requester,
		},
	}, nil
}

func authorizationForMessage(payload messageCreatedPayload) preparedAuthorization {
	if payload.Sender.Type != "user" || payload.Sender.ID == "" || payload.Message.ID == "" {
		return preparedAuthorization{}
	}
	ref := fmt.Sprintf("auth_%d", payload.Message.Seq)
	if payload.Message.Seq <= 0 {
		ref = "auth_current"
	}
	senderName := payload.Sender.Name
	if payload.Sender.Nickname != "" {
		senderName = payload.Sender.Nickname
	}
	return preparedAuthorization{
		Authorization: builtintools.Authorization{
			ActorUserID:      payload.Sender.ID,
			TriggerMessageID: payload.Message.ID,
		},
		Candidate: agent.AuthorizationCandidate{
			Ref:            ref,
			SenderID:       payload.Sender.ID,
			SenderName:     senderName,
			MessageSeq:     payload.Message.Seq,
			MessageSummary: payload.Message.Summary,
		},
		Ref: ref,
	}
}

func isSupportedIncomingMessageType(messageType string) bool {
	switch messageType {
	case "text", "markdown", "image", "file":
		return true
	default:
		return false
	}
}

func shouldHandleIncomingMessage(appID string, payload messageCreatedPayload, body messageBody) bool {
	switch payload.Conversation.Type {
	case "app":
		return true
	case "group":
		if strings.TrimSpace(appID) == "" {
			return false
		}
		switch body.Type {
		case "text", "markdown":
			return contentMentionsApp(body.Content, appID)
		default:
			return false
		}
	default:
		return false
	}
}

func contentMentionsApp(content string, appID string) bool {
	for _, match := range appMentionTokenPattern.FindAllStringSubmatch(content, -1) {
		if len(match) == 2 && strings.EqualFold(match[1], appID) {
			return true
		}
	}

	return false
}

func buildAgentMessageContent(body messageBody, fileURLs map[string]temporaryFileReadURLPayload) (string, error) {
	switch body.Type {
	case "text", "markdown":
		return body.Content, nil
	case "image":
		content := fmt.Sprintf("用户发送了一张图片。\n文件 ID：%s", body.FileID)
		if readURL, ok := temporaryFileURLForBody(body, fileURLs); ok {
			content += "\n临时访问地址：" + readURL.URL
		} else {
			content += "\n临时访问地址：未获取到"
		}
		return content, nil
	case "file":
		content := fmt.Sprintf("用户发送了一个文件。\n文件名：%s\n文件大小：%d 字节\n文件 ID：%s", body.Name, body.SizeBytes, body.FileID)
		if readURL, ok := temporaryFileURLForBody(body, fileURLs); ok {
			content += "\n临时访问地址：" + readURL.URL
		} else {
			content += "\n临时访问地址：未获取到"
		}
		return content, nil
	default:
		return "", fmt.Errorf("unsupported message type %q", body.Type)
	}
}

func readTemporaryFileURLsForMessage(ctx context.Context, requester appRequester, body messageBody) (map[string]temporaryFileReadURLPayload, error) {
	fileIDs, err := collectTemporaryFileIDs(body)
	if err != nil {
		return nil, err
	}
	return readTemporaryFileURLs(ctx, requester, fileIDs)
}

func collectTemporaryFileIDs(body messageBody) ([]string, error) {
	fileIDs := make([]string, 0)
	seen := map[string]struct{}{}
	add := func(fileID string) {
		if _, ok := seen[fileID]; ok {
			return
		}
		seen[fileID] = struct{}{}
		fileIDs = append(fileIDs, fileID)
	}
	if err := collectTemporaryFileIDFromBody(body, add); err != nil {
		return nil, err
	}

	return fileIDs, nil
}

func collectTemporaryFileIDFromBody(body messageBody, add func(string)) error {
	switch body.Type {
	case "image", "file":
		if body.FileID == "" {
			return nil
		}
		add(body.FileID)
	}
	return nil
}

func readTemporaryFileURLs(ctx context.Context, requester appRequester, fileIDs []string) (map[string]temporaryFileReadURLPayload, error) {
	if len(fileIDs) == 0 {
		return map[string]temporaryFileReadURLPayload{}, nil
	}
	urls, err := requestTemporaryFileURLs(ctx, requester, fileIDs)
	if err == nil || errors.Is(err, context.Canceled) {
		return urls, err
	}

	urls = make(map[string]temporaryFileReadURLPayload, len(fileIDs))
	for _, fileID := range fileIDs {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		singleURLs, err := requestTemporaryFileURLs(ctx, requester, []string{fileID})
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return nil, err
			}
			continue
		}
		for fileID, readURL := range singleURLs {
			urls[fileID] = readURL
		}
	}

	return urls, nil
}

func requestTemporaryFileURLs(ctx context.Context, requester appRequester, fileIDs []string) (map[string]temporaryFileReadURLPayload, error) {
	raw, err := requester.Request(ctx, methodTemporaryFilesReadURLs, readTemporaryFileURLsRequestPayload{
		FileIDs: fileIDs,
	})
	if err != nil {
		return nil, err
	}

	var response readTemporaryFileURLsResponsePayload
	if err := json.Unmarshal(raw, &response); err != nil {
		return nil, err
	}
	urls := make(map[string]temporaryFileReadURLPayload, len(response.URLs))
	for _, item := range response.URLs {
		if item.FileID != "" && item.URL != "" {
			urls[item.FileID] = item
		}
	}

	return urls, nil
}

func temporaryFileURLForBody(body messageBody, fileURLs map[string]temporaryFileReadURLPayload) (temporaryFileReadURLPayload, bool) {
	if body.FileID == "" {
		return temporaryFileReadURLPayload{}, false
	}
	readURL, ok := fileURLs[body.FileID]
	if !ok || readURL.URL == "" {
		return temporaryFileReadURLPayload{}, false
	}
	return readURL, true
}

func loadConversationHistoryMessages(ctx context.Context, requester appRequester, payload messageCreatedPayload) ([]historyMessagePayload, error) {
	raw, err := requester.Request(ctx, methodConversationMessagesList, appListConversationMessagesRequestPayload{
		BeforeOrEqualSeq: payload.Message.Seq,
		ConversationID:   payload.Conversation.ID,
		Limit:            defaultConversationContextLimit,
	})
	if err != nil {
		return nil, err
	}

	var response appListConversationMessagesResponsePayload
	if err := json.Unmarshal(raw, &response); err != nil {
		return nil, err
	}

	return response.Messages, nil
}

func buildAgentHistory(currentMessageID string, messages []historyMessagePayload) ([]agent.HistoryMessage, error) {
	history := make([]agent.HistoryMessage, 0, len(messages))
	for _, message := range messages {
		if message.ID == currentMessageID {
			continue
		}
		senderName := message.Sender.Name
		if message.Sender.Nickname != "" {
			senderName = message.Sender.Nickname
		}
		body, err := buildHistoryMessageBody(message.Body)
		if err != nil {
			return nil, fmt.Errorf("history message %s body: %w", message.ID, err)
		}
		history = append(history, agent.HistoryMessage{
			Body:       body,
			Seq:        message.Seq,
			SenderType: message.Sender.Type,
			SenderName: senderName,
			Summary:    message.Summary,
		})
	}

	return history, nil
}

func buildHistoryMessageBody(raw json.RawMessage) (json.RawMessage, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var fields map[string]any
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, err
	}
	if fields == nil {
		return nil, nil
	}

	messageType, _ := fields["type"].(string)
	switch messageType {
	case "image", "file":
		delete(fields, "url")
	}

	body, err := json.Marshal(fields)
	if err != nil {
		return nil, err
	}
	return body, nil
}

func sendMarkdownReply(ctx context.Context, writeJSON func(context.Context, envelope) error, conversation conversationPayload, content string) error {
	targetType := conversation.Type
	switch targetType {
	case "app", "group":
	default:
		return nil
	}

	payload, err := json.Marshal(sendMessageRequestPayload{
		Target: sendMessageTarget{
			Type:           targetType,
			ConversationID: conversation.ID,
		},
		Message: messageBody{
			Type:    "markdown",
			Content: content,
		},
	})
	if err != nil {
		return err
	}

	return writeJSON(ctx, envelope{
		V:       protocolVersion,
		Kind:    kindRequest,
		ID:      newRequestID(),
		Method:  methodMessageSend,
		Payload: payload,
	})
}

func newRequestID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return fmt.Sprintf("assistant-%d", time.Now().UnixNano())
	}

	return "assistant-" + hex.EncodeToString(value[:])
}
