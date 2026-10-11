#!/usr/bin/env bash
# Probe for web-overhaul-2 item 4 (runs-actions): drive a 2-ticket Run
# through /api/runs against real agents and exercise the per-ticket
# Steer, Retry, Park and Discard routes plus the needs-you banner data.
#
# What it does, step by step:
#  1. Builds el-web and starts a throwaway Daemon (temp state dir, port
#     7943, restart-check off) with the real Backend for HARNESS.
#  2. Creates a fixture repo with a 2-ticket spec (two trivial one-line
#     tickets, no blockers) and starts a Run through POST /api/runs.
#  3. Lists GET /api/runs (the sidebar Run picker endpoint) and polls the
#     Run until ticket 01 has a worker Session.
#  4. Steers ticket 01 through its ticket route and polls the worker
#     transcript until the steer text lands (proves worker-only delivery).
#  5. Parks ticket 02 (state reads parked), then retries it (unparks;
#     RetryGen bumps so the driver restarts the ticket at once).
#  6. Prints each ticket's state, needsYou reason and last refusal.
#
# Verdict (manual): every route answers 200 with a Run snapshot, the steer
# text shows in the worker transcript, 02 reads parked then leaves it, and
# the table carries state/needsYou per ticket for the banner. A needs-you
# reason only appears if a ticket actually stalls (same refusal x3, 10 idle
# minutes, 3 stall nudges): do not wait for one here. Never runs in the
# automated check.
#
# Prerequisites: go, git, curl, python3, network access, and a login for
# HARNESS (claude via the pinned ACP adapter, pig via its auth). Costs real
# money: two workers start and implement until killed.
#
# Usage: scripts/runs-actions-probe.sh
#   EL_RUNS_PROBE_HARNESS=pig EL_RUNS_PROBE_MODEL=sonnet scripts/runs-actions-probe.sh
set -uo pipefail
export PATH="$HOME/go/bin:$PATH"

HARNESS="${EL_RUNS_PROBE_HARNESS:-claude}"
MODEL="${EL_RUNS_PROBE_MODEL:-haiku}"
PORT="${EL_RUNS_PROBE_PORT:-7943}"

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
tmp="$(mktemp -d /tmp/el-runs-actions-probe.XXXXXX)"
trap 'kill "$daemon" 2>/dev/null; rm -rf "$tmp"' EXIT

echo "building el-web..." >&2
go -C "$root/emberloop-web" build -o "$tmp/emberloop-web" ./cmd/emberloop-web

export GIT_AUTHOR_NAME=el-probe GIT_AUTHOR_EMAIL=probe@example.com
export GIT_COMMITTER_NAME=el-probe GIT_COMMITTER_EMAIL=probe@example.com
export GIT_CONFIG_NOSYSTEM=1
repo="$tmp/repo"
mkdir -p "$repo/.scratch/probe/issues"
git -C "$repo" init -qb main . >/dev/null
cat >"$repo/.scratch/probe/spec.md" <<'EOF'
# Probe spec

Two trivial tickets with no blockers.
EOF
cat >"$repo/.scratch/probe/issues/01-first.md" <<'EOF'
# 01: First

**Blocked by:** None

**Status:** open

- [ ] write the line `probe-one` into probe-one.txt
EOF
cat >"$repo/.scratch/probe/issues/02-second.md" <<'EOF'
# 02: Second

**Blocked by:** None

**Status:** open

- [ ] write the line `probe-two` into probe-two.txt
EOF
git -C "$repo" add -A >/dev/null
git -C "$repo" commit -qm spec >/dev/null

mkdir -p "$tmp/state"
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

# Sessions and Runs belong to a signed-in User: the first signup of this fresh state dir is the admin.
jar="$tmp/jar"
curl -sf -c "$jar" -H 'Content-Type: application/json' -d '{"username":"probe","password":"probe-password"}' "$base/api/auth/signup" >/dev/null

spec="$repo/.scratch/probe/spec.md"
echo "starting run: harness=$HARNESS model=$MODEL" >&2
run="$(curl -b "$jar" -s -X POST "$base/api/runs" -H 'Content-Type: application/json' \
  -d "$(python3 -c 'import json,sys; print(json.dumps({"spec":sys.argv[1],"repo":sys.argv[2],"harness":sys.argv[3],"model":sys.argv[4]}))' "$spec" "$repo" "$HARNESS" "$MODEL")")"
runid="$(echo "$run" | python3 -c 'import json,sys; print(json.load(sys.stdin)["run"]["id"])')"
echo "run=$runid" >&2

echo "--- GET /api/runs (picker endpoint)" >&2
curl -b "$jar" -s "$base/api/runs" | python3 -c 'import json,sys; print("\n".join(f"{r[\"id\"]} {r[\"status\"]} {r[\"spec\"]}" for r in json.load(sys.stdin)["runs"]))'

getrun() { curl -b "$jar" -s "$base/api/runs/$runid"; }
echo "waiting for ticket 01 worker (up to 180s)..." >&2
worker=""
for _ in $(seq 1 60); do
  worker="$(getrun | python3 -c 'import json,sys; ts={t["id"]:t for t in json.load(sys.stdin)["run"]["tickets"]}; print(ts["01-first"].get("workerSession") or "")')"
  [ -n "$worker" ] && break
  sleep 3
done
[ -n "$worker" ] || { echo "NO WORKER for 01-first; daemon log:" >&2; tail -20 "$tmp/daemon.log" >&2; exit 1; }
echo "worker=$worker" >&2

echo "--- steer ticket 01 through its route" >&2
curl -b "$jar" -s -X POST "$base/api/runs/$runid/tickets/01-first/steer" -H 'Content-Type: application/json' \
  -d '{"text":"PROBE-STEER: keep going, and end one assistant message with PROBE-STEER-ACK."}' \
  | python3 -c 'import json,sys; print("steer:", json.load(sys.stdin)["run"]["id"])'
echo "waiting for the steer text in the worker transcript (up to 120s)..." >&2
for _ in $(seq 1 40); do
  curl -b "$jar" -s "$base/api/sessions/$worker" | grep -q PROBE-STEER && break
  sleep 3
done
curl -b "$jar" -s "$base/api/sessions/$worker" | grep -q PROBE-STEER && echo "steer landed in worker transcript" || { echo "STEER MISSING from transcript" >&2; exit 1; }

echo "--- park ticket 02, then retry (unpark)" >&2
curl -b "$jar" -s -X POST "$base/api/runs/$runid/tickets/02-second/park" \
  | python3 -c 'import json,sys; print("park:", {t["id"]:t["state"] for t in json.load(sys.stdin)["run"]["tickets"]})'
getrun | python3 -c 'import json,sys; ts={t["id"]:t for t in json.load(sys.stdin)["run"]["tickets"]}; assert ts["02-second"]["state"]=="parked", ts; print("02-second reads parked")'
curl -b "$jar" -s -X POST "$base/api/runs/$runid/tickets/02-second/retry" -H 'Content-Type: application/json' -d '{"text":""}' \
  | python3 -c 'import json,sys; print("retry:", {t["id"]:t["state"] for t in json.load(sys.stdin)["run"]["tickets"]})'

echo "--- ticket table" >&2
getrun | python3 -c '
import json, sys
for t in json.load(sys.stdin)["run"]["tickets"]:
    print(f"{t[\"id\"]:>10} state={t[\"state\"]:<10} needsYou={t.get(\"needsYou\") or \"-\":.80} refusal={(t.get(\"lastRefusal\") or \"-\"):.80}")'
echo "Probe done: routes answered, steer landed, park/unpark cycled. Kill leaves the run behind (temp state $tmp/state)." >&2
