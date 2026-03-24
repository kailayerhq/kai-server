-- Anonymous CLI telemetry events
CREATE TABLE IF NOT EXISTS telemetry_events (
  id           BIGSERIAL PRIMARY KEY,
  event_name   TEXT NOT NULL,
  timestamp    TEXT NOT NULL,
  install_id   TEXT NOT NULL,
  version      TEXT,
  os           TEXT,
  arch         TEXT,
  command      TEXT,
  dur_ms       BIGINT,
  result       TEXT,
  error_class  TEXT,
  stats        JSONB,
  created_at   BIGINT NOT NULL DEFAULT EXTRACT(EPOCH FROM NOW())::BIGINT
);

CREATE INDEX IF NOT EXISTS idx_telemetry_install_id ON telemetry_events(install_id);
CREATE INDEX IF NOT EXISTS idx_telemetry_event_name ON telemetry_events(event_name);
CREATE INDEX IF NOT EXISTS idx_telemetry_created_at ON telemetry_events(created_at);
