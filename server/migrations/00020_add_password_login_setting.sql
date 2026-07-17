-- +goose Up
ALTER TABLE app_settings
  ADD COLUMN password_login_enabled boolean NOT NULL DEFAULT true;

-- +goose Down
ALTER TABLE app_settings
  DROP COLUMN password_login_enabled;
