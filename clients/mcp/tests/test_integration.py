"""End-to-end test against a real oura-reader instance.

Skipped unless OURA_MCP_TEST_BASE_URL and OURA_MCP_TEST_API_KEY are set.
The test calls sync_status via the tool dispatch path — it exercises the
full httpx → oura-reader round trip, but does not trigger any Oura API
call (sync_status is local to oura-reader).
"""

import os

import pytest

from oura_mcp.client import OuraReaderClient
from oura_mcp.config import Config
from oura_mcp.tools import dispatch


pytestmark = pytest.mark.skipif(
    not (os.environ.get("OURA_MCP_TEST_BASE_URL") and os.environ.get("OURA_MCP_TEST_API_KEY")),
    reason="requires OURA_MCP_TEST_BASE_URL and OURA_MCP_TEST_API_KEY",
)


@pytest.mark.asyncio
async def test_sync_status_round_trip():
    cfg = Config(
        api_key=os.environ["OURA_MCP_TEST_API_KEY"],
        base_url=os.environ["OURA_MCP_TEST_BASE_URL"].rstrip("/"),
        timeout=10.0,
        log_level="info",
    )
    client = OuraReaderClient(cfg)
    try:
        result = await dispatch("sync_status", {}, client)
    finally:
        await client.close()
    assert isinstance(result, dict)
