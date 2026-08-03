# Deepen the Ingestion → Knowledge Pipeline

**Feature:** Connector-resolution seam, object-store-backed file uploads, a `documents` table, and recorded embedding model.
**Status:** Proposed
**Companion architecture:** `docs/adr/0003`, `docs/adr/0005`

---

## Problem Statement

The ingestion pipeline cannot actually ingest a real DataSource. The Ingestion Workers module selects a Connector from a static `map[string]Connector` keyed by source ID, but that map is empty at the composition root and a Connector is only buildable from per-source configuration and credentials. No module reads those to build a Connector, so file uploads, websites, and CRMs never reach the Knowledge Base.

The pipeline's seams are also inconsistent with how data is stored today:

- Uploaded file bytes are written into the DataSource's encrypted `credentials` column, but the file Connector reads from a `Path` on disk — the two cannot meet.
- The `Chunk` type — a cross-module type — advertises an `Embedding []float32` field that callers never set and the store overwrites internally.
- The Ingestion Workers module declares an `embedder` dependency it never calls; embedding happens inside the store.
- Chunks carry only a flat metadata blob; there is no way to fetch the full document a found chunk came from.

## Solution

Deepen the ingestion → knowledge pipeline around a single Connector-resolution seam and a persistent document model:

- Introduce a **ConnectorResolver** in the Ingestion module that turns a tenant-scoped DataSource into a Connector at ingest time, instead of indexing a static map. The resolver is the single place that maps a SourceType to extraction behaviour.
- Store uploaded file bytes in an **object store** (MinIO/S3), not in the credentials column. The DataSource config references the object; the Connector reads bytes from the object store.
- Keep chained documents in a new **`documents` table**; each Chunk references its source document via `document_id`, so a found chunk can be expanded to its full document with one lookup.
- Record the **embedding model** per chunk in a new `embedding_model` column, so future dimension/model migrations know which vectors to re-embed.
- Keep embedding fully behind the Knowledge Store's seam: drop `Chunk.Embedding` from the shared interface and drop the unused `embedder` from the Ingestion Workers module.

## User Stories

### File uploads survive ingestion

1. As a tenant admin, I want to upload a file (PDF, DOCX, CSV, XLSX, TXT, MD), so that its content becomes part of my tenant's Knowledge Base.
2. As a tenant admin, I want the uploaded file's bytes stored in the object store, so that they are not duplicated in the database and survive worker restarts.
3. As a tenant admin, I want the DataSource config to reference the stored object, so that re-ingestion can read the same bytes without re-uploading.
4. As a tenant admin, I want an uploaded file to be chunked and embedded, so that its chunks are queryable during report generation.
5. As a platform operator, I want object-store keys to be tenant-scoped, so that no tenant can read another tenant's uploaded file.

### Connector resolution

6. As a platform operator, I want the ingestion worker to resolve a Connector from a DataSource at ingest time, so that any configured source can actually be ingested.
7. As a platform operator, I want resolution to be tenant-scoped, so that a worker cannot extract a source that does not belong to the job's tenant.
8. As a platform operator, I want a failed resolution to mark the DataSource as `error` and leave retry to manual action, so that a broken source does not spam the queue.
9. As a platform operator, I want a Connector to be built from the DataSource's config and decrypted credentials, so that CRM and website sources can extract with their stored credentials.

### Full-document retrieval

10. As a report user, I want the report generation context to include the full document behind a matched chunk, so that the LLM can reason over complete context rather than a fragment.
11. As a platform operator, I want chunk documents persisted in a `documents` table, so that retrieval is a single lookup rather than a key-value join.
12. As a platform operator, I want the `documents` table to work for all source types, so that a website or CRM crawl is also retrievable as a full document.

### Embedding model provenance

13. As a platform operator, I want each chunk to record which embedding model produced its vector, so that future model or dimension migrations can identify which rows to re-embed.
14. As a platform operator, I want the embedding model recorded by the store itself, so that the recorded value matches the model actually used.

### Re-ingestion and deduplication

15. As a tenant admin, I want re-ingesting a source to replace that source's previous documents and chunks, so that my Knowledge Base does not accumulate duplicates.
16. As a tenant admin, I want duplicate content within a single ingest run to be collapsed, so that the same content is not stored twice.

### Tenant deletion

17. As a platform operator, I want to delete a tenant's chunks, documents, and object-store objects together, so that a deleted tenant leaves no fragments in the Knowledge Base or the object store.

## Implementation Decisions

### Modules

- **Ingestion module** — build the `ConnectorResolver` seam. It takes a tenant-scoped source reader and an object-store reader (narrow interfaces it defines) and returns a `Connector`. It does **not** import the `sources` package. Remove the unused `embedder` field from the Ingestion Workers module; retain `embeddingModel` for the usage event.
- **Sources module** — change the file-upload DataSource to store bytes in the object store and reference the object in its config. On DataSource delete, best-effort-delete the object via a narrow object-store seam. The `credentials` column remains for CRM sources.
- **Storage module** (new) — an `ObjectStore` interface with a MinIO adapter and an in-memory fake. Two adapters make the seam real. Keys are tenant-prefixed (`tenant/{tenantID}/sources/{sourceID}/file`).
- **Knowledge module** — add a `documents` table and `document_id` on chunks; add an `embedding_model` column; add `GetDocument` to the `KnowledgeStore`; expand `DeleteTenantData` to cascade to documents; drop `Chunk.Embedding` from the shared interface (the store owns embedding internally).

### Interfaces

- `ConnectorResolver` (Ingestion): `Resolve(ctx, tenantID, sourceID) (Connector, error)` — tenant-scoped, returns a Connector built from the DataSource's config and decrypted credentials.
- `SourceReader` (narrow, Ingestion-defined): returns a tenant-scoped DataSource projection; implemented by the Sources module.
- `ObjectStore` (Storage): `Put`, `Get`, `Delete`, all tenant-scoped; MinIO adapter + in-memory fake.
- `Embedder` (Knowledge): add `Model() string` so the store records the model it actually used.
- `KnowledgeStore` (Knowledge): add `GetDocument(ctx, tenantID, documentID) (Document, error)`; `DeleteTenantData` now also removes documents.

### Schema changes (new migration)

- `chunks` gains `embedding_model TEXT NOT NULL` and `document_id TEXT REFERENCES documents(id)`.
- New `documents` table: `id`, `tenant_id`, `source`, `content`, `metadata`, `created_at`, indexed by `(tenant_id, source)`.
- The `data_source_configs.config` JSONB shape for file uploads changes from `{filename, size}` to `{filename, size, object_key}` — no column change needed.

### Directives

- The Connector interface stays (Extract → `[]RawDocument`); `RawDocument` gains an identifier so the chunker can stamp `document_id` on every chunk it produces.
- The file Connector reads bytes from the object store (via the resolver's object-store reader) rather than a `Path`.
- The resolver and worker verify the object key is under the requesting tenant's prefix before reading.
- Failed resolution marks the DataSource `error`; retry is manual (no auto-retry).
- Re-ingestion of a source replaces that source's prior documents and chunks.

## Testing Decisions

- **Seam discipline:** test at the highest existing seam. The primary new seam is `ConnectorResolver` — unit-test it against a fake `SourceReader` and a fake `ObjectStore`, exercising tenant scoping, missing sources, and object-key enforcement.
- **Knowledge Store:** `GetDocument` and `DeleteTenantData`-cascade are tested against the real store (integration, as the existing `store_test.go` does with testcontainers). The `embedding_model` column is asserted via a fake `Embedder` whose `Model()` is recorded.
- **Storage module:** the MinIO adapter is integration-tested; the in-memory fake is unit-tested. The fake is the one used by `ConnectorResolver` tests.
- **Prior art:** mirror `internal/knowledge/store_test.go` (testcontainers integration) and `internal/sources/service_test.go` (mock seams via interfaces). `store_test.go` already constructs `Chunk` without `Embedding`, so dropping the field breaks no existing callers.
- **External behaviour only:** tests assert what is stored and returned, not internal bookkeeping.

## Out of Scope

- Implementing the actual CRM/website Connectors (HubSpot, Salesforce, web crawler) — only the resolution seam and the file connector are built here.
- Changing `Query` to filter by `embedding_model` — the column is recorded for future migrations only.
- The dimension-migration tooling itself (re-embedding a tenant end-to-end) — the `documents` table and `embedding_model` column are the enablers, not the migration.
- PDF/DOCX parsing beyond existing `extensionToDocType` classification.
- Object-store GC / retention policy — best-effort delete on source removal is the primary path; GC is a future safety net.

## Further Notes

- **New glossary terms** to add to `CONTEXT.md`: **Connector** (refine to note it is built by a resolver from a DataSource), **Document** (the full parsed source text behind a chunk), **ObjectStore** (tenant-scoped file storage). These are sharpened during implementation.
- **Dependency direction:** `ingestion` → (narrow interfaces) → `sources`/`storage`; `ingestion` does not import `sources`. `sources` owns the DataSource's real shape; `ingestion` consumes read-only projections.
- **Risk — mixed models:** a tenant re-embedding with a new model produces vectors of a possibly different dimension in the same `vector(1024)` column. This is acknowledged as future work; the `embedding_model` column is the provenance that makes it addressable.
- **Risk — re-ingest replacement:** replacement-on-re-ingest must be atomic per source (delete prior documents + chunks for the source, then insert) to avoid a window of partial state.
- **MinIO** is available in the devcontainer; the Storage module should default to it in development.