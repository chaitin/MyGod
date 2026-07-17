-- +goose Up
ALTER TABLE app_settings
  ALTER COLUMN smtp_port SET DEFAULT 465,
  ALTER COLUMN smtp_security SET DEFAULT 'tls';

UPDATE app_settings
SET smtp_port = 465,
    smtp_security = 'tls'
WHERE email_code_login_enabled = false
  AND smtp_host = ''
  AND smtp_username = ''
  AND smtp_password = ''
  AND smtp_from_email = ''
  AND smtp_from_name = ''
  AND smtp_port = 587
  AND smtp_security = 'starttls';

-- +goose Down
UPDATE app_settings
SET smtp_port = 587,
    smtp_security = 'starttls'
WHERE email_code_login_enabled = false
  AND smtp_host = ''
  AND smtp_username = ''
  AND smtp_password = ''
  AND smtp_from_email = ''
  AND smtp_from_name = ''
  AND smtp_port = 465
  AND smtp_security = 'tls';

ALTER TABLE app_settings
  ALTER COLUMN smtp_port SET DEFAULT 587,
  ALTER COLUMN smtp_security SET DEFAULT 'starttls';
