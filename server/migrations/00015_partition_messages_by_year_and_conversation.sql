-- +goose Up
CREATE TABLE message_registry (
  id uuid PRIMARY KEY,
  conversation_id uuid NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
  seq bigint NOT NULL,
  sender_type text NOT NULL,
  sender_id uuid,
  client_message_id text,
  reply_to_message_id uuid,
  created_at timestamptz NOT NULL,
  partition_year smallint NOT NULL,
  summary text NOT NULL DEFAULT '',
  revoked_at timestamptz,
  revoked_by_user_id uuid REFERENCES users(id) ON DELETE SET NULL,
  deleted_at timestamptz,
  CONSTRAINT message_registry_conversation_seq_unique UNIQUE (conversation_id, seq),
  CONSTRAINT message_registry_sender_type_check CHECK (sender_type IN ('user', 'app', 'system')),
  CONSTRAINT message_registry_sender_id_check CHECK (
    (sender_type = 'system' AND sender_id IS NULL)
    OR (sender_type <> 'system' AND sender_id IS NOT NULL)
  ),
  CONSTRAINT message_registry_partition_year_check CHECK (partition_year BETWEEN 1970 AND 9999)
);

CREATE UNIQUE INDEX message_registry_client_message_unique
  ON message_registry (conversation_id, sender_type, sender_id, client_message_id)
  WHERE client_message_id IS NOT NULL;

CREATE INDEX message_registry_conversation_seq_visible_index
  ON message_registry (conversation_id, seq DESC)
  WHERE deleted_at IS NULL;

CREATE INDEX message_registry_conversation_year_seq_visible_index
  ON message_registry (conversation_id, partition_year DESC, seq DESC)
  WHERE deleted_at IS NULL;

CREATE INDEX message_registry_reply_to_message_id_index
  ON message_registry (reply_to_message_id)
  WHERE reply_to_message_id IS NOT NULL;

INSERT INTO message_registry (
  id,
  conversation_id,
  seq,
  sender_type,
  sender_id,
  client_message_id,
  reply_to_message_id,
  created_at,
  partition_year,
  summary,
  revoked_at,
  revoked_by_user_id,
  deleted_at
)
SELECT
  id,
  conversation_id,
  seq,
  sender_type,
  sender_id,
  client_message_id,
  reply_to_message_id,
  created_at,
  EXTRACT(YEAR FROM created_at AT TIME ZONE 'UTC')::smallint,
  summary,
  revoked_at,
  revoked_by_user_id,
  deleted_at
FROM messages
ORDER BY conversation_id, seq;

ALTER TABLE message_registry
  ADD CONSTRAINT message_registry_reply_to_message_id_fkey
  FOREIGN KEY (reply_to_message_id) REFERENCES message_registry(id) ON DELETE SET NULL;

ALTER TABLE messages
  DROP CONSTRAINT messages_reply_to_message_id_fkey;

ALTER TABLE messages RENAME TO messages_unpartitioned;

CREATE TABLE messages (
  id uuid NOT NULL REFERENCES message_registry(id) ON DELETE CASCADE,
  conversation_id uuid NOT NULL,
  seq bigint NOT NULL,
  sender_type text NOT NULL,
  sender_id uuid,
  client_message_id text,
  delegated_by_type text,
  delegated_by_id uuid,
  delegated_by_name text NOT NULL DEFAULT '',
  reply_to_message_id uuid,
  body jsonb NOT NULL,
  summary text NOT NULL DEFAULT '',
  revoked_at timestamptz,
  revoked_by_user_id uuid REFERENCES users(id) ON DELETE SET NULL,
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL,
  deleted_at timestamptz,
  CONSTRAINT messages_partitioned_primary_key PRIMARY KEY (created_at, conversation_id, id),
  CONSTRAINT messages_partitioned_sender_type_check CHECK (sender_type IN ('user', 'app', 'system')),
  CONSTRAINT messages_partitioned_sender_id_check CHECK (
    (sender_type = 'system' AND sender_id IS NULL)
    OR (sender_type <> 'system' AND sender_id IS NOT NULL)
  ),
  CONSTRAINT messages_partitioned_delegated_by_type_check CHECK (
    delegated_by_type IS NULL OR delegated_by_type IN ('user', 'app')
  ),
  CONSTRAINT messages_partitioned_delegated_by_id_check CHECK (
    (delegated_by_type IS NULL AND delegated_by_id IS NULL AND delegated_by_name = '')
    OR (delegated_by_type IS NOT NULL AND delegated_by_id IS NOT NULL AND delegated_by_name <> '')
  ),
  CONSTRAINT messages_partitioned_body_object_check CHECK (jsonb_typeof(body) = 'object')
) PARTITION BY RANGE (created_at);

CREATE INDEX messages_partitioned_conversation_seq_visible_index
  ON messages (conversation_id, seq DESC)
  WHERE deleted_at IS NULL;

CREATE INDEX messages_partitioned_conversation_id_index
  ON messages (conversation_id, id);

CREATE INDEX messages_partitioned_reply_to_message_id_index
  ON messages (conversation_id, reply_to_message_id)
  WHERE reply_to_message_id IS NOT NULL;

CREATE INDEX messages_partitioned_revoked_at_index
  ON messages (revoked_at)
  WHERE revoked_at IS NOT NULL;

-- +goose StatementBegin
CREATE FUNCTION ensure_message_year_partitions(requested_year integer)
RETURNS void
LANGUAGE plpgsql
AS $$
DECLARE
  year_start timestamptz;
  year_end timestamptz;
  year_table text;
  hash_table text;
  remainder integer;
BEGIN
  IF requested_year < 1970 OR requested_year > 9999 THEN
    RAISE EXCEPTION 'message partition year % is outside the supported range', requested_year;
  END IF;

  year_start := make_timestamptz(requested_year, 1, 1, 0, 0, 0, 'UTC');
  year_end := make_timestamptz(requested_year + 1, 1, 1, 0, 0, 0, 'UTC');
  year_table := format('messages_%s', requested_year);

  EXECUTE format(
    'CREATE TABLE IF NOT EXISTS %I PARTITION OF messages FOR VALUES FROM (%L) TO (%L) PARTITION BY HASH (conversation_id)',
    year_table,
    year_start,
    year_end
  );

  FOR remainder IN 0..31 LOOP
    hash_table := format('%s_p%s', year_table, lpad(remainder::text, 2, '0'));
    EXECUTE format(
      'CREATE TABLE IF NOT EXISTS %I PARTITION OF %I FOR VALUES WITH (MODULUS 32, REMAINDER %s)',
      hash_table,
      year_table,
      remainder
    );
  END LOOP;

END;
$$;
-- +goose StatementEnd

-- +goose StatementBegin
DO $$
DECLARE
  partition_year integer;
  current_year integer := EXTRACT(YEAR FROM now() AT TIME ZONE 'UTC')::integer;
BEGIN
  FOR partition_year IN
    SELECT DISTINCT EXTRACT(YEAR FROM created_at AT TIME ZONE 'UTC')::integer
    FROM messages_unpartitioned
  LOOP
    PERFORM ensure_message_year_partitions(partition_year);
  END LOOP;

  FOR partition_year IN current_year - 1..current_year + 2 LOOP
    PERFORM ensure_message_year_partitions(partition_year);
  END LOOP;
END;
$$;
-- +goose StatementEnd

INSERT INTO messages (
  id,
  conversation_id,
  seq,
  sender_type,
  sender_id,
  client_message_id,
  delegated_by_type,
  delegated_by_id,
  delegated_by_name,
  reply_to_message_id,
  body,
  summary,
  revoked_at,
  revoked_by_user_id,
  created_at,
  updated_at,
  deleted_at
)
SELECT
  id,
  conversation_id,
  seq,
  sender_type,
  sender_id,
  client_message_id,
  delegated_by_type,
  delegated_by_id,
  delegated_by_name,
  reply_to_message_id,
  body,
  summary,
  revoked_at,
  revoked_by_user_id,
  created_at,
  updated_at,
  deleted_at
FROM messages_unpartitioned
ORDER BY created_at, conversation_id, seq;

DROP TABLE messages_unpartitioned;

-- +goose StatementBegin
CREATE FUNCTION register_message_partition_row()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
  INSERT INTO message_registry (
    id,
    conversation_id,
    seq,
    sender_type,
    sender_id,
    client_message_id,
    reply_to_message_id,
    created_at,
    partition_year,
    summary,
    revoked_at,
    revoked_by_user_id,
    deleted_at
  ) VALUES (
    NEW.id,
    NEW.conversation_id,
    NEW.seq,
    NEW.sender_type,
    NEW.sender_id,
    NEW.client_message_id,
    NEW.reply_to_message_id,
    NEW.created_at,
    EXTRACT(YEAR FROM NEW.created_at AT TIME ZONE 'UTC')::smallint,
    NEW.summary,
    NEW.revoked_at,
    NEW.revoked_by_user_id,
    NEW.deleted_at
  );
  RETURN NEW;
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER messages_register_partition_row
BEFORE INSERT ON messages
FOR EACH ROW
EXECUTE FUNCTION register_message_partition_row();

-- +goose StatementBegin
CREATE FUNCTION sync_message_partition_registry()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
  IF NEW.id IS DISTINCT FROM OLD.id
    OR NEW.conversation_id IS DISTINCT FROM OLD.conversation_id
    OR NEW.seq IS DISTINCT FROM OLD.seq
    OR NEW.sender_type IS DISTINCT FROM OLD.sender_type
    OR NEW.sender_id IS DISTINCT FROM OLD.sender_id
    OR NEW.client_message_id IS DISTINCT FROM OLD.client_message_id
    OR NEW.created_at IS DISTINCT FROM OLD.created_at THEN
    RAISE EXCEPTION 'message identity and partition fields are immutable';
  END IF;

  UPDATE message_registry
  SET
    reply_to_message_id = NEW.reply_to_message_id,
    summary = NEW.summary,
    revoked_at = NEW.revoked_at,
    revoked_by_user_id = NEW.revoked_by_user_id,
    deleted_at = NEW.deleted_at
  WHERE id = NEW.id;

  RETURN NEW;
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER messages_sync_partition_registry
AFTER UPDATE ON messages
FOR EACH ROW
EXECUTE FUNCTION sync_message_partition_registry();

-- +goose Down
DROP TRIGGER messages_sync_partition_registry ON messages;
DROP TRIGGER messages_register_partition_row ON messages;
DROP FUNCTION sync_message_partition_registry();
DROP FUNCTION register_message_partition_row();

ALTER TABLE messages RENAME TO messages_partitioned_old;

CREATE TABLE messages (
  id uuid PRIMARY KEY,
  conversation_id uuid NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
  seq bigint NOT NULL,
  sender_type text NOT NULL,
  sender_id uuid,
  client_message_id text,
  delegated_by_type text,
  delegated_by_id uuid,
  delegated_by_name text NOT NULL DEFAULT '',
  reply_to_message_id uuid,
  body jsonb NOT NULL,
  summary text NOT NULL DEFAULT '',
  revoked_at timestamptz,
  revoked_by_user_id uuid REFERENCES users(id) ON DELETE SET NULL,
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
  CONSTRAINT messages_delegated_by_type_check CHECK (
    delegated_by_type IS NULL OR delegated_by_type IN ('user', 'app')
  ),
  CONSTRAINT messages_delegated_by_id_check CHECK (
    (delegated_by_type IS NULL AND delegated_by_id IS NULL AND delegated_by_name = '')
    OR (delegated_by_type IS NOT NULL AND delegated_by_id IS NOT NULL AND delegated_by_name <> '')
  ),
  CONSTRAINT messages_body_object_check CHECK (jsonb_typeof(body) = 'object')
);

INSERT INTO messages (
  id,
  conversation_id,
  seq,
  sender_type,
  sender_id,
  client_message_id,
  delegated_by_type,
  delegated_by_id,
  delegated_by_name,
  reply_to_message_id,
  body,
  summary,
  revoked_at,
  revoked_by_user_id,
  created_at,
  updated_at,
  deleted_at
)
SELECT
  id,
  conversation_id,
  seq,
  sender_type,
  sender_id,
  client_message_id,
  delegated_by_type,
  delegated_by_id,
  delegated_by_name,
  reply_to_message_id,
  body,
  summary,
  revoked_at,
  revoked_by_user_id,
  created_at,
  updated_at,
  deleted_at
FROM messages_partitioned_old
ORDER BY conversation_id, seq;

ALTER TABLE messages
  ADD CONSTRAINT messages_reply_to_message_id_fkey
  FOREIGN KEY (reply_to_message_id) REFERENCES messages(id) ON DELETE SET NULL;

CREATE INDEX messages_conversation_seq_index ON messages (conversation_id, seq DESC);
CREATE INDEX messages_created_at_index ON messages (created_at);
CREATE INDEX messages_reply_to_message_id_index ON messages (reply_to_message_id);
CREATE INDEX messages_revoked_at_index ON messages (revoked_at);

DROP TABLE messages_partitioned_old CASCADE;
DROP FUNCTION ensure_message_year_partitions(integer);
DROP TABLE message_registry;
