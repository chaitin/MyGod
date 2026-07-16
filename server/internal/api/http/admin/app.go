package admin

import (
	"net/http"
	"strings"
	"time"

	appapp "app/internal/application/app"

	"github.com/labstack/echo/v4"
)

const maxAppAvatarRequestBytes = appapp.MaxAvatarBytes + 1*1024*1024

type AppAPI struct {
	apps appapp.AdminService
}

type adminAppRequest struct {
	Description string `json:"description"`
	Name        string `json:"name"`
	Visibility  string `json:"visibility"`
}

type adminAppResponse struct {
	Avatar           string    `json:"avatar"`
	ConnectionSecret string    `json:"connection_secret"`
	ConnectionStatus string    `json:"connection_status"`
	CreatedAt        time.Time `json:"created_at" format:"date-time"`
	CreatorUserID    *string   `json:"creator_user_id"`
	Description      string    `json:"description"`
	Enabled          bool      `json:"enabled"`
	ID               string    `json:"id"`
	Name             string    `json:"name"`
	System           bool      `json:"system"`
	UpdatedAt        time.Time `json:"updated_at" format:"date-time"`
	Visibility       string    `json:"visibility"`
}

type listAdminAppsResponse struct {
	Apps []adminAppResponse `json:"apps"`
}

type adminAppEnvelope struct {
	App adminAppResponse `json:"app"`
}

func NewAppAPI(apps appapp.AdminService) *AppAPI {
	return &AppAPI{apps: apps}
}

func (a *AppAPI) RegisterRoutes(group *echo.Group) {
	group.GET("/apps", a.list)
	group.POST("/apps", a.create)
	group.PUT("/apps/:id", a.update)
	group.POST("/apps/:id/avatar", a.uploadAvatar)
	group.POST("/apps/:id/enable", a.enable)
	group.POST("/apps/:id/disable", a.disable)
	group.POST("/apps/:id/secret/regenerate", a.regenerateSecret)
	group.DELETE("/apps/:id", a.delete)
}

// list godoc
//
// @Summary 列出应用
// @Description 管理员读取应用配置，包含连接密钥和连接状态。
// @Tags 管理员应用
// @Produce json
// @Success 200 {object} successEnvelope{data=listAdminAppsResponse}
// @Failure 401 {object} errorEnvelope
// @Failure 500 {object} errorEnvelope
// @Router /api/admin/apps [get]
func (a *AppAPI) list(c echo.Context) error {
	apps, err := a.apps.List(c.Request().Context())
	if err != nil {
		return writeAppError(c, err)
	}
	responses := make([]adminAppResponse, 0, len(apps))
	for _, value := range apps {
		responses = append(responses, newAdminAppResponse(value))
	}
	return writeSuccess(c, http.StatusOK, listAdminAppsResponse{Apps: responses})
}

// create godoc
//
// @Summary 创建应用
// @Description 管理员创建一个应用配置。连接密钥由服务端生成。
// @Tags 管理员应用
// @Accept json
// @Produce json
// @Param body body adminAppRequest true "应用配置"
// @Success 201 {object} successEnvelope{data=adminAppEnvelope}
// @Failure 400 {object} errorEnvelope
// @Failure 401 {object} errorEnvelope
// @Failure 500 {object} errorEnvelope
// @Router /api/admin/apps [post]
func (a *AppAPI) create(c echo.Context) error {
	var req adminAppRequest
	if err := c.Bind(&req); err != nil {
		return writeFailure(c, http.StatusBadRequest, string(appapp.CodeInvalidRequest), "请求格式错误")
	}
	created, err := a.apps.Create(c.Request().Context(), appapp.CreateCommand{
		Name: req.Name, Description: req.Description, Visibility: req.Visibility,
	})
	if err != nil {
		return writeAppError(c, err)
	}
	return writeSuccess(c, http.StatusCreated, adminAppEnvelope{App: newAdminAppResponse(created)})
}

// update godoc
//
// @Summary 更新应用
// @Description 管理员更新一个应用配置。茉莉的可见范围固定为所有人。
// @Tags 管理员应用
// @Accept json
// @Produce json
// @Param id path string true "应用 ID"
// @Param body body adminAppRequest true "应用配置"
// @Success 200 {object} successEnvelope{data=adminAppEnvelope}
// @Failure 400 {object} errorEnvelope
// @Failure 401 {object} errorEnvelope
// @Failure 404 {object} errorEnvelope
// @Failure 500 {object} errorEnvelope
// @Router /api/admin/apps/{id} [put]
func (a *AppAPI) update(c echo.Context) error {
	if _, err := a.apps.Get(c.Request().Context(), c.Param("id")); err != nil {
		return writeAppError(c, err)
	}
	var req adminAppRequest
	if err := c.Bind(&req); err != nil {
		return writeFailure(c, http.StatusBadRequest, string(appapp.CodeInvalidRequest), "请求格式错误")
	}
	updated, err := a.apps.Update(c.Request().Context(), appapp.UpdateCommand{
		AppID: c.Param("id"), Name: req.Name, Description: req.Description, Visibility: req.Visibility,
	})
	if err != nil {
		return writeAppError(c, err)
	}
	return writeSuccess(c, http.StatusOK, adminAppEnvelope{App: newAdminAppResponse(updated)})
}

// enable godoc
//
// @Summary 启用应用
// @Tags 管理员应用
// @Produce json
// @Param id path string true "应用 ID"
// @Success 200 {object} successEnvelope{data=adminAppEnvelope}
// @Failure 401 {object} errorEnvelope
// @Failure 404 {object} errorEnvelope
// @Failure 500 {object} errorEnvelope
// @Router /api/admin/apps/{id}/enable [post]
func (a *AppAPI) enable(c echo.Context) error {
	return a.setEnabled(c, true)
}

// disable godoc
//
// @Summary 禁用应用
// @Tags 管理员应用
// @Produce json
// @Param id path string true "应用 ID"
// @Success 200 {object} successEnvelope{data=adminAppEnvelope}
// @Failure 401 {object} errorEnvelope
// @Failure 404 {object} errorEnvelope
// @Failure 500 {object} errorEnvelope
// @Router /api/admin/apps/{id}/disable [post]
func (a *AppAPI) disable(c echo.Context) error {
	return a.setEnabled(c, false)
}

// regenerateSecret godoc
//
// @Summary 生成应用连接密钥
// @Description 普通应用可以生成新密钥。茉莉密钥由配置管理，不能在后台生成。
// @Tags 管理员应用
// @Produce json
// @Param id path string true "应用 ID"
// @Success 200 {object} successEnvelope{data=adminAppEnvelope}
// @Failure 401 {object} errorEnvelope
// @Failure 403 {object} errorEnvelope
// @Failure 404 {object} errorEnvelope
// @Failure 500 {object} errorEnvelope
// @Router /api/admin/apps/{id}/secret/regenerate [post]
func (a *AppAPI) regenerateSecret(c echo.Context) error {
	updated, err := a.apps.RegenerateSecret(c.Request().Context(), c.Param("id"))
	if err != nil {
		return writeAppError(c, err)
	}
	return writeSuccess(c, http.StatusOK, adminAppEnvelope{App: newAdminAppResponse(updated)})
}

// delete godoc
//
// @Summary 删除应用
// @Description 管理员删除普通应用。茉莉不能删除。
// @Tags 管理员应用
// @Produce json
// @Param id path string true "应用 ID"
// @Success 200 {object} successEnvelope
// @Failure 401 {object} errorEnvelope
// @Failure 403 {object} errorEnvelope
// @Failure 404 {object} errorEnvelope
// @Failure 500 {object} errorEnvelope
// @Router /api/admin/apps/{id} [delete]
func (a *AppAPI) delete(c echo.Context) error {
	if err := a.apps.Delete(c.Request().Context(), c.Param("id")); err != nil {
		return writeAppError(c, err)
	}
	return writeSuccess(c, http.StatusOK, map[string]any{})
}

// uploadAvatar godoc
//
// @Summary 上传应用头像
// @Description 管理员上传裁切后的 WebP 应用头像。头像必须是 256x256，文件会写入 public bucket，并更新应用头像。
// @Tags 管理端应用
// @Accept multipart/form-data
// @Produce json
// @Param id path string true "应用 ID"
// @Param file formData file true "WebP 应用头像"
// @Success 200 {object} successEnvelope{data=adminAppEnvelope}
// @Failure 400 {object} errorEnvelope
// @Failure 401 {object} errorEnvelope
// @Failure 404 {object} errorEnvelope
// @Failure 413 {object} errorEnvelope
// @Failure 500 {object} errorEnvelope
// @Router /api/admin/apps/{id}/avatar [post]
func (a *AppAPI) uploadAvatar(c echo.Context) error {
	if _, err := a.apps.Get(c.Request().Context(), c.Param("id")); err != nil {
		return writeAppError(c, err)
	}
	c.Request().Body = http.MaxBytesReader(c.Response().Writer, c.Request().Body, maxAppAvatarRequestBytes)
	fileHeader, err := c.FormFile("file")
	if err != nil {
		if isRequestBodyTooLarge(err) {
			return writeFailure(c, http.StatusRequestEntityTooLarge, string(appapp.CodeRequestTooLarge), "头像文件不能超过 1MiB")
		}
		return writeFailure(c, http.StatusBadRequest, string(appapp.CodeInvalidRequest), "请选择要上传的头像")
	}
	if fileHeader.Size > appapp.MaxAvatarBytes {
		return writeFailure(c, http.StatusRequestEntityTooLarge, string(appapp.CodeRequestTooLarge), "头像文件不能超过 1MiB")
	}
	if fileHeader.Size == 0 {
		return writeFailure(c, http.StatusBadRequest, string(appapp.CodeInvalidRequest), "头像文件不能为空")
	}
	file, err := fileHeader.Open()
	if err != nil {
		return writeFailure(c, http.StatusBadRequest, string(appapp.CodeInvalidRequest), "读取头像失败")
	}
	defer file.Close()

	updated, err := a.apps.UploadAvatar(c.Request().Context(), appapp.UploadAvatarCommand{
		AppID: c.Param("id"), Content: file, Size: fileHeader.Size,
	})
	if err != nil {
		return writeAppError(c, err)
	}
	return writeSuccess(c, http.StatusOK, adminAppEnvelope{App: newAdminAppResponse(updated)})
}

func (a *AppAPI) setEnabled(c echo.Context, enabled bool) error {
	updated, err := a.apps.SetEnabled(c.Request().Context(), appapp.SetEnabledCommand{
		AppID: c.Param("id"), Enabled: enabled,
	})
	if err != nil {
		return writeAppError(c, err)
	}
	return writeSuccess(c, http.StatusOK, adminAppEnvelope{App: newAdminAppResponse(updated)})
}

func newAdminAppResponse(value appapp.App) adminAppResponse {
	return adminAppResponse{
		Avatar: value.Avatar, ConnectionSecret: value.ConnectionSecret,
		ConnectionStatus: value.ConnectionStatus, CreatedAt: value.CreatedAt,
		CreatorUserID: value.CreatorUserID, Description: value.Description,
		Enabled: value.Enabled, ID: value.ID, Name: value.Name, System: value.System,
		UpdatedAt: value.UpdatedAt, Visibility: value.Visibility,
	}
}

func writeAppError(c echo.Context, err error) error {
	code := appapp.ErrorCodeOf(err)
	status := http.StatusInternalServerError
	switch code {
	case appapp.CodeInvalidRequest:
		status = http.StatusBadRequest
	case appapp.CodeNotFound:
		status = http.StatusNotFound
	case appapp.CodeForbidden:
		status = http.StatusForbidden
	case appapp.CodeRequestTooLarge:
		status = http.StatusRequestEntityTooLarge
	}
	return writeFailure(c, status, string(code), appapp.ErrorMessage(err))
}

func isRequestBodyTooLarge(err error) bool {
	return err != nil && strings.Contains(err.Error(), "request body too large")
}
