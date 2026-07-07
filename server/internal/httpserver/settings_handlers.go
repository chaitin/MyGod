package httpserver

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"app/internal/auth"
	"app/internal/store"

	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
)

type infoSettingsResponse struct {
	AppName             string                             `json:"app_name" example:"MyGod"`
	Authenticated       *bool                              `json:"authenticated,omitempty" example:"false"`
	OrganizationName    string                             `json:"organization_name" example:"长亭科技"`
	ThirdPartyProviders []publicThirdPartyProviderResponse `json:"third_party_providers"`
}

type publicThirdPartyProviderResponse struct {
	Key  string `json:"key" example:"company-sso"`
	Name string `json:"name" example:"企业 SSO"`
}

type updateInfoSettingsRequest struct {
	AppName          string `json:"app_name" example:"MyGod"`
	OrganizationName string `json:"organization_name" example:"长亭科技"`
}

// clientInfo godoc
//
// @Summary 获取客户端公开信息
// @Description 返回客户端启动和登录页展示所需的公开信息，不需要登录。
// @Tags 客户端信息
// @Produce json
// @Success 200 {object} successEnvelope{data=infoSettingsResponse}
// @Failure 500 {object} errorEnvelope
// @Router /api/client/info [get]
func (s *Server) clientInfo(c echo.Context) error {
	settings, err := s.getOrCreateAppSettings()
	if err != nil {
		return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
	}
	providers, err := s.listPublicThirdPartyProviders()
	if err != nil {
		return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
	}
	authenticated, err := s.isClientInfoRequestAuthenticated(c)
	if err != nil {
		return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
	}

	return success(c, http.StatusOK, newClientInfoSettingsResponse(settings, providers, authenticated))
}

// getInfoSettings godoc
//
// @Summary 获取系统基础信息设置
// @Description 管理员读取 App 名称和组织名称。
// @Tags 管理员设置
// @Produce json
// @Success 200 {object} successEnvelope{data=infoSettingsResponse}
// @Failure 401 {object} errorEnvelope
// @Failure 500 {object} errorEnvelope
// @Router /api/admin/settings/info [get]
func (s *Server) getInfoSettings(c echo.Context) error {
	settings, err := s.getOrCreateAppSettings()
	if err != nil {
		return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
	}

	return success(c, http.StatusOK, newInfoSettingsResponse(settings, nil))
}

// updateInfoSettings godoc
//
// @Summary 更新系统基础信息设置
// @Description 管理员更新 App 名称和组织名称。
// @Tags 管理员设置
// @Accept json
// @Produce json
// @Param body body updateInfoSettingsRequest true "基础信息设置"
// @Success 200 {object} successEnvelope{data=infoSettingsResponse}
// @Failure 400 {object} errorEnvelope
// @Failure 401 {object} errorEnvelope
// @Failure 500 {object} errorEnvelope
// @Router /api/admin/settings/info [put]
func (s *Server) updateInfoSettings(c echo.Context) error {
	var req updateInfoSettingsRequest
	if err := c.Bind(&req); err != nil {
		return failure(c, http.StatusBadRequest, "invalid_request", "请求格式错误")
	}

	appName := strings.TrimSpace(req.AppName)
	if appName == "" {
		return failure(c, http.StatusBadRequest, "invalid_request", "App 名称不能为空")
	}
	organizationName := strings.TrimSpace(req.OrganizationName)
	if organizationName == "" {
		return failure(c, http.StatusBadRequest, "invalid_request", "组织名称不能为空")
	}

	settings, err := s.getOrCreateAppSettings()
	if err != nil {
		return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
	}

	if err := s.db.Model(&settings).Updates(map[string]any{
		"app_name":          appName,
		"organization_name": organizationName,
	}).Error; err != nil {
		return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
	}
	settings.AppName = appName
	settings.OrganizationName = organizationName

	return success(c, http.StatusOK, newInfoSettingsResponse(settings, nil))
}

func (s *Server) getOrCreateAppSettings() (store.AppSettings, error) {
	var count int64
	if err := s.db.Model(&store.AppSettings{}).Where("id = ?", store.AppSettingsID).Count(&count).Error; err != nil {
		return store.AppSettings{}, err
	}

	if count == 0 {
		settings := store.AppSettings{
			ID:               store.AppSettingsID,
			AppName:          store.DefaultAppName,
			OrganizationName: store.DefaultOrganizationName,
		}
		if err := s.db.Create(&settings).Error; err != nil {
			return store.AppSettings{}, err
		}

		return settings, nil
	}

	var settings store.AppSettings
	err := s.db.First(&settings, "id = ?", store.AppSettingsID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return store.AppSettings{}, gorm.ErrRecordNotFound
	}
	if err != nil {
		return store.AppSettings{}, err
	}

	return settings, nil
}

func (s *Server) listPublicThirdPartyProviders() ([]publicThirdPartyProviderResponse, error) {
	var providers []store.ThirdPartyLoginProvider
	if err := s.db.
		Where("enabled = ?", true).
		Order("sort_order ASC").
		Order("name ASC").
		Find(&providers).Error; err != nil {
		return nil, err
	}

	responses := make([]publicThirdPartyProviderResponse, 0, len(providers))
	for _, provider := range providers {
		responses = append(responses, publicThirdPartyProviderResponse{
			Key:  provider.Key,
			Name: provider.Name,
		})
	}

	return responses, nil
}

func (s *Server) isClientInfoRequestAuthenticated(c echo.Context) (bool, error) {
	cookie, err := c.Cookie(userSessionCookieName)
	if err != nil || cookie.Value == "" {
		return false, nil
	}

	var session store.UserSession
	err = s.db.Preload("User").Where(
		"token_hash = ? AND expires_at > ?",
		auth.HashSessionToken(cookie.Value),
		time.Now().UTC(),
	).First(&session).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	return session.User.Status == store.UserStatusActive, nil
}

func newInfoSettingsResponse(settings store.AppSettings, providers []publicThirdPartyProviderResponse) infoSettingsResponse {
	if providers == nil {
		providers = []publicThirdPartyProviderResponse{}
	}

	return infoSettingsResponse{
		AppName:             settings.AppName,
		OrganizationName:    settings.OrganizationName,
		ThirdPartyProviders: providers,
	}
}

func newClientInfoSettingsResponse(settings store.AppSettings, providers []publicThirdPartyProviderResponse, authenticated bool) infoSettingsResponse {
	response := newInfoSettingsResponse(settings, providers)
	response.Authenticated = &authenticated

	return response
}
