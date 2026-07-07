package httpserver

import (
	"net/http"
	"os"
	"path/filepath"
	"time"

	"app/internal/config"
	"app/internal/realtime"

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
	db               *gorm.DB
	cfg              config.Config
	realtime         *realtime.ConnectionPool
	llmHealthChecker *LLMHealthChecker
}

func NewRouter(db *gorm.DB, cfg config.Config) *echo.Echo {
	return NewRouterWithRealtimeOptions(db, cfg, realtime.Options{})
}

func NewRouterWithRealtimeOptions(db *gorm.DB, cfg config.Config, realtimeOptions realtime.Options) *echo.Echo {
	return NewRouterWithRealtimeAndLLMHealthChecker(db, cfg, realtimeOptions, NewLLMHealthChecker(db))
}

func NewRouterWithLLMHealthChecker(db *gorm.DB, cfg config.Config, llmHealthChecker *LLMHealthChecker) *echo.Echo {
	return NewRouterWithRealtimeAndLLMHealthChecker(db, cfg, realtime.Options{}, llmHealthChecker)
}

func NewRouterWithRealtimeAndLLMHealthChecker(db *gorm.DB, cfg config.Config, realtimeOptions realtime.Options, llmHealthChecker *LLMHealthChecker) *echo.Echo {
	server := &Server{
		db:               db,
		cfg:              cfg,
		llmHealthChecker: llmHealthChecker,
	}
	realtimeOptions.RecordUserPong = server.recordUserPong
	server.realtime = realtime.NewConnectionPool(realtimeOptions)

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
	router.POST("/api/client/auth/logout", server.userLogout)
	router.GET("/api/client/auth/third-party/:key/start", server.startThirdPartyLogin)
	router.GET("/api/client/auth/third-party/:key/callback", server.finishThirdPartyLogin)
	router.GET("/api/client/info", server.clientInfo)

	client := router.Group("/api/client", server.requireUserSession)
	client.GET("/me", server.getCurrentUser)
	client.PATCH("/me", server.updateCurrentUser)
	client.POST("/me/avatar", server.uploadCurrentUserAvatar)
	client.POST("/temporary-files", server.createTemporaryFile)
	client.POST("/temporary-files/read-urls", server.readTemporaryFileURLs)
	client.GET("/contacts/users", server.listContactUsers)
	client.GET("/assistant/models", server.listClientLLMModels)
	client.GET("/conversations", server.listClientConversations)
	client.POST("/conversations/direct", server.createDirectConversation)
	client.POST("/conversations/groups", server.createGroupConversation)
	client.POST("/conversations/:conversation_id/avatar", server.uploadGroupConversationAvatar)
	client.POST("/conversations/:conversation_id/members", server.addGroupConversationMembers)
	client.POST("/conversations/:conversation_id/read", server.markConversationRead)
	client.GET("/conversations/:conversation_id/messages", server.listConversationMessages)
	client.POST("/conversations/:conversation_id/messages", server.createConversationMessage)
	client.POST("/conversations/:conversation_id/messages/files", server.createConversationFileMessage)
	client.GET("/ws", server.clientWebSocket)

	admin := router.Group("/api/admin", server.requireAdminSession)
	admin.GET("/settings/info", server.getInfoSettings)
	admin.PUT("/settings/info", server.updateInfoSettings)
	admin.GET("/third-party/providers", server.listThirdPartyProviders)
	admin.POST("/third-party/providers", server.createThirdPartyProvider)
	admin.PUT("/third-party/providers/:id", server.updateThirdPartyProvider)
	admin.POST("/third-party/providers/:id/enable", server.enableThirdPartyProvider)
	admin.POST("/third-party/providers/:id/disable", server.disableThirdPartyProvider)
	admin.POST("/third-party/providers/:id/move", server.moveThirdPartyProvider)
	admin.DELETE("/third-party/providers/:id", server.deleteThirdPartyProvider)
	admin.GET("/assistant/models", server.listLLMModels)
	admin.POST("/assistant/models", server.createLLMModel)
	admin.POST("/assistant/models/discover", server.discoverLLMModels)
	admin.PUT("/assistant/models/:id", server.updateLLMModel)
	admin.POST("/assistant/models/:id/enable", server.enableLLMModel)
	admin.POST("/assistant/models/:id/disable", server.disableLLMModel)
	admin.POST("/assistant/models/:id/move", server.moveLLMModel)
	admin.POST("/assistant/models/:id/health-check", server.checkLLMModelHealth)
	admin.DELETE("/assistant/models/:id", server.deleteLLMModel)
	admin.GET("/users", server.listUsers)
	admin.POST("/users", server.createUser)
	admin.POST("/users/:id/disable", server.disableUser)
	admin.POST("/users/:id/enable", server.enableUser)
	admin.POST("/users/:id/reset-password", server.resetUserPassword)

	return router
}

func findAPIDocsDir() (string, bool) {
	return findDirContaining("api-docs", "swagger.json")
}

func findDirContaining(relativeDir string, requiredFile string) (string, bool) {
	dir, err := os.Getwd()
	if err != nil {
		return "", false
	}

	for {
		candidate := filepath.Join(dir, relativeDir)
		statPath := candidate
		if requiredFile != "" {
			statPath = filepath.Join(candidate, requiredFile)
		}
		if _, err := os.Stat(statPath); err == nil {
			return candidate, true
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}
