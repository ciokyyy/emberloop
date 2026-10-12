#!/usr/bin/env bash
# Live probe for Run-upgrade ticket 21: switch a paused Ticket's real Worker
# Backend/model, then manually resume it on the same branch with a Handoff.
#
# The probe starts a throwaway one-ticket Run on Pig (default), asks its Worker
# to wait for the operator, pauses the Ticket, and changes its active
# Worker/Reviewer configuration. It verifies Switch itself starts no Session,
# then manually resumes and checks the replacement Worker uses the selected
# configuration, carries a Handoff, and remains on the original Ticket branch.
# It pauses again before cleanup.
#
# Verdict: PASS when the paused Ticket stays paused and on its branch through
# Switch, and explicit Resume starts the selected Backend/model with a Handoff.
# Manual only; never part of `.emberloop/check`.
#
# Prerequisites: go, git, curl, python3 and authenticated real Backend(s). Set
# `PIG_LIVE_MODEL`, `EL_RUN_UPGRADE_SWITCH_FROM_MODEL` and/or
# `EL_RUN_UPGRADE_SWITCH_MODEL`. Defaults to Pig -> Claude/haiku. Usage:
#   scripts/run-upgrade-switch-probe.sh
#   PIG_LIVE_MODEL=provider/model scripts/run-upgrade-switch-probe.sh
set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/run-upgrade-probe-lib.sh"

SOURCE_HARNESS="${EL_RUN_UPGRADE_SWITCH_FROM:-pig}"
TARGET_HARNESS="${EL_RUN_UPGRADE_SWITCH_HARNESS:-claude}"
TIMEOUT="${EL_RUN_UPGRADE_TIMEOUT:-600}"
case "$SOURCE_HARNESS" in
  claude) SOURCE_MODEL="${EL_RUN_UPGRADE_SWITCH_FROM_MODEL:-haiku}" ;;
  pig) SOURCE_MODEL="${EL_RUN_UPGRADE_SWITCH_FROM_MODEL:-${PIG_LIVE_MODEL:-}}" ;;
  *) echo "FAIL - switch source must be claude or pig" >&2; exit 1 ;;
esac
case "$TARGET_HARNESS" in
  claude) TARGET_MODEL="${EL_RUN_UPGRADE_SWITCH_MODEL:-haiku}" ;;
  pig) TARGET_MODEL="${EL_RUN_UPGRADE_SWITCH_MODEL:-${PIG_LIVE_MODEL:-}}" ;;
  *) echo "FAIL - switch target must be claude or pig" >&2; exit 1 ;;
esac
[ -n "$SOURCE_MODEL" ] && [ -n "$TARGET_MODEL" ] || { echo "FAIL - configure a model for both source and target Backends" >&2; exit 1; }
[ "$SOURCE_HARNESS" != "$TARGET_HARNESS" ] || [ "$SOURCE_MODEL" != "$TARGET_MODEL" ] || { echo "FAIL - Switch must change the Backend or model" >&2; exit 1; }
if [ "$SOURCE_HARNESS" = claude ] || [ "$TARGET_HARNESS" = claude ]; then probe_require node npx || exit 1; fi
for model in "$SOURCE_MODEL" "$TARGET_MODEL"; do
  case "$model" in
    opus|sonnet|haiku|*/?*) ;;
    *) echo "FAIL - unsupported model name $model" >&2; exit 1 ;;
  esac
done

probe_init switch
repo="$PROBE_DIR/repo"
probe_git_init "$repo"
probe_add_green_check "$repo"
mkdir -p "$repo/.scratch/switch-probe/issues"
cat >"$repo/AGENTS.md" <<'AGENTS'
# AGENTS.md — Switch probe

## Purpose

A disposable fixture for a live paused-Ticket Switch probe.

## Child DOX Index

No child docs.
AGENTS
cat >"$repo/.scratch/switch-probe/spec.md" <<'SPEC'
# Switch live probe

This fixture exercises a Ticket pause, Backend Switch and manual Resume.
SPEC
cat >"$repo/.scratch/switch-probe/issues/01-switch-probe.md" <<'TICKET'
# 01: Switch probe

**Blocked by:** None

**Status:** open

- [ ] Create `probe.txt` containing `switch-live-probe` after the operator resumes the paused Worker.
TICKET
probe_git_commit "$repo" 'Switch probe fixture'
spec="$repo/.scratch/switch-probe/spec.md"

run="$(probe_start_run "$spec" "$repo" "$SOURCE_HARNESS" "$SOURCE_MODEL")"
runid="$(printf '%s' "$run" | python3 -c 'import json,sys; print(json.load(sys.stdin)["run"]["id"])')"
probe_track_run "$runid"
echo "run=$runid starting $SOURCE_HARNESS Worker; target is $TARGET_HARNESS/$TARGET_MODEL" >&2

worker=""
branch=""
deadline=$((SECONDS + TIMEOUT))
while [ "$SECONDS" -lt "$deadline" ]; do
  snapshot="$(probe_api_get "/api/runs/$runid")"
  pair="$(printf '%s' "$snapshot" | python3 -c '
import json,sys
r=json.load(sys.stdin)["run"]
t=next((x for x in r.get("tickets",[]) if x.get("id")=="01-switch-probe"),{})
print((t.get("workerSession","")+"\n"+t.get("branch","")))')"
  worker="$(printf '%s' "$pair" | sed -n '1p')"
  branch="$(printf '%s' "$pair" | sed -n '2p')"
  [ -n "$worker" ] && [ -n "$branch" ] && break
  sleep 2
done
[ -n "$worker" ] || { echo "FAIL - Worker did not start within ${TIMEOUT}s" >&2; tail -30 "$PROBE_LOG" >&2; exit 1; }

# Pause as soon as the real Worker has received its initial task; its specific
# response is not part of this probe, and Ticket remains incomplete by design.
old_session="$(probe_api_get "/api/sessions/$worker")"
old_backend="$(printf '%s' "$old_session" | python3 -c 'import json,sys; print(json.load(sys.stdin)["session"].get("harness",""))')"
[ "$old_backend" = "$SOURCE_HARNESS" ] || { echo "FAIL - initial Worker Backend is $old_backend, want $SOURCE_HARNESS" >&2; exit 1; }
pause="$(probe_api_post "/api/runs/$runid/tickets/01-switch-probe/pause" '{}')"
paused_state="$(printf '%s' "$pause" | python3 -c 'import json,sys; r=json.load(sys.stdin)["run"]; t=next(t for t in r["tickets"] if t["id"]=="01-switch-probe"); print(t["state"])')"
[ "$paused_state" = paused ] || { echo "FAIL - pause returned Ticket state $paused_state" >&2; exit 1; }

body="$(python3 -c 'import json,sys; print(json.dumps({"backend":sys.argv[1],"model":sys.argv[2]}))' "$TARGET_HARNESS" "$TARGET_MODEL")"
switched="$(probe_api_post "/api/runs/$runid/tickets/01-switch-probe/switch" "$body")"
switched_ticket="$(printf '%s' "$switched" | python3 -c 'import json,sys; r=json.load(sys.stdin)["run"]; print(json.dumps(next(t for t in r["tickets"] if t["id"]=="01-switch-probe")))')"
active="$(printf '%s' "$switched_ticket" | python3 -c 'import json,sys; print(json.load(sys.stdin).get("chain",{}).get("worker",{}).get("backend",""))')"
model="$(printf '%s' "$switched_ticket" | python3 -c 'import json,sys; print(json.load(sys.stdin).get("chain",{}).get("worker",{}).get("model",""))')"
[ "$active" = "$TARGET_HARNESS" ] && [ "$model" = "$TARGET_MODEL" ] || { echo "FAIL - active Worker Chain is $active/$model" >&2; exit 1; }
[ "$(printf '%s' "$switched_ticket" | python3 -c 'import json,sys; print(json.load(sys.stdin).get("workerSession",""))')" = "$worker" ] || { echo "FAIL - Switch started a Session instead of waiting for Resume" >&2; exit 1; }
[ "$(printf '%s' "$switched_ticket" | python3 -c 'import json,sys; print(json.load(sys.stdin).get("branch",""))')" = "$branch" ] || { echo "FAIL - Ticket branch changed during Switch" >&2; exit 1; }

resumed="$(probe_api_post "/api/runs/$runid/tickets/01-switch-probe/resume" '{}')"
new_worker="$(printf '%s' "$resumed" | python3 -c 'import json,sys; r=json.load(sys.stdin)["run"]; print(next(t for t in r["tickets"] if t["id"]=="01-switch-probe").get("workerSession",""))')"
[ -n "$new_worker" ] && [ "$new_worker" != "$worker" ] || { echo "FAIL - Resume did not replace Worker $worker" >&2; exit 1; }

new_session=""
handoff=0
for _ in $(seq 1 90); do
  new_session="$(probe_api_get "/api/sessions/$new_worker")"
  meta="$(printf '%s' "$new_session" | python3 -c 'import json,sys; print(json.dumps(json.load(sys.stdin)["session"]))')"
  got_harness="$(printf '%s' "$meta" | python3 -c 'import json,sys; print(json.load(sys.stdin).get("harness",""))')"
  got_model="$(printf '%s' "$meta" | python3 -c 'import json,sys; print(json.load(sys.stdin).get("model",""))')"
  [ "$got_harness" = "$TARGET_HARNESS" ] && [ "$got_model" = "$TARGET_MODEL" ] || { echo "FAIL - resumed Worker is $got_harness/$got_model" >&2; exit 1; }
  got_branch="$(printf '%s' "$resumed" | python3 -c 'import json,sys; r=json.load(sys.stdin)["run"]; print(next(t for t in r["tickets"] if t["id"]=="01-switch-probe").get("branch",""))')"
  [ "$got_branch" = "$branch" ] || { echo "FAIL - resumed Ticket branch changed to $got_branch" >&2; exit 1; }
  if printf '%s' "$new_session" | python3 -c 'import json,sys; sys.exit(0 if any(e.get("kind")=="prompt" and e.get("handoff") for e in json.load(sys.stdin).get("events",[])) else 1)'; then
    handoff=1
    break
  fi
  sleep 2
done
[ "$handoff" -eq 1 ] || { echo "FAIL - resumed Worker has no Handoff prompt" >&2; exit 1; }

probe_api_post "/api/runs/$runid/tickets/01-switch-probe/pause" '{}' >/dev/null
printf 'PASS - paused Ticket switched %s/%s -> %s/%s without auto-start, resumed on the same branch with a Handoff (Worker %s).\n' "$SOURCE_HARNESS" "$SOURCE_MODEL" "$TARGET_HARNESS" "$TARGET_MODEL" "$new_worker"
