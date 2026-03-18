#!/usr/bin/env bash

set -euo pipefail

GATEWAY_URL="${GATEWAY_URL:-http://127.0.0.1:8080}"
COPILOT_URL="${COPILOT_URL:-http://127.0.0.1:8000}"

echo "[1/4] Gateway healthz"
curl -sS "${GATEWAY_URL}/healthz" | jq .

echo "[2/4] Ingest risk event"
INGEST_RESP="$(curl -sS -X POST "${GATEWAY_URL}/api/v1/risk/events/ingest" \
  -H "Content-Type: application/json" \
  -d '{
    "merchant_id":"merchant-1001",
    "event_type":"abnormal_listing_activity",
    "evidence":["sku_spike","ip_anomaly"],
    "risk_score":0.87,
    "triggered_by":"rule-engine"
  }')"
echo "${INGEST_RESP}" | jq .

CASE_ID="$(echo "${INGEST_RESP}" | jq -r '.case_id')"

echo "[3/4] Query risk case ${CASE_ID}"
curl -sS "${GATEWAY_URL}/api/v1/risk/cases/${CASE_ID}" | jq .

echo "[4/4] Copilot summary"
curl -sS -X POST "${COPILOT_URL}/copilot/risk/summary" \
  -H "Content-Type: application/json" \
  -d "{
    \"case_id\":\"${CASE_ID}\",
    \"merchant_id\":\"merchant-1001\",
    \"risk_category\":\"abnormal_listing_activity\",
    \"evidence_items\":[\"sku_spike\",\"ip_anomaly\"],
    \"operator_note\":\"need triage\"
  }" | jq .
