#!/usr/bin/env bash
# Proves tell_worker reaches a live worker: the worker must abandon its ticket
# and do what it was told instead. Checked by looking for the marker commit on
# the worker's branch, not by trusting the merger's report.
set -uo pipefail
export PATH="$HOME/go/bin:$PATH"
WICK=/home/cioky/Projects/wick-simple/wick
REPORT=/home/cioky/Projects/wick-simple/herdr-report
DIR=/tmp/wick-tell

rm -rf "$DIR" ~/.pig/state/wick; mkdir -p "$DIR/.scratch/tell/issues"
printf 'alpha\nbeta\n' > "$DIR/app.txt"
cat > "$DIR/.scratch/tell/issues/01-tell.md" <<'TICKET'
Change line 1 of app.txt from alpha to GAMMA.
TICKET
git -C "$DIR" init -q
git -C "$DIR" -c user.email=w@x -c user.name=wick add -A >/dev/null
git -C "$DIR" -c user.email=w@x -c user.name=wick commit -qm base

cd "$DIR" && timeout 420 env WICK_HERDR_REPORT="$REPORT" \
  pig --no-skills -e "$WICK" -p '
Do these in order, no other tool calls:
1. spawn_worker with ticket="/tmp/wick-tell/.scratch/tell/issues/01-tell.md" and repo="/tmp/wick-tell".
2. wait_workers with timeout_ms=60000.
3. tell_worker with worker set to the id spawn_worker returned, and message:
   "Stop working on the ticket. Instead create a file named TELLMARKER.txt containing exactly the text marker-received and nothing else, commit it, then stop."
4. wait_workers with timeout_ms=180000.
Report each tool result verbatim.' 2>&1 | tail -18

echo "=== ground truth: does the branch contain the marker? ==="
if git -C "$DIR" log --all --oneline --name-only 2>/dev/null | grep -q TELLMARKER; then
  echo "PASS — the worker did what it was told:"
  git -C "$DIR" log --all --oneline --name-only | grep -B3 TELLMARKER | head -8
else
  echo "FAIL — no TELLMARKER on any branch:"
  git -C "$DIR" log --all --oneline | head -5
fi

for w in $(herdr workspace list 2>/dev/null | python3 -c "
import sys,json
for w in json.load(sys.stdin)['result']['workspaces']:
    if w['workspace_id']!='wY': print(w['workspace_id'])"); do herdr workspace close "$w" >/dev/null 2>&1; done
rm -rf "$DIR" ~/.pig/state/wick