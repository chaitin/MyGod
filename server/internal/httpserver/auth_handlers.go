package httpserver

import (
	crand "crypto/rand"
	"crypto/subtle"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"net/mail"
	"strconv"
	"strings"
	"time"

	"app/internal/auth"
	"app/internal/store"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type loginRequest struct {
	Email    string `json:"email" example:"user@example.com"`
	Password string `json:"password" example:"password"`
}

type createUserRequest struct {
	Email string `json:"email" example:"user@example.com"`
	Name  string `json:"name" example:"张三"`
	Phone string `json:"phone" example:"13812345678"`
}

type updateCurrentUserRequest struct {
	Avatar   *string `json:"avatar" example:"/assets/avatars/builtin/07.webp"`
	Nickname *string `json:"nickname" example:"小张"`
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

type adminResponse struct {
	Email string `json:"email" example:"admin"`
}

type adminLoginResponse struct {
	Admin adminResponse `json:"admin"`
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

type userLoginResponse struct {
	User userResponse `json:"user"`
}

type currentUserResponse struct {
	User userResponse `json:"user"`
}

// adminLogin godoc
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
func (s *Server) adminLogin(c echo.Context) error {
	var req loginRequest
	if err := c.Bind(&req); err != nil {
		return failure(c, http.StatusBadRequest, "invalid_request", "请求格式错误")
	}

	if strings.TrimSpace(req.Email) != "admin" ||
		subtle.ConstantTimeCompare([]byte(req.Password), []byte(s.cfg.Admin.Password)) != 1 {
		return failure(c, http.StatusUnauthorized, "invalid_credentials", "邮箱或密码错误")
	}

	token, err := auth.GenerateSessionToken()
	if err != nil {
		return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
	}

	now := time.Now().UTC()
	session := store.AdminSession{
		ID:         uuid.NewString(),
		TokenHash:  auth.HashSessionToken(token),
		ExpiresAt:  now.Add(sessionTTL),
		LastSeenAt: now,
		UserAgent:  c.Request().UserAgent(),
		IP:         c.RealIP(),
	}
	if err := s.db.Create(&session).Error; err != nil {
		return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
	}

	setSessionCookie(c, adminSessionCookieName, token, session.ExpiresAt)
	return success(c, http.StatusOK, adminLoginResponse{
		Admin: adminResponse{
			Email: "admin",
		},
	})
}

// listUsers godoc
//
// @Summary 列出普通用户
// @Description 管理员列出普通用户。keyword 会同时搜索邮箱、名称、昵称和手机号；sort 仅支持 email、created_at、status；order 仅支持 asc、desc。
// @Tags 管理员用户
// @Produce json
// @Param keyword query string false "搜索关键字，匹配邮箱、名称、昵称或手机号"
// @Param page query int false "页码，从 1 开始"
// @Param page_size query int false "每页数量，最大 1000"
// @Param sort query string false "排序字段：email、created_at、status"
// @Param order query string false "排序方向：asc、desc"
// @Success 200 {object} successEnvelope{data=listUsersResponse}
// @Failure 400 {object} errorEnvelope
// @Failure 401 {object} errorEnvelope
// @Failure 500 {object} errorEnvelope
// @Router /api/admin/users [get]
func (s *Server) listUsers(c echo.Context) error {
	sortField, sortColumn, desc, order, err := parseUserListSort(c.QueryParam("sort"), c.QueryParam("order"))
	if err != nil {
		return failure(c, http.StatusBadRequest, "invalid_request", err.Error())
	}
	page, pageSize, err := parseUserListPagination(c.QueryParam("page"), c.QueryParam("page_size"))
	if err != nil {
		return failure(c, http.StatusBadRequest, "invalid_request", err.Error())
	}

	query := s.db.Model(&store.User{})
	keyword := strings.ToLower(strings.TrimSpace(c.QueryParam("keyword")))
	if keyword != "" {
		like := "%" + keyword + "%"
		query = query.Where("LOWER(email) LIKE ? OR LOWER(name) LIKE ? OR LOWER(nickname) LIKE ? OR phone LIKE ?", like, like, like, like)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
	}

	var users []store.User
	if err := query.Order(clause.OrderByColumn{
		Column: clause.Column{Name: sortColumn},
		Desc:   desc,
	}).Limit(pageSize).Offset((page - 1) * pageSize).Find(&users).Error; err != nil {
		return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
	}

	responses := make([]userResponse, 0, len(users))
	userIDs := make([]string, 0, len(users))
	for _, user := range users {
		userIDs = append(userIDs, user.ID)
	}
	onlineStatus := s.realtime.OnlineStatus(userIDs)
	for _, user := range users {
		responses = append(responses, newAdminUserResponse(user, onlineStatus[user.ID]))
	}

	return success(c, http.StatusOK, listUsersResponse{
		Users:    responses,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
		Sort:     sortField,
		Order:    order,
	})
}

// createUser godoc
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
func (s *Server) createUser(c echo.Context) error {
	var req createUserRequest
	if err := c.Bind(&req); err != nil {
		return failure(c, http.StatusBadRequest, "invalid_request", "请求格式错误")
	}

	email, err := normalizeEmail(req.Email)
	if err != nil {
		return failure(c, http.StatusBadRequest, "invalid_request", "邮箱格式错误")
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return failure(c, http.StatusBadRequest, "invalid_request", "名称不能为空")
	}
	phone, err := normalizePhone(req.Phone)
	if err != nil {
		return failure(c, http.StatusBadRequest, "invalid_request", "手机号格式错误")
	}

	var existingCount int64
	if err := s.db.Model(&store.User{}).Where("email = ?", email).Count(&existingCount).Error; err != nil {
		return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
	}
	if existingCount > 0 {
		return failure(c, http.StatusConflict, "conflict", "邮箱已存在")
	}
	if phone != nil {
		if err := s.db.Model(&store.User{}).Where("phone = ?", *phone).Count(&existingCount).Error; err != nil {
			return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
		}
		if existingCount > 0 {
			return failure(c, http.StatusConflict, "conflict", "手机号已存在")
		}
	}

	initialPassword, err := auth.GenerateInitialPassword(16)
	if err != nil {
		return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
	}
	passwordHash, err := auth.HashPassword(initialPassword)
	if err != nil {
		return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
	}

	user := store.User{
		ID:           uuid.NewString(),
		Avatar:       randomBuiltinAvatar(),
		Email:        email,
		Name:         name,
		Nickname:     "",
		Phone:        phone,
		PasswordHash: passwordHash,
		Status:       store.UserStatusActive,
	}
	var userInsertErr error
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&user).Error; err != nil {
			userInsertErr = err
			return err
		}

		return createPersonalProject(tx, user, time.Now().UTC())
	}); err != nil {
		if userInsertErr != nil && isUniqueConstraintError(userInsertErr) {
			return failure(c, http.StatusConflict, "conflict", "邮箱或手机号已存在")
		}
		return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
	}

	return success(c, http.StatusCreated, createUserResponse{
		User:            newAdminUserResponse(user, false),
		InitialPassword: initialPassword,
	})
}

// disableUser godoc
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
func (s *Server) disableUser(c echo.Context) error {
	return s.updateUserStatus(c, store.UserStatusDisabled)
}

// enableUser godoc
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
func (s *Server) enableUser(c echo.Context) error {
	return s.updateUserStatus(c, store.UserStatusActive)
}

// resetUserPassword godoc
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
func (s *Server) resetUserPassword(c echo.Context) error {
	id := strings.TrimSpace(c.Param("id"))
	if _, err := uuid.Parse(id); err != nil {
		return failure(c, http.StatusBadRequest, "invalid_request", "用户 ID 格式错误")
	}

	var user store.User
	err := s.db.First(&user, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return failure(c, http.StatusNotFound, "not_found", "用户不存在")
	}
	if err != nil {
		return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
	}

	newPassword, err := auth.GenerateInitialPassword(16)
	if err != nil {
		return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
	}
	passwordHash, err := auth.HashPassword(newPassword)
	if err != nil {
		return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
	}

	if err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&user).Update("password_hash", passwordHash).Error; err != nil {
			return err
		}
		if err := tx.Where("user_id = ?", user.ID).Delete(&store.UserSession{}).Error; err != nil {
			return err
		}
		user.PasswordHash = passwordHash
		return nil
	}); err != nil {
		return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
	}

	return success(c, http.StatusOK, resetUserPasswordResponse{
		User:        newAdminUserResponse(user, s.realtime.IsOnline(user.ID)),
		NewPassword: newPassword,
	})
}

// userLogin godoc
//
// @Summary 普通用户登录
// @Description 普通用户使用管理员创建的邮箱和密码登录。
// @Tags 客户端认证
// @Accept json
// @Produce json
// @Param body body loginRequest true "登录参数"
// @Success 200 {object} successEnvelope{data=userLoginResponse}
// @Failure 400 {object} errorEnvelope
// @Failure 401 {object} errorEnvelope
// @Failure 500 {object} errorEnvelope
// @Router /api/client/auth/login [post]
func (s *Server) userLogin(c echo.Context) error {
	var req loginRequest
	if err := c.Bind(&req); err != nil {
		return failure(c, http.StatusBadRequest, "invalid_request", "请求格式错误")
	}

	email, err := normalizeEmail(req.Email)
	if err != nil {
		return failure(c, http.StatusUnauthorized, "invalid_credentials", "邮箱或密码错误")
	}

	var user store.User
	err = s.db.Where("email = ?", email).First(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return failure(c, http.StatusUnauthorized, "invalid_credentials", "邮箱或密码错误")
	}
	if err != nil {
		return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
	}
	if user.Status != store.UserStatusActive {
		return failure(c, http.StatusUnauthorized, "invalid_credentials", "邮箱或密码错误")
	}

	ok, err := auth.VerifyPassword(req.Password, user.PasswordHash)
	if err != nil || !ok {
		return failure(c, http.StatusUnauthorized, "invalid_credentials", "邮箱或密码错误")
	}

	token, err := auth.GenerateSessionToken()
	if err != nil {
		return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
	}

	now := time.Now().UTC()
	session := store.UserSession{
		ID:         uuid.NewString(),
		TokenHash:  auth.HashSessionToken(token),
		UserID:     user.ID,
		ExpiresAt:  now.Add(sessionTTL),
		LastSeenAt: now,
		UserAgent:  c.Request().UserAgent(),
		IP:         c.RealIP(),
	}
	if err := s.db.Create(&session).Error; err != nil {
		return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
	}

	setSessionCookie(c, userSessionCookieName, token, session.ExpiresAt)
	return success(c, http.StatusOK, userLoginResponse{
		User: newUserResponse(user),
	})
}

// userLogout godoc
//
// @Summary 普通用户退出登录
// @Description 删除当前普通用户会话并清除客户端登录 Cookie。重复调用也会返回成功。
// @Tags 客户端认证
// @Produce json
// @Success 200 {object} successEnvelope
// @Failure 500 {object} errorEnvelope
// @Router /api/client/auth/logout [post]
func (s *Server) userLogout(c echo.Context) error {
	cookie, err := c.Cookie(userSessionCookieName)
	if err == nil && cookie.Value != "" {
		if err := s.db.Where("token_hash = ?", auth.HashSessionToken(cookie.Value)).Delete(&store.UserSession{}).Error; err != nil {
			return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
		}
	}

	clearSessionCookie(c, userSessionCookieName)
	return success(c, http.StatusOK, map[string]any{})
}

// getCurrentUser godoc
//
// @Summary 获取当前用户
// @Description 普通用户获取当前登录用户信息。
// @Tags 客户端认证
// @Produce json
// @Success 200 {object} successEnvelope{data=currentUserResponse}
// @Failure 401 {object} errorEnvelope
// @Failure 500 {object} errorEnvelope
// @Router /api/client/me [get]
func (s *Server) getCurrentUser(c echo.Context) error {
	user, ok := currentUser(c)
	if !ok {
		return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
	}

	return success(c, http.StatusOK, currentUserResponse{
		User: newUserResponse(user),
	})
}

// updateCurrentUser godoc
//
// @Summary 修改当前用户资料
// @Description 普通用户修改自己的昵称和头像。avatar 与 nickname 均可单独传入；nickname 可传空字符串用于清空。
// @Tags 客户端认证
// @Accept json
// @Produce json
// @Param body body updateCurrentUserRequest true "用户资料"
// @Success 200 {object} successEnvelope{data=currentUserResponse}
// @Failure 400 {object} errorEnvelope
// @Failure 401 {object} errorEnvelope
// @Failure 500 {object} errorEnvelope
// @Router /api/client/me [patch]
func (s *Server) updateCurrentUser(c echo.Context) error {
	user, ok := currentUser(c)
	if !ok {
		return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
	}

	var req updateCurrentUserRequest
	if err := c.Bind(&req); err != nil {
		return failure(c, http.StatusBadRequest, "invalid_request", "请求格式错误")
	}

	updates := map[string]any{}
	if req.Avatar != nil {
		avatar, err := normalizeBuiltinAvatar(*req.Avatar)
		if err != nil {
			return failure(c, http.StatusBadRequest, "invalid_request", "头像格式错误")
		}
		updates["avatar"] = avatar
	}
	if req.Nickname != nil {
		updates["nickname"] = strings.TrimSpace(*req.Nickname)
	}
	if len(updates) == 0 {
		return failure(c, http.StatusBadRequest, "invalid_request", "至少需要修改一个字段")
	}

	if err := s.db.Model(&store.User{}).Where("id = ?", user.ID).Updates(updates).Error; err != nil {
		return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
	}

	var updatedUser store.User
	if err := s.db.First(&updatedUser, "id = ?", user.ID).Error; err != nil {
		return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
	}

	return success(c, http.StatusOK, currentUserResponse{
		User: newUserResponse(updatedUser),
	})
}

func normalizeEmail(raw string) (string, error) {
	email := strings.ToLower(strings.TrimSpace(raw))
	address, err := mail.ParseAddress(email)
	if err != nil || address.Address != email {
		return "", errors.New("invalid email")
	}

	return email, nil
}

func normalizePhone(raw string) (*string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, nil
	}

	var builder strings.Builder
	for index, char := range trimmed {
		switch {
		case char >= '0' && char <= '9':
			builder.WriteRune(char)
		case char == '+' && index == 0:
			builder.WriteRune(char)
		case char == ' ' || char == '\t' || char == '\n' || char == '\r' || char == '-' || char == '(' || char == ')':
			continue
		default:
			return nil, errors.New("invalid phone")
		}
	}

	normalized := builder.String()
	if normalized == "" || normalized == "+" {
		return nil, errors.New("invalid phone")
	}
	if strings.HasPrefix(normalized, "+") {
		digits := strings.TrimPrefix(normalized, "+")
		if len(digits) < 6 || len(digits) > 15 {
			return nil, errors.New("invalid phone")
		}
		return &normalized, nil
	}
	if len(normalized) != 11 {
		return nil, errors.New("invalid phone")
	}

	normalized = "+86" + normalized
	return &normalized, nil
}

func normalizeBuiltinAvatar(raw string) (string, error) {
	avatar := strings.TrimSpace(raw)
	const prefix = "/assets/avatars/builtin/"
	const suffix = ".webp"
	if !strings.HasPrefix(avatar, prefix) || !strings.HasSuffix(avatar, suffix) {
		return "", errors.New("invalid avatar")
	}

	id := strings.TrimSuffix(strings.TrimPrefix(avatar, prefix), suffix)
	if len(id) != 2 {
		return "", errors.New("invalid avatar")
	}
	index, err := strconv.Atoi(id)
	if err != nil || index < 1 || index > 64 {
		return "", errors.New("invalid avatar")
	}
	if fmt.Sprintf("%02d", index) != id {
		return "", errors.New("invalid avatar")
	}

	return avatar, nil
}

func parseUserListSort(rawSort string, rawOrder string) (string, string, bool, string, error) {
	sortField := strings.ToLower(strings.TrimSpace(rawSort))
	if sortField == "" {
		sortField = "created_at"
	}

	sortColumns := map[string]string{
		"email":      "email",
		"created_at": "created_at",
		"status":     "status",
	}
	sortColumn, ok := sortColumns[sortField]
	if !ok {
		return "", "", false, "", errors.New("排序字段不支持")
	}

	order := strings.ToLower(strings.TrimSpace(rawOrder))
	if order == "" {
		order = "desc"
	}

	switch order {
	case "asc":
		return sortField, sortColumn, false, order, nil
	case "desc":
		return sortField, sortColumn, true, order, nil
	default:
		return "", "", false, "", errors.New("排序方向不支持")
	}
}

func parseUserListPagination(rawPage string, rawPageSize string) (int, int, error) {
	page, err := parsePositiveIntQuery(rawPage, 1, "页码")
	if err != nil {
		return 0, 0, err
	}

	pageSize, err := parsePositiveIntQuery(rawPageSize, 20, "每页数量")
	if err != nil {
		return 0, 0, err
	}
	if pageSize > 1000 {
		return 0, 0, errors.New("每页数量不能超过 1000")
	}

	return page, pageSize, nil
}

func parsePositiveIntQuery(raw string, defaultValue int, label string) (int, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return defaultValue, nil
	}

	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 1 {
		return 0, errors.New(label + "必须是正整数")
	}

	return parsed, nil
}

func newUserResponse(user store.User) userResponse {
	phone := ""
	if user.Phone != nil {
		phone = *user.Phone
	}
	avatar := user.Avatar
	if avatar == "" {
		avatar = store.DefaultUserAvatar
	}

	return userResponse{
		ID:           user.ID,
		Avatar:       avatar,
		Email:        user.Email,
		LastOnlineAt: formatOptionalTime(user.LastOnlineAt),
		Name:         user.Name,
		Nickname:     user.Nickname,
		Phone:        phone,
		Status:       user.Status,
		CreatedAt:    user.CreatedAt,
	}
}

func newAdminUserResponse(user store.User, online bool) userResponse {
	response := newUserResponse(user)
	response.Online = &online
	return response
}

func randomBuiltinAvatar() string {
	index, err := crand.Int(crand.Reader, big.NewInt(64))
	if err != nil {
		return store.DefaultUserAvatar
	}

	return fmt.Sprintf("/assets/avatars/builtin/%02d.webp", index.Int64()+1)
}

func (s *Server) updateUserStatus(c echo.Context, status string) error {
	id := strings.TrimSpace(c.Param("id"))
	if _, err := uuid.Parse(id); err != nil {
		return failure(c, http.StatusBadRequest, "invalid_request", "用户 ID 格式错误")
	}

	var user store.User
	err := s.db.First(&user, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return failure(c, http.StatusNotFound, "not_found", "用户不存在")
	}
	if err != nil {
		return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
	}

	if err := s.db.Transaction(func(tx *gorm.DB) error {
		if user.Status != status {
			if err := tx.Model(&user).Update("status", status).Error; err != nil {
				return err
			}
			user.Status = status
		}
		if status == store.UserStatusDisabled {
			if err := tx.Where("user_id = ?", user.ID).Delete(&store.UserSession{}).Error; err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
	}
	if status == store.UserStatusDisabled {
		s.realtime.CloseUser(user.ID)
	}

	return success(c, http.StatusOK, updateUserStatusResponse{
		User: newAdminUserResponse(user, s.realtime.IsOnline(user.ID)),
	})
}

func isUniqueConstraintError(err error) bool {
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "unique") || strings.Contains(message, "duplicate")
}
