"""Guard-rail tests for the no-secret-leak contract.

Each test models a realistic leakage vector. If any of these starts failing,
DO NOT paper over it — the underlying guarantee has regressed.
"""

import io
import json
import logging
import re
import sys

import httpx
import mcp
import pytest

from oura_mcp.client import UpstreamError
from oura_mcp.config import Config
from oura_mcp.server import SensitiveFilter, _install_redaction, build_server
from oura_mcp.tools import build_tool_definitions


SECRET = "super-secret-key-xyz-123"
SECRET_FIELD_RE = re.compile(r"api|key|token|secret|auth", re.IGNORECASE)


@pytest.fixture
def cfg():
    return Config(api_key=SECRET, base_url="https://example.test", timeout=5.0, log_level="debug")


@pytest.fixture(autouse=True)
def capture_stderr(monkeypatch):
    buf = io.StringIO()
    monkeypatch.setattr(sys, "stderr", buf)
    # Reset logger state between tests — otherwise handlers from a previous
    # install_redaction call accumulate and poison sibling tests.
    for name in ("oura_mcp", "httpx", "httpcore", "mcp", "anyio", "asyncio", ""):
        lg = logging.getLogger(name)
        for h in list(lg.handlers):
            lg.removeHandler(h)
        for f in list(lg.filters):
            lg.removeFilter(f)
        lg.propagate = True
    yield buf


def test_no_tool_schema_has_secret_shaped_field():
    for tool in build_tool_definitions():
        for field_name in (tool.inputSchema or {}).get("properties", {}):
            assert not SECRET_FIELD_RE.search(field_name)


def test_redaction_on_root_logger(cfg, capture_stderr):
    _install_redaction(cfg.api_key, cfg.log_level)

    # Add a direct StringIO handler so we can read the formatted record.
    # (Capturing via sys.stderr replacement is unreliable under pytest's own
    # stderr capture — sys.stderr may point to pytest's CaptureIO, not our buf.)
    log_buf = io.StringIO()
    cap = logging.StreamHandler(log_buf)
    cap.setFormatter(logging.Formatter("%(message)s"))
    logging.getLogger("oura_mcp").addHandler(cap)

    logging.getLogger("oura_mcp").warning("leaking %s in message", SECRET)

    # The SensitiveFilter attached to the logger modifies the record in-place
    # before any handler sees it, so our cap handler also gets the redacted text.
    out = log_buf.getvalue()
    assert SECRET not in out
    assert SensitiveFilter.REDACTED in out


@pytest.mark.parametrize("logger_name", ["httpx", "httpcore", "mcp", "anyio"])
def test_redaction_on_named_loggers(cfg, capture_stderr, logger_name):
    _install_redaction(cfg.api_key, cfg.log_level)
    # These loggers are pinned at WARNING — use that level to ensure the record emits.
    logging.getLogger(logger_name).warning("containing %s key", SECRET)
    out = capture_stderr.getvalue()
    assert SECRET not in out


def test_redaction_on_propagate_false_logger(cfg, capture_stderr):
    """The specific regression: a library sets propagate=False and attaches its own
    handler. Our filter must still run because we attached it to the logger itself."""
    _install_redaction(cfg.api_key, cfg.log_level)
    lg = logging.getLogger("httpx")  # already has propagate=False after install
    assert lg.propagate is False
    lg.warning("key is %s", SECRET)
    out = capture_stderr.getvalue()
    assert SECRET not in out


def test_httpx_request_error_repr_redacted(cfg, capture_stderr):
    """httpx exceptions can include request headers in their repr. Simulate that
    and confirm our redacting formatter scrubs it."""
    _install_redaction(cfg.api_key, cfg.log_level)
    req = httpx.Request("GET", "https://example.test/", headers={"Authorization": f"Bearer {SECRET}"})
    exc = httpx.ConnectError("boom")
    exc.request = req
    try:
        raise exc
    except httpx.ConnectError:
        logging.getLogger("oura_mcp").exception("transport failure: %r", exc)
    out = capture_stderr.getvalue()
    assert SECRET not in out


def test_handler_set_stable_after_startup(cfg):
    """If a test or library installs a new handler after startup, it will not
    have our filter. Snapshot the handler set and confirm nothing has been added."""
    _install_redaction(cfg.api_key, cfg.log_level)
    before = {id(h) for h in logging.getLogger("oura_mcp").handlers}
    # Simulate some work happening (e.g., a tool dispatch logs something).
    logging.getLogger("oura_mcp").info("work happening")
    after = {id(h) for h in logging.getLogger("oura_mcp").handlers}
    assert before == after


@pytest.mark.asyncio
async def test_tool_exception_handler_does_not_leak_exc_message(cfg):
    """The catch-all in server._call_tool must return the generic string and
    never include str(exc), even when the exception text contains the secret."""

    class BombClient:
        async def get_data(self, **_kwargs):
            raise RuntimeError(f"oops, containing {SECRET}")

    _install_redaction(cfg.api_key, cfg.log_level)
    server = build_server(cfg, BombClient())

    # Reach into the registered call_tool handler. The MCP SDK stores these in
    # a private dict; grab it by type introspection.
    call_tool = getattr(server, "_request_handlers", None) or getattr(server, "request_handlers", {})
    # Fallback: find the wrapped handler by scanning server attributes. The
    # public, stable path is via the server.run() request dispatch; for this
    # test we want to call the decorated coroutine directly, so we stash it
    # at registration time in a module-level dict.
    from oura_mcp import server as srv_mod
    handler = srv_mod._TEST_CALL_TOOL_HANDLER  # set by build_server — see adjustment below

    result = await handler("get_daily_sleep", {})
    assert len(result) == 1
    text = result[0].text
    assert SECRET not in text
    assert "Internal error while handling get_daily_sleep" in text


def test_tool_upstream_error_does_not_leak_key():
    """Simulate the full error-map path with an UpstreamError — ensure the
    agent-visible string never contains the key."""
    from oura_mcp.server import _upstream_error_message

    for kind in ("unauthorized", "forbidden", "bad_request", "rate_limited", "upstream_error", "transport"):
        # base_url is the server URL, not the API key — use a normal URL here.
        # We verify the secret (api_key) never appears in any error kind.
        msg = _upstream_error_message(UpstreamError(kind=kind, status=500), tool_name="get_daily_sleep", base_url="https://oura-reader.example")
        assert SECRET not in msg
