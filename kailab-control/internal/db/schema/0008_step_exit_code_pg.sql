-- Add exit_code to steps table
ALTER TABLE steps ADD COLUMN IF NOT EXISTS exit_code INTEGER DEFAULT NULL;
