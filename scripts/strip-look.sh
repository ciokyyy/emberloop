#!/usr/bin/env bash
# Probe for ticket 10 (beautify strip): run pig under `emberloop wrap` with
# each symbol preset (nerd, unicode, ascii) and each separator, and record
# what reads well and what does not.
#
# Prerequisites: built emberloop on PATH, a herdr session with one free pane,
# the pig CLI logged in, and a Nerd Font active in the terminal showing the
# pane (the nerd preset is boxes without one; unicode is the default and
# stays correct). Each check is manual: the script types the commands, a
# human confirms the behavior and records it in the ticket (live-probe
# result box). Never runs in the automated check.
set -uo pipefail
export PATH="$HOME/go/bin:$PATH"

PANE="${1:-}"
if [[ -z "$PANE" ]]; then
  echo "usage: strip-look.sh <pane>" >&2
  exit 2
fi

# Codepoint coverage first: every nerd glyph must exist in the installed
# font (Nerd Fonts v3 ranges: FA ED00-F2FF, powerline E0B0-E0B3, extra
# E0B4-E0C8). Meslo Nerd Font passes; a missing codepoint here means the
# font, not the strip, is stale.
for cp in f007 f200 f1c0 f155 f2db f0ad f066 f111 f10c f017 f021 f071 e0b0 e0b1 e0b4 e0b6; do
  echo "codepoint $cp: check the nerd strip below renders one glyph, not a box"
done

run_preset() {
  local symbols="$1" separator="$2"
  local settings
  settings="$(mktemp)"
  printf '{"context":{"strip_symbols":"%s","strip_separator":"%s"}}' "$symbols" "$separator" >"$settings"
  herdr pane run "$PANE" "EMBERLOOP_SETTINGS='$settings' emberloop wrap --harness pig --agent probe-strip --type worker -- pig"
}

cat <<'CHECKS'
Confirm per preset, then record pass/fail plus what reads well and what
does not in the ticket:
1. nerd/powerline: one icon per segment, powerline gaps between segments,
   rounded caps around the agent chip and the state field, segment colors
   from the emberloop palette (model mauve, cost peach, tools blue)
2. nerd with each other separator (powerline-thin, rounded, slash, pipe,
   none): the gap glyph changes, nothing else moves, no box glyphs
3. unicode (default): the current plain-symbol look, unchanged when the
   separator is none; pipe/slash read as plain dividers
4. ascii: labels only, ASCII gaps, no high bytes at all
5. narrow: shrink the pane; segments drop whole (tools, compress count,
   model, cost) keeping percent and state, never a split glyph
6. rows: `strip-rows 1` keeps one row in every preset
EMBERLOOP_HEADER_STRIP=0 anywhere restores the old unwrapped behavior.
CHECKS

echo "== nerd/powerline =="
run_preset nerd powerline
echo "rerun: $0 $PANE types one preset; edit symbols/separator below for the rest"
echo "presets: unicode (default), ascii; separators: powerline-thin rounded slash pipe none"
