package externalauth

import (
	"context"
	"encoding/json"
	"time"

	"app/internal/application/identityprovider"
)

type Profile struct {
	ExternalUserID string
	Email          string
	Name           string
	Nickname       string
	Phone          string
	Avatar         string
	Raw            json.RawMessage
}

type OAuthPort interface {
	BuildAuthorizeURL(provider identityprovider.Provider, state, redirectURI, verifier string) (string, error)
	FetchProfile(ctx context.Context, provider identityprovider.Provider, code, redirectURI, verifier string) (Profile, error)
}

type StartCommand struct {
	ProviderKey            string
	Redirect               string
	CallbackURLForProvider func(string) string
	IP                     string
	UserAgent              string
}

type StartResult struct {
	AuthorizeURL string
	State        string
	ExpiresAt    time.Time
}

type FinishCommand struct {
	ProviderKey            string
	Code                   string
	State                  string
	CookieState            string
	CallbackURLForProvider func(string) string
	IP                     string
	UserAgent              string
}

type SessionCredential struct {
	Token     string
	ExpiresAt time.Time
}

type FinishResult struct {
	RedirectPath string
	Session      SessionCredential
}

type ServiceAPI interface {
	Start(context.Context, StartCommand) (StartResult, error)
	Finish(context.Context, FinishCommand) (FinishResult, error)
}
