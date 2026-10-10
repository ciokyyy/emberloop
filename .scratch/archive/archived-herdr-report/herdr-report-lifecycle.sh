#!/usr/bin/env bash
# Exercises all four herdr-report handlers and checks the claim in the package
# doc comment that the reported name + session identity make the agent
# addressable by `herdr agent prompt`.
set -uo pipefail
export PATH="$HOME/go/bin:$PATH"
REPORT=/home/cioky/Projects/emberloop/.scratch/archive/archived-herdr-report
LABEL=plugintest
DIR=/tmp/herdr-report-test

rm -rf "$DIR"; mkdir -p "$DIR"

WS=$(herdr workspace create --cwd "$DIR" --label plugin-test --no-focus 2>&1)
PANE=$(printf '%s' "$WS" | python3 -c "import sys,json;print(json.load(sys.stdin)['result']['root_pane']['pane_id'])")
WID=$(printf '%s' "$WS" | python3 -c "import sys,json;print(json.load(sys.stdin)['result']['workspace']['workspace_id'])")
echo "pane: $PANE  workspace: $WID"

# argv[0] is the program, and pane run types argv into the shell as one line, so
# the prompt is pre-quoted as a single argument.
herdr pane run "$PANE" env EL_AGENT_LABEL=$LABEL pig --no-skills --no-tools \
  -e "$REPORT" -p '"Write 150 numbered lines about the history of the bicycle."' >/dev/null 2>&1

state() {
  herdr agent list 2>/dev/null | python3 -c "
import sys,json
try: agents=json.load(sys.stdin)['result']['agents']
except Exception: print('-'); raise SystemExit
for a in agents:
    if a.get('pane_id')=='$1': print('%s|%s'%(a.get('agent'),a.get('agent_status'))); break
else: print('-')"
}

seen="-"; prompted=no
for i in $(seq 1 180); do
  cur=$(state "$PANE")
  if [ "$cur" != "$seen" ]; then echo "  t=$((i))s  $cur"; seen="$cur"; fi
  case "$cur" in *plugintest*)
    if [ "$prompted" = no ]; then
      prompted=yes
      echo "  --- prompt by name '$LABEL' (claim under test) ---"
      herdr agent prompt "$LABEL" "reply with the single word PONG" 2>&1 | head -c 200; echo
    fi ;;
  esac
  [ "$seen" = "-" ] && [ "$i" -gt 20 ] && break
  sleep 1
done

echo "final pane state: $(state "$PANE")"
echo "--- after exit: prompt by name ---"; herdr agent prompt "$LABEL" "ping" 2>&1 | head -c 200; echo
echo "--- after exit: prompt by pane ---"; herdr agent prompt "$PANE" "ping" 2>&1 | head -c 200; echo
echo "--- herdr's own agent list ---"
herdr agent list 2>&1 | python3 -c "
import sys,json
for a in json.load(sys.stdin)['result']['agents']:
    print('  %s | agent=%s | name=%s | %s'%(a.get('pane_id'),a.get('agent'),a.get('name'),a.get('agent_status')))"

herdr workspace close "$WID" >/dev/null 2>&1
rm -rf "$DIR"