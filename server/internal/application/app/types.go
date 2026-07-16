package app

import (
	"context"
	"io"
	"time"
)

const (
	ConnectionStatusDisabled = "disabled"
	ConnectionStatusOffline  = "offline"
	ConnectionStatusOnline   = "online"

	VisibilityCreator = "creator"
	VisibilityPublic  = "public"

	MaxAvatarBytes = 1 * 1024 * 1024
)

type App struct {
	Avatar           string
	ConnectionSecret string
	ConnectionStatus string
	CreatedAt        time.Time
	CreatorUserID    *string
	Description      string
	Enabled          bool
	ID               string
	Name             string
	System           bool
	UpdatedAt        time.Time
	Visibility       string
}

type CreateCommand struct {
	Description string
	Name        string
	Visibility  string
}

type UpdateCommand struct {
	AppID       string
	Description string
	Name        string
	Visibility  string
}

type SetEnabledCommand struct {
	AppID   string
	Enabled bool
}

type UploadAvatarCommand struct {
	AppID   string
	Content io.Reader
	Size    int64
}

type AdminService interface {
	List(context.Context) ([]App, error)
	Get(context.Context, string) (App, error)
	Create(context.Context, CreateCommand) (App, error)
	Update(context.Context, UpdateCommand) (App, error)
	SetEnabled(context.Context, SetEnabledCommand) (App, error)
	RegenerateSecret(context.Context, string) (App, error)
	Delete(context.Context, string) error
	UploadAvatar(context.Context, UploadAvatarCommand) (App, error)
}

type ConnectionService interface {
	GetForConnection(context.Context, string) (App, error)
}
