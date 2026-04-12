"""MCP server composition root.

Startup sequence, in order:
1. Load config (may SystemExit).
2. Install redacting filter on every handler and every named noisy logger.
3. Configure log levels: oura_mcp respects OURA_MCP_LOG_LEVEL; httpx, httpcore,
   mcp.*, anyio are pinned at WARNING.
4. Build MCP Server, register tools, wire handlers.
5. Run over stdio.

The SensitiveFilter is attached exactly once, before any log line is emitted.
No subsequent logging.config calls are permitted — a regression test verifies
this.
"""

from __future__ import annotations

import asyncio
import json
import logging
import sys
from typing import Any

from mcp.server import Server
from mcp.server.stdio import stdio_server
from mcp.types import TextContent, Tool

from oura_mcp.client import OuraReaderClient, UpstreamError
from oura_mcp.config import Config, load_config
from oura_mcp.tools import build_tool_definitions, dispatch


_NOISY_LOGGERS = ("httpx", "httpcore", "mcp", "anyio", "asyncio")
_logger = logging.getLogger("oura_mcp")

_TEST_CALL_TOOL_HANDLER = None  # set by build_server(); used only in tests


class SensitiveFilter(logging.Filter):
    """Redact the API key wherever it appears in a log record.

    Applied to every handler and every named noisy logger. The instance is
    constructed once with the key; records flowing through any handler with
    this filter attached get the key string replaced with '***REDACTED***'
    in their formatted message and args.
    """

    REDACTED = "***REDACTED***"

    def __init__(self, secret: str):
        super().__init__()
        self._secret = secret

    def filter(self, record: logging.LogRecord) -> bool:
        try:
            if isinstance(record.msg, str) and self._secret in record.msg:
                record.msg = record.msg.replace(self._secret, self.REDACTED)
            if record.args:
                record.args = tuple(self._redact(a) for a in record.args)  # type: ignore[assignment]
            # Pre-format the message so traceback text (exc_info) path also hits our scrub later.
            # exc_info is rendered lazily by the handler; we scrub via formatter injection below.
        except Exception:
            pass
        return True

    def _redact(self, value: Any) -> Any:
        if isinstance(value, str) and self._secret in value:
            return value.replace(self._secret, self.REDACTED)
        return value


class _RedactingFormatter(logging.Formatter):
    """Wraps a base formatter and redacts the final rendered string — including
    tracebacks formatted from exc_info, which SensitiveFilter can't scrub
    because they're rendered inside Formatter.format()."""

    def __init__(self, base: logging.Formatter, secret: str):
        super().__init__()
        self._base = base
        self._secret = secret

    def format(self, record: logging.LogRecord) -> str:
        rendered = self._base.format(record)
        return rendered.replace(self._secret, SensitiveFilter.REDACTED) if self._secret in rendered else rendered


def _install_redaction(secret: str, level: str) -> None:
    sensitive = SensitiveFilter(secret)
    base_formatter = logging.Formatter("%(asctime)s %(name)s %(levelname)s %(message)s")
    redacting_formatter = _RedactingFormatter(base_formatter, secret)

    # Logs go to stderr so stdio transport (stdout) stays pristine.
    handler = logging.StreamHandler(sys.stderr)
    handler.setFormatter(redacting_formatter)
    handler.addFilter(sensitive)

    # Attach filter to logging.lastResort too (defense in depth)
    if logging.lastResort is not None:
        logging.lastResort.addFilter(sensitive)

    # Configure our own namespace
    logging.getLogger("oura_mcp").setLevel(getattr(logging, level.upper()))
    logging.getLogger("oura_mcp").addHandler(handler)
    logging.getLogger("oura_mcp").addFilter(sensitive)
    logging.getLogger("oura_mcp").propagate = False

    # Pin noisy loggers at WARNING and attach filter (defense in depth even though
    # they rarely produce records containing the key; tracebacks from httpx could).
    for name in _NOISY_LOGGERS:
        noisy = logging.getLogger(name)
        noisy.setLevel(logging.WARNING)
        noisy.addFilter(sensitive)
        # Also attach the handler so any record emitted lands through our filter
        # even if propagation is disabled elsewhere.
        noisy.addHandler(handler)
        noisy.propagate = False

    # Root: catch anything that escapes.
    root = logging.getLogger()
    root.setLevel(logging.WARNING)
    root.addFilter(sensitive)
    if not any(isinstance(h, logging.StreamHandler) for h in root.handlers):
        root.addHandler(handler)


def _upstream_error_message(err: UpstreamError, tool_name: str, base_url: str) -> str:
    kind = err.kind
    if kind == "unauthorized":
        return "Authentication failed. Check OURA_MCP_API_KEY in the MCP client config."
    if kind == "forbidden":
        return (
            "Access denied, most likely because this user has not completed Oura OAuth. "
            "Visit /api/v1/auth/login on your oura-reader instance."
        )
    if kind == "bad_request":
        return f"Unknown or invalid request for tool '{tool_name}'."
    if kind == "rate_limited":
        return "Upstream rate-limited; retry in a few seconds."
    if kind == "upstream_error":
        return f"Upstream oura-reader error (status {err.status})."
    if kind == "transport":
        return f"Could not reach oura-reader at {base_url} (timeout/connection error)."
    return f"Upstream error handling '{tool_name}'."


def build_server(cfg: Config, client: OuraReaderClient) -> Server:
    server: Server = Server("oura-reader-mcp")
    tools = build_tool_definitions()

    @server.list_tools()
    async def _list_tools() -> list[Tool]:
        return tools

    @server.call_tool()
    async def _call_tool(name: str, arguments: dict[str, Any] | None) -> list[TextContent]:
        args = arguments or {}
        try:
            result = await dispatch(name, args, client)
            return [TextContent(type="text", text=json.dumps(result))]
        except UpstreamError as err:
            # Log operator-side detail; agent sees a generic mapped message
            _logger.warning("upstream error in tool %s: kind=%s status=%s", name, err.kind, err.status)
            return [TextContent(type="text", text=_upstream_error_message(err, name, cfg.base_url))]
        except KeyError:
            # Registry drift — should never happen if make check-endpoints is wired
            _logger.error("unknown tool name %s", name)
            return [TextContent(type="text", text=f"Unknown tool '{name}'.")]
        except Exception:  # noqa: BLE001 — this is the whole point
            # The one-and-only backstop. Never leak str(exc).
            _logger.exception("unexpected error in tool %s", name)
            return [TextContent(type="text", text=f"Internal error while handling {name}. Check server logs.")]

    # Test hook — allows the leak-guard test to invoke the handler directly
    # without a full stdio round-trip. No behavioral impact in production.
    import oura_mcp.server as _self
    _self._TEST_CALL_TOOL_HANDLER = _call_tool

    return server


async def _run() -> None:
    cfg = load_config()
    _install_redaction(cfg.api_key, cfg.log_level)
    _logger.info("oura-reader-mcp starting (base_url=%s)", cfg.base_url)

    client = OuraReaderClient(cfg)
    server = build_server(cfg, client)

    try:
        async with stdio_server() as (read_stream, write_stream):
            await server.run(read_stream, write_stream, server.create_initialization_options())
    finally:
        await client.close()


def main() -> None:
    try:
        asyncio.run(_run())
    except KeyboardInterrupt:
        pass


if __name__ == "__main__":
    main()
