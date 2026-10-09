# 08: One herdr key compresses the focused pane

**What to build:** A herdr plugin shipped in this repo and linked at install adds one action. A key bound through the herdr keybinding config sends it the focused pane ID, and it signals the supervisor for that pane, whatever the harness. If a tool call is running the request queues to the next boundary and the header strip shows `compress queued`. A second press forces the compress now. Repo install docs state the keybinding line the user adds.

**Blocked by:** 04-header-strip, 06-summarizer-and-manual-compress

**Status:** done

- [x] Plugin manifest with one compress action, linked by the install step
- [x] Pressing the key compresses the focused pane for pig, claude and the top-level session
- [x] Mid-tool-call press queues and the header strip shows `compress queued`
- [x] A second press forces the compress
- [x] The user-facing install step documents the keybinding config line
- [x] Tests for the action's signal path through the exec seam; no live herdr in the check
- [x] Spec stories 12, 13, 14

Spec: `.scratch/archive/context-compress/spec.md`. Read it for the decisions this ticket implements and its Out of Scope section.
