package usermanagement

import (
	"context"
	crand "crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"net/mail"
	"strconv"
	"strings"
	"time"

	projectapp "app/internal/application/project"
	"app/internal/auth"
	"app/internal/store"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const initialPasswordLength = 16

type Dependencies struct {
	DB                      *gorm.DB
	Presence                PresencePort
	AppConnections          AppConnectionPort
	Now                     func() time.Time
	NewID                   func() string
	GenerateInitialPassword func(int) (string, error)
	HashPassword            func(string) (string, error)
	GenerateAvatar          func() string
}

type Service struct {
	db                      *gorm.DB
	presence                PresencePort
	appConnections          AppConnectionPort
	now                     func() time.Time
	newID                   func() string
	generateInitialPassword func(int) (string, error)
	hashPassword            func(string) (string, error)
	generateAvatar          func() string
}

func NewService(deps Dependencies) *Service {
	now := deps.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	newID := deps.NewID
	if newID == nil {
		newID = uuid.NewString
	}
	generateInitialPassword := deps.GenerateInitialPassword
	if generateInitialPassword == nil {
		generateInitialPassword = auth.GenerateInitialPassword
	}
	hashPassword := deps.HashPassword
	if hashPassword == nil {
		hashPassword = auth.HashPassword
	}
	generateAvatar := deps.GenerateAvatar
	if generateAvatar == nil {
		generateAvatar = randomBuiltinAvatar
	}
	return &Service{
		db: deps.DB, presence: deps.Presence, appConnections: deps.AppConnections, now: now, newID: newID,
		generateInitialPassword: generateInitialPassword, hashPassword: hashPassword,
		generateAvatar: generateAvatar,
	}
}

func (s *Service) List(ctx context.Context, cmd ListCommand) (ListResult, error) {
	sortField, sortColumn, desc, order, err := parseListSort(cmd.Sort, cmd.Order)
	if err != nil {
		return ListResult{}, err
	}
	page, pageSize, err := parseListPagination(cmd.Page, cmd.PageSize)
	if err != nil {
		return ListResult{}, err
	}
	onlineFilter, err := parseListOnline(cmd.Online)
	if err != nil {
		return ListResult{}, err
	}
	query := s.db.WithContext(ctx).Model(&store.User{})
	keyword := strings.ToLower(strings.TrimSpace(cmd.Keyword))
	if keyword != "" {
		like := "%" + keyword + "%"
		query = query.Where("LOWER(email) LIKE ? OR LOWER(name) LIKE ? OR LOWER(nickname) LIKE ? OR phone LIKE ?", like, like, like, like)
	}
	var onlineStatus map[string]bool
	if onlineFilter != nil {
		var candidateIDs []string
		if err := query.Pluck("id", &candidateIDs).Error; err != nil {
			return ListResult{}, internalError(err)
		}
		onlineStatus = map[string]bool{}
		if s.presence != nil {
			onlineStatus = s.presence.OnlineStatus(candidateIDs)
		}
		filteredIDs := make([]string, 0, len(candidateIDs))
		for _, userID := range candidateIDs {
			if onlineStatus[userID] == *onlineFilter {
				filteredIDs = append(filteredIDs, userID)
			}
		}
		if len(filteredIDs) == 0 {
			query = query.Where("1 = 0")
		} else {
			query = query.Where("id IN ?", filteredIDs)
		}
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return ListResult{}, internalError(err)
	}
	var storedUsers []store.User
	if err := query.Order(clause.OrderByColumn{
		Column: clause.Column{Name: sortColumn}, Desc: desc,
	}).Limit(pageSize).Offset((page - 1) * pageSize).Find(&storedUsers).Error; err != nil {
		return ListResult{}, internalError(err)
	}
	userIDs := make([]string, 0, len(storedUsers))
	for _, user := range storedUsers {
		userIDs = append(userIDs, user.ID)
	}
	if onlineStatus == nil {
		onlineStatus = map[string]bool{}
	}
	if onlineFilter == nil && s.presence != nil {
		onlineStatus = s.presence.OnlineStatus(userIDs)
	}
	users := make([]User, 0, len(storedUsers))
	for _, user := range storedUsers {
		users = append(users, newUser(user, onlineStatus[user.ID]))
	}
	return ListResult{
		Users: users, Total: total, Page: page, PageSize: pageSize, Sort: sortField, Order: order,
	}, nil
}

func parseListOnline(rawOnline string) (*bool, error) {
	switch strings.ToLower(strings.TrimSpace(rawOnline)) {
	case "":
		return nil, nil
	case "true":
		value := true
		return &value, nil
	case "false":
		value := false
		return &value, nil
	default:
		return nil, newError(CodeInvalidRequest, "在线状态筛选参数不支持", nil)
	}
}

func (s *Service) Create(ctx context.Context, cmd CreateCommand) (CreateResult, error) {
	email, err := normalizeEmail(cmd.Email)
	if err != nil {
		return CreateResult{}, newError(CodeInvalidRequest, "邮箱格式错误", err)
	}
	name := strings.TrimSpace(cmd.Name)
	if name == "" {
		return CreateResult{}, newError(CodeInvalidRequest, "名称不能为空", nil)
	}
	phone, err := normalizePhone(cmd.Phone)
	if err != nil {
		return CreateResult{}, newError(CodeInvalidRequest, "手机号格式错误", err)
	}
	var existingCount int64
	if err := s.db.WithContext(ctx).Model(&store.User{}).Where("email = ?", email).Count(&existingCount).Error; err != nil {
		return CreateResult{}, internalError(err)
	}
	if existingCount > 0 {
		return CreateResult{}, newError(CodeConflict, "邮箱已存在", nil)
	}
	if phone != nil {
		if err := s.db.WithContext(ctx).Model(&store.User{}).Where("phone = ?", *phone).Count(&existingCount).Error; err != nil {
			return CreateResult{}, internalError(err)
		}
		if existingCount > 0 {
			return CreateResult{}, newError(CodeConflict, "手机号已存在", nil)
		}
	}
	initialPassword, err := s.generateInitialPassword(initialPasswordLength)
	if err != nil {
		return CreateResult{}, internalError(err)
	}
	passwordHash, err := s.hashPassword(initialPassword)
	if err != nil {
		return CreateResult{}, internalError(err)
	}
	storedUser := store.User{
		ID: s.newID(), Avatar: s.generateAvatar(), Email: email, Name: name, Nickname: "",
		Phone: phone, PasswordHash: passwordHash, Status: store.UserStatusActive,
	}
	var userInsertErr error
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&storedUser).Error; err != nil {
			userInsertErr = err
			return err
		}
		return projectapp.ProvisionPersonalWorkspace(tx, storedUser.ID, s.now().UTC())
	}); err != nil {
		if userInsertErr != nil && isUniqueConstraintError(userInsertErr) {
			return CreateResult{}, newError(CodeConflict, "邮箱或手机号已存在", userInsertErr)
		}
		return CreateResult{}, internalError(err)
	}
	return CreateResult{
		User: newUser(storedUser, false), InitialPassword: initialPassword,
	}, nil
}

func (s *Service) SetStatus(ctx context.Context, cmd SetStatusCommand) (User, error) {
	storedUser, err := s.find(ctx, cmd.UserID)
	if err != nil {
		return User{}, err
	}
	ownedAppIDs := []string{}
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if storedUser.Status != cmd.Status {
			if err := tx.Model(&storedUser).Update("status", cmd.Status).Error; err != nil {
				return err
			}
			storedUser.Status = cmd.Status
		}
		if cmd.Status == store.UserStatusDisabled {
			if err := tx.Where("user_id = ?", storedUser.ID).Delete(&store.UserSession{}).Error; err != nil {
				return err
			}
			if err := tx.Model(&store.App{}).Where("creator_user_id = ?", storedUser.ID).
				Pluck("id", &ownedAppIDs).Error; err != nil {
				return err
			}
			if len(ownedAppIDs) > 0 {
				if err := tx.Model(&store.App{}).Where("id IN ?", ownedAppIDs).
					Updates(map[string]any{"enabled": false, "updated_at": s.now().UTC()}).Error; err != nil {
					return err
				}
			}
		}
		return nil
	}); err != nil {
		return User{}, internalError(err)
	}
	if cmd.Status == store.UserStatusDisabled && s.presence != nil {
		s.presence.CloseUser(storedUser.ID)
	}
	if cmd.Status == store.UserStatusDisabled && s.appConnections != nil {
		for _, appID := range ownedAppIDs {
			s.appConnections.CloseApp(appID)
		}
	}
	return newUser(storedUser, s.isOnline(storedUser.ID)), nil
}

func (s *Service) ResetPassword(ctx context.Context, userID string) (ResetPasswordResult, error) {
	storedUser, err := s.find(ctx, userID)
	if err != nil {
		return ResetPasswordResult{}, err
	}
	newPassword, err := s.generateInitialPassword(initialPasswordLength)
	if err != nil {
		return ResetPasswordResult{}, internalError(err)
	}
	passwordHash, err := s.hashPassword(newPassword)
	if err != nil {
		return ResetPasswordResult{}, internalError(err)
	}
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&storedUser).Update("password_hash", passwordHash).Error; err != nil {
			return err
		}
		if err := tx.Where("user_id = ?", storedUser.ID).Delete(&store.UserSession{}).Error; err != nil {
			return err
		}
		storedUser.PasswordHash = passwordHash
		return nil
	}); err != nil {
		return ResetPasswordResult{}, internalError(err)
	}
	return ResetPasswordResult{
		User: newUser(storedUser, s.isOnline(storedUser.ID)), NewPassword: newPassword,
	}, nil
}

func (s *Service) find(ctx context.Context, userID string) (store.User, error) {
	id := strings.TrimSpace(userID)
	if _, err := uuid.Parse(id); err != nil {
		return store.User{}, newError(CodeInvalidRequest, "用户 ID 格式错误", err)
	}
	var storedUser store.User
	err := s.db.WithContext(ctx).First(&storedUser, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return store.User{}, newError(CodeNotFound, "用户不存在", err)
	}
	if err != nil {
		return store.User{}, internalError(err)
	}
	return storedUser, nil
}

func (s *Service) isOnline(userID string) bool {
	return s.presence != nil && s.presence.IsOnline(userID)
}

func newUser(storedUser store.User, online bool) User {
	phone := ""
	if storedUser.Phone != nil {
		phone = *storedUser.Phone
	}
	avatar := storedUser.Avatar
	if avatar == "" {
		avatar = store.DefaultUserAvatar
	}
	return User{
		ID: storedUser.ID, Avatar: avatar, Email: storedUser.Email,
		LastOnlineAt: storedUser.LastOnlineAt, Name: storedUser.Name, Nickname: storedUser.Nickname,
		Online: online, Phone: phone, Status: storedUser.Status, CreatedAt: storedUser.CreatedAt,
	}
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

func parseListSort(rawSort string, rawOrder string) (string, string, bool, string, error) {
	sortField := strings.ToLower(strings.TrimSpace(rawSort))
	if sortField == "" {
		sortField = "created_at"
	}
	sortColumns := map[string]string{"email": "email", "created_at": "created_at", "status": "status"}
	sortColumn, ok := sortColumns[sortField]
	if !ok {
		return "", "", false, "", newError(CodeInvalidRequest, "排序字段不支持", nil)
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
		return "", "", false, "", newError(CodeInvalidRequest, "排序方向不支持", nil)
	}
}

func parseListPagination(rawPage string, rawPageSize string) (int, int, error) {
	page, err := parsePositiveInt(rawPage, 1, "页码")
	if err != nil {
		return 0, 0, err
	}
	pageSize, err := parsePositiveInt(rawPageSize, 20, "每页数量")
	if err != nil {
		return 0, 0, err
	}
	if pageSize > 1000 {
		return 0, 0, newError(CodeInvalidRequest, "每页数量不能超过 1000", nil)
	}
	return page, pageSize, nil
}

func parsePositiveInt(raw string, defaultValue int, label string) (int, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return defaultValue, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 1 {
		return 0, newError(CodeInvalidRequest, label+"必须是正整数", err)
	}
	return parsed, nil
}

func randomBuiltinAvatar() string {
	index, err := crand.Int(crand.Reader, big.NewInt(64))
	if err != nil {
		return store.DefaultUserAvatar
	}
	return fmt.Sprintf("/assets/avatars/builtin/%02d.webp", index.Int64()+1)
}

func isUniqueConstraintError(err error) bool {
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "unique") || strings.Contains(message, "duplicate")
}

var _ AdminService = (*Service)(nil)
