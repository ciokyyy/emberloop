# 08: Borderless look: tinted blocks instead of lines and frame columns

**What to build:** The agent pane drops its drawn borders and separates areas by background tint instead. The agent-colored frame columns beside the agent screen go off by default (the setting already exists). The strip's top rule line is replaced by a soft edge: a half-block row in the strip background, so the strip reads as a tinted block with no line. The generated claude and pig themes pull their own border and rule colors (input box borders, code block and quote borders, horizontal rules) down to the surface tone, so the agent's input box and the strip form one calm, line-free column. Agent identity still shows in the name chip and ctx label. A probe in the spec's `theme/` directory (`run-borderless.sh`, `claude-wickb.json`, `pig-wickb.json`) shows the target; the soft-edge row was prototyped by a throwaway patch to the strip's top rule that is not in the repo.

**Blocked by:** 06-shared-theme

**Status:** done

- [x] Frame columns default off; the `strip-frame` setting still turns them on
- [x] The strip's top row is a soft-edge row in the strip background by default
- [x] A setting can restore the rule line
- [x] Child size stays correct with the frame off and the soft-edge row on, including after resize and alt-screen
- [x] The generated claude and pig themes set every border and rule token to the surface tone, from the same palette source as the strip
- [x] The strip background stays one step off the pane background in dark and light palettes
- [x] Golden tests for the soft-edge row at each color profile and for the generated theme border tokens
- [x] Live probe covers the borderless look and records what reads well and what does not
- [x] Spec stories 22, 26, 30, 31

Spec: `.scratch/input-queue/spec.md`. Read it for the decisions this ticket implements and its Out of Scope section.
