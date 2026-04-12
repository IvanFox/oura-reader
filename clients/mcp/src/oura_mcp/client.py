"""Thin async httpx wrapper around the oura-reader REST API.

Exception contract: on any non-2xx or transport failure, raises UpstreamError
with a 'kind' string that tools.py maps to an agent-visible message. The
exception carries status code and a bounded detail string, never the response
body or headers (which could echo the auth header in some proxies).
"""

from __future__ import annotations

from dataclasses import dataclass
from typing import Any, Literal

import httpx

from oura_mcp.config import Config


ErrorKind = Literal[
    "unauthorized",     # 401
    "forbidden",        # 403
    "bad_request",      # 400
    "rate_limited",     # 429
    "upstream_error",   # 5xx
    "transport",        # network/timeout
]


@dataclass
class UpstreamError(Exception):
    kind: ErrorKind
    status: int | None = None
    detail: str = ""

    def __str__(self) -> str:
        return f"UpstreamError(kind={self.kind}, status={self.status})"


class OuraReaderClient:
    def __init__(self, cfg: Config):
        self._cfg = cfg
        self._http = httpx.AsyncClient(
            base_url=cfg.base_url,
            timeout=cfg.timeout,
            headers={"Authorization": f"Bearer {cfg.api_key}"},
        )

    async def close(self) -> None:
        await self._http.aclose()

    async def get_data(
        self,
        endpoint: str,
        start_date: str | None = None,
        end_date: str | None = None,
        limit: int | None = None,
    ) -> dict[str, Any]:
        params: dict[str, str] = {}
        if start_date:
            params["start_date"] = start_date
        if end_date:
            params["end_date"] = end_date
        if limit is not None:
            params["limit"] = str(limit)
        return await self._request("GET", f"/api/v1/data/{endpoint}", params=params)

    async def sync(self, endpoint: str | None = None) -> dict[str, Any]:
        path = f"/api/v1/sync/{endpoint}" if endpoint else "/api/v1/sync"
        return await self._request("POST", path)

    async def sync_status(self) -> dict[str, Any]:
        return await self._request("GET", "/api/v1/sync/status")

    async def _request(
        self,
        method: str,
        path: str,
        params: dict[str, str] | None = None,
    ) -> dict[str, Any]:
        try:
            resp = await self._http.request(method, path, params=params)
        except (httpx.TimeoutException, httpx.TransportError):
            # Deliberately do NOT pass the exception detail through — it may repr() headers
            raise UpstreamError(kind="transport")

        if resp.status_code == 401:
            raise UpstreamError(kind="unauthorized", status=401)
        if resp.status_code == 403:
            raise UpstreamError(kind="forbidden", status=403)
        if resp.status_code == 400:
            raise UpstreamError(kind="bad_request", status=400)
        if resp.status_code == 429:
            raise UpstreamError(kind="rate_limited", status=429)
        if resp.status_code >= 500:
            raise UpstreamError(kind="upstream_error", status=resp.status_code)
        if not 200 <= resp.status_code < 300:
            raise UpstreamError(kind="upstream_error", status=resp.status_code)

        try:
            return resp.json()
        except ValueError:
            raise UpstreamError(kind="upstream_error", status=resp.status_code)
