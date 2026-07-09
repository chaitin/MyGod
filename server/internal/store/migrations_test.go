package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMigrationDirectoryContainsExpectedMigrations(t *testing.T) {
	matches, err := filepath.Glob("../../migrations/*.sql")
	if err != nil {
		t.Fatalf("glob migrations: %v", err)
	}
	want := []string{
		"00001_init_schema.sql",
		"00002_add_message_delegation.sql",
		"00003_add_message_reply_to.sql",
		"00004_add_app_soft_delete.sql",
		"00005_add_message_revoke.sql",
		"00006_add_conversation_member_mentions.sql",
	}
	if len(matches) != len(want) {
		t.Fatalf("migration file count = %d, want %d: %v", len(matches), len(want), matches)
	}
	for index, match := range matches {
		if got := filepath.Base(match); got != want[index] {
			t.Fatalf("migration file %d = %q, want %q", index, got, want[index])
		}
	}
}

func TestInitialSchemaMigrationDefinesBaseSchema(t *testing.T) {
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
		"create table admin_sessions",
		"constraint admin_sessions_token_hash_unique unique (token_hash)",
		"create table user_sessions",
		"user_id uuid not null references users(id) on delete cascade",
		"constraint user_sessions_token_hash_unique unique (token_hash)",
		"create table app_settings",
		"constraint app_settings_singleton_check check (id = 1)",
		"'mygod'",
		"'长亭科技'",
		"create table third_party_login_providers",
		"key text not null",
		"type text not null",
		"client_secret text not null",
		"scopes jsonb not null default '[\"openid\",\"email\",\"profile\"]'",
		"config jsonb not null default '{}'",
		"constraint third_party_login_providers_type_check check (type in ('dingtalk', 'wecom', 'feishu', 'github', 'google', 'oidc'))",
		"create unique index third_party_login_providers_key_unique on third_party_login_providers (key)",
		"create table third_party_login_states",
		"state_hash text primary key",
		"code_verifier text not null",
		"redirect_path text not null",
		"create table third_party_accounts",
		"external_user_id text not null",
		"profile jsonb not null default '{}'",
		"constraint third_party_accounts_provider_external_unique unique (provider_id, external_user_id)",
		"create table conversations",
		"created_by_user_id uuid not null references users(id) on delete restrict",
		"avatar text not null default ''",
		"visibility text not null default 'private'",
		"last_message_seq bigint not null default 0",
		"last_message_summary text not null default ''",
		"constraint conversations_kind_check check (kind in ('direct', 'group', 'app'))",
		"constraint conversations_status_check check (status in ('active', 'dissolved'))",
		"constraint conversations_posting_policy_check check (posting_policy in ('open', 'muted'))",
		"constraint conversations_visibility_check check (visibility in ('private', 'public'))",
		"create index conversations_visibility_index on conversations (visibility)",
		"create table conversation_members",
		"user_member_id uuid generated always as",
		"case when member_type = 'user' then member_id else null end",
		"stored references users(id) on delete restrict",
		"history_visible_from_seq bigint not null default 1",
		"last_read_seq bigint not null default 0",
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
		"create table temporary_files",
		"object_key text not null",
		"size_bytes bigint not null",
		"created_at timestamptz not null default now()",
		"constraint temporary_files_object_key_unique unique (object_key)",
		"constraint temporary_files_size_bytes_check check (size_bytes >= 0)",
		"create table apps",
		"creator_user_id uuid references users(id) on delete set null",
		"visibility text not null",
		"connection_secret text not null",
		"constraint apps_visibility_check check (visibility in ('creator', 'public'))",
		"constraint apps_connection_secret_unique unique (connection_secret)",
		"create table app_conversations",
		"app_id uuid not null references apps(id) on delete cascade",
		"user_id uuid not null references users(id) on delete cascade",
		"conversation_id uuid not null references conversations(id) on delete cascade",
		"primary key (app_id, user_id)",
		"constraint app_conversations_conversation_unique unique (conversation_id)",
		"drop table app_conversations",
		"drop table apps",
		"drop table temporary_files",
		"-- +goose down",
		"create unique index users_phone_unique",
		"where phone is not null",
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
		"delegated_by",
		"llm_models",
		"rename column",
		"if not exists",
	} {
		if strings.Contains(sql, forbidden) {
			t.Fatalf("init schema migration contains legacy fragment %q", forbidden)
		}
	}
}

func TestConversationMemberMentionsMigrationAddsLastMentionedSeq(t *testing.T) {
	rawSQL, err := os.ReadFile("../../migrations/00006_add_conversation_member_mentions.sql")
	if err != nil {
		t.Fatalf("read conversation member mentions migration: %v", err)
	}
	sql := normalizeSQL(string(rawSQL))

	for _, required := range []string{
		"-- +goose up",
		"alter table conversation_members",
		"add column last_mentioned_seq bigint not null default 0",
		"-- +goose down",
		"drop column last_mentioned_seq",
	} {
		if !strings.Contains(sql, required) {
			t.Fatalf("conversation member mentions migration missing %q", required)
		}
	}
}

func TestMessageDelegationMigrationAddsDelegationColumns(t *testing.T) {
	rawSQL, err := os.ReadFile("../../migrations/00002_add_message_delegation.sql")
	if err != nil {
		t.Fatalf("read message delegation migration: %v", err)
	}
	sql := normalizeSQL(string(rawSQL))

	for _, required := range []string{
		"-- +goose up",
		"alter table messages add column delegated_by_type text, add column delegated_by_id uuid, add column delegated_by_name text not null default ''",
		"add constraint messages_delegated_by_type_check check",
		"delegated_by_type is null or delegated_by_type in ('user', 'app')",
		"add constraint messages_delegated_by_id_check check",
		"delegated_by_type is null and delegated_by_id is null and delegated_by_name = ''",
		"delegated_by_type is not null and delegated_by_id is not null and delegated_by_name <> ''",
		"-- +goose down",
		"drop constraint messages_delegated_by_id_check",
		"drop constraint messages_delegated_by_type_check",
		"drop column delegated_by_name",
		"drop column delegated_by_id",
		"drop column delegated_by_type",
	} {
		if !strings.Contains(sql, required) {
			t.Fatalf("message delegation migration missing %q", required)
		}
	}
}

func TestMessageReplyToMigrationAddsReplyToColumn(t *testing.T) {
	rawSQL, err := os.ReadFile("../../migrations/00003_add_message_reply_to.sql")
	if err != nil {
		t.Fatalf("read message reply-to migration: %v", err)
	}
	sql := normalizeSQL(string(rawSQL))

	for _, required := range []string{
		"-- +goose up",
		"alter table messages add column reply_to_message_id uuid",
		"create index messages_reply_to_message_id_index on messages (reply_to_message_id)",
		"add constraint messages_reply_to_message_id_fkey foreign key (reply_to_message_id) references messages(id) on delete set null",
		"-- +goose down",
		"drop constraint messages_reply_to_message_id_fkey",
		"drop column reply_to_message_id",
	} {
		if !strings.Contains(sql, required) {
			t.Fatalf("message reply-to migration missing %q", required)
		}
	}
}

func TestAppSoftDeleteMigrationAddsDeletedAt(t *testing.T) {
	rawSQL, err := os.ReadFile("../../migrations/00004_add_app_soft_delete.sql")
	if err != nil {
		t.Fatalf("read app soft delete migration: %v", err)
	}
	sql := normalizeSQL(string(rawSQL))

	for _, required := range []string{
		"-- +goose up",
		"alter table apps add column deleted_at timestamptz",
		"create index apps_deleted_at_index on apps (deleted_at)",
		"-- +goose down",
		"drop index apps_deleted_at_index",
		"drop column deleted_at",
	} {
		if !strings.Contains(sql, required) {
			t.Fatalf("app soft delete migration missing %q", required)
		}
	}
}

func TestMessageRevokeMigrationAddsRevokeColumns(t *testing.T) {
	rawSQL, err := os.ReadFile("../../migrations/00005_add_message_revoke.sql")
	if err != nil {
		t.Fatalf("read message revoke migration: %v", err)
	}
	sql := normalizeSQL(string(rawSQL))

	for _, required := range []string{
		"-- +goose up",
		"alter table messages add column revoked_at timestamptz, add column revoked_by_user_id uuid",
		"create index messages_revoked_at_index on messages (revoked_at)",
		"add constraint messages_revoked_by_user_id_fkey foreign key (revoked_by_user_id) references users(id) on delete set null",
		"-- +goose down",
		"drop constraint messages_revoked_by_user_id_fkey",
		"drop column revoked_by_user_id",
		"drop column revoked_at",
	} {
		if !strings.Contains(sql, required) {
			t.Fatalf("message revoke migration missing %q", required)
		}
	}
}

func normalizeSQL(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(value), " "))
}
