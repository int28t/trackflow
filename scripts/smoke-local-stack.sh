#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
COMPOSE_FILE="${ROOT_DIR}/docker-compose.yml"
TIMEOUT_SECONDS="${SMOKE_TIMEOUT_SECONDS:-120}"
POLL_INTERVAL_SECONDS=2
AUTO_DOWN=false
SKIP_BUILD=false

usage() {
	echo "Usage: scripts/smoke-local-stack.sh [--down] [--skip-build]"
	echo "  --down       Stop and remove stack after smoke check"
	echo "  --skip-build Start stack without --build"
}

while [[ $# -gt 0 ]]; do
	case "$1" in
	--down)
		AUTO_DOWN=true
		;;
	--skip-build)
		SKIP_BUILD=true
		;;
	-h|--help)
		usage
		exit 0
		;;
	*)
		echo "Unknown argument: $1"
		usage
		exit 1
		;;
	esac
	shift
done

if ! command -v docker >/dev/null 2>&1; then
	echo "[smoke] docker CLI is not installed"
	exit 1
fi

if ! docker info >/dev/null 2>&1; then
	echo "[smoke] docker daemon is not available"
	exit 1
fi

cd "${ROOT_DIR}"

compose_cmd=(docker compose -f "${COMPOSE_FILE}")
up_args=(up -d)
if [[ "${SKIP_BUILD}" == "false" ]]; then
	up_args+=(--build)
fi

echo "[smoke] Starting local stack..."
"${compose_cmd[@]}" "${up_args[@]}"

on_error() {
	echo "[smoke] Smoke check failed"
	"${compose_cmd[@]}" ps || true
	"${compose_cmd[@]}" logs --tail=50 nginx api-gateway order-service tracking-service carrier-sync-service notification-service || true
	if [[ "${AUTO_DOWN}" == "true" ]]; then
		echo "[smoke] Bringing stack down (--down enabled)..."
		"${compose_cmd[@]}" down || true
	fi
}

trap on_error ERR

mapped_port="$("${compose_cmd[@]}" port nginx 80 | tail -n1 || true)"
if [[ -z "${mapped_port}" ]]; then
	echo "[smoke] Could not determine mapped nginx port"
	exit 1
fi

host_port="${mapped_port##*:}"
health_url="http://127.0.0.1:${host_port}/health"

echo "[smoke] Waiting for ${health_url} ..."
deadline=$((SECONDS + TIMEOUT_SECONDS))
until curl -fsS "${health_url}" >/dev/null 2>&1; do
	if ((SECONDS >= deadline)); then
		echo "[smoke] Timeout waiting for health endpoint"
		exit 1
	fi
	sleep "${POLL_INTERVAL_SECONDS}"
done

trap - ERR

echo "[smoke] Local stack is healthy"
"${compose_cmd[@]}" ps

if [[ "${AUTO_DOWN}" == "true" ]]; then
	echo "[smoke] Bringing stack down..."
	"${compose_cmd[@]}" down
else
	echo "[smoke] Stack is up. Use 'docker compose down' to stop it."
fi
