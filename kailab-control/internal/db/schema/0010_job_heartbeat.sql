-- Add heartbeat tracking to jobs for orphan detection
ALTER TABLE jobs ADD COLUMN last_heartbeat_at BIGINT;
