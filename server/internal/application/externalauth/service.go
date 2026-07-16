package externalauth

import (
	"context"
	crand "crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/mail"
	"net/url"
	"strings"
	"time"

	"app/internal/application/identityprovider"
	projectapp "app/internal/application/project"
	"app/internal/auth"
	"app/internal/store"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	defaultStateTTL   = 10 * time.Minute
	defaultSessionTTL = 7 * 24 * time.Hour
)

type Dependencies struct {
	DB                   *gorm.DB
	Providers            identityprovider.LoginProviderService
	OAuth                OAuthPort
	Now                  func() time.Time
	NewID                func() string
	GenerateRandomValue  func(int) (string, error)
	GenerateSessionToken func() (string, error)
	RandomAvatar         func() string
	StateTTL             time.Duration
	SessionTTL           time.Duration
}

type Service struct {
	db                   *gorm.DB
	providers            identityprovider.LoginProviderService
	oauth                OAuthPort
	now                  func() time.Time
	newID                func() string
	generateRandomValue  func(int) (string, error)
	generateSessionToken func() (string, error)
	randomAvatar         func() string
	stateTTL             time.Duration
	sessionTTL           time.Duration
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
	generateRandomValue := deps.GenerateRandomValue
	if generateRandomValue == nil {
		generateRandomValue = generateRandomValueDefault
	}
	generateSessionToken := deps.GenerateSessionToken
	if generateSessionToken == nil {
		generateSessionToken = auth.GenerateSessionToken
	}
	randomAvatar := deps.RandomAvatar
	if randomAvatar == nil {
		randomAvatar = randomBuiltinAvatar
	}
	stateTTL := deps.StateTTL
	if stateTTL <= 0 {
		stateTTL = defaultStateTTL
	}
	sessionTTL := deps.SessionTTL
	if sessionTTL <= 0 {
		sessionTTL = defaultSessionTTL
	}
	return &Service{
		db: deps.DB, providers: deps.Providers, oauth: deps.OAuth, now: now, newID: newID,
		generateRandomValue: generateRandomValue, generateSessionToken: generateSessionToken,
		randomAvatar: randomAvatar, stateTTL: stateTTL, sessionTTL: sessionTTL,
	}
}

func (s *Service) Start(ctx context.Context, cmd StartCommand) (StartResult, error) {
	provider, err := s.enabledProvider(ctx, cmd.ProviderKey)
	if err != nil {
		return StartResult{}, err
	}
	redirectPath, err := normalizeRedirectPath(cmd.Redirect)
	if err != nil {
		return StartResult{}, newError(CodeInvalidRequest, "登录跳转地址格式错误", err)
	}
	state, err := s.generateRandomValue(32)
	if err != nil {
		return StartResult{}, internalError(err)
	}
	codeVerifier, err := s.generateRandomValue(32)
	if err != nil {
		return StartResult{}, internalError(err)
	}

	now := s.now().UTC()
	if err := s.cleanupLoginStates(ctx, now); err != nil {
		return StartResult{}, internalError(err)
	}
	expiresAt := now.Add(s.stateTTL)
	loginState := store.ThirdPartyLoginState{
		StateHash: auth.HashSessionToken(state), ProviderID: provider.ID, CodeVerifier: codeVerifier,
		RedirectPath: redirectPath, ExpiresAt: expiresAt, IP: cmd.IP, UserAgent: cmd.UserAgent,
	}
	if err := s.db.WithContext(ctx).Create(&loginState).Error; err != nil {
		return StartResult{}, internalError(err)
	}
	result := StartResult{State: state, ExpiresAt: expiresAt}
	if s.oauth == nil {
		return result, internalError(errors.New("external auth oauth adapter is not configured"))
	}
	callbackURL := callbackURLForProvider(cmd.CallbackURLForProvider, provider.Key)
	result.AuthorizeURL, err = s.oauth.BuildAuthorizeURL(provider, state, callbackURL, codeVerifier)
	if err != nil {
		return result, internalError(err)
	}
	return result, nil
}

func (s *Service) Finish(ctx context.Context, cmd FinishCommand) (FinishResult, error) {
	provider, err := s.enabledProvider(ctx, cmd.ProviderKey)
	if err != nil {
		return FinishResult{}, err
	}
	code := strings.TrimSpace(cmd.Code)
	state := strings.TrimSpace(cmd.State)
	if code == "" || state == "" {
		return FinishResult{}, newError(CodeInvalidRequest, "第三方登录回调参数错误", nil)
	}
	if !sameState(state, cmd.CookieState) {
		return FinishResult{}, newError(CodeInvalidRequest, "第三方登录状态已失效", nil)
	}
	loginState, err := s.consumeLoginState(ctx, provider.ID, state)
	if err != nil {
		return FinishResult{}, newError(CodeInvalidRequest, "第三方登录状态已失效", err)
	}
	if s.oauth == nil {
		return FinishResult{}, internalError(errors.New("external auth oauth adapter is not configured"))
	}
	callbackURL := callbackURLForProvider(cmd.CallbackURLForProvider, provider.Key)
	profile, err := s.oauth.FetchProfile(ctx, provider, code, callbackURL, loginState.CodeVerifier)
	if err != nil {
		return FinishResult{}, oauthFailure(err)
	}
	user, err := s.ResolveUser(ctx, provider, profile)
	if err != nil {
		return FinishResult{}, err
	}
	session, err := s.createSession(ctx, user.ID, cmd.UserAgent, cmd.IP)
	if err != nil {
		return FinishResult{}, internalError(err)
	}
	return FinishResult{RedirectPath: loginState.RedirectPath, Session: session}, nil
}

func (s *Service) ResolveUser(ctx context.Context, provider identityprovider.Provider, profile Profile) (store.User, error) {
	if strings.TrimSpace(profile.ExternalUserID) == "" {
		return store.User{}, newError(CodeInvalidThirdPartyLogin, "第三方用户标识为空", nil)
	}
	email, emailFromProvider, err := profileEmail(profile)
	if err != nil {
		return store.User{}, newError(CodeInvalidThirdPartyLogin, "第三方邮箱格式错误", err)
	}
	if !emailFromProvider {
		return store.User{}, newError(CodeInvalidThirdPartyLogin, "第三方邮箱为空", nil)
	}
	if len(profile.Raw) == 0 {
		profile.Raw = json.RawMessage(`{}`)
	}

	var resultUser store.User
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		user, found, findErr := findUserByEmail(tx, email)
		if findErr != nil {
			return findErr
		}
		if found {
			updatedUser, updateErr := syncUserFields(tx, user, profile)
			if updateErr != nil {
				return updateErr
			}
			if upsertErr := s.upsertAccount(tx, provider, profile, updatedUser.ID); upsertErr != nil {
				return upsertErr
			}
			resultUser = updatedUser
			return nil
		}

		var account store.ThirdPartyAccount
		err := tx.Preload("User").Where(
			"provider_id = ? AND external_user_id = ?", provider.ID, profile.ExternalUserID,
		).First(&account).Error
		if err == nil {
			if account.User.ID == "" {
				return internalError(errors.New("third-party account user is empty"))
			}
			if account.User.Status != store.UserStatusActive {
				return newError(CodeInvalidCredentials, "用户已被禁用", nil)
			}
			if updateErr := tx.Model(&account).Update("profile", profile.Raw).Error; updateErr != nil {
				return updateErr
			}
			updatedUser, updateErr := syncUserFields(tx, account.User, profile)
			if updateErr != nil {
				return updateErr
			}
			resultUser = updatedUser
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		user, err = s.findOrCreateBoundUser(tx, profile, email)
		if err != nil {
			return err
		}
		if err := s.upsertAccount(tx, provider, profile, user.ID); err != nil {
			return err
		}
		resultUser = user
		return nil
	})
	if err != nil {
		var externalErr *Error
		if errors.As(err, &externalErr) {
			return store.User{}, err
		}
		return store.User{}, internalError(err)
	}
	return resultUser, nil
}

func (s *Service) enabledProvider(ctx context.Context, key string) (identityprovider.Provider, error) {
	if s.providers == nil {
		return identityprovider.Provider{}, internalError(errors.New("identity provider service is not configured"))
	}
	provider, err := s.providers.GetEnabledByKey(ctx, key)
	if err == nil {
		return provider, nil
	}
	if identityprovider.ErrorCodeOf(err) == identityprovider.CodeNotFound {
		return identityprovider.Provider{}, newError(CodeNotFound, "第三方登录方式不存在", err)
	}
	return identityprovider.Provider{}, internalError(err)
}

func (s *Service) cleanupLoginStates(ctx context.Context, now time.Time) error {
	consumedBefore := now.Add(-s.stateTTL)
	return s.db.WithContext(ctx).Where(
		"expires_at <= ? OR (consumed_at IS NOT NULL AND consumed_at <= ?)", now, consumedBefore,
	).Delete(&store.ThirdPartyLoginState{}).Error
}

func (s *Service) consumeLoginState(ctx context.Context, providerID, state string) (store.ThirdPartyLoginState, error) {
	now := s.now().UTC()
	var loginState store.ThirdPartyLoginState
	result := s.db.WithContext(ctx).Model(&loginState).Clauses(clause.Returning{}).Where(
		"state_hash = ? AND provider_id = ? AND consumed_at IS NULL AND expires_at > ?",
		auth.HashSessionToken(state), providerID, now,
	).Update("consumed_at", now)
	if result.Error != nil {
		return store.ThirdPartyLoginState{}, result.Error
	}
	if result.RowsAffected == 0 {
		return store.ThirdPartyLoginState{}, gorm.ErrRecordNotFound
	}
	return loginState, nil
}

func (s *Service) createSession(ctx context.Context, userID, userAgent, ip string) (SessionCredential, error) {
	token, err := s.generateSessionToken()
	if err != nil {
		return SessionCredential{}, err
	}
	now := s.now().UTC()
	session := store.UserSession{
		ID: s.newID(), TokenHash: auth.HashSessionToken(token), UserID: userID,
		ExpiresAt: now.Add(s.sessionTTL), CreatedAt: now, LastSeenAt: now, UserAgent: userAgent, IP: ip,
	}
	if err := s.db.WithContext(ctx).Create(&session).Error; err != nil {
		return SessionCredential{}, err
	}
	return SessionCredential{Token: token, ExpiresAt: session.ExpiresAt}, nil
}

func (s *Service) upsertAccount(tx *gorm.DB, provider identityprovider.Provider, profile Profile, userID string) error {
	var account store.ThirdPartyAccount
	err := tx.Where("provider_id = ? AND external_user_id = ?", provider.ID, profile.ExternalUserID).First(&account).Error
	if err == nil {
		return tx.Model(&account).Updates(map[string]any{"profile": profile.Raw, "user_id": userID}).Error
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	account = store.ThirdPartyAccount{
		ID: s.newID(), ProviderID: provider.ID, ExternalUserID: profile.ExternalUserID,
		UserID: userID, Profile: profile.Raw,
	}
	if err := tx.Create(&account).Error; err != nil {
		if isUniqueConstraintError(err) {
			return newError(CodeConflict, "第三方账号已绑定", err)
		}
		return err
	}
	return nil
}

func (s *Service) findOrCreateBoundUser(tx *gorm.DB, profile Profile, email string) (store.User, error) {
	var user store.User
	err := tx.Where("email = ?", email).First(&user).Error
	if err == nil {
		if user.Status != store.UserStatusActive {
			return store.User{}, newError(CodeInvalidCredentials, "用户已被禁用", nil)
		}
		return syncUserFields(tx, user, profile)
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return store.User{}, err
	}

	phone, err := normalizePhone(profile.Phone)
	if err != nil {
		return store.User{}, newError(CodeInvalidThirdPartyLogin, "手机号格式错误", err)
	}
	if err := ensurePhoneAvailable(tx, phone, ""); err != nil {
		return store.User{}, err
	}
	password, err := auth.GenerateInitialPassword(32)
	if err != nil {
		return store.User{}, err
	}
	passwordHash, err := auth.HashPassword(password)
	if err != nil {
		return store.User{}, err
	}
	name := strings.TrimSpace(profile.Name)
	if name == "" {
		name = strings.TrimSpace(profile.Nickname)
	}
	if name == "" {
		name = emailPrefix(email)
	}
	if name == "" {
		name = profile.ExternalUserID
	}
	avatar := normalizeAvatar(profile.Avatar)
	if avatar == "" {
		avatar = s.randomAvatar()
	}
	user = store.User{
		ID: s.newID(), Avatar: avatar, Email: email, Name: name,
		Nickname: strings.TrimSpace(profile.Nickname), Phone: phone,
		PasswordHash: passwordHash, Status: store.UserStatusActive,
	}
	if err := tx.Create(&user).Error; err != nil {
		if isUniqueConstraintError(err) {
			return store.User{}, newError(CodeConflict, "邮箱或手机号已存在", err)
		}
		return store.User{}, err
	}
	if err := projectapp.ProvisionPersonalWorkspace(tx, user.ID, s.now().UTC()); err != nil {
		return store.User{}, err
	}
	return user, nil
}

func findUserByEmail(tx *gorm.DB, email string) (store.User, bool, error) {
	var user store.User
	err := tx.Where("email = ?", email).First(&user).Error
	if err == nil {
		if user.Status != store.UserStatusActive {
			return store.User{}, false, newError(CodeInvalidCredentials, "用户已被禁用", nil)
		}
		return user, true, nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return store.User{}, false, nil
	}
	return store.User{}, false, err
}

func syncUserFields(tx *gorm.DB, user store.User, profile Profile) (store.User, error) {
	updates := map[string]any{}
	name := strings.TrimSpace(profile.Name)
	if name != "" && name != strings.TrimSpace(user.Name) {
		updates["name"] = name
		user.Name = name
	}
	if email, fromProvider, err := profileEmail(profile); err != nil {
		return store.User{}, newError(CodeInvalidThirdPartyLogin, "第三方邮箱格式错误", err)
	} else if fromProvider && email != strings.TrimSpace(strings.ToLower(user.Email)) && isSyntheticEmail(user.Email) {
		updates["email"] = email
		user.Email = email
	}
	rawPhone := strings.TrimSpace(profile.Phone)
	if rawPhone != "" {
		phone, err := normalizePhone(rawPhone)
		if err != nil {
			return store.User{}, newError(CodeInvalidThirdPartyLogin, "手机号格式错误", err)
		}
		if phone != nil {
			currentPhone := ""
			if user.Phone != nil {
				currentPhone = *user.Phone
			}
			if *phone != currentPhone {
				if err := ensurePhoneAvailable(tx, phone, user.ID); err != nil {
					return store.User{}, err
				}
				updates["phone"] = *phone
				user.Phone = phone
			}
		}
	}
	if len(updates) == 0 {
		return user, nil
	}
	if err := tx.Model(&store.User{}).Where("id = ?", user.ID).Updates(updates).Error; err != nil {
		if isUniqueConstraintError(err) {
			return store.User{}, newError(CodeConflict, "邮箱或手机号已存在", err)
		}
		return store.User{}, err
	}
	return user, nil
}

func ensurePhoneAvailable(tx *gorm.DB, phone *string, userID string) error {
	if phone == nil {
		return nil
	}
	query := tx.Model(&store.User{}).Where("phone = ?", *phone)
	if userID != "" {
		query = query.Where("id <> ?", userID)
	}
	var count int64
	if err := query.Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return newError(CodeConflict, "手机号已存在", nil)
	}
	return nil
}

func profileEmail(profile Profile) (string, bool, error) {
	rawEmail := strings.TrimSpace(profile.Email)
	if rawEmail == "" {
		return "", false, nil
	}
	email, err := normalizeEmail(rawEmail)
	return email, true, err
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

func normalizeRedirectPath(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "/init", nil
	}
	if !strings.HasPrefix(trimmed, "/") || strings.HasPrefix(trimmed, "//") {
		return "", errors.New("invalid redirect path")
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.IsAbs() || parsed.Host != "" {
		return "", errors.New("invalid redirect path")
	}
	return trimmed, nil
}

func callbackURLForProvider(callback func(string) string, providerKey string) string {
	if callback == nil {
		return ""
	}
	return callback(providerKey)
}

func sameState(state, cookieState string) bool {
	if strings.TrimSpace(cookieState) == "" {
		return false
	}
	stateHash := auth.HashSessionToken(state)
	cookieHash := auth.HashSessionToken(cookieState)
	return subtle.ConstantTimeCompare([]byte(stateHash), []byte(cookieHash)) == 1
}

func generateRandomValueDefault(size int) (string, error) {
	value := make([]byte, size)
	if _, err := crand.Read(value); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

func randomBuiltinAvatar() string {
	index, err := crand.Int(crand.Reader, big.NewInt(64))
	if err != nil {
		return store.DefaultUserAvatar
	}
	return fmt.Sprintf("/assets/avatars/builtin/%02d.webp", index.Int64()+1)
}

func normalizeAvatar(value string) string {
	trimmed := strings.TrimSpace(value)
	if strings.HasPrefix(trimmed, "https://") || strings.HasPrefix(trimmed, "http://") {
		return trimmed
	}
	return ""
}

func emailPrefix(email string) string {
	local, _, ok := strings.Cut(email, "@")
	if !ok || strings.TrimSpace(local) == "" {
		return email
	}
	return local
}

func isSyntheticEmail(email string) bool {
	return strings.HasSuffix(strings.ToLower(strings.TrimSpace(email)), "@third-party.local")
}

func isUniqueConstraintError(err error) bool {
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "unique") || strings.Contains(message, "duplicate")
}
