# 06: Claude, pig and the pane follow the strip's palette

**What to build:** One palette definition (Catppuccin Mocha and Latte, adaptive) drives the strip, a generated claude custom theme, a generated pig theme file, and the pane's own colors. El launches claude with the generated theme through its launch settings and pig with its theme-file and use-theme options, so the user's saved theme choice is never changed. At start the wrapper sets the pane's default foreground, background, cursor and 16 ANSI colors with OSC 4, 10, 11 and 12, and resets them on exit, on signals and across supervisor restarts. A probe in the spec's `theme/` directory shows the target look. Mocha is live-verified in herdr; Latte is not.

**Blocked by:** None (can start immediately)

**Status:** done

- [x] One palette source feeds the strip, both theme generators and the OSC setter
- [x] Generated claude and pig themes match the approved probe look; golden tests compare them to fixtures
- [x] Launch uses per-run flags only; saved claude and pig settings are untouched
- [x] OSC set at start and reset at exit, including on signal and restart, asserted on a fake terminal stream
- [x] A Latte variant exists and is generated from the same source
- [x] Spec stories 22, 23, 24, 26

Spec: `.scratch/archive/input-queue/spec.md`. Read it for the decisions this ticket implements and its Out of Scope section.
