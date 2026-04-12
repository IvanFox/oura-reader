# Oura MCP Server — Design

**Date:** 2026-04-12
**Status:** Draft — pending review

## 1. Overview & goals

Expose the oura-reader REST API to AI agents through a Model Context Protocol (MCP) server. The MCP is a standalone Python process, launched as a subprocess by an MCP client (Claude Desktop, Cursor, etc.) over stdio. Each process serves exactly one user: the user's `oura-reader` API key is injected via env var from the MCP client's config. The model never sees, handles, or can exfiltrate the key.

**Goals**
- AI agents can read all 18 Oura data endpoints via named, discoverable tools.
- Agents can trigger a sync and check freshness on demand.
- Secrets live in MCP-client config only. They are never in tool arguments, tool results, logs, or error messages.
- Tool surface mirrors the REST API 1:1 — no new semantic layer to keep in sync.

**Non-goals**
- HTTP/SSE transport for remote multi-agent access.
- Multi-user sessions within a single MCP process.
- Auto-sync on read (explicitly rejected; syncs must be explicit).
- Response transformation or aggregation beyond what the REST API returns.

## 2. Architecture

```
┌────────────────────┐   stdio (JSON-RPC)   ┌──────────────────────┐   HTTPS   ┌────────────────┐
│  AI agent / client │ ────────────────────▶│  oura-mcp (Python)   │ ─────────▶│  oura-reader   │
│  (Claude Desktop,  │                      │  - reads env vars    │           │  REST API      │
│   Cursor, etc.)    │ ◀────────────────────│  - 18 data tools     │ ◀─────────│                │
└────────────────────┘                      │  - sync/status tools │           └────────────────┘
                                            │  - stateless         │
                                            └──────────────────────┘
```

**Process model.** Short-lived subprocess launched per MCP client session. Stateless: no daemon, no local DB, no cached tokens. Exits when the client closes the stdio stream.

**Data flow per tool call**
1. Client sends `tools/call` over stdio with endpoint name plus optional `start_date`, `end_date`, `limit`.
2. MCP builds `GET /api/v1/data/{endpoint}?start_date=...&end_date=...&limit=...` with `Authorization: Bearer <OURA_READER_API_KEY>` sourced from env.
3. The JSON body of the REST response is returned as the tool result, untouched.

**Why this is safe against secret leakage**
- The API key is read once at startup from `os.environ`, held in a module-level variable, and never serialized into any tool result, error, or log line.
- Tool input schemas have no `api_key`/`token`/`auth` field — the model literally cannot pass one.
- Errors from the upstream API are sanitized (Section 5) before being surfaced to the agent.
- stdio transport means the key never transits a socket the model can introspect.

**Dependencies.** `mcp` (official SDK), `httpx`, `pydantic`. We do NOT depend on the existing `oura-reader-client` Python package, to avoid inheriting its auto-sync behavior. The HTTP layer is ~30 lines of thin httpx code.

## 3. Package layout & tool inventory

New sibling to `clients/python/`:

```
clients/mcp/
├── pyproject.toml              # package: oura-reader-mcp, entry: oura-reader-mcp
├── README.md                   # install + MCP-client config examples
├── src/oura_mcp/
│   ├── __init__.py
│   ├── __main__.py             # python -m oura_mcp
│   ├── server.py               # MCP server setup, tool registration loop
│   ├── config.py               # env-var loading + validation
│   ├── client.py               # httpx wrapper: get_data, sync, sync_status
│   ├── endpoints.py            # mirrors Registry from internal/oura/endpoints.go
│   └── tools.py                # tool generator: one per endpoint, plus meta tools
└── tests/
    ├── test_config.py
    ├── test_tools.py
    ├── test_client.py
    ├── test_no_leak.py
    └── test_integration.py     # skipped unless env configured
```

### Data tools (18, auto-generated from `endpoints.py`)

| Tool | Args | Notes |
|---|---|---|
| `get_daily_sleep` | `start_date?`, `end_date?`, `limit?` | |
| `get_sleep` | same | |
| `get_sleep_time` | same | |
| `get_daily_activity` | same | |
| `get_daily_readiness` | same | |
| `get_heartrate` | same | timestamp-keyed |
| `get_daily_resilience` | same | |
| `get_daily_stress` | same | |
| `get_daily_spo2` | same | |
| `get_daily_cardiovascular_age` | same | |
| `get_vo2_max` | same | |
| `get_workout` | same | |
| `get_session` | same | |
| `get_tag` | same | |
| `get_enhanced_tag` | same | |
| `get_ring_configuration` | `limit?` | no date params |
| `get_rest_mode_period` | same as dated | keyed on `start_day` |
| `get_personal_info` | — | single object |

Dates use `YYYY-MM-DD` format. No relative/period shortcuts — the model can compute ranges from today.

### Meta tools

| Tool | Args | Behaviour |
|---|---|---|
| `sync` | — | `POST /api/v1/sync` — triggers full sync |
| `sync_endpoint` | `endpoint: enum(18 names)` | `POST /api/v1/sync/{endpoint}` |
| `sync_status` | — | `GET /api/v1/sync/status` |
| `list_endpoints` | — | returns the 18 names and short descriptions |

### Endpoint metadata parity

`endpoints.py` is a hand-written Python mirror of the Go `Registry` in `internal/oura/endpoints.go`. CI enforces parity via a `make check-endpoints` target that diffs the two representations and fails on drift. Code generation from the Go source is more machinery than a ~20-line table warrants.

## 4. Configuration & secrets

### Environment variables

| Name | Required | Purpose |
|---|---|---|
| `OURA_READER_API_KEY` | yes | Per-user API key, sent as `Authorization: Bearer <key>` |
| `OURA_READER_BASE_URL` | yes | Where oura-reader is reachable, e.g. `https://oura.example.com` |
| `OURA_READER_TIMEOUT` | no | HTTP timeout in seconds (default `30`) |
| `OURA_MCP_LOG_LEVEL` | no | `debug` \| `info` \| `warning` \| `error` (default `info`) |

All env vars are read at startup from `os.environ` only. No `.env` file support — that's the MCP client's job.

### Startup validation

If `OURA_READER_API_KEY` or `OURA_READER_BASE_URL` is missing, the process exits non-zero with a clear stderr message. The MCP client surfaces this to the user so they can fix their config. No defaults, no fallbacks.

### No-leak rules (enforced in code)

1. **No secret fields in any tool schema.** Unit test walks every registered tool and asserts its input schema contains no keys matching `/api|key|token|secret|auth/i`.
2. **No secret in logs.** A `SensitiveFilter` is attached to the root logger; it redacts any occurrence of the API-key string to `***REDACTED***` before handlers see the record.
3. **No secret in error responses.** HTTP error handler builds the agent-visible error string from `status_code` plus a generic description — never from raw response headers or bodies (some proxies echo the `Authorization` header).
4. **No secret in tool results.** Tool results are the JSON body of the REST response; oura-reader does not echo auth headers. The log-filter redactor runs over tool results as a defense-in-depth backstop.

### Sample MCP-client config

```json
{
  "mcpServers": {
    "oura": {
      "command": "uvx",
      "args": ["oura-reader-mcp"],
      "env": {
        "OURA_READER_API_KEY": "ork_live_...",
        "OURA_READER_BASE_URL": "https://oura.example.com"
      }
    }
  }
}
```

The key lives in the user's local config file. It never appears in any tool arg and never transits to the model.

## 5. Error handling & testing

### Agent-visible error surface

Each tool returns either a successful result or a structured `isError: true` content block with a short, safe message. The model sees enough to react, never enough to leak.

| Upstream condition | Tool result |
|---|---|
| Missing/invalid API key (`401`) | `"Authentication failed. Check OURA_READER_API_KEY in the MCP client config."` |
| User not OAuth-connected (`403`/specific body) | `"The configured user has not completed Oura OAuth. Visit /api/v1/auth/login."` |
| Unknown endpoint (`400`) | `"Unknown endpoint '<name>'."` — only reachable if `endpoints.py` drifts from Go Registry |
| Rate-limited (`429`) | `"Upstream rate-limited; retry in a few seconds."` |
| Server error (`5xx`) | `"Upstream oura-reader error (status <code>)."` |
| Network/timeout | `"Could not reach oura-reader at <base_url> (timeout/connection error)."` |
| Model-supplied arg validation | Rejected by MCP SDK before our handler runs |

Upstream response bodies, headers, and stack traces are never surfaced to the agent. Full details are logged at `debug` level for the operator, always with redaction applied.

### Tests

- **Config** (`test_config.py`): missing env vars exit non-zero with clear stderr; whitespace in key is trimmed; no default values.
- **Tool schemas** (`test_tools.py`): all 18 endpoint tools registered; no secret-shaped input fields (regex assertion); date-free tools (`ring_configuration`, `personal_info`) don't expose date params; `sync_endpoint`'s `endpoint` arg is an enum of exactly the 18 names.
- **HTTP client** (`test_client.py`, via `respx`): `Authorization: Bearer <key>` is sent; `401/403/429/5xx` map to the correct agent-visible strings; timeout mapped correctly; response JSON passes through unmodified.
- **Leak guards** (`test_no_leak.py`): log-redaction works; error path with key-in-upstream-body does not surface the key; no tool schema allows a secret-shaped field.
- **Integration** (`test_integration.py`): end-to-end against a real oura-reader instance hitting `sync_status`. Skipped unless env is set.
- **Registry parity**: `make check-endpoints` diffs `endpoints.py` against `internal/oura/endpoints.go` in CI; fails on drift.

### Out of scope

- No MCP conformance suite of our own — we trust the official SDK.
- No load testing — this is a single-user stdio process.
