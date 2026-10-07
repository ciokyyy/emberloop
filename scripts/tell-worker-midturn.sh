#!/usr/bin/env bash
# tell can only reach a worker whose pig session is still alive. A worker run
# with `pig -p` exits the moment its turn ends, so its pane stops being an
# agent. This checks the narrower claim that it DOES reach a worker mid-turn.
set -uo pipefail
export PATH="$HOME/go/bin:$PATH"
EMBERLOOP=/home/cioky/Projects/emberloop/emberloop
REPORT=/home/cioky/Projects/emberloop/herdr-report
DIR=/tmp/emberloop-tell2

rm -rf "$DIR" ~/.pig/state/emberloop; mkdir -p "$DIR/.scratch/tell/issues"
printf 'alpha\nbeta\n' > "$DIR/app.txt"
cat > "$DIR/.scratch/tell/issues/01-slow.md" <<'TICKET'
Work slowly and carefully. Before changing anything, re-read app.txt five times
with a short pause between each. Then change line 1 from alpha to GAMMA.
TICKET
git -C "$DIR" init -q
git -C "$DIR" -c user.email=w@x -c user.name=emberloop add -A >/dev/null
git -C "$DIR" -c user.email=w@x -c user.name=emberloop commit -qm base

cd "$DIR" && timeout 600 env EMBERLOOP_HERDR_REPORT="$REPORT" \
  pig --no-skills -e "$EMBERLOOP" -p '
Do these in order, no other tool calls:
1. spawn with agents=[{"type": "worker", "ticket": "/tmp/emberloop-tell2/.scratch/tell/issues/01-slow.md", "repo": "/tmp/emberloop-tell2"}].
2. bash with command "sleep 25", so the worker is in its turn, not done.
3. tell with agent set to the id the spawn returned, and text:
   "Stop the current task. Create a file named TELLMARKER.txt containing exactly marker-received, commit it, then stop."
   A tell to a worker returns at once: the worker result arrives in this pane
   as a system notice, so end your turn and report that notice when it lands.
Report each tool result verbatim.' 2>&1 | grep -E "worker 01-slow|delivered|could not|TELLMARKER|not_ready|system-notice" | head -12

echo "=== ground truth ==="
if git -C "$DIR" log --all --oneline --name-only 2>/dev/null | grep -q TELLMARKER; then
  echo "PASS — tell reached a mid-turn worker"
  git -C "$DIR" log --all --oneline --name-only | grep -B3 TELLMARKER | head -8
else
  echo "FAIL — marker absent"
  git -C "$DIR" log --all --oneline | head -5
fi

for w in $(herdr workspace list 2>/dev/null | python3 -c "
import sys,json
for w in json.load(sys.stdin)['result']['workspaces']:
    if w['workspace_id']!='wY': print(w['workspace_id'])"); do herdr workspace close "$w" >/dev/null 2>&1; done
rm -rf "$DIR" ~/.pig/state/emberloop
