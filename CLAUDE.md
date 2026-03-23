# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Test Commands

```bash
make build              # Build binary to bin/oura-reader
make test               # Run all Go tests (go test ./...)
make lint               # Run go vet
make docker             # Build Docker image
make docker-run         # Start via docker compose
make docker-stop        # Stop via docker compose

# Run a single Go test
go test ./internal/crypto -run TestEncryptDecrypt

# Python client tests (from repo root)
source clients/python/.venv/bin/activate && python -m pytest clients/python/tests/ -v
```

## Architecture

Multi-user Go service that fetches Oura Ring health data via OAuth2, stores raw JSON in SQLite, and serves it over a REST API. A Python client (`clients/python/`) wraps the API for remote LLM-based analysis.

### Dependency Graph

All components are wired in `cmd/oura-reader/main.go` via constructor injection:

```
Config → Store (SQLite) → Cipher (AES-GCM)
                        → user.Manager
                        → oauth.Store → oauth.Manager
                        → oura.Client
                        → scheduler.Scheduler
                        → server.Server
```

No circular dependencies. Each package depends only on its explicit constructor arguments.

### Key Design Patterns

**Metadata-driven endpoint registry** (`internal/oura/endpoints.go`): All 18 Oura API endpoints are declared as `EndpointSpec` structs in a `Registry` array. The `Fetch()`, sync, and storage code is written once and driven by metadata (path, hasDate, idField, dayField). Adding an endpoint means adding one line to the registry — no handler code, no new routes.

**Multi-user context flow**: API key auth middleware (`internal/server/middleware.go`) hashes the bearer token, looks up the user, and injects `*user.User` into `r.Context()`. All handlers extract the user via `userFromContext(ctx)`. Data is scoped per-user via `user_id` foreign keys with `ON DELETE CASCADE`.

**Persisting token source** (`internal/oauth/manager.go`): Wraps `oauth2.ReuseTokenSource` with a `persistingTokenSource` that re-encrypts and saves tokens to SQLite on every refresh. Oura's refresh tokens are single-use, so each refresh must persist the new pair.

### Key Interfaces

| Interface | Defined in | Implemented by | Purpose |
|-----------|-----------|---------------|---------|
| `HTTPClientProvider` | `internal/oura/client.go` | `oauth.Manager` | Provides per-user authenticated HTTP clients |
| `UserLister` | `internal/scheduler/scheduler.go` | `user.Manager` | Lists users with OAuth tokens for scheduled sync |
| `TokenChecker` | `internal/scheduler/scheduler.go` | `oauth.Manager` | Checks if a user has stored tokens |

### Storage Model

Raw Oura API responses are stored as JSON blobs in `oura_data.data` (TEXT column). Identity columns (`user_id`, `endpoint`, `day`, `oura_id`) are extracted from the JSON at insert time using the endpoint's metadata fields. This avoids per-endpoint schema migrations while still allowing indexed queries. SQLite `json_extract()` is available for ad-hoc queries into the blob.

### Secrets

Config loading (`internal/config/config.go`) reads secrets from files first (`/run/secrets/<name>`), falls back to environment variables. `secretOrEnv()` handles this — file takes priority, whitespace is trimmed. The secrets directory is configurable via `OURA_SECRETS_DIR` (default `/run/secrets`). In Docker, `./secrets/` is mounted read-only to `/run/secrets/`.

### CLI Subcommands

The binary has two top-level commands: `serve` (starts HTTP server + scheduler) and `user` (add/list/rotate/remove). User management writes directly to SQLite — no running server required.

### Python Client

`clients/python/` — pip-installable `oura-reader-client` package using `httpx`. Has an auto-sync feature: each `get_*()` call checks `sync_status` and triggers a sync if stale (configurable threshold, default 1h).
