package realtime

import (
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	defaultSendBuffer               = 16
	defaultLastOnlineUpdateInterval = time.Minute
	defaultWriteWait                = 10 * time.Second
	defaultPongWait                 = 70 * time.Second
	defaultPingInterval             = 25 * time.Second
	defaultMaxMessageBytes          = 64 * 1024
)

type Options struct {
	LastOnlineUpdateInterval time.Duration
	SendBuffer               int
	WriteWait                time.Duration
	PongWait                 time.Duration
	PingInterval             time.Duration
	MaxMessageBytes          int64
	Now                      func() time.Time
	RecordUserPong           func(userID string, at time.Time)
}

type ConnectionPool struct {
	mu sync.RWMutex

	connsByUser       map[string]map[*Connection]struct{}
	lastPongRecorded  map[string]time.Time
	lastOnlineEvery   time.Duration
	now               func() time.Time
	recordUserPong    func(userID string, at time.Time)
	defaultSendBuffer int
	writeWait         time.Duration
	pongWait          time.Duration
	pingInterval      time.Duration
	maxMessageBytes   int64
}

type Connection struct {
	ID     string
	UserID string

	closeOnce sync.Once
	closeFunc func()
	done      chan struct{}
	pool      *ConnectionPool
	send      chan Envelope
	socket    *websocket.Conn

	requestHandler RequestHandler
}

func NewConnectionPool(options Options) *ConnectionPool {
	now := options.Now
	if now == nil {
		now = func() time.Time {
			return time.Now().UTC()
		}
	}
	lastOnlineEvery := options.LastOnlineUpdateInterval
	if lastOnlineEvery == 0 {
		lastOnlineEvery = defaultLastOnlineUpdateInterval
	}
	sendBuffer := options.SendBuffer
	if sendBuffer <= 0 {
		sendBuffer = defaultSendBuffer
	}
	writeWait := options.WriteWait
	if writeWait == 0 {
		writeWait = defaultWriteWait
	}
	pongWait := options.PongWait
	if pongWait == 0 {
		pongWait = defaultPongWait
	}
	pingInterval := options.PingInterval
	if pingInterval == 0 {
		pingInterval = defaultPingInterval
	}
	maxMessageBytes := options.MaxMessageBytes
	if maxMessageBytes == 0 {
		maxMessageBytes = defaultMaxMessageBytes
	}

	return &ConnectionPool{
		connsByUser:       make(map[string]map[*Connection]struct{}),
		lastPongRecorded:  make(map[string]time.Time),
		lastOnlineEvery:   lastOnlineEvery,
		now:               now,
		recordUserPong:    options.RecordUserPong,
		defaultSendBuffer: sendBuffer,
		writeWait:         writeWait,
		pongWait:          pongWait,
		pingInterval:      pingInterval,
		maxMessageBytes:   maxMessageBytes,
	}
}

func NewConnection(id string, userID string, sendBuffer int, closeFunc func()) *Connection {
	if sendBuffer <= 0 {
		sendBuffer = defaultSendBuffer
	}

	return &Connection{
		ID:        id,
		UserID:    userID,
		closeFunc: closeFunc,
		done:      make(chan struct{}),
		send:      make(chan Envelope, sendBuffer),
	}
}

func (c *Connection) Outgoing() <-chan Envelope {
	return c.send
}

func (c *Connection) Enqueue(message Envelope) bool {
	select {
	case <-c.done:
		return false
	case c.send <- message:
		return true
	default:
		return false
	}
}

func (c *Connection) Close() {
	c.closeOnce.Do(func() {
		close(c.done)
		if c.closeFunc != nil {
			c.closeFunc()
		}
	})
}

func (p *ConnectionPool) Register(conn *Connection) bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	before := len(p.connsByUser[conn.UserID])
	if p.connsByUser[conn.UserID] == nil {
		p.connsByUser[conn.UserID] = make(map[*Connection]struct{})
	}
	p.connsByUser[conn.UserID][conn] = struct{}{}

	return before == 0 && len(p.connsByUser[conn.UserID]) == 1
}

func (p *ConnectionPool) Unregister(conn *Connection) bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	userConns := p.connsByUser[conn.UserID]
	if userConns == nil {
		return false
	}

	before := len(userConns)
	delete(userConns, conn)
	after := len(userConns)
	if after == 0 {
		delete(p.connsByUser, conn.UserID)
	}

	return before > 0 && after == 0
}

func (p *ConnectionPool) CloseUser(userID string) int {
	p.mu.Lock()
	userConns := p.connsByUser[userID]
	if len(userConns) == 0 {
		p.mu.Unlock()
		return 0
	}
	connections := make([]*Connection, 0, len(userConns))
	for conn := range userConns {
		connections = append(connections, conn)
	}
	delete(p.connsByUser, userID)
	p.mu.Unlock()

	for _, conn := range connections {
		conn.Close()
	}

	return len(connections)
}

func (p *ConnectionPool) Count(userID string) int {
	p.mu.RLock()
	defer p.mu.RUnlock()

	return len(p.connsByUser[userID])
}

func (p *ConnectionPool) IsOnline(userID string) bool {
	return p.Count(userID) > 0
}

func (p *ConnectionPool) OnlineStatus(userIDs []string) map[string]bool {
	p.mu.RLock()
	defer p.mu.RUnlock()

	status := make(map[string]bool, len(userIDs))
	for _, userID := range userIDs {
		status[userID] = len(p.connsByUser[userID]) > 0
	}

	return status
}

func (p *ConnectionPool) SendToUser(userID string, message Envelope) int {
	return sendToConnections(p.userConnections(userID), message)
}

func (p *ConnectionPool) SendToUsers(userIDs []string, message Envelope) int {
	connections := make([]*Connection, 0)
	for _, userID := range userIDs {
		connections = append(connections, p.userConnections(userID)...)
	}

	return sendToConnections(connections, message)
}

func (p *ConnectionPool) Broadcast(message Envelope) int {
	p.mu.RLock()
	connections := make([]*Connection, 0)
	for _, userConns := range p.connsByUser {
		for conn := range userConns {
			connections = append(connections, conn)
		}
	}
	p.mu.RUnlock()

	return sendToConnections(connections, message)
}

func (p *ConnectionPool) RecordPong(userID string) {
	if p.recordUserPong == nil {
		return
	}

	now := p.now()
	p.mu.Lock()
	lastRecorded := p.lastPongRecorded[userID]
	if !lastRecorded.IsZero() && now.Sub(lastRecorded) < p.lastOnlineEvery {
		p.mu.Unlock()
		return
	}
	p.lastPongRecorded[userID] = now
	p.mu.Unlock()

	p.recordUserPong(userID, now)
}

func (p *ConnectionPool) userConnections(userID string) []*Connection {
	p.mu.RLock()
	defer p.mu.RUnlock()

	userConns := p.connsByUser[userID]
	connections := make([]*Connection, 0, len(userConns))
	for conn := range userConns {
		connections = append(connections, conn)
	}

	return connections
}

func sendToConnections(connections []*Connection, message Envelope) int {
	sent := 0
	for _, conn := range connections {
		if conn.Enqueue(message) {
			sent += 1
			continue
		}
		conn.Close()
	}

	return sent
}
