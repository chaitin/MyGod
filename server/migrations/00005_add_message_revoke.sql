-- +goose Up
ALTER TABLE messages
  ADD COLUMN revoked_at timestamptz,
  ADD COLUMN revoked_by_user_id uuid;

CREATE INDEX messages_revoked_at_index ON messages (revoked_at);

ALTER TABLE messages
  ADD CONSTRAINT messages_revoked_by_user_id_fkey
  FOREIGN KEY (revoked_by_user_id) REFERENCES users(id) ON DELETE SET NULL;

-- +goose Down
ALTER TABLE messages
  DROP CONSTRAINT messages_revoked_by_user_id_fkey;

DROP INDEX messages_revoked_at_index;

ALTER TABLE messages
  DROP COLUMN revoked_by_user_id,
  DROP COLUMN revoked_at;
