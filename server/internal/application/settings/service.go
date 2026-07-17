package settings

import (
	"context"
	"errors"
	"net/mail"
	"strings"
	"time"

	"app/internal/store"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Dependencies struct {
	DB  *gorm.DB
	Now func() time.Time
}

type Service struct {
	db  *gorm.DB
	now func() time.Time
}

func NewService(deps Dependencies) *Service {
	now := deps.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &Service{db: deps.DB, now: now}
}

func (s *Service) Get(ctx context.Context) (Settings, error) {
	value, err := s.getOrCreate(ctx)
	if err != nil {
		return Settings{}, internalError(err)
	}
	return newSettings(value), nil
}

func (s *Service) Update(ctx context.Context, cmd UpdateCommand) (Settings, error) {
	appName := strings.TrimSpace(cmd.AppName)
	if appName == "" {
		return Settings{}, newError(CodeInvalidRequest, "App 名称不能为空", nil)
	}
	organizationName := strings.TrimSpace(cmd.OrganizationName)
	if organizationName == "" {
		return Settings{}, newError(CodeInvalidRequest, "组织名称不能为空", nil)
	}

	if _, err := s.getOrCreate(ctx); err != nil {
		return Settings{}, internalError(err)
	}
	if err := s.db.WithContext(ctx).Model(&store.AppSettings{}).
		Where("id = ?", store.AppSettingsID).
		Updates(map[string]any{
			"app_name":          appName,
			"organization_name": organizationName,
			"updated_at":        s.now().UTC(),
		}).Error; err != nil {
		return Settings{}, internalError(err)
	}
	return Settings{AppName: appName, OrganizationName: organizationName}, nil
}

func (s *Service) GetPublicInfo(ctx context.Context) (PublicInfo, error) {
	stored, err := s.getOrCreate(ctx)
	if err != nil {
		return PublicInfo{}, internalError(err)
	}
	value := newSettings(stored)
	var providers []store.ThirdPartyLoginProvider
	if err := s.db.WithContext(ctx).
		Where("enabled = ?", true).
		Order("sort_order ASC").
		Order("name ASC").
		Find(&providers).Error; err != nil {
		return PublicInfo{}, internalError(err)
	}
	result := make([]PublicProvider, 0, len(providers))
	for _, provider := range providers {
		result = append(result, PublicProvider{Key: provider.Key, Name: provider.Name})
	}
	return PublicInfo{
		Settings:              value,
		Providers:             result,
		EmailCodeLoginEnabled: stored.EmailCodeLoginEnabled,
		PasswordLoginEnabled:  stored.PasswordLoginEnabled,
	}, nil
}

func (s *Service) GetEmailLogin(ctx context.Context) (EmailLoginSettings, error) {
	value, err := s.getOrCreate(ctx)
	if err != nil {
		return EmailLoginSettings{}, internalError(err)
	}
	return newEmailLoginSettings(value), nil
}

func (s *Service) PasswordLoginEnabled(ctx context.Context) (bool, error) {
	value, err := s.GetPasswordLogin(ctx)
	if err != nil {
		return false, err
	}
	return value.Enabled, nil
}

func (s *Service) GetPasswordLogin(ctx context.Context) (PasswordLoginSettings, error) {
	value, err := s.getOrCreate(ctx)
	if err != nil {
		return PasswordLoginSettings{}, internalError(err)
	}
	return PasswordLoginSettings{Enabled: value.PasswordLoginEnabled}, nil
}

func (s *Service) UpdatePasswordLogin(ctx context.Context, cmd UpdatePasswordLoginCommand) (PasswordLoginSettings, error) {
	if _, err := s.getOrCreate(ctx); err != nil {
		return PasswordLoginSettings{}, internalError(err)
	}
	if err := s.db.WithContext(ctx).Model(&store.AppSettings{}).
		Where("id = ?", store.AppSettingsID).
		Updates(map[string]any{
			"password_login_enabled": cmd.Enabled,
			"updated_at":             s.now().UTC(),
		}).Error; err != nil {
		return PasswordLoginSettings{}, internalError(err)
	}
	return PasswordLoginSettings{Enabled: cmd.Enabled}, nil
}

func (s *Service) UpdateEmailLogin(ctx context.Context, cmd UpdateEmailLoginCommand) (EmailLoginSettings, error) {
	stored, err := s.getOrCreate(ctx)
	if err != nil {
		return EmailLoginSettings{}, internalError(err)
	}

	value := EmailLoginSettings{
		Enabled:      cmd.Enabled,
		SMTPHost:     strings.TrimSpace(cmd.SMTPHost),
		SMTPPort:     cmd.SMTPPort,
		SMTPSecurity: strings.ToLower(strings.TrimSpace(cmd.SMTPSecurity)),
		SMTPUsername: strings.TrimSpace(cmd.SMTPUsername),
		SMTPPassword: stored.SMTPPassword,
		FromEmail:    strings.ToLower(strings.TrimSpace(cmd.FromEmail)),
		FromName:     strings.TrimSpace(cmd.FromName),
	}
	if cmd.SMTPPassword != nil {
		value.SMTPPassword = *cmd.SMTPPassword
	}
	if value.SMTPPort == 0 {
		value.SMTPPort = 465
	}
	if value.SMTPSecurity == "" {
		value.SMTPSecurity = SMTPSecurityTLS
	}
	if err := validateEmailLoginSettings(value); err != nil {
		return EmailLoginSettings{}, err
	}

	if err := s.db.WithContext(ctx).Model(&store.AppSettings{}).
		Where("id = ?", store.AppSettingsID).
		Updates(map[string]any{
			"email_code_login_enabled": value.Enabled,
			"smtp_host":                value.SMTPHost,
			"smtp_port":                value.SMTPPort,
			"smtp_security":            value.SMTPSecurity,
			"smtp_username":            value.SMTPUsername,
			"smtp_password":            value.SMTPPassword,
			"smtp_from_email":          value.FromEmail,
			"smtp_from_name":           value.FromName,
			"updated_at":               s.now().UTC(),
		}).Error; err != nil {
		return EmailLoginSettings{}, internalError(err)
	}
	return value, nil
}

func (s *Service) getOrCreate(ctx context.Context) (store.AppSettings, error) {
	var value store.AppSettings
	err := s.db.WithContext(ctx).First(&value, "id = ?", store.AppSettingsID).Error
	if err == nil {
		return value, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return store.AppSettings{}, err
	}

	now := s.now().UTC()
	defaults := store.AppSettings{
		ID:                   store.AppSettingsID,
		AppName:              store.DefaultAppName,
		OrganizationName:     store.DefaultOrganizationName,
		PasswordLoginEnabled: true,
		SMTPPort:             465,
		SMTPSecurity:         SMTPSecurityTLS,
		CreatedAt:            now,
		UpdatedAt:            now,
	}
	if err := s.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&defaults).Error; err != nil {
		return store.AppSettings{}, err
	}
	if err := s.db.WithContext(ctx).First(&value, "id = ?", store.AppSettingsID).Error; err != nil {
		return store.AppSettings{}, err
	}
	return value, nil
}

func newSettings(value store.AppSettings) Settings {
	return Settings{AppName: value.AppName, OrganizationName: value.OrganizationName}
}

func newEmailLoginSettings(value store.AppSettings) EmailLoginSettings {
	return EmailLoginSettings{
		Enabled:      value.EmailCodeLoginEnabled,
		SMTPHost:     value.SMTPHost,
		SMTPPort:     value.SMTPPort,
		SMTPSecurity: value.SMTPSecurity,
		SMTPUsername: value.SMTPUsername,
		SMTPPassword: value.SMTPPassword,
		FromEmail:    value.SMTPFromEmail,
		FromName:     value.SMTPFromName,
	}
}

func validateEmailLoginSettings(value EmailLoginSettings) error {
	if value.SMTPPort < 1 || value.SMTPPort > 65535 {
		return newError(CodeInvalidRequest, "SMTP 端口必须在 1 到 65535 之间", nil)
	}
	switch value.SMTPSecurity {
	case SMTPSecurityNone, SMTPSecuritySTARTTLS, SMTPSecurityTLS:
	default:
		return newError(CodeInvalidRequest, "SMTP 安全类型不支持", nil)
	}
	if (value.SMTPUsername == "") != (value.SMTPPassword == "") {
		return newError(CodeInvalidRequest, "SMTP 用户名和密码必须同时配置", nil)
	}
	if value.SMTPSecurity == SMTPSecurityNone && value.SMTPUsername != "" {
		return newError(CodeInvalidRequest, "无加密 SMTP 连接不能使用用户名密码认证", nil)
	}
	if !value.Enabled {
		return nil
	}
	if value.SMTPHost == "" {
		return newError(CodeInvalidRequest, "SMTP 主机不能为空", nil)
	}
	if strings.Contains(value.SMTPHost, "://") || strings.ContainsAny(value.SMTPHost, "/?#\t\r\n ") {
		return newError(CodeInvalidRequest, "SMTP 主机格式错误", nil)
	}
	if value.SMTPUsername == "" {
		return newError(CodeInvalidRequest, "SMTP 用户名不能为空", nil)
	}
	if value.SMTPPassword == "" {
		return newError(CodeInvalidRequest, "SMTP 密码不能为空", nil)
	}
	address, err := mail.ParseAddress(value.FromEmail)
	if err != nil || address.Address != value.FromEmail {
		return newError(CodeInvalidRequest, "发件人邮箱格式错误", nil)
	}
	return nil
}

var _ AdminService = (*Service)(nil)
var _ PublicService = (*Service)(nil)
var _ EmailLoginSettingsService = (*Service)(nil)
var _ PasswordLoginSettingsService = (*Service)(nil)
var _ PasswordLoginPolicy = (*Service)(nil)
