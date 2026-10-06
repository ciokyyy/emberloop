# 10: The strip gets icons, segment styling and rounded edges, in the style of omp's status line

**What to build:** The header strip is restyled as a row of colored segments, the way omp's status line reads: each stat (agent chip, ctx bar and percent, tokens, cost, model, top tools, state) is its own segment with an icon, a color from the shared palette, and a separator between segments. A symbol preset setting picks the glyph set: `nerd` (Nerd Font icons and rounded or powerline caps), `unicode` (plain symbols and half-block edges, the current look, the default) and `ascii`. The chip and state field use rounded caps in the `nerd` preset. A separator setting picks `powerline`, `powerline-thin`, `rounded`, `slash`, `pipe` or `none`. Segment colors map to named tokens (model, ctx, tokens, cost, tools, state, separator) generated from the one palette source, so a theme change recolors the strip, claude, pig and the pane together. Narrow widths drop segments in a fixed priority order, never mid-glyph. Wide characters and icons are width-measured, so alignment and the child's reduced height stay correct.

Reference: omp's theme and status-line system. Its docs describe symbol presets (`unicode`, `nerd`, `ascii`) with per-symbol overrides, rounded box tokens, and thirteen status-line color tokens; a third-party port lists the separator set above. Take the structure, not the code. Known trap from omp's own issue tracker: it shipped stale Nerd Font codepoints (legacy Material Design ranges) that render as wrong glyphs in Nerd Fonts v3, so every codepoint here must be checked against v3 and tested.

**Blocked by:** 06-shared-theme, 08-borderless-look

**Status:** ready-for-agent (restart on the current strip; the earlier attempt `116b6ee` on `implement/10-beautify-strip` is stale)

**Current strip (supersedes "the current look" above):** The spec branch strip already has rounded top and bottom edges, inner padding, background between cells (`0b7c03c`), pane padding (ticket 12), a running-tool label (`header_tool.go`), usage on the right for claude, opencode Go and Antigravity (`a5d954a`, `15a1a69`), no idle word, and a top-level chip. `unicode` must reproduce this look, not the older one. Keep those segments: tool label and right-side usage become segments of the new row. Do not rewrite what works: extend `header.go` rendering with presets, separators and tokens. The branch `116b6ee` branched before these changes and conflicts in `header.go`, `header_run.go`, `tools/settings.go` and `wick-pig/AGENTS.md`; reuse its `strip_symbols.go`, codepoint table and tests where they fit, and start from `spec/input-queue`, not from that branch.

- [ ] Strip renders as styled segments with an icon per segment in the `nerd` preset, and a readable text label per segment in `unicode` and `ascii`
- [ ] Symbol preset and separator are settings; the default stays `unicode` so a terminal without a Nerd Font looks right
- [ ] Rounded caps on the agent chip and state field in `nerd`; a half-block or plain edge in the other presets
- [ ] Segment color tokens come from the shared palette, in dark and light, and follow the generated themes
- [ ] Narrow width drops segments in a fixed order, tested at each tier, with no split glyph and no overflow
- [ ] All Nerd Font codepoints are Nerd Fonts v3 and listed in one table with a test that pins them
- [ ] Golden tests per preset, separator, width tier and color profile (truecolor, 256, 16)
- [ ] Live probe in a herdr pane for each preset with the installed Nerd Font, findings recorded in this ticket
- [ ] The strip height stays at the configured rows and the child size is unchanged
- [ ] Spec stories 32, 33, 34

Spec: `.scratch/input-queue/spec.md`. Read it for the decisions this ticket implements and its Out of Scope section.
