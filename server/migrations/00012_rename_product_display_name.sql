-- +goose Up
UPDATE app_settings
SET app_name = '即应', updated_at = now()
WHERE id = 1
  AND app_name = 'MyGod';

-- +goose Down
UPDATE app_settings
SET app_name = 'MyGod', updated_at = now()
WHERE id = 1
  AND app_name = '即应';
