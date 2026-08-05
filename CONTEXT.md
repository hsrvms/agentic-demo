# Context — Multi-Tenant AI Report Platform

This is the ubiquitous language for the platform. All modules share these terms.
When a term appears in code, it means exactly what this glossary says it means.

---

## Core Entities

### Tenant
The top-level organizational unit. A company that signs up for the platform.
Every resource (data sources, chunks, reports, schedules, usage) is scoped to a
tenant. A tenant has a status (active, suspended, deleted) and a monthly budget.

### User
A registered person with a login. A user belongs to one or more tenants.
Users have an identity (email, password hash) managed by the platform's
auth system.

### TenantMembership
The link between a User and a Tenant, carrying a Role. A user can have
different roles in different tenants.

### Member
A User viewed within the context of a specific Tenant. "Member" is a
convenience term — it is not a separate entity, just a User with a
TenantMembership for the tenant in question.

### Role
A permission level within a tenant. Two roles exist: **Admin** (full
configuration access) and **Viewer** (read-only access to reports and usage).

---

## Data

### DataSource
A configuration record that tells the platform how to pull data from an
external system on behalf of a tenant. Contains source type, name, connection
config, encrypted credentials, and lifecycle status (inactive, active, error).

### Connector
The code that performs the actual extraction from an external system.
A connector is an implementation detail of the ingestion pipeline — it
reads from a DataSource's configuration and produces RawDocuments.

### SourceType
The kind of external system a DataSource connects to: file_upload, website,
crm_hubspot, crm_salesforce.

### RawDocument
Unparsed, unstructured content extracted by a Connector. The chunker converts
RawDocuments into Chunks.

### Chunk
A semantically coherent segment of a document with its vector embedding.
This is the unit of storage and retrieval in the Knowledge Base. Each chunk
carries metadata (source, document type, date) and references its source
Document via `document_id`.

### Document
The full parsed source text behind a chunk. Each Chunk stored in the
Knowledge Base references its source Document, so a matched chunk can be
expanded to complete context with a single lookup.

### KnowledgeBase
The collection of all Chunks for a tenant. The Knowledge Base is what the
platform queries during report generation to gather context.

### KnowledgeStore
The module/interface that stores, queries, and manages Chunks. It enforces
tenant isolation — every operation requires a tenant ID. The Knowledge Store
is backed by pgvector but callers never see that.

### TenantIsolation
The guarantee that no tenant can access another tenant's data. Enforced at
three layers: (1) application — every data access function takes a tenant ID
as a required parameter, (2) database — Postgres Row-Level Security policies
on every table, (3) storage — chunk tables are partitioned by tenant ID so
similarity search cannot read cross-tenant vectors by construction. All
tenants share a single Postgres database.

---

## Reports

### Report
The abstract domain concept: an AI-generated strategic document that a tenant
receives. Can be daily, weekly, monthly, or on-demand.

### StoredReport
The persisted record of a generated Report in the database. Contains the
content, citations, focus areas, and metadata.

### ReportType
The cadence of a report: **daily** (last 24h, anomalies and signals),
**weekly** (last 7d, trends and patterns), **monthly** (last 30d, strategic
trends and recommendations), **on_demand** (user-triggered, immediate).

### ReportSchedule
A recurring rule that triggers report generation on a cron schedule. Defines
the report type, focus areas, format, and delivery method for a tenant.

### ReportGeneration
The pipeline that produces a Report. It runs in three phases: context
gathering (query the Knowledge Base), agent loop (LLM reasons with tools),
and synthesis (LLM produces the final report). Report generation is
asynchronous — it runs as a job on the queue.

### AgentLoop
The core reasoning phase of ReportGeneration. The LLM receives context from
the Knowledge Base, then iteratively calls Tools until it has enough
information to produce the report. Bounded by max tool calls, max LLM calls,
and total time. Not interactive — no human in the loop.

---

## AI & Tools

### LLM
The large language model that powers report generation. The platform uses
Qwen (via DashScope) as the primary provider, with a fallback provider for
resilience. The LLM is invoked through the LLM Client module, which handles
retries, fallback, and usage tracking.

### Tool
A capability the LLM can invoke during the AgentLoop. Tools have a schema
(name, description, parameters) and an execution function. Two categories:
**generic tools** (platform-managed, shared across tenants, e.g., web search)
and **private tools** (tenant-specific, requiring tenant credentials, e.g.,
CRM query).

### ToolResult
The outcome of a Tool invocation. Contains either an output string or an
error string. The LLM receives the result and decides whether to retry, skip,
or proceed.

---

## Delivery

### Delivery
The act of getting a generated Report to a tenant. Delivery is abstract —
it can happen through any channel.

### DeliveryMethod
The channel used for Delivery: **web** (viewable in the dashboard) or
**email** (sent to the tenant admin's inbox). Email delivery is handled by
the Delivery module and runs as a low-priority job on the queue.

---

## Billing

### Usage
The record of resource consumption. Every costly action (LLM call, tool
invocation, embedding generation) emits a UsageEvent. Usage is the source of
truth for billing — it describes what happened, without prescribing limits.

### UsageEvent
A single recorded consumption event. Carries the tenant ID, event type
(llm_usage, tool_usage, embedding_usage), and type-specific fields (token
counts, cost estimate, latency).

### Budget
A spending limit configured per tenant per month. Enforced by the Budget
Checker before each LLM call. If the tenant exceeds their budget, calls are
rejected. Platform operators are alerted at 80% and 95% thresholds.

### Invoice
A billing document generated from Usage data at the end of a billing period.
Contains line items (per-model token counts, costs) and a total in USD. An
invoice has a lifecycle status: draft, issued, paid, overdue.

---

## Infrastructure

### JobQueue
The asynchronous backbone of the platform. All non-interactive work
(ingestion, report generation, delivery) is expressed as jobs on the queue.
Jobs have types (ingestion:scheduled, report:daily, delivery:email) and
priorities (high, normal, low). Implemented via asynq on Redis.

### Ingestion
The pipeline that pulls data from a DataSource, chunks it, generates
embeddings, and stores the results in the Knowledge Base. Ingestion runs as
a job on the queue and can be triggered by a schedule, a manual request, or
a file upload.