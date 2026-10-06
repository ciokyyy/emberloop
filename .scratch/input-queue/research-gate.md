# Research: can wick gate injected prompts against user typing?

Question: with a wick-owned input row (option B), can wick reliably keep injected prompts from mangling with what the user is typing, including in raw passthrough mode?

## Findings

1. **Wick cannot read the child's input buffer.** Claude Code exposes no hook or API for draft text. UserPromptSubmit only receives the submitted prompt (https://code.claude.com/docs/en/hooks, "UserPromptSubmit input"). The Notification `idle_prompt` fires about 60 seconds after a reply, so it is no typing signal. No such repo signal exists either (scout: no partial-input detection anywhere in `wick-pig`).
2. **Claude's input editing is rich and stateful.** Ctrl+A/E/K/U/W/Y, Alt+B/F/D, undo, history, vim mode, multiline, `!` shell mode, paste handling (https://code.claude.com/docs/en/interactive-mode, "Text editing"). Shadowing it from raw keystrokes (a "dirty" flag) is only a conservative guess. Ctrl+U on an empty line clears nothing, and history recall fills the buffer with no keystroke that looks like text.
3. **Claude already queues natively.** Enter mid-turn queues the message. It goes out after the running tool calls finish, in typed order. Ctrl+Enter sends queued messages now, with the draft behind them (interactive-mode, "Queue messages while Claude works", v2.1.275+). Under B wick owns this behaviour, so it must not rely on the child queue.
4. **Today's injection path would collide with B.** `DeliverPrompt` (`wick-pig/transcript.go:106-128`) uses `herdr agent prompt` or `pane send-text` plus `send-keys Enter`. Those write to the pane's terminal input, which under the wrapper is the wrapper's stdin. Wick text and user keystrokes then arrive on one indistinguishable stream.
5. **The wrapper does not exist yet.** No PTY wrapper, no spike result (ticket 04 unchecked).

## Verdict

Gating against an unknown child buffer is not reliable. Avoiding the problem by design is reliable:

- **Rule 1: the child's input box is empty by construction.** In normal mode the user types only in the wick row. The wrapper writes whole messages to the child PTY master.
- **Rule 2: wick injection never goes through herdr send-text.** Under B the wrapper owns a control channel (unix socket). Compress, reviewer follow-ups, steering and macros all send to it. The wrapper holds the queue and is the only writer of the PTY master. `DeliverPrompt` gets a wrapper-socket path, and herdr stays the fallback only when no wrapper runs (workers without a widget).
- **Rule 3: raw mode holds all injections.** While raw passthrough is on, the queue pauses. The strip shows `raw, N queued`.
- **Rule 4: raw exit may leave a draft in the child's box.** Needs a per-harness clear (claude: Ctrl+U, repeated for multiline) or a screen-grid read. The spec's fallback virtual-terminal compositor would give the grid read. Open decision.

## Open items for the spec

- How the wrapper detects when the child accepted a message (turn state from the transcript, existing `TurnState`).
- Raw-exit draft handling per harness (rule 4).
- Steer semantics: steer is the default, Alt+Enter queues (user decision). Steer maps to delivery at the next tool boundary (`WaitForToolBoundary`), queue to delivery after the turn ends.
