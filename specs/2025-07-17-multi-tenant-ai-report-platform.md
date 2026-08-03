# Multi-Tenant AI Report Platform — Architecture Spec (Go)

> **Companion spec:** A Python-stack version of this document exists at [`2025-07-17-multi-tenant-ai-report-platform-python.md`](2025-07-17-multi-tenant-ai-report-platform-python.md). The two are identical in user stories, module interfaces, agent loop design, isolation model, usage tracking, and build phases. They differ only in the technology stack.

## Problem Statement

Companies need strategic decision support — daily, weekly, and monthly reports derived from their entire data universe (CRM, ERP, databases, social media, websites, uploaded files). Building this in-house requires AI expertise, data engineering, and infrastructure that most companies cannot justify. There is no turnkey platform that ingests a company's data from all these sources, reasons over it with AI, and produces actionable strategic reports.

## Solution

A multi-tenant platform where a company signs up, connects their data sources, and receives AI-generated strategic reports on a schedule. The platform handles ingestion, knowledge base construction, AI reasoning with tool use, and report delivery. Companies interact through a web UI to configure data sources, set report schedules, and view generated reports.

The platform is designed to be built and operated by a small engineering team.

## User Stories

### Platform Administration

1. As a platform operator, I want to create a new tenant, so that a company can start using the platform.
2. As a platform operator, I want to suspend a tenant, so that a non-paying or abusive company stops consuming resources.
3. As a platform operator, I want to delete a tenant and all their data, so that I comply with data erasure requests.
4. As a platform operator, I want to view per-tenant token consumption, so that I can bill accurately and detect anomalies.
5. As a platform operator, I want to configure which LLM providers are available and their fallback order, so that I can manage cost and reliability.

### Tenant Configuration

6. As a tenant admin, I want to configure a data source (CRM, database, website, social media, file upload), so that the platform can ingest my company data.
7. As a tenant admin, I want to test a data source connection before enabling it, so that I know my credentials and configuration are correct.
8. As a tenant admin, I want to set a report schedule (daily at 9am, weekly on Monday, monthly on the 1st), so that I receive reports when I need them.
9. As a tenant admin, I want to configure which generic tools are available to the AI (web search, etc.), so that I control what external data the AI can access.
10. As a tenant admin, I want to configure which private tools are available (CRM query, database query), so that the AI can access my internal systems during reasoning.
11. As a tenant admin, I want to set the report format and focus areas (e.g., "focus on sales trends and customer churn"), so that reports are relevant to my business.
12. As a tenant admin, I want to invite team members with view-only or admin roles, so that my team can access reports without full configuration access.

### Data Ingestion

13. As a tenant, I want the platform to automatically pull data from my configured sources on a schedule, so that my knowledge base stays current.
14. As a tenant, I want to upload files (PDF, DOCX, CSV, XLSX), so that unstructured internal documents become part of my knowledge base.
15. As a tenant, I want to see ingestion status per source (last sync time, records ingested, errors), so that I know my knowledge base is healthy.
16. As a tenant, I want the platform to deduplicate data across syncs, so that my knowledge base doesn't bloat with repeated content.
17. As a tenant, I want to manually trigger a re-ingestion of a specific source, so that I can refresh data after making changes on my end.

### Report Generation

18. As a tenant user, I want to receive a daily strategic report summarizing key business signals, so that I start my day informed.
19. As a tenant user, I want to receive a weekly strategic report with trend analysis, so that I can spot patterns over time.
20. As a tenant user, I want to receive a monthly strategic report with deeper analysis and recommendations, so that I can make informed strategic decisions.
21. As a tenant user, I want to trigger an on-demand report, so that I can get an immediate analysis when something urgent happens.
22. As a tenant user, I want the AI to use tools during report generation (query my CRM, search the web for competitor news), so that reports include live data beyond what's in the knowledge base.
23. As a tenant user, I want to view past reports in the web UI, so that I can reference previous analyses.
24. As a tenant user, I want to receive reports via email, so that I don't have to log into the platform to read them.

### Usage Tracking and Invoicing

33. As a platform operator, I want to see a per-tenant usage dashboard (tokens, LLM calls, tool calls, cost) for any time range, so that I can bill accurately and spot anomalies.
34. As a platform operator, I want to set a token budget per tenant per month, so that a runaway job doesn't generate unexpected costs.
35. As a platform operator, I want to receive an alert when a tenant is approaching their budget limit, so that I can intervene before overage.
36. As a platform operator, I want to generate a monthly invoice per tenant from usage data, so that billing is automated.
37. As a tenant admin, I want to see my current month's usage and remaining budget in the dashboard, so that I can manage my consumption.
38. As a tenant admin, I want to view past invoices in the dashboard, so that I can reconcile billing.

### Error Handling and Resilience

25. As a tenant, I want to be notified when ingestion fails for a data source, so that I can fix credentials or configuration.
26. As a tenant, I want report generation to retry on transient LLM failures, so that a temporary API outage doesn't cost me a report.
27. As a tenant, I want a report to still generate (with degraded context) if one data source is unavailable, so that I'm not blocked by a single failing connector.
28. As a platform operator, I want failed jobs to appear in a dead letter queue with error details, so that I can diagnose and reprocess them.

### Security and Audit

29. As a tenant admin, I want my data encrypted at rest, so that a database breach doesn't expose my business data.
30. As a tenant admin, I want all data access to be logged (who accessed what, when), so that I have an audit trail.
31. As a tenant admin, I want my data source credentials stored encrypted, so that they are not exposed in plaintext.
32. As a tenant, I want my knowledge base to be completely isolated from other tenants, so that no other company can access my data.

## Implementation Decisions

### Key Architecture Decisions

**Why Go?** The job queue workers are the system's backbone — long-running, concurrent, memory-sensitive processes. Go excels at exactly this: goroutines cost ~2KB each (vs Python process), startup is <10ms, workers peak at 50-100MB, and a single static binary deploys with no venv or dependency management. The LLM and embedding providers both have Go SDKs (DashScope has `casibase/dashscope-go-sdk`, plus the API is OpenAI-compatible). The AI/ML ecosystem advantage of Python (sentence-transformers, HuggingFace) doesn't apply here — we're not training models; we're calling APIs over HTTP. With Go, the whole stack compiles to a single binary per service.

**Web framework: Echo, not bare `net/http` + `chi`.** `labstack/echo` is a batteries-included web framework with built-in routing, middleware composition, request binding, validation, and OpenAPI generation via `swaggo/swag`. For a CRUD-heavy + HTMX app, Echo removes more boilerplate than `chi` (which is intentionally minimal). The tradeoff is slightly more abstraction vs. the stdlib, but Echo is widely adopted and the team is unlikely to outgrow it.

**Embedding strategy:** Use DashScope's hosted `text-embedding-v3` API (multilingual, 1024-dim, very low cost). This eliminates the need to run `sentence-transformers` in Python and avoids any Python dependency in the stack. If local embeddings are later needed (to cut API cost at scale), replace with ONNX-Runtime-in-Go via `gomlx/onnx-gomlx` — the Embedder is behind an internal seam, so swapping is a non-breaking change.

**Why no API gateway?** An API gateway (Kong, Traefik, AWS API Gateway) solves problems this project doesn't have yet: multiple independently deployed services, edge-level rate limiting, request transformation across services. In our architecture, all services are co-located Go binaries behind a single Echo server. Echo handles routing, middleware-based auth, and rate limiting. Tools are called by workers, not by clients, so there is no external tool routing need. Add Traefik later if/when services are independently deployed — it's a configuration change, not an architecture change.

**Why RLS + partitioning over per-tenant infrastructure?** Operating N databases is a heavy operational burden for a multi-tenant platform. RLS enforces tenant scoping at the database level (even a buggy application cannot leak data). Table partitioning physically separates tenant data on disk. Together they satisfy any non-regulated enterprise customer. If a regulated entity demands physical separation, the module interfaces allow pointing a tenant at a dedicated Postgres instance via configuration change — no code changes.

**Why `hibiken/asynq` over RQ/Celery?** RQ is Python-only. Celery is powerful but complex (multiple brokers, multiple result backends, configuration-heavy). `asynq` is Go-native, uses plain Redis, and provides job scheduling, retries, dead letter queues, unique jobs, and per-queue concurrency control in one well-maintained library.

**Why self-managed Postgres with custom RLS?** A single Postgres instance with Row-Level Security and table partitioning gives us tenant isolation without the operational overhead of per-tenant databases. We implement our own auth (JWT, password hashing, session management) rather than depending on a third-party auth provider — this keeps the platform self-contained and avoids lock-in to any particular infrastructure vendor. File uploads are handled by a dedicated Go endpoint backed by local or S3-compatible storage.

### Architecture Overview

The system is organized into six modules communicating through a shared job queue and a shared database:

```
Control Plane ──(config)──► Ingestion Workers ──► Knowledge Store
    │                                                    │
    ├──(schedule)──► Report Workers ◄──(query)───────────┘
    │                    │    │
    │                    │    └──► Tool Registry
    │                    │              │
    │                    └──► LLM Client
    │
    └──(job queue)──► All Workers
```

All asynchronous work (ingestion, report generation, delivery) flows through a single job queue. The job queue is the system's backbone — not an API gateway, not an event bus.

### Module 1: Control Plane

**Interface:**
- `create_tenant(config) → tenant_credentials`
- `update_tenant(tenant_id, config) → void`
- `suspend_tenant(tenant_id) → void`
- `delete_tenant(tenant_id) → void`
- `add_data_source(tenant_id, source_config) → source_id`
- `remove_data_source(tenant_id, source_id) → void`
- `set_report_schedule(tenant_id, schedule_config) → void`
- `configure_tool_permissions(tenant_id, tool_list) → void`
- `get_usage(tenant_id, date_range) → usage_report`

**Responsibilities:**
- Tenant lifecycle (create, configure, suspend, delete with full data erasure)
- Data source registry and credential management
- Report scheduling (cron-based schedule definitions)
- Tool permission management (generic and private tools per tenant)
- Usage tracking: **aggregates** usage metadata emitted by LLM Client and Tool Registry. These modules emit usage events; Control Plane stores and queries them.

**Auth note:** Authentication is handled by the platform's own auth module. Users register and log in via the Echo API, receiving a JWT. The Echo middleware verifies the JWT on every request, reads the user's active tenant from their session/cookie, and injects `tenant_id` into context. The Control Plane never deals with login/token logic — it receives an already-authenticated `tenant_id`.

**Storage:** Self-managed Postgres with pgvector. Stores tenants, users, data source configs (credentials encrypted at rest), schedules, tool permissions, and usage aggregates. RLS is enforced by Postgres policies at the database layer.

### Module 2: Ingestion Workers

**Interface:**
- `ingest(tenant_id, source_id) → ingestion_result`

**Responsibilities:**
- Connect to data sources using tenant-specific credentials
- Extract raw data (documents, records, posts, pages)
- Detect changes since last ingestion (skip unchanged data)
- Chunk documents into semantically coherent segments
- Generate embeddings for each chunk via an embedding API
- Store chunks and vectors in the Knowledge Store
- Report ingestion metrics (records processed, errors, duration)

**Connector types (Phase 1):**
- File upload (PDF, DOCX, CSV, XLSX) — extracted server-side
- Website crawler — using an established crawling library

**Connector types (Phase 2):**
- CRM connector (HubSpot or Salesforce API)
- Social media connector (one platform)

**Connector types (Phase 3+):**
- Additional CRM/ERP connectors
- Database connectors (Postgres, MySQL — read-only)
- Additional social media platforms

Each connector is an internal seam within the Ingestion module — swappable, independently testable, hidden behind the module's interface.

### Module 3: Knowledge Store

**Interface:**
- `store(tenant_id, chunks_with_metadata) → void`
- `query(tenant_id, text, top_k, filters) → ranked_chunks`
- `delete_tenant_data(tenant_id) → void`
- `get_stats(tenant_id) → knowledge_base_stats`

**Responsibilities:**
- Store document chunks with metadata (source, date, document type, tenant_id)
- Store and index vector embeddings
- Perform similarity search scoped to a tenant_id (always, with no exception)
- Support metadata filtering (by source, date range, document type)
- Enforce tenant isolation at the storage layer (row-level or namespace-level)
- Handle full tenant data deletion

**Storage:** pgvector (Postgres extension). Chosen over Qdrant to minimize infrastructure. The vector store lives in the same Postgres instance as the control plane, in a separate schema.

**Isolation model:** Every query function receives `tenant_id` as a required parameter. The storage layer enforces scoping — a query without a tenant_id is impossible by construction, not by convention.

### Module 4: LLM Client

**Interface:**
- `complete(messages, options) → completion_result`

Where `completion_result` includes the response text, token counts (input/output), model used, and latency.

**Responsibilities:**
- Call the configured LLM provider (Qwen/DashScope by default) using API keys managed by the platform
- Retry on transient failures (rate limits, timeouts, 5xx errors) with exponential backoff
- Fall back to a secondary provider if the primary is unavailable
- Count tokens per request (from API response metadata)
- Enforce per-tenant rate limits and concurrency caps
- Record usage metadata (tenant_id, model, token counts, latency, cost estimate)
- Handle structured output when the agent loop requires JSON responses

**LLM provider: Qwen (DashScope API).** Primary: Qwen3.7-max. Fallback: Qwen3.7-plus. The LLM Client module abstracts the provider behind its interface, so switching to a different provider (OpenAI, Anthropic) or self-hosting via vLLM/Ollama requires no changes to callers.

**Provider strategy:** Configure a primary provider and a fallback provider. If the primary returns an error, the client retries N times, then falls back. Token usage is tracked per-provider for billing reconciliation.

**Embedding model: DashScope `text-embedding-v3` (hosted).** Multi-lingual, 1024-dimensional vectors. Cost is ~¥0.0007 per 1000 tokens — negligible compared to LLM token cost. This eliminates any Python/sentence-transformers dependency from the stack. The embedding provider is isolated behind an internal interface in the Ingestion module: if you want local ONNX inference later (to cut API cost at scale), swap the adapter without changing any callers.

### Module 5: Report Workers

**Interface:**
- `generate_report(tenant_id, report_config) → report`

Where `report_config` specifies the report type (daily/weekly/monthly/on_demand), focus areas, and delivery method.

**Responsibilities:**
- Execute the report generation pipeline:
  1. **Gather context** — query the Knowledge Store with report-specific prompts tailored to the report type (e.g., daily focuses on immediate signals, monthly on trends and recommendations)
  2. **Agent loop** — run an LLM reasoning loop over the gathered context. The LLM decides which tool calls to make based on the report focus areas and available tools. Tool results are incorporated back into the reasoning context. The loop is bounded (max N tool calls per report) to prevent runaway execution.
  3. **Synthesize** — the LLM produces the final report in the configured format
  4. **Store** — persist the report for historical access
  5. **Deliver** — send via configured channels (web UI, email)
  6. **Record usage** — emit token and tool usage metadata

**Agent loop design:**
- The loop is **not interactive** — it runs to completion without human input
- Max tool calls per report: configurable, default 10
- Max LLM calls per report: configurable, default 15
- If a tool call fails, the agent receives the error and decides whether to retry, skip, or abort
- If the LLM enters a loop (same tool call twice with same params), the worker breaks the loop
- Total execution time cap: 10 minutes per report

**Agent loop design (detailed):**

The agent loop is the most technically complex part of the system. Here is the full protocol:

```
Phase 1: CONTEXT GATHERING
  ┌─────────────────────────────────────────────────┐
  │ 1. Build system prompt from report config       │
  │    - Tenant context, focus areas, report type   │
  │    - Available tools (from Tool Registry)       │
  │ 2. Query Knowledge Store with report-type prompt│
  │    - Daily: "recent signals, anomalies"         │
  │    - Weekly: "trends, patterns vs prior week"   │
  │    - Monthly: "strategic trends, recommendations"│
  │ 3. Compose gathered context into messages       │
  └─────────────────────────────────────────────────┘
                          │
Phase 2: AGENT LOOP       ▼
  ┌─────────────────────────────────────────────────┐
  │ LLM receives: [system] + [context] + [task]    │
  │                                                  │
  │ Loop (max N iterations):                         │
  │   LLM response →                                │
  │     if text response → Phase 3 (synthesize)      │
  │     if tool_calls →                              │
  │       validate params via Tool Registry          │
  │       invoke tool(s)                             │
  │       append tool results to messages            │
  │       continue loop                              │
  │                                                  │
  │ Termination conditions (any one):                │
  │   ✓ LLM returns text-only response (done)        │
  │   ✓ Max tool calls reached (default: 10)         │
  │   ✓ Max LLM calls reached (default: 15)          │
  │   ✓ Total time exceeded (default: 10 min)        │
  │   ✓ Duplicate tool call detected (break loop)    │
  └─────────────────────────────────────────────────┘
                          │
Phase 3: SYNTHESIS        ▼
  ┌─────────────────────────────────────────────────┐
  │ Final LLM call: "Produce the report using all   │
  │ context gathered. Include source citations."     │
  │ → Structured output (report sections, citations) │
  └─────────────────────────────────────────────────┘
```

- The LLM is instructed to produce a text-only response (no tool calls) when it has enough information to write the report. This is the primary termination signal.
- If a tool call fails, the error message is appended as a tool result: `{"error": "tool_name failed: <reason>"}`. The LLM decides whether to retry or work around it.
- Duplicate detection: if the LLM calls the same tool with the same params twice, the second call returns `{"error": "duplicate_call", "message": "This call was already made with the same parameters."}` rather than executing again.
- The agent loop runs entirely in-process in the report worker. No human in the loop.

**Report types differ in context gathering strategy, not in architecture:**
- Daily: recent data (last 24h), focus on anomalies and immediate signals
- Weekly: trend data (last 7d), focus on patterns and comparisons
- Monthly: broader context (last 30d), focus on strategic trends and recommendations
- On-demand: user-specified focus, same pipeline as daily

### Module 6: Tool Registry

**Interface:**
- `list_tools(tenant_id) → tool_descriptions` (for LLM prompt construction)
- `invoke(tenant_id, tool_name, params) → tool_result`

**Responsibilities:**
- Maintain a catalog of available tools with their schemas and descriptions
- Enforce per-tenant tool permissions (check authorization before every invocation)
- Execute two categories of tools:
  - **Generic tools** — platform-managed, shared across tenants (e.g., web search via a third-party API). The registry executes these directly.
  - **Private tools** — tenant-specific, require tenant credentials (e.g., CRM query uses the tenant's HubSpot API key). The registry retrieves the tenant's credentials from the control plane and uses them to call the external system.
- Validate tool parameters against the tool's schema before execution
- Record tool invocation metadata (tenant_id, tool_name, params summary, duration, success/failure)
- Handle tool execution timeouts (configurable per tool, default 30 seconds)

**Private tool security:** Tenant credentials for private tools are fetched from the control plane at invocation time (not cached long-term in the registry). Credentials are transmitted encrypted and used in-memory only for the duration of the tool call.

### Job Queue

**Role:** The asynchronous backbone of the entire system. All non-interactive work is expressed as jobs.

**Job types:**

| Job Type | Trigger | Priority |
|---|---|---|
| `ingestion.scheduled` | Cron schedule per data source | Normal |
| `ingestion.manual` | Tenant triggers re-ingestion | Normal |
| `ingestion.file_upload` | Tenant uploads a file | High |
| `report.daily` | Daily cron per tenant | Normal |
| `report.weekly` | Weekly cron per tenant | Normal |
| `report.monthly` | Monthly cron per tenant | Normal |
| `report.on_demand` | Tenant requests immediate report | High |
| `delivery.email` | Report ready for email delivery | Low |

**Implementation:** `hibiken/asynq` on Redis. Chosen because it is Go-native, simple, and provides job scheduling, retries, dead letter queues, unique jobs, and per-queue concurrency control — all in one library. BullMQ is JavaScript-only; RQ is Python-only; Celery is overly complex. `asynq` is the natural Go equivalent.

**Rate limiting per queue:** Ingestion and report jobs are rate-limited per-tenant to prevent one tenant from monopolizing workers.

**Rate limiting per queue:** Ingestion and report jobs are rate-limited per-tenant to prevent one tenant from monopolizing workers.

### Data Isolation Model

**Strict logical isolation via RLS + partitioning.** All tenants share the same infrastructure (same Postgres, same Redis, same worker processes). Isolation is enforced at **three layers** — a bug in any one layer does not cause cross-tenant leakage:

1. **Application layer:** Every data access function takes `tenant_id` as its first required parameter. Every query, every embedding search, every tool invocation is scoped. A function that doesn't check tenant_id cannot exist by type signature.
2. **Database layer (RLS):** Row-level security policies on every table — `CREATE POLICY tenant_isolation ON {table} USING (tenant_id = current_setting('app.tenant_id'))`. Even if the application has a bug, Postgres will not return cross-tenant data.
3. **Vector store layer (partitioning):** Chunk and embedding tables are partitioned by `tenant_id`. Postgres physically separates data on disk. The query planner only scans the relevant partition. Similarity search cannot read another tenant's vectors by construction.

**LLM isolation:** The LLM never sees raw tenant data from the knowledge store. It receives only the chunks retrieved for the current tenant's report. Each report generation runs in a separate context window. There is no shared prompt cache or shared context between tenants.

**Credential isolation:** Data source credentials are encrypted per-tenant, decrypted only at invocation time, held in memory for the duration of the tool call, then zeroed.

**Upgrade path:** If a future enterprise customer requires physical isolation, the module interfaces are designed so that a tenant's Knowledge Store and Ingestion can be pointed at a dedicated Postgres instance without changing caller code. This is a configuration change, not an architectural change.

### Usage Tracking and Invoicing

Every costly action in the system emits a usage event. The platform collects these events, maintains real-time counters for dashboards, persists raw records for audit, and rolls up daily aggregates for billing and invoicing.

#### Events and emitters

| Source Module | Event Type | Fields |
|---|---|---|
| LLM Client | `llm_usage` | tenant_id, model, input_tokens (int64), output_tokens (int64), cost_estimate_usd (numeric), latency_ms, timestamp |
| Tool Registry | `tool_usage` | tenant_id, tool_name, duration_ms (int64), success (bool), timestamp |
| Ingestion Workers | `embedding_usage` | tenant_id, chunks_processed (int), embedding_tokens (int64), latency_ms, timestamp |

Each emitter calls a single shared interface:

```go
// UsageEmitter abstracts the transport of usage events.
// Implemented by Redis-backed writer in production,
// by no-op writer in tests.
type UsageEmitter interface {
    EmitUsage(ctx context.Context, event UsageEvent) error
}
```

The `UsageEvent` is a discriminated union carrying one of the three event types above. All events carry `tenant_id` and `timestamp`.

#### Flow

```
LLM Client / Tool Registry / Ingestion
           │  (call EmitUsage after every action)
           ▼
     ┌──────────────────┐
     │  Usage Emitter   │  ──► Redis (write-through, <1ms latency)
     └──────────────────┘         │
                                  │
                                  ▼
                       ┌──────────────────────┐
                       │  Usage Collector      │
                       │  (background Go worker)│
                       │                       │
                       │  • Flush raw events    │
                       │    from Redis → PG     │
                       │  • Hourly roll-up into │
                       │    usage_daily table   │
                       └──────────────────────┘
                                  │
                                  ▼
                       ┌──────────────────────┐
                       │  Postgres            │
                       │                      │
                       │  usage_events (raw)  │
                       │  usage_daily (agg)   │
                       │  invoices            │
                       └──────────────────────┘
```

#### Redis: real-time counters

For dashboard "Tenant X has used N tokens today" views, Redis holds per-tenant counters:

```redis
# Per-tenant daily counters (atomic INCRBY)
HINCRBY tenant:abc:today:llm:qwen3.7-max input_tokens 42000
HINCRBY tenant:abc:today:llm:qwen3.7-max output_tokens 12500
HINCRBY tenant:abc:today:embedding input_tokens 8000
HINCRBY tenant:abc:today:tools tool_calls 1
```

Redis keys are TTL'd at 48 hours (dashboard only needs current day + yesterday).

#### Postgres: durable records

**Raw events** are persisted for audit and anomaly detection:

```sql
CREATE TABLE usage_events (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id  TEXT NOT NULL,
    event_type TEXT NOT NULL,  -- 'llm_usage' | 'tool_usage' | 'embedding_usage'
    payload    JSONB NOT NULL, -- type-specific fields
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_usage_events_tenant_time
    ON usage_events (tenant_id, created_at DESC);
```

**Daily aggregates** are rolled up by the Usage Collector worker every hour. This is the table billing and invoicing reads from:

```sql
CREATE TABLE usage_daily (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id           TEXT NOT NULL,
    date                DATE NOT NULL,
    llm_model           TEXT NOT NULL,
    input_tokens        BIGINT NOT NULL DEFAULT 0,
    output_tokens       BIGINT NOT NULL DEFAULT 0,
    tool_calls          INTEGER NOT NULL DEFAULT 0,
    embedding_tokens    BIGINT NOT NULL DEFAULT 0,
    estimated_cost_usd  NUMERIC(12,6) NOT NULL DEFAULT 0,
    reports_generated   INTEGER NOT NULL DEFAULT 0,
    UNIQUE (tenant_id, date, llm_model)
);
CREATE INDEX idx_usage_daily_tenant_date ON usage_daily (tenant_id, date DESC);
```

**Invoices** are generated from `usage_daily` at the end of each billing period:

```sql
CREATE TABLE invoices (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id         TEXT NOT NULL,
    period_start      DATE NOT NULL,
    period_end        DATE NOT NULL,
    total_cost_usd    NUMERIC(12,6) NOT NULL,
    line_items        JSONB NOT NULL,   -- [{model, input_tokens, output_tokens, cost}]
    status            TEXT NOT NULL DEFAULT 'draft',  -- draft | issued | paid | overdue
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

#### Budget enforcement

Each tenant has a monthly budget in the tenant config. The LLM Client checks the budget *before* each call:

- If `today_cost + current_call_cost > daily_budget_fraction` (e.g. 1/30 of monthly budget), emit a warning to the Usage Collector
- If `month_to_date_cost > monthly_budget`, reject the call with a budget-exceeded error (tenant is notified)
- Platform operators are alerted at 80% and 95% thresholds

#### Invoice generation

Invoice generation is a scheduled job that runs at the start of each billing period (e.g. 1st of month):

1. Read `usage_daily` rows for the closed billing period
2. Apply pricing rules (per-token, per-report, or flat-tier — configured per tenant)
3. Compute line items and total
4. Insert an `invoices` row with status `issued`
5. Optionally email the invoice to the tenant admin

Pricing rules live in the tenant config and are evaluated as a simple formula — no external billing service is required at launch.

#### Control Plane interface additions

```go
// On Control Plane module:
GetUsageSummary(tenantID string, from, to time.Time) (*UsageSummary, error)
GenerateInvoice(tenantID string, periodStart, periodEnd time.Time) (*Invoice, error)
GetInvoices(tenantID string) ([]*Invoice, error)
SetTenantBudget(tenantID string, monthlyBudgetUSD decimal.Decimal) error
```

### Security Baseline

- All data encrypted at rest (Postgres TDE or application-level encryption for sensitive fields)
- All data source credentials stored encrypted (AES-256 or equivalent, keys managed via environment variables, never in code)
- All inter-service communication over TLS (or localhost for co-located services)
- Audit log for all data access: who (user or system), what (operation), when (timestamp), which tenant
- Right to erasure: `delete_tenant` cascades to all data — knowledge base, reports, credentials, audit logs (after retention period), usage records (after billing period)

### Technology Choices

| Concern | Choice | Rationale |
|---|---|---|
| Language | Go 1.22+ | Single static binary per service. Low memory (workers: 50-100MB). Fast startup (<10ms). Native concurrency via goroutines. Compile-time type safety across all modules. |
| HTTP Framework | `labstack/echo` | Batteries-included: routing, middleware composition, request binding/validation, OpenAPI generation via `swaggo/swag`. Less boilerplate than bare `net/http` + `chi` for CRUD-heavy + HTMX apps. |
| Web UI | Go `a-h/templ` + HTMX | Type-safe server-rendered HTML templates compiled at build time. Zero JS build step. ~14KB frontend JS (HTMX only). Compile-time catches template errors before deployment. |
| Platform / Database | Self-managed Postgres 16 + pgvector | Full control over infrastructure. RLS and table partitioning for tenant isolation. Single database, zero vendor lock-in. |
| Auth | Custom JWT auth (golang-jwt) | Platform-owned auth with no third-party dependency. JWT issuance, password hashing, and session management built in Go. |
| File Storage | Local filesystem or S3-compatible (MinIO) | Files uploaded via the Echo API, stored on local disk or S3-compatible object storage. Workers access files from the same storage. |
| Job Queue | `hibiken/asynq` + Redis | Go-native job queue on Redis. Scheduling, retries, dead letters, unique jobs, per-queue concurrency. Simpler than Celery, equivalent to RQ. |
| LLM Provider | Qwen3.7-max (primary), Qwen3.7-plus (fallback) | Via DashScope API. Strong tool-use and reasoning. Fallback ensures resilience. |
| Embeddings | DashScope `text-embedding-v3` | Hosted, multilingual, 1024-dim, very low cost. No local model dependency. Local ONNX inference available later via `gomlx/onnx-gomlx` if needed. |
| File Parsing | `unidoc/unipdf` (PDF), `nguyenthenguyen/docx` (DOCX), `tealeg/xlsx` (XLSX), `gocarina/gocsv` (CSV) | Don't build parsers. |

### Web UI Architecture

The web UI is a **thin client** — all business logic lives in the backend API. The UI compiles at build time via `a-h/templ` (type-safe Go → HTML) and is served by the main Go binary, with HTMX for dynamic interactions.

- **Not a module.** The web UI has no domain logic. It is a presentation layer over the Control Plane API.
- **Auth is custom JWT.** The platform issues its own JWTs (golang-jwt) on login. Echo middleware verifies the JWT on each request. The UI posts credentials to the Echo API and stores the returned JWT.
- **File uploads** go through the Echo API to local or S3-compatible storage. The UI sends files to the Go API, which stores them and notifies the ingestion pipeline.
- **Report viewing** uses Server-Sent Events (Echo `StreamResponseWriter`) to stream generation progress.
- **Compile-time safety.** Templ templates are compiled Go code — a template error is a compile error, not a runtime surprise.
- **Upgrade path:** If a richer SPA is needed later, any JS frontend can hit the same Echo API endpoints. The UI technology is fully swappable.

### Auth & Tenant Isolation

Authentication is handled by the platform's own auth module (golang-jwt, bcrypt password hashing). All business logic lives in Go.

#### Multi-tenant Auth Design

The auth system uses global email uniqueness (one account per email across the entire platform). To support a user belonging to multiple tenants with different roles:

```sql
-- Which tenants can this user access, and in what role?
CREATE TABLE tenant_memberships (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID NOT NULL REFERENCES users(id),
    tenant_id  TEXT NOT NULL REFERENCES tenants(id),
    role       TEXT NOT NULL,   -- 'admin' | 'viewer'
    UNIQUE (user_id, tenant_id)
);
```

On login, the client receives a JWT. The user's active tenant is tracked in a session cookie (or local state). The Echo middleware sets `app.tenant_id` in the Postgres session for RLS.

RLS policies enforce isolation using both the JWT and the session context:

```sql
-- Tenant-scoped data: must match current tenant context
CREATE POLICY tenant_isolation ON chunks
    FOR ALL
    USING (tenant_id = current_setting('app.tenant_id'));

-- Auth: only members of the tenant can access tenant data
CREATE POLICY membership_check ON tenants
    FOR SELECT
    USING (id IN (
        SELECT tenant_id FROM tenant_memberships
        WHERE user_id = current_setting('app.user_id')::uuid
    ));

-- Credentials: only admins can read (for tool invocations)
CREATE POLICY credential_access ON data_source_credentials
    FOR SELECT
    USING (
        tenant_id = current_setting('app.tenant_id')
        AND EXISTS (
            SELECT 1 FROM tenant_memberships
            WHERE user_id = current_setting('app.user_id')::uuid
            AND role = 'admin'
        )
    );
```

This is the standard Postgres RLS pattern. The application sets context correctly; the database enforces isolation regardless of application bugs.

#### Tenant switching flow

A user belonging to multiple tenants has one active tenant at a time:

1. Login → Platform returns JWT
2. Client lists user's tenants: `SELECT tenant_id, role FROM tenant_memberships WHERE user_id = $1`
3. User picks a tenant → client sets `current_tenant` in local session (cookie)
4. Every subsequent request includes `x-tenant-id` header (or cookie)
5. Echo middleware reads it, sets `app.tenant_id` in Postgres session → enables RLS
6. User can switch tenants without re-login

## Testing Decisions

### What makes a good test

Tests exercise a module through its **interface only**. No reaching into internals. If a test needs to verify something that isn't observable through the interface, the interface is wrong.

Tests should be deterministic. Mock external services (LLM APIs, CRM APIs, web search APIs). Use real Postgres and Redis for integration tests via test containers.

### Modules and seams

| Module | Primary Seam | What's tested |
|---|---|---|
| Control Plane | Its public interface | Tenant CRUD, schedule management, tool permissions, credential encryption/decryption, data erasure completeness |
| Ingestion Workers | `ingest(tenant_id, source_id) → result` | Each connector produces correct chunks. Deduplication works. Errors are reported, not swallowed. Knowledge Store receives properly formed chunks. |
| Knowledge Store | `store/query/delete` interface | Tenant isolation (cross-tenant queries return nothing). Similarity search returns relevant results. Metadata filtering works. Deletion is complete. |
| LLM Client | `complete(messages, options) → result` | Retry behavior on transient errors. Fallback to secondary provider. Token counting accuracy. Rate limit enforcement. |
| Report Workers | `generate_report(tenant_id, config) → report` | Full pipeline with mocked LLM and tools. Agent loop terminates within bounds. Degraded reports when a tool fails. Report content matches expected structure. |
| Tool Registry | `invoke(tenant_id, tool, params) → result` | Authorization enforcement. Schema validation. Timeout handling. Generic vs private tool routing. |

### Integration tests

- **End-to-end report generation:** Seed a knowledge base with known data, mock the LLM to return a fixed response, verify the report is generated, stored, and delivery metadata is emitted.
- **Tenant isolation:** Create two tenants with overlapping data, verify that queries from one tenant never return the other's data.
- **Data erasure:** Create a tenant, ingest data, generate reports, then delete the tenant. Verify no data remains in any store.
- **Resilience:** Kill the LLM provider mid-report, verify retry and fallback behavior. Kill a data source mid-ingestion, verify graceful error reporting.

### What not to unit test

- `asynq` internals (it's a dependency, not our code)
- Postgres/pgvector internals
- LLM provider response formats (mock at the HTTP boundary)
- DashScope embedding API internals (mock at the HTTP boundary)

## Out of Scope

- **Interactive chat/query mode** — the platform generates reports on schedule. Real-time conversational AI is a future feature.
- **Self-hosted LLM at launch** — start with DashScope API. Self-hosting Qwen via vLLM is a Phase 4 optimization, accommodated by the LLM Client interface.
- **Per-tenant dedicated infrastructure** — all tenants share infrastructure. Dedicated compute is a future enterprise tier, achievable via configuration change.
- **Mobile apps** — web UI only at launch.
- **White-labeling** — all tenants use the same branded interface.
- **Advanced analytics/BI dashboards** — reports are generated documents, not interactive dashboards.
- **Real-time data streaming** — ingestion is periodic (batch), not real-time.
- **SOC2/HIPAA certification** — the architecture supports future certification, but the certification process itself is out of scope for launch.
- **API gateway** — the Echo server handles routing, auth (via JWT middleware), and rate limiting for all services. An API gateway (e.g. Traefik) is added only when independently deployed services emerge.

## Further Notes

### Risks

- **Connector maintenance** — third-party APIs (CRM, social media) change without notice. Each connector is a maintenance liability. Mitigation: start with few connectors, add based on customer demand, and build connectors behind a stable internal interface so changes are localized.
- **LLM cost unpredictability** — report generation with tool use can consume significant tokens. Mitigation: per-tenant token budgets, cost alerts, and the ability to cap report complexity.
- **Report quality** — LLM-generated strategic reports may hallucinate or miss key insights. Mitigation: ground every report in retrieved context (RAG), include source citations in reports, and implement a feedback mechanism for tenants to flag poor reports.
- **Data source credential security** — the platform stores and uses tenants' CRM/DB credentials. A breach exposes not just tenant data but tenant system access. Mitigation: encrypt credentials at rest, minimize credential lifetime in memory, and consider OAuth flows where possible (so the platform never holds raw passwords).

### Resolved Decisions

- **LLM provider:** Qwen3.7-max (primary), Qwen3.7-plus (fallback) via DashScope API. Strong tool-use and reasoning, no GPU infrastructure needed.
- **Embedding model:** DashScope `text-embedding-v3`. Hosted, multilingual, 1024 dimensions.
- **Web framework:** Go `labstack/echo` for HTTP API, `a-h/templ` + HTMX for UI. Full-stack Go, single binary per service.
- **Platform / Database:** Self-managed Postgres 16 + pgvector. RLS + table partitioning for tenant isolation. Single database, zero vendor lock-in.
- **Job queue:** `hibiken/asynq` (Redis Queue). Go-native, simpler than Celery.
- **Data isolation:** RLS + table partitioning. No per-tenant infrastructure needed.
- **API gateway:** Not needed for prototype. Echo handles routing and rate limiting. Add Traefik later if multi-service architecture emerges.
- **Multi-language support:** Solved by DashScope `text-embedding-v3` (multilingual) + Qwen's multilingual capabilities.

### Remaining Questions

- **Report delivery channels beyond email?** Slack, webhook, dashboard embedding — to be determined based on customer feedback.
- **Pricing model?** Per-report, per-token, flat monthly with tiers? Affects how usage tracking and billing are built.
- **Self-hosted Qwen vs DashScope API?** Start with DashScope API. Evaluate self-hosting via vLLM when volume justifies it.

### Build Sequence

**Phase 0 — Walking skeleton (2 weeks):** Get the end-to-end pipeline working synchronously for a single hardcoded tenant.
- Upload a file → chunk → embed (DashScope) → store in pgvector
- Query knowledge store → call Qwen → produce a report
- All via Go CLI / simple HTTP server, no job queue, no scheduling, minimal UI
- Goal: demonstrate the complete pipeline works. File in, report out.

**Phase 1 — Multi-tenant + async (4-6 weeks):**
- Add tenant creation, user auth, credential management (Control Plane)
- Add RLS policies and tenant-isolated partitioning to all tables
- Move ingestion and report generation to `asynq` job queue
- Add cron scheduling for daily reports
- Web UI with `templ` + HTMX: file upload, report viewing, tenant config
- Usage tracking: real-time Redis counters, daily Postgres aggregates
- Goal: a registered tenant uploads files, gets a daily report by schedule, and sees usage in the dashboard.

**Phase 2 — Expand data and reasoning (6-8 weeks):**
- Website crawler connector, one CRM connector
- Tool Registry with web search and CRM query
- Full agent loop with tool use in report generation
- Weekly and monthly report types
- Budget enforcement and alerting
- Goal: reports that use tools and pull from live external sources.

**Phase 3 — Production readiness (4-6 weeks):**
- Automated invoice generation, billing integration
- Email delivery, monitoring and alerting
- Error notifications to tenants
- Audit logging
- Goal: shippable to paying customers.

**Phase 4 — Scale (ongoing):**
- More connectors
- Interactive query mode
- Self-hosted Qwen evaluation (vLLM)
- Local ONNX embedding inference (replace DashScope embedding API)
- Dedicated compute tier for enterprise customers
