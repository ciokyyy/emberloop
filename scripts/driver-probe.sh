#!/usr/bin/env bash
# Probe: the deterministic driver merges a throwaway two-ticket spec through
# a real herdr, survives a mid-run kill, and resumes without a second worker.
#
# What it covers, step by step:
#  1. Throwaway repo: a temp git repo with a spec and two trivial tickets
#     (each creates one file, so the merges never conflict) is driven with
#     real workers, reviewers, tells and finishes on a real herdr.
#  2. Mid-run kill: once both workers have started, the driver is killed.
#     The workers keep running in their panes; the driver holds no private
#     state file.
#  3. Resume: a second start picks up the running workers, starts no second
#     worker for either ticket, and merges both. The probe prints each
#     ticket's state (its Status line and its report line) at the end.
# This is the proof step before the default flips from --llm to --driver: it
# would have caught the idle/done wait bug, which no unit test could, because
# the reviewers spawn into real (unfocused) panes here.
#
# Prerequisites: herdr server running, a working pig or claude login (workers
# and reviewers are real agents), `go`, `herdr`, `timeout`, `python3` and `git`
# on PATH, run from the worktree root (the directory holding `emberloop/`, so
# the driver builds from this checkout). Costs two
# worker starts plus their reviewers and leaves nothing behind: EXIT cleanup
# kills a stray driver, closes the workspace, and removes the temp repo.
#
# Never runs in the automated check and never inside ticket work: run by
# hand, then read the PASS/FAIL lines. Logs stay in /tmp for inspection.
set -uo pipefail
export PATH="$HOME/go/bin:$PATH"

REPO=$(git rev-parse --show-toplevel 2>/dev/null || true)
[ -d "$REPO/emberloop" ] || { echo "FAIL - run from the worktree root"; exit 1; }
for bin in herdr go timeout python3 git; do
  command -v "$bin" >/dev/null || { echo "FAIL - need $bin on PATH"; exit 1; }
done

DIR=$(mktemp -d /tmp/emberloop-driver-probe.XXXXXX)
BIN="$DIR/driver-bin/emberloop"
LOG1="/tmp/emberloop-driver-probe-first.$$.log"
LOG2="/tmp/emberloop-driver-probe-second.$$.log"
BRANCHES_AT_KILL="/tmp/emberloop-driver-probe-branches.$$.txt"
SPEC="$DIR/.scratch/probe"
ISSUES="$SPEC/issues"
WID=""
DRIVER=""

cleanup() {
  if [ -n "$DRIVER" ] && kill -0 "$DRIVER" >/dev/null 2>&1; then
    kill "$DRIVER" >/dev/null 2>&1 || true
    sleep 2
    kill -9 "$DRIVER" >/dev/null 2>&1 || true
  fi
  [ -n "$WID" ] && herdr workspace close "$WID" >/dev/null 2>&1 || true
  rm -rf "$DIR"
}
trap cleanup EXIT

echo "== build the driver from this checkout =="
mkdir -p "$DIR/driver-bin"
(cd "$REPO/emberloop" && go build -o "$BIN" ./cmd/emberloop) || { echo "FAIL - driver build failed"; exit 1; }

echo "== throwaway repo and two-ticket spec =="
mkdir -p "$ISSUES"
git -C "$DIR" init -qb master
git -C "$DIR" config user.email "probe@emberloop"
git -C "$DIR" config user.name "emberloop probe"
git -C "$DIR" commit -q --allow-empty -m base
cat > "$SPEC/spec.md" <<'SPEC'
# Probe spec

A two-ticket spec for the driver probe. Each ticket creates one file, so the
merges never conflict.

- `one.txt` holds exactly `one`.
- `two.txt` holds exactly `two`.
SPEC
cat > "$ISSUES/01-probe-one.md" <<'TICKET'
# 01: Probe worker one

**Status:** ready

**Blocked by:** None

Create a file named one.txt in the repo root containing exactly the text `one` and nothing else.
TICKET
cat > "$ISSUES/02-probe-two.md" <<'TICKET'
# 02: Probe worker two

**Status:** ready

**Blocked by:** None

Create a file named two.txt in the repo root containing exactly the text `two` and nothing else.
TICKET
git -C "$DIR" add -A >/dev/null
git -C "$DIR" commit -qm base

# The driver refuses implement outside a herdr pane (its workers' results
# arrive as notices to the parent pane), so the probe lends it the root pane
# of a throwaway workspace and runs the binary itself: that keeps the kill
# and the restart under the probe's control while every worker, reviewer,
# tell and finish still goes through the real herdr.
WS=$(herdr workspace create --cwd "$DIR" --label driver-probe --no-focus) || { echo "FAIL - workspace create refused"; exit 1; }
WID=$(printf '%s' "$WS" | python3 -c "import sys,json;print(json.load(sys.stdin)['result']['workspace']['workspace_id'])") || { echo "FAIL - workspace create returned no workspace id"; exit 1; }
PANE=$(printf '%s' "$WS" | python3 -c "import sys,json;print(json.load(sys.stdin)['result']['root_pane']['pane_id'])") || { echo "FAIL - workspace create returned no root pane"; exit 1; }
echo "workspace: $WID  driver pane: $PANE (driver runs here as a subprocess)"
export HERDR_PANE_ID="$PANE"

echo "== first start, killed mid-run =="
(cd "$DIR" && exec env HERDR_PANE_ID="$PANE" "$BIN" implement --driver "$SPEC" >"$LOG1" 2>&1) &
DRIVER=$!
STARTED=""
for _ in $(seq 1 300); do
  STARTED=$(git -C "$DIR" branch --list 'implement/*' 2>/dev/null | wc -l)
  [ "${STARTED:-0}" -ge 2 ] && break
  kill -0 "$DRIVER" >/dev/null 2>&1 || break
  sleep 2
done
echo "  implement branches at kill time: ${STARTED:-0}"
[ "${STARTED:-0}" -ge 2 ] || { echo "FAIL - second worker never started; log: $LOG1"; tail -5 "$LOG1"; exit 1; }
sleep 30
kill "$DRIVER" >/dev/null 2>&1 || true
for _ in $(seq 1 15); do
  kill -0 "$DRIVER" >/dev/null 2>&1 || break
  sleep 1
done
kill -9 "$DRIVER" >/dev/null 2>&1 || true
if kill -0 "$DRIVER" >/dev/null 2>&1; then
  echo "FAIL - driver survived SIGKILL"
  exit 1
fi
DRIVER=""
git -C "$DIR" branch --list 'implement/*' 2>/dev/null | sort > "$BRANCHES_AT_KILL"
echo "PASS - driver killed mid-run with $(wc -l < "$BRANCHES_AT_KILL") worker branch(es) up"

echo "== second start resumes =="
if ! (cd "$DIR" && HERDR_PANE_ID="$PANE" timeout 2400 "$BIN" implement --driver "$SPEC" >"$LOG2" 2>&1); then
  echo "FAIL - second start exited nonzero; log: $LOG2"
  tail -10 "$LOG2"
  exit 1
fi
echo "PASS - second start exited 0"

echo "== ticket states =="
fail=0
for id in 01-probe-one 02-probe-two; do
  status=$(grep -im1 '^ *\**Status:\** *' "$ISSUES/$id.md" | sed 's/.*Status:\** *//I; s/[* ]//g' || true)
  line=$(grep -h "^merged $id\|^unmerged $id" "$LOG1" "$LOG2" | tail -1 || true)
  echo "  $id | Status: ${status:-?} | ${line:-no report line}"
  case "$status" in
    [Dd][Oo][Nn][Ee]) ;;
    *) echo "FAIL - $id is not done"; fail=1 ;;
  esac
  case "$line" in
    merged*) ;;
    *) echo "FAIL - $id has no merged report line"; fail=1 ;;
  esac
done
if grep -q "could not start" "$LOG2"; then
  echo "FAIL - second start tried to start a worker twice"
  fail=1
fi
nbranches=$(git -C "$DIR" branch --list 'implement/*' 2>/dev/null | wc -l)
if [ "$nbranches" -gt 2 ]; then
  echo "FAIL - $nbranches implement branches, more workers than tickets"
  fail=1
fi
[ "$fail" -ne 0 ] && exit 1
echo "PASS - both tickets merged, no second worker started"
echo "  first-run log:  $LOG1"
echo "  second-run log: $LOG2"
echo "ALL PASS"
