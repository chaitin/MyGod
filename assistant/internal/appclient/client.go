package appclient

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
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
	maxMessageBytes     = 64 * 1024
	maxReconnectBackoff = 30 * time.Second
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

	defaultConversationContextLimit = 30
)

type Client struct {
	cfg            config.Config
	dialer         *websocket.Dialer
	assistantAgent replyAgent
	mcpSources     []mcpclient.Source
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
	ConversationID string   `json:"conversation_id"`
	FileIDs        []string `json:"file_ids"`
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
	CreatedAt time.Time     `json:"created_at"`
	ID        string        `json:"id"`
	Seq       int64         `json:"seq"`
	Sender    senderPayload `json:"sender"`
	Summary   string        `json:"summary"`
}

func New(ctx context.Context, cfg config.Config) (*Client, error) {
	registry, sources, err := newToolRegistry(ctx, cfg.MCP.Servers)
	if err != nil {
		return nil, err
	}

	return &Client{
		cfg:            cfg,
		dialer:         websocket.DefaultDialer,
		assistantAgent: agent.New(llm.NewAnthropicClient(cfg.LLM), agent.WithToolRegistry(registry), agent.WithMaxTurns(cfg.Agent.MaxTurns)),
		mcpSources:     sources,
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
	mcpclient.CloseSources(c.mcpSources)
}

func (c *Client) Run(ctx context.Context) error {
	backoff := time.Second
	for {
		if err := ctx.Err(); err != nil {
			return nil
		}

		connected, err := c.connectOnce(ctx)
		if connected {
			backoff = time.Second
		}
		if err != nil && ctx.Err() == nil {
			log.Printf("app websocket disconnected: %v", err)
		}

		select {
		case <-ctx.Done():
			return nil
		case <-time.After(backoff):
		}

		backoff *= 2
		if backoff > maxReconnectBackoff {
			backoff = maxReconnectBackoff
		}
	}
}

func (c *Client) connectOnce(ctx context.Context) (bool, error) {
	header := http.Header{}
	header.Set("X-MyGod-App-ID", c.cfg.AppID)
	header.Set("Authorization", "Bearer "+c.cfg.AppSecret)

	conn, resp, err := c.dialer.DialContext(ctx, c.cfg.WebSocketURL, header)
	if err != nil {
		status := 0
		if resp != nil {
			status = resp.StatusCode
		}

		return false, fmt.Errorf("dial %s failed: %w, status=%d", c.cfg.WebSocketURL, err, status)
	}
	defer conn.Close()

	log.Printf("app websocket connected to %s", c.cfg.WebSocketURL)
	return true, serveConnection(ctx, conn, c.assistantAgent)
}

func serveConnection(ctx context.Context, conn *websocket.Conn, assistantAgent replyAgent) error {
	connCtx, cancelConnection := context.WithCancel(ctx)
	defer cancelConnection()

	var writeMu sync.Mutex
	writeJSON := func(message envelope) error {
		writeMu.Lock()
		defer writeMu.Unlock()

		_ = conn.SetWriteDeadline(time.Now().Add(writeWait))
		return conn.WriteJSON(message)
	}
	writeControl := func(messageType int, data []byte) error {
		writeMu.Lock()
		defer writeMu.Unlock()

		return conn.WriteControl(messageType, data, time.Now().Add(writeWait))
	}
	requester := newConnectionRequester(writeJSON)
	runner := newUserAgentRunner()
	defer runner.CancelAll()

	conn.SetReadLimit(maxMessageBytes)
	_ = conn.SetReadDeadline(time.Now().Add(pongWait))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(pongWait))
	})
	conn.SetPingHandler(func(message string) error {
		return writeControl(websocket.PongMessage, []byte(message))
	})

	readErr := make(chan error, 1)
	go func() {
		for {
			messageType, data, err := conn.ReadMessage()
			if err != nil {
				readErr <- err
				return
			}
			message, ok := decodeServerMessage(messageType, data)
			if !ok {
				continue
			}
			if message.Kind == kindResponse {
				requester.HandleResponse(message)
				continue
			}
			go handleParsedServerMessage(connCtx, message, requester, assistantAgent, runner, writeJSON)
		}
	}()

	ticker := time.NewTicker(pingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-connCtx.Done():
			_ = writeControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "shutdown"))
			return nil
		case err := <-readErr:
			cancelConnection()
			return err
		case <-ticker.C:
			if err := writeControl(websocket.PingMessage, nil); err != nil {
				return err
			}
		}
	}
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
	responseCh := make(chan envelope, 1)
	r.mu.Lock()
	r.pending[id] = pendingResponse{ch: responseCh}
	r.mu.Unlock()
	defer r.forget(id)

	if err := r.write(envelope{
		V:       protocolVersion,
		Kind:    kindRequest,
		ID:      id,
		Method:  method,
		Payload: content,
	}); err != nil {
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

func handleServerMessage(ctx context.Context, messageType int, data []byte, requester appRequester, assistantAgent replyAgent, writeJSON func(envelope) error) {
	message, ok := decodeServerMessage(messageType, data)
	if !ok {
		return
	}
	handleParsedServerMessage(ctx, message, requester, assistantAgent, directAgentRunner{}, writeJSON)
}

func handleParsedServerMessage(ctx context.Context, message envelope, requester appRequester, assistantAgent replyAgent, runner agentRunner, writeJSON func(envelope) error) {
	if message.Kind == kindResponse {
		return
	}
	if message.Kind != kindEvent || message.Event != eventMessageCreated {
		return
	}

	var payload messageCreatedPayload
	if err := json.Unmarshal(message.Payload, &payload); err != nil {
		log.Printf("ignore invalid message.created payload: %v", err)
		return
	}
	var body messageBody
	if err := json.Unmarshal(payload.Message.Body, &body); err != nil {
		log.Printf("ignore invalid message body: %v", err)
		return
	}
	if !isSupportedIncomingMessageType(body.Type) {
		return
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
		return sendMarkdownReply(writeJSON, payload.Conversation, content)
	})
	runner.Start(ctx, userAgentKey(payload.Sender), sink, func(ctx context.Context, sink agent.OutputSink) error {
		content, err := buildAgentMessageContent(ctx, requester, payload.Conversation.ID, body)
		if err != nil {
			if !errors.Is(err, context.Canceled) {
				log.Printf("prepare agent message content failed: %v", err)
			}
			return err
		}
		history, err := loadConversationHistory(ctx, requester, payload)
		if err != nil {
			if !errors.Is(err, context.Canceled) {
				log.Printf("load conversation history failed: %v", err)
			}
			return err
		}
		agentCtx := builtintools.WithScope(ctx, builtintools.Scope{
			ConversationID:   payload.Conversation.ID,
			ConversationType: payload.Conversation.Type,
			CurrentUserID:    payload.Sender.ID,
			Requester:        requester,
			TriggerMessageID: payload.Message.ID,
		})
		err = assistantAgent.Run(agentCtx, agent.Request{
			Conversation: agent.Conversation{
				ID:   payload.Conversation.ID,
				Name: payload.Conversation.Name,
				Type: payload.Conversation.Type,
			},
			Sender: agent.Sender{
				ID:   payload.Sender.ID,
				Name: senderName,
				Type: payload.Sender.Type,
			},
			MessageID: payload.Message.ID,
			Content:   content,
			History:   history,
		}, sink)
		if err != nil && !errors.Is(err, context.Canceled) {
			log.Printf("agent reply failed: %v", err)
		}
		return err
	})
}

func isSupportedIncomingMessageType(messageType string) bool {
	switch messageType {
	case "text", "markdown", "image", "file":
		return true
	default:
		return false
	}
}

func buildAgentMessageContent(ctx context.Context, requester appRequester, conversationID string, body messageBody) (string, error) {
	switch body.Type {
	case "text", "markdown":
		return body.Content, nil
	case "image":
		readURL, err := readTemporaryFileURL(ctx, requester, conversationID, body.FileID)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("用户发送了一张图片。\n文件 ID：%s\n临时访问地址：%s", body.FileID, readURL.URL), nil
	case "file":
		readURL, err := readTemporaryFileURL(ctx, requester, conversationID, body.FileID)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("用户发送了一个文件。\n文件名：%s\n文件大小：%d 字节\n文件 ID：%s\n临时访问地址：%s", body.Name, body.SizeBytes, body.FileID, readURL.URL), nil
	default:
		return "", fmt.Errorf("unsupported message type %q", body.Type)
	}
}

func readTemporaryFileURL(ctx context.Context, requester appRequester, conversationID string, fileID string) (temporaryFileReadURLPayload, error) {
	if fileID == "" {
		return temporaryFileReadURLPayload{}, fmt.Errorf("file_id is required")
	}
	raw, err := requester.Request(ctx, methodTemporaryFilesReadURLs, readTemporaryFileURLsRequestPayload{
		ConversationID: conversationID,
		FileIDs:        []string{fileID},
	})
	if err != nil {
		return temporaryFileReadURLPayload{}, err
	}

	var response readTemporaryFileURLsResponsePayload
	if err := json.Unmarshal(raw, &response); err != nil {
		return temporaryFileReadURLPayload{}, err
	}
	for _, item := range response.URLs {
		if item.FileID == fileID && item.URL != "" {
			return item, nil
		}
	}

	return temporaryFileReadURLPayload{}, fmt.Errorf("temporary file read URL not found for %s", fileID)
}

func userAgentKey(sender senderPayload) string {
	if sender.ID != "" {
		return sender.ID
	}
	if sender.Type != "" || sender.Name != "" {
		return sender.Type + ":" + sender.Name
	}
	return "unknown"
}

func loadConversationHistory(ctx context.Context, requester appRequester, payload messageCreatedPayload) ([]agent.HistoryMessage, error) {
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

	history := make([]agent.HistoryMessage, 0, len(response.Messages))
	for _, message := range response.Messages {
		if message.ID == payload.Message.ID {
			continue
		}
		senderName := message.Sender.Name
		if message.Sender.Nickname != "" {
			senderName = message.Sender.Nickname
		}
		history = append(history, agent.HistoryMessage{
			Seq:        message.Seq,
			SenderType: message.Sender.Type,
			SenderName: senderName,
			Summary:    message.Summary,
		})
	}

	return history, nil
}

func sendMarkdownReply(writeJSON func(envelope) error, conversation conversationPayload, content string) error {
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

	return writeJSON(envelope{
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
