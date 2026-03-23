-- Early access signups
CREATE TABLE IF NOT EXISTS signups (
  id           TEXT PRIMARY KEY,
  name         TEXT NOT NULL,
  email        TEXT NOT NULL,
  company      TEXT,
  repo_url     TEXT,
  ai_usage     TEXT,
  status       TEXT NOT NULL DEFAULT 'pending_review',
  notes        TEXT,
  submitted_at BIGINT NOT NULL,
  updated_at   BIGINT
);
CREATE INDEX IF NOT EXISTS signups_email ON signups(email);
CREATE INDEX IF NOT EXISTS signups_status ON signups(status);
