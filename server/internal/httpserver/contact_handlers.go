package httpserver

import (
	"net/http"
	"strings"

	"app/internal/store"

	"github.com/labstack/echo/v4"
)

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

type listContactUsersResponse struct {
	Contacts []contactUserResponse `json:"contacts"`
}

// listContactUsers godoc
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
func (s *Server) listContactUsers(c echo.Context) error {
	query := s.db.Model(&store.User{}).Where("status = ?", store.UserStatusActive)
	keyword := strings.ToLower(strings.TrimSpace(c.QueryParam("keyword")))
	if keyword != "" {
		like := "%" + keyword + "%"
		query = query.Where("LOWER(email) LIKE ? OR LOWER(name) LIKE ? OR LOWER(nickname) LIKE ? OR phone LIKE ?", like, like, like, like)
	}

	var users []store.User
	if err := query.Order("name ASC").Order("email ASC").Order("id ASC").Find(&users).Error; err != nil {
		return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
	}

	contacts := make([]contactUserResponse, 0, len(users))
	userIDs := make([]string, 0, len(users))
	for _, user := range users {
		userIDs = append(userIDs, user.ID)
	}
	onlineStatus := s.realtime.OnlineStatus(userIDs)
	for _, user := range users {
		contacts = append(contacts, newContactUserResponse(user, onlineStatus[user.ID]))
	}

	return success(c, http.StatusOK, listContactUsersResponse{
		Contacts: contacts,
	})
}

func newContactUserResponse(user store.User, online bool) contactUserResponse {
	phone := ""
	if user.Phone != nil {
		phone = *user.Phone
	}
	avatar := user.Avatar
	if avatar == "" {
		avatar = store.DefaultUserAvatar
	}

	return contactUserResponse{
		Avatar:       avatar,
		Email:        user.Email,
		ID:           user.ID,
		LastOnlineAt: formatOptionalTime(user.LastOnlineAt),
		Name:         user.Name,
		Nickname:     user.Nickname,
		Online:       online,
		Phone:        phone,
		Type:         "user",
	}
}
