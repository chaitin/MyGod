package externalauth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"

	appauth "app/internal/application/externalauth"
	"app/internal/application/identityprovider"
)

const (
	dingTalkAppTokenURL        = "https://api.dingtalk.com/v1.0/oauth2/accessToken"
	dingTalkUserIDByUnionIDURL = "https://oapi.dingtalk.com/user/getUseridByUnionid"
	dingTalkUserDetailURL      = "https://oapi.dingtalk.com/topapi/v2/user/get"
)

func (o *OAuth) fetchDingTalkProfile(ctx context.Context, provider identityprovider.Provider, code string) (appauth.Profile, error) {
	accessToken, err := o.exchangeDingTalkCode(ctx, provider, code)
	if err != nil {
		return appauth.Profile{}, err
	}
	requestContext, cancel := context.WithTimeout(ctx, httpTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(
		requestContext, http.MethodGet, identityprovider.StringConfigValue(provider.Config, "userinfo_url"), nil,
	)
	if err != nil {
		return appauth.Profile{}, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("x-acs-dingtalk-access-token", accessToken)
	claims, err := doJSONMapRequest(req)
	if err != nil {
		return appauth.Profile{}, err
	}
	organizationClaims, err := o.fetchDingTalkOrganizationClaims(ctx, provider, claims)
	if err == nil && len(organizationClaims) > 0 {
		claims = mergeClaims(claims, organizationClaims)
	}
	profile, err := profileFromClaims(provider, claims)
	if err != nil {
		return appauth.Profile{}, err
	}
	if organizationName := stringField(organizationClaims, "name"); organizationName != "" {
		profile.Name = organizationName
	}
	if profile.Nickname == "" {
		profile.Nickname = stringField(claims, "nick")
	}
	return profile, nil
}

func (o *OAuth) fetchDingTalkOrganizationClaims(ctx context.Context, provider identityprovider.Provider, userClaims map[string]any) (map[string]any, error) {
	unionID := firstNonEmptyField(userClaims, "unionId", "unionid")
	userID := firstNonEmptyField(userClaims, "userid", "userId")
	if unionID == "" && userID == "" {
		return nil, errors.New("dingtalk user id is empty")
	}
	appTokenURL := firstNonEmpty(identityprovider.StringConfigValue(provider.Config, "app_token_url"), dingTalkAppTokenURL)
	appAccessToken, err := o.exchangeDingTalkAppToken(ctx, provider, appTokenURL)
	if err != nil {
		return nil, err
	}
	if userID == "" {
		endpoint := firstNonEmpty(identityprovider.StringConfigValue(provider.Config, "userid_by_unionid_url"), dingTalkUserIDByUnionIDURL)
		userID, err = o.fetchDingTalkUserIDByUnionID(ctx, endpoint, appAccessToken, unionID)
		if err != nil {
			return nil, err
		}
	}
	endpoint := firstNonEmpty(identityprovider.StringConfigValue(provider.Config, "userdetail_url"), dingTalkUserDetailURL)
	return o.fetchDingTalkUserDetailClaims(ctx, endpoint, appAccessToken, userID)
}

func (o *OAuth) exchangeDingTalkCode(ctx context.Context, provider identityprovider.Provider, code string) (string, error) {
	payload, err := json.Marshal(map[string]string{
		"clientId": provider.ClientID, "clientSecret": provider.ClientSecret,
		"code": code, "grantType": "authorization_code",
	})
	if err != nil {
		return "", err
	}
	requestContext, cancel := context.WithTimeout(ctx, httpTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(
		requestContext, http.MethodPost, identityprovider.StringConfigValue(provider.Config, "token_url"), bytes.NewReader(payload),
	)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	return exchangeToken(req)
}

func (o *OAuth) exchangeDingTalkAppToken(ctx context.Context, provider identityprovider.Provider, endpoint string) (string, error) {
	payload, err := json.Marshal(map[string]string{"appKey": provider.ClientID, "appSecret": provider.ClientSecret})
	if err != nil {
		return "", err
	}
	requestContext, cancel := context.WithTimeout(ctx, httpTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(requestContext, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	return exchangeToken(req)
}

func (o *OAuth) fetchDingTalkUserIDByUnionID(ctx context.Context, endpoint, appAccessToken, unionID string) (string, error) {
	userIDURL, err := url.Parse(endpoint)
	if err != nil {
		return "", err
	}
	query := userIDURL.Query()
	query.Set("access_token", appAccessToken)
	query.Set("unionid", unionID)
	userIDURL.RawQuery = query.Encode()
	requestContext, cancel := context.WithTimeout(ctx, httpTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(requestContext, http.MethodGet, userIDURL.String(), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/json")
	claims, err := doJSONMapRequest(req)
	if err != nil {
		return "", err
	}
	if errCode := int64Field(claims, "errcode"); errCode != 0 {
		return "", fmt.Errorf("dingtalk userid error %d", errCode)
	}
	userID := firstNonEmptyField(claims, "userid", "userId")
	if userID == "" {
		return "", errors.New("dingtalk userid is empty")
	}
	return userID, nil
}

func (o *OAuth) fetchDingTalkUserDetailClaims(ctx context.Context, endpoint, appAccessToken, userID string) (map[string]any, error) {
	userDetailURL, err := url.Parse(endpoint)
	if err != nil {
		return nil, err
	}
	query := userDetailURL.Query()
	query.Set("access_token", appAccessToken)
	userDetailURL.RawQuery = query.Encode()
	payload, err := json.Marshal(map[string]string{"userid": userID, "language": "zh_CN"})
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
		return nil, fmt.Errorf("dingtalk userdetail error %d", errCode)
	}
	if result, ok := claims["result"].(map[string]any); ok && len(result) > 0 {
		return result, nil
	}
	return claims, nil
}
