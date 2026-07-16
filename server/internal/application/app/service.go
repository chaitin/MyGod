package app

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	fileapp "app/internal/application/file"
	"app/internal/appregistry"
	"app/internal/config"
	"app/internal/media"
	"app/internal/store"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	appSecretBytes    = 32
	maxAppNameLength  = 120
	avatarContentType = "image/webp"
	avatarSize        = 256
)

type Dependencies struct {
	DB             *gorm.DB
	Apps           config.AppsConfig
	Files          fileapp.PublicUploader
	Connections    ConnectionPort
	Now            func() time.Time
	NewID          func() string
	GenerateSecret func() (string, error)
}

type Service struct {
	db             *gorm.DB
	apps           config.AppsConfig
	files          fileapp.PublicUploader
	connections    ConnectionPort
	now            func() time.Time
	newID          func() string
	generateSecret func() (string, error)
}

func NewService(deps Dependencies) *Service {
	now := deps.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	newID := deps.NewID
	if newID == nil {
		newID = uuid.NewString
	}
	generateSecret := deps.GenerateSecret
	if generateSecret == nil {
		generateSecret = randomSecret
	}
	return &Service{
		db: deps.DB, apps: deps.Apps, files: deps.Files, connections: deps.Connections,
		now: now, newID: newID, generateSecret: generateSecret,
	}
}

func (s *Service) List(ctx context.Context) ([]App, error) {
	if _, err := s.ensureAIAssistant(ctx); err != nil {
		return nil, internalError(err)
	}

	var storedApps []store.App
	if err := s.db.WithContext(ctx).Find(&storedApps).Error; err != nil {
		return nil, internalError(err)
	}
	sortApps(storedApps)

	result := make([]App, 0, len(storedApps))
	for _, storedApp := range storedApps {
		result = append(result, s.newApp(storedApp))
	}
	return result, nil
}

func (s *Service) Get(ctx context.Context, appID string) (App, error) {
	storedApp, err := s.find(ctx, appID)
	if err != nil {
		return App{}, err
	}
	return s.newApp(storedApp), nil
}

func (s *Service) GetForConnection(ctx context.Context, appID string) (App, error) {
	id, err := normalizeAppID(appID)
	if err != nil {
		return App{}, err
	}
	if appregistry.IsAIAssistantAppID(id) {
		storedApp, ensureErr := s.ensureAIAssistant(ctx)
		if ensureErr != nil {
			return App{}, internalError(ensureErr)
		}
		return s.newApp(storedApp), nil
	}

	var storedApp store.App
	err = s.db.WithContext(ctx).First(&storedApp, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return App{}, newError(CodeNotFound, "应用不存在", err)
	}
	if err != nil {
		return App{}, internalError(err)
	}
	return s.newApp(storedApp), nil
}

func (s *Service) Create(ctx context.Context, cmd CreateCommand) (App, error) {
	name, description, visibility, err := normalizeDetails(cmd.Name, cmd.Description, cmd.Visibility)
	if err != nil {
		return App{}, err
	}
	now := s.now().UTC()
	storedApp := store.App{
		ID: s.newID(), Name: name, Description: description, Enabled: true,
		Visibility: visibility, CreatedAt: now, UpdatedAt: now,
	}
	secret, err := s.generateUniqueSecret(ctx)
	if err != nil {
		return App{}, internalError(err)
	}
	storedApp.ConnectionSecret = secret
	if err := s.db.WithContext(ctx).Create(&storedApp).Error; err != nil {
		return App{}, internalError(err)
	}
	return s.newApp(storedApp), nil
}

func (s *Service) Update(ctx context.Context, cmd UpdateCommand) (App, error) {
	storedApp, err := s.find(ctx, cmd.AppID)
	if err != nil {
		return App{}, err
	}
	name, description, visibility, err := normalizeDetails(cmd.Name, cmd.Description, cmd.Visibility)
	if err != nil {
		return App{}, err
	}
	if appregistry.IsAIAssistantAppID(storedApp.ID) {
		visibility = store.AppVisibilityPublic
	}
	if err := s.db.WithContext(ctx).Model(&storedApp).Updates(map[string]any{
		"description": description,
		"name":        name,
		"visibility":  visibility,
		"updated_at":  s.now().UTC(),
	}).Error; err != nil {
		return App{}, internalError(err)
	}
	return s.reload(ctx, storedApp.ID)
}

func (s *Service) SetEnabled(ctx context.Context, cmd SetEnabledCommand) (App, error) {
	storedApp, err := s.find(ctx, cmd.AppID)
	if err != nil {
		return App{}, err
	}
	if storedApp.Enabled == cmd.Enabled {
		return s.newApp(storedApp), nil
	}
	if err := s.db.WithContext(ctx).Model(&storedApp).Updates(map[string]any{
		"enabled": cmd.Enabled, "updated_at": s.now().UTC(),
	}).Error; err != nil {
		return App{}, internalError(err)
	}
	updated, err := s.reload(ctx, storedApp.ID)
	if err != nil {
		return App{}, err
	}
	if !cmd.Enabled && s.connections != nil {
		s.connections.CloseApp(storedApp.ID)
	}
	return updated, nil
}

func (s *Service) RegenerateSecret(ctx context.Context, appID string) (App, error) {
	storedApp, err := s.find(ctx, appID)
	if err != nil {
		return App{}, err
	}
	if appregistry.IsAIAssistantAppID(storedApp.ID) {
		return App{}, newError(CodeForbidden, "茉莉密钥由配置管理", nil)
	}
	secret, err := s.generateUniqueSecret(ctx)
	if err != nil {
		return App{}, internalError(err)
	}
	if err := s.db.WithContext(ctx).Model(&storedApp).Updates(map[string]any{
		"connection_secret": secret, "updated_at": s.now().UTC(),
	}).Error; err != nil {
		return App{}, internalError(err)
	}
	reloaded, err := s.reloadStored(ctx, storedApp.ID)
	if err != nil {
		return App{}, err
	}
	if s.connections != nil {
		s.connections.CloseApp(storedApp.ID)
	}
	return s.newApp(reloaded), nil
}

func (s *Service) Delete(ctx context.Context, appID string) error {
	storedApp, err := s.find(ctx, appID)
	if err != nil {
		return err
	}
	if appregistry.IsAIAssistantAppID(storedApp.ID) {
		return newError(CodeForbidden, "茉莉不能删除", nil)
	}
	if err := s.db.WithContext(ctx).Delete(&store.App{}, "id = ?", storedApp.ID).Error; err != nil {
		return internalError(err)
	}
	if s.connections != nil {
		s.connections.CloseApp(storedApp.ID)
	}
	return nil
}

func (s *Service) UploadAvatar(ctx context.Context, cmd UploadAvatarCommand) (App, error) {
	storedApp, err := s.find(ctx, cmd.AppID)
	if err != nil {
		return App{}, err
	}
	if cmd.Size > MaxAvatarBytes {
		return App{}, newError(CodeRequestTooLarge, "头像文件不能超过 1MiB", nil)
	}
	if cmd.Size == 0 || cmd.Content == nil {
		return App{}, newError(CodeInvalidRequest, "头像文件不能为空", nil)
	}
	content, err := io.ReadAll(io.LimitReader(cmd.Content, MaxAvatarBytes+1))
	if err != nil {
		return App{}, newError(CodeInvalidRequest, "读取头像失败", err)
	}
	if len(content) > MaxAvatarBytes {
		return App{}, newError(CodeRequestTooLarge, "头像文件不能超过 1MiB", nil)
	}
	if len(content) == 0 {
		return App{}, newError(CodeInvalidRequest, "读取头像失败", nil)
	}
	width, height, err := media.WebPDimensions(content)
	if err != nil || width != avatarSize || height != avatarSize {
		return App{}, newError(CodeInvalidRequest, "头像必须是 256x256 的 WebP 图片", err)
	}
	if s.files == nil {
		return App{}, wrapInternal("头像存储未配置", nil)
	}
	objectKey := fmt.Sprintf("avatars/apps/%s/%s.webp", storedApp.ID, strings.TrimSpace(s.newID()))
	uploaded, err := s.files.UploadPublic(ctx, fileapp.UploadPublicCommand{
		ObjectKey: objectKey, Content: bytes.NewReader(content),
		ContentType: avatarContentType, SizeBytes: int64(len(content)),
	})
	if err != nil {
		if fileapp.ErrorCodeOf(err) == fileapp.CodeStorageUnavailable {
			return App{}, wrapInternal("头像存储未配置", err)
		}
		return App{}, wrapInternal("上传头像失败", err)
	}
	if err := s.db.WithContext(ctx).Model(&store.App{}).
		Where("id = ?", storedApp.ID).Update("avatar", uploaded.URL).Error; err != nil {
		return App{}, wrapInternal("保存头像失败", err)
	}
	return s.reload(ctx, storedApp.ID)
}

func (s *Service) find(ctx context.Context, appID string) (store.App, error) {
	id, err := normalizeAppID(appID)
	if err != nil {
		return store.App{}, err
	}
	var storedApp store.App
	err = s.db.WithContext(ctx).First(&storedApp, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		if appregistry.IsAIAssistantAppID(id) {
			storedApp, err = s.ensureAIAssistant(ctx)
			if err == nil {
				return storedApp, nil
			}
			return store.App{}, internalError(err)
		}
		return store.App{}, newError(CodeNotFound, "应用不存在", err)
	}
	if err != nil {
		return store.App{}, internalError(err)
	}
	return storedApp, nil
}

func (s *Service) reload(ctx context.Context, appID string) (App, error) {
	storedApp, err := s.reloadStored(ctx, appID)
	if err != nil {
		return App{}, err
	}
	return s.newApp(storedApp), nil
}

func (s *Service) reloadStored(ctx context.Context, appID string) (store.App, error) {
	var storedApp store.App
	if err := s.db.WithContext(ctx).First(&storedApp, "id = ?", appID).Error; err != nil {
		return store.App{}, internalError(err)
	}
	return storedApp, nil
}

func (s *Service) ensureAIAssistant(ctx context.Context) (store.App, error) {
	return appregistry.EnsureAIAssistantApp(s.db.WithContext(ctx), s.apps)
}

func (s *Service) newApp(storedApp store.App) App {
	status := ConnectionStatusOffline
	if !storedApp.Enabled {
		status = ConnectionStatusDisabled
	} else if s.connections != nil && s.connections.IsOnline(storedApp.ID) {
		status = ConnectionStatusOnline
	}
	return App{
		Avatar: storedApp.Avatar, ConnectionSecret: storedApp.ConnectionSecret,
		ConnectionStatus: status, CreatedAt: storedApp.CreatedAt,
		CreatorUserID: storedApp.CreatorUserID, Description: storedApp.Description,
		Enabled: storedApp.Enabled, ID: storedApp.ID, Name: storedApp.Name,
		System: appregistry.IsAIAssistantAppID(storedApp.ID), UpdatedAt: storedApp.UpdatedAt,
		Visibility: storedApp.Visibility,
	}
}

func (s *Service) generateUniqueSecret(ctx context.Context) (string, error) {
	for attempts := 0; attempts < 5; attempts++ {
		secret, err := s.generateSecret()
		if err != nil {
			return "", err
		}
		var count int64
		if err := s.db.WithContext(ctx).Model(&store.App{}).
			Where("connection_secret = ?", secret).Count(&count).Error; err != nil {
			return "", err
		}
		if count == 0 {
			return secret, nil
		}
	}
	return "", errors.New("generate unique app secret failed")
}

func normalizeAppID(value string) (string, error) {
	id := strings.TrimSpace(value)
	if _, err := uuid.Parse(id); err != nil {
		return "", newError(CodeInvalidRequest, "应用 ID 格式错误", err)
	}
	return id, nil
}

func normalizeDetails(rawName string, rawDescription string, rawVisibility string) (string, string, string, error) {
	name := strings.TrimSpace(rawName)
	if name == "" {
		return "", "", "", newError(CodeInvalidRequest, "应用名称不能为空", nil)
	}
	if len([]rune(name)) > maxAppNameLength {
		return "", "", "", newError(CodeInvalidRequest, "应用名称不能超过 120 个字符", nil)
	}
	visibility, err := normalizeVisibility(rawVisibility)
	if err != nil {
		return "", "", "", err
	}
	return name, strings.TrimSpace(rawDescription), visibility, nil
}

func normalizeVisibility(value string) (string, error) {
	switch strings.TrimSpace(value) {
	case "", store.AppVisibilityPublic:
		return store.AppVisibilityPublic, nil
	case store.AppVisibilityCreator:
		return "", newError(CodeInvalidRequest, "后台创建的应用暂不支持仅创建者可见", nil)
	default:
		return "", newError(CodeInvalidRequest, "可见范围不支持", nil)
	}
}

func sortApps(values []store.App) {
	slices.SortFunc(values, func(left store.App, right store.App) int {
		if appregistry.IsAIAssistantAppID(left.ID) && !appregistry.IsAIAssistantAppID(right.ID) {
			return -1
		}
		if !appregistry.IsAIAssistantAppID(left.ID) && appregistry.IsAIAssistantAppID(right.ID) {
			return 1
		}
		if strings.EqualFold(left.Name, right.Name) {
			return strings.Compare(left.ID, right.ID)
		}
		return strings.Compare(strings.ToLower(left.Name), strings.ToLower(right.Name))
	})
}

func randomSecret() (string, error) {
	value := make([]byte, appSecretBytes)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

var _ AdminService = (*Service)(nil)
var _ ConnectionService = (*Service)(nil)
