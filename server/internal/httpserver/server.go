package httpserver

import (
	"net/http"
	"os"
	"path/filepath"
	"time"

	"app/internal/config"

	"github.com/labstack/echo/v4"
	echoSwagger "github.com/swaggo/echo-swagger"
	"gorm.io/gorm"
)

const (
	adminSessionCookieName = "admin_session"
	userSessionCookieName  = "user_session"
	sessionTTL             = 7 * 24 * time.Hour
)

type Server struct {
	db  *gorm.DB
	cfg config.Config
}

func NewRouter(db *gorm.DB, cfg config.Config) *echo.Echo {
	server := &Server{
		db:  db,
		cfg: cfg,
	}

	router := echo.New()
	router.HideBanner = true
	router.HidePort = true

	router.GET("/healthz", func(c echo.Context) error {
		return success(c, http.StatusOK, map[string]any{"status": "ok"})
	})
	if docsDir, ok := findAPIDocsDir(); ok {
		router.Static("/api-docs", docsDir)
		router.GET("/swagger/*", echoSwagger.EchoWrapHandler(echoSwagger.URL("/api-docs/swagger.json")))
	}
	router.POST("/api/admin/auth/login", server.adminLogin)
	router.POST("/api/client/auth/login", server.userLogin)
	router.GET("/api/client/info", server.clientInfo)

	client := router.Group("/api/client", server.requireUserSession)
	client.POST("/conversations/groups", server.createGroupConversation)

	admin := router.Group("/api/admin", server.requireAdminSession)
	admin.GET("/settings/info", server.getInfoSettings)
	admin.PUT("/settings/info", server.updateInfoSettings)
	admin.GET("/users", server.listUsers)
	admin.POST("/users", server.createUser)
	admin.POST("/users/:id/disable", server.disableUser)
	admin.POST("/users/:id/enable", server.enableUser)
	admin.POST("/users/:id/reset-password", server.resetUserPassword)

	return router
}

func findAPIDocsDir() (string, bool) {
	dir, err := os.Getwd()
	if err != nil {
		return "", false
	}

	for {
		candidate := filepath.Join(dir, "api-docs")
		if _, err := os.Stat(filepath.Join(candidate, "swagger.json")); err == nil {
			return candidate, true
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}
