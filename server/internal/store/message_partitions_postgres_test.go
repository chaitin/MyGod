package store

import (
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/pressly/goose/v3"
	"gorm.io/gorm"
)

func TestPostgresMessagePartitionsRouteRetainAndDownMigrate(t *testing.T) {
	baseDSN := strings.TrimSpace(os.Getenv("POSTGRES_TEST_DSN"))
	if baseDSN == "" {
		t.Skip("POSTGRES_TEST_DSN is not configured")
	}
	baseDB, err := OpenPostgres(baseDSN)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}

	schema := "message_partition_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if err := baseDB.Exec("CREATE SCHEMA " + quotePostgresTestIdentifier(schema)).Error; err != nil {
		t.Fatalf("create test schema: %v", err)
	}
	t.Cleanup(func() {
		_ = baseDB.Exec("DROP SCHEMA IF EXISTS " + quotePostgresTestIdentifier(schema) + " CASCADE").Error
	})

	testDSN, err := postgresDSNWithSearchPath(baseDSN, schema)
	if err != nil {
		t.Fatalf("build postgres test dsn: %v", err)
	}
	db, err := OpenPostgres(testDSN)
	if err != nil {
		t.Fatalf("open schema postgres: %v", err)
	}
	if err := runPostgresTestMigrationsTo(db, 14); err != nil {
		t.Fatalf("migrate postgres test schema to legacy messages: %v", err)
	}

	now := time.Now().UTC()
	oldYear := MessageMinimumOnlineYear(now) - 1
	oldTime := time.Date(oldYear, time.December, 31, 23, 59, 0, 0, time.UTC)
	user := User{
		ID: uuid.NewString(), Email: "partition-postgres@example.com", Name: "Partition User",
		Avatar: DefaultUserAvatar, PasswordHash: "hash", Status: UserStatusActive,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	conversation := Conversation{
		ID: uuid.NewString(), Kind: ConversationKindDirect, Name: "Partition conversation",
		CreatedByUserID: user.ID, Status: ConversationStatusActive, PostingPolicy: ConversationPostingPolicyOpen,
		Visibility: ConversationVisibilityPrivate, CreatedAt: now, UpdatedAt: now,
	}
	if err := db.Create(&conversation).Error; err != nil {
		t.Fatalf("create conversation: %v", err)
	}
	oldClientMessageID := "old-partitioned-message"
	oldMessage := Message{
		ID: uuid.NewString(), ConversationID: conversation.ID, Seq: 1,
		SenderType: MessageSenderTypeUser, SenderID: &user.ID, ClientMessageID: &oldClientMessageID,
		Body: []byte(`{"type":"text","content":"retained"}`), Summary: "retained",
		CreatedAt: oldTime, UpdatedAt: oldTime,
	}
	if err := db.Create(&oldMessage).Error; err != nil {
		t.Fatalf("create legacy message: %v", err)
	}
	if err := runPostgresTestMigrationsTo(db, 15); err != nil {
		t.Fatalf("partition existing postgres messages: %v", err)
	}

	oldParent := fmt.Sprintf("messages_%d", oldYear)
	var hashPartitionCount int64
	if err := db.Raw(`
		SELECT COUNT(*)
		FROM pg_inherits i
		JOIN pg_class parent ON parent.oid = i.inhparent
		JOIN pg_namespace namespace ON namespace.oid = parent.relnamespace
		WHERE namespace.nspname = ? AND parent.relname = ?
	`, schema, oldParent).Scan(&hashPartitionCount).Error; err != nil {
		t.Fatalf("count old hash partitions: %v", err)
	}
	if hashPartitionCount != MessageHashPartitionCount {
		t.Fatalf("old hash partition count = %d", hashPartitionCount)
	}
	var oldParentAttached bool
	if err := db.Raw(`
		SELECT EXISTS (
			SELECT 1
			FROM pg_inherits i
			JOIN pg_class child ON child.oid = i.inhrelid
			JOIN pg_class parent ON parent.oid = i.inhparent
			JOIN pg_namespace namespace ON namespace.oid = child.relnamespace
			WHERE namespace.nspname = ? AND child.relname = ? AND parent.relname = 'messages'
		)
	`, schema, oldParent).Scan(&oldParentAttached).Error; err != nil {
		t.Fatalf("inspect old annual partition attachment: %v", err)
	}
	if !oldParentAttached {
		t.Fatal("old annual partition is not attached to messages")
	}

	var oldRegistry MessageRegistry
	if err := db.First(&oldRegistry, "id = ?", oldMessage.ID).Error; err != nil {
		t.Fatalf("load old registry: %v", err)
	}
	if int(oldRegistry.PartitionYear) != oldYear {
		t.Fatalf("old registry year = %d, want %d", oldRegistry.PartitionYear, oldYear)
	}
	retained, err := LoadMessageByRegistry(t.Context(), db, oldRegistry)
	if err != nil {
		t.Fatalf("load retained old message: %v", err)
	}
	if retained.ID != oldMessage.ID || retained.Summary != oldMessage.Summary {
		t.Fatalf("retained old message = %#v", retained)
	}

	currentClientMessageID := "current-partitioned-message"
	currentMessage := Message{
		ID: uuid.NewString(), ConversationID: conversation.ID, Seq: 2,
		SenderType: MessageSenderTypeUser, SenderID: &user.ID, ClientMessageID: &currentClientMessageID,
		Body: []byte(`{"type":"text","content":"current"}`), Summary: "current",
		CreatedAt: now, UpdatedAt: now,
	}
	if err := db.Create(&currentMessage).Error; err != nil {
		t.Fatalf("create current message: %v", err)
	}
	var physicalPartition string
	if err := db.Raw("SELECT tableoid::regclass::text FROM messages WHERE id = ?", currentMessage.ID).
		Scan(&physicalPartition).Error; err != nil {
		t.Fatalf("find current physical partition: %v", err)
	}
	wantCurrentPrefix := fmt.Sprintf("messages_%d_p", now.Year())
	if !strings.Contains(physicalPartition, wantCurrentPrefix) {
		t.Fatalf("current physical partition = %q, want prefix %q", physicalPartition, wantCurrentPrefix)
	}
	if err := db.Model(&Message{}).Where("conversation_id = ? AND id = ?", conversation.ID, currentMessage.ID).
		Update("summary", "updated current").Error; err != nil {
		t.Fatalf("update current message: %v", err)
	}
	var currentRegistry MessageRegistry
	if err := db.First(&currentRegistry, "id = ?", currentMessage.ID).Error; err != nil {
		t.Fatalf("load current registry: %v", err)
	}
	if currentRegistry.Summary != "updated current" {
		t.Fatalf("current registry summary = %q", currentRegistry.Summary)
	}

	if err := runPostgresTestMigrationsDownTo(db, 14); err != nil {
		t.Fatalf("roll back message partitions: %v", err)
	}
	var restoredCount int64
	if err := db.Model(&Message{}).Where("conversation_id = ?", conversation.ID).Count(&restoredCount).Error; err != nil {
		t.Fatalf("count down-migrated messages: %v", err)
	}
	if restoredCount != 2 {
		t.Fatalf("down-migrated message count = %d", restoredCount)
	}
}

func quotePostgresTestIdentifier(value string) string {
	return `"` + value + `"`
}

func postgresDSNWithSearchPath(rawDSN, schema string) (string, error) {
	parsed, err := url.Parse(rawDSN)
	if err != nil {
		return "", fmt.Errorf("parse postgres dsn: %w", err)
	}
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func runPostgresTestMigrationsTo(db *gorm.DB, version int64) error {
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	gooseMigrationMu.Lock()
	defer gooseMigrationMu.Unlock()
	if err := goose.SetDialect("postgres"); err != nil {
		return err
	}
	return goose.UpTo(sqlDB, "../../migrations", version)
}

func runPostgresTestMigrationsDownTo(db *gorm.DB, version int64) error {
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	gooseMigrationMu.Lock()
	defer gooseMigrationMu.Unlock()
	if err := goose.SetDialect("postgres"); err != nil {
		return err
	}
	return goose.DownTo(sqlDB, "../../migrations", version)
}
