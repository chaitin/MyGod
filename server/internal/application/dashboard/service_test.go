package dashboard

import (
	"context"
	"testing"
	"time"

	"app/internal/store"

	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func TestServiceReturnsDashboardStats(t *testing.T) {
	db := openDashboardTestDB(t)
	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	users := []store.User{
		newDashboardUser(now.Add(-2 * time.Hour)),
		newDashboardUser(now.Add(-3 * 24 * time.Hour)),
		newDashboardUser(now.Add(-30 * 24 * time.Hour)),
		newDashboardUser(time.Time{}),
	}
	if err := db.Create(&users).Error; err != nil {
		t.Fatalf("create users: %v", err)
	}

	conversations := []store.Conversation{
		newDashboardConversation(users[0].ID, now.Add(-10*time.Hour)),
		newDashboardConversation(users[0].ID, now.Add(-4*24*time.Hour)),
		newDashboardConversation(users[0].ID, now.Add(-30*24*time.Hour)),
	}
	if err := db.Create(&conversations).Error; err != nil {
		t.Fatalf("create conversations: %v", err)
	}
	messages := []store.Message{
		newDashboardMessage(conversations[0].ID, 1, now.Add(-time.Hour)),
		newDashboardMessage(conversations[0].ID, 2, now.Add(-2*time.Hour)),
		newDashboardMessage(conversations[1].ID, 1, now.Add(-3*24*time.Hour)),
		newDashboardMessage(conversations[2].ID, 1, now.Add(-30*24*time.Hour)),
	}
	if err := db.Create(&messages).Error; err != nil {
		t.Fatalf("create messages: %v", err)
	}

	presence := dashboardPresence{online: map[string]bool{users[3].ID: true}}
	service := NewService(Dependencies{
		DB: db, Presence: presence, Now: func() time.Time { return now },
	})
	stats, err := service.GetStats(context.Background())
	if err != nil {
		t.Fatalf("get stats: %v", err)
	}
	want := Stats{
		TotalUsers: 4, VisitedUsers24Hours: 2, VisitedUsers7Days: 3, OnlineUsers: 1,
		Messages24Hours: 2, Messages7Days: 3,
		ActiveConversations24H: 1, ActiveConversations7D: 2,
	}
	if stats != want {
		t.Fatalf("stats = %#v, want %#v", stats, want)
	}
}

type dashboardPresence struct {
	online map[string]bool
}

func (p dashboardPresence) OnlineStatus(userIDs []string) map[string]bool {
	result := make(map[string]bool, len(userIDs))
	for _, userID := range userIDs {
		result[userID] = p.online[userID]
	}
	return result
}

func openDashboardTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if err := db.AutoMigrate(&store.User{}, &store.Conversation{}, &store.Message{}); err != nil {
		t.Fatalf("migrate database: %v", err)
	}
	return db
}

func newDashboardUser(lastOnlineAt time.Time) store.User {
	var lastOnline *time.Time
	if !lastOnlineAt.IsZero() {
		lastOnline = &lastOnlineAt
	}
	return store.User{
		ID: uuid.NewString(), Email: uuid.NewString() + "@example.com", Name: "User",
		PasswordHash: "hash", Status: store.UserStatusActive, LastOnlineAt: lastOnline,
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
}

func newDashboardConversation(userID string, createdAt time.Time) store.Conversation {
	return store.Conversation{
		ID: uuid.NewString(), Kind: store.ConversationKindGroup, Name: "Group",
		CreatedByUserID: userID, Status: store.ConversationStatusActive,
		PostingPolicy: store.ConversationPostingPolicyOpen, Visibility: store.ConversationVisibilityPrivate,
		CreatedAt: createdAt, UpdatedAt: createdAt, LastMessageAt: &createdAt,
	}
}

func newDashboardMessage(conversationID string, seq int64, createdAt time.Time) store.Message {
	return store.Message{
		ID: uuid.NewString(), ConversationID: conversationID, Seq: seq,
		SenderType: store.MessageSenderTypeSystem, Body: []byte(`{"type":"system","text":"event"}`),
		Summary: "event", CreatedAt: createdAt, UpdatedAt: createdAt,
	}
}
