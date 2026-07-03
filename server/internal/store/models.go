package store

import (
	"encoding/json"
	"time"
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

	ConversationMemberTypeUser = "user"
	ConversationMemberTypeApp  = "app"

	MessageSenderTypeUser   = "user"
	MessageSenderTypeApp    = "app"
	MessageSenderTypeSystem = "system"

	ConversationMemberRoleOwner  = "owner"
	ConversationMemberRoleAdmin  = "admin"
	ConversationMemberRoleMember = "member"

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
	CreatedByUserID    string    `gorm:"type:uuid;not null;index"`
	CreatedByUser      User      `gorm:"foreignKey:CreatedByUserID;constraint:OnDelete:RESTRICT;"`
	Status             string    `gorm:"size:32;not null;index"`
	PostingPolicy      string    `gorm:"size:32;not null"`
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
}

type Message struct {
	ID              string          `gorm:"type:uuid;primaryKey"`
	ConversationID  string          `gorm:"type:uuid;not null;uniqueIndex:messages_conversation_seq_unique,priority:1;uniqueIndex:messages_client_message_unique,priority:1;index:messages_conversation_seq_index,priority:1"`
	Conversation    Conversation    `gorm:"constraint:OnDelete:CASCADE;"`
	Seq             int64           `gorm:"not null;uniqueIndex:messages_conversation_seq_unique,priority:2;index:messages_conversation_seq_index,priority:2,sort:desc"`
	SenderType      string          `gorm:"size:32;not null;uniqueIndex:messages_client_message_unique,priority:2"`
	SenderID        *string         `gorm:"type:uuid;uniqueIndex:messages_client_message_unique,priority:3"`
	ClientMessageID *string         `gorm:"size:128;uniqueIndex:messages_client_message_unique,priority:4"`
	Body            json.RawMessage `gorm:"type:jsonb;not null;serializer:json"`
	Summary         string          `gorm:"not null;default:''"`
	CreatedAt       time.Time       `gorm:"not null"`
	UpdatedAt       time.Time       `gorm:"not null"`
	DeletedAt       *time.Time      `gorm:"index"`
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

type AppSettings struct {
	ID               int       `gorm:"primaryKey"`
	AppName          string    `gorm:"size:120;not null"`
	OrganizationName string    `gorm:"size:160;not null"`
	CreatedAt        time.Time `gorm:"not null"`
	UpdatedAt        time.Time `gorm:"not null"`
}
