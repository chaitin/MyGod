package contact

import (
	"strings"

	"app/internal/config"

	"gorm.io/gorm"
)

type Dependencies struct {
	DB           *gorm.DB
	Apps         config.AppsConfig
	UserPresence UserPresencePort
	AppPresence  AppPresencePort
}

type Service struct {
	db           *gorm.DB
	apps         config.AppsConfig
	userPresence UserPresencePort
	appPresence  AppPresencePort
}

func NewService(deps Dependencies) *Service {
	return &Service{
		db: deps.DB, apps: deps.Apps,
		userPresence: deps.UserPresence, appPresence: deps.AppPresence,
	}
}

func normalizeKeyword(keyword string) string {
	return strings.ToLower(strings.TrimSpace(keyword))
}

var _ ClientService = (*Service)(nil)
var _ AppService = (*Service)(nil)
