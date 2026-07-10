package appconnection

import (
	"sync"
	"time"

	"app/internal/realtime"

	"github.com/gorilla/websocket"
)

const (
	defaultSendBuffer      = 16
	defaultPingInterval    = 30 * time.Second
	defaultPongWait        = 60 * time.Second
	defaultWriteWait       = 10 * time.Second
	defaultMaxMessageBytes = 1 << 20
)

type Options struct {
	SendBuffer      int
	PingInterval    time.Duration
	PongWait        time.Duration
	WriteWait       time.Duration
	MaxMessageBytes int64
	RequestHandler  RequestHandler
}

type RequestHandler func(appID string, request realtime.Envelope) realtime.Envelope

type Manager struct {
	mu sync.RWMutex

	connsByApp map[string]map[*Connection]struct{}

	sendBuffer      int
	pingInterval    time.Duration
	pongWait        time.Duration
	writeWait       time.Duration
	maxMessageBytes int64
	requestHandler  RequestHandler
	requestCache    *requestCache
}

func NewManager(options Options) *Manager {
	sendBuffer := options.SendBuffer
	if sendBuffer <= 0 {
		sendBuffer = defaultSendBuffer
	}
	pingInterval := options.PingInterval
	if pingInterval <= 0 {
		pingInterval = defaultPingInterval
	}
	pongWait := options.PongWait
	if pongWait <= 0 {
		pongWait = defaultPongWait
	}
	writeWait := options.WriteWait
	if writeWait <= 0 {
		writeWait = defaultWriteWait
	}
	maxMessageBytes := options.MaxMessageBytes
	if maxMessageBytes <= 0 {
		maxMessageBytes = defaultMaxMessageBytes
	}

	return &Manager{
		connsByApp:      make(map[string]map[*Connection]struct{}),
		sendBuffer:      sendBuffer,
		pingInterval:    pingInterval,
		pongWait:        pongWait,
		writeWait:       writeWait,
		maxMessageBytes: maxMessageBytes,
		requestHandler:  options.RequestHandler,
		requestCache:    newRequestCache(requestCacheOptions{}),
	}
}

func (m *Manager) HandleRequest(appID string, request realtime.Envelope) realtime.Envelope {
	if m.requestHandler == nil {
		return realtime.NewErrorResponse(request.ID, "unknown_method", "未知应用方法")
	}
	return m.requestCache.Do(appID, request, func() realtime.Envelope {
		return m.requestHandler(appID, request)
	})
}

func (m *Manager) NewConnection(appID string, socket *websocket.Conn) *Connection {
	return &Connection{
		appID:   appID,
		done:    make(chan struct{}),
		manager: m,
		send:    make(chan realtime.Envelope, m.sendBuffer),
		socket:  socket,
	}
}

func (m *Manager) Register(conn *Connection) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.connsByApp[conn.appID] == nil {
		m.connsByApp[conn.appID] = make(map[*Connection]struct{})
	}
	m.connsByApp[conn.appID][conn] = struct{}{}
}

func (m *Manager) Unregister(conn *Connection) {
	m.mu.Lock()
	defer m.mu.Unlock()

	appConns := m.connsByApp[conn.appID]
	if appConns == nil {
		return
	}
	delete(appConns, conn)
	if len(appConns) == 0 {
		delete(m.connsByApp, conn.appID)
	}
}

func (m *Manager) IsOnline(appID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return len(m.connsByApp[appID]) > 0
}

func (m *Manager) SendToApp(appID string, message realtime.Envelope) int {
	connections := m.appConnections(appID)

	sent := 0
	for _, conn := range connections {
		if conn.Enqueue(message) {
			sent++
		} else {
			conn.Close()
		}
	}

	return sent
}

func (m *Manager) CloseApp(appID string) int {
	m.mu.Lock()
	appConns := m.connsByApp[appID]
	connections := make([]*Connection, 0, len(appConns))
	for conn := range appConns {
		connections = append(connections, conn)
	}
	delete(m.connsByApp, appID)
	m.mu.Unlock()

	for _, conn := range connections {
		conn.Close()
	}

	return len(connections)
}

func (m *Manager) appConnections(appID string) []*Connection {
	m.mu.RLock()
	defer m.mu.RUnlock()

	appConns := m.connsByApp[appID]
	connections := make([]*Connection, 0, len(appConns))
	for conn := range appConns {
		connections = append(connections, conn)
	}

	return connections
}
