# Bruno API Collection

API test collection for the Agentic Demo project. Uses [Bruno](https://www.usebruno.com/) — a git-friendly, plain-text API client.

## Prerequisites

- [Bruno](https://www.usebruno.com/downloads) installed
- Server running: `go run ./cmd/server`
- Migrations applied: `make migrate-up`

## Open the collection

```bash
# From the project root, open this folder in Bruno:
bruno open bruno/
```

Or in the Bruno GUI: **Open Collection** → select the `bruno/` folder.

## Environments

Environments live in `bruno/environments/`. Select the active environment in Bruno's top-right dropdown.

### `Agentic-Demo` — local development

```bru
vars {
  base_url: http://localhost:3000
}
```

Non-sensitive defaults are stored directly in the environment file. Secrets are handled separately (see below).

## Secrets

**`.bru` files never contain secret values.** They only reference variables via `{{variable_name}}` placeholders.

### Three ways to provide secrets

| Method | Best for | How |
|--------|----------|-----|
| **Bruno Secret Variables** | Local dev (GUI) | Right-click environment → **Add Secret Variable** → toggle 🔒. Stored encrypted in Bruno's local database, never on disk. |
| **`.env` file** | Team sync, automation | Copy `.env.example` to `.env` and fill in real values. Bruno resolves these at runtime. |
| **Environment variables** | CI/CD | Inject `PROCESS_ENV_VAR` via your pipeline. Bruno resolves `{{process.env.VAR}}`. |

### Setup for a new developer

```bash
# 1. Copy the template
cp bruno/.env.example bruno/.env

# 2. Edit with your real keys
# bruno/.env is gitignored — never committed
```

Alternatively, use Bruno's **Secret Variables** in the GUI and skip the `.env` file entirely.

### Adding a new secret

1. Add a placeholder reference in the relevant `.bru` request file:

   ```bru
   headers {
     Authorization: Bearer {{third_party_api_key}}
   }
   ```

2. Add an entry to `.env.example` so the team knows it exists:

   ```bash
   # THIRD_PARTY_API_KEY=sk-...
   ```

3. Provide the value through one of the three methods above.

## What's committed vs ignored

| File | Committed | Purpose |
|------|-----------|---------|
| `*.bru` | ✅ | Request definitions, assertions, variable references |
| `environments/*.bru` | ✅ | Non-sensitive defaults (`base_url`, etc.) |
| `.env.example` | ✅ | Template listing all required secrets |
| `.env` | ❌ | Real secret values |
| `.env.*` | ❌ | Any other env overrides |

## Running tests

Select the **Agentic-Demo** environment in Bruno, then:

- **Run collection**: Click the collection name → **Run**
- **Run folder**: Right-click a folder (Auth, Tenants, Health) → **Run**

The tests are ordered to build state progressively: register → login → create tenant → tenant-scoped operations → cross-tenant isolation.

## Test order

```
Health/
  Health Check
Auth/
  Register              → sets {{token}}
  Register Duplicate
  Register Weak Password
  Login                 → sets {{token}}
  Login Wrong Password
  Reject Missing Token
  Reject Invalid Token
  Reject Missing Tenant ID
  Reject Nonexistent Tenant
Tenants/
  Create First Tenant   → sets {{tenant_id}}
  Get Me                → uses {{token}} + {{tenant_id}}
  Reject Empty Name
  List Tenants
  Create Second Tenant
  List After Create
  Cross-Tenant Isolation
  Register Second User  → sets {{token2}}
  Multi-User Isolation Assert
```