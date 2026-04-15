package db

// Schema is the DDL the Phase-4 implementation will apply to a
// freshly-built cache entry. Kept in this file in Phase 1 so the
// Phase-4 agent has a reviewable starting point and so maintainers
// can read the intended schema alongside RFC 0001 §SQLite schema
// (internal contract).
//
// This constant is intentionally unused in Phase 1; it becomes
// load-bearing when the klzcache build tag flips on in Phase 4.
const Schema = `
CREATE TABLE sources (
  id INTEGER PRIMARY KEY,
  source_locale TEXT NOT NULL,
  source_hash TEXT NOT NULL,
  source_text TEXT NOT NULL,
  source_runs TEXT NOT NULL,
  context TEXT,
  block_type TEXT,
  created_at INTEGER NOT NULL,
  UNIQUE (source_locale, source_hash, context)
);

CREATE TABLE targets (
  id INTEGER PRIMARY KEY,
  source_id INTEGER NOT NULL REFERENCES sources(id) ON DELETE CASCADE,
  locale TEXT NOT NULL,
  target_runs TEXT NOT NULL,
  status TEXT NOT NULL CHECK (
    status IN ('new','translated','reviewed','signed-off','rejected')
  ),
  origin TEXT NOT NULL,
  origin_detail TEXT,
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL
);

CREATE UNIQUE INDEX idx_targets_active ON targets(source_id, locale);
CREATE INDEX idx_sources_hash ON sources(source_hash);
CREATE INDEX idx_targets_locale ON targets(locale);

CREATE VIRTUAL TABLE sources_fts USING fts5(
  source_text,
  content='sources',
  content_rowid='id'
);

CREATE TABLE blocks (
  id TEXT PRIMARY KEY,
  document_path TEXT NOT NULL,
  hash TEXT NOT NULL,
  type TEXT NOT NULL,
  component TEXT,
  jsx_path TEXT,
  optional_placeholders INTEGER NOT NULL DEFAULT 0,
  required_placeholders INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX idx_blocks_document ON blocks(document_path);
CREATE INDEX idx_blocks_hash ON blocks(hash);
CREATE INDEX idx_blocks_component ON blocks(component);

CREATE TABLE source_hashes (
  source_hash TEXT PRIMARY KEY,
  block_ids TEXT NOT NULL
);

CREATE TABLE cache_meta (
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL
);
`
