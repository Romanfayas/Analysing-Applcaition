"""P1.9.20: unimplemented analytics routes must say so instead of returning fabricated success."""
import pytest
from fastapi.testclient import TestClient

from src.api.app import app

client = TestClient(app)

UNIMPLEMENTED = [
    "/api/quantitative/returns",
    "/api/quantitative/risk",
    "/api/quantitative/ratios",
    "/api/quantitative/market-relationship",
]


@pytest.mark.parametrize("path", UNIMPLEMENTED)
def test_quantitative_stubs_answer_501(path):
    r = client.post(path, json={})
    assert r.status_code == 501
    assert "not implemented" in r.json()["detail"]


@pytest.mark.parametrize("path", ["/api/v1/technical/support-resistance", "/api/v1/technical/trend"])
def test_technical_stubs_do_not_invent_results(path):
    body = {"timestamps": [], "open": [], "high": [], "low": [], "close": [], "volume": []}
    r = client.post(path, json=body)
    assert r.status_code == 501
    assert "trend" not in r.json() and "levels" not in r.json()


def test_implemented_risk_metrics_still_work():
    r = client.post("/api/v1/risk/metrics", json={"returns": [0.01, -0.02, 0.015, 0.003, -0.004] * 6})
    assert r.status_code == 200
    assert r.json()["volatility_daily"] is not None
