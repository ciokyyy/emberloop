#!/usr/bin/env bash
# Probe for ticket 07 (live probe): run claude and pig under
# `el wrap --input-row` and confirm the el row, steer and queue
# delivery, the stale clear, raw mode with its dirty flag, the OSC pane
# colors and the generated themes. It also covers a startup dialog that
# swallows typed text. Companion to scripts/header-strip-spike.sh, which
# covers the strip alone; this probe covers the row, queue and theme on
# top. Injections go through scripts/probe-ctrl.sh.
#
# Prerequisites: built el on PATH, a herdr session with two free
# panes, and the agent CLIs logged in. Each check is manual: the script
# types the commands, a human confirms the behavior and records it in the
# ticket (findings box). Never runs in the automated check.
#
# Safety: by default the script sends nothing to a model: no row submit to
# a live agent, no injection enqueued, no Enter with text in a live box.
# The fake-child smoke submits only to the fake. Everything that delivers
# text to a model (order, stale delivery, raw hold, startup dialog,
# steer/queue) runs only with --submit.
# The script uses isolated panes and never touches running agents: pass two
# free panes and fresh probe agent ids.
set -uo pipefail
export PATH="$HOME/go/bin:$PATH"

SUBMIT=""
PANES=()
for arg in "$@"; do
  if [[ "$arg" == "--submit" ]]; then SUBMIT="--submit"; else PANES+=("$arg"); fi
done
PANE_PIG="${PANES[0]:-}"; PANE_CLAUDE="${PANES[1]:-}"
if [[ -z "$PANE_PIG" || -z "$PANE_CLAUDE" || "${#PANES[@]}" -ne 2 ]]; then
  echo "usage: input-queue-probe.sh <pig-pane> <claude-pane> [--submit]" >&2
  exit 2
fi

command -v el >/dev/null || { echo "FAIL - el not on PATH; build and install first"; exit 1; }
command -v herdr >/dev/null || { echo "FAIL - herdr not on PATH"; exit 1; }

HERE="$(cd "$(dirname "$0")" && pwd)"
CTRL="$HERE/probe-ctrl.sh"
STATE_DIR="${HOME}/.pig/state/emberloop"
FIXDIR="$(cd "$HERE/../emberloop/testdata/theme" && pwd)"

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
FAKE="$(mktemp /tmp/el-row-fake.XXXXXX.sh)"
cat > "$FAKE" <<'FAKE_EOF'
#!/usr/bin/env bash
echo "fake agent ready"
read -r line
echo "heard: $line"
FAKE_EOF
chmod +x "$FAKE"
herdr pane run "$PANE_PIG" "el wrap --input-row --harness pig --agent probe-fake --type worker -- $FAKE"

cat <<'CHECKS'
Confirm in the fake pane, then record pass/fail in ticket 07:
1. row: an input row sits between the child screen and the strip
2. hold: typing in the row sends nothing to the child (no echo) until Enter
3. submit: Enter delivers the line; the fake prints `heard: <line>`
4. strip: no `N pending`, no `raw` while idle
Kill the fake (Ctrl+C) when done, and delete the fake script.
CHECKS

echo "== 2. live agents under the row (idle: nothing sent, safe by default) =="
herdr pane run "$PANE_PIG" "el wrap --input-row --harness pig --agent probe-pig-07 --type worker -- pig"
herdr pane run "$PANE_CLAUDE" "el wrap --input-row --harness claude --agent probe-claude-07 --type worker -- claude"

cat <<'CHECKS'
Confirm per pane (pig, then claude). Send nothing: do NOT press Enter with
text, do NOT enqueue. Backspace any draft away afterwards.
5. row: the el row is visible; the child's own box still renders above it
6. hold: type a draft in the row, do NOT submit; the child shows nothing
   new (spec: typing in the row never reaches the child until submit)
7. keys (submit-free): Shift+Enter inserts a newline, bracketed paste lands
   as one edit, Ctrl+C/Esc forward an interrupt, Up on an empty history
   recalls nothing
8. resize: shrink the pane; row and strip stay, child gets the reduced height
9. theme: agent colors follow the strip palette (borders/rules at the
   surface tone); pane background matches the strip; saved theme choice
   untouched (pig/claude settings files unmodified: check file mtime
   before and after)
10. exit colors: quit the agent; pane colors reset to the terminal defaults
    (OSC 104/110/111/112 on exit, including on signal: kill -INT one wrap
    and confirm the reset too)
CHECKS

echo "== 3. raw mode (idle: no Enter with text, safe by default) =="
cat <<'CHECKS'
11. toggle: Ctrl+O switches row <-> child; the strip shows `raw`
12. dirty: type a few chars in the child's box (do NOT press Enter); the
    strip holds them as dirty; press Ctrl+C to reset (interrupt, no submit)
    and delivery resumes at once when not dirty
13. dialogs: in raw mode reach one of the agent's own dialogs/menus; raw
    keys work as the bare harness (claude Enter/Ctrl+Enter, pig
    Alt+Enter/Alt+Up apply here only); leave menus without choosing
    anything that sends
CHECKS

if [[ "$SUBMIT" == "--submit" ]]; then
cat <<CHECKS
== 4. queue, order and stale delivery (--submit given: sends to the model) ==
14. hold: type a draft in the row, do NOT submit; in another shell run
    \`$CTRL probe-pig-07 pending\` (old count), then
    \`$CTRL probe-pig-07 steer held-probe-text\`; the strip shows
    \`1 pending\` and nothing reaches the child until the draft is
    submitted (spec: injections wait while the row holds a draft)
15. order: with that injection pending, submit the row draft with Enter;
    the user's own message goes out before the pending injection (spec:
    user first, then injections in arrival order)
16. history: Up recalls the submitted row text
17. row stale: type a draft, enqueue an injection as in 14, wait 10 s
    without a keystroke; the row clears with no clear sequence and the
    held injection is delivered
18. raw stale + hold: toggle raw (Ctrl+O), type a dirty draft, enqueue an
    injection; nothing reaches the child until the stale clear; after
    10 s the wrapper sends six Ctrl+U, then delivers (Ctrl+C is never the
    clear sequence); \`raw, N queued\` shows while items wait
19. steer: while the agent runs a tool call, submit the row with Enter;
    the message steers at the next tool boundary, mid-turn
20. queue: submit with Alt+Enter mid-turn; it waits until the turn ends
21. repeat 14-20 per harness (pig AND claude): both deliver through the
    shared path, never typed into the child's box byte by byte
22. dialog: on a fresh claude launch (first start can show a
    fullscreen-renderer dialog), enqueue at once with
    \`$CTRL probe-claude-07 steer dialog-probe-text\`; the text the dialog
    swallows is detected as not accepted (no user turn in the transcript),
    the item stays at the head of the queue (\`pending\` stays 1, strip
    keeps \`N pending\`), and it is delivered after the dialog is
    dismissed — never lost, never duplicated
Record the claude version (\`claude --version\`) with any failure.
CHECKS
else
  echo "SKIP - checks 14-22 need --submit (they send prompts to the model)"
fi

cat <<'RECORD'
== findings box: paste into ticket 07, one line per behavior ==
- row/hold/submit (fake 1-4): pass|fail ...
- row hold, keys, resize (dry 5-8): pass|fail ...
- theme + saved settings untouched (dry 9): pass|fail ...
- OSC set + exit reset + signal (dry 10): pass|fail ...
- raw toggle/dirty/dialogs (dry 11-13): pass|fail ...
- hold + user-first order (--submit 14-16): pass|fail|skip ...
- stale clear row + raw six-Ctrl+U (--submit 17-18): pass|fail|skip ...
- steer/queue per harness (--submit 19-21): pass|fail|skip ...
- startup dialog swallow + retry (--submit 22): pass|fail|skip ...
- claude version: ...
- terminal(s) tested: ...
RECORD

if [[ "$autofail" -ne 0 ]]; then exit 1; fi
