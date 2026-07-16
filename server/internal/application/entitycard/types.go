package entitycard

import (
	"context"

	projectapp "app/internal/application/project"
)

const (
	TypeUser    = "user"
	TypeApp     = "app"
	TypeGroup   = "group"
	TypeProject = "project"
	TypeTask    = "task"
)

type Card struct {
	Description string
	Title       string
	URL         string
}

type ResolveCommand struct {
	AccountID  string
	EntityID   string
	EntityType string
}

type ProjectReader interface {
	Get(context.Context, projectapp.ProjectCommand) (projectapp.Project, error)
}

type Resolver interface {
	Resolve(context.Context, ResolveCommand) (Card, error)
}

type Detail struct {
	Label string
	Value string
}
