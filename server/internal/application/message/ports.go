package message

import (
	"context"
	"encoding/json"
	"sync"
)

type BodyProcessor interface {
	Prepare(context.Context, string, json.RawMessage) (json.RawMessage, error)
	Finalize(context.Context, json.RawMessage) (json.RawMessage, string, error)
}

type ForwardBodyMetrics struct {
	BundleDepth int
	LeafCount   int
}

type ForwardBodySanitizer interface {
	SanitizeForwardBody(json.RawMessage, map[string]string, int) (json.RawMessage, string, ForwardBodyMetrics, error)
}

type TemporaryFileValidator interface {
	ValidateTemporaryFiles(context.Context, []string) error
}

type TaskNotificationBodyBuilder interface {
	BuildTaskNotificationBody(context.Context, TaskNotificationCommand) (json.RawMessage, string, error)
}

type TaskReminderBodyBuilder interface {
	BuildTaskReminderBody(context.Context, TaskReminderNotificationCommand) (json.RawMessage, string, error)
}

type Delivery struct {
	Message Message
	UserID  string
}

type NotificationPort interface {
	PublishMessageCreated(context.Context, []Delivery)
	PublishSharedMessageCreated(context.Context, []string, Message)
	PublishMessageUpdated(context.Context, []Delivery)
	PublishMembersMentioned(context.Context, []string, string, int64)
	PublishMembersChoiceReceived(context.Context, []string, string, int64)
	PublishMessageChoiceUpdated(context.Context, []string, ChoiceUpdatedEvent)
}

type ReactionNotificationPort interface {
	PublishMessageReactionsUpdated(context.Context, []string, ReactionEvent)
}

type AppEvent struct {
	AppID   string
	Cursor  int64
	Event   string
	Payload json.RawMessage
}

type AppEventPort interface {
	DeliverAppEvents(context.Context, []AppEvent)
}

type AppEventLocker interface {
	sync.Locker
}
