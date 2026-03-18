from fastapi.testclient import TestClient

from app.main import create_app


def test_healthz():
    client = TestClient(create_app())
    resp = client.get("/healthz")
    assert resp.status_code == 200
    assert resp.json()["status"] == "ok"
    assert resp.json()["service"] == "ai-copilot"


def test_risk_summary():
    client = TestClient(create_app())
    payload = {
        "case_id": "case-risk-001",
        "merchant_id": "merchant-1001",
        "risk_category": "listing_fraud",
        "evidence_items": ["sku_spike", "ip_anomaly"],
        "operator_note": "need triage",
    }
    resp = client.post("/copilot/risk/summary", json=payload)
    assert resp.status_code == 200
    body = resp.json()
    assert body["case_id"] == "case-risk-001"
    assert body["grounded"] is True
    assert len(body["summary"]) > 10
    assert isinstance(body["suggestions"], list)
    assert len(body["suggestions"]) > 0
