# 04: Every agent pane shows a header strip

**What to build:** Every agent run through wick, for any harness and for the user's own top-level session, shows a designed header strip at the bottom of its terminal. The process that owns the agent child runs it on a PTY sized to the terminal minus the strip rows, forwards input, mouse, resize and alt-screen traffic untouched, and draws the strip in the reserved rows using a scroll region so child output never scrolls it. The strip shows context use against the window with a gradient bar, an estimated cost with a tilde (same estimate the dashboard uses), the top three tools, model, compress count and state, redrawn on each new usage snapshot. Visual rules and mockups are in the spec. It is drawn with Lip Gloss from one adaptive palette, with humanized numbers, aligned columns, empty stats hidden, and width tiers. It is not a herdr label.

The first step is a spike: run claude and pig under the wrapper in a live probe and confirm rendering, typing, resize, mouse, alt-screen and herdr's agent detection all still work. If they do not, build the fallback described in the spec (a virtual-terminal compositor) and record the finding in this ticket.

**Blocked by:** 02-context-core

**Status:** done

- [x] Spike probe in the scripts area shows claude and pig usable under the wrapper; result recorded here
- [x] Pure render function: snapshot and width in, styled string out, golden tests for each tier
- [x] Each agent gets a stable hue from the spec's identity set (hash of the agent ID, mauve for the orchestrator), used for the strip top rule, the name chip and the ctx label; threshold colors never use agent hues
- [x] Optional agent-colored frame: one column each side beside the agent screen, setting default on; survives resize and alt-screen in the spike
- [x] The result matches the mockup in the spec directory (layout, tiers, states)
- [x] Colors follow the spec palette (Catppuccin Mocha and Latte, adaptive), gradient bar, background one step off the agent's; golden tests also run at 16-color and 256-color downgrade
- [x] Bar thresholds: calm below 60%, warm 60 to 80%, hot above 80%
- [x] Cost shown as an estimate with a tilde, never a raw float; zero or empty stats not printed
- [x] Strip height is a setting, default two rows, narrow terminals drop the second row first
- [x] Child resize is forwarded with the reduced height; child output never overwrites or scrolls the strip
- [x] Works for spawned panes and the top-level session, harness-independent
- [x] Wrapper tests through the exec seam with a fake child; no live agent in the check
- [x] Spec stories 1 to 8

Spec: `.scratch/context-compress/spec.md`. Read it for the decisions this ticket implements and its Out of Scope section.
