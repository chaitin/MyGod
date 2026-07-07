package httpserver

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"app/internal/auth"
	"app/internal/store"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	thirdPartyHTTPTimeout          = 10 * time.Second
	thirdPartyLoginStateCookieName = "third_party_login_state"
	thirdPartyLoginStateTTL        = 10 * time.Minute
	dingTalkAppTokenURL            = "https://api.dingtalk.com/v1.0/oauth2/accessToken"
	dingTalkUserIDByUnionIDURL     = "https://oapi.dingtalk.com/user/getUseridByUnionid"
	dingTalkUserDetailURL          = "https://oapi.dingtalk.com/topapi/v2/user/get"
)

type thirdPartyTokenResponse struct {
	AccessToken      string `json:"access_token"`
	AccessTokenCamel string `json:"accessToken"`
	UserAccessToken  string `json:"user_access_token"`
	TokenType        string `json:"token_type"`
	Data             *struct {
		AccessToken     string `json:"access_token"`
		UserAccessToken string `json:"user_access_token"`
	} `json:"data"`
}

func (response thirdPartyTokenResponse) token() string {
	for _, value := range []string{
		response.AccessToken,
		response.AccessTokenCamel,
		response.UserAccessToken,
	} {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	if response.Data != nil {
		for _, value := range []string{
			response.Data.AccessToken,
			response.Data.UserAccessToken,
		} {
			if strings.TrimSpace(value) != "" {
				return strings.TrimSpace(value)
			}
		}
	}

	return ""
}

type weComTokenResponse struct {
	ErrCode     int64  `json:"errcode"`
	ErrMessage  string `json:"errmsg"`
	AccessToken string `json:"access_token"`
}

type externalUserProfile struct {
	ExternalUserID string
	Email          string
	Name           string
	Nickname       string
	Phone          string
	Avatar         string
	Raw            json.RawMessage
}

type thirdPartyUserError struct {
	status  int
	code    string
	message string
}

func (err thirdPartyUserError) Error() string {
	return err.message
}

// startThirdPartyLogin godoc
//
// @Summary 发起第三方登录
// @Description 根据登录方式 Key 创建第三方登录状态，并重定向到第三方授权地址。
// @Tags 客户端认证
// @Produce json
// @Param key path string true "第三方登录方式 Key"
// @Param redirect query string false "登录成功后的站内跳转路径"
// @Success 302
// @Failure 400 {object} errorEnvelope
// @Failure 404 {object} errorEnvelope
// @Failure 500 {object} errorEnvelope
// @Router /api/client/auth/third-party/{key}/start [get]
func (s *Server) startThirdPartyLogin(c echo.Context) error {
	provider, err := s.findEnabledThirdPartyProviderByKey(c.Param("key"))
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return failure(c, http.StatusNotFound, "not_found", "第三方登录方式不存在")
	}
	if err != nil {
		return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
	}

	redirectPath, err := normalizeThirdPartyRedirectPath(c.QueryParam("redirect"))
	if err != nil {
		return failure(c, http.StatusBadRequest, "invalid_request", "登录跳转地址格式错误")
	}

	state, err := generateThirdPartyRandomValue(32)
	if err != nil {
		return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
	}
	codeVerifier, err := generateThirdPartyRandomValue(32)
	if err != nil {
		return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
	}

	now := time.Now().UTC()
	if err := s.cleanupThirdPartyLoginStates(now); err != nil {
		return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
	}
	loginState := store.ThirdPartyLoginState{
		StateHash:    auth.HashSessionToken(state),
		ProviderID:   provider.ID,
		CodeVerifier: codeVerifier,
		RedirectPath: redirectPath,
		ExpiresAt:    now.Add(thirdPartyLoginStateTTL),
		IP:           c.RealIP(),
		UserAgent:    c.Request().UserAgent(),
	}
	if err := s.db.Create(&loginState).Error; err != nil {
		return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
	}
	setThirdPartyLoginStateCookie(c, state, loginState.ExpiresAt)

	authorizeURL, err := buildThirdPartyAuthorizeURL(provider, state, thirdPartyCallbackURL(c, provider.Key), codeVerifier)
	if err != nil {
		return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
	}

	return c.Redirect(http.StatusFound, authorizeURL)
}

// finishThirdPartyLogin godoc
//
// @Summary 完成第三方登录
// @Description 处理第三方登录回调，创建或关联普通用户并写入用户登录 Session。
// @Tags 客户端认证
// @Produce json
// @Param key path string true "第三方登录方式 Key"
// @Param code query string true "第三方授权码"
// @Param state query string true "第三方登录状态"
// @Success 302
// @Failure 400 {object} errorEnvelope
// @Failure 401 {object} errorEnvelope
// @Failure 404 {object} errorEnvelope
// @Failure 409 {object} errorEnvelope
// @Failure 500 {object} errorEnvelope
// @Router /api/client/auth/third-party/{key}/callback [get]
func (s *Server) finishThirdPartyLogin(c echo.Context) error {
	defer clearThirdPartyLoginStateCookie(c)

	provider, err := s.findEnabledThirdPartyProviderByKey(c.Param("key"))
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return failure(c, http.StatusNotFound, "not_found", "第三方登录方式不存在")
	}
	if err != nil {
		return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
	}

	code := thirdPartyCallbackCode(c)
	state := strings.TrimSpace(c.QueryParam("state"))
	if code == "" || state == "" {
		return failure(c, http.StatusBadRequest, "invalid_request", "第三方登录回调参数错误")
	}
	if !validateThirdPartyLoginStateCookie(c, state) {
		return failure(c, http.StatusBadRequest, "invalid_request", "第三方登录状态已失效")
	}

	loginState, err := s.consumeThirdPartyLoginState(provider.ID, state)
	if err != nil {
		return failure(c, http.StatusBadRequest, "invalid_request", "第三方登录状态已失效")
	}

	profile, err := fetchThirdPartyUserProfile(
		c.Request().Context(),
		provider,
		code,
		thirdPartyCallbackURL(c, provider.Key),
		loginState.CodeVerifier,
	)
	if err != nil {
		return failure(c, http.StatusUnauthorized, "invalid_third_party_login", "第三方登录失败")
	}

	user, err := s.findOrCreateThirdPartyUser(provider, profile)
	if err != nil {
		var userErr thirdPartyUserError
		if errors.As(err, &userErr) {
			return failure(c, userErr.status, userErr.code, userErr.message)
		}

		return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
	}
	if err := s.createUserSession(c, user); err != nil {
		return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
	}

	return c.Redirect(http.StatusFound, loginState.RedirectPath)
}

func (s *Server) findEnabledThirdPartyProviderByKey(key string) (store.ThirdPartyLoginProvider, error) {
	var provider store.ThirdPartyLoginProvider
	err := s.db.Where("key = ? AND enabled = ?", strings.TrimSpace(key), true).First(&provider).Error
	return provider, err
}

func (s *Server) consumeThirdPartyLoginState(providerID string, state string) (store.ThirdPartyLoginState, error) {
	stateHash := auth.HashSessionToken(state)
	now := time.Now().UTC()
	var loginState store.ThirdPartyLoginState

	result := s.db.
		Model(&loginState).
		Clauses(clause.Returning{}).
		Where(
			"state_hash = ? AND provider_id = ? AND consumed_at IS NULL AND expires_at > ?",
			stateHash,
			providerID,
			now,
		).
		Update("consumed_at", now)
	if result.Error != nil {
		return store.ThirdPartyLoginState{}, result.Error
	}
	if result.RowsAffected == 0 {
		return store.ThirdPartyLoginState{}, gorm.ErrRecordNotFound
	}

	return loginState, nil
}

func (s *Server) cleanupThirdPartyLoginStates(now time.Time) error {
	consumedBefore := now.Add(-thirdPartyLoginStateTTL)
	return s.db.
		Where("expires_at <= ? OR (consumed_at IS NOT NULL AND consumed_at <= ?)", now, consumedBefore).
		Delete(&store.ThirdPartyLoginState{}).
		Error
}

func buildThirdPartyAuthorizeURL(provider store.ThirdPartyLoginProvider, state string, redirectURI string, codeVerifier string) (string, error) {
	config, err := parseThirdPartyProviderConfig(provider.Config)
	if err != nil {
		return "", err
	}
	scopes, err := parseThirdPartyScopes(provider.Scopes)
	if err != nil {
		return "", err
	}
	authorizeURL, err := url.Parse(stringConfigValue(config, "authorize_url"))
	if err != nil {
		return "", err
	}
	query := authorizeURL.Query()
	query.Set("redirect_uri", redirectURI)
	query.Set("state", state)

	if provider.Type == store.ThirdPartyLoginProviderTypeWeCom {
		buildWeComAuthorizeQuery(authorizeURL, query, provider, config, scopes)
	} else {
		query.Set("response_type", "code")
		query.Set("client_id", provider.ClientID)
		if len(scopes) > 0 {
			query.Set("scope", strings.Join(scopes, " "))
		}
		if thirdPartyProviderUsesPKCE(provider.Type) {
			query.Set("code_challenge", thirdPartyCodeChallenge(codeVerifier))
			query.Set("code_challenge_method", "S256")
		}
	}
	authorizeURL.RawQuery = query.Encode()

	return authorizeURL.String(), nil
}

func buildWeComAuthorizeQuery(authorizeURL *url.URL, query url.Values, provider store.ThirdPartyLoginProvider, config map[string]any, scopes []string) {
	query.Set("appid", provider.ClientID)
	query.Set("agentid", stringConfigValue(config, "agent_id"))
	if weComUsesWebLogin(authorizeURL, config) {
		query.Set("login_type", firstNonEmptyString(stringConfigValue(config, "login_type"), "CorpApp"))
		return
	}

	query.Set("response_type", "code")
	query.Set("scope", firstScopeOrDefault(scopes, "snsapi_base"))
	authorizeURL.Fragment = "wechat_redirect"
}

func weComUsesWebLogin(authorizeURL *url.URL, config map[string]any) bool {
	if stringConfigValue(config, "login_type") != "" {
		return true
	}

	return authorizeURL.Host == "login.work.weixin.qq.com" || strings.Contains(authorizeURL.Path, "/wwlogin/sso/login")
}

func thirdPartyProviderUsesPKCE(providerType string) bool {
	switch providerType {
	case store.ThirdPartyLoginProviderTypeOIDC,
		store.ThirdPartyLoginProviderTypeGoogle,
		store.ThirdPartyLoginProviderTypeFeishu:
		return true
	default:
		return false
	}
}

func firstScopeOrDefault(scopes []string, fallback string) string {
	if len(scopes) == 0 {
		return fallback
	}
	return scopes[0]
}

func fetchThirdPartyUserProfile(ctx context.Context, provider store.ThirdPartyLoginProvider, code string, redirectURI string, codeVerifier string) (externalUserProfile, error) {
	switch provider.Type {
	case store.ThirdPartyLoginProviderTypeDingTalk:
		return fetchDingTalkUserProfile(ctx, provider, code, redirectURI)
	case store.ThirdPartyLoginProviderTypeWeCom:
		return fetchWeComUserProfile(ctx, provider, code)
	case store.ThirdPartyLoginProviderTypeFeishu:
		return fetchFeishuUserProfile(ctx, provider, code, redirectURI, codeVerifier)
	case store.ThirdPartyLoginProviderTypeGitHub:
		return fetchGitHubUserProfile(ctx, provider, code, redirectURI)
	default:
		return fetchBearerUserProfile(ctx, provider, code, redirectURI, codeVerifier)
	}
}

func fetchBearerUserProfile(ctx context.Context, provider store.ThirdPartyLoginProvider, code string, redirectURI string, codeVerifier string) (externalUserProfile, error) {
	accessToken, err := exchangeFormOAuthCode(ctx, provider, code, redirectURI, codeVerifier)
	if err != nil {
		return externalUserProfile{}, err
	}
	claims, err := fetchJSONWithBearer(ctx, provider, stringConfigFromProvider(provider, "userinfo_url"), accessToken)
	if err != nil {
		return externalUserProfile{}, err
	}
	claims = unwrapThirdPartyDataClaims(claims)

	return profileFromClaims(provider, claims)
}

func fetchFeishuUserProfile(ctx context.Context, provider store.ThirdPartyLoginProvider, code string, redirectURI string, codeVerifier string) (externalUserProfile, error) {
	accessToken, err := exchangeJSONOAuthCode(ctx, provider, code, redirectURI, codeVerifier)
	if err != nil {
		return externalUserProfile{}, err
	}
	claims, err := fetchJSONWithBearer(ctx, provider, stringConfigFromProvider(provider, "userinfo_url"), accessToken)
	if err != nil {
		return externalUserProfile{}, err
	}
	claims = unwrapThirdPartyDataClaims(claims)

	return profileFromClaims(provider, claims)
}

func fetchGitHubUserProfile(ctx context.Context, provider store.ThirdPartyLoginProvider, code string, redirectURI string) (externalUserProfile, error) {
	accessToken, err := exchangeFormOAuthCode(ctx, provider, code, redirectURI, "")
	if err != nil {
		return externalUserProfile{}, err
	}
	config, err := parseThirdPartyProviderConfig(provider.Config)
	if err != nil {
		return externalUserProfile{}, err
	}
	claims, err := fetchJSONWithBearer(ctx, provider, stringConfigValue(config, "userinfo_url"), accessToken)
	if err != nil {
		return externalUserProfile{}, err
	}
	if stringFieldFromClaims(claims, "email") == "" && stringConfigValue(config, "emails_url") != "" {
		if email := fetchGitHubPrimaryEmail(ctx, stringConfigValue(config, "emails_url"), accessToken); email != "" {
			claims["email"] = email
		}
	}

	return profileFromClaims(provider, claims)
}

func fetchDingTalkUserProfile(ctx context.Context, provider store.ThirdPartyLoginProvider, code string, redirectURI string) (externalUserProfile, error) {
	config, err := parseThirdPartyProviderConfig(provider.Config)
	if err != nil {
		return externalUserProfile{}, err
	}
	accessToken, err := exchangeDingTalkCode(ctx, provider, code)
	if err != nil {
		return externalUserProfile{}, err
	}
	requestContext, cancel := context.WithTimeout(ctx, thirdPartyHTTPTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(requestContext, http.MethodGet, stringConfigValue(config, "userinfo_url"), nil)
	if err != nil {
		return externalUserProfile{}, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("x-acs-dingtalk-access-token", accessToken)

	claims, err := doJSONMapRequest(req)
	if err != nil {
		return externalUserProfile{}, err
	}
	organizationClaims, err := fetchDingTalkOrganizationUserClaims(ctx, provider, config, claims)
	if err == nil && len(organizationClaims) > 0 {
		claims = mergeThirdPartyClaims(claims, organizationClaims)
	}

	profile, err := profileFromClaims(provider, claims)
	if err != nil {
		return externalUserProfile{}, err
	}
	if organizationName := stringFieldFromClaims(organizationClaims, "name"); organizationName != "" {
		profile.Name = organizationName
	}
	if profile.Nickname == "" {
		profile.Nickname = stringFieldFromClaims(claims, "nick")
	}

	return profile, nil
}

func fetchDingTalkOrganizationUserClaims(ctx context.Context, provider store.ThirdPartyLoginProvider, config map[string]any, userClaims map[string]any) (map[string]any, error) {
	unionID := firstNonEmptyStringField(userClaims, "unionId", "unionid")
	userID := firstNonEmptyStringField(userClaims, "userid", "userId")
	if unionID == "" && userID == "" {
		return nil, errors.New("dingtalk user id is empty")
	}

	appAccessToken, err := exchangeDingTalkAppToken(ctx, provider, firstNonEmptyString(stringConfigValue(config, "app_token_url"), dingTalkAppTokenURL))
	if err != nil {
		return nil, err
	}
	if userID == "" {
		userID, err = fetchDingTalkUserIDByUnionID(
			ctx,
			firstNonEmptyString(stringConfigValue(config, "userid_by_unionid_url"), dingTalkUserIDByUnionIDURL),
			appAccessToken,
			unionID,
		)
		if err != nil {
			return nil, err
		}
	}

	return fetchDingTalkUserDetailClaims(
		ctx,
		firstNonEmptyString(stringConfigValue(config, "userdetail_url"), dingTalkUserDetailURL),
		appAccessToken,
		userID,
	)
}

func exchangeDingTalkAppToken(ctx context.Context, provider store.ThirdPartyLoginProvider, endpoint string) (string, error) {
	payload, err := json.Marshal(map[string]string{
		"appKey":    provider.ClientID,
		"appSecret": provider.ClientSecret,
	})
	if err != nil {
		return "", err
	}

	requestContext, cancel := context.WithTimeout(ctx, thirdPartyHTTPTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(requestContext, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	return exchangeThirdPartyToken(req)
}

func fetchDingTalkUserIDByUnionID(ctx context.Context, endpoint string, appAccessToken string, unionID string) (string, error) {
	userIDURL, err := url.Parse(endpoint)
	if err != nil {
		return "", err
	}
	query := userIDURL.Query()
	query.Set("access_token", appAccessToken)
	query.Set("unionid", unionID)
	userIDURL.RawQuery = query.Encode()

	requestContext, cancel := context.WithTimeout(ctx, thirdPartyHTTPTimeout)
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
	if errCode := int64FieldFromClaims(claims, "errcode"); errCode != 0 {
		return "", fmt.Errorf("dingtalk userid error %d", errCode)
	}
	userID := firstNonEmptyStringField(claims, "userid", "userId")
	if userID == "" {
		return "", errors.New("dingtalk userid is empty")
	}

	return userID, nil
}

func fetchDingTalkUserDetailClaims(ctx context.Context, endpoint string, appAccessToken string, userID string) (map[string]any, error) {
	userDetailURL, err := url.Parse(endpoint)
	if err != nil {
		return nil, err
	}
	query := userDetailURL.Query()
	query.Set("access_token", appAccessToken)
	userDetailURL.RawQuery = query.Encode()

	payload, err := json.Marshal(map[string]string{
		"userid":   userID,
		"language": "zh_CN",
	})
	if err != nil {
		return nil, err
	}

	requestContext, cancel := context.WithTimeout(ctx, thirdPartyHTTPTimeout)
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
	if errCode := int64FieldFromClaims(claims, "errcode"); errCode != 0 {
		return nil, fmt.Errorf("dingtalk userdetail error %d", errCode)
	}
	result, ok := claims["result"].(map[string]any)
	if ok && len(result) > 0 {
		return result, nil
	}

	return claims, nil
}

func fetchWeComUserProfile(ctx context.Context, provider store.ThirdPartyLoginProvider, code string) (externalUserProfile, error) {
	config, err := parseThirdPartyProviderConfig(provider.Config)
	if err != nil {
		return externalUserProfile{}, err
	}

	tokenURL, err := url.Parse(stringConfigValue(config, "token_url"))
	if err != nil {
		return externalUserProfile{}, err
	}
	tokenQuery := tokenURL.Query()
	tokenQuery.Set("corpid", provider.ClientID)
	tokenQuery.Set("corpsecret", provider.ClientSecret)
	tokenURL.RawQuery = tokenQuery.Encode()

	requestContext, cancel := context.WithTimeout(ctx, thirdPartyHTTPTimeout)
	defer cancel()
	tokenReq, err := http.NewRequestWithContext(requestContext, http.MethodGet, tokenURL.String(), nil)
	if err != nil {
		return externalUserProfile{}, err
	}
	tokenReq.Header.Set("Accept", "application/json")
	tokenResp, err := doJSONRequest[weComTokenResponse](tokenReq)
	if err != nil {
		return externalUserProfile{}, err
	}
	if tokenResp.ErrCode != 0 || strings.TrimSpace(tokenResp.AccessToken) == "" {
		return externalUserProfile{}, fmt.Errorf("wecom token error %d %s", tokenResp.ErrCode, tokenResp.ErrMessage)
	}

	userinfoURL, err := url.Parse(stringConfigValue(config, "userinfo_url"))
	if err != nil {
		return externalUserProfile{}, err
	}
	userinfoQuery := userinfoURL.Query()
	userinfoQuery.Set("access_token", tokenResp.AccessToken)
	userinfoQuery.Set("code", code)
	userinfoURL.RawQuery = userinfoQuery.Encode()

	userinfoContext, userinfoCancel := context.WithTimeout(ctx, thirdPartyHTTPTimeout)
	defer userinfoCancel()
	userinfoReq, err := http.NewRequestWithContext(userinfoContext, http.MethodGet, userinfoURL.String(), nil)
	if err != nil {
		return externalUserProfile{}, err
	}
	userinfoReq.Header.Set("Accept", "application/json")
	claims, err := doJSONMapRequest(userinfoReq)
	if err != nil {
		return externalUserProfile{}, err
	}
	if errCode := int64FieldFromClaims(claims, "errcode"); errCode != 0 {
		return externalUserProfile{}, fmt.Errorf("wecom userinfo error %d", errCode)
	}

	externalID := firstNonEmptyStringField(claims, "userid", "openid", "external_userid")
	if externalID == "" {
		return externalUserProfile{}, errors.New("wecom external user id is empty")
	}
	if userTicket := stringFieldFromClaims(claims, "user_ticket"); userTicket != "" && stringConfigValue(config, "userdetail_url") != "" {
		if detailClaims, err := fetchWeComUserDetail(ctx, stringConfigValue(config, "userdetail_url"), tokenResp.AccessToken, userTicket); err == nil {
			claims = mergeThirdPartyClaims(claims, detailClaims)
		}
	}
	raw, err := json.Marshal(claims)
	if err != nil {
		return externalUserProfile{}, err
	}

	return externalUserProfile{
		ExternalUserID: externalID,
		Email:          firstNonEmptyStringField(claims, "email", "biz_mail"),
		Name:           firstNonEmptyStringField(claims, "name", "userid", "openid", "external_userid"),
		Nickname:       firstNonEmptyStringField(claims, "alias", "name", "userid", "openid", "external_userid"),
		Phone:          stringFieldFromClaims(claims, "mobile"),
		Avatar:         stringFieldFromClaims(claims, "avatar"),
		Raw:            raw,
	}, nil
}

func fetchWeComUserDetail(ctx context.Context, endpoint string, accessToken string, userTicket string) (map[string]any, error) {
	userdetailURL, err := url.Parse(endpoint)
	if err != nil {
		return nil, err
	}
	query := userdetailURL.Query()
	query.Set("access_token", accessToken)
	userdetailURL.RawQuery = query.Encode()

	payload, err := json.Marshal(map[string]string{
		"user_ticket": userTicket,
	})
	if err != nil {
		return nil, err
	}

	requestContext, cancel := context.WithTimeout(ctx, thirdPartyHTTPTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(requestContext, http.MethodPost, userdetailURL.String(), bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	claims, err := doJSONMapRequest(req)
	if err != nil {
		return nil, err
	}
	if errCode := int64FieldFromClaims(claims, "errcode"); errCode != 0 {
		return nil, fmt.Errorf("wecom userdetail error %d", errCode)
	}

	return claims, nil
}

func exchangeFormOAuthCode(ctx context.Context, provider store.ThirdPartyLoginProvider, code string, redirectURI string, codeVerifier string) (string, error) {
	config, err := parseThirdPartyProviderConfig(provider.Config)
	if err != nil {
		return "", err
	}
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", redirectURI)
	form.Set("client_id", provider.ClientID)
	form.Set("client_secret", provider.ClientSecret)
	if codeVerifier != "" && thirdPartyProviderUsesPKCE(provider.Type) {
		form.Set("code_verifier", codeVerifier)
	}

	requestContext, cancel := context.WithTimeout(ctx, thirdPartyHTTPTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(requestContext, http.MethodPost, stringConfigValue(config, "token_url"), strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	return exchangeThirdPartyToken(req)
}

func exchangeJSONOAuthCode(ctx context.Context, provider store.ThirdPartyLoginProvider, code string, redirectURI string, codeVerifier string) (string, error) {
	config, err := parseThirdPartyProviderConfig(provider.Config)
	if err != nil {
		return "", err
	}
	payload := map[string]string{
		"grant_type":    "authorization_code",
		"code":          code,
		"redirect_uri":  redirectURI,
		"client_id":     provider.ClientID,
		"client_secret": provider.ClientSecret,
	}
	if codeVerifier != "" && thirdPartyProviderUsesPKCE(provider.Type) {
		payload["code_verifier"] = codeVerifier
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	requestContext, cancel := context.WithTimeout(ctx, thirdPartyHTTPTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(requestContext, http.MethodPost, stringConfigValue(config, "token_url"), bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Accept", "application/json")

	return exchangeThirdPartyToken(req)
}

func exchangeDingTalkCode(ctx context.Context, provider store.ThirdPartyLoginProvider, code string) (string, error) {
	config, err := parseThirdPartyProviderConfig(provider.Config)
	if err != nil {
		return "", err
	}
	payload, err := json.Marshal(map[string]string{
		"clientId":     provider.ClientID,
		"clientSecret": provider.ClientSecret,
		"code":         code,
		"grantType":    "authorization_code",
	})
	if err != nil {
		return "", err
	}

	requestContext, cancel := context.WithTimeout(ctx, thirdPartyHTTPTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(requestContext, http.MethodPost, stringConfigValue(config, "token_url"), bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	return exchangeThirdPartyToken(req)
}

func exchangeThirdPartyToken(req *http.Request) (string, error) {
	token, err := doJSONRequest[thirdPartyTokenResponse](req)
	if err != nil {
		return "", err
	}
	accessToken := token.token()
	if accessToken == "" {
		return "", errors.New("access token is empty")
	}

	return accessToken, nil
}

func fetchJSONWithBearer(ctx context.Context, provider store.ThirdPartyLoginProvider, endpoint string, accessToken string) (map[string]any, error) {
	requestContext, cancel := context.WithTimeout(ctx, thirdPartyHTTPTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(requestContext, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)
	if provider.Type == store.ThirdPartyLoginProviderTypeGitHub {
		req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	}

	return doJSONMapRequest(req)
}

func fetchGitHubPrimaryEmail(ctx context.Context, endpoint string, accessToken string) string {
	requestContext, cancel := context.WithTimeout(ctx, thirdPartyHTTPTimeout)
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
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ""
	}

	var emails []map[string]any
	if err := decodeJSONBody(resp.Body, &emails); err != nil {
		return ""
	}
	var fallback string
	for _, email := range emails {
		value := stringFieldFromClaims(email, "email")
		if value == "" || !boolFieldFromClaims(email, "verified") {
			continue
		}
		if fallback == "" {
			fallback = value
		}
		if boolFieldFromClaims(email, "primary") {
			return value
		}
	}

	return fallback
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
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return zero, fmt.Errorf("%s status %d", req.URL.String(), resp.StatusCode)
	}

	var decoded T
	if err := decodeJSONBody(resp.Body, &decoded); err != nil {
		return zero, err
	}

	return decoded, nil
}

func profileFromClaims(provider store.ThirdPartyLoginProvider, claims map[string]any) (externalUserProfile, error) {
	config, err := parseThirdPartyProviderConfig(provider.Config)
	if err != nil {
		return externalUserProfile{}, err
	}
	externalID := stringFieldFromClaims(claims, stringConfigValue(config, "external_id_field"))
	if externalID == "" {
		externalID = fallbackExternalUserID(provider.Type, claims)
	}
	if externalID == "" {
		return externalUserProfile{}, errors.New("external user id is empty")
	}

	raw, err := json.Marshal(claims)
	if err != nil {
		return externalUserProfile{}, err
	}

	return externalUserProfile{
		ExternalUserID: externalID,
		Email:          stringFieldFromClaims(claims, stringConfigValue(config, "email_field")),
		Name:           stringFieldFromClaims(claims, stringConfigValue(config, "name_field")),
		Nickname:       stringFieldFromClaims(claims, stringConfigValue(config, "nickname_field")),
		Phone:          stringFieldFromClaims(claims, stringConfigValue(config, "phone_field")),
		Avatar:         stringFieldFromClaims(claims, stringConfigValue(config, "avatar_field")),
		Raw:            raw,
	}, nil
}

func fallbackExternalUserID(providerType string, claims map[string]any) string {
	switch providerType {
	case store.ThirdPartyLoginProviderTypeDingTalk:
		return firstNonEmptyStringField(claims, "unionId", "openId", "userid", "userId")
	case store.ThirdPartyLoginProviderTypeFeishu:
		return firstNonEmptyStringField(claims, "union_id", "open_id", "user_id")
	case store.ThirdPartyLoginProviderTypeGitHub:
		return firstNonEmptyStringField(claims, "id", "node_id", "login")
	case store.ThirdPartyLoginProviderTypeGoogle:
		return stringFieldFromClaims(claims, "sub")
	default:
		return firstNonEmptyStringField(claims, "sub", "id", "user_id", "open_id", "union_id")
	}
}

func unwrapThirdPartyDataClaims(claims map[string]any) map[string]any {
	data, ok := claims["data"].(map[string]any)
	if !ok || len(data) == 0 {
		return claims
	}

	return data
}

func mergeThirdPartyClaims(base map[string]any, extra map[string]any) map[string]any {
	for key, value := range extra {
		base[key] = value
	}

	return base
}

func (s *Server) findOrCreateThirdPartyUser(provider store.ThirdPartyLoginProvider, profile externalUserProfile) (store.User, error) {
	if strings.TrimSpace(profile.ExternalUserID) == "" {
		return store.User{}, thirdPartyUserError{status: http.StatusBadRequest, code: "invalid_third_party_login", message: "第三方用户标识为空"}
	}
	if len(profile.Raw) == 0 {
		profile.Raw = json.RawMessage(`{}`)
	}

	var resultUser store.User
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var account store.ThirdPartyAccount
		err := tx.Preload("User").
			Where("provider_id = ? AND external_user_id = ?", provider.ID, profile.ExternalUserID).
			First(&account).Error
		if err == nil {
			if account.User.ID == "" {
				return thirdPartyUserError{status: http.StatusInternalServerError, code: "internal_error", message: "服务端错误"}
			}
			if account.User.Status != store.UserStatusActive {
				return thirdPartyUserError{status: http.StatusUnauthorized, code: "invalid_credentials", message: "用户已被禁用"}
			}
			if updateErr := tx.Model(&account).Update("profile", profile.Raw).Error; updateErr != nil {
				return updateErr
			}
			updatedUser, updateErr := refreshThirdPartyBoundUserFromProfile(tx, provider, account.User, profile)
			if updateErr != nil {
				return updateErr
			}
			resultUser = updatedUser
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		user, err := s.findOrCreateThirdPartyBoundUser(tx, provider, profile)
		if err != nil {
			return err
		}
		account = store.ThirdPartyAccount{
			ID:             uuid.NewString(),
			ProviderID:     provider.ID,
			ExternalUserID: profile.ExternalUserID,
			UserID:         user.ID,
			Profile:        profile.Raw,
		}
		if err := tx.Create(&account).Error; err != nil {
			if isUniqueConstraintError(err) {
				return thirdPartyUserError{status: http.StatusConflict, code: "conflict", message: "第三方账号已绑定"}
			}
			return err
		}
		resultUser = user
		return nil
	})
	if err != nil {
		return store.User{}, err
	}

	return resultUser, nil
}

func refreshThirdPartyBoundUserFromProfile(tx *gorm.DB, provider store.ThirdPartyLoginProvider, user store.User, profile externalUserProfile) (store.User, error) {
	if provider.Type != store.ThirdPartyLoginProviderTypeDingTalk {
		return user, nil
	}
	name := strings.TrimSpace(profile.Name)
	if name == "" || name == strings.TrimSpace(user.Name) {
		return user, nil
	}
	if err := tx.Model(&store.User{}).Where("id = ?", user.ID).Update("name", name).Error; err != nil {
		return store.User{}, err
	}
	user.Name = name

	return user, nil
}

func (s *Server) findOrCreateThirdPartyBoundUser(tx *gorm.DB, provider store.ThirdPartyLoginProvider, profile externalUserProfile) (store.User, error) {
	email, emailFromProvider, err := thirdPartyProfileEmail(provider, profile)
	if err != nil {
		return store.User{}, thirdPartyUserError{status: http.StatusBadRequest, code: "invalid_third_party_login", message: "第三方邮箱格式错误"}
	}

	var user store.User
	err = tx.Where("email = ?", email).First(&user).Error
	if err == nil {
		if user.Status != store.UserStatusActive {
			return store.User{}, thirdPartyUserError{status: http.StatusUnauthorized, code: "invalid_credentials", message: "用户已被禁用"}
		}
		return user, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return store.User{}, err
	}

	phone, err := normalizePhone(profile.Phone)
	if err != nil {
		return store.User{}, thirdPartyUserError{status: http.StatusBadRequest, code: "invalid_third_party_login", message: "手机号格式错误"}
	}
	if phone != nil {
		var phoneCount int64
		if err := tx.Model(&store.User{}).Where("phone = ?", *phone).Count(&phoneCount).Error; err != nil {
			return store.User{}, err
		}
		if phoneCount > 0 {
			return store.User{}, thirdPartyUserError{status: http.StatusConflict, code: "conflict", message: "手机号已存在"}
		}
	}

	password, err := auth.GenerateInitialPassword(32)
	if err != nil {
		return store.User{}, err
	}
	passwordHash, err := auth.HashPassword(password)
	if err != nil {
		return store.User{}, err
	}

	name := strings.TrimSpace(profile.Name)
	if name == "" {
		name = strings.TrimSpace(profile.Nickname)
	}
	if name == "" && emailFromProvider {
		name = emailPrefix(email)
	}
	if name == "" {
		name = profile.ExternalUserID
	}
	avatar := normalizeThirdPartyAvatar(profile.Avatar)
	if avatar == "" {
		avatar = randomBuiltinAvatar()
	}
	user = store.User{
		ID:           uuid.NewString(),
		Avatar:       avatar,
		Email:        email,
		Name:         name,
		Nickname:     strings.TrimSpace(profile.Nickname),
		Phone:        phone,
		PasswordHash: passwordHash,
		Status:       store.UserStatusActive,
	}
	if err := tx.Create(&user).Error; err != nil {
		if isUniqueConstraintError(err) {
			return store.User{}, thirdPartyUserError{status: http.StatusConflict, code: "conflict", message: "邮箱或手机号已存在"}
		}
		return store.User{}, err
	}

	return user, nil
}

func thirdPartyProfileEmail(provider store.ThirdPartyLoginProvider, profile externalUserProfile) (string, bool, error) {
	rawEmail := strings.TrimSpace(profile.Email)
	if rawEmail != "" {
		email, err := normalizeEmail(rawEmail)
		return email, true, err
	}

	return syntheticThirdPartyEmail(provider, profile.ExternalUserID), false, nil
}

func syntheticThirdPartyEmail(provider store.ThirdPartyLoginProvider, externalUserID string) string {
	sum := sha256.Sum256([]byte(provider.ID + ":" + externalUserID))
	prefix := sanitizeEmailLocalPart(provider.Key)
	if prefix == "" {
		prefix = "third-party"
	}

	return fmt.Sprintf("%s.%s@third-party.local", prefix, hex.EncodeToString(sum[:8]))
}

func sanitizeEmailLocalPart(value string) string {
	var builder strings.Builder
	for _, currentRune := range strings.ToLower(value) {
		switch {
		case currentRune >= 'a' && currentRune <= 'z':
			builder.WriteRune(currentRune)
		case currentRune >= '0' && currentRune <= '9':
			builder.WriteRune(currentRune)
		case currentRune == '.' || currentRune == '_' || currentRune == '+' || currentRune == '-':
			builder.WriteRune(currentRune)
		default:
			builder.WriteByte('-')
		}
	}

	return strings.Trim(builder.String(), ".-+_")
}

func (s *Server) createUserSession(c echo.Context, user store.User) error {
	token, err := auth.GenerateSessionToken()
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	session := store.UserSession{
		ID:         uuid.NewString(),
		TokenHash:  auth.HashSessionToken(token),
		UserID:     user.ID,
		ExpiresAt:  now.Add(sessionTTL),
		LastSeenAt: now,
		UserAgent:  c.Request().UserAgent(),
		IP:         c.RealIP(),
	}
	if err := s.db.Create(&session).Error; err != nil {
		return err
	}

	setSessionCookie(c, userSessionCookieName, token, session.ExpiresAt)
	return nil
}

func setThirdPartyLoginStateCookie(c echo.Context, state string, expiresAt time.Time) {
	c.SetCookie(&http.Cookie{
		Name:     thirdPartyLoginStateCookieName,
		Value:    state,
		Path:     "/api/client/auth/third-party/",
		Expires:  expiresAt,
		MaxAge:   int(time.Until(expiresAt).Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func clearThirdPartyLoginStateCookie(c echo.Context) {
	c.SetCookie(&http.Cookie{
		Name:     thirdPartyLoginStateCookieName,
		Value:    "",
		Path:     "/api/client/auth/third-party/",
		Expires:  time.Unix(0, 0).UTC(),
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func validateThirdPartyLoginStateCookie(c echo.Context, state string) bool {
	cookie, err := c.Cookie(thirdPartyLoginStateCookieName)
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		return false
	}
	stateHash := auth.HashSessionToken(state)
	cookieStateHash := auth.HashSessionToken(cookie.Value)

	return subtle.ConstantTimeCompare([]byte(stateHash), []byte(cookieStateHash)) == 1
}

func thirdPartyCallbackCode(c echo.Context) string {
	if code := strings.TrimSpace(c.QueryParam("code")); code != "" {
		return code
	}

	return strings.TrimSpace(c.QueryParam("authCode"))
}

func decodeJSONBody(body io.Reader, target any) error {
	decoder := json.NewDecoder(body)
	decoder.UseNumber()
	return decoder.Decode(target)
}

func generateThirdPartyRandomValue(size int) (string, error) {
	value := make([]byte, size)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}

	return base64.RawURLEncoding.EncodeToString(value), nil
}

func thirdPartyCodeChallenge(codeVerifier string) string {
	sum := sha256.Sum256([]byte(codeVerifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func thirdPartyCallbackURL(c echo.Context, providerKey string) string {
	return externalRequestBaseURL(c) + "/api/client/auth/third-party/" + url.PathEscape(providerKey) + "/callback"
}

func externalRequestBaseURL(c echo.Context) string {
	req := c.Request()
	scheme := firstForwardedValue(req.Header.Get("X-Forwarded-Proto"))
	if scheme == "" {
		if req.TLS != nil {
			scheme = "https"
		} else {
			scheme = "http"
		}
	}
	host := firstForwardedValue(req.Header.Get("X-Forwarded-Host"))
	if host == "" {
		host = req.Host
	}

	return scheme + "://" + host
}

func firstForwardedValue(value string) string {
	if value == "" {
		return ""
	}
	parts := strings.Split(value, ",")
	return strings.TrimSpace(parts[0])
}

func normalizeThirdPartyRedirectPath(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "/init", nil
	}
	if !strings.HasPrefix(trimmed, "/") || strings.HasPrefix(trimmed, "//") {
		return "", errors.New("invalid redirect path")
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.IsAbs() || parsed.Host != "" {
		return "", errors.New("invalid redirect path")
	}

	return trimmed, nil
}

func stringConfigFromProvider(provider store.ThirdPartyLoginProvider, key string) string {
	config, err := parseThirdPartyProviderConfig(provider.Config)
	if err != nil {
		return ""
	}

	return stringConfigValue(config, key)
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}

	return ""
}

func firstNonEmptyStringField(claims map[string]any, fields ...string) string {
	for _, field := range fields {
		if value := stringFieldFromClaims(claims, field); value != "" {
			return value
		}
	}

	return ""
}

func stringFieldFromClaims(claims map[string]any, field string) string {
	field = strings.TrimSpace(field)
	if field == "" {
		return ""
	}

	var current any = claims
	for _, part := range strings.Split(field, ".") {
		currentMap, ok := current.(map[string]any)
		if !ok {
			return ""
		}
		current = currentMap[part]
	}

	switch value := current.(type) {
	case string:
		return strings.TrimSpace(value)
	case json.Number:
		return value.String()
	case float64:
		return fmt.Sprintf("%.0f", value)
	default:
		return ""
	}
}

func int64FieldFromClaims(claims map[string]any, field string) int64 {
	value := stringFieldFromClaims(claims, field)
	if value == "" {
		return 0
	}
	var number json.Number = json.Number(value)
	parsed, err := number.Int64()
	if err != nil {
		return 0
	}

	return parsed
}

func boolFieldFromClaims(claims map[string]any, field string) bool {
	value, ok := claims[field].(bool)
	return ok && value
}

func emailPrefix(email string) string {
	local, _, ok := strings.Cut(email, "@")
	if !ok || strings.TrimSpace(local) == "" {
		return email
	}

	return local
}

func normalizeThirdPartyAvatar(value string) string {
	trimmed := strings.TrimSpace(value)
	if strings.HasPrefix(trimmed, "https://") || strings.HasPrefix(trimmed, "http://") {
		return trimmed
	}

	return ""
}
