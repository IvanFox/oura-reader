import httpx
import pytest
import respx

from oura_mcp.client import OuraReaderClient, UpstreamError
from oura_mcp.config import Config


@pytest.fixture
def cfg():
    return Config(api_key="secret-key-123", base_url="https://api.example", timeout=5.0, log_level="info")


@pytest.fixture
def client(cfg):
    return OuraReaderClient(cfg)


@respx.mock
async def test_get_data_sends_bearer_token(client):
    route = respx.get("https://api.example/api/v1/data/daily_sleep").mock(
        return_value=httpx.Response(200, json={"data": [], "count": 0})
    )
    result = await client.get_data("daily_sleep")
    assert result == {"data": [], "count": 0}
    assert route.calls.last.request.headers["Authorization"] == "Bearer secret-key-123"


@respx.mock
async def test_get_data_passes_date_range_and_limit(client):
    route = respx.get("https://api.example/api/v1/data/daily_sleep").mock(
        return_value=httpx.Response(200, json={"data": []})
    )
    await client.get_data("daily_sleep", start_date="2026-04-01", end_date="2026-04-12", limit=50)
    params = dict(route.calls.last.request.url.params)
    assert params == {"start_date": "2026-04-01", "end_date": "2026-04-12", "limit": "50"}


@respx.mock
async def test_401_raises_upstream_error_with_status(client):
    respx.get("https://api.example/api/v1/data/daily_sleep").mock(return_value=httpx.Response(401))
    with pytest.raises(UpstreamError) as exc:
        await client.get_data("daily_sleep")
    assert exc.value.kind == "unauthorized"
    assert exc.value.status == 401


@respx.mock
async def test_403_raises_upstream_error(client):
    respx.get("https://api.example/api/v1/data/daily_sleep").mock(return_value=httpx.Response(403))
    with pytest.raises(UpstreamError) as exc:
        await client.get_data("daily_sleep")
    assert exc.value.kind == "forbidden"


@respx.mock
async def test_429_raises_rate_limited(client):
    respx.get("https://api.example/api/v1/data/daily_sleep").mock(return_value=httpx.Response(429))
    with pytest.raises(UpstreamError) as exc:
        await client.get_data("daily_sleep")
    assert exc.value.kind == "rate_limited"


@respx.mock
async def test_5xx_raises_upstream_error(client):
    respx.get("https://api.example/api/v1/data/daily_sleep").mock(return_value=httpx.Response(503))
    with pytest.raises(UpstreamError) as exc:
        await client.get_data("daily_sleep")
    assert exc.value.kind == "upstream_error"
    assert exc.value.status == 503


@respx.mock
async def test_network_error_raises_transport(client):
    respx.get("https://api.example/api/v1/data/daily_sleep").mock(side_effect=httpx.ConnectError("boom"))
    with pytest.raises(UpstreamError) as exc:
        await client.get_data("daily_sleep")
    assert exc.value.kind == "transport"


@respx.mock
async def test_sync_posts(client):
    route = respx.post("https://api.example/api/v1/sync").mock(
        return_value=httpx.Response(200, json={"queued": True})
    )
    await client.sync()
    assert route.called


@respx.mock
async def test_sync_endpoint_posts_with_path_param(client):
    route = respx.post("https://api.example/api/v1/sync/daily_sleep").mock(
        return_value=httpx.Response(200, json={})
    )
    await client.sync(endpoint="daily_sleep")
    assert route.called


@respx.mock
async def test_sync_status_gets(client):
    route = respx.get("https://api.example/api/v1/sync/status").mock(
        return_value=httpx.Response(200, json={"endpoints": []})
    )
    result = await client.sync_status()
    assert "endpoints" in result


def test_upstream_error_never_includes_key_in_repr(cfg):
    err = UpstreamError(kind="upstream_error", status=500, detail="")
    # repr() is what gets logged if someone does f"{err!r}" — must not carry key
    assert cfg.api_key not in repr(err)
    assert cfg.api_key not in str(err)
