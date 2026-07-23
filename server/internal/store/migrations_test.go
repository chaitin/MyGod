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
		"00007_legacy_placeholder.sql",
		"00008_add_app_event_outbox.sql",
		"00009_add_projects_and_tasks.sql",
		"00010_restore_third_party_login.sql",
		"00011_rename_ai_assistant.sql",
		"00012_rename_product_display_name.sql",
		"00013_add_task_reminders.sql",
		"00014_normalize_task_reminder_time.sql",
		"00015_partition_messages_by_year_and_conversation.sql",
		"00016_add_temporary_file_expiration.sql",
		"00017_add_user_managed_apps.sql",
		"00018_add_email_code_login_settings.sql",
		"00019_default_email_login_to_tls.sql",
		"00020_add_password_login_setting.sql",
		"00021_add_app_owned_groups.sql",
		"00022_add_conversation_topics.sql",
		"00023_add_conversation_pins.sql",
		"00024_repair_conversation_topic_app_columns.sql",
		"00025_add_message_reactions.sql",
		"00026_add_conversation_user_preferences.sql",
		"00027_add_message_choices.sql",
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

func TestMessageChoicesMigration(t *testing.T) {
	rawSQL, err := os.ReadFile("../../migrations/00027_add_message_choices.sql")
	if err != nil {
		t.Fatalf("read message choices migration: %v", err)
	}
	sql := normalizeSQL(string(rawSQL))
	for _, required := range []string{
		"add column last_choice_seq bigint not null default 0",
		"create table message_choice_responses",
		"conversation_id uuid not null references conversations(id) on delete cascade",
		"message_id uuid not null references message_registry(id) on delete cascade",
		"user_id uuid not null references users(id) on delete cascade",
		"option_ids jsonb not null",
		"unique (message_id, user_id)",
		"drop table message_choice_responses",
		"drop column last_choice_seq",
	} {
		if !strings.Contains(sql, required) {
			t.Fatalf("message choices migration missing %q", required)
		}
	}
}

func TestConversationUserPreferencesMigration(t *testing.T) {
	rawSQL, err := os.ReadFile("../../migrations/00026_add_conversation_user_preferences.sql")
	if err != nil {
		t.Fatalf("read conversation user preferences migration: %v", err)
	}
	sql := normalizeSQL(string(rawSQL))
	for _, required := range []string{
		"create table conversation_user_preferences",
		"pinned boolean not null default false",
		"notification_muted boolean not null default false",
		"hidden_through_seq bigint",
		"primary key (user_id, conversation_id)",
		"insert into conversation_user_preferences",
		"from conversation_pins",
		"drop table conversation_pins",
		"create table conversation_pins",
		"where pinned = true",
		"drop table conversation_user_preferences",
	} {
		if !strings.Contains(sql, required) {
			t.Fatalf("conversation user preferences migration missing %q", required)
		}
	}
}

func TestConversationTopicAppColumnsRepairMigration(t *testing.T) {
	rawSQL, err := os.ReadFile("../../migrations/00024_repair_conversation_topic_app_columns.sql")
	if err != nil {
		t.Fatalf("read conversation topic app columns repair migration: %v", err)
	}
	sql := normalizeSQL(string(rawSQL))
	for _, required := range []string{
		"add column if not exists created_by_app_id uuid references apps(id) on delete restrict",
		"add column if not exists archived_by_app_id uuid references apps(id) on delete set null",
		"create index if not exists conversation_topics_created_by_app_id_index",
		"-- +goose down",
		"select 1",
	} {
		if !strings.Contains(sql, required) {
			t.Fatalf("conversation topic app columns repair migration missing %q", required)
		}
	}
}

func TestMessageReactionsMigration(t *testing.T) {
	rawSQL, err := os.ReadFile("../../migrations/00025_add_message_reactions.sql")
	if err != nil {
		t.Fatalf("read message reactions migration: %v", err)
	}
	sql := normalizeSQL(string(rawSQL))
	for _, required := range []string{
		"create table message_reactions",
		"message_id uuid not null references message_registry(id) on delete cascade",
		"user_id uuid not null references users(id) on delete cascade",
		"text varchar(128) not null",
		"primary key (message_id, user_id, text)",
		"create index message_reactions_message_text_index",
		"create table message_reaction_states",
		"version bigint not null default 0",
		"drop table message_reaction_states",
		"drop table message_reactions",
	} {
		if !strings.Contains(sql, required) {
			t.Fatalf("message reactions migration missing %q", required)
		}
	}
}

func TestConversationPinsMigration(t *testing.T) {
	rawSQL, err := os.ReadFile("../../migrations/00023_add_conversation_pins.sql")
	if err != nil {
		t.Fatalf("read conversation pins migration: %v", err)
	}
	sql := normalizeSQL(string(rawSQL))
	for _, required := range []string{
		"create table conversation_pins",
		"user_id uuid not null references users(id) on delete cascade",
		"conversation_id uuid not null references conversations(id) on delete cascade",
		"primary key (user_id, conversation_id)",
		"create index conversation_pins_conversation_id_index",
		"drop table conversation_pins",
	} {
		if !strings.Contains(sql, required) {
			t.Fatalf("conversation pins migration missing %q", required)
		}
	}
}

func TestAppOwnedGroupsMigration(t *testing.T) {
	rawSQL, err := os.ReadFile("../../migrations/00021_add_app_owned_groups.sql")
	if err != nil {
		t.Fatalf("read app-owned groups migration: %v", err)
	}
	sql := normalizeSQL(string(rawSQL))
	for _, required := range []string{
		"add column created_by_app_id uuid references apps(id) on delete restrict",
		"create index conversations_created_by_app_id_index",
		"drop index conversations_created_by_app_id_index",
		"drop column created_by_app_id",
	} {
		if !strings.Contains(sql, required) {
			t.Fatalf("app-owned groups migration missing %q", required)
		}
	}
}

func TestConversationTopicsMigration(t *testing.T) {
	rawSQL, err := os.ReadFile("../../migrations/00022_add_conversation_topics.sql")
	if err != nil {
		t.Fatalf("read conversation topics migration: %v", err)
	}
	sql := normalizeSQL(string(rawSQL))
	for _, required := range []string{
		"check (kind in ('direct', 'group', 'app', 'topic'))",
		"create table conversation_topics",
		"created_by_app_id uuid references apps(id) on delete restrict",
		"archived_by_app_id uuid references apps(id) on delete set null",
		"constraint conversation_topics_source_message_unique unique (source_message_id)",
		"create index conversation_topics_created_by_app_id_index",
		"create table conversation_topic_participants",
		"delete from conversations where kind = 'topic'",
		"drop table conversation_topic_participants",
		"drop table conversation_topics",
	} {
		if !strings.Contains(sql, required) {
			t.Fatalf("conversation topics migration missing %q", required)
		}
	}
}

func TestPasswordLoginSettingMigration(t *testing.T) {
	rawSQL, err := os.ReadFile("../../migrations/00020_add_password_login_setting.sql")
	if err != nil {
		t.Fatalf("read password login setting migration: %v", err)
	}
	sql := normalizeSQL(string(rawSQL))
	for _, required := range []string{
		"-- +goose up",
		"add column password_login_enabled boolean not null default true",
		"-- +goose down",
		"drop column password_login_enabled",
	} {
		if !strings.Contains(sql, required) {
			t.Fatalf("password login setting migration missing %q", required)
		}
	}
}

func TestDefaultEmailLoginToTLSMigration(t *testing.T) {
	rawSQL, err := os.ReadFile("../../migrations/00019_default_email_login_to_tls.sql")
	if err != nil {
		t.Fatalf("read default email login TLS migration: %v", err)
	}
	sql := normalizeSQL(string(rawSQL))
	for _, required := range []string{
		"-- +goose up",
		"alter column smtp_port set default 465",
		"alter column smtp_security set default 'tls'",
		"set smtp_port = 465",
		"smtp_security = 'tls'",
		"where email_code_login_enabled = false",
		"and smtp_host = ''",
		"and smtp_port = 587",
		"and smtp_security = 'starttls'",
		"-- +goose down",
		"alter column smtp_port set default 587",
		"alter column smtp_security set default 'starttls'",
	} {
		if !strings.Contains(sql, required) {
			t.Fatalf("default email login TLS migration missing %q", required)
		}
	}
}

func TestEmailCodeLoginSettingsMigrationDefinesSMTPConfiguration(t *testing.T) {
	rawSQL, err := os.ReadFile("../../migrations/00018_add_email_code_login_settings.sql")
	if err != nil {
		t.Fatalf("read email-code login settings migration: %v", err)
	}
	sql := normalizeSQL(string(rawSQL))
	for _, required := range []string{
		"-- +goose up",
		"add column email_code_login_enabled boolean not null default false",
		"add column smtp_host text not null default ''",
		"add column smtp_port integer not null default 587",
		"add column smtp_security text not null default 'starttls'",
		"add column smtp_password text not null default ''",
		"check (smtp_port between 1 and 65535)",
		"check (smtp_security in ('none', 'starttls', 'tls'))",
		"-- +goose down",
		"drop column email_code_login_enabled",
	} {
		if !strings.Contains(sql, required) {
			t.Fatalf("email-code login settings migration missing %q", required)
		}
	}
}

func TestUserManagedAppsMigrationDefinesRestrictedAccess(t *testing.T) {
	rawSQL, err := os.ReadFile("../../migrations/00017_add_user_managed_apps.sql")
	if err != nil {
		t.Fatalf("read user-managed apps migration: %v", err)
	}
	sql := normalizeSQL(string(rawSQL))
	for _, required := range []string{
		"-- +goose up",
		"check (visibility in ('creator', 'restricted', 'public'))",
		"create table app_user_grants",
		"app_id uuid not null references apps(id) on delete cascade",
		"user_id uuid not null references users(id) on delete cascade",
		"granted_by_user_id uuid references users(id) on delete set null",
		"primary key (app_id, user_id)",
		"create index app_user_grants_user_id_index",
		"-- +goose down",
		"drop table app_user_grants",
	} {
		if !strings.Contains(sql, required) {
			t.Fatalf("user-managed apps migration missing %q", required)
		}
	}
}

func TestRestoreThirdPartyLoginMigrationDefinesSchema(t *testing.T) {
	rawSQL, err := os.ReadFile("../../migrations/00010_restore_third_party_login.sql")
	if err != nil {
		t.Fatalf("read third-party login restore migration: %v", err)
	}
	sql := normalizeSQL(string(rawSQL))

	for _, required := range []string{
		"-- +goose up",
		"create table if not exists third_party_login_providers",
		"create unique index if not exists third_party_login_providers_key_unique",
		"create table if not exists third_party_login_states",
		"create table if not exists third_party_accounts",
		"external_user_id text not null",
		"user_id uuid not null references users(id) on delete cascade",
		"-- +goose down",
		"select 1",
	} {
		if !strings.Contains(sql, required) {
			t.Fatalf("third-party login restore migration missing %q", required)
		}
	}
}

func TestLegacyMigrationSevenIsReservedAsNoOp(t *testing.T) {
	rawSQL, err := os.ReadFile("../../migrations/00007_legacy_placeholder.sql")
	if err != nil {
		t.Fatalf("read migration 7 placeholder: %v", err)
	}
	sql := normalizeSQL(string(rawSQL))
	for _, required := range []string{"-- +goose up", "select 1", "-- +goose down"} {
		if !strings.Contains(sql, required) {
			t.Fatalf("migration 7 placeholder missing %q", required)
		}
	}
	for _, forbidden := range []string{"create table", "alter table", "drop table", "insert into", "update ", "delete from"} {
		if strings.Contains(sql, forbidden) {
			t.Fatalf("migration 7 placeholder contains mutating SQL %q", forbidden)
		}
	}
}

func TestProjectsAndTasksMigrationDefinesSchema(t *testing.T) {
	rawSQL, err := os.ReadFile("../../migrations/00009_add_projects_and_tasks.sql")
	if err != nil {
		t.Fatalf("read projects and tasks migration: %v", err)
	}
	sql := normalizeSQL(string(rawSQL))

	for _, required := range []string{
		"-- +goose up",
		"create table if not exists projects",
		"id uuid primary key",
		"name text not null",
		"description text not null default ''",
		"avatar text not null default ''",
		"owner_user_id uuid not null references users(id) on delete restrict",
		"created_by_user_id uuid not null references users(id) on delete restrict",
		"is_personal boolean not null default false",
		"deleted_at timestamptz",
		"constraint projects_name_check check (char_length(btrim(name)) between 1 and 120)",
		"create unique index if not exists projects_one_personal_per_owner",
		"where is_personal and deleted_at is null",
		"create index if not exists projects_owner_user_id_index",
		"create index if not exists projects_updated_at_index",
		"create table if not exists project_groups",
		"project_id uuid not null references projects(id) on delete cascade",
		"conversation_id uuid not null references conversations(id) on delete cascade",
		"linked_by_user_id uuid not null references users(id) on delete restrict",
		"primary key (project_id, conversation_id)",
		"create index if not exists project_groups_conversation_id_index",
		"create table if not exists tasks",
		"project_id uuid not null references projects(id) on delete cascade",
		"title text not null",
		"description text not null default ''",
		"status text not null default 'todo'",
		"priority smallint not null default 2",
		"assignee_user_id uuid references users(id) on delete set null",
		"start_date date",
		"due_date date",
		"labels text[] not null default '{}'",
		"created_by_user_id uuid not null references users(id) on delete restrict",
		"completed_at timestamptz",
		"canceled_at timestamptz",
		"constraint tasks_title_check check (char_length(btrim(title)) between 1 and 240)",
		"constraint tasks_status_check check (status in ('todo', 'in_progress', 'done', 'canceled'))",
		"constraint tasks_priority_check check (priority between 1 and 3)",
		"constraint tasks_date_order_check check (start_date is null or due_date is null or start_date <= due_date)",
		"constraint tasks_completed_at_check check",
		"constraint tasks_canceled_at_check check",
		"create index if not exists tasks_project_updated_at_index",
		"create index if not exists tasks_status_index",
		"create index if not exists tasks_assignee_user_id_index",
		"create index if not exists tasks_start_date_index",
		"create index if not exists tasks_due_date_index",
		"create index if not exists tasks_labels_gin_index on tasks using gin (labels)",
		"insert into projects",
		"select gen_random_uuid(), '个人工作区', '', '', id, id, true, created_at, updated_at",
		"on conflict (owner_user_id) where is_personal and deleted_at is null do nothing",
		"-- +goose down",
		"drop table tasks",
		"drop table project_groups",
		"drop table projects",
	} {
		if !strings.Contains(sql, required) {
			t.Fatalf("projects/tasks migration missing %q", required)
		}
	}
}

func TestTaskRemindersMigrationDefinesSchema(t *testing.T) {
	rawSQL, err := os.ReadFile("../../migrations/00013_add_task_reminders.sql")
	if err != nil {
		t.Fatalf("read task reminders migration: %v", err)
	}
	sql := normalizeSQL(string(rawSQL))
	for _, required := range []string{
		"-- +goose up",
		"create table task_reminders",
		"task_id uuid not null unique references tasks(id) on delete cascade",
		"mode text not null",
		"timezone text not null",
		"once_at timestamptz",
		"time_of_day time(0)",
		"weekdays smallint[] not null default '{}'",
		"day_of_month smallint",
		"next_trigger_at timestamptz",
		"constraint task_reminders_schedule_check check",
		"create index task_reminders_due_index",
		"-- +goose down",
		"drop table task_reminders",
	} {
		if !strings.Contains(sql, required) {
			t.Fatalf("task reminders migration missing %q", required)
		}
	}
}

func TestTaskReminderTimeMigrationNormalizesStorageAndTimezone(t *testing.T) {
	rawSQL, err := os.ReadFile("../../migrations/00014_normalize_task_reminder_time.sql")
	if err != nil {
		t.Fatalf("read task reminder time migration: %v", err)
	}
	sql := normalizeSQL(string(rawSQL))
	for _, required := range []string{
		"-- +goose up",
		"alter column time_of_day type varchar(5)",
		"using to_char(time_of_day, 'hh24:mi')",
		"constraint task_reminders_time_of_day_check check",
		"update task_reminders set timezone = 'asia/shanghai'",
		"-- +goose down",
		"alter column time_of_day type time(0)",
		"using time_of_day::time(0)",
	} {
		if !strings.Contains(sql, required) {
			t.Fatalf("task reminder time migration missing %q", required)
		}
	}
}

func TestAppEventOutboxMigrationDefinesCursorStorage(t *testing.T) {
	rawSQL, err := os.ReadFile("../../migrations/00008_add_app_event_outbox.sql")
	if err != nil {
		t.Fatalf("read app event outbox migration: %v", err)
	}
	sql := normalizeSQL(string(rawSQL))
	for _, required := range []string{
		"-- +goose up",
		"create table app_event_outbox",
		"app_id uuid not null references apps(id) on delete cascade",
		"event text not null",
		"payload jsonb not null",
		"create index app_event_outbox_app_cursor_index on app_event_outbox (app_id, id)",
		"create table app_event_acks",
		"last_acked_cursor bigint not null default 0",
		"-- +goose down",
		"drop table app_event_acks",
		"drop table app_event_outbox",
	} {
		if !strings.Contains(sql, required) {
			t.Fatalf("app event outbox migration missing %q", required)
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
		"'即应'",
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

func TestMessagePartitionMigrationDefinesRegistryAndYearHashPartitions(t *testing.T) {
	rawSQL, err := os.ReadFile("../../migrations/00015_partition_messages_by_year_and_conversation.sql")
	if err != nil {
		t.Fatalf("read message partition migration: %v", err)
	}
	sql := normalizeSQL(string(rawSQL))

	for _, required := range []string{
		"-- +goose up",
		"create table message_registry",
		"constraint message_registry_conversation_seq_unique unique (conversation_id, seq)",
		"create unique index message_registry_client_message_unique",
		"where client_message_id is not null",
		"create index message_registry_conversation_year_seq_visible_index",
		"partition by range (created_at)",
		"partition by hash (conversation_id)",
		"modulus 32",
		"create function ensure_message_year_partitions",
		"before insert on messages",
		"execute function register_message_partition_row()",
		"after update on messages",
		"execute function sync_message_partition_registry()",
		"-- +goose down",
		"drop table message_registry",
	} {
		if !strings.Contains(sql, required) {
			t.Fatalf("message partition migration missing %q", required)
		}
	}
	for _, removed := range []string{
		"message_partition_catalog",
		"create schema if not exists archive",
		"detach partition",
		"set schema archive",
	} {
		if strings.Contains(sql, removed) {
			t.Fatalf("message partition migration still contains removed archive behavior %q", removed)
		}
	}
}

func TestTemporaryFileExpirationMigrationDefinesTieredRetention(t *testing.T) {
	rawSQL, err := os.ReadFile("../../migrations/00016_add_temporary_file_expiration.sql")
	if err != nil {
		t.Fatalf("read temporary file expiration migration: %v", err)
	}
	sql := normalizeSQL(string(rawSQL))

	for _, required := range []string{
		"-- +goose up",
		"alter table temporary_files add column expires_at timestamptz",
		"update temporary_files set expires_at = created_at + case",
		"when size_bytes > 10485760 then interval '30 days'",
		"else interval '180 days'",
		"alter column expires_at set not null",
		"constraint temporary_files_expires_at_check check (expires_at > created_at)",
		"create index temporary_files_expires_at_index on temporary_files (expires_at)",
		"-- +goose down",
		"drop index temporary_files_expires_at_index",
		"drop constraint temporary_files_expires_at_check",
		"drop column expires_at",
	} {
		if !strings.Contains(sql, required) {
			t.Fatalf("temporary file expiration migration missing %q", required)
		}
	}
}

func normalizeSQL(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(value), " "))
}
