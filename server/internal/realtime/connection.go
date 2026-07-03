package realtime

import (
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

type RequestHandler func(userID string, request Envelope) Envelope

func NewWebSocketConnection(userID string, socket *websocket.Conn, pool *ConnectionPool, handler RequestHandler) *Connection {
	conn := NewConnection(uuid.NewString(), userID, pool.defaultSendBuffer, func() {
		_ = socket.Close()
	})
	conn.socket = socket
	conn.pool = pool
	conn.requestHandler = handler

	return conn
}

func (c *Connection) Serve() {
	go c.writeLoop()
	c.readLoop()
}

func (c *Connection) readLoop() {
	defer c.Close()

	c.socket.SetReadLimit(c.pool.maxMessageBytes)
	_ = c.socket.SetReadDeadline(c.pool.now().Add(c.pool.pongWait))
	c.socket.SetPongHandler(func(string) error {
		_ = c.socket.SetReadDeadline(c.pool.now().Add(c.pool.pongWait))
		c.pool.RecordPong(c.UserID)
		return nil
	})

	for {
		var message Envelope
		if err := c.socket.ReadJSON(&message); err != nil {
			return
		}
		c.handleClientMessage(message)
	}
}

func (c *Connection) writeLoop() {
	ticker := time.NewTicker(c.pool.pingInterval)
	defer func() {
		ticker.Stop()
		c.Close()
	}()

	for {
		select {
		case <-c.done:
			return
		case message := <-c.send:
			if err := c.writeJSON(message); err != nil {
				return
			}
		case <-ticker.C:
			if err := c.writePing(); err != nil {
				return
			}
		}
	}
}

func (c *Connection) handleClientMessage(message Envelope) {
	if message.V != ProtocolVersion {
		c.Enqueue(NewErrorResponse(message.ID, "unsupported_version", "不支持的实时协议版本"))
		return
	}
	if message.Kind != KindRequest {
		c.Enqueue(NewErrorResponse(message.ID, "invalid_message", "实时消息类型不支持"))
		return
	}
	if message.ID == "" || message.Method == "" {
		c.Enqueue(NewErrorResponse(message.ID, "invalid_request", "实时请求格式错误"))
		return
	}
	if c.requestHandler == nil {
		c.Enqueue(NewErrorResponse(message.ID, "unknown_method", "未知实时方法"))
		return
	}

	c.Enqueue(c.requestHandler(c.UserID, message))
}

func (c *Connection) writeJSON(message Envelope) error {
	_ = c.socket.SetWriteDeadline(c.pool.now().Add(c.pool.writeWait))
	return c.socket.WriteJSON(message)
}

func (c *Connection) writePing() error {
	_ = c.socket.SetWriteDeadline(c.pool.now().Add(c.pool.writeWait))
	return c.socket.WriteMessage(websocket.PingMessage, nil)
}
