package account

import (
	"context"
	"io"
	"time"
)

const (
	StatusActive   = "active"
	StatusDisabled = "disabled"
)

type Account struct {
	ID           string
	Avatar       string
	Email        string
	LastOnlineAt *time.Time
	Name         string
	Nickname     string
	Phone        string
	Status       string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type SessionCredential struct {
	Token     string
	ExpiresAt time.Time
}

type AuthenticatedSession struct {
	ID      string
	Account Account
}

type LoginCommand struct {
	Email     string
	Password  string
	UserAgent string
	IP        string
}

type LoginResult struct {
	Account Account
	Session SessionCredential
}

type VerifiedEmailLoginCommand struct {
	Email     string
	UserAgent string
	IP        string
}

type LogoutCommand struct {
	Token string
}

type UpdateProfileCommand struct {
	AccountID string
	Avatar    *string
	Nickname  *string
}

type UploadAvatarCommand struct {
	AccountID string
	Size      int64
	Content   io.Reader
}

type ClientService interface {
	Login(context.Context, LoginCommand) (LoginResult, error)
	Logout(context.Context, LogoutCommand) error
	GetProfile(context.Context, string) (Account, error)
	UpdateProfile(context.Context, UpdateProfileCommand) (Account, error)
	UploadAvatar(context.Context, UploadAvatarCommand) (Account, error)
}

type SessionAuthenticator interface {
	AuthenticateSession(context.Context, string) (AuthenticatedSession, error)
}

type VerifiedEmailLoginService interface {
	CanLoginWithEmail(context.Context, string) (bool, error)
	LoginWithVerifiedEmail(context.Context, VerifiedEmailLoginCommand) (LoginResult, error)
}

type ActivityRecorder interface {
	RecordOnlineActivity(context.Context, string, time.Time) error
}

type PasswordLoginPolicy interface {
	PasswordLoginEnabled(context.Context) (bool, error)
}
