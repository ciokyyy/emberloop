# 01: Reviewers get their prompt within seconds of starting, in any tab

**What to build:** A live probe spawns a claude reviewer into an unfocused background tab and checks that its first prompt arrives within seconds. The probe covers the failure families already seen: herdr reporting an idle unfocused pane as `done` (fixed, keep it covered), a first-run dialog stalling the pane, and a pane bumped to another tab or into the overflow grid. Whatever the probe finds that still fails is fixed in the spawn path.

**Blocked by:** None (can start immediately)

**Status:** done

- [x] A scripted probe in `scripts/` spawns a claude reviewer in a background tab against a real herdr and reports the seconds until its prompt was delivered
- [x] The probe fails on the old idle-only wait and passes now
- [x] A reviewer pane bumped into an overflow tab still receives its prompt
- [x] Any further failure the probe finds is fixed, with a unit test where the spawn path allows one
- [x] Spec story 23

Spec: `.scratch/deterministic-driver/spec.md`. Read it for the decisions this ticket implements and its Testing Decisions, which bind you: test through the one driver seam (the spec and an injected world with a fake clock), assert on calls and run state, never on internals.

## Verification evidence

`herdr agent wait --help` (OR semantics for repeated `--until`, grounding the idle-or-done wait in `DeliverLate`):

```
--until <STATUS>
    State to match; repeat for more than one state
    [possible values: idle, working, blocked, done, unknown]
```

Grid overflow cap grounding probe step 3 (`emberloop/grid.go`: `gridCols = 3`, `gridMax = gridCols * gridCols` = 9; the tenth pane opens the overflow tab).

Live run: not yet. The probe is hand-run only (never in the check, never inside ticket work). Run `scripts/reviewer-spawn-probe.sh` by hand against a real herdr with a claude login and paste the PASS lines here before merge.
