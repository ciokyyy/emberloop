#!/usr/bin/env bash
# Env-gated live end-to-end probe for the per-Session agent-browser path.
# It creates isolated Daemons and real Sessions; each model call can cost money.
# Requires go, curl, python3, agent-browser in el's bin dir, Chromium/Chrome,
# and authenticated Claude/Pig ACP/Pig SDK model providers.
# Usage: EL_LIVE_BROWSER=1 EL_LIVE_CLAUDE_MODEL=haiku \
#   EL_LIVE_PIG_ACP_MODEL=provider/model EL_LIVE_PIG_SDK_MODEL=provider/model \
#   scripts/browser-probe.sh
# Verdicts are printed per Backend. A failed read-image verdict means that
# Backend should use snapshot-only Designer guidance until image forwarding works.
set -uo pipefail
export PATH="$HOME/go/bin:$PATH"

if [ "${EL_LIVE_BROWSER:-}" != 1 ]; then
  echo "SKIP - set EL_LIVE_BROWSER=1 to run the real-agent browser probe" >&2
  exit 2
fi
CLAUDE_MODEL="${EL_LIVE_CLAUDE_MODEL:-}"
PIG_ACP_MODEL="${EL_LIVE_PIG_ACP_MODEL:-}"
PIG_SDK_MODEL="${EL_LIVE_PIG_SDK_MODEL:-}"
for item in "claude:$CLAUDE_MODEL" "pig-acp:$PIG_ACP_MODEL" "pig-sdk:$PIG_SDK_MODEL"; do
  [ -n "${item#*:}" ] || { echo "FAIL - set EL_LIVE_${item%%:*}_MODEL to a cheap vision-capable model" >&2; exit 1; }
done
browser_bin="${EL_BIN_DIR:-$HOME/.emberloop/bin}/agent-browser"
[ -x "$browser_bin" ] || { echo "FAIL - install agent-browser with el deps (expected $browser_bin)" >&2; exit 1; }
for bin in go curl python3; do
  command -v "$bin" >/dev/null 2>&1 || { echo "FAIL - need $bin on PATH" >&2; exit 1; }
done
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
tmp="$(mktemp -d /tmp/el-browser-probe.XXXXXX)"
daemon=""
cleanup() {
  if [ -n "$daemon" ] && kill -0 "$daemon" 2>/dev/null; then kill "$daemon" 2>/dev/null || true; wait "$daemon" 2>/dev/null || true; fi
  rm -rf "$tmp"
}
trap cleanup EXIT

echo "Building Daemon..." >&2
go -C "$root/emberloop-web" build -o "$tmp/emberloop-web" ./cmd/emberloop-web || exit 1
mkdir -p "$tmp/work"
cat >"$tmp/work/browser-probe.html" <<'HTML'
<!doctype html><html><head><meta name="viewport" content="width=device-width, initial-scale=1"><style>body{background:#132b45;color:white;font:24px sans-serif;padding:24px}.card{background:#e96b42;color:#111;padding:22px;border-radius:14px}strong{color:#ffe45e}</style></head><body><main class="card"><strong>ORANGE HARBOR</strong><p>A yellow lighthouse stands beside a blue sea.</p></main></body></html>
HTML

start_daemon() {
  local mode="$1" dir="$tmp/$mode"
  mkdir -p "$dir/state"
  if [ "$mode" = pig-sdk ]; then
    EL_PIG_SDK=1 EL_SETTINGS="$dir/settings.json" "$tmp/emberloop-web" -addr 127.0.0.1:0 -state-dir "$dir/state" -restart-check 0 >"$dir/daemon.log" 2>&1 &
  else
    env -u EL_PIG_SDK EL_SETTINGS="$dir/settings.json" "$tmp/emberloop-web" -addr 127.0.0.1:0 -state-dir "$dir/state" -restart-check 0 >"$dir/daemon.log" 2>&1 &
  fi
  daemon=$!
  local ready=0
  for _ in $(seq 1 150); do
    if ! kill -0 "$daemon" 2>/dev/null; then tail -30 "$dir/daemon.log" >&2; return 1; fi
    PROBE_TOKEN="$(cat "$dir/state/enderbloom/daemon.token" 2>/dev/null || true)"
    PROBE_BASE="$(sed -n 's/^el-web on //p' "$dir/daemon.log" | head -1)"
    if [ -n "$PROBE_TOKEN" ] && [ -n "$PROBE_BASE" ] && curl -fsS --max-time 2 -H "Authorization: Bearer $PROBE_TOKEN" "$PROBE_BASE/api/health" >/dev/null 2>&1; then ready=1; break; fi
    sleep .2
  done
  [ "$ready" = 1 ] || { echo "FAIL - $mode Daemon did not become healthy" >&2; return 1; }
  PROBE_JAR="$dir/cookies"
  curl -fsS -c "$PROBE_JAR" -H 'Content-Type: application/json' -d '{"username":"probe","password":"probe-password"}' "$PROBE_BASE/api/auth/signup" >/dev/null || return 1
}
stop_daemon() {
  if [ -n "$daemon" ]; then kill "$daemon" 2>/dev/null || true; wait "$daemon" 2>/dev/null || true; daemon=""; fi
}

probe_session() {
  local name="$1" harness="$2" model="$3" work="$4" id body start prompt response status
  body="$(python3 -c 'import json,sys; print(json.dumps({"harness":sys.argv[1],"model":sys.argv[2],"workDir":sys.argv[3]}))' "$harness" "$model" "$work")"
  id="$(curl -fsS -b "$PROBE_JAR" -H 'Content-Type: application/json' -d "$body" "$PROBE_BASE/api/sessions" | python3 -c 'import json,sys; print(json.load(sys.stdin)["id"])')" || return 1
  prompt="In bash, print AGENT_BROWSER_SESSION and PATH. In execute, print AGENT_BROWSER_SESSION and PATH. Use agent-browser open file://$work/browser-probe.html; take and report a snapshot; set the viewport to 390x844; save a JPEG screenshot at \"\$AGENT_BROWSER_SCREENSHOT_DIR/probe.jpg\"; then use the el read tool on that exact file. Report both tool environments, whether the snapshot contains ORANGE HARBOR, and the screenshot-only visual colors (page background navy blue, card orange) plus what the image depicts. Do not infer colors from page source or guess if image reading failed."
  body="$(python3 -c 'import json,sys; print(json.dumps({"text":sys.argv[1]}))' "$prompt")"
  curl -fsS -b "$PROBE_JAR" -X POST -H 'Content-Type: application/json' -d "$body" "$PROBE_BASE/api/sessions/$id/prompt" >/dev/null || return 1
  echo "--- $name session=$id model=$model" >&2
  local deadline=$((SECONDS+600))
  while [ "$SECONDS" -lt "$deadline" ]; do
    response="$(curl -fsS --max-time 10 -b "$PROBE_JAR" "$PROBE_BASE/api/sessions/$id" 2>/dev/null || true)"
    status="$(printf '%s' "$response" | python3 -c 'import json,sys; print(json.load(sys.stdin)["session"]["status"])' 2>/dev/null || true)"
    [ "$status" = idle ] && break
    sleep 3
  done
  [ "$status" = idle ] || { echo "FAIL $name - session did not settle; artifacts: $tmp"; return 1; }
  printf '%s' "$response" >"$tmp/$name-session.json"
  python3 - "$name" "$response" <<'PY'
import json, sys
name, raw = sys.argv[1:]
data = json.loads(raw)
events = data.get("events", [])
text = "".join(e.get("text", "") for e in events if e.get("kind") == "assistant_chunk")
checks = {
    "env inherited": "AGENT_BROWSER_SESSION" in text and "PATH" in text and "bash" in text.lower() and "execute" in text.lower(),
    "snapshot works": "ORANGE HARBOR" in text,
    "screenshot read reaches model": "navy" in text.lower() and "orange" in text.lower(),
}
print(f"{name} verdict: " + "; ".join(f"{'PASS' if ok else 'FAIL'} {label}" for label, ok in checks.items()))
for label, ok in checks.items():
    if not ok:
        print(f"  {label} evidence missing; transcript: {text[-2500:]}")
sys.exit(0 if all(checks.values()) else 1)
PY
}

result=0
for backend in claude pig-acp; do
  start_daemon "$backend" || exit 1
  if [ "$backend" = claude ]; then model="$CLAUDE_MODEL"; harness=claude; else model="$PIG_ACP_MODEL"; harness=pig; fi
  probe_session "$backend" "$harness" "$model" "$tmp/work" || result=1
  stop_daemon
done
start_daemon pig-sdk || exit 1
probe_session pig-sdk pig "$PIG_SDK_MODEL" "$tmp/work" || result=1
if [ "$result" -ne 0 ]; then
  echo "At least one Backend failed. Screenshot-image failures require snapshot-only Designer fallback for that Backend." >&2
  exit 1
fi
echo "PASS - all three browser Backend probes passed"
