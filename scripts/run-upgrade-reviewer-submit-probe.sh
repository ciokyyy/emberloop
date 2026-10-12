#!/usr/bin/env bash
# Live probe for Run-upgrade ticket 21: a real Run Reviewer reports through
# the typed submit tool, and the Daemon records a fresh verdict at the Ticket tip.
#
# The probe builds el-web and an isolated fixture repo, then starts a Run with
# a fake Worker and one real Pig spec Reviewer. The fixture commits the one-line
# implementation in the Worker worktree and posts its done submit through the
# API, so the only model turn is the real Reviewer. It inspects the Reviewer's
# Session Event log for a typed `submit` payload and matching fresh verdict.
#
# Verdict: PASS when a fresh verdict matches a typed submit Event with a full
# tip and valid verdict payload (a pass also carries `ran`). Manual only; it
# may use real Reviewer turns and is never part of `.emberloop/check`.
#
# Prerequisites: go, git, curl, python3 and a real Pig login/model that supports
# tool calls. Set `PIG_LIVE_MODEL` or `EL_RUN_UPGRADE_REVIEWER_MODEL`. Usage:
#   PIG_LIVE_MODEL=provider/model scripts/run-upgrade-reviewer-submit-probe.sh
set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/run-upgrade-probe-lib.sh"

PIG_MODEL="${EL_RUN_UPGRADE_REVIEWER_MODEL:-${PIG_LIVE_MODEL:-}}"
TIMEOUT="${EL_RUN_UPGRADE_TIMEOUT:-900}"
[ -n "$PIG_MODEL" ] || { echo "FAIL - set PIG_LIVE_MODEL or EL_RUN_UPGRADE_REVIEWER_MODEL to a tool-capable Pig model" >&2; exit 1; }
export EL_PIG_SDK=1
probe_init reviewer-submit
repo="$PROBE_DIR/repo"
probe_git_init "$repo"
probe_add_green_check "$repo"
mkdir -p "$repo/.scratch/reviewer-probe/issues"
cat >"$repo/AGENTS.md" <<'AGENTS'
# AGENTS.md — Reviewer submit probe

## Purpose

A disposable fixture for one live Run Reviewer probe.

## Child DOX Index

No child docs.
AGENTS
CHECKBOX='Create `probe.txt` in the repository root containing exactly `reviewer-submit-live-probe` on one line.'
cat >"$repo/.scratch/reviewer-probe/issues/01-reviewer-submit.md" <<TICKET
# 01: Reviewer submit probe

**Blocked by:** None

**Status:** open

- [ ] $CHECKBOX
TICKET
cat >"$repo/.scratch/reviewer-probe/spec.md" <<'SPEC'
# Reviewer submit live probe

The Ticket's repository root is the Worker's current branch worktree, not the
original fixture checkout under /tmp. The required `probe.txt` is present at
the current branch root; do not judge against the unmodified fixture checkout.
This probe tests the Reviewer's typed `submit` tool: after inspecting the change,
you MUST call `submit` with verdict `pass`, the current full tip SHA, no findings,
and the command you ran with its exit code. Do not answer with a free-text verdict.
Run `git status --porcelain && git rev-parse HEAD` before submitting.
SPEC
probe_git_commit "$repo" 'reviewer submit probe fixture'

chains="$(python3 -c 'import json,sys; print(json.dumps({"chains":{"reviewer-spec":[{"backend":"pig","model":sys.argv[1]}]}}))' "$PIG_MODEL")"
probe_api_put /api/settings "$chains" >/dev/null
spec="$repo/.scratch/reviewer-probe/spec.md"
echo "starting Run with real Pig spec Reviewer: model=$PIG_MODEL timeout=${TIMEOUT}s" >&2
run="$(probe_start_run "$spec" "$repo" fake fake-model)"
runid="$(printf '%s' "$run" | python3 -c 'import json,sys; print(json.load(sys.stdin)["run"]["id"])')"
probe_track_run "$runid"
echo "run=$runid" >&2

worker=""
deadline=$((SECONDS + 180))
while [ "$SECONDS" -lt "$deadline" ]; do
  snapshot="$(probe_api_get "/api/runs/$runid")"
  worker="$(printf '%s' "$snapshot" | python3 -c 'import json,sys; r=json.load(sys.stdin)["run"]; t=next((t for t in r.get("tickets",[]) if t.get("id")=="01-reviewer-submit"),{}); print(t.get("workerSession",""))')"
  [ -n "$worker" ] && break
  sleep 1
done
[ -n "$worker" ] || { echo "FAIL - fake fixture Worker did not start" >&2; exit 1; }
session="$(probe_api_get "/api/sessions/$worker")"
worktree="$(printf '%s' "$session" | python3 -c 'import json,sys; print(json.load(sys.stdin)["session"]["workDir"])')"
ticket="$worktree/.scratch/reviewer-probe/issues/01-reviewer-submit.md"
printf 'reviewer-submit-live-probe\n' >"$worktree/probe.txt"
sed -i 's/^- \[ \]/- [x]/' "$ticket"
git -C "$worktree" add probe.txt .scratch/reviewer-probe/issues/01-reviewer-submit.md
git -C "$worktree" commit -qm 'prepare reviewer submit probe result'
submit="$(python3 -c 'import json,sys; print(json.dumps({"status":"done","completed":[sys.argv[1]],"report":"Created the requested file and committed the completed Ticket."}))' "$CHECKBOX")"
probe_api_post "/api/sessions/$worker/submit" "$submit" >/dev/null

result=""
deadline=$((SECONDS + TIMEOUT))
while [ "$SECONDS" -lt "$deadline" ]; do
  snapshot="$(probe_api_get "/api/runs/$runid")"
  result="$(printf '%s' "$snapshot" | python3 -c '
import json,sys
r=json.load(sys.stdin)["run"]
t=next((x for x in r.get("tickets",[]) if x.get("id")=="01-reviewer-submit"),{})
print(json.dumps({"status":r.get("status"),"ticket":t}))')"
  ready="$(printf '%s' "$result" | python3 -c '
import json,sys
t=json.load(sys.stdin)["ticket"]
for role,sid in (t.get("reviewerSessions") or {}).items():
    if sid: print(role+"="+sid)
')"
  found=0
  while IFS='=' read -r role sid; do
    [ -n "${sid:-}" ] || continue
    session="$(probe_api_get "/api/sessions/$sid")"
    typed="$(printf '%s' "$session" | python3 -c '
import json,sys
s=json.load(sys.stdin); chosen=None
for e in s.get("events",[]):
    if e.get("kind")!="submit": continue
    try: p=json.loads(e.get("text",""))
    except Exception: continue
    if p.get("verdict") not in ("pass","changes") or len(p.get("tip",""))<40: continue
    if p.get("verdict")=="pass" and not p.get("ran"): continue
    if p.get("verdict")=="changes" and not any(f.get("severity") in ("blocker","major") for f in p.get("findings",[])): continue
    chosen=p
if chosen: print(json.dumps(chosen))
')"
    if [ -n "$typed" ]; then
      tip="$(printf '%s' "$typed" | python3 -c 'import json,sys; print(json.load(sys.stdin)["tip"])')"
      verdict="$(printf '%s' "$typed" | python3 -c 'import json,sys; print(json.load(sys.stdin)["verdict"])')"
      fresh="$(printf '%s' "$result" | ROLE="$role" TIP="$tip" VERDICT="$verdict" python3 -c '
import json,os,sys
r=json.load(sys.stdin); t=r["ticket"]
v=(t.get("verdicts") or {}).get(os.environ["ROLE"],{})
same=v.get("at")==os.environ["TIP"] and v.get("verdict")==os.environ["VERDICT"]
current=bool(v.get("fresh"))
merged=t.get("state")=="merged" and r.get("status")=="done"
print("yes" if same and (current or merged) else "no")')"
      if [ "$fresh" = yes ]; then
        printf '%s' "$typed" | ROLE="$role" python3 -c 'import json,os,sys; p=json.load(sys.stdin); print("PASS - real reviewer submit recorded: role=%s verdict=%s tip=%s ran=%d"%(os.environ["ROLE"],p["verdict"],p["tip"],len(p.get("ran",[]))))'
        found=1
        break
      fi
    fi
  done <<<"$ready"
  [ "$found" -eq 1 ] && break
  case "$(printf '%s' "$result" | python3 -c 'import json,sys; print(json.load(sys.stdin)["status"])')" in
    failed|abandoned)
      echo "FAIL - Run ended before a fresh typed Reviewer verdict: $result" >&2
      tail -30 "$PROBE_LOG" >&2
      exit 1
      ;;
  esac
  sleep 3
done
if [ "$SECONDS" -ge "$deadline" ]; then
  echo "FAIL - timed out waiting for a fresh Reviewer submit Event: $result" >&2
  tail -30 "$PROBE_LOG" >&2
  exit 1
fi
