#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OPENAPI_FILE="${OPENAPI_FILE:-${ROOT_DIR}/docs/api/openapi-v1.yaml}"
PORT="${SWAGGER_UI_PORT:-8089}"

if ! command -v docker >/dev/null 2>&1; then
  echo "[swagger] docker CLI is not installed"
  exit 1
fi

if ! docker info >/dev/null 2>&1; then
  echo "[swagger] docker daemon is not available"
  exit 1
fi

if [[ ! -f "${OPENAPI_FILE}" ]]; then
  echo "[swagger] OpenAPI file not found: ${OPENAPI_FILE}"
  exit 1
fi

spec_dir="$(cd "$(dirname "${OPENAPI_FILE}")" && pwd)"
spec_file="$(basename "${OPENAPI_FILE}")"

echo "[swagger] Starting Swagger UI"
echo "[swagger] Spec: ${OPENAPI_FILE}"
echo "[swagger] URL: http://127.0.0.1:${PORT}"

docker run --rm \
  -p "${PORT}:8080" \
  -e SWAGGER_JSON="/docs/${spec_file}" \
  -v "${spec_dir}:/docs:ro" \
  swaggerapi/swagger-ui
