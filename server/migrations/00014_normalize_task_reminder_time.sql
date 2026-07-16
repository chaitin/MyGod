-- +goose Up
ALTER TABLE task_reminders
  ALTER COLUMN time_of_day TYPE varchar(5)
  USING to_char(time_of_day, 'HH24:MI');

ALTER TABLE task_reminders
  ADD CONSTRAINT task_reminders_time_of_day_check CHECK (
    time_of_day IS NULL OR time_of_day ~ '^([01][0-9]|2[0-3]):[0-5][0-9]$'
  );

UPDATE task_reminders
SET timezone = 'Asia/Shanghai';

-- +goose Down
ALTER TABLE task_reminders
  DROP CONSTRAINT task_reminders_time_of_day_check;

ALTER TABLE task_reminders
  ALTER COLUMN time_of_day TYPE time(0)
  USING time_of_day::time(0);
