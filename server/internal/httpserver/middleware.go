package httpserver

import (
	"app/internal/store"

	"github.com/labstack/echo/v4"
)

const currentUserContextKey = "current_user"

func currentUser(c echo.Context) (store.User, bool) {
	user, ok := c.Get(currentUserContextKey).(store.User)
	return user, ok
}
