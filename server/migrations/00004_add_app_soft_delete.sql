-- +goose Up
ALTER TABLE apps
    ADD COLUMN deleted_at timestamptz;

CREATE INDEX apps_deleted_at_index ON apps (deleted_at);

-- +goose Down
DROP INDEX apps_deleted_at_index;

ALTER TABLE apps
    DROP COLUMN deleted_at;
