package conversationaccess

import (
	"errors"

	appapp "app/internal/application/app"
	"app/internal/store"

	"gorm.io/gorm"
)

var ErrDirectAppAccessDenied = errors.New("direct app access denied")

func DirectAppConversationID(value Context) (string, bool) {
	if value.Conversation.Kind == store.ConversationKindApp {
		return value.Conversation.ID, true
	}
	if value.ParentConversation != nil && value.ParentConversation.Kind == store.ConversationKindApp {
		return value.ParentConversation.ID, true
	}
	return "", false
}

func RequireUserDirectAppAccess(db *gorm.DB, value Context, userID string) error {
	conversationID, ok := DirectAppConversationID(value)
	if !ok {
		return nil
	}
	var relation store.AppConversation
	if err := db.Select("app_id", "user_id").First(
		&relation,
		"conversation_id = ? AND user_id = ?",
		conversationID,
		userID,
	).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrDirectAppAccessDenied
		}
		return err
	}
	if _, err := appapp.LockUserAccessibleApp(db, relation.AppID, relation.UserID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrDirectAppAccessDenied
		}
		return err
	}
	return nil
}

func RequireAppDirectUserAccess(db *gorm.DB, value Context, appID string) error {
	conversationID, ok := DirectAppConversationID(value)
	if !ok {
		return nil
	}
	var relation store.AppConversation
	if err := db.Select("app_id", "user_id").First(
		&relation,
		"conversation_id = ? AND app_id = ?",
		conversationID,
		appID,
	).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrDirectAppAccessDenied
		}
		return err
	}
	if _, err := appapp.LockUserAccessibleApp(db, relation.AppID, relation.UserID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrDirectAppAccessDenied
		}
		return err
	}
	return nil
}
