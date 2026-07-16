package admin

import (
	"net/http"
	"time"

	"app/internal/application/adminauth"

	"github.com/labstack/echo/v4"
)

const AdminSessionCookieName = "admin_session"

type AuthAPI struct {
	login    adminauth.LoginService
	sessions adminauth.SessionAuthenticator
}

type loginRequest struct {
	Email    string `json:"email" example:"user@example.com"`
	Password string `json:"password" example:"password"`
}

type adminResponse struct {
	Email string `json:"email" example:"admin"`
}

type adminLoginResponse struct {
	Admin adminResponse `json:"admin"`
}

func NewAuthAPI(login adminauth.LoginService, sessions adminauth.SessionAuthenticator) *AuthAPI {
	return &AuthAPI{login: login, sessions: sessions}
}

func (a *AuthAPI) RegisterPublicRoutes(router *echo.Echo) {
	router.POST("/api/admin/auth/login", a.loginAdmin)
}

func (a *AuthAPI) RequireSession(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		cookie, err := c.Cookie(AdminSessionCookieName)
		if err != nil || cookie.Value == "" {
			return writeFailure(c, http.StatusUnauthorized, string(adminauth.CodeUnauthorized), "未登录")
		}
		if _, err := a.sessions.AuthenticateSession(c.Request().Context(), cookie.Value); err != nil {
			return writeAdminAuthError(c, err)
		}
		return next(c)
	}
}

// loginAdmin godoc
//
// @Summary 管理员登录
// @Description 默认管理员账号固定为 admin，密码来自服务端配置。
// @Tags 管理员认证
// @Accept json
// @Produce json
// @Param body body loginRequest true "登录参数"
// @Success 200 {object} successEnvelope{data=adminLoginResponse}
// @Failure 400 {object} errorEnvelope
// @Failure 401 {object} errorEnvelope
// @Failure 500 {object} errorEnvelope
// @Router /api/admin/auth/login [post]
func (a *AuthAPI) loginAdmin(c echo.Context) error {
	var req loginRequest
	if err := c.Bind(&req); err != nil {
		return writeFailure(c, http.StatusBadRequest, "invalid_request", "请求格式错误")
	}
	result, err := a.login.Login(c.Request().Context(), adminauth.LoginCommand{
		Email: req.Email, Password: req.Password, UserAgent: c.Request().UserAgent(), IP: c.RealIP(),
	})
	if err != nil {
		return writeAdminAuthError(c, err)
	}
	setAdminSessionCookie(c, result.Session.Token, result.Session.ExpiresAt)
	return writeSuccess(c, http.StatusOK, adminLoginResponse{
		Admin: adminResponse{Email: result.Admin.Email},
	})
}

func writeAdminAuthError(c echo.Context, err error) error {
	code := adminauth.ErrorCodeOf(err)
	status := http.StatusInternalServerError
	if code == adminauth.CodeInvalidCredentials || code == adminauth.CodeUnauthorized {
		status = http.StatusUnauthorized
	}
	return writeFailure(c, status, string(code), adminauth.ErrorMessage(err))
}

func setAdminSessionCookie(c echo.Context, token string, expiresAt time.Time) {
	c.SetCookie(&http.Cookie{
		Name: AdminSessionCookieName, Value: token, Path: "/", Expires: expiresAt,
		MaxAge: int(time.Until(expiresAt).Seconds()), HttpOnly: true, SameSite: http.SameSiteLaxMode,
	})
}
