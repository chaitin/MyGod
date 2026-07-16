package externalauth

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	appauth "app/internal/application/externalauth"
	"app/internal/application/identityprovider"
)

const httpTimeout = 10 * time.Second

type OAuth struct{}

func NewOAuth() *OAuth {
	return &OAuth{}
}

type tokenResponse struct {
	AccessToken      string `json:"access_token"`
	AccessTokenCamel string `json:"accessToken"`
	UserAccessToken  string `json:"user_access_token"`
	TokenType        string `json:"token_type"`
	Data             *struct {
		AccessToken     string `json:"access_token"`
		UserAccessToken string `json:"user_access_token"`
	} `json:"data"`
}

func (response tokenResponse) token() string {
	for _, value := range []string{response.AccessToken, response.AccessTokenCamel, response.UserAccessToken} {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	if response.Data != nil {
		for _, value := range []string{response.Data.AccessToken, response.Data.UserAccessToken} {
			if strings.TrimSpace(value) != "" {
				return strings.TrimSpace(value)
			}
		}
	}
	return ""
}

func (o *OAuth) BuildAuthorizeURL(provider identityprovider.Provider, state, redirectURI, verifier string) (string, error) {
	authorizeURL, err := url.Parse(identityprovider.StringConfigValue(provider.Config, "authorize_url"))
	if err != nil {
		return "", err
	}
	query := authorizeURL.Query()
	query.Set("redirect_uri", redirectURI)
	query.Set("state", state)
	if provider.Type == identityprovider.TypeWeCom {
		buildWeComAuthorizeQuery(authorizeURL, query, provider)
	} else {
		query.Set("response_type", "code")
		query.Set("client_id", provider.ClientID)
		if len(provider.Scopes) > 0 {
			query.Set("scope", strings.Join(provider.Scopes, " "))
		}
		if usesPKCE(provider.Type) {
			query.Set("code_challenge", codeChallenge(verifier))
			query.Set("code_challenge_method", "S256")
		}
	}
	authorizeURL.RawQuery = query.Encode()
	return authorizeURL.String(), nil
}

func (o *OAuth) FetchProfile(ctx context.Context, provider identityprovider.Provider, code, redirectURI, verifier string) (appauth.Profile, error) {
	switch provider.Type {
	case identityprovider.TypeDingTalk:
		return o.fetchDingTalkProfile(ctx, provider, code)
	case identityprovider.TypeWeCom:
		return o.fetchWeComProfile(ctx, provider, code)
	case identityprovider.TypeFeishu:
		return o.fetchFeishuProfile(ctx, provider, code, redirectURI, verifier)
	case identityprovider.TypeGitHub:
		return o.fetchGitHubProfile(ctx, provider, code, redirectURI)
	default:
		return o.fetchBearerProfile(ctx, provider, code, redirectURI, verifier)
	}
}

func (o *OAuth) fetchBearerProfile(ctx context.Context, provider identityprovider.Provider, code, redirectURI, verifier string) (appauth.Profile, error) {
	accessToken, err := o.exchangeFormCode(ctx, provider, code, redirectURI, verifier)
	if err != nil {
		return appauth.Profile{}, err
	}
	claims, err := o.fetchJSONWithBearer(ctx, provider, identityprovider.StringConfigValue(provider.Config, "userinfo_url"), accessToken)
	if err != nil {
		return appauth.Profile{}, err
	}
	return profileFromClaims(provider, unwrapDataClaims(claims))
}

func (o *OAuth) fetchFeishuProfile(ctx context.Context, provider identityprovider.Provider, code, redirectURI, verifier string) (appauth.Profile, error) {
	accessToken, err := o.exchangeJSONCode(ctx, provider, code, redirectURI, verifier)
	if err != nil {
		return appauth.Profile{}, err
	}
	claims, err := o.fetchJSONWithBearer(ctx, provider, identityprovider.StringConfigValue(provider.Config, "userinfo_url"), accessToken)
	if err != nil {
		return appauth.Profile{}, err
	}
	return profileFromClaims(provider, unwrapDataClaims(claims))
}

func (o *OAuth) fetchGitHubProfile(ctx context.Context, provider identityprovider.Provider, code, redirectURI string) (appauth.Profile, error) {
	accessToken, err := o.exchangeFormCode(ctx, provider, code, redirectURI, "")
	if err != nil {
		return appauth.Profile{}, err
	}
	userinfoURL := identityprovider.StringConfigValue(provider.Config, "userinfo_url")
	claims, err := o.fetchJSONWithBearer(ctx, provider, userinfoURL, accessToken)
	if err != nil {
		return appauth.Profile{}, err
	}
	emailsURL := identityprovider.StringConfigValue(provider.Config, "emails_url")
	if stringField(claims, "email") == "" && emailsURL != "" {
		if email := o.fetchGitHubPrimaryEmail(ctx, emailsURL, accessToken); email != "" {
			claims["email"] = email
		}
	}
	return profileFromClaims(provider, claims)
}

func (o *OAuth) exchangeFormCode(ctx context.Context, provider identityprovider.Provider, code, redirectURI, verifier string) (string, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", redirectURI)
	form.Set("client_id", provider.ClientID)
	form.Set("client_secret", provider.ClientSecret)
	if verifier != "" && usesPKCE(provider.Type) {
		form.Set("code_verifier", verifier)
	}
	requestContext, cancel := context.WithTimeout(ctx, httpTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(
		requestContext, http.MethodPost, identityprovider.StringConfigValue(provider.Config, "token_url"), strings.NewReader(form.Encode()),
	)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	return exchangeToken(req)
}

func (o *OAuth) exchangeJSONCode(ctx context.Context, provider identityprovider.Provider, code, redirectURI, verifier string) (string, error) {
	payload := map[string]string{
		"grant_type": "authorization_code", "code": code, "redirect_uri": redirectURI,
		"client_id": provider.ClientID, "client_secret": provider.ClientSecret,
	}
	if verifier != "" && usesPKCE(provider.Type) {
		payload["code_verifier"] = verifier
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	requestContext, cancel := context.WithTimeout(ctx, httpTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(
		requestContext, http.MethodPost, identityprovider.StringConfigValue(provider.Config, "token_url"), bytes.NewReader(body),
	)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Accept", "application/json")
	return exchangeToken(req)
}

func exchangeToken(req *http.Request) (string, error) {
	token, err := doJSONRequest[tokenResponse](req)
	if err != nil {
		return "", err
	}
	accessToken := token.token()
	if accessToken == "" {
		return "", errors.New("access token is empty")
	}
	return accessToken, nil
}

func (o *OAuth) fetchJSONWithBearer(ctx context.Context, provider identityprovider.Provider, endpoint, accessToken string) (map[string]any, error) {
	requestContext, cancel := context.WithTimeout(ctx, httpTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(requestContext, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)
	if provider.Type == identityprovider.TypeGitHub {
		req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	}
	return doJSONMapRequest(req)
}

func (o *OAuth) fetchGitHubPrimaryEmail(ctx context.Context, endpoint, accessToken string) string {
	requestContext, cancel := context.WithTimeout(ctx, httpTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(requestContext, http.MethodGet, endpoint, nil)
	if err != nil {
		return ""
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return ""
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ""
	}
	var emails []map[string]any
	if err := decodeJSONBody(resp.Body, &emails); err != nil {
		return ""
	}
	var fallback string
	for _, email := range emails {
		value := stringField(email, "email")
		if value == "" || !boolField(email, "verified") {
			continue
		}
		if fallback == "" {
			fallback = value
		}
		if boolField(email, "primary") {
			return value
		}
	}
	return fallback
}

func usesPKCE(providerType string) bool {
	switch providerType {
	case identityprovider.TypeOIDC, identityprovider.TypeGoogle, identityprovider.TypeFeishu:
		return true
	default:
		return false
	}
}

func codeChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func doJSONMapRequest(req *http.Request) (map[string]any, error) {
	return doJSONRequest[map[string]any](req)
}

func doJSONRequest[T any](req *http.Request) (T, error) {
	var zero T
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return zero, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return zero, fmt.Errorf("%s status %d", req.URL.String(), resp.StatusCode)
	}
	var decoded T
	if err := decodeJSONBody(resp.Body, &decoded); err != nil {
		return zero, err
	}
	return decoded, nil
}

func decodeJSONBody(body io.Reader, target any) error {
	decoder := json.NewDecoder(body)
	decoder.UseNumber()
	return decoder.Decode(target)
}

var _ appauth.OAuthPort = (*OAuth)(nil)
