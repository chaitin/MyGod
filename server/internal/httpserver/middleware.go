package httpserver

import (
	"errors"
	"net/http"
	"time"

	"app/internal/auth"
	"app/internal/store"

	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
)

const currentUserContextKey = "current_user"

func (s *Server) requireAdminSession(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		cookie, err := c.Cookie(adminSessionCookieName)
		if err != nil || cookie.Value == "" {
			return failure(c, http.StatusUnauthorized, "unauthorized", "未登录")
		}

		var session store.AdminSession
		err = s.db.Where(
			"token_hash = ? AND expires_at > ?",
			auth.HashSessionToken(cookie.Value),
			time.Now().UTC(),
		).First(&session).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return failure(c, http.StatusUnauthorized, "unauthorized", "未登录")
		}
		if err != nil {
			return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
		}

		_ = s.db.Model(&session).Update("last_seen_at", time.Now().UTC()).Error
		return next(c)
	}
}

func (s *Server) requireUserSession(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		cookie, err := c.Cookie(userSessionCookieName)
		if err != nil || cookie.Value == "" {
			return failure(c, http.StatusUnauthorized, "unauthorized", "未登录")
		}

		var session store.UserSession
		err = s.db.Preload("User").Where(
			"token_hash = ? AND expires_at > ?",
			auth.HashSessionToken(cookie.Value),
			time.Now().UTC(),
		).First(&session).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return failure(c, http.StatusUnauthorized, "unauthorized", "未登录")
		}
		if err != nil {
			return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
		}
		if session.User.Status != store.UserStatusActive {
			return failure(c, http.StatusUnauthorized, "unauthorized", "未登录")
		}

		_ = s.db.Model(&session).Update("last_seen_at", time.Now().UTC()).Error
		c.Set(currentUserContextKey, session.User)
		return next(c)
	}
}

func currentUser(c echo.Context) (store.User, bool) {
	user, ok := c.Get(currentUserContextKey).(store.User)
	return user, ok
}

func setSessionCookie(c echo.Context, name string, token string, expiresAt time.Time) {
	c.SetCookie(&http.Cookie{
		Name:     name,
		Value:    token,
		Path:     "/",
		Expires:  expiresAt,
		MaxAge:   int(time.Until(expiresAt).Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}
