-- Document queries for the knowledge store.
-- All queries are tenant-scoped — tenant_id is always required.

-- name: InsertDocument :exec
INSERT INTO documents (id, tenant_id, source, content, metadata)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (id) DO UPDATE SET
    content  = EXCLUDED.content,
    metadata = EXCLUDED.metadata;

-- name: DeleteSourceDocuments :exec
DELETE FROM documents WHERE tenant_id = $1 AND source = $2;

-- name: GetDocument :one
SELECT id, tenant_id, source, content, metadata, created_at
FROM documents
WHERE id = $1 AND tenant_id = $2;

-- name: DeleteTenantDocuments :exec
DELETE FROM documents WHERE tenant_id = $1;