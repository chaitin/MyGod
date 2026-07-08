package appclient

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"assistant/internal/config"

	"github.com/gorilla/websocket"
)

const (
	pingInterval        = 30 * time.Second
	pongWait            = 60 * time.Second
	writeWait           = 10 * time.Second
	maxMessageBytes     = 64 * 1024
	maxReconnectBackoff = 30 * time.Second
)

const (
	protocolVersion     = 1
	kindRequest         = "request"
	kindResponse        = "response"
	kindEvent           = "event"
	eventMessageCreated = "message.created"
	methodMessageSend   = "message.send"
)

const fallbackReplyContent = "助手服务暂时还没有接入回复能力，我先收到你的消息了"

type Client struct {
	cfg    config.Config
	dialer *websocket.Dialer
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
	Summary string          `json:"summary"`
}

type textMessageBody struct {
	Content string `json:"content"`
	Type    string `json:"type"`
}

type sendMessageRequestPayload struct {
	Target  sendMessageTarget `json:"target"`
	Message textMessageBody   `json:"message"`
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

func New(cfg config.Config) *Client {
	return &Client{
		cfg:    cfg,
		dialer: websocket.DefaultDialer,
	}
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
	return true, serveConnection(ctx, conn)
}

func serveConnection(ctx context.Context, conn *websocket.Conn) error {
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
			handleServerMessage(messageType, data, writeJSON)
		}
	}()

	ticker := time.NewTicker(pingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			_ = writeControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "shutdown"))
			return nil
		case err := <-readErr:
			return err
		case <-ticker.C:
			if err := writeControl(websocket.PingMessage, nil); err != nil {
				return err
			}
		}
	}
}

func handleServerMessage(messageType int, data []byte, writeJSON func(envelope) error) {
	if messageType != websocket.TextMessage && messageType != websocket.BinaryMessage {
		return
	}

	var message envelope
	if err := json.Unmarshal(data, &message); err != nil {
		log.Printf("ignore invalid app websocket message: %v", err)
		return
	}
	if message.V != protocolVersion {
		return
	}
	if message.Kind == kindResponse {
		if message.OK != nil && !*message.OK && message.Error != nil {
			log.Printf("app websocket request failed: reply_to=%s code=%s message=%s", message.ReplyTo, message.Error.Code, message.Error.Message)
		}
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
	var body textMessageBody
	if err := json.Unmarshal(payload.Message.Body, &body); err != nil {
		log.Printf("ignore invalid message body: %v", err)
		return
	}
	if body.Type != "text" {
		return
	}

	senderName := payload.Sender.Name
	if payload.Sender.Nickname != "" {
		senderName = payload.Sender.Nickname
	}
	log.Printf(
		"received text message from %s (%s) in conversation %s: %s",
		senderName,
		payload.Sender.ID,
		payload.Conversation.ID,
		body.Content,
	)
	if err := sendFallbackReply(writeJSON, payload.Conversation); err != nil {
		log.Printf("send fallback reply failed: %v", err)
	}
}

func sendFallbackReply(writeJSON func(envelope) error, conversation conversationPayload) error {
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
		Message: textMessageBody{
			Type:    "text",
			Content: fallbackReplyContent,
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
