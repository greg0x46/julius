#!/usr/bin/env bash

set -Eeuo pipefail

repo_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
cd "$repo_dir"

# Optional untracked local config (KEY=value lines), e.g. JULIUS_DB or ports.
if [[ -f "$repo_dir/.env.local" ]]; then
  set -a
  # shellcheck source=/dev/null
  source "$repo_dir/.env.local"
  set +a
fi

backend_addr="${JULIUS_DEV_ADDR:-localhost:8000}"
frontend_host="${VITE_DEV_HOST:-127.0.0.1}"
frontend_port="${VITE_DEV_PORT:-5173}"
backend_url="http://${backend_addr}"
export JULIUS_PUBLIC_URL="${JULIUS_PUBLIC_URL:-http://${frontend_host}:${frontend_port}}"

if ! command -v go >/dev/null 2>&1; then
  echo "Error: Go is not installed or not in PATH." >&2
  exit 1
fi

if ! command -v npm >/dev/null 2>&1; then
  echo "Error: npm is not installed or not in PATH." >&2
  exit 1
fi

if [[ ! -d "$repo_dir/frontend/node_modules" ]]; then
  echo "Frontend dependencies not found. Run: (cd frontend && npm install)" >&2
  exit 1
fi

has_setsid=false
if command -v setsid >/dev/null 2>&1; then
  has_setsid=true
fi

backend_pid=""
frontend_pid=""
last_pid=""

start_process() {
  if [[ "$has_setsid" == true ]]; then
    setsid --wait "$@" &
  else
    "$@" &
  fi
  last_pid=$!
}

stop_process() {
  local pid="$1"

  if [[ "$has_setsid" == true ]]; then
    kill -TERM -- "-$pid" 2>/dev/null || true
  else
    kill -TERM "$pid" 2>/dev/null || true
  fi
}

cleanup() {
  local exit_code=$?
  trap - EXIT INT TERM

  [[ -z "$frontend_pid" ]] || stop_process "$frontend_pid"
  [[ -z "$backend_pid" ]] || stop_process "$backend_pid"

  [[ -z "$frontend_pid" ]] || wait "$frontend_pid" 2>/dev/null || true
  [[ -z "$backend_pid" ]] || wait "$backend_pid" 2>/dev/null || true

  exit "$exit_code"
}

trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

echo "Backend:  http://${backend_addr}"
echo "Frontend: ${JULIUS_PUBLIC_URL}"
echo "Press Ctrl-C to stop both processes."

start_process go run ./cmd/julius -addr "$backend_addr"
backend_pid="$last_pid"

start_process env JULIUS_DEV_API_URL="$backend_url" npm --prefix "$repo_dir/frontend" run dev -- \
  --host "$frontend_host" \
  --port "$frontend_port"
frontend_pid="$last_pid"

status=0
while kill -0 "$backend_pid" 2>/dev/null && kill -0 "$frontend_pid" 2>/dev/null; do
  sleep 0.2
done
if ! kill -0 "$backend_pid" 2>/dev/null; then
  wait "$backend_pid" || status=$?
else
  wait "$frontend_pid" || status=$?
fi
exit "$status"
