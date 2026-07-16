package conversation

import (
	"context"
	"errors"

	"app/internal/appregistry"
	"app/internal/store"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func (s *Service) CreateApp(ctx context.Context, cmd CreateAppCommand) (OpenResult, error) {
	current := actorUser(cmd.Actor)
	appID, err := normalizeUUID(cmd.AppID, "应用 ID 格式错误")
	if err != nil {
		return OpenResult{}, invalidRequest(err.Error(), err)
	}
	db := s.db
	app, ok, err := s.findVisibleApp(db, appID, current.ID)
	if err != nil {
		return OpenResult{}, internalError(err)
	}
	if !ok {
		return OpenResult{}, notFound("应用不存在", gorm.ErrRecordNotFound)
	}
	conversation, created, err := s.getOrCreateApp(db, current, app)
	if err != nil {
		return OpenResult{}, internalError(err)
	}
	item := newItem(
		conversation, current.ID,
		[]store.ConversationMember{
			{ConversationID: conversation.ID, MemberType: store.ConversationMemberTypeUser, MemberID: current.ID},
			{ConversationID: conversation.ID, MemberType: store.ConversationMemberTypeApp, MemberID: app.ID},
		},
		map[string]store.User{current.ID: current}, map[string]store.App{app.ID: app},
	)
	return OpenResult{Conversation: item, Created: created}, nil
}

func (s *Service) OpenAppForUser(ctx context.Context, current Identity, app AppIdentity) (Reference, bool, error) {
	conversation, created, err := s.getOrCreateApp(s.db, actorUser(current), appStoreValue(app))
	if err != nil {
		return Reference{}, false, err
	}
	return newReference(conversation), created, nil
}

func (s *Service) findVisibleApp(db *gorm.DB, appID, currentUserID string) (store.App, bool, error) {
	if appregistry.IsAIAssistantAppID(appID) {
		if _, err := appregistry.EnsureAIAssistantApp(db, s.apps); err != nil {
			return store.App{}, false, err
		}
	}
	var app store.App
	err := db.Where("id = ? AND enabled = ?", appID, true).
		Where("visibility = ? OR (visibility = ? AND creator_user_id = ?)", store.AppVisibilityPublic, store.AppVisibilityCreator, currentUserID).
		First(&app).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return store.App{}, false, nil
	}
	if err != nil {
		return store.App{}, false, err
	}
	return app, true, nil
}

func (s *Service) getOrCreateApp(db *gorm.DB, current store.User, app store.App) (store.Conversation, bool, error) {
	existing, err := findAppByUser(db, app.ID, current.ID)
	if err == nil {
		return existing, false, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return store.Conversation{}, false, err
	}
	now := s.now().UTC()
	conversation := store.Conversation{ID: uuid.NewString(), Kind: store.ConversationKindApp, Name: app.Name, Avatar: app.Avatar, CreatedByUserID: current.ID, Status: store.ConversationStatusActive, PostingPolicy: store.ConversationPostingPolicyOpen, Visibility: store.ConversationVisibilityPrivate, CreatedAt: now, UpdatedAt: now}
	created := false
	err = db.Transaction(func(tx *gorm.DB) error {
		existing, err := findAppByUser(tx, app.ID, current.ID)
		if err == nil {
			conversation, created = existing, false
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err := tx.Create(&conversation).Error; err != nil {
			return err
		}
		members := []store.ConversationMember{
			{ConversationID: conversation.ID, MemberType: store.ConversationMemberTypeUser, MemberID: current.ID, Role: store.ConversationMemberRoleOwner, JoinedAt: now, HistoryVisibleFromSeq: 1},
			{ConversationID: conversation.ID, MemberType: store.ConversationMemberTypeApp, MemberID: app.ID, Role: store.ConversationMemberRoleMember, JoinedAt: now, HistoryVisibleFromSeq: 1},
		}
		if err := tx.Create(&members).Error; err != nil {
			return err
		}
		if err := tx.Create(&store.AppConversation{AppID: app.ID, UserID: current.ID, ConversationID: conversation.ID, CreatedAt: now}).Error; err != nil {
			return err
		}
		created = true
		return nil
	})
	if err != nil {
		if isUniqueConstraintError(err) {
			if existing, findErr := findAppByUser(db, app.ID, current.ID); findErr == nil {
				return existing, false, nil
			}
		}
		return store.Conversation{}, false, err
	}
	return conversation, created, nil
}

func findAppByUser(db *gorm.DB, appID, userID string) (store.Conversation, error) {
	var relation store.AppConversation
	if err := db.First(&relation, "app_id = ? AND user_id = ?", appID, userID).Error; err != nil {
		return store.Conversation{}, err
	}
	var conversation store.Conversation
	if err := db.First(&conversation, "id = ?", relation.ConversationID).Error; err != nil {
		return store.Conversation{}, err
	}
	return conversation, nil
}
