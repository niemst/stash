-- Initial schema for store
BEGIN;

-- Records table
CREATE TABLE IF NOT EXISTS records (
    _row_id BIGSERIAL,
    id TEXT PRIMARY KEY,
    content TEXT NOT NULL DEFAULT '',
    metadata JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ NULL
);

-- Vectors table (supports multiple named vectors per record)
CREATE TABLE IF NOT EXISTS record_vectors (
    record_id TEXT NOT NULL REFERENCES records(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    model TEXT NOT NULL,
    vector vector NOT NULL, -- dimension set by store configuration
    PRIMARY KEY (record_id, name)
);

-- Indexes
CREATE INDEX IF NOT EXISTS idx_records_row_id ON records(_row_id);
CREATE INDEX IF NOT EXISTS idx_records_deleted_at ON records(deleted_at);
CREATE INDEX IF NOT EXISTS idx_records_created_at ON records(created_at);
CREATE INDEX IF NOT EXISTS idx_records_updated_at ON records(updated_at);

-- GIN index for JSONB queries
CREATE INDEX IF NOT EXISTS idx_records_metadata_gin ON records USING GIN (metadata);

-- Full-text search index
CREATE INDEX IF NOT EXISTS idx_records_content_tsvector ON records USING GIN (to_tsvector('english', content));

-- Schema version tracking
CREATE TABLE IF NOT EXISTS schema_version (
    version INTEGER PRIMARY KEY,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO schema_version (version) VALUES (1)
ON CONFLICT (version) DO NOTHING;

COMMIT;