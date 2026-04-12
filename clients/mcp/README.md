# oura-reader-mcp

MCP server that exposes the [oura-reader](../../) REST API to AI agents over stdio.

## Install

```bash
uvx oura-reader-mcp          # ephemeral, recommended for MCP clients
# or
pipx install oura-reader-mcp # persistent install
```

## Configure your MCP client

Example `claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "oura": {
      "command": "uvx",
      "args": ["oura-reader-mcp"],
      "env": {
        "OURA_MCP_API_KEY": "your-oura-reader-api-key",
        "OURA_MCP_BASE_URL": "https://your-oura-reader-instance.example"
      }
    }
  }
}
```

### Env vars

| Name | Required | Default | Purpose |
|---|---|---|---|
| `OURA_MCP_API_KEY` | yes | — | oura-reader API key for the user |
| `OURA_MCP_BASE_URL` | yes | — | URL of the oura-reader instance |
| `OURA_MCP_TIMEOUT` | no | `30` | HTTP timeout in seconds |
| `OURA_MCP_LOG_LEVEL` | no | `info` | `debug` / `info` / `warning` / `error` (affects only `oura_mcp` namespace) |

## Tools

- 18 `get_*` tools, one per Oura endpoint (see `endpoints.py`)
- `sync` — trigger full sync (fire-and-forget)
- `sync_endpoint` — trigger sync for one endpoint
- `sync_status` — check freshness per endpoint

## Security

The API key is read once from env at startup and is never:
- exposed in any tool input schema
- included in tool results
- logged (a redacting filter is attached to every handler — see `server.py`)
- surfaced in error messages (catch-all returns generic strings only)

Design document: [`docs/specs/2026-04-12-oura-mcp-design.md`](../../docs/specs/2026-04-12-oura-mcp-design.md).

## Develop

```bash
python3 -m venv .venv && . .venv/bin/activate
pip install -e .[dev]
pytest -v
```

Registry parity with the Go service is enforced by `make check-endpoints` at the repo root.
