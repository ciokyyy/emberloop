# Context visibility and session compression

Status: ready-for-agent. Local tracker: this directory, tickets under `issues/`.

## Problem Statement

A user running agents through emberloop cannot see how full an agent's context window is, how many tokens it has used, what it costs, or which tools fill the context. Claude Code cannot compact on the user's terms: the user cannot choose the summary prompt, the threshold, or the summarizer model. Long sessions degrade or hit the window with no warning.

Separately, emberloop still makes agents spend an extra turn reading a file when it steers or re-prompts them (reviewer follow-ups, steering a running agent). Emberloop's principle is to inject context directly.

## Solution

1. Every agent pane, for any harness, shows a header strip: context use against the window, cost estimate, top tools and state. The strip is drawn by emberloop's own launcher process with a PTY wrapper, inside the terminal, not through herdr. It looks designed: colors, a gradient context bar, aligned columns.
2. Emberloop compresses a session itself. When context passes a threshold (default 200k tokens) and the agent sits at a tool-call boundary, emberloop runs a one-off pig summarizer over the transcript, restarts the agent in the same pane, and delivers the handoff as the first message. The user can also trigger it with one herdr key for the focused pane.
3. Emberloop delivers every prompt as message text. No "read this file" nudges anywhere.

## Design decisions made in the interview

- Applies to all sessions: pig, claude, the user's own top-level session, and any future harness.
- Threshold: 200k tokens of current context, changeable in settings (global, with per-harness override).
- Context size = latest request's input plus cache read plus cache write plus that message's output. Not cumulative tokens. Same formula for every harness parser.
- Cost: estimated, same method the `emberloop-tui` stats already use. Shown with a `~` mark.
- Header: drawn by the process that owns the agent child, using a PTY. Not a herdr pane label. Placed at the bottom of the pane, two rows by default (height in settings), so the agent's own screen keeps its top-left origin. Spike first: prove claude and pig render correctly under the wrapper before building on it.
- Auto trigger: only at a tool boundary (every tool call in the transcript has its result) or at idle. Never interrupts a running tool call.
- Manual trigger: a herdr plugin action bound to a key. It compresses the focused pane. If a tool call is running, the request queues to the next boundary and the header shows `compress queued`. A second press forces it. No slash commands.
- Compress recipe v1: Matt Pocock handoff structure, with caveman ultra for prose, fabricated into one combined prompt. Code, paths, errors and IDs stay verbatim. No pruning of tool output, no raw tail kept in v1.
- Summarizer: always a pig one-off, no tools, no system prompt beyond the combined prompt. Model comes from a new per-harness `compressModel` setting. Unset means auto-compress refuses and the header shows a warning. No silent default.
- Handoff delivery: the text is the first message of the new session, passed directly. A temp file carries it to the supervisor and is then deleted. A copy stays under the agent's state for rollback and audit.
- Failure: the old session stays untouched unless the summary was written and passes a sanity check (non-empty, expected sections). The old transcript is kept.
- After an auto-compress between tool calls, the new session gets a final line telling it to continue the interrupted step. Manual compress resumes only if a task was open.
- Identity: the emberloop agent ID and the pane stay. The harness session ID changes at each restart. Usage rows keep accumulating on the agent ID. The header shows a `compressed xN` count.
- Guards: a setting to disable auto-compress (manual still works), and a cooldown of N tool calls after a compress. No loop guard: a handoff is always far under the threshold.
- Restart mechanism: the process that owns the child (the top-level `emberloop` command for the user's session, the launch executor for spawned panes) becomes a supervisor loop. It stops the child, runs the summary, starts a new child, and keeps the pane.

## Visual design

The header is one strip drawn by the launcher around the agent's terminal. One palette, one set of rules.

Rules, from the Go TUI practice (Bubble Tea, Lip Gloss, Bubbles):
- One adaptive palette via Lip Gloss, no raw ANSI numbers. One bold accent, dim secondary text, status by glyph plus color.
- Numbers humanized and rounded: `142k`, `~$1.82`. Never raw floats. Empty or zero stats hidden, not printed as `0 tok`.
- Fixed-width columns and joins for alignment, not manual padding.
- Context bar with threshold colors: calm below 60%, warm 60 to 80%, hot above 80%.
- Width tiers, so a narrow pane drops the tools list, then the cost, and never cuts a number in half.

Strip mockup, two rows at the bottom of the pane (wide tier):
- Row 1: `ctx ▰▰▰▰▰▰▱▱▱▱ 71%  142k / 200k   ~$1.82   opus  compressed x2`
- Row 2: `top  bash 31k   read 22k   grep 9k          ● working`
- Medium: row 1 only. Narrow: `ctx 71% · ~$1.82`.
- State field: `working`, `idle`, `compress queued`, `compressing…`, or a warning such as `no compressModel`. The bar colors follow the thresholds above.



Palette: Catppuccin, Mocha for dark terminals and Latte for light, picked through Lip Gloss adaptive colors. Truecolor first; Lip Gloss downgrades to 256 and 16 colors on its own, so the strip stays readable everywhere.

| Role | Mocha (dark) | Latte (light) |
|---|---|---|
| Strip background | #181825 | #e6e9ef |
| Text | #cdd6f4 | #4c4f69 |
| Dim labels | #6c7086 | #9ca0b0 |
| Accent (ctx label, model) | #cba6f7 | #8839ef |
| Bar calm, working | #a6e3a1 | #40a02b |
| Bar warm | #f9e2af | #df8e1d |
| Bar hot, warning | #f38ba8 | #d20f39 |
| Cost | #fab387 | #fe640b |
| Tool names | #89b4fa | #1e66f5 |
| Idle, compressing | #94e2d5 | #179299 |

The bar is a gradient: each filled cell blends green to yellow to red by its position, and unfilled cells use the dim color. The percent and state glyph take the threshold color. Compressing pulses between the accent and idle colors. The strip background differs from the agent's background by one step, so the strip reads as a bar without a border line.

### Visual guide

The approved mockup is `mockup.html` in this directory. Open it in a browser. It is the reference for layout, spacing, colors, states and tiers. Where this text and the mockup differ, ask; they are meant to agree. Browser CSS approximates a terminal in two ways: the state pills use translucent fills (a terminal uses plain colored text, or a solid dim background), and bar cells are CSS blocks (a terminal uses `▰` and `▱` or colored block cells).

Everything is drawn in the terminal by the wrapper. The mockup is only a picture.

### Per-agent identity color

Each agent gets one hue, so panes are told apart at a glance. The hue comes from a fixed set, picked by hashing the stable agent ID, so it never changes across restarts or compressions. The orchestrator always uses mauve.

| Slot | Mocha | Latte |
|---|---|---|
| mauve (orchestrator) | #cba6f7 | #8839ef |
| pink | #f5c2e7 | #ea76cb |
| lavender | #b4befe | #7287fd |
| sapphire | #74c7ec | #209fb5 |
| sky | #89dceb | #04a5e5 |
| flamingo | #f2cdcd | #dd7878 |
| maroon | #eba0ac | #e64553 |

The agent hue is used for: the strip's top rule, a name chip at the start of row 1 (the agent type name, same as the herdr pane name: orchestrator, implementer, spec-reviewer, code-reviewer, scout), and the `ctx` label. Threshold colors (green, yellow, red) and the cost, tool and state colors do not change per agent, so status never gets confused with identity. Green, yellow and red are not in the agent set for that reason.

Pane frame: herdr draws pane borders and emberloop cannot color them per agent, so the frame is emberloop's own. The wrapper reserves one column on each side and draws a thin frame line in the agent hue beside the agent's screen, plus the strip's top rule. This costs two columns and is a setting (default on, can be turned off to only keep the top rule and chip). The spike must confirm the frame survives the agent's resizes and alt-screen use.

The strip has a pure render function (width and snapshot in, styled string out) so it can be golden-tested.

## User Stories

1. As a user, I want each agent pane to show how full its context is, so that I know when it will degrade.
2. As a user, I want the header strip to show tokens used against the window, so that I can read the real number.
3. As a user, I want an estimated cost in the header, so that I can watch spend.
4. As a user, I want the top three tools by context share, so that I can see what fills the window.
5. As a user, I want the same header for pig, claude and future harnesses, so that I do not learn one per tool.
6. As a user, I want my own top-level session to show the header too, so that I see the session I type in.
7. As a user, I want the bar to change color as it fills, so that I notice it without reading.
8. As a user, I want the strip to shrink on narrow panes, so that it never truncates mid-number.
9. As a user, I want auto-compress when context passes my threshold, so that sessions keep working.
10. As a user, I want compress to wait for a tool call to finish, so that no running tool dies.
11. As a user, I want to set the threshold in settings, so that I can fit my model and budget.
12. As a user, I want one key to compress the focused pane, so that I need no slash commands per agent.
13. As a user, I want a queued manual compress shown in the header, so that I know it will happen.
14. As a user, I want a second press to force a compress, so that I can act when a tool hangs.
15. As a user, I want a `compressing…` state in the header, so that I know why the agent paused.
16. As a user, I want the new session in the same pane, so that my layout and focus stay.
17. As a user, I want the agent to continue its interrupted step after auto-compress, so that work resumes without me.
18. As a user, I want the handoff delivered as the first message, so that the agent needs no extra turn.
19. As a user, I want a separate compress model setting per harness, so that I pick a cheap long-window model.
20. As a user, I want a visible warning when no compress model is set, so that I know why auto-compress is off.
21. As a user, I want a failed summary to leave the old session running, so that I never lose work.
22. As a user, I want the old transcript and the handoff kept, so that I can inspect or roll back.
23. As a user, I want a `compressed xN` count, so that I see how many handoffs a session went through.
24. As a user, I want usage and cost to keep accumulating across a compress, so that totals stay true.
25. As a user, I want to disable auto-compress and keep the manual key, so that I stay in control.
26. As a user, I want a cooldown after a compress, so that a bad run cannot compress repeatedly.
27. As an orchestrator, I want reviewer follow-ups and steering sent as message text, so that agents spend no turn reading a file.
28. As a reviewer, I want the incremental diff inline in the follow-up, so that I read it in one message.
29. As a maintainer, I want one shared prompt-delivery path, so that compress, steering and follow-ups behave the same.
30. As a maintainer, I want a new harness to plug in with a parser and a header render only, so that support costs little.

## Implementation Decisions

Modules, by responsibility (no file paths):

- **Context core.** A per-harness transcript parser reports: current context size, count of outstanding tool calls, per-tool result sizes, and the model. One shared formula for context size. A model window table with settings overrides; today none exists.
- **Cost estimate.** Reuse the cost estimation `emberloop-tui` already uses. The strip calls the same function.
- **Header strip.** The agent's parent process runs the child on a PTY sized to the terminal minus the strip rows, tells the child that size and forwards resize, input, mouse and alt-screen traffic untouched, and draws the strip in the reserved rows at the bottom, using a scroll region so child output never scrolls the strip. It redraws on each new usage snapshot and on a timer for state. Child sequences that would reset the scroll region or clear the screen are handled so the strip survives. If the spike shows this fails for claude or pig, the fallback is a virtual-terminal compositor that renders the child's screen into a grid and paints the strip beneath it.
- **Supervisor.** The parent of each agent child becomes a loop. It watches a compress request (auto from the core, manual from the plugin), waits for a boundary, stops the child, runs the summarizer, starts the new child with the handoff as the first prompt, and updates the agent's current session ID. It never closes the pane.
- **Summarizer.** Spawns a pig one-off with no tools. Input is the full transcript rendered as text (user messages, assistant text, tool calls and results, all in full). If the input exceeds the compress model's window, compression is refused with a header warning. Settings validation checks that the compress model's window is large enough.
- **Compress prompt.** A pinned, combined prompt stored with emberloop: the Matt Pocock handoff structure plus the caveman ultra rules from the existing caveman skill, plus the continue-the-interrupted-step line. It is a pinned copy, not read from the installed library at runtime.
- **Prompt delivery.** One shared helper that sends text to an agent by message. Used by compress, reviewer follow-ups, steering of a running agent, and late prompts. The temp files stay only as archive copies. The incremental review diff goes inline, always.
- **Session identity.** Most state is already keyed by the agent ID: usage rows, agent state, submit marks, gate state, error marks, review state, pane mapping. The only session-ID keys are transcript lookup and the usage tailer's identity, plus launch and config file names. The agent's state gains a current session ID, updated at each restart. Transcript lookup and the tailer read it. A new child gets a fresh UUID, which also avoids any reuse of an old session ID.
- **Settings.** New fields: `compressModel` (per harness), compress threshold (global, per-harness override), auto-compress on/off, cooldown, model window overrides, price table overrides if needed. Exposed through the existing settings CLI and `emberloop-tui` settings.
- **Pane names.** Emberloop names each herdr pane after its agent type (orchestrator, implementer, spec-reviewer, code-reviewer, scout). Names need not be unique. One source of names feeds herdr and the strip chip.
- **Herdr plugin.** A plugin shipped in this repo, linked at install. One action, bound to a key through the herdr keybinding config, that takes the focused pane ID and signals the supervisor. Herdr is used for this key only, not for the header.
- **`emberloop-tui`.** Not changed by this spec.

Order, so tickets do not block each other badly: prompt delivery first (it fixes today's extra turn and compress needs it), then context core, then header publisher, then supervisor and compress, then the plugin.

## Testing Decisions

A good test checks external behavior: given a transcript, a request, or a width, assert the observable result. No tests on internals.

Seams, highest first. Prefer one:
- **Supervisor behavior through the existing exec seam** with a fake exec, a fake child and temp transcripts. Covers the trigger rule, restart, handoff delivery, failure keeps old session, identity update, cooldown.
- **Context core as a pure function over transcript fixtures** (existing test data area). Covers the formula, outstanding-tool detection, and per-tool sizes for each harness.
- **Header render as a pure function of width and a snapshot**, golden strings per tier.
- **Prompt delivery** through the existing tell and nudge tests, asserting the text sent, never a file path.

Prior art: the exec-seam tests in the main package, the usage parser tests with transcript fixtures, and `emberloop-tui` model tests. No test starts a real agent or harness, touches the real home, or uses the network beyond loopback.

A spike and live probe in `scripts/` (not part of the check) run claude and pig under the wrapper and confirm rendering, input, resize, mouse, alt-screen and herdr's agent detection still work.

## Out of Scope

- Pruning tool output, keeping a raw tail of recent turns, session-memory shortcuts, server-side compact.
- Parent agents compressing their workers through a tool. Only auto and the user's key in v1.
- Redesign of the whole `emberloop-tui` dashboard.
- Header as a herdr pane label or sidebar text.
- Handoff quality tuning beyond the first combined prompt.

## Further Notes

- Research basis: other harnesses summarize with a structured prompt and keep recent turns (Codex around 20k tokens, Gemini 30% of the tail), and OpenCode prunes tool output first. v1 deliberately uses the handoff skill only. Revisit pruning if the compress model window becomes a problem.
- The strip costs terminal rows. Height is a setting; narrow terminals drop the second row first.
- Open check for implementation: whether claude accepts a reused `--session-id`. The design avoids the question by using a fresh UUID per restart.
- Open check for implementation: herdr detects agent state from the pane; the wrapper must not break that. Wrapper processes already sit in each spawned pane today, so this is likely fine, but the spike confirms it.
