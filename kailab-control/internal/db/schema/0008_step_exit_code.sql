-- Add exit_code to steps table
ALTER TABLE steps ADD COLUMN exit_code INTEGER DEFAULT NULL;
