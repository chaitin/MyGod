-- +goose Up
UPDATE apps
SET name = '茉莉', updated_at = now()
WHERE id = '00000000-0000-0000-0000-000000000001'
  AND name IN ('AI 女菩萨', '女菩萨');

-- +goose Down
UPDATE apps
SET name = 'AI 女菩萨', updated_at = now()
WHERE id = '00000000-0000-0000-0000-000000000001'
  AND name = '茉莉';
