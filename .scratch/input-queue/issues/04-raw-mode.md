# 04: Raw mode hands the keyboard to the agent, with injections held

**What to build:** A toggle key switches between the emberloop row and raw passthrough, so the user can reach an agent's own dialogs and menus. The strip shows `raw` and `raw, N queued`. In raw mode the wrapper keeps a dirty flag: any forwarded byte except a bare Enter sets it, and Enter, Ctrl+C or a emberloop clear resets it. Injections are held while raw mode is on and the box is dirty. After 10 seconds with no keystroke, emberloop sends six Ctrl+U to the child, then delivers. A per-harness table holds the clear, newline and interrupt sequences for claude and pig; a harness with no entry never gets a clear and holds while dirty.

**Blocked by:** 02-emberloop-input-row

**Status:** done

- [x] The toggle key switches modes and the strip shows the mode
- [x] Typed text in raw mode sets dirty; Enter and Ctrl+C reset it
- [x] Injections are held while dirty and delivered right away when not dirty
- [x] After 10 s stale, six Ctrl+U are sent, then the message
- [x] Ctrl+C is never used as the clear sequence
- [x] The harness table has entries for claude and pig, and a safe default for any other
- [x] Tests with a fake child PTY and fake clock assert dirty transitions and the exact bytes sent
- [x] Spec stories 13, 14, 15, 16, 27

Spec: `.scratch/input-queue/spec.md`. Read it for the decisions this ticket implements and its Out of Scope section.
