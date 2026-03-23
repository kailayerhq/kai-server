-- CI access flag on users (disabled by default for early access)
ALTER TABLE users ADD COLUMN ci_access INTEGER NOT NULL DEFAULT 0;
