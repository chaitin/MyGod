-- +goose Up
ALTER TABLE conversation_members
  ADD COLUMN last_mentioned_seq bigint NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE conversation_members
  DROP COLUMN last_mentioned_seq;
