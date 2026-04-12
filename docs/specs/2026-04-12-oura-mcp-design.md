# Oura MCP Server — Design

**Date:** 2026-04-12
**Status:** Draft — revised after review

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
2. MCP builds `GET /api/v1/data/{endpoint}?start_date=...&end_date=...&limit=...` with `Authorization: Bearer <OURA_MCP_API_KEY>` sourced from env. Query-param names (`start_date`, `end_date`, `limit`, `offset`) are snake_case, matching the REST API's handlers exactly.
3. The JSON body of the REST response is returned as the tool result, untouched.

**Environment isolation.** The MCP process's HTTP calls carry only the `Authorization` header plus standard httpx defaults — no environ pass-through, no shell invocation, no subprocess spawning after startup.

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
| `sync` | — | `POST /api/v1/sync` — triggers full sync; fire-and-forget |
| `sync_endpoint` | `endpoint: enum(18 names)` | `POST /api/v1/sync/{endpoint}` |
| `sync_status` | — | `GET /api/v1/sync/status` — agent polls this after calling `sync` |

**Why one `sync_endpoint` with an enum, not 18 sync tools.** Reads are the hot path agents reach for constantly and benefit from named, typed tools. Syncs are rare administrative actions triggered occasionally; collapsing them into one parametric tool keeps the surface small where granularity doesn't help. `list_endpoints` was considered and cut — the MCP `tools/list` call already enumerates the 18 `get_*` tools, so it would duplicate static data.

### Endpoint metadata parity

`endpoints.py` is a hand-written Python mirror of the Go `Registry` in `internal/oura/endpoints.go`. CI enforces parity via a `make check-endpoints` target. The diff contract is narrow — only the fields the MCP actually uses must match:

- `Name` (exact match)
- `HasDates` (controls whether the generated tool exposes `start_date`/`end_date`)
- `IsList` (controls whether `limit` is exposed)
- Whether `DayField == ""` (boolean; the concrete value doesn't matter to the MCP)

`Path`, `IDField`, and the exact value of `DayField` are not checked — they are internal to the Go service and irrelevant to the MCP surface. Code generation from the Go source is more machinery than a ~20-line table warrants.

## 4. Configuration & secrets

### Environment variables

| Name | Required | Purpose |
|---|---|---|
| `OURA_MCP_API_KEY` | yes | Per-user API key, sent as `Authorization: Bearer <key>` |
| `OURA_MCP_BASE_URL` | yes | Where oura-reader is reachable, e.g. `https://oura.example.com` |
| `OURA_MCP_TIMEOUT` | no | HTTP timeout in seconds (default `30`) |
| `OURA_MCP_LOG_LEVEL` | no | `debug` \| `info` \| `warning` \| `error` (default `info`) |

`OURA_MCP_LOG_LEVEL` affects only the `oura_mcp` logger namespace. It does NOT enable the MCP SDK's internal JSON-RPC frame logging, does NOT change `httpx`/`httpcore` log levels, and does NOT enable `mcp.*` debug logs. Those loggers are held at `WARNING` regardless of this setting. The rationale is that JSON-RPC frames contain full tool results (potentially large health payloads), and httpx request reprs can include the `Authorization` header — neither should be logged even in debug mode.

All env vars are read at startup from `os.environ` only. No `.env` file support — that's the MCP client's job.

### Startup validation

If `OURA_MCP_API_KEY` or `OURA_MCP_BASE_URL` is missing, the process exits non-zero with a clear stderr message. The MCP client surfaces this to the user so they can fix their config. No defaults, no fallbacks.

### No-leak rules (enforced in code)

1. **No secret fields in any tool schema.** Unit test walks every registered tool and asserts its input schema contains no keys matching `/api|key|token|secret|auth/i`.
2. **No secret in logs, covering all handlers.** A `SensitiveFilter` redacts any occurrence of the API-key string to `***REDACTED***`. Critically, Python logger propagation cannot be relied on — many libraries set `propagate=False` or attach their own handlers. The filter is therefore attached to **every handler** installed by the process (including `logging.lastResort`), and additionally installed on the known noisy loggers by name: `httpx`, `httpcore`, `mcp`, `anyio`, `asyncio`, plus the root logger. The filter is called once at process startup before any log line is emitted.
3. **No secret in exception stringification.** The real leak vector is an `httpx.RequestError` whose `repr()` can include request headers. All tool handlers wrap their body in a top-level `try/except Exception` that:
   - Logs `exc` via `logger.exception(...)` (goes through the redacting filter).
   - Returns to the agent only a generic `"Internal error while handling <tool_name>. Check server logs."` isError result — never `str(exc)`, never `repr(exc)`, never a traceback.
4. **No secret in HTTP error responses.** HTTP error handler builds the agent-visible error string from `status_code` plus a generic description only — never from raw response headers or bodies (some proxies echo the `Authorization` header). Note: `SensitiveFilter` protects log records; it does not run over tool-result text. Error-message safety is therefore enforced by construction (generic strings), not by post-hoc redaction.

6. **No post-startup logging reconfiguration.** The filter-install step runs once in `server.py`'s startup. After that, no further `logging.config` calls, no new handlers added, no third-party logging setup. A regression test (`test_no_leak.py`) asserts that the set of handlers-with-filter attached after startup equals the set at any later point during a simulated tool call.
5. **No secret in tool results.** Tool results are the JSON body of the REST response; oura-reader does not echo auth headers. The redacting filter runs as a defense-in-depth backstop, and tool-result serialization is audited against the key string in `test_no_leak.py`.

### Sample MCP-client config

```json
{
  "mcpServers": {
    "oura": {
      "command": "uvx",
      "args": ["oura-reader-mcp"],
      "env": {
        "OURA_MCP_API_KEY": "ork_live_...",
        "OURA_MCP_BASE_URL": "https://oura.example.com"
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
| Missing/invalid API key (`401`) | `"Authentication failed. Check OURA_MCP_API_KEY in the MCP client config."` |
| Forbidden (`403`) | `"Access denied, most likely because this user has not completed Oura OAuth. Visit /api/v1/auth/login on your oura-reader instance."` — all 403s map to this message; we do not sniff the response body |
| Unknown endpoint (`400`) | `"Unknown endpoint '<name>'."` — only reachable if `endpoints.py` drifts from Go Registry |
| Rate-limited (`429`) | `"Upstream rate-limited; retry in a few seconds."` |
| Server error (`5xx`) | `"Upstream oura-reader error (status <code>)."` |
| Network/timeout | `"Could not reach oura-reader at <base_url> (timeout/connection error)."` |
| Unexpected Python exception | `"Internal error while handling <tool_name>. Check server logs."` (see no-leak rule #3) |
| Model-supplied arg validation | Rejected by MCP SDK before our handler runs |

Upstream response bodies, headers, and stack traces are never surfaced to the agent. Operator-side detail is logged at `debug` level via the `oura_mcp` namespace only; `httpx`/`httpcore`/`mcp.*` loggers remain at WARNING regardless of `OURA_MCP_LOG_LEVEL` (see §4). This means "debug" reveals only what the MCP's own code emits — never JSON-RPC frames or httpx request reprs. All emissions pass through the redacting filter.

### Tests

- **Config** (`test_config.py`): missing env vars exit non-zero with clear stderr; whitespace in key is trimmed; no default values.
- **Tool schemas** (`test_tools.py`): all 18 endpoint tools registered; no secret-shaped input fields (regex assertion); date-free tools (`ring_configuration`, `personal_info`) don't expose date params; `sync_endpoint`'s `endpoint` enum values are computed from `endpoints.py` at test time (not hardcoded), so drift in the registry automatically updates the test.
- **HTTP client** (`test_client.py`, via `respx`): `Authorization: Bearer <key>` is sent; `401/403/429/5xx` map to the correct agent-visible strings; timeout mapped correctly; response JSON passes through unmodified.
- **Leak guards** (`test_no_leak.py`):
  - Logger redaction is applied to every handler (not relying on propagation). Verified by logging the key via `httpx`, `httpcore`, `mcp`, and the root logger; all outputs contain `***REDACTED***`.
  - Additional test: a logger configured with `propagate=False` and its own handler also produces redacted output — the specific regression this design guards against.
  - Handler-set regression test: snapshots the attached handlers at startup and again during a simulated tool call; asserts no new handlers appear without the filter.
  - Stringifying an `httpx.RequestError` built from a request with the `Authorization` header set, then logging it, produces a redacted log line.
  - The top-level exception handler in every tool returns the generic `"Internal error..."` string and nothing derived from the exception — verified by raising a `ValueError(f"contains {key}")` inside a handler and asserting the tool's returned content is exactly the generic message.
  - No registered tool's input schema contains a secret-shaped field.
- **Integration** (`test_integration.py`): end-to-end against a real oura-reader instance hitting `sync_status`. Skipped unless env is set.
- **Registry parity**: `make check-endpoints` diffs `endpoints.py` against `internal/oura/endpoints.go` in CI; fails on drift.

### Out of scope

- No MCP conformance suite of our own — we trust the official SDK.
- No load testing — this is a single-user stdio process.
