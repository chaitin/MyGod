-- +goose Up
CREATE TABLE app_settings (
  id integer PRIMARY KEY,
  app_name text NOT NULL,
  organization_name text NOT NULL,
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL,
  CONSTRAINT app_settings_singleton_check CHECK (id = 1)
);

INSERT INTO app_settings (
  id,
  app_name,
  organization_name,
  created_at,
  updated_at
) VALUES (
  1,
  'MyGod',
  '长亭科技',
  now(),
  now()
);

-- +goose Down
DROP TABLE app_settings;
