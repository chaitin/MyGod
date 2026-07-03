package httpserver

import (
	"net/http"
	"net/url"
	"strings"
	"time"

	"app/internal/realtime"
	"app/internal/store"

	"github.com/gorilla/websocket"
	"github.com/labstack/echo/v4"
)

var clientWebSocketUpgrader = websocket.Upgrader{
	CheckOrigin: allowClientWebSocketOrigin,
}

func (s *Server) clientWebSocket(c echo.Context) error {
	user, ok := currentUser(c)
	if !ok {
		return failure(c, http.StatusUnauthorized, "unauthorized", "未登录")
	}

	socket, err := clientWebSocketUpgrader.Upgrade(c.Response().Writer, c.Request(), nil)
	if err != nil {
		return err
	}

	conn := realtime.NewWebSocketConnection(user.ID, socket, s.realtime, s.handleRealtimeRequest)
	s.realtime.Register(conn)
	conn.Enqueue(realtime.NewEvent(realtime.EventSystemReady, map[string]any{}))

	conn.Serve()

	s.realtime.Unregister(conn)

	return nil
}

func (s *Server) handleRealtimeRequest(_ string, request realtime.Envelope) realtime.Envelope {
	return realtime.NewErrorResponse(request.ID, "unknown_method", "未知实时方法")
}

func (s *Server) recordUserPong(userID string, at time.Time) {
	_ = s.db.Model(&store.User{}).
		Where("id = ?", userID).
		Update("last_online_at", at.UTC()).
		Error
}

func formatOptionalTime(value *time.Time) *string {
	if value == nil {
		return nil
	}

	formatted := value.UTC().Format(time.RFC3339)
	return &formatted
}

func allowClientWebSocketOrigin(r *http.Request) bool {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return true
	}

	originURL, err := url.Parse(origin)
	if err != nil {
		return false
	}
	if strings.EqualFold(originURL.Host, r.Host) {
		return true
	}

	return isLocalhost(originURL.Hostname()) && isLocalhost(hostnameWithoutPort(r.Host))
}

func isLocalhost(host string) bool {
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

func hostnameWithoutPort(host string) string {
	if parsedURL, err := url.Parse("//" + host); err == nil {
		return parsedURL.Hostname()
	}

	return host
}
