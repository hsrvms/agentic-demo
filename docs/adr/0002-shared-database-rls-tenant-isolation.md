# Single shared database with RLS + partitioning for tenant isolation

All tenants share a single Postgres database. Isolation is enforced through
Row-Level Security policies and table partitioning by tenant ID — not through
per-tenant databases. Physical isolation (one database per tenant) would be
hard to operate for a multi-tenant platform: provisioning, migrations,
connection pooling, and monitoring all multiply by tenant count. RLS +
partitioning gives us the same isolation guarantees at the database level
without the operational burden. If a future enterprise customer requires
physical separation, the module interfaces allow pointing a tenant at a
dedicated Postgres instance — a configuration change, not an architecture
change.