from __future__ import annotations

from datetime import datetime, timezone
from typing import Any

import httpx

from oura_reader.exceptions import AuthError, OuraReaderError, SyncError

# All available Oura data endpoints.
ENDPOINTS = [
    "daily_sleep",
    "sleep",
    "sleep_time",
    "daily_activity",
    "daily_readiness",
    "heartrate",
    "daily_resilience",
    "daily_stress",
    "daily_spo2",
    "daily_cardiovascular_age",
    "vo2_max",
    "workout",
    "session",
    "tag",
    "enhanced_tag",
    "ring_configuration",
    "rest_mode_period",
    "personal_info",
]


class OuraClient:
    """Client for the oura-reader REST API.

    Args:
        base_url: Server URL (e.g. "http://macmini:8080").
        api_key: API key for authentication.
        stale_threshold: Seconds after which data is considered stale
            and a sync is triggered automatically. Set to 0 to disable.
        timeout: HTTP request timeout in seconds.
    """

    def __init__(
        self,
        base_url: str,
        api_key: str,
        stale_threshold: int = 3600,
        timeout: int = 120,
    ) -> None:
        self._base_url = base_url.rstrip("/")
        self._api_key = api_key
        self._stale_threshold = stale_threshold
        self._client = httpx.Client(
            base_url=self._base_url,
            headers={"Authorization": f"Bearer {api_key}"},
            timeout=timeout,
        )

    def close(self) -> None:
        self._client.close()

    def __enter__(self) -> OuraClient:
        return self

    def __exit__(self, *args: object) -> None:
        self.close()

    # --- Data retrieval ---

    def get_sleep(
        self, start_date: str | None = None, end_date: str | None = None, *, auto_sync: bool = True
    ) -> list[dict[str, Any]]:
        return self._get("daily_sleep", start_date, end_date, auto_sync=auto_sync)

    def get_detailed_sleep(
        self, start_date: str | None = None, end_date: str | None = None, *, auto_sync: bool = True
    ) -> list[dict[str, Any]]:
        return self._get("sleep", start_date, end_date, auto_sync=auto_sync)

    def get_activity(
        self, start_date: str | None = None, end_date: str | None = None, *, auto_sync: bool = True
    ) -> list[dict[str, Any]]:
        return self._get("daily_activity", start_date, end_date, auto_sync=auto_sync)

    def get_readiness(
        self, start_date: str | None = None, end_date: str | None = None, *, auto_sync: bool = True
    ) -> list[dict[str, Any]]:
        """Includes temperature_deviation and temperature_trend_deviation."""
        return self._get("daily_readiness", start_date, end_date, auto_sync=auto_sync)

    def get_heartrate(
        self, start_date: str | None = None, end_date: str | None = None, *, auto_sync: bool = True
    ) -> list[dict[str, Any]]:
        return self._get("heartrate", start_date, end_date, auto_sync=auto_sync)

    def get_stress(
        self, start_date: str | None = None, end_date: str | None = None, *, auto_sync: bool = True
    ) -> list[dict[str, Any]]:
        return self._get("daily_stress", start_date, end_date, auto_sync=auto_sync)

    def get_spo2(
        self, start_date: str | None = None, end_date: str | None = None, *, auto_sync: bool = True
    ) -> list[dict[str, Any]]:
        return self._get("daily_spo2", start_date, end_date, auto_sync=auto_sync)

    def get_workouts(
        self, start_date: str | None = None, end_date: str | None = None, *, auto_sync: bool = True
    ) -> list[dict[str, Any]]:
        return self._get("workout", start_date, end_date, auto_sync=auto_sync)

    def get_data(
        self, endpoint: str, start_date: str | None = None, end_date: str | None = None, *, auto_sync: bool = True
    ) -> list[dict[str, Any]]:
        """Fetch data for any endpoint by name."""
        return self._get(endpoint, start_date, end_date, auto_sync=auto_sync)

    def get_all(
        self, start_date: str | None = None, end_date: str | None = None, *, auto_sync: bool = True
    ) -> dict[str, list[dict[str, Any]]]:
        """Fetch all endpoints' data for a date range."""
        if auto_sync and self._stale_threshold > 0:
            self._auto_sync_if_stale("daily_sleep")  # Check staleness once.

        params: dict[str, str] = {}
        if start_date:
            params["start_date"] = start_date
        if end_date:
            params["end_date"] = end_date

        resp = self._request("GET", "/api/v1/data", params=params)
        return resp

    # --- Sync ---

    def sync(self, endpoint: str | None = None) -> dict[str, Any]:
        """Trigger a sync. If endpoint is given, sync only that endpoint."""
        path = f"/api/v1/sync/{endpoint}" if endpoint else "/api/v1/sync"
        return self._request("POST", path)

    def sync_status(self) -> dict[str, Any]:
        return self._request("GET", "/api/v1/sync/status")

    # --- Auth ---

    def auth_status(self) -> dict[str, Any]:
        return self._request("GET", "/api/v1/auth/status")

    # --- Health ---

    def health(self) -> dict[str, Any]:
        # Health endpoint is unauthenticated; use a plain request.
        resp = httpx.get(f"{self._base_url}/api/v1/health", timeout=10)
        resp.raise_for_status()
        return resp.json()

    # --- Internal ---

    def _get(
        self,
        endpoint: str,
        start_date: str | None,
        end_date: str | None,
        *,
        auto_sync: bool = True,
    ) -> list[dict[str, Any]]:
        if auto_sync and self._stale_threshold > 0:
            self._auto_sync_if_stale(endpoint)

        params: dict[str, str] = {}
        if start_date:
            params["start_date"] = start_date
        if end_date:
            params["end_date"] = end_date

        resp = self._request("GET", f"/api/v1/data/{endpoint}", params=params)
        return resp.get("data", [])

    def _auto_sync_if_stale(self, endpoint: str) -> None:
        """Trigger a sync if the endpoint's last sync is older than stale_threshold."""
        try:
            status = self.sync_status()
        except OuraReaderError:
            return

        info = status.get(endpoint)
        if info is None:
            # Never synced — trigger sync.
            self.sync(endpoint)
            return

        last_sync_str = info.get("last_sync_at")
        if not last_sync_str:
            self.sync(endpoint)
            return

        try:
            last_sync = datetime.fromisoformat(last_sync_str)
        except ValueError:
            self.sync(endpoint)
            return

        # Ensure timezone-aware comparison.
        now = datetime.now(timezone.utc)
        if last_sync.tzinfo is None:
            last_sync = last_sync.replace(tzinfo=timezone.utc)

        age = (now - last_sync).total_seconds()
        if age > self._stale_threshold:
            self.sync(endpoint)

    def _request(self, method: str, path: str, params: dict[str, str] | None = None) -> Any:
        try:
            resp = self._client.request(method, path, params=params)
        except httpx.HTTPError as exc:
            raise OuraReaderError(f"HTTP error: {exc}") from exc

        if resp.status_code == 401:
            raise AuthError("Invalid API key or not authenticated")
        if resp.status_code >= 400:
            try:
                body = resp.json()
                msg = body.get("error", resp.text)
            except Exception:
                msg = resp.text
            raise OuraReaderError(f"API error {resp.status_code}: {msg}")

        return resp.json()
