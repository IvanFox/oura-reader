# oura-reader-mcp

MCP server that exposes the [oura-reader](../../) REST API to AI agents over stdio.

## Prerequisites

Before configuring any MCP client you need a running oura-reader instance with a user account that has completed Oura OAuth.

### 1. Deploy oura-reader

Follow [docs/DEPLOYMENT.md](../../docs/DEPLOYMENT.md). At the end you should have:

- A reachable HTTPS URL, e.g. `https://your-server.tail1234.ts.net`
- The container running (`docker compose ps` shows it up)

### 2. Create a user and get an API key

```bash
# Docker
docker exec oura-reader-oura-reader-1 oura-reader user add --name "YourName"

# Local binary
./bin/oura-reader user add --name "YourName"
```

The command prints a one-time API key: `oura_ak_Xk9m2...`. Save it — it cannot be retrieved again.

### 3. Authorize with Oura

The user must complete the OAuth flow once before the MCP server can return any data. From a device on the same network as oura-reader:

```bash
curl -v -H "Authorization: Bearer oura_ak_..." \
  https://your-server.tail1234.ts.net/api/v1/auth/login
```

Open the `Location` URL from the response in a browser, approve the Oura consent screen, and wait for the callback to complete.

Confirm it worked:

```bash
curl -H "Authorization: Bearer oura_ak_..." \
  https://your-server.tail1234.ts.net/api/v1/auth/status
# → {"authenticated": true, ...}
```

---

## Install the MCP server

```bash
uvx oura-reader-mcp          # ephemeral — recommended for MCP clients
# or
pipx install oura-reader-mcp # persistent install
```

---

## Configure your MCP client

The MCP server is a subprocess launched by the AI client over stdio. You configure it by pointing the client at the `uvx` (or `pipx`) entry-point and injecting two required env vars: `OURA_MCP_API_KEY` and `OURA_MCP_BASE_URL`.

### Claude Desktop

Config file locations:
- **macOS**: `~/Library/Application Support/Claude/claude_desktop_config.json`
- **Windows**: `%APPDATA%\Claude\claude_desktop_config.json`

```json
{
  "mcpServers": {
    "oura": {
      "command": "uvx",
      "args": ["oura-reader-mcp"],
      "env": {
        "OURA_MCP_API_KEY": "oura_ak_...",
        "OURA_MCP_BASE_URL": "https://your-server.tail1234.ts.net"
      }
    }
  }
}
```

Restart Claude Desktop after saving. The "oura" server should appear in the MCP integrations list (hammer icon).

### Claude Code (CLI)

Add to `~/.claude/settings.json` for a global install, or `.claude/settings.json` inside a project for project-scoped access:

```json
{
  "mcpServers": {
    "oura": {
      "command": "uvx",
      "args": ["oura-reader-mcp"],
      "env": {
        "OURA_MCP_API_KEY": "oura_ak_...",
        "OURA_MCP_BASE_URL": "https://your-server.tail1234.ts.net"
      }
    }
  }
}
```

Verify the server is visible:

```bash
claude mcp list
```

### Cursor

Add to `~/.cursor/mcp.json` (global) or `.cursor/mcp.json` (project-scoped):

```json
{
  "mcpServers": {
    "oura": {
      "command": "uvx",
      "args": ["oura-reader-mcp"],
      "env": {
        "OURA_MCP_API_KEY": "oura_ak_...",
        "OURA_MCP_BASE_URL": "https://your-server.tail1234.ts.net"
      }
    }
  }
}
```

Restart Cursor and confirm the server appears under **Settings → MCP**.

---

## Env vars reference

| Name | Required | Default | Purpose |
|---|---|---|---|
| `OURA_MCP_API_KEY` | yes | — | oura-reader API key for this user |
| `OURA_MCP_BASE_URL` | yes | — | Base URL of the oura-reader instance (no trailing slash) |
| `OURA_MCP_TIMEOUT` | no | `30` | HTTP timeout in seconds |
| `OURA_MCP_LOG_LEVEL` | no | `info` | `debug` / `info` / `warning` / `error` — controls the `oura_mcp` logger only |

`OURA_MCP_LOG_LEVEL=debug` increases verbosity of the MCP server's own logs. It does **not** enable httpx request logging or MCP SDK frame logging — those are held at `WARNING` regardless, because they can contain auth headers or large data payloads.

---

## Tools

### Data tools (18)

One tool per Oura endpoint, named `get_<endpoint>`. All return the raw JSON from oura-reader.

| Tool | Parameters |
|---|---|
| `get_daily_sleep` | `start_date?`, `end_date?`, `limit?` |
| `get_sleep` | same |
| `get_sleep_time` | same |
| `get_daily_activity` | same |
| `get_daily_readiness` | same |
| `get_heartrate` | same |
| `get_daily_resilience` | same |
| `get_daily_stress` | same |
| `get_daily_spo2` | same |
| `get_daily_cardiovascular_age` | same |
| `get_vo2_max` | same |
| `get_workout` | same |
| `get_session` | same |
| `get_tag` | same |
| `get_enhanced_tag` | same |
| `get_ring_configuration` | `limit?` |
| `get_rest_mode_period` | `start_date?`, `end_date?`, `limit?` |
| `get_personal_info` | — |

Dates use `YYYY-MM-DD` format. `limit` defaults to 100 server-side.

### Meta tools

| Tool | Parameters | Description |
|---|---|---|
| `sync` | — | Trigger a full sync across all endpoints. Fire-and-forget — poll `sync_status` for progress. |
| `sync_endpoint` | `endpoint` (enum) | Trigger a sync for one endpoint. |
| `sync_status` | — | Return per-endpoint last-sync time and any error state. |

---

## Security

The API key is read once from env at startup and is never:
- exposed in any tool input schema
- included in tool results
- logged (a redacting filter covers every log handler — see `server.py`)
- surfaced in error messages (all tool handlers use a catch-all that returns generic strings)

Design document: [`docs/specs/2026-04-12-oura-mcp-design.md`](../../docs/specs/2026-04-12-oura-mcp-design.md).

---

## Troubleshooting

**"OURA_MCP_API_KEY is not set" on stderr**
The MCP client is not passing the env vars. Check that the `env` block is inside the correct `mcpServers` entry and restart the client.

**`sync_status` returns errors for all endpoints**
The user has not completed OAuth. Re-run the `auth/login` step from Prerequisites §3.

**Connection timeout / "Could not reach oura-reader"**
The machine running the MCP client cannot reach `OURA_MCP_BASE_URL`. If oura-reader is on Tailscale, confirm the client machine is on the same tailnet and the `tailscale serve` proxy is active.

**"Authentication failed. Check OURA_MCP_API_KEY"**
The API key is wrong or has been rotated. Re-run `oura-reader user rotate --name "..."` to get a new key, then update the MCP client config.

**MCP server doesn't appear in the client**
Run the server manually to check for import errors:

```bash
OURA_MCP_API_KEY=test OURA_MCP_BASE_URL=http://localhost:9999 uvx oura-reader-mcp
```

The process should start and block on stdin. Any startup errors (missing deps, Python version) will appear on stderr.

---

## Develop

```bash
cd clients/mcp
python3 -m venv .venv && . .venv/bin/activate
pip install -e .[dev]
pytest -v
```

Registry parity with the Go service is enforced by `make check-endpoints` at the repo root.
