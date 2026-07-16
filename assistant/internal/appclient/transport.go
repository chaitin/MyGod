package appclient

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"assistant/internal/config"

	"github.com/gorilla/websocket"
)

type dialWebSocketFunc func(context.Context, string, http.Header) (*websocket.Conn, *http.Response, error)

type webSocketManagerOptions struct {
	Dial       dialWebSocketFunc
	MaxRetries int
	Sleep      func(context.Context, time.Duration) error
}

type connectionGeneration struct {
	id   uint64
	conn *websocket.Conn
	done chan struct{}

	closeOnce sync.Once
	writeMu   sync.Mutex
}

func (g *connectionGeneration) close() {
	g.closeOnce.Do(func() {
		close(g.done)
		_ = g.conn.Close()
	})
}

type webSocketManager struct {
	cfg        config.Config
	dial       dialWebSocketFunc
	maxRetries int
	sleep      func(context.Context, time.Duration) error

	mu          sync.Mutex
	current     *connectionGeneration
	nextID      uint64
	stateChange chan struct{}
}

func newWebSocketManager(cfg config.Config, options webSocketManagerOptions) *webSocketManager {
	dial := options.Dial
	if dial == nil {
		dial = websocket.DefaultDialer.DialContext
	}
	maxRetries := options.MaxRetries
	if maxRetries <= 0 {
		maxRetries = 10
	}
	sleep := options.Sleep
	if sleep == nil {
		sleep = sleepContext
	}
	return &webSocketManager{
		cfg:         cfg,
		dial:        dial,
		maxRetries:  maxRetries,
		sleep:       sleep,
		stateChange: make(chan struct{}),
	}
}

func (m *webSocketManager) Run(ctx context.Context, handle func(envelope) bool) error {
	for attempt := 0; attempt <= m.maxRetries; attempt++ {
		conn, resp, err := m.dial(ctx, m.cfg.WebSocketURL, m.headers())
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			if resp != nil && (resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden) {
				return fmt.Errorf("%w: dial %s failed: %v, status=%d", errWebSocketAuthentication, m.cfg.WebSocketURL, err, resp.StatusCode)
			}
			if attempt == m.maxRetries {
				return fmt.Errorf("%w after %d retries: %v", errWebSocketUnavailable, m.maxRetries, err)
			}
			if err := m.sleep(ctx, retryDelay(attempt)); err != nil {
				if ctx.Err() != nil {
					return nil
				}
				return err
			}
			continue
		}

		generation := m.install(conn)
		log.Printf("app websocket connected to %s", m.cfg.WebSocketURL)
		err = m.serveGeneration(ctx, generation, handle)
		m.clear(generation)
		generation.close()
		if ctx.Err() != nil {
			return nil
		}
		if err != nil {
			log.Printf("app websocket disconnected: %v", err)
		}
		if err := m.sleep(ctx, retryDelay(0)); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		attempt = -1
	}
	return nil
}

func (m *webSocketManager) Send(ctx context.Context, message envelope) (<-chan struct{}, error) {
	encoded, err := encodeEnvelope(message)
	if err != nil {
		return nil, err
	}
	generation, err := m.waitForConnection(ctx)
	if err != nil {
		return nil, err
	}

	generation.writeMu.Lock()
	defer generation.writeMu.Unlock()
	select {
	case <-generation.done:
		return generation.done, errWebSocketUnavailable
	default:
	}
	_ = generation.conn.SetWriteDeadline(time.Now().Add(writeWait))
	if err := generation.conn.WriteMessage(websocket.TextMessage, encoded); err != nil {
		generation.close()
		m.clear(generation)
		return generation.done, err
	}
	return generation.done, nil
}

func (m *webSocketManager) headers() http.Header {
	header := http.Header{}
	header.Set("X-MagicChat-App-ID", m.cfg.AppID)
	header.Set("Authorization", "Bearer "+m.cfg.AppSecret)
	return header
}

func (m *webSocketManager) install(conn *websocket.Conn) *connectionGeneration {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nextID++
	generation := &connectionGeneration{id: m.nextID, conn: conn, done: make(chan struct{})}
	previous := m.current
	m.current = generation
	m.notifyStateChangeLocked()
	if previous != nil {
		previous.close()
	}
	return generation
}

func (m *webSocketManager) clear(generation *connectionGeneration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.current == generation {
		m.current = nil
		m.notifyStateChangeLocked()
	}
}

func (m *webSocketManager) waitForConnection(ctx context.Context) (*connectionGeneration, error) {
	for {
		m.mu.Lock()
		if m.current != nil {
			generation := m.current
			m.mu.Unlock()
			return generation, nil
		}
		changed := m.stateChange
		m.mu.Unlock()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-changed:
		}
	}
}

func (m *webSocketManager) notifyStateChangeLocked() {
	close(m.stateChange)
	m.stateChange = make(chan struct{})
}

func (m *webSocketManager) serveGeneration(ctx context.Context, generation *connectionGeneration, handle func(envelope) bool) error {
	conn := generation.conn
	conn.SetReadLimit(maxMessageBytes)
	_ = conn.SetReadDeadline(time.Now().Add(pongWait))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(pongWait))
	})
	conn.SetPingHandler(func(message string) error {
		generation.writeMu.Lock()
		defer generation.writeMu.Unlock()
		return conn.WriteControl(websocket.PongMessage, []byte(message), time.Now().Add(writeWait))
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
			if ok && handle != nil && !handle(message) {
				readErr <- errAppEventQueueFull
				return
			}
		}
	}()

	ticker := time.NewTicker(pingInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case err := <-readErr:
			return err
		case <-generation.done:
			return errWebSocketUnavailable
		case <-ticker.C:
			generation.writeMu.Lock()
			err := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(writeWait))
			generation.writeMu.Unlock()
			if err != nil {
				return err
			}
		}
	}
}
