-- Chunk queries for the knowledge store.
-- All queries are tenant-scoped — tenant_id is always required.

-- name: GetChunkEmbeddingModel :one
SELECT embedding_model
FROM chunks
WHERE tenant_id = $1 AND id = $2;

-- name: InsertChunk :exec
INSERT INTO chunks (id, tenant_id, content, embedding, source, document_type, date, metadata, document_id, embedding_model)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
ON CONFLICT (id) DO UPDATE SET
    content         = EXCLUDED.content,
    embedding       = EXCLUDED.embedding,
    metadata        = EXCLUDED.metadata,
    date            = EXCLUDED.date,
    document_id     = EXCLUDED.document_id,
    embedding_model = EXCLUDED.embedding_model;

-- name: QueryChunks :many
SELECT c.id,
       c.content,
       c.source,
       c.document_type,
       c.date,
       c.metadata,
       c.document_id,
       (c.embedding <=> $2)::float8 AS distance
FROM chunks c
WHERE c.tenant_id = $1
  AND (sqlc.narg('source')::text IS NULL        OR c.source = sqlc.narg('source'))
  AND (sqlc.narg('date_from')::timestamptz IS NULL OR c.date >= sqlc.narg('date_from'))
  AND (sqlc.narg('date_to')::timestamptz IS NULL   OR c.date <= sqlc.narg('date_to'))
ORDER BY c.embedding <=> $2
LIMIT $3;

-- name: GetChunkStats :many
SELECT source, COUNT(*) AS chunk_count
FROM chunks
WHERE tenant_id = $1
GROUP BY source;