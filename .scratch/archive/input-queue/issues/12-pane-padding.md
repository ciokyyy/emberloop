# 12: The agent pane has padding on the left, right and bottom

**What to build:** The wrapped pane gets breathing room. The agent's screen and the strip are inset by a few columns on the left and right, and a few rows of empty space sit below the strip, so text no longer touches the pane edge and the bottom of the strip no longer sits on the last terminal row. Padding is empty space in the pane background: no lines, no frame, so the borderless look stays. The child is told the reduced size and gets it again on every resize, the same way it does for the strip rows. Defaults are four columns left and right and two rows at the bottom. Settings change each value, and zero restores today's edge-to-edge layout. Narrow terminals shrink the padding first, before the strip drops a row or a segment.

**Blocked by:** None (can start immediately)

**Status:** done

- [x] Left and right padding columns and bottom padding rows are settings with the defaults above (4, 4, 2); zero for all three gives today's layout
- [x] The child's size is the terminal minus the padding and the strip, at start and after every resize, with alt-screen output and region resets repaired inside it
- [x] The strip is drawn inside the left and right padding and above the bottom padding
- [x] Padding is background only: no characters are drawn in it
- [x] A narrow terminal shrinks padding to zero before any strip segment or row is dropped
- [x] Golden tests for the child size and strip placement with default, zero and oversized padding
- [x] Live probe with claude and pig confirms that nothing draws into the padding, including the child's first full redraw
- [x] Spec stories 30, 34

Spec: `.scratch/archive/input-queue/spec.md`. Read it for the decisions this ticket implements and its Out of Scope section.
