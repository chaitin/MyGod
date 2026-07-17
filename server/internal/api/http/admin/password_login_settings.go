package admin

import (
	"net/http"

	settingsapp "app/internal/application/settings"

	"github.com/labstack/echo/v4"
)

type PasswordLoginSettingsAPI struct {
	settings settingsapp.PasswordLoginSettingsService
}

type passwordLoginSettingsResponse struct {
	Enabled bool `json:"enabled" example:"true"`
}

type updatePasswordLoginSettingsRequest struct {
	Enabled *bool `json:"enabled"`
}

func NewPasswordLoginSettingsAPI(settings settingsapp.PasswordLoginSettingsService) *PasswordLoginSettingsAPI {
	return &PasswordLoginSettingsAPI{settings: settings}
}

func (a *PasswordLoginSettingsAPI) RegisterRoutes(group *echo.Group) {
	group.GET("/settings/password-login", a.get)
	group.PUT("/settings/password-login", a.update)
}

// get godoc
//
// @Summary 获取密码登录设置
// @Description 管理员读取普通用户是否可以使用邮箱和密码登录。
// @Tags 管理员设置
// @Produce json
// @Success 200 {object} successEnvelope{data=passwordLoginSettingsResponse}
// @Failure 401 {object} errorEnvelope
// @Failure 500 {object} errorEnvelope
// @Router /api/admin/settings/password-login [get]
func (a *PasswordLoginSettingsAPI) get(c echo.Context) error {
	value, err := a.settings.GetPasswordLogin(c.Request().Context())
	if err != nil {
		return writeSettingsError(c, err)
	}
	return writeSuccess(c, http.StatusOK, passwordLoginSettingsResponse{Enabled: value.Enabled})
}

// update godoc
//
// @Summary 更新密码登录设置
// @Description 管理员启用或关闭普通用户的邮箱密码登录。
// @Tags 管理员设置
// @Accept json
// @Produce json
// @Param body body updatePasswordLoginSettingsRequest true "密码登录设置"
// @Success 200 {object} successEnvelope{data=passwordLoginSettingsResponse}
// @Failure 400 {object} errorEnvelope
// @Failure 401 {object} errorEnvelope
// @Failure 500 {object} errorEnvelope
// @Router /api/admin/settings/password-login [put]
func (a *PasswordLoginSettingsAPI) update(c echo.Context) error {
	var req updatePasswordLoginSettingsRequest
	if err := c.Bind(&req); err != nil {
		return writeFailure(c, http.StatusBadRequest, string(settingsapp.CodeInvalidRequest), "请求格式错误")
	}
	if req.Enabled == nil {
		return writeFailure(c, http.StatusBadRequest, string(settingsapp.CodeInvalidRequest), "是否启用密码登录不能为空")
	}
	value, err := a.settings.UpdatePasswordLogin(c.Request().Context(), settingsapp.UpdatePasswordLoginCommand{Enabled: *req.Enabled})
	if err != nil {
		return writeSettingsError(c, err)
	}
	return writeSuccess(c, http.StatusOK, passwordLoginSettingsResponse{Enabled: value.Enabled})
}
