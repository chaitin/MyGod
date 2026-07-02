package store

import "time"

const (
	UserStatusActive   = "active"
	UserStatusDisabled = "disabled"

	ConversationKindDirect    = "direct"
	ConversationKindGroup     = "group"
	ConversationKindAssistant = "assistant"

	ConversationStatusActive    = "active"
	ConversationStatusDissolved = "dissolved"

	ConversationPostingPolicyOpen  = "open"
	ConversationPostingPolicyMuted = "muted"

	ConversationMemberTypeUser      = "user"
	ConversationMemberTypeAssistant = "assistant"

	ConversationMemberRoleOwner  = "owner"
	ConversationMemberRoleAdmin  = "admin"
	ConversationMemberRoleMember = "member"

	AppSettingsID           = 1
	DefaultAppName          = "MyGod"
	DefaultOrganizationName = "长亭科技"
)

type User struct {
	ID           string    `gorm:"type:uuid;primaryKey"`
	Email        string    `gorm:"size:320;not null;uniqueIndex"`
	Name         string    `gorm:"size:120;not null"`
	PasswordHash string    `gorm:"not null"`
	Status       string    `gorm:"size:32;not null;index"`
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
	ID              string    `gorm:"type:uuid;primaryKey"`
	Kind            string    `gorm:"size:32;not null;index"`
	Name            string    `gorm:"size:160;not null"`
	CreatedByUserID string    `gorm:"type:uuid;not null;index"`
	CreatedByUser   User      `gorm:"foreignKey:CreatedByUserID;constraint:OnDelete:RESTRICT;"`
	Status          string    `gorm:"size:32;not null;index"`
	PostingPolicy   string    `gorm:"size:32;not null"`
	CreatedAt       time.Time `gorm:"not null"`
	UpdatedAt       time.Time `gorm:"not null"`
	DissolvedAt     *time.Time
	LastMessageID   *string    `gorm:"type:uuid"`
	LastMessageAt   *time.Time `gorm:"index"`
	Members         []ConversationMember
}

type ConversationMember struct {
	ConversationID    string       `gorm:"type:uuid;primaryKey"`
	Conversation      Conversation `gorm:"constraint:OnDelete:CASCADE;"`
	MemberType        string       `gorm:"size:32;primaryKey"`
	MemberID          string       `gorm:"type:uuid;primaryKey"`
	Role              string       `gorm:"size:32;not null;index"`
	JoinedAt          time.Time    `gorm:"not null"`
	LeftAt            *time.Time   `gorm:"index"`
	LastReadMessageID *string      `gorm:"type:uuid"`
}

type AppSettings struct {
	ID               int       `gorm:"primaryKey"`
	AppName          string    `gorm:"size:120;not null"`
	OrganizationName string    `gorm:"size:160;not null"`
	CreatedAt        time.Time `gorm:"not null"`
	UpdatedAt        time.Time `gorm:"not null"`
}
