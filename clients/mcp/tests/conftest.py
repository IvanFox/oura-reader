import os
import pytest


@pytest.fixture(autouse=True)
def clean_env(monkeypatch):
    """Clear OURA_MCP_* env vars before each test."""
    for key in list(os.environ):
        if key.startswith("OURA_MCP_"):
            monkeypatch.delenv(key, raising=False)
