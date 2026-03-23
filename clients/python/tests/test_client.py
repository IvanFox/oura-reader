import json

import pytest
import httpx
from pytest_httpx import HTTPXMock

from oura_reader import OuraClient, AuthError, OuraReaderError


BASE_URL = "http://localhost:8080"
API_KEY = "oura_ak_test1234567890"


@pytest.fixture
def client():
    c = OuraClient(base_url=BASE_URL, api_key=API_KEY, stale_threshold=0)
    yield c
    c.close()


def test_get_sleep(client: OuraClient, httpx_mock: HTTPXMock):
    httpx_mock.add_response(
        url=f"{BASE_URL}/api/v1/data/daily_sleep",
        json={"data": [{"day": "2026-03-22", "score": 85}], "count": 1, "limit": 100, "offset": 0},
    )

    result = client.get_sleep()
    assert len(result) == 1
    assert result[0]["score"] == 85


def test_get_readiness_with_temperature(client: OuraClient, httpx_mock: HTTPXMock):
    httpx_mock.add_response(
        url=f"{BASE_URL}/api/v1/data/daily_readiness",
        json={
            "data": [
                {
                    "day": "2026-03-22",
                    "score": 82,
                    "temperature_deviation": -0.1,
                    "temperature_trend_deviation": 0.05,
                }
            ],
            "count": 1,
            "limit": 100,
            "offset": 0,
        },
    )

    result = client.get_readiness()
    assert result[0]["temperature_deviation"] == -0.1


def test_auth_error(client: OuraClient, httpx_mock: HTTPXMock):
    httpx_mock.add_response(
        url=f"{BASE_URL}/api/v1/auth/status",
        status_code=401,
        json={"error": "invalid API key"},
    )

    with pytest.raises(AuthError):
        client.auth_status()


def test_sync(client: OuraClient, httpx_mock: HTTPXMock):
    httpx_mock.add_response(
        url=f"{BASE_URL}/api/v1/sync",
        method="POST",
        json={"status": "ok"},
    )

    result = client.sync()
    assert result["status"] == "ok"


def test_sync_single_endpoint(client: OuraClient, httpx_mock: HTTPXMock):
    httpx_mock.add_response(
        url=f"{BASE_URL}/api/v1/sync/daily_sleep",
        method="POST",
        json={"status": "ok", "endpoint": "daily_sleep"},
    )

    result = client.sync("daily_sleep")
    assert result["endpoint"] == "daily_sleep"


def test_sync_status(client: OuraClient, httpx_mock: HTTPXMock):
    httpx_mock.add_response(
        url=f"{BASE_URL}/api/v1/sync/status",
        json={"daily_sleep": {"last_sync_date": "2026-03-22", "last_sync_at": "2026-03-22T10:00:00Z"}},
    )

    result = client.sync_status()
    assert "daily_sleep" in result


def test_get_all(client: OuraClient, httpx_mock: HTTPXMock):
    httpx_mock.add_response(
        url=f"{BASE_URL}/api/v1/data",
        json={"daily_sleep": [{"day": "2026-03-22"}], "daily_activity": []},
    )

    result = client.get_all()
    assert "daily_sleep" in result


def test_date_params(client: OuraClient, httpx_mock: HTTPXMock):
    httpx_mock.add_response(
        json={"data": [], "count": 0, "limit": 100, "offset": 0},
    )

    client.get_sleep(start_date="2026-03-01", end_date="2026-03-22")

    request = httpx_mock.get_requests()[0]
    assert "start_date=2026-03-01" in str(request.url)
    assert "end_date=2026-03-22" in str(request.url)
