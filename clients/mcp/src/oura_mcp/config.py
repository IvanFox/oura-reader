"""Env-var-only config loading. No .env, no defaults for required values."""

from __future__ import annotations

import os
import sys
from dataclasses import dataclass


_VALID_LOG_LEVELS = {"debug", "info", "warning", "error"}


@dataclass(frozen=True)
class Config:
    api_key: str
    base_url: str
    timeout: float
    log_level: str


def _require(var: str) -> str:
    value = os.environ.get(var, "").strip()
    if not value:
        print(
            f"error: {var} is not set. The MCP client config must provide it via the 'env' field.",
            file=sys.stderr,
        )
        raise SystemExit(2)
    return value


def load_config() -> Config:
    api_key = _require("OURA_MCP_API_KEY")
    base_url = _require("OURA_MCP_BASE_URL").rstrip("/")

    timeout_raw = os.environ.get("OURA_MCP_TIMEOUT", "30").strip()
    try:
        timeout = float(timeout_raw)
    except ValueError:
        print(f"error: OURA_MCP_TIMEOUT must be numeric (got {timeout_raw!r})", file=sys.stderr)
        raise SystemExit(2)

    log_level = os.environ.get("OURA_MCP_LOG_LEVEL", "info").strip().lower()
    if log_level not in _VALID_LOG_LEVELS:
        print(
            f"error: OURA_MCP_LOG_LEVEL must be one of {sorted(_VALID_LOG_LEVELS)} (got {log_level!r})",
            file=sys.stderr,
        )
        raise SystemExit(2)

    return Config(api_key=api_key, base_url=base_url, timeout=timeout, log_level=log_level)
