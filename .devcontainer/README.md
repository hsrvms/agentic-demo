# Go web dev container

A plain-Docker development environment for Zed and the Dev Container CLI. It uses Docker Compose for the workspace, PostgreSQL, Redis, and MinIO. There are no editor-specific customizations.

## Layout

```text
.devcontainer/
├── Dockerfile
├── devcontainer.json
├── docker-compose.yml
└── README.md
```

Copy this directory to a repository root. Usually only the Compose project name, service credentials, exposed application port, and optional PostgreSQL image need changing.

## Host prerequisites

- Linux with Docker Engine, Compose v2, and BuildKit/buildx
- Zed **or** Node.js plus the Dev Container CLI
- Existing Pi configuration at `~/.pi`
- (Optional) GitHub CLI authenticated on the host for `gh` inside the container

Install the CLI on the host if needed:

```bash
npm install --global @devcontainers/cli
```

`docker-compose.yml` uses `${HOME}/.pi`; run Zed or `devcontainer` with `HOME` set normally. The directory must already exist, and Compose is configured to fail rather than create it. This template deliberately does not initialize or modify host Pi configuration.

For host files to be created with your UID/GID, either use the common Linux default (`1000:1000`) or export your IDs before building:

```bash
export DEV_UID="$(id -u)"
export DEV_GID="$(id -g)"
```

The Dev Container CLI also updates the `dev` account to the invoking Linux user's UID/GID. The build arguments matter when using `docker compose` directly, and avoid an initial ownership mismatch on non-1000 hosts.

## Start

### Zed

Open the repository and accept **Open in Container**, or run **Project: Open Remote** and choose the dev container. Zed runs terminals, tasks, and language servers in `workspace`.

### Dev Container CLI

From the repository root:

```bash
devcontainer up --workspace-folder .
devcontainer exec --workspace-folder . bash
```

### Plain Docker Compose

This is useful for diagnostics, though the CLI/Zed path is preferred:

```bash
docker compose -f .devcontainer/docker-compose.yml build workspace
docker compose -f .devcontainer/docker-compose.yml up -d
docker compose -f .devcontainer/docker-compose.yml exec workspace bash
```

Stop without deleting persistent data:

```bash
docker compose -f .devcontainer/docker-compose.yml down
```

Delete service data and language caches only when intentionally resetting everything:

```bash
docker compose -f .devcontainer/docker-compose.yml down --volumes
```

## Verify

Run inside `workspace`:

```bash
go version
node --version
pi --version
air -v
golangci-lint version
dlv version
sqlc version
migrate -version
templ version
tailwindcss --help >/dev/null
gopls version
staticcheck -version
tailwindcss --help >/dev/null

psql "$DATABASE_URL" -c 'select version();'
redis-cli -u "$REDIS_URL" ping
mc alias set local "http://${MINIO_ENDPOINT}" "$MINIO_ROOT_USER" "$MINIO_ROOT_PASSWORD"
mc ready local

test -d "$HOME/.pi" && test "$(findmnt -n -o FSTYPE "$HOME/.pi")" = none
pi --version

gh --version
gh auth status
```

The `findmnt` filesystem type for a Docker bind mount is commonly `none`; if the host/runtime reports a different type, inspect it with `findmnt "$HOME/.pi"` instead. The important property is that `$HOME/.pi` is the host bind mount, not container-managed state.

For this repository, also run:

```bash
go vet ./...
golangci-lint run
go test -race ./...
go build ./...
```

## Design decisions

### Debian Bookworm Slim rather than Alpine

The final workspace starts from `debian:bookworm-slim`. Alpine is smaller, but the official Go image documentation calls the Alpine variant experimental and notes its musl-libc compatibility risk. A development environment runs debuggers, language servers, native/cgo builds, Node, and third-party binaries, so glibc compatibility is worth the modest size increase. Bookworm is explicitly pinned to prevent an unplanned distribution jump. Microsoft Dev Container images and Features are not used.

Go is copied from the official `golang:1.26.5-bookworm` image rather than downloaded manually. Node is copied from the official `node:24.18.0-bookworm-slim` image because Pi's official plain-Docker example uses Node 24 Bookworm Slim.

### Fast, reproducible tool installation

The Dockerfile has independent, version-pinned stages/layers for Go tools and Node tools. Changing application source does not invalidate them because source is bind-mounted and never copied into the image. BuildKit cache mounts retain downloaded Go modules and build artifacts across tool upgrades. Runtime Go module, Go build, and npm caches are named volumes, avoiding writes to the repository and surviving container rebuilds.

Go CLIs are built with `CGO_ENABLED=0`, which avoids shipping their build toolchain dependencies. `gcc` and `libc6-dev` remain in the workspace for project cgo/race builds. `gopls` is included because Zed's Go language support needs it; `staticcheck` supports normal Go analysis.

Tailwind CSS v4's standalone CLI package is globally available as `tailwindcss`. HTMX, Echo, and application Tailwind dependencies belong in each project's `go.mod` or frontend assets; globally installing application libraries would reduce reproducibility.

### Non-root and ownership

The image creates `dev`, sets it as the container user, and uses Dev Container's standard `updateRemoteUserUID` behavior on Linux. The workspace is a bind mount, so writes are reflected directly on the host as required by Pi's plain-Docker model. No privileged mode, Docker socket, or broad capabilities are granted.

Delve can debug programs started by the same `dev` user without extra privileges in typical setups. If ptrace is blocked by your Docker/kernel policy, enable the optional debugger settings below rather than granting them by default.

### Pi integration

The whole Pi process runs inside `workspace`; therefore Pi tools, shell commands, and extensions execute inside the same container boundary. The entire existing host `~/.pi` is bind-mounted at `/home/dev/.pi`, and `HOME=/home/dev`, so Pi discovers it automatically. The image installs Pi but does not run Pi during build and does not create, initialize, copy, or manage configuration.

This is intentionally a read-write mount because Pi sessions and normal configuration state may be updated. It also exposes host Pi authentication, provider credentials, sessions, prompts, skills, and extensions to the container. Only use this template with trusted projects and images. Provider API keys consumed by Pi enter the container, as called out by Pi's documentation.

### Services and networking

Compose service DNS gives stable endpoints: `postgres:5432`, `redis:6379`, and `minio:9000`. Only the web application and MinIO ports are forwarded through the Dev Container tool. PostgreSQL and Redis stay on the private Compose network; use their CLIs from `workspace`. Health-gated dependencies prevent the workspace from being considered ready before services respond.

The PostgreSQL service uses the small, maintained pgvector image because this repository's migration enables `vector`. For a generic stack that does not use pgvector, switch to `postgres:18.4-bookworm`. PostgreSQL 18's volume is mounted at `/var/lib/postgresql`, matching the official image's 18+ data-layout guidance. Redis uses AOF persistence. MinIO uses pinned server/client releases and a named volume.

The service credentials are development-only defaults, not production secrets. They are intentionally explicit and isolated from the host network. Change them for shared or remotely reachable Docker hosts.

## Caveats

- Zed's dev-container support is still evolving. It does not automatically rebuild after `devcontainer.json` changes; stop the existing container and reopen/rebuild it.
- Zed requires Docker/Podman in its launch environment's `PATH`. This template targets Docker only.
- `devcontainer.json` intentionally has no `customizations` object. Zed extensions remain a user-level Zed choice, and no VS Code settings/extensions are present.
- The host Pi mount assumes Linux/macOS-style `${HOME}`. The requested target is Linux; native Windows paths need a Compose override.
- Remote Docker contexts resolve bind-mount source paths on the Docker daemon host, so local `~/.pi` and workspace mounts do not work unchanged against a remote daemon.
- Mounting `~/.pi` shares configuration across projects as requested, but concurrent Pi processes also share its mutable state.
- Named volumes are scoped by Compose project name. Set a unique `COMPOSE_PROJECT_NAME` if two copied projects have the same directory/project identity.
- Port forwarding support can vary by client. For plain Compose-only use, add a local override that publishes ports rather than committing broad host exposure to the template.

## Optional improvements (disabled by default)

1. **Extra ptrace permissions for Delve** — add to `workspace` only when required:

   ```yaml
   cap_add: [SYS_PTRACE]
   security_opt: [seccomp=unconfined]
   ```

2. **Seed a MinIO bucket** — add a one-shot `minio-init` service using the pinned `minio/mc` image. Application migrations or explicit scripts are usually clearer than hidden startup mutation.
3. **Publish database/cache ports** — add a developer-local Compose override with `127.0.0.1:5432:5432` and `127.0.0.1:6379:6379` when host-native tools require access.
4. **Read-only Pi config** — mount `~/.pi` read-only only if session/config writes are not needed. This is safer but can break expected Pi behavior.
5. **GitHub CLI** — `gh` is installed via the official GitHub APT repo. Authentication uses the host's `~/.config/gh` (bind-mounted read-only). Run `gh auth login` on the host once; the token is stored in `~/.config/gh/hosts.yml` and picked up automatically inside the container. If you prefer a per-project token, set `GH_TOKEN` or `GITHUB_TOKEN` in the workspace environment instead.
6. **SSH/Git credential forwarding** — prefer agent/socket forwarding supported by your client; never bake keys or tokens into this image. Client portability differs, so it is not enabled here.
7. **Digest pinning and dependency automation** — pin all image tags to digests for maximum byte-for-byte reproducibility, then use Renovate/Dependabot to keep them patched. Tags are easier for a copyable template.
8. **Multi-architecture prebuilt workspace image** — publish the final image to a registry to make onboarding faster for teams. Local builds are simpler and avoid a supply-chain/publishing workflow by default.
9. **Frontend package lockfile** — if a project needs npm dependencies beyond the global Tailwind CLI, keep `package.json` and its lockfile in the project and use a project-specific named `node_modules` volume.
