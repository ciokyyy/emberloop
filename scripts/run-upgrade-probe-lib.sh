#!/usr/bin/env bash
# Shared setup for the manual live probes in the Run-upgrade ticket.
# This file is a library, not a probe; source it from a probe script.

PROBE_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PROBE_DIR=""
PROBE_DAEMON=""
PROBE_BASE=""
PROBE_TOKEN=""
PROBE_JAR=""
PROBE_LOG=""
PROBE_RUN_IDS=()

probe_require() {
  for bin in "$@"; do
    command -v "$bin" >/dev/null 2>&1 || { echo "FAIL - need $bin on PATH" >&2; return 1; }
  done
}

probe_init() {
  local name="$1"
  export PATH="$HOME/go/bin:$PATH"
  probe_require go curl python3 git || return 1
  export GIT_AUTHOR_NAME=el-live-probe GIT_AUTHOR_EMAIL=probe@example.com
  export GIT_COMMITTER_NAME=el-live-probe GIT_COMMITTER_EMAIL=probe@example.com
  export GIT_CONFIG_NOSYSTEM=1

  PROBE_DIR="$(mktemp -d "/tmp/el-run-upgrade-${name}.XXXXXX")"
  PROBE_LOG="$PROBE_DIR/daemon.log"
  PROBE_JAR="$PROBE_DIR/cookies.txt"
  mkdir -p "$PROBE_DIR/state"
  export EL_SETTINGS="$PROBE_DIR/settings.json"
  trap probe_cleanup EXIT

  echo "building el-web for $name probe..." >&2
  if ! go -C "$PROBE_ROOT/emberloop-web" build -o "$PROBE_DIR/emberloop-web" ./cmd/emberloop-web; then
    echo "FAIL - el-web build failed" >&2
    return 1
  fi
  "$PROBE_DIR/emberloop-web" -addr 127.0.0.1:0 -state-dir "$PROBE_DIR/state" -restart-check 0 >"$PROBE_LOG" 2>&1 &
  PROBE_DAEMON=$!

  local ready=0
  for _ in $(seq 1 150); do
    if ! kill -0 "$PROBE_DAEMON" >/dev/null 2>&1; then
      echo "FAIL - Daemon exited during startup; log: $PROBE_LOG" >&2
      tail -30 "$PROBE_LOG" >&2
      return 1
    fi
    PROBE_TOKEN="$(cat "$PROBE_DIR/state/enderbloom/daemon.token" 2>/dev/null || true)"
    PROBE_BASE="$(sed -n 's/^el-web on //p' "$PROBE_LOG" | head -1)"
    if [ -n "$PROBE_TOKEN" ] && [ -n "$PROBE_BASE" ] && \
      curl -fsS --max-time 2 -H "Authorization: Bearer $PROBE_TOKEN" "$PROBE_BASE/api/health" >/dev/null 2>&1; then
      ready=1
      break
    fi
    sleep 0.2
  done
  if [ "$ready" -ne 1 ]; then
    echo "FAIL - Daemon did not become healthy; log: $PROBE_LOG" >&2
    tail -30 "$PROBE_LOG" >&2
    return 1
  fi

  if ! curl -fsS -c "$PROBE_JAR" -H 'Content-Type: application/json' \
    -d '{"username":"probe","password":"probe-password"}' "$PROBE_BASE/api/auth/signup" >/dev/null; then
    echo "FAIL - signup refused; log: $PROBE_LOG" >&2
    return 1
  fi
  echo "Daemon ready: $PROBE_BASE (state isolated under $PROBE_DIR)" >&2
}

probe_cleanup() {
  local id
  if [ -n "$PROBE_BASE" ] && [ -f "$PROBE_JAR" ]; then
    for id in "${PROBE_RUN_IDS[@]:-}"; do
      [ -n "$id" ] || continue
      curl -fsS --max-time 90 -b "$PROBE_JAR" -X POST "$PROBE_BASE/api/runs/$id/pause" \
        -H 'Content-Type: application/json' -d '{}' >/dev/null 2>&1 || true
    done
  fi
  if [ -n "$PROBE_DAEMON" ] && kill -0 "$PROBE_DAEMON" >/dev/null 2>&1; then
    kill "$PROBE_DAEMON" >/dev/null 2>&1 || true
    wait "$PROBE_DAEMON" 2>/dev/null || true
  fi
  if [ -n "$PROBE_DIR" ]; then
    if [ "${EL_RUN_UPGRADE_KEEP:-0}" = 1 ]; then
      echo "probe artifacts retained at $PROBE_DIR" >&2
    else
      rm -rf "$PROBE_DIR"
    fi
  fi
}

probe_api_get() {
  curl -fsS --max-time 60 -b "$PROBE_JAR" "$PROBE_BASE$1"
}

probe_api_post() {
  local body="${2-}"
  [ -n "$body" ] || body='{}'
  curl -fsS --max-time 90 -b "$PROBE_JAR" -X POST "$PROBE_BASE$1" \
    -H 'Content-Type: application/json' -d "$body"
}

probe_api_put() {
  local body="${2-}"
  [ -n "$body" ] || body='{}'
  curl -fsS --max-time 60 -b "$PROBE_JAR" -X PUT "$PROBE_BASE$1" \
    -H 'Content-Type: application/json' -d "$body"
}

probe_git_init() {
  local repo="$1"
  mkdir -p "$repo"
  git -C "$repo" init -q -b main
  git -C "$repo" config user.name "$GIT_AUTHOR_NAME"
  git -C "$repo" config user.email "$GIT_AUTHOR_EMAIL"
}

probe_git_commit() {
  local repo="$1" message="$2"
  git -C "$repo" add -A
  git -C "$repo" commit -q -m "$message"
}

probe_add_green_check() {
  local repo="$1"
  mkdir -p "$repo/.emberloop"
  cat >"$repo/.emberloop/check" <<'CHECK'
#!/bin/sh
set -eu
exit 0
CHECK
}

probe_start_run() {
  local spec="$1" repo="$2" harness="$3" model="$4"
  local body
  body="$(python3 -c 'import json,sys; print(json.dumps({"spec":sys.argv[1],"repo":sys.argv[2],"harness":sys.argv[3],"model":sys.argv[4]}))' "$spec" "$repo" "$harness" "$model")"
  probe_api_post /api/runs "$body"
}

probe_track_run() {
  PROBE_RUN_IDS+=("$1")
}
