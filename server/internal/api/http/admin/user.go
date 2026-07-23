package admin

import (
	"net/http"
	"time"

	"app/internal/application/usermanagement"

	"github.com/labstack/echo/v4"
)

type UserAPI struct {
	users usermanagement.AdminService
}

type createUserRequest struct {
	Email string `json:"email" example:"user@example.com"`
	Name  string `json:"name" example:"张三"`
	Phone string `json:"phone" example:"13812345678"`
}

type userResponse struct {
	ID           string    `json:"id" example:"7f8d8b84-6d2c-4b12-9a8a-019a7e2787d4"`
	Avatar       string    `json:"avatar" example:"/assets/avatars/builtin/07.webp"`
	Email        string    `json:"email" example:"user@example.com"`
	LastOnlineAt *string   `json:"last_online_at" example:"2026-07-03T01:00:00Z"`
	Name         string    `json:"name" example:"张三"`
	Nickname     string    `json:"nickname" example:"小张"`
	Online       *bool     `json:"online,omitempty" example:"true"`
	Phone        string    `json:"phone" example:"+8613812345678"`
	Status       string    `json:"status" example:"active"`
	CreatedAt    time.Time `json:"created_at" format:"date-time"`
}

type createUserResponse struct {
	User            userResponse `json:"user"`
	InitialPassword string       `json:"initial_password" example:"aB3dE5gH7jK9mN2p"`
}

type listUsersResponse struct {
	Users    []userResponse `json:"users"`
	Total    int64          `json:"total" example:"12"`
	Page     int            `json:"page" example:"1"`
	PageSize int            `json:"page_size" example:"20"`
	Sort     string         `json:"sort" example:"created_at"`
	Order    string         `json:"order" example:"desc"`
}

type updateUserStatusResponse struct {
	User userResponse `json:"user"`
}

type resetUserPasswordResponse struct {
	User        userResponse `json:"user"`
	NewPassword string       `json:"new_password" example:"aB3dE5gH7jK9mN2p"`
}

func NewUserAPI(users usermanagement.AdminService) *UserAPI {
	return &UserAPI{users: users}
}

func (a *UserAPI) RegisterRoutes(group *echo.Group) {
	group.GET("/users", a.list)
	group.POST("/users", a.create)
	group.POST("/users/:id/disable", a.disable)
	group.POST("/users/:id/enable", a.enable)
	group.POST("/users/:id/reset-password", a.resetPassword)
}

// list godoc
//
// @Summary 列出普通用户
// @Description 管理员列出普通用户。keyword 会同时搜索邮箱、名称、昵称和手机号；online 仅支持 true、false；sort 仅支持 email、created_at、status；order 仅支持 asc、desc。
// @Tags 管理员用户
// @Produce json
// @Param keyword query string false "搜索关键字，匹配邮箱、名称、昵称或手机号"
// @Param online query bool false "在线状态，true 表示在线，false 表示不在线"
// @Param page query int false "页码，从 1 开始"
// @Param page_size query int false "每页数量，最大 1000"
// @Param sort query string false "排序字段：email、created_at、status"
// @Param order query string false "排序方向：asc、desc"
// @Success 200 {object} successEnvelope{data=listUsersResponse}
// @Failure 400 {object} errorEnvelope
// @Failure 401 {object} errorEnvelope
// @Failure 500 {object} errorEnvelope
// @Router /api/admin/users [get]
func (a *UserAPI) list(c echo.Context) error {
	result, err := a.users.List(c.Request().Context(), usermanagement.ListCommand{
		Keyword: c.QueryParam("keyword"), Online: c.QueryParam("online"), Page: c.QueryParam("page"), PageSize: c.QueryParam("page_size"),
		Sort: c.QueryParam("sort"), Order: c.QueryParam("order"),
	})
	if err != nil {
		return writeUserManagementError(c, err)
	}
	return writeSuccess(c, http.StatusOK, listUsersResponse{
		Users: newUserResponses(result.Users), Total: result.Total, Page: result.Page,
		PageSize: result.PageSize, Sort: result.Sort, Order: result.Order,
	})
}

// create godoc
//
// @Summary 创建普通用户
// @Description 管理员创建普通用户。邮箱会规范化为小写并全局唯一，手机号可选且非空时全局唯一，初始密码只在本次响应中返回。
// @Tags 管理员用户
// @Accept json
// @Produce json
// @Param body body createUserRequest true "用户信息"
// @Success 201 {object} successEnvelope{data=createUserResponse}
// @Failure 400 {object} errorEnvelope
// @Failure 401 {object} errorEnvelope
// @Failure 409 {object} errorEnvelope
// @Failure 500 {object} errorEnvelope
// @Router /api/admin/users [post]
func (a *UserAPI) create(c echo.Context) error {
	var req createUserRequest
	if err := c.Bind(&req); err != nil {
		return writeFailure(c, http.StatusBadRequest, string(usermanagement.CodeInvalidRequest), "请求格式错误")
	}
	result, err := a.users.Create(c.Request().Context(), usermanagement.CreateCommand{
		Email: req.Email, Name: req.Name, Phone: req.Phone,
	})
	if err != nil {
		return writeUserManagementError(c, err)
	}
	return writeSuccess(c, http.StatusCreated, createUserResponse{
		User: newUserResponse(result.User), InitialPassword: result.InitialPassword,
	})
}

// disable godoc
//
// @Summary 禁用普通用户
// @Description 管理员禁用普通用户。禁用后该用户不能继续登录。
// @Tags 管理员用户
// @Produce json
// @Param id path string true "用户 ID"
// @Success 200 {object} successEnvelope{data=updateUserStatusResponse}
// @Failure 400 {object} errorEnvelope
// @Failure 401 {object} errorEnvelope
// @Failure 404 {object} errorEnvelope
// @Failure 500 {object} errorEnvelope
// @Router /api/admin/users/{id}/disable [post]
func (a *UserAPI) disable(c echo.Context) error {
	return a.setStatus(c, usermanagement.StatusDisabled)
}

// enable godoc
//
// @Summary 启用普通用户
// @Description 管理员启用普通用户。启用后该用户可以正常登录。
// @Tags 管理员用户
// @Produce json
// @Param id path string true "用户 ID"
// @Success 200 {object} successEnvelope{data=updateUserStatusResponse}
// @Failure 400 {object} errorEnvelope
// @Failure 401 {object} errorEnvelope
// @Failure 404 {object} errorEnvelope
// @Failure 500 {object} errorEnvelope
// @Router /api/admin/users/{id}/enable [post]
func (a *UserAPI) enable(c echo.Context) error {
	return a.setStatus(c, usermanagement.StatusActive)
}

// resetPassword godoc
//
// @Summary 重置普通用户密码
// @Description 管理员为普通用户重新生成随机密码。新密码只在本次响应中返回一次，并会清理该用户已有登录 session。
// @Tags 管理员用户
// @Produce json
// @Param id path string true "用户 ID"
// @Success 200 {object} successEnvelope{data=resetUserPasswordResponse}
// @Failure 400 {object} errorEnvelope
// @Failure 401 {object} errorEnvelope
// @Failure 404 {object} errorEnvelope
// @Failure 500 {object} errorEnvelope
// @Router /api/admin/users/{id}/reset-password [post]
func (a *UserAPI) resetPassword(c echo.Context) error {
	result, err := a.users.ResetPassword(c.Request().Context(), c.Param("id"))
	if err != nil {
		return writeUserManagementError(c, err)
	}
	return writeSuccess(c, http.StatusOK, resetUserPasswordResponse{
		User: newUserResponse(result.User), NewPassword: result.NewPassword,
	})
}

func (a *UserAPI) setStatus(c echo.Context, status string) error {
	user, err := a.users.SetStatus(c.Request().Context(), usermanagement.SetStatusCommand{
		UserID: c.Param("id"), Status: status,
	})
	if err != nil {
		return writeUserManagementError(c, err)
	}
	return writeSuccess(c, http.StatusOK, updateUserStatusResponse{User: newUserResponse(user)})
}

func newUserResponses(values []usermanagement.User) []userResponse {
	result := make([]userResponse, 0, len(values))
	for _, value := range values {
		result = append(result, newUserResponse(value))
	}
	return result
}

func newUserResponse(value usermanagement.User) userResponse {
	var lastOnlineAt *string
	if value.LastOnlineAt != nil {
		formatted := value.LastOnlineAt.UTC().Format(time.RFC3339)
		lastOnlineAt = &formatted
	}
	online := value.Online
	return userResponse{
		ID: value.ID, Avatar: value.Avatar, Email: value.Email, LastOnlineAt: lastOnlineAt,
		Name: value.Name, Nickname: value.Nickname, Online: &online, Phone: value.Phone,
		Status: value.Status, CreatedAt: value.CreatedAt,
	}
}

func writeUserManagementError(c echo.Context, err error) error {
	code := usermanagement.ErrorCodeOf(err)
	status := http.StatusInternalServerError
	switch code {
	case usermanagement.CodeInvalidRequest:
		status = http.StatusBadRequest
	case usermanagement.CodeNotFound:
		status = http.StatusNotFound
	case usermanagement.CodeConflict:
		status = http.StatusConflict
	}
	return writeFailure(c, status, string(code), usermanagement.ErrorMessage(err))
}
