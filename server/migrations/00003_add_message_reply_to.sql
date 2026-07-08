-- +goose Up
ALTER TABLE messages ADD COLUMN reply_to_message_id uuid;

CREATE INDEX messages_reply_to_message_id_index ON messages (reply_to_message_id);

ALTER TABLE messages
  ADD CONSTRAINT messages_reply_to_message_id_fkey
  FOREIGN KEY (reply_to_message_id) REFERENCES messages(id) ON DELETE SET NULL;

-- +goose Down
ALTER TABLE messages
  DROP CONSTRAINT messages_reply_to_message_id_fkey;

DROP INDEX messages_reply_to_message_id_index;

ALTER TABLE messages
  DROP COLUMN reply_to_message_id;
