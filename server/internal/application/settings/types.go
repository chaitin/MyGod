package settings

import "context"

type Settings struct {
	AppName          string
	OrganizationName string
}

type PublicProvider struct {
	Key  string
	Name string
}

type PublicInfo struct {
	Settings              Settings
	Providers             []PublicProvider
	EmailCodeLoginEnabled bool
	PasswordLoginEnabled  bool
}

type UpdateCommand struct {
	AppName          string
	OrganizationName string
}

type PasswordLoginSettings struct {
	Enabled bool
}

type UpdatePasswordLoginCommand struct {
	Enabled bool
}

const (
	SMTPSecurityNone     = "none"
	SMTPSecuritySTARTTLS = "starttls"
	SMTPSecurityTLS      = "tls"
)

type EmailLoginSettings struct {
	Enabled      bool
	SMTPHost     string
	SMTPPort     int
	SMTPSecurity string
	SMTPUsername string
	SMTPPassword string
	FromEmail    string
	FromName     string
}

type UpdateEmailLoginCommand struct {
	Enabled      bool
	SMTPHost     string
	SMTPPort     int
	SMTPSecurity string
	SMTPUsername string
	SMTPPassword *string
	FromEmail    string
	FromName     string
}

type AdminService interface {
	Get(context.Context) (Settings, error)
	Update(context.Context, UpdateCommand) (Settings, error)
}

type PublicService interface {
	GetPublicInfo(context.Context) (PublicInfo, error)
}

type EmailLoginSettingsService interface {
	GetEmailLogin(context.Context) (EmailLoginSettings, error)
	UpdateEmailLogin(context.Context, UpdateEmailLoginCommand) (EmailLoginSettings, error)
}

type EmailLoginSettingsProvider interface {
	GetEmailLogin(context.Context) (EmailLoginSettings, error)
}

type PasswordLoginSettingsService interface {
	GetPasswordLogin(context.Context) (PasswordLoginSettings, error)
	UpdatePasswordLogin(context.Context, UpdatePasswordLoginCommand) (PasswordLoginSettings, error)
}

type PasswordLoginPolicy interface {
	PasswordLoginEnabled(context.Context) (bool, error)
}
