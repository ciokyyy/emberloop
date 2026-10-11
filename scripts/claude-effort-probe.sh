#!/usr/bin/env bash
# Probe for web-overhaul-2 item 1 (pig reasoning effort): verify claude
# really honors the SDK `effort` option the Daemon passes on session/new.
#
# What it does, step by step:
#  1. Builds el-web and starts a throwaway Daemon (temp state dir, port
#     7942, restart-check off) with the real claude Backend.
#  2. Creates two claude Sessions with the same model, one with
#     effort=low and one with effort=max, and sends both the same
#     reasoning-heavy prompt with a verifiable tail marker.
#  3. Polls each Session until it settles, then prints wall time plus the
#     thinking_chunk and assistant_chunk totals side by side.
#
# Verdict (manual): the max Session should show substantially more thinking
# output than the low one. Similar thinking on both means the SDK effort
# option is not honored and the picker is a placebo; record the numbers in
# the ticket either way. Never runs in the automated check.
#
# Prerequisites: go, node with npx (the pinned claude ACP adapter), a claude
# login the adapter accepts, network access, curl, python3. Costs real
# money: two model turns, one at max effort.
#
# Usage: scripts/claude-effort-probe.sh
#   EL_EFFORT_MODEL=haiku scripts/claude-effort-probe.sh   # cheaper run
set -uo pipefail
export PATH="$HOME/go/bin:$PATH"

MODEL="${EL_EFFORT_MODEL:-sonnet}"
PORT="${EL_EFFORT_PORT:-7942}"
PROMPT='Three logicians walk into a bar. The bartender asks "Does everyone want a drink?" The first logician says "I don'"'"'t know." The second says "I don'"'"'t know." The third says "Yes." Explain step by step why the third can be sure, consider whether the puzzle changes if the first had said "No", and end your reply with exactly PROBE-DONE:YES on its own line.'

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
tmp="$(mktemp -d /tmp/el-effort-probe.XXXXXX)"
trap 'kill "$daemon" 2>/dev/null; rm -rf "$tmp"' EXIT

echo "building el-web..." >&2
go -C "$root/emberloop-web" build -o "$tmp/emberloop-web" ./cmd/emberloop-web
mkdir -p "$tmp/state" "$tmp/work"
"$tmp/emberloop-web" -addr "127.0.0.1:$PORT" -state-dir "$tmp/state" -restart-check 0 >"$tmp/daemon.log" 2>&1 &
daemon=$!
base="http://127.0.0.1:$PORT"
# The Daemon gates /api with its machine token, written to its state dir at start.
token=""
for _ in $(seq 1 50); do
  token="$(cat "$tmp/state/enderbloom/daemon.token" 2>/dev/null)" && [ -n "$token" ] && break
  sleep 0.2
done
for _ in $(seq 1 50); do
  curl -H "Authorization: Bearer $token" -sf "$base/api/health" >/dev/null 2>&1 && break
  sleep 0.2
done

create() { # effort -> session id
  curl -H "Authorization: Bearer $token" -s -X POST "$base/api/sessions" -H 'Content-Type: application/json' \
    -d "$(python3 -c 'import json,sys; print(json.dumps({"harness":"claude","model":sys.argv[1],"effort":sys.argv[2],"workDir":sys.argv[3]}))' "$MODEL" "$1" "$tmp/work")"
}
run_one() { # effort -> stats line
  local effort="$1" id start end status
  id="$(create "$effort" | python3 -c 'import json,sys; print(json.load(sys.stdin)["id"])')"
  echo "session $effort: $id" >&2
  curl -H "Authorization: Bearer $token" -s -X POST "$base/api/sessions/$id/prompt" -H 'Content-Type: application/json' \
    -d "$(python3 -c 'import json,sys; print(json.dumps({"text":sys.argv[1]}))' "$PROMPT")" >/dev/null
  start="$(date +%s)"
  while :; do
    status="$(curl -H "Authorization: Bearer $token" -s "$base/api/sessions/$id" | python3 -c 'import json,sys; print(json.load(sys.stdin)["session"]["status"])')"
    [ "$status" = idle ] || [ "$status" = failed ] && break
    sleep 5
  done
  end="$(date +%s)"
  curl -H "Authorization: Bearer $token" -s "$base/api/sessions/$id" | EFFORT="$effort" WALL="$((end - start))" STATUS="$status" python3 -c '
import json, os, sys
body = json.load(sys.stdin)
think = sum(len(e.get("text", "")) for e in body["events"] if e.get("kind") == "thinking_chunk")
chat = sum(len(e.get("text", "")) for e in body["events"] if e.get("kind") == "assistant_chunk")
done = any(e.get("kind") == "assistant_chunk" and "PROBE-DONE:YES" in e.get("text", "") for e in body["events"])
print(f"{os.environ[\"EFFORT\"]:>4}  status={os.environ[\"STATUS\"]:<6} wall={os.environ[\"WALL\"]}s  thinking_chars={think}  reply_chars={chat}  marker={\"yes\" if done else \"NO\"}")'
}

echo "model=$MODEL (override with EL_EFFORT_MODEL)" >&2
echo "effort  status  wall  thinking_chars  reply_chars  marker"
run_one low
run_one max
echo "Compare the thinking_chars columns: max should clearly exceed low." >&2
