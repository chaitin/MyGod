-- +goose Up
CREATE TABLE task_reminders (
  id uuid PRIMARY KEY,
  task_id uuid NOT NULL UNIQUE REFERENCES tasks(id) ON DELETE CASCADE,
  mode text NOT NULL,
  frequency text,
  timezone text NOT NULL,
  once_at timestamptz,
  time_of_day time(0),
  weekdays smallint[] NOT NULL DEFAULT '{}',
  day_of_month smallint,
  next_trigger_at timestamptz,
  last_occurrence_at timestamptz,
  last_processed_at timestamptz,
  last_result text NOT NULL DEFAULT '',
  consecutive_failures integer NOT NULL DEFAULT 0,
  retry_at timestamptz,
  last_error text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL,
  CONSTRAINT task_reminders_mode_check CHECK (mode IN ('once', 'recurring')),
  CONSTRAINT task_reminders_frequency_check CHECK (
    frequency IS NULL OR frequency IN ('daily', 'weekly', 'monthly')
  ),
  CONSTRAINT task_reminders_timezone_check CHECK (char_length(btrim(timezone)) BETWEEN 1 AND 64),
  CONSTRAINT task_reminders_day_of_month_check CHECK (day_of_month IS NULL OR day_of_month BETWEEN 1 AND 31),
  CONSTRAINT task_reminders_weekdays_check CHECK (
    weekdays <@ ARRAY[1, 2, 3, 4, 5, 6, 7]::smallint[]
  ),
  CONSTRAINT task_reminders_schedule_check CHECK (
    (
      mode = 'once'
      AND frequency IS NULL
      AND once_at IS NOT NULL
      AND time_of_day IS NULL
      AND cardinality(weekdays) = 0
      AND day_of_month IS NULL
    )
    OR
    (
      mode = 'recurring'
      AND once_at IS NULL
      AND time_of_day IS NOT NULL
      AND (
        (frequency = 'daily' AND cardinality(weekdays) = 0 AND day_of_month IS NULL)
        OR (frequency = 'weekly' AND cardinality(weekdays) > 0 AND day_of_month IS NULL)
        OR (frequency = 'monthly' AND cardinality(weekdays) = 0 AND day_of_month IS NOT NULL)
      )
    )
  )
);

CREATE INDEX task_reminders_due_index
  ON task_reminders (next_trigger_at, task_id)
  WHERE next_trigger_at IS NOT NULL;

CREATE INDEX task_reminders_retry_index
  ON task_reminders (retry_at)
  WHERE retry_at IS NOT NULL;

-- +goose Down
DROP TABLE task_reminders;
