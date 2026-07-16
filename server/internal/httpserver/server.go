package httpserver

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	adminapi "app/internal/api/http/admin"
	clientapi "app/internal/api/http/client"
	"app/internal/appconnection"
	"app/internal/application/account"
	"app/internal/application/adminauth"
	appapp "app/internal/application/app"
	contactapp "app/internal/application/contact"
	conversationapp "app/internal/application/conversation"
	entitycardapp "app/internal/application/entitycard"
	externalauthapp "app/internal/application/externalauth"
	fileapp "app/internal/application/file"
	"app/internal/application/identityprovider"
	messageapp "app/internal/application/message"
	messagecontentapp "app/internal/application/messagecontent"
	projectapp "app/internal/application/project"
	settingsapp "app/internal/application/settings"
	taskapp "app/internal/application/task"
	"app/internal/application/usermanagement"
	"app/internal/config"
	externalauthinfra "app/internal/infrastructure/externalauth"
	"app/internal/infrastructure/filestorage"
	"app/internal/realtime"
	"app/internal/store"

	"github.com/labstack/echo/v4"
	echoSwagger "github.com/swaggo/echo-swagger"
	"gorm.io/gorm"
)

type Server struct {
	db                  *gorm.DB
	cfg                 config.Config
	accounts            *account.Service
	clientAccounts      *clientapi.AccountAPI
	adminAuth           *adminauth.Service
	adminAuthAPI        *adminapi.AuthAPI
	userManagement      *usermanagement.Service
	adminUsers          *adminapi.UserAPI
	identityProviders   *identityprovider.Service
	adminProviders      *adminapi.IdentityProviderAPI
	externalAuth        *externalauthapp.Service
	clientExternalAuth  *clientapi.ExternalAuthAPI
	apps                *appapp.Service
	adminApps           *adminapi.AppAPI
	files               *fileapp.Service
	clientFiles         *clientapi.FileAPI
	contacts            *contactapp.Service
	clientContacts      *clientapi.ContactAPI
	conversations       *conversationapp.Service
	clientConversations *clientapi.ConversationAPI
	messages            *messageapp.Service
	messageContents     *messagecontentapp.Service
	clientMessages      *clientapi.MessageAPI
	settings            *settingsapp.Service
	clientInfo          *clientapi.InfoAPI
	adminSettings       *adminapi.SettingsAPI
	projects            *projectapp.Service
	clientProjects      *clientapi.ProjectAPI
	entityCards         entitycardapp.Resolver
	tasks               *taskapp.Service
	clientTasks         *clientapi.TaskAPI
	appConnections      *appconnection.Manager
	realtime            *realtime.ConnectionPool
	appEventMu          sync.Mutex

	beforeAppEventLock     func(store.Message)
	afterUserMessageCommit func(store.Message)
}

func NewRouter(db *gorm.DB, cfg config.Config) *echo.Echo {
	return NewRouterWithRealtimeOptions(db, cfg, realtime.Options{})
}

func NewRouterWithRealtimeOptions(db *gorm.DB, cfg config.Config, realtimeOptions realtime.Options) *echo.Echo {
	return newRouter(db, cfg, realtimeOptions, nil)
}

func NewRouterWithTaskReminderWorker(ctx context.Context, db *gorm.DB, cfg config.Config) *echo.Echo {
	return newRouter(db, cfg, realtime.Options{}, ctx)
}

func newRouter(db *gorm.DB, cfg config.Config, realtimeOptions realtime.Options, workerContext context.Context) *echo.Echo {
	server := &Server{
		db:  db,
		cfg: cfg,
	}
	server.files = fileapp.NewService(fileapp.Dependencies{
		DB:                  db,
		Storage:             filestorage.New(cfg.Storage),
		TemporaryExpireDays: cfg.Storage.Lifecycle.TemporaryExpireDays,
	})
	server.clientFiles = clientapi.NewFileAPI(server.files)
	server.adminAuth = adminauth.NewService(adminauth.Dependencies{DB: db, Password: cfg.Admin.Password})
	server.adminAuthAPI = adminapi.NewAuthAPI(server.adminAuth, server.adminAuth)
	server.appConnections = appconnection.NewManager(appconnection.Options{
		RequestHandler: server.handleAppRequest,
	})
	server.apps = appapp.NewService(appapp.Dependencies{
		DB: db, Apps: cfg.Apps, Files: server.files, Connections: server.appConnections,
	})
	server.adminApps = adminapi.NewAppAPI(server.apps)
	server.settings = settingsapp.NewService(settingsapp.Dependencies{DB: db})
	server.adminSettings = adminapi.NewSettingsAPI(server.settings)
	server.accounts = account.NewService(account.Dependencies{
		DB:    db,
		Files: server.files,
	})
	server.clientAccounts = clientapi.NewAccountAPI(
		server.accounts,
		server.accounts,
		func(c echo.Context, session account.AuthenticatedSession) {
			c.Set(currentUserContextKey, legacyUserFromAccount(session.Account))
		},
	)
	server.clientInfo = clientapi.NewInfoAPI(server.settings, server.accounts)
	server.projects = projectapp.NewService(projectapp.Dependencies{
		DB:    db,
		Files: server.files,
	})
	server.clientProjects = clientapi.NewProjectAPI(server.projects)
	server.entityCards = entitycardapp.NewService(entitycardapp.Dependencies{
		DB: db, Projects: server.projects,
	})
	server.conversations = conversationapp.NewService(conversationapp.Dependencies{
		DB:            db,
		Apps:          cfg.Apps,
		Files:         server.files,
		Projects:      server.projects,
		Notifications: server,
	})
	server.clientConversations = clientapi.NewConversationAPI(server.conversations, server.projects)
	server.tasks = taskapp.NewService(taskapp.Dependencies{
		DB:            db,
		Notifications: server,
	})
	server.clientTasks = clientapi.NewTaskAPI(server.tasks)
	realtimeOptions.RecordUserPong = server.recordUserPong
	server.realtime = realtime.NewConnectionPool(realtimeOptions)
	server.userManagement = usermanagement.NewService(usermanagement.Dependencies{
		DB: db, Presence: server.realtime,
	})
	server.adminUsers = adminapi.NewUserAPI(server.userManagement)
	server.identityProviders = identityprovider.NewService(identityprovider.Dependencies{DB: db})
	server.adminProviders = adminapi.NewIdentityProviderAPI(server.identityProviders, cfg.Server.ClientHostname)
	server.externalAuth = externalauthapp.NewService(externalauthapp.Dependencies{
		DB: db, Providers: server.identityProviders, OAuth: externalauthinfra.NewOAuth(),
	})
	server.clientExternalAuth = clientapi.NewExternalAuthAPI(server.externalAuth)
	server.contacts = contactapp.NewService(contactapp.Dependencies{
		DB: db, Apps: cfg.Apps, UserPresence: server.realtime, AppPresence: server.appConnections,
	})
	server.clientContacts = clientapi.NewContactAPI(server.contacts)
	server.messageContents = messagecontentapp.NewService(messagecontentapp.Dependencies{
		EntityCards: server.entityCards,
		FetchLinkTitle: func(ctx context.Context, linkURL string) (string, error) {
			return fetchLinkPreviewTitle(ctx, linkURL)
		},
	})
	server.messages = messageapp.NewService(messageapp.Dependencies{
		DB: db, Bodies: server.messageContents,
		ForwardBodies: server.messageContents, Files: server.files,
		TaskNotificationBodies: server.messageContents, Apps: cfg.Apps,
		TaskReminderBodies: server.messageContents,
		Notifications:      server, AppEvents: server, AppEventLocker: &server.appEventMu,
		BeforeAppEventLock: func(message messageapp.Message) {
			if server.beforeAppEventLock != nil {
				server.beforeAppEventLock(legacyStoredMessage(message))
			}
		},
		AfterUserMessageCommit: func(message messageapp.Message) {
			if server.afterUserMessageCommit != nil {
				server.afterUserMessageCommit(legacyStoredMessage(message))
			}
		},
	})
	server.clientMessages = clientapi.NewMessageAPI(server.messages, server.files)

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
	server.adminAuthAPI.RegisterPublicRoutes(router)
	server.clientAccounts.RegisterPublicRoutes(router)
	server.clientExternalAuth.RegisterPublicRoutes(router)
	server.clientInfo.RegisterPublicRoutes(router)
	router.GET("/api/app/ws", server.appWebSocket)

	client := router.Group("/api/client", server.clientAccounts.RequireSession)
	server.clientAccounts.RegisterProtectedRoutes(client)
	server.clientFiles.RegisterRoutes(client)
	server.clientProjects.RegisterRoutes(client)
	server.clientTasks.RegisterRoutes(client)
	server.clientConversations.RegisterRoutes(client)
	server.clientContacts.RegisterRoutes(client)
	server.clientMessages.RegisterRoutes(client)
	client.GET("/ws", server.clientWebSocket)

	admin := router.Group("/api/admin", server.adminAuthAPI.RequireSession)
	server.adminSettings.RegisterRoutes(admin)
	server.adminApps.RegisterRoutes(admin)
	server.adminUsers.RegisterRoutes(admin)
	server.adminProviders.RegisterRoutes(admin)
	if workerContext != nil {
		go server.tasks.RunReminderWorker(workerContext)
	}
	return router
}

func legacyUserFromAccount(value account.Account) store.User {
	var phone *string
	if value.Phone != "" {
		phoneValue := value.Phone
		phone = &phoneValue
	}
	return store.User{
		ID:           value.ID,
		Email:        value.Email,
		Name:         value.Name,
		Nickname:     value.Nickname,
		Phone:        phone,
		Avatar:       value.Avatar,
		Status:       value.Status,
		LastOnlineAt: value.LastOnlineAt,
		CreatedAt:    value.CreatedAt,
		UpdatedAt:    value.UpdatedAt,
	}
}

func (s *Server) messageContentService() *messagecontentapp.Service {
	if s.messageContents != nil {
		return s.messageContents
	}
	return messagecontentapp.NewService(messagecontentapp.Dependencies{
		EntityCards: s.entityCards,
		FetchLinkTitle: func(ctx context.Context, linkURL string) (string, error) {
			return fetchLinkPreviewTitle(ctx, linkURL)
		},
	})
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
