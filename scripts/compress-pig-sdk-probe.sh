#!/usr/bin/env bash
# Probe for web-v1 ticket 32 (live probes): compress a real pig SDK Session
# by Handoff restart and prove the new handle answers from the Handoff alone.
#
# What it does, step by step:
#  1. Builds el-web and starts a throwaway Daemon (temp state dir, temp
#     EL_SETTINGS, port 7951, restart-check off) with EL_PIG_SDK=1 so the
#     pig Backend is the in-process SDK one.
#  2. Sets compress_model to PIG_LIVE_MODEL through PUT /api/settings.
#  3. Creates a pig Session, plants a random codeword, runs one filler turn
#     so the fact is not the last prompt, then compresses through
#     POST /api/sessions/{id}/compress and waits for the compressed Event
#     plus idle.
#  4. Asks for the codeword and checks the new handle replies FACT:<codeword>.
#
# Verdict (manual): PASS when the post-compress answer carries the planted
# codeword; FAIL otherwise. A missing codeword means the Handoff dropped the
# fact. Never runs in the automated check.
#
# Prerequisites: go, curl, python3, network access, and pig auth for
# PIG_LIVE_MODEL. Costs real money: several model turns plus the summarizer
# Handoff call.
#
# Usage: scripts/compress-pig-sdk-probe.sh
#   PIG_LIVE_MODEL=provider/model scripts/compress-pig-sdk-probe.sh
set -uo pipefail
export PATH="$HOME/go/bin:$PATH"

MODEL="${PIG_LIVE_MODEL:-}"
PORT="${EL_COMPRESS_SDK_PORT:-7951}"

[ -n "$MODEL" ] || { echo "FAIL - set PIG_LIVE_MODEL to a healthy provider/model"; exit 1; }
for bin in go curl python3; do
  command -v "$bin" >/dev/null || { echo "FAIL - need $bin on PATH"; exit 1; }
done

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
tmp="$(mktemp -d /tmp/el-compress-sdk-probe.XXXXXX)"
daemon=""
trap 'kill "$daemon" 2>/dev/null; rm -rf "$tmp"' EXIT

CODE="FALCON-$(python3 -c 'import secrets; print("%04X" % secrets.randbelow(65536))')"
echo "model=$MODEL code=$CODE" >&2

echo "building el-web..." >&2
go -C "$root/emberloop-web" build -o "$tmp/emberloop-web" ./cmd/emberloop-web || { echo "FAIL - build failed"; exit 1; }
mkdir -p "$tmp/state" "$tmp/work"
export EL_SETTINGS="$tmp/settings.json"
EL_PIG_SDK=1 "$tmp/emberloop-web" -addr "127.0.0.1:$PORT" -state-dir "$tmp/state" -restart-check 0 >"$tmp/daemon.log" 2>&1 &
daemon=$!
base="http://127.0.0.1:$PORT"
token=""
for _ in $(seq 1 50); do
  token="$(cat "$tmp/state/enderbloom/daemon.token" 2>/dev/null)" && [ -n "$token" ] && break
  sleep 0.2
done
[ -n "$token" ] || { echo "FAIL - no daemon token; log:" >&2; tail -20 "$tmp/daemon.log" >&2; exit 1; }
for _ in $(seq 1 50); do
  curl -H "Authorization: Bearer $token" -sf "$base/api/health" >/dev/null 2>&1 && break
  sleep 0.2
done

# Sessions belong to a signed-in User: the first signup of this fresh state dir is the admin.
jar="$tmp/jar"
curl -sf -c "$jar" -H 'Content-Type: application/json' -d '{"username":"probe","password":"probe-password"}' "$base/api/auth/signup" >/dev/null || { echo "FAIL - signup refused"; exit 1; }

echo "--- setting compress_model=$MODEL" >&2
code="$(curl -b "$jar" -s -o "$tmp/settings.out" -w '%{http_code}' -X PUT "$base/api/settings" -H 'Content-Type: application/json' \
  -d "$(python3 -c 'import json,sys; print(json.dumps({"compress":{"compress_model":sys.argv[1]}}))' "$MODEL")")"
[ "$code" = 200 ] || { echo "FAIL - compress_model rejected ($code): $(cat "$tmp/settings.out")"; exit 1; }

echo "--- creating pig SDK session" >&2
id="$(curl -b "$jar" -s -X POST "$base/api/sessions" -H 'Content-Type: application/json' \
  -d "$(python3 -c 'import json,sys; print(json.dumps({"harness":"pig","model":sys.argv[1],"workDir":sys.argv[2]}))' "$MODEL" "$tmp/work")" \
  | python3 -c 'import json,sys; print(json.load(sys.stdin)["id"])')"
[ -n "$id" ] || { echo "FAIL - no session id"; exit 1; }
echo "session=$id" >&2

status_of() { curl -b "$jar" -s "$base/api/sessions/$id" | python3 -c 'import json,sys; print(json.load(sys.stdin)["session"]["status"])'; }
assistant_has() { # marker -> 0 when the joined assistant text holds it
  curl -b "$jar" -s "$base/api/sessions/$id" | MARK="$1" python3 -c '
import json, os, sys
evs = json.load(sys.stdin)["events"]
text = "".join(e.get("text", "") for e in evs if e.get("kind") == "assistant_chunk")
sys.exit(0 if os.environ["MARK"] in text else 1)'
}
event_has() { # kind -> 0 when any event of that kind exists
  curl -b "$jar" -s "$base/api/sessions/$id" | KIND="$1" python3 -c '
import json, os, sys
sys.exit(0 if any(e.get("kind") == os.environ["KIND"] for e in json.load(sys.stdin)["events"]) else 1)'
}
wait_idle() { # seconds -> 0 on idle
  for _ in $(seq 1 "$((($1 + 4) / 5))"); do
    [ "$(status_of)" = idle ] && return 0
    sleep 5
  done
  return 1
}
prompt() { curl -b "$jar" -s -o /dev/null -X POST "$base/api/sessions/$id/prompt" -H 'Content-Type: application/json' \
  -d "$(python3 -c 'import json,sys; print(json.dumps({"text":sys.argv[1]}))' "$1")"; }

echo "--- planting fact" >&2
prompt "Please remember this codeword for later in this session: $CODE. It will be asked again after a context compress. Reply with exactly PLANTED-OK and nothing else."
wait_idle 240 || { echo "FAIL - plant turn never settled"; exit 1; }
assistant_has "PLANTED-OK" || { echo "FAIL - PLANTED-OK missing from transcript"; exit 1; }
echo "PASS - fact planted" >&2

echo "--- filler turn (fact is no longer the last prompt)" >&2
prompt "What is 17 + 25? Reply with exactly FILLER-42 and nothing else."
wait_idle 180 || { echo "FAIL - filler turn never settled"; exit 1; }
assistant_has "FILLER-42" || { echo "FAIL - FILLER-42 missing from transcript"; exit 1; }

echo "--- compressing" >&2
code="$(curl -b "$jar" -s -o "$tmp/compress.out" -w '%{http_code}' -X POST "$base/api/sessions/$id/compress" -H 'Content-Type: application/json' -d '{}')"
[ "$code" = 202 ] || { echo "FAIL - compress refused ($code): $(cat "$tmp/compress.out")"; exit 1; }
for _ in $(seq 1 120); do
  event_has "compressed" && [ "$(status_of)" = idle ] && break
  sleep 5
done
event_has "compressed" || { echo "FAIL - no compressed event after 600s"; exit 1; }
wait_idle 120 || { echo "FAIL - session never idle after compress"; exit 1; }
echo "PASS - compressed event present, session idle" >&2

echo "--- asking for the fact from the new handle" >&2
prompt "What was the codeword I asked you to remember earlier? Reply with exactly FACT:$CODE and nothing else."
wait_idle 300 || { echo "FAIL - fact turn never settled"; exit 1; }
if assistant_has "FACT:$CODE"; then
  echo "PASS - new handle answered FACT:$CODE from the Handoff alone"
else
  echo "FAIL - FACT:$CODE missing after compress; the Handoff dropped the fact"
  exit 1
fi
