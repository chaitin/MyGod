package app

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"app/internal/store"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const maxGrantedUsersPerApp = 500

func (s *Service) ListOwned(ctx context.Context, accountID string) ([]App, error) {
	accountID, err := normalizeAccountID(accountID)
	if err != nil {
		return nil, err
	}
	var values []store.App
	if err := s.db.WithContext(ctx).
		Where("creator_user_id = ?", accountID).
		Order("updated_at DESC").Order("id DESC").
		Find(&values).Error; err != nil {
		return nil, internalError(err)
	}
	grants, err := loadAppGrantIDs(s.db.WithContext(ctx), appIDs(values))
	if err != nil {
		return nil, internalError(err)
	}
	result := make([]App, 0, len(values))
	for _, value := range values {
		item := s.newApp(value)
		item.GrantedUserIDs = grants[value.ID]
		result = append(result, item)
	}
	return result, nil
}

func (s *Service) GetOwned(ctx context.Context, cmd OwnedAppCommand) (App, error) {
	value, err := s.findOwned(ctx, cmd.AccountID, cmd.AppID)
	if err != nil {
		return App{}, err
	}
	return s.newOwnedApp(ctx, value)
}

func (s *Service) CreateOwned(ctx context.Context, cmd CreateOwnedCommand) (App, error) {
	accountID, err := normalizeAccountID(cmd.AccountID)
	if err != nil {
		return App{}, err
	}
	name, description, visibility, err := normalizeOwnedDetails(cmd.Name, cmd.Description, cmd.Visibility)
	if err != nil {
		return App{}, err
	}
	var userIDs []string
	if visibility == store.AppVisibilityRestricted {
		userIDs, err = s.normalizeGrantedUserIDs(ctx, accountID, cmd.UserIDs)
		if err != nil {
			return App{}, err
		}
	}
	secret, err := s.generateUniqueSecret(ctx)
	if err != nil {
		return App{}, internalError(err)
	}
	now := s.now().UTC()
	creatorID := accountID
	stored := store.App{
		ID: s.newID(), Name: name, Description: description, CreatorUserID: &creatorID,
		Enabled: true, Visibility: visibility, ConnectionSecret: secret,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var owner store.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Select("id", "status").First(&owner, "id = ?", accountID).Error; err != nil {
			return err
		}
		if owner.Status != store.UserStatusActive {
			return newError(CodeForbidden, "用户不可用", nil)
		}
		var count int64
		if err := tx.Model(&store.App{}).Where("creator_user_id = ?", accountID).Count(&count).Error; err != nil {
			return err
		}
		if count >= MaxOwnedAppsPerAccount {
			return newError(CodeInvalidRequest, "每个用户最多创建 20 个应用", nil)
		}
		if err := tx.Create(&stored).Error; err != nil {
			return err
		}
		return replaceAppGrants(tx, stored.ID, accountID, userIDs, now)
	}); err != nil {
		if ErrorCodeOf(err) != CodeInternal {
			return App{}, err
		}
		return App{}, internalError(err)
	}
	result := s.newApp(stored)
	result.GrantedUserIDs = userIDs
	return result, nil
}

func (s *Service) UpdateOwned(ctx context.Context, cmd UpdateOwnedCommand) (App, error) {
	accountID, err := normalizeAccountID(cmd.AccountID)
	if err != nil {
		return App{}, err
	}
	appID, err := normalizeAppID(cmd.AppID)
	if err != nil {
		return App{}, err
	}
	updates := map[string]any{}
	if cmd.Name != nil {
		name := strings.TrimSpace(*cmd.Name)
		if name == "" {
			return App{}, newError(CodeInvalidRequest, "应用名称不能为空", nil)
		}
		if len([]rune(name)) > maxAppNameLength {
			return App{}, newError(CodeInvalidRequest, "应用名称不能超过 120 个字符", nil)
		}
		updates["name"] = name
	}
	if cmd.Description != nil {
		description, descriptionErr := normalizeDescription(*cmd.Description)
		if descriptionErr != nil {
			return App{}, descriptionErr
		}
		updates["description"] = description
	}
	var requestedVisibility *string
	if cmd.Visibility != nil {
		visibility, visibilityErr := normalizeOwnedVisibility(*cmd.Visibility)
		if visibilityErr != nil {
			return App{}, visibilityErr
		}
		requestedVisibility = &visibility
	}
	now := s.now().UTC()
	authorizationChanged := cmd.Visibility != nil || cmd.UserIDs != nil
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		stored, err := lockOwnedAppForUpdate(tx, appID, accountID)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return newError(CodeNotFound, "应用不存在", err)
		}
		if err != nil {
			return err
		}
		visibility := stored.Visibility
		if requestedVisibility != nil {
			visibility = *requestedVisibility
			updates["visibility"] = visibility
		}
		var userIDs []string
		if cmd.UserIDs != nil && visibility == store.AppVisibilityRestricted {
			userIDs, err = s.normalizeGrantedUserIDsWithDB(tx, accountID, *cmd.UserIDs)
			if err != nil {
				return err
			}
		}
		if len(updates) > 0 || cmd.UserIDs != nil {
			updates["updated_at"] = now
		}
		if len(updates) > 0 {
			if err := tx.Model(&store.App{}).Where("id = ?", appID).Updates(updates).Error; err != nil {
				return err
			}
		}
		switch {
		case visibility != store.AppVisibilityRestricted:
			if err := tx.Where("app_id = ?", appID).Delete(&store.AppUserGrant{}).Error; err != nil {
				return err
			}
		case cmd.UserIDs != nil:
			if err := replaceAppGrants(tx, appID, accountID, userIDs, now); err != nil {
				return err
			}
		}
		if authorizationChanged {
			grantedUserIDs := userIDs
			if visibility == store.AppVisibilityRestricted && cmd.UserIDs == nil {
				grants, loadErr := loadAppGrantIDs(tx, []string{appID})
				if loadErr != nil {
					return loadErr
				}
				grantedUserIDs = grants[appID]
			}
			return revokeUnauthorizedDirectAppEvents(tx, appID, accountID, visibility, grantedUserIDs)
		}
		return nil
	}); err != nil {
		if ErrorCodeOf(err) != CodeInternal {
			return App{}, err
		}
		return App{}, internalError(err)
	}
	if authorizationChanged && s.connections != nil {
		s.connections.CloseApp(appID)
	}
	return s.reloadOwned(ctx, accountID, appID)
}

func (s *Service) SetOwnedEnabled(ctx context.Context, cmd SetOwnedEnabledCommand) (App, error) {
	accountID, err := normalizeAccountID(cmd.AccountID)
	if err != nil {
		return App{}, err
	}
	appID, err := normalizeAppID(cmd.AppID)
	if err != nil {
		return App{}, err
	}
	changed := false
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		stored, err := lockOwnedAppForUpdate(tx, appID, accountID)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return newError(CodeNotFound, "应用不存在", err)
		}
		if err != nil {
			return err
		}
		if cmd.Enabled {
			active, err := creatorIsActive(tx, stored)
			if err != nil {
				return err
			}
			if !active {
				return newError(CodeForbidden, "应用创建者不可用", nil)
			}
		}
		if stored.Enabled == cmd.Enabled {
			return nil
		}
		changed = true
		return tx.Model(&stored).Updates(map[string]any{
			"enabled": cmd.Enabled, "updated_at": s.now().UTC(),
		}).Error
	}); err != nil {
		if ErrorCodeOf(err) != CodeInternal {
			return App{}, err
		}
		return App{}, internalError(err)
	}
	if changed && !cmd.Enabled && s.connections != nil {
		s.connections.CloseApp(appID)
	}
	return s.reloadOwned(ctx, accountID, appID)
}

func (s *Service) RegenerateOwnedSecret(ctx context.Context, cmd OwnedAppCommand) (App, error) {
	stored, err := s.findOwned(ctx, cmd.AccountID, cmd.AppID)
	if err != nil {
		return App{}, err
	}
	secret, err := s.generateUniqueSecret(ctx)
	if err != nil {
		return App{}, internalError(err)
	}
	if err := s.db.WithContext(ctx).Model(&stored).Updates(map[string]any{
		"connection_secret": secret, "updated_at": s.now().UTC(),
	}).Error; err != nil {
		return App{}, internalError(err)
	}
	if s.connections != nil {
		s.connections.CloseApp(stored.ID)
	}
	return s.reloadOwned(ctx, strings.TrimSpace(cmd.AccountID), stored.ID)
}

func (s *Service) DeleteOwned(ctx context.Context, cmd OwnedAppCommand) error {
	stored, err := s.findOwned(ctx, cmd.AccountID, cmd.AppID)
	if err != nil {
		return err
	}
	now := s.now().UTC()
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return deleteStoredApp(tx, &stored, now)
	}); err != nil {
		return internalError(err)
	}
	if s.connections != nil {
		s.connections.CloseApp(stored.ID)
	}
	return nil
}

func (s *Service) UploadOwnedAvatar(ctx context.Context, cmd UploadOwnedAvatarCommand) (App, error) {
	stored, err := s.findOwned(ctx, cmd.AccountID, cmd.AppID)
	if err != nil {
		return App{}, err
	}
	if _, err := s.uploadAvatar(ctx, stored, cmd.Content, cmd.Size); err != nil {
		return App{}, err
	}
	return s.reloadOwned(ctx, strings.TrimSpace(cmd.AccountID), stored.ID)
}

func (s *Service) findOwned(ctx context.Context, rawAccountID string, rawAppID string) (store.App, error) {
	accountID, err := normalizeAccountID(rawAccountID)
	if err != nil {
		return store.App{}, err
	}
	appID, err := normalizeAppID(rawAppID)
	if err != nil {
		return store.App{}, err
	}
	var stored store.App
	err = s.db.WithContext(ctx).First(&stored, "id = ? AND creator_user_id = ?", appID, accountID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return store.App{}, newError(CodeNotFound, "应用不存在", err)
	}
	if err != nil {
		return store.App{}, internalError(err)
	}
	return stored, nil
}

func (s *Service) reloadOwned(ctx context.Context, accountID string, appID string) (App, error) {
	stored, err := s.findOwned(ctx, accountID, appID)
	if err != nil {
		return App{}, err
	}
	return s.newOwnedApp(ctx, stored)
}

func (s *Service) newOwnedApp(ctx context.Context, stored store.App) (App, error) {
	grants, err := loadAppGrantIDs(s.db.WithContext(ctx), []string{stored.ID})
	if err != nil {
		return App{}, internalError(err)
	}
	result := s.newApp(stored)
	result.GrantedUserIDs = grants[stored.ID]
	return result, nil
}

func (s *Service) normalizeGrantedUserIDs(ctx context.Context, ownerID string, raw []string) ([]string, error) {
	return s.normalizeGrantedUserIDsWithDB(s.db.WithContext(ctx), ownerID, raw)
}

func (s *Service) normalizeGrantedUserIDsWithDB(db *gorm.DB, ownerID string, raw []string) ([]string, error) {
	if len(raw) > maxGrantedUsersPerApp {
		return nil, newError(CodeInvalidRequest, "单个应用最多授权 500 名用户", nil)
	}
	seen := make(map[string]struct{}, len(raw))
	ids := make([]string, 0, len(raw))
	for _, value := range raw {
		id := strings.TrimSpace(value)
		if id == ownerID {
			continue
		}
		if _, err := uuid.Parse(id); err != nil {
			return nil, newError(CodeInvalidRequest, "授权用户 ID 格式错误", err)
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	sort.Strings(ids)
	if len(ids) == 0 {
		return ids, nil
	}
	var count int64
	if err := db.Model(&store.User{}).
		Where("id IN ? AND status = ?", ids, store.UserStatusActive).
		Count(&count).Error; err != nil {
		return nil, internalError(err)
	}
	if count != int64(len(ids)) {
		return nil, newError(CodeInvalidRequest, "授权用户不存在或不可用", nil)
	}
	return ids, nil
}

func replaceAppGrants(tx *gorm.DB, appID string, grantedBy string, userIDs []string, now time.Time) error {
	if err := tx.Where("app_id = ?", appID).Delete(&store.AppUserGrant{}).Error; err != nil {
		return err
	}
	if len(userIDs) == 0 {
		return nil
	}
	grants := make([]store.AppUserGrant, 0, len(userIDs))
	for _, userID := range userIDs {
		granter := grantedBy
		grants = append(grants, store.AppUserGrant{
			AppID: appID, UserID: userID, GrantedByUserID: &granter, CreatedAt: now,
		})
	}
	return tx.Create(&grants).Error
}

func loadAppGrantIDs(db *gorm.DB, ids []string) (map[string][]string, error) {
	result := make(map[string][]string, len(ids))
	if len(ids) == 0 {
		return result, nil
	}
	var grants []store.AppUserGrant
	if err := db.Select("app_id", "user_id").Where("app_id IN ?", ids).
		Order("app_id ASC").Order("user_id ASC").Find(&grants).Error; err != nil {
		return nil, err
	}
	for _, grant := range grants {
		result[grant.AppID] = append(result[grant.AppID], grant.UserID)
	}
	return result, nil
}

func appIDs(values []store.App) []string {
	ids := make([]string, 0, len(values))
	for _, value := range values {
		ids = append(ids, value.ID)
	}
	return ids
}

func normalizeAccountID(value string) (string, error) {
	id := strings.TrimSpace(value)
	if _, err := uuid.Parse(id); err != nil {
		return "", newError(CodeInvalidRequest, "用户 ID 格式错误", err)
	}
	return id, nil
}

func normalizeOwnedDetails(name string, description string, visibility string) (string, string, string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", "", "", newError(CodeInvalidRequest, "应用名称不能为空", nil)
	}
	if len([]rune(name)) > maxAppNameLength {
		return "", "", "", newError(CodeInvalidRequest, "应用名称不能超过 120 个字符", nil)
	}
	visibility, err := normalizeOwnedVisibility(visibility)
	if err != nil {
		return "", "", "", err
	}
	description, err = normalizeDescription(description)
	if err != nil {
		return "", "", "", err
	}
	return name, description, visibility, nil
}

func normalizeOwnedVisibility(value string) (string, error) {
	switch strings.TrimSpace(value) {
	case "", store.AppVisibilityCreator:
		return store.AppVisibilityCreator, nil
	case store.AppVisibilityRestricted:
		return store.AppVisibilityRestricted, nil
	case store.AppVisibilityPublic:
		return store.AppVisibilityPublic, nil
	default:
		return "", newError(CodeInvalidRequest, "可见范围不支持", nil)
	}
}
