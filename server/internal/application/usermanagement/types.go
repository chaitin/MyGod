package usermanagement

import (
	"context"
	"time"
)

const (
	StatusActive   = "active"
	StatusDisabled = "disabled"
)

type User struct {
	ID           string
	Avatar       string
	Email        string
	LastOnlineAt *time.Time
	Name         string
	Nickname     string
	Online       bool
	Phone        string
	Status       string
	CreatedAt    time.Time
}

type ListCommand struct {
	Keyword  string
	Online   string
	Page     string
	PageSize string
	Sort     string
	Order    string
}

type ListResult struct {
	Users    []User
	Total    int64
	Page     int
	PageSize int
	Sort     string
	Order    string
}

type CreateCommand struct {
	Email string
	Name  string
	Phone string
}

type CreateResult struct {
	User            User
	InitialPassword string
}

type SetStatusCommand struct {
	UserID string
	Status string
}

type ResetPasswordResult struct {
	User        User
	NewPassword string
}

type AdminService interface {
	List(context.Context, ListCommand) (ListResult, error)
	Create(context.Context, CreateCommand) (CreateResult, error)
	SetStatus(context.Context, SetStatusCommand) (User, error)
	ResetPassword(context.Context, string) (ResetPasswordResult, error)
}
