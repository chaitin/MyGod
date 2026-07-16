package client

import (
	"context"
	"net/http"
	"strings"

	"app/internal/application/account"
	settingsapp "app/internal/application/settings"

	"github.com/labstack/echo/v4"
)

type InfoAPI struct {
	settings settingsapp.PublicService
	sessions account.SessionAuthenticator
}

type infoSettingsResponse struct {
	AppName             string                             `json:"app_name" example:"即应"`
	Authenticated       *bool                              `json:"authenticated,omitempty" example:"false"`
	OrganizationName    string                             `json:"organization_name" example:"长亭科技"`
	ThirdPartyProviders []publicThirdPartyProviderResponse `json:"third_party_providers"`
}

type publicThirdPartyProviderResponse struct {
	Key  string `json:"key" example:"company-sso"`
	Name string `json:"name" example:"企业 SSO"`
}

func NewInfoAPI(settings settingsapp.PublicService, sessions account.SessionAuthenticator) *InfoAPI {
	return &InfoAPI{settings: settings, sessions: sessions}
}

func (a *InfoAPI) RegisterPublicRoutes(router *echo.Echo) {
	router.GET("/api/client/info", a.clientInfo)
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
func (a *InfoAPI) clientInfo(c echo.Context) error {
	info, err := a.settings.GetPublicInfo(c.Request().Context())
	if err != nil {
		return writeFailure(c, http.StatusInternalServerError, string(settingsapp.CodeInternal), "服务端错误")
	}
	authenticated, err := a.isAuthenticated(c.Request().Context(), sessionToken(c))
	if err != nil {
		return writeFailure(c, http.StatusInternalServerError, string(settingsapp.CodeInternal), "服务端错误")
	}
	return writeSuccess(c, http.StatusOK, newClientInfoSettingsResponse(info, authenticated))
}

func (a *InfoAPI) isAuthenticated(ctx context.Context, token string) (bool, error) {
	if strings.TrimSpace(token) == "" {
		return false, nil
	}
	_, err := a.sessions.AuthenticateSession(ctx, token)
	if err == nil {
		return true, nil
	}
	if account.ErrorCodeOf(err) == account.CodeUnauthorized {
		return false, nil
	}
	return false, err
}

func sessionToken(c echo.Context) string {
	cookie, err := c.Cookie(UserSessionCookieName)
	if err != nil {
		return ""
	}
	return cookie.Value
}

func newClientInfoSettingsResponse(info settingsapp.PublicInfo, authenticated bool) infoSettingsResponse {
	providers := make([]publicThirdPartyProviderResponse, 0, len(info.Providers))
	for _, provider := range info.Providers {
		providers = append(providers, publicThirdPartyProviderResponse{Key: provider.Key, Name: provider.Name})
	}
	return infoSettingsResponse{
		AppName:             info.Settings.AppName,
		Authenticated:       &authenticated,
		OrganizationName:    info.Settings.OrganizationName,
		ThirdPartyProviders: providers,
	}
}
