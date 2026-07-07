-- +goose Up
CREATE TABLE apps (
  id uuid PRIMARY KEY,
  name text NOT NULL,
  avatar text NOT NULL DEFAULT '',
  description text NOT NULL DEFAULT '',
  creator_user_id uuid REFERENCES users(id) ON DELETE SET NULL,
  enabled boolean NOT NULL DEFAULT true,
  visibility text NOT NULL,
  callback_url text NOT NULL DEFAULT '',
  callback_secret text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT apps_visibility_check CHECK (visibility IN ('creator', 'public')),
  CONSTRAINT apps_callback_secret_unique UNIQUE (callback_secret)
);

CREATE INDEX apps_creator_user_id_index ON apps (creator_user_id);
CREATE INDEX apps_enabled_index ON apps (enabled);
CREATE INDEX apps_visibility_index ON apps (visibility);

CREATE TABLE app_conversations (
  app_id uuid NOT NULL REFERENCES apps(id) ON DELETE CASCADE,
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  conversation_id uuid NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (app_id, user_id),
  CONSTRAINT app_conversations_conversation_unique UNIQUE (conversation_id)
);

CREATE INDEX app_conversations_user_id_index ON app_conversations (user_id);

-- +goose Down
DROP TABLE app_conversations;
DROP TABLE apps;
