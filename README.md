# oura-reader

A multi-user Go service that fetches health data from the Oura Ring API v2, stores it in SQLite, and exposes a REST API. Includes a Python client for remote access.

## Architecture

```
┌──────────────────┐         ┌─────────────────────────────────────────┐
│  Python Client   │  HTTP   │            oura-reader (Go)            │
│  (remote machine)│────────▶│                                         │
│  → feeds LLMs    │  API    │  HTTP API ← Scheduler ← OAuth2 Manager │
└──────────────────┘  key    │       │          │            │         │
                      auth   │       └──── Oura API Client ──┘         │
                             │                  │                      │
                             │              SQLite                     │
                             └─────────────────────────────────────────┘
```

## Prerequisites

1. **Oura Ring** with an active membership
2. **Oura Developer App** — register at [cloud.ouraring.com/oauth/applications](https://cloud.ouraring.com/oauth/applications)
   - Set the redirect URI to `http://<your-server-ip>:8080/api/v1/auth/callback`
   - Note your `client_id` and `client_secret`
3. **Docker** (recommended) or **Go 1.25+** for building from source

## Quick Start

### 1. Configure Secrets

Secrets are read from files in a `secrets/` directory (mounted into the container at `/run/secrets/`). This avoids storing secrets in environment variables or `.env` files.

```bash
mkdir -p secrets

# Get client_id and client_secret from cloud.ouraring.com/oauth/applications
echo "your_client_id" > secrets/oura_client_id
echo "your_client_secret" > secrets/oura_client_secret
openssl rand -hex 32 > secrets/oura_encryption_key

# Lock down permissions
chmod 600 secrets/*
```

Non-secret configuration goes in `.env` (optional):

```bash
cp .env.example .env
# Edit OURA_LISTEN_ADDR, OURA_FETCH_INTERVAL if needed
```

**Fallback:** If no secret files are found, the app falls back to environment variables (`OURA_CLIENT_ID`, `OURA_CLIENT_SECRET`, `OURA_ENCRYPTION_KEY`). Files take priority over env vars.

### 2. Build and Run

**With Docker (recommended):**

```bash
make docker
make docker-run
```

**From source:**

```bash
make build
./bin/oura-reader serve
```

### 3. Create Users

Each person needs their own user account with an API key.

```bash
# Docker
docker exec oura-reader-oura-reader-1 oura-reader user add --name "Ivan"

# Local
./bin/oura-reader user add --name "Ivan"
```

This prints an API key like `oura_ak_Xk9m2...`. Save it — it cannot be retrieved later.

### 4. Link Oura Account

Each user must authorize the app with their Oura account once. Open this URL in a browser (replace the API key):

```bash
curl -v -H "Authorization: Bearer oura_ak_..." http://localhost:8080/api/v1/auth/login
```

Or visit the login URL directly — it redirects to Oura's consent page. After authorization, the server stores encrypted OAuth tokens and handles refresh automatically.

### 5. Verify

```bash
# Health check (no auth)
curl http://localhost:8080/api/v1/health

# Check auth status
curl -H "Authorization: Bearer oura_ak_..." http://localhost:8080/api/v1/auth/status

# Check sync status
curl -H "Authorization: Bearer oura_ak_..." http://localhost:8080/api/v1/sync/status
```

## User Management

```bash
oura-reader user add --name "Ivan"       # Create user, prints API key
oura-reader user list                     # List all users
oura-reader user rotate --name "Ivan"     # Rotate API key (old key invalidated)
oura-reader user remove --name "Ivan"     # Delete user and all their data
```

API keys are stored as SHA-256 hashes — the raw key is only shown at creation and rotation.

## REST API

All endpoints except `/health` and `/auth/callback` require the `Authorization: Bearer <api_key>` header.

### Health

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/health` | Liveness check. Returns `{"status": "ok"}` |

### Authentication

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/auth/login` | Redirects to Oura OAuth consent page |
| GET | `/api/v1/auth/callback` | OAuth callback (called by Oura, no API key needed) |
| GET | `/api/v1/auth/status` | Returns `{authenticated, token_expiry}` |

### Sync

| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/v1/sync` | Trigger full sync of all endpoints |
| POST | `/api/v1/sync/{endpoint}` | Sync a single endpoint |
| GET | `/api/v1/sync/status` | Last sync time per endpoint |

### Data

| Method | Path | Query Params | Description |
|--------|------|--------------|-------------|
| GET | `/api/v1/data/{endpoint}` | `start_date`, `end_date`, `limit` (default 100), `offset` (default 0) | Fetch stored data for one endpoint |
| GET | `/api/v1/data` | `start_date`, `end_date`, `limit` (default 100) | Fetch all endpoints' data |

**Response format for single endpoint:**

```json
{
  "data": [...],
  "count": 42,
  "limit": 100,
  "offset": 0
}
```

## Available Data Endpoints

These are the Oura API v2 data categories you can query:

| Endpoint | Description |
|----------|-------------|
| `daily_sleep` | Daily sleep summary with score and contributors |
| `sleep` | Detailed sleep periods with stage breakdowns |
| `sleep_time` | Sleep time recommendations |
| `daily_activity` | Daily activity summary with score |
| `daily_readiness` | Readiness score, **temperature deviation**, and contributors |
| `heartrate` | Heart rate at 5-minute intervals |
| `daily_resilience` | Stress/resilience metrics |
| `daily_stress` | Daily stress levels |
| `daily_spo2` | Blood oxygen levels |
| `daily_cardiovascular_age` | Cardiovascular age estimate |
| `vo2_max` | VO2 max (cardio capacity) |
| `workout` | Workout sessions |
| `session` | Training/relaxation sessions |
| `tag` | User-defined tags |
| `enhanced_tag` | System-generated tags |
| `ring_configuration` | Ring hardware/firmware info |
| `rest_mode_period` | Rest mode episodes |
| `personal_info` | User profile (age, weight, height) |

### Temperature Data

Temperature is part of the `daily_readiness` response:

- `temperature_deviation` — deviation from your personal baseline in °C
- `temperature_trend_deviation` — 3-day weighted average trend in °C
- `contributors.body_temperature` — temperature contribution score (0-100)

## Python Client

The Python client library is in `clients/python/`. It wraps the REST API for use on remote machines (e.g., feeding data to LLMs).

### Install

```bash
cd clients/python
pip install .

# With dev dependencies (for running tests)
pip install ".[dev]"
```

### Usage

```python
from oura_reader import OuraClient

client = OuraClient(
    base_url="http://your-server:8080",
    api_key="oura_ak_...",
    stale_threshold=3600,  # auto-sync if last sync > 1 hour ago (0 to disable)
)

# Fetch data — triggers sync automatically if stale
sleep = client.get_sleep(start_date="2026-03-01", end_date="2026-03-23")
readiness = client.get_readiness(start_date="2026-03-20")
heartrate = client.get_heartrate(start_date="2026-03-22")
activity = client.get_activity(start_date="2026-03-20")
stress = client.get_stress(start_date="2026-03-20")
spo2 = client.get_spo2(start_date="2026-03-20")
workouts = client.get_workouts(start_date="2026-03-01")

# Fetch all endpoints at once
all_data = client.get_all(start_date="2026-03-20")

# Fetch any endpoint by name
vo2 = client.get_data("vo2_max", start_date="2026-03-01")

# Explicit sync
client.sync()                    # All endpoints
client.sync("daily_sleep")       # Single endpoint

# Status
client.auth_status()
client.sync_status()
client.health()
```

### Auto-Sync Behavior

Every `get_*()` call checks when the endpoint was last synced. If the data is older than `stale_threshold` (default: 1 hour), it triggers a sync before returning results. This is transparent — the caller always gets fresh-enough data.

Disable per-call with `auto_sync=False`:

```python
sleep = client.get_sleep(start_date="2026-03-20", auto_sync=False)
```

Or disable globally:

```python
client = OuraClient(base_url="...", api_key="...", stale_threshold=0)
```

### Context Manager

```python
with OuraClient(base_url="http://your-server:8080", api_key="oura_ak_...") as client:
    sleep = client.get_sleep(start_date="2026-03-20")
```

### All Client Methods

| Method | Returns | Description |
|--------|---------|-------------|
| `get_sleep(start_date, end_date)` | `list[dict]` | Daily sleep summaries |
| `get_detailed_sleep(start_date, end_date)` | `list[dict]` | Detailed sleep with stages |
| `get_activity(start_date, end_date)` | `list[dict]` | Daily activity |
| `get_readiness(start_date, end_date)` | `list[dict]` | Readiness + temperature |
| `get_heartrate(start_date, end_date)` | `list[dict]` | Heart rate intervals |
| `get_stress(start_date, end_date)` | `list[dict]` | Daily stress |
| `get_spo2(start_date, end_date)` | `list[dict]` | Blood oxygen |
| `get_workouts(start_date, end_date)` | `list[dict]` | Workouts |
| `get_data(endpoint, start_date, end_date)` | `list[dict]` | Any endpoint by name |
| `get_all(start_date, end_date)` | `dict[str, list]` | All endpoints |
| `sync(endpoint?)` | `dict` | Trigger sync |
| `sync_status()` | `dict` | Last sync times |
| `auth_status()` | `dict` | OAuth status |
| `health()` | `dict` | Server health |

## Configuration Reference

### Secrets (file-based, preferred)

Place secret files in `secrets/` (mounted to `/run/secrets/` in Docker):

| File | Description |
|------|-------------|
| `secrets/oura_client_id` | Oura OAuth2 client ID |
| `secrets/oura_client_secret` | Oura OAuth2 client secret |
| `secrets/oura_encryption_key` | AES-256 encryption key for OAuth tokens at rest |

Files take priority over environment variables. Whitespace is trimmed.

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `OURA_CLIENT_ID` | — | Fallback if secret file not found |
| `OURA_CLIENT_SECRET` | — | Fallback if secret file not found |
| `OURA_ENCRYPTION_KEY` | — | Fallback if secret file not found |
| `OURA_DB_PATH` | `data/oura.db` | SQLite database file path |
| `OURA_LISTEN_ADDR` | `0.0.0.0:8080` | Server bind address |
| `OURA_FETCH_INTERVAL` | `6h` | Periodic background sync interval |
| `OURA_SECRETS_DIR` | `/run/secrets` | Override secret files directory |

## Security

- **Secrets** are read from files, not environment variables — avoids leaking via `/proc`, `docker inspect`, or logs
- **API keys** are hashed with SHA-256 before storage — raw keys are never persisted
- **OAuth tokens** are encrypted at rest with AES-256-GCM
- The encryption key is derived from `OURA_ENCRYPTION_KEY` via SHA-256
- The `secrets/` directory is gitignored and should be `chmod 600`
- The server binds to `0.0.0.0` by default for LAN access — restrict with `OURA_LISTEN_ADDR` if needed

## Make Targets

```
make build        Build the binary to bin/oura-reader
make run          Build and run the server
make test         Run all Go tests
make lint         Run go vet
make clean        Remove build artifacts
make docker       Build Docker image
make docker-run   Start via docker compose
make docker-stop  Stop via docker compose
make help         Show available targets
```
