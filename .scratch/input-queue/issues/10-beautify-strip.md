# 10: The strip gets icons, segment styling and rounded edges, in the style of omp's status line

**What to build:** The header strip is restyled as a row of colored segments, the way omp's status line reads: each stat (agent chip, ctx bar and percent, tokens, cost, model, top tools, state) is its own segment with an icon, a color from the shared palette, and a separator between segments. A symbol preset setting picks the glyph set: `nerd` (Nerd Font icons and rounded or powerline caps), `unicode` (plain symbols and half-block edges, the current look, the default) and `ascii`. The chip and state field use rounded caps in the `nerd` preset. A separator setting picks `powerline`, `powerline-thin`, `rounded`, `slash`, `pipe` or `none`. Segment colors map to named tokens (model, ctx, tokens, cost, tools, state, separator) generated from the one palette source, so a theme change recolors the strip, claude, pig and the pane together. Narrow widths drop segments in a fixed priority order, never mid-glyph. Wide characters and icons are width-measured, so alignment and the child's reduced height stay correct.

Reference: omp's theme and status-line system. Its docs describe symbol presets (`unicode`, `nerd`, `ascii`) with per-symbol overrides, rounded box tokens, and thirteen status-line color tokens; a third-party port lists the separator set above. Take the structure, not the code. Known trap from omp's own issue tracker: it shipped stale Nerd Font codepoints (legacy Material Design ranges) that render as wrong glyphs in Nerd Fonts v3, so every codepoint here must be checked against v3 and tested.

**Blocked by:** 06-shared-theme, 08-borderless-look

**Status:** done

**Current strip (supersedes "the current look" above):** The spec branch strip already has rounded top and bottom edges, inner padding, background between cells (`0b7c03c`), pane padding (ticket 12), a running-tool label (`header_tool.go`), usage on the right for claude, opencode Go and Antigravity (`a5d954a`, `15a1a69`), no idle word, and a top-level chip. `unicode` must reproduce this look, not the older one. Keep those segments: tool label and right-side usage become segments of the new row. Do not rewrite what works: extend `header.go` rendering with presets, separators and tokens. The branch `116b6ee` branched before these changes and conflicts in `header.go`, `header_run.go`, `tools/settings.go` and `emberloop/AGENTS.md`; reuse its `strip_symbols.go`, codepoint table and tests where they fit, and start from `spec/input-queue`, not from that branch.

- [x] Strip renders as styled segments with an icon per segment in the `nerd` preset, and a readable text label per segment in `unicode` and `ascii`
- [x] Symbol preset and separator are settings; the default stays `unicode` so a terminal without a Nerd Font looks right
- [x] Rounded caps on the agent chip and state field in `nerd`; a half-block or plain edge in the other presets
- [x] Segment color tokens come from the shared palette, in dark and light, and follow the generated themes
- [x] Narrow width drops segments in a fixed order, tested at each tier, with no split glyph and no overflow
- [x] All Nerd Font codepoints are Nerd Fonts v3 and listed in one table with a test that pins them
- [x] Golden tests per preset, separator, width tier and color profile (truecolor, 256, 16)
- [x] Live probe in a herdr pane for each preset with the installed Nerd Font, findings recorded in this ticket
- [x] The strip height stays at the configured rows and the child size is unchanged
- [x] Spec stories 32, 33, 34

## Live-probe result (implement/10-beautify-strip)

Automated part done; herdr-pane visual pass still manual
(`scripts/strip-look.sh`; scripts never run inside ticket work).

- Codepoint coverage: all 21 nerd codepoints (12 FA icons, 3 powerline
  separators, 2 rounded caps, plus pipe/arrow fallbacks) resolve in the
  installed Meslo Nerd Font (`fc-match` charset check: 13554 codepoints,
  zero missing). No Material Design icon is used, dodging the stale
  legacy-range trap: FA lives at U+ED00-U+F2FF and powerline extras at
  U+E0B0-U+E0D7 in v3.
- Offline renders (71% ctx fixture, separator powerline): nerd shows
  capped chip/pill with one icon per segment; unicode shows plain labels
  with ▶ gaps; ascii is pure high-byte-free labels with > gaps. Narrow
  widths keep percent and state, dropping chip, cost, model, tools and
  compress count whole in that order.
- Manual remainder: run `scripts/strip-look.sh <pane>` once per preset
  with the Nerd Font active and confirm no box glyphs, then check the
  box above.

Spec: `.scratch/input-queue/spec.md`. Read it for the decisions this ticket implements and its Out of Scope section.
