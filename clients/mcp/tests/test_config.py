import os
import pytest

from oura_mcp.config import Config, load_config


def test_load_valid(monkeypatch):
    monkeypatch.setenv("OURA_MCP_API_KEY", "k")
    monkeypatch.setenv("OURA_MCP_BASE_URL", "https://x.example")
    cfg = load_config()
    assert cfg.api_key == "k"
    assert cfg.base_url == "https://x.example"
    assert cfg.timeout == 30.0
    assert cfg.log_level == "info"


def test_strips_whitespace(monkeypatch):
    monkeypatch.setenv("OURA_MCP_API_KEY", "  k \n")
    monkeypatch.setenv("OURA_MCP_BASE_URL", " https://x.example/ ")
    cfg = load_config()
    assert cfg.api_key == "k"
    # Trailing slash stripped so URL joins are predictable
    assert cfg.base_url == "https://x.example"


def test_missing_api_key_exits(monkeypatch, capsys):
    monkeypatch.setenv("OURA_MCP_BASE_URL", "https://x.example")
    with pytest.raises(SystemExit) as exc:
        load_config()
    assert exc.value.code == 2
    err = capsys.readouterr().err
    assert "OURA_MCP_API_KEY" in err


def test_missing_base_url_exits(monkeypatch, capsys):
    monkeypatch.setenv("OURA_MCP_API_KEY", "k")
    with pytest.raises(SystemExit) as exc:
        load_config()
    assert exc.value.code == 2
    assert "OURA_MCP_BASE_URL" in capsys.readouterr().err


def test_timeout_default_and_override(monkeypatch):
    monkeypatch.setenv("OURA_MCP_API_KEY", "k")
    monkeypatch.setenv("OURA_MCP_BASE_URL", "https://x.example")
    monkeypatch.setenv("OURA_MCP_TIMEOUT", "5")
    assert load_config().timeout == 5.0


def test_log_level_override(monkeypatch):
    monkeypatch.setenv("OURA_MCP_API_KEY", "k")
    monkeypatch.setenv("OURA_MCP_BASE_URL", "https://x.example")
    monkeypatch.setenv("OURA_MCP_LOG_LEVEL", "debug")
    assert load_config().log_level == "debug"


def test_invalid_log_level_exits(monkeypatch):
    monkeypatch.setenv("OURA_MCP_API_KEY", "k")
    monkeypatch.setenv("OURA_MCP_BASE_URL", "https://x.example")
    monkeypatch.setenv("OURA_MCP_LOG_LEVEL", "trace")
    with pytest.raises(SystemExit):
        load_config()
