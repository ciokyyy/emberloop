# 02: The user types in a emberloop input row, not the child's box

**What to build:** For the user's top-level session, the wrapper draws an input row between the child's screen and the strip and takes the user's typing there. Enter submits as a steer, Alt+Enter queues, Shift+Enter inserts a newline, paste works as one edit, Up recalls row history, Ctrl+C or Esc forward an interrupt to the child. Emberloop injections are held while the row holds a draft. After 10 seconds with no keystroke the row is cleared and the held items are delivered. The user's own message always goes before pending injections. The child PTY is sized to the screen minus the strip and the row.

**Blocked by:** 01-control-channel-and-queue-core

**Status:** done

- [x] Typing in the row never reaches the child until submit
- [x] A queued emberloop injection is not delivered while the row has a draft
- [x] After 10 s of no keystrokes the row clears and the injection is delivered
- [x] A user submit goes before pending injections
- [x] Enter steers at the tool boundary; Alt+Enter delivers at turn end
- [x] Multiline, paste, history and interrupt keys behave as described
- [x] Resize keeps the row and strip and forwards the reduced height to the child
- [x] Tests with a fake child PTY and fake clock assert bytes written and row and strip text
- [x] Spec stories 1, 2, 3, 5, 6, 7, 17, 18, 19

Spec: `.scratch/input-queue/spec.md`. Read it for the decisions this ticket implements and its Out of Scope section.
