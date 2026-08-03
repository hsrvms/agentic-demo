# Self-managed Postgres with custom auth, not Supabase

Supabase bundles Auth, managed Postgres, RLS, and S3-compatible Storage into a
single platform. We chose self-managed Postgres with a custom JWT auth module
instead. Supabase is a heavy platform — we needed a simple, performant auth
system without the operational overhead and vendor lock-in of a managed
platform. Building our own auth (golang-jwt, bcrypt) and connecting directly to
Postgres via `pgx` keeps the stack self-contained, easier to reason about, and
fully portable.