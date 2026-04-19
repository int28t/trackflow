#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

GATEWAY_BASE_URL="${E2E_GATEWAY_BASE_URL:-http://127.0.0.1:8080}"
COURIER_ID="${E2E_COURIER_ID:-c1111111-1111-1111-1111-111111111111}"

echo "[e2e-smoke] Running gateway happy path smoke test"
echo "[e2e-smoke] Gateway URL: ${GATEWAY_BASE_URL}"
echo "[e2e-smoke] Courier ID: ${COURIER_ID}"

cd "${ROOT_DIR}"
E2E_GATEWAY_BASE_URL="${GATEWAY_BASE_URL}" \
E2E_COURIER_ID="${COURIER_ID}" \
go test -tags e2e ./services/api-gateway/e2e -count=1 -run TestGatewayHappyPathSmoke -v
