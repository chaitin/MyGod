package appregistry

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"app/internal/config"
	"app/internal/store"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	AIAssistantAppID          = "00000000-0000-0000-0000-000000000001"
	AIAssistantDefaultName    = "茉莉"
	AIAssistantDefaultAvatar  = "/assets/apps/assistant.webp"
	AIAssistantDefaultSummary = "专属 AI 助理"
)

func EnsureAIAssistantApp(db *gorm.DB, cfg config.AppsConfig) (store.App, error) {
	secret := strings.TrimSpace(cfg.AIAssistantSecret)
	if secret == "" {
		return store.App{}, fmt.Errorf("apps.ai_assistant_secret is required")
	}

	now := time.Now().UTC()
	app := store.App{}
	err := db.Transaction(func(tx *gorm.DB) error {
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&app, "id = ?", AIAssistantAppID).Error
		if err == nil {
			return ensureAIAssistantAppFields(tx, &app, secret, now)
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		app = store.App{
			ID:               AIAssistantAppID,
			Name:             AIAssistantDefaultName,
			Avatar:           AIAssistantDefaultAvatar,
			Description:      AIAssistantDefaultSummary,
			Enabled:          true,
			Visibility:       store.AppVisibilityPublic,
			ConnectionSecret: secret,
			CreatedAt:        now,
			UpdatedAt:        now,
		}
		return tx.Create(&app).Error
	})
	if err != nil {
		if isUniqueConstraintError(err) {
			if findErr := db.First(&app, "id = ?", AIAssistantAppID).Error; findErr == nil {
				return app, nil
			}
		}
		return store.App{}, err
	}

	return app, nil
}

func IsAIAssistantAppID(id string) bool {
	return strings.EqualFold(strings.TrimSpace(id), AIAssistantAppID)
}

func ensureAIAssistantAppFields(db *gorm.DB, app *store.App, secret string, now time.Time) error {
	updates := map[string]any{}
	if strings.TrimSpace(app.Name) == "" {
		updates["name"] = AIAssistantDefaultName
	}
	if strings.TrimSpace(app.Avatar) == "" {
		updates["avatar"] = AIAssistantDefaultAvatar
	}
	if strings.TrimSpace(app.Description) == "" {
		updates["description"] = AIAssistantDefaultSummary
	}
	if app.CreatorUserID != nil {
		updates["creator_user_id"] = nil
	}
	if app.Visibility != store.AppVisibilityPublic {
		updates["visibility"] = store.AppVisibilityPublic
	}
	if app.ConnectionSecret != secret {
		updates["connection_secret"] = secret
	}
	if len(updates) == 0 {
		return nil
	}

	updates["updated_at"] = now
	if err := db.Model(&store.App{}).Where("id = ?", app.ID).Updates(updates).Error; err != nil {
		return err
	}
	if err := db.First(app, "id = ?", app.ID).Error; err != nil {
		return err
	}

	return nil
}

func isUniqueConstraintError(err error) bool {
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "unique") || strings.Contains(message, "duplicate")
}
