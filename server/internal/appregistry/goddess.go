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
	GoddessAppID          = "00000000-0000-0000-0000-000000000001"
	GoddessDefaultName    = "女菩萨"
	GoddessDefaultAvatar  = "/assets/apps/goddess.webp"
	GoddessDefaultSummary = "专属 AI 助理"
)

func EnsureGoddessApp(db *gorm.DB, cfg config.AppsConfig) (store.App, error) {
	secret := strings.TrimSpace(cfg.GoddessSecret)
	if secret == "" {
		return store.App{}, fmt.Errorf("apps.goddess_secret is required")
	}

	now := time.Now().UTC()
	app := store.App{}
	err := db.Transaction(func(tx *gorm.DB) error {
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&app, "id = ?", GoddessAppID).Error
		if err == nil {
			return ensureGoddessAppFields(tx, &app, secret, now)
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		app = store.App{
			ID:             GoddessAppID,
			Name:           GoddessDefaultName,
			Avatar:         GoddessDefaultAvatar,
			Description:    GoddessDefaultSummary,
			Enabled:        true,
			Visibility:     store.AppVisibilityPublic,
			CallbackSecret: secret,
			CreatedAt:      now,
			UpdatedAt:      now,
		}
		return tx.Create(&app).Error
	})
	if err != nil {
		if isUniqueConstraintError(err) {
			if findErr := db.First(&app, "id = ?", GoddessAppID).Error; findErr == nil {
				return app, nil
			}
		}
		return store.App{}, err
	}

	return app, nil
}

func IsGoddessAppID(id string) bool {
	return strings.EqualFold(strings.TrimSpace(id), GoddessAppID)
}

func ensureGoddessAppFields(db *gorm.DB, app *store.App, secret string, now time.Time) error {
	updates := map[string]any{}
	if strings.TrimSpace(app.Name) == "" {
		updates["name"] = GoddessDefaultName
	}
	if strings.TrimSpace(app.Avatar) == "" {
		updates["avatar"] = GoddessDefaultAvatar
	}
	if strings.TrimSpace(app.Description) == "" {
		updates["description"] = GoddessDefaultSummary
	}
	if app.CreatorUserID != nil {
		updates["creator_user_id"] = nil
	}
	if app.Visibility != store.AppVisibilityPublic {
		updates["visibility"] = store.AppVisibilityPublic
	}
	if app.CallbackSecret != secret {
		updates["callback_secret"] = secret
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
