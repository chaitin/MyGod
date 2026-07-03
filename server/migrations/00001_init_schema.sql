-- +goose Up
CREATE TABLE users (
  id uuid PRIMARY KEY,
  email text NOT NULL,
  name text NOT NULL,
  nickname text NOT NULL DEFAULT '',
  phone text,
  avatar text NOT NULL DEFAULT '/assets/avatars/builtin/01.webp',
  password_hash text NOT NULL,
  status text NOT NULL,
  last_online_at timestamptz,
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL,
  CONSTRAINT users_status_check CHECK (status IN ('active', 'disabled'))
);

CREATE UNIQUE INDEX users_email_lower_unique ON users (lower(email));
CREATE UNIQUE INDEX users_phone_unique ON users (phone) WHERE phone IS NOT NULL;
CREATE INDEX users_status_index ON users (status);

CREATE TABLE admin_sessions (
  id uuid PRIMARY KEY,
  token_hash text NOT NULL,
  expires_at timestamptz NOT NULL,
  created_at timestamptz NOT NULL,
  last_seen_at timestamptz NOT NULL,
  user_agent text NOT NULL DEFAULT '',
  ip text NOT NULL DEFAULT '',
  CONSTRAINT admin_sessions_token_hash_unique UNIQUE (token_hash)
);

CREATE INDEX admin_sessions_expires_at_index ON admin_sessions (expires_at);

CREATE TABLE user_sessions (
  id uuid PRIMARY KEY,
  token_hash text NOT NULL,
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  expires_at timestamptz NOT NULL,
  created_at timestamptz NOT NULL,
  last_seen_at timestamptz NOT NULL,
  user_agent text NOT NULL DEFAULT '',
  ip text NOT NULL DEFAULT '',
  CONSTRAINT user_sessions_token_hash_unique UNIQUE (token_hash)
);

CREATE INDEX user_sessions_user_id_index ON user_sessions (user_id);
CREATE INDEX user_sessions_expires_at_index ON user_sessions (expires_at);

CREATE TABLE app_settings (
  id integer PRIMARY KEY,
  app_name text NOT NULL,
  organization_name text NOT NULL,
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL,
  CONSTRAINT app_settings_singleton_check CHECK (id = 1)
);

INSERT INTO app_settings (
  id,
  app_name,
  organization_name,
  created_at,
  updated_at
) VALUES (
  1,
  'MyGod',
  '长亭科技',
  now(),
  now()
);

CREATE TABLE conversations (
  id uuid PRIMARY KEY,
  kind text NOT NULL,
  name text NOT NULL DEFAULT '',
  created_by_user_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  status text NOT NULL DEFAULT 'active',
  posting_policy text NOT NULL DEFAULT 'open',
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL,
  dissolved_at timestamptz,
  last_message_id uuid,
  last_message_seq bigint NOT NULL DEFAULT 0,
  last_message_summary text NOT NULL DEFAULT '',
  last_message_at timestamptz,
  CONSTRAINT conversations_kind_check CHECK (kind IN ('direct', 'group', 'app')),
  CONSTRAINT conversations_status_check CHECK (status IN ('active', 'dissolved')),
  CONSTRAINT conversations_posting_policy_check CHECK (posting_policy IN ('open', 'muted'))
);

CREATE INDEX conversations_kind_updated_index ON conversations (kind, updated_at DESC);
CREATE INDEX conversations_created_by_user_id_index ON conversations (created_by_user_id);
CREATE INDEX conversations_last_message_at_index ON conversations (last_message_at);

CREATE TABLE conversation_members (
  conversation_id uuid NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
  member_type text NOT NULL,
  member_id uuid NOT NULL,
  user_member_id uuid GENERATED ALWAYS AS (
    CASE WHEN member_type = 'user' THEN member_id ELSE NULL END
  ) STORED REFERENCES users(id) ON DELETE RESTRICT,
  role text NOT NULL DEFAULT 'member',
  joined_at timestamptz NOT NULL,
  history_visible_from_seq bigint NOT NULL DEFAULT 1,
  left_at timestamptz,
  last_read_message_id uuid,
  PRIMARY KEY (conversation_id, member_type, member_id),
  CONSTRAINT conversation_members_member_type_check CHECK (member_type IN ('user', 'app')),
  CONSTRAINT conversation_members_role_check CHECK (role IN ('owner', 'admin', 'member')),
  CONSTRAINT conversation_members_history_visible_from_seq_check CHECK (history_visible_from_seq >= 1)
);

CREATE INDEX conversation_members_member_index
  ON conversation_members (member_type, member_id, left_at);

CREATE UNIQUE INDEX conversation_members_one_owner_per_conversation
  ON conversation_members (conversation_id)
  WHERE role = 'owner' AND left_at IS NULL;

CREATE TABLE messages (
  id uuid PRIMARY KEY,
  conversation_id uuid NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
  seq bigint NOT NULL,
  sender_type text NOT NULL,
  sender_id uuid,
  client_message_id text,
  body jsonb NOT NULL,
  summary text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL,
  deleted_at timestamptz,
  CONSTRAINT messages_conversation_seq_unique UNIQUE (conversation_id, seq),
  CONSTRAINT messages_client_message_unique UNIQUE (conversation_id, sender_type, sender_id, client_message_id),
  CONSTRAINT messages_sender_type_check CHECK (sender_type IN ('user', 'app', 'system')),
  CONSTRAINT messages_sender_id_check CHECK (
    (sender_type = 'system' AND sender_id IS NULL)
    OR (sender_type <> 'system' AND sender_id IS NOT NULL)
  ),
  CONSTRAINT messages_body_object_check CHECK (jsonb_typeof(body) = 'object')
);

CREATE INDEX messages_conversation_seq_index ON messages (conversation_id, seq DESC);
CREATE INDEX messages_created_at_index ON messages (created_at);

CREATE TABLE direct_conversations (
  conversation_id uuid PRIMARY KEY REFERENCES conversations(id) ON DELETE CASCADE,
  user_low_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  user_high_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  created_at timestamptz NOT NULL,
  CONSTRAINT direct_conversations_user_pair_unique UNIQUE (user_low_id, user_high_id),
  CONSTRAINT direct_conversations_user_order_check CHECK (user_low_id < user_high_id)
);

-- +goose Down
DROP TABLE direct_conversations;
DROP TABLE messages;
DROP TABLE conversation_members;
DROP TABLE conversations;
DROP TABLE app_settings;
DROP TABLE user_sessions;
DROP TABLE admin_sessions;
DROP TABLE users;
