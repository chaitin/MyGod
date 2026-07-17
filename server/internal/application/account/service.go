package account

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/mail"
	"strconv"
	"strings"
	"time"

	fileapp "app/internal/application/file"
	"app/internal/auth"
	"app/internal/media"
	"app/internal/store"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	defaultSessionTTL = 7 * 24 * time.Hour
	avatarContentType = "image/webp"
	avatarSize        = 256
	maxAvatarBytes    = 1 * 1024 * 1024
)

type Dependencies struct {
	DB                   *gorm.DB
	Files                fileapp.PublicUploader
	PasswordLoginPolicy  PasswordLoginPolicy
	Now                  func() time.Time
	GenerateSessionToken func() (string, error)
	SessionTTL           time.Duration
}

type Service struct {
	db                   *gorm.DB
	files                fileapp.PublicUploader
	passwordLoginPolicy  PasswordLoginPolicy
	now                  func() time.Time
	generateSessionToken func() (string, error)
	sessionTTL           time.Duration
}

func NewService(deps Dependencies) *Service {
	now := deps.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	generateSessionToken := deps.GenerateSessionToken
	if generateSessionToken == nil {
		generateSessionToken = auth.GenerateSessionToken
	}
	sessionTTL := deps.SessionTTL
	if sessionTTL <= 0 {
		sessionTTL = defaultSessionTTL
	}

	return &Service{
		db:                   deps.DB,
		files:                deps.Files,
		passwordLoginPolicy:  deps.PasswordLoginPolicy,
		now:                  now,
		generateSessionToken: generateSessionToken,
		sessionTTL:           sessionTTL,
	}
}

func (s *Service) Login(ctx context.Context, cmd LoginCommand) (LoginResult, error) {
	if s.passwordLoginPolicy != nil {
		enabled, err := s.passwordLoginPolicy.PasswordLoginEnabled(ctx)
		if err != nil {
			return LoginResult{}, internalError(err)
		}
		if !enabled {
			return LoginResult{}, newError(CodeLoginUnavailable, "密码登录未启用", nil)
		}
	}
	email, err := normalizeEmail(cmd.Email)
	if err != nil {
		return LoginResult{}, invalidCredentials()
	}

	var user store.User
	err = s.db.WithContext(ctx).Where("email = ?", email).First(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return LoginResult{}, invalidCredentials()
	}
	if err != nil {
		return LoginResult{}, internalError(err)
	}
	if user.Status != store.UserStatusActive {
		return LoginResult{}, invalidCredentials()
	}

	valid, err := auth.VerifyPassword(cmd.Password, user.PasswordHash)
	if err != nil || !valid {
		return LoginResult{}, invalidCredentials()
	}
	return s.createLoginSession(ctx, user, cmd.UserAgent, cmd.IP)
}

func (s *Service) CanLoginWithEmail(ctx context.Context, rawEmail string) (bool, error) {
	email, err := normalizeEmail(rawEmail)
	if err != nil {
		return false, nil
	}
	var count int64
	if err := s.db.WithContext(ctx).Model(&store.User{}).
		Where("email = ? AND status = ?", email, store.UserStatusActive).
		Count(&count).Error; err != nil {
		return false, internalError(err)
	}
	return count > 0, nil
}

func (s *Service) LoginWithVerifiedEmail(ctx context.Context, cmd VerifiedEmailLoginCommand) (LoginResult, error) {
	email, err := normalizeEmail(cmd.Email)
	if err != nil {
		return LoginResult{}, invalidCredentials()
	}
	var user store.User
	err = s.db.WithContext(ctx).
		Where("email = ? AND status = ?", email, store.UserStatusActive).
		First(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return LoginResult{}, invalidCredentials()
	}
	if err != nil {
		return LoginResult{}, internalError(err)
	}
	return s.createLoginSession(ctx, user, cmd.UserAgent, cmd.IP)
}

func (s *Service) createLoginSession(ctx context.Context, user store.User, userAgent string, ip string) (LoginResult, error) {

	token, err := s.generateSessionToken()
	if err != nil {
		return LoginResult{}, internalError(err)
	}
	now := s.now().UTC()
	session := store.UserSession{
		ID:         uuid.NewString(),
		TokenHash:  auth.HashSessionToken(token),
		UserID:     user.ID,
		ExpiresAt:  now.Add(s.sessionTTL),
		CreatedAt:  now,
		LastSeenAt: now,
		UserAgent:  userAgent,
		IP:         ip,
	}
	if err := s.db.WithContext(ctx).Create(&session).Error; err != nil {
		return LoginResult{}, internalError(err)
	}

	return LoginResult{
		Account: newAccount(user),
		Session: SessionCredential{Token: token, ExpiresAt: session.ExpiresAt},
	}, nil
}

func (s *Service) Logout(ctx context.Context, cmd LogoutCommand) error {
	token := strings.TrimSpace(cmd.Token)
	if token == "" {
		return nil
	}
	if err := s.db.WithContext(ctx).
		Where("token_hash = ?", auth.HashSessionToken(token)).
		Delete(&store.UserSession{}).Error; err != nil {
		return internalError(err)
	}
	return nil
}

func (s *Service) AuthenticateSession(ctx context.Context, token string) (AuthenticatedSession, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return AuthenticatedSession{}, unauthorized()
	}

	var session store.UserSession
	err := s.db.WithContext(ctx).Preload("User").Where(
		"token_hash = ? AND expires_at > ?",
		auth.HashSessionToken(token),
		s.now().UTC(),
	).First(&session).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return AuthenticatedSession{}, unauthorized()
	}
	if err != nil {
		return AuthenticatedSession{}, internalError(err)
	}
	if session.User.Status != store.UserStatusActive {
		return AuthenticatedSession{}, unauthorized()
	}

	_ = s.db.WithContext(ctx).Model(&session).Update("last_seen_at", s.now().UTC()).Error

	return AuthenticatedSession{
		ID:      session.ID,
		Account: newAccount(session.User),
	}, nil
}

func (s *Service) GetProfile(ctx context.Context, accountID string) (Account, error) {
	var user store.User
	err := s.db.WithContext(ctx).First(&user, "id = ?", strings.TrimSpace(accountID)).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Account{}, newError(CodeNotFound, "用户不存在", err)
	}
	if err != nil {
		return Account{}, internalError(err)
	}
	return newAccount(user), nil
}

func (s *Service) UpdateProfile(ctx context.Context, cmd UpdateProfileCommand) (Account, error) {
	updates := map[string]any{}
	if cmd.Avatar != nil {
		avatar, err := normalizeBuiltinAvatar(*cmd.Avatar)
		if err != nil {
			return Account{}, newError(CodeInvalidRequest, "头像格式错误", err)
		}
		updates["avatar"] = avatar
	}
	if cmd.Nickname != nil {
		updates["nickname"] = strings.TrimSpace(*cmd.Nickname)
	}
	if len(updates) == 0 {
		return Account{}, newError(CodeInvalidRequest, "至少需要修改一个字段", nil)
	}

	if err := s.db.WithContext(ctx).Model(&store.User{}).
		Where("id = ?", strings.TrimSpace(cmd.AccountID)).
		Updates(updates).Error; err != nil {
		return Account{}, internalError(err)
	}
	return s.GetProfile(ctx, cmd.AccountID)
}

func (s *Service) UploadAvatar(ctx context.Context, cmd UploadAvatarCommand) (Account, error) {
	if cmd.Size > maxAvatarBytes {
		return Account{}, newError(CodeRequestTooLarge, "头像文件不能超过 1MiB", nil)
	}
	if cmd.Size == 0 || cmd.Content == nil {
		return Account{}, newError(CodeInvalidRequest, "头像文件不能为空", nil)
	}

	content, err := io.ReadAll(io.LimitReader(cmd.Content, maxAvatarBytes+1))
	if err != nil {
		return Account{}, newError(CodeInvalidRequest, "读取头像失败", err)
	}
	if len(content) > maxAvatarBytes {
		return Account{}, newError(CodeRequestTooLarge, "头像文件不能超过 1MiB", nil)
	}
	if len(content) == 0 {
		return Account{}, newError(CodeInvalidRequest, "头像文件不能为空", nil)
	}
	width, height, err := media.WebPDimensions(content)
	if err != nil || width != avatarSize || height != avatarSize {
		return Account{}, newError(CodeInvalidRequest, "头像必须是 256x256 的 WebP 图片", err)
	}
	if s.files == nil {
		return Account{}, wrapInternal("头像存储未配置", nil)
	}

	accountID := strings.TrimSpace(cmd.AccountID)
	objectKey := fmt.Sprintf("avatars/users/%s/%s.webp", accountID, uuid.NewString())
	uploaded, err := s.files.UploadPublic(ctx, fileapp.UploadPublicCommand{
		ObjectKey:   objectKey,
		Content:     bytes.NewReader(content),
		ContentType: avatarContentType,
		SizeBytes:   int64(len(content)),
	})
	if err != nil {
		if fileapp.ErrorCodeOf(err) == fileapp.CodeStorageUnavailable {
			return Account{}, wrapInternal("头像存储未配置", err)
		}
		return Account{}, wrapInternal("上传头像失败", err)
	}
	avatarURL := uploaded.URL
	if strings.TrimSpace(avatarURL) == "" {
		return Account{}, wrapInternal("头像存储未配置", nil)
	}
	if err := s.db.WithContext(ctx).Model(&store.User{}).
		Where("id = ?", accountID).
		Update("avatar", avatarURL).Error; err != nil {
		return Account{}, wrapInternal("保存头像失败", err)
	}
	return s.GetProfile(ctx, accountID)
}

func (s *Service) RecordOnlineActivity(ctx context.Context, accountID string, at time.Time) error {
	if err := s.db.WithContext(ctx).Model(&store.User{}).
		Where("id = ?", strings.TrimSpace(accountID)).
		Update("last_online_at", at.UTC()).Error; err != nil {
		return internalError(err)
	}
	return nil
}

func newAccount(user store.User) Account {
	phone := ""
	if user.Phone != nil {
		phone = *user.Phone
	}
	avatar := user.Avatar
	if avatar == "" {
		avatar = store.DefaultUserAvatar
	}
	return Account{
		ID:           user.ID,
		Avatar:       avatar,
		Email:        user.Email,
		LastOnlineAt: user.LastOnlineAt,
		Name:         user.Name,
		Nickname:     user.Nickname,
		Phone:        phone,
		Status:       user.Status,
		CreatedAt:    user.CreatedAt,
		UpdatedAt:    user.UpdatedAt,
	}
}

func normalizeEmail(raw string) (string, error) {
	email := strings.ToLower(strings.TrimSpace(raw))
	address, err := mail.ParseAddress(email)
	if err != nil || address.Address != email {
		return "", errors.New("invalid email")
	}
	return email, nil
}

func normalizeBuiltinAvatar(raw string) (string, error) {
	avatar := strings.TrimSpace(raw)
	const prefix = "/assets/avatars/builtin/"
	const suffix = ".webp"
	if !strings.HasPrefix(avatar, prefix) || !strings.HasSuffix(avatar, suffix) {
		return "", errors.New("invalid avatar")
	}

	id := strings.TrimSuffix(strings.TrimPrefix(avatar, prefix), suffix)
	if len(id) != 2 {
		return "", errors.New("invalid avatar")
	}
	index, err := strconv.Atoi(id)
	if err != nil || index < 1 || index > 64 || fmt.Sprintf("%02d", index) != id {
		return "", errors.New("invalid avatar")
	}
	return avatar, nil
}

func invalidCredentials() error {
	return newError(CodeInvalidCredentials, "邮箱或密码错误", nil)
}

func unauthorized() error {
	return newError(CodeUnauthorized, "未登录", nil)
}

var _ ClientService = (*Service)(nil)
var _ SessionAuthenticator = (*Service)(nil)
var _ ActivityRecorder = (*Service)(nil)
var _ VerifiedEmailLoginService = (*Service)(nil)
