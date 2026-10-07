# 15: The input row decodes real terminal key input and shows its cursor

**What to build:** Replace the hand-matched byte strings in the input row (`wick-pig/inputrow.go`, `handleInput`) with a real terminal input decoder, and draw a visible cursor in the row. The decoder turns raw bytes into typed events: key (with modifiers), bracketed paste, focus in, focus out. The row acts on those events, never on raw byte strings.

**Blocked by:** 02-wick-input-row

**Status:** open

## Observed defects (user report, kitty-protocol terminal)

- Ctrl+Backspace inserts the literal text `[127;5u` into the row. The terminal sends CSI-u (kitty keyboard protocol): key code 127, modifier 5 (ctrl). The row only knows a few CSI-u forms (Enter variants) and treats the rest as text. Ctrl+Backspace should delete the previous word.
- Alt+Tab (focus leaving and returning to the terminal) inserts `[O` and `[I` into the row. These are focus-out and focus-in reports (CSI O, CSI I, DEC mode 1004). The row has no handler and types them as text. Focus events must be consumed, never inserted.
- The row shows no cursor. While editing in the middle of a draft, the user cannot see the insert position. Newly typed text appears to land at the end. The row already tracks `cursor` (`inputRow.cursor`); the paint path does not draw it.
- Other CSI-u keys (arrows with modifiers, Ctrl+Left/Right, Home/End, Delete, Ctrl+W, Alt+Backspace) likely leak the same way. Audit them.

## Library research

Candidates, all Go:

- `github.com/charmbracelet/ultraviolet` (`uv` package, and the older `github.com/charmbracelet/x/input` it grew out of). Decodes legacy xterm, kitty keyboard protocol, SGR mouse, bracketed paste, focus, resize. Same org and same `x/ansi` base that wick-pig already pulls in through lipgloss, so the dependency cost is low. First choice.
- `github.com/unxed/vtinput`: decodes keys, SGR mouse, bracketed paste, focus tracking. Smaller project.
- `github.com/charmbracelet/bubbletea` v2: full decoder, but it owns the event loop. `wick-tui` already uses it in its own module; wick-pig must stay a plain byte pipe, so use only the decoder.

Decide between them in the first step. Check: license, maintenance, ability to feed it a byte slice from the existing stdin read loop with no event loop, incomplete-sequence handling (a sequence split across two reads), and ESC-alone timing.

## Acceptance criteria

- [ ] A decoder library is chosen and the choice is recorded in this ticket
- [ ] Ctrl+Backspace deletes the previous word in the row; nothing is inserted
- [ ] Focus in and focus out reports never reach the row buffer
- [ ] Focus reports do not reach the child either, unless the child asked for them (check what wick forwards today and keep the child correct)
- [ ] The row draws a visible cursor at `inputRow.cursor`, inside multi-line drafts too, and typing inserts at the cursor
- [ ] Left, Right, Home, End, Delete, Ctrl+Left, Ctrl+Right, Ctrl+W, Alt+Backspace work in plain and CSI-u encodings
- [ ] A key sequence split across two stdin reads decodes as one key
- [ ] Existing row keys keep working: Enter, Alt+Enter, Shift+Enter, bracketed paste, Up history, Ctrl+C and Esc interrupt
- [ ] Raw mode still forwards bytes to the child untouched (decoder is used for the wick row only)
- [ ] Tests with the fake child PTY: feed the exact byte sequences `\x1b[127;5u`, `\x1b[O`, `\x1b[I`, assert row text, cursor and bytes written to the PTY

Spec: `.scratch/input-queue/spec.md`. Read it for the row key decisions (Enter, Alt+Enter, Shift+Enter, paste, history, interrupt) this ticket must keep.
