package settings

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"app/internal/store"

	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func TestServiceCreatesDefaultsAndUpdatesSettings(t *testing.T) {
	db := openSettingsTestDB(t)
	now := time.Date(2026, 7, 15, 8, 0, 0, 0, time.UTC)
	service := NewService(Dependencies{DB: db, Now: func() time.Time { return now }})

	value, err := service.Get(context.Background())
	if err != nil {
		t.Fatalf("get settings: %v", err)
	}
	if value.AppName != store.DefaultAppName || value.OrganizationName != store.DefaultOrganizationName {
		t.Fatalf("default settings = %#v", value)
	}

	updated, err := service.Update(context.Background(), UpdateCommand{
		AppName: "  星环协作  ", OrganizationName: " 长亭科技企业安全 ",
	})
	if err != nil {
		t.Fatalf("update settings: %v", err)
	}
	if updated.AppName != "星环协作" || updated.OrganizationName != "长亭科技企业安全" {
		t.Fatalf("updated settings = %#v", updated)
	}

	var stored store.AppSettings
	if err := db.First(&stored, "id = ?", store.AppSettingsID).Error; err != nil {
		t.Fatalf("load stored settings: %v", err)
	}
	if stored.AppName != updated.AppName || stored.OrganizationName != updated.OrganizationName || !stored.UpdatedAt.Equal(now) {
		t.Fatalf("stored settings = %#v", stored)
	}

	passwordSettings, err := service.GetPasswordLogin(context.Background())
	if err != nil || !passwordSettings.Enabled {
		t.Fatalf("default password login settings = %#v, error = %v", passwordSettings, err)
	}
	passwordSettings, err = service.UpdatePasswordLogin(context.Background(), UpdatePasswordLoginCommand{Enabled: false})
	if err != nil || passwordSettings.Enabled {
		t.Fatalf("updated password login settings = %#v, error = %v", passwordSettings, err)
	}
	if enabled, err := service.PasswordLoginEnabled(context.Background()); err != nil || enabled {
		t.Fatalf("password login enabled = %t, error = %v", enabled, err)
	}

	if _, err := service.Update(context.Background(), UpdateCommand{OrganizationName: "长亭科技"}); ErrorCodeOf(err) != CodeInvalidRequest {
		t.Fatalf("empty app name error = %v, code = %q", err, ErrorCodeOf(err))
	}
}

func TestServiceReturnsEnabledPublicProvidersInDisplayOrder(t *testing.T) {
	db := openSettingsTestDB(t)
	service := NewService(Dependencies{DB: db})
	providers := []store.ThirdPartyLoginProvider{
		newSettingsTestProvider("disabled", "Disabled", false, 0),
		newSettingsTestProvider("zeta", "Zeta", true, 2),
		newSettingsTestProvider("beta", "Beta", true, 1),
		newSettingsTestProvider("alpha", "Alpha", true, 1),
	}
	if err := db.Create(&providers).Error; err != nil {
		t.Fatalf("create providers: %v", err)
	}

	info, err := service.GetPublicInfo(context.Background())
	if err != nil {
		t.Fatalf("get public info: %v", err)
	}
	if len(info.Providers) != 3 || info.Providers[0].Key != "alpha" || info.Providers[1].Key != "beta" || info.Providers[2].Key != "zeta" {
		t.Fatalf("providers = %#v", info.Providers)
	}
}

func TestServiceUpdatesEmailLoginSettingsAndPreservesPassword(t *testing.T) {
	db := openSettingsTestDB(t)
	service := NewService(Dependencies{DB: db})
	password := "smtp-secret"
	updated, err := service.UpdateEmailLogin(context.Background(), UpdateEmailLoginCommand{
		Enabled: true, SMTPHost: " smtp.example.com ", SMTPPort: 587,
		SMTPSecurity: " STARTTLS ", SMTPUsername: " mailer@example.com ",
		SMTPPassword: &password, FromEmail: " MAILER@Example.com ", FromName: " 即应通知 ",
	})
	if err != nil {
		t.Fatalf("update email login: %v", err)
	}
	if !updated.Enabled || updated.SMTPHost != "smtp.example.com" || updated.SMTPSecurity != SMTPSecuritySTARTTLS ||
		updated.SMTPPassword != password || updated.FromEmail != "mailer@example.com" || updated.FromName != "即应通知" {
		t.Fatalf("updated email login = %#v", updated)
	}

	preserved, err := service.UpdateEmailLogin(context.Background(), UpdateEmailLoginCommand{
		Enabled: true, SMTPHost: "smtp.example.com", SMTPPort: 465,
		SMTPSecurity: SMTPSecurityTLS, SMTPUsername: "mailer@example.com",
		FromEmail: "mailer@example.com", FromName: "即应",
	})
	if err != nil {
		t.Fatalf("preserve email login password: %v", err)
	}
	if preserved.SMTPPassword != password || preserved.SMTPPort != 465 || preserved.SMTPSecurity != SMTPSecurityTLS {
		t.Fatalf("preserved email login = %#v", preserved)
	}
	loaded, err := service.GetEmailLogin(context.Background())
	if err != nil || loaded.SMTPPassword != password {
		t.Fatalf("loaded email login = %#v, error = %v", loaded, err)
	}
	public, err := service.GetPublicInfo(context.Background())
	if err != nil || !public.EmailCodeLoginEnabled {
		t.Fatalf("public email login enabled = %t, error = %v", public.EmailCodeLoginEnabled, err)
	}
	emptyPassword := ""
	disabled, err := service.UpdateEmailLogin(context.Background(), UpdateEmailLoginCommand{
		Enabled: false, SMTPPort: 25, SMTPSecurity: SMTPSecurityNone,
		SMTPPassword: &emptyPassword,
	})
	if err != nil || disabled.SMTPPassword != "" || disabled.SMTPUsername != "" {
		t.Fatalf("disabled email login = %#v, error = %v", disabled, err)
	}
}

func TestServiceDefaultsEmailLoginToTLS(t *testing.T) {
	service := NewService(Dependencies{DB: openSettingsTestDB(t)})

	settings, err := service.GetEmailLogin(context.Background())
	if err != nil {
		t.Fatalf("get email login settings: %v", err)
	}
	if settings.SMTPPort != 465 || settings.SMTPSecurity != SMTPSecurityTLS {
		t.Fatalf("default email login settings = %#v", settings)
	}

	updated, err := service.UpdateEmailLogin(context.Background(), UpdateEmailLoginCommand{})
	if err != nil {
		t.Fatalf("update email login settings with defaults: %v", err)
	}
	if updated.SMTPPort != 465 || updated.SMTPSecurity != SMTPSecurityTLS {
		t.Fatalf("updated email login settings = %#v", updated)
	}
}

func TestServiceValidatesEnabledEmailLoginSettings(t *testing.T) {
	service := NewService(Dependencies{DB: openSettingsTestDB(t)})
	password := "smtp-secret"
	tests := []UpdateEmailLoginCommand{
		{Enabled: true, SMTPPort: 587, SMTPSecurity: SMTPSecuritySTARTTLS, FromEmail: "mailer@example.com"},
		{Enabled: true, SMTPHost: "smtp.example.com", SMTPPort: 587, SMTPSecurity: SMTPSecuritySTARTTLS, FromEmail: "mailer@example.com"},
		{Enabled: true, SMTPHost: "smtp.example.com", SMTPPort: 70000, SMTPSecurity: SMTPSecuritySTARTTLS, FromEmail: "mailer@example.com"},
		{Enabled: true, SMTPHost: "smtp.example.com", SMTPPort: 587, SMTPSecurity: "invalid", FromEmail: "mailer@example.com"},
		{Enabled: true, SMTPHost: "smtp.example.com", SMTPPort: 587, SMTPSecurity: SMTPSecuritySTARTTLS, FromEmail: "invalid"},
		{Enabled: true, SMTPHost: "smtp.example.com", SMTPPort: 587, SMTPSecurity: SMTPSecuritySTARTTLS, SMTPUsername: "mailer", FromEmail: "mailer@example.com"},
		{Enabled: true, SMTPHost: "smtp.example.com", SMTPPort: 25, SMTPSecurity: SMTPSecurityNone, SMTPUsername: "mailer", SMTPPassword: &password, FromEmail: "mailer@example.com"},
		{Enabled: false, SMTPPort: 25, SMTPSecurity: SMTPSecurityNone, SMTPUsername: "mailer", SMTPPassword: &password},
		{Enabled: false, SMTPPort: 587, SMTPSecurity: SMTPSecuritySTARTTLS, SMTPPassword: &password},
	}
	for index, cmd := range tests {
		if _, err := service.UpdateEmailLogin(context.Background(), cmd); ErrorCodeOf(err) != CodeInvalidRequest {
			t.Fatalf("case %d error = %v, code = %q", index, err, ErrorCodeOf(err))
		}
	}
}

func openSettingsTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if err := db.AutoMigrate(&store.AppSettings{}, &store.ThirdPartyLoginProvider{}); err != nil {
		t.Fatalf("migrate database: %v", err)
	}
	return db
}

func newSettingsTestProvider(key, name string, enabled bool, sortOrder int) store.ThirdPartyLoginProvider {
	now := time.Date(2026, 7, 15, 8, 0, 0, 0, time.UTC)
	return store.ThirdPartyLoginProvider{
		ID:           uuid.NewString(),
		Name:         name,
		Key:          key,
		Type:         "oidc",
		Enabled:      enabled,
		ClientID:     "client-id",
		ClientSecret: "client-secret",
		Scopes:       json.RawMessage(`[]`),
		Config:       json.RawMessage(`{}`),
		SortOrder:    sortOrder,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
}
