#!/usr/bin/env bash
# Live probe for Run-upgrade ticket 21: a real Claude authentication failure
# advances a Run Worker to the configured Pig Chain entry on the same branch.
#
# The probe runs an isolated el-web with an intentionally invalid Anthropic API
# key. The temporary Daemon and its agent subprocesses inherit it; a one-ticket
# Run uses a Claude -> Pig Worker Chain.
# It verifies the original Claude Session's provider auth error, the active Pig
# Chain entry, the preserved branch and a Handoff prompt, then pauses the Ticket.
#
# Verdict: PASS only when the real provider rejects the bad key and the Run
# starts the real Pig fallback with a Handoff. Never use a production daemon or
# credentials for the primary entry: the bad key is scoped to this process.
# This is manual, costs a Claude auth attempt and a Pig turn, and is never part
# of `.emberloop/check`.
#
# Prerequisites: go, git, curl, python3, node/npm for Claude ACP, a real Pig
# login and healthy model. Set PIG_LIVE_MODEL or
# EL_RUN_UPGRADE_CHAIN_PIG_MODEL. Usage:
#   scripts/run-upgrade-chain-probe.sh
#   EL_RUN_UPGRADE_CHAIN_PIG_MODEL=provider/model scripts/run-upgrade-chain-probe.sh
set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/run-upgrade-probe-lib.sh"

probe_require node npx || exit 1
CLAUDE_MODEL="${EL_RUN_UPGRADE_CHAIN_CLAUDE_MODEL:-haiku}"
PIG_MODEL="${EL_RUN_UPGRADE_CHAIN_PIG_MODEL:-${PIG_LIVE_MODEL:-}}"
TIMEOUT="${EL_RUN_UPGRADE_TIMEOUT:-900}"
[ -n "$PIG_MODEL" ] || { echo "FAIL - set PIG_LIVE_MODEL or EL_RUN_UPGRADE_CHAIN_PIG_MODEL to a healthy Pig model" >&2; exit 1; }
case "$CLAUDE_MODEL" in opus|sonnet|haiku) ;; *) echo "FAIL - Claude model must be opus, sonnet or haiku" >&2; exit 1 ;; esac

# Claude Code gives ANTHROPIC_API_KEY precedence over the owner's login. This
# deliberate invalid value exercises a real provider auth error without
# changing the host's credentials or config files.
export ANTHROPIC_API_KEY=sk-ant-run-upgrade-probe-invalid
probe_init chain
repo="$PROBE_DIR/repo"
probe_git_init "$repo"
probe_add_green_check "$repo"
mkdir -p "$repo/.scratch/chain-probe/issues"
cat >"$repo/AGENTS.md" <<'AGENTS'
# AGENTS.md — Chain probe

## Purpose

A disposable fixture for a live provider Chain probe.

## Child DOX Index

No child docs.
AGENTS
cat >"$repo/.scratch/chain-probe/spec.md" <<'SPEC'
# Chain live probe

Wait for the probe operator after resuming from the fallback. Do not edit or
submit; the probe only checks the Chain handoff.
SPEC
cat >"$repo/.scratch/chain-probe/issues/01-chain-probe.md" <<'TICKET'
# 01: Chain probe

**Blocked by:** None

**Status:** open

- [ ] Create `probe.txt` containing `chain-live-probe` after the probe operator resumes the test.
TICKET
probe_git_commit "$repo" 'Chain probe fixture'
chains="$(python3 -c 'import json,sys; print(json.dumps({"chains":{"worker":[{"backend":"claude","model":sys.argv[1]},{"backend":"pig","model":sys.argv[2]}]}}))' "$CLAUDE_MODEL" "$PIG_MODEL")"
probe_api_put /api/settings "$chains" >/dev/null

spec="$repo/.scratch/chain-probe/spec.md"
echo "starting real Chain probe: claude/$CLAUDE_MODEL -> pig/$PIG_MODEL" >&2
run="$(probe_start_run "$spec" "$repo" claude "$CLAUDE_MODEL")"
runid="$(printf '%s' "$run" | python3 -c 'import json,sys; print(json.load(sys.stdin)["run"]["id"])')"
probe_track_run "$runid"
echo "run=$runid" >&2

old_worker=""
new_worker=""
branch=""
error=""
deadline=$((SECONDS + TIMEOUT))
while [ "$SECONDS" -lt "$deadline" ]; do
  snapshot="$(probe_api_get "/api/runs/$runid")"
  state="$(printf '%s' "$snapshot" | python3 -c '
import json,sys
r=json.load(sys.stdin)["run"]
t=next((x for x in r.get("tickets",[]) if x.get("id")=="01-chain-probe"),{})
print(json.dumps({"status":r.get("status"),"ticket":t}))')"
  active_worker="$(printf '%s' "$state" | python3 -c 'import json,sys; print(json.load(sys.stdin)["ticket"].get("workerSession", ""))')"
  branch="$(printf '%s' "$state" | python3 -c 'import json,sys; print(json.load(sys.stdin)["ticket"].get("branch", ""))')"
  position="$(printf '%s' "$state" | python3 -c 'import json,sys; print((json.load(sys.stdin)["ticket"].get("chain",{}).get("worker",{})).get("position",0))')"
  backend="$(printf '%s' "$state" | python3 -c 'import json,sys; print((json.load(sys.stdin)["ticket"].get("chain",{}).get("worker",{})).get("backend", ""))')"
  if [ "$position" = 2 ] && [ "$backend" = pig ] && [ -n "$active_worker" ]; then
    new_worker="$active_worker"
    sessions="$(probe_api_get /api/sessions)"
    old_worker="$(printf '%s' "$sessions" | RUN_ID="$runid" ACTIVE_WORKER="$new_worker" python3 -c '
import json,os,sys
for s in json.load(sys.stdin):
    if s.get("parentId")==os.environ["RUN_ID"] and s.get("runWorker") and s.get("harness")=="claude" and s.get("status")=="failed" and s.get("id")!=os.environ["ACTIVE_WORKER"]:
        print(s["id"]); break')"
    if [ -n "$old_worker" ]; then
      old_session="$(probe_api_get "/api/sessions/$old_worker")"
      error="$(printf '%s' "$old_session" | python3 -c '
import json,re,sys
s=json.load(sys.stdin)
for e in s.get("events",[]):
    if e.get("kind")=="error" and re.search(r"api key|unauthori[sz]ed|authentication|401|credential",e.get("text",""),re.I):
        print(e.get("text","")); break')"
      [ -n "$error" ] && break
    fi
  fi
  sleep 2
done

if [ -z "$old_worker" ] || [ -z "$new_worker" ] || [ -z "$error" ]; then
  echo "FAIL - no real Claude auth failure followed by a Pig Chain advance within ${TIMEOUT}s" >&2
  echo "last run snapshot: ${state:-none}" >&2
  tail -40 "$PROBE_LOG" >&2
  exit 1
fi

new_session="$(probe_api_get "/api/sessions/$new_worker")"
new_meta="$(printf '%s' "$new_session" | python3 -c 'import json,sys; print(json.dumps(json.load(sys.stdin)["session"]))')"
new_harness="$(printf '%s' "$new_meta" | python3 -c 'import json,sys; print(json.load(sys.stdin).get("harness",""))')"
new_model="$(printf '%s' "$new_meta" | python3 -c 'import json,sys; print(json.load(sys.stdin).get("model",""))')"
[ "$new_harness" = pig ] && [ "$new_model" = "$PIG_MODEL" ] || { echo "FAIL - fallback Session is $new_harness/$new_model, want pig/$PIG_MODEL" >&2; exit 1; }

handoff=0
for _ in $(seq 1 45); do
  new_session="$(probe_api_get "/api/sessions/$new_worker")"
  if printf '%s' "$new_session" | python3 -c 'import json,sys; sys.exit(0 if any(e.get("kind")=="prompt" and e.get("handoff") for e in json.load(sys.stdin).get("events",[])) else 1)'; then
    handoff=1
    break
  fi
  sleep 2
done
[ "$handoff" -eq 1 ] || { echo "FAIL - Pig fallback has no Handoff prompt" >&2; exit 1; }

current="$(probe_api_get "/api/runs/$runid")"
current_branch="$(printf '%s' "$current" | python3 -c 'import json,sys; t=next(t for t in json.load(sys.stdin)["run"]["tickets"] if t["id"]=="01-chain-probe"); print(t.get("branch",""))')"
[ "$current_branch" = "$branch" ] || { echo "FAIL - Ticket branch changed: $branch -> $current_branch" >&2; exit 1; }
pause="$(probe_api_post "/api/runs/$runid/tickets/01-chain-probe/pause" '{}')"
paused="$(printf '%s' "$pause" | python3 -c 'import json,sys; r=json.load(sys.stdin)["run"]; t=next(t for t in r["tickets"] if t["id"]=="01-chain-probe"); print(t["state"])')"
[ "$paused" = paused ] || { echo "FAIL - Ticket state after pause is $paused" >&2; exit 1; }
printf 'PASS - Claude auth failure advanced to Pig on branch %s; replacement Session %s received a Handoff and the Ticket is paused.\n' "$branch" "$new_worker"
