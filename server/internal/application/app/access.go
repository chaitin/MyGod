package app

import (
	"context"
	"strings"

	"app/internal/store"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	activeCreatorSQL   = "apps.creator_user_id IS NULL OR EXISTS (SELECT 1 FROM users WHERE users.id = apps.creator_user_id AND users.status = ?)"
	userGrantExistsSQL = "EXISTS (SELECT 1 FROM app_user_grants aug WHERE aug.app_id = apps.id AND aug.user_id = ?)"
)

// ApplyUsableScope limits an apps query to enabled applications whose creator,
// when present, is still active.
func ApplyUsableScope(query *gorm.DB) *gorm.DB {
	return query.
		Where("apps.enabled = ?", true).
		Where("("+activeCreatorSQL+")", store.UserStatusActive)
}

// ApplyUserAccessScope limits an apps query to usable applications the user
// may discover and use. The creator always keeps access regardless of the
// selected visibility.
func ApplyUserAccessScope(query *gorm.DB, userID string) *gorm.DB {
	userID = strings.TrimSpace(userID)
	return ApplyUsableScope(query).Where(
		"visibility = ? OR creator_user_id = ? OR (visibility = ? AND "+userGrantExistsSQL+")",
		store.AppVisibilityPublic,
		userID,
		store.AppVisibilityRestricted,
		userID,
	)
}

// ApplyPublicAccessScope limits an apps query to usable public applications.
func ApplyPublicAccessScope(query *gorm.DB) *gorm.DB {
	return ApplyUsableScope(query).Where("apps.visibility = ?", store.AppVisibilityPublic)
}

// LockUsableApp acquires a shared row lock and returns an application only if
// it is currently usable. The shared lock serializes the caller with
// authorization, status, ownership-status and deletion changes.
func LockUsableApp(db *gorm.DB, appID string) (store.App, error) {
	return lockApp(ApplyUsableScope(appLockQuery(db, []string{strings.TrimSpace(appID)})))
}

// LockUserAccessibleApp is LockUsableApp plus the user's visibility grant.
func LockUserAccessibleApp(db *gorm.DB, appID string, userID string) (store.App, error) {
	query := ApplyUserAccessScope(appLockQuery(db, []string{strings.TrimSpace(appID)}), userID)
	return lockApp(query)
}

// LockUserAccessibleApps acquires shared row locks in stable ID order and
// returns all usable applications the user may discover and use directly.
func LockUserAccessibleApps(db *gorm.DB, appIDs []string, userID string) ([]store.App, error) {
	if len(appIDs) == 0 {
		return nil, nil
	}
	return findLockedApps(ApplyUserAccessScope(appLockQuery(db, appIDs), userID))
}

// LockUsableApps acquires shared row locks in stable ID order and returns all
// currently usable applications from appIDs.
func LockUsableApps(db *gorm.DB, appIDs []string) ([]store.App, error) {
	if len(appIDs) == 0 {
		return nil, nil
	}
	return findLockedApps(ApplyUsableScope(appLockQuery(db, appIDs)))
}

// LockPublicApps acquires shared row locks in stable ID order and returns all
// currently usable public applications from appIDs.
func LockPublicApps(db *gorm.DB, appIDs []string) ([]store.App, error) {
	if len(appIDs) == 0 {
		return nil, nil
	}
	return findLockedApps(ApplyPublicAccessScope(appLockQuery(db, appIDs)))
}

func appLockQuery(db *gorm.DB, appIDs []string) *gorm.DB {
	return db.Model(&store.App{}).
		Clauses(clause.Locking{Strength: "SHARE"}).
		Where("apps.id IN ?", appIDs)
}

func lockApp(query *gorm.DB) (store.App, error) {
	var value store.App
	if err := query.Order("apps.id ASC").First(&value).Error; err != nil {
		return store.App{}, err
	}
	return value, nil
}

func findLockedApps(query *gorm.DB) ([]store.App, error) {
	var values []store.App
	if err := query.Order("apps.id ASC").Find(&values).Error; err != nil {
		return nil, err
	}
	return values, nil
}

func lockAppForUpdate(db *gorm.DB, appID string) (store.App, error) {
	var value store.App
	if err := db.Clauses(clause.Locking{Strength: "UPDATE"}).
		First(&value, "id = ?", strings.TrimSpace(appID)).Error; err != nil {
		return store.App{}, err
	}
	return value, nil
}

func lockOwnedAppForUpdate(db *gorm.DB, appID string, accountID string) (store.App, error) {
	var value store.App
	if err := db.Clauses(clause.Locking{Strength: "UPDATE"}).
		First(&value, "id = ? AND creator_user_id = ?", strings.TrimSpace(appID), strings.TrimSpace(accountID)).Error; err != nil {
		return store.App{}, err
	}
	return value, nil
}

func creatorIsActive(db *gorm.DB, value store.App) (bool, error) {
	if value.CreatorUserID == nil {
		return true, nil
	}
	var count int64
	if err := db.Model(&store.User{}).
		Where("id = ? AND status = ?", *value.CreatorUserID, store.UserStatusActive).
		Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func (s *Service) CanUserAccess(ctx context.Context, appID string, userID string) (bool, error) {
	appID = strings.TrimSpace(appID)
	userID = strings.TrimSpace(userID)
	if appID == "" || userID == "" {
		return false, nil
	}
	var count int64
	query := ApplyUserAccessScope(s.db.WithContext(ctx).Model(&store.App{}), userID).
		Where("apps.id = ?", appID)
	if err := query.Count(&count).Error; err != nil {
		return false, internalError(err)
	}
	return count > 0, nil
}
