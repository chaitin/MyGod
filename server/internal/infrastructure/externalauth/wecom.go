package externalauth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	appauth "app/internal/application/externalauth"
	"app/internal/application/identityprovider"
)

type weComTokenResponse struct {
	ErrCode     int64  `json:"errcode"`
	ErrMessage  string `json:"errmsg"`
	AccessToken string `json:"access_token"`
}

func buildWeComAuthorizeQuery(authorizeURL *url.URL, query url.Values, provider identityprovider.Provider) {
	query.Set("appid", provider.ClientID)
	query.Set("agentid", identityprovider.StringConfigValue(provider.Config, "agent_id"))
	if weComUsesWebLogin(authorizeURL, provider.Config) {
		query.Set("login_type", firstNonEmpty(identityprovider.StringConfigValue(provider.Config, "login_type"), "CorpApp"))
		return
	}
	query.Set("response_type", "code")
	query.Set("scope", firstScopeOrDefault(provider.Scopes, "snsapi_base"))
	authorizeURL.Fragment = "wechat_redirect"
}

func weComUsesWebLogin(authorizeURL *url.URL, config map[string]any) bool {
	if identityprovider.StringConfigValue(config, "login_type") != "" {
		return true
	}
	return authorizeURL.Host == "login.work.weixin.qq.com" || strings.Contains(authorizeURL.Path, "/wwlogin/sso/login")
}

func (o *OAuth) fetchWeComProfile(ctx context.Context, provider identityprovider.Provider, code string) (appauth.Profile, error) {
	tokenURL, err := url.Parse(identityprovider.StringConfigValue(provider.Config, "token_url"))
	if err != nil {
		return appauth.Profile{}, err
	}
	tokenQuery := tokenURL.Query()
	tokenQuery.Set("corpid", provider.ClientID)
	tokenQuery.Set("corpsecret", provider.ClientSecret)
	tokenURL.RawQuery = tokenQuery.Encode()
	requestContext, cancel := context.WithTimeout(ctx, httpTimeout)
	defer cancel()
	tokenReq, err := http.NewRequestWithContext(requestContext, http.MethodGet, tokenURL.String(), nil)
	if err != nil {
		return appauth.Profile{}, err
	}
	tokenReq.Header.Set("Accept", "application/json")
	tokenResp, err := doJSONRequest[weComTokenResponse](tokenReq)
	if err != nil {
		return appauth.Profile{}, err
	}
	if tokenResp.ErrCode != 0 || strings.TrimSpace(tokenResp.AccessToken) == "" {
		return appauth.Profile{}, fmt.Errorf("wecom token error %d %s", tokenResp.ErrCode, tokenResp.ErrMessage)
	}

	userinfoURL, err := url.Parse(identityprovider.StringConfigValue(provider.Config, "userinfo_url"))
	if err != nil {
		return appauth.Profile{}, err
	}
	userinfoQuery := userinfoURL.Query()
	userinfoQuery.Set("access_token", tokenResp.AccessToken)
	userinfoQuery.Set("code", code)
	userinfoURL.RawQuery = userinfoQuery.Encode()
	userinfoContext, userinfoCancel := context.WithTimeout(ctx, httpTimeout)
	defer userinfoCancel()
	userinfoReq, err := http.NewRequestWithContext(userinfoContext, http.MethodGet, userinfoURL.String(), nil)
	if err != nil {
		return appauth.Profile{}, err
	}
	userinfoReq.Header.Set("Accept", "application/json")
	claims, err := doJSONMapRequest(userinfoReq)
	if err != nil {
		return appauth.Profile{}, err
	}
	if errCode := int64Field(claims, "errcode"); errCode != 0 {
		return appauth.Profile{}, fmt.Errorf("wecom userinfo error %d", errCode)
	}
	externalID := firstNonEmptyField(claims, "userid", "openid", "external_userid")
	if externalID == "" {
		return appauth.Profile{}, errors.New("wecom external user id is empty")
	}
	userDetailURL := identityprovider.StringConfigValue(provider.Config, "userdetail_url")
	if userTicket := stringField(claims, "user_ticket"); userTicket != "" && userDetailURL != "" {
		if detailClaims, err := o.fetchWeComUserDetail(ctx, userDetailURL, tokenResp.AccessToken, userTicket); err == nil {
			claims = mergeClaims(claims, detailClaims)
		}
	}
	raw, err := json.Marshal(claims)
	if err != nil {
		return appauth.Profile{}, err
	}
	return appauth.Profile{
		ExternalUserID: externalID,
		Email:          firstNonEmptyField(claims, "biz_mail", "email"),
		Name:           firstNonEmptyField(claims, "name", "userid", "openid", "external_userid"),
		Nickname:       firstNonEmptyField(claims, "alias", "name", "userid", "openid", "external_userid"),
		Phone:          stringField(claims, "mobile"), Avatar: stringField(claims, "avatar"), Raw: raw,
	}, nil
}

func (o *OAuth) fetchWeComUserDetail(ctx context.Context, endpoint, accessToken, userTicket string) (map[string]any, error) {
	userDetailURL, err := url.Parse(endpoint)
	if err != nil {
		return nil, err
	}
	query := userDetailURL.Query()
	query.Set("access_token", accessToken)
	userDetailURL.RawQuery = query.Encode()
	payload, err := json.Marshal(map[string]string{"user_ticket": userTicket})
	if err != nil {
		return nil, err
	}
	requestContext, cancel := context.WithTimeout(ctx, httpTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(requestContext, http.MethodPost, userDetailURL.String(), bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	claims, err := doJSONMapRequest(req)
	if err != nil {
		return nil, err
	}
	if errCode := int64Field(claims, "errcode"); errCode != 0 {
		return nil, fmt.Errorf("wecom userdetail error %d", errCode)
	}
	return claims, nil
}

func firstScopeOrDefault(scopes []string, fallback string) string {
	if len(scopes) == 0 {
		return fallback
	}
	return scopes[0]
}
