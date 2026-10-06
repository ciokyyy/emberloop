# 11: Claude agents under the wrapper register with herdr

**What to build:** A claude worker, reviewer or scout started through the launch file registers with herdr as a named agent and reports its state, the way pig panes do through the herdr-report extension. Today it does not: the wrapper is the pane's foreground process, so herdr no longer sees `claude` as the process and never marks the pane an agent. The spawn's readiness wait (`agent wait`) then fails with "agent target <pane> not found" for the whole wait, the spawn gives up and closes the pane, and every review round that uses a claude reviewer fails. The wrapper (or the exec path that starts it) reports the agent to herdr for a claude child: name, working and idle from the existing transcript turn state, and release on exit and on crash. Pig keeps the extension path; nothing is double-reported.

Evidence from the live check: reviewer panes for the code-review slot (harness claude) showed the strip for 20 seconds, never appeared in the agent list, and were closed by wick at the 30 second readiness limit. Pig reviewers in the same run registered fine. The herdr server log shows no agent-change event for the failed panes, and earlier claude panes (before the wrapper) did log one.

**Blocked by:** None (can start immediately)

**Status:** ready-for-agent

- [ ] A claude pane started through the launch file appears in the agent list with its type name within a few seconds of the child starting
- [ ] Its state follows the transcript: working while a turn runs, idle when it settles, released on exit, crash and signal
- [ ] The spawn readiness wait succeeds for claude reviewers and scouts; a spawn no longer closes a healthy pane
- [ ] A pane with a pig child is not reported twice; the extension stays the single reporter for pig
- [ ] A herdr that is unavailable or rejects the report never stops the child; the failure is logged
- [ ] Tests through the exec seam with a fake herdr: the exact report and release calls, for a claude child and for a pig child
- [ ] A live probe in a throwaway pane confirms `herdr agent list`, `agent wait` and `agent prompt` all work on a wrapped claude
- [ ] Spec: context-compress story 1 and the herdr agent identity rules

Spec: `.scratch/context-compress/spec.md`. Read it for the decisions this ticket implements and its Out of Scope section.
