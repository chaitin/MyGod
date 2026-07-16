package identityprovider

import "context"

const (
	TypeDingTalk = "dingtalk"
	TypeWeCom    = "wecom"
	TypeFeishu   = "feishu"
	TypeGitHub   = "github"
	TypeGoogle   = "google"
	TypeOIDC     = "oidc"
)

type Provider struct {
	ID           string
	Name         string
	Key          string
	Type         string
	Enabled      bool
	ClientID     string
	ClientSecret string
	Scopes       []string
	Config       map[string]any
	SortOrder    int
}

type WriteCommand struct {
	Name         string
	Type         string
	ClientID     string
	ClientSecret string
	Scopes       []string
	Config       map[string]any
}

type UpdateCommand struct {
	ProviderID string
	WriteCommand
}

type SetEnabledCommand struct {
	ProviderID string
	Enabled    bool
}

type MoveCommand struct {
	ProviderID string
	Direction  string
}

type AdminService interface {
	List(context.Context) ([]Provider, error)
	Get(context.Context, string) (Provider, error)
	Create(context.Context, WriteCommand) (Provider, error)
	Update(context.Context, UpdateCommand) (Provider, error)
	SetEnabled(context.Context, SetEnabledCommand) (Provider, error)
	Move(context.Context, MoveCommand) ([]Provider, error)
	Delete(context.Context, string) error
}

type LoginProviderService interface {
	GetEnabledByKey(context.Context, string) (Provider, error)
}
