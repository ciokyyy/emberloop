#!/usr/bin/env bash
# Probe: a claude reviewer spawned into an unfocused background tab gets its
# first prompt within seconds, and a reviewer pane bumped to another tab or
# into an overflow tab still receives its prompt.
#
# What it covers, step by step:
#  1. Background tab: a pig worker in a never-focused workspace spawns its
#     spec reviewer on claude. The probe times pane launch, readiness
#     (`herdr agent wait --until idle --until done`, the same wait the spawn
#     path uses), and the first working state after the prompt is delivered.
#     A background pane is not focused, so herdr reports its idle as done
#     until someone looks (`herdr agent wait --help`: one --until per state,
#     "repeat for more than one state"). Against the old idle-only wait the
#     spawn path timed out here, the reviewer pane died, and this step
#     FAILed with no working reviewer; it passes with the idle-or-done wait.
#  2. Bumped pane: the reviewer pane moves to a fresh tab (`herdr pane move
#     --new-tab`) and a follow-up prompt must still land (the agent works
#     again). Delivery is pane-addressed, so the tab must not matter.
#  3. Overflow tab: the ticket tab is filled to nine panes (the grid holds
#     gridMax 3x3) with bare shells, the worker spawns its standards
#     reviewer after its sleep, and the tenth pane must land in the
#     `<ticket> (2)` overflow tab and still get its prompt within seconds.
#  A fresh repo dir means claude's folder-trust dialog would stall the pane
#  if the spawn path did not pre-accept it (harness Trust); a stall here
#  FAILs step 1 at the readiness wait and points at the dialog.
#
# Prerequisites: herdr server running, `pig`, `claude` and `python3` on PATH
# with a claude login, run from inside an el checkout (the pig session
# loads this checkout's el extension). Costs two claude reviewer
# starts and leaves both unanswered: EXIT cleanup closes the whole workspace
# with their panes, nothing merges.
#
# Never runs in the automated check and never inside ticket work: run by
# hand, then read the PASS/FAIL lines.
set -uo pipefail
export PATH="$HOME/go/bin:$PATH"

REPO=$(git rev-parse --show-toplevel 2>/dev/null || true)
EL="$REPO/el"
REPORT="$REPO/.scratch/archive/archived-herdr-report"
for bin in herdr pig claude; do
  command -v "$bin" >/dev/null || { echo "FAIL - need $bin on PATH"; exit 1; }
done
[ -d "$EL" ] || { echo "FAIL - run from inside an el checkout"; exit 1; }

DIR=$(mktemp -d /tmp/el-reviewer-probe.XXXXXX)
mkdir -p "$DIR/.scratch/probe/issues"
printf 'alpha\nbeta\n' > "$DIR/app.txt"
git -C "$DIR" init -qb master
git -C "$DIR" -c user.email=w@x -c user.name=el add -A >/dev/null
git -C "$DIR" -c user.email=w@x -c user.name=el commit -qm base
printf 'gamma\n' >> "$DIR/app.txt"
cat > "$DIR/.scratch/probe/issues/01-probe.md" <<'TICKET'
Do these in order, no other tool calls:
1. Spawn your reviewer-spec reviewer with no extra note.
2. Run bash with command "sleep 420", so the probe has time to fill this
   tab before the next reviewer comes.
3. Spawn your reviewer-standards reviewer with no extra note, then submit done.
TICKET
WS=$(herdr workspace create --cwd "$DIR" --label reviewer-probe --no-focus 2>&1)
WID=$(printf '%s' "$WS" | python3 -c "import sys,json;print(json.load(sys.stdin)['result']['workspace']['workspace_id'])")
PANE=$(printf '%s' "$WS" | python3 -c "import sys,json;print(json.load(sys.stdin)['result']['root_pane']['pane_id'])")
echo "workspace: $WID  driver pane: $PANE (never focused)"
DRIVER=""
cleanup() {
  kill "$DRIVER" >/dev/null 2>&1 || true
  herdr workspace close "$WID" >/dev/null 2>&1 || true
  rm -rf "$DIR"
}
trap cleanup EXIT

panes_in() {
  herdr pane list --workspace "$WID" 2>/dev/null | python3 -c "
import sys,json
try: panes=json.load(sys.stdin)['result']['panes']
except Exception: raise SystemExit
for p in panes:
    print('%s|%s|%s|%s'%(p.get('pane_id'),p.get('tab_id'),p.get('label'),p.get('agent_status')))"
}

# The worker inherits this process's env through every pane launch, so its
# own reviewer spawns resolve to claude too.
export EL_READONLY_HARNESS=claude
export EL_HERDR_REPORT="$REPORT"

T_SPAWN=$(date +%s)
DRIVER_PROMPT=$(cat <<EOF
Do these in order, no other tool calls:
1. spawn with agents=[{"type": "worker", "ticket": "$DIR/.scratch/probe/issues/01-probe.md", "repo": "$DIR"}].
2. bash with command "sleep 420", so this session stays alive while the probe watches.
A spawn returns at once: the worker result arrives in this pane as a system notice, so end your turn and report that notice when it lands.
EOF
)
timeout 600 pig --no-skills -e "$EL" -p "$DRIVER_PROMPT" >/tmp/el-reviewer-probe-driver.$$.log 2>&1 &
DRIVER=$!

echo "== 1. reviewer in a background tab =="
REVIEWER=""
for i in $(seq 1 120); do
  REVIEWER=$(panes_in | awk -F'|' '$3=="spec-reviewer" {print $1; exit}')
  [ -n "$REVIEWER" ] && break
  sleep 2
done
[ -n "$REVIEWER" ] || { echo "FAIL - no spec-reviewer pane in 240s (old idle-only wait kills the launch)"; exit 1; }
T_PANE=$(date +%s)
echo "  pane $REVIEWER launched $((T_PANE - T_SPAWN))s after spawn"
if herdr agent wait "$REVIEWER" --until idle --until done --timeout 90000 >/dev/null 2>&1; then
  T_READY=$(date +%s)
  echo "  ready (idle or done) $((T_READY - T_PANE))s after launch"
else
  echo "FAIL - reviewer pane never became ready in 90s (stalled: first-run dialog?)"
  exit 1
fi
if herdr agent wait "$REVIEWER" --until working --timeout 120000 >/dev/null 2>&1; then
  T_WORK=$(date +%s)
  echo "  working (prompt delivered) $((T_WORK - T_READY))s after ready"
  echo "PASS - background-tab prompt delivered in $((T_WORK - T_SPAWN))s spawn to working"
else
  echo "FAIL - prompt never delivered: reviewer idle 120s after ready"
  exit 1
fi
echo "== 2. reviewer bumped to another tab =="
herdr pane move "$REVIEWER" --new-tab >/dev/null 2>&1 || { echo "FAIL - pane move refused"; exit 1; }
# Plain prompt plus a separate wait: `herdr agent prompt --help` says --wait
# needs working within 5000ms of submission (agent_prompt_stalled), too
# tight for a cold claude start.
herdr agent prompt "$REVIEWER" "probe ping: acknowledge with one word" >/dev/null 2>&1 || { echo "FAIL - bumped pane refused the prompt"; exit 1; }
if herdr agent wait "$REVIEWER" --until working --timeout 120000 >/dev/null 2>&1; then
  echo "PASS - bumped pane still receives its prompt"
else
  echo "FAIL - bumped pane took no prompt within 120s"
  exit 1
fi

echo "== 3. reviewer in the overflow tab =="
WORKER=$(panes_in | awk -F'|' '$3=="implementer" {print $1; exit}')
[ -n "$WORKER" ] || { echo "FAIL - worker pane gone, cannot fill its tab"; exit 1; }
WTAB=$(panes_in | awk -F'|' -v w="$WORKER" '$1==w {print $2; exit}')
count_in_tab() { panes_in | awk -F'|' -v t="$WTAB" '$2==t' | wc -l; }
for _ in $(seq 1 12); do
  [ "$(count_in_tab)" -ge 9 ] && break
  herdr pane split "$WORKER" >/dev/null 2>&1
  sleep 1
done
echo "  ticket tab holds $(count_in_tab) panes; the worker spawns its standards reviewer after its sleep"
OVERFLOW=""
for i in $(seq 1 200); do
  OVERFLOW=$(panes_in | awk -F'|' '$3=="code-reviewer" {print $1"|"$2; exit}')
  [ -n "$OVERFLOW" ] && break
  sleep 2
done
[ -n "$OVERFLOW" ] || { echo "FAIL - no code-reviewer pane in 400s"; exit 1; }
OPANE=${OVERFLOW%%|*}; OTAB=${OVERFLOW##*|}
echo "  code reviewer in tab $OTAB (ticket tab is $WTAB)"
[ "$OTAB" != "$WTAB" ] || { echo "FAIL - reviewer stayed in the full tab, no overflow"; exit 1; }
if herdr agent wait "$OPANE" --until idle --until done --timeout 90000 >/dev/null 2>&1 \
  && herdr agent wait "$OPANE" --until working --timeout 120000 >/dev/null 2>&1; then
  echo "PASS - overflow-tab prompt delivered"
else
  echo "FAIL - overflow reviewer never worked within the waits"
  exit 1
fi

echo "ALL PASS"
