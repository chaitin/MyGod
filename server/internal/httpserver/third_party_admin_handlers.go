package httpserver

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"unicode"

	"app/internal/store"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
)

type thirdPartyProviderRequest struct {
	Name         string         `json:"name"`
	Type         string         `json:"type"`
	ClientID     string         `json:"client_id"`
	ClientSecret string         `json:"client_secret"`
	Scopes       []string       `json:"scopes"`
	Config       map[string]any `json:"config"`
}

type thirdPartyProviderMoveRequest struct {
	Direction string `json:"direction"`
}

type thirdPartyProviderResponse struct {
	ID           string         `json:"id"`
	Name         string         `json:"name"`
	Key          string         `json:"key"`
	CallbackURL  string         `json:"callback_url"`
	Type         string         `json:"type"`
	Enabled      bool           `json:"enabled"`
	ClientID     string         `json:"client_id"`
	ClientSecret string         `json:"client_secret"`
	Scopes       []string       `json:"scopes"`
	Config       map[string]any `json:"config"`
	SortOrder    int            `json:"sort_order"`
}

type listThirdPartyProvidersResponse struct {
	Providers []thirdPartyProviderResponse `json:"providers"`
}

type thirdPartyProviderEnvelope struct {
	Provider thirdPartyProviderResponse `json:"provider"`
}

// listThirdPartyProviders godoc
//
// @Summary 列出第三方登录方式
// @Description 管理员读取已配置的第三方登录方式，包含 Client Secret。
// @Tags 管理员第三方登录
// @Produce json
// @Success 200 {object} successEnvelope{data=listThirdPartyProvidersResponse}
// @Failure 401 {object} errorEnvelope
// @Failure 500 {object} errorEnvelope
// @Router /api/admin/third-party/providers [get]
func (s *Server) listThirdPartyProviders(c echo.Context) error {
	var providers []store.ThirdPartyLoginProvider
	if err := s.db.Order("sort_order ASC").Order("name ASC").Order("id ASC").Find(&providers).Error; err != nil {
		return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
	}

	responses := make([]thirdPartyProviderResponse, 0, len(providers))
	for _, provider := range providers {
		response, err := s.newThirdPartyProviderResponse(provider)
		if err != nil {
			return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
		}
		responses = append(responses, response)
	}

	return success(c, http.StatusOK, listThirdPartyProvidersResponse{
		Providers: responses,
	})
}

// createThirdPartyProvider godoc
//
// @Summary 创建第三方登录方式
// @Description 管理员创建一个普通用户可用的第三方登录方式。
// @Tags 管理员第三方登录
// @Accept json
// @Produce json
// @Param body body thirdPartyProviderRequest true "第三方登录方式"
// @Success 201 {object} successEnvelope{data=thirdPartyProviderEnvelope}
// @Failure 400 {object} errorEnvelope
// @Failure 401 {object} errorEnvelope
// @Failure 409 {object} errorEnvelope
// @Failure 500 {object} errorEnvelope
// @Router /api/admin/third-party/providers [post]
func (s *Server) createThirdPartyProvider(c echo.Context) error {
	var req thirdPartyProviderRequest
	if err := c.Bind(&req); err != nil {
		return failure(c, http.StatusBadRequest, "invalid_request", "请求格式错误")
	}

	provider, err := newThirdPartyProviderFromRequest(req)
	if err != nil {
		return failure(c, http.StatusBadRequest, "invalid_request", err.Error())
	}
	provider.ID = uuid.NewString()
	provider.Enabled = true

	key, err := s.generateUniqueThirdPartyProviderKey(provider.Name, provider.Type)
	if err != nil {
		return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
	}
	provider.Key = key

	sortOrder, err := s.nextThirdPartyProviderSortOrder()
	if err != nil {
		return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
	}
	provider.SortOrder = sortOrder

	if err := s.db.Create(&provider).Error; err != nil {
		if isUniqueConstraintError(err) {
			return failure(c, http.StatusConflict, "conflict", "第三方登录方式标识已存在")
		}
		return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
	}

	response, err := s.newThirdPartyProviderResponse(provider)
	if err != nil {
		return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
	}

	return success(c, http.StatusCreated, thirdPartyProviderEnvelope{Provider: response})
}

// updateThirdPartyProvider godoc
//
// @Summary 更新第三方登录方式
// @Description 管理员更新一个第三方登录方式。Client Secret 每次更新都需要提交完整值。
// @Tags 管理员第三方登录
// @Accept json
// @Produce json
// @Param id path string true "第三方登录方式 ID"
// @Param body body thirdPartyProviderRequest true "第三方登录方式"
// @Success 200 {object} successEnvelope{data=thirdPartyProviderEnvelope}
// @Failure 400 {object} errorEnvelope
// @Failure 401 {object} errorEnvelope
// @Failure 404 {object} errorEnvelope
// @Failure 409 {object} errorEnvelope
// @Failure 500 {object} errorEnvelope
// @Router /api/admin/third-party/providers/{id} [put]
func (s *Server) updateThirdPartyProvider(c echo.Context) error {
	id, err := parseUUIDParam(c, "id", "第三方登录方式 ID 格式错误")
	if err != nil {
		return err
	}

	var provider store.ThirdPartyLoginProvider
	err = s.db.First(&provider, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return failure(c, http.StatusNotFound, "not_found", "第三方登录方式不存在")
	}
	if err != nil {
		return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
	}

	var req thirdPartyProviderRequest
	if err := c.Bind(&req); err != nil {
		return failure(c, http.StatusBadRequest, "invalid_request", "请求格式错误")
	}

	updatedProvider, err := newThirdPartyProviderFromRequest(req)
	if err != nil {
		return failure(c, http.StatusBadRequest, "invalid_request", err.Error())
	}
	updates := map[string]any{
		"name":          updatedProvider.Name,
		"type":          updatedProvider.Type,
		"client_id":     updatedProvider.ClientID,
		"client_secret": updatedProvider.ClientSecret,
		"scopes":        updatedProvider.Scopes,
		"config":        updatedProvider.Config,
	}
	if err := s.db.Model(&provider).Updates(updates).Error; err != nil {
		if isUniqueConstraintError(err) {
			return failure(c, http.StatusConflict, "conflict", "第三方登录方式标识已存在")
		}
		return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
	}

	if err := s.db.First(&provider, "id = ?", id).Error; err != nil {
		return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
	}
	response, err := s.newThirdPartyProviderResponse(provider)
	if err != nil {
		return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
	}

	return success(c, http.StatusOK, thirdPartyProviderEnvelope{Provider: response})
}

// enableThirdPartyProvider godoc
//
// @Summary 启用第三方登录方式
// @Description 管理员启用一个第三方登录方式。
// @Tags 管理员第三方登录
// @Produce json
// @Param id path string true "第三方登录方式 ID"
// @Success 200 {object} successEnvelope{data=thirdPartyProviderEnvelope}
// @Failure 400 {object} errorEnvelope
// @Failure 401 {object} errorEnvelope
// @Failure 404 {object} errorEnvelope
// @Failure 500 {object} errorEnvelope
// @Router /api/admin/third-party/providers/{id}/enable [post]
func (s *Server) enableThirdPartyProvider(c echo.Context) error {
	return s.updateThirdPartyProviderEnabled(c, true)
}

// disableThirdPartyProvider godoc
//
// @Summary 禁用第三方登录方式
// @Description 管理员禁用一个第三方登录方式。
// @Tags 管理员第三方登录
// @Produce json
// @Param id path string true "第三方登录方式 ID"
// @Success 200 {object} successEnvelope{data=thirdPartyProviderEnvelope}
// @Failure 400 {object} errorEnvelope
// @Failure 401 {object} errorEnvelope
// @Failure 404 {object} errorEnvelope
// @Failure 500 {object} errorEnvelope
// @Router /api/admin/third-party/providers/{id}/disable [post]
func (s *Server) disableThirdPartyProvider(c echo.Context) error {
	return s.updateThirdPartyProviderEnabled(c, false)
}

// moveThirdPartyProvider godoc
//
// @Summary 移动第三方登录方式
// @Description 管理员将一个第三方登录方式上移或下移，服务端会重新归一化所有登录方式的排序值。
// @Tags 管理员第三方登录
// @Accept json
// @Produce json
// @Param id path string true "第三方登录方式 ID"
// @Param body body thirdPartyProviderMoveRequest true "移动方向"
// @Success 200 {object} successEnvelope{data=listThirdPartyProvidersResponse}
// @Failure 400 {object} errorEnvelope
// @Failure 401 {object} errorEnvelope
// @Failure 404 {object} errorEnvelope
// @Failure 500 {object} errorEnvelope
// @Router /api/admin/third-party/providers/{id}/move [post]
func (s *Server) moveThirdPartyProvider(c echo.Context) error {
	id, err := parseUUIDParam(c, "id", "第三方登录方式 ID 格式错误")
	if err != nil {
		return err
	}

	var req thirdPartyProviderMoveRequest
	if err := c.Bind(&req); err != nil {
		return failure(c, http.StatusBadRequest, "invalid_request", "请求格式错误")
	}
	direction := strings.TrimSpace(req.Direction)
	if direction != "up" && direction != "down" {
		return failure(c, http.StatusBadRequest, "invalid_request", "移动方向只能是 up 或 down")
	}

	var responses []thirdPartyProviderResponse
	err = s.db.Transaction(func(tx *gorm.DB) error {
		var providers []store.ThirdPartyLoginProvider
		if err := tx.Order("sort_order ASC").Order("name ASC").Order("id ASC").Find(&providers).Error; err != nil {
			return err
		}

		index := -1
		for currentIndex, provider := range providers {
			if provider.ID == id {
				index = currentIndex
				break
			}
		}
		if index == -1 {
			return gorm.ErrRecordNotFound
		}

		targetIndex := index
		if direction == "up" && index > 0 {
			targetIndex = index - 1
		}
		if direction == "down" && index < len(providers)-1 {
			targetIndex = index + 1
		}
		providers[index], providers[targetIndex] = providers[targetIndex], providers[index]

		responses = make([]thirdPartyProviderResponse, 0, len(providers))
		for currentIndex := range providers {
			sortOrder := (currentIndex + 1) * 10
			if err := tx.Model(&store.ThirdPartyLoginProvider{}).
				Where("id = ?", providers[currentIndex].ID).
				Update("sort_order", sortOrder).Error; err != nil {
				return err
			}
			providers[currentIndex].SortOrder = sortOrder

			response, err := s.newThirdPartyProviderResponse(providers[currentIndex])
			if err != nil {
				return err
			}
			responses = append(responses, response)
		}

		return nil
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return failure(c, http.StatusNotFound, "not_found", "第三方登录方式不存在")
	}
	if err != nil {
		return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
	}

	return success(c, http.StatusOK, listThirdPartyProvidersResponse{Providers: responses})
}

// deleteThirdPartyProvider godoc
//
// @Summary 删除第三方登录方式
// @Description 管理员删除一个第三方登录方式。
// @Tags 管理员第三方登录
// @Produce json
// @Param id path string true "第三方登录方式 ID"
// @Success 200 {object} successEnvelope
// @Failure 400 {object} errorEnvelope
// @Failure 401 {object} errorEnvelope
// @Failure 404 {object} errorEnvelope
// @Failure 500 {object} errorEnvelope
// @Router /api/admin/third-party/providers/{id} [delete]
func (s *Server) deleteThirdPartyProvider(c echo.Context) error {
	id, err := parseUUIDParam(c, "id", "第三方登录方式 ID 格式错误")
	if err != nil {
		return err
	}

	var provider store.ThirdPartyLoginProvider
	err = s.db.First(&provider, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return failure(c, http.StatusNotFound, "not_found", "第三方登录方式不存在")
	}
	if err != nil {
		return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
	}
	if err := s.db.Delete(&provider).Error; err != nil {
		return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
	}

	return success(c, http.StatusOK, map[string]any{})
}

func (s *Server) updateThirdPartyProviderEnabled(c echo.Context, enabled bool) error {
	id, err := parseUUIDParam(c, "id", "第三方登录方式 ID 格式错误")
	if err != nil {
		return err
	}

	var provider store.ThirdPartyLoginProvider
	err = s.db.First(&provider, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return failure(c, http.StatusNotFound, "not_found", "第三方登录方式不存在")
	}
	if err != nil {
		return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
	}
	if err := s.db.Model(&provider).Update("enabled", enabled).Error; err != nil {
		return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
	}
	provider.Enabled = enabled

	response, err := s.newThirdPartyProviderResponse(provider)
	if err != nil {
		return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
	}

	return success(c, http.StatusOK, thirdPartyProviderEnvelope{Provider: response})
}

func newThirdPartyProviderFromRequest(req thirdPartyProviderRequest) (store.ThirdPartyLoginProvider, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return store.ThirdPartyLoginProvider{}, errors.New("名称不能为空")
	}
	providerType, err := normalizeThirdPartyProviderType(req.Type)
	if err != nil {
		return store.ThirdPartyLoginProvider{}, err
	}
	clientID := strings.TrimSpace(req.ClientID)
	if clientID == "" {
		return store.ThirdPartyLoginProvider{}, errors.New("Client ID 不能为空")
	}
	clientSecret := strings.TrimSpace(req.ClientSecret)
	if clientSecret == "" {
		return store.ThirdPartyLoginProvider{}, errors.New("Client Secret 不能为空")
	}
	scopes, err := normalizeThirdPartyScopes(providerType, req.Scopes)
	if err != nil {
		return store.ThirdPartyLoginProvider{}, err
	}
	scopesJSON, err := json.Marshal(scopes)
	if err != nil {
		return store.ThirdPartyLoginProvider{}, err
	}
	config, err := normalizeThirdPartyProviderConfig(providerType, req.Config)
	if err != nil {
		return store.ThirdPartyLoginProvider{}, err
	}
	configJSON, err := json.Marshal(config)
	if err != nil {
		return store.ThirdPartyLoginProvider{}, err
	}

	return store.ThirdPartyLoginProvider{
		Name:         name,
		Type:         providerType,
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Scopes:       scopesJSON,
		Config:       configJSON,
	}, nil
}

func (s *Server) newThirdPartyProviderResponse(provider store.ThirdPartyLoginProvider) (thirdPartyProviderResponse, error) {
	scopes, err := parseThirdPartyScopes(provider.Scopes)
	if err != nil {
		return thirdPartyProviderResponse{}, err
	}
	config, err := parseThirdPartyProviderConfig(provider.Config)
	if err != nil {
		return thirdPartyProviderResponse{}, err
	}

	return thirdPartyProviderResponse{
		ID:           provider.ID,
		Name:         provider.Name,
		Key:          provider.Key,
		CallbackURL:  thirdPartyCallbackURLForHostname(s.cfg.Server.ClientHostname, provider.Key),
		Type:         provider.Type,
		Enabled:      provider.Enabled,
		ClientID:     provider.ClientID,
		ClientSecret: provider.ClientSecret,
		Scopes:       scopes,
		Config:       config,
		SortOrder:    provider.SortOrder,
	}, nil
}

func thirdPartyCallbackURLForHostname(hostname string, providerKey string) string {
	return "https://" + strings.TrimSpace(hostname) + "/api/client/auth/third-party/" + url.PathEscape(providerKey) + "/callback"
}

func normalizeThirdPartyProviderType(providerType string) (string, error) {
	switch strings.TrimSpace(strings.ToLower(providerType)) {
	case store.ThirdPartyLoginProviderTypeDingTalk:
		return store.ThirdPartyLoginProviderTypeDingTalk, nil
	case store.ThirdPartyLoginProviderTypeWeCom:
		return store.ThirdPartyLoginProviderTypeWeCom, nil
	case store.ThirdPartyLoginProviderTypeFeishu:
		return store.ThirdPartyLoginProviderTypeFeishu, nil
	case store.ThirdPartyLoginProviderTypeGitHub:
		return store.ThirdPartyLoginProviderTypeGitHub, nil
	case store.ThirdPartyLoginProviderTypeGoogle:
		return store.ThirdPartyLoginProviderTypeGoogle, nil
	case store.ThirdPartyLoginProviderTypeOIDC:
		return store.ThirdPartyLoginProviderTypeOIDC, nil
	case "":
		return "", errors.New("登录方式类型不能为空")
	default:
		return "", errors.New("登录方式类型不支持")
	}
}

func normalizeThirdPartyScopes(providerType string, rawScopes []string) ([]string, error) {
	if len(rawScopes) == 0 {
		rawScopes = defaultThirdPartyProviderScopes(providerType)
	}

	seen := map[string]struct{}{}
	scopes := make([]string, 0, len(rawScopes))
	for _, rawScope := range rawScopes {
		scope := strings.TrimSpace(rawScope)
		if scope == "" {
			continue
		}
		if strings.ContainsAny(scope, " \t\r\n") {
			return nil, errors.New("Scope 不能包含空白字符")
		}
		if _, ok := seen[scope]; ok {
			continue
		}
		seen[scope] = struct{}{}
		scopes = append(scopes, scope)
	}

	if scopes == nil {
		scopes = []string{}
	}
	return scopes, nil
}

func parseThirdPartyScopes(raw json.RawMessage) ([]string, error) {
	var scopes []string
	if len(raw) == 0 {
		return []string{}, nil
	}
	if err := json.Unmarshal(raw, &scopes); err != nil {
		return nil, err
	}
	if scopes == nil {
		return []string{}, nil
	}

	return scopes, nil
}

func normalizeThirdPartyProviderConfig(providerType string, rawConfig map[string]any) (map[string]any, error) {
	config := defaultThirdPartyProviderConfig(providerType)
	for key, rawValue := range rawConfig {
		normalizedKey := strings.TrimSpace(key)
		if normalizedKey == "" {
			continue
		}
		switch value := rawValue.(type) {
		case string:
			config[normalizedKey] = strings.TrimSpace(value)
		default:
			config[normalizedKey] = value
		}
	}

	for _, key := range []string{"authorize_url", "token_url", "userinfo_url", "emails_url", "app_token_url", "userid_by_unionid_url", "userdetail_url"} {
		value := stringConfigValue(config, key)
		if value == "" {
			continue
		}
		normalizedURL, err := normalizeHTTPURL(value, thirdPartyConfigURLMessage(key))
		if err != nil {
			return nil, err
		}
		config[key] = normalizedURL
	}

	for _, key := range []string{"authorize_url", "token_url", "userinfo_url"} {
		if stringConfigValue(config, key) == "" {
			return nil, errors.New(thirdPartyConfigURLMessage(key))
		}
	}
	if providerType == store.ThirdPartyLoginProviderTypeWeCom && stringConfigValue(config, "agent_id") == "" {
		return nil, errors.New("企业微信 Agent ID 不能为空")
	}

	return config, nil
}

func parseThirdPartyProviderConfig(raw json.RawMessage) (map[string]any, error) {
	if len(raw) == 0 {
		return map[string]any{}, nil
	}
	var config map[string]any
	if err := json.Unmarshal(raw, &config); err != nil {
		return nil, err
	}
	if config == nil {
		return map[string]any{}, nil
	}

	return config, nil
}

func defaultThirdPartyProviderScopes(providerType string) []string {
	switch providerType {
	case store.ThirdPartyLoginProviderTypeDingTalk:
		return []string{"openid"}
	case store.ThirdPartyLoginProviderTypeWeCom:
		return []string{"snsapi_base"}
	case store.ThirdPartyLoginProviderTypeFeishu:
		return []string{}
	case store.ThirdPartyLoginProviderTypeGitHub:
		return []string{"read:user", "user:email"}
	case store.ThirdPartyLoginProviderTypeGoogle:
		return []string{"openid", "email", "profile"}
	default:
		return []string{"openid", "email", "profile"}
	}
}

func defaultThirdPartyProviderConfig(providerType string) map[string]any {
	switch providerType {
	case store.ThirdPartyLoginProviderTypeDingTalk:
		return map[string]any{
			"authorize_url":         "https://login.dingtalk.com/oauth2/auth?prompt=consent",
			"token_url":             "https://api.dingtalk.com/v1.0/oauth2/userAccessToken",
			"userinfo_url":          "https://api.dingtalk.com/v1.0/contact/users/me",
			"app_token_url":         "https://api.dingtalk.com/v1.0/oauth2/accessToken",
			"userid_by_unionid_url": "https://oapi.dingtalk.com/user/getUseridByUnionid",
			"userdetail_url":        "https://oapi.dingtalk.com/topapi/v2/user/get",
			"external_id_field":     "unionId",
			"email_field":           "email",
			"phone_field":           "mobile",
			"name_field":            "name",
			"nickname_field":        "nick",
			"avatar_field":          "avatarUrl",
		}
	case store.ThirdPartyLoginProviderTypeWeCom:
		return map[string]any{
			"authorize_url":  "https://login.work.weixin.qq.com/wwlogin/sso/login",
			"token_url":      "https://qyapi.weixin.qq.com/cgi-bin/gettoken",
			"userinfo_url":   "https://qyapi.weixin.qq.com/cgi-bin/auth/getuserinfo",
			"userdetail_url": "https://qyapi.weixin.qq.com/cgi-bin/auth/getuserdetail",
			"login_type":     "CorpApp",
		}
	case store.ThirdPartyLoginProviderTypeFeishu:
		return map[string]any{
			"authorize_url":     "https://accounts.feishu.cn/open-apis/authen/v1/authorize",
			"token_url":         "https://accounts.feishu.cn/oauth/v3/token",
			"userinfo_url":      "https://open.feishu.cn/open-apis/authen/v1/user_info",
			"external_id_field": "union_id",
			"email_field":       "email",
			"name_field":        "name",
			"nickname_field":    "en_name",
			"avatar_field":      "avatar_url",
		}
	case store.ThirdPartyLoginProviderTypeGitHub:
		return map[string]any{
			"authorize_url":     "https://github.com/login/oauth/authorize",
			"token_url":         "https://github.com/login/oauth/access_token",
			"userinfo_url":      "https://api.github.com/user",
			"emails_url":        "https://api.github.com/user/emails",
			"external_id_field": "id",
			"email_field":       "email",
			"name_field":        "name",
			"nickname_field":    "login",
			"avatar_field":      "avatar_url",
		}
	case store.ThirdPartyLoginProviderTypeGoogle:
		return map[string]any{
			"authorize_url":     "https://accounts.google.com/o/oauth2/v2/auth",
			"token_url":         "https://oauth2.googleapis.com/token",
			"userinfo_url":      "https://openidconnect.googleapis.com/v1/userinfo",
			"external_id_field": "sub",
			"email_field":       "email",
			"name_field":        "name",
			"avatar_field":      "picture",
		}
	default:
		return map[string]any{
			"external_id_field": "sub",
			"email_field":       "email",
			"phone_field":       "phone",
			"name_field":        "name",
			"nickname_field":    "nickname",
			"avatar_field":      "picture",
		}
	}
}

func thirdPartyConfigURLMessage(key string) string {
	switch key {
	case "authorize_url":
		return "Authorize URL 格式错误"
	case "token_url":
		return "Access Token URL 格式错误"
	case "userinfo_url":
		return "用户信息 URL 格式错误"
	case "emails_url":
		return "邮箱 API URL 格式错误"
	case "app_token_url":
		return "应用 Access Token URL 格式错误"
	case "userid_by_unionid_url":
		return "UnionID 查询 UserID URL 格式错误"
	case "userdetail_url":
		return "用户详情 URL 格式错误"
	default:
		return "URL 格式错误"
	}
}

func stringConfigValue(config map[string]any, key string) string {
	value, _ := config[key].(string)
	return strings.TrimSpace(value)
}

func (s *Server) generateUniqueThirdPartyProviderKey(name string, providerType string) (string, error) {
	base := slugifyThirdPartyProviderKey(name)
	if base == "" {
		base = slugifyThirdPartyProviderKey(providerType)
	}
	if base == "" {
		base = "third-party"
	}
	if len(base) > 80 {
		base = strings.Trim(base[:80], "-_")
	}
	if base == "" {
		base = "third-party"
	}

	key := base
	for attempt := 0; attempt < 8; attempt++ {
		var count int64
		if err := s.db.Model(&store.ThirdPartyLoginProvider{}).Where("key = ?", key).Count(&count).Error; err != nil {
			return "", err
		}
		if count == 0 {
			return key, nil
		}

		suffix, err := randomThirdPartyProviderKeySuffix()
		if err != nil {
			return "", err
		}
		prefixMaxLength := 80 - len(suffix) - 1
		prefix := base
		if len(prefix) > prefixMaxLength {
			prefix = strings.Trim(prefix[:prefixMaxLength], "-_")
		}
		if prefix == "" {
			prefix = "third-party"
		}
		key = prefix + "-" + suffix
	}

	return "", errors.New("generate unique third-party provider key")
}

func (s *Server) nextThirdPartyProviderSortOrder() (int, error) {
	var maxSortOrder sql.NullInt64
	if err := s.db.Model(&store.ThirdPartyLoginProvider{}).Select("MAX(sort_order)").Scan(&maxSortOrder).Error; err != nil {
		return 0, err
	}
	if !maxSortOrder.Valid {
		return 10, nil
	}

	return int(maxSortOrder.Int64) + 10, nil
}

func slugifyThirdPartyProviderKey(name string) string {
	var builder strings.Builder
	lastSeparator := true
	for _, currentRune := range strings.ToLower(name) {
		switch {
		case currentRune >= 'a' && currentRune <= 'z':
			builder.WriteRune(currentRune)
			lastSeparator = false
		case currentRune >= '0' && currentRune <= '9':
			builder.WriteRune(currentRune)
			lastSeparator = false
		case currentRune == '-' || currentRune == '_' || unicode.IsSpace(currentRune):
			if !lastSeparator {
				builder.WriteByte('-')
				lastSeparator = true
			}
		}
	}

	return strings.Trim(builder.String(), "-_")
}

func randomThirdPartyProviderKeySuffix() (string, error) {
	var value [4]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}

	return hex.EncodeToString(value[:]), nil
}

func normalizeHTTPURL(value string, message string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", errors.New(message)
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", errors.New(message)
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return "", errors.New(message)
	}

	return trimmed, nil
}

func parseUUIDParam(c echo.Context, paramName string, message string) (string, error) {
	id := strings.TrimSpace(c.Param(paramName))
	if _, err := uuid.Parse(id); err != nil {
		return "", failure(c, http.StatusBadRequest, "invalid_request", message)
	}

	return id, nil
}
