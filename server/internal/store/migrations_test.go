package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMigrationDirectoryContainsOnlyInitialSchema(t *testing.T) {
	matches, err := filepath.Glob("../../migrations/*.sql")
	if err != nil {
		t.Fatalf("glob migrations: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("migration file count = %d, want 1: %v", len(matches), matches)
	}
	if got := filepath.Base(matches[0]); got != "00001_init_schema.sql" {
		t.Fatalf("migration file = %q, want 00001_init_schema.sql", got)
	}
}

func TestInitialSchemaMigrationDefinesCurrentSchema(t *testing.T) {
	rawSQL, err := os.ReadFile("../../migrations/00001_init_schema.sql")
	if err != nil {
		t.Fatalf("read init schema migration: %v", err)
	}
	sql := normalizeSQL(string(rawSQL))

	for _, required := range []string{
		"-- +goose up",
		"create table users",
		"email text not null",
		"name text not null",
		"nickname text not null default ''",
		"phone text",
		"avatar text not null default '/assets/avatars/builtin/01.webp'",
		"password_hash text not null",
		"status text not null",
		"last_online_at timestamptz",
		"constraint users_status_check check (status in ('active', 'disabled'))",
		"create unique index users_email_lower_unique on users (lower(email))",
		"create unique index users_phone_unique",
		"where phone is not null",
		"create table admin_sessions",
		"constraint admin_sessions_token_hash_unique unique (token_hash)",
		"create table user_sessions",
		"user_id uuid not null references users(id) on delete cascade",
		"constraint user_sessions_token_hash_unique unique (token_hash)",
		"create table app_settings",
		"constraint app_settings_singleton_check check (id = 1)",
		"'mygod'",
		"'长亭科技'",
		"create table conversations",
		"created_by_user_id uuid not null references users(id) on delete restrict",
		"last_message_seq bigint not null default 0",
		"last_message_summary text not null default ''",
		"constraint conversations_kind_check check (kind in ('direct', 'group', 'app'))",
		"constraint conversations_status_check check (status in ('active', 'dissolved'))",
		"constraint conversations_posting_policy_check check (posting_policy in ('open', 'muted'))",
		"create table conversation_members",
		"user_member_id uuid generated always as",
		"case when member_type = 'user' then member_id else null end",
		"stored references users(id) on delete restrict",
		"history_visible_from_seq bigint not null default 1",
		"constraint conversation_members_member_type_check check (member_type in ('user', 'app'))",
		"constraint conversation_members_role_check check (role in ('owner', 'admin', 'member'))",
		"constraint conversation_members_history_visible_from_seq_check check (history_visible_from_seq >= 1)",
		"create unique index conversation_members_one_owner_per_conversation",
		"where role = 'owner' and left_at is null",
		"create table messages",
		"body jsonb not null",
		"constraint messages_conversation_seq_unique unique (conversation_id, seq)",
		"constraint messages_client_message_unique unique (conversation_id, sender_type, sender_id, client_message_id)",
		"constraint messages_sender_type_check check (sender_type in ('user', 'app', 'system'))",
		"constraint messages_sender_id_check check",
		"constraint messages_body_object_check check (jsonb_typeof(body) = 'object')",
		"create table direct_conversations",
		"constraint direct_conversations_user_pair_unique unique (user_low_id, user_high_id)",
		"constraint direct_conversations_user_order_check check (user_low_id < user_high_id)",
		"-- +goose down",
	} {
		if !strings.Contains(sql, required) {
			t.Fatalf("init schema migration missing %q", required)
		}
	}

	for _, forbidden := range []string{
		"'assistant'",
		"alter table users",
		"alter table conversations",
		"alter table conversation_members",
		"if not exists",
	} {
		if strings.Contains(sql, forbidden) {
			t.Fatalf("init schema migration contains legacy fragment %q", forbidden)
		}
	}
}

func normalizeSQL(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(value), " "))
}
