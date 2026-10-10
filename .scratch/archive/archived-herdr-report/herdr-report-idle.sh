#!/usr/bin/env bash
# The previous run only ever saw `working`: pig -p runs one turn and exits, so
# session_start's `idle` and agent_settled's `idle` had no window to appear in.
# An interactive session that sits at a prompt gives them one.
set -uo pipefail
export PATH="$HOME/go/bin:$PATH"
REPORT=/home/cioky/Projects/emberloop/.scratch/archive/archived-herdr-report
DIR=/tmp/herdr-report-idle
rm -rf "$DIR"; mkdir -p "$DIR"

WS=$(herdr workspace create --cwd "$DIR" --label idle-probe --no-focus 2>&1)
PANE=$(printf '%s' "$WS" | python3 -c "import sys,json;print(json.load(sys.stdin)['result']['root_pane']['pane_id'])")
WID=$(printf '%s' "$WS" | python3 -c "import sys,json;print(json.load(sys.stdin)['result']['workspace']['workspace_id'])")

# No -p: pig starts an interactive session and waits at the prompt.
herdr pane run "$PANE" env EL_AGENT_LABEL=idleprobe pig --no-skills --no-tools -e "$REPORT" >/dev/null 2>&1

for i in $(seq 1 20); do
  printf "  t=%2ds  " "$i"
  herdr agent list 2>/dev/null | python3 -c "
import sys,json
try: agents=json.load(sys.stdin)['result']['agents']
except Exception: print('-'); raise SystemExit
for a in agents:
    if a.get('pane_id')=='$PANE':
        print('agent=%s name=%s status=%s'%(a.get('agent'),a.get('name'),a.get('agent_status'))); break
else: print('-')"
  sleep 1
done

echo "--- get by reported name ---"; herdr agent get idleprobe 2>&1 | head -c 200; echo
herdr workspace close "$WID" >/dev/null 2>&1
rm -rf "$DIR"