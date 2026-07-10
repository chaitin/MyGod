package appconnection

import (
	"encoding/json"
	"log"
	"sync"
	"time"

	"app/internal/realtime"

	"github.com/gorilla/websocket"
)

type Connection struct {
	appID   string
	done    chan struct{}
	manager *Manager
	send    chan realtime.Envelope
	socket  *websocket.Conn

	closeOnce sync.Once
	writeMu   sync.Mutex
}

func (c *Connection) Serve() {
	go c.writeLoop()
	c.readLoop()
}

func (c *Connection) Close() {
	c.closeOnce.Do(func() {
		close(c.done)
		_ = c.socket.Close()
	})
}

func (c *Connection) Enqueue(message realtime.Envelope) bool {
	select {
	case <-c.done:
		return false
	case c.send <- message:
		return true
	default:
		return false
	}
}

func (c *Connection) readLoop() {
	defer c.Close()

	c.socket.SetReadLimit(c.manager.maxMessageBytes)
	_ = c.socket.SetReadDeadline(time.Now().Add(c.manager.pongWait))
	c.socket.SetPongHandler(func(string) error {
		return c.socket.SetReadDeadline(time.Now().Add(c.manager.pongWait))
	})
	c.socket.SetPingHandler(func(message string) error {
		return c.writeControl(websocket.PongMessage, []byte(message))
	})

	for {
		var message realtime.Envelope
		if err := c.socket.ReadJSON(&message); err != nil {
			return
		}
		c.handleAppMessage(message)
	}
}

func (c *Connection) writeLoop() {
	ticker := time.NewTicker(c.manager.pingInterval)
	defer func() {
		ticker.Stop()
		c.Close()
	}()

	for {
		select {
		case <-c.done:
			return
		case message := <-c.send:
			limited, ok := limitOutboundEnvelope(message, c.manager.maxMessageBytes)
			if !ok {
				log.Printf("skip oversized app websocket event: app_id=%s event=%s", c.appID, message.Event)
				continue
			}
			if err := c.writeJSON(limited); err != nil {
				return
			}
		case <-ticker.C:
			if err := c.writeControl(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func limitOutboundEnvelope(message realtime.Envelope, maxMessageBytes int64) (realtime.Envelope, bool) {
	encoded, err := json.Marshal(message)
	if err == nil && int64(len(encoded)) <= maxMessageBytes {
		return message, true
	}
	if message.Kind == realtime.KindResponse {
		return realtime.NewErrorResponse(message.ReplyTo, "response_too_large", "应用响应超过 1MiB 限制"), true
	}
	return realtime.Envelope{}, false
}

func (c *Connection) writeJSON(message realtime.Envelope) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	_ = c.socket.SetWriteDeadline(time.Now().Add(c.manager.writeWait))
	return c.socket.WriteJSON(message)
}

func (c *Connection) writeControl(messageType int, data []byte) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	return c.socket.WriteControl(messageType, data, time.Now().Add(c.manager.writeWait))
}

func (c *Connection) handleAppMessage(message realtime.Envelope) {
	if message.V != realtime.ProtocolVersion {
		c.Enqueue(realtime.NewErrorResponse(message.ID, "unsupported_version", "不支持的应用协议版本"))
		return
	}
	if message.Kind != realtime.KindRequest {
		c.Enqueue(realtime.NewErrorResponse(message.ID, "invalid_message", "应用消息类型不支持"))
		return
	}
	if message.ID == "" || message.Method == "" {
		c.Enqueue(realtime.NewErrorResponse(message.ID, "invalid_request", "应用请求格式错误"))
		return
	}
	if c.manager.requestHandler == nil {
		c.Enqueue(realtime.NewErrorResponse(message.ID, "unknown_method", "未知应用方法"))
		return
	}

	c.Enqueue(c.manager.requestHandler(c.appID, message))
}
