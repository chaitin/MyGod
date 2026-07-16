package contact

import (
	"context"
	"time"
)

const (
	IdentityTypeUser = "user"
	IdentityTypeApp  = "app"

	ContactTypeUser  = "user"
	ContactTypeApp   = "app"
	ContactTypeGroup = "group"
)

type Identity struct {
	ID   string
	Type string
}

type User struct {
	Avatar       string
	Email        string
	ID           string
	LastOnlineAt *time.Time
	Name         string
	Nickname     string
	Online       bool
	Phone        string
	Type         string
}

type App struct {
	Avatar      string
	Description string
	ID          string
	Name        string
	Online      bool
	Type        string
}

type GroupAvatarMember struct {
	Avatar   string
	Name     string
	Nickname string
	Role     string
}

type Group struct {
	Avatar        string
	AvatarMembers []GroupAvatarMember
	ID            string
	Joined        bool
	MemberCount   int
	Name          string
	Type          string
	Visibility    string
}

type ListCommand struct {
	AccountID string
	Keyword   string
}

type ListResult struct {
	Apps   []App
	Groups []Group
	Users  []User
}

type ListUsersCommand struct {
	Keyword string
}

type ListUsersResult struct {
	Users []User
}

type ListForIdentityCommand struct {
	Identity Identity
	Keyword  string
}

type ListAppsResult struct {
	Apps []App
}

type ListGroupsResult struct {
	Groups []Group
}

type ClientService interface {
	List(context.Context, ListCommand) (ListResult, error)
	ListUsers(context.Context, ListUsersCommand) (ListUsersResult, error)
}

type AppService interface {
	ListUsers(context.Context, ListUsersCommand) (ListUsersResult, error)
	ListAppsForIdentity(context.Context, ListForIdentityCommand) (ListAppsResult, error)
	ListGroupsForIdentity(context.Context, ListForIdentityCommand) (ListGroupsResult, error)
}
