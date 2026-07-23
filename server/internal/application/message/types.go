package message

import (
	"context"
	"encoding/json"
	"time"
)

const (
	DefaultHistoryLimit       = 20
	MaxHistoryLimit           = 20
	MaxClientMessageID        = 128
	MaxCreateRequestBody      = 64*1024 + 1024
	MaxSetReactionRequestBody = 1024
	MaxReactionSnapshotIDs    = 100
	MaxReactionSnapshotBody   = 8 * 1024
	MaxChoiceResponseBody     = 8 * 1024
	MaxChoiceSnapshotBody     = 8 * 1024
	ForwardModeMerged         = "merged"
	ForwardModeSeparate       = "separate"
	MaxForwardCount           = 50
	MaxForwardTargets         = 20
)

type Identity struct {
	Email    string
	ID       string
	Name     string
	Nickname string
	Type     string
}

type Reply struct {
	ID      string
	Sender  Identity
	Seq     int64
	Summary string
}

type Message struct {
	Body             json.RawMessage
	Choice           *ChoiceState
	ClientMessageID  string
	ConversationID   string
	CreatedAt        time.Time
	DelegatedBy      *Identity
	ID               string
	ReplyTo          *Reply
	ReplyToMessageID string
	ReactionVersion  int64
	Reactions        []ReactionSummary
	RevokedAt        *time.Time
	RevokedByUserID  string
	Sender           Identity
	Seq              int64
	Summary          string
	Topic            *MessageTopic
}

type ChoiceOptionState struct {
	ID            string
	ResponseCount int64
}

type ChoiceState struct {
	MyOptionIDs   []string
	Options       []ChoiceOptionState
	ResponseCount int64
}

type SubmitChoiceResponseCommand struct {
	AccountID      string
	ConversationID string
	MessageID      string
	OptionIDs      []string
}

type SubmitChoiceResponseResult struct {
	Choice         ChoiceState
	ConversationID string
	Created        bool
	MessageID      string
	Response       ChoiceResponse
}

type ChoiceResponse struct {
	CreatedAt time.Time
	ID        string
	OptionIDs []string
	UserID    string
}

type ChoiceUpdatedEvent struct {
	ActorOptionIDs []string
	ActorUserID    string
	Choice         ChoiceState
	ConversationID string
	MessageID      string
}

type ListChoiceSnapshotsCommand struct {
	AccountID      string
	ConversationID string
	MessageIDs     []string
}

type ChoiceSnapshot struct {
	Choice    *ChoiceState
	MessageID string
	Status    string
}

type ListChoiceSnapshotsResult struct {
	ConversationID string
	Snapshots      []ChoiceSnapshot
}

type ReactionSummary struct {
	Count       int64
	ReactedByMe bool
	Text        string
	Users       []ReactionUser
}

type ReactionCount struct {
	Count int64
	Text  string
	Users []ReactionUser
}

type ReactionUser struct {
	ID   string
	Name string
}

type SetReactionCommand struct {
	AccountID      string
	ConversationID string
	MessageID      string
	Reacted        bool
	Text           string
}

type SetReactionResult struct {
	Changed         bool
	ConversationID  string
	MessageID       string
	ReactionVersion int64
	Reactions       []ReactionSummary
}

type ReactionEvent struct {
	ActorReacted    bool
	ActorText       string
	ActorUserID     string
	ConversationID  string
	MessageID       string
	ReactionVersion int64
	Reactions       []ReactionCount
}

type ListReactionSnapshotsCommand struct {
	AccountID      string
	ConversationID string
	MessageIDs     []string
}

type ReactionSnapshot struct {
	MessageID       string
	ReactionVersion int64
	Reactions       []ReactionSummary
}

type ListReactionSnapshotsResult struct {
	ConversationID string
	Snapshots      []ReactionSnapshot
}

type ListReactionUsersCommand struct {
	AccountID      string
	ConversationID string
	MessageID      string
	Text           string
}

type ListReactionUsersResult struct {
	ConversationID string
	MessageID      string
	Text           string
	Users          []ReactionUser
}

type MessageTopic struct {
	Archived       bool
	ConversationID string
	RecentReplies  []MessageTopicReply
}

type MessageTopicReply struct {
	CreatedAt time.Time
	ID        string
	Sender    Identity
	Summary   string
}

type Page struct {
	HasMoreAfter  bool
	HasMoreBefore bool
	Limit         int
	NewestSeq     int64
	OldestSeq     int64
}

type ListCommand struct {
	AccountID      string
	ConversationID string
	AfterSeq       *int64
	BeforeSeq      *int64
	Limit          int
}

type ListResult struct {
	Messages []Message
	Page     Page
}

type CreateCommand struct {
	AccountID        string
	Body             json.RawMessage
	ClientMessageID  string
	ConversationID   string
	ReplyToMessageID string
}

type CreateResult struct {
	Created bool
	Message Message
}

type PrepareUploadCommand struct {
	AccountID        string
	ClientMessageID  string
	ConversationID   string
	ReplyToMessageID string
}

type PrepareUploadResult struct {
	Existing *Message
}

type CreatePreparedCommand struct {
	AccountID        string
	Body             json.RawMessage
	ClientMessageID  string
	ConversationID   string
	ReplyToMessageID string
	Summary          string
}

type RevokeCommand struct {
	AccountID      string
	ConversationID string
	MessageID      string
}

type RevokeResult struct {
	Message       Message
	SystemMessage Message
}

type ForwardCommand struct {
	AccountID             string
	ClientForwardID       string
	MessageIDs            []string
	Mode                  string
	SourceConversationID  string
	TargetConversationIDs []string
}

type ForwardResult struct {
	FailedCount int
	Results     []ForwardTargetResult
	SentCount   int
}

type ForwardTargetResult struct {
	ConversationID string
	Error          *ForwardTargetError
	Messages       []Message
	Status         string
}

type ForwardTargetError struct {
	Code    string
	Message string
}

type FinalizeBody func(context.Context, json.RawMessage) (json.RawMessage, string, error)

type CreateAsAppCommand struct {
	AppID           string
	Body            json.RawMessage
	ClientMessageID string
	ConversationID  string
	Finalize        FinalizeBody
}

type CreateDelegatedCommand struct {
	AccountID       string
	Body            json.RawMessage
	ClientMessageID string
	ConversationID  string
	DelegatedBy     Identity
	Finalize        FinalizeBody
}

type AppConversationAccessCommand struct {
	AppID          string
	ConversationID string
}

type AppConversationAccess struct {
	HistoryVisibleFromSeq int64
}

type RunAsTriggerCommand struct {
	ActorID                     string
	ActorType                   string
	AppID                       string
	AuthorizationConversationID string
	TriggerMessageID            string
}

type ListForAppCommand struct {
	BeforeOrEqualSeq      int64
	ConversationID        string
	HistoryVisibleFromSeq int64
	Limit                 int
}

type AppHistoryMessage struct {
	Body      json.RawMessage
	CreatedAt time.Time
	ID        string
	Sender    Identity
	Seq       int64
	Summary   string
}

type ListForAppResult struct {
	Messages []AppHistoryMessage
}

type ReadForUserCommand struct {
	AccountID      string
	AppID          string
	BeforeSeq      int64
	ConversationID string
	Limit          int
	UserID         string
}

type AppConversationSummary struct {
	ConversationID string
	LastActiveAt   time.Time
	MemberCount    int
	Name           string
	Type           string
}

type ReadForUserResult struct {
	Conversation AppConversationSummary
	Messages     []AppHistoryMessage
}

type TaskNotificationCommand struct {
	AssigneeUserID  *string
	AssigneeName    string
	CreatedByUserID string
	DueDate         *time.Time
	ID              string
	ProjectID       string
	Status          string
	Title           string
	UpdatedAt       time.Time
}

type TaskNotificationBatchResult struct {
	Notifications []TaskNotificationResult
}

type TaskNotificationResult struct {
	Created         bool
	Message         Message
	RecipientUserID string
}

type TaskReminderNotificationCommand struct {
	AssigneeUserID *string
	Description    string
	ID             string
	ProjectID      string
	Title          string
	OccurrenceAt   time.Time
	Timezone       string
}

type ClientService interface {
	List(context.Context, ListCommand) (ListResult, error)
	Create(context.Context, CreateCommand) (CreateResult, error)
	PrepareUpload(context.Context, PrepareUploadCommand) (PrepareUploadResult, error)
	CreatePrepared(context.Context, CreatePreparedCommand) (CreateResult, error)
	Revoke(context.Context, RevokeCommand) (RevokeResult, error)
	Forward(context.Context, ForwardCommand) (ForwardResult, error)
	SetReaction(context.Context, SetReactionCommand) (SetReactionResult, error)
	ListReactionSnapshots(context.Context, ListReactionSnapshotsCommand) (ListReactionSnapshotsResult, error)
	ListReactionUsers(context.Context, ListReactionUsersCommand) (ListReactionUsersResult, error)
	SubmitChoiceResponse(context.Context, SubmitChoiceResponseCommand) (SubmitChoiceResponseResult, error)
	ListChoiceSnapshots(context.Context, ListChoiceSnapshotsCommand) (ListChoiceSnapshotsResult, error)
}

type AppService interface {
	CreateAsApp(context.Context, CreateAsAppCommand) (CreateResult, error)
	CreateDelegated(context.Context, CreateDelegatedCommand) (CreateResult, error)
	AuthorizeAppConversation(context.Context, AppConversationAccessCommand) (AppConversationAccess, error)
	AuthorizeAppConversationSend(context.Context, AppConversationAccessCommand) error
	AuthorizeRunAsTrigger(context.Context, RunAsTriggerCommand) error
	ListForApp(context.Context, ListForAppCommand) (ListForAppResult, error)
	ReadForUser(context.Context, ReadForUserCommand) (ReadForUserResult, error)
}
