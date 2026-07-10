package httpserver

import (
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"app/internal/appconnection"
	"app/internal/config"
	"app/internal/realtime"
	"app/internal/store"

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
	db             *gorm.DB
	cfg            config.Config
	appConnections *appconnection.Manager
	realtime       *realtime.ConnectionPool
	appEventMu     sync.Mutex

	beforeAppEventLock     func(store.Message)
	afterUserMessageCommit func(store.Message)
}

func NewRouter(db *gorm.DB, cfg config.Config) *echo.Echo {
	return NewRouterWithRealtimeOptions(db, cfg, realtime.Options{})
}

func NewRouterWithRealtimeOptions(db *gorm.DB, cfg config.Config, realtimeOptions realtime.Options) *echo.Echo {
	server := &Server{
		db:  db,
		cfg: cfg,
	}
	server.appConnections = appconnection.NewManager(appconnection.Options{
		RequestHandler: server.handleAppRequest,
	})
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
	router.GET("/api/app/ws", server.appWebSocket)

	client := router.Group("/api/client", server.requireUserSession)
	client.GET("/me", server.getCurrentUser)
	client.PATCH("/me", server.updateCurrentUser)
	client.POST("/me/avatar", server.uploadCurrentUserAvatar)
	client.POST("/temporary-files", server.createTemporaryFile)
	client.POST("/temporary-files/read-urls", server.readTemporaryFileURLs)
	client.GET("/contacts", server.listClientContacts)
	client.GET("/contacts/users", server.listContactUsers)
	client.GET("/projects", server.listProjects)
	client.POST("/projects", server.createProject)
	client.GET("/projects/:project_id/tasks", server.listTasks)
	client.POST("/projects/:project_id/tasks", server.createTask)
	client.GET("/projects/:project_id/tasks/:task_id", server.getTask)
	client.PATCH("/projects/:project_id/tasks/:task_id", server.updateTask)
	client.DELETE("/projects/:project_id/tasks/:task_id", server.deleteTask)
	client.GET("/projects/:project_id/groups", server.listProjectGroups)
	client.PUT("/projects/:project_id/groups/:group_id", server.bindProjectGroup)
	client.DELETE("/projects/:project_id/groups/:group_id", server.unbindProjectGroup)
	client.GET("/projects/:project_id/members", server.listProjectMembers)
	client.GET("/projects/:project_id", server.getProject)
	client.PATCH("/projects/:project_id", server.updateProject)
	client.DELETE("/projects/:project_id", server.deleteProject)
	client.GET("/conversations", server.listClientConversations)
	client.POST("/conversations/apps", server.createAppConversation)
	client.POST("/conversations/direct", server.createDirectConversation)
	client.POST("/conversations/groups", server.createGroupConversation)
	client.PATCH("/conversations/groups/:conversation_id/name", server.updateGroupConversationName)
	client.POST("/conversations/groups/:conversation_id/public", server.setGroupConversationPublic)
	client.POST("/conversations/groups/:conversation_id/private", server.setGroupConversationPrivate)
	client.POST("/conversations/groups/:conversation_id/join", server.joinPublicGroupConversation)
	client.POST("/conversations/groups/:conversation_id/leave", server.leaveGroupConversation)
	client.DELETE("/conversations/groups/:conversation_id", server.dissolveGroupConversation)
	client.DELETE("/conversations/groups/:conversation_id/members/:member_type/:member_id", server.removeTypedGroupConversationMember)
	client.DELETE("/conversations/groups/:conversation_id/members/:member_id", server.removeGroupConversationMember)
	client.POST("/conversations/:conversation_id/avatar", server.uploadGroupConversationAvatar)
	client.POST("/conversations/:conversation_id/members", server.addGroupConversationMembers)
	client.POST("/conversations/:conversation_id/read", server.markConversationRead)
	client.GET("/conversations/:conversation_id/messages", server.listConversationMessages)
	client.POST("/conversations/:conversation_id/messages", server.createConversationMessage)
	client.POST("/conversations/:conversation_id/messages/files", server.createConversationFileMessage)
	client.POST("/conversations/:conversation_id/messages/images", server.createConversationImageMessage)
	client.POST("/conversations/:conversation_id/messages/:message_id/revoke", server.revokeConversationMessage)
	client.GET("/ws", server.clientWebSocket)

	admin := router.Group("/api/admin", server.requireAdminSession)
	admin.GET("/settings/info", server.getInfoSettings)
	admin.PUT("/settings/info", server.updateInfoSettings)
	admin.GET("/apps", server.listAdminApps)
	admin.POST("/apps", server.createAdminApp)
	admin.PUT("/apps/:id", server.updateAdminApp)
	admin.POST("/apps/:id/enable", server.enableAdminApp)
	admin.POST("/apps/:id/disable", server.disableAdminApp)
	admin.POST("/apps/:id/secret/regenerate", server.regenerateAdminAppSecret)
	admin.DELETE("/apps/:id", server.deleteAdminApp)
	admin.GET("/third-party/providers", server.listThirdPartyProviders)
	admin.POST("/third-party/providers", server.createThirdPartyProvider)
	admin.PUT("/third-party/providers/:id", server.updateThirdPartyProvider)
	admin.POST("/third-party/providers/:id/enable", server.enableThirdPartyProvider)
	admin.POST("/third-party/providers/:id/disable", server.disableThirdPartyProvider)
	admin.POST("/third-party/providers/:id/move", server.moveThirdPartyProvider)
	admin.DELETE("/third-party/providers/:id", server.deleteThirdPartyProvider)
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
