-- CI access flag on users (disabled by default for early access)
ALTER TABLE users ADD COLUMN IF NOT EXISTS ci_access BOOLEAN NOT NULL DEFAULT FALSE;
