package admin

import (
	"net/http"
	"net/url"
	"strings"

	"app/internal/application/identityprovider"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

type IdentityProviderAPI struct {
	providers      identityprovider.AdminService
	clientHostname string
}

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

func NewIdentityProviderAPI(providers identityprovider.AdminService, clientHostname string) *IdentityProviderAPI {
	return &IdentityProviderAPI{providers: providers, clientHostname: clientHostname}
}

func (a *IdentityProviderAPI) RegisterRoutes(group *echo.Group) {
	group.GET("/third-party/providers", a.list)
	group.POST("/third-party/providers", a.create)
	group.PUT("/third-party/providers/:id", a.update)
	group.POST("/third-party/providers/:id/enable", a.enable)
	group.POST("/third-party/providers/:id/disable", a.disable)
	group.POST("/third-party/providers/:id/move", a.move)
	group.DELETE("/third-party/providers/:id", a.delete)
}

// list godoc
//
// @Summary 列出第三方登录方式
// @Description 管理员读取已配置的第三方登录方式，包含 Client Secret。
// @Tags 管理员第三方登录
// @Produce json
// @Success 200 {object} successEnvelope{data=listThirdPartyProvidersResponse}
// @Failure 401 {object} errorEnvelope
// @Failure 500 {object} errorEnvelope
// @Router /api/admin/third-party/providers [get]
func (a *IdentityProviderAPI) list(c echo.Context) error {
	providers, err := a.providers.List(c.Request().Context())
	if err != nil {
		return writeIdentityProviderError(c, err)
	}
	return writeSuccess(c, http.StatusOK, listThirdPartyProvidersResponse{Providers: a.newProviderResponses(providers)})
}

// create godoc
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
func (a *IdentityProviderAPI) create(c echo.Context) error {
	var req thirdPartyProviderRequest
	if err := c.Bind(&req); err != nil {
		return writeFailure(c, http.StatusBadRequest, string(identityprovider.CodeInvalidRequest), "请求格式错误")
	}
	provider, err := a.providers.Create(c.Request().Context(), newIdentityProviderWriteCommand(req))
	if err != nil {
		return writeIdentityProviderError(c, err)
	}
	return writeSuccess(c, http.StatusCreated, thirdPartyProviderEnvelope{Provider: a.newProviderResponse(provider)})
}

// update godoc
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
func (a *IdentityProviderAPI) update(c echo.Context) error {
	if _, err := a.providers.Get(c.Request().Context(), c.Param("id")); err != nil {
		return writeIdentityProviderError(c, err)
	}
	var req thirdPartyProviderRequest
	if err := c.Bind(&req); err != nil {
		return writeFailure(c, http.StatusBadRequest, string(identityprovider.CodeInvalidRequest), "请求格式错误")
	}
	provider, err := a.providers.Update(c.Request().Context(), identityprovider.UpdateCommand{
		ProviderID: c.Param("id"), WriteCommand: newIdentityProviderWriteCommand(req),
	})
	if err != nil {
		return writeIdentityProviderError(c, err)
	}
	return writeSuccess(c, http.StatusOK, thirdPartyProviderEnvelope{Provider: a.newProviderResponse(provider)})
}

// enable godoc
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
func (a *IdentityProviderAPI) enable(c echo.Context) error {
	return a.setEnabled(c, true)
}

// disable godoc
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
func (a *IdentityProviderAPI) disable(c echo.Context) error {
	return a.setEnabled(c, false)
}

// move godoc
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
func (a *IdentityProviderAPI) move(c echo.Context) error {
	id := strings.TrimSpace(c.Param("id"))
	if _, err := uuid.Parse(id); err != nil {
		return writeFailure(c, http.StatusBadRequest, string(identityprovider.CodeInvalidRequest), "第三方登录方式 ID 格式错误")
	}
	var req thirdPartyProviderMoveRequest
	if err := c.Bind(&req); err != nil {
		return writeFailure(c, http.StatusBadRequest, string(identityprovider.CodeInvalidRequest), "请求格式错误")
	}
	providers, err := a.providers.Move(c.Request().Context(), identityprovider.MoveCommand{ProviderID: id, Direction: req.Direction})
	if err != nil {
		return writeIdentityProviderError(c, err)
	}
	return writeSuccess(c, http.StatusOK, listThirdPartyProvidersResponse{Providers: a.newProviderResponses(providers)})
}

// delete godoc
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
func (a *IdentityProviderAPI) delete(c echo.Context) error {
	if err := a.providers.Delete(c.Request().Context(), c.Param("id")); err != nil {
		return writeIdentityProviderError(c, err)
	}
	return writeSuccess(c, http.StatusOK, map[string]any{})
}

func (a *IdentityProviderAPI) setEnabled(c echo.Context, enabled bool) error {
	provider, err := a.providers.SetEnabled(c.Request().Context(), identityprovider.SetEnabledCommand{
		ProviderID: c.Param("id"), Enabled: enabled,
	})
	if err != nil {
		return writeIdentityProviderError(c, err)
	}
	return writeSuccess(c, http.StatusOK, thirdPartyProviderEnvelope{Provider: a.newProviderResponse(provider)})
}

func newIdentityProviderWriteCommand(req thirdPartyProviderRequest) identityprovider.WriteCommand {
	return identityprovider.WriteCommand{
		Name: req.Name, Type: req.Type, ClientID: req.ClientID, ClientSecret: req.ClientSecret,
		Scopes: req.Scopes, Config: req.Config,
	}
}

func (a *IdentityProviderAPI) newProviderResponses(values []identityprovider.Provider) []thirdPartyProviderResponse {
	result := make([]thirdPartyProviderResponse, 0, len(values))
	for _, value := range values {
		result = append(result, a.newProviderResponse(value))
	}
	return result
}

func (a *IdentityProviderAPI) newProviderResponse(value identityprovider.Provider) thirdPartyProviderResponse {
	return thirdPartyProviderResponse{
		ID: value.ID, Name: value.Name, Key: value.Key,
		CallbackURL: "https://" + strings.TrimSpace(a.clientHostname) + "/api/client/auth/third-party/" + url.PathEscape(value.Key) + "/callback",
		Type:        value.Type, Enabled: value.Enabled, ClientID: value.ClientID, ClientSecret: value.ClientSecret,
		Scopes: value.Scopes, Config: value.Config, SortOrder: value.SortOrder,
	}
}

func writeIdentityProviderError(c echo.Context, err error) error {
	code := identityprovider.ErrorCodeOf(err)
	status := http.StatusInternalServerError
	switch code {
	case identityprovider.CodeInvalidRequest:
		status = http.StatusBadRequest
	case identityprovider.CodeNotFound:
		status = http.StatusNotFound
	case identityprovider.CodeConflict:
		status = http.StatusConflict
	}
	return writeFailure(c, status, string(code), identityprovider.ErrorMessage(err))
}
