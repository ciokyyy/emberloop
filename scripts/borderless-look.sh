#!/usr/bin/env bash
# Probe for ticket 08 (borderless look): run claude and pig under
# `wick-pig wrap` with the borderless defaults, then with the rule line and
# frame restored, and record what reads well and what does not.
#
# Prerequisites: built wick-pig on PATH, a herdr session with two free
# panes, the agent CLIs logged in, and the theme probe applied
# (.scratch/input-queue/theme/run-borderless.sh for the pane palette plus
# the wickb themes for the agents). Each check is manual: the script types
# the commands, a human confirms the behavior and records it in the ticket
# (live-probe result box). Never runs in the automated check.
set -uo pipefail
export PATH="$HOME/go/bin:$PATH"

PANE_PIG="${1:-}"; PANE_CLAUDE="${2:-}"
if [[ -z "$PANE_PIG" || -z "$PANE_CLAUDE" ]]; then
  echo "usage: borderless-look.sh <pig-pane> <claude-pane>" >&2
  exit 2
fi

echo "== borderless defaults (soft edge, no frame) =="
herdr pane run "$PANE_PIG" "wick-pig wrap --harness pig --agent probe-pig --type worker -- pig --theme /tmp/inprobe/pigthemeb --use-theme wickb"
herdr pane run "$PANE_CLAUDE" "wick-pig wrap --harness claude --agent probe-claude --type worker -- claude --settings '{\"theme\":\"custom:wickb\"}'"

cat <<'CHECKS'
Confirm per pane, then record pass/fail plus what reads well and what does
not in the ticket:
1. tint, not lines: no frame columns beside the agent screen, no rule line
   above the strip; the strip reads as a tinted block with a soft top edge
2. identity: the agent name chip and ctx label still carry the agent hue
3. calm column: the agent's input box border and any code/quote/hr lines sit
   at the surface tone, so input box and strip form one line-free column
4. contrast: the strip background sits one step off the pane background
5. resize: resize the pane, child and soft edge follow (reduced height)
6. alt-screen: run `vim` under wrap, use it, quit; the soft strip returns
Then restore the old look and compare (WICK_SETTINGS with
{"context":{"strip_rule":true,"strip_frame":true}}):
7. rule: the agent-hue dash line returns above the strip
8. frame: the hue columns return beside the screen; the child narrows by two
Record: which of soft/rule reads calmer, whether the input box still
competes with the strip, and any terminal where the tints blur together.
WICK_HEADER_STRIP=0 anywhere restores the old unwrapped behavior.
CHECKS
