package adminauth

import (
	"context"
	"crypto/subtle"
	"errors"
	"strings"
	"time"

	"app/internal/auth"
	"app/internal/store"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	adminEmail        = "admin"
	defaultSessionTTL = 7 * 24 * time.Hour
)

type Dependencies struct {
	DB                   *gorm.DB
	Password             string
	Now                  func() time.Time
	GenerateSessionToken func() (string, error)
	NewID                func() string
	SessionTTL           time.Duration
}

type Service struct {
	db                   *gorm.DB
	password             string
	now                  func() time.Time
	generateSessionToken func() (string, error)
	newID                func() string
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
	newID := deps.NewID
	if newID == nil {
		newID = uuid.NewString
	}
	sessionTTL := deps.SessionTTL
	if sessionTTL <= 0 {
		sessionTTL = defaultSessionTTL
	}
	return &Service{
		db: deps.DB, password: deps.Password, now: now,
		generateSessionToken: generateSessionToken, newID: newID, sessionTTL: sessionTTL,
	}
}

func (s *Service) Login(ctx context.Context, cmd LoginCommand) (LoginResult, error) {
	if strings.TrimSpace(cmd.Email) != adminEmail ||
		subtle.ConstantTimeCompare([]byte(cmd.Password), []byte(s.password)) != 1 {
		return LoginResult{}, newError(CodeInvalidCredentials, "邮箱或密码错误", nil)
	}
	token, err := s.generateSessionToken()
	if err != nil {
		return LoginResult{}, internalError(err)
	}
	now := s.now().UTC()
	session := store.AdminSession{
		ID: s.newID(), TokenHash: auth.HashSessionToken(token), ExpiresAt: now.Add(s.sessionTTL),
		LastSeenAt: now, UserAgent: cmd.UserAgent, IP: cmd.IP,
	}
	if err := s.db.WithContext(ctx).Create(&session).Error; err != nil {
		return LoginResult{}, internalError(err)
	}
	return LoginResult{
		Admin:   Admin{Email: adminEmail},
		Session: SessionCredential{Token: token, ExpiresAt: session.ExpiresAt},
	}, nil
}

func (s *Service) AuthenticateSession(ctx context.Context, token string) (AuthenticatedSession, error) {
	if token == "" {
		return AuthenticatedSession{}, newError(CodeUnauthorized, "未登录", nil)
	}
	var session store.AdminSession
	err := s.db.WithContext(ctx).Where(
		"token_hash = ? AND expires_at > ?", auth.HashSessionToken(token), s.now().UTC(),
	).First(&session).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return AuthenticatedSession{}, newError(CodeUnauthorized, "未登录", err)
	}
	if err != nil {
		return AuthenticatedSession{}, internalError(err)
	}
	_ = s.db.WithContext(ctx).Model(&session).Update("last_seen_at", s.now().UTC()).Error
	return AuthenticatedSession{ID: session.ID}, nil
}

var _ LoginService = (*Service)(nil)
var _ SessionAuthenticator = (*Service)(nil)
