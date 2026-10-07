#!/usr/bin/env bash
# Probe for ticket 07 (live probe): run claude and pig under
# `wick-pig wrap --input-row` and confirm the wick row, steer and queue
# delivery, the stale clear, raw mode with its dirty flag, the OSC pane
# colors and the generated themes. It also covers a startup dialog that
# swallows typed text. It extends the header-strip spike script.
#
# Prerequisites: built wick-pig on PATH, a herdr session with two free
# panes, and the agent CLIs logged in. Each check is manual: the script
# types the commands, a human confirms the behavior and records it in the
# ticket (findings box). Never runs in the automated check.
#
# Safety: by default the script never submits a prompt to a model. The
# delivery checks that do (steer mid-turn, queue at turn end, order) run
# only with --submit. Every other check types into the row, toggles modes,
# or reads files, and the agents idle at their own input box.
# The script uses isolated panes and never touches running agents: pass two
# free panes and fresh probe agent ids.
set -uo pipefail
export PATH="$HOME/go/bin:$PATH"

PANE_PIG="${1:-}"; PANE_CLAUDE="${2:-}"; SUBMIT="${3:-}"
if [[ -z "$PANE_PIG" || -z "$PANE_CLAUDE" ]]; then
  echo "usage: input-queue-probe.sh <pig-pane> <claude-pane> [--submit]" >&2
  exit 2
fi
if [[ -n "$SUBMIT" && "$SUBMIT" != "--submit" ]]; then
  echo "usage: input-queue-probe.sh <pig-pane> <claude-pane> [--submit]" >&2
  exit 2
fi

command -v wick-pig >/dev/null || { echo "FAIL - wick-pig not on PATH; build and install first"; exit 1; }
command -v herdr >/dev/null || { echo "FAIL - herdr not on PATH"; exit 1; }
command -v python3 >/dev/null || { echo "FAIL - python3 needed for the control-channel helper"; exit 1; }

STATE_DIR="${HOME}/.pig/state/wick"
FIXDIR="$(cd "$(dirname "$0")/../wick-pig/testdata/theme" && pwd)"

# probe_ctrl <agent-id> <pending|steer|queue> [text]: one control-channel
# round trip to a wrapped agent. Matches wick-pig/inputqueue.go: JSON
# {"op":...} over the agent's unix socket, socket name sanitized like
# lockName (non [A-Za-z0-9-_] bytes become '-').
probe_ctrl() {
  AGENT_ID="$1" OP="$2" TEXT="${3:-}" python3 - <<'PY'
import json, os, socket
agent = os.environ["AGENT_ID"]
op = os.environ["OP"]
text = os.environ["TEXT"]
name = "".join(c if (c.isascii() and (c.isalnum() or c in "-_")) else "-" for c in agent) or "lock"
path = os.path.join(os.path.expanduser("~"), ".pig", "state", "wick", "agents", f"ctrl-{name}.sock")
req = {"op": "pending"} if op == "pending" else {"op": "enqueue", "kind": op, "text": text, "source": "probe-07"}
s = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
s.settimeout(3)
try:
    s.connect(path)
except OSError as e:
    print(f"FAIL - no wrapper socket for {agent}: {e}")
    raise SystemExit(1)
s.sendall((json.dumps(req) + "\n").encode())
res = json.loads(s.recv(4096).decode())
if not res.get("ok"):
    print(f"FAIL - channel refused: {res.get('error')}")
    raise SystemExit(1)
print(f"OK pending={res['pending']}")
PY
}

echo "== 0. automated: generated themes match fixtures =="
autofail=0
for f in claude-mocha pig-mocha; do
  if [[ -f "$STATE_DIR/themes/$f.json" && -f "$FIXDIR/$f.json" ]]; then
    if diff -q "$STATE_DIR/themes/$f.json" "$FIXDIR/$f.json" >/dev/null; then
      echo "PASS - $f matches fixture"
    else
      echo "FAIL - $f differs from fixture (generated file changed?)"
      autofail=1
    fi
  else
    echo "SKIP - $f not generated yet; launch an agent under wrap first, then rerun"
  fi
done

echo "== 1. smoke: fake child under the row (no model) =="
cat > /tmp/wick-row-fake.sh <<'FAKE'
#!/usr/bin/env bash
echo "fake agent ready"
read -r line
echo "heard: $line"
FAKE
chmod +x /tmp/wick-row-fake.sh
herdr pane run "$PANE_PIG" "wick-pig wrap --input-row --harness pig --agent probe-fake --type worker -- /tmp/wick-row-fake.sh"

cat <<'CHECKS'
Confirm in the fake pane, then record pass/fail in ticket 07:
1. row: an input row sits between the child screen and the strip
2. hold: typing in the row sends nothing to the child (no echo) until Enter
3. submit: Enter delivers the line; the fake prints `heard: <line>`
4. strip: no `N pending`, no `raw` while idle
Kill the fake (Ctrl+C) when done.
CHECKS

echo "== 2. live agents under the row (idle, no submission) =="
herdr pane run "$PANE_PIG" "wick-pig wrap --input-row --harness pig --agent probe-pig-07 --type worker -- pig"
herdr pane run "$PANE_CLAUDE" "wick-pig wrap --input-row --harness claude --agent probe-claude-07 --type worker -- claude"

cat <<'CHECKS'
Confirm per pane (pig, then claude), no prompt submitted yet:
5.  row: the wick row is visible; the child's own box still renders above it
6.  hold: type a draft in the row, do NOT submit; `probe_ctrl <agent> pending`
    still reports the old count and the strip shows no new `N pending`,
    because nothing was enqueued — then check the reverse: enqueue one
    injection with `probe_ctrl <agent> steer held-probe-text` while the
    draft sits; the strip shows `1 pending` and nothing reaches the child
    until the draft is submitted (spec: injections wait while the row
    holds a draft)
7.  order: with that injection pending, submit the row draft with Enter;
    the user's own message goes out before the pending injection (spec:
    user first, then injections in arrival order)
8.  keys: Shift+Enter inserts a newline, Alt+Enter queues (no steer until
    the turn ends), Up recalls row history, Ctrl+C/Esc forward an
    interrupt to the child, bracketed paste lands as one edit
9.  resize: shrink the pane; row and strip stay, child gets the reduced height
10. theme: agent colors follow the strip palette (borders/rules at the
    surface tone); pane background matches the strip; saved theme choice
    untouched (pig/claude settings files unmodified: check with git or
    file mtime before and after)
11. exit colors: quit the agent; pane colors reset to the terminal defaults
    (OSC 104/110/111/112 on exit, including on signal: kill -INT one wrap
    and confirm the reset too)
CHECKS

echo "== 3. stale clear (10 s, no keystroke) =="
cat <<'CHECKS'
12. row stale: type a draft in the row, wait 10 s without a keystroke; the
    row clears and the held injection is delivered (row mode sends no clear
    sequence: the row just clears)
13. raw stale: toggle raw (Ctrl+O), type a dirty draft in the child's box,
    wait 10 s; the wrapper sends six Ctrl+U, then delivers the queued item
CHECKS

echo "== 4. raw mode =="
cat <<'CHECKS'
14. toggle: Ctrl+O switches row <-> child; the strip shows `raw`, and
    `raw, N queued` while items wait
15. dirty: any forwarded byte except a bare Enter marks the box dirty
    (strip holds); Enter, Ctrl+C or the wick clear resets it (delivery
    resumes at once when not dirty)
16. hold: enqueue an injection while dirty; nothing reaches the child until
    the stale clear or a reset; Ctrl+C is never used as the clear sequence
17. dialogs: in raw mode reach one of the agent's own dialogs/menus;
    raw keys work as the bare harness (claude Enter/Ctrl+Enter, pig
    Alt+Enter/Alt+Up apply here only)
CHECKS

echo "== 5. startup dialog that swallows typed text =="
cat <<'CHECKS'
18. dialog: on a fresh claude launch (first start can show a
    fullscreen-renderer dialog), enqueue an injection at once with
    `probe_ctrl probe-claude-07 steer dialog-probe-text`; the text the
    dialog swallows is detected as not accepted (no user turn in the
    transcript), the item stays at the head of the queue (`pending` stays
    1, strip keeps `N pending`), and it is delivered after the dialog is
    dismissed — never lost, never duplicated
CHECKS

if [[ "$SUBMIT" == "--submit" ]]; then
cat <<'CHECKS'
== 6. delivery to the model (--submit given) ==
19. steer: while the agent runs a tool call, submit the row with Enter;
    the message steers at the next tool boundary, mid-turn
20. queue: submit with Alt+Enter mid-turn; it waits until the turn ends
21. repeat per harness (pig AND claude): both deliver through the shared
    path, never typed into the child's box byte by byte
Record the claude version (`claude --version`) with any failure.
CHECKS
else
  echo "SKIP - checks 19-21 need --submit (they send prompts to the model)"
fi

cat <<'RECORD'
== findings box: paste into ticket 07, one line per behavior ==
- row/hold/submit (fake): pass|fail ...
- row hold + strip pending: pass|fail ...
- user-before-injections order: pass|fail ...
- keys (newline/queue/history/interrupt/paste): pass|fail ...
- resize: pass|fail ...
- stale clear row + raw (six Ctrl+U): pass|fail ...
- raw toggle/strip/dirty/hold/dialogs: pass|fail ...
- OSC set + exit reset (+signal): pass|fail ...
- generated themes + saved settings untouched: pass|fail ...
- startup dialog swallow + retry: pass|fail ...
- steer/queue delivery (--submit): pass|fail|skip ...
- claude version: ...
- terminal(s) tested: ...
RECORD

if [[ "$autofail" -ne 0 ]]; then exit 1; fi
