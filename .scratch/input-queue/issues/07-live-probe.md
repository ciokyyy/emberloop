# 07: Live probe proves the row, raw mode and colors with real agents

**What to build:** A probe script in the scripts area, outside the repo check, runs claude and pig under the wrapper and confirms the wick row, steer and queue delivery, the stale clear, raw mode with its dirty flag, the OSC pane colors and the claude and pig themes. It also covers a startup dialog that swallows typed text. It extends the header-strip spike script and records its findings in this ticket. It uses isolated terminals and never touches running agents.

**Blocked by:** 02-wick-input-row, 04-raw-mode, 06-shared-theme

**Status:** ready-for-agent

- [ ] Probe runs claude and pig and prints pass or fail per behavior
- [ ] Startup dialog case is covered and recorded
- [ ] The script never submits a prompt to a model unless a flag says so
- [ ] Findings are recorded in this ticket
- [ ] Spec Testing Decisions, live probe paragraph

Spec: `.scratch/input-queue/spec.md`. Read it for the decisions this ticket implements and its Out of Scope section.

## Live-probe result (implement/07-live-probe)

Probe scripts written; herdr-pane visual pass still manual
(`scripts/input-queue-probe.sh` with `scripts/probe-ctrl.sh`; scripts
never run inside ticket work).

- Automated part: generated `wick-mocha` themes match the fixtures
  (`wick-pig/testdata/theme/claude-mocha.json`, `pig-mocha.json`); the
  script prints PASS/SKIP per file and exits 1 on a mismatch.
- Safety: the default run sends nothing to a model (no row submit, no
  injection, no Enter with text in a live box; the fake-child smoke
  submits only to the fake). Checks 14-22, which deliver text to a
  model, run only with `--submit`. `--submit` parses in any position.
- Injections go through `scripts/probe-ctrl.sh` (socket
  `~/.pig/state/wick/agents/ctrl-<id>.sock`, ops `pending`/`enqueue`
  with kinds `steer`/`queue`, lockName sanitizing plus the >100-byte
  FNV-1a fallback, all matching `wick-pig/inputqueue.go`).
- Out of scope for this probe: macros (ticket 03) and restart survival
  (ticket 05) are still open, so no macro or compress-restart step is
  included; the startup-dialog case needs `--submit` because the
  swallowed injection is redelivered to the model.
- Manual remainder: run `scripts/input-queue-probe.sh <pig-pane>
  <claude-pane>` for the dry checks (fake smoke, row hold, submit-free
  keys, resize, theme, OSC set/reset incl. signal, raw toggle/dirty),
  rerun with `--submit` for order, stale delivery, steer/queue and the
  startup-dialog swallow case, then check the boxes above.
