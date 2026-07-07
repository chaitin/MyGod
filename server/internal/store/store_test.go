package store

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func openTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := "file:" + uuid.NewString() + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite db: %v", err)
	}
	if err := migrateTestSchema(db); err != nil {
		t.Fatalf("migrate test schema: %v", err)
	}

	return db
}

func TestStoreSchemaSupportsAuthTables(t *testing.T) {
	db := openTestDB(t)
	now := time.Now().UTC()

	user := User{
		ID:           uuid.NewString(),
		Avatar:       DefaultUserAvatar,
		Email:        "wenlei@example.com",
		Name:         "Wenlei Zhu",
		Nickname:     "",
		PasswordHash: "hash",
		Status:       UserStatusActive,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	adminSession := AdminSession{
		ID:         uuid.NewString(),
		TokenHash:  "admin-token-hash",
		ExpiresAt:  now.Add(7 * 24 * time.Hour),
		LastSeenAt: now,
	}
	if err := db.Create(&adminSession).Error; err != nil {
		t.Fatalf("create admin session: %v", err)
	}

	userSession := UserSession{
		ID:         uuid.NewString(),
		TokenHash:  "user-token-hash",
		UserID:     user.ID,
		ExpiresAt:  now.Add(7 * 24 * time.Hour),
		LastSeenAt: now,
	}
	if err := db.Create(&userSession).Error; err != nil {
		t.Fatalf("create user session: %v", err)
	}
}

func TestPingChecksDatabaseConnection(t *testing.T) {
	db := openTestDB(t)

	if err := Ping(db); err != nil {
		t.Fatalf("Ping() error = %v", err)
	}
}

func TestUserEmailIsUnique(t *testing.T) {
	db := openTestDB(t)

	first := User{
		ID:           uuid.NewString(),
		Avatar:       DefaultUserAvatar,
		Email:        "wenlei@example.com",
		Name:         "Wenlei Zhu",
		Nickname:     "",
		PasswordHash: "hash",
		Status:       UserStatusActive,
	}
	if err := db.Create(&first).Error; err != nil {
		t.Fatalf("create first user: %v", err)
	}

	second := User{
		ID:           uuid.NewString(),
		Avatar:       DefaultUserAvatar,
		Email:        "wenlei@example.com",
		Name:         "Another User",
		Nickname:     "",
		PasswordHash: "hash",
		Status:       UserStatusActive,
	}
	if err := db.Create(&second).Error; err == nil {
		t.Fatal("create duplicate email user error = nil, want unique constraint error")
	}
}

func TestStoreSchemaSupportsMessageStorageTables(t *testing.T) {
	db := openTestDB(t)
	now := time.Now().UTC()
	aliceID := "00000000-0000-0000-0000-000000000001"
	bobID := "00000000-0000-0000-0000-000000000002"

	alice := User{
		ID:           aliceID,
		Avatar:       DefaultUserAvatar,
		Email:        "alice@example.com",
		Name:         "Alice",
		Nickname:     "",
		PasswordHash: "hash",
		Status:       UserStatusActive,
	}
	if err := db.Create(&alice).Error; err != nil {
		t.Fatalf("create alice: %v", err)
	}
	bob := User{
		ID:           bobID,
		Avatar:       DefaultUserAvatar,
		Email:        "bob@example.com",
		Name:         "Bob",
		Nickname:     "",
		PasswordHash: "hash",
		Status:       UserStatusActive,
	}
	if err := db.Create(&bob).Error; err != nil {
		t.Fatalf("create bob: %v", err)
	}

	conversation := Conversation{
		ID:              uuid.NewString(),
		Kind:            ConversationKindDirect,
		Name:            "",
		CreatedByUserID: alice.ID,
		Status:          ConversationStatusActive,
		PostingPolicy:   ConversationPostingPolicyOpen,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := db.Create(&conversation).Error; err != nil {
		t.Fatalf("create conversation: %v", err)
	}

	body := json.RawMessage(`{"type":"text","content":"hello"}`)
	message := Message{
		ID:              uuid.NewString(),
		ConversationID:  conversation.ID,
		Seq:             1,
		SenderType:      MessageSenderTypeUser,
		SenderID:        &alice.ID,
		ClientMessageID: ptrString("client-message-1"),
		Body:            body,
		Summary:         "hello",
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := db.Create(&message).Error; err != nil {
		t.Fatalf("create message: %v", err)
	}

	conversation.LastMessageID = &message.ID
	conversation.LastMessageSeq = message.Seq
	conversation.LastMessageSummary = message.Summary
	conversation.LastMessageAt = &message.CreatedAt
	if err := db.Save(&conversation).Error; err != nil {
		t.Fatalf("update conversation last message: %v", err)
	}

	direct := DirectConversation{
		ConversationID: conversation.ID,
		UserLowID:      alice.ID,
		UserHighID:     bob.ID,
		CreatedAt:      now,
	}
	if err := db.Create(&direct).Error; err != nil {
		t.Fatalf("create direct conversation: %v", err)
	}

	duplicateMessage := Message{
		ID:              uuid.NewString(),
		ConversationID:  conversation.ID,
		Seq:             2,
		SenderType:      MessageSenderTypeUser,
		SenderID:        &alice.ID,
		ClientMessageID: ptrString("client-message-1"),
		Body:            body,
		Summary:         "duplicate",
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := db.Create(&duplicateMessage).Error; err == nil {
		t.Fatal("create duplicate client message error = nil, want unique constraint error")
	}

	duplicateDirect := DirectConversation{
		ConversationID: uuid.NewString(),
		UserLowID:      alice.ID,
		UserHighID:     bob.ID,
		CreatedAt:      now,
	}
	if err := db.Create(&duplicateDirect).Error; err == nil {
		t.Fatal("create duplicate direct conversation error = nil, want unique constraint error")
	}

	reversedConversation := Conversation{
		ID:              uuid.NewString(),
		Kind:            ConversationKindDirect,
		Name:            "",
		CreatedByUserID: alice.ID,
		Status:          ConversationStatusActive,
		PostingPolicy:   ConversationPostingPolicyOpen,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := db.Create(&reversedConversation).Error; err != nil {
		t.Fatalf("create reversed conversation: %v", err)
	}

	reversedDirect := DirectConversation{
		ConversationID: reversedConversation.ID,
		UserLowID:      bob.ID,
		UserHighID:     alice.ID,
		CreatedAt:      now,
	}
	if err := db.Create(&reversedDirect).Error; err == nil {
		t.Fatal("create reversed direct conversation error = nil, want low/high ordering constraint error")
	}
}

func ptrString(value string) *string {
	return &value
}

func migrateTestSchema(db *gorm.DB) error {
	return db.AutoMigrate(
		&User{},
		&AdminSession{},
		&UserSession{},
		&Conversation{},
		&ConversationMember{},
		&Message{},
		&DirectConversation{},
		&TemporaryFile{},
		&App{},
		&AppConversation{},
		&AppSettings{},
		&ThirdPartyLoginProvider{},
		&ThirdPartyLoginState{},
		&ThirdPartyAccount{},
		&LLMModel{},
	)
}
