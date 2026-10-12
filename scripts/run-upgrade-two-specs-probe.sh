#!/usr/bin/env bash
# Live probe for Run-upgrade ticket 21: two independent specs run concurrently
# against real Backends, each on its own spec/<slug> integration branch.
#
# It creates two trivial spec trees in one disposable repo, starts both Runs
# through /api/runs before either finishes, waits for both real Worker Sessions,
# and verifies distinct spec branches and live Run states. It then pauses both
# Runs to stop the Workers without merging or changing the fixture's main.
#
# Verdict: PASS when both Runs have active real Worker Sessions concurrently,
# each on a different spec branch, and both pause cleanly. Costs two real
# Worker starts. Manual only; never part of `.emberloop/check`.
#
# Prerequisites: go, git, curl, python3 and an authenticated real Backend.
# Claude also needs node/npm access for its pinned ACP adapter. Default:
# claude/haiku. Usage:
#   scripts/run-upgrade-two-specs-probe.sh
#   EL_RUN_UPGRADE_HARNESS=pig PIG_LIVE_MODEL=provider/model scripts/run-upgrade-two-specs-probe.sh
set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/run-upgrade-probe-lib.sh"

HARNESS="${EL_RUN_UPGRADE_HARNESS:-claude}"
if [ -n "${EL_RUN_UPGRADE_MODEL:-}" ]; then
  MODEL="$EL_RUN_UPGRADE_MODEL"
elif [ "$HARNESS" = pig ]; then
  MODEL="${PIG_LIVE_MODEL:-}"
else
  MODEL=haiku
fi
TIMEOUT="${EL_RUN_UPGRADE_TIMEOUT:-600}"
case "$HARNESS" in
  claude) probe_require node npx || exit 1 ;;
  pig) [ -n "$MODEL" ] || { echo "FAIL - set PIG_LIVE_MODEL or EL_RUN_UPGRADE_MODEL" >&2; exit 1; } ;;
  *) echo "FAIL - choose claude or pig for EL_RUN_UPGRADE_HARNESS" >&2; exit 1 ;;
esac

probe_init two-specs
repo="$PROBE_DIR/repo"
probe_git_init "$repo"
probe_add_green_check "$repo"
cat >"$repo/AGENTS.md" <<'AGENTS'
# AGENTS.md — Two-spec probe

## Purpose

A disposable fixture for testing two concurrent live Runs.

## Child DOX Index

No child docs.
AGENTS
for slug in probe-north probe-south; do
  mkdir -p "$repo/.scratch/$slug/issues"
  cat >"$repo/.scratch/$slug/spec.md" <<SPEC
# $slug live probe

Wait for the probe operator after receiving the task. Do not edit or submit;
the probe checks only concurrent Run creation and branch isolation.
SPEC
  if [ "$slug" = probe-north ]; then ticket_id=01-probe-north; else ticket_id=01-probe-south; fi
  cat >"$repo/.scratch/$slug/issues/$ticket_id.md" <<TICKET
# Concurrent Run probe

**Blocked by:** None

**Status:** open

- [ ] Create ${slug}.txt containing ${slug} after the probe operator resumes the test.
TICKET
done
probe_git_commit "$repo" 'two-spec probe fixtures'
NORTH_TICKET=01-probe-north
SOUTH_TICKET=01-probe-south



north_spec="$repo/.scratch/probe-north/spec.md"
south_spec="$repo/.scratch/probe-south/spec.md"
echo "starting two concurrent real Runs: harness=$HARNESS model=$MODEL" >&2
north="$(probe_start_run "$north_spec" "$repo" "$HARNESS" "$MODEL")"
north_id="$(printf '%s' "$north" | python3 -c 'import json,sys; print(json.load(sys.stdin)["run"]["id"])')"
probe_track_run "$north_id"
south="$(probe_start_run "$south_spec" "$repo" "$HARNESS" "$MODEL")"
south_id="$(printf '%s' "$south" | python3 -c 'import json,sys; print(json.load(sys.stdin)["run"]["id"])')"
probe_track_run "$south_id"
echo "north=$north_id south=$south_id" >&2

north_worker=""
south_worker=""
north_branch=""
south_branch=""
deadline=$((SECONDS + TIMEOUT))
while [ "$SECONDS" -lt "$deadline" ]; do
  north_state="$(probe_api_get "/api/runs/$north_id")"
  south_state="$(probe_api_get "/api/runs/$south_id")"
  north_info="$(printf '%s' "$north_state" | TICKET_ID="$NORTH_TICKET" python3 -c '
import json,os,sys
r=json.load(sys.stdin)["run"]; t=next((t for t in r.get("tickets",[]) if t.get("id")==os.environ["TICKET_ID"]),{})
print(json.dumps({"status":r.get("status"),"branch":r.get("specBranch",""),"worker":t.get("workerSession","")}))')"
  south_info="$(printf '%s' "$south_state" | TICKET_ID="$SOUTH_TICKET" python3 -c '
import json,os,sys
r=json.load(sys.stdin)["run"]; t=next((t for t in r.get("tickets",[]) if t.get("id")==os.environ["TICKET_ID"]),{})
print(json.dumps({"status":r.get("status"),"branch":r.get("specBranch",""),"worker":t.get("workerSession","")}))')"
  north_worker="$(printf '%s' "$north_info" | python3 -c 'import json,sys; print(json.load(sys.stdin)["worker"])')"
  south_worker="$(printf '%s' "$south_info" | python3 -c 'import json,sys; print(json.load(sys.stdin)["worker"])')"
  north_branch="$(printf '%s' "$north_info" | python3 -c 'import json,sys; print(json.load(sys.stdin)["branch"])')"
  south_branch="$(printf '%s' "$south_info" | python3 -c 'import json,sys; print(json.load(sys.stdin)["branch"])')"
  north_status="$(printf '%s' "$north_info" | python3 -c 'import json,sys; print(json.load(sys.stdin)["status"])')"
  south_status="$(printf '%s' "$south_info" | python3 -c 'import json,sys; print(json.load(sys.stdin)["status"])')"
  if [ "$north_status" != running ] || [ "$south_status" != running ]; then
    echo "FAIL - concurrent Runs are not both running: north=$north_status south=$south_status" >&2
    exit 1
  fi
  if [ -n "$north_worker" ] && [ -n "$south_worker" ] && [ -n "$north_branch" ] && [ -n "$south_branch" ]; then
    break
  fi
  sleep 2
done
[ -n "$north_worker" ] && [ -n "$south_worker" ] || { echo "FAIL - both Workers did not start within ${TIMEOUT}s" >&2; exit 1; }
[ "$north_worker" != "$south_worker" ] || { echo "FAIL - both Runs reference the same Worker Session" >&2; exit 1; }
[ "$north_branch" != "$south_branch" ] || { echo "FAIL - both Runs use spec branch $north_branch" >&2; exit 1; }
case "$north_branch" in spec/*) ;; *) echo "FAIL - unexpected north spec branch: $north_branch" >&2; exit 1 ;; esac
case "$south_branch" in spec/*) ;; *) echo "FAIL - unexpected south spec branch: $south_branch" >&2; exit 1 ;; esac

north_session="$(probe_api_get "/api/sessions/$north_worker")"
south_session="$(probe_api_get "/api/sessions/$south_worker")"
for item in "$north_session" "$south_session"; do
  printf '%s' "$item" | HARNESS="$HARNESS" MODEL="$MODEL" python3 -c '
import json,os,sys
meta=json.load(sys.stdin)["session"]
assert meta.get("harness")==os.environ["HARNESS"], meta
assert meta.get("model")==os.environ["MODEL"], meta
assert meta.get("status") in ("working","idle"), meta
'
done

north_pause="$(probe_api_post "/api/runs/$north_id/pause" '{}')"
south_pause="$(probe_api_post "/api/runs/$south_id/pause" '{}')"
for item in "$north_pause" "$south_pause"; do
  printf '%s' "$item" | python3 -c 'import json,sys; assert json.load(sys.stdin)["run"]["status"]=="paused"'
done
printf 'PASS - concurrent Runs %s (%s) and %s (%s) used distinct real Worker Sessions and spec branches; both Runs paused.\n' "$north_id" "$north_branch" "$south_id" "$south_branch"
