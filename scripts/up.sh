#!/usr/bin/env bash
# One-shot dev launcher.
#   ./scripts/up.sh                 build + run API, watcher, dashboard in foreground
#   ./scripts/up.sh --detach        run all three as background daemons, write PIDs
#   ./scripts/up.sh --stop          stop daemons started with --detach
set -euo pipefail

cd "$(dirname "$0")/.."
ROOT=$(pwd -P)
STATE="$HOME/.local/share/claude-search"
PID_API="$STATE/api.pid"
PID_WATCH="$STATE/watch.pid"
PID_DASH="$STATE/dash.pid"
LOG_API="$STATE/api.log"
LOG_WATCH="$STATE/watch.log"
LOG_DASH="$STATE/dash.log"
mkdir -p "$STATE"

BIN="$ROOT/bin/claude-search"
API_ADDR="${CLAUDE_SEARCH_ADDR:-127.0.0.1:7070}"
DASH_PORT="${CLAUDE_SEARCH_DASH_PORT:-3737}"

stop_pidfile() {
  local f=$1 name=$2
  if [ -f "$f" ]; then
    local p; p=$(cat "$f")
    if ps -p "$p" >/dev/null 2>&1; then
      kill "$p" 2>/dev/null && echo "stopped $name (pid $p)"
    fi
    rm -f "$f"
  fi
}

if [[ "${1:-}" == "--stop" ]]; then
  stop_pidfile "$PID_API"   "api"
  stop_pidfile "$PID_WATCH" "watch"
  stop_pidfile "$PID_DASH"  "dashboard"
  exit 0
fi

echo "▸ building Go binary"
make build >/dev/null

echo "▸ checking dashboard deps"
if [ ! -d dashboard/node_modules ]; then
  ( cd dashboard && pnpm install --ignore-workspace )
fi

if [[ "${1:-}" == "--detach" ]]; then
  echo "▸ launching detached"
  nohup "$BIN" serve --addr "$API_ADDR" >"$LOG_API" 2>&1 &
  echo $! > "$PID_API"; disown
  nohup "$BIN" watch                   >"$LOG_WATCH" 2>&1 &
  echo $! > "$PID_WATCH"; disown
  nohup bash -c "cd '$ROOT/dashboard' && pnpm --ignore-workspace dev" >"$LOG_DASH" 2>&1 &
  echo $! > "$PID_DASH"; disown
  sleep 2
  cat <<EOF

✓ Running detached. PIDs in $STATE/*.pid, logs in $STATE/*.log

  API        http://$API_ADDR        (pid $(cat "$PID_API"))
  Dashboard  http://127.0.0.1:$DASH_PORT  (pid $(cat "$PID_DASH"))
  Watcher    fsnotify daemon         (pid $(cat "$PID_WATCH"))

  Stop:      ./scripts/up.sh --stop
  Tail:      tail -f $STATE/{api,watch,dash}.log
EOF
  exit 0
fi

# Foreground mode: interleaved logs, Ctrl-C cleans up.
PIDS=()
cleanup() {
  echo
  echo "▸ shutting down"
  for p in "${PIDS[@]}"; do kill "$p" 2>/dev/null || true; done
  wait 2>/dev/null || true
  exit 0
}
trap cleanup INT TERM

prefix() { local label=$1; sed -u "s/^/[$label] /"; }

echo "▸ starting API on $API_ADDR"
("$BIN" serve --addr "$API_ADDR" 2>&1 | prefix api) &
PIDS+=($!)

echo "▸ starting watcher"
("$BIN" watch 2>&1 | prefix watch) &
PIDS+=($!)

echo "▸ starting dashboard on :$DASH_PORT"
( cd dashboard && pnpm --ignore-workspace dev 2>&1 ) | prefix dash &
PIDS+=($!)

echo
echo "Dashboard → http://127.0.0.1:$DASH_PORT"
echo "API       → http://$API_ADDR"
echo "Ctrl-C to stop all."
echo

wait
