package store

import (
	"encoding/json"
	"time"

	"gorm.io/gorm"
)

const (
	UserStatusActive   = "active"
	UserStatusDisabled = "disabled"

	ConversationKindDirect = "direct"
	ConversationKindGroup  = "group"
	ConversationKindApp    = "app"

	ConversationStatusActive    = "active"
	ConversationStatusDissolved = "dissolved"

	ConversationPostingPolicyOpen  = "open"
	ConversationPostingPolicyMuted = "muted"

	ConversationVisibilityPrivate = "private"
	ConversationVisibilityPublic  = "public"

	ConversationMemberTypeUser = "user"
	ConversationMemberTypeApp  = "app"

	MessageSenderTypeUser   = "user"
	MessageSenderTypeApp    = "app"
	MessageSenderTypeSystem = "system"

	ThirdPartyLoginProviderTypeDingTalk = "dingtalk"
	ThirdPartyLoginProviderTypeWeCom    = "wecom"
	ThirdPartyLoginProviderTypeFeishu   = "feishu"
	ThirdPartyLoginProviderTypeGitHub   = "github"
	ThirdPartyLoginProviderTypeGoogle   = "google"
	ThirdPartyLoginProviderTypeOIDC     = "oidc"

	ConversationMemberRoleOwner  = "owner"
	ConversationMemberRoleAdmin  = "admin"
	ConversationMemberRoleMember = "member"

	AppVisibilityCreator = "creator"
	AppVisibilityPublic  = "public"

	AppSettingsID           = 1
	DefaultAppName          = "MyGod"
	DefaultOrganizationName = "长亭科技"
	DefaultUserAvatar       = "/assets/avatars/builtin/01.webp"
)

type User struct {
	ID           string  `gorm:"type:uuid;primaryKey"`
	Email        string  `gorm:"size:320;not null;uniqueIndex"`
	Name         string  `gorm:"size:120;not null"`
	Nickname     string  `gorm:"size:120;not null;default:''"`
	Phone        *string `gorm:"size:32;uniqueIndex"`
	Avatar       string  `gorm:"size:512;not null;default:/assets/avatars/builtin/01.webp"`
	PasswordHash string  `gorm:"not null"`
	Status       string  `gorm:"size:32;not null;index"`
	LastOnlineAt *time.Time
	CreatedAt    time.Time `gorm:"not null"`
	UpdatedAt    time.Time `gorm:"not null"`
}

type AdminSession struct {
	ID         string    `gorm:"type:uuid;primaryKey"`
	TokenHash  string    `gorm:"size:64;not null;uniqueIndex"`
	ExpiresAt  time.Time `gorm:"not null;index"`
	CreatedAt  time.Time `gorm:"not null"`
	LastSeenAt time.Time `gorm:"not null"`
	UserAgent  string    `gorm:"size:512"`
	IP         string    `gorm:"size:64"`
}

type UserSession struct {
	ID         string    `gorm:"type:uuid;primaryKey"`
	TokenHash  string    `gorm:"size:64;not null;uniqueIndex"`
	UserID     string    `gorm:"type:uuid;not null;index"`
	User       User      `gorm:"constraint:OnDelete:CASCADE;"`
	ExpiresAt  time.Time `gorm:"not null;index"`
	CreatedAt  time.Time `gorm:"not null"`
	LastSeenAt time.Time `gorm:"not null"`
	UserAgent  string    `gorm:"size:512"`
	IP         string    `gorm:"size:64"`
}

type Conversation struct {
	ID                 string    `gorm:"type:uuid;primaryKey"`
	Kind               string    `gorm:"size:32;not null;index"`
	Name               string    `gorm:"size:160;not null"`
	Avatar             string    `gorm:"size:512;not null;default:''"`
	CreatedByUserID    string    `gorm:"type:uuid;not null;index"`
	CreatedByUser      User      `gorm:"foreignKey:CreatedByUserID;constraint:OnDelete:RESTRICT;"`
	Status             string    `gorm:"size:32;not null;index"`
	PostingPolicy      string    `gorm:"size:32;not null"`
	Visibility         string    `gorm:"size:32;not null;default:private;index"`
	CreatedAt          time.Time `gorm:"not null"`
	UpdatedAt          time.Time `gorm:"not null"`
	DissolvedAt        *time.Time
	LastMessageID      *string    `gorm:"type:uuid"`
	LastMessageSeq     int64      `gorm:"not null;default:0"`
	LastMessageSummary string     `gorm:"not null;default:''"`
	LastMessageAt      *time.Time `gorm:"index"`
	Members            []ConversationMember
}

type ConversationMember struct {
	ConversationID        string       `gorm:"type:uuid;primaryKey"`
	Conversation          Conversation `gorm:"constraint:OnDelete:CASCADE;"`
	MemberType            string       `gorm:"size:32;primaryKey"`
	MemberID              string       `gorm:"type:uuid;primaryKey"`
	Role                  string       `gorm:"size:32;not null;index"`
	JoinedAt              time.Time    `gorm:"not null"`
	HistoryVisibleFromSeq int64        `gorm:"not null;default:1"`
	LeftAt                *time.Time   `gorm:"index"`
	LastReadMessageID     *string      `gorm:"type:uuid"`
	LastReadSeq           int64        `gorm:"not null;default:0"`
}

type Message struct {
	ID               string          `gorm:"type:uuid;primaryKey"`
	ConversationID   string          `gorm:"type:uuid;not null;uniqueIndex:messages_conversation_seq_unique,priority:1;uniqueIndex:messages_client_message_unique,priority:1;index:messages_conversation_seq_index,priority:1"`
	Conversation     Conversation    `gorm:"constraint:OnDelete:CASCADE;"`
	Seq              int64           `gorm:"not null;uniqueIndex:messages_conversation_seq_unique,priority:2;index:messages_conversation_seq_index,priority:2,sort:desc"`
	SenderType       string          `gorm:"size:32;not null;uniqueIndex:messages_client_message_unique,priority:2"`
	SenderID         *string         `gorm:"type:uuid;uniqueIndex:messages_client_message_unique,priority:3"`
	ClientMessageID  *string         `gorm:"size:128;uniqueIndex:messages_client_message_unique,priority:4"`
	DelegatedByType  *string         `gorm:"size:32"`
	DelegatedByID    *string         `gorm:"type:uuid"`
	DelegatedByName  string          `gorm:"not null;default:''"`
	ReplyToMessageID *string         `gorm:"type:uuid;index"`
	Body             json.RawMessage `gorm:"type:jsonb;not null;serializer:json"`
	Summary          string          `gorm:"not null;default:''"`
	RevokedAt        *time.Time      `gorm:"index"`
	RevokedByUserID  *string         `gorm:"type:uuid"`
	CreatedAt        time.Time       `gorm:"not null"`
	UpdatedAt        time.Time       `gorm:"not null"`
	DeletedAt        *time.Time      `gorm:"index"`
}

type DirectConversation struct {
	ConversationID string       `gorm:"type:uuid;primaryKey"`
	Conversation   Conversation `gorm:"constraint:OnDelete:CASCADE;"`
	UserLowID      string       `gorm:"type:uuid;not null;uniqueIndex:direct_conversations_user_pair_unique,priority:1"`
	UserLow        User         `gorm:"foreignKey:UserLowID;constraint:OnDelete:RESTRICT;"`
	UserHighID     string       `gorm:"type:uuid;not null;uniqueIndex:direct_conversations_user_pair_unique,priority:2;check:direct_conversations_user_order_check,user_low_id < user_high_id"`
	UserHigh       User         `gorm:"foreignKey:UserHighID;constraint:OnDelete:RESTRICT;"`
	CreatedAt      time.Time    `gorm:"not null"`
}

type TemporaryFile struct {
	ID        string    `gorm:"type:uuid;primaryKey"`
	ObjectKey string    `gorm:"not null;uniqueIndex"`
	SizeBytes int64     `gorm:"not null;check:size_bytes >= 0"`
	CreatedAt time.Time `gorm:"not null"`
}

func (TemporaryFile) TableName() string {
	return "temporary_files"
}

type App struct {
	ID               string         `gorm:"type:uuid;primaryKey"`
	Name             string         `gorm:"size:120;not null"`
	Avatar           string         `gorm:"size:512;not null;default:''"`
	Description      string         `gorm:"not null;default:''"`
	CreatorUserID    *string        `gorm:"type:uuid;index"`
	CreatorUser      *User          `gorm:"foreignKey:CreatorUserID;constraint:OnDelete:SET NULL;"`
	Enabled          bool           `gorm:"not null;default:true;index"`
	Visibility       string         `gorm:"size:32;not null;index"`
	ConnectionSecret string         `gorm:"not null;uniqueIndex"`
	CreatedAt        time.Time      `gorm:"not null"`
	UpdatedAt        time.Time      `gorm:"not null"`
	DeletedAt        gorm.DeletedAt `gorm:"index"`
}

type AppConversation struct {
	AppID          string       `gorm:"type:uuid;primaryKey"`
	App            App          `gorm:"constraint:OnDelete:CASCADE;"`
	UserID         string       `gorm:"type:uuid;primaryKey;index"`
	User           User         `gorm:"constraint:OnDelete:CASCADE;"`
	ConversationID string       `gorm:"type:uuid;not null;uniqueIndex"`
	Conversation   Conversation `gorm:"constraint:OnDelete:CASCADE;"`
	CreatedAt      time.Time    `gorm:"not null"`
}

func (AppConversation) TableName() string {
	return "app_conversations"
}

type AppSettings struct {
	ID               int       `gorm:"primaryKey"`
	AppName          string    `gorm:"size:120;not null"`
	OrganizationName string    `gorm:"size:160;not null"`
	CreatedAt        time.Time `gorm:"not null"`
	UpdatedAt        time.Time `gorm:"not null"`
}

type ThirdPartyLoginProvider struct {
	ID           string          `gorm:"type:uuid;primaryKey"`
	Name         string          `gorm:"size:120;not null"`
	Key          string          `gorm:"size:80;not null;uniqueIndex"`
	Type         string          `gorm:"size:32;not null;index"`
	Enabled      bool            `gorm:"not null;index"`
	ClientID     string          `gorm:"size:512;not null"`
	ClientSecret string          `gorm:"not null"`
	Scopes       json.RawMessage `gorm:"type:jsonb;not null;serializer:json"`
	Config       json.RawMessage `gorm:"type:jsonb;not null;serializer:json"`
	SortOrder    int             `gorm:"not null;default:0;index"`
	CreatedAt    time.Time       `gorm:"not null"`
	UpdatedAt    time.Time       `gorm:"not null"`
}

func (ThirdPartyLoginProvider) TableName() string {
	return "third_party_login_providers"
}

type ThirdPartyLoginState struct {
	StateHash    string                  `gorm:"primaryKey"`
	ProviderID   string                  `gorm:"type:uuid;not null;index"`
	Provider     ThirdPartyLoginProvider `gorm:"constraint:OnDelete:CASCADE;"`
	CodeVerifier string                  `gorm:"not null"`
	RedirectPath string                  `gorm:"size:2048;not null"`
	ExpiresAt    time.Time               `gorm:"not null;index"`
	ConsumedAt   *time.Time              `gorm:"index"`
	IP           string                  `gorm:"size:64;not null;default:''"`
	UserAgent    string                  `gorm:"size:512;not null;default:''"`
}

func (ThirdPartyLoginState) TableName() string {
	return "third_party_login_states"
}

type ThirdPartyAccount struct {
	ID             string                  `gorm:"type:uuid;primaryKey"`
	ProviderID     string                  `gorm:"type:uuid;not null;uniqueIndex:third_party_accounts_provider_external_unique,priority:1;index"`
	Provider       ThirdPartyLoginProvider `gorm:"constraint:OnDelete:CASCADE;"`
	ExternalUserID string                  `gorm:"size:256;not null;uniqueIndex:third_party_accounts_provider_external_unique,priority:2"`
	UserID         string                  `gorm:"type:uuid;not null;index"`
	User           User                    `gorm:"constraint:OnDelete:CASCADE;"`
	Profile        json.RawMessage         `gorm:"type:jsonb;not null;serializer:json"`
	CreatedAt      time.Time               `gorm:"not null"`
	UpdatedAt      time.Time               `gorm:"not null"`
}

func (ThirdPartyAccount) TableName() string {
	return "third_party_accounts"
}
