package conversation

import (
	"strings"
	"time"

	fileapp "app/internal/application/file"
	"app/internal/config"
	"app/internal/store"

	"gorm.io/gorm"
)

type Dependencies struct {
	DB            *gorm.DB
	Apps          config.AppsConfig
	Files         fileapp.PublicUploader
	Projects      ProjectReader
	Notifications NotificationPort
	Now           func() time.Time
}

type Service struct {
	db            *gorm.DB
	apps          config.AppsConfig
	files         fileapp.PublicUploader
	projects      ProjectReader
	notifications NotificationPort
	now           func() time.Time
}

func NewService(deps Dependencies) *Service {
	now := deps.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &Service{
		db: deps.DB, apps: deps.Apps, files: deps.Files, projects: deps.Projects,
		notifications: deps.Notifications, now: now,
	}
}

func actorUser(actor Actor) store.User {
	var phone *string
	if actor.Phone != "" {
		value := actor.Phone
		phone = &value
	}
	return store.User{
		ID: strings.TrimSpace(actor.ID), Email: actor.Email, Name: actor.Name,
		Nickname: actor.Nickname, Phone: phone, Avatar: actor.Avatar,
	}
}

func appStoreValue(app AppIdentity) store.App {
	return store.App{ID: strings.TrimSpace(app.ID), Name: app.Name, Avatar: app.Avatar}
}

func newReference(conversation store.Conversation) Reference {
	return Reference{ID: conversation.ID, Name: conversation.Name, Type: conversation.Kind}
}

func newMessage(message store.Message) Message {
	senderID := ""
	if message.SenderID != nil {
		senderID = *message.SenderID
	}
	clientMessageID := ""
	if message.ClientMessageID != nil {
		clientMessageID = *message.ClientMessageID
	}
	replyToMessageID := ""
	if message.ReplyToMessageID != nil {
		replyToMessageID = *message.ReplyToMessageID
	}
	revokedByUserID := ""
	if message.RevokedByUserID != nil {
		revokedByUserID = *message.RevokedByUserID
	}
	var delegatedBy *MessageIdentity
	if message.DelegatedByType != nil && message.DelegatedByID != nil {
		delegatedBy = &MessageIdentity{ID: *message.DelegatedByID, Name: message.DelegatedByName, Type: *message.DelegatedByType}
	}
	return Message{
		ClientMessageID: clientMessageID, Body: message.Body, ConversationID: message.ConversationID,
		CreatedAt: message.CreatedAt, DelegatedBy: delegatedBy, ID: message.ID,
		ReplyToMessageID: replyToMessageID, RevokedAt: message.RevokedAt,
		RevokedByUserID: revokedByUserID, Sender: MessageIdentity{ID: senderID, Type: message.SenderType},
		Seq: message.Seq, Summary: message.Summary,
	}
}

func newOptionalMessage(message *store.Message) *Message {
	if message == nil {
		return nil
	}
	converted := newMessage(*message)
	return &converted
}

var _ ClientService = (*Service)(nil)
var _ AppService = (*Service)(nil)
