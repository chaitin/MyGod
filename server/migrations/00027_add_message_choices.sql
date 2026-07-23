-- +goose Up
ALTER TABLE conversation_members
  ADD COLUMN last_choice_seq bigint NOT NULL DEFAULT 0;

ALTER TABLE conversation_topic_participants
  ADD COLUMN last_choice_seq bigint NOT NULL DEFAULT 0;

CREATE TABLE message_choice_responses (
  id uuid PRIMARY KEY,
  conversation_id uuid NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
  message_id uuid NOT NULL REFERENCES message_registry(id) ON DELETE CASCADE,
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  option_ids jsonb NOT NULL,
  created_at timestamptz NOT NULL,
  CONSTRAINT message_choice_responses_option_ids_array_check
    CHECK (jsonb_typeof(option_ids) = 'array'),
  CONSTRAINT message_choice_responses_message_user_unique
    UNIQUE (message_id, user_id)
);

CREATE INDEX message_choice_responses_conversation_message_index
  ON message_choice_responses (conversation_id, message_id, created_at);

CREATE INDEX message_choice_responses_user_message_index
  ON message_choice_responses (user_id, message_id);

-- +goose Down
DROP TABLE message_choice_responses;

ALTER TABLE conversation_topic_participants
  DROP COLUMN last_choice_seq;

ALTER TABLE conversation_members
  DROP COLUMN last_choice_seq;
