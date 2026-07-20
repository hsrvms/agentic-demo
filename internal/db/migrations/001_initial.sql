-- 001_initial.sql
-- Phase 0 schema: knowledge store chunks with pgvector embeddings.

CREATE EXTENSION IF NOT EXISTS vector;

-- Chunks: document segments with vector embeddings.
-- Partitioned by tenant_id for physical data isolation (Phase 1).
CREATE TABLE IF NOT EXISTS chunks (
    id            TEXT PRIMARY KEY,
    tenant_id     TEXT NOT NULL,
    content       TEXT NOT NULL,
    embedding     vector(1024) NOT NULL,
    source        TEXT NOT NULL,
    document_type TEXT NOT NULL DEFAULT 'text',
    date          TIMESTAMPTZ NOT NULL DEFAULT now(),
    metadata      JSONB NOT NULL DEFAULT '{}',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_chunks_tenant_id ON chunks (tenant_id);
CREATE INDEX IF NOT EXISTS idx_chunks_tenant_date ON chunks (tenant_id, date DESC);
CREATE INDEX IF NOT EXISTS idx_chunks_source ON chunks (tenant_id, source);

-- Vector similarity index (IVFFlat, cosine distance).
-- lists = sqrt(num_rows) is a reasonable starting point.
-- Rebuild after significant data changes.
CREATE INDEX IF NOT EXISTS idx_chunks_embedding ON chunks
    USING ivfflat (embedding vector_cosine_ops) WITH (lists = 100);
