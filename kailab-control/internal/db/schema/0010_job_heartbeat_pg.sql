-- Add heartbeat tracking to jobs for orphan detection
ALTER TABLE jobs ADD COLUMN IF NOT EXISTS last_heartbeat_at BIGINT;
