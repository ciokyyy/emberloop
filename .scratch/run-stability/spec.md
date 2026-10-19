# Run stability

One spec, from the 2026-10-18 grilling Session. It makes Runs reach `done` without a person by removing the mechanical causes of stranded Runs (Part A) and adding a Babysitter that diagnoses and fixes what is left (Part B). Part C replaces the scattered Daemon connection banners with one global banner. Decision record for Part B: `docs/adr/0006-babysitter-escalate-only.md`.

## Problem Statement

A retro of 25 live Run records, the session archive and the last six specs shows that Runs rarely stall inside a turn; the driver already has stall nudges, dead-turn detection, ACP respawn and Chains. Runs end in states that need a person:

1. Four fully merged Runs (about 60 reviewed Tickets) sit in needs-you at the final merge for days. Every cause is mechanical: a dirty master checkout (untracked `output/*`, `.pig/mcp.json`, acp-bench files) or rebase conflicts on generated `web/dist` files.
2. Nine Runs were abandoned at birth on a red base. spec-05 started at least eight times against the same red base; each attempt cost a 2 to 10 minute base check and a human triage. Others failed at Start on a stale `implement/*` branch or a missing `.emberloop/check` in the base commit.
3. Reviewer loops have no ceiling: 46 rounds on one Ticket, and 114 `changes` rounds in web-v1 with one nit repeated about 90 times on an unchanged tip. Worker rework turns take 3 to 18 minutes each.
4. Daemon restart waits up to 5 minutes for Runs, then force-kills. Runs resume afterwards, so the wait is pure delay.
5. Infra flakes lose finished work: a 60 second submit proxy timeout dropped a completed Worker turn, and a "session is resuming" race failed a Run.
6. A few waits are unbounded: the final repo check, the generated build, git operations, and a Sudo approval.
7. Connection state shows in five places: the `App` "Daemon unreachable" error, the `Root` offline page, the `ChatStatus` header glyph, and a "Reconnecting to live updates…" line each in the Ticket and Runs views. The `App` error sets on a failed Session list refresh and clears only when a Session opens, so on a phone it stays after the Daemon recovers. It also floats over the transcript on a phone and covers message text. An installed PWA has no refresh gesture to recover with.

Constraint from the User: no caps on Reviewer rounds and no automatic stops of agents.

## Solution

Part A fixes the deterministic causes in driver code: the final merge runs in its own worktree so the User's master checkout never blocks it, generated `web/dist` conflicts resolve by regeneration, Start refuses a bad base with a reason (a red base spawns one "fix base" Ticket first), the restart drain no longer waits on Runs, and the unbounded command waits get timeouts. These bound commands, not agents.

Part B adds the Babysitter, a cheap per-Run agent Session that wakes on trouble signals, investigates with read-only tools, and acts through named, driver-validated actions: Steer a Worker or Reviewer, retry a needs-you, run a named hygiene step, or raise needs-you with a diagnosis. It never interrupts, pauses, Switches or ends a Session, and a Reviewer loop ends only because the Babysitter Steers or escalates. It ships in shadow mode first.

Part C adds one connection banner in the app shell, driven by one connection state. It shows *reconnecting* or *unreachable*, hides on its own on recovery, and carries a Refresh button. It replaces the per-view copies and is small, one line and quiet.

## User Stories

1. As a User, I want the final merge to run outside my master checkout, so that my uncommitted work never strands a finished Run.
2. As a User, I want a final merge that hits generated `web/dist` conflicts to regenerate them, so that a rebase conflict on build output needs no person.
3. As a User, I want a real source conflict at final merge to still raise needs-you with the conflicting files named, so that I resolve what only I can.
4. As a User, I want Start to refuse a Run on a red base with the failing test named, so that I do not launch a Run that parks every Ticket.
5. As a User, I want a red base to spawn one "fix base" Ticket that runs before the spec Tickets, so that the Run can proceed once it is green.
6. As a User, I want Start to refuse when a stale `implement/*` branch blocks it and to remove branches the Run owns, so that a leftover branch does not fail the Run.
7. As a User, I want Start to refuse when `.emberloop/check` is missing in the base commit, so that the cause is named instead of every Ticket being parked.
8. As a User, I want the restart drain to stop waiting when only Runs are busy, so that a restart takes seconds, not five minutes.
9. As a User, I want restarted Runs to resume at a tool boundary on the same Session, so that no turn is lost (today's behavior is kept).
10. As a User, I want the final repo check, the generated build and git operations to have timeouts, so that a hung command cannot block a Run silently.
11. As a User, I want a timed-out command to raise needs-you naming the command, so that I know what hung.
12. As a User, I want no cap on Reviewer rounds, so that real findings are never dropped.
13. As a User, I want a `babysitter` role with a model and effort Chain in Settings > Agents, so that I choose its model.
14. As a User, I want a Babysitter mode `off`, `shadow` or `active`, default `shadow`, so that I judge diagnosis quality before it acts.
15. As a User, I want a healthy Run to start no Babysitter Session, so that it costs nothing when nothing is wrong.
16. As a User, I want the Babysitter woken by a stall nudge, a repeated Reviewer finding on an unchanged tip, review round 4 or later, a needs-you, a refused final merge, a Reviewer silent for 10 minutes, or a tool call open for 30 minutes, so that it sees trouble early.
17. As a User, I want at most one wake per Ticket per five minutes and identical signals deduplicated, so that a noisy signal does not burn tokens.
18. As a User, I want the Babysitter to read the Event log, Run state, review history, the Ticket file and Run worktrees with `read`, `find` and an allowlisted `git` tool, so that it can diagnose a merge or a loop.
19. As a User, I want the Babysitter to be unable to write files, run `bash`, spawn, cancel or steer directly, so that it cannot damage a worktree or my master checkout.
20. As a User, I want the Babysitter to Steer a Worker with a concrete instruction such as "commit your work" or "answer finding X in a reply", so that a stuck turn gets unstuck.
21. As a User, I want the Babysitter to Steer a Reviewer that a finding was already raised on this tip, so that a loop breaks without the Reviewer losing its verdict.
22. As a User, I want the Babysitter to call a named hygiene step such as removing a stale Run branch or cleaning untracked build artifacts in a Run worktree, so that mechanical dirt is cleared.
23. As a User, I want a Babysitter action dropped when the tip, round or state changed since it looked, so that it never acts on stale facts.
24. As a User, I want the Babysitter to raise needs-you with its diagnosis when two Steers fail or no action fits, so that a loop never runs forever unseen.
25. As a User, I want each Babysitter diagnosis and action shown as a muted line on the Ticket timeline, so that I read it where I already look.
26. As a User, I want an escalation to appear in the needs-you reason, so that I see the diagnosis without opening a log.
27. As a User, I want shadow mode to keep read tools real and log what actions it would have taken, so that I can compare its judgement with what happened.
28. As a User, I want a Run to continue on today's nudges and alerts when the Babysitter errors, hangs or goes silent, so that it never blocks a Run.
29. As a User, I want the Babysitter renewed by Handoff at about 100k tokens, only at a turn boundary, so that its context stays small and no investigation is cut mid-turn.
30. As a User, I want a Run-wide Babysitter budget of about 1M tokens, after which it goes silent with one timeline note, so that spend is bounded.
31. As a User, I want the Babysitter Session to end with its Run, so that nothing lingers.
32. As a User, I want a per-Run `babysitter.log` of its diagnoses and actions, so that the log feeds the Handoff and my review.
33. As a Reviewer, I want to receive a Babysitter Steer like any other Steer, so that I need no new protocol.
34. As a User, I want one connection banner for the whole app, so that I read connection state in one place.
35. As a User, I want the banner to show *reconnecting* while the live stream is down or a refresh fails, and *unreachable* when the Daemon has not answered for more than 15 seconds, so that I tell a blip from an outage.
36. As a User, I want the banner to hide by itself when a request or the stream succeeds, so that it never stays after recovery.
37. As a User, I want a Refresh button on the banner that checks for a new version and reloads, so that I recover from an installed PWA with no refresh gesture.
38. As a User, I want the banner to be one small line of quiet text with an icon, so that it does not clutter the screen.
39. As a phone User, I want the banner under the header, above the composer and clear of the keyboard, so that it never hides what I am typing.
40. As a phone User, I want the transcript scroll position to hold when the banner appears or hides, so that I do not lose my place.
41. As a User, I want to see a static mock of the banner states and approve it before it is built, so that the look is mine.

## Implementation Decisions

Part A, deterministic:

- **Final merge worktree.** The final merge of the spec branch into master runs in a dedicated worktree. The rebase onto master, the generated build and the final check happen there, and the result fast-forwards master only if the User's master checkout can take it without touching uncommitted work; otherwise it stays on a named branch and the Run raises a single needs-you with the exact one-line command to land it. A dirty master checkout is never a reason to refuse.
- **Generated-file conflicts.** A rebase conflict whose files are all generated build output (the `web/dist` set) resolves by regenerating the build and continuing. A conflict touching any other file raises needs-you naming those files.
- **Start preflight.** Before the first Ticket starts: the base commit has `.emberloop/check`; no stale `implement/*` branch belongs to another Run; the base check is green. A stale branch owned by an abandoned or failed Run is removed; an unowned one is named in the refusal. A red base refuses Start with the failing test named, and offers one "fix base" Ticket that must merge before spec Tickets begin. The base check result is cached and reused by the Ticket checks as today.
- **Restart drain.** Restart already pauses live Runs at tool boundaries, resumes the same Worker after boot, and resumes after a hard kill. The drain wait ends when no Session other than a paused Run Session is busy, so a restart no longer waits five minutes on Runs.
- **Command timeouts.** The final repo check, the generated build and driver git operations get timeouts (the final check reuses the per-check cap). A timeout raises needs-you naming the command. Per-agent-turn deadlines are not added. A Sudo approval keeps waiting for the User, but the Run shows it as the reason.

Part B, Babysitter:

- **Role and settings.** A new `babysitter` role in Agents settings holds a model and effort Chain, host-global and admin-only, like Memory settings. A mode `off | shadow | active` sits beside it, default `shadow`. Per-action enablement (Steer first, hygiene second) is a driver flag, not UI.
- **Lifecycle.** One Session per Run, started lazily on the first wake, woken by Follow-up, ended with the Run. Wakes are throttled to one per Ticket per five minutes with dedupe. At about 100k tokens it finishes the current turn and renews by Handoff into a fresh Session. A Run-wide budget of about 1M tokens ends its activity with one timeline note.
- **First prompt.** Carries the Run summary, the trigger signal, and pointers: Ticket file, `babysitter.log`, Event log path, worktree paths. The Babysitter reads the rest.
- **Tools.** `read`, `find`, and an allowlisted `git` tool (`status`, `diff`, `log`, `show`, `merge-base`, `ls-files`). Scope is Run-owned worktrees and the spec branch; the master checkout is read-only. No `bash`, no write or edit, no `spawn`, `cancel` or `steer`.
- **Actions.** An MCP of named actions: `steer_worker`, `steer_reviewer`, `retry`, `hygiene(name)`, `escalate(note)`. Hygiene names are a fixed driver-side set. Actions queue as pending; a driver pass consumes them under the per-Ticket lock and drops any whose tip, round or state differs from what the Babysitter saw. Steers go through the Ticket `steer` action, not a raw Session steer.
- **Repeat detection.** "Unchanged tip" is mechanical, from the stored round tip and diff patch id. "Same topic" is the Babysitter's judgement over stored findings. The existing prompt-only late-round Reviewer note stays.
- **Fail-open.** An error, a hang or silence leaves the Run on today's nudge and needs-you behavior; the driver never waits on the Babysitter. After three consecutive failed wakes in a Run it stays silent until the User re-enables it.
- **Visibility.** Timeline lines and needs-you reasons, as above. No new push notification; needs-you already has one.
- **Shadow mode.** Read tools work; action tools only write to `babysitter.log` and the timeline what they would have done.
- **Glossary and ADR.** `GLOSSARY.md` defines Babysitter. ADR 0006 records the decision.

Part C, connection banner:

- **Connection state.** One store derives `ok | reconnecting | unreachable` from the live stream and failed refreshes. It clears on any successful request or stream hello. `unreachable` applies after the Daemon has not answered for 15 seconds; background retry continues throughout.
- **One banner.** The app shell renders the banner for the store state. The `App` unreachable error, the Ticket view line and the Runs view line are removed. `loadError` stays for non-connectivity errors. The `ChatStatus` glyph stays. The `Root` offline page uses the same copy and Refresh button, because `App` is not mounted there.
- **Refresh.** The button asks the service worker to check for a new version, then reloads the page, which also picks up a new Daemon build.
- **Look.** One line: small icon, short label in caption-size text, small Refresh button. No box, no dismiss. Smaller type than the current error state; the Designer chooses the exact scale from the design tokens.
- **Phone.** A strip in the layout flow under the header (it takes its own height and never overlays the transcript or the Reports tray), safe-area aware, above the composer, kept clear of the keyboard through the visual viewport as the composer is. It does not shift transcript scroll.
- **Ticket split.** A `code` Ticket for the connection store and the removals, then a `design` Ticket, blocked by it, that builds the banner. The static mock (reconnecting, unreachable, phone with keyboard, lifecycle, desktop) was shown to the User during the grilling and lives in `mock/` as the reference; the design Ticket builds to it. The mock adds a brief green "Back online" state before the strip hides.

## Testing Decisions

- A good test drives the Run driver through its world seam and asserts externally visible state (Ticket status, Run state, sent Steers, files on disk), not internals.
- Highest seam: the Run driver with a fake world, as in the existing driver tests. Prior art: the Run driver tests for stall nudges, Chains and restart resume, and the offline `fakellm` server in `scripts/`.
- Part A: tests for each preflight refusal, a dirty master checkout during final merge, a `web/dist` rebase conflict, a source conflict, command timeouts, and a restart with busy Runs.
- Part B: Babysitter logic tests against a fake model seam. The fixture corpus is recorded digests and git states of real stranded cases: a Reviewer loop (the 46-round Ticket), a `web/dist` merge conflict, a dirty-master refusal, and a lost submit. Each fixture asserts the expected action.
- One live probe in `scripts/` runs a real cheap model against a recorded Run; it stays out of the check.
- Part C: the store maps stream and refresh outcomes to states, clears on recovery and reaches `unreachable` after 15 seconds; the banner appears in each state and hides on recovery; Refresh calls the service worker update and reloads; the replaced per-view copies are gone. Prior art: the `ChatStatus`, `Root` and layout-sweep tests. The design Ticket adds a phone layout assertion that scroll position holds.
- Settings: the role appears, the mode switch persists, and `off` starts no Session.
- The repo check stays `.emberloop/check`.

## Out of Scope

- Reviewer round caps, per-turn deadlines, and any automatic interrupt, pause, Switch or Session end.
- The Babysitter editing code, running a shell, or waiving Reviewer findings.
- Changing the Reviewer seats or their models.
- Model churn and Chain defaults; the 4 attempts for designer-agents are not addressed here.
- Pig backend watchdog tuning and the 60 second submit proxy timeout beyond what a Babysitter diagnosis can flag; fixing the proxy timeout is a candidate follow-up.
- Querying `emberloop.db` for deeper history.
- A text-size change across the rest of the app; only the banner is in scope.

## Further Notes

- Retro data caveat: the history scout had no shell. Large Run records were head and tail sampled, `emberloop.db` (232 MB) was not read, and the project-init Run `db193a3a` that findings.md documents has no record. Numbers are directional.
- Slowest verified waits: base check evaluations of 104 to 600 seconds, Worker rework turns of 3 to 18 minutes, Reviewer rounds of 10 to 257 seconds. Speed is secondary to reaching `done`; per-phase timing on the Ticket timeline is a possible follow-up.
- Spec-mandated live probes stay open longest (run-upgrade 23, designer-agents 15, ssr 03); the Babysitter probe here is optional so it does not join them.
