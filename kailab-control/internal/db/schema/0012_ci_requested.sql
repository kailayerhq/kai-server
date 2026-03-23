-- Track CI access requests
ALTER TABLE users ADD COLUMN ci_requested INTEGER NOT NULL DEFAULT 0;
