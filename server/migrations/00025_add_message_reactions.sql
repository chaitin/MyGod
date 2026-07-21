-- +goose Up
CREATE TABLE message_reactions (
  message_id uuid NOT NULL REFERENCES message_registry(id) ON DELETE CASCADE,
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  text varchar(128) NOT NULL,
  created_at timestamptz NOT NULL,
  PRIMARY KEY (message_id, user_id, text)
);

CREATE INDEX message_reactions_message_text_index
  ON message_reactions (message_id, text, created_at);

CREATE INDEX message_reactions_user_message_index
  ON message_reactions (user_id, message_id);

CREATE TABLE message_reaction_states (
  message_id uuid PRIMARY KEY REFERENCES message_registry(id) ON DELETE CASCADE,
  version bigint NOT NULL DEFAULT 0,
  updated_at timestamptz NOT NULL
);

-- +goose Down
DROP TABLE message_reaction_states;
DROP TABLE message_reactions;
