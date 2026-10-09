# 09: Every pig executor shows the header strip

**What to build:** Pig workers, reviewers and scouts show the same header strip as the orchestrator. Today they do not: a pig launch takes the debug tools-log path, which runs the child unwrapped so its stderr stays separate from the terminal, and that path is on by default for every pig session started through the launch file. Only the top-level orchestrator, which starts through the top-level wrapper, gets the strip. Fix it so the tools log and the strip coexist: the wrapper gives the child's stderr its own pipe while stdin and stdout stay on the PTY, so the tools log still reads raw stderr and the strip is drawn. A bad exit still lands in the agent's state with its stderr tail. Claude executors already take the wrapped path; confirm they show the strip too.

Evidence from the live check: the orchestrator pane shows the strip; the two pig worker panes started after the install show pig's own footer only, and their tools logs were created at launch time.

**Blocked by:** None (can start immediately)

**Status:** done (code merged in a08d9e0 and afc8242; live-pane check of claude and pig workers still open)

- [x] A pig worker, reviewer and scout launched through the launch file show the strip, with the name chip of their type
- [x] The tools log still receives pig's raw stderr, one stamped line at a time
- [x] A pig child that exits badly records its exit and stderr tail as before
- [x] `EL_PIG_TOOLS_LOG=0` and `EL_HEADER_STRIP=0` still switch each feature off independently
- [ ] Claude workers and reviewers show the strip, confirmed in a live pane
- [x] Tests through the wrapper's exec seam with a fake child that writes to stderr: the strip is painted and the stderr lines reach the log
- [x] Spec: context-compress story 1 (every agent pane shows the header strip)

Spec: `.scratch/archive/context-compress/spec.md`. Read it for the decisions this ticket implements and its Out of Scope section.
