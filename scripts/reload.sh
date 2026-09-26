#!/usr/bin/env bash
# pulse-eight-cec-linux — kill → compile → start → poll /api/health
set -euo pipefail

PROJECT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BINARY="$PROJECT_DIR/pulse-eight-cec-linux"
PID_FILE="$PROJECT_DIR/pulse-eight-cec-linux.pid"
LOG_FILE="$PROJECT_DIR/pulse-eight-cec-linux.log"

# Ensure Go toolchain is on PATH regardless of how this script was invoked.
export PATH="/usr/local/go/bin:$HOME/go/bin:$PATH"

# Source .env so PORT and all app env vars are available for build and start.
if [[ -f "$PROJECT_DIR/.env" ]]; then
  set -a
  # shellcheck disable=SC1091
  source "$PROJECT_DIR/.env"
  set +a
fi
PORT="${PORT:-9390}"

# ── shared start-lock — don't race deck-hub / GoApe / a second manual run ──────
# Directory lock at $PROJECT_DIR/.reload.lock; mkdir is atomic so exactly one
# starter wins. A lock whose pid is dead or older than 5min is stolen (the
# previous starter crashed or hung). RELOAD_LOCK_SKIP=1 means an outer caller
# (deck-hub/GoApe) already holds it, so we skip locking and just run.
_LOCK_DIR="$PROJECT_DIR/.reload.lock"
if [[ "${RELOAD_LOCK_SKIP:-0}" != "1" ]]; then
  _waited=0
  while ! mkdir "$_LOCK_DIR" 2>/dev/null; do
    read -r _lpid _lts _lwho < "$_LOCK_DIR/info" 2>/dev/null || true
    _age=$(( $(date +%s) - ${_lts:-0} ))
    if [[ -z "${_lpid:-}" ]] || ! kill -0 "${_lpid:-0}" 2>/dev/null || (( _age > 300 )); then
      echo "[pulse-eight-cec-linux] stealing stale start lock (pid=${_lpid:-?} age=${_age}s owner=${_lwho:-?})"
      rm -rf "$_LOCK_DIR"; continue
    fi
    if (( _waited >= 15 )); then
      echo "[pulse-eight-cec-linux] another start already in progress (pid=$_lpid age=${_age}s) — backing off"; exit 0
    fi
    sleep 1; _waited=$(( _waited + 1 ))
  done
  printf '%s %s %s\n' "$$" "$(date +%s)" "reload.sh" > "$_LOCK_DIR/info"
  trap 'rm -rf "$_LOCK_DIR"' EXIT
fi

echo "[pulse-eight-cec-linux] stopping..."
# Kill by recorded PID (precise — avoids killing unrelated processes with similar names).
if [[ -f "$PID_FILE" ]]; then
  OLD_PID="$(cat "$PID_FILE")"
  if kill "$OLD_PID" 2>/dev/null; then
    for _i in $(seq 1 20); do kill -0 "$OLD_PID" 2>/dev/null || break; sleep 0.1; done
  fi
  rm -f "$PID_FILE"
fi
# Kill by port — catches any survivor not tracked by PID file (e.g. started outside this script).
# fuser is preferred (kills by socket fd); fall back to ss+kill if unavailable.
if command -v fuser >/dev/null 2>&1; then
  fuser -k "${PORT}/tcp" 2>/dev/null && sleep 0.2 || true
else
  PORT_PID=$(ss -tlnp 2>/dev/null | awk -F'pid=' "/\:${PORT}[[:space:]]/{print \$2}" | cut -d, -f1)
  [[ -n "$PORT_PID" ]] && kill "$PORT_PID" 2>/dev/null && sleep 0.3 || true
fi

echo "[pulse-eight-cec-linux] compiling..."
cd "$PROJECT_DIR"
go build -o "$BINARY" ./cmd/pulse-eight-cec-linux

echo "[pulse-eight-cec-linux] starting on port $PORT..."
nohup "$BINARY" > "$LOG_FILE" 2>&1 &
NEW_PID=$!
echo "$NEW_PID" > "$PID_FILE"
echo "  PID $NEW_PID"

echo "[pulse-eight-cec-linux] waiting for /api/health..."
for _i in $(seq 1 40); do
  sleep 0.25
  if ! kill -0 "$NEW_PID" 2>/dev/null; then
    echo "✗ Process $NEW_PID died (port conflict or crash) — check $LOG_FILE"
    tail -10 "$LOG_FILE" 2>/dev/null | sed 's/^/  /' || true
    exit 1
  fi
  if curl -sf "http://localhost:$PORT/api/health" >/dev/null 2>&1; then
    echo "✓ Server ready at http://localhost:$PORT (PID $NEW_PID)"
    exit 0
  fi
done
echo "✗ Server did not start — check $LOG_FILE"
exit 1
