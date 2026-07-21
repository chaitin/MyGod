package message

import (
	"app/internal/config"

	"gorm.io/gorm"
)

type Dependencies struct {
	DB                     *gorm.DB
	Bodies                 BodyProcessor
	ForwardBodies          ForwardBodySanitizer
	Files                  TemporaryFileValidator
	TaskNotificationBodies TaskNotificationBodyBuilder
	TaskReminderBodies     TaskReminderBodyBuilder
	Apps                   config.AppsConfig
	Notifications          NotificationPort
	ReactionNotifications  ReactionNotificationPort
	AppEvents              AppEventPort
	AppEventLocker         AppEventLocker
	BeforeAppEventLock     func(Message)
	AfterUserMessageCommit func(Message)
}

type Service struct {
	db                     *gorm.DB
	bodies                 BodyProcessor
	forwardBodies          ForwardBodySanitizer
	files                  TemporaryFileValidator
	taskNotificationBodies TaskNotificationBodyBuilder
	taskReminderBodies     TaskReminderBodyBuilder
	apps                   config.AppsConfig
	notifications          NotificationPort
	reactionNotifications  ReactionNotificationPort
	appEvents              AppEventPort
	appEventLocker         AppEventLocker
	beforeAppEventLock     func(Message)
	afterUserMessageCommit func(Message)
}

func NewService(deps Dependencies) *Service {
	return &Service{
		db: deps.DB, bodies: deps.Bodies, forwardBodies: deps.ForwardBodies, files: deps.Files,
		taskNotificationBodies: deps.TaskNotificationBodies, taskReminderBodies: deps.TaskReminderBodies,
		apps: deps.Apps, notifications: deps.Notifications,
		reactionNotifications: deps.ReactionNotifications,
		appEvents: deps.AppEvents, appEventLocker: deps.AppEventLocker,
		beforeAppEventLock:     deps.BeforeAppEventLock,
		afterUserMessageCommit: deps.AfterUserMessageCommit,
	}
}

var _ ClientService = (*Service)(nil)
var _ AppService = (*Service)(nil)
