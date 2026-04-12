"""Tool definitions and dispatch for the MCP server.

Data tools are generated from REGISTRY at module-import time. Schemas are
tight (no secret-shaped fields, no extras). Dispatch is a flat function
so server.py can call it from its request handler without subclassing.
"""

from __future__ import annotations

from typing import Any

from mcp.types import Tool

from oura_mcp.endpoints import ENDPOINT_NAMES, REGISTRY, REGISTRY_BY_NAME


_DATE_DESC = "ISO date in YYYY-MM-DD format."
_LIMIT_DESC = "Maximum number of rows to return (default 100, server-side capped)."


def _data_tool(spec) -> Tool:
    properties: dict[str, Any] = {}
    if spec.has_dates:
        properties["start_date"] = {"type": "string", "description": f"Start of range. {_DATE_DESC}"}
        properties["end_date"] = {"type": "string", "description": f"End of range (inclusive). {_DATE_DESC}"}
    if spec.is_list:
        properties["limit"] = {"type": "integer", "minimum": 1, "description": _LIMIT_DESC}

    return Tool(
        name=f"get_{spec.name}",
        description=f"Fetch {spec.name.replace('_', ' ')} data from the oura-reader store.",
        inputSchema={
            "type": "object",
            "properties": properties,
            "additionalProperties": False,
        },
    )


def _meta_tools() -> list[Tool]:
    return [
        Tool(
            name="sync",
            description="Trigger a full sync across all endpoints for the configured user. Fire-and-forget; poll sync_status for progress.",
            inputSchema={"type": "object", "properties": {}, "additionalProperties": False},
        ),
        Tool(
            name="sync_endpoint",
            description="Trigger a sync for a single endpoint. Fire-and-forget; poll sync_status for progress.",
            inputSchema={
                "type": "object",
                "properties": {
                    "endpoint": {
                        "type": "string",
                        "enum": list(ENDPOINT_NAMES),
                        "description": "Name of the Oura endpoint to sync.",
                    },
                },
                "required": ["endpoint"],
                "additionalProperties": False,
            },
        ),
        Tool(
            name="sync_status",
            description="Return per-endpoint sync freshness and last-error state for the configured user.",
            inputSchema={"type": "object", "properties": {}, "additionalProperties": False},
        ),
    ]


def build_tool_definitions() -> list[Tool]:
    data_tools = [_data_tool(spec) for spec in REGISTRY]
    return data_tools + _meta_tools()


async def dispatch(name: str, args: dict[str, Any], client) -> dict[str, Any]:
    """Route a tool call to the client. Raises UpstreamError on upstream failure,
    KeyError on unknown tool name (should never happen — the MCP SDK validates
    against the registered tool list before we're called)."""
    if name.startswith("get_"):
        endpoint = name[len("get_"):]
        if endpoint not in REGISTRY_BY_NAME:
            raise KeyError(name)
        return await client.get_data(
            endpoint=endpoint,
            start_date=args.get("start_date"),
            end_date=args.get("end_date"),
            limit=args.get("limit"),
        )
    if name == "sync":
        return await client.sync()
    if name == "sync_endpoint":
        return await client.sync(endpoint=args["endpoint"])
    if name == "sync_status":
        return await client.sync_status()
    raise KeyError(name)
