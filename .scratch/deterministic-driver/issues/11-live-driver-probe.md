# 11: The driver is proven against a real herdr

**What to build:** A scripted probe runs the driver against a throwaway spec with a real herdr and prints each ticket's state. It is the proof step before the default flips from `--llm` to `--driver`, and it would have caught the idle/done wait bug that no unit test could.

**Blocked by:** 08 Resume

**Status:** done

- [x] A probe script in `scripts/` creates a throwaway repo and a two-ticket spec, runs the driver, and prints each ticket's state
- [x] The probe kills the driver mid-run once and checks that a second start resumes
- [x] It is never part of the automated check and never runs inside the check command
- [x] Spec stories 31, 42

Spec: `.scratch/deterministic-driver/spec.md`. Read it for the decisions this ticket implements and its Testing Decisions, which bind you: test through the one driver seam (the spec and an injected world with a fake clock), assert on calls and run state, never on internals.
