-- +goose Up
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
  last_message_at timestamptz,
  CONSTRAINT conversations_kind_check CHECK (kind IN ('direct', 'group', 'assistant')),
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
  left_at timestamptz,
  last_read_message_id uuid,
  PRIMARY KEY (conversation_id, member_type, member_id),
  CONSTRAINT conversation_members_member_type_check CHECK (member_type IN ('user', 'assistant')),
  CONSTRAINT conversation_members_role_check CHECK (role IN ('owner', 'admin', 'member'))
);

CREATE INDEX conversation_members_member_index
  ON conversation_members (member_type, member_id, left_at);

CREATE UNIQUE INDEX conversation_members_one_owner_per_conversation
  ON conversation_members (conversation_id)
  WHERE role = 'owner' AND left_at IS NULL;

-- +goose Down
DROP TABLE conversation_members;
DROP TABLE conversations;
