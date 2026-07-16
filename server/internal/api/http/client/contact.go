package client

import (
	"net/http"
	"time"

	contactapp "app/internal/application/contact"

	"github.com/labstack/echo/v4"
)

type ContactAPI struct {
	contacts contactapp.ClientService
}

type contactAppResponse struct {
	Avatar      string `json:"avatar" example:"/assets/apps/assistant.webp"`
	Description string `json:"description" example:"AI 助手"`
	ID          string `json:"id" example:"7f8d8b84-6d2c-4b12-9a8a-019a7e2787d4"`
	Name        string `json:"name" example:"茉莉"`
	Online      bool   `json:"online" example:"false"`
	Type        string `json:"type" example:"app"`
}

type contactGroupResponse struct {
	Avatar        string                             `json:"avatar" example:"/assets/avatars/groups/07.webp"`
	AvatarMembers []contactGroupAvatarMemberResponse `json:"avatar_members,omitempty"`
	ID            string                             `json:"id" example:"7f8d8b84-6d2c-4b12-9a8a-019a7e2787d4"`
	Joined        bool                               `json:"joined" example:"false"`
	MemberCount   int                                `json:"member_count" example:"8"`
	Name          string                             `json:"name" example:"IM探索"`
	Type          string                             `json:"type" example:"group"`
	Visibility    string                             `json:"visibility" example:"public"`
}

type contactGroupAvatarMemberResponse struct {
	Avatar   string `json:"avatar"`
	Name     string `json:"name"`
	Nickname string `json:"nickname"`
	Role     string `json:"role"`
}

type contactUserResponse struct {
	Avatar       string  `json:"avatar" example:"/assets/avatars/builtin/07.webp"`
	Email        string  `json:"email" example:"user@example.com"`
	ID           string  `json:"id" example:"7f8d8b84-6d2c-4b12-9a8a-019a7e2787d4"`
	LastOnlineAt *string `json:"last_online_at" example:"2026-07-03T01:00:00Z"`
	Name         string  `json:"name" example:"张三"`
	Nickname     string  `json:"nickname" example:"小张"`
	Online       bool    `json:"online" example:"true"`
	Phone        string  `json:"phone" example:"+8613812345678"`
	Type         string  `json:"type" example:"user"`
}

type listClientContactsResponse struct {
	Apps   []contactAppResponse   `json:"apps"`
	Groups []contactGroupResponse `json:"groups"`
	Users  []contactUserResponse  `json:"users"`
}

type listContactUsersResponse struct {
	Contacts []contactUserResponse `json:"contacts"`
}

func NewContactAPI(contacts contactapp.ClientService) *ContactAPI {
	return &ContactAPI{contacts: contacts}
}

func (a *ContactAPI) RegisterRoutes(group *echo.Group) {
	group.GET("/contacts", a.list)
	group.GET("/contacts/users", a.listUsers)
}

// list godoc
//
// @Summary 列出通讯录
// @Description 普通用户获取统一通讯录。返回对当前用户可见的应用、启用用户，以及当前用户已加入或公开的 active 群组。
// @Tags 客户端通讯录
// @Produce json
// @Success 200 {object} successEnvelope{data=listClientContactsResponse}
// @Failure 401 {object} errorEnvelope
// @Failure 500 {object} errorEnvelope
// @Router /api/client/contacts [get]
func (a *ContactAPI) list(c echo.Context) error {
	current, ok := CurrentAccount(c)
	if !ok {
		return writeFailure(c, http.StatusInternalServerError, string(contactapp.CodeInternal), "服务端错误")
	}
	result, err := a.contacts.List(c.Request().Context(), contactapp.ListCommand{
		AccountID: current.ID,
		Keyword:   c.QueryParam("keyword"),
	})
	if err != nil {
		return writeContactError(c, err)
	}
	return writeSuccess(c, http.StatusOK, listClientContactsResponse{
		Apps: newContactAppsResponse(result.Apps), Groups: newContactGroupsResponse(result.Groups),
		Users: newContactUsersResponse(result.Users),
	})
}

// listUsers godoc
//
// @Summary 列出通讯录用户
// @Description 普通用户获取通讯录。返回所有启用用户，包含当前用户；keyword 会搜索名称、昵称、邮箱和手机号。
// @Tags 客户端通讯录
// @Produce json
// @Param keyword query string false "搜索关键字，匹配名称、昵称、邮箱或手机号"
// @Success 200 {object} successEnvelope{data=listContactUsersResponse}
// @Failure 401 {object} errorEnvelope
// @Failure 500 {object} errorEnvelope
// @Router /api/client/contacts/users [get]
func (a *ContactAPI) listUsers(c echo.Context) error {
	result, err := a.contacts.ListUsers(c.Request().Context(), contactapp.ListUsersCommand{Keyword: c.QueryParam("keyword")})
	if err != nil {
		return writeContactError(c, err)
	}
	return writeSuccess(c, http.StatusOK, listContactUsersResponse{Contacts: newContactUsersResponse(result.Users)})
}

func newContactAppsResponse(values []contactapp.App) []contactAppResponse {
	result := make([]contactAppResponse, 0, len(values))
	for _, value := range values {
		result = append(result, contactAppResponse{
			Avatar: value.Avatar, Description: value.Description, ID: value.ID,
			Name: value.Name, Online: value.Online, Type: value.Type,
		})
	}
	return result
}

func newContactGroupsResponse(values []contactapp.Group) []contactGroupResponse {
	result := make([]contactGroupResponse, 0, len(values))
	for _, value := range values {
		var avatarMembers []contactGroupAvatarMemberResponse
		if value.AvatarMembers != nil {
			avatarMembers = make([]contactGroupAvatarMemberResponse, 0, len(value.AvatarMembers))
			for _, member := range value.AvatarMembers {
				avatarMembers = append(avatarMembers, contactGroupAvatarMemberResponse{
					Avatar: member.Avatar, Name: member.Name, Nickname: member.Nickname, Role: member.Role,
				})
			}
		}
		result = append(result, contactGroupResponse{
			Avatar: value.Avatar, AvatarMembers: avatarMembers, ID: value.ID, Joined: value.Joined,
			MemberCount: value.MemberCount, Name: value.Name, Type: value.Type, Visibility: value.Visibility,
		})
	}
	return result
}

func newContactUsersResponse(values []contactapp.User) []contactUserResponse {
	result := make([]contactUserResponse, 0, len(values))
	for _, value := range values {
		var lastOnlineAt *string
		if value.LastOnlineAt != nil {
			formatted := value.LastOnlineAt.UTC().Format(time.RFC3339)
			lastOnlineAt = &formatted
		}
		result = append(result, contactUserResponse{
			Avatar: value.Avatar, Email: value.Email, ID: value.ID, LastOnlineAt: lastOnlineAt,
			Name: value.Name, Nickname: value.Nickname, Online: value.Online, Phone: value.Phone, Type: value.Type,
		})
	}
	return result
}

func writeContactError(c echo.Context, err error) error {
	return writeFailure(c, http.StatusInternalServerError, string(contactapp.ErrorCodeOf(err)), contactapp.ErrorMessage(err))
}
