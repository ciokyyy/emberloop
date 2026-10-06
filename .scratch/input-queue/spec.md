# Wick-owned input row, prompt queue, macros and shared theme

Builds on `.scratch/context-compress/spec.md` (header strip, supervisor, shared prompt delivery). Research notes: `research-gate.md` in this directory. Theme probe assets: `theme/` in this directory.

## Problem Statement

A user typing a prompt in an agent's own input box can have it mangled when wick delivers text to the same agent: compress handoffs, reviewer follow-ups, steering and late prompts all arrive on the same terminal input the user types on. Half-typed text and injected text interleave. Wick also has no place to offer its own commands. Claude's `/compress`-style commands do not exist for pig, and wick launches claude with its slash commands disabled. Finally the agents look nothing like the header strip: each harness paints its own palette.

## Solution

The process that owns the agent child (the wrapper from the header-strip ticket) also owns the user's input. The user types in a wick input row beneath the child's screen, never in the child's own box. Wick is the only writer to the child's PTY, so injected prompts and user messages are serialized through one queue and never interleave. Wick-defined macros such as `/compress` work for every harness with no per-harness extension. A manual raw mode hands the keyboard to the child for its own dialogs, and wick holds injections while raw mode is active or the user is mid-draft. The same palette that drives the strip themes claude, pig and the pane's own colors.

## Decisions made in the interview

- **Option B.** The wick input row replaces the child's own input box for the user's top-level session. Native child editing (vim mode, `@` completion, history of the child) is given up in the row. A raw mode toggle gives it back on demand.
- **Row keys, v1.** Enter submits, Alt+Enter queues, Shift+Enter inserts a newline, bracketed paste, Up recalls row history, Ctrl+C or Esc forwards an interrupt to the child. No `@` completion, no vim mode in the row.
- **Steer by default.** A submitted message goes to the agent at the next tool boundary, as steering. Alt+Enter queues it instead: it goes out after the current turn ends. (Pig names Alt+Enter "queue follow-up" too.)
- **Plain text delivery.** Row text goes to the child by the shared message-delivery path, never typed into the child's own box byte by byte.
- **Macros: only wick's own.** Wick handles registered macros only. Any other `/x` text is a normal message. Wick macros win on a name collision, and settings validation warns about it. Claude's own slash commands are already disabled under wick's launch flags.
- **Macro kinds, v1.** Prompt template macros (name to prompt text with `{{args}}`) and built-in action macros. `/compress` ships as the first built-in. Macros are defined in the settings file. Sequences are out.
- **Scope.** The top-level user session gets the input row. Workers, reviewers and scouts have no human typing, so they get the queue and its control channel with no row.
- **Wick is the only writer.** The wrapper owns a control channel (unix socket). Compress, reviewer follow-ups, steering, late prompts and macros all send to it. The existing herdr `agent prompt` and `send-text` path stays as the fallback only for agents that run with no wrapper.
- **Hold rules.** Injections wait while the wick row holds a draft, while raw mode is on, and while the child has not finished starting up. In raw mode the wrapper keeps a conservative dirty flag: any forwarded byte except a bare Enter marks the child's box dirty. Enter, Ctrl+C or a wick clear resets it. The flag errs towards dirty.
- **Stale draft.** After 10 seconds with no keystroke, wick clears the draft and delivers. In the wick row it clears the row. In raw mode it sends the clear sequence to the child, then delivers.
- **Child clear sequence.** Ctrl+U sent six times. Probed: it fully clears a three-line draft in both claude and pig, and is harmless on an empty box. Ctrl+C is never used for clearing, because a second Ctrl+C within 500 ms exits pig.
- **Order.** The user's own submitted message goes before pending injections. After a compress restart the queue and the row draft survive, and the handoff goes ahead of everything.
- **Visibility.** The strip shows pending count and mode: `N pending`, `raw`, `raw, N queued`, alongside the existing `compress queued` and `compressing` states.
- **Borderless look.** Areas are separated by background tint, not lines. The agent-colored frame columns are off by default (the setting stays). The strip's top edge is a soft row of lower half blocks in the strip background instead of a rule line. The generated claude and pig themes set border and rule tokens to the surface tone. Agent identity stays in the name chip and ctx label. The frame default and the soft edge already shipped; the themed borders are ticket 08.
- **Styled strip.** After the borderless look, the strip is restyled as colored segments in the manner of omp's status line: icon per segment, symbol preset (`nerd`, `unicode` default, `ascii`), separator choice, rounded caps in `nerd`, segment colors from named palette tokens. Default stays `unicode` so a terminal with no Nerd Font is correct. Ticket 10.
- **Padding.** The agent screen and the strip are inset by default two columns left and right, and one empty row sits below the strip. Padding is background only and settings change or zero it. Ticket 12.
- **Theme.** One palette source drives the strip and the agents. Catppuccin Mocha for dark, Latte for light, the strip's table. Wick launches claude with a generated custom theme and pig with a generated theme file, both per run, never touching the user's saved selection. The wrapper also sets the pane's default foreground and background, cursor and 16 ANSI colors with OSC 4, 10, 11 and 12 at start and resets them on exit. Verified live in herdr: the pane background and the agents' colors follow. Not verified: Latte, and what other terminals do.

## User Stories

1. As a user, I want to type my prompt in a wick input row, so that nothing wick sends can mangle what I am typing.
2. As a user, I want wick prompts held while I have a draft, so that my text and wick's text never interleave.
3. As a user, I want a draft that sits for 10 seconds to be cleared before wick delivers, so that a forgotten draft never blocks a compress.
4. As a user, I want to see how many prompts are pending in the strip, so that I know wick is waiting on me.
5. As a user, I want my own message to go before pending wick prompts, so that it is answered on the latest context.
6. As a user, I want Enter to steer a running agent at the next tool boundary, so that I can correct it quickly.
7. As a user, I want Alt+Enter to queue a message until the turn ends, so that I do not interrupt a running task.
8. As a user, I want to type `/compress` in the row, so that I compress the session with no plugin or key binding.
9. As a user, I want to define my own macros in settings, so that I can send long prompts by name.
10. As a user, I want macro arguments, so that one macro serves many cases.
11. As a user, I want unknown `/x` text sent as a normal message, so that I can type text that starts with a slash.
12. As a user, I want a warning when a macro name collides with a built-in, so that I do not shadow one by accident.
13. As a user, I want a key to switch to raw mode, so that I can use the agent's own dialogs and menus.
14. As a user, I want the strip to show I am in raw mode, so that I know where my keys go.
15. As a user, I want wick prompts held in raw mode while my box is dirty, so that raw typing is never mangled.
16. As a user, I want raw mode with a stale dirty box to be cleared after 10 seconds, so that the queue does not stall forever.
17. As a user, I want the row to support multiline and paste, so that I can write real prompts.
18. As a user, I want Up to recall my row history, so that I can resend a prompt.
19. As a user, I want Ctrl+C and Esc in the row to interrupt the agent, so that I can stop it as before.
20. As a user, I want my draft and the queue to survive a compress restart, so that I lose nothing.
21. As a user, I want the compress handoff to go first after a restart, so that the new session starts with its context.
22. As a user, I want claude, pig and the pane to share the strip's colors, so that wick looks like one tool.
23. As a user, I want my saved claude and pig theme choice left alone, so that wick never rewrites my config.
24. As a user, I want the pane colors restored when the agent exits, so that my terminal is not left recolored.
25. As a maintainer, I want one control channel into the wrapper, so that every sender goes through one queue.
26. As a maintainer, I want one palette definition, so that the strip, claude, pig and the pane cannot drift.
27. As a maintainer, I want per-harness input keys in one table, so that adding a harness is one entry.
28. As a maintainer, I want agents with no human typing to share the queue with no row, so that workers use the same delivery code.
29. As a maintainer, I want a fallback to the herdr delivery path when no wrapper runs, so that nothing breaks during rollout.
30. As a user, I want areas separated by tint instead of drawn lines, so that the pane looks calm and uniform.
31. As a user, I want the agent's own input box and rules dimmed to the surface tone, so that they stop competing with the strip.
32. As a user, I want the strip to show icons and colored segments, so that I read its stats at a glance.
33. As a user, I want to choose the symbol preset and separator, so that the strip looks right in my terminal and font.
35. As a user, I want padding on the left, right and bottom of the pane, so that text does not touch the edge.
34. As a user, I want rounded caps and a clean look with a Nerd Font and a safe plain look without one, so that I never see broken glyphs.

## Implementation Decisions

- **Wrapper input stage.** Extends the header-strip wrapper. It reads the user's terminal input, feeds the row editor in normal mode, and forwards bytes untouched in raw mode. It draws the row between the child's screen and the strip. The reserved rows grow by the row height, and the child PTY is sized to the rest.
- **Queue.** One queue inside the wrapper holds items: user steer, user queued, and wick injections, each with a kind and a source. It delivers when no hold rule applies. Ordering: user message first, then injections in arrival order, with the restart handoff pinned ahead of all.
- **Delivery stage.** The wrapper is the sole writer to the child PTY master. It writes whole messages, using bracketed paste where the harness accepts it, followed by Enter. Steer waits for the transcript's tool boundary signal; queue waits for the turn to end. Both reuse existing signals (outstanding tool calls, turn state).
- **Acceptance.** An item counts as delivered when the transcript shows the user turn, not when bytes were written. A startup dialog that swallows the text is detected as not accepted, and the item stays at the head of the queue.
- **Control channel.** A unix socket per wrapper, named by agent identity. The shared delivery helper tries it first and falls back to the existing herdr path. Messages: enqueue (steer or queue), run macro, pause and resume, query pending.
- **Macro layer.** A registry in settings: name, kind (prompt template or built-in action), template. Built-ins ship with wick. `/compress` calls the supervisor's compress request. Validation runs on settings load and rejects duplicates, and warns on shadowed built-ins.
- **Harness table.** Per harness: clear sequence, newline sequence for raw handoff, interrupt keys, accepts-bracketed-paste. Entries for claude and pig from the probes in this spec. A harness without an entry gets the conservative default: hold injections while raw and dirty, never clear.
- **Raw mode.** A toggle key. Dirty flag as in the decisions. The child's own queue keys (claude Enter and Ctrl+Enter, pig Alt+Enter and Alt+Up) apply only here and are not used by wick's own delivery.
- **Palette.** A single palette definition keyed light and dark, used by the strip, a claude theme generator, a pig theme generator and the OSC palette setter. Generated theme files are written under wick's state dir. Claude loads its theme through the launch settings JSON; pig through its theme-file and use-theme flags. Only the launch changes; user settings are untouched.
- **Pane colors.** The wrapper emits OSC 4 (16 ANSI), 10, 11 and 12 on start and OSC 104, 110, 111 and 112 on exit, including on supervisor restarts and signals.
- **Seam.** The highest seam is the wrapper's control channel plus a fake child PTY. Tests drive the channel and key input, and assert the exact bytes the child PTY receives and the strip state.

## Testing Decisions

- Test external behavior only: bytes written to a fake child PTY, strip state text, queue order. Never assert internal fields.
- Typed draft then enqueue: nothing reaches the PTY until the draft is submitted or the 10 s stale clear fires (inject a fake clock). After the clear, the PTY receives the clear sequence (row mode: none) then the message.
- Order: pending injection plus a user submit yields user first. Restart plus pending items yields handoff first and the draft intact.
- Steer versus queue: steer delivers at the fake tool boundary, queue at the fake turn end.
- Raw mode: dirty flag set by any non-Enter byte, reset by Enter and Ctrl+C. Held while dirty, released after the stale clear.
- Macros: template expansion with args, unknown `/x` sent as text, collision warning at settings load.
- Fallback: with no control socket the helper takes the herdr path, asserted through the existing deliver and nudge test seams.
- Palette: generator tests compare generated theme files and the OSC byte sequences to fixtures from the palette table. Exit resets are asserted on normal exit and signal.
- Prior art: the tell and nudge tests, the supervisor tests with fake pane runners, and the transcript turn-state tests.
- Live probe (script under `scripts/`, not part of the check): run claude and pig under the wrapper and confirm the row, clear sequence, raw mode and OSC colors, extending the header-strip spike.

## Out of Scope

- `@` file completion and vim mode in the row.
- Macro sequences and conditional macros.
- A row for workers, reviewers and scouts.
- Detecting the child's own dialogs. Raw mode is the manual escape, and wick-launched agents have tools restricted already.
- Light palette verification and non-herdr terminals.
- Replacing herdr's own prompt command for third parties.

## Further Notes

- Dependency: the header-strip ticket (04) owns the PTY wrapper. This spec cannot start before that spike confirms claude and pig render under the wrapper. If the spike falls back to the virtual-terminal compositor, the input row draws through it.
- The herdr plugin key ticket (08) can shrink: `/compress` in the row replaces the key. Keep it as a fallback until this lands.
- Probe facts used here: claude Ctrl+U clears a line per press, needs three presses for a three-line draft; both harnesses cleared fully with six. Claude Alt+Enter and pig Shift+Enter insert a newline. Claude's first start can show a fullscreen-renderer dialog that swallows typed text.
- Context-compress spec line 25 says "No slash commands". This spec supersedes it for wick-owned macros only.
