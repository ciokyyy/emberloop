#!/usr/bin/env bash
# tell_worker can only reach a worker whose pig session is still alive. A
# worker run with `pig -p` exits the moment its turn ends, so its pane stops
# being an agent. This checks the narrower claim that it DOES reach a worker
# mid-turn.
set -uo pipefail
export PATH="$HOME/go/bin:$PATH"
WICK=/home/cioky/Projects/wick-simple/wick-pig
REPORT=/home/cioky/Projects/wick-simple/herdr-report
DIR=/tmp/wick-tell2

rm -rf "$DIR" ~/.pig/state/wick; mkdir -p "$DIR/.scratch/tell/issues"
printf 'alpha\nbeta\n' > "$DIR/app.txt"
cat > "$DIR/.scratch/tell/issues/01-slow.md" <<'TICKET'
Work slowly and carefully. Before changing anything, re-read app.txt five times
with a short pause between each. Then change line 1 from alpha to GAMMA.
TICKET
git -C "$DIR" init -q
git -C "$DIR" -c user.email=w@x -c user.name=wick add -A >/dev/null
git -C "$DIR" -c user.email=w@x -c user.name=wick commit -qm base

cd "$DIR" && timeout 600 env WICK_HERDR_REPORT="$REPORT" \
  pig --no-skills -e "$WICK" -p '
1. spawn_worker with ticket="/tmp/wick-tell2/.scratch/tell/issues/01-slow.md" and repo="/tmp/wick-tell2".
2. wait_workers with timeout_ms=25000.
3. tell_worker with worker set to the id spawn_worker returned, and message:
   "Stop the current task. Create a file named TELLMARKER.txt containing exactly marker-received, commit it, then stop."
4. wait_workers with timeout_ms=240000.
Report each tool result verbatim.' 2>&1 | grep -E "spawned worker|delivered|could not|^\[|01-slow|all settled|TELLMARKER|not_ready" | head -12

echo "=== ground truth ==="
if git -C "$DIR" log --all --oneline --name-only 2>/dev/null | grep -q TELLMARKER; then
  echo "PASS — tell_worker reached a mid-turn worker"
  git -C "$DIR" log --all --oneline --name-only | grep -B3 TELLMARKER | head -8
else
  echo "FAIL — marker absent"
  git -C "$DIR" log --all --oneline | head -5
fi

for w in $(herdr workspace list 2>/dev/null | python3 -c "
import sys,json
for w in json.load(sys.stdin)['result']['workspaces']:
    if w['workspace_id']!='wY': print(w['workspace_id'])"); do herdr workspace close "$w" >/dev/null 2>&1; done
rm -rf "$DIR" ~/.pig/state/wick