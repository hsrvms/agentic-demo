-- Chunk queries for the knowledge store.
-- All queries are tenant-scoped — tenant_id is always required.

-- name: InsertChunk :exec
INSERT INTO chunks (id, tenant_id, content, embedding, source, document_type, date, metadata)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (id) DO UPDATE SET
    content   = EXCLUDED.content,
    embedding = EXCLUDED.embedding,
    metadata  = EXCLUDED.metadata,
    date      = EXCLUDED.date;

-- name: QueryChunks :many
SELECT c.id,
       c.content,
       c.source,
       c.document_type,
       c.date,
       c.metadata,
       (c.embedding <=> $2)::float8 AS distance
FROM chunks c
WHERE c.tenant_id = $1
  AND (sqlc.narg('source')::text IS NULL        OR c.source = sqlc.narg('source'))
  AND (sqlc.narg('date_from')::timestamptz IS NULL OR c.date >= sqlc.narg('date_from'))
  AND (sqlc.narg('date_to')::timestamptz IS NULL   OR c.date <= sqlc.narg('date_to'))
ORDER BY c.embedding <=> $2
LIMIT $3;

-- name: DeleteTenantChunks :exec
DELETE FROM chunks WHERE tenant_id = $1;

-- name: GetChunkStats :many
SELECT source, COUNT(*) AS chunk_count
FROM chunks
WHERE tenant_id = $1
GROUP BY source;