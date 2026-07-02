package store

import (
	"os"
	"strings"
	"testing"
)

func TestConversationMigrationDefinesConversationMemberConstraints(t *testing.T) {
	rawSQL, err := os.ReadFile("../../migrations/00005_create_conversations.sql")
	if err != nil {
		t.Fatalf("read conversation migration: %v", err)
	}
	sql := strings.ToLower(string(rawSQL))

	for _, required := range []string{
		"user_member_id uuid generated always as",
		"when member_type = 'user' then member_id",
		"references users(id) on delete restrict",
		"conversation_members_one_owner_per_conversation",
		"where role = 'owner' and left_at is null",
	} {
		if !strings.Contains(sql, required) {
			t.Fatalf("conversation migration missing %q", required)
		}
	}
}
