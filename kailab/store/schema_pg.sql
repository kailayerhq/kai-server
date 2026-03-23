-- Kailab data plane schema for Postgres
-- Uses the existing repos table from kailab-control (shared database)
-- All data plane tables are scoped by repo_id (repos.id UUID)

-- Segments: append-only zstd blobs that contain many objects
CREATE TABLE IF NOT EXISTS segments (
  id BIGSERIAL PRIMARY KEY,
  repo_id TEXT NOT NULL,
  ts BIGINT NOT NULL,
  checksum BYTEA NOT NULL,
  size INTEGER NOT NULL,
  blob BYTEA NOT NULL
);
CREATE INDEX IF NOT EXISTS segments_repo ON segments(repo_id);

-- Objects: index into segments by digest
CREATE TABLE IF NOT EXISTS objects (
  repo_id TEXT NOT NULL,
  digest BYTEA NOT NULL,
  segment_id BIGINT NOT NULL REFERENCES segments(id),
  off BIGINT NOT NULL,
  len BIGINT NOT NULL,
  kind TEXT NOT NULL,
  created_at BIGINT NOT NULL,
  PRIMARY KEY (repo_id, digest)
);
CREATE INDEX IF NOT EXISTS objects_segment ON objects(segment_id);
CREATE INDEX IF NOT EXISTS objects_kind ON objects(repo_id, kind);

-- Refs: moving pointers to Kai nodes
CREATE TABLE IF NOT EXISTS refs (
  repo_id TEXT NOT NULL,
  name TEXT NOT NULL,
  target BYTEA NOT NULL,
  updated_at BIGINT NOT NULL,
  actor TEXT NOT NULL DEFAULT '',
  push_id TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (repo_id, name)
);
CREATE INDEX IF NOT EXISTS refs_target ON refs(repo_id, target);

-- Append-only ref history
CREATE TABLE IF NOT EXISTS ref_history (
  seq BIGSERIAL PRIMARY KEY,
  repo_id TEXT NOT NULL,
  id BYTEA NOT NULL,
  parent BYTEA,
  time BIGINT NOT NULL,
  actor TEXT NOT NULL,
  ref TEXT NOT NULL,
  old BYTEA,
  new BYTEA NOT NULL,
  meta TEXT,
  UNIQUE(repo_id, id)
);
CREATE INDEX IF NOT EXISTS ref_history_ref_time ON ref_history(repo_id, ref, time);

-- Node publish log
CREATE TABLE IF NOT EXISTS node_publish (
  seq BIGSERIAL PRIMARY KEY,
  repo_id TEXT NOT NULL,
  id BYTEA NOT NULL,
  parent BYTEA,
  time BIGINT NOT NULL,
  actor TEXT NOT NULL,
  node_id BYTEA NOT NULL,
  kind TEXT NOT NULL,
  UNIQUE(repo_id, id)
);
CREATE INDEX IF NOT EXISTS node_publish_node_id ON node_publish(repo_id, node_id);

-- Enrichment queue
CREATE TABLE IF NOT EXISTS enrich_queue (
  id BIGSERIAL PRIMARY KEY,
  repo_id TEXT NOT NULL,
  node_id BYTEA NOT NULL,
  kind TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'pending',
  created_at BIGINT NOT NULL,
  started_at BIGINT,
  finished_at BIGINT,
  error TEXT
);
CREATE INDEX IF NOT EXISTS enrich_queue_status ON enrich_queue(repo_id, status);

-- Edges: relationships between nodes
CREATE TABLE IF NOT EXISTS edges (
  repo_id TEXT NOT NULL,
  src BYTEA NOT NULL,
  type TEXT NOT NULL,
  dst BYTEA NOT NULL,
  at BYTEA,
  created_at BIGINT NOT NULL,
  PRIMARY KEY (repo_id, src, type, dst, at)
);
CREATE INDEX IF NOT EXISTS edges_src ON edges(repo_id, src);
CREATE INDEX IF NOT EXISTS edges_dst ON edges(repo_id, dst);
CREATE INDEX IF NOT EXISTS edges_at ON edges(repo_id, at);
