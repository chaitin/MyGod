package adminauth

import (
	"context"
	"time"
)

type Admin struct {
	Email string
}

type SessionCredential struct {
	Token     string
	ExpiresAt time.Time
}

type AuthenticatedSession struct {
	ID string
}

type LoginCommand struct {
	Email     string
	Password  string
	UserAgent string
	IP        string
}

type LoginResult struct {
	Admin   Admin
	Session SessionCredential
}

type LoginService interface {
	Login(context.Context, LoginCommand) (LoginResult, error)
}

type SessionAuthenticator interface {
	AuthenticateSession(context.Context, string) (AuthenticatedSession, error)
}
