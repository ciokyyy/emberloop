#!/usr/bin/env bash
# Spike for ticket 04 (header strip): run claude and pig under
# `emberloop wrap` in live panes and confirm the wrapper keeps them usable.
#
# Prerequisites: built emberloop on PATH, a herdr session with two free
# panes, and the agent CLIs logged in. Each check is manual: the script
# types the commands, a human confirms the behavior and records it in the
# ticket (spike result box). Never runs in the automated check.
set -uo pipefail
export PATH="$HOME/go/bin:$PATH"

PANE_PIG="${1:-}"; PANE_CLAUDE="${2:-}"
if [[ -z "$PANE_PIG" || -z "$PANE_CLAUDE" ]]; then
  echo "usage: header-strip-spike.sh <pig-pane> <claude-pane>" >&2
  exit 2
fi

# A fake agent for the smoke pass: prints, reads one line, resizes, uses
# alt-screen, then exits. Run this first; the live harnesses second.
cat > /tmp/emberloop-strip-fake.sh <<'FAKE'
#!/usr/bin/env bash
echo "fake agent ready"
printf '\e[?1049hTop of alt screen\e[?1049l\n'
read -r line
echo "heard: $line"
FAKE
chmod +x /tmp/emberloop-strip-fake.sh

echo "== pig pane $PANE_PIG =="
herdr pane run "$PANE_PIG" "emberloop wrap --harness pig --agent spike-pig --type worker -- /tmp/emberloop-strip-fake.sh"
echo "== claude pane $PANE_CLAUDE =="
herdr pane run "$PANE_CLAUDE" "EMBERLOOP_HARNESS=claude emberloop wrap --agent spike-claude --type worker -- /tmp/emberloop-strip-fake.sh"

cat <<'CHECKS'
Confirm per pane, then record pass/fail in the ticket:
1. strip: two rows show under the child (ctx bar, percent, cost, state)
2. typing: type a line, the child echoes it back (input forwarding)
3. resize: resize the pane, child and strip follow (reduced height)
4. mouse: run `vim` under wrap, click/scroll still work
5. alt-screen: the fake's alt-screen flip keeps the strip behind it
6. herdr: `herdr agent list` still detects the live pig/claude session
Then replace the panes' commands with the real agents:
  herdr pane run <pig> 'emberloop exec <launch>'  # unchanged: exec wraps now
  and use claude/pig for five minutes (edits, tool calls, a TUI app).
EMBERLOOP_HEADER_STRIP=0 anywhere restores the old unwrapped behavior.
CHECKS
