package httpserver

import (
	"context"
	"crypto/subtle"
	"net/http"
	"strings"

	appapp "app/internal/application/app"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/labstack/echo/v4"
)

const (
	appIDHeader = "X-MagicChat-App-ID"
)

var appWebSocketUpgrader = websocket.Upgrader{
	CheckOrigin: func(*http.Request) bool {
		return true
	},
}

// appWebSocket godoc
//
// @Summary 应用 WebSocket 连接
// @Description 应用使用 App ID 和连接密钥连接，连接存在且心跳正常时视为在线。
// @Tags 应用连接
// @Param X-MagicChat-App-ID header string true "应用 ID"
// @Param Authorization header string true "Bearer 连接密钥"
// @Success 101
// @Failure 400 {object} errorEnvelope
// @Failure 401 {object} errorEnvelope
// @Failure 403 {object} errorEnvelope
// @Failure 500 {object} errorEnvelope
// @Router /api/app/ws [get]
func (s *Server) appWebSocket(c echo.Context) error {
	appID := strings.TrimSpace(c.Request().Header.Get(appIDHeader))
	if _, err := uuid.Parse(appID); err != nil {
		return failure(c, http.StatusBadRequest, "invalid_request", "应用 ID 格式错误")
	}

	app, ok, err := s.findAppForConnection(appID)
	if err != nil {
		return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
	}
	if !ok || !validAppBearer(c.Request().Header.Get("Authorization"), app.ConnectionSecret) {
		return failure(c, http.StatusUnauthorized, "unauthorized", "应用认证失败")
	}
	if !app.Enabled {
		return failure(c, http.StatusForbidden, "forbidden", "应用已禁用")
	}

	socket, err := appWebSocketUpgrader.Upgrade(c.Response().Writer, c.Request(), nil)
	if err != nil {
		return err
	}

	s.appEventMu.Lock()
	conn := s.appConnections.NewConnection(app.ID, socket)
	s.appConnections.Register(conn)
	serveDone := make(chan struct{})
	go func() {
		conn.Serve()
		close(serveDone)
	}()
	replayErr := s.replayAppEvents(app.ID, conn)
	s.appEventMu.Unlock()
	if replayErr != nil {
		s.appConnections.Unregister(conn)
		conn.Close()
		<-serveDone
		return replayErr
	}
	<-serveDone
	s.appConnections.Unregister(conn)

	return nil
}

func (s *Server) findAppForConnection(appID string) (appapp.App, bool, error) {
	value, err := s.apps.GetForConnection(context.Background(), appID)
	if appapp.ErrorCodeOf(err) == appapp.CodeNotFound {
		return appapp.App{}, false, nil
	}
	if err != nil {
		return appapp.App{}, false, err
	}
	return value, true, nil
}

func validAppBearer(header string, secret string) bool {
	auth := strings.TrimSpace(header)
	prefix := "Bearer "
	if !strings.HasPrefix(auth, prefix) {
		return false
	}
	token := strings.TrimSpace(strings.TrimPrefix(auth, prefix))
	expected := strings.TrimSpace(secret)
	if token == "" || expected == "" || len(token) != len(expected) {
		return false
	}

	return subtle.ConstantTimeCompare([]byte(token), []byte(expected)) == 1
}
