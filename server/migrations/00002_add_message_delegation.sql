-- +goose Up
ALTER TABLE messages
  ADD COLUMN delegated_by_type text,
  ADD COLUMN delegated_by_id uuid,
  ADD COLUMN delegated_by_name text NOT NULL DEFAULT '';

ALTER TABLE messages
  ADD CONSTRAINT messages_delegated_by_type_check CHECK (
    delegated_by_type IS NULL OR delegated_by_type IN ('user', 'app')
  ),
  ADD CONSTRAINT messages_delegated_by_id_check CHECK (
    (delegated_by_type IS NULL AND delegated_by_id IS NULL AND delegated_by_name = '')
    OR (delegated_by_type IS NOT NULL AND delegated_by_id IS NOT NULL AND delegated_by_name <> '')
  );

-- +goose Down
ALTER TABLE messages
  DROP CONSTRAINT messages_delegated_by_id_check,
  DROP CONSTRAINT messages_delegated_by_type_check;

ALTER TABLE messages
  DROP COLUMN delegated_by_name,
  DROP COLUMN delegated_by_id,
  DROP COLUMN delegated_by_type;
