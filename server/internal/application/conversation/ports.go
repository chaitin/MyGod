package conversation

import (
	"context"

	projectapp "app/internal/application/project"
)

type ProjectReader interface {
	ListForConversations(context.Context, []string) (map[string][]projectapp.ConversationProject, error)
}

type NotificationPort interface {
	PublishConversationMessage(context.Context, []string, Message)
	PublishConversationRemoved(context.Context, []string, string)
}
