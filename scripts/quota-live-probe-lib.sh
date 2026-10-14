#!/usr/bin/env bash
# Shared isolated-Daemon runner for the provider-specific quota live probes.
QUOTA_LIVE_REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"


quota_live_pig_auth_path() {
  if [[ -n "${EL_PIG_AUTH:-}" ]]; then
    printf '%s' "$EL_PIG_AUTH"
    return
  fi

  local agent_dir=""
  if [[ "${PIG_USE_PI_DIRS:-}" == "1" ]]; then
    agent_dir="${PI_CODING_AGENT_DIR:-}"
    if [[ -z "$agent_dir" ]]; then
      agent_dir="$HOME/.pi/agent"
    fi
  else
    agent_dir="${PIG_CODING_AGENT_DIR:-}"
    if [[ -z "$agent_dir" ]]; then
      local config_root="${PIG_HOME:-}"
      if [[ -z "$config_root" ]]; then
        if [[ -n "${XDG_CONFIG_HOME:-}" ]]; then
          config_root="${XDG_CONFIG_HOME%/}/pig"
        else
          config_root="$HOME/.pig"
        fi
      fi
      if [[ "$config_root" == "~" ]]; then
        config_root="$HOME"
      elif [[ "$config_root" == ~/* ]]; then
        config_root="$HOME/${config_root#~/}"
      fi
      agent_dir="$config_root/agent"
    fi
  fi

  if [[ "$agent_dir" == "~" ]]; then
    agent_dir="$HOME"
  elif [[ "$agent_dir" == ~/* ]]; then
    agent_dir="$HOME/${agent_dir#~/}"
  fi
  printf '%s/auth.json' "${agent_dir%/}"
}

quota_live_file_snapshot() {
  python3 - "$1" <<'PY'
import hashlib
import os
import sys

path = sys.argv[1]
try:
    link = os.lstat(path)
    target = os.stat(path)
    with open(path, "rb") as stream:
        digest = hashlib.sha256(stream.read()).hexdigest()
    print("|".join(str(value) for value in (
        link.st_dev, link.st_ino, link.st_mode, link.st_mtime_ns,
        target.st_dev, target.st_ino, target.st_mode, target.st_mtime_ns,
        target.st_size, digest,
    )))
except FileNotFoundError:
    print("missing")
PY
}

quota_live_probe_cleanup() {
  local result="$1"
  trap - EXIT INT TERM
  if [[ -n "${quota_live_probe_daemon_pid:-}" ]]; then
    kill "$quota_live_probe_daemon_pid" 2>/dev/null || true
    wait "$quota_live_probe_daemon_pid" 2>/dev/null || true
  fi
  if [[ -n "${quota_live_probe_auth_path:-}" && -n "${quota_live_probe_auth_before:-}" ]]; then
    local after
    after="$(quota_live_file_snapshot "$quota_live_probe_auth_path")"
    if [[ "$after" != "$quota_live_probe_auth_before" ]]; then
      echo "FAIL - provider login file changed during the quota probe" >&2
      result=1
    fi
  fi
  if [[ -n "${quota_live_probe_tmp:-}" ]]; then
    rm -rf -- "$quota_live_probe_tmp"
  fi
  exit "$result"
}

quota_live_probe_run() {
  local key="$1" label="$2" auth_path="$3"
  local cmd

  for cmd in go curl python3; do
    command -v "$cmd" >/dev/null 2>&1 || { echo "FAIL - need $cmd on PATH" >&2; return 1; }
  done
  if [[ ! -r "$auth_path" ]]; then
    echo "SKIP - required host login is not readable; no upstream request made" >&2
    return 2
  fi

  quota_live_probe_tmp="$(mktemp -d "${TMPDIR:-/tmp}/el-quota-live-probe.XXXXXX")"
  quota_live_probe_auth_path="$auth_path"
  quota_live_probe_auth_before=""
  quota_live_probe_daemon_pid=""
  trap 'quota_live_probe_cleanup $?' EXIT
  trap 'exit 130' INT
  trap 'exit 143' TERM
  chmod 700 "$quota_live_probe_tmp"
  quota_live_probe_auth_before="$(quota_live_file_snapshot "$auth_path")"


  mkdir -p "$quota_live_probe_tmp/daemon-state" "$quota_live_probe_tmp/cache"
  echo "Building isolated el-web quota probe..." >&2
  go -C "$QUOTA_LIVE_REPO_ROOT/emberloop-web" build -o "$quota_live_probe_tmp/el-web" ./cmd/emberloop-web


  EL_STATE_DIR="$quota_live_probe_tmp/cache" "$quota_live_probe_tmp/el-web" \
    -addr 127.0.0.1:0 -state-dir "$quota_live_probe_tmp/daemon-state" -restart-check 0 \
    >"$quota_live_probe_tmp/daemon.log" 2>&1 &
  quota_live_probe_daemon_pid=$!

  local token="" base="" jar="$quota_live_probe_tmp/cookies.txt"
  for ((attempt = 0; attempt < 100; attempt++)); do
    token="$(cat "$quota_live_probe_tmp/daemon-state/enderbloom/daemon.token" 2>/dev/null || true)"
    base="$(sed -n 's/^el-web on //p' "$quota_live_probe_tmp/daemon.log" | head -n 1)"
    if [[ -n "$token" && -n "$base" ]] && \
      curl --max-time 1 -fsS -H "Authorization: Bearer $token" "$base/api/health" >/dev/null 2>&1; then
      break
    fi
    if ! kill -0 "$quota_live_probe_daemon_pid" 2>/dev/null; then
      echo "FAIL - isolated el-web did not start" >&2
      return 1
    fi
    sleep 0.1
  done
  if [[ -z "$token" || -z "$base" ]] || ! curl --max-time 1 -fsS \
    -H "Authorization: Bearer $token" "$base/api/health" >/dev/null 2>&1; then
    echo "FAIL - isolated el-web did not become healthy" >&2
    return 1
  fi

  curl --max-time 5 -fsS -c "$jar" -H 'Content-Type: application/json' \
    -d '{"username":"quota-live-probe","password":"temporary-probe-password"}' \
    "$base/api/auth/signup" >/dev/null

  local status
  status="$(curl --max-time 35 -sS -o "$quota_live_probe_tmp/response.json" -w '%{http_code}' \
    -b "$jar" -X POST "$base/api/quota/refresh" -H 'Content-Type: application/json' \
    -d "{\"key\":\"$key\"}")" || {
      echo "FAIL - quota refresh request failed" >&2
      return 1
    }
  if [[ "$status" != "200" ]]; then
    echo "FAIL - local quota refresh returned HTTP $status" >&2
    return 1
  fi

  echo "Live $label quota poller result (normalized provider response):"
  python3 - "$quota_live_probe_tmp/response.json" "$key" <<'PY'
import json
import sys

with open(sys.argv[1], encoding="utf-8") as stream:
    providers = json.load(stream).get("providers", [])
provider = next((item for item in providers if item.get("key") == sys.argv[2]), None)
if provider is None:
    print("FAIL - provider missing from quota response", file=sys.stderr)
    sys.exit(1)
print(f"state={provider.get('state', '')} label={provider.get('label', '')} plan={provider.get('plan', '') or '-'}")
if provider.get("state") == "not_configured":
    print("SKIP - provider login is not configured; no upstream request made", file=sys.stderr)
    sys.exit(2)
if provider.get("state") != "ok":
    print("FAIL - provider did not return a current quota snapshot", file=sys.stderr)
    sys.exit(1)
windows = provider.get("windows", [])
expected = ["5-hour", "Weekly"]
if [window.get("label") for window in windows] != expected:
    print(f"FAIL - expected windows {expected}, received {[window.get('label') for window in windows]}", file=sys.stderr)
    sys.exit(1)
for window in windows:
    pct = window.get("pct")
    reset = window.get("resets_at")
    print(f"window={window['label']} pct={pct} resets_at={reset}")
    if not isinstance(pct, int) or not 0 <= pct <= 100 or not isinstance(reset, int) or reset <= 0:
        print("FAIL - quota window has an invalid percent or reset timestamp", file=sys.stderr)
        sys.exit(1)
PY
}
