import re

import pytest

from oura_mcp.endpoints import ENDPOINT_NAMES, REGISTRY
from oura_mcp.tools import build_tool_definitions, dispatch


SECRET_FIELD_RE = re.compile(r"api|key|token|secret|auth", re.IGNORECASE)


def test_all_18_get_tools_registered():
    tools = build_tool_definitions()
    names = {t.name for t in tools}
    for endpoint_name in ENDPOINT_NAMES:
        assert f"get_{endpoint_name}" in names


def test_three_meta_tools_registered():
    tools = {t.name for t in build_tool_definitions()}
    assert {"sync", "sync_endpoint", "sync_status"}.issubset(tools)


def test_total_tool_count():
    # 18 data + 3 meta = 21
    assert len(build_tool_definitions()) == 21


def test_no_secret_shaped_field_in_any_schema():
    for tool in build_tool_definitions():
        props = (tool.inputSchema or {}).get("properties", {})
        for field_name in props:
            assert not SECRET_FIELD_RE.search(field_name), (
                f"tool {tool.name} has suspicious input field {field_name!r}"
            )


def test_date_free_tools_have_no_date_params():
    tools = {t.name: t for t in build_tool_definitions()}
    for name in ("get_ring_configuration", "get_personal_info"):
        props = (tools[name].inputSchema or {}).get("properties", {})
        assert "start_date" not in props
        assert "end_date" not in props


def test_personal_info_has_no_limit():
    tools = {t.name: t for t in build_tool_definitions()}
    props = (tools["get_personal_info"].inputSchema or {}).get("properties", {})
    assert "limit" not in props


def test_dated_tools_have_iso_date_description():
    tools = {t.name: t for t in build_tool_definitions()}
    props = (tools["get_daily_sleep"].inputSchema or {}).get("properties", {})
    assert "start_date" in props
    assert "YYYY-MM-DD" in props["start_date"].get("description", "")


def test_sync_endpoint_enum_derived_from_registry():
    """Enum values must equal REGISTRY names at runtime — not a hardcoded list.
    If someone adds a new endpoint to REGISTRY, this test keeps passing without edits.
    """
    tools = {t.name: t for t in build_tool_definitions()}
    enum = tools["sync_endpoint"].inputSchema["properties"]["endpoint"]["enum"]
    assert list(enum) == list(ENDPOINT_NAMES)


class FakeClient:
    def __init__(self):
        self.calls: list[tuple[str, tuple, dict]] = []

    async def get_data(self, *args, **kwargs):
        self.calls.append(("get_data", args, kwargs))
        return {"data": "stub"}

    async def sync(self, *args, **kwargs):
        self.calls.append(("sync", args, kwargs))
        return {"queued": True}

    async def sync_status(self, *args, **kwargs):
        self.calls.append(("sync_status", args, kwargs))
        return {"ok": True}


async def test_dispatch_get_daily_sleep():
    fake = FakeClient()
    result = await dispatch("get_daily_sleep", {"start_date": "2026-04-01", "limit": 10}, fake)
    assert result == {"data": "stub"}
    assert fake.calls == [("get_data", (), {"endpoint": "daily_sleep", "start_date": "2026-04-01", "end_date": None, "limit": 10})]


async def test_dispatch_sync_endpoint_maps_to_client():
    fake = FakeClient()
    await dispatch("sync_endpoint", {"endpoint": "daily_sleep"}, fake)
    assert fake.calls == [("sync", (), {"endpoint": "daily_sleep"})]


async def test_dispatch_unknown_tool_raises():
    fake = FakeClient()
    with pytest.raises(KeyError):
        await dispatch("get_nonexistent", {}, fake)
