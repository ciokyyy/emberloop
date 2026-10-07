#!/usr/bin/env bash
# Proves tell reaches a live worker: the worker must abandon its ticket and do
# what it was told instead. Checked by looking for the marker commit on the
# worker's branch, not by trusting the parent's report.
set -uo pipefail
export PATH="$HOME/go/bin:$PATH"
EMBERLOOP=/home/cioky/Projects/emberloop/emberloop
REPORT=/home/cioky/Projects/emberloop/herdr-report
DIR=/tmp/emberloop-tell

rm -rf "$DIR" ~/.pig/state/emberloop; mkdir -p "$DIR/.scratch/tell/issues"
printf 'alpha\nbeta\n' > "$DIR/app.txt"
cat > "$DIR/.scratch/tell/issues/01-tell.md" <<'TICKET'
Change line 1 of app.txt from alpha to GAMMA.
TICKET
git -C "$DIR" init -q
git -C "$DIR" -c user.email=w@x -c user.name=emberloop add -A >/dev/null
git -C "$DIR" -c user.email=w@x -c user.name=emberloop commit -qm base

cd "$DIR" && timeout 420 env EMBERLOOP_HERDR_REPORT="$REPORT" \
  pig --no-skills -e "$EMBERLOOP" -p '
Do these in order, no other tool calls:
1. spawn with agents=[{"type": "worker", "ticket": "/tmp/emberloop-tell/.scratch/tell/issues/01-tell.md", "repo": "/tmp/emberloop-tell"}].
2. bash with command "sleep 20", so the worker is well into its turn.
3. tell with agent set to the id the spawn returned, and text:
   "Stop working on the ticket. Instead create a file named TELLMARKER.txt containing exactly the text marker-received and nothing else, commit it, then stop."
   A tell to a worker returns at once: the worker result arrives in this pane
   as a system notice, so end your turn and report that notice when it lands.
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
rm -rf "$DIR" ~/.pig/state/emberloop
